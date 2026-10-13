package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/sarielhp/mail_cli/app"
	"github.com/sarielhp/mail_cli/watcher"

	"github.com/sarielhp/clihelp"
)

// WatchCmd returns the clihelp.Command for the watch command.
func WatchCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:        "watch",
		Group:       "Manage",
		Description: "Watch accounts and notify on new mail.",
		LongDescription: "Poll every account in the background, notify on messages that arrived since the " +
			"last poll, run a script per message, and flush the offline outbox. It runs in the " +
			"foreground until interrupted; put it under systemd --user or in a tmux window to keep " +
			"it alive. The first poll of an account records what is already there without notifying.",
		UsageLine: "mail_cli watch [flags]",
		Options: []clihelp.Option{
			clihelp.Duration(&flagWatchInterval, "-i, --interval <duration>", watcher.DefaultInterval,
				"How often to poll (minimum 30s)"),
			clihelp.String(&flagWatchOnMail, "--on-mail <path>", "",
				"Run this program once per new message"),
			clihelp.Bool(&flagWatchNoNotify, "--no-notify", false,
				"Do not send desktop notifications"),
			clihelp.Bool(&flagWatchNoFlush, "--no-flush", false,
				"Do not flush the offline outbox"),
		},
		Notes: []clihelp.Note{
			{Heading: "Which accounts", Text: "Every account is watched by default; pass the global -A/--account to watch just one."},
			{Heading: "Notifications", Text: "New mail is announced as a desktop notification when notify-send is available, and as a log line otherwise."},
			{Heading: "The --on-mail hook", Text: "--on-mail passes the message in the environment: MAIL_CLI_ACCOUNT, MAIL_CLI_FROM, MAIL_CLI_SUBJECT, MAIL_CLI_DATE, MAIL_CLI_ID. A hook still running after 30 seconds is killed."},
			{Heading: "The first poll", Text: "The first poll of an account records existing messages as seen and announces nothing, so starting the watcher does not notify you about the whole inbox."},
			{Heading: "What is announced", Text: "Mail in spam, trash, sent and drafts folders is never announced, nor is mail dated more than six hours before the previous poll: an older message that came into view because newer ones were deleted or moved."},
			{Heading: "Failures", Text: "After a failed poll it waits longer each time, up to ten minutes, and reports the failure instead of exiting."},
		},
		Examples: []clihelp.Example{
			{Line: "mail_cli watch", Description: "Watch every account, notifying on new mail"},
			{Line: "mail_cli watch -A work --no-notify", Description: "Watch one account, log instead of notifying"},
			{Line: "mail_cli watch --on-mail ~/bin/on-mail.sh", Description: "Run a script for each new message"},
		},
		Args: clihelp.NoArgs,
		Run: func(ctx *clihelp.Context) error {
			return runWatch(session, ctx)
		},
	}
}

func runWatch(session *Session, ctx *clihelp.Context) error {
	if flagWatchInterval < watcher.MinInterval {
		return fmt.Errorf("%w: --interval must be at least %s", app.ErrUsage, watcher.MinInterval)
	}

	base := context.Background()
	if ctx != nil && ctx.Context != nil {
		base = ctx.Context
	}
	// The watcher runs until it is told to stop. Ctrl-C in a terminal and
	// SIGTERM from a supervisor both cancel this context, which the poll loop
	// notices between accounts and between sleeps.
	runCtx, stop := signal.NotifyContext(base, os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Watch every account unless one was named, by -A/--account or by
	// --account-index. Those are the CLI's own globals, and BeforeRun has
	// already folded either into Config.SelectedAccount; checking them here
	// keeps the default -- no flag -- from being read as "just the default
	// account".
	account := ""
	if app.FlagAccountName != "" || flagAccountIndex != "" {
		account = string(session.Config.SelectedAccount)
	}

	return session.watch().Watch(runCtx, session.Config, watcher.Options{
		Interval: flagWatchInterval,
		Account:  account,
		OnMail:   flagWatchOnMail,
		NoNotify: flagWatchNoNotify,
		NoFlush:  flagWatchNoFlush,
	})
}
