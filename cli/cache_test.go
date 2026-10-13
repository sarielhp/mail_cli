package cli

import (
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"github.com/sarielhp/mail_cli/email"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sarielhp/mail_cli/cache"
	"github.com/sarielhp/mail_cli/cache/folderindex"
	"github.com/sarielhp/mail_cli/cache/msg"
	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/mailclient"
)

func TestCachePruneWithFolder(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "mail_cli_cache_prune_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	subDir := filepath.Join(tempDir, "inbox")
	_ = os.MkdirAll(subDir, 0700)

	testFile := filepath.Join(subDir, "msg1.eml")
	_ = os.WriteFile(testFile, []byte("email"), 0600)

	session := &Session{
		Config: &cfg_g.Config{SelectedAccount: "inbox", Accounts: []cfg_acc.AccountConfig{
			{Name: "inbox", Type: "gmail"},
		}, Paths: cfg_g.Paths{CacheRoot: tempDir}},
	}
	cliApp := InitCLI(session)

	if err := cliApp.Execute([]string{"utils", "cache", "prune", "0"}); err != nil {
		t.Fatalf("failed to execute cache prune command: %v", err)
	}

	if _, err := os.Stat(testFile); !os.IsNotExist(err) {
		t.Errorf("expected file %s to be deleted, but it still exists", testFile)
	}
}

func TestCacheReset(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "mail_cli_cache_reset_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	accDir := filepath.Join(tempDir, "test_account")
	otherDir := filepath.Join(tempDir, "other_account")
	_ = os.MkdirAll(accDir, 0700)
	_ = os.MkdirAll(otherDir, 0700)

	accFile := filepath.Join(accDir, "msg.eml")
	otherFile := filepath.Join(otherDir, "msg.eml")
	historyFile := filepath.Join(tempDir, "suggested_contacts.json")
	_ = os.WriteFile(accFile, []byte("email"), 0600)
	_ = os.WriteFile(otherFile, []byte("email"), 0600)
	_ = os.WriteFile(historyFile, []byte(`{"contacts":[]}`), 0600)

	session := &Session{
		Config: &cfg_g.Config{SelectedAccount: "test_account", Accounts: []cfg_acc.AccountConfig{
			{Name: "test_account", Type: "gmail"},
			{Name: "other_account", Type: "gmail"},
		}, Paths: cfg_g.Paths{CacheRoot: tempDir}},
		Deps: Deps{
			Clients: stubClients{
				ForSelectedFn: func(config *cfg_g.Config) (mailclient.MailClient, error) {
					return &mailclient.MockMailClient{
						Cfg: config,
					}, nil
				},
			},
		},
	}
	cliApp := InitCLI(session)

	// Without a terminal there is nobody to prompt, so the command refuses
	// rather than guessing -- which is the point of the guard.
	if execErr := cliApp.Execute([]string{"utils", "cache", "reset"}); execErr == nil {
		t.Fatal("expected cache reset to refuse without --yes")
	}
	if _, statErr := os.Stat(accFile); os.IsNotExist(statErr) {
		t.Fatalf("the refused reset deleted %s anyway", accFile)
	}

	if execErr := cliApp.Execute([]string{"utils", "cache", "reset", "--yes"}); execErr != nil {
		t.Fatalf("failed to execute cache reset --yes: %v", execErr)
	}

	if _, statErr := os.Stat(accFile); !os.IsNotExist(statErr) {
		t.Errorf("expected file %s in target account cache to be deleted, but it still exists", accFile)
	}

	if _, statErr := os.Stat(otherFile); os.IsNotExist(statErr) {
		t.Errorf("expected file %s in other account cache to still exist, but it was deleted", otherFile)
	}

	if _, statErr := os.Stat(historyFile); !os.IsNotExist(statErr) {
		t.Errorf("expected contact history %s to be deleted, but it still exists", historyFile)
	}

	info, err := os.Stat(accDir)
	if err != nil {
		t.Errorf("expected per-account cache directory %s to be recreated, but it does not exist", accDir)
	} else if info.Mode().Perm()&0077 != 0 {
		t.Errorf("expected cache directory to have restricted permissions (0700), got %o", info.Mode().Perm())
	}
}

func TestCachePrune_RemovalFailureHandling(t *testing.T) {
	tempDir := t.TempDir()

	roDir := filepath.Join(tempDir, "readonly_dir")
	if err := os.MkdirAll(roDir, 0700); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}

	emlFile := filepath.Join(roDir, "test.eml")
	if err := os.WriteFile(emlFile, []byte("msg"), 0600); err != nil {
		t.Fatalf("failed to write eml: %v", err)
	}

	if err := os.Chmod(roDir, 0500); err != nil {
		t.Fatalf("failed to chmod: %v", err)
	}
	defer func() { _ = os.Chmod(roDir, 0700) }()

	cutoff := time.Now().Add(time.Hour)
	deleted, err := pruneCacheDir(cachedir.At(tempDir), cutoff)
	if err != nil {
		t.Fatalf("pruneCacheDir returned unexpected error: %v", err)
	}
	if deleted != 0 {
		t.Errorf("expected 0 deleted due to permission failure, got %d", deleted)
	}
}

func TestPruneScoresDir_RemovalFailureHandling(t *testing.T) {
	tempDir := t.TempDir()
	scoresDir := filepath.Join(tempDir, ".scores")
	if err := os.MkdirAll(scoresDir, 0700); err != nil {
		t.Fatalf("failed to create scores dir: %v", err)
	}
	scoreFile := filepath.Join(scoresDir, "score.json")
	if err := os.WriteFile(scoreFile, []byte("{}"), 0600); err != nil {
		t.Fatalf("failed to write score file: %v", err)
	}

	if err := os.Chmod(scoresDir, 0500); err != nil {
		t.Fatalf("failed to chmod: %v", err)
	}
	defer func() { _ = os.Chmod(scoresDir, 0700) }()

	cutoff := time.Now().Add(time.Hour)
	deleted := pruneScoresDir(scoresDir, cutoff)
	if deleted != 0 {
		t.Errorf("expected 0 deleted scores due to permission failure, got %d", deleted)
	}
}

func TestCachePrune_RemovesAuxiliarySummaries(t *testing.T) {
	tempDir := t.TempDir()
	now := time.Now()

	if err := msg.Store(cachedir.At(tempDir), "msg1", []byte("From: test\r\n\r\ndata"), now); err != nil {
		t.Fatalf("failed to store msg: %v", err)
	}
	if err := msg.StoreParsedSummary(cachedir.At(tempDir), "msg1", []byte("{}")); err != nil {
		t.Fatalf("failed to store parsed summary: %v", err)
	}
	if err := msg.StoreSummary(cachedir.At(tempDir), "msg1", "summary"); err != nil {
		t.Fatalf("failed to store summary: %v", err)
	}

	cutoff := now.Add(time.Hour)
	deleted, err := pruneCacheDir(cachedir.At(tempDir), cutoff)
	if err != nil {
		t.Fatalf("pruneCacheDir failed: %v", err)
	}
	if deleted != 1 {
		t.Errorf("expected 1 deleted eml, got %d", deleted)
	}

	exists, err := msg.Exists(cachedir.At(tempDir), "msg1")
	if err != nil {
		t.Errorf("msg.Exists failed: %v", err)
	}
	if exists {
		t.Error("expected msg1 to be deleted from cache")
	}
	if msg.HasSummary(cachedir.At(tempDir), "msg1") {
		t.Error("expected summary for msg1 to be deleted")
	}
	if _, err := msg.ReadParsedSummary(cachedir.At(tempDir), "msg1"); err == nil {
		t.Error("expected parsed summary for msg1 to be deleted")
	}
}

func TestCachePrune_PurgesMessageInfoAndFolderIndexes(t *testing.T) {
	tempDir := t.TempDir()
	now := time.Now()

	msgID := "prune-target-123"
	if err := msg.Store(cachedir.At(tempDir), email.ID(msgID), []byte("From: test@example.com\r\n\r\nHello"), now); err != nil {
		t.Fatalf("msg.Store: %v", err)
	}

	if err := folderindex.Add(cachedir.At(tempDir), "INBOX", email.ID(msgID)); err != nil {
		t.Fatalf("label.Add: %v", err)
	}

	foundID, folder, err := cache.FindCachedEmailByID(cachedir.At(tempDir), msgID)
	if err != nil || string(foundID) != msgID || folder != "inbox" {
		t.Fatalf("FindCachedEmailByID before prune: id=%s folder=%s err=%v", foundID, folder, err)
	}

	cutoff := now.Add(time.Hour)
	deleted, err := pruneCacheDir(cachedir.At(tempDir), cutoff)
	if err != nil {
		t.Fatalf("pruneCacheDir failed: %v", err)
	}
	if deleted != 1 {
		t.Errorf("expected 1 deleted file, got %d", deleted)
	}

	exists, err := msg.Exists(cachedir.At(tempDir), email.ID(msgID))
	if err != nil {
		t.Errorf("msg.Exists: %v", err)
	}
	if exists {
		t.Error("msg.Exists = true after pruneCacheDir; message_info was not cleaned")
	}

	_, _, err = cache.FindCachedEmailByID(cachedir.At(tempDir), msgID)
	if err == nil {
		t.Error("FindCachedEmailByID succeeded after prune")
	} else if !strings.Contains(err.Error(), "no cached email found") {
		t.Errorf("expected 'no cached email found' error, got %v", err)
	}

	ids, err := folderindex.IDs(cachedir.At(tempDir), "INBOX")
	if err != nil {
		t.Fatalf("label.IDs: %v", err)
	}
	for _, id := range ids {
		if string(id) == msgID {
			t.Errorf("folder INBOX still contains pruned message ID %s", msgID)
		}
	}
}

// cacheSession builds a Session whose account cache resolves under root for the
// named selected account and account list.
func cacheSession(root string, selected cfg_g.AccountName, accounts []cfg_acc.AccountConfig, clients ClientProvider) *Session {
	return &Session{
		Config: &cfg_g.Config{
			SelectedAccount: selected,
			Accounts:        accounts,
			Paths:           cfg_g.Paths{CacheRoot: root},
		},
		Deps: Deps{Clients: clients},
	}
}

func TestResolveAccountCacheDir_NoClientConstruction(t *testing.T) {
	tempDir := t.TempDir()
	failIfCalledClients := stubClients{
		ForSelectedFn: func(_ *cfg_g.Config) (mailclient.MailClient, error) {
			t.Fatal("ForSelected should not be called during cache dir resolution")
			return nil, nil
		},
	}

	tests := []struct {
		name     string
		session  *Session
		expected string
	}{
		{
			name:     "nil session",
			session:  nil,
			expected: "",
		},
		{
			name:     "nil config",
			session:  &Session{Config: nil, Deps: Deps{Clients: failIfCalledClients}},
			expected: "",
		},
		{
			name: "explicit selected account",
			session: cacheSession(tempDir, "work", []cfg_acc.AccountConfig{
				{Name: "work", Type: "gmail"},
				{Name: "personal", Type: "gmail"},
			}, failIfCalledClients),
			expected: filepath.Join(tempDir, "work"),
		},
		{
			name: "aliased selected account",
			session: cacheSession(tempDir, "alias-work", []cfg_acc.AccountConfig{
				{Name: "work", Aliases: []string{"alias-work"}, Type: "gmail"},
			}, failIfCalledClients),
			expected: filepath.Join(tempDir, "work"),
		},
		{
			name: "empty selected account defaults to first",
			session: cacheSession(tempDir, "", []cfg_acc.AccountConfig{
				{Name: "primary", Type: "gmail"},
				{Name: "secondary", Type: "gmail"},
			}, failIfCalledClients),
			expected: filepath.Join(tempDir, "primary"),
		},
		{
			name:    "no configured accounts returns base download dir",
			session: cacheSession(tempDir, "", nil, failIfCalledClients),
			// With no account there is no account directory -- not the root,
			// which holds drafts and mbox views a prune must not touch.
			expected: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveAccountCacheDir(tc.session)
			if got.Path() != tc.expected {
				t.Errorf("resolveAccountCacheDir() = %q, want %q", got, tc.expected)
			}
		})
	}
}

func TestCacheReset_ContactHistoryRemovalFailure(t *testing.T) {
	tempDir := t.TempDir()
	session := &Session{
		Config: &cfg_g.Config{
			SelectedAccount: "acc",
			Accounts: []cfg_acc.AccountConfig{
				{Name: "acc", Type: "gmail"},
			},
			Paths: cfg_g.Paths{CacheRoot: tempDir},
		},
	}
	historyPath := filepath.Join(tempDir, "suggested_contacts.json")
	if err := os.Mkdir(historyPath, 0700); err != nil {
		t.Fatalf("mkdir historyPath: %v", err)
	}
	blockingFile := filepath.Join(historyPath, "blocker")
	_ = os.WriteFile(blockingFile, []byte("x"), 0600)

	if err := runCacheReset(session); err != nil {
		t.Fatalf("runCacheReset failed unexpectedly: %v", err)
	}
}
