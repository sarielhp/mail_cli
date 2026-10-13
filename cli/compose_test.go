package cli

import (
	"reflect"
	"testing"

	"github.com/sarielhp/mail_cli/cfg_g"
)

func TestResolveComposeBody_Inline(t *testing.T) {
	assertInlineBody(t, &flagComposeBody, &flagComposeInclude, resolveComposeBody, "Initial compose body")
}

func TestResolveComposeBody_IncludeFile(t *testing.T) {
	assertBodyFromFile(t, &flagComposeBody, &flagComposeInclude, resolveComposeBody, "Template body from file")
}

func TestResolveComposeBody_MissingFile(t *testing.T) {
	flagComposeBody = ""
	flagComposeInclude = "/nonexistent/path/to/compose_body.txt"
	defer func() { flagComposeInclude = "" }()

	_, err := resolveComposeBody()
	if err == nil {
		t.Fatalf("expected error for nonexistent file, got nil")
	}
}

func TestRunCompose_PassesOptionsToTUI(t *testing.T) {
	var capturedOpts ComposeOptions
	called := false

	session := &Session{
		Config: &cfg_g.Config{
			SelectedAccount: "work",
		},
		Deps: Deps{
			TUI: stubTUI{
				ComposeFn: func(_ *cfg_g.Config, opts ComposeOptions) error {
					called = true
					capturedOpts = opts
					return nil
				},
			},
		},
	}

	flagComposeSubject = "Discussion"
	flagComposeCc = "dev1@example.com, dev2@example.com"
	flagComposeBcc = "archive@example.com"
	flagComposeAttach = []string{"spec.pdf"}
	flagComposeBody = "Let's review the specs."
	defer func() {
		flagComposeSubject = ""
		flagComposeCc = ""
		flagComposeBcc = ""
		flagComposeAttach = nil
		flagComposeBody = ""
	}()

	recipients := []string{"boss@example.com"}
	err := runCompose(session, recipients)
	if err != nil {
		t.Fatalf("runCompose failed: %v", err)
	}
	if !called {
		t.Fatalf("expected TUI.Compose to be called")
	}

	if !reflect.DeepEqual(capturedOpts.To, recipients) {
		t.Errorf("To = %v, want %v", capturedOpts.To, recipients)
	}
	if capturedOpts.Subject != "Discussion" {
		t.Errorf("Subject = %q, want 'Discussion'", capturedOpts.Subject)
	}
	if capturedOpts.Account != "work" {
		t.Errorf("Account = %q, want 'work'", capturedOpts.Account)
	}
	if len(capturedOpts.Cc) != 2 || capturedOpts.Cc[0] != "dev1@example.com" || capturedOpts.Cc[1] != "dev2@example.com" {
		t.Errorf("Cc = %v, want [dev1@example.com dev2@example.com]", capturedOpts.Cc)
	}
	if len(capturedOpts.Bcc) != 1 || capturedOpts.Bcc[0] != "archive@example.com" {
		t.Errorf("Bcc = %v, want [archive@example.com]", capturedOpts.Bcc)
	}
	if len(capturedOpts.Attachments) != 1 || capturedOpts.Attachments[0] != "spec.pdf" {
		t.Errorf("Attachments = %v, want [spec.pdf]", capturedOpts.Attachments)
	}
	if capturedOpts.Body != "Let's review the specs." {
		t.Errorf("Body = %q, want 'Let\\'s review the specs.'", capturedOpts.Body)
	}
}
