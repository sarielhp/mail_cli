package cli

import (
	"bytes"
	"errors"
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/mailclient"
)

func TestLabelsListFull(t *testing.T) {
	mockClient := &mailclient.MockMailClient{
		LabelItems: []cfg_acc.Item{
			{Name: "Inbox", FullName: "Inbox", MessagesTotal: 5},
			{Name: "wuna", FullName: "sort/mailing_list/wuna", MessagesTotal: 2},
			{Name: "empty", FullName: "sort/empty", MessagesTotal: 0},
		},
		Cfg: &cfg_g.Config{},
	}

	session := &Session{
		Config: mockClient.Cfg,
		Deps: Deps{
			Clients: stubClients{
				ForSelectedFn: func(_ *cfg_g.Config) (mailclient.MailClient, error) {
					return mockClient, nil
				},
			},
		},
	}

	cliApp := InitCLI(session)

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := cliApp.Execute([]string{"labels", "list", "full"})

	_ = w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("unexpected error running labels list full: %v", err)
	}

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	output := buf.String()

	if !strings.Contains(output, "Inbox") {
		t.Errorf("expected output to contain Inbox, got %q", output)
	}
	if !strings.Contains(output, "sort/mailing_list/wuna") {
		t.Errorf("expected output to contain sort/mailing_list/wuna, got %q", output)
	}
	if strings.Contains(output, "sort/empty") {
		t.Errorf("expected zero-count label sort/empty to be hidden when -a is not passed, got %q", output)
	}

	// Now test with -a (all labels)
	flagLabelsListAll = false
	oldStdout = os.Stdout
	r2, w2, _ := os.Pipe()
	os.Stdout = w2

	err = cliApp.Execute([]string{"labels", "list", "full", "-a"})

	_ = w2.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("unexpected error running labels list full -a: %v", err)
	}

	var buf2 bytes.Buffer
	_, _ = io.Copy(&buf2, r2)
	outputAll := buf2.String()

	if !strings.Contains(outputAll, "sort/empty") {
		t.Errorf("expected output to contain sort/empty when -a is passed, got %q", outputAll)
	}

	// Test labels list full ext
	flagLabelsListAll = false
	mockClient.LabelItems = []cfg_acc.Item{
		{Name: "Inbox", FullName: "Inbox", MessagesUnread: 2, MessagesTotal: 10},
		{Name: "wuna", FullName: "sort/mailing_list/wuna", MessagesUnread: 0, MessagesTotal: 5},
	}

	oldStdout = os.Stdout
	r3, w3, _ := os.Pipe()
	os.Stdout = w3

	err = cliApp.Execute([]string{"labels", "list", "full", "ext"})

	_ = w3.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("unexpected error running labels list full ext: %v", err)
	}

	var buf3 bytes.Buffer
	_, _ = io.Copy(&buf3, r3)
	outputExt := buf3.String()

	if !strings.Contains(outputExt, "Inbox (2/10)") {
		t.Errorf("expected output to contain 'Inbox (2/10)', got %q", outputExt)
	}
	if !strings.Contains(outputExt, "sort/mailing_list/wuna (0/5)") {
		t.Errorf("expected output to contain 'sort/mailing_list/wuna (0/5)', got %q", outputExt)
	}

	// Test labels list full ext nozero (filters out zero-message folders)
	flagLabelsListAll = false
	mockClient.LabelItems = []cfg_acc.Item{
		{Name: "Inbox", FullName: "Inbox", MessagesUnread: 2, MessagesTotal: 10},
		{Name: "wuna", FullName: "sort/mailing_list/wuna", MessagesUnread: 0, MessagesTotal: 5},
		{Name: "empty", FullName: "sort/empty", MessagesUnread: 0, MessagesTotal: 0},
	}

	oldStdout = os.Stdout
	r4, w4, _ := os.Pipe()
	os.Stdout = w4

	err = cliApp.Execute([]string{"labels", "list", "full", "ext", "nozero"})

	_ = w4.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("unexpected error running labels list full ext nozero: %v", err)
	}

	var buf4 bytes.Buffer
	_, _ = io.Copy(&buf4, r4)
	outputNoZero := buf4.String()

	if !strings.Contains(outputNoZero, "Inbox (2/10)") {
		t.Errorf("expected output to contain 'Inbox (2/10)', got %q", outputNoZero)
	}
	if !strings.Contains(outputNoZero, "sort/mailing_list/wuna (0/5)") {
		t.Errorf("expected output to contain 'sort/mailing_list/wuna (0/5)', got %q", outputNoZero)
	}
	if strings.Contains(outputNoZero, "sort/empty") {
		t.Errorf("expected zero-message folder 'sort/empty' to be omitted when 'nozero' is passed, got %q", outputNoZero)
	}
}

func TestLabelsCount(t *testing.T) {
	mockClient1 := &mailclient.MockMailClient{
		LabelItems: []cfg_acc.Item{
			{Name: "Inbox", FullName: "Inbox", MessagesUnread: 5, MessagesTotal: 42},
			{Name: "wuna", FullName: "sort/mailing_list/wuna", MessagesUnread: 1, MessagesTotal: 10},
		},
		Cfg: &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: "/tmp/acc1"}, AccountCache: cachedir.At("/tmp/acc1")},
	}
	mockClient2 := &mailclient.MockMailClient{
		LabelItems: []cfg_acc.Item{
			{Name: "received", FullName: "received", MessagesUnread: 2, MessagesTotal: 7},
		},
		Cfg: &cfg_g.Config{SelectedAccount: "acc2", Paths: cfg_g.Paths{CacheRoot: "/tmp/acc2"}, AccountCache: cachedir.At("/tmp/acc2")},
	}

	session := &Session{
		Config: &cfg_g.Config{Accounts: []cfg_acc.AccountConfig{
			{Name: "acc1", Type: "gmail"},
			{Name: "acc2", Type: "jmap"},
		}, Paths: cfg_g.Paths{CacheRoot: "/tmp/download"}},
		Deps: Deps{
			Clients: stubClients{
				ForSelectedFn: func(cfg *cfg_g.Config) (mailclient.MailClient, error) {
					if strings.EqualFold(string(cfg.SelectedAccount), "acc2") {
						return mockClient2, nil
					}
					return mockClient1, nil
				},
			},
		},
	}

	cliApp := InitCLI(session)

	// 1. Quiet total count
	out, err := captureOutput(func() error {
		return cliApp.Execute([]string{"labels", "count", "%inbox", "-q"})
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.TrimSpace(out) != "42" {
		t.Errorf("expected '42', got %q", out)
	}

	// 2. Quiet unread count
	out, err = captureOutput(func() error {
		return cliApp.Execute([]string{"labels", "count", "%inbox", "-q", "-u"})
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.TrimSpace(out) != "5" {
		t.Errorf("expected '5', got %q", out)
	}

	// 3. Verbose count
	out, err = captureOutput(func() error {
		return cliApp.Execute([]string{"labels", "count", "%inbox"})
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "42 message(s)") || !strings.Contains(out, "5 message(s)") {
		t.Errorf("expected verbose output to contain counts, got %q", out)
	}

	// 4. Suffix match
	out, err = captureOutput(func() error {
		return cliApp.Execute([]string{"labels", "count", "%wuna", "-q"})
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.TrimSpace(out) != "10" {
		t.Errorf("expected '10', got %q", out)
	}

	// 5. Account prefix
	out, err = captureOutput(func() error {
		return cliApp.Execute([]string{"labels", "count", "%acc2:received", "-q"})
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.TrimSpace(out) != "7" {
		t.Errorf("expected '7', got %q", out)
	}

	// 6. Non-existent label
	_, err = captureOutput(func() error {
		return cliApp.Execute([]string{"labels", "count", "%nonexistent"})
	})
	if err == nil {
		t.Fatalf("expected error for nonexistent label, got nil")
	}
}

func captureOutput(fn func() error) (string, error) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := fn()

	_ = w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	return buf.String(), err
}

func TestAsyncRefreshLabelsCache_PanicRecovery(_ *testing.T) {
	// Verify that panic in GetClient is recovered safely
	sessionPanic := &Session{
		Config: &cfg_g.Config{},
		Deps: Deps{
			Clients: stubClients{
				ForSelectedFn: func(_ *cfg_g.Config) (mailclient.MailClient, error) {
					panic("simulated panic in GetClient")
				},
			},
		},
	}
	// Must not panic or crash
	asyncRefreshLabelsCache(sessionPanic)

	// Verify normal error does not panic
	sessionErr := &Session{
		Config: &cfg_g.Config{},
		Deps: Deps{
			Clients: stubClients{
				ForSelectedFn: func(_ *cfg_g.Config) (mailclient.MailClient, error) {
					return nil, errors.New("client creation failed")
				},
			},
		},
	}
	asyncRefreshLabelsCache(sessionErr)
}
