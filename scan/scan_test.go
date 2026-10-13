package scan

import (
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"github.com/sarielhp/mail_cli/email"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sarielhp/mail_cli/cache/msg"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/mailclient"
	"github.com/sarielhp/mail_cli/report"
)

func TestPerformPatternMatching(t *testing.T) {
	tmpDir := t.TempDir()
	downloadDir := filepath.Join(tmpDir, "download")
	_ = os.MkdirAll(downloadDir, 0755)

	// Create fake emails under download/test-account
	accountDir := filepath.Join(downloadDir, "test-account")
	_ = os.MkdirAll(accountDir, 0755)

	// Store fake emails using msg.Store so the compact binary is updated
	err := msg.Store(cachedir.At(accountDir), "msg1", []byte("Date: Mon, 2 Jan 2026 15:04:05 -0700\nFrom: bob@example.com\nSubject: Urgent Invoice\n\nbody1"), time.Date(2026, 1, 2, 15, 4, 5, 0, time.UTC))
	if err != nil {
		t.Fatalf("failed to store msg1: %v", err)
	}
	err = msg.Store(cachedir.At(accountDir), "msg2", []byte("Date: Mon, 2 Jan 2026 15:04:05 -0700\nFrom: alice@example.com\nSubject: Meeting Update\n\nbody2"), time.Date(2026, 1, 2, 15, 4, 5, 0, time.UTC))
	if err != nil {
		t.Fatalf("failed to store msg2: %v", err)
	}

	cfg := &cfg_g.Config{
		SelectedAccount: "test-account",
		Paths:           cfg_g.Paths{CacheRoot: accountDir, ConfigDir: tmpDir},
		AccountCache:    cachedir.At(accountDir),
	}
	mockClient := &mailclient.MockMailClient{
		Labels:        []string{"received"},
		DownloadedIDs: map[string][]email.ID{"received": {"msg1", "msg2"}},
		Cfg:           cfg,
	}

	// Set classification so they are treated as cached (skips calling bogofilter executable)
	stamp := Stamp(mockClient.Cfg)
	err = msg.SetEvidence(cachedir.At(accountDir), "msg1", msg.Evidence{BogoScore: 0.0, Stamp: stamp})
	if err != nil {
		t.Fatalf("failed to set classification for msg1: %v", err)
	}
	err = msg.SetEvidence(cachedir.At(accountDir), "msg2", msg.Evidence{BogoScore: 0.0, Stamp: stamp})
	if err != nil {
		t.Fatalf("failed to set classification for msg2: %v", err)
	}

	// 1. With pattern "Invoice"
	_, _, err = Perform(io.Discard, Options{Pattern: "Invoice"}, report.Discard(), mockClient, mockClient.Cfg, "received")
	if err != nil {
		t.Fatalf("Perform failed: %v", err)
	}

	// 2. With pattern "Nonexistent" -> should not find any messages
	_, _, err = Perform(io.Discard, Options{Pattern: "Nonexistent"}, report.Discard(), mockClient, mockClient.Cfg, "received")
	if err != nil {
		t.Fatalf("Perform failed: %v", err)
	}
}

func TestCountUncachedEmails_SkipsClassified(t *testing.T) {
	tmpDir := t.TempDir()
	accountDir := filepath.Join(tmpDir, "test-account")
	_ = os.MkdirAll(accountDir, 0755)

	fixedDate := time.Date(2026, 1, 2, 15, 4, 5, 0, time.UTC)
	_ = msg.Store(cachedir.At(accountDir), "msg1", []byte("From: bob@example.com\nSubject: Item 1\n\nbody1"), fixedDate)
	_ = msg.Store(cachedir.At(accountDir), "msg2", []byte("From: alice@example.com\nSubject: Item 2\n\nbody2"), fixedDate)
	_ = msg.SetEvidence(cachedir.At(accountDir), "msg1", msg.Evidence{BogoScore: 0.0})

	count := countUncachedEmails([]email.ID{"msg1", "msg2"}, cachedir.At(accountDir), &cfg_g.Config{}, 0)
	if count != 1 {
		t.Fatalf("expected count 1 (only unclassified msg2), got %d", count)
	}

	// Remove msg1 from disk; since msg1 is classified, it must be skipped before msg.Read
	_ = os.Remove(filepath.Join(accountDir, "msg1.eml"))
	countAfterDelete := countUncachedEmails([]email.ID{"msg1", "msg2"}, cachedir.At(accountDir), &cfg_g.Config{}, 0)
	if countAfterDelete != 1 {
		t.Fatalf("expected count 1 even when msg1.eml is removed from disk, got %d", countAfterDelete)
	}
}
