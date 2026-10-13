package cli

import (
	"github.com/sarielhp/clihelp"
)

// ArcCmd returns the clihelp.Command for the archive command.
func ArcCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:        "archive",
		Group:       "Messages",
		Aliases:     []string{"arc"},
		Hidden:      true,
		Description: "Archive messages by ID or folder prefix.",
		UsageLine:   "mail_cli archive <all [label] | message-id...>",
		Parameters: []clihelp.Param{
			{Name: "all [label]", Description: "Archive all emails in the Inbox or specified label prefix."},
			{Name: "<message-id...>", Description: "One or more message IDs to archive (short 8-char or full).", Variadic: true},
		},
		Examples: []clihelp.Example{
			{Line: "mail_cli archive abc123de"},
			{Line: "mail_cli archive all"},
			{Line: "mail_cli archive all receipts"},
		},
		Args: clihelp.MinimumNArgs(1),
		Run: func(ctx *clihelp.Context) error {
			args := ctx.Args

			// The old name did two things depending on this word, which is
			// exactly why it was split. Each half names its replacement.
			if args[0] == "all" {
				if len(args) > 2 {
					return ErrInvalidArgs("archive all", "archive all [label]")
				}
				ref := ""
				if len(args) > 1 {
					ref = args[1]
				}
				session.status().Warnf("%q is now %q. The old spelling still works and will be removed in a future release.",
					"archive all", "label archive")
				return archiveFolder(session, ref)
			}

			session.status().Warnf("%q is now %q. The old spelling still works and will be removed in a future release.",
				"archive <id>", "msg archive")
			return archiveByIDs(session, args)
		},
	}
}
