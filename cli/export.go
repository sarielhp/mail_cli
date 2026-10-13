package cli

import (
	"fmt"
	"strings"

	"github.com/sarielhp/mail_cli/app"

	"github.com/sarielhp/clihelp"
)

// ExportCmd returns the clihelp.Command for exporting emails or threads to mbox/eml.
func ExportCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:            "export",
		Group:           "Messages",
		Description:     "Export emails or threads to mbox or eml.",
		LongDescription: "Export an email or conversation thread to standard mbox or eml format.",
		UsageLine:       "mail_cli msg export [flags] <message-id>",
		Parameters: []clihelp.Param{
			{Name: "<message-id>", Description: "Message ID or prefix hash of an email in the thread to export."},
		},
		Options: []clihelp.Option{
			clihelp.String(&flagExportOutput, "-o, --output <path>", "", "Output file (mbox) or directory (eml)."),
			clihelp.Enum(&flagExportFormat, "-f, --format <format>", []string{"mbox", "eml"}, "mbox", "Export format: 'mbox' (default) or 'eml'."),
			clihelp.Bool(&flagExportSingle, "--single", false, "Export only this message, not its thread."),
		},
		Args: clihelp.ExactArgs(1),
		Run: func(ctx *clihelp.Context) error {
			return runExport(session, ctx.Args)
		},
		Examples: []clihelp.Example{
			{Line: "mail_cli msg export 18fb1234abcd > thread.mbox", Description: "Export full thread to mbox file via stdout"},
			{Line: "mail_cli msg export -o thread.mbox 18fb1234abcd", Description: "Export full thread directly to mbox file"},
			{Line: "mail_cli msg export --format eml -o ./emails/ 18fb1234abcd", Description: "Export each message in thread to individual .eml files"},
			{Line: "mail_cli msg export --single 18fb1234abcd | git am", Description: "Pipe single message patch directly into git am"},
			{Line: "mail_cli msg export --dry-run 18fb1234abcd", Description: "Preview thread messages and sizes without writing"},
		},
	}
}

func runExport(session *Session, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("%w: export requires a message ID", app.ErrUsage)
	}
	msgID := strings.TrimSpace(args[0])
	if msgID == "" {
		return fmt.Errorf("%w: export requires a non-empty message ID", app.ErrUsage)
	}

	selectedAcc := ""
	dryRun := false
	if session != nil && session.Config != nil {
		selectedAcc = string(session.Config.SelectedAccount)
		dryRun = session.Config.DryRun
	}

	format := strings.ToLower(strings.TrimSpace(flagExportFormat))
	if format == "" {
		format = "mbox"
	}
	if format != "mbox" && format != "eml" {
		return fmt.Errorf("%w: invalid format %q: must be 'mbox' or 'eml'", app.ErrUsage, format)
	}

	opts := ExportOptions{
		MessageID: msgID,
		Account:   selectedAcc,
		Output:    flagExportOutput,
		Format:    format,
		Single:    flagExportSingle,
		DryRun:    dryRun,
	}

	return session.mail().Export(session.Config, opts)
}
