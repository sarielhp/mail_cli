package cli

import (
	"testing"

	"github.com/sarielhp/mail_cli/cfg_g"
)

func TestPipeCmd_Validation(t *testing.T) {
	var calledOpts PipeOptions
	var calledArgs []string
	called := false

	session := &Session{
		Config: &cfg_g.Config{
			SelectedAccount: "testacc",
		},
		Deps: Deps{
			Mail: stubMail{
				PipeFn: func(_ *cfg_g.Config, opts PipeOptions, args []string) error {
					called = true
					calledOpts = opts
					calledArgs = args
					return nil
				},
			},
		},
	}

	app := InitCLI(session)

	// Missing arguments
	err := app.Execute([]string{"msg", "pipe"})
	if err == nil {
		t.Errorf("expected error when no args passed to msg pipe")
	}

	err = app.Execute([]string{"msg", "pipe", "18fb1234"})
	if err == nil {
		t.Errorf("expected error when only message-id passed to msg pipe")
	}

	// Valid pipe command
	called = false
	err = app.Execute([]string{"msg", "pipe", "18fb1234", "git", "am"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatalf("expected Mail.Pipe to be called")
	}
	if calledOpts.MessageID != "18fb1234" {
		t.Errorf("expected MessageID 18fb1234, got %q", calledOpts.MessageID)
	}
	if calledOpts.Account != "testacc" {
		t.Errorf("expected Account testacc, got %q", calledOpts.Account)
	}
	if len(calledArgs) != 2 || calledArgs[0] != "git" || calledArgs[1] != "am" {
		t.Errorf("expected args ['git', 'am'], got %v", calledArgs)
	}

	// Flags: --body
	called = false
	err = app.Execute([]string{"msg", "pipe", "--body", "18fb1234", "wc"})
	if err != nil {
		t.Fatalf("unexpected error with --body: %v", err)
	}
	if !calledOpts.BodyOnly {
		t.Errorf("expected BodyOnly to be true")
	}

	// Flags: --headers-only
	called = false
	err = app.Execute([]string{"msg", "pipe", "--headers-only", "18fb1234", "grep", "Subject"})
	if err != nil {
		t.Fatalf("unexpected error with --headers-only: %v", err)
	}
	if !calledOpts.HeadersOnly {
		t.Errorf("expected HeadersOnly to be true")
	}

	// Double-dash to pass flags to external command
	called = false
	err = app.Execute([]string{"msg", "pipe", "--body", "18fb1234", "--", "wc", "-w"})
	if err != nil {
		t.Fatalf("unexpected error with --: %v", err)
	}
	if !calledOpts.BodyOnly {
		t.Errorf("expected BodyOnly to be true")
	}
}
