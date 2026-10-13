package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/sarielhp/mail_cli/app"

	"github.com/sarielhp/clihelp"
)

// ReplyCmd returns the clihelp.Command for replying to an email.
func ReplyCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:            "reply",
		Group:           "Messages",
		Description:     "Reply to an email preserving thread headers.",
		LongDescription: "Reply to an email with auto-quoting, Re: subject, and threading headers.",
		UsageLine:       "mail_cli msg reply [flags] <message-id>",
		Parameters: []clihelp.Param{
			{Name: "<message-id>", Description: "Message ID or prefix hash of the email to reply to."},
		},
		Options: []clihelp.Option{
			clihelp.Bool(&flagReplyGroup, "-g, --group", false, "Group reply (reply to all recipients)."),
			clihelp.String(&flagReplySubject, "-s, --subject <text>", "", "Override reply subject line."),
			clihelp.String(&flagReplyCc, "-c, --cc <address>", "", "Additional Cc recipient address (comma-separated)."),
			clihelp.String(&flagReplyBcc, "-b, --bcc <address>", "", "Bcc recipient (comma-separated)."),
			clihelp.String(&flagReplyAttach, "-a, --attach <path>", "", "Attachment file path (comma-separated for multiples)."),
			clihelp.String(&flagReplyInclude, "-i, --include <path>", "", "Path to file containing reply body."),
			clihelp.String(&flagReplyBody, "--body <text>", "", "Inline reply body text."),
			clihelp.Bool(&flagReplyNoQuote, "--no-quote", false, "Omit quoted text from original email."),
			clihelp.Bool(&flagReplyHeadless, "--headless", false, "Send directly without interactive editor or TUI review."),
		},
		Args: clihelp.ExactArgs(1),
		OptionsValidator: clihelp.ValidateOptions(
			clihelp.MutuallyExclusive("body", "include"),
		),
		Run: func(ctx *clihelp.Context) error {
			return runReply(session, ctx.Args)
		},
		Examples: []clihelp.Example{
			{Line: `mail_cli msg reply 18fb1234abcd`, Description: "Reply interactively to message in editor and review screen"},
			{Line: `mail_cli msg reply -g 18fb1234abcd`, Description: "Group reply (reply-all) interactively to all recipients"},
			{Line: `mail_cli msg reply --headless --body "Thanks, received!" 18fb1234abcd`, Description: "Send reply non-interactively with inline body"},
			{Line: `mail_cli msg reply --headless -i reply.txt 18fb1234abcd`, Description: "Send reply non-interactively reading body from file"},
			{Line: `mail_cli msg reply --headless 18fb1234abcd < reply.txt`, Description: "Send reply non-interactively reading body from stdin pipe"},
			{Line: `mail_cli msg reply --no-quote -s "New topic" 18fb1234abcd`, Description: "Reply without quoting original message with custom subject"},
		},
	}
}

func runReply(session *Session, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("%w: reply requires a message ID", app.ErrUsage)
	}
	msgID := strings.TrimSpace(args[0])
	if msgID == "" {
		return fmt.Errorf("%w: reply requires a non-empty message ID", app.ErrUsage)
	}

	body, err := resolveReplyBody(flagReplyHeadless)
	if err != nil {
		return err
	}

	selectedAcc := ""
	dryRun := false
	if session != nil && session.Config != nil {
		selectedAcc = string(session.Config.SelectedAccount)
		dryRun = session.Config.DryRun
	}

	opts := ReplyOptions{
		MessageID:   msgID,
		Account:     selectedAcc,
		Group:       flagReplyGroup,
		Subject:     flagReplySubject,
		Cc:          splitCommaList(flagReplyCc),
		Bcc:         splitCommaList(flagReplyBcc),
		Attachments: splitCommaList(flagReplyAttach),
		Body:        body,
		IncludeFile: flagReplyInclude,
		Headless:    flagReplyHeadless,
		NoQuote:     flagReplyNoQuote,
		DryRun:      dryRun,
	}

	return session.send().Reply(session.Config, opts)
}

func resolveReplyBody(headless bool) (string, error) {
	if flagReplyBody != "" {
		return flagReplyBody, nil
	}
	if flagReplyInclude != "" {
		bytes, err := os.ReadFile(flagReplyInclude)
		if err != nil {
			return "", fmt.Errorf("%w: failed to read included reply file %s: %v", app.ErrUsage, flagReplyInclude, err)
		}
		return string(bytes), nil
	}

	stat, err := os.Stdin.Stat()
	if err == nil && (stat.Mode()&os.ModeCharDevice) == 0 {
		bytes, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("reading reply body from stdin: %w", err)
		}
		return string(bytes), nil
	}

	if headless {
		return "", fmt.Errorf("%w: --headless reply requires body via stdin pipe, -i/--include <path>, or --body <text>", app.ErrUsage)
	}
	return "", nil
}
