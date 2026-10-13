package cli

import (
	"fmt"
	"strings"

	"github.com/sarielhp/mail_cli/app"

	"github.com/sarielhp/clihelp"
)

var (
	flagPipeBody        bool
	flagPipeHeadersOnly bool
)

// PipeCmd returns the clihelp.Command for piping a message to an external command.
func PipeCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:            "pipe",
		Group:           "Messages",
		Description:     "Pipe message raw RFC822 or body to command.",
		LongDescription: "Fetch the specified message and pipe its contents (raw RFC822 by default, or body text with --body, or headers with --headers-only) into the standard input of an external command.",
		UsageLine:       "mail_cli msg pipe [flags] <message-id> <command...>",
		Parameters: []clihelp.Param{
			{Name: "<message-id>", Description: "Message ID or prefix hash of the email to pipe."},
			{Name: "<command...>", Description: "Command and arguments to execute.", Variadic: true},
		},
		Options: []clihelp.Option{
			clihelp.Bool(&flagPipeBody, "--body", false, "Pipe only the extracted body text instead of raw RFC822."),
			clihelp.Bool(&flagPipeHeadersOnly, "--headers-only", false, "Pipe only the RFC822 headers."),
		},
		Args: clihelp.MinimumNArgs(2),
		Run: func(ctx *clihelp.Context) error {
			return runPipe(session, ctx.Args[0], ctx.Args[1:])
		},
		Examples: []clihelp.Example{
			{Line: "mail_cli msg pipe 18fb1234 git am", Description: "Pipe email directly to git am"},
			{Line: "mail_cli msg pipe --body 18fb1234 -- wc -w", Description: "Count words in message body"},
			{Line: "mail_cli msg pipe 18fb1234 'xclip -selection clipboard'", Description: "Copy raw RFC822 email to system clipboard"},
		},
	}
}

func runPipe(session *Session, msgID string, cmdArgs []string) error {
	msgID = strings.TrimSpace(msgID)
	if msgID == "" {
		return fmt.Errorf("%w: pipe requires a non-empty message ID", app.ErrUsage)
	}
	if len(cmdArgs) == 0 {
		return fmt.Errorf("%w: pipe requires a command to execute", app.ErrUsage)
	}

	selectedAcc := ""
	if session != nil && session.Config != nil {
		selectedAcc = string(session.Config.SelectedAccount)
	}

	opts := PipeOptions{
		MessageID:   msgID,
		Account:     selectedAcc,
		BodyOnly:    flagPipeBody,
		HeadersOnly: flagPipeHeadersOnly,
	}

	return session.mail().Pipe(session.Config, opts, cmdArgs)
}
