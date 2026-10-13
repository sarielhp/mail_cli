package cli

import (
	"fmt"
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"strings"

	"github.com/sarielhp/mail_cli/cache/folderindex"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/contacts"
	"github.com/sarielhp/mail_cli/label"

	"github.com/sarielhp/clihelp"
)

func runMarkSpam(session *Session, args []string) error {
	for _, id := range args {
		if err := session.spam().MarkByID(session.Config, id); err != nil {
			return err
		}
	}
	return nil
}

func newSpamDelCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:            "empty",
		NoAbbrev:        true,
		Aliases:         []string{"del", "delete"},
		Title:           "spam empty",
		Description:     "Permanently purge all emails in Spam.",
		LongDescription: "Permanently delete all messages currently residing in the remote Spam folder. Unlike archiving or trashing, this operation is immediate and irreversible on the server.",
		UsageLine:       "mail_cli spam empty [--yes]",
		Args:            clihelp.NoArgs,
		Options: []clihelp.Option{
			clihelp.Bool(&flagSpamDelYes, "-y, --yes", false, "Proceed without the confirmation prompt."),
		},
		Run: func(_ *clihelp.Context) error {
			if session.Config.ReadOnly {
				return fmt.Errorf("cannot permanently delete messages while in read-only mode")
			}
			client, err := GetValidatedClient(session)
			if err != nil {
				return err
			}
			if err := session.Confirm("permanently delete every message in the Spam folder", flagSpamDelYes); err != nil {
				return declined(session, err)
			}
			return client.DeleteAllSpam()
		},
		Examples: []clihelp.Example{
			{Line: "mail_cli spam empty --yes", Description: "Permanently purge the Spam folder"},
		},
	}
}

func newSpamByeCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:            "sweep",
		Aliases:         []string{"bye"},
		Title:           "spam sweep",
		Description:     "Sweep, unsubscribe, train, and purge Spam.",
		LongDescription: "Execute a complete sweep: unsubscribe from political lists, train spam filters on all messages in Spam, and purge the Spam folder.",
		UsageLine:       "mail_cli spam sweep",
		Args:            clihelp.NoArgs,
		Run: func(_ *clihelp.Context) error {
			client, err := GetValidatedClient(session)
			if err != nil {
				return err
			}
			destLabel := session.Config.SpamLearn
			if destLabel == "" {
				destLabel = "LearnSpam"
			}
			if session.Config.ReadOnly {
				fmt.Fprintf(session.out(), "[DRY-RUN] Would mark spam and move to %s\n", destLabel)
				return nil
			}
			if err := client.ShowPoliticalSpam(true); err != nil {
				return err
			}
			if err := client.LearnSpam(); err != nil {
				return err
			}
			purgeSpamFolderSenders(session.Config, client.Config().AccountCache)
			return client.MoveAllSpam(label.FromServer(destLabel))
		},
		Examples: []clihelp.Example{
			{Line: "mail_cli spam sweep", Description: "Move spam out of the inbox and train the filter"},
		},
	}
}

func purgeSpamFolderSenders(config *cfg_g.Config, downloadDir cachedir.Account) {
	for _, folder := range []string{"[Gmail]/Spam", "Spam", "spam", "Junk", "Junk Email"} {
		ids, err := folderindex.IDs(downloadDir, folder)
		if err == nil && len(ids) > 0 {
			contacts.PurgeMessageSenders(config, downloadDir, ids)
		}
	}
}

func newSpamLearnCmd(session *Session) clihelp.Command {
	hamOpt := clihelp.String(&flagLearnHamFolder, "--ham <label>", "",
		"Train on known-good mail, not Spam.")
	hamOpt.Complete = CompleteLabelRefs

	return clihelp.Command{
		Name:            "learn",
		Title:           "spam learn",
		Description:     "Train Bogofilter on Spam messages.",
		LongDescription: "Train Bogofilter on all messages currently in the Spam folder, then move them to the LearnSpam folder.",
		UsageLine:       "mail_cli spam learn [dest_label]",
		Parameters: []clihelp.Param{
			{Name: "[dest_label]", Description: "Destination label (default: SpamLearn folder)", Complete: CompleteLabelRefs},
		},
		Args: clihelp.MaximumNArgs(1),
		Options: []clihelp.Option{
			hamOpt,
			clihelp.Bool(&flagForceLearnHam, "--force", false,
				"Bypass the trained-message ledger and re-train everything."),
		},
		Run: func(ctx *clihelp.Context) error {
			if flagLearnHamFolder != "" {
				t, err := session.ResolveFolder(flagLearnHamFolder)
				if err != nil {
					return err
				}
				return session.spam().LearnHam(t.Config, t.Folder, flagForceLearnHam)
			}
			destRef := ""
			if len(ctx.Args) > 0 {
				destRef = ctx.Args[0]
			}
			return runLearnSpam(session, destRef)
		},
		Examples: []clihelp.Example{
			{Line: "mail_cli spam learn", Description: "Train the filter on everything in the Spam folder"},
			{Line: "mail_cli spam learn --ham %received", Description: "Train on a folder of known-good mail"},
			{Line: "mail_cli spam unmark abc123de", Description: "This one was not spam: train as ham and move it back"},
			{Line: "mail_cli spam allow add friend@example.com", Description: "Never treat this sender as spam"},
			{Line: "mail_cli spam block add spammer@example.com", Description: "Always treat this sender as spam"},
			{Line: "mail_cli spam learn %Archive/Spam"},
		},
	}
}

// runLearnSpam trains on the Spam folder and files what it trained on. A
// destination reference also picks the account; without one, the selected
// account's configured SpamLearn folder is used.
func runLearnSpam(session *Session, destRef string) error {
	t, err := learnSpamDest(session, destRef)
	if err != nil {
		return err
	}
	if err := t.Client.Validate(); err != nil {
		return err
	}
	if err := t.Client.LearnSpam(); err != nil {
		return err
	}
	return t.Client.MoveAllSpam(t.Folder)
}

func learnSpamDest(session *Session, destRef string) (FolderTarget, error) {
	if destRef != "" {
		return session.ResolveDestFolder(destRef)
	}
	client, err := session.clients().ForSelected(session.Config)
	if err != nil {
		return FolderTarget{}, err
	}
	dest := session.Config.SpamLearn
	if dest == "" {
		dest = "LearnSpam"
	}
	return FolderTarget{Config: session.Config, Client: client, Folder: label.FromServer(dest)}, nil
}

// politicalSpamSpec names one `spam pol` subcommand: the wording it shows and
// whether it only reports or actually sends opt-outs.
type politicalSpamSpec struct {
	name        string
	description string
	longDesc    string
	usageLine   string
	exampleDesc string
	act         bool
}

func newPoliticalSpamCmd(session *Session, spec politicalSpamSpec) clihelp.Command {
	return clihelp.Command{
		Name:            spec.name,
		Description:     spec.description,
		LongDescription: spec.longDesc,
		UsageLine:       spec.usageLine,
		Args:            clihelp.NoArgs,
		Run: func(_ *clihelp.Context) error {
			client, err := GetValidatedClient(session)
			if err != nil {
				return err
			}
			return client.ShowPoliticalSpam(spec.act)
		},
		Examples: []clihelp.Example{
			{Line: spec.usageLine, Description: spec.exampleDesc},
		},
	}
}

func newSpamPolCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:            "pol",
		Title:           "spam pol",
		Description:     "Manage political spam and opt-outs.",
		LongDescription: "Inspect or unsubscribe from political fundraising emails in Spam. 'audit' inspects scoring details without mutating anything; 'unsub' sends opt-out requests and purges the emails.",
		UsageLine:       "mail_cli spam pol <subcommand>",
		Subcommands: []clihelp.Command{
			newPoliticalSpamCmd(session, politicalSpamSpec{
				name:        "audit",
				description: "Scan Spam for political mail scoring.",
				longDesc:    "Scan Spam folder for political fundraising emails and print scoring details.",
				usageLine:   "mail_cli spam pol audit",
				exampleDesc: "Report political mail in Spam, changing nothing",
				act:         false,
			}),
			newPoliticalSpamCmd(session, politicalSpamSpec{
				name:        "unsub",
				description: "Scan Spam and execute opt-outs.",
				longDesc:    "Scan the Spam folder for political messages and execute unsubscription opt-outs.",
				usageLine:   "mail_cli spam pol unsub",
				exampleDesc: "Send opt-outs and delete what was unsubscribed from",
				act:         true,
			}),
		},
		Examples: []clihelp.Example{
			{Line: "mail_cli spam pol audit", Description: "Report political mail in Spam, changing nothing"},
			{Line: "mail_cli spam pol unsub", Description: "Send opt-outs and delete what was unsubscribed from"},
		},
	}
}

// SpamCmd returns the clihelp.Command for the spam command.
func SpamCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:            "spam",
		Group:           "Spam",
		Description:     "Manage Spam folder and train filters.",
		LongDescription: "Manage Spam folder, train filters, and unsubscribe from political mail.",
		UsageLine:       "mail_cli spam <subcommand>\nmail_cli spam <message_id...>",
		Parameters: []clihelp.Param{
			{Name: "<message_id...>", Description: "One or more message IDs to mark as spam.", Variadic: true},
		},
		Examples: []clihelp.Example{
			{Line: "mail_cli spam empty --yes", Description: "Permanently purge the Spam folder"},
			{Line: "mail_cli spam sweep", Description: "Move spam out of the inbox and train the filter"},
			{Line: "mail_cli spam learn", Description: "Train the filter on everything in the Spam folder"},
			{Line: "mail_cli spam learn --ham %received", Description: "Train on a folder of known-good mail"},
			{Line: "mail_cli spam unmark abc123de", Description: "This one was not spam: train as ham and move it back"},
			{Line: "mail_cli spam allow add friend@example.com", Description: "Never treat this sender as spam"},
			{Line: "mail_cli spam block add spammer@example.com", Description: "Always treat this sender as spam"},
			{Line: "mail_cli spam pol audit", Description: "Report political mail in Spam, changing nothing"},
			{Line: "mail_cli spam pol unsub", Description: "Send opt-outs and delete what was unsubscribed from"},
			{Line: "mail_cli spam mark abc123de", Description: "Mark one message as spam"},
		},
		Run: func(ctx *clihelp.Context) error {
			if len(ctx.Args) > 0 {
				return runMarkSpam(session, ctx.Args)
			}
			ctx.App.RenderCommand(clihelp.Options{Writer: ctx.Stdout}, "spam")
			return nil
		},
		Subcommands: []clihelp.Command{
			newSpamDelCmd(session),
			newSpamByeCmd(session),
			newSpamLearnCmd(session),
			newSpamPolCmd(session),
			SpamAllowCmd(session),
			SpamBlockCmd(session),
			SpamUnmarkCmd(session),
			spamResetCmd(session),
			spamMarkCmd(session),
			spamForgetCmd(session),
		},
	}
}

// spamResetCmd discards the local classifications, one level shallower than
// "learning reset" and under the noun it belongs to.
func spamResetCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:            "reset",
		NoAbbrev:        true,
		Title:           "spam reset",
		Description:     "Discard local spam scores for account.",
		LongDescription: "Discard every local spam classification and score for this account.",
		UsageLine:       "mail_cli spam reset",
		Args:            clihelp.NoArgs,
		Run: func(_ *clihelp.Context) error {
			return session.spam().ResetLearning(session.Config)
		},
		Examples: []clihelp.Example{
			{Line: "mail_cli spam reset", Description: "Forget what the filter learned locally and re-classify"},
		},
	}
}

// spamForgetCmd drops the user's spam/not-spam decision, the only way to
// hand a message back to the filter once it has one.
func spamForgetCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:        "forget",
		Title:       "spam forget <message_id...|%folder>",
		Description: "Let the filter judge messages again.",
		LongDescription: "Drop your spam/not-spam decision on messages, made by mark, unmark or learn --ham, " +
			"so the next scan judges them. Training is kept and nothing changes on the server; " +
			"a %folder reference covers every message the cache lists in that folder.",
		UsageLine: "mail_cli spam forget <message_id...|%folder>",
		Parameters: []clihelp.Param{
			{Name: "<message_id...|%folder>", Description: "Message IDs, or one folder reference.", Variadic: true, Complete: CompleteLabelRefs},
		},
		Args: clihelp.MinimumNArgs(1),
		Run: func(ctx *clihelp.Context) error {
			if len(ctx.Args) == 1 && strings.HasPrefix(ctx.Args[0], cfg_g.LabelSigil) {
				t, err := session.ResolveFolder(ctx.Args[0])
				if err != nil {
					return err
				}
				return session.spam().ForgetFolder(t.Config, t.Folder)
			}
			return session.spam().ForgetByID(session.Config, ctx.Args)
		},
		Examples: []clihelp.Example{
			{Line: "mail_cli spam forget abc123de", Description: "Let the filter decide about this one again"},
			{Line: "mail_cli spam forget %received", Description: "Undo a learn --ham on a whole folder"},
		},
	}
}

// spamMarkCmd names what `spam <id>` does, so mark and unmark read as the pair
// they are. The bare form keeps working.
func spamMarkCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:            "mark",
		Title:           "spam mark <message_id...>",
		Description:     "Mark messages as spam and move to Spam.",
		LongDescription: "Mark one or more messages as spam: train the filter and move them to the Spam folder.",
		UsageLine:       "mail_cli spam mark <message_id...>",
		Parameters: []clihelp.Param{
			{Name: "<message_id...>", Description: "One or more message IDs to mark as spam.", Variadic: true},
		},
		Args: clihelp.MinimumNArgs(1),
		Run: func(ctx *clihelp.Context) error {
			return runMarkSpam(session, ctx.Args)
		},
		Examples: []clihelp.Example{
			{Line: "mail_cli spam mark abc123de", Description: "Mark one message as spam"},
		},
	}
}

// SpamSweepShortcutCmd is `sb`, the shortcut for `spam sweep`.
func SpamSweepShortcutCmd(session *Session) clihelp.Command {
	cmd := newSpamByeCmd(session)
	cmd.Name = "sb"
	cmd.Aliases = nil
	cmd.Group = "Spam"
	cmd.Hidden = true
	cmd.Title = "sb"
	cmd.UsageLine = "mail_cli sb"
	cmd.Description = "Shortcut for `spam sweep`."
	cmd.Examples = []clihelp.Example{
		{Line: "mail_cli sb", Description: "Move spam out of the inbox and train the filter"},
	}
	return cmd
}
