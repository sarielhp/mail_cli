package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/sarielhp/mail_cli/app"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/scan"
)

func TestReadOnlyFlag_Setting(t *testing.T) {
	app.FlagReadOnly = false

	session := &Session{
		Config: &cfg_g.Config{},
		Deps: Deps{
			Mail: stubMail{
				ScanFn: func(_ scan.Options, _ *cfg_g.Config, _ string) (int, error) {
					return 0, nil
				},
			},
		},
	}

	cliApp := InitCLI(session)
	_ = cliApp.Execute([]string{"--read-only", "scan", "%inbox"})

	if !session.Config.ReadOnly {
		t.Errorf("expected session.Config.ReadOnly to be true with --read-only flag")
	}

	// --dry-run is the wider mode and sets ReadOnly with it.
	app.FlagReadOnly, app.FlagDryRun = false, false
	session.Config.ReadOnly, session.Config.DryRun = false, false
	if err := cliApp.Execute([]string{"--dry-run", "scan", "%inbox"}); err != nil {
		t.Errorf("unexpected error for --dry-run: %v", err)
	}
	if !session.Config.ReadOnly || !session.Config.DryRun {
		t.Errorf("expected --dry-run to set both DryRun and ReadOnly, got %v/%v",
			session.Config.DryRun, session.Config.ReadOnly)
	}

	// --ro was a hidden alias for --read-only and is gone. It must be refused,
	// not silently ignored: a script still passing it is asking for a promise
	// the program would otherwise not keep.
	app.FlagReadOnly = false
	session.Config.ReadOnly = false
	err := cliApp.Execute([]string{"--ro", "scan", "%inbox"})
	if err == nil {
		t.Fatal("--ro was accepted; it has been removed")
	}
	if session.Config.ReadOnly {
		t.Error("--ro was refused but still set ReadOnly")
	}
}

func TestReadOnly_SpamDelBlocked(t *testing.T) {
	session := &Session{
		Config: &cfg_g.Config{
			ReadOnly: true,
		},
	}

	cliApp := InitCLI(session)
	err := cliApp.Execute([]string{"spam", "del"})

	if err == nil {
		t.Fatal("expected error for spam del in read-only mode, got nil")
	}

	if !strings.Contains(err.Error(), "cannot permanently delete messages while in read-only mode") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// TestBareInvocationDoesNotScan guards against the regression where running
// mail_cli with no arguments performed a full inbox scan across every
// configured account -- applying routing rules and moving mail server-side.
func TestBareInvocationDoesNotScan(t *testing.T) {
	rec := &sessionRecorder{}
	session := recordingSession(rec)

	cliApp := InitCLI(session)
	var out bytes.Buffer
	cliApp.Stdout = &out
	if err := cliApp.Execute([]string{}); err != nil {
		t.Fatalf("bare invocation returned an error: %v", err)
	}
	if rec.scanned {
		t.Error("bare invocation must not run a scan")
	}
	if rec.prechecked {
		t.Error("bare invocation must not contact the server for a pre-flight check")
	}
	if out.Len() == 0 {
		t.Error("bare invocation should render help")
	}
}
