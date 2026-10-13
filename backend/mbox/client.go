package mbox

import (
	"context"
	"errors"
	"fmt"
	"github.com/sarielhp/mail_cli/label"
	"os"
	"strings"

	"github.com/sarielhp/mail_cli/cache/folderindex"
	"github.com/sarielhp/mail_cli/cache/msg"
	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/mailclient"
)

type client struct {
	acc      cfg_acc.AccountConfig
	localCfg *cfg_g.Config
	mboxPath string
	dispName string
}

// NewClient returns a MailClient backed by the mbox file at mboxPath, using acc
// for account metadata and localCfg for cache and display settings.
func NewClient(acc cfg_acc.AccountConfig, localCfg *cfg_g.Config, mboxPath string) mailclient.MailClient {
	disp := acc.DisplayName
	if disp == "" {
		disp = acc.Name
	}
	return &client{
		acc:      acc,
		localCfg: localCfg,
		mboxPath: mboxPath,
		dispName: disp,
	}
}

func (c *client) Validate() error {
	fi, err := os.Stat(c.mboxPath)
	if err != nil {
		return fmt.Errorf("cannot access mbox %q: %w", c.mboxPath, err)
	}
	if fi.IsDir() {
		return fmt.Errorf("%q is a directory, not an mbox file", c.mboxPath)
	}
	return nil
}

func (c *client) BackendType() string {
	return "mbox"
}

func (c *client) Config() *cfg_g.Config {
	return c.localCfg
}

func (c *client) inboxName() string {
	return "INBOX"
}

func (c *client) GetMatchingLabels(_ string) ([]string, error) {
	return []string{"INBOX"}, nil
}

func (c *client) GetLabelItems() ([]cfg_acc.Item, error) {
	return []cfg_acc.Item{
		{Name: "INBOX", FullName: "INBOX"},
	}, nil
}

func (c *client) FetchAndDownloadEmails(_ label.Folder, _ string) ([]email.ID, error) {
	return IngestMbox(c.mboxPath, c.localCfg.AccountCache, c.dispName)
}

func (c *client) FetchLatestAccountEmails(limit int) ([]cfg_acc.MessageFolderRef, error) {
	ids, err := folderindex.IDs(c.localCfg.AccountCache, "INBOX")
	if err != nil {
		return nil, err
	}
	if limit > 0 && len(ids) > limit {
		ids = ids[:limit]
	}
	refs := make([]cfg_acc.MessageFolderRef, len(ids))
	for i, id := range ids {
		refs[i] = cfg_acc.MessageFolderRef{MessageID: id, Folder: "INBOX"}
	}
	return refs, nil
}

func (c *client) GetRawMessageContext(ctx context.Context, msgID email.ID) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return msg.Read(c.localCfg.AccountCache, msgID)
}

func (c *client) GetRawMessage(msgID email.ID) ([]byte, error) {
	return c.GetRawMessageContext(context.Background(), msgID)
}

// batchFetch retrieves each ID with fetch, returning the results in input order
// and joining every per-ID error. A cancelled context returns what was fetched
// so far with the cancellation.
func batchFetch[T any](ctx context.Context, msgIDs []email.ID, fetch func(context.Context, email.ID) (T, error)) ([]T, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	result := make([]T, len(msgIDs))
	var errs []error
	for i, id := range msgIDs {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		value, err := fetch(ctx, id)
		if err != nil {
			errs = append(errs, fmt.Errorf("id %s: %w", id, err))
			continue
		}
		result[i] = value
	}
	return result, errors.Join(errs...)
}

func (c *client) GetRawMessagesBatchContext(ctx context.Context, msgIDs []email.ID) ([][]byte, error) {
	return batchFetch(ctx, msgIDs, c.GetRawMessageContext)
}

func (c *client) GetRawMessagesBatch(msgIDs []email.ID) ([][]byte, error) {
	return c.GetRawMessagesBatchContext(context.Background(), msgIDs)
}

func (c *client) GetMessageSummaryContext(ctx context.Context, msgID email.ID) (*email.Email, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return msg.CachedEmail(c.localCfg.AccountCache, msgID)
}

func (c *client) GetMessageSummary(msgID email.ID) (*email.Email, error) {
	return c.GetMessageSummaryContext(context.Background(), msgID)
}

func (c *client) GetMessageSummariesBatchContext(ctx context.Context, msgIDs []email.ID) ([]*email.Email, error) {
	return batchFetch(ctx, msgIDs, c.GetMessageSummaryContext)
}

func (c *client) GetMessageSummariesBatch(msgIDs []email.ID) ([]*email.Email, error) {
	return c.GetMessageSummariesBatchContext(context.Background(), msgIDs)
}

func (c *client) PrefetchMessagesContext(ctx context.Context, _ []email.ID) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

func (c *client) PrefetchMessages(_ []email.ID) error {
	return nil
}

func (c *client) SearchEmails(query string, limit int) ([]email.ID, error) {
	ids, err := folderindex.IDs(c.localCfg.AccountCache, "INBOX")
	if err != nil {
		return nil, err
	}
	if query == "" {
		if limit > 0 && len(ids) > limit {
			return ids[:limit], nil
		}
		return ids, nil
	}
	qLower := strings.ToLower(query)
	var matches []email.ID
	for _, id := range ids {
		em, err := msg.CachedEmail(c.localCfg.AccountCache, id)
		if err != nil {
			continue
		}
		if strings.Contains(strings.ToLower(em.Subject), qLower) ||
			strings.Contains(strings.ToLower(em.FromEmail), qLower) ||
			strings.Contains(strings.ToLower(em.FromRaw), qLower) {
			matches = append(matches, id)
			if limit > 0 && len(matches) >= limit {
				break
			}
		}
	}
	return matches, nil
}

// LabelWriter stubs
func (c *client) RenameLabel(_ label.Folder, _ label.Folder) error {
	return mailclient.ErrReadOnlyOperationBlocked
}

func (c *client) FixLabels() error {
	return nil
}

func (c *client) DeleteLabel(_ label.Folder) error {
	return mailclient.ErrReadOnlyOperationBlocked
}

func (c *client) EnsureLabelExists(_ label.Folder) error {
	return nil
}

// EmailWriter stubs
func (c *client) MoveEmail(_ []email.ID, _ label.Folder, _ label.Folder) error {
	return mailclient.ErrReadOnlyOperationBlocked
}

func (c *client) CopyEmail(_ []email.ID, _ label.Folder, _ label.Folder) error {
	return mailclient.ErrReadOnlyOperationBlocked
}

func (c *client) MarkAsRead(_ []email.ID) error {
	return nil
}

func (c *client) MoveToInbox(_ []email.ID, _ label.Folder) error {
	return mailclient.ErrReadOnlyOperationBlocked
}

func (c *client) UploadRawEmail(_ []byte, _ label.Folder) (email.ID, error) {
	return "", mailclient.ErrReadOnlyOperationBlocked
}

func (c *client) DeleteMessages(_ []email.ID, _ label.Folder) error {
	return mailclient.ErrReadOnlyOperationBlocked
}

// SpamManager stubs
func (c *client) ReportSpam(_ []email.ID, _ label.Folder) error {
	return mailclient.ErrReadOnlyOperationBlocked
}

func (c *client) MoveAllSpam(_ label.Folder) error {
	return mailclient.ErrReadOnlyOperationBlocked
}

func (c *client) DeleteAllSpam() error {
	return mailclient.ErrReadOnlyOperationBlocked
}

func (c *client) ShowPoliticalSpam(_ bool) error {
	return nil
}

func (c *client) LearnSpam() error {
	return nil
}

// RuleManager stubs
func (c *client) CheckAndApplyRules(_ []email.ID, _ label.Folder, _ string) ([]email.ID, error) {
	return nil, nil
}

func (c *client) ExportRules() error {
	return nil
}

func (c *client) GetFilters() (mailclient.FilterListing, error) {
	return mailclient.FilterListing{Title: "Mbox (Local Archive)"}, nil
}

func (c *client) DeleteFilter(_ string) error {
	return fmt.Errorf("mbox archives do not support server-side filters")
}

func (c *client) RewriteRuleLabelCasing(label string) string {
	return label
}

// SendEmail stub
func (c *client) SendEmail(_ []byte) error {
	return mailclient.ErrReadOnlyOperationBlocked
}

// InboxFolder is the account's inbox, as the server names it.
func (c *client) InboxFolder() label.Folder {
	return label.FromServer(c.inboxName())
}
