package cli

import (
	"fmt"
	"github.com/sarielhp/mail_cli/app"
	"github.com/sarielhp/mail_cli/last"
	"strconv"

	"github.com/sarielhp/clihelp"
)

// LastCmd returns the clihelp.Command for the last command.
func LastCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:            "last",
		Group:           "Daily",
		Description:     "Show recent emails across all folders.",
		LongDescription: "Check all folders in the account, collect the last N emails received, save them to a virtual mailbox, and print each email with its folder name.",
		UsageLine:       "mail_cli msg last <N>",
		Parameters: []clihelp.Param{
			{Name: "<N>", Description: "Number of most recent emails to retrieve and display."},
		},
		Examples: []clihelp.Example{
			{Line: "mail_cli msg last 10"},
			{Line: "mail_cli msg last 25"},
			{Line: "mail_cli -A work-jmap msg last 20"},
		},
		Args: clihelp.ExactArgs(1),
		Run: func(ctx *clihelp.Context) error {
			args := ctx.Args
			n, err := strconv.Atoi(args[0])
			if err != nil || n <= 0 {
				return fmt.Errorf("%w: invalid number of emails %q (must be a positive integer)", app.ErrUsage, args[0])
			}
			return session.mail().Last(last.Options{JSON: wantJSON()}, session.Config, n)
		},
	}
}
