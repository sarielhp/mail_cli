package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/outbox"
)

func TestOutboxCmdListEmpty(t *testing.T) {
	var buf bytes.Buffer
	session := &Session{
		Config: &cfg_g.Config{},
		Out:    &buf,
		Deps: Deps{
			Outbox: stubOutbox{
				ListFn: func(*cfg_g.Config) ([]outbox.Entry, error) {
					return nil, nil
				},
			},
		},
	}

	cliApp := InitCLI(session)
	cliApp.Stdout = &buf
	if err := cliApp.Execute([]string{"msg", "outbox", "ls"}); err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	if !strings.Contains(buf.String(), "Outbox is empty") {
		t.Errorf("expected 'Outbox is empty', got %q", buf.String())
	}
}

func TestOutboxCmdListEntriesAndJSON(t *testing.T) {
	entries := []outbox.Entry{
		{
			ID:        "20261011-120000-0001",
			Account:   "work",
			To:        []string{"alice@example.com"},
			Subject:   "Status Update",
			CreatedAt: time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC),
			Attempts:  1,
		},
	}

	var buf bytes.Buffer
	session := &Session{
		Config: &cfg_g.Config{},
		Out:    &buf,
		Deps: Deps{
			Outbox: stubOutbox{
				ListFn: func(*cfg_g.Config) ([]outbox.Entry, error) {
					return entries, nil
				},
			},
		},
	}

	cliApp := InitCLI(session)
	cliApp.Stdout = &buf
	if err := cliApp.Execute([]string{"msg", "outbox", "ls"}); err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	if !strings.Contains(buf.String(), "Status Update") || !strings.Contains(buf.String(), "work") {
		t.Errorf("expected entry in output, got %q", buf.String())
	}

	// Test --json
	buf.Reset()
	if err := cliApp.Execute([]string{"--json", "msg", "outbox", "ls"}); err != nil {
		t.Fatalf("execute --json failed: %v", err)
	}
	if !strings.Contains(buf.String(), `"Status Update"`) {
		t.Errorf("expected json output, got %q", buf.String())
	}
}

func TestOutboxCmdFlushAndRemove(t *testing.T) {
	flushCalled := false
	removeCalled := false

	var buf bytes.Buffer
	session := &Session{
		Config: &cfg_g.Config{},
		Out:    &buf,
		Deps: Deps{
			Outbox: stubOutbox{
				FlushFn: func(*cfg_g.Config) ([]outbox.FlushResult, error) {
					flushCalled = true
					return []outbox.FlushResult{
						{
							ID:      "id-1",
							Account: "work",
							Subject: "Report",
							Success: true,
						},
					}, nil
				},
				RemoveFn: func(_ *cfg_g.Config, _ string) error {
					removeCalled = true
					return nil
				},
			},
		},
	}

	cliApp := InitCLI(session)
	cliApp.Stdout = &buf
	if err := cliApp.Execute([]string{"msg", "outbox", "flush"}); err != nil {
		t.Fatalf("flush failed: %v", err)
	}
	if !flushCalled {
		t.Error("expected FlushFn called")
	}
	if !strings.Contains(buf.String(), "Flushed 1 message(s): 1 sent") {
		t.Errorf("unexpected flush output: %q", buf.String())
	}

	buf.Reset()
	if err := cliApp.Execute([]string{"msg", "outbox", "rm", "id-1"}); err != nil {
		t.Fatalf("rm failed: %v", err)
	}
	if !removeCalled {
		t.Error("expected RemoveFn called")
	}
	if !strings.Contains(buf.String(), "Removed outbox message id-1") {
		t.Errorf("unexpected rm output: %q", buf.String())
	}
}

func TestOutboxFlushFailsWhenAMessageFailed(t *testing.T) {
	var buf bytes.Buffer
	session := &Session{
		Config: &cfg_g.Config{},
		Out:    &buf,
		Deps: Deps{Outbox: stubOutbox{FlushFn: func(*cfg_g.Config) ([]outbox.FlushResult, error) {
			return []outbox.FlushResult{
				{ID: "a", Success: true},
				{ID: "b", Success: false, Err: "offline"},
			}, nil
		}}},
	}
	if err := runOutboxFlush(session); err == nil || !strings.Contains(err.Error(), "1 of 2") {
		t.Errorf("a failed send must fail the command, got %v", err)
	}
}

func TestOutboxFlushDryRunSendsNothing(t *testing.T) {
	var buf bytes.Buffer
	session := &Session{
		Config: &cfg_g.Config{DryRun: true},
		Out:    &buf,
		Deps: Deps{Outbox: stubOutbox{
			FlushFn: func(*cfg_g.Config) ([]outbox.FlushResult, error) {
				t.Fatal("flush must not run under --dry-run")
				return nil, nil
			},
			ListFn: func(*cfg_g.Config) ([]outbox.Entry, error) {
				return []outbox.Entry{{ID: "a", Account: "work", Subject: "Hi"}}, nil
			},
		}},
	}
	if err := runOutboxFlush(session); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "[DRY-RUN] Would send [work] Hi (a)") {
		t.Errorf("unexpected output %q", buf.String())
	}
}
