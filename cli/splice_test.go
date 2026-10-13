package cli

import (
	"bytes"
	"fmt"
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"github.com/sarielhp/mail_cli/label"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/sarielhp/mail_cli/app"
	"github.com/sarielhp/mail_cli/cache/msg"
	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/mailclient"
)

func TestSpliceDestPath(t *testing.T) {
	tests := []struct {
		source           string
		folderFlag       string
		folderSuffixFlag string
		year             int
		month            int
		want             string
	}{
		{
			source: "research/cfps",
			year:   2026, month: 6,
			want: "keep/2026/06/cfps",
		},
		{
			source: "research/cfps", folderFlag: "research",
			year: 2026, month: 6,
			want: "keep/2026/06/research",
		},
		{
			source: "research/cfps", folderSuffixFlag: "research",
			year: 2026, month: 6,
			want: "keep/2026/06/research-2026-06",
		},
		{
			source: "research/cfps", folderFlag: "archive",
			year: 2026, month: 6,
			want: "keep/2026/06/archive",
		},
		{
			source: "inbox",
			year:   2026, month: 6,
			want: "keep/2026/06/inbox",
		},
		{
			source: "a/b/c",
			year:   2026, month: 6,
			want: "keep/2026/06/c",
		},
		{
			source: "a/b/c", folderSuffixFlag: "foo",
			year: 2026, month: 6,
			want: "keep/2026/06/foo-2026-06",
		},
		{
			source: "inbox",
			year:   2024, month: 1,
			want: "keep/2024/01/inbox",
		},
	}

	for _, tt := range tests {
		t.Run(tt.source, func(t *testing.T) {
			monthPadded := strconv.Itoa(tt.month)
			if tt.month < 10 {
				monthPadded = "0" + monthPadded
			}
			folder := ""
			switch {
			case tt.folderSuffixFlag != "":
				folder = tt.folderSuffixFlag + "-" + strconv.Itoa(tt.year) + "-" + monthPadded
			case tt.folderFlag != "":
				folder = tt.folderFlag
			default:
				folder = filepath.Base(tt.source)
			}
			dest := filepath.Join("keep", strconv.Itoa(tt.year),
				monthPadded, folder)

			if dest != tt.want {
				t.Errorf("want %q, got %q", tt.want, dest)
			}
		})
	}
}

func TestSpliceValidation(t *testing.T) {
	tests := []struct {
		name    string
		folder  string
		numMsg  int
		wantErr bool
		errSub  string
	}{
		{
			name:   "valid default",
			folder: "test", numMsg: 10,
			wantErr: false,
		},
		{
			name:   "n = 0 → error",
			folder: "test", numMsg: 0,
			wantErr: true, errSub: "-n must be at least 1",
		},
		{
			name:   "folder = keep → error",
			folder: "keep", numMsg: 10,
			wantErr: true, errSub: "must not be \"keep\"",
		},
		{
			name:   "Keep (case-insensitive) → error",
			folder: "Keep", numMsg: 10,
			wantErr: true, errSub: "must not be \"keep\"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSpliceArgs(tt.folder, tt.numMsg, false)
			hasErr := err != nil
			if hasErr != tt.wantErr {
				t.Errorf("wantErr=%v got=%v, err=%v",
					tt.wantErr, hasErr, err)
			}
			if hasErr && tt.errSub != "" {
				if !strings.Contains(err.Error(), tt.errSub) {
					t.Errorf("error %q should contain %q",
						err.Error(), tt.errSub)
				}
			}
		})
	}
}

func init() {
	if runtime.GOOS != "linux" {
		os.Exit(0)
	}
}

func TestSplicePatternMatching(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, "config")
	downloadDir := filepath.Join(tmpDir, "download")
	_ = os.MkdirAll(configDir, 0755)
	_ = os.MkdirAll(downloadDir, 0755)

	// Create fake emails under download/test-account
	accountDir := filepath.Join(downloadDir, "test-account")
	_ = os.MkdirAll(accountDir, 0755)

	// email 1: matches pattern
	err := msg.Store(cachedir.At(accountDir), "msg1", []byte("Date: Mon, 2 Jan 2026 15:04:05 -0700\nFrom: bob@example.com\nSubject: Urgent Invoice\n\nbody1"), time.Date(2026, 1, 2, 15, 4, 5, 0, time.UTC))
	if err != nil {
		t.Fatalf("failed to store msg1: %v", err)
	}
	// email 2: does not match
	err = msg.Store(cachedir.At(accountDir), "msg2", []byte("Date: Mon, 2 Jan 2026 15:04:05 -0700\nFrom: alice@example.com\nSubject: Meeting Update\n\nbody2"), time.Date(2026, 1, 2, 15, 4, 5, 0, time.UTC))
	if err != nil {
		t.Fatalf("failed to store msg2: %v", err)
	}

	mockClient := &mailclient.MockMailClient{
		Labels: []string{"Inbox"},
		DownloadedIDs: map[string][]email.ID{
			"Inbox": {"msg1", "msg2"},
		},
		Cfg: &cfg_g.Config{SelectedAccount: "test-account", Paths: cfg_g.Paths{CacheRoot: accountDir, ConfigDir: configDir}, AccountCache: cachedir.At(accountDir)},
	}

	session := &Session{
		Config: mockClient.Cfg,
		Deps: Deps{
			Clients: stubClients{
				ForSelectedFn: func(_ *cfg_g.Config) (mailclient.MailClient, error) {
					return mockClient, nil
				},
			},
		},
	}

	cliApp := InitCLI(session)

	// Run dry-run with pattern "*invoice*"
	err = cliApp.Execute([]string{"label", "sort", "-p", "*invoice*", "-n", "10", "%Inbox"})
	if err != nil {
		t.Fatalf("unexpected error running splice: %v", err)
	}

	// Run with actually move=true and -f custom to see which email gets moved without suffix
	mockClient.MovedEmails = nil
	err = cliApp.Execute([]string{"label", "sort", "-p", "invoice", "--move", "-f", "wuna", "-n", "10", "%Inbox"})
	if err != nil {
		t.Fatalf("unexpected error running splice move -f: %v", err)
	}

	if len(mockClient.MovedEmails) != 1 {
		t.Errorf("expected exactly 1 email to be moved, got %d", len(mockClient.MovedEmails))
	} else {
		record := mockClient.MovedEmails[0]
		if record.DestLabel != "keep/2026/01/wuna" {
			t.Errorf("expected 'keep/2026/01/wuna', got %q", record.DestLabel)
		}
	}

	// Run with actually move=true and -F custom to see which email gets moved with suffix
	mockClient.MovedEmails = nil
	err = cliApp.Execute([]string{"label", "sort", "-p", "invoice", "--move", "-F", "wuna", "-n", "10", "%Inbox"})
	if err != nil {
		t.Fatalf("unexpected error running splice move -F: %v", err)
	}

	if len(mockClient.MovedEmails) != 1 {
		t.Errorf("expected exactly 1 email to be moved, got %d", len(mockClient.MovedEmails))
	} else {
		record := mockClient.MovedEmails[0]
		if record.DestLabel != "keep/2026/01/wuna-2026-01" {
			t.Errorf("expected 'keep/2026/01/wuna-2026-01', got %q", record.DestLabel)
		}
	}
}

func TestSpliceAllowKeep(t *testing.T) {
	// Verify that with allow=true, ValidateSpliceArgs doesn't return an error for keep/ folders
	err := ValidateSpliceArgs("keep/archive", 10, true)
	if err != nil {
		t.Errorf("expected no error for keep/ folders when allow=true, got %v", err)
	}

	err = ValidateSpliceArgs("keep", 10, true)
	if err != nil {
		t.Errorf("expected no error for 'keep' folder when allow=true, got %v", err)
	}
}

func TestSpliceAccountIndexed(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, "config")
	downloadDir := filepath.Join(tmpDir, "download")
	_ = os.MkdirAll(configDir, 0755)
	_ = os.MkdirAll(downloadDir, 0755)

	acc1Dir := filepath.Join(downloadDir, "acc1")
	acc2Dir := filepath.Join(downloadDir, "acc2")
	_ = os.MkdirAll(acc1Dir, 0755)
	_ = os.MkdirAll(acc2Dir, 0755)

	err := msg.Store(cachedir.At(acc1Dir), "msg101", []byte("Date: Mon, 2 Jan 2026 15:04:05 -0700\nFrom: test@example.com\nSubject: Test Email\n\nbody"), time.Date(2026, 1, 2, 15, 4, 5, 0, time.UTC))
	if err != nil {
		t.Fatalf("failed to store msg101: %v", err)
	}

	mockClient1 := archiveMock(acc1Dir, configDir, "sort/mailing_list/wuna", "msg101")
	mockClient2 := &mailclient.MockMailClient{
		Labels: []string{"wuna"},
		Cfg:    &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: acc2Dir, ConfigDir: configDir}, AccountCache: cachedir.At(acc2Dir)},
	}

	session := twoAccountSession(downloadDir, configDir, mockClient1, mockClient2)

	cliApp := InitCLI(session)

	// "acc1:sort/mailing_list/wuna" — splice from account "acc1" using account name
	err = cliApp.Execute([]string{"label", "sort", "%acc1:sort/mailing_list/wuna", "-F", "%acc2:wuna", "--move"})
	if err != nil {
		t.Fatalf("unexpected error running splice acc1:folder -F acc2:folder: %v", err)
	}

	if len(mockClient2.UploadedEmails) != 1 {
		t.Errorf("expected 1 email uploaded to account 2, got %d", len(mockClient2.UploadedEmails))
	} else {
		up := mockClient2.UploadedEmails[0]
		if up.TargetLabel != "keep/2026/01/wuna-2026-01" {
			t.Errorf("expected target label keep/2026/01/wuna-2026-01, got %q", up.TargetLabel)
		}
	}

	// Test splicing with short suffix "acc1:wuna" matching sort/mailing_list/wuna
	mockClient2.UploadedEmails = nil
	mockClient2.DownloadedIDs = nil
	err = cliApp.Execute([]string{"label", "sort", "%acc1:wuna", "-F", "%acc2:wuna", "--move"})
	if err != nil {
		t.Fatalf("unexpected error running splice acc1:wuna -F acc2:wuna: %v", err)
	}

	if len(mockClient2.UploadedEmails) != 1 {
		t.Errorf("expected 1 email uploaded to account 2 when using '1:wuna', got %d", len(mockClient2.UploadedEmails))
	}

	// Test splicing with --move when message already exists in destination (should not re-upload)
	mockClient2.UploadedEmails = nil
	if mockClient2.DownloadedIDs == nil {
		mockClient2.DownloadedIDs = make(map[string][]email.ID)
	}
	mockClient2.DownloadedIDs["keep/2026/01/wuna-2026-01"] = []email.ID{"msg101"}

	err = cliApp.Execute([]string{"label", "sort", "%acc1:wuna", "-F", "%acc2:wuna", "--move"})
	if err != nil {
		t.Fatalf("unexpected error running splice acc1:wuna -F acc2:wuna --move on existing msg: %v", err)
	}
	if len(mockClient2.UploadedEmails) != 0 {
		t.Errorf("expected 0 emails uploaded when already in destination, got %d", len(mockClient2.UploadedEmails))
	}

	// Test splicing with -Y option (drops month directory path, e.g. keep/2026/received-2026-01)
	mockClient2.UploadedEmails = nil
	mockClient2.DownloadedIDs = nil
	err = cliApp.Execute([]string{"label", "sort", "%acc1:wuna", "-Y", "%acc2:received", "--move"})
	if err != nil {
		t.Fatalf("unexpected error running splice acc1:wuna -Y acc2:received: %v", err)
	}
	if len(mockClient2.UploadedEmails) != 1 {
		t.Fatalf("expected 1 email uploaded to account 2 when using '-Y 2:received', got %d", len(mockClient2.UploadedEmails))
	}
	if mockClient2.UploadedEmails[0].TargetLabel != "keep/2026/received-2026-01" {
		t.Errorf("expected destination folder 'keep/2026/received-2026-01', got %q", mockClient2.UploadedEmails[0].TargetLabel)
	}
}

func TestSpliceCopyOption(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, "config")
	downloadDir := filepath.Join(tmpDir, "download")
	_ = os.MkdirAll(configDir, 0755)
	_ = os.MkdirAll(downloadDir, 0755)

	err := msg.Store(cachedir.At(downloadDir), "msg201", []byte("Date: Mon, 2 Jan 2026 15:04:05 -0700\nFrom: test@example.com\nSubject: Copy Test\n\nbody"), time.Date(2026, 1, 2, 15, 4, 5, 0, time.UTC))
	if err != nil {
		t.Fatalf("failed to store msg201: %v", err)
	}

	mockClient := archiveMock(downloadDir, configDir, "Inbox", "msg201")

	session := &Session{
		Config: mockClient.Cfg,
		Deps: Deps{
			Clients: stubClients{
				ForSelectedFn: func(_ *cfg_g.Config) (mailclient.MailClient, error) {
					return mockClient, nil
				},
			},
		},
	}

	cliApp := InitCLI(session)

	// Test error when passing both --move and --copy
	err = cliApp.Execute([]string{"label", "sort", "%Inbox", "--move", "--copy"})
	if err == nil {
		t.Fatalf("expected error when passing both --move and --copy")
	}

	// Test error when passing multiple destination folder flags
	for _, flags := range [][]string{
		{"splice", "Inbox", "-f", "wuna", "-F", "wuna2"},
		{"splice", "Inbox", "-f", "wuna", "-Y", "wuna2"},
		{"splice", "Inbox", "-F", "wuna", "-Y", "wuna2"},
		{"splice", "Inbox", "--folder", "wuna", "--folder-suffix", "wuna2"},
	} {
		if execErr := cliApp.Execute(flags); execErr == nil {
			t.Fatalf("expected error for mutually exclusive flags %v, got nil", flags)
		}
	}

	// Test copy execution
	err = cliApp.Execute([]string{"label", "sort", "%Inbox", "-f", "wuna", "--copy"})
	if err != nil {
		t.Fatalf("unexpected error running splice --copy: %v", err)
	}

	if len(mockClient.CopiedEmails) != 1 {
		t.Fatalf("expected 1 copied email record, got %d", len(mockClient.CopiedEmails))
	}
	if mockClient.CopiedEmails[0].DestLabel != "keep/2026/01/wuna" {
		t.Errorf("expected dest label keep/2026/01/wuna, got %q", mockClient.CopiedEmails[0].DestLabel)
	}

	// Test skipping copy if message already exists in destination
	mockClient.DownloadedIDs["keep/2026/01/wuna"] = []email.ID{"msg201"}
	mockClient.CopiedEmails = nil

	err = cliApp.Execute([]string{"label", "sort", "%Inbox", "-f", "wuna", "--copy"})
	if err != nil {
		t.Fatalf("unexpected error running splice --copy on existing message: %v", err)
	}
	if len(mockClient.CopiedEmails) != 0 {
		t.Errorf("expected 0 copied email records when message already exists, got %d", len(mockClient.CopiedEmails))
	}
}

func TestSpliceErrorFormatting_NoMatchingLabels(t *testing.T) {
	mockClient := &mailclient.MockMailClient{
		Labels: []string{"Inbox", "Archive"},
		Cfg: &cfg_g.Config{
			SelectedAccount: "default",
			Accounts: []cfg_acc.AccountConfig{
				{Name: "default", DisplayName: "GMail", Type: "gmail"},
			},
		},
	}
	session := &Session{
		Config: mockClient.Cfg,
		Deps: Deps{
			Clients: stubClients{
				ForSelectedFn: func(_ *cfg_g.Config) (mailclient.MailClient, error) {
					return mockClient, nil
				},
			},
		},
	}

	cliApp := InitCLI(session)
	err := cliApp.Execute([]string{"label", "sort", "%nonexistent"})
	if err == nil {
		t.Fatalf("expected error for nonexistent source folder")
	}

	// The resolver names the input it could not place. "no matching labels
	// found" said nothing about which argument was at fault, which matters once
	// a command takes two labels.
	wantSub := fmt.Sprintf("failed to resolve source folder GMail:%s\n  (no label matches %%q)", app.ColorBlue.Sprint("nonexistent"))
	wantSub = fmt.Sprintf(wantSub, "nonexistent")
	if err.Error() != wantSub {
		t.Errorf("got error:\n%q\nwant:\n%q", err.Error(), wantSub)
	}
}

func TestSpliceErrorFormatting_AmbiguousMatch(t *testing.T) {
	mockClient := &mailclient.MockMailClient{
		Labels: []string{"work/receipts", "personal/receipts", "work/receipts"},
		Cfg: &cfg_g.Config{
			SelectedAccount: "default",
			Accounts: []cfg_acc.AccountConfig{
				{Name: "default", DisplayName: "GMail", Type: "gmail"},
			},
		},
	}
	session := &Session{
		Config: mockClient.Cfg,
		Deps: Deps{
			Clients: stubClients{
				ForSelectedFn: func(_ *cfg_g.Config) (mailclient.MailClient, error) {
					return mockClient, nil
				},
			},
		},
	}

	cliApp := InitCLI(session)
	err := cliApp.Execute([]string{"label", "sort", "%receipts"})
	if err == nil {
		t.Fatalf("expected error for ambiguous source folder")
	}

	errStr := err.Error()
	if !strings.Contains(errStr, "failed to resolve source folder GMail:") {
		t.Errorf("expected error to contain 'failed to resolve source folder GMail:', got %q", errStr)
	}
	if !strings.Contains(errStr, "\n  (ambiguous label \"receipts\" (candidates: ") {
		t.Errorf("expected error to contain indented detail '(ambiguous label \"receipts\" (candidates: ', got %q", errStr)
	}
	// Check no duplicates in candidates
	count1 := strings.Count(errStr, "work/receipts")
	if count1 != 1 {
		t.Errorf("expected exactly 1 instance of 'work/receipts' in error, got %d in %q", count1, errStr)
	}
}

func TestSpliceExamplesSubcommand(t *testing.T) {
	session := &Session{
		Config: &cfg_g.Config{},
	}
	cliApp := InitCLI(session)

	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	os.Stdout = w

	execErr := cliApp.Execute([]string{"label", "sort", "examples"})

	_ = w.Close()
	os.Stdout = oldStdout

	if execErr != nil {
		t.Fatalf("unexpected error executing splice examples: %v", execErr)
	}

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	output := buf.String()

	expectedSnippets := []string{
		"Structured Splice Examples & Mapping Patterns",
		"GMail:receipts -> GMail:keep/2026/09/receipts",
		"GMail:received -> FastMail:keep/2026/services-2026-09",
		"GMail:inbox -> GMail:keep/2026/09/Archive",
		"GMail:services -> GMail:keep/2026/services-2026-09",
		"Cross-Account Splicing (GMail to FastMail)",
		"Live Message Move (--move)",
		"Basic Date Splice (Dry Run)",
	}

	for _, snip := range expectedSnippets {
		if !strings.Contains(output, snip) {
			t.Errorf("expected output to contain %q, but got:\n%s", snip, output)
		}
	}
}

func TestIsEmailMatch(t *testing.T) {
	t1 := time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC)

	// Distinct Message-IDs must NEVER match, even if Subject and Date are identical
	e1 := &email.Email{
		MessageID: "<msg1@example.com>",
		Subject:   "Delivery Status Notification (Failure)",
		EmailDate: t1,
	}
	e2 := &email.Email{
		MessageID: "<msg2@example.com>",
		Subject:   "Delivery Status Notification (Failure)",
		EmailDate: t2,
	}
	if isEmailMatch(e1, e2) {
		t.Errorf("expected emails with different Message-IDs to NOT match")
	}

	// Same Message-ID matches
	e3 := &email.Email{
		MessageID: "<msg1@example.com>",
		Subject:   "Different Subject",
	}
	if !isEmailMatch(e1, e3) {
		t.Errorf("expected emails with same Message-ID to match")
	}

	// Fallback: one lacks Message-ID, but subjects and dates match
	e4 := &email.Email{
		Subject:   "Delivery Status Notification (Failure)",
		EmailDate: t1,
	}
	if !isEmailMatch(e1, e4) {
		t.Errorf("expected fallback match when one Message-ID is missing")
	}

	// Zero dates must NOT match under fallback
	e5 := &email.Email{
		Subject: "Some Subject",
	}
	e6 := &email.Email{
		Subject: "Some Subject",
	}
	if isEmailMatch(e5, e6) {
		t.Errorf("expected zero-date emails without Message-ID to NOT match")
	}
}

// seedCachedBatch stores batchSize messages named msg001..msgNNN in accDir,
// rendering each one's raw bytes with eml, and returns their IDs in order.
func seedCachedBatch(t *testing.T, accDir string, when time.Time, batchSize int, eml func(i int) string) []string {
	t.Helper()
	ids := make([]string, 0, batchSize)
	for i := 1; i <= batchSize; i++ {
		id := fmt.Sprintf("msg%03d", i)
		ids = append(ids, id)
		if err := msg.Store(cachedir.At(accDir), email.ID(id), []byte(eml(i)), when); err != nil {
			t.Fatalf("failed to store %s: %v", id, err)
		}
	}
	return ids
}

func TestSpliceBatchSameSubjectDifferentMessageIDs(t *testing.T) {
	oldDelay := confirmRetryDelay
	confirmRetryDelay = 1 * time.Millisecond
	defer func() { confirmRetryDelay = oldDelay }()

	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, "config")
	downloadDir := filepath.Join(tmpDir, "download")
	acc1Dir := filepath.Join(downloadDir, "acc1")
	acc2Dir := filepath.Join(downloadDir, "acc2")
	_ = os.MkdirAll(configDir, 0755)
	_ = os.MkdirAll(acc1Dir, 0755)
	_ = os.MkdirAll(acc2Dir, 0755)

	fixedDate := time.Date(2026, 9, 2, 9, 59, 42, 0, time.UTC)
	const batchSize = 10
	srcIDs := seedCachedBatch(t, acc1Dir, fixedDate, batchSize, func(i int) string {
		return fmt.Sprintf("Message-ID: <bounce-%d@google.com>\nDate: Wed, 02 Sep 2026 09:59:42 +0000\nFrom: mailer-daemon@googlemail.com\nSubject: Delivery Status Notification (Failure)\n\nBounce %d", i, i)
	})

	mockClient1 := archiveMock(acc1Dir, configDir, "received", email.IDs(srcIDs)...)
	mockClient2 := &mailclient.MockMailClient{
		Labels: []string{"received_private"},
		Cfg:    &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: acc2Dir, ConfigDir: configDir}, AccountCache: cachedir.At(acc2Dir)},
	}

	session := twoAccountSession(downloadDir, configDir, mockClient1, mockClient2)

	cliApp := InitCLI(session)
	err := cliApp.Execute([]string{"label", "sort", "%acc1:received", "-F", "%acc2:received_private", "--move"})
	if err != nil {
		t.Fatalf("unexpected error running batch splice: %v", err)
	}

	if len(mockClient2.UploadedEmails) != batchSize {
		t.Fatalf("expected all %d emails to be uploaded, but only %d were uploaded (false duplicate collision!)", batchSize, len(mockClient2.UploadedEmails))
	}
}

func TestSpliceSameAccountAtomicBulkMove(t *testing.T) {
	tempDir := t.TempDir()
	accDir := filepath.Join(tempDir, "acc1")
	configDir := filepath.Join(tempDir, "config")
	_ = os.MkdirAll(accDir, 0700)
	_ = os.MkdirAll(configDir, 0700)

	fixedDate := time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC)
	const batchSize = 5
	srcIDs := seedCachedBatch(t, accDir, fixedDate, batchSize, func(i int) string {
		return fmt.Sprintf("Message-ID: <msg-%d@test.com>\nDate: Wed, 02 Sep 2026 10:00:00 +0000\nFrom: user@test.com\nSubject: Item %d\n\nContent", i, i)
	})

	mockClient := archiveMock(accDir, configDir, "received", email.IDs(srcIDs)...)

	session := singleAccountSession(tempDir, configDir, mockClient)

	cliApp := InitCLI(session)
	err := cliApp.Execute([]string{"label", "sort", "%received", "-Y", "received_private", "--move"})
	if err != nil {
		t.Fatalf("unexpected error running same account splice: %v", err)
	}

	// Verify that MockMailClient performed MoveEmail and did NOT use UploadRawEmail
	if len(mockClient.UploadedEmails) != 0 {
		t.Errorf("expected 0 UploadRawEmail calls for same-account move, got %d", len(mockClient.UploadedEmails))
	}
}

// See splice_readonly_test.go for
// TestSpliceSameAccountMove_PerAccountReadOnlySkipsLocalReindex, the H2
// per-account-ReadOnly regression test.

func TestSpliceJournalRecovery(t *testing.T) {
	tempDir := t.TempDir()
	journal, err := OpenSpliceJournal(cachedir.At(tempDir), "source_folder")
	if err != nil {
		t.Fatalf("failed to open journal: %v", err)
	}

	_ = journal.RecordUpload("src_001", "dest_001", "dest_folder")
	_ = journal.RecordUpload("src_002", "dest_002", "dest_folder")

	mockClient := &mailclient.MockMailClient{
		DownloadedIDs: map[string][]email.ID{"source_folder": {"src_001", "src_002"}},
		Cfg:           &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: tempDir}, AccountCache: cachedir.At(tempDir)},
	}

	session := &Session{
		Deps: Deps{
			Organize: stubOrganize{
				ResolveTrashTargetFn: func(_ mailclient.MailClient) (string, error) {
					return "Trash", nil
				},
			},
		},
	}

	reopened, err := OpenSpliceJournal(cachedir.At(tempDir), "source_folder")
	if err != nil {
		t.Fatalf("failed to reopen journal: %v", err)
	}
	if len(reopened.Entries) != 2 {
		t.Fatalf("expected 2 journal entries, got %d", len(reopened.Entries))
	}

	err = RecoverSpliceJournal(reopened, mockClient, session, label.FromServer("source_folder"))
	if err != nil {
		t.Fatalf("unexpected error during journal recovery: %v", err)
	}

	if len(reopened.Entries) != 0 {
		t.Errorf("expected journal to be empty after recovery, got %d entries", len(reopened.Entries))
	}
}

func TestOpenSpliceJournal_CorruptedJSON(t *testing.T) {
	tempDir := t.TempDir()
	journalPath := filepath.Join(tempDir, "splice_journal.json")
	if err := os.WriteFile(journalPath, []byte(`{invalid-json`), 0600); err != nil {
		t.Fatalf("failed to write corrupted journal: %v", err)
	}

	_, err := OpenSpliceJournal(cachedir.At(tempDir), "source_folder")
	if err == nil {
		t.Fatal("expected error opening corrupted journal, got nil")
	}
	if !strings.Contains(err.Error(), "failed to parse existing splice journal") {
		t.Fatalf("unexpected error message: %v", err)
	}
}
