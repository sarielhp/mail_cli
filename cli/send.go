package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/sarielhp/mail_cli/app"
	"github.com/sarielhp/mail_cli/contacts"

	"github.com/sarielhp/clihelp"
)

// SendCmd returns the clihelp.Command for sending outbound emails.
func SendCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:            "send",
		NoAbbrev:        true,
		Group:           "Messages",
		Description:     "Send an email directly or from stdin.",
		LongDescription: "Send an email via configured account or standard input pipe, with optional attachments and Cc/Bcc recipients.",
		UsageLine:       "mail_cli msg send [flags] <recipient...>",
		Parameters: []clihelp.Param{
			{Name: "<recipient...>", Description: "One or more recipient email addresses or contact aliases.", Variadic: true, Complete: completeRecipients(session)},
		},
		Options: []clihelp.Option{
			clihelp.String(&flagSendSubject, "-s, --subject <text>", "", "Email subject line."),
			withCompleter(clihelp.String(&flagSendCc, "-c, --cc <address>", "", "Carbon copy (Cc) recipient address (comma-separated)."), completeRecipients(session)),
			withCompleter(clihelp.String(&flagSendBcc, "-b, --bcc <address>", "", "Bcc recipient (comma-separated)."), completeRecipients(session)),
			withCompleter(clihelp.StringSlice(&flagSendAttach, "-a, --attach <path>", nil, "Attach files; repeat, comma-separate, or quote a glob."), completeAttachments),
			clihelp.String(&flagSendInclude, "-i, --include <path>", "", "Path to file containing email body."),
			clihelp.String(&flagSendBody, "--body <text>", "", "Inline email body text."),
			clihelp.Bool(&flagSendRaw, "-t, --raw", false, "Read a full RFC822 message from stdin."),
			clihelp.Bool(&flagSendQueue, "-q, --queue", false, "Queue message in outbox without sending."),
			clihelp.Bool(&flagSendSign, "--sign", false, "PGP-sign the message body."),
			clihelp.Bool(&flagSendEncrypt, "--encrypt", false, "PGP-encrypt; fails unless all recipients have keys."),
			clihelp.String(&flagSendPgpKey, "--pgp-key <id>", "", "Key to sign with (default: account key, else gpg's)."),
		},
		OptionsValidator: clihelp.ValidateOptions(
			clihelp.MutuallyExclusive("body", "include", "raw"),
		),
		Run: func(ctx *clihelp.Context) error {
			return runSend(session, ctx.Args)
		},
		Examples: []clihelp.Example{
			{Line: `mail_cli msg send -s "Meeting notes" user@example.com < notes.txt`, Description: "Send email with body from stdin pipe"},
			{Line: `mail_cli msg send -s "Quick update" --body "Everything on track" boss@company.com`, Description: "Send quick email with inline body"},
			{Line: `mail_cli msg send -s "Report" -a report.pdf -c team@company.com boss < body.txt`, Description: "Send with attachment, CC, and alias"},
			{Line: `mail_cli msg send -s "Dry run test" --dry-run user@example.com < body.txt`, Description: "Preview RFC822 message without sending"},
			{Line: `mail_cli msg send --raw < patch.eml`, Description: "Send pre-formatted RFC822 message directly (git send-email mode)"},
		},
	}
}

func runSend(session *Session, recipients []string) error {
	if flagSendRaw {
		if flagSendSign || flagSendEncrypt {
			return fmt.Errorf("%w: --sign and --encrypt apply to a message this command builds; a --raw message must be signed or encrypted before it is piped in", app.ErrUsage)
		}
		return runSendRaw(session, recipients)
	}
	if len(recipients) == 0 {
		return fmt.Errorf("%w: send requires at least one recipient", app.ErrUsage)
	}

	body, err := resolveSendBody()
	if err != nil {
		return err
	}
	// send has no review screen: an unquoted -a *.pdf would mail the other
	// matches' names as addresses the moment it ran.
	attachments, err := expandAttachments(flagSendAttach)
	if err != nil {
		return err
	}
	book, _ := contacts.LoadBook(contacts.DefaultPath(sessionCfg(session)))
	if err := checkRecipientsAreNotFiles(book, recipients); err != nil {
		return err
	}

	selectedAcc := ""
	dryRun := false
	if session != nil && session.Config != nil {
		selectedAcc = string(session.Config.SelectedAccount)
		dryRun = session.Config.DryRun
	}

	opts := SendOptions{
		To:          recipients,
		Cc:          splitCommaList(flagSendCc),
		Bcc:         splitCommaList(flagSendBcc),
		Attachments: attachments,
		Subject:     flagSendSubject,
		Body:        body,
		Account:     selectedAcc,
		DryRun:      dryRun,
		Queue:       flagSendQueue,
		Sign:        flagSendSign,
		Encrypt:     flagSendEncrypt,
		PgpKeyID:    flagSendPgpKey,
	}

	return session.send().Send(session.Config, opts)
}

func runSendRaw(session *Session, recipients []string) error {
	rawBytes, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("reading raw message from stdin: %w", err)
	}
	if len(strings.TrimSpace(string(rawBytes))) == 0 {
		return fmt.Errorf("%w: --raw requires RFC822 message on stdin", app.ErrUsage)
	}

	selectedAcc := ""
	dryRun := false
	if session != nil && session.Config != nil {
		selectedAcc = string(session.Config.SelectedAccount)
		dryRun = session.Config.DryRun
	}

	opts := SendOptions{
		To:       recipients,
		Account:  selectedAcc,
		DryRun:   dryRun,
		Raw:      true,
		RawBytes: rawBytes,
		Queue:    flagSendQueue,
	}

	return session.send().Send(session.Config, opts)
}

func resolveSendBody() (string, error) {
	if flagSendBody != "" {
		return flagSendBody, nil
	}
	if flagSendInclude != "" {
		data, err := os.ReadFile(flagSendInclude)
		if err != nil {
			return "", fmt.Errorf("reading body file %q: %w", flagSendInclude, err)
		}
		return string(data), nil
	}

	stat, err := os.Stdin.Stat()
	if err == nil && (stat.Mode()&os.ModeCharDevice) == 0 {
		bytes, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("reading body from stdin: %w", err)
		}
		return string(bytes), nil
	}

	return "", fmt.Errorf("%w: no message body provided; pipe body via stdin (< file.txt), use -i/--include <path>, or pass --body <text>", app.ErrUsage)
}

func splitCommaList(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	res := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			res = append(res, trimmed)
		}
	}
	return res
}
