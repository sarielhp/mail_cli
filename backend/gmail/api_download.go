package gmail

import (
	"context"
	"errors"
	"fmt"
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"github.com/sarielhp/mail_cli/email"
	"log/slog"
	"math/rand/v2"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	gmailapi "google.golang.org/api/gmail/v1"
	"google.golang.org/api/googleapi"

	"github.com/sarielhp/mail_cli/cache"
	"github.com/sarielhp/mail_cli/cache/folderindex"
	"github.com/sarielhp/mail_cli/cache/msgstore"
	"github.com/sarielhp/mail_cli/label"
	"github.com/sarielhp/mail_cli/report"
)

const maxGmailListAttempts = 4

var gmailListRetryBaseDelay = 500 * time.Millisecond

func isRetryableGmailError(err error) bool {
	if err == nil {
		return false
	}
	var gErr *googleapi.Error
	if errors.As(err, &gErr) {
		return gErr.Code == http.StatusTooManyRequests || gErr.Code == http.StatusServiceUnavailable
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	return false
}

func executeGmailOpWithRetry[T any](ctx context.Context, opName string, do func() (T, error)) (T, error) {
	var zero T
	if ctx == nil {
		ctx = context.Background()
	}
	for attempt := 0; attempt < maxGmailListAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return zero, err
		}
		res, err := do()
		if err == nil {
			return res, nil
		}
		if !isRetryableGmailError(err) || attempt == maxGmailListAttempts-1 {
			return zero, err
		}
		wait := gmailListRetryBaseDelay * (1 << attempt)
		half := int64(wait) / 2
		var jitter time.Duration
		if half > 0 {
			jitter = time.Duration(rand.Int64N(half))
		}
		sleepDur := time.Duration(half) + jitter
		slog.Debug("Gmail API throttled, retrying",
			slog.String("op", opName),
			slog.Int("attempt", attempt+1),
			slog.Duration("wait", sleepDur),
			slog.Any("error", err))
		select {
		case <-ctx.Done():
			return zero, ctx.Err()
		case <-time.After(sleepDur):
		}
	}
	return zero, fmt.Errorf("exhausted retries")
}

func executeGmailCallWithRetry(ctx context.Context, do func() (*gmailapi.ListMessagesResponse, error)) (*gmailapi.ListMessagesResponse, error) {
	return executeGmailOpWithRetry(ctx, "list messages", do)
}

func executeListMessagesWithRetry(ctx context.Context, call *gmailapi.UsersMessagesListCall) (*gmailapi.ListMessagesResponse, error) {
	return executeGmailCallWithRetry(ctx, func() (*gmailapi.ListMessagesResponse, error) {
		return call.Do()
	})
}

func executeLabelGetWithRetry(ctx context.Context, srv *gmailapi.Service, labelID string) (*gmailapi.Label, error) {
	return executeGmailOpWithRetry(ctx, "get label", func() (*gmailapi.Label, error) {
		return srv.Users.Labels.Get("me", labelID).Context(ctx).Do()
	})
}

func listFolderMessageIDs(ctx context.Context, srv *gmailapi.Service, folderName, labelID string, limit int) ([]*gmailapi.Message, error) {
	var allMsgs []*gmailapi.Message
	pageToken := ""

	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		call := srv.Users.Messages.List("me").IncludeSpamTrash(true).Context(ctx)
		if labelID != "" {
			call = call.LabelIds(labelID)
		} else {
			call = call.Q(fmt.Sprintf("label:%q", folderName))
		}
		if pageToken != "" {
			call = call.PageToken(pageToken)
		}
		res, err := executeListMessagesWithRetry(ctx, call)
		if err != nil {
			return nil, fmt.Errorf("failed to list messages: %w", err)
		}

		allMsgs = append(allMsgs, res.Messages...)
		if len(allMsgs) >= limit || res.NextPageToken == "" {
			break
		}
		pageToken = res.NextPageToken
	}

	if len(allMsgs) > limit {
		allMsgs = allMsgs[:limit]
	}
	return allMsgs, nil
}

// collectFolderMessageIDs returns the folder's message IDs. Working out which
// of them are absent locally is the message store's job, not this function's.
func collectFolderMessageIDs(downloadDir string, allMsgs []*gmailapi.Message) ([]email.ID, error) {
	if err := os.MkdirAll(filepath.Join(downloadDir, "messages"), 0700); err != nil {
		return nil, fmt.Errorf("failed to create messages cache directory: %w", err)
	}
	allIDs := make([]email.ID, 0, len(allMsgs))
	for _, m := range allMsgs {
		allIDs = append(allIDs, email.ID(m.Id))
	}
	return allIDs, nil
}

func fetchFolderUnreadIDs(ctx context.Context, srv *gmailapi.Service, folderName, labelID string, limit int) ([]string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var unreadIDs []string
	unreadPageToken := ""
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		call := srv.Users.Messages.List("me").Q("is:unread").IncludeSpamTrash(true).Context(ctx)
		if labelID != "" {
			call = call.LabelIds(labelID)
		} else {
			call = call.Q(fmt.Sprintf("label:%q is:unread", folderName))
		}
		if unreadPageToken != "" {
			call = call.PageToken(unreadPageToken)
		}
		res, err := executeListMessagesWithRetry(ctx, call)
		if err != nil {
			return nil, err
		}
		for _, m := range res.Messages {
			unreadIDs = append(unreadIDs, m.Id)
		}
		if len(unreadIDs) >= limit || res.NextPageToken == "" {
			break
		}
		unreadPageToken = res.NextPageToken
	}
	return unreadIDs, nil
}

func syncFolderReadState(ctx context.Context, srv *gmailapi.Service, downloadDir cachedir.Account, folderName string, labelID string, allIDs []string, limit int) {
	unreadIDs, err := fetchFolderUnreadIDs(ctx, srv, folderName, labelID, limit)
	if err != nil {
		slog.Error("Failed to fetch unread message IDs from server", slog.Any("error", err))
		return
	}
	unreadSet := make(map[string]bool, len(unreadIDs))
	for _, id := range unreadIDs {
		unreadSet[id] = true
	}
	err = cache.UpdateReadState(downloadDir, func(state map[email.ID]bool) {
		for _, id := range allIDs {
			state[email.ID(id)] = !unreadSet[id]
		}
	})
	if err != nil {
		slog.Error("Failed to save read state after server sync", slog.Any("error", err))
	}
}

func fetchAndDownloadEmailsREST(rep report.Reporter, config *Config, folderName string, _ string) ([]email.ID, error) {
	ctx, cancel := context.WithTimeout(context.Background(), msgstore.DefaultFetchTimeout)
	defer cancel()

	srv, err := GetGmailServiceContext(ctx, rep, config)
	if err != nil {
		return nil, err
	}

	labelID, errLabel := resolveLabelIDByNameContext(ctx, srv, config, folderName)
	if errLabel != nil {
		slog.Warn("label ID resolution failed, falling back to query search",
			slog.String("folder", folderName), slog.Any("error", errLabel))
		labelID = ""
	}

	if config.Verbose {
		rep.Infof("Searching for messages in folder %q (labelID: %s)...\n", folderName, labelID)
	}

	allMsgs, err := listFolderMessageIDs(ctx, srv, folderName, labelID, config.Limit)
	if err != nil {
		return nil, err
	}
	if len(allMsgs) == 0 {
		if replaceErr := folderindex.ReplaceAll(config.AccountCache, folderName, []email.ID{}); replaceErr != nil {
			slog.Error("Failed to save folder index", slog.Any("error", replaceErr))
		}
		return nil, nil
	}

	allIDs, err := collectFolderMessageIDs(config.AccountCache.Path(), allMsgs)
	if err != nil {
		return nil, err
	}

	syncFolderReadState(ctx, srv, config.AccountCache, folderName, labelID, email.IDStrings(allIDs), config.Limit)

	// A message that will not download does not invalidate the folder listing:
	// the ID is still in the folder, and the UI renders an unreadable body as
	// such. Gmail used to abort the whole sync on the first failure, which let
	// one dead message block a scan of a thousand good ones.
	if err := newMessageStore(config, srv).Prefetch(allIDs); err != nil {
		slog.Warn("some messages in folder could not be downloaded",
			slog.String("folder", folderName), slog.Any("error", err))
	}

	if err := folderindex.ReplaceAll(config.AccountCache, folderName, allIDs); err != nil {
		slog.Error("Failed to save folder index", slog.Any("error", err))
	}

	return allIDs, nil
}

func ensureLabelHierarchyREST(rep report.Reporter, srv *gmailapi.Service, labelName string, labelNameToID map[string]string, labelIDToName map[string]string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), msgstore.DefaultFetchTimeout)
	defer cancel()
	return ensureLabelHierarchyRESTContext(ctx, rep, srv, labelName, labelNameToID, labelIDToName)
}

func ensureLabelHierarchyRESTContext(ctx context.Context, rep report.Reporter, srv *gmailapi.Service, labelName string, labelNameToID map[string]string, labelIDToName map[string]string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var parts []string
	for _, p := range strings.Split(labelName, "/") {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	if len(parts) == 0 {
		return "", fmt.Errorf("invalid empty label name: %q", labelName)
	}

	var currentPath []string
	var targetLabelID string

	for _, part := range parts {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		currentPath = append(currentPath, part)
		prefixLabel := strings.Join(currentPath, "/")
		lowerPrefix := strings.ToLower(prefixLabel)

		if id, ok := labelNameToID[lowerPrefix]; ok {
			targetLabelID = id
			if labelIDToName[id] != prefixLabel {
				prefixLabel = labelIDToName[id]
				currentPath = strings.Split(prefixLabel, "/")
			}
		} else {
			if err := label.ValidateNewName(prefixLabel); err != nil {
				return "", err
			}
			newL, err := executeGmailOpWithRetry(ctx, "create label", func() (*gmailapi.Label, error) {
				return srv.Users.Labels.Create("me", &gmailapi.Label{
					Name:                  prefixLabel,
					LabelListVisibility:   "labelShow",
					MessageListVisibility: "show",
				}).Context(ctx).Do()
			})
			if err != nil {
				return "", fmt.Errorf("failed to create label %q: %w", prefixLabel, err)
			}
			targetLabelID = newL.Id
			labelNameToID[lowerPrefix] = targetLabelID
			labelIDToName[targetLabelID] = prefixLabel
			rep.Successf("Created new Gmail label: %q\n", prefixLabel)
		}
	}

	return targetLabelID, nil
}

func rewriteRuleLabelCasing(rep report.Reporter, config *Config, labelToAdd string) string {
	if config == nil {
		slog.Warn("label casing rewrite: nil config, using original casing")
		return labelToAdd
	}

	ctx, cancel := context.WithTimeout(context.Background(), msgstore.DefaultFetchTimeout)
	defer cancel()

	srv, err := GetGmailServiceContext(ctx, rep, config)
	if err != nil {
		slog.Warn("label casing rewrite: service unavailable, using original casing", slog.Any("error", err))
		return labelToAdd
	}

	labelsRes, err := executeGmailOpWithRetry(ctx, "list labels", func() (*gmailapi.ListLabelsResponse, error) {
		return srv.Users.Labels.List("me").Context(ctx).Do()
	})
	if err != nil {
		slog.Warn("label casing rewrite: label list failed, using original casing", slog.Any("error", err))
		return labelToAdd
	}

	gmailLabels := make(map[string]string)
	for _, l := range labelsRes.Labels {
		gmailLabels[strings.ToLower(l.Name)] = l.Name
	}

	parts := strings.Split(labelToAdd, "/")
	rewritten := false
	for i := 1; i <= len(parts); i++ {
		prefix := strings.Join(parts[:i], "/")
		lowerPrefix := strings.ToLower(prefix)
		if exactName, ok := gmailLabels[lowerPrefix]; ok {
			exactParts := strings.Split(exactName, "/")
			for idx, val := range exactParts {
				if idx >= len(parts) {
					break
				}
				if parts[idx] != val {
					parts[idx] = val
					rewritten = true
				}
			}
		}
	}

	if rewritten {
		newLabel := strings.Join(parts, "/")
		if newLabel != labelToAdd {
			rep.Infof("Casing mismatch detected: Rewriting label %q to %q to match existing Gmail folders.\n", labelToAdd, newLabel)
			return newLabel
		}
	}

	return labelToAdd
}
