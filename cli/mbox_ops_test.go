package cli

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sarielhp/mail_cli/cache/cachedir"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/mailclient"
)

func makeTestEmail(mid, subject, dateStr, from, body string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("From %s %s\n", from, dateStr))
	if mid != "" {
		sb.WriteString(fmt.Sprintf("Message-ID: %s\n", mid))
	}
	sb.WriteString(fmt.Sprintf("From: %s\n", from))
	sb.WriteString(fmt.Sprintf("Subject: %s\n", subject))
	if dateStr != "" {
		sb.WriteString(fmt.Sprintf("Date: %s\n", dateStr))
	}
	sb.WriteString("\n")
	sb.WriteString(body)
	sb.WriteString("\n")
	return sb.String()
}

func makeMultipartAttachmentEmail(mid, subject, from string, filename, content string) string {
	b64Data := base64.StdEncoding.EncodeToString([]byte(content))
	boundary := "===BOUNDARY_TEST==="
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("From %s Wed, 02 Sep 2026 10:00:00 +0000\n", from))
	if mid != "" {
		sb.WriteString(fmt.Sprintf("Message-ID: %s\n", mid))
	}
	sb.WriteString(fmt.Sprintf("From: %s\n", from))
	sb.WriteString(fmt.Sprintf("Subject: %s\n", subject))
	sb.WriteString("Date: Wed, 02 Sep 2026 10:00:00 +0000\n")
	sb.WriteString(fmt.Sprintf("Content-Type: multipart/mixed; boundary=\"%s\"\n\n", boundary))

	sb.WriteString(fmt.Sprintf("--%s\n", boundary))
	sb.WriteString("Content-Type: text/plain\n\n")
	sb.WriteString("Email message body text.\n")

	sb.WriteString(fmt.Sprintf("--%s\n", boundary))
	sb.WriteString(fmt.Sprintf("Content-Type: application/octet-stream; name=\"%s\"\n", filename))
	sb.WriteString(fmt.Sprintf("Content-Disposition: attachment; filename=\"%s\"\n", filename))
	sb.WriteString("Content-Transfer-Encoding: base64\n\n")
	sb.WriteString(b64Data)
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("--%s--\n", boundary))
	return sb.String()
}

func createMboxFile(t *testing.T, path string, emails []string) {
	t.Helper()
	var content bytes.Buffer
	for _, em := range emails {
		content.WriteString(em)
	}
	if err := os.WriteFile(path, content.Bytes(), 0600); err != nil {
		t.Fatalf("failed to write mbox file %s: %v", path, err)
	}
}

func TestFormatWithCommas(t *testing.T) {
	tests := []struct {
		input int64
		want  string
	}{
		{0, "0"},
		{9, "9"},
		{99, "99"},
		{999, "999"},
		{1000, "1,000"},
		{12345, "12,345"},
		{123456, "123,456"},
		{1234567, "1,234,567"},
		{-1234567, "-1,234,567"},
		{1000000000, "1,000,000,000"},
	}
	for _, tt := range tests {
		got := FormatWithCommas(tt.input)
		if got != tt.want {
			t.Errorf("FormatWithCommas(%d) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestFormatHumanSize(t *testing.T) {
	tests := []struct {
		input int64
		want  string
	}{
		{0, "0 B"},
		{500, "500 B"},
		{1023, "1023 B"},
		{1024, "1.00 KB"},
		{1536, "1.50 KB"},
		{1234567, "1.18 MB"},
		{1048576, "1.00 MB"},
		{1073741824, "1.00 GB"},
		{1099511627776, "1.00 TB"},
		{-1024, "-1.00 KB"},
	}
	for _, tt := range tests {
		got := FormatHumanSize(tt.input)
		if got != tt.want {
			t.Errorf("FormatHumanSize(%d) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestIsMboxEmailMatch(t *testing.T) {
	t1 := time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 9, 2, 11, 0, 0, 0, time.UTC)

	// Authoritative: identical Message-IDs match
	e1 := &email.Email{MessageID: "<m1@test.com>", Subject: "Sub A", EmailDate: t1}
	e2 := &email.Email{MessageID: "<m1@test.com>", Subject: "Sub B", EmailDate: t2}
	if !isMboxEmailMatch(e1, e2) {
		t.Errorf("expected matching Message-IDs to match")
	}

	// Authoritative: different Message-IDs must NEVER match
	e3 := &email.Email{MessageID: "<m2@test.com>", Subject: "Sub A", EmailDate: t1}
	if isMboxEmailMatch(e1, e3) {
		t.Errorf("expected differing Message-IDs to NOT match")
	}

	// Fallback: one lacks Message-ID, Subject + non-zero Date match
	e4 := &email.Email{Subject: "Sub A", EmailDate: t1}
	if !isMboxEmailMatch(e1, e4) {
		t.Errorf("expected fallback match when one Message-ID is missing")
	}

	// Fallback: zero dates must NOT match
	e5 := &email.Email{Subject: "Sub A"}
	e6 := &email.Email{Subject: "Sub A"}
	if isMboxEmailMatch(e5, e6) {
		t.Errorf("expected zero-date emails to NOT match under fallback")
	}

	// Nil safety
	if isMboxEmailMatch(nil, e1) || isMboxEmailMatch(e1, nil) {
		t.Errorf("expected nil comparison to be false")
	}
}

func TestFilterNewMessages_Deduplication(t *testing.T) {
	t1 := time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 9, 2, 11, 0, 0, 0, time.UTC)

	dest := []mboxEntry{
		{email: &email.Email{MessageID: "<existing1@test.com>", Subject: "Sub A", EmailDate: t1}},
		{email: &email.Email{Subject: "Fallback Unique", EmailDate: t1}},
	}

	src := []mboxEntry{
		// Duplicate by Message-ID (different case)
		{email: &email.Email{MessageID: "<EXISTING1@test.com>", Subject: "Different Sub", EmailDate: t2}},
		// Different Message-ID but same subject/date as existing1 -> should NOT match (must be kept)
		{email: &email.Email{MessageID: "<new1@test.com>", Subject: "Sub A", EmailDate: t1}},
		// Match fallback by subject + date
		{email: &email.Email{Subject: "Fallback Unique", EmailDate: t1}},
		// Novel message
		{email: &email.Email{MessageID: "<new2@test.com>", Subject: "Brand New", EmailDate: t2}},
		// Duplicate of novel message within srcMsgs
		{email: &email.Email{MessageID: "<new2@test.com>", Subject: "Brand New Duplicate", EmailDate: t2}},
	}

	filtered := filterNewMessages(src, dest)
	if len(filtered) != 2 {
		t.Fatalf("expected 2 new messages, got %d", len(filtered))
	}
	if filtered[0].email.MessageID != "<new1@test.com>" {
		t.Errorf("expected first new msg to be new1, got %s", filtered[0].email.MessageID)
	}
	if filtered[1].email.MessageID != "<new2@test.com>" {
		t.Errorf("expected second new msg to be new2, got %s", filtered[1].email.MessageID)
	}
}

func TestMboxCheck_SingleFileValid(t *testing.T) {
	tempDir := t.TempDir()
	mboxPath := filepath.Join(tempDir, "valid.mbox")

	em1 := makeTestEmail("<m1@test.com>", "Hello 1", "Wed, 02 Sep 2026 10:00:00 +0000", "u1@test.com", "Body 1")
	em2 := makeTestEmail("<m2@test.com>", "Hello 2", "Wed, 02 Sep 2026 10:00:00 +0000", "u2@test.com", "Body 2")
	createMboxFile(t, mboxPath, []string{em1, em2})

	var buf bytes.Buffer
	err := runMboxCheckTo(&buf, mboxPath)
	if err != nil {
		t.Fatalf("expected valid mbox check to return nil, got: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "[VALID]") || !strings.Contains(out, "2 message(s)") {
		t.Errorf("unexpected output: %s", out)
	}
}

func TestMboxCheck_SingleFileInvalid(t *testing.T) {
	tempDir := t.TempDir()

	// 1. Empty file
	emptyPath := filepath.Join(tempDir, "empty.mbox")
	_ = os.WriteFile(emptyPath, []byte(""), 0600)
	var buf1 bytes.Buffer
	if err := runMboxCheckTo(&buf1, emptyPath); err == nil {
		t.Errorf("expected error for empty file, got nil")
	}

	// 2. Missing "From " delimiter at start
	notMboxPath := filepath.Join(tempDir, "not_mbox.txt")
	_ = os.WriteFile(notMboxPath, []byte("Subject: Hello\n\nBody without From\n"), 0600)
	var buf2 bytes.Buffer
	if err := runMboxCheckTo(&buf2, notMboxPath); err == nil {
		t.Errorf("expected error for non-mbox file, got nil")
	}

	// 3. Corrupted header without colon
	corruptHeaderPath := filepath.Join(tempDir, "corrupt_hdr.mbox")
	corruptContent := "From u@test.com Wed Sep  2 10:00:00 2026\nCorruptedHeaderWithoutColon\n\nBody"
	_ = os.WriteFile(corruptHeaderPath, []byte(corruptContent), 0600)
	var buf3 bytes.Buffer
	if err := runMboxCheckTo(&buf3, corruptHeaderPath); err == nil {
		t.Errorf("expected error for corrupted header, got nil")
	}

	// 4. Invalid Date header
	badDatePath := filepath.Join(tempDir, "bad_date.mbox")
	badDateContent := "From u@test.com Wed Sep  2 10:00:00 2026\nSubject: Test\nDate: Not A Date\n\nBody\n"
	_ = os.WriteFile(badDatePath, []byte(badDateContent), 0600)
	var buf4 bytes.Buffer
	if err := runMboxCheckTo(&buf4, badDatePath); err == nil {
		t.Errorf("expected error for bad Date header, got nil")
	}

	// 5. Invalid From header
	badFromPath := filepath.Join(tempDir, "bad_from.mbox")
	badFromContent := "From u@test.com Wed Sep  2 10:00:00 2026\nSubject: Test\nFrom: <unclosed_address\n\nBody\n"
	_ = os.WriteFile(badFromPath, []byte(badFromContent), 0600)
	var buf5 bytes.Buffer
	if err := runMboxCheckTo(&buf5, badFromPath); err == nil {
		t.Errorf("expected error for bad From header, got nil")
	}
}

func TestMboxCheck_Directory(t *testing.T) {
	tempDir := t.TempDir()

	em1 := makeTestEmail("<m1@test.com>", "Hello 1", "Wed, 02 Sep 2026 10:00:00 +0000", "u1@test.com", "Body 1")
	em2 := makeTestEmail("<m2@test.com>", "Hello 2", "Wed, 02 Sep 2026 10:00:00 +0000", "u2@test.com", "Body 2")

	validDir := filepath.Join(tempDir, "all_valid")
	_ = os.MkdirAll(validDir, 0700)
	createMboxFile(t, filepath.Join(validDir, "f1.mbox"), []string{em1})
	createMboxFile(t, filepath.Join(validDir, "f2.mbox"), []string{em2})

	var buf1 bytes.Buffer
	if err := runMboxCheckTo(&buf1, validDir); err != nil {
		t.Fatalf("expected all valid directory check to succeed, got: %v", err)
	}

	// Mixed directory with an invalid file
	mixedDir := filepath.Join(tempDir, "mixed")
	_ = os.MkdirAll(mixedDir, 0700)
	createMboxFile(t, filepath.Join(mixedDir, "good.mbox"), []string{em1})
	_ = os.WriteFile(filepath.Join(mixedDir, "bad.mbox"), []byte("not an mbox"), 0600)

	var buf2 bytes.Buffer
	err := runMboxCheckTo(&buf2, mixedDir)
	if err == nil {
		t.Fatalf("expected mixed directory check to return error, got nil")
	}
	out2 := buf2.String()
	if !strings.Contains(out2, "[VALID]") || !strings.Contains(out2, "[INVALID]") {
		t.Errorf("expected output to contain both [VALID] and [INVALID], got:\n%s", out2)
	}
}

func TestMboxMerge_Deduplication(t *testing.T) {
	tempDir := t.TempDir()
	xPath := filepath.Join(tempDir, "x.mbox")
	yPath := filepath.Join(tempDir, "y.mbox")

	emA := makeTestEmail("<msg-a@test.com>", "Email A", "Wed, 02 Sep 2026 10:00:00 +0000", "u@test.com", "Body A")
	emB := makeTestEmail("<msg-b@test.com>", "Email B", "Wed, 02 Sep 2026 10:00:00 +0000", "u@test.com", "Body B")
	emC := makeTestEmail("<msg-c@test.com>", "Email C", "Wed, 02 Sep 2026 10:00:00 +0000", "u@test.com", "Body C")

	// X has A, B, C; Y has A
	createMboxFile(t, xPath, []string{emA, emB, emC})
	createMboxFile(t, yPath, []string{emA})

	var buf bytes.Buffer
	err := runMboxMergeTo(&buf, xPath, yPath)
	if err != nil {
		t.Fatalf("runMboxMerge failed: %v", err)
	}

	count, err := ValidateMboxFile(yPath)
	if err != nil {
		t.Fatalf("merged y.mbox validation failed: %v", err)
	}
	if count != 3 {
		t.Fatalf("expected y.mbox to have 3 messages after merge, got %d", count)
	}

	// Merging again should detect 0 new messages
	var buf2 bytes.Buffer
	err = runMboxMergeTo(&buf2, xPath, yPath)
	if err != nil {
		t.Fatalf("second runMboxMerge failed: %v", err)
	}
	if !strings.Contains(buf2.String(), "No new messages to merge") {
		t.Errorf("expected 'No new messages to merge' message, got: %s", buf2.String())
	}
}

// mergeAndCountMessages merges the y mbox into x and returns how many messages
// y holds afterwards, failing the test if the merge or the validation errors.
func mergeAndCountMessages(t *testing.T, xEmail, yEmail string) int {
	t.Helper()
	tempDir := t.TempDir()
	xPath := filepath.Join(tempDir, "x.mbox")
	yPath := filepath.Join(tempDir, "y.mbox")
	createMboxFile(t, xPath, []string{xEmail})
	createMboxFile(t, yPath, []string{yEmail})

	var buf bytes.Buffer
	if err := runMboxMergeTo(&buf, xPath, yPath); err != nil {
		t.Fatalf("merge failed: %v", err)
	}
	count, err := ValidateMboxFile(yPath)
	if err != nil {
		t.Fatalf("ValidateMboxFile after merge: %v", err)
	}
	return count
}

func TestMboxMerge_FallbackMatching(t *testing.T) {
	// Message without Message-ID, identical Subject + non-zero Date
	emNoIDA := makeTestEmail("", "Monthly Statement", "Wed, 02 Sep 2026 10:00:00 +0000", "bank@test.com", "Statement A")
	emNoIDB := makeTestEmail("", "Monthly Statement", "Wed, 02 Sep 2026 10:00:00 +0000", "bank@test.com", "Statement B")

	if got := mergeAndCountMessages(t, emNoIDA, emNoIDB); got != 1 {
		t.Fatalf("expected 1 message in y (fallback match deduplicated), got %d", got)
	}
}

func TestMboxMerge_MessageIDAuthoritativeMismatch(t *testing.T) {
	// Same Subject & Date, but DIFFERENT Message-IDs -> MUST NOT match!
	emX := makeTestEmail("<src-unique@test.com>", "Duplicate Subject", "Wed, 02 Sep 2026 10:00:00 +0000", "u@test.com", "Body X")
	emY := makeTestEmail("<dest-unique@test.com>", "Duplicate Subject", "Wed, 02 Sep 2026 10:00:00 +0000", "u@test.com", "Body Y")

	if got := mergeAndCountMessages(t, emX, emY); got != 2 {
		t.Fatalf("expected 2 messages in y (different Message-IDs do not match), got %d", got)
	}
}

func TestMboxMerge_PreValidation(t *testing.T) {
	tempDir := t.TempDir()
	validPath := filepath.Join(tempDir, "valid.mbox")
	invalidPath := filepath.Join(tempDir, "invalid.mbox")

	em := makeTestEmail("<m@test.com>", "Sub", "Wed, 02 Sep 2026 10:00:00 +0000", "u@test.com", "Body")
	createMboxFile(t, validPath, []string{em})
	_ = os.WriteFile(invalidPath, []byte("broken content"), 0600)

	// Merge invalid into valid -> aborted
	var buf1 bytes.Buffer
	if err := runMboxMergeTo(&buf1, invalidPath, validPath); err == nil {
		t.Errorf("expected error when source is invalid, got nil")
	}

	// Merge valid into invalid -> aborted
	var buf2 bytes.Buffer
	if err := runMboxMergeTo(&buf2, validPath, invalidPath); err == nil {
		t.Errorf("expected error when destination is invalid, got nil")
	}

	// Same file merge -> error
	var buf3 bytes.Buffer
	if err := runMboxMergeTo(&buf3, validPath, validPath); err == nil {
		t.Errorf("expected error when merging file into itself, got nil")
	}
}

func TestMboxMerge_AtomicSafety(t *testing.T) {
	tempDir := t.TempDir()
	xPath := filepath.Join(tempDir, "x_atomic.mbox")
	yPath := filepath.Join(tempDir, "y_atomic.mbox")

	emX := makeTestEmail("<m-x@test.com>", "Sub X", "Wed, 02 Sep 2026 10:00:00 +0000", "u@test.com", "Body X")
	emY := makeTestEmail("<m-y@test.com>", "Sub Y", "Wed, 02 Sep 2026 10:00:00 +0000", "u@test.com", "Body Y")

	createMboxFile(t, xPath, []string{emX})
	createMboxFile(t, yPath, []string{emY})

	var buf bytes.Buffer
	err := runMboxMergeTo(&buf, xPath, yPath)
	if err != nil {
		t.Fatalf("merge failed: %v", err)
	}

	// Ensure temp file is cleaned up after successful merge
	tmpPath := yPath + ".tmp"
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Errorf("expected temp file %s to be removed, but it exists", tmpPath)
	}
}

func TestMboxInfo_CountsAndSizes(t *testing.T) {
	tempDir := t.TempDir()
	mboxPath := filepath.Join(tempDir, "info_test.mbox")

	emPlain := makeTestEmail("<m1@test.com>", "Plain Email", "Wed, 02 Sep 2026 10:00:00 +0000", "u1@test.com", "Plain Body")
	att1Content := "Attachment 1 sample content that has some length"
	emAtt1 := makeMultipartAttachmentEmail("<m2@test.com>", "Att 1", "u2@test.com", "doc1.txt", att1Content)
	att2Content := "Attachment 2 another file with data"
	emAtt2 := makeMultipartAttachmentEmail("<m3@test.com>", "Att 2", "u3@test.com", "doc2.pdf", att2Content)

	createMboxFile(t, mboxPath, []string{emPlain, emAtt1, emAtt2})

	var buf bytes.Buffer
	err := runMboxInfoTo(&buf, mboxPath)
	if err != nil {
		t.Fatalf("runMboxInfo failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Messages:         3") {
		t.Errorf("expected 3 messages in info output, got:\n%s", out)
	}
	if !strings.Contains(out, "Attachment Count: 2") {
		t.Errorf("expected 2 attachments in info output, got:\n%s", out)
	}

	expectedAttSize := int64(len(att1Content) + len(att2Content))
	expectedAttStr := fmt.Sprintf("%s bytes (%s)", FormatWithCommas(expectedAttSize), FormatHumanSize(expectedAttSize))
	if !strings.Contains(out, expectedAttStr) {
		t.Errorf("expected attachment size %q in output, got:\n%s", expectedAttStr, out)
	}
}

func TestMboxInfo_Errors(t *testing.T) {
	tempDir := t.TempDir()

	// Non-existent file
	var buf1 bytes.Buffer
	if err := runMboxInfoTo(&buf1, filepath.Join(tempDir, "nonexistent.mbox")); err == nil {
		t.Errorf("expected error for non-existent file, got nil")
	}

	// Directory instead of file
	var buf2 bytes.Buffer
	if err := runMboxInfoTo(&buf2, tempDir); err == nil {
		t.Errorf("expected error when file is directory, got nil")
	}

	// Invalid mbox file
	badFile := filepath.Join(tempDir, "invalid.mbox")
	_ = os.WriteFile(badFile, []byte("not valid"), 0600)
	var buf3 bytes.Buffer
	if err := runMboxInfoTo(&buf3, badFile); err == nil {
		t.Errorf("expected error for invalid mbox file, got nil")
	}
}

func TestMboxCLI_Commands(t *testing.T) {
	tempDir := t.TempDir()
	session := &Session{Config: &cfg_g.Config{}}
	cliApp := InitCLI(session)

	mbox1 := filepath.Join(tempDir, "cli1.mbox")
	mbox2 := filepath.Join(tempDir, "cli2.mbox")
	em1 := makeTestEmail("<c1@test.com>", "CLI 1", "Wed, 02 Sep 2026 10:00:00 +0000", "u@test.com", "Body 1")
	em2 := makeTestEmail("<c2@test.com>", "CLI 2", "Wed, 02 Sep 2026 10:00:00 +0000", "u@test.com", "Body 2")
	createMboxFile(t, mbox1, []string{em1})
	createMboxFile(t, mbox2, []string{em2})

	// 1. check
	if err := cliApp.Execute([]string{"mbox", "check", mbox1}); err != nil {
		t.Errorf("mbox check via CLI failed: %v", err)
	}

	// 2. info
	if err := cliApp.Execute([]string{"mbox", "info", mbox1}); err != nil {
		t.Errorf("mbox info via CLI failed: %v", err)
	}

	// 3. merge
	if err := cliApp.Execute([]string{"mbox", "merge", mbox2, mbox1}); err != nil {
		t.Errorf("mbox merge via CLI failed: %v", err)
	}

	// 4. view (nonexistent)
	if err := cliApp.Execute([]string{"mbox", "view", filepath.Join(tempDir, "nonexistent.mbox")}); err == nil {
		t.Errorf("expected error viewing nonexistent mbox file")
	}

	// 5. view (success)
	var openedFolder string
	var clientBackend string
	var clientIsReadOnly bool
	var mboxAccountReadOnly bool
	session.Deps.Accounts = stubAccounts{
		OpenMboxFn: func(cfg *cfg_g.Config, _ string, _ cachedir.Account) (mailclient.MailClient, error) {
			// runMboxView is responsible for the synthetic account: read-only,
			// its own cache directory. Assembling the client stack over it is
			// the factory's job and is covered in mailops.
			mboxAccountReadOnly = len(cfg.Accounts) == 1 && cfg.Accounts[0].ReadOnly
			return &mailclient.MockMailClient{Cfg: cfg, BackendTypeVal: "mbox"}, nil
		},
	}
	session.Deps.TUI = stubTUI{
		RunWithClientFn: func(cfg *cfg_g.Config, client mailclient.MailClient, labelPrefix string) error {
			openedFolder = labelPrefix
			clientBackend = client.BackendType()
			clientIsReadOnly = cfg.ReadOnly
			return nil
		},
	}
	if err := cliApp.Execute([]string{"mbox", "view", mbox1}); err != nil {
		t.Errorf("mbox view via CLI failed: %v", err)
	}
	if openedFolder != "INBOX" {
		t.Errorf("expected opened folder INBOX, got %q", openedFolder)
	}
	if clientBackend != "mbox" {
		t.Errorf("expected client backend mbox, got %q", clientBackend)
	}
	if !clientIsReadOnly {
		t.Errorf("expected clientIsReadOnly=true")
	}
	if !mboxAccountReadOnly {
		t.Errorf("expected the synthetic mbox account to be marked read-only")
	}
}
