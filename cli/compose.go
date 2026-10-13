package cli

import (
	"fmt"
	"os"

	"github.com/sarielhp/mail_cli/app"
	"github.com/sarielhp/mail_cli/contacts"

	"github.com/sarielhp/clihelp"
)

// ComposeCmd returns the clihelp.Command for interactive email composition.
func ComposeCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:        "write",
		Group:       "Messages",
		Description: "Write a message in your editor, then review.",
		UsageLine:   "mail_cli write [flags] [recipient...]",
		Parameters: []clihelp.Param{
			{Name: "[recipient...]", Description: "Optional recipient email addresses or contact aliases.", Variadic: true, Complete: completeRecipients(session)},
		},
		Options: []clihelp.Option{
			clihelp.String(&flagComposeSubject, "-s, --subject <text>", "", "Email subject line."),
			withCompleter(clihelp.String(&flagComposeCc, "-c, --cc <address>", "", "Carbon copy (Cc) recipient address (comma-separated)."), completeRecipients(session)),
			withCompleter(clihelp.String(&flagComposeBcc, "-b, --bcc <address>", "", "Bcc recipient (comma-separated)."), completeRecipients(session)),
			withCompleter(clihelp.StringSlice(&flagComposeAttach, "-a, --attach <path>", nil, "Attach files; repeat, comma-separate, or quote a glob."), completeAttachments),
			clihelp.String(&flagComposeInclude, "-i, --include <path>", "", "Path to file containing initial email body."),
			clihelp.String(&flagComposeBody, "--body <text>", "", "Inline initial email body text."),
			clihelp.String(&flagComposeResume, "-r, --resume <id>", "", "Reopen a saved draft by ID (see 'drafts ls')."),
		},
		Args: clihelp.MinimumNArgs(0),
		OptionsValidator: clihelp.ValidateOptions(
			clihelp.MutuallyExclusive("body", "include"),
		),
		Run: func(ctx *clihelp.Context) error {
			return runCompose(session, ctx.Args)
		},
		Examples: []clihelp.Example{
			{Line: `mail_cli write`, Description: "Open interactive editor to compose a new message"},
			{Line: `mail_cli write boss@company.com -s "Status Report"`, Description: "Compose email with pre-filled recipient and subject"},
			{Line: `mail_cli write -s "Patch review" -a fix.patch dev-list`, Description: "Compose with attachment and contact alias"},
			{Line: `mail_cli write -i notes.txt colleague@example.com`, Description: "Compose starting with body from file in editor"},
			{Line: `mail_cli write -a "*.pdf" -a notes.txt boss@company.com`, Description: "Attach every PDF here, quoted so mail_cli expands it, and one more file"},
			{Line: `mail_cli write --resume 2026/09/30/125`, Description: "Reopen a saved draft on the review screen"},
		},
	}
}

func runCompose(session *Session, recipients []string) error {
	if flagComposeResume != "" && composeContentGiven(recipients) {
		return fmt.Errorf("%w: --resume reopens a saved draft as it was; it cannot be combined with recipients or -s, -c, -b, -a, -i or --body (change them on the review screen)", app.ErrUsage)
	}
	body, err := resolveComposeBody()
	if err != nil {
		return err
	}
	attachments, err := expandAttachments(flagComposeAttach)
	if err != nil {
		return err
	}
	book, _ := contacts.LoadBook(contacts.DefaultPath(sessionCfg(session)))
	if err := checkRecipientsAreNotFiles(book, recipients); err != nil {
		return err
	}

	selectedAcc := ""
	if session != nil && session.Config != nil {
		selectedAcc = string(session.Config.SelectedAccount)
	}

	opts := ComposeOptions{
		To:          recipients,
		Cc:          splitCommaList(flagComposeCc),
		Bcc:         splitCommaList(flagComposeBcc),
		Subject:     flagComposeSubject,
		Body:        body,
		Attachments: attachments,
		Account:     selectedAcc,
	}
	if flagComposeResume != "" {
		path, err := draftStore(session).Resolve(flagComposeResume)
		if err != nil {
			return fmt.Errorf("%w: %v", app.ErrUsage, err)
		}
		opts.DraftPath = path
	}

	return session.tui().Compose(session.Config, opts)
}

// composeContentGiven reports whether the command line supplied any part of
// the message, which --resume would otherwise silently ignore.
func composeContentGiven(recipients []string) bool {
	return len(recipients) > 0 || flagComposeSubject != "" || flagComposeCc != "" || flagComposeBcc != "" ||
		len(flagComposeAttach) > 0 || flagComposeInclude != "" || flagComposeBody != ""
}

func resolveComposeBody() (string, error) {
	if flagComposeBody != "" {
		return flagComposeBody, nil
	}
	if flagComposeInclude == "" {
		return "", nil
	}
	bytes, err := os.ReadFile(flagComposeInclude)
	if err != nil {
		return "", fmt.Errorf("%w: failed to read included body file %s: %v", app.ErrUsage, flagComposeInclude, err)
	}
	return string(bytes), nil
}
