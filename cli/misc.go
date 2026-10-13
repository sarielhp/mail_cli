package cli

import (
	"strings"

	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/show"

	"github.com/sarielhp/clihelp"
)

// ScanCmd returns the clihelp.Command for the scan command.
func ScanCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:            "scan",
		Group:           "Daily",
		Description:     "Scan folders matching label prefix for spam.",
		LongDescription: "Scan all folders starting with the given label prefix (case-insensitive) for spam emails using Bogofilter.",
		UsageLine:       "mail_cli scan <lbl_prefix> [flags]",
		Parameters: []clihelp.Param{
			{Name: "<lbl_prefix>", Description: "Label prefix to scan (e.g. inbox)", Complete: CompleteLabelRefs},
		},
		// Two flags, because they are two things. One string with an optional
		// value carried a boolean as the literal "true" and an address
		// otherwise, and every reader downstream had to decode it. The set is
		// shared with the root's bare `%label` form, which runs the same helper.
		Options: scanOptions(),
		Examples: []clihelp.Example{
			{Line: "mail_cli scan inbox"},
			{Line: "mail_cli scan inbox -m"},
			{Line: "mail_cli scan receipts --move-from spammer@example.com"},
		},
		Args: clihelp.MaximumNArgs(1),
		Run: func(ctx *clihelp.Context) error {
			args := ctx.Args
			if len(args) == 0 {
				ctx.App.RenderCommand(clihelp.Options{Writer: ctx.Stdout}, "scan")
				return nil
			}
			// The bare form `mail_cli %inbox` runs the same helper, so the two
			// spellings cannot disagree. They did: this copy compared the raw
			// argument against "inbox" to decide whether the inbox had been
			// named explicitly, which stopped being true the moment a label
			// carried its sigil -- so `scan %inbox` silently lost the cached
			// classification clearing that naming the inbox is what licenses.
			return scanLabelRef(session, args[0])
		},
	}
}

// ShowCmd returns the clihelp.Command for the show command.
func ShowCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:        "show",
		Group:       "Daily",
		Description: "Show emails by label prefix or message ID.",
		UsageLine:   "mail_cli msg show <lbl_prefix|message-id> [message-id] [flags]",
		Parameters: []clihelp.Param{
			{Name: "<lbl_prefix|message-id>", Description: "Folder prefix, or a message ID on its own", Complete: CompleteLabelRefs},
			{Name: "[message-id]", Description: "Message ID or prefix within the folder"},
		},
		Options: []clihelp.Option{
			clihelp.Bool(&flagShowWeb, "-w, --web", false, "Open the HTML body of the email in your configured browser"),
			clihelp.Bool(&flagShowRaw, "--raw", false, "Output verbatim unformatted RFC822 message to stdout"),
		},
		Examples: []clihelp.Example{
			{Line: "mail_cli msg show receipts"},
			{Line: "mail_cli msg show receipts 1234abcd"},
			{Line: "mail_cli msg show receipts 1234abcd --web"},
			{Line: "mail_cli msg show 1234abcd --raw", Description: "Output raw RFC822 message for piping"},
			{Line: "mail_cli msg show 19f3a8216d52dfc6", Description: "Show a message by ID alone, downloading it if needed"},
		},
		Notes: []clihelp.Note{
			{
				Heading: "JSON output",
				Text: "With --json a folder listing is one JSON array with an element per matched " +
					"folder in every account -- {account, folder, message_ids, total} -- even when " +
					"only one folder matches, so the output always parses. A message shown by ID " +
					"is a single JSON object.",
			},
		},
		Args: clihelp.RangeArgs(1, 2),
		Run: func(ctx *clihelp.Context) error {
			args := ctx.Args
			labelPrefix := args[0]
			targetMsgID := ""
			if len(args) > 1 {
				targetMsgID = args[1]
			}
			return session.mail().Show(show.Options{Web: flagShowWeb, Raw: flagShowRaw, JSON: wantJSON()}, session.Config, labelPrefix, targetMsgID)
		},
	}
}

// testCmd builds the diagnostic under the name it is mounted at.
func testCmd(session *Session, name string) clihelp.Command {
	runTest := func(_ *clihelp.Context) error {
		return session.mail().RunBackendTests(session.Config)
	}

	return clihelp.Command{
		Name:            name,
		Group:           "Other",
		Description:     "Run integration tests for credentials.",
		LongDescription: "Run system and integration self-tests to verify API credentials and mail flow. Creates and deletes a temporary label and message in the account.",
		UsageLine:       "mail_cli utils " + name + " run",
		Run:             runTest,
		Subcommands: []clihelp.Command{
			{
				Name:        "run",
				Description: "Execute connection and integration tests.",
				UsageLine:   "mail_cli utils " + name + " run",
				Args:        clihelp.NoArgs,
				Run:         runTest,
				Examples: []clihelp.Example{
					{Line: "mail_cli utils " + name + " run"},
				},
			},
		},
		Examples: []clihelp.Example{
			{Line: "mail_cli utils " + name + " run"},
		},
	}
}

// UnspamCmd returns the clihelp.Command for the unspam command.
func unmarkCmd(session *Session, name, path string) clihelp.Command {
	return clihelp.Command{
		Name:            name,
		Group:           "Spam",
		Description:     "Mark email as ham and return to Inbox.",
		LongDescription: "Mark a message as not being spam: train Bogofilter as ham and move it from Spam back to Inbox on the server.",
		UsageLine:       "mail_cli " + path + " <message_id...>\n  mail_cli unspam folder <folder_name>",
		Parameters: []clihelp.Param{
			{Name: "<message_id...>", Description: "One or more message IDs to unspam (short 8-char or full).", Variadic: true},
		},
		Subcommands: []clihelp.Command{
			{
				Name:            "folder",
				Title:           path + " folder",
				Description:     "Mark all messages in folder as ham.",
				LongDescription: "Mark all messages in the specified folder as ham and move them back to Inbox.",
				UsageLine:       "mail_cli " + path + " folder <folder_name>",
				Parameters: []clihelp.Param{
					{Name: "<folder_name>", Description: "Folder name containing emails to unspam.", Complete: CompleteLabelRefs},
				},
				Args: clihelp.ExactArgs(1),
				Run: func(ctx *clihelp.Context) error {
					return unspamFolderRef(session, ctx.Args[0])
				},
				Examples: []clihelp.Example{
					{Line: "mail_cli " + path + " folder %spam"},
				},
			},
		},
		Args: clihelp.MinimumNArgs(1),
		Run: func(ctx *clihelp.Context) error {
			args := ctx.Args
			// A label carries its sigil, so one argument that has it names a
			// folder and anything else is a message ID. This used to look the
			// argument up as a bare folder name first, which a sigil-carrying
			// reference never matched, so `unmark %archive` went on to be
			// treated as a message ID.
			if len(args) == 1 && strings.HasPrefix(args[0], cfg_g.LabelSigil) {
				return unspamFolderRef(session, args[0])
			}

			for _, arg := range args {
				if err := session.spam().UnspamByID(session.Config, arg); err != nil {
					return err
				}
			}
			return nil
		},
		Examples: []clihelp.Example{
			{Line: "mail_cli " + path + " abc123de"},
			{Line: "mail_cli " + path + " folder %spam"},
		},
	}
}

// unspamFolderRef unspams every message in the folder a reference names.
func unspamFolderRef(session *Session, ref string) error {
	t, err := session.ResolveFolder(ref)
	if err != nil {
		return err
	}
	return session.spam().UnspamFolder(t.Config, t.Folder)
}

// SpamUnmarkCmd is the counterpart to marking a message as spam, under the same
// noun: mark and unmark are a pair, where "unspam" had no partner.
func SpamUnmarkCmd(session *Session) clihelp.Command {
	return unmarkCmd(session, "unmark", "spam unmark")
}

// DoctorCmd diagnoses the setup: credentials, connectivity, mail flow. "test"
// read like a test suite, which is what it is not.
func DoctorCmd(session *Session) clihelp.Command {
	cmd := testCmd(session, "doctor")
	cmd.Description = "Diagnose credentials, connectivity, mail flow."
	cmd.LongDescription = "Diagnose the setup: verify credentials, connectivity and mail flow. It proves the account is writable by creating a label and a message and then deleting both, so it is refused under --read-only and --dry-run."
	return cmd
}

// ScanSpamCmd is `ss`, the shortcut for scanning the spam folder. A shortcut
// that is a real command is one help and completion can see; this used to be an
// argv rewrite in preprocessArgs, invisible to both.
func ScanSpamCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:        "ss",
		Group:       "Daily",
		Hidden:      true,
		Description: "Shortcut for `scan spam`.",
		UsageLine:   "mail_cli ss",
		Args:        clihelp.NoArgs,
		Run: func(_ *clihelp.Context) error {
			return scanLabelRef(session, "spam")
		},
		Examples: []clihelp.Example{
			{Line: "mail_cli ss", Description: "Scan the spam folder"},
		},
	}
}
