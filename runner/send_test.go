package runner

import (
	"bytes"
	"context"
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"github.com/sarielhp/mail_cli/email"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sarielhp/mail_cli/cache/folderindex"
	"github.com/sarielhp/mail_cli/cache/msg"
	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/cli"
	"github.com/sarielhp/mail_cli/contacts"
	"github.com/sarielhp/mail_cli/mailclient"
	"github.com/sarielhp/mail_cli/outbox"
	"github.com/sarielhp/mail_cli/pgp"
	"github.com/sarielhp/mail_cli/report"
)

func TestLoadAttachments(t *testing.T) {
	// 1. Empty list
	atts, err := loadAttachments(nil)
	if err != nil || atts != nil {
		t.Fatalf("expected nil, nil, got %v, %v", atts, err)
	}

	// 2. Real file
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.txt")
	if writeErr := os.WriteFile(filePath, []byte("Hello world"), 0600); writeErr != nil {
		t.Fatalf("write file failed: %v", writeErr)
	}

	atts, err = loadAttachments([]string{filePath})
	if err != nil {
		t.Fatalf("loadAttachments failed: %v", err)
	}
	if len(atts) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(atts))
	}
	if atts[0].Filename != "test.txt" {
		t.Errorf("expected filename test.txt, got %s", atts[0].Filename)
	}
	if string(atts[0].Data) != "Hello world" {
		t.Errorf("expected data Hello world, got %s", string(atts[0].Data))
	}

	// 3. Nonexistent file
	_, err = loadAttachments([]string{"/nonexistent/test.txt"})
	if err == nil {
		t.Fatalf("expected error for nonexistent file, got nil")
	}
}

func TestExpandRecipients(t *testing.T) {
	dir := t.TempDir()
	cfg := &cfg_g.Config{
		Paths: cfg_g.Paths{ConfigDir: dir},
	}
	bookPath := contacts.DefaultPath(cfg)
	book := contacts.NewBook()
	book.Contacts = append(book.Contacts, contacts.Contact{
		Name:      "Alice Smith",
		Emails:    []string{"alice@example.com"},
		Nicknames: []string{"alice"},
	})
	_ = contacts.SaveBook(bookPath, book)

	raw := []string{"alice", "bob@example.com"}
	expanded := expandRecipients(cfg, raw)

	if len(expanded) != 2 {
		t.Fatalf("expected 2 expanded recipients, got %d", len(expanded))
	}
	if expanded[0] != "Alice Smith <alice@example.com>" {
		t.Errorf("expected Alice Smith <alice@example.com>, got %s", expanded[0])
	}
	if expanded[1] != "bob@example.com" {
		t.Errorf("expected bob@example.com, got %s", expanded[1])
	}
}

func TestSendService_DryRun(t *testing.T) {
	var buf bytes.Buffer
	svc := SendService{
		Rep: report.Discard(),
		Out: &buf,
	}

	cfg := &cfg_g.Config{
		Accounts: []cfg_acc.AccountConfig{
			{Name: "work", Username: "user@work.com", UserName: "Worker", Type: "jmap"},
		},
	}

	opts := cli.SendOptions{
		To:      []string{"recipient@example.com"},
		Subject: "Dry Run Test",
		Body:    "This is a dry run body.",
		DryRun:  true,
	}

	err := svc.Send(cfg, opts)
	if err != nil {
		t.Fatalf("Send dry-run failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "To: recipient@example.com") {
		t.Errorf("expected To header, got:\n%s", out)
	}
	if !strings.Contains(out, "From: Worker <user@work.com>") {
		t.Errorf("expected From header, got:\n%s", out)
	}
	if !strings.Contains(out, "Subject: Dry Run Test") {
		t.Errorf("expected Subject header, got:\n%s", out)
	}
	if !strings.Contains(out, "This is a dry run body.") {
		t.Errorf("expected Body, got:\n%s", out)
	}
}

// gpgKeyring points the test process at a throwaway keyring holding one
// passphrase-less key, and returns the user id. It skips when gpg is not
// installed, because none of these tests can mean anything without it.
func gpgKeyring(t *testing.T) string {
	t.Helper()
	if !pgp.Available() {
		t.Skip("gpg is not installed")
	}
	home := t.TempDir()
	t.Setenv("GNUPGHOME", home)
	cmd := exec.Command("gpg", "--batch", "--pinentry-mode", "loopback", "--passphrase", "",
		"--quick-generate-key", "Test User <test@example.com>", "default", "default", "never")
	cmd.Env = append(os.Environ(), "GNUPGHOME="+home)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generate test key: %v\n%s", err, out)
	}
	return "Test User <test@example.com>"
}

func sendServiceFor(t *testing.T, out io.Writer, account cfg_acc.AccountConfig) (SendService, *cfg_g.Config) {
	t.Helper()
	svc := SendService{Rep: report.Discard(), Out: out}
	return svc, &cfg_g.Config{Accounts: []cfg_acc.AccountConfig{account}}
}

// A dry run prints the message that would go out, so with --sign it must print
// the signed one -- otherwise the flag cannot be reviewed before it is used.
func TestSendService_DryRunSignsTheMessage(t *testing.T) {
	gpgKeyring(t)
	var buf bytes.Buffer
	svc, cfg := sendServiceFor(t, &buf, cfg_acc.AccountConfig{
		// The sender is the key's owner: a signature by a key that does not
		// carry the From address is reported as a sender mismatch.
		Name: "work", Username: "test@example.com", UserName: "Worker", Type: "jmap",
	})

	opts := cli.SendOptions{
		To:      []string{"recipient@example.com"},
		Subject: "Signed Test",
		Body:    "a signed body",
		DryRun:  true,
		Sign:    true,
	}
	if err := svc.Send(cfg, opts); err != nil {
		t.Fatalf("Send: %v", err)
	}

	result := pgp.Process(context.Background(), buf.Bytes(), pgp.Options{Verify: true})
	if result.Status == nil || !result.Status.ValidSig {
		t.Fatalf("status = %+v, want the dry-run output to verify", result.Status)
	}
	if !strings.Contains(buf.String(), "Subject: Signed Test") {
		t.Error("the routing headers were lost")
	}
	if !strings.Contains(buf.String(), "a signed body") {
		t.Error("the body was lost")
	}
}

func TestSendService_DryRunEncryptsTheMessage(t *testing.T) {
	user := gpgKeyring(t)
	var buf bytes.Buffer
	svc, cfg := sendServiceFor(t, &buf, cfg_acc.AccountConfig{
		Name: "work", Username: "user@work.com", UserEmail: user, UserName: "Worker", Type: "jmap",
	})

	opts := cli.SendOptions{
		To:      []string{user},
		Subject: "Encrypted Test",
		Body:    "top secret body",
		DryRun:  true,
		Encrypt: true,
	}
	if err := svc.Send(cfg, opts); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if strings.Contains(buf.String(), "top secret body") {
		t.Fatal("the plaintext was written out despite --encrypt")
	}

	result := pgp.Process(context.Background(), buf.Bytes(), pgp.Options{Decrypt: true})
	if result.Status == nil || !result.Status.Decrypted {
		t.Fatalf("status = %+v, want the output to decrypt", result.Status)
	}
	if !bytes.Contains(result.Raw, []byte("top secret body")) {
		t.Fatalf("decrypted = %q, want the body back", result.Raw)
	}
}

// Encrypting to someone with no key would send them ciphertext they cannot
// read, so it is refused, and the refusal names them.
func TestSendService_RefusesEncryptionWhenARecipientHasNoKey(t *testing.T) {
	gpgKeyring(t)
	svc, cfg := sendServiceFor(t, io.Discard, cfg_acc.AccountConfig{
		Name: "work", Username: "user@work.com", UserName: "Worker", Type: "jmap",
	})

	err := svc.Send(cfg, cli.SendOptions{
		To:      []string{"nobody@example.com"},
		Subject: "Encrypted Test",
		Body:    "body",
		DryRun:  true,
		Encrypt: true,
	})
	if err == nil {
		t.Fatal("expected a refusal")
	}
	if !strings.Contains(err.Error(), "nobody@example.com") {
		t.Fatalf("err = %v, want it to name the recipient with no key", err)
	}
}

// The From header carries the person's name (user_name), never the account's
// label (display_name): an account labelled "FastMail" must not sign its mail
// "FastMail".
func TestSendService_SenderNameIsThePersonNotTheAccountLabel(t *testing.T) {
	var buf bytes.Buffer
	svc := SendService{Rep: report.Discard(), Out: &buf}

	cfg := &cfg_g.Config{
		Accounts: []cfg_acc.AccountConfig{
			{Name: "fastmail", Username: "sarielhp@sarielhp.org", UserEmail: "sariel@sarielhp.org", DisplayName: "FastMail", UserName: "Sariel Har-Peled", Type: "jmap"},
		},
	}

	opts := cli.SendOptions{
		To:      []string{"recipient@example.com"},
		Subject: "Hello",
		Body:    "Body.",
		DryRun:  true,
	}

	if err := svc.Send(cfg, opts); err != nil {
		t.Fatalf("Send dry-run failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "From: Sariel Har-Peled <sariel@sarielhp.org>") {
		t.Errorf("expected the user name and send address in From, got:\n%s", out)
	}
	if strings.Contains(out, "FastMail") {
		t.Errorf("the account label leaked into the message, got:\n%s", out)
	}
	if strings.Contains(out, "sarielhp@sarielhp.org") {
		t.Errorf("the login username leaked into the message instead of the send address, got:\n%s", out)
	}
}

func TestSendService_RawDryRun(t *testing.T) {
	var buf bytes.Buffer
	svc := SendService{
		Rep: report.Discard(),
		Out: &buf,
	}

	cfg := &cfg_g.Config{
		Accounts: []cfg_acc.AccountConfig{
			{Name: "work", Username: "user@work.com", UserName: "Worker", Type: "jmap"},
		},
	}

	rawMsg := "From: user@work.com\r\nTo: recipient@example.com\r\nSubject: Raw Patch\r\n\r\nPatch content here.\r\n"
	opts := cli.SendOptions{
		Raw:      true,
		RawBytes: []byte(rawMsg),
		DryRun:   true,
	}

	err := svc.Send(cfg, opts)
	if err != nil {
		t.Fatalf("Send raw dry-run failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Subject: Raw Patch") {
		t.Errorf("expected Subject header, got:\n%s", out)
	}
	if !strings.Contains(out, "Patch content here.") {
		t.Errorf("expected Body, got:\n%s", out)
	}
}

type stubLauncher struct {
	composeFn func(*cfg_g.Config, cli.ComposeOptions) error
}

func (s stubLauncher) Run(*cfg_g.Config, string, ...string) error                       { return nil }
func (s stubLauncher) RunWithClient(*cfg_g.Config, mailclient.MailClient, string) error { return nil }
func (s stubLauncher) Compose(c *cfg_g.Config, opts cli.ComposeOptions) error {
	if s.composeFn != nil {
		return s.composeFn(c, opts)
	}
	return nil
}

func setupTestCachedMessage(t *testing.T, downloadDir, accName, msgID string, raw []byte) {
	t.Helper()
	accDir := filepath.Join(downloadDir, cfg_g.SanitizeLabelForCache(accName))
	if err := msg.Store(cachedir.At(accDir), email.ID(msgID), raw, time.Now()); err != nil {
		t.Fatalf("msg.Store failed: %v", err)
	}
	if err := folderindex.Add(cachedir.At(accDir), "INBOX", email.ID(msgID)); err != nil {
		t.Fatalf("folderindex.Add failed: %v", err)
	}
}

func TestReplyService_HeadlessDryRun(t *testing.T) {
	tempDir := t.TempDir()
	rawMsg := "From: Boss <boss@company.com>\r\n" +
		"To: Worker <user@work.com>\r\n" +
		"Subject: Quarterly Review\r\n" +
		"Message-ID: <rev123@company.com>\r\n" +
		"Date: Sun, 20 Sep 2026 10:00:00 +0000\r\n" +
		"Content-Type: text/plain\r\n" +
		"\r\n" +
		"Please send your self-evaluation.\r\n"

	setupTestCachedMessage(t, tempDir, "work", "msg001", []byte(rawMsg))

	cfg := &cfg_g.Config{
		Paths: cfg_g.Paths{
			ConfigDir: tempDir,
			CacheRoot: tempDir,
		},
		Accounts: []cfg_acc.AccountConfig{
			{Name: "work", Username: "user@work.com", UserName: "Worker", Type: "jmap"},
		},
	}

	var buf bytes.Buffer
	svc := SendService{
		Rep: report.Discard(),
		Out: &buf,
	}

	opts := cli.ReplyOptions{
		MessageID: "msg001",
		Headless:  true,
		DryRun:    true,
		Body:      "Sending my evaluation attached.",
	}

	if err := svc.Reply(cfg, opts); err != nil {
		t.Fatalf("Reply failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "To: Boss <boss@company.com>") {
		t.Errorf("expected To header, got:\n%s", out)
	}
	if !strings.Contains(out, "From: Worker <user@work.com>") {
		t.Errorf("expected From header, got:\n%s", out)
	}
	if !strings.Contains(out, "Subject: Re: Quarterly Review") {
		t.Errorf("expected Subject header, got:\n%s", out)
	}
	if !strings.Contains(out, "In-Reply-To: <rev123@company.com>") {
		t.Errorf("expected In-Reply-To header, got:\n%s", out)
	}
	if !strings.Contains(out, "References: <rev123@company.com>") {
		t.Errorf("expected References header, got:\n%s", out)
	}
	if !strings.Contains(out, "Sending my evaluation attached.") {
		t.Errorf("expected reply body text, got:\n%s", out)
	}
	if !strings.Contains(out, "> Please send your self-evaluation.") {
		t.Errorf("expected quoted body, got:\n%s", out)
	}
}

func TestReplyService_GroupReply(t *testing.T) {
	tempDir := t.TempDir()
	rawMsg := "From: Alice <alice@example.com>\r\n" +
		"To: Worker <user@work.com>, Bob <bob@example.com>\r\n" +
		"Cc: Charlie <charlie@example.com>\r\n" +
		"Subject: Team Sync\r\n" +
		"Message-ID: <sync456@example.com>\r\n" +
		"Date: Sun, 20 Sep 2026 11:00:00 +0000\r\n" +
		"Content-Type: text/plain\r\n" +
		"\r\n" +
		"Sync at 3pm today.\r\n"

	setupTestCachedMessage(t, tempDir, "work", "msg002", []byte(rawMsg))

	cfg := &cfg_g.Config{
		Paths: cfg_g.Paths{
			ConfigDir: tempDir,
			CacheRoot: tempDir,
		},
		Accounts: []cfg_acc.AccountConfig{
			{Name: "work", Username: "user@work.com", UserName: "Worker", Type: "jmap"},
		},
	}

	var buf bytes.Buffer
	svc := SendService{
		Rep: report.Discard(),
		Out: &buf,
	}

	opts := cli.ReplyOptions{
		MessageID: "msg002",
		Group:     true,
		Headless:  true,
		DryRun:    true,
		Body:      "I will attend.",
	}

	if err := svc.Reply(cfg, opts); err != nil {
		t.Fatalf("Reply failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "To: Alice <alice@example.com>") {
		t.Errorf("expected To header, got:\n%s", out)
	}
	if !strings.Contains(out, "bob@example.com") || !strings.Contains(out, "charlie@example.com") {
		t.Errorf("expected Cc to contain Bob and Charlie, got:\n%s", out)
	}
	if strings.Contains(out, "Cc: user@work.com") {
		t.Errorf("ourEmail should not be in Cc, got:\n%s", out)
	}
}

func TestReplyService_NoQuote(t *testing.T) {
	tempDir := t.TempDir()
	rawMsg := "From: Alice <alice@example.com>\r\n" +
		"Subject: Notice\r\n" +
		"Message-ID: <not789@example.com>\r\n" +
		"Date: Sun, 20 Sep 2026 12:00:00 +0000\r\n" +
		"Content-Type: text/plain\r\n" +
		"\r\n" +
		"Secret content to avoid quoting.\r\n"

	setupTestCachedMessage(t, tempDir, "work", "msg003", []byte(rawMsg))

	cfg := &cfg_g.Config{
		Paths: cfg_g.Paths{
			ConfigDir: tempDir,
			CacheRoot: tempDir,
		},
		Accounts: []cfg_acc.AccountConfig{
			{Name: "work", Username: "user@work.com", UserName: "Worker", Type: "jmap"},
		},
	}

	var buf bytes.Buffer
	svc := SendService{
		Rep: report.Discard(),
		Out: &buf,
	}

	opts := cli.ReplyOptions{
		MessageID: "msg003",
		NoQuote:   true,
		Headless:  true,
		DryRun:    true,
		Body:      "Short ack.",
	}

	if err := svc.Reply(cfg, opts); err != nil {
		t.Fatalf("Reply failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Short ack.") {
		t.Errorf("expected body, got:\n%s", out)
	}
	if strings.Contains(out, "Secret content to avoid quoting") {
		t.Errorf("expected quoted body to be omitted, got:\n%s", out)
	}
}

func TestReplyService_InteractiveLaunchesTUI(t *testing.T) {
	tempDir := t.TempDir()
	rawMsg := "From: Client <client@example.com>\r\n" +
		"Subject: Question\r\n" +
		"Message-ID: <q123@example.com>\r\n" +
		"Date: Sun, 20 Sep 2026 13:00:00 +0000\r\n" +
		"Content-Type: text/plain\r\n" +
		"\r\n" +
		"How do I configure this?\r\n"

	setupTestCachedMessage(t, tempDir, "work", "msg004", []byte(rawMsg))

	cfg := &cfg_g.Config{
		Paths: cfg_g.Paths{
			ConfigDir: tempDir,
			CacheRoot: tempDir,
		},
		Accounts: []cfg_acc.AccountConfig{
			{Name: "work", Username: "user@work.com", UserName: "Worker", Type: "jmap"},
		},
	}

	var capturedOpts cli.ComposeOptions
	called := false
	launcher := stubLauncher{
		composeFn: func(_ *cfg_g.Config, opts cli.ComposeOptions) error {
			called = true
			capturedOpts = opts
			return nil
		},
	}

	svc := SendService{
		Rep: report.Discard(),
		TUI: launcher,
	}

	opts := cli.ReplyOptions{
		MessageID: "msg004",
		Headless:  false,
	}

	if err := svc.Reply(cfg, opts); err != nil {
		t.Fatalf("Reply failed: %v", err)
	}
	if !called {
		t.Fatalf("expected launcher.Compose to be called")
	}

	if capturedOpts.InReplyTo != "<q123@example.com>" {
		t.Errorf("InReplyTo = %q, want '<q123@example.com>'", capturedOpts.InReplyTo)
	}
	if capturedOpts.References != "<q123@example.com>" {
		t.Errorf("References = %q, want '<q123@example.com>'", capturedOpts.References)
	}
	if capturedOpts.Subject != "Re: Question" {
		t.Errorf("Subject = %q, want 'Re: Question'", capturedOpts.Subject)
	}
	if capturedOpts.OriginalID != "msg004" {
		t.Errorf("OriginalID = %q, want 'msg004'", capturedOpts.OriginalID)
	}
	if len(capturedOpts.To) != 1 || capturedOpts.To[0] != "Client <client@example.com>" {
		t.Errorf("To = %v, want ['Client <client@example.com>']", capturedOpts.To)
	}
}

func TestSend_Queue(t *testing.T) {
	tempDir := t.TempDir()
	cfg := &cfg_g.Config{
		Paths: cfg_g.Paths{
			ConfigDir: tempDir,
			CacheRoot: tempDir,
		},
		Accounts: []cfg_acc.AccountConfig{
			{Name: "work", Username: "user@work.com", Type: "jmap"},
		},
		SelectedAccount: "work",
	}

	svc := SendService{
		Rep: report.Discard(),
	}

	opts := cli.SendOptions{
		To:      []string{"target@example.com"},
		Subject: "Queued Note",
		Body:    "Stored for later",
		Queue:   true,
	}

	if err := svc.Send(cfg, opts); err != nil {
		t.Fatalf("Send with Queue=true failed: %v", err)
	}

	entries, err := outbox.List(outbox.Dir(tempDir))
	if err != nil {
		t.Fatalf("outbox.List failed: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 queued entry, got %d", len(entries))
	}
	if entries[0].Subject != "Queued Note" || entries[0].Account != "work" {
		t.Errorf("unexpected entry data: %+v", entries[0])
	}
}
