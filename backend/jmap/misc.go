package jmap

import (
	"fmt"
	"github.com/sarielhp/mail_cli/app"
	"github.com/sarielhp/mail_cli/backend/spamlearn"
	"github.com/sarielhp/mail_cli/cache/msg"
	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/label"
	"github.com/sarielhp/mail_cli/mailclient"
	"github.com/sarielhp/mail_cli/report"
	"github.com/sarielhp/mail_cli/spam"
	"github.com/sarielhp/mail_cli/train"
	"strings"

	"git.sr.ht/~rockorager/go-jmap"
	"git.sr.ht/~rockorager/go-jmap/mail/mailbox"
)

func (c *client) GetLabelItems() ([]cfg_acc.Item, error) {
	if err := c.init(); err != nil {
		return nil, err
	}

	req := &jmap.Request{}
	req.Invoke(&mailbox.Get{
		Account: c.accID,
	})
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}

	var mbs []*mailbox.Mailbox
	for _, inv := range resp.Responses {
		if r, ok := inv.Args.(*mailbox.GetResponse); ok {
			mbs = r.List
		}
	}

	getFullPath := mailboxFullPath(mbs)

	var items []cfg_acc.Item
	for _, mb := range mbs {
		items = append(items, cfg_acc.Item{
			Name:           mb.Name,
			FullName:       getFullPath(mb),
			MessagesTotal:  int64(mb.TotalEmails),
			MessagesUnread: int64(mb.UnreadEmails),
			IsLabel:        true,
		})
	}

	return items, nil
}

// mailboxFullPath resolves each mailbox's full path by walking ParentID links.
// A mailbox whose parent is not in mbs keeps its own name.
func mailboxFullPath(mbs []*mailbox.Mailbox) func(*mailbox.Mailbox) string {
	byID := make(map[jmap.ID]*mailbox.Mailbox, len(mbs))
	for _, mb := range mbs {
		byID[mb.ID] = mb
	}
	var fullPath func(mb *mailbox.Mailbox) string
	fullPath = func(mb *mailbox.Mailbox) string {
		if mb.ParentID == "" {
			return mb.Name
		}
		parent, exists := byID[mb.ParentID]
		if !exists {
			return mb.Name
		}
		return fullPath(parent) + "/" + mb.Name
	}
	return fullPath
}

func (c *client) RenameLabel(oldFolder label.Folder, newFolder label.Folder) error {
	oldName := oldFolder.String()
	newName := newFolder.String()
	if err := label.ValidateNewName(newName); err != nil {
		return err
	}
	if err := c.init(); err != nil {
		return err
	}

	mbID, err := c.getMailboxID(oldName)
	if err != nil {
		return err
	}

	parts := strings.Split(newName, "/")
	localName := parts[len(parts)-1]

	var parentID jmap.ID
	if len(parts) > 1 {
		parentPath := strings.Join(parts[:len(parts)-1], "/")
		parentID, err = c.getOrCreateMailbox(parentPath)
		if err != nil {
			return err
		}
	}

	req := &jmap.Request{}
	patch := jmap.Patch{
		"name": localName,
	}
	if len(parts) > 1 {
		patch["parentId"] = string(parentID)
	} else {
		patch["parentId"] = nil
	}

	req.Invoke(&mailbox.Set{
		Account: c.accID,
		Update: map[jmap.ID]jmap.Patch{
			mbID: patch,
		},
	})

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}

	for _, inv := range resp.Responses {
		if r, ok := inv.Args.(*mailbox.SetResponse); ok {
			if errSet, failed := r.NotUpdated[mbID]; failed {
				return fmt.Errorf("failed to rename JMAP mailbox: %s", safeStr(errSet.Description))
			}
		}
	}

	c.rep.Successf("Successfully renamed JMAP mailbox %q to %q.\n", oldName, newName)
	return c.loadMailboxes()
}

func (c *client) FixLabels() error {
	c.rep.Infof("JMAP mailboxes hierarchy is managed automatically by the JMAP server.\n")
	return nil
}

func (c *client) DeleteLabel(folder label.Folder) error {
	name := folder.String()
	if err := c.init(); err != nil {
		return err
	}

	mbID, err := c.getMailboxID(name)
	if err != nil {
		return err
	}

	req := &jmap.Request{}
	req.Invoke(&mailbox.Set{
		Account: c.accID,
		Destroy: []jmap.ID{mbID},
	})

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}

	for _, inv := range resp.Responses {
		if r, ok := inv.Args.(*mailbox.SetResponse); ok {
			if errSet, failed := r.NotDestroyed[mbID]; failed {
				return fmt.Errorf("failed to delete JMAP mailbox: %s", safeStr(errSet.Description))
			}
		}
	}

	c.rep.Successf("Successfully deleted JMAP mailbox %q.\n", name)
	return c.loadMailboxes()
}

func selectJMAPRulesToExport(rep report.Reporter, rules []cfg_acc.Rule, forceLearn bool, accountName string) []cfg_acc.Rule {
	var rulesToExport []cfg_acc.Rule
	for _, rule := range rules {
		if rule.Internal {
			continue
		}
		if forceLearn || !rule.Exported {
			rulesToExport = append(rulesToExport, rule)
		}
	}
	switch {
	case forceLearn:
		rep.Infof("Force mode: will export all rules (server-side filters not supported by this JMAP server).\n")
	case len(rulesToExport) == 0:
		rep.Infof("All rules for account %s are already exported.\n", accountName)
	default:
		rep.Infof("Exporting %d unexported rule(s)...\n", len(rulesToExport))
	}
	return rulesToExport
}

func markRulesExported(rules []cfg_acc.Rule) {
	for i := range rules {
		if !rules[i].Internal {
			rules[i].Exported = true
		}
	}
}

func (c *client) ExportRules() error {
	if err := c.init(); err != nil {
		return err
	}

	resolved, err := cfg_g.Resolve(c.config)
	if err != nil {
		return err
	}

	if len(resolved.Account.Rules) == 0 {
		c.rep.Warnf("No rules found to export for account %s.\n", resolved.Account.Name)
		return nil
	}

	rulesToExport := selectJMAPRulesToExport(c.rep, resolved.Account.Rules, c.config.ForceLearn, resolved.Account.Name)
	if len(rulesToExport) == 0 {
		return nil
	}

	c.rep.Warnf("Note: Server-side filter creation is not supported by this JMAP server.\n")
	c.rep.Warnf("Rules will be marked as exported locally in config.\n\n")
	for _, r := range rulesToExport {
		c.rep.Printf("  - %s -> %s\n", r.Sender, r.Label)
	}

	markRulesExported(resolved.Account.Rules)
	if err := cfg_g.SaveConfigFile(resolved.Path, resolved.File); err != nil {
		return err
	}
	markRulesExported(c.config.Rules)

	c.rep.Successf("Marked %d rule(s) as exported locally for account %s.\n", len(rulesToExport), resolved.Account.Name)
	return nil
}

// GetFilters reports the local routing rules. JMAP has no server-side filters
// at all, so the note explains what the user is actually looking at.
func (c *client) GetFilters() (mailclient.FilterListing, error) {
	resolved, err := cfg_g.Resolve(c.config)
	if err != nil {
		return mailclient.FilterListing{}, err
	}

	listing := mailclient.FilterListing{
		Title: "SERVER FILTERS FOR ACCOUNT: " + strings.ToUpper(resolved.Account.Name),
		Note: "JMAP servers (including FastMail) do not support server-side\n" +
			"  mail filters. The following are local routing rules.",
	}

	for _, rule := range resolved.Account.Rules {
		status := "unexported"
		if rule.Exported {
			status = "exported"
		}
		// A rule may match on sender, on subject, or on both. Showing only the
		// sender left a subject rule with a blank identifier.
		var match []string
		if rule.Sender != "" {
			match = append(match, "From: "+rule.Sender)
		}
		if rule.Subject != "" {
			match = append(match, "Subject: "+rule.Subject)
		}
		id := rule.Sender
		if id == "" {
			id = rule.Subject
		}
		if id == "" {
			id = "(no criteria)"
		}
		listing.Filters = append(listing.Filters, mailclient.Filter{
			ID:     id,
			Status: status,
			Match:  strings.Join(match, " AND "),
			Action: rule.Label,
		})
	}
	return listing, nil
}

func (c *client) DeleteFilter(_ string) error {
	return fmt.Errorf("JMAP does not support server-side filters")
}

func (c *client) LearnSpam() error {
	if err := c.init(); err != nil {
		return err
	}

	spamFolder := c.account.SpamFolder
	if spamFolder == "" {
		spamFolder = "Spam"
	}

	ids, err := c.FetchAndDownloadEmails(label.FromServer(spamFolder), "spam")
	if err != nil {
		return err
	}

	if len(ids) == 0 {
		c.rep.Infof("JMAP Spam folder is empty.\n")
		return nil
	}

	total := len(ids)

	if err := app.RunPreFlightCheck(); err != nil {
		return err
	}

	trainedUidsPath := train.SpamUIDPath(c.config.AccountCache)
	trainedUids := make(map[string]bool)
	if !c.config.ForceLearn {
		// A ledger that will not parse stops the run: carrying on would
		// retrain every message in the mailbox and skew the corpus.
		ledger, lErr := train.Open(trainedUidsPath)
		if lErr != nil {
			return fmt.Errorf("%w; delete the file to start the trained-spam record over", lErr)
		}
		for _, id := range ledger.IDs() {
			trainedUids[id] = true
		}
	}

	successCount := 0
	alreadyLearnedCount := 0
	ignoredCount := 0

	for i, id := range ids {
		if trainedUids[string(id)] {
			alreadyLearnedCount++
			continue
		}

		data, rErr := msg.Read(c.config.AccountCache, id)
		if rErr != nil {
			ignoredCount++
			continue
		}
		emailBytes := data

		errRun := spam.TrainSpam(emailBytes)
		if errRun == nil {
			successCount++
			trainedUids[string(id)] = true
			_ = msg.ForgetEvidence(c.config.AccountCache, id)
		} else if c.config.Verbose {
			c.rep.Warnf("    Bogofilter training failed for ID %s: %v\n", id, errRun)
		}

		c.rep.Progress(i+1, total, "Training classifier")
	}

	spamlearn.RecordTrained(trainedUidsPath, trainedUids)

	c.rep.Successf("%s\n", app.TrainingSummary("spam message(s)", successCount, alreadyLearnedCount, ignoredCount))
	return nil
}
