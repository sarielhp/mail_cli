package gmail

import (
	"context"
	"fmt"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/label"
	"sort"
	"strings"

	"github.com/sarielhp/mail_cli/cache/msgstore"
	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/mailclient"
	"github.com/sarielhp/mail_cli/report"

	gmailapi "google.golang.org/api/gmail/v1"
)

// client implements the MailClient interface using the Gmail REST API wrapper.
type client struct {
	config  *Config
	account AccountConfig
	rep     report.Reporter
}

// NewClient builds a client that reports progress through rep. A nil rep
// discards, which suits a caller that has nowhere to show it.
func NewClient(acc AccountConfig, config *Config, rep report.Reporter) mailclient.MailClient {
	return &client{config: config, account: acc, rep: report.Or(rep)}
}

func (c *client) Validate() error {
	if err := cfg_g.ValidateAccountParams(c.account); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), msgstore.DefaultFetchTimeout)
	defer cancel()
	srv, err := GetGmailServiceContext(ctx, c.rep, c.config)
	if err != nil {
		return err
	}
	if _, err := srv.Users.Labels.List("me").Context(ctx).Do(); err != nil {
		return err
	}
	return nil
}

func (c *client) GetMatchingLabels(prefix string) ([]string, error) {
	return GetMatchingLabels(c.rep, c.config, prefix)
}

func (c *client) FetchAndDownloadEmails(folder label.Folder, cacheSubdir string) ([]email.ID, error) {
	folderName := folder.String()
	return fetchAndDownloadEmailsREST(c.rep, c.config, folderName, cacheSubdir)
}

func (c *client) FetchLatestAccountEmails(limit int) ([]cfg_acc.MessageFolderRef, error) {
	return fetchLatestAccountEmailsREST(c.rep, c.config, limit)
}

func (c *client) CheckAndApplyRules(messageIDs []email.ID, source label.Folder, cacheSubdir string) ([]email.ID, error) {
	sourceLabelName := source.String()
	return checkAndApplyRulesREST(c.rep, messageIDs, c.config, sourceLabelName, cacheSubdir)
}

func (c *client) ReportSpam(messageIDs []email.ID, source label.Folder) error {
	sourceLabelName := source.String()
	return reportSpamToGmailREST(c.rep, messageIDs, c.config, sourceLabelName)
}

func (c *client) MoveToInbox(messageIDs []email.ID, source label.Folder) error {
	sourceLabelName := source.String()
	return moveToInboxGmailREST(c.rep, messageIDs, c.config, sourceLabelName)
}

func (c *client) MoveEmail(messageIDs []email.ID, source label.Folder, dest label.Folder) error {
	sourceLabelName := source.String()
	destLabelName := dest.String()
	return moveEmailGmailREST(c.rep, messageIDs, c.config, sourceLabelName, destLabelName)
}

func (c *client) CopyEmail(messageIDs []email.ID, source label.Folder, dest label.Folder) error {
	sourceLabelName := source.String()
	destLabelName := dest.String()
	return copyEmailGmailREST(c.rep, c.config, messageIDs, sourceLabelName, destLabelName)
}

func (c *client) MoveAllSpam(dest label.Folder) error {
	destLabel := dest.String()
	return moveAllSpamInGmailREST(c.rep, c.config, destLabel)
}

func (c *client) DeleteAllSpam() error {
	return deleteAllSpamInGmailREST(c.rep, c.config)
}

func (c *client) ShowPoliticalSpam(autoBlacklist bool) error {
	return showPoliticalSpamInGmailREST(c.rep, c.config, autoBlacklist)
}

func (c *client) LearnSpam() error {
	return learnSpamFromGmailREST(c.rep, c.config)
}

func (c *client) GetLabelItems() ([]item, error) {
	return getLabelItemsGmailREST(c.rep, c.config)
}

func (c *client) MarkAsRead(messageIDs []email.ID) error {
	return markAsReadGmailREST(c.rep, c.config, messageIDs)
}

func (c *client) RenameLabel(oldFolder label.Folder, newFolder label.Folder) error {
	oldName := oldFolder.String()
	newName := newFolder.String()
	return renameGmailLabelREST(c.rep, c.config, oldName, newName)
}

func (c *client) FixLabels() error {
	return fixGmailLabelsREST(c.rep, c.config)
}

func (c *client) DeleteLabel(folder label.Folder) error {
	name := folder.String()
	return deleteGmailLabelREST(c.rep, c.config, name)
}

func (c *client) ExportRules() error {
	return exportRulesToGmailREST(c.rep, c.config)
}

func (c *client) GetFilters() (mailclient.FilterListing, error) {
	return gmailFilters(c.rep, c.config)
}

func (c *client) DeleteFilter(id string) error {
	return deleteGmailFilterREST(c.rep, c.config, id)
}

func (c *client) Config() *Config {
	return c.config
}

func (c *client) RewriteRuleLabelCasing(label string) string {
	return rewriteRuleLabelCasing(c.rep, c.config, label)
}

func (c *client) inboxName() string {
	return "INBOX"
}

func (c *client) BackendType() string { return "gmail" }

// GetMatchingLabels fetches Gmail labels matching a prefix.
func GetMatchingLabels(rep report.Reporter, config *Config, prefix string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), msgstore.DefaultFetchTimeout)
	defer cancel()
	srv, err := GetGmailServiceContext(ctx, rep, config)
	if err != nil {
		return nil, err
	}

	labelsRes, err := executeGmailOpWithRetry(ctx, "list labels", func() (*gmailapi.ListLabelsResponse, error) {
		return srv.Users.Labels.List("me").Context(ctx).Do()
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list Gmail labels: %w", err)
	}

	var matched []string
	prefixLower := strings.ToLower(prefix)
	for _, l := range labelsRes.Labels {
		if strings.HasPrefix(strings.ToLower(l.Name), prefixLower) {
			matched = append(matched, l.Name)
		}
	}

	if len(matched) == 0 {
		switch prefixLower {
		case "inbox", "[gmail]/inbox", "[google mail]/inbox":
			matched = append(matched, "INBOX")
		case "spam", "[gmail]/spam", "[google mail]/spam":
			matched = append(matched, "SPAM")
		case "sent", "sent mail", "[gmail]/sent mail", "[gmail]/sent", "[google mail]/sent mail":
			matched = append(matched, "SENT")
		case "trash", "bin", "[gmail]/trash", "[gmail]/bin", "[google mail]/trash":
			matched = append(matched, "TRASH")
		case "drafts", "draft", "[gmail]/drafts", "[google mail]/drafts":
			matched = append(matched, "DRAFT")
		}
	}

	sort.Strings(matched)
	return matched, nil
}

// InboxFolder is the account's inbox, as the server names it.
func (c *client) InboxFolder() label.Folder {
	return label.FromServer(c.inboxName())
}
