package gmail

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/report"

	gmailapi "google.golang.org/api/gmail/v1"
)

var ignoredGmailLabels = map[string]bool{
	"UNREAD":    true,
	"IMPORTANT": true,
	"CHAT":      true,
	"STARRED":   true,
}

func fetchAccountLabelMapping(srv *gmailapi.Service) (map[string]string, error) {
	labelsRes, err := srv.Users.Labels.List("me").Do()
	if err != nil {
		return nil, fmt.Errorf("failed to list labels: %w", err)
	}
	idToName := make(map[string]string, len(labelsRes.Labels))
	for _, l := range labelsRes.Labels {
		idToName[l.Id] = l.Name
	}
	return idToName, nil
}

func fetchAccountMessageList(srv *gmailapi.Service, limit int) ([]*gmailapi.Message, error) {
	var allMsgs []*gmailapi.Message
	pageToken := ""
	for {
		call := srv.Users.Messages.List("me").IncludeSpamTrash(true).MaxResults(int64(limit))
		if pageToken != "" {
			call = call.PageToken(pageToken)
		}
		res, err := call.Do()
		if err != nil {
			return nil, fmt.Errorf("failed to list account messages: %w", err)
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

func resolveMessageFolder(srv *gmailapi.Service, msgID email.ID, idToName map[string]string) string {
	meta, err := srv.Users.Messages.Get("me", string(msgID)).Format("minimal").Do()
	if err != nil || len(meta.LabelIds) == 0 {
		return "INBOX"
	}
	var candidateFolders []string
	for _, lid := range meta.LabelIds {
		name := idToName[lid]
		if name == "" {
			name = lid
		}
		if ignoredGmailLabels[name] || strings.HasPrefix(name, "CATEGORY_") {
			continue
		}
		candidateFolders = append(candidateFolders, name)
	}

	for _, c := range candidateFolders {
		if !gmailSystemLabels[c] {
			return c
		}
	}
	if len(candidateFolders) > 0 {
		return candidateFolders[0]
	}
	return "INBOX"
}

// fetchLatestAccountEmailsREST fetches the latest N emails across the entire Gmail account in a single query.
func fetchLatestAccountEmailsREST(rep report.Reporter, config *Config, limit int) ([]cfg_acc.MessageFolderRef, error) {
	srv, err := GetGmailService(rep, config)
	if err != nil {
		return nil, err
	}

	if limit <= 0 {
		limit = 100
	}

	idToName, err := fetchAccountLabelMapping(srv)
	if err != nil {
		return nil, err
	}

	allMsgs, err := fetchAccountMessageList(srv, limit)
	if err != nil {
		return nil, err
	}
	if len(allMsgs) == 0 {
		return nil, nil
	}

	_ = os.MkdirAll(filepath.Join(config.AccountCache.Path(), "messages"), 0700)

	ids := make([]email.ID, 0, len(allMsgs))
	for _, m := range allMsgs {
		ids = append(ids, email.ID(m.Id))
	}
	// A download failure here is logged rather than fatal: the caller wants the
	// list of recent messages, and a message that could not be cached is still
	// worth listing.
	if err := newMessageStore(config, srv).Prefetch(ids); err != nil {
		slog.Warn("some recent messages could not be downloaded", slog.Any("error", err))
	}

	refs := make([]cfg_acc.MessageFolderRef, len(allMsgs))
	for i, m := range allMsgs {
		refs[i] = cfg_acc.MessageFolderRef{
			MessageID: email.ID(m.Id),
			Folder:    resolveMessageFolder(srv, email.ID(m.Id), idToName),
		}
	}
	return refs, nil
}
