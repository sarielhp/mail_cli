package cli

import (
	"bytes"
	"errors"
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/label"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sarielhp/mail_cli/cache"
	"github.com/sarielhp/mail_cli/cache/folderindex"
	"github.com/sarielhp/mail_cli/cache/msg"
	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/mailclient"
)

func TestResolveUniqueLabel(t *testing.T) {
	mockClient := &mailclient.MockMailClient{
		Labels: []string{
			"INBOX",
			"Work",
			"Work/ProjectA",
			"Work/ProjectB",
			"Personal/Finance",
			"Archive",
		},
		Cfg: &cfg_g.Config{Policy: cfg_g.Policy{ReceivedFolder: "INBOX"}},
	}

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{"Exact match", "Work", "Work", false},
		{"Case-insensitive exact match", "work", "Work", false},
		{"Ambiguous prefix match", "Work/Project", "", true},
		{"Unique suffix match", "Finance", "Personal/Finance", false},
		{"Unique prefix match", "Personal", "Personal/Finance", false},
		{"Inbox alias resolution", "inbox", "INBOX", false},
		{"Not found", "Nonexistent", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveUniqueLabel(mockClient, tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("resolveUniqueLabel() error = %v, wantErr = %v", err, tt.wantErr)
			}
			if !tt.wantErr && got.String() != tt.want {
				t.Errorf("resolveUniqueLabel() got = %q, want = %q", got, tt.want)
			}
		})
	}
}

func TestResolveUniqueLabelError(t *testing.T) {
	mockErrClient := &mailclient.MockMailClient{
		Err: errors.New("API error"),
	}
	_, err := resolveUniqueLabel(mockErrClient, "inbox")
	if err == nil {
		t.Error("expected error from client failure, got nil")
	}
}

func TestSearchLabels(t *testing.T) {
	tmpDir := t.TempDir()
	config := &cfg_g.Config{Accounts: []cfg_acc.AccountConfig{
		{Name: "test-account", Type: "gmail", ReceivedFolder: "INBOX", SpamFolder: "Spam"},
	}, SelectedAccount: "test-account", Paths: cfg_g.Paths{ConfigDir: tmpDir, CacheRoot: tmpDir}}

	configJSON := `{
		"accounts": [
			{"name": "test-account", "type": "gmail", "received_folder": "INBOX", "spam_folder": "Spam"}
		]
	}`
	err := os.WriteFile(filepath.Join(tmpDir, "config.json"), []byte(configJSON), 0600)
	if err != nil {
		t.Fatalf("failed to write config.json: %v", err)
	}

	cacheDir := filepath.Join(tmpDir, "test-account")
	err = os.MkdirAll(cacheDir, 0700)
	if err != nil {
		t.Fatalf("failed to create cache dir: %v", err)
	}

	cs := &cache.DiskCacheStore{Dir: cachedir.At(cacheDir)}
	items := []cfg_acc.Item{
		{Name: "INBOX", FullName: "INBOX"},
		{Name: "Work/ProjectA", FullName: "Work/ProjectA"},
		{Name: "Work/ProjectB", FullName: "Work/ProjectB"},
		{Name: "Personal/Finance", FullName: "Personal/Finance"},
	}
	err = cs.SaveLabelItems(items)
	if err != nil {
		t.Fatalf("failed to save label items: %v", err)
	}

	tests := []struct {
		name     string
		patterns []string
		want     []string
	}{
		{"Single match", []string{"Finance"}, []string{"Personal/Finance"}},
		{"Conjunction match", []string{"Work", "ProjectA"}, []string{"Work/ProjectA"}},
		{"Multiple matches", []string{"Project"}, []string{"Work/ProjectA", "Work/ProjectB"}},
		{"No match", []string{"Work", "Finance"}, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SearchLabels(config, tt.patterns)
			if err != nil {
				t.Fatalf("SearchLabels() returned unexpected error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("SearchLabels() returned %v, want %v", got, tt.want)
			}
			for i, v := range got {
				if v != tt.want[i] {
					t.Errorf("SearchLabels() got[%d] = %q, want %q", i, v, tt.want[i])
				}
			}
		})
	}
}

// TestSplitSameAccountUsesMove guards against the regression where split
// compared clients by pointer -- always false, since each resolution allocates a
// new CheckingMailClient -- and so duplicated same-account messages by uploading
// a copy to the target and moving the original to Trash.
func TestSplitSameAccountUsesMove(t *testing.T) {
	cfg := &cfg_g.Config{SelectedAccount: "acct-a", Paths: cfg_g.Paths{CacheRoot: t.TempDir()}, AccountCache: cachedir.At(t.TempDir())}
	mkClient := func() mailclient.MailClient {
		// A distinct pointer each call, as NewMailClient produces in production.
		return &mailclient.MockMailClient{Cfg: cfg, BackendTypeVal: "gmail"}
	}
	a, b := mkClient(), mkClient()

	if a == b {
		t.Fatal("test premise broken: expected distinct client pointers")
	}
	if !isSameAccount(a, b) {
		t.Error("isSameAccount must recognise two clients for the same account despite differing pointers")
	}

	// And a genuinely different account must still be treated as cross-account.
	other := &mailclient.MockMailClient{
		Cfg:            &cfg_g.Config{SelectedAccount: "acct-b", Paths: cfg_g.Paths{CacheRoot: t.TempDir()}, AccountCache: cachedir.At(t.TempDir())},
		BackendTypeVal: "gmail",
	}
	if isSameAccount(a, other) {
		t.Error("isSameAccount must not conflate different accounts")
	}
}

// TestExecuteSplitMove_PerAccountReadOnlySkipsLocalReindex is a regression
// for a bug the H2 fix itself introduced: it gated the local folderindex
// reindex on session.Config.ReadOnly (the global --read-only/--dry-run
// flag), but an account can carry its own `read_only: true` without that
// global flag being set -- mailops.BuildAccountLocalConfig ORs it into the
// client's own Config().ReadOnly. That account's MoveEmail is still only
// simulated by ReadOnlyMailClient in production, so the gate has to read
// the client's config, not the session's.
// TestExecuteSplitMove_PerAccountReadOnly wraps srcClient in
// CheckingMailClient the same way mailops.NewMailClient always does in
// production, since split no longer reindexes manually (that was found to
// duplicate CheckingMailClient.MoveEmail's own reindexMoved, once with the
// wrong ReadOnly source). It checks both the read-only and the ordinary
// case, so the read-only assertion means something: without the contrast,
// "nothing moved" would be true whether the gate worked or nothing ever
// reindexes at all.
func TestExecuteSplitMove_PerAccountReadOnly(t *testing.T) {
	tests := []struct {
		name       string
		readOnly   bool
		wantDest   int
		wantSrcLen int
	}{
		{"read-only account: move is simulated, index untouched", true, 0, 1},
		{"ordinary account: move reindexes as usual", false, 1, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			accDir := t.TempDir()
			// session.Config below stays false: no --read-only/--dry-run
			// was passed. Only the account's own setting (cfg) varies.
			cfg := &cfg_g.Config{SelectedAccount: "acct-a", Paths: cfg_g.Paths{CacheRoot: accDir}, AccountCache: cachedir.At(accDir), ReadOnly: tt.readOnly}
			mockSrc := &mailclient.MockMailClient{Cfg: cfg, BackendTypeVal: "gmail"}
			mockTarget := &mailclient.MockMailClient{Cfg: cfg, BackendTypeVal: "gmail"}
			srcClient := mailclient.NewCheckingMailClient(mockSrc, "acct-a", &cache.NoOpCacheStore{})
			targetClient := mailclient.NewCheckingMailClient(mockTarget, "acct-a", &cache.NoOpCacheStore{})

			srcID := "msg001"
			eml := "Message-ID: <msg-1@test.com>\nDate: Wed, 02 Sep 2026 10:00:00 +0000\nFrom: user@test.com\nSubject: Item 1\n\nContent"
			if err := msg.Store(cachedir.At(accDir), email.ID(srcID), []byte(eml), time.Now()); err != nil {
				t.Fatalf("msg.Store: %v", err)
			}
			if err := folderindex.Add(cachedir.At(accDir), "INBOX", email.ID(srcID)); err != nil {
				t.Fatalf("folderindex.Add: %v", err)
			}

			session := &Session{Config: &cfg_g.Config{}, Out: &bytes.Buffer{}, Err: &bytes.Buffer{}}
			if err := executeSplitMove(session, srcClient, targetClient, []email.ID{email.ID(srcID)}, label.FromServer("INBOX"), label.FromServer("Archive"), cachedir.At(accDir), "src", "dst"); err != nil {
				t.Fatalf("executeSplitMove: %v", err)
			}

			destIDs, _ := folderindex.IDs(cachedir.At(accDir), "Archive")
			if len(destIDs) != tt.wantDest {
				t.Errorf("expected %d message(s) in the destination index, got %v", tt.wantDest, destIDs)
			}
			srcIDs, err := folderindex.IDs(cachedir.At(accDir), "INBOX")
			if err != nil || len(srcIDs) != tt.wantSrcLen {
				t.Errorf("expected source folder index length %d, got %v (err %v)", tt.wantSrcLen, srcIDs, err)
			}
		})
	}
}

func TestFilterMessagesBySubjectPattern_Precompiled(t *testing.T) {
	tmpDir := t.TempDir()
	d := time.Date(2026, 9, 18, 10, 0, 0, 0, time.UTC)

	_ = msg.Store(cachedir.At(tmpDir), "m1", []byte("Subject: Monthly Statement 2026\n\nbody"), d)
	_ = msg.Store(cachedir.At(tmpDir), "m2", []byte("Subject: Lunch tomorrow?\n\nbody"), d)
	_ = msg.Store(cachedir.At(tmpDir), "m3", []byte("Subject: Urgent: statement required\n\nbody"), d)

	matches, err := filterMessagesBySubjectPattern(cachedir.At(tmpDir), []email.ID{"m1", "m2", "m3"}, "*statement*")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matches) != 2 {
		t.Fatalf("expected 2 matches, got %d: %v", len(matches), matches)
	}
	if matches[0] != "m1" || matches[1] != "m3" {
		t.Errorf("expected [m1, m3], got %v", matches)
	}

	none, err := filterMessagesBySubjectPattern(cachedir.At(tmpDir), []email.ID{"m1", "m2", "m3"}, "*nonexistent*")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(none) != 0 {
		t.Errorf("expected 0 matches, got %v", none)
	}
}
