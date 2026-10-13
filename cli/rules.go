package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/sarielhp/mail_cli/app"
	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/email"

	"github.com/sarielhp/clihelp"
)

func runRuleRoot(ctx *clihelp.Context, session *Session) error {
	if flagRuleExportFile != "" {
		return session.rules().Export(session.Config, flagRuleExportFile)
	}
	if flagRuleImportFile != "" {
		return session.rules().Import(session.Config, flagRuleImportFile)
	}
	ctx.App.RenderCommand(clihelp.Options{Writer: ctx.Stdout}, "rule")
	return nil
}

func newRuleAddCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:            "add",
		Title:           "rule add <email|message_id> [lbl]",
		Description:     "Add rule by sender or message ID.",
		LongDescription: "Add a server filter rule to auto-label emails from a sender or from a cached message ID.",
		UsageLine:       "mail_cli rule add <email|message_id> [lbl]",
		Parameters: []clihelp.Param{
			{Name: "<email|message_id>", Description: "The sender email address or cached message ID."},
			{Name: "[lbl]", Description: "Target label (optional with message_id).", Complete: CompleteLabelRefs},
		},
		Args: clihelp.RangeArgs(1, 2),
		Run: func(ctx *clihelp.Context) error {
			arg0 := ctx.Args[0]
			label, err := ruleLabelArg(session, ctx.Args, 1)
			if err != nil {
				return err
			}
			if email.IsValidEmailOrDomain(arg0) {
				if label == "" {
					return fmt.Errorf("target label required when adding rule by email address: mail_cli rule add %s <lbl>", arg0)
				}
				return addRuleToConfig(session, arg0, label, "")
			}
			return addRuleFromMessageID(session, arg0, label)
		},
		Examples: []clihelp.Example{
			{Line: `mail_cli rule add notifications@github.com %Dev/GitHub`},
			{Line: `mail_cli rule add 12345 %Dev/GitHub`},
			{Line: "mail_cli rule add 12345"},
		},
	}
}

func newRuleAddByTitleCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:            "add-title",
		Title:           "rule add-title <title> <lbl>",
		Description:     "Add rule matching subject prefix.",
		LongDescription: "Add a server filter rule to auto-label emails with matching subject prefix.",
		UsageLine:       "mail_cli rule add-title <title> <lbl>",
		Parameters: []clihelp.Param{
			{Name: "<title>", Description: "Subject prefix/substring to match."},
			{Name: "<lbl>", Description: "The target label/folder hierarchy.", Complete: CompleteLabelRefs},
		},
		Args: clihelp.ExactArgs(2),
		Run: func(ctx *clihelp.Context) error {
			label, err := ruleLabelArg(session, ctx.Args, 1)
			if err != nil {
				return err
			}
			return addRuleToConfig(session, "", label, ctx.Args[0])
		},
		Examples: []clihelp.Example{
			{Line: `mail_cli rule add-title "[JIRA]" %Work/Jira`},
		},
	}
}

func newRuleDelCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:            "rm",
		NoAbbrev:        true,
		Aliases:         []string{"del"},
		Title:           "rule del <email|title>",
		Description:     "Remove rule by sender or subject.",
		LongDescription: "Remove an auto-labeling rule by sender email address or subject title.",
		UsageLine:       "mail_cli rule rm <email|title>",
		Parameters: []clihelp.Param{
			{Name: "<email|title>", Description: "The sender email or subject title to remove rule for."},
		},
		Args: clihelp.ExactArgs(1),
		Run: func(ctx *clihelp.Context) error {
			return deleteRuleFromConfig(session, ctx.Args[0], os.Stdin)
		},
		Examples: []clihelp.Example{
			{Line: "mail_cli rule rm notifications@github.com"},
		},
	}
}

func newRuleListCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:            "list",
		Aliases:         []string{"ls"},
		Title:           "rule list",
		Description:     "List custom routing rules.",
		LongDescription: "List custom routing rules with their target labels and export status.",
		UsageLine:       "mail_cli rule list [flags]",
		Options: []clihelp.Option{
			clihelp.Bool(&flagRuleListAll, "-a, --all", false, "List all custom routing rules, including those already exported"),
		},
		Args: clihelp.NoArgs,
		Run: func(_ *clihelp.Context) error {
			return listRules(session)
		},
		Examples: []clihelp.Example{
			{Line: "mail_cli rule list"},
			{Line: "mail_cli rule list --all"},
		},
	}
}

// ruleActionSpec names a no-argument `rule` subcommand that runs an action.
type ruleActionSpec struct {
	name        string
	title       string
	description string
	longDesc    string
	usageLine   string
	action      func(*Session) error
}

func newRuleActionCmd(session *Session, spec ruleActionSpec) clihelp.Command {
	return clihelp.Command{
		Name:            spec.name,
		Title:           spec.title,
		Description:     spec.description,
		LongDescription: spec.longDesc,
		UsageLine:       spec.usageLine,
		Args:            clihelp.NoArgs,
		Run: func(_ *clihelp.Context) error {
			return spec.action(session)
		},
		Examples: []clihelp.Example{
			{Line: spec.usageLine},
		},
	}
}

func newRuleTidyCmd(session *Session) clihelp.Command {
	return newRuleActionCmd(session, ruleActionSpec{
		name:        "tidy",
		title:       "rule tidy",
		description: "Remove duplicate routing rules.",
		longDesc:    "Remove duplicate routing rules, keeping the last rule for each sender or subject.",
		usageLine:   "mail_cli rule tidy",
		action:      tidyRules,
	})
}

func newRuleDeleteAllCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:            "rm-all",
		Title:           "rule rm-all",
		Description:     "Delete all rules for account.",
		LongDescription: "Delete all routing rules for the selected account.",
		UsageLine:       "mail_cli rule rm-all [--yes]",
		Args:            clihelp.NoArgs,
		Options: []clihelp.Option{
			clihelp.Bool(&flagRuleDeleteAllYes, "-y, --yes", false, "Proceed without the confirmation prompt."),
		},
		Run: func(_ *clihelp.Context) error {
			if err := session.Confirm("delete every routing rule for this account", flagRuleDeleteAllYes); err != nil {
				return declined(session, err)
			}
			return deleteAllRules(session)
		},
		Examples: []clihelp.Example{
			{Line: "mail_cli rule rm-all"},
		},
	}
}

func newRuleUpdateCmd(session *Session) clihelp.Command {
	return newRuleActionCmd(session, ruleActionSpec{
		name:        "update",
		title:       "rule update",
		description: "Sync blacklisted senders to rules.",
		longDesc:    "Ensure all blacklisted senders have a corresponding local rule.",
		usageLine:   "mail_cli rule update",
		action:      syncBlacklistToRules,
	})
}

func newRuleExportCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:            "push",
		Aliases:         []string{"export"},
		Title:           "rule push",
		Description:     "Export local rules to server filters.",
		LongDescription: "Export local rules to mail server filters (e.g. Gmail filters or Sieve script).",
		UsageLine:       "mail_cli rule push [force] [flags]",
		Parameters: []clihelp.Param{
			{
				Name:        "[force]",
				Description: "Overwrite existing remote filters.",
				Complete: func(toComplete string) []string {
					const forceArg = "force"
					if strings.HasPrefix(forceArg, strings.ToLower(toComplete)) {
						return []string{"force\toverwrite conflicting remote filters"}
					}
					return nil
				},
			},
		},
		Options: []clihelp.Option{
			clihelp.String(&flagSieveExport, "--sieve <file>", "", "Export rules as a Sieve script to file path"),
			clihelp.Bool(&flagForceRuleExport, "-f, --force", false, "Force overwrite conflicting remote filters"),
		},
		Args: clihelp.MaximumNArgs(1),
		Run: func(ctx *clihelp.Context) error {
			args := ctx.Args
			if flagForceRuleExport || (len(args) > 0 && strings.EqualFold(args[0], "force")) {
				session.Config.ForceLearn = true
			}
			if flagSieveExport != "" {
				return session.rules().ExportSieve(session.Config, flagSieveExport)
			}
			client, err := GetValidatedClient(session)
			if err != nil {
				return err
			}
			return client.ExportRules()
		},
		Examples: []clihelp.Example{
			{Line: "mail_cli rule push"},
			{Line: "mail_cli rule push --sieve ~/rules.sieve"},
		},
	}
}

// RuleCmd returns the clihelp.Command for the rule command.
func RuleCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:            "rule",
		Group:           "Rules",
		Description:     "Manage routing rules and server filters.",
		LongDescription: "Configure local routing rules and sync them as remote server-side filters (e.g. Gmail filters or JMAP/Sieve scripts) to automatically label or discard incoming messages.",
		UsageLine:       "mail_cli rule <subcommand> [args...]",
		// No positional arguments of its own: that is what lets clihelp tell a
		// mistyped subcommand from an argument, and suggest the right one.
		Args: clihelp.NoArgs,
		Options: []clihelp.Option{
			clihelp.String(&flagRuleExportFile, "--export <file>", "", "Export all existing rules to a file"),
			clihelp.String(&flagRuleImportFile, "--import <file>", "", "Import rules from a file"),
		},
		Subcommands: []clihelp.Command{
			newRuleAddCmd(session),
			newRuleAddByTitleCmd(session),
			ruleAddDomainCmd(session),
			newRuleDelCmd(session),
			newRuleListCmd(session),
			newRuleTidyCmd(session),
			newRuleDeleteAllCmd(session),
			newRuleUpdateCmd(session),
			newRuleExportCmd(session),
			RuleRemoteCmd(session),
			hiddenRuleSubcommand(newRuleAddByTitleCmd(session), "add-by-title", "mail_cli rule add-by-title <title> <lbl>"),
			hiddenRuleSubcommand(newRuleDeleteAllCmd(session), "delete-all", "mail_cli rule delete-all [--yes]"),
			hiddenRuleSubcommand(newRuleDeleteAllCmd(session), "del-all", "mail_cli rule del-all [--yes]"),
			hiddenRuleSubcommand(newRuleDelCmd(session), "delete", "mail_cli rule delete <email|title>"),
		},
		Run: func(ctx *clihelp.Context) error {
			return runRuleRoot(ctx, session)
		},
		Examples: []clihelp.Example{
			{Line: "mail_cli rule list"},
			{Line: `mail_cli rule add notifications@github.com %Dev/GitHub`},
			{Line: `mail_cli rule add-title "[ALERT]" "Alerts"`},
			{Line: "mail_cli rule push"},
		},
	}
}

func hiddenRuleSubcommand(cmd clihelp.Command, name, usageLine string) clihelp.Command {
	cmd.Name = name
	cmd.Aliases = nil
	cmd.Hidden = true
	cmd.UsageLine = usageLine
	return cmd
}

// ruleLabelArg resolves the label argument at index i, if one was given, to
// the folder as the server spells it. A folder that does not exist yet is
// taken as written: a rule may be set up before the first message arrives.
//
// A rule belongs to the selected account, so a reference naming another one
// is refused rather than silently filed under the wrong account.
func ruleLabelArg(session *Session, args []string, i int) (string, error) {
	if len(args) <= i {
		return "", nil
	}
	raw := args[i]
	ref, err := cfg_g.ParseLabelRef(raw)
	if err != nil {
		return "", err
	}
	if ref.Account != "" {
		return "", fmt.Errorf("%w: rules belong to the selected account; use -A %s and write the label as %s%s",
			clihelp.ErrUsage, ref.Account, cfg_g.LabelSigil, ref.Label)
	}
	t, err := session.ResolveDestFolder(raw)
	if err != nil {
		return "", err
	}
	// Rules are stored as text in the config file.
	return t.Folder.String(), nil
}

func addRuleToConfig(session *Session, emailAddr, label, title string) error {
	resolved, err := cfg_g.Resolve(session.Config)
	if err != nil {
		return err
	}

	if emailAddr != "" {
		if !email.IsValidEmailOrDomain(emailAddr) {
			return fmt.Errorf("%q is not a valid email address or domain pattern (must contain '@' and valid domain)", emailAddr)
		}
		return upsertRule(session, resolved, cfg_acc.Rule{Sender: emailAddr, Label: label}, false)
	}
	return upsertRule(session, resolved, cfg_acc.Rule{Subject: title, Label: label}, true)
}

// upsertRule updates the label of the existing rule that matches newRule's
// sender or, when bySubject is set, its subject (marking it unexported), or
// appends newRule; either way it saves the config and reports what it did.
func upsertRule(session *Session, resolved cfg_g.Resolved, newRule cfg_acc.Rule, bySubject bool) error {
	desc := newRule.Sender
	if bySubject {
		desc = "title " + newRule.Subject
	}
	for i, rule := range resolved.Account.Rules {
		matches := strings.EqualFold(rule.Sender, newRule.Sender)
		if bySubject {
			matches = strings.EqualFold(rule.Subject, newRule.Subject)
		}
		if !matches {
			continue
		}
		oldLabel := rule.Label
		resolved.Account.Rules[i].Label = newRule.Label
		resolved.Account.Rules[i].Exported = false
		if err := cfg_g.SaveConfigFile(resolved.Path, resolved.File); err != nil {
			return err
		}
		fmt.Fprintf(session.errOut(), "%s Updated existing rule for %s: %s -> %s (Account: %s)\n", app.PrefixSuccess, desc, oldLabel, newRule.Label, resolved.Account.Name)
		return nil
	}
	resolved.Account.Rules = append(resolved.Account.Rules, newRule)
	if err := cfg_g.SaveConfigFile(resolved.Path, resolved.File); err != nil {
		return err
	}
	fmt.Fprintf(session.errOut(), "%s Added rule for %s -> %s (Account: %s)\n", app.PrefixSuccess, desc, newRule.Label, resolved.Account.Name)
	return nil
}

func deleteRuleFromConfig(session *Session, target string, stdin io.Reader) error {
	resolved, err := cfg_g.Resolve(session.Config)
	if err != nil {
		return err
	}

	var targetRuleIndex = -1
	var targetRule cfg_acc.Rule
	isIndexMatch := false

	// First, try matching as a 1-based index (rule number)
	if num, err := strconv.Atoi(target); err == nil && num > 0 {
		idx := num - 1
		if idx < len(resolved.Account.Rules) {
			targetRuleIndex = idx
			targetRule = resolved.Account.Rules[idx]
			isIndexMatch = true
		}
	}

	// If not matched as a valid index, search by sender or subject
	if targetRuleIndex == -1 {
		targetClean := strings.TrimSpace(target)
		targetAddr := strings.ToLower(email.ParseEmailAddress(targetClean))

		for i, r := range resolved.Account.Rules {
			senderClean := strings.TrimSpace(r.Sender)
			senderAddr := strings.ToLower(email.ParseEmailAddress(senderClean))

			if strings.EqualFold(senderClean, targetClean) ||
				strings.EqualFold(r.Subject, targetClean) ||
				(targetAddr != "" && (strings.EqualFold(senderAddr, targetAddr) || strings.EqualFold(senderClean, targetAddr))) {
				targetRuleIndex = i
				targetRule = r
				break
			}
		}
	}

	if targetRuleIndex == -1 {
		fmt.Fprintf(session.errOut(), "%s Rule %s not found.\n", app.PrefixWarn, target)
		return nil
	}

	// Show rule details
	ident := targetRule.Sender
	if targetRule.Subject != "" {
		ident = "[Subject: " + targetRule.Subject + "]"
	}
	status := "pending"
	if targetRule.Exported {
		status = "exported"
	}
	if targetRule.Internal {
		status = "internal"
	}

	if isIndexMatch {
		fmt.Fprintf(session.out(), "Rule %d: %s -> %s [%s]\n", targetRuleIndex+1, ident, targetRule.Label, status)

		// Ask for confirmation Y/N from the user
		fmt.Fprint(session.out(), "Delete this rule? (y/N): ")
		reader := bufio.NewReader(stdin)
		input, err := reader.ReadString('\n')
		if err != nil {
			return err
		}
		input = strings.TrimSpace(strings.ToLower(input))
		if input != "y" && input != "yes" {
			fmt.Fprintln(session.out(), "Cancelled.")
			return nil
		}
	}

	// Delete this rule
	resolved.Account.Rules = append(resolved.Account.Rules[:targetRuleIndex], resolved.Account.Rules[targetRuleIndex+1:]...)
	fmt.Fprintf(session.errOut(), "%s Deleted rule for %s on account %s.\n", app.PrefixSuccess, ident, resolved.Account.Name)
	return cfg_g.SaveConfigFile(resolved.Path, resolved.File)
}

// tidyRules deduplicates rules and persists the result. This is what
// 'rule list' used to do silently as a side effect of listing.
func tidyRules(session *Session) error {
	resolved, err := cfg_g.Resolve(session.Config)
	if err != nil {
		return err
	}
	kept, removed := dedupeRules(session.out(), resolved.Account.Rules, true)
	if removed == 0 {
		fmt.Fprintf(session.errOut(), "%s No duplicate rules found for account %s.\n", app.PrefixInfo, resolved.Account.Name)
		return nil
	}
	resolved.Account.Rules = kept
	if saveErr := cfg_g.SaveConfigFile(resolved.Path, resolved.File); saveErr != nil {
		return fmt.Errorf("failed to save config during rule deduplication: %w", saveErr)
	}
	fmt.Fprintf(session.errOut(), "%s Removed %d duplicate rule(s) for account %s.\n", app.PrefixSuccess, removed, resolved.Account.Name)
	return nil
}

// dedupeRules returns the rules with earlier duplicates dropped (the last rule
// for a given sender or subject wins) and the number removed. When report is
// true each removal is printed. It never writes to disk.
func dedupeRules(w io.Writer, rules []cfg_acc.Rule, report bool) ([]cfg_acc.Rule, int) {
	seen := make(map[string]int)
	keyOf := func(r cfg_acc.Rule) string {
		if r.Sender != "" {
			return "sender:" + strings.ToLower(r.Sender)
		}
		if r.Subject != "" {
			return "subject:" + strings.ToLower(r.Subject)
		}
		return ""
	}
	for i, r := range rules {
		if k := keyOf(r); k != "" {
			seen[k] = i
		}
	}
	var kept []cfg_acc.Rule
	removed := 0
	for i, r := range rules {
		k := keyOf(r)
		if k == "" || i == seen[k] {
			kept = append(kept, r)
			continue
		}
		removed++
		if report {
			ident := r.Sender
			if r.Subject != "" {
				ident = "[Subject: " + r.Subject + "]"
			}
			fmt.Fprintf(w, "%s Duplicate rule for '%s' -> %q (superseded by %q).\n",
				app.PrefixInfo, ident, r.Label, rules[seen[k]].Label)
		}
	}
	return kept, removed
}

func listRules(session *Session) error {
	resolved, err := cfg_g.Resolve(session.Config)
	if err != nil {
		return err
	}

	// Listing must not modify state. Report duplicates and let the user decide.
	// The notice is status, so it does not land in a redirected listing.
	if _, removed := dedupeRules(session.errOut(), resolved.Account.Rules, false); removed > 0 {
		fmt.Fprintf(session.errOut(), "%s %d duplicate rule(s) present. Run 'mail_cli rule tidy' to remove them.\n",
			app.PrefixWarn, removed)
	}

	if wantJSON() {
		return session.emitJSON(ruleListing(resolved.Account.Rules))
	}

	if len(resolved.Account.Rules) == 0 {
		fmt.Fprintf(session.out(), "No rules found for account %s.\n", resolved.Account.Name)
		return nil
	}

	fmt.Fprintf(session.out(), "Rules for account %s:\n", resolved.Account.Name)
	for i, r := range resolved.Account.Rules {
		if !flagRuleListAll && r.Exported {
			continue
		}
		status := "pending"
		if r.Exported {
			status = "exported"
		}
		if r.Internal {
			status = "internal"
		}
		ident := r.Sender
		if r.Subject != "" {
			ident = "[Subject: " + r.Subject + "]"
		}
		fmt.Fprintf(session.out(), "  %d. %s -> %s [%s]\n", i+1, ident, r.Label, status)
	}
	return nil
}

func deleteAllRules(session *Session) error {
	resolved, err := cfg_g.Resolve(session.Config)
	if err != nil {
		return err
	}

	resolved.Account.Rules = nil
	fmt.Fprintf(session.errOut(), "%s Deleted all rules for account %s.\n", app.PrefixSuccess, resolved.Account.Name)
	return cfg_g.SaveConfigFile(resolved.Path, resolved.File)
}

func syncBlacklistToRules(session *Session) error {
	resolved, err := cfg_g.Resolve(session.Config)
	if err != nil {
		return err
	}

	if len(resolved.Account.Blacklist) == 0 {
		fmt.Fprintf(session.out(), "No blacklisted senders for account %s.\n", resolved.Account.Name)
		return nil
	}

	if resolved.Account.SpamLearn == "" {
		return fmt.Errorf("no SpamLearn folder configured for account %s", resolved.Account.Name)
	}

	added := 0
	for _, emailAddr := range resolved.Account.Blacklist {
		found := false
		for _, r := range resolved.Account.Rules {
			if strings.EqualFold(r.Sender, emailAddr) {
				found = true
				break
			}
		}
		if !found {
			resolved.Account.Rules = append(resolved.Account.Rules, cfg_acc.Rule{Sender: emailAddr, Label: resolved.Account.SpamLearn})
			fmt.Fprintf(session.out(), "  + %s -> %s\n", emailAddr, resolved.Account.SpamLearn)
			added++
		}
	}

	if added > 0 {
		fmt.Fprintf(session.errOut(), "%s Added %d rule(s) for blacklisted sender(s) on account %s.\n", app.PrefixSuccess, added, resolved.Account.Name)
	} else {
		fmt.Fprintf(session.out(), "All blacklisted senders already have rules for account %s.\n", resolved.Account.Name)
	}
	return cfg_g.SaveConfigFile(resolved.Path, resolved.File)
}

// ruleJSON is one routing rule as a script sees it.
type ruleJSON struct {
	Sender   string `json:"sender,omitempty"`
	Subject  string `json:"subject,omitempty"`
	Label    string `json:"label"`
	Exported bool   `json:"exported"`
	Internal bool   `json:"internal"`
}

// ruleListing renders the rules for --json, honouring --all the same way the
// text listing does.
func ruleListing(rules []cfg_acc.Rule) []ruleJSON {
	out := make([]ruleJSON, 0, len(rules))
	for _, r := range rules {
		if !flagRuleListAll && r.Exported {
			continue
		}
		out = append(out, ruleJSON{
			Sender:   r.Sender,
			Subject:  r.Subject,
			Label:    r.Label,
			Exported: r.Exported,
			Internal: r.Internal,
		})
	}
	return out
}
