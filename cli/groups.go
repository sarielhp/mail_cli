package cli

import (
	"strings"

	"github.com/sarielhp/clihelp"
)

func buildAccountCommands(session *Session) clihelp.Command {
	cmd := AccountsCmd(session)
	cmd.Description = "Configure accounts and sign in."
	cmd.LongDescription = "Start with account setup for guided onboarding, or account new to add an account explicitly. Use account login to renew credentials and account migrate to copy configuration to another machine over SSH."
	cmd.Subcommands = append(cmd.Subcommands, SetupCmd(session), MigrateCmd(session))
	return cmd
}

func buildConfigCommands(session *Session) clihelp.Command {
	cmd := ConfigCmd(session)
	cmd.LongDescription = "Use config show, get, set, and unset for preferences. Run config check to validate configuration, or config llm test to check the configured model endpoint. Account credentials are managed under account; local maintenance lives under utils."
	cmd.Subcommands = append(cmd.Subcommands, LLMCmd(session))
	return cmd
}

func buildUtilsCommands(session *Session) clihelp.Command {
	color := ColorCmd(session)
	// Keep test-color as a hidden root shortcut; the concise name is color.
	color.Aliases = nil
	return clihelp.Command{
		Name:            "utils",
		Group:           "Setup",
		Description:     "Maintain cache and shell integration.",
		LongDescription: "Use utils cache for local download maintenance, utils color to inspect terminal colors, and utils completion or utils manpage for shell integration. utils doctor verifies server writes by creating and deleting a temporary folder and message; it changes the real account and refuses --read-only and --dry-run.",
		UsageLine:       "mail_cli utils <subcommand>",
		Subcommands: []clihelp.Command{
			CacheCmd(session),
			color,
			DoctorCmd(session),
			clihelp.CompletionCommand(),
			clihelp.ManPageCommand(),
		},
		Examples: []clihelp.Example{
			{Line: "mail_cli utils cache prune 30", Description: "Prune old local cached messages"},
			{Line: "mail_cli utils completion install", Description: "Install shell completion"},
			{Line: "mail_cli utils manpage", Description: "Print the manual page"},
		},
	}
}

// legacyCommand gives an old root spelling its original usage while keeping
// the canonical builder's flags, validators, and handler. Clone the help slices
// because changing compatibility help must not change the canonical subtree.
func legacyCommand(cmd clihelp.Command, path string) clihelp.Command {
	rewrite := strings.NewReplacer("mail_cli "+path, "mail_cli "+cmd.Name, path, cmd.Name)
	return rewriteCommandHelp(cmd, rewrite)
}

func rewriteCommandHelp(cmd clihelp.Command, rewrite *strings.Replacer) clihelp.Command {
	cmd.Title = rewrite.Replace(cmd.Title)
	cmd.UsageLine = rewrite.Replace(cmd.UsageLine)
	cmd.LongDescription = rewrite.Replace(cmd.LongDescription)
	cmd.Examples = append([]clihelp.Example(nil), cmd.Examples...)
	for i := range cmd.Examples {
		cmd.Examples[i].Line = rewrite.Replace(cmd.Examples[i].Line)
		cmd.Examples[i].Description = rewrite.Replace(cmd.Examples[i].Description)
	}
	cmd.Notes = append([]clihelp.Note(nil), cmd.Notes...)
	for i := range cmd.Notes {
		cmd.Notes[i].Text = rewrite.Replace(cmd.Notes[i].Text)
	}
	cmd.Subcommands = append([]clihelp.Command(nil), cmd.Subcommands...)
	for i := range cmd.Subcommands {
		cmd.Subcommands[i] = rewriteCommandHelp(cmd.Subcommands[i], rewrite)
	}
	return cmd
}
