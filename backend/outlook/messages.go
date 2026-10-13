package outlook

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/label"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/sarielhp/mail_cli/cache"
	"github.com/sarielhp/mail_cli/cache/folderindex"
	"github.com/sarielhp/mail_cli/mailclient"
)

type graphMessage struct {
	ID               string    `json:"id"`
	IsRead           bool      `json:"isRead"`
	ReceivedDateTime time.Time `json:"receivedDateTime"`
}

// graphThrottleRetries is how many times a throttled download waits and tries
// again. Graph throttling is transient and it tells us how long to wait, so
// giving up on the first 429 discards a message the server was willing to send.
const graphThrottleRetries = 3
const maxMessageDownloadBytes = 50 * 1024 * 1024 // 50MB ceiling

func (c *client) downloadRawMessage(messageID email.ID) ([]byte, error) {
	return c.downloadRawMessageContext(context.Background(), messageID)
}

func (c *client) downloadRawMessageContext(ctx context.Context, messageID email.ID) ([]byte, error) {
	urlStr := fmt.Sprintf("https://graph.microsoft.com/v1.0/me/messages/%s/$value", messageID)

	var lastStatus int
	var lastBody string
	for attempt := range graphThrottleRetries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, "GET", urlStr, nil)
		if err != nil {
			return nil, err
		}
		resp, err := c.client.Do(req)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode == http.StatusOK {
			lr := io.LimitReader(resp.Body, maxMessageDownloadBytes+1)
			data, readErr := io.ReadAll(lr)
			_ = resp.Body.Close()
			if readErr != nil {
				return nil, readErr
			}
			if int64(len(data)) > maxMessageDownloadBytes {
				return nil, fmt.Errorf("message download exceeded maximum allowed size (%d bytes)", maxMessageDownloadBytes)
			}
			return data, nil
		}

		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		lastStatus, lastBody = resp.StatusCode, strings.TrimSpace(string(body))
		retryAfter, throttled := graphRetryAfter(resp)
		_ = resp.Body.Close()

		if !throttled || attempt == graphThrottleRetries-1 {
			break
		}
		slog.Debug("throttled by Graph, waiting before retry",
			slog.String("msgID", string(messageID)), slog.Duration("wait", retryAfter))
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(retryAfter):
		}
	}

	return nil, fmt.Errorf("failed to download raw message %s, status %d: %s",
		messageID, lastStatus, lastBody)
}

// graphRetryAfter reports how long to wait before retrying, and whether the
// response was a throttle at all. Graph sends Retry-After with a 429; without
// it, a short default still spreads the retries out.
func graphRetryAfter(resp *http.Response) (time.Duration, bool) {
	if resp.StatusCode != http.StatusTooManyRequests && resp.StatusCode != http.StatusServiceUnavailable {
		return 0, false
	}
	if secs, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && secs >= 0 {
		return time.Duration(secs) * time.Second, true
	}
	return 2 * time.Second, true
}

func (c *client) fetchFolderMessages(folderID string, limit int) ([]graphMessage, error) {
	urlStr := fmt.Sprintf("https://graph.microsoft.com/v1.0/me/mailFolders/%s/messages?$top=%d&$select=id,isRead,receivedDateTime", folderID, limit)
	resp, err := c.client.Get(urlStr)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b := readErrorBody(resp.Body)
		return nil, fmt.Errorf("fetching messages failed: status %d: %s", resp.StatusCode, b)
	}

	var res struct {
		Value []graphMessage `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}
	return res.Value, nil
}

func (c *client) FetchAndDownloadEmails(folder label.Folder, _ string) ([]email.ID, error) {
	folderName := folder.String()
	if err := c.init(); err != nil {
		return nil, err
	}

	folderID, err := c.getFolderID(folderName)
	if err != nil {
		return nil, err
	}

	limit := c.config.Limit
	if limit <= 0 {
		limit = 100
	}

	slog.Info("FetchAndDownloadEmails: querying mailbox", slog.String("folder", folderName), slog.String("folderID", folderID))
	messages, err := c.fetchFolderMessages(folderID, limit)
	if err != nil {
		return nil, err
	}
	if len(messages) == 0 {
		if err := folderindex.ReplaceAll(c.config.AccountCache, folderName, []email.ID{}); err != nil {
			return nil, err
		}
		return nil, nil
	}

	cacheDir := c.config.AccountCache
	ids := make([]email.ID, len(messages))
	for i, msgItem := range messages {
		ids[i] = email.ID(msgItem.ID)
	}
	if err := cache.UpdateReadState(cacheDir, func(state map[email.ID]bool) {
		for _, msgItem := range messages {
			state[email.ID(msgItem.ID)] = msgItem.IsRead
		}
	}); err != nil {
		slog.Error("Failed to save read state after Outlook sync", slog.Any("error", err))
	}

	_ = os.MkdirAll(filepath.Join(cacheDir.Path(), "messages"), 0700)
	slog.Info("Refreshing Outlook email cache", slog.Int("count", len(ids)), slog.String("folder", folderName))
	if err := c.newMessageStore().Prefetch(ids); err != nil {
		slog.Warn("some messages in folder could not be downloaded",
			slog.String("folder", folderName), slog.Any("error", err))
	}

	if err := folderindex.ReplaceAll(cacheDir, folderName, ids); err != nil {
		slog.Error("FetchAndDownloadEmails: ReplaceAll failed", slog.String("folder", folderName), slog.Any("error", err))
	}

	return ids, nil
}

func (c *client) postGraphWithRetry(urlStr string, bodyBytes []byte) (*http.Response, error) {
	for attempt := range graphThrottleRetries {
		req, err := http.NewRequest(http.MethodPost, urlStr, bytes.NewReader(bodyBytes))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := c.client.Do(req)
		if err != nil {
			return nil, err
		}
		retryAfter, throttled := graphRetryAfter(resp)
		if !throttled || attempt == graphThrottleRetries-1 {
			return resp, nil
		}
		_ = resp.Body.Close()
		slog.Debug("throttled by Graph, waiting before retry",
			slog.String("url", urlStr), slog.Duration("wait", retryAfter))
		time.Sleep(retryAfter)
	}
	return nil, fmt.Errorf("post to %s failed after %d retries", urlStr, graphThrottleRetries)
}

func (c *client) moveSingleEmail(messageID email.ID, destFolderID string) (email.ID, error) {
	bodyBytes, _ := json.Marshal(map[string]string{
		"destinationId": destFolderID,
	})
	urlStr := fmt.Sprintf("https://graph.microsoft.com/v1.0/me/messages/%s/move", messageID)
	resp, err := c.postGraphWithRetry(urlStr, bodyBytes)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		b := readErrorBody(resp.Body)
		return "", fmt.Errorf("move failed: status %d: %s", resp.StatusCode, b)
	}
	var res struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err == nil && res.ID != "" {
		return email.ID(res.ID), nil
	}
	return messageID, nil
}

// DeleteMessages permanently deletes messages with Graph's permanentDelete,
// so they do not land in Deleted Items or Recoverable Items.
func (c *client) DeleteMessages(messageIDs []email.ID, _ label.Folder) error {
	if err := c.init(); err != nil {
		return err
	}
	for _, id := range messageIDs {
		urlStr := fmt.Sprintf("https://graph.microsoft.com/v1.0/me/messages/%s/permanentDelete", id)
		resp, err := c.postGraphWithRetry(urlStr, nil)
		if err != nil {
			return err
		}
		ok := resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusOK
		detail := ""
		if !ok {
			detail = readErrorBody(resp.Body)
		}
		_ = resp.Body.Close()
		if !ok {
			return fmt.Errorf("permanent delete failed: status %d: %s", resp.StatusCode, detail)
		}
	}
	return nil
}

func (c *client) MoveEmail(messageIDs []email.ID, _ label.Folder, dest label.Folder) error {
	destLabelName := dest.String()
	if err := c.init(); err != nil {
		return err
	}
	destFolderID, err := c.getFolderID(destLabelName)
	if err != nil {
		if ensureErr := c.EnsureLabelExists(dest); ensureErr != nil {
			return ensureErr
		}
		destFolderID, err = c.getFolderID(destLabelName)
		if err != nil {
			return err
		}
	}
	var failed []email.ID
	var firstErr error
	for _, id := range messageIDs {
		if _, err := c.moveSingleEmail(id, destFolderID); err != nil {
			failed = append(failed, id)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	if firstErr != nil {
		return &mailclient.PartialMoveError{Failed: failed, Err: firstErr}
	}
	return nil
}

func (c *client) copySingleEmail(messageID email.ID, destFolderID string) error {
	bodyBytes, _ := json.Marshal(map[string]string{
		"destinationId": destFolderID,
	})
	urlStr := fmt.Sprintf("https://graph.microsoft.com/v1.0/me/messages/%s/copy", messageID)
	resp, err := c.postGraphWithRetry(urlStr, bodyBytes)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		b := readErrorBody(resp.Body)
		return fmt.Errorf("copy failed: status %d: %s", resp.StatusCode, b)
	}
	return nil
}

func (c *client) CopyEmail(messageIDs []email.ID, _ label.Folder, dest label.Folder) error {
	destLabelName := dest.String()
	if err := c.init(); err != nil {
		return err
	}
	destFolderID, err := c.getFolderID(destLabelName)
	if err != nil {
		if ensureErr := c.EnsureLabelExists(dest); ensureErr != nil {
			return ensureErr
		}
		destFolderID, err = c.getFolderID(destLabelName)
		if err != nil {
			return err
		}
	}
	var failed []email.ID
	var firstErr error
	for _, id := range messageIDs {
		if err := c.copySingleEmail(id, destFolderID); err != nil {
			failed = append(failed, id)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	if firstErr != nil {
		return &mailclient.PartialMoveError{Failed: failed, Err: firstErr}
	}
	return nil
}

func (c *client) MoveToInbox(messageIDs []email.ID, source label.Folder) error {
	return c.MoveEmail(messageIDs, source, label.FromServer("Inbox"))
}

func (c *client) MoveAllSpam(dest label.Folder) error {
	destLabel := dest.String()
	if err := c.init(); err != nil {
		return err
	}
	spamFolder := c.account.SpamFolder
	if spamFolder == "" {
		spamFolder = "Junk Email"
	}
	// Never escalate a move into a permanent purge. This was previously silent.
	if strings.EqualFold(spamFolder, destLabel) {
		return fmt.Errorf("spam_learn is set to %q, which is the spam folder itself; refusing to silently purge it. Run 'mail_cli spam del' to delete deliberately, or set spam_learn to a different folder", destLabel)
	}

	moved, err := c.eachSpamBatch(spamFolder, func(ids []email.ID) (int, error) {
		if err := c.MoveEmail(ids, label.FromServer(spamFolder), dest); err != nil {
			return 0, err
		}
		return len(ids), nil
	})
	if err != nil {
		return err
	}
	if moved == 0 {
		c.rep.Infof("Outlook Junk folder is empty.\n")
	}
	return nil
}

func (c *client) DeleteAllSpam() error {
	if err := c.init(); err != nil {
		return err
	}
	spamFolder := c.account.SpamFolder
	if spamFolder == "" {
		spamFolder = "Junk Email"
	}
	destroyedCount, err := c.eachSpamBatch(spamFolder, func(ids []email.ID) (int, error) {
		return c.destroyBatch(ids), nil
	})
	if err != nil {
		return err
	}
	if destroyedCount == 0 {
		c.rep.Infof("Outlook Junk folder is already empty.\n")
		return nil
	}
	c.rep.Successf("Successfully purged %d spam emails permanently from Outlook server.\n", destroyedCount)
	return nil
}

// destroyBatch deletes each message and returns how many the server confirmed.
func (c *client) destroyBatch(ids []email.ID) int {
	c.rep.Infof("Permanently destroying %d message(s) in Outlook Junk folder...\n", len(ids))
	destroyed := 0
	for _, id := range ids {
		req, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("https://graph.microsoft.com/v1.0/me/messages/%s", id), nil)
		if err != nil {
			continue
		}
		resp, err := c.client.Do(req)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusOK {
				destroyed++
			}
		}
	}
	return destroyed
}

func (c *client) MarkAsRead(messageIDs []email.ID) error {
	if err := c.init(); err != nil {
		return err
	}
	for _, id := range messageIDs {
		bodyBytes, _ := json.Marshal(map[string]bool{
			"isRead": true,
		})
		req, err := http.NewRequest(http.MethodPatch, fmt.Sprintf("https://graph.microsoft.com/v1.0/me/messages/%s", id), bytes.NewBuffer(bodyBytes))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := c.client.Do(req)
		if err != nil {
			return err
		}
		if resp != nil {
			resp.Body.Close()
			if resp.StatusCode >= 400 {
				return fmt.Errorf("outlook mark as read returned status %d", resp.StatusCode)
			}
		}
	}
	return nil
}

func (c *client) UploadRawEmail(rawBytes []byte, target label.Folder) (email.ID, error) {
	targetLabel := target.String()
	if err := c.init(); err != nil {
		return "", err
	}

	targetFolderID, err := c.getFolderID(targetLabel)
	if err != nil {
		_ = c.EnsureLabelExists(target)
		targetFolderID, err = c.getFolderID(targetLabel)
		if err != nil {
			return "", err
		}
	}

	b64Content := base64.StdEncoding.EncodeToString(rawBytes)
	req, err := http.NewRequest(http.MethodPost, "https://graph.microsoft.com/v1.0/me/messages", strings.NewReader(b64Content))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "text/plain")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		b := readErrorBody(resp.Body)
		return "", fmt.Errorf("upload raw email returned status %d: %s", resp.StatusCode, b)
	}

	var res struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", err
	}

	return c.moveSingleEmail(email.ID(res.ID), targetFolderID)
}

func (c *client) SendEmail(rawBytes []byte) error {
	if err := c.init(); err != nil {
		return err
	}

	b64Content := base64.StdEncoding.EncodeToString(rawBytes)
	req, err := http.NewRequest(http.MethodPost, "https://graph.microsoft.com/v1.0/me/messages", strings.NewReader(b64Content))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "text/plain")

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		b := readErrorBody(resp.Body)
		return fmt.Errorf("create draft for sending returned status %d: %s", resp.StatusCode, b)
	}

	var res struct {
		ID string `json:"id"`
	}
	if decodeErr := json.NewDecoder(resp.Body).Decode(&res); decodeErr != nil {
		return decodeErr
	}

	sendURL := fmt.Sprintf("https://graph.microsoft.com/v1.0/me/messages/%s/send", res.ID)
	reqSend, err := http.NewRequest(http.MethodPost, sendURL, nil)
	if err != nil {
		return err
	}

	respSend, err := c.client.Do(reqSend)
	if err != nil {
		return err
	}
	defer respSend.Body.Close()

	if respSend.StatusCode != http.StatusAccepted && respSend.StatusCode != http.StatusOK {
		b := readErrorBody(respSend.Body)
		return fmt.Errorf("send draft returned status %d: %s", respSend.StatusCode, b)
	}

	return nil
}
