package cli

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sarielhp/mail_cli/app"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/watcher"

	"github.com/sarielhp/clihelp"
)

// setWatchFlags binds the watcher's package-level flags for one test and
// restores them afterwards, since clihelp writes into those variables.
func setWatchFlags(t *testing.T, interval time.Duration, account, onMail string, noNotify, noFlush bool) {
	t.Helper()
	prevInterval, prevAccount, prevOnMail := flagWatchInterval, app.FlagAccountName, flagWatchOnMail
	prevNoNotify, prevNoFlush := flagWatchNoNotify, flagWatchNoFlush
	t.Cleanup(func() {
		flagWatchInterval, app.FlagAccountName, flagWatchOnMail = prevInterval, prevAccount, prevOnMail
		flagWatchNoNotify, flagWatchNoFlush = prevNoNotify, prevNoFlush
	})
	flagWatchInterval, app.FlagAccountName, flagWatchOnMail = interval, account, onMail
	flagWatchNoNotify, flagWatchNoFlush = noNotify, noFlush
}

func TestWatchCommandIsRegistered(t *testing.T) {
	cliApp := InitCLI(&Session{Config: &cfg_g.Config{}})
	found := false
	_ = cliApp.Walk(func(_ []string, cmd *clihelp.Command) error {
		if cmd.Name == "watch" {
			found = true
			if cmd.Args == nil {
				t.Error("watch accepts arguments but declares no validator")
			}
			if len(cmd.Description) > 45 {
				t.Errorf("watch description is %d chars, over the 45-char budget: %q",
					len(cmd.Description), cmd.Description)
			}
		}
		return nil
	})
	if !found {
		t.Fatal("watch is not registered as a command")
	}
}

func TestRunWatchPassesOptionsToWatcher(t *testing.T) {
	setWatchFlags(t, watcher.MinInterval, "work", "/bin/true", true, true)

	var got watcher.Options
	var gotConfig *cfg_g.Config
	session := &Session{Config: &cfg_g.Config{SelectedAccount: "work"}}
	session.Deps.Watch = stubWatch{WatchFn: func(_ context.Context, config *cfg_g.Config, opts watcher.Options) error {
		gotConfig, got = config, opts
		return nil
	}}

	if err := runWatch(session, nil); err != nil {
		t.Fatalf("runWatch: %v", err)
	}
	if gotConfig != session.Config {
		t.Fatal("runWatch did not pass the session config to the watcher")
	}
	if got.Interval != watcher.MinInterval || got.Account != "work" || got.OnMail != "/bin/true" {
		t.Fatalf("options = %+v, want the flags as given", got)
	}
	if !got.NoNotify || !got.NoFlush {
		t.Fatalf("options = %+v, want --no-notify and --no-flush honoured", got)
	}
}

func TestRunWatchRejectsIntervalBelowMinimum(t *testing.T) {
	setWatchFlags(t, time.Second, "", "", false, false)

	called := false
	session := &Session{Config: &cfg_g.Config{}}
	session.Deps.Watch = stubWatch{WatchFn: func(context.Context, *cfg_g.Config, watcher.Options) error {
		called = true
		return nil
	}}

	err := runWatch(session, nil)
	if !errors.Is(err, app.ErrUsage) {
		t.Fatalf("err = %v, want a usage error", err)
	}
	if called {
		t.Fatal("the watcher ran despite an invalid interval")
	}
}
