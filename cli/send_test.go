package cli

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/sarielhp/mail_cli/app"
	"github.com/sarielhp/mail_cli/cfg_g"
)

// --encrypt fails closed when any recipient lacks a key; the help used to say
// it encrypted "for every recipient with a key", as if the rest got plaintext.
func TestSendEncryptHelpSaysItFailsClosed(t *testing.T) {
	var buf bytes.Buffer
	cliApp := InitCLI(&Session{Config: &cfg_g.Config{}})
	cliApp.Stdout, cliApp.Stderr = &buf, &buf
	if err := cliApp.Execute([]string{"send", "-h"}); err != nil {
		t.Fatalf("send -h: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "--encrypt") {
		t.Fatalf("send -h does not list --encrypt:\n%s", out)
	}
	if strings.Contains(out, "every recipient with a key") || !strings.Contains(out, "fails unless all recipients have keys") {
		t.Errorf("--encrypt help misdescribes what happens to a recipient with no key:\n%s", out)
	}
}

func TestSplitCommaList(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"", nil},
		{"   ", nil},
		{"a@example.com", []string{"a@example.com"}},
		{"a@example.com, b@example.com", []string{"a@example.com", "b@example.com"}},
		{" a@example.com , , b@example.com ", []string{"a@example.com", "b@example.com"}},
	}

	for _, tc := range tests {
		got := splitCommaList(tc.input)
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("splitCommaList(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestResolveSendBody_Inline(t *testing.T) {
	assertInlineBody(t, &flagSendBody, &flagSendInclude, resolveSendBody, "Hello world")
}

func TestResolveSendBody_IncludeFile(t *testing.T) {
	assertBodyFromFile(t, &flagSendBody, &flagSendInclude, resolveSendBody, "File content")
}

func TestResolveSendBody_MissingFile(t *testing.T) {
	flagSendBody = ""
	flagSendInclude = "/nonexistent/path/to/body.txt"
	defer func() { flagSendInclude = "" }()

	_, err := resolveSendBody()
	if err == nil {
		t.Fatalf("expected error for nonexistent file, got nil")
	}
}

func TestResolveSendBody_NoBodyReturnsUsageError(t *testing.T) {
	flagSendBody = ""
	flagSendInclude = ""

	_, err := resolveSendBody()
	if !errors.Is(err, app.ErrUsage) {
		t.Fatalf("expected ErrUsage, got %v", err)
	}
}

func TestRunSend_InvokesService(t *testing.T) {
	flagSendBody = "Test message"
	flagSendSubject = "Test Subject"
	flagSendCc = "cc@example.com"
	flagSendBcc = "bcc@example.com"
	flagSendAttach = nil
	defer func() {
		flagSendBody = ""
		flagSendSubject = ""
		flagSendCc = ""
		flagSendBcc = ""
	}()

	var calledWith SendOptions
	sendCalled := false

	session := dryRunSession("work")
	session.Deps.Send = stubSend{SendFn: func(_ *cfg_g.Config, opts SendOptions) error {
		sendCalled = true
		calledWith = opts
		return nil
	}}

	err := runSend(session, []string{"to@example.com"})
	if err != nil {
		t.Fatalf("runSend failed: %v", err)
	}

	if !sendCalled {
		t.Errorf("expected Send to be invoked")
	}
	if len(calledWith.To) != 1 || calledWith.To[0] != "to@example.com" {
		t.Errorf("unexpected To: %v", calledWith.To)
	}
	if calledWith.Subject != "Test Subject" {
		t.Errorf("unexpected Subject: %s", calledWith.Subject)
	}
	if calledWith.Body != "Test message" {
		t.Errorf("unexpected Body: %s", calledWith.Body)
	}
	if calledWith.Account != "work" {
		t.Errorf("unexpected Account: %s", calledWith.Account)
	}
	if !calledWith.DryRun {
		t.Errorf("expected DryRun true")
	}
}

// send has no review screen, so what `write` catches on the way to the editor
// send has to catch before mailing: an unquoted -a *.pdf leaves the other
// matches among the recipients, and a quoted pattern must arrive as files.
func TestRunSendExpandsAttachmentsAndRefusesFileRecipients(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	touch(t, dir, "a.pdf", "b.pdf")
	flagSendBody = "body"
	defer func() { flagSendBody = ""; flagSendAttach = nil }()

	var sent *SendOptions
	session := &Session{
		Config: &cfg_g.Config{Paths: cfg_g.Paths{ConfigDir: t.TempDir()}},
		Deps: Deps{Send: stubSend{SendFn: func(_ *cfg_g.Config, opts SendOptions) error {
			sent = &opts
			return nil
		}}},
	}

	flagSendAttach = []string{"a.pdf"}
	err := runSend(session, []string{"boss@example.com", "b.pdf"})
	if !errors.Is(err, app.ErrUsage) || sent != nil {
		t.Fatalf("a file among the recipients: err = %v, sent = %v; want a usage error and nothing sent", err, sent)
	}

	flagSendAttach = []string{"*.pdf"}
	if err := runSend(session, []string{"boss@example.com"}); err != nil {
		t.Fatal(err)
	}
	if want := []string{"a.pdf", "b.pdf"}; sent == nil || !reflect.DeepEqual(sent.Attachments, want) {
		t.Errorf("Attachments = %v, want %q", sent, want)
	}
}

func TestRunSendPassesPgpFlagsToTheService(t *testing.T) {
	flagSendBody = "Test message"
	flagSendSubject = "Test Subject"
	flagSendSign = true
	flagSendEncrypt = true
	flagSendPgpKey = "0xDEADBEEF"
	defer func() {
		flagSendBody, flagSendSubject = "", ""
		flagSendSign, flagSendEncrypt, flagSendPgpKey = false, false, ""
	}()

	var calledWith SendOptions
	session := &Session{
		Config: &cfg_g.Config{SelectedAccount: "work"},
		Deps: Deps{
			Send: stubSend{
				SendFn: func(_ *cfg_g.Config, opts SendOptions) error {
					calledWith = opts
					return nil
				},
			},
		},
	}

	if err := runSend(session, []string{"to@example.com"}); err != nil {
		t.Fatalf("runSend failed: %v", err)
	}
	if !calledWith.Sign || !calledWith.Encrypt {
		t.Errorf("Sign/Encrypt = %v/%v, want both set", calledWith.Sign, calledWith.Encrypt)
	}
	if calledWith.PgpKeyID != "0xDEADBEEF" {
		t.Errorf("PgpKeyID = %q", calledWith.PgpKeyID)
	}
}

// --raw means the message is already built. Signing it would sign a body this
// command cannot see the boundaries of, so it is refused rather than guessed at.
func TestRunSendRefusesSigningARawMessage(t *testing.T) {
	flagSendRaw = true
	flagSendSign = true
	defer func() { flagSendRaw, flagSendSign = false, false }()

	session := &Session{Config: &cfg_g.Config{}, Deps: Deps{Send: stubSend{}}}
	err := runSend(session, nil)
	if err == nil {
		t.Fatal("expected a usage error")
	}
	if !errors.Is(err, app.ErrUsage) {
		t.Fatalf("err = %v, want a usage error", err)
	}
}
