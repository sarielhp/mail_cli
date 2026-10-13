package usage

import (
	"fmt"
	"os"

	"github.com/sarielhp/mail_cli/app"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/cli"

	"github.com/sarielhp/clihelp"
	"github.com/sarielhp/clihelp/doc"
)

// MarkdownOptions re-exports doc.MarkdownOptions from clihelp/doc.
type MarkdownOptions = doc.MarkdownOptions

// GetApp returns the global clihelp.App instance for mail_cli.
func GetApp() *clihelp.App {
	cliApp := cli.InitCLI(&cli.Session{Config: &cfg_g.Config{}})
	cliApp.Commands = append(cliApp.Commands, DocumentationCommand())
	return cliApp
}

// DocumentationCommand composes clihelp's Markdown command with mail_cli's
// source-package inventory. Neither needs an initialized runtime session.
func DocumentationCommand() clihelp.Command {
	cmd := doc.MarkdownCommand(doc.MarkdownOptions{Dir: "docs/clihelp"})
	cmd.UsageLine = "mail_cli docs [flags]"
	generateHelp := cmd.Run
	cmd.Run = func(ctx *clihelp.Context) error {
		if err := generateHelp(ctx); err != nil {
			return err
		}
		changed, err := ApplyPackageInventory(".", "docs/ARCHITECTURE.md")
		if err != nil {
			return fmt.Errorf("generate package inventory: %w", err)
		}
		if changed {
			fmt.Fprintln(ctx.Stderr, "package inventory updated in docs/ARCHITECTURE.md")
		}
		return nil
	}
	return cmd
}

// PrintUsage prints the global CLI help overview to os.Stderr.
func PrintUsage() {
	cliApp := GetApp()
	cliApp.Version = app.Version
	cliApp.RenderGlobal(clihelp.Options{Writer: os.Stderr})
}

// PrintDetailedUsage prints detailed usage for a command or subcommand to os.Stdout.
func PrintDetailedUsage(cmd, subcmd string) {
	var path []string
	if cmd != "" {
		path = append(path, cmd)
	}
	if subcmd != "" {
		path = append(path, subcmd)
	}
	cliApp := GetApp()
	cliApp.RenderCommand(clihelp.Options{Writer: os.Stdout}, path...)
}

// Render writes help text for the specified path to the given options destination.
func Render(opts clihelp.Options, path ...string) bool {
	cliApp := GetApp()
	if len(path) == 0 {
		cliApp.RenderGlobal(opts)
		return true
	}
	return cliApp.RenderCommand(opts, path...)
}

// RenderMarkdown generates markdown documentation in the given directory.
func RenderMarkdown(opts MarkdownOptions) (bool, error) {
	cliApp := GetApp()
	return doc.RenderMarkdown(cliApp, opts)
}
