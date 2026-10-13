package cli

import (
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sarielhp/mail_cli/cache/msg"
	"github.com/sarielhp/mail_cli/cache/virtual"
	"github.com/sarielhp/mail_cli/email"
)

func searchTestEmail(id, subject string, date time.Time) email.Email {
	return email.Email{ID: email.ID(id), Subject: subject, EmailDate: date}
}

func day(n int) time.Time {
	return time.Date(2025, 1, n, 0, 0, 0, 0, time.UTC)
}

// Listings run oldest first, so the newest match ends up next to the prompt.
func TestSortSearchEmailsOldestFirst(t *testing.T) {
	flagSearchReverse = false
	t.Cleanup(func() { flagSearchReverse = false })

	emails := []email.Email{
		searchTestEmail("b", "middle", day(15)),
		searchTestEmail("c", "newest", day(30)),
		searchTestEmail("a", "oldest", day(1)),
	}
	sortSearchEmails(emails)

	want := []string{"a", "b", "c"}
	for i, id := range want {
		if string(emails[i].ID) != id {
			t.Fatalf("order = %s, want %s", idsOf(emails), strings.Join(want, ""))
		}
	}
}

func TestSortSearchEmailsReversed(t *testing.T) {
	flagSearchReverse = true
	t.Cleanup(func() { flagSearchReverse = false })

	emails := []email.Email{
		searchTestEmail("a", "oldest", day(1)),
		searchTestEmail("c", "newest", day(30)),
		searchTestEmail("b", "middle", day(15)),
	}
	sortSearchEmails(emails)

	if emails[0].ID != "c" || emails[2].ID != "a" {
		t.Errorf("order = %s, want cba", idsOf(emails))
	}
}

func idsOf(emails []email.Email) string {
	var b strings.Builder
	for _, e := range emails {
		b.WriteString(string(e.ID))
	}
	return b.String()
}

func seedSearchMessage(t *testing.T, dir, id, subject string, date time.Time) {
	t.Helper()
	body := "From: alice@example.com\r\nTo: bob@example.com\r\nSubject: " + subject +
		"\r\nDate: " + date.Format(time.RFC1123Z) +
		"\r\nMessage-ID: <" + id + "@example.com>\r\n\r\n" +
		"body\r\nFrom the top this line must be escaped\r\n"
	if err := msg.Store(cachedir.At(dir), email.ID(id), []byte(body), date); err != nil {
		t.Fatalf("seeding %s: %v", id, err)
	}
}

func TestWriteSearchMboxOrdersAndEscapes(t *testing.T) {
	cacheDir := t.TempDir()
	seedSearchMessage(t, cacheDir, "m1", "first", day(1))
	seedSearchMessage(t, cacheDir, "m2", "second", day(2))

	dest := filepath.Join(t.TempDir(), "out.mbox")
	written, err := writeSearchMbox(cachedir.At(cacheDir), dest, []email.ID{"m1", "m2"})
	if err != nil {
		t.Fatalf("writeSearchMbox: %v", err)
	}
	if written != 2 {
		t.Fatalf("wrote %d messages, want 2", written)
	}

	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("reading archive: %v", err)
	}
	text := string(data)

	if strings.Index(text, "Subject: first") > strings.Index(text, "Subject: second") {
		t.Error("messages were not written in the order given")
	}
	// A body line starting with "From " must be escaped, or it reads as the
	// start of the next message and the archive silently gains a message.
	if !strings.Contains(text, ">From the top this line must be escaped") {
		t.Error("a body line beginning with \"From \" was not escaped")
	}
	// Count envelope lines properly: the first is at the start of the file, so
	// it has no newline before it, and escaped body lines begin ">From ".
	var envelopes int
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "From ") {
			envelopes++
		}
	}
	if envelopes != 2 {
		t.Errorf("archive has %d envelope lines, want 2", envelopes)
	}
}

// A failed write must not leave a partial archive where the real one was.
func TestWriteSearchMboxLeavesNoTempOnSuccess(t *testing.T) {
	cacheDir := t.TempDir()
	seedSearchMessage(t, cacheDir, "m1", "only", day(1))

	destDir := t.TempDir()
	dest := filepath.Join(destDir, "out.mbox")
	if _, err := writeSearchMbox(cachedir.At(cacheDir), dest, []email.ID{"m1"}); err != nil {
		t.Fatalf("writeSearchMbox: %v", err)
	}

	entries, err := os.ReadDir(destDir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("directory holds %d files, want only the archive", len(entries))
	}
}

// Messages that are not cached are skipped rather than aborting the export.
func TestWriteSearchMboxSkipsMissingMessages(t *testing.T) {
	cacheDir := t.TempDir()
	seedSearchMessage(t, cacheDir, "present", "here", day(1))

	dest := filepath.Join(t.TempDir(), "out.mbox")
	written, err := writeSearchMbox(cachedir.At(cacheDir), dest, []email.ID{"present", "absent"})
	if err != nil {
		t.Fatalf("writeSearchMbox: %v", err)
	}
	if written != 1 {
		t.Errorf("wrote %d messages, want 1", written)
	}
}

// The result set is saved so `show <id>` and the TUI can walk it afterwards.
func TestSaveSearchMailboxRecordsResults(t *testing.T) {
	dir := t.TempDir()
	client := &stubSearchClient{downloadDir: dir}

	emails := []email.Email{
		searchTestEmail("a", "one", day(1)),
		searchTestEmail("b", "two", day(2)),
	}
	saveSearchMailbox(client, emails, "taxes")

	vm, err := virtual.Load(cachedir.At(dir), "search")
	if err != nil {
		t.Fatalf("the search result set was not saved: %v", err)
	}
	if len(vm.MessageIDs) != 2 || vm.MessageIDs[0] != "a" {
		t.Errorf("saved IDs = %v, want [a b]", vm.MessageIDs)
	}
	if !strings.Contains(vm.Description, "taxes") {
		t.Errorf("description %q does not name the query", vm.Description)
	}
}
