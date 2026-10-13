package cli

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/sarielhp/mail_cli/app"
	"github.com/sarielhp/mail_cli/cfg_g"
)

type exitPanic struct {
	code int
}

func interceptExit(fn func()) (called bool, code int) {
	origExit := exitFunc
	defer func() { exitFunc = origExit }()

	defer func() {
		if r := recover(); r != nil {
			if ep, ok := r.(exitPanic); ok {
				called = true
				code = ep.code
				return
			}
			panic(r)
		}
	}()

	exitFunc = func(c int) {
		panic(exitPanic{code: c})
	}

	fn()
	return false, 0
}

func TestVersionFlag_BareInvocation(t *testing.T) {
	app.FlagVersion = false
	session := &Session{Config: &cfg_g.Config{}}
	cliApp := InitCLI(session)

	var buf bytes.Buffer
	cliApp.Stdout = &buf
	cliApp.Stderr = &buf

	called, code := interceptExit(func() {
		_ = cliApp.Execute([]string{"--version"})
	})

	if !called {
		t.Fatal("expected exitFunc to be called for --version")
	}
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}

	expected := fmt.Sprintf("mail_cli version %s\n", app.Version)
	if !strings.Contains(buf.String(), expected) {
		t.Errorf("expected stdout to contain %q, got: %q", expected, buf.String())
	}
}

func TestVersionFlag_WithGlobalOption(t *testing.T) {
	app.FlagVersion = false
	session := &Session{Config: &cfg_g.Config{}}
	cliApp := InitCLI(session)

	var buf bytes.Buffer
	cliApp.Stdout = &buf
	cliApp.Stderr = &buf

	called, code := interceptExit(func() {
		_ = cliApp.Execute([]string{"-v", "--version"})
	})

	if !called {
		t.Fatal("expected exitFunc to be called for -v --version")
	}
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}

	expected := fmt.Sprintf("mail_cli version %s\n", app.Version)
	if !strings.Contains(buf.String(), expected) {
		t.Errorf("expected stdout to contain %q, got: %q", expected, buf.String())
	}
}

func TestVersionFlag_SubcommandTrailing(t *testing.T) {
	app.FlagVersion = false
	rec := &sessionRecorder{}
	session := recordingSession(rec)
	cliApp := InitCLI(session)

	var buf bytes.Buffer
	cliApp.Stdout = &buf
	cliApp.Stderr = &buf

	called, code := interceptExit(func() {
		_ = cliApp.Execute([]string{"scan", "%inbox", "--version"})
	})

	if !called {
		t.Fatal("expected exitFunc to be called for scan inbox --version")
	}
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
	if rec.scanned {
		t.Error("subcommand must NOT execute when --version is specified")
	}
	if rec.prechecked {
		t.Error("precheck must NOT run when --version is specified")
	}

	expected := fmt.Sprintf("mail_cli version %s\n", app.Version)
	if !strings.Contains(buf.String(), expected) {
		t.Errorf("expected stdout to contain %q, got: %q", expected, buf.String())
	}
}

func TestVersionFlag_SubcommandLeading(t *testing.T) {
	app.FlagVersion = false
	rec := &sessionRecorder{}
	session := recordingSession(rec)
	cliApp := InitCLI(session)

	var buf bytes.Buffer
	cliApp.Stdout = &buf
	cliApp.Stderr = &buf

	called, code := interceptExit(func() {
		_ = cliApp.Execute([]string{"--version", "scan", "%inbox"})
	})

	if !called {
		t.Fatal("expected exitFunc to be called for --version scan inbox")
	}
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
	if rec.scanned {
		t.Error("subcommand must NOT execute when --version is specified leading")
	}

	expected := fmt.Sprintf("mail_cli version %s\n", app.Version)
	if !strings.Contains(buf.String(), expected) {
		t.Errorf("expected stdout to contain %q, got: %q", expected, buf.String())
	}
}
