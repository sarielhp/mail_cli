package cli

import (
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"github.com/sarielhp/mail_cli/email"
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

// TestSpliceSameAccountMove_PerAccountReadOnlyLeavesTheLocalIndexUntouched
// is an end-to-end check that `label sort --move` on a per-account
// read-only account (cfg_acc.AccountConfig.ReadOnly, independent of any
// global --read-only/--dry-run flag) never records a move locally that the
// server never made. That property lives entirely in
// mailclient.CheckingMailClient.MoveEmail's own reindexMoved now (see
// mailclient.TestCheckingMailClient_MoveEmail_ReindexesAndRespectsReadOnly
// for the focused unit coverage) -- splice/split/msg no longer reindex
// manually, having been found to duplicate exactly that work, once with
// the wrong ReadOnly source (session.Config.ReadOnly instead of the
// client's own). This wraps the mock the same way
// mailops.NewMailClient always does in production, so it is the real code
// path, not a bypass of it, and includes the not-read-only case so the
// read-only assertion means something: without it, "nothing moved" would
// be true whether the gate worked or nothing ever reindexes at all.
func TestSpliceSameAccountMove_PerAccountReadOnlyLeavesTheLocalIndexUntouched(t *testing.T) {
	tests := []struct {
		name       string
		readOnly   bool
		wantMoved  bool
		wantSrcLen int
	}{
		{"read-only account: move is simulated, index untouched", true, false, 1},
		{"ordinary account: move reindexes as usual", false, true, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			accDir := filepath.Join(tempDir, "acc1")
			configDir := filepath.Join(tempDir, "config")
			_ = os.MkdirAll(accDir, 0700)
			_ = os.MkdirAll(configDir, 0700)

			fixedDate := time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC)
			srcID := "msg001"
			eml := "Message-ID: <msg-1@test.com>\nDate: Wed, 02 Sep 2026 10:00:00 +0000\nFrom: user@test.com\nSubject: Item 1\n\nContent"
			if err := msg.Store(cachedir.At(accDir), email.ID(srcID), []byte(eml), fixedDate); err != nil {
				t.Fatalf("failed to store %s: %v", srcID, err)
			}
			if err := folderindex.Add(cachedir.At(accDir), "received", email.ID(srcID)); err != nil {
				t.Fatalf("folderindex.Add: %v", err)
			}

			// printSpliceTotals re-fetches every folder involved after the
			// move to report final counts. CheckingMailClient.FetchAndDownloadEmails
			// treats an empty fetch result as "the server now reports this
			// folder empty" and wipes its local index to match -- so the
			// destination needs an entry too, or that post-move refetch
			// would erase what reindexMoved just correctly wrote, for a
			// reason with nothing to do with what this test checks.
			destFolder := "keep/2026/received_private-2026-09"
			mockClient := &mailclient.MockMailClient{
				Labels:        []string{"received"},
				DownloadedIDs: map[string][]email.ID{"received": {email.ID(srcID)}, destFolder: {email.ID(srcID)}},
				Cfg:           &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: accDir, ConfigDir: configDir}, AccountCache: cachedir.At(accDir), ReadOnly: tt.readOnly},
			}

			session := &Session{
				// The session's own (global) ReadOnly always stays false
				// here: no --read-only/--dry-run flag was passed. Only the
				// account's own setting (above) varies.
				Config: &cfg_g.Config{Accounts: []cfg_acc.AccountConfig{
					{Name: "acc1", Type: "jmap"},
				}, Paths: cfg_g.Paths{CacheRoot: tempDir, ConfigDir: configDir}},
				Deps: Deps{
					Clients: stubClients{
						ForSelectedFn: func(_ *cfg_g.Config) (mailclient.MailClient, error) {
							return mailclient.NewCheckingMailClient(mockClient, "acc1", &cache.NoOpCacheStore{}), nil
						},
					},
				},
			}

			cliApp := InitCLI(session)
			if err := cliApp.Execute([]string{"label", "sort", "%received", "-Y", "received_private", "--move"}); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			destIDs, _ := folderindex.IDs(cachedir.At(accDir), destFolder)
			moved := len(destIDs) != 0
			if moved != tt.wantMoved {
				t.Errorf("destination folder index has %v, want moved=%v", destIDs, tt.wantMoved)
			}
			srcIDs, err := folderindex.IDs(cachedir.At(accDir), "received")
			if err != nil || len(srcIDs) != tt.wantSrcLen {
				t.Errorf("expected source folder index length %d, got %v (err %v)", tt.wantSrcLen, srcIDs, err)
			}
		})
	}
}
