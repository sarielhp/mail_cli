package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/term"

	"github.com/sarielhp/mail_cli/app"
	"github.com/sarielhp/mail_cli/cfg_g"

	"github.com/sarielhp/clihelp"
)

// TuiCmd returns the clihelp.Command for the tui command.
func TuiCmd(session *Session) clihelp.Command {
	write := rewriteCommandHelp(ComposeCmd(session), strings.NewReplacer("mail_cli write", "mail_cli tui write"))
	write.Group = ""
	return clihelp.Command{
		Name:            "tui",
		Group:           "Daily",
		Description:     "Open interactive terminal email browser.",
		LongDescription: "Open the interactive terminal email browser. With an optional label_prefix argument, open the TUI with the matching label as the initial folder. The prefix is matched case-insensitively as a substring against the full label path. If exactly one label matches, the TUI opens on that label. If multiple match, all matching labels are printed and the program exits. Given an existing .eml file instead, open that one message read-only, as mbox view does. Use tui write to compose a message; it accepts the same arguments and flags as write, including --resume.",
		UsageLine:       "mail_cli tui [label_prefix]\n  mail_cli tui <file.eml>\n  mail_cli tui write [flags] [recipient...]",
		Subcommands:     []clihelp.Command{write},
		Parameters: []clihelp.Param{
			{Name: "[label_prefix]", Description: "Folder/label prefix to open into, or a .eml file to view.", Complete: CompleteLabelRefs},
		},
		Examples: []clihelp.Example{
			{Line: "mail_cli tui"},
			{Line: "mail_cli tui %receipts"},
			{Line: "mail_cli tui saved.eml", Description: "View one saved message, read-only"},
			{Line: "mail_cli tui write boss@example.com -s Update", Description: "Compose through a wrapper that opens the TUI"},
		},
		Args: clihelp.MaximumNArgs(1),
		Run: func(ctx *clihelp.Context) error {
			if len(ctx.Args) > 0 && isEmlFile(ctx.Args[0]) {
				return runMboxView(session, ctx.Args[0])
			}
			if session != nil && session.Config != nil && cfg_g.NeedsOnboarding(session.Config) {
				if term.IsTerminal(int(os.Stdin.Fd())) {
					fmt.Fprintf(session.errOut(), "%s No email accounts configured yet.\n", app.PrefixWarn)
					if promptYesNo("Would you like to run the setup wizard now?", true) {
						if err := runSetup(session); err != nil {
							return err
						}
					} else {
						return cfg_g.ErrNoAccounts
					}
				} else {
					return fmt.Errorf("%w: run 'mail_cli account setup' to configure an account", cfg_g.ErrNoAccounts)
				}
			}

			labelPrefix := ""
			initialNotice := ""
			if len(ctx.Args) > 0 && ctx.Args[0] != "" {
				// The argument is a label reference like everywhere else. This
				// used to take it raw, so completion -- which offers the %
				// form -- suggested arguments the command then rejected.
				ref, err := cfg_g.ParseLabelRef(ctx.Args[0])
				if err != nil {
					return err
				}
				prefix, notice, err := resolveTuiFolderPrefix(session, ref.Label)
				if err != nil {
					return err
				}
				labelPrefix = prefix
				initialNotice = notice
			}

			if initialNotice != "" {
				return session.tui().Run(session.Config, labelPrefix, initialNotice)
			}
			return session.tui().Run(session.Config, labelPrefix)
		},
	}
}

func resolveTuiFolderPrefix(session *Session, arg string) (string, string, error) {
	matches, err := SearchLabels(session.Config, []string{arg})
	if err != nil {
		return "", fmt.Sprintf("Labels cache not available; opening TUI without folder filter %q", arg), nil
	}
	if len(matches) == 0 {
		return "", "", fmt.Errorf("%w: no folder matches %q. Run %q to list folders",
			clihelp.ErrUsage, arg, app.AppName+" labels")
	}
	resolved := ResolveUniqueMatch(arg, matches)
	if resolved == "" {
		return "", "", fmt.Errorf("%w: %d folders match %q: %s",
			clihelp.ErrUsage, len(matches), arg, strings.Join(matches, ", "))
	}
	return resolved, "", nil
}

// isEmlFile reports whether arg names an existing .eml file, which tui opens
// in place of a folder. Requiring the file to exist keeps label prefixes
// working: a label that happens to end in .eml still resolves as one.
func isEmlFile(arg string) bool {
	if !strings.EqualFold(filepath.Ext(arg), ".eml") {
		return false
	}
	fi, err := os.Stat(arg)
	return err == nil && fi.Mode().IsRegular()
}
