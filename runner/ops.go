package runner

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/sarielhp/mail_cli/cache"
	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/last"
	"github.com/sarielhp/mail_cli/mailclient"
	"github.com/sarielhp/mail_cli/mailops"
	"github.com/sarielhp/mail_cli/report"
	"github.com/sarielhp/mail_cli/scan"
	"github.com/sarielhp/mail_cli/show"
)

// RunScanOnAccounts runs spam scanning across one or all matching accounts.
func RunScanOnAccounts(out io.Writer, opts scan.Options, config *cfg_g.Config, labelPrefix string, rep report.Reporter) (int, error) {
	totalMoved := 0
	var reports []scan.FolderReport
	err := mailops.ForEachAccount(config, labelPrefix, rep, func(acc cfg_acc.AccountConfig, prefix string) (bool, error) {
		if config.Verbose {
			rep.Infof("Running scan for account: %s (%s)...\n", acc.Name, acc.Type)
		}
		client, err := mailops.NewMailClient(acc, config, rep)
		if err != nil {
			return true, err
		}
		if validateErr := client.Validate(); validateErr != nil {
			return true, validateErr
		}
		realPrefix := prefix
		if strings.EqualFold(realPrefix, "inbox") {
			realPrefix = client.InboxFolder().String()
		}
		resolvedPrefix, err := mailclient.ResolveLabel(rep, client, realPrefix)
		if err != nil {
			return true, err
		}
		moved, folderReports, err := scan.Perform(out, opts, rep, client, client.Config(), resolvedPrefix.String())
		if err != nil {
			return true, err
		}
		totalMoved += moved
		reports = append(reports, folderReports...)
		return false, nil
	})
	if err != nil {
		return totalMoved, err
	}
	// One JSON document across every account scanned: a per-account array
	// would concatenate into invalid JSON the moment -A named more than one.
	if opts.JSON {
		return totalMoved, scan.MarshalReports(out, reports)
	}
	return totalMoved, nil
}

// showByMessageID displays arg as a message ID, fetched through the
// read-through accessor without being told which folder holds it -- the case
// `search` produces, where a user has seen an ID listed and nothing local
// knows about it yet.
func showByMessageID(opts show.Options, out io.Writer, client mailclient.Reader, id email.ID) bool {
	raw, err := client.GetRawMessage(id)
	if err != nil {
		slog.Debug("argument is not a retrievable message ID",
			slog.String("id", string(id)), slog.Any("error", err))
		return false
	}
	if err := show.Render(opts, out, client.Config(), id, "", raw); err != nil {
		slog.Warn("failed to render message", slog.String("id", string(id)), slog.Any("error", err))
		return false
	}
	return true
}

// errShortIDNotCached explains a short ID that no local cache can expand. A
// short ID is a hash of the full one, so a server cannot look it up; sending
// it there anyway produced a bare "not found".
func errShortIDNotCached(id email.ShortID) error {
	return fmt.Errorf("no downloaded message has short ID %s: a short ID names a message already in the local cache; list its folder first, or use the full message ID", id)
}

// showMessageByRef shows one message named by an ID the user typed: a full
// backend ID or a short one, in the selected account or, with none selected,
// in any account.
//
// `msg show <id>` used to pass the ID to the label-reference parser, which
// rejected it for lacking the sigil, so showing a message by ID alone had
// stopped working.
func showMessageByRef(opts show.Options, out io.Writer, config *cfg_g.Config, ref string, rep report.Reporter) error {
	var clients []mailclient.MailClient
	err := mailops.ForEachAccount(config, "", rep, func(acc cfg_acc.AccountConfig, _ string) (bool, error) {
		client, err := mailops.NewMailClient(acc, config, rep)
		if err != nil {
			return true, err
		}
		if err := client.Validate(); err != nil {
			return true, err
		}
		clients = append(clients, client)
		return false, nil
	})
	if err != nil {
		return err
	}
	return showRefFromClients(opts, out, clients, ref)
}

// showRefFromClients looks ref up in each client's cache, then -- for a full
// ID only -- asks each server.
func showRefFromClients(opts show.Options, out io.Writer, clients []mailclient.MailClient, ref string) error {
	for _, client := range clients {
		id, folder, errFind := cache.FindCachedEmailByID(client.Config().AccountCache, ref)
		if errFind == nil {
			return show.ByID(opts, out, client, client.Config(), id, folder)
		}
	}
	if short, ok := email.ParseShortID(ref); ok {
		return errShortIDNotCached(short)
	}
	// Nothing local knows ref and it is not a short ID, so it can only be a
	// full server ID.
	for _, client := range clients {
		if showByMessageID(opts, out, client, email.ID(ref)) {
			return nil
		}
	}
	return fmt.Errorf("no message with ID %q", ref)
}

// RunShowOnAccounts displays a message or lists messages across matching accounts.
//
// A first argument without the label sigil is a message ID; with the sigil it
// is a folder, optionally followed by a message ID within it.
func RunShowOnAccounts(opts show.Options, out io.Writer, config *cfg_g.Config, labelPrefix string, targetMsgID string, rep report.Reporter) error {
	if targetMsgID == "" && !strings.HasPrefix(labelPrefix, cfg_g.LabelSigil) {
		return showMessageByRef(opts, out, config, labelPrefix, rep)
	}
	var listings []show.FolderListing
	err := mailops.ForEachAccount(config, labelPrefix, rep, func(acc cfg_acc.AccountConfig, prefix string) (bool, error) {
		client, err := mailops.NewMailClient(acc, config, rep)
		if err != nil {
			return true, err
		}
		if validateErr := client.Validate(); validateErr != nil {
			return true, validateErr
		}
		realPrefix := prefix
		if strings.EqualFold(realPrefix, "inbox") {
			realPrefix = client.InboxFolder().String()
		}
		resolvedPrefix, err := mailclient.ResolveLabel(rep, client, realPrefix)
		if err != nil {
			return true, err
		}
		found, err := show.Perform(opts, out, client, client.Config(), resolvedPrefix.String(), targetMsgID)
		if err != nil {
			return true, err
		}
		listings = append(listings, found...)
		return false, nil
	})
	if err != nil {
		return err
	}
	// One JSON document across every folder and account, as scan does it; a
	// target message has already been written as its own object.
	if opts.JSON && targetMsgID == "" {
		return show.MarshalListings(out, listings)
	}
	return nil
}

// RunLastOnAccounts prints the last N messages for the selected account.
func RunLastOnAccounts(opts last.Options, out io.Writer, config *cfg_g.Config, n int, rep report.Reporter) error {
	client, err := mailops.GetClientForSelected(config, rep)
	if err != nil {
		return err
	}
	if err := client.Validate(); err != nil {
		return err
	}
	return last.Perform(opts, out, rep, client, client.Config(), n)
}

// ExportRulesToFile exports the active account's rules to a JSON file.
func ExportRulesToFile(rep report.Reporter, config *cfg_g.Config, filePath string) error {
	resolved, err := cfg_g.Resolve(config)
	if err != nil {
		return err
	}

	rules := resolved.Account.Rules
	if rules == nil {
		rules = []cfg_acc.Rule{}
	}

	rulesBytes, err := json.MarshalIndent(rules, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal rules: %w", err)
	}

	if err := os.WriteFile(filePath, rulesBytes, 0600); err != nil {
		return fmt.Errorf("failed to write rules to file %s: %w", filePath, err)
	}

	rep.Successf("Successfully exported %d rule(s) to %s\n", len(rules), filePath)
	return nil
}

// ImportRulesFromFile imports rules from a JSON file into the active account.
func ImportRulesFromFile(rep report.Reporter, config *cfg_g.Config, filePath string) error {
	importData, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read import file %s: %w", filePath, err)
	}

	var importedRules []cfg_acc.Rule
	if unmarshalErr := json.Unmarshal(importData, &importedRules); unmarshalErr != nil {
		return fmt.Errorf("failed to parse import file %s: %w", filePath, unmarshalErr)
	}

	resolved, err := cfg_g.Resolve(config)
	if err != nil {
		return err
	}

	if resolved.Account.Rules == nil {
		resolved.Account.Rules = []cfg_acc.Rule{}
	}

	existingMap := make(map[string]bool)
	for _, r := range resolved.Account.Rules {
		existingMap[strings.ToLower(strings.TrimSpace(r.Sender))] = true
	}

	importedCount := 0
	ignoredCount := 0

	for _, r := range importedRules {
		cleanSender := strings.ToLower(strings.TrimSpace(r.Sender))
		if cleanSender == "" {
			continue
		}
		if existingMap[cleanSender] {
			ignoredCount++
		} else {
			resolved.Account.Rules = append(resolved.Account.Rules, r)
			existingMap[cleanSender] = true
			importedCount++
		}
	}

	err = cfg_g.SaveConfigFile(resolved.Path, resolved.File)
	if err != nil {
		return fmt.Errorf("failed to save config file: %w", err)
	}

	rep.Successf("Successfully imported %d rule(s) from %s (ignored %d already existing rule(s))\n", importedCount, filePath, ignoredCount)
	return nil
}
