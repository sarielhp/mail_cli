package watcher

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/mailclient"
	"github.com/sarielhp/mail_cli/report"
)

type recordingReporter struct{ lines []string }

func (r *recordingReporter) add(prefix, format string, a ...any) {
	r.lines = append(r.lines, prefix+fmt.Sprintf(format, a...))
}
func (r *recordingReporter) Infof(f string, a ...any)    { r.add("info: ", f, a...) }
func (r *recordingReporter) Successf(f string, a ...any) { r.add("ok: ", f, a...) }
func (r *recordingReporter) Warnf(f string, a ...any)    { r.add("warn: ", f, a...) }
func (r *recordingReporter) Errorf(f string, a ...any)   { r.add("error: ", f, a...) }
func (r *recordingReporter) Printf(f string, a ...any)   { r.add("", f, a...) }
func (r *recordingReporter) Progress(int, int, string)   {}

func (r *recordingReporter) has(substr string) bool {
	for _, line := range r.lines {
		if strings.Contains(line, substr) {
			return true
		}
	}
	return false
}

type fakeNotifier struct {
	got []Notification
	err error
}

func (f *fakeNotifier) Notify(_ context.Context, n Notification) error {
	f.got = append(f.got, n)
	return f.err
}

func rawMessage(subject, from string) []byte {
	return []byte("From: " + from + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"Date: Mon, 12 Oct 2026 10:00:00 +0000\r\n" +
		"Message-ID: <" + subject + "@example.com>\r\n\r\nBody\r\n")
}

func fixedNow() time.Time { return time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC) }

func testDeps(t *testing.T, client mailclient.Reader, rep report.Reporter) Deps {
	t.Helper()
	account := cfg_acc.AccountConfig{Name: "work", Type: "jmap"}
	return Deps{
		Config:   &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: t.TempDir()}, Accounts: []cfg_acc.AccountConfig{account}},
		Reporter: rep,
		ClientFor: func(cfg_acc.AccountConfig) (mailclient.Reader, error) {
			return client, nil
		},
		Now: fixedNow,
	}
}

func newMock(refs []cfg_acc.MessageFolderRef, messages map[email.ID][]byte) *mailclient.MockMailClient {
	return &mailclient.MockMailClient{AccountLatestRefs: refs, RawMessages: messages}
}

func TestCheckAccountFirstRunRecordsWithoutNotifying(t *testing.T) {
	rep := &recordingReporter{}
	notifier := &fakeNotifier{}
	client := newMock(
		[]cfg_acc.MessageFolderRef{{MessageID: "a", Folder: "Inbox"}, {MessageID: "b", Folder: "Inbox"}},
		map[email.ID][]byte{"a": rawMessage("old-a", "alice@example.com"), "b": rawMessage("old-b", "bob@example.com")},
	)
	deps := testDeps(t, client, rep)

	err := checkAccount(context.Background(), deps, Options{}, rep, notifier,
		deps.Config.Accounts[0], deps.Now)
	if err != nil {
		t.Fatalf("checkAccount: %v", err)
	}
	if len(notifier.got) != 0 {
		t.Fatalf("first run notified %d message(s), want none", len(notifier.got))
	}
	if !rep.has("first run") {
		t.Fatalf("first run did not report itself: %v", rep.lines)
	}

	state, err := LoadState(deps.Config.CacheRoot)
	if err != nil {
		t.Fatal(err)
	}
	if got := state.Unseen("work", []email.ID{"a", "b"}); len(got) != 0 {
		t.Fatalf("first run left %v unseen, want none", got)
	}
}

func TestCheckAccountAnnouncesOnlyNewMail(t *testing.T) {
	rep := &recordingReporter{}
	notifier := &fakeNotifier{}
	messages := map[email.ID][]byte{
		"a": rawMessage("old", "alice@example.com"),
		"b": rawMessage("fresh", "bob@example.com"),
	}
	client := newMock([]cfg_acc.MessageFolderRef{{MessageID: "a", Folder: "Inbox"}}, messages)
	deps := testDeps(t, client, rep)

	// First run: seed the watermark with "a" alone.
	if err := checkAccount(context.Background(), deps, Options{}, rep, notifier,
		deps.Config.Accounts[0], deps.Now); err != nil {
		t.Fatal(err)
	}

	// Second run: "b" has arrived.
	client.AccountLatestRefs = []cfg_acc.MessageFolderRef{
		{MessageID: "b", Folder: "Inbox"}, {MessageID: "a", Folder: "Inbox"},
	}
	if err := checkAccount(context.Background(), deps, Options{}, rep, notifier,
		deps.Config.Accounts[0], deps.Now); err != nil {
		t.Fatal(err)
	}

	if len(notifier.got) != 1 {
		t.Fatalf("notified %d message(s), want exactly the new one: %+v", len(notifier.got), notifier.got)
	}
	got := notifier.got[0]
	if got.ID != "b" || got.Subject != "fresh" || got.Account != "work" {
		t.Fatalf("notification = %+v, want id=b subject=fresh account=work", got)
	}
	if !strings.Contains(got.From, "bob@example.com") {
		t.Fatalf("notification From = %q, want the sender address", got.From)
	}
}

// Mail filed in spam, trash, sent or drafts is recorded but never announced:
// the account-wide listing covers every folder, and a draft autosave is not
// new mail.
func TestCheckAccountSkipsQuietFolders(t *testing.T) {
	rep := &recordingReporter{}
	notifier := &fakeNotifier{}
	messages := map[email.ID][]byte{
		"a": rawMessage("old", "alice@example.com"),
		"s": rawMessage("spam", "spammer@example.com"),
		"d": rawMessage("draft", "me@example.com"),
		"g": rawMessage("gmail-sent", "me@example.com"),
		"b": rawMessage("fresh", "bob@example.com"),
	}
	client := newMock([]cfg_acc.MessageFolderRef{{MessageID: "a", Folder: "Inbox"}}, messages)
	deps := testDeps(t, client, rep)
	account := deps.Config.Accounts[0]
	if err := checkAccount(context.Background(), deps, Options{}, rep, notifier, account, deps.Now); err != nil {
		t.Fatal(err)
	}

	client.AccountLatestRefs = []cfg_acc.MessageFolderRef{
		{MessageID: "s", Folder: "Junk"}, {MessageID: "d", Folder: "Drafts"},
		{MessageID: "g", Folder: "[Gmail]/Sent Mail"}, {MessageID: "b", Folder: "Inbox"},
		{MessageID: "a", Folder: "Inbox"},
	}
	if err := checkAccount(context.Background(), deps, Options{}, rep, notifier, account, deps.Now); err != nil {
		t.Fatal(err)
	}
	if len(notifier.got) != 1 || notifier.got[0].ID != "b" {
		t.Fatalf("notified %+v, want only the inbox message b", notifier.got)
	}
	state, err := LoadState(deps.Config.CacheRoot)
	if err != nil {
		t.Fatal(err)
	}
	if got := state.Unseen("work", []email.ID{"s", "d", "g"}); len(got) != 0 {
		t.Fatalf("quiet-folder messages left unseen: %v", got)
	}
}

// An unseen message dated well before the previous poll did not just arrive:
// it slid into the window because something above it was deleted.
func TestCheckAccountSkipsMailOlderThanLastPoll(t *testing.T) {
	rep := &recordingReporter{}
	notifier := &fakeNotifier{}
	messages := map[email.ID][]byte{
		"a": rawMessage("old", "alice@example.com"),
		"b": rawMessage("slid-in", "bob@example.com"),
	}
	client := newMock([]cfg_acc.MessageFolderRef{{MessageID: "a", Folder: "Inbox"}}, messages)
	deps := testDeps(t, client, rep)
	// The fixtures are dated 2026-10-12 10:00 UTC; poll a day later.
	deps.Now = func() time.Time { return time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC) }
	account := deps.Config.Accounts[0]
	if err := checkAccount(context.Background(), deps, Options{}, rep, notifier, account, deps.Now); err != nil {
		t.Fatal(err)
	}

	client.AccountLatestRefs = []cfg_acc.MessageFolderRef{{MessageID: "b", Folder: "Inbox"}}
	if err := checkAccount(context.Background(), deps, Options{}, rep, notifier, account, deps.Now); err != nil {
		t.Fatal(err)
	}
	if len(notifier.got) != 0 {
		t.Fatalf("announced stale mail: %+v", notifier.got)
	}
}

func TestCheckAccountLogsWhenNotificationUnavailable(t *testing.T) {
	rep := &recordingReporter{}
	notifier := &fakeNotifier{err: errors.New("notify-send: not found")}
	messages := map[email.ID][]byte{
		"a": rawMessage("old", "alice@example.com"),
		"b": rawMessage("fresh", "bob@example.com"),
	}
	client := newMock([]cfg_acc.MessageFolderRef{{MessageID: "a", Folder: "Inbox"}}, messages)
	deps := testDeps(t, client, rep)

	if err := checkAccount(context.Background(), deps, Options{}, rep, notifier,
		deps.Config.Accounts[0], deps.Now); err != nil {
		t.Fatal(err)
	}
	client.AccountLatestRefs = []cfg_acc.MessageFolderRef{{MessageID: "b", Folder: "Inbox"}}
	if err := checkAccount(context.Background(), deps, Options{}, rep, notifier,
		deps.Config.Accounts[0], deps.Now); err != nil {
		t.Fatal(err)
	}

	if !rep.has("desktop notification failed") {
		t.Fatalf("no warning about the failed notification: %v", rep.lines)
	}
	if !rep.has("[work]") || !rep.has("fresh") {
		t.Fatalf("no log-line fallback for the new message: %v", rep.lines)
	}
}

func TestCheckAccountRunsOnMailHook(t *testing.T) {
	rep := &recordingReporter{}
	notifier := &fakeNotifier{}
	out := filepath.Join(t.TempDir(), "hook.out")
	t.Setenv("WATCH_HOOK_OUT", out)

	script := filepath.Join(t.TempDir(), "hook.sh")
	body := "#!/bin/sh\nprintf '%s\\n' \"$MAIL_CLI_ACCOUNT|$MAIL_CLI_SUBJECT|$MAIL_CLI_ID\" >> \"$WATCH_HOOK_OUT\"\n"
	if err := os.WriteFile(script, []byte(body), 0700); err != nil {
		t.Fatal(err)
	}

	messages := map[email.ID][]byte{
		"a": rawMessage("old", "alice@example.com"),
		"b": rawMessage("fresh", "bob@example.com"),
	}
	client := newMock([]cfg_acc.MessageFolderRef{{MessageID: "a", Folder: "Inbox"}}, messages)
	deps := testDeps(t, client, rep)
	opts := Options{OnMail: script, NoNotify: true}

	if err := checkAccount(context.Background(), deps, opts, rep, notifier,
		deps.Config.Accounts[0], deps.Now); err != nil {
		t.Fatal(err)
	}
	client.AccountLatestRefs = []cfg_acc.MessageFolderRef{{MessageID: "b", Folder: "Inbox"}}
	if err := checkAccount(context.Background(), deps, opts, rep, notifier,
		deps.Config.Accounts[0], deps.Now); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("hook did not write its output: %v", err)
	}
	if got := strings.TrimSpace(string(data)); got != "work|fresh|b" {
		t.Fatalf("hook saw %q, want %q", got, "work|fresh|b")
	}
}

func TestCheckAccountFetchFailureIsReturned(t *testing.T) {
	rep := &recordingReporter{}
	client := newMock(nil, nil)
	client.FailOn = map[string]error{"FetchLatestAccountEmails": errors.New("dns: no such host")}
	deps := testDeps(t, client, rep)

	err := checkAccount(context.Background(), deps, Options{}, rep, &fakeNotifier{},
		deps.Config.Accounts[0], deps.Now)
	if err == nil || !strings.Contains(err.Error(), "dns: no such host") {
		t.Fatalf("err = %v, want the fetch failure", err)
	}
}

func TestPollOnceContinuesAfterOneAccountFails(t *testing.T) {
	rep := &recordingReporter{}
	good := newMock([]cfg_acc.MessageFolderRef{{MessageID: "a", Folder: "Inbox"}},
		map[email.ID][]byte{"a": rawMessage("hi", "a@example.com")})
	bad := newMock(nil, nil)
	bad.FailOn = map[string]error{"FetchLatestAccountEmails": errors.New("offline")}

	deps := Deps{
		Config: &cfg_g.Config{
			Paths: cfg_g.Paths{CacheRoot: t.TempDir()},
			Accounts: []cfg_acc.AccountConfig{
				{Name: "bad", Type: "jmap"},
				{Name: "good", Type: "jmap"},
			},
		},
		Reporter: rep,
		ClientFor: func(acc cfg_acc.AccountConfig) (mailclient.Reader, error) {
			if acc.Name == "bad" {
				return bad, nil
			}
			return good, nil
		},
		Now: fixedNow,
	}

	err := pollOnce(context.Background(), deps, Options{}, rep, &fakeNotifier{}, deps.Config.Accounts, deps.Now)
	if err == nil || !strings.Contains(err.Error(), "bad") {
		t.Fatalf("pollOnce err = %v, want the bad account named", err)
	}
	state, loadErr := LoadState(deps.Config.CacheRoot)
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if !state.Known("good") {
		t.Fatal("the healthy account was not polled after the failing one")
	}
}

func TestWatchedAccounts(t *testing.T) {
	config := &cfg_g.Config{Accounts: []cfg_acc.AccountConfig{{Name: "work"}, {Name: "home"}}}

	all, err := watchedAccounts(config, "")
	if err != nil || len(all) != 2 {
		t.Fatalf("watchedAccounts(all) = %v, %v; want both accounts", all, err)
	}
	one, err := watchedAccounts(config, "home")
	if err != nil || len(one) != 1 || one[0].Name != "home" {
		t.Fatalf("watchedAccounts(home) = %v, %v", one, err)
	}
	if _, err := watchedAccounts(config, "missing"); err == nil {
		t.Fatal("an unknown account name must be an error")
	}
	if _, err := watchedAccounts(&cfg_g.Config{}, ""); !errors.Is(err, cfg_g.ErrNoAccounts) {
		t.Fatalf("watchedAccounts(no accounts) err = %v, want ErrNoAccounts", err)
	}
}

func TestFlushOutboxHonoursReadOnlyAndNoFlush(t *testing.T) {
	rep := &recordingReporter{}
	flushed := 0
	deps := Deps{Flush: func() error { flushed++; return nil }}

	deps.Config = &cfg_g.Config{ReadOnly: true}
	flushOutbox(deps, Options{}, rep)
	if flushed != 0 {
		t.Fatal("flush ran under --read-only")
	}

	deps.Config = &cfg_g.Config{DryRun: true}
	flushOutbox(deps, Options{}, rep)
	if flushed != 0 {
		t.Fatal("flush ran under --dry-run")
	}

	deps.Config = &cfg_g.Config{}
	flushOutbox(deps, Options{NoFlush: true}, rep)
	if flushed != 0 {
		t.Fatal("flush ran with --no-flush")
	}

	flushOutbox(deps, Options{}, rep)
	if flushed != 1 {
		t.Fatalf("flush ran %d times, want once", flushed)
	}
}

func TestBackoffGrowsAndCaps(t *testing.T) {
	if got := backoff(DefaultInterval); got != 2*DefaultInterval {
		t.Fatalf("backoff(default) = %s, want %s", got, 2*DefaultInterval)
	}
	if got := backoff(MaxBackoff); got != MaxBackoff {
		t.Fatalf("backoff(max) = %s, want the cap %s", got, MaxBackoff)
	}
	if got := backoff(time.Second); got != DefaultInterval {
		t.Fatalf("backoff(below default) = %s, want the floor %s", got, DefaultInterval)
	}
}

func TestSleepStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if sleep(ctx, time.Hour) {
		t.Fatal("sleep returned true after cancellation")
	}
	if !sleep(context.Background(), time.Millisecond) {
		t.Fatal("sleep returned false without cancellation")
	}
}
