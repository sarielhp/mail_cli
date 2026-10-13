package cli

import (
	"errors"
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarielhp/mail_cli/cache"
	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"

	"github.com/sarielhp/clihelp"
)

func setupTestSessionWithLabels(t *testing.T, labels []string) *Session {
	t.Helper()
	tmpDir := t.TempDir()

	configJSON := `{
		"accounts": [
			{"name": "test-account", "type": "gmail", "username": "user@example.com", "password": "app-password", "received_folder": "INBOX"}
		]
	}`
	cfgPath := filepath.Join(tmpDir, "config.json")
	if err := os.WriteFile(cfgPath, []byte(configJSON), 0600); err != nil {
		t.Fatalf("failed to write config.json: %v", err)
	}

	cacheDir := filepath.Join(tmpDir, "test-account")
	if err := os.MkdirAll(cacheDir, 0700); err != nil {
		t.Fatalf("failed to create cache dir: %v", err)
	}

	cs := &cache.DiskCacheStore{Dir: cachedir.At(cacheDir)}
	var items []cfg_acc.Item
	for _, l := range labels {
		items = append(items, cfg_acc.Item{Name: l, FullName: l})
	}
	if err := cs.SaveLabelItems(items); err != nil {
		t.Fatalf("failed to save label items: %v", err)
	}

	cfg := &cfg_g.Config{
		Paths: cfg_g.Paths{
			ConfigDir: tmpDir,
			CacheRoot: tmpDir,
		},
		Accounts: []cfg_acc.AccountConfig{
			{Name: "test-account", Type: "gmail", Username: "user@example.com", Password: "app-password", ReceivedFolder: "INBOX"},
		},
		SelectedAccount: "test-account",
	}

	return &Session{Config: cfg}
}

// assertFolderPrefixError runs resolveTuiFolderPrefix for pattern and checks it
// fails as a usage error whose message contains every want substring.
func assertFolderPrefixError(t *testing.T, session *Session, pattern string, want ...string) {
	t.Helper()
	prefix, notice, err := resolveTuiFolderPrefix(session, pattern)
	if err == nil {
		t.Fatalf("expected error for pattern %q, got prefix=%q notice=%q", pattern, prefix, notice)
	}
	if !errors.Is(err, clihelp.ErrUsage) {
		t.Errorf("expected ErrUsage, got: %v", err)
	}
	for _, substr := range want {
		if !strings.Contains(err.Error(), substr) {
			t.Errorf("expected error to mention %q, got: %v", substr, err)
		}
	}
}

func TestResolveTuiFolderPrefix_ZeroMatches(t *testing.T) {
	session := setupTestSessionWithLabels(t, []string{"Inbox", "Sent"})

	assertFolderPrefixError(t, session, "archive",
		`no folder matches "archive"`,
		`Run "mail_cli labels" to list folders`)
}

func TestResolveTuiFolderPrefix_AmbiguousMatches(t *testing.T) {
	session := setupTestSessionWithLabels(t, []string{"Receipts 2025", "Receipts 2026"})

	assertFolderPrefixError(t, session, "receipts",
		`2 folders match "receipts"`,
		"Receipts 2025, Receipts 2026")
}

func TestResolveTuiFolderPrefix_CacheUnavailable(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &cfg_g.Config{
		Paths: cfg_g.Paths{
			ConfigDir: tmpDir,
			CacheRoot: tmpDir,
		},
		Accounts: []cfg_acc.AccountConfig{
			{Name: "test-account", Type: "gmail", ReceivedFolder: "INBOX"},
		},
		SelectedAccount: "test-account",
	}
	session := &Session{Config: cfg}

	prefix, notice, err := resolveTuiFolderPrefix(session, "work")
	if err != nil {
		t.Fatalf("expected nil error on cache unavailable, got: %v", err)
	}
	if prefix != "" {
		t.Errorf("expected empty prefix when cache unavailable, got: %q", prefix)
	}
	expectedNotice := "Labels cache not available; opening TUI without folder filter \"work\""
	if notice != expectedNotice {
		t.Errorf("expected notice %q, got %q", expectedNotice, notice)
	}
}

func TestTuiCommand_CacheUnavailable_PassesNoticeToLauncher(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &cfg_g.Config{
		Paths: cfg_g.Paths{
			ConfigDir: tmpDir,
			CacheRoot: tmpDir,
		},
		Accounts: []cfg_acc.AccountConfig{
			{Name: "test-account", Type: "gmail", Username: "user@example.com", Password: "app-password", ReceivedFolder: "INBOX"},
		},
		SelectedAccount: "test-account",
	}

	var launchedPrefix string
	var launchedNotices []string
	session := &Session{
		Config: cfg,
		Deps: Deps{
			TUI: stubTUI{
				RunFn: func(_ *cfg_g.Config, labelPrefix string, notices ...string) error {
					launchedPrefix = labelPrefix
					launchedNotices = notices
					return nil
				},
			},
		},
	}

	cliApp := InitCLI(session)
	if err := cliApp.Execute([]string{"tui", "%work"}); err != nil {
		t.Fatalf("unexpected execute error: %v", err)
	}

	if launchedPrefix != "" {
		t.Errorf("expected empty prefix, got %q", launchedPrefix)
	}
	if len(launchedNotices) == 0 {
		t.Fatalf("expected initial notice passed to TUI launcher")
	}
	expectedNotice := "Labels cache not available; opening TUI without folder filter \"work\""
	if launchedNotices[0] != expectedNotice {
		t.Errorf("expected notice %q, got %q", expectedNotice, launchedNotices[0])
	}
}

func TestTuiCommand_AmbiguousMatch_ReturnsUsageError(t *testing.T) {
	session := setupTestSessionWithLabels(t, []string{"Receipts 2025", "Receipts 2026"})
	session.Deps.TUI = stubTUI{
		RunFn: func(_ *cfg_g.Config, _ string, _ ...string) error {
			return nil
		},
	}

	cliApp := InitCLI(session)
	err := cliApp.Execute([]string{"tui", "%receipts"})
	if err == nil {
		t.Fatal("expected error executing tui with ambiguous folder match, got nil")
	}
	if !errors.Is(err, clihelp.ErrUsage) {
		t.Errorf("expected ErrUsage, got: %v", err)
	}
}
