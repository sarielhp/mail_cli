package outlook

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sarielhp/mail_cli/cache"
	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/email"
)

type graphAccountMessage struct {
	ID               string    `json:"id"`
	ParentFolderID   string    `json:"parentFolderId"`
	IsRead           bool      `json:"isRead"`
	ReceivedDateTime time.Time `json:"receivedDateTime"`
}

func (c *client) buildOutlookFolderMap() (map[string]string, error) {
	items, _, err := c.fetchFoldersRecursive("", "")
	if err != nil {
		return nil, err
	}
	folderMap := make(map[string]string, len(items))
	for _, it := range items {
		folderMap[it.Name] = it.FullName
	}
	return folderMap, nil
}

func (c *client) fetchAccountMessages(limit int) ([]graphAccountMessage, error) {
	// The space in "receivedDateTime desc" must be percent-encoded. net/url
	// leaves RawQuery exactly as given, so a literal space reaches the wire and
	// Graph answers 400 for the whole request -- which is what it did, making
	// `last` and the account-wide query fail on every Outlook account.
	urlStr := fmt.Sprintf("https://graph.microsoft.com/v1.0/me/messages"+
		"?$top=%d&$orderby=receivedDateTime%%20desc"+
		"&$select=id,parentFolderId,isRead,receivedDateTime", limit)
	resp, err := c.client.Get(urlStr)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Graph explains a rejected query in the body; without it a 400 gives
		// nothing to act on.
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("fetching account messages failed: status %d: %s",
			resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var res struct {
		Value []graphAccountMessage `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}
	return res.Value, nil
}

// FetchLatestAccountEmails queries Microsoft Graph account-wide for the latest N emails.
func (c *client) FetchLatestAccountEmails(limit int) ([]cfg_acc.MessageFolderRef, error) {
	if err := c.init(); err != nil {
		return nil, err
	}

	if limit <= 0 {
		limit = 100
	}

	folderMap, err := c.buildOutlookFolderMap()
	if err != nil {
		return nil, err
	}

	messages, err := c.fetchAccountMessages(limit)
	if err != nil {
		return nil, err
	}

	cacheDir := c.config.AccountCache
	_ = os.MkdirAll(filepath.Join(cacheDir.Path(), "messages"), 0700)

	_ = cache.UpdateReadState(cacheDir, func(state map[email.ID]bool) {
		for _, m := range messages {
			state[email.ID(m.ID)] = m.IsRead
		}
	})

	ids := make([]email.ID, len(messages))
	for i, m := range messages {
		ids[i] = email.ID(m.ID)
	}
	// A download failure is logged rather than fatal: the caller wants the list
	// of recent messages, and one that could not be cached is still worth
	// listing.
	if err := c.newMessageStore().Prefetch(ids); err != nil {
		slog.Warn("some recent messages could not be downloaded", slog.Any("error", err))
	}

	refs := make([]cfg_acc.MessageFolderRef, len(messages))
	for i, m := range messages {
		folderName := "Inbox"
		if name, ok := folderMap[m.ParentFolderID]; ok && name != "" {
			folderName = name
		}
		refs[i] = cfg_acc.MessageFolderRef{
			MessageID: email.ID(m.ID),
			Folder:    folderName,
		}
	}

	return refs, nil
}
