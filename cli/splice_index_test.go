package cli

import (
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"github.com/sarielhp/mail_cli/label"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sarielhp/mail_cli/cache/msg"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/mailclient"
)

func TestIsMessageInDest_IndexMatching(t *testing.T) {
	tempDir := t.TempDir()
	fixedDate := time.Date(2026, 9, 18, 10, 0, 0, 0, time.UTC)

	eml1 := "Message-ID: <dest1@example.com>\nDate: Fri, 18 Sep 2026 10:00:00 +0000\nSubject: Project Alpha\n\nbody"
	eml2 := "Date: Fri, 18 Sep 2026 10:00:00 +0000\nSubject: Fallback Topic\n\nbody"

	_ = msg.Store(cachedir.At(tempDir), "dest_001", []byte(eml1), fixedDate)
	_ = msg.Store(cachedir.At(tempDir), "dest_002", []byte(eml2), fixedDate)

	mockClient := &mailclient.MockMailClient{
		Labels:        []string{"Archive"},
		DownloadedIDs: map[string][]email.ID{"Archive": {"dest_001", "dest_002"}},
		Cfg:           &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: tempDir}, AccountCache: cachedir.At(tempDir)},
	}

	destCache := make(map[label.Folder]*destFolderIndex)

	// 1. Authoritative match by Message-ID
	srcMatchID := &email.Email{
		MessageID: "<dest1@example.com>",
		Subject:   "Completely Different Subject",
		EmailDate: fixedDate,
	}
	if !isMessageInDest(mockClient, label.FromServer("Archive"), "src_001", srcMatchID, destCache) {
		t.Error("expected match by authoritative Message-ID")
	}

	// 2. Authoritative mismatch: both have Message-ID, but differ (even if Subject and Date match!)
	srcMismatchID := &email.Email{
		MessageID: "<different@example.com>",
		Subject:   "Project Alpha",
		EmailDate: fixedDate,
	}
	if isMessageInDest(mockClient, label.FromServer("Archive"), "src_002", srcMismatchID, destCache) {
		t.Error("expected mismatch when both possess differing Message-IDs despite same Subject and Date")
	}

	// 3. Fallback heuristic match when dest lacks Message-ID
	srcFallbackMatch := &email.Email{
		MessageID: "<src_has_id@example.com>",
		Subject:   "Fallback Topic",
		EmailDate: fixedDate,
	}
	if !isMessageInDest(mockClient, label.FromServer("Archive"), "src_003", srcFallbackMatch, destCache) {
		t.Error("expected fallback match when dest lacks Message-ID and Subject+Date match")
	}

	// 4. Mismatch when Subject and Date don't match any dest email
	srcNoMatch := &email.Email{
		Subject:   "Unrelated Topic",
		EmailDate: fixedDate,
	}
	if isMessageInDest(mockClient, label.FromServer("Archive"), "src_004", srcNoMatch, destCache) {
		t.Error("expected no match for unrelated message")
	}

	// 5. Index caching verification: remove files from disk to prove lookups use in-memory index
	_ = os.Remove(filepath.Join(tempDir, "dest_001.eml"))
	_ = os.Remove(filepath.Join(tempDir, "dest_002.eml"))
	if !isMessageInDest(mockClient, label.FromServer("Archive"), "src_001", srcMatchID, destCache) {
		t.Error("expected match from cached index even after deleting disk files")
	}
}
