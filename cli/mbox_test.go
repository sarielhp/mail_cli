package cli

import (
	"bufio"
	"bytes"
	"errors"
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sarielhp/mail_cli/cache/msg"
	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/mailclient"
)

// writeMboxFromAddr writes one fixed message to a buffer through writeToMbox,
// reporting the sender line writeToMbox derived from fromAddr.
func writeMboxFromAddr(t *testing.T, fromAddr string) string {
	t.Helper()
	var buf bytes.Buffer
	date := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	rawEmail := []byte("Subject: Test\r\n\r\nBody")

	if err := writeToMbox(&buf, rawEmail, fromAddr, date); err != nil {
		t.Fatalf("writeToMbox failed: %v", err)
	}
	return buf.String()
}

func TestWriteToMbox_ValidAddress(t *testing.T) {
	output := writeMboxFromAddr(t, "test@example.com")
	if !strings.HasPrefix(output, "From test@example.com") {
		t.Errorf("Expected 'From test@example.com', got: %s", output[:50])
	}
}

func TestWriteToMbox_EmptyAddress(t *testing.T) {
	output := writeMboxFromAddr(t, "")
	if !strings.HasPrefix(output, "From unknown@example.com") {
		t.Errorf("Expected 'From unknown@example.com', got: %s", output[:50])
	}
}

func TestWriteToMbox_InvalidAddress(t *testing.T) {
	output := writeMboxFromAddr(t, "invalid address without @")
	if !strings.HasPrefix(output, "From invalid@example.com") {
		t.Errorf("Expected 'From invalid@example.com', got: %s", output[:50])
	}
}

func TestWriteToMbox_WithFromLine(t *testing.T) {
	var buf bytes.Buffer
	fromAddr := "test@example.com"
	date := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	rawEmail := []byte("From original@example.com\r\n\r\nBody")

	err := writeToMbox(&buf, rawEmail, fromAddr, date)
	if err != nil {
		t.Fatalf("writeToMbox failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, ">From original@example.com") {
		t.Errorf("Expected escaped 'From' line, got: %s", output)
	}
}

func TestWriteToMbox_StreamingBuffered(t *testing.T) {
	var buf bytes.Buffer
	bw := bufio.NewWriterSize(&buf, 4096)
	fromAddr := "stream@example.com"
	date := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	rawEmail := []byte("Subject: Large Email\r\nFrom me@example.com\r\n\r\nLine 1\r\nFrom Another Person\r\nLine 2")

	err := writeToMbox(bw, rawEmail, fromAddr, date)
	if err != nil {
		t.Fatalf("writeToMbox failed: %v", err)
	}
	if flushErr := bw.Flush(); flushErr != nil {
		t.Fatalf("bw.Flush failed: %v", flushErr)
	}

	msgs, err := readFromMbox(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("readFromMbox failed: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].From != "stream@example.com" {
		t.Errorf("expected sender stream@example.com, got %q", msgs[0].From)
	}
	if !strings.Contains(buf.String(), ">From me@example.com") || !strings.Contains(buf.String(), ">From Another Person") {
		t.Errorf("expected escaped From lines in buffer, got: %s", buf.String())
	}
}

func TestReadFromMbox_Valid(t *testing.T) {
	mboxContent := `From user1@example.com Mon Jan  2 15:04:05 2006
Subject: Test 1

Body 1
From user2@example.com Tue Jan  3 16:04:05 2006
Subject: Test 2

Body 2
`

	msgs, err := readFromMbox(strings.NewReader(mboxContent))
	if err != nil {
		t.Fatalf("readFromMbox failed: %v", err)
	}

	if len(msgs) != 2 {
		t.Errorf("Expected 2 messages, got %d", len(msgs))
	}

	if msgs[0].From != "user1@example.com" {
		t.Errorf("Expected 'user1@example.com', got '%s'", msgs[0].From)
	}

	if msgs[1].From != "user2@example.com" {
		t.Errorf("Expected 'user2@example.com', got '%s'", msgs[1].From)
	}
}

func TestReadFromMbox_EmptyFrom(t *testing.T) {
	mboxContent := `From unknown Mon Jan  2 15:04:05 2006
Subject: Test

Body
`

	msgs, err := readFromMbox(strings.NewReader(mboxContent))
	if err != nil {
		t.Fatalf("readFromMbox failed: %v", err)
	}

	if len(msgs) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(msgs))
	}

	if msgs[0].From != "unknown" {
		t.Errorf("Expected 'unknown', got '%s'", msgs[0].From)
	}
}

func TestReadFromMbox_Malformed(t *testing.T) {
	mboxContent := `Subject: Test

Body without From line
`

	msgs, err := readFromMbox(strings.NewReader(mboxContent))
	if err != nil {
		t.Fatalf("readFromMbox failed: %v", err)
	}

	if len(msgs) != 0 {
		t.Errorf("Expected 0 messages (no From line), got %d", len(msgs))
	}
}

func TestReadFromMbox_EscapedFrom(t *testing.T) {
	mboxContent := `From user@example.com Mon Jan  2 15:04:05 2006
Subject: Test

>From escaped@example.com
Body
`

	msgs, err := readFromMbox(strings.NewReader(mboxContent))
	if err != nil {
		t.Fatalf("readFromMbox failed: %v", err)
	}

	if len(msgs) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(msgs))
	}

	if !strings.Contains(string(msgs[0].Body), "From escaped@example.com") {
		t.Errorf("Expected unescaped 'From' line in body")
	}
}

func TestDownload_SingleLabelSafeAppendAndDeduplication(t *testing.T) {
	tempDir := t.TempDir()
	accDir := filepath.Join(tempDir, "acc1")
	configDir := filepath.Join(tempDir, "config")
	_ = os.MkdirAll(accDir, 0700)
	_ = os.MkdirAll(configDir, 0700)

	fixedDate := time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC)
	eml1 := "Message-ID: <msg1@test.com>\nDate: Wed, 02 Sep 2026 10:00:00 +0000\nFrom: user1@test.com\nSubject: Item 1\n\nContent 1"
	eml2 := "Message-ID: <msg2@test.com>\nDate: Wed, 02 Sep 2026 10:00:00 +0000\nFrom: user2@test.com\nSubject: Item 2\n\nContent 2"
	_ = msg.Store(cachedir.At(accDir), "msg001", []byte(eml1), fixedDate)
	_ = msg.Store(cachedir.At(accDir), "msg002", []byte(eml2), fixedDate)

	mockClient := archiveMock(accDir, configDir, "receipts", "msg001", "msg002")
	session := singleAccountSession(tempDir, configDir, mockClient)

	mboxPath := filepath.Join(tempDir, "receipts.mbox")
	cliApp := InitCLI(session)

	flagDownloadDryRun = false
	flagDownloadOverwrite = false
	flagDownloadRecursive = false
	err := cliApp.Execute([]string{"label", "download", "%receipts", mboxPath})
	if err != nil {
		t.Fatalf("first download failed: %v", err)
	}

	existingIdx, count, err := getMboxExistingMessages(mboxPath)
	if err != nil || count != 2 {
		t.Fatalf("expected 2 messages in mbox, got %d (err: %v)", count, err)
	}
	if !existingIdx.contains("", &email.Email{MessageID: "<msg1@test.com>"}) || !existingIdx.contains("", &email.Email{MessageID: "<msg2@test.com>"}) {
		t.Errorf("missing message IDs in existing index")
	}

	eml3 := "Message-ID: <msg3@test.com>\nDate: Wed, 02 Sep 2026 10:00:00 +0000\nFrom: user3@test.com\nSubject: Item 3\n\nContent 3"
	_ = msg.Store(cachedir.At(accDir), "msg003", []byte(eml3), fixedDate)
	mockClient.DownloadedIDs["receipts"] = []email.ID{"msg001", "msg002", "msg003"}

	flagDownloadDryRun = false
	flagDownloadOverwrite = false
	flagDownloadRecursive = false
	err = cliApp.Execute([]string{"label", "download", "%receipts", mboxPath})
	if err != nil {
		t.Fatalf("second download failed: %v", err)
	}

	existingIdx, count, err = getMboxExistingMessages(mboxPath)
	if err != nil || count != 3 {
		t.Fatalf("expected 3 messages in mbox after safe append, got %d (err: %v)", count, err)
	}
	if !existingIdx.contains("", &email.Email{MessageID: "<msg3@test.com>"}) {
		t.Errorf("expected msg3 to be appended, but it is not in the existing index")
	}
}

// TestDownload_HeaderlessMessageIsNotDuplicatedOnReExport is M3's regression:
// a message with no Message-ID header has to be recognized as already
// archived by its Subject and Date (the same fallback isEmailMatch and
// destFolderIndex use), or exporting the same folder twice appends it a
// second time.
func TestDownload_HeaderlessMessageIsNotDuplicatedOnReExport(t *testing.T) {
	tempDir := t.TempDir()
	accDir := filepath.Join(tempDir, "acc1")
	configDir := filepath.Join(tempDir, "config")
	_ = os.MkdirAll(accDir, 0700)
	_ = os.MkdirAll(configDir, 0700)

	fixedDate := time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC)
	// No Message-ID header at all.
	eml := "Date: Wed, 02 Sep 2026 10:00:00 +0000\nFrom: user@test.com\nSubject: No Message-ID Here\n\nContent"
	_ = msg.Store(cachedir.At(accDir), "msg001", []byte(eml), fixedDate)

	mockClient := archiveMock(accDir, configDir, "receipts", "msg001")
	session := singleAccountSession(tempDir, configDir, mockClient)

	mboxPath := filepath.Join(tempDir, "receipts.mbox")
	cliApp := InitCLI(session)

	flagDownloadDryRun = false
	flagDownloadOverwrite = false
	flagDownloadRecursive = false
	if err := cliApp.Execute([]string{"label", "download", "%receipts", mboxPath}); err != nil {
		t.Fatalf("first export failed: %v", err)
	}
	if _, count, err := getMboxExistingMessages(mboxPath); err != nil || count != 1 {
		t.Fatalf("expected 1 message after first export, got %d (err: %v)", count, err)
	}

	// Exporting the same folder again must not duplicate the header-less
	// message: same Subject, same Date, no Message-ID on either side.
	if err := cliApp.Execute([]string{"label", "download", "%receipts", mboxPath}); err != nil {
		t.Fatalf("second export failed: %v", err)
	}
	if _, count, err := getMboxExistingMessages(mboxPath); err != nil || count != 1 {
		t.Fatalf("expected the header-less message to be deduplicated, still 1 message, got %d (err: %v)", count, err)
	}
}

func TestDownload_DryRun(t *testing.T) {
	tempDir := t.TempDir()
	accDir := filepath.Join(tempDir, "acc1")
	configDir := filepath.Join(tempDir, "config")
	_ = os.MkdirAll(accDir, 0700)
	_ = os.MkdirAll(configDir, 0700)

	fixedDate := time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC)
	eml1 := "Message-ID: <msg1@test.com>\nDate: Wed, 02 Sep 2026 10:00:00 +0000\nFrom: user1@test.com\nSubject: Item 1\n\nContent 1"
	_ = msg.Store(cachedir.At(accDir), "msg001", []byte(eml1), fixedDate)

	mockClient := archiveMock(accDir, configDir, "receipts", "msg001")
	session := singleAccountSession(tempDir, configDir, mockClient)

	mboxPath := filepath.Join(tempDir, "dryrun.mbox")
	cliApp := InitCLI(session)

	flagDownloadDryRun = false
	flagDownloadOverwrite = false
	flagDownloadRecursive = false
	err := cliApp.Execute([]string{"label", "download", "-d", "%receipts", mboxPath})
	if err != nil {
		t.Fatalf("dry run failed: %v", err)
	}

	if _, err := os.Stat(mboxPath); !os.IsNotExist(err) {
		t.Errorf("dry run should not create file %s", mboxPath)
	}
}

func TestDownload_RecursiveDirectoryMirroring(t *testing.T) {
	tempDir := t.TempDir()
	accDir := filepath.Join(tempDir, "acc1")
	configDir := filepath.Join(tempDir, "config")
	_ = os.MkdirAll(accDir, 0700)
	_ = os.MkdirAll(configDir, 0700)

	fixedDate := time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC)
	eml1 := "Message-ID: <g1@test.com>\nDate: Wed, 02 Sep 2026 10:00:00 +0000\nFrom: user@test.com\nSubject: Geometry\n\nContent"
	eml2 := "Message-ID: <s1@test.com>\nDate: Wed, 02 Sep 2026 10:00:00 +0000\nFrom: user@test.com\nSubject: SOCG\n\nContent"
	_ = msg.Store(cachedir.At(accDir), "g001", []byte(eml1), fixedDate)
	_ = msg.Store(cachedir.At(accDir), "s001", []byte(eml2), fixedDate)

	mockClient := &mailclient.MockMailClient{
		Labels: []string{"research", "research/geometry", "research/conferences/socg"},
		DownloadedIDs: map[string][]email.ID{
			"research":                  {},
			"research/geometry":         {"g001"},
			"research/conferences/socg": {"s001"},
		},
		Cfg: &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: accDir, ConfigDir: configDir}, AccountCache: cachedir.At(accDir)},
	}

	session := singleAccountSession(tempDir, configDir, mockClient)

	outDir := filepath.Join(tempDir, "archive")
	cliApp := InitCLI(session)

	flagDownloadDryRun = false
	flagDownloadOverwrite = false
	flagDownloadRecursive = true
	err := cliApp.Execute([]string{"label", "download", "-r", "%research", outDir})
	if err != nil {
		t.Fatalf("recursive download failed: %v", err)
	}

	geoMbox := filepath.Join(outDir, "geometry")
	socgMbox := filepath.Join(outDir, "conferences", "socg")

	if _, err := os.Stat(geoMbox); err != nil {
		t.Errorf("expected %s to exist: %v", geoMbox, err)
	}
	if _, err := os.Stat(socgMbox); err != nil {
		t.Errorf("expected %s to exist: %v", socgMbox, err)
	}
}

func TestResolveSafeMboxPath(t *testing.T) {
	destDir := filepath.Join(string(filepath.Separator), "var", "mail", "backup")

	tests := []struct {
		name      string
		relPath   string
		want      string
		shouldErr bool
	}{
		{
			name:      "simple relative path",
			relPath:   "inbox",
			want:      filepath.Join(destDir, "inbox"),
			shouldErr: false,
		},
		{
			name:      "nested relative path",
			relPath:   "work/projects",
			want:      filepath.Join(destDir, "work", "projects"),
			shouldErr: false,
		},
		{
			name:      "directory traversal escape",
			relPath:   "../../etc/passwd",
			shouldErr: true,
		},
		{
			name:      "embedded traversal escape",
			relPath:   "work/../../.ssh/authorized_keys",
			shouldErr: true,
		},
		{
			name:      "absolute path",
			relPath:   "/etc/shadow",
			shouldErr: true,
		},
		{
			name:      "empty relative path",
			relPath:   "",
			shouldErr: true,
		},
		{
			name:      "dot relative path",
			relPath:   ".",
			shouldErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveSafeMboxPath(destDir, tt.relPath)
			if tt.shouldErr {
				if err == nil {
					t.Errorf("expected error for relPath %q, got nil (result: %q)", tt.relPath, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestDownloadRecursive_RejectsPathTraversal(t *testing.T) {
	tempDir := t.TempDir()
	configDir := filepath.Join(tempDir, "config")
	accDir := filepath.Join(tempDir, "cache")
	_ = os.MkdirAll(configDir, 0700)
	_ = os.MkdirAll(accDir, 0700)

	fixedDate := time.Date(2026, 9, 18, 10, 0, 0, 0, time.UTC)
	eml := "Message-ID: <pwn@test.com>\nDate: Wed, 02 Sep 2026 10:00:00 +0000\nFrom: user@test.com\nSubject: Test\n\nContent"
	_ = msg.Store(cachedir.At(accDir), "pwn1", []byte(eml), fixedDate)

	mockClient := &mailclient.MockMailClient{
		Labels: []string{"archive", "archive/../../escaped_target"},
		DownloadedIDs: map[string][]email.ID{
			"archive":                      {},
			"archive/../../escaped_target": {"pwn1"},
		},
		Cfg: &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: accDir, ConfigDir: configDir}, AccountCache: cachedir.At(accDir)},
	}

	session := singleAccountSession(tempDir, configDir, mockClient)

	outDir := filepath.Join(tempDir, "archive_dest")
	cliApp := InitCLI(session)

	flagDownloadDryRun = false
	flagDownloadOverwrite = false
	flagDownloadRecursive = true
	err := cliApp.Execute([]string{"label", "download", "-r", "%archive", outDir})
	if err != nil {
		t.Fatalf("recursive download failed: %v", err)
	}

	escapedFile := filepath.Join(tempDir, "escaped_target")
	if _, err := os.Stat(escapedFile); !os.IsNotExist(err) {
		t.Fatalf("security violation: path traversal allowed creation of %s", escapedFile)
	}
}

func TestUploadCmd_SuccessAndFailure(t *testing.T) {
	tempDir := t.TempDir()
	mboxPath := filepath.Join(tempDir, "test.mbox")

	var mboxBuf bytes.Buffer
	date := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	_ = writeToMbox(&mboxBuf, []byte("Subject: One\r\n\r\nBody 1"), "a@test.com", date)
	_ = writeToMbox(&mboxBuf, []byte("Subject: Two\r\n\r\nBody 2"), "b@test.com", date)
	if err := os.WriteFile(mboxPath, mboxBuf.Bytes(), 0600); err != nil {
		t.Fatalf("failed to write test mbox: %v", err)
	}

	mockClient := &mailclient.MockMailClient{
		Labels: []string{"receipts"},
	}
	session := &Session{
		Config: &cfg_g.Config{
			Accounts: []cfg_acc.AccountConfig{{Name: "acc1", Type: "jmap"}},
		},
		Deps: Deps{
			Clients: stubClients{
				ForSelectedFn: func(_ *cfg_g.Config) (mailclient.MailClient, error) {
					return mockClient, nil
				},
			},
		},
	}

	// 1. Success case
	cliApp := InitCLI(session)
	err := cliApp.Execute([]string{"label", "upload", "%receipts", mboxPath})
	if err != nil {
		t.Fatalf("unexpected error in upload: %v", err)
	}
	if len(mockClient.UploadedEmails) != 2 {
		t.Fatalf("expected 2 uploaded emails, got %d", len(mockClient.UploadedEmails))
	}

	// 2. Failure case
	mockClient.FailOn = map[string]error{"UploadRawEmail": errors.New("simulated network drop")}
	err = cliApp.Execute([]string{"label", "upload", "%receipts", mboxPath})
	if err == nil {
		t.Fatal("expected error on failed upload, got nil")
	}
	if !strings.Contains(err.Error(), "uploaded 0/2 messages to \"receipts\"") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestWriteMessagesToMbox_ExistingFileErrors(t *testing.T) {
	tempDir := t.TempDir()
	dirMbox := filepath.Join(tempDir, "is_a_directory")
	if err := os.Mkdir(dirMbox, 0755); err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}

	raw := [][]byte{[]byte("Subject: New\r\n\r\nBody")}
	// Appending to a directory should fail during copy or open
	_, err := writeMessagesToMbox(dirMbox, raw, false)
	if err == nil {
		t.Fatal("expected error when existing mbox path is a directory, got nil")
	}

	// Permission error case
	unreadableFile := filepath.Join(tempDir, "unreadable.mbox")
	if writeErr := os.WriteFile(unreadableFile, []byte("From x@test.com\n\n"), 0000); writeErr != nil {
		t.Fatalf("failed to write unreadable file: %v", writeErr)
	}
	defer func() { _ = os.Chmod(unreadableFile, 0600) }()

	_, err = writeMessagesToMbox(unreadableFile, raw, false)
	if os.Getuid() != 0 && err == nil {
		t.Fatal("expected permission error when existing mbox cannot be opened, got nil")
	}
}

func TestMboxrd_EscapingRoundTrip(t *testing.T) {
	originalBody := "Subject: Mboxrd Test\r\n\r\n" +
		"From alice@example.com Mon Jan  1 00:00:00 2024\n" +
		">From bob@example.com Mon Jan  1 00:00:00 2024\n" +
		">>From charlie@example.com Mon Jan  1 00:00:00 2024\n" +
		"Normal body line\n" +
		">Not a From line\n"

	var buf bytes.Buffer
	date := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	if err := writeToMbox(&buf, []byte(originalBody), "sender@example.com", date); err != nil {
		t.Fatalf("writeToMbox failed: %v", err)
	}

	written := buf.String()
	// Assert proper mboxrd escaping
	if !strings.Contains(written, "\n>From alice@example.com") {
		t.Errorf("expected >From alice, got:\n%s", written)
	}
	if !strings.Contains(written, "\n>>From bob@example.com") {
		t.Errorf("expected >>From bob, got:\n%s", written)
	}
	if !strings.Contains(written, "\n>>>From charlie@example.com") {
		t.Errorf("expected >>>From charlie, got:\n%s", written)
	}
	if !strings.Contains(written, "\n>Not a From line\n") {
		t.Errorf("non-From line should not be escaped, got:\n%s", written)
	}

	msgs, err := readFromMbox(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("readFromMbox failed: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected exactly 1 message, got %d", len(msgs))
	}

	readBody := string(msgs[0].Body)
	expectedBody := "Subject: Mboxrd Test\n\n" +
		"From alice@example.com Mon Jan  1 00:00:00 2024\n" +
		">From bob@example.com Mon Jan  1 00:00:00 2024\n" +
		">>From charlie@example.com Mon Jan  1 00:00:00 2024\n" +
		"Normal body line\n" +
		">Not a From line\n\n"

	if readBody != expectedBody {
		t.Errorf("roundtrip mismatch:\ngot:\n%q\nwant:\n%q", readBody, expectedBody)
	}
}
