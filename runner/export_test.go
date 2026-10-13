package runner

import (
	"bytes"
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarielhp/mail_cli/backend/mbox"
	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/cli"
	"github.com/sarielhp/mail_cli/report"
)

func TestRunExport_SingleMboxStdout(t *testing.T) {
	tempDir := t.TempDir()
	rawMsg := "From: Alice <alice@example.com>\r\n" +
		"To: Bob <bob@example.com>\r\n" +
		"Subject: Single Test\r\n" +
		"Message-ID: <single1@example.com>\r\n" +
		"Date: Sun, 20 Sep 2026 12:00:00 +0000\r\n" +
		"Content-Type: text/plain\r\n" +
		"\r\n" +
		"This is a single message.\r\n"

	setupTestCachedMessage(t, tempDir, "work", "single1", []byte(rawMsg))

	cfg := &cfg_g.Config{
		Paths: cfg_g.Paths{
			ConfigDir: tempDir,
			CacheRoot: tempDir,
		},
		Accounts: []cfg_acc.AccountConfig{
			{Name: "work", Username: "bob@example.com", Type: "jmap"},
		},
	}

	var buf bytes.Buffer
	opts := cli.ExportOptions{
		MessageID: "single1",
		Format:    "mbox",
		Single:    true,
	}

	if err := RunExport(&buf, cfg, opts, report.Discard()); err != nil {
		t.Fatalf("RunExport failed: %v", err)
	}

	out := buf.String()
	if !strings.HasPrefix(out, "From alice@example.com ") {
		t.Errorf("expected mbox From delimiter, got:\n%s", out)
	}
	if !strings.Contains(out, "Subject: Single Test") {
		t.Errorf("expected Subject header, got:\n%s", out)
	}
	if !strings.Contains(out, "This is a single message.") {
		t.Errorf("expected body, got:\n%s", out)
	}
}

func TestRunExport_ThreadMboxRoundTrip(t *testing.T) {
	tempDir := t.TempDir()

	msg1 := "From: Alice <alice@example.com>\r\n" +
		"To: Bob <bob@example.com>\r\n" +
		"Subject: Discussion Root\r\n" +
		"Message-ID: <root1@example.com>\r\n" +
		"Date: Sun, 20 Sep 2026 10:00:00 +0000\r\n" +
		"Content-Type: text/plain\r\n" +
		"\r\n" +
		"Starting a discussion.\r\n"

	msg2 := "From: Bob <bob@example.com>\r\n" +
		"To: Alice <alice@example.com>\r\n" +
		"Subject: Re: Discussion Root\r\n" +
		"Message-ID: <reply1@example.com>\r\n" +
		"In-Reply-To: <root1@example.com>\r\n" +
		"References: <root1@example.com>\r\n" +
		"Date: Sun, 20 Sep 2026 11:00:00 +0000\r\n" +
		"Content-Type: text/plain\r\n" +
		"\r\n" +
		"I agree with the points.\r\n"

	msg3 := "From: Charlie <charlie@example.com>\r\n" +
		"To: Bob <bob@example.com>, Alice <alice@example.com>\r\n" +
		"Subject: Re: Discussion Root\r\n" +
		"Message-ID: <reply2@example.com>\r\n" +
		"In-Reply-To: <reply1@example.com>\r\n" +
		"References: <root1@example.com> <reply1@example.com>\r\n" +
		"Date: Sun, 20 Sep 2026 12:00:00 +0000\r\n" +
		"Content-Type: text/plain\r\n" +
		"\r\n" +
		"Here is additional feedback.\r\n"

	setupTestCachedMessage(t, tempDir, "work", "m1", []byte(msg1))
	setupTestCachedMessage(t, tempDir, "work", "m2", []byte(msg2))
	setupTestCachedMessage(t, tempDir, "work", "m3", []byte(msg3))

	cfg := &cfg_g.Config{
		Paths: cfg_g.Paths{
			ConfigDir: tempDir,
			CacheRoot: tempDir,
		},
		Accounts: []cfg_acc.AccountConfig{
			{Name: "work", Username: "bob@example.com", Type: "jmap"},
		},
	}

	exportPath := filepath.Join(tempDir, "thread.mbox")
	opts := cli.ExportOptions{
		MessageID: "m2", // target reply message, export should find full thread
		Format:    "mbox",
		Output:    exportPath,
	}

	if err := RunExport(nil, cfg, opts, report.Discard()); err != nil {
		t.Fatalf("RunExport failed: %v", err)
	}

	// Verify file exists
	fi, err := os.Stat(exportPath)
	if err != nil || fi.Size() == 0 {
		t.Fatalf("export file missing or empty: %v", err)
	}

	// Test Round-Trip fidelity with backend/mbox.IngestMbox
	ingestDir := filepath.Join(tempDir, "ingest_cache")
	ingestedIDs, err := mbox.IngestMbox(exportPath, cachedir.At(ingestDir), "IngestTest")
	if err != nil {
		t.Fatalf("mbox.IngestMbox failed to parse exported mbox: %v", err)
	}
	if len(ingestedIDs) != 3 {
		t.Errorf("expected 3 ingested messages from exported mbox, got %d", len(ingestedIDs))
	}
}

func TestRunExport_ThreadEML(t *testing.T) {
	tempDir := t.TempDir()

	msg1 := "From: Alice <alice@example.com>\r\n" +
		"Subject: Feature Idea\r\n" +
		"Message-ID: <feat1@example.com>\r\n" +
		"Date: Sun, 20 Sep 2026 10:00:00 +0000\r\n" +
		"Content-Type: text/plain\r\n" +
		"\r\n" +
		"Body 1\r\n"

	msg2 := "From: Bob <bob@example.com>\r\n" +
		"Subject: Re: Feature Idea\r\n" +
		"Message-ID: <feat2@example.com>\r\n" +
		"In-Reply-To: <feat1@example.com>\r\n" +
		"Date: Sun, 20 Sep 2026 11:00:00 +0000\r\n" +
		"Content-Type: text/plain\r\n" +
		"\r\n" +
		"Body 2\r\n"

	setupTestCachedMessage(t, tempDir, "work", "f1", []byte(msg1))
	setupTestCachedMessage(t, tempDir, "work", "f2", []byte(msg2))

	cfg := &cfg_g.Config{
		Paths: cfg_g.Paths{
			ConfigDir: tempDir,
			CacheRoot: tempDir,
		},
		Accounts: []cfg_acc.AccountConfig{
			{Name: "work", Username: "bob@example.com", Type: "jmap"},
		},
	}

	emlDir := filepath.Join(tempDir, "eml_export")
	opts := cli.ExportOptions{
		MessageID: "f1",
		Format:    "eml",
		Output:    emlDir,
	}

	if err := RunExport(nil, cfg, opts, report.Discard()); err != nil {
		t.Fatalf("RunExport failed: %v", err)
	}

	entries, err := os.ReadDir(emlDir)
	if err != nil {
		t.Fatalf("reading emlDir failed: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 .eml files, got %d", len(entries))
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".eml") {
			t.Errorf("unexpected file in emlDir: %s", e.Name())
		}
	}
}

func TestRunExport_DryRun(t *testing.T) {
	tempDir := t.TempDir()
	rawMsg := "From: Alice <alice@example.com>\r\n" +
		"Subject: Dry Run Topic\r\n" +
		"Message-ID: <dry1@example.com>\r\n" +
		"Date: Sun, 20 Sep 2026 12:00:00 +0000\r\n" +
		"Content-Type: text/plain\r\n" +
		"\r\n" +
		"Dry run content\r\n"

	setupTestCachedMessage(t, tempDir, "work", "dry1", []byte(rawMsg))

	cfg := &cfg_g.Config{
		Paths: cfg_g.Paths{
			ConfigDir: tempDir,
			CacheRoot: tempDir,
		},
		Accounts: []cfg_acc.AccountConfig{
			{Name: "work", Username: "bob@example.com", Type: "jmap"},
		},
	}

	var buf bytes.Buffer
	outPath := filepath.Join(tempDir, "should_not_exist.mbox")
	opts := cli.ExportOptions{
		MessageID: "dry1",
		Format:    "mbox",
		Output:    outPath,
		DryRun:    true,
	}

	if err := RunExport(&buf, cfg, opts, report.Discard()); err != nil {
		t.Fatalf("RunExport dry-run failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "[DRY-RUN] Would export 1 message(s) in mbox format:") {
		t.Errorf("expected dry-run header, got:\n%s", out)
	}
	if !strings.Contains(out, "Dry Run Topic") {
		t.Errorf("expected subject in dry-run output, got:\n%s", out)
	}

	if _, err := os.Stat(outPath); !os.IsNotExist(err) {
		t.Errorf("expected output file to not be created in dry-run mode")
	}
}

func TestRunExport_MboxRDEscaping(t *testing.T) {
	raw := []byte("Header: value\n\nFrom someone\n>From someone\nNormal line\n")
	var buf bytes.Buffer
	if err := writeMboxRDEscaped(&buf, raw); err != nil {
		t.Fatalf("writeMboxRDEscaped failed: %v", err)
	}

	escaped := buf.String()
	if !strings.Contains(escaped, "\n>From someone\n") {
		t.Errorf("expected 'From someone' to be escaped as '>From someone', got:\n%s", escaped)
	}
	if !strings.Contains(escaped, "\n>>From someone\n") {
		t.Errorf("expected '>From someone' to be escaped as '>>From someone', got:\n%s", escaped)
	}
	if !strings.Contains(escaped, "\nNormal line\n") {
		t.Errorf("expected 'Normal line' to remain unchanged, got:\n%s", escaped)
	}
}
