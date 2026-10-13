package cli

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/sarielhp/mail_cli/app"
	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/mailclient"
	"github.com/sarielhp/mail_cli/ui"

	"github.com/sarielhp/clihelp"
)

// senderListRemoveSpec names the wording and list type of a sender list's `rm`
// subcommand, which the allowlist and blocklist otherwise spell identically.
type senderListRemoveSpec struct {
	path        string
	description string
	paramDesc   string
	listType    string
	example     string
}

func newSenderListRemoveCmd(session *Session, spec senderListRemoveSpec) clihelp.Command {
	return clihelp.Command{
		Name:        "rm",
		NoAbbrev:    true,
		Aliases:     []string{"del"},
		Title:       spec.path + " rm <email...>",
		Description: spec.description,
		UsageLine:   "mail_cli " + spec.path + " rm <email...>",
		Parameters: []clihelp.Param{
			{Name: "<email...>", Description: spec.paramDesc, Variadic: true},
		},
		Args: clihelp.MinimumNArgs(1),
		Run: func(ctx *clihelp.Context) error {
			return whitelistOrBlacklist(session, spec.listType, ctx.Args)
		},
		Examples: []clihelp.Example{
			{Line: "mail_cli " + spec.path + " rm " + spec.example},
		},
	}
}

// newSenderListListCmd builds the `list` subcommand of a sender list.
func newSenderListListCmd(session *Session, path, description, listType string) clihelp.Command {
	return clihelp.Command{
		Name:        "list",
		Description: description,
		UsageLine:   "mail_cli " + path + " list",
		Args:        clihelp.NoArgs,
		Run: func(ctx *clihelp.Context) error {
			return whitelistOrBlacklist(session, listType, ctx.Args)
		},
		Examples: []clihelp.Example{
			{Line: "mail_cli " + path + " list"},
		},
	}
}

// WlistCmd returns the clihelp.Command for the whitelist command.
func wlistCmd(session *Session, name, path string) clihelp.Command {
	return clihelp.Command{
		Name:            name,
		Group:           "Spam",
		Description:     "Manage sender allowlist to bypass spam.",
		LongDescription: "Manage the personal sender whitelist to bypass spam checks.",
		UsageLine:       "mail_cli " + path + " <subcommand> [args...]",
		Subcommands: []clihelp.Command{
			{
				Name:        "add",
				Title:       path + " add <email...>",
				Description: "Add email addresses to allowlist.",
				UsageLine:   "mail_cli " + path + " add <email...>",
				Parameters: []clihelp.Param{
					{Name: "<email...>", Description: "Email addresses to add to whitelist.", Variadic: true},
				},
				Args: clihelp.MinimumNArgs(1),
				Run: func(ctx *clihelp.Context) error {
					return whitelistOrBlacklist(session, "whitelistadd", ctx.Args)
				},
				Examples: []clihelp.Example{
					{Line: "mail_cli " + path + " add friend@example.com"},
				},
			},
			newSenderListRemoveCmd(session, senderListRemoveSpec{
				path:        path,
				description: "Remove email addresses from allowlist.",
				paramDesc:   "Email addresses to remove from whitelist.",
				listType:    "whitelistdel",
				example:     "friend@example.com",
			}),
			newSenderListListCmd(session, path, "List allowed email addresses.", "whitelistlist"),
		},
		Examples: []clihelp.Example{
			{Line: "mail_cli " + path + " list"},
			{Line: "mail_cli " + path + " add friend@example.com"},
		},
	}
}

// BlistCmd returns the clihelp.Command for the blacklist command.
func blistCmd(session *Session, name, path string) clihelp.Command {
	return clihelp.Command{
		Name:            name,
		Group:           "Spam",
		Description:     "Manage sender blocklist to classify as spam.",
		LongDescription: "Manage the personal sender blacklist to instantly classify messages as spam.",
		UsageLine:       "mail_cli " + path + " <subcommand> [args...]",
		Subcommands: []clihelp.Command{
			{
				Name:        "add",
				Title:       path + " add <email...>",
				Description: "Add email addresses to blocklist.",
				UsageLine:   "mail_cli " + path + " add <email...>",
				Parameters: []clihelp.Param{
					{Name: "<email...>", Description: "Email addresses to add to blacklist.", Variadic: true},
				},
				Options: []clihelp.Option{
					clihelp.Bool(&flagBlockTrash, "--trash", false, "Route to Trash instead of Spam (no spam filter training)"),
					clihelp.Bool(&flagBlockPush, "--push", false, "Immediately push updated rules to the server"),
				},
				Args: clihelp.MinimumNArgs(1),
				Run: func(ctx *clihelp.Context) error {
					return whitelistOrBlacklist(session, "blacklistadd", ctx.Args)
				},
				Examples: []clihelp.Example{
					{Line: "mail_cli " + path + " add spammer@example.com"},
				},
			},
			newSenderListRemoveCmd(session, senderListRemoveSpec{
				path:        path,
				description: "Remove email addresses from blocklist.",
				paramDesc:   "Email addresses to remove from blacklist.",
				listType:    "blacklistdel",
				example:     "spammer@example.com",
			}),
			newSenderListListCmd(session, path, "List blocked email addresses.", "blacklistlist"),
		},
		Examples: []clihelp.Example{
			{Line: "mail_cli " + path + " list"},
			{Line: "mail_cli " + path + " add spammer@example.com"},
		},
	}
}

// FilterCmd returns the clihelp.Command for the filter command.
func remoteCmd(session *Session, name, path string) clihelp.Command {
	return clihelp.Command{
		Name:        name,
		Group:       "Rules",
		Description: "Manage remote server-side filters.",
		LongDescription: "Manage remote Gmail server-side filters and forwarding rules.\n\n" +
			"Remote filters are server-side mail rules enforced directly on incoming messages by the mail provider (e.g. Gmail) before mail_cli ever connects. In mail_cli, filters and rules share the same conceptual model:\n\n" +
			"  1. Define rules locally in config.json using 'rule add' or 'rule add-title'.\n" +
			"  2. Export rules to the server as remote filters using 'rule push' (or 'rule export'). This translates local rules into server filters (auto-labeling, archiving, or forwarding) and marks them as exported.\n" +
			"  3. If a remote filter already exists with a different action, overwrite it with 'rule push --force'.\n" +
			"  4. Inspect active server-side filters with 'rule remote list'.\n" +
			"  5. Delete an active server filter and its matching local rule with 'rule remote rm'.",
		UsageLine: "mail_cli " + path + " <subcommand>",
		Notes: []clihelp.Note{
			{
				Heading: "Filters are Server-Side Rules",
				Text:    "Gmail filters run on Google servers 24/7 as incoming mail arrives. mail_cli manages them declaratively: you create routing rules locally with 'rule add', then push them to Gmail with 'rule push'.",
			},
			{
				Heading: "Creating Filters via Export / Push",
				Text:    "Run 'mail_cli rule push' to export all unexported local rules to Gmail server filters. If a remote filter conflicts with a local rule, pass '--force' (or 'rule push force') to overwrite the remote filter.",
			},
		},
		Subcommands: []clihelp.Command{
			{
				Name:        "list",
				Description: "List server-side filters for this account.",
				UsageLine:   "mail_cli " + path + " list",
				Args:        clihelp.NoArgs,
				Run: func(_ *clihelp.Context) error {
					client, err := GetValidatedClient(session)
					if err != nil {
						return err
					}
					listing, err := client.GetFilters()
					if err != nil {
						return err
					}
					ui.FormatFilterListing(session.out(), listing)
					return nil
				},
				Examples: []clihelp.Example{
					{Line: "mail_cli " + path + " list"},
				},
			},
			newRuleRemoteDelCmd(session, path),
		},
		Examples: []clihelp.Example{
			{Line: "mail_cli " + path + " list"},
			{Line: "mail_cli " + path + " rm 1"},
		},
	}
}

func newRuleRemoteDelCmd(session *Session, path string) clihelp.Command {
	return clihelp.Command{
		Name:        "rm",
		Title:       "rule remote rm <target>",
		Description: "Delete a server filter and its local rule.",
		UsageLine:   "mail_cli " + path + " rm <target> [flags]",
		Parameters: []clihelp.Param{
			{Name: "<target>", Description: "The filter index, filter ID, or sender/subject to delete."},
		},
		Options: []clihelp.Option{
			clihelp.Bool(&flagRuleRemoteRmYes, "-y, --yes", false, "Proceed without the confirmation prompt."),
			clihelp.Bool(&flagRuleRemoteKeepLocal, "--keep-local", false, "Keep matching local rule in config.json (mark as unexported)."),
		},
		Args: clihelp.ExactArgs(1),
		Run: func(ctx *clihelp.Context) error {
			return deleteRemoteRule(session, ctx.Args[0], flagRuleRemoteRmYes, flagRuleRemoteKeepLocal)
		},
		Examples: []clihelp.Example{
			{Line: "mail_cli " + path + " rm 1"},
			{Line: "mail_cli " + path + " rm AN63b9f"},
			{Line: "mail_cli " + path + " rm alerts@github.com"},
		},
	}
}

func resolveRemoteFilter(filters []mailclient.Filter, target string) (*mailclient.Filter, int, error) {
	trimmed := strings.TrimSpace(target)
	if trimmed == "" {
		return nil, -1, fmt.Errorf("filter target cannot be empty")
	}

	if num, err := strconv.Atoi(trimmed); err == nil && num >= 1 && num <= len(filters) {
		return &filters[num-1], num - 1, nil
	}

	for i := range filters {
		if filters[i].RemoteID != "" && strings.EqualFold(filters[i].RemoteID, trimmed) {
			return &filters[i], i, nil
		}
	}

	cleanPrefix := strings.TrimSuffix(trimmed, "...")
	if cleanPrefix != "" {
		var prefixMatches []int
		for i := range filters {
			if filters[i].RemoteID != "" && strings.HasPrefix(strings.ToLower(filters[i].RemoteID), strings.ToLower(cleanPrefix)) {
				prefixMatches = append(prefixMatches, i)
			}
		}
		if len(prefixMatches) == 1 {
			return &filters[prefixMatches[0]], prefixMatches[0], nil
		}
		if len(prefixMatches) > 1 {
			return nil, -1, fmt.Errorf("ambiguous filter ID %q matches %d remote filters; use index or full ID", trimmed, len(prefixMatches))
		}
	}

	for i := range filters {
		if strings.EqualFold(filters[i].ID, trimmed) {
			return &filters[i], i, nil
		}
	}

	var queryMatches []int
	targetAddr := email.ParseEmailAddress(trimmed)
	for i := range filters {
		f := &filters[i]
		matchSender := f.Sender != "" && (strings.EqualFold(f.Sender, trimmed) ||
			(targetAddr != "" && strings.EqualFold(email.ParseEmailAddress(f.Sender), targetAddr)))
		matchSubject := f.Subject != "" && strings.EqualFold(strings.TrimSpace(f.Subject), trimmed)
		if matchSender || matchSubject {
			queryMatches = append(queryMatches, i)
		}
	}
	if len(queryMatches) == 1 {
		return &filters[queryMatches[0]], queryMatches[0], nil
	}
	if len(queryMatches) > 1 {
		return nil, -1, fmt.Errorf("ambiguous query %q matches %d remote filters; use index or filter ID", trimmed, len(queryMatches))
	}

	return nil, -1, fmt.Errorf("no remote filter found matching %q", trimmed)
}

func findMatchingLocalRule(rules []cfg_acc.Rule, filter *mailclient.Filter) (int, *cfg_acc.Rule) {
	cleanSender := strings.ToLower(strings.TrimSpace(filter.Sender))
	cleanSubject := strings.ToLower(strings.TrimSpace(filter.Subject))
	if cleanSender == "" && cleanSubject == "" {
		return -1, nil
	}

	senderAddr := email.ParseEmailAddress(cleanSender)
	for i := range rules {
		r := &rules[i]
		matchSender := r.Sender != "" && cleanSender != "" &&
			(strings.EqualFold(strings.TrimSpace(r.Sender), cleanSender) ||
				(senderAddr != "" && strings.EqualFold(email.ParseEmailAddress(r.Sender), senderAddr)))
		matchSubject := r.Subject != "" && cleanSubject != "" &&
			strings.EqualFold(strings.TrimSpace(r.Subject), cleanSubject)

		if r.Sender != "" && r.Subject != "" {
			if matchSender && matchSubject {
				return i, r
			}
			continue
		}
		if matchSender || matchSubject {
			return i, r
		}
	}
	return -1, nil
}

func deleteRemoteRule(session *Session, target string, yes, keepLocal bool) error {
	client, err := GetValidatedClient(session)
	if err != nil {
		return err
	}
	listing, err := client.GetFilters()
	if err != nil {
		return err
	}
	if len(listing.Filters) == 0 {
		return fmt.Errorf("no remote filters found")
	}

	matchedFilter, filterIdx, err := resolveRemoteFilter(listing.Filters, target)
	if err != nil {
		return err
	}

	resolved, resErr := cfg_g.Resolve(session.Config)
	var localRuleIdx = -1
	var localRule *cfg_acc.Rule
	if resErr == nil && resolved.Account != nil {
		localRuleIdx, localRule = findMatchingLocalRule(resolved.Account.Rules, matchedFilter)
	}

	printFilterDeletionPreview(session.out(), matchedFilter, filterIdx, localRule)

	if session.Config.DryRun {
		reportRemoteRuleDryRun(session, matchedFilter, localRule, keepLocal)
		return nil
	}

	confirmDesc := fmt.Sprintf("delete remote filter %s from server", matchedFilter.ID)
	if localRule != nil && !keepLocal {
		confirmDesc += " and matching local rule"
	}
	if err := session.Confirm(confirmDesc, yes); err != nil {
		return declined(session, err)
	}

	deleteID := matchedFilter.RemoteID
	if deleteID == "" {
		deleteID = matchedFilter.ID
	}
	if err := client.DeleteFilter(deleteID); err != nil {
		return fmt.Errorf("failed to delete remote filter: %w", err)
	}
	fmt.Fprintf(session.out(), "%s Deleted remote filter %s from server.\n", app.PrefixSuccess, matchedFilter.ID)

	if localRule != nil && resErr == nil {
		applyLocalRuleDeletion(session, resolved, localRuleIdx, localRule, keepLocal)
	}
	return nil
}

func applyLocalRuleDeletion(session *Session, resolved cfg_g.Resolved, localRuleIdx int, localRule *cfg_acc.Rule, keepLocal bool) {
	if keepLocal {
		resolved.Account.Rules[localRuleIdx].Exported = false
		if err := cfg_g.SaveConfigFile(resolved.Path, resolved.File); err != nil {
			fmt.Fprintf(session.errOut(), "%s Warning: Failed to update local rule in config: %v\n", app.PrefixWarn, err)
			return
		}
		fmt.Fprintf(session.out(), "%s Preserved local rule for %s (marked as unexported).\n", app.PrefixSuccess, localRuleDesc(localRule))
		return
	}
	ruleName := localRuleDesc(localRule)
	resolved.Account.Rules = append(resolved.Account.Rules[:localRuleIdx], resolved.Account.Rules[localRuleIdx+1:]...)
	if err := cfg_g.SaveConfigFile(resolved.Path, resolved.File); err != nil {
		fmt.Fprintf(session.errOut(), "%s Warning: Failed to remove local rule from config: %v\n", app.PrefixWarn, err)
		return
	}
	fmt.Fprintf(session.out(), "%s Removed matching local rule for %s from config.json.\n", app.PrefixSuccess, ruleName)
}

func printFilterDeletionPreview(w io.Writer, filter *mailclient.Filter, filterIdx int, localRule *cfg_acc.Rule) {
	fmt.Fprintf(w, "Remote filter [%d]: %s\n", filterIdx+1, filter.ID)
	if filter.Match != "" {
		fmt.Fprintf(w, "  Matches: %s\n", filter.Match)
	}
	if filter.Action != "" {
		fmt.Fprintf(w, "  Action:  %s\n", filter.Action)
	}
	if localRule != nil {
		fmt.Fprintf(w, "  Matching local rule: %s -> %s\n", localRuleDesc(localRule), localRule.Label)
	}
}

func reportRemoteRuleDryRun(session *Session, filter *mailclient.Filter, localRule *cfg_acc.Rule, keepLocal bool) {
	fmt.Fprintf(session.out(), "%s [DRY-RUN] Would delete remote filter %s from server.\n", app.PrefixInfo, filter.ID)
	if localRule == nil {
		return
	}
	if keepLocal {
		fmt.Fprintf(session.out(), "%s [DRY-RUN] Would mark local rule for %s as unexported.\n", app.PrefixInfo, localRuleDesc(localRule))
		return
	}
	fmt.Fprintf(session.out(), "%s [DRY-RUN] Would remove matching local rule for %s from config.json.\n", app.PrefixInfo, localRuleDesc(localRule))
}

func localRuleDesc(r *cfg_acc.Rule) string {
	if r.Sender != "" {
		return r.Sender
	}
	return "[Subject: " + r.Subject + "]"
}

func whitelistOrBlacklist(session *Session, listType string, args []string) error {
	resolved, err := cfg_g.Resolve(session.Config)
	if err != nil {
		return err
	}

	field := "whitelist"
	if listType == "blacklistadd" || listType == "blacklistdel" || listType == "blacklistlist" {
		field = "blacklist"
	}

	switch listType {
	case "whitelistadd":
		for _, emailAddr := range args {
			if !email.IsValidEmailOrDomain(emailAddr) {
				return fmt.Errorf("%q is not a valid email address or domain pattern (must contain '@' and valid domain)", emailAddr)
			}
			AddToList(session.out(), resolved.Account, field, emailAddr)
		}
	case "blacklistadd":
		for _, emailAddr := range args {
			if !email.IsValidEmailOrDomain(emailAddr) {
				return fmt.Errorf("%q is not a valid email address or domain pattern (must contain '@' and valid domain)", emailAddr)
			}
			if flagBlockTrash {
				ensureRuleForTrash(session.out(), resolved.Account, emailAddr)
				RemoveFromList(session.out(), resolved.Account, field, emailAddr)
			} else {
				AddToList(session.out(), resolved.Account, field, emailAddr)
				ensureRuleForBlacklist(session.out(), resolved.Account, emailAddr)
			}
			_ = session.contacts().Purge(session.Config, emailAddr)
		}
	case "whitelistdel":
		for _, emailAddr := range args {
			RemoveFromList(session.out(), resolved.Account, field, emailAddr)
		}
	case "blacklistdel":
		for _, emailAddr := range args {
			RemoveFromList(session.out(), resolved.Account, field, emailAddr)
			removeRuleForBlacklist(session.out(), resolved.Account, emailAddr)
		}
	case "whitelistlist", "blacklistlist":
		ListEntries(session.out(), resolved.Account, field)
		return nil
	}

	if err := cfg_g.SaveConfigFile(resolved.Path, resolved.File); err != nil {
		return err
	}
	if flagBlockPush && listType == "blacklistadd" {
		client, err := GetValidatedClient(session)
		if err != nil {
			return err
		}
		if err := client.ExportRules(); err != nil {
			return fmt.Errorf("exporting rules to server failed: %w", err)
		}
		fmt.Fprintf(session.out(), "%s Pushed updated rules to server.\n", app.PrefixSuccess)
	}
	return nil
}

func ensureRuleForTrash(w io.Writer, targetAcc *cfg_acc.AccountConfig, emailAddr string) {
	trashTarget := "Trash"
	for i, r := range targetAcc.Rules {
		if strings.EqualFold(r.Sender, emailAddr) {
			targetAcc.Rules[i].Label = trashTarget
			targetAcc.Rules[i].Exported = false
			fmt.Fprintf(w, "%s Updated rule: %q -> %q for account %s.\n", app.PrefixSuccess, emailAddr, trashTarget, targetAcc.Name)
			return
		}
	}
	targetAcc.Rules = append(targetAcc.Rules, cfg_acc.Rule{Sender: emailAddr, Label: trashTarget})
	fmt.Fprintf(w, "%s Created rule: %q -> %q for account %s.\n", app.PrefixSuccess, emailAddr, trashTarget, targetAcc.Name)
}

func ensureRuleForBlacklist(w io.Writer, targetAcc *cfg_acc.AccountConfig, emailAddr string) {
	if targetAcc.SpamLearn == "" {
		fmt.Fprintf(w, "%s Warning: no SpamLearn folder configured for account %s; skipping rule creation.\n", app.PrefixWarn, targetAcc.Name)
		return
	}
	for _, r := range targetAcc.Rules {
		if strings.EqualFold(r.Sender, emailAddr) {
			return
		}
	}
	targetAcc.Rules = append(targetAcc.Rules, cfg_acc.Rule{Sender: emailAddr, Label: targetAcc.SpamLearn})
	fmt.Fprintf(w, "%s Created rule: %q -> %q for account %s.\n", app.PrefixSuccess, emailAddr, targetAcc.SpamLearn, targetAcc.Name)
}

func removeRuleForBlacklist(w io.Writer, targetAcc *cfg_acc.AccountConfig, emailAddr string) {
	for i, r := range targetAcc.Rules {
		if strings.EqualFold(r.Sender, emailAddr) {
			targetAcc.Rules = append(targetAcc.Rules[:i], targetAcc.Rules[i+1:]...)
			fmt.Fprintf(w, "%s Removed rule for %q on account %s.\n", app.PrefixSuccess, emailAddr, targetAcc.Name)
			return
		}
	}
}

// SpamAllowCmd is the sender allow-list, under the concept it serves: an
// allowed sender bypasses the spam checks.
func SpamAllowCmd(session *Session) clihelp.Command {
	return wlistCmd(session, "allow", "spam allow")
}

// SpamBlockCmd is the sender block-list. A blocked sender is classified as spam
// outright, and blacklist entries already auto-create spam rules -- which is
// why these belong under spam rather than beside it.
func SpamBlockCmd(session *Session) clihelp.Command {
	return blistCmd(session, "block", "spam block")
}

// RuleRemoteCmd lists the rules the server itself enforces, under rule -- where
// a reader looking for "what rules are in force" will look.
func RuleRemoteCmd(session *Session) clihelp.Command {
	return remoteCmd(session, "remote", "rule remote")
}

// WlistShortcutCmd and blistShortcutCmd keep `wlist` and `blist` working.
//
// The `whitelist` and `blacklist` spellings they used to hang off were
// deprecated duplicates and are gone. These two are not duplicates: they are
// real commands, hidden from help, that reach the same place `spam allow` and
// `spam block` do -- the shape `ss` and `sb` already use. A shortcut that is a
// real command is one help and completion can see; a shortcut that is an argv
// rewrite is invisible to both.
func WlistShortcutCmd(session *Session) clihelp.Command {
	cmd := wlistCmd(session, "wlist", "wlist")
	cmd.Hidden = true
	cmd.Description = "Shortcut for `spam allow`."
	return cmd
}

func blistShortcutCmd(session *Session) clihelp.Command {
	cmd := blistCmd(session, "blist", "blist")
	cmd.Hidden = true
	cmd.Description = "Shortcut for `spam block`."
	return cmd
}
