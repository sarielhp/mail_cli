package cli

import (
	"github.com/sarielhp/clihelp"
)

// ConfigCmd returns the clihelp.Command for the config command.
func ConfigCmd(session *Session) clihelp.Command {
	showRun := func(_ *clihelp.Context) error {
		return session.configOps().Show(session.Config)
	}

	return clihelp.Command{
		Name:        "config",
		Group:       "Setup",
		Description: "Show or manage configuration options.",
		UsageLine:   "mail_cli config [subcommand]",
		Run:         showRun,
		Subcommands: []clihelp.Command{
			{
				Name:        "show",
				Title:       "config show",
				Description: "Display the active configuration values.",
				UsageLine:   "mail_cli config show",
				Args:        clihelp.NoArgs,
				Run:         showRun,
				Examples: []clihelp.Example{
					{Line: "mail_cli config show"},
					{Line: "mail_cli config"},
				},
			},
			{
				Name:        "get",
				Title:       "config get <key>",
				Description: "Print a configuration parameter's value.",
				UsageLine:   "mail_cli config get <key>",
				Parameters: []clihelp.Param{
					{Name: "<key>", Description: "The configuration parameter name.", Complete: CompleteConfigKeys},
				},
				Examples: []clihelp.Example{
					{Line: "mail_cli config get mutt_alias_file"},
					{Line: "mail_cli config get browser"},
					{Line: "mail_cli config get download_dir"},
				},
				Args: clihelp.ExactArgs(1),
				Run: func(ctx *clihelp.Context) error {
					return session.configOps().Get(session.Config, ctx.Args[0])
				},
			},
			{
				Name:            "set",
				Title:           "config set <key> <value>",
				Description:     "Set a configuration parameter value.",
				LongDescription: "Set a configuration parameter to a new value. Keys that belong to an account are written to the account -A selects.",
				UsageLine:       "mail_cli config set <key> <value>",
				Parameters: []clihelp.Param{
					{Name: "<key>", Description: "The configuration parameter name.", Complete: CompleteConfigKeys},
					{Name: "<value>", Description: "The value to set for the parameter."},
				},
				Examples: []clihelp.Example{
					{Line: "mail_cli config set spam_folder Junk"},
					{Line: "mail_cli config set limit 50"},
					{Line: "mail_cli -A work config set badge 💼", Description: "Tag the work account in All Inboxes"},
					{Line: "mail_cli config set mutt_alias_file ~/.mutt/aliases"},
					{Line: "mail_cli config set llm_endpoint http://localhost:4000/v1/chat/completions"},
					{Line: "mail_cli config set llm_api_key sk-litellm-master-key"},
					{Line: "mail_cli config set llm_auto_summary true"},
					{Line: "mail_cli config set attachment_save_dir ~/info/misc/%YY/%MM/%DD/"},
				},
				// Two arguments, exactly. RangeArgs(2, 3) existed for a third
				// positional that selected an account override and was never
				// read, so a stray word vanished without a word. The account is
				// chosen with -A, like everywhere else.
				Args: clihelp.ExactArgs(2),
				Run: func(ctx *clihelp.Context) error {
					args := ctx.Args
					return session.configOps().Set(session.Config, args[0], args[1])
				},
			},
			{
				Name:        "unset",
				Aliases:     []string{"reset"},
				Title:       "config unset <key>",
				Description: "Reset a configuration parameter to default.",
				UsageLine:   "mail_cli config unset <key>",
				Parameters: []clihelp.Param{
					{Name: "<key>", Description: "The configuration parameter name to reset.", Complete: CompleteConfigKeys},
				},
				Examples: []clihelp.Example{
					{Line: "mail_cli config unset score_threshold"},
					{Line: "mail_cli config unset mutt_alias_file"},
				},
				Args: clihelp.ExactArgs(1),
				Run: func(ctx *clihelp.Context) error {
					key := ctx.Args[0]
					return session.configOps().Reset(session.Config, key)
				},
			},
			{
				Name:            "check",
				Aliases:         []string{"validate"},
				Title:           "config check",
				Description:     "Validate config, DNS, and Bogofilter.",
				LongDescription: "Validate configurations, account parameters, DNS reachability, and Bogofilter service.",
				UsageLine:       "mail_cli config check [--fix]",
				Args:            clihelp.NoArgs,
				Options: []clihelp.Option{
					clihelp.Bool(&flagConfigFix, "--fix", false,
						"Repair a received_folder set to \"inbox\" instead of only reporting it."),
				},
				Run: func(_ *clihelp.Context) error {
					// The repair used to ride on a persistent --fix, so every
					// command carried it and any of them could rewrite the
					// config file from Validate(). Only the command whose job is
					// repair sets it now.
					session.Config.FixConfig = flagConfigFix
					return session.configOps().Validate(session.Config)
				},
				Examples: []clihelp.Example{
					{Line: "mail_cli config check"},
					{Line: "mail_cli config check --fix", Description: "Repair what the check reports"},
				},
			},
		},
		Examples: []clihelp.Example{
			{Line: "mail_cli config show"},
			{Line: "mail_cli config get mutt_alias_file"},
			{Line: "mail_cli config set mutt_alias_file ~/.mutt/aliases"},
			{Line: "mail_cli config set spam_folder Junk"},
			{Line: "mail_cli config unset score_threshold"},
			{Line: "mail_cli config check"},
		},
	}
}
