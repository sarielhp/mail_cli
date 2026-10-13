package scan

import (
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"github.com/sarielhp/mail_cli/email"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sarielhp/mail_cli/cache/msg"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/report"
)

// TestScanEmails_DuplicateIDsAreProcessedOnce is a concurrency-race
// regression: Emails dispatches one goroutine per entry in its ids
// slice, without deduplicating it first. A caller that merges message IDs
// from more than one source -- a virtual/search result set that lists a
// message alongside its folder listing, or a backend that returns the same
// ID twice across pages -- can hand Emails the same ID more than once.
// Two workers then run scanSingleMessageWorker concurrently for that one
// message: the "already classified" check (msg.GetInfo) and the eventual
// classification write (msg.SetVerdict) are not atomic with each
// other, so both workers pass the check, both redo the classification work,
// and both append their result -- here, both append the same ID to
// blacklistedIDs, so a message a caller expects to move once would be acted
// on twice.
//
// The blacklisted-sender path is used because it is deterministic and
// bypasses the bogofilter subprocess entirely, so the test needs no external
// binary and is not a timing-dependent race: with the ids slice containing
// the same ID twice, both dispatched workers take this same branch every
// run.
func TestScanEmails_DuplicateIDsAreProcessedOnce(t *testing.T) {
	tmpDir := t.TempDir()
	accountDir := filepath.Join(tmpDir, "test-account")
	if err := os.MkdirAll(accountDir, 0755); err != nil {
		t.Fatalf("failed to create account dir: %v", err)
	}

	raw := []byte("Date: Mon, 2 Jan 2026 15:04:05 -0700\nFrom: evil@example.com\nSubject: Buy now\n\nbody")
	if err := msg.Store(cachedir.At(accountDir), "msg1", raw, time.Date(2026, 1, 2, 15, 4, 5, 0, time.UTC)); err != nil {
		t.Fatalf("failed to store msg1: %v", err)
	}

	config := &cfg_g.Config{
		SelectedAccount: "test-account",
		Paths:           cfg_g.Paths{CacheRoot: tmpDir, ConfigDir: tmpDir},
		AccountCache:    cachedir.At(accountDir),
		Policy:          cfg_g.Policy{Blacklist: []string{"evil@example.com"}},
	}

	_, blacklistedIDs, scanResults, err := Emails(report.Discard(), []email.ID{"msg1", "msg1"}, config, "")
	if err != nil {
		t.Fatalf("Emails failed: %v", err)
	}

	if len(blacklistedIDs) != 1 {
		t.Errorf("expected msg1 to be recorded as blacklisted once, got %d entries: %v", len(blacklistedIDs), blacklistedIDs)
	}
	if len(scanResults) != 1 {
		t.Errorf("expected one scan result, got %d", len(scanResults))
	}
}
