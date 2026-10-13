package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sarielhp/mail_cli/app"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/cli"
	"github.com/sarielhp/mail_cli/usage"

	"github.com/sarielhp/clihelp"
)

// Configuration loading happens after flag parsing, so the no-write choice
// follows the parser's handling of --, values, and repeated boolean options.
func initializeRuntime(ctx context.Context, opts cli.StartupOptions) (*cfg_g.Config, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("initialize session: %w", err)
	}
	loadConfig := cfg_g.LoadConfig
	if opts.NoConfigWrite {
		loadConfig = cfg_g.LoadConfigNoWrite
	}
	config, err := loadConfig()
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("detect home directory: %w", err)
	}
	config.ConfigDir = filepath.Join(homeDir, ".config", app.AppName)
	app.InitLogger(config.ConfigDir, opts.AppendLogs)
	return config, nil
}

func buildCLI() *clihelp.App {
	session := buildSession(&cfg_g.Config{})
	session.Bootstrap = initializeRuntime
	cliApp := cli.InitCLI(session)
	cliApp.Commands = append(cliApp.Commands, usage.DocumentationCommand())
	docsCmd := &cliApp.Commands[len(cliApp.Commands)-1]
	beforeRun := cliApp.BeforeRun
	cliApp.BeforeRun = func(ctx *clihelp.Context) error {
		if ctx.Command == docsCmd || (ctx.Command != nil && ctx.Command.Name == "version") {
			return nil
		}
		return beforeRun(ctx)
	}
	return cliApp
}

// Exit codes: 0 success, 1 a runtime failure, 2 the command line was wrong.
//
// Separating the last one lets a script tell "I asked for something
// impossible" from "the server refused", which are different things to retry.
const (
	exitRuntimeError = 1
	exitUsageError   = 2
)

func exitCodeFor(err error) int {
	// Naming an account that does not exist, or one that names two, is a wrong
	// command line rather than a failure to carry it out. cfg_g cannot reach
	// app.ErrUsage -- app imports cfg_g -- so main, which sees both, classifies
	// them here.
	var unknownAccount *cfg_g.UnknownAccountError
	var ambiguousAccount *cfg_g.AmbiguousAccountError

	switch {
	case errors.Is(err, clihelp.ErrUsage),
		errors.Is(err, app.ErrUsage),
		errors.Is(err, cfg_g.ErrNoSenderAccount),
		errors.As(err, &unknownAccount),
		errors.As(err, &ambiguousAccount):
		return exitUsageError
	default:
		return exitRuntimeError
	}
}

func main() {
	cliApp := buildCLI()
	args, err := normalizeAccountArgs(cliApp, os.Args[1:])
	if err == nil {
		err = cliApp.Execute(args)
	}
	// Execute can fail after initialization. Close before os.Exit, which does
	// not run defers; success-only AfterRun hooks cannot guarantee cleanup.
	app.CloseLogger()
	if err != nil {
		app.PrintError(err)
		os.Exit(exitCodeFor(err))
	}
}
