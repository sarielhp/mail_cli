package cli

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/sarielhp/mail_cli/app"
	"github.com/sarielhp/mail_cli/cfg_g"
)

func TestRunReply_MissingArgs(t *testing.T) {
	session := &Session{
		Config: &cfg_g.Config{},
	}
	err := runReply(session, nil)
	if err == nil {
		t.Fatalf("expected error when running reply without arguments")
	}
	if !errors.Is(err, app.ErrUsage) {
		t.Errorf("expected ErrUsage, got %v", err)
	}

	err = runReply(session, []string{"   "})
	if err == nil {
		t.Fatalf("expected error when running reply with empty ID")
	}
}

func TestResolveReplyBody_Inline(t *testing.T) {
	flagReplyBody = "Replying inline"
	flagReplyInclude = ""
	defer func() { flagReplyBody = "" }()

	body, err := resolveReplyBody(false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if body != "Replying inline" {
		t.Errorf("got %q, want 'Replying inline'", body)
	}
}

func TestResolveReplyBody_IncludeFile(t *testing.T) {
	flagReplyBody = ""
	tmpFile := filepath.Join(t.TempDir(), "reply_body.txt")
	if err := os.WriteFile(tmpFile, []byte("Included reply content"), 0600); err != nil {
		t.Fatalf("failed to write tmp file: %v", err)
	}
	flagReplyInclude = tmpFile
	defer func() { flagReplyInclude = "" }()

	body, err := resolveReplyBody(false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if body != "Included reply content" {
		t.Errorf("got %q, want 'Included reply content'", body)
	}
}

func TestResolveReplyBody_MissingFile(t *testing.T) {
	flagReplyBody = ""
	flagReplyInclude = "/nonexistent/path/to/reply_body.txt"
	defer func() { flagReplyInclude = "" }()

	_, err := resolveReplyBody(false)
	if err == nil {
		t.Fatalf("expected error for nonexistent file, got nil")
	}
}

func TestResolveReplyBody_HeadlessRequiresBody(t *testing.T) {
	flagReplyBody = ""
	flagReplyInclude = ""

	_, err := resolveReplyBody(true)
	if err == nil {
		t.Fatalf("expected error when headless reply has no body")
	}
	if !errors.Is(err, app.ErrUsage) {
		t.Errorf("expected ErrUsage, got %v", err)
	}
}

func TestRunReply_PassesOptionsToSend(t *testing.T) {
	var capturedOpts ReplyOptions
	called := false

	session := dryRunSession("personal")
	session.Deps.Send = stubSend{ReplyFn: func(_ *cfg_g.Config, opts ReplyOptions) error {
		called = true
		capturedOpts = opts
		return nil
	}}

	flagReplyGroup = true
	flagReplySubject = "Re: Custom Subject"
	flagReplyCc = "cc1@example.com, cc2@example.com"
	flagReplyBcc = "bcc@example.com"
	flagReplyAttach = "patch.diff"
	flagReplyBody = "Acknowledged"
	flagReplyHeadless = true
	flagReplyNoQuote = true
	defer func() {
		flagReplyGroup = false
		flagReplySubject = ""
		flagReplyCc = ""
		flagReplyBcc = ""
		flagReplyAttach = ""
		flagReplyBody = ""
		flagReplyHeadless = false
		flagReplyNoQuote = false
	}()

	err := runReply(session, []string{"msg12345"})
	if err != nil {
		t.Fatalf("runReply failed: %v", err)
	}
	if !called {
		t.Fatalf("expected Send.Reply to be called")
	}

	if capturedOpts.MessageID != "msg12345" {
		t.Errorf("MessageID = %q, want 'msg12345'", capturedOpts.MessageID)
	}
	if capturedOpts.Account != "personal" {
		t.Errorf("Account = %q, want 'personal'", capturedOpts.Account)
	}
	if !capturedOpts.Group {
		t.Errorf("Group = false, want true")
	}
	if capturedOpts.Subject != "Re: Custom Subject" {
		t.Errorf("Subject = %q, want 'Re: Custom Subject'", capturedOpts.Subject)
	}
	if !reflect.DeepEqual(capturedOpts.Cc, []string{"cc1@example.com", "cc2@example.com"}) {
		t.Errorf("Cc = %v, want [cc1@example.com cc2@example.com]", capturedOpts.Cc)
	}
	if !reflect.DeepEqual(capturedOpts.Bcc, []string{"bcc@example.com"}) {
		t.Errorf("Bcc = %v, want [bcc@example.com]", capturedOpts.Bcc)
	}
	if !reflect.DeepEqual(capturedOpts.Attachments, []string{"patch.diff"}) {
		t.Errorf("Attachments = %v, want [patch.diff]", capturedOpts.Attachments)
	}
	if capturedOpts.Body != "Acknowledged" {
		t.Errorf("Body = %q, want 'Acknowledged'", capturedOpts.Body)
	}
	if !capturedOpts.Headless {
		t.Errorf("Headless = false, want true")
	}
	if !capturedOpts.NoQuote {
		t.Errorf("NoQuote = false, want true")
	}
	if !capturedOpts.DryRun {
		t.Errorf("DryRun = false, want true")
	}
}
