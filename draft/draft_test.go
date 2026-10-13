package draft

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRootFollowsCacheRoot(t *testing.T) {
	if got := Root("/x/cache"); got != filepath.Join("/x/cache", "drafts") {
		t.Errorf("Root = %q", got)
	}
	if got := Root(""); !strings.HasSuffix(got, filepath.Join(".cache", "mail_cli", "drafts")) {
		t.Errorf("fallback Root = %q", got)
	}
}

func TestCreateNumbersWithinTheDay(t *testing.T) {
	s := Store{Root: t.TempDir()}
	day := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)

	p1, err := s.Create(day, []byte("one"))
	if err != nil {
		t.Fatal(err)
	}
	p2, err := s.Create(day.Add(time.Hour), []byte("two"))
	if err != nil {
		t.Fatal(err)
	}
	p3, err := s.Create(day.AddDate(0, 0, 1), []byte("three"))
	if err != nil {
		t.Fatal(err)
	}

	want := func(p, rel string) {
		t.Helper()
		if p != filepath.Join(s.Root, filepath.FromSlash(rel), FileName) {
			t.Errorf("got %q, want %s", p, rel)
		}
	}
	want(p1, "2026/09/30/1")
	want(p2, "2026/09/30/2")
	want(p3, "2026/10/01/1")

	info, err := os.Stat(filepath.Dir(p1))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0077 != 0 {
		t.Errorf("draft directory is %o, want private", info.Mode().Perm())
	}
}

func TestListNewestFirstWithIDs(t *testing.T) {
	s := Store{Root: t.TempDir()}
	day := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	older, _ := s.Create(day, []byte("old"))
	newer, _ := s.Create(day, []byte("new"))
	past := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(older, past, past); err != nil {
		t.Fatal(err)
	}

	entries, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Path != newer || entries[1].Path != older {
		t.Fatalf("entries = %+v", entries)
	}
	if entries[0].ID != "2026/09/30/2" || entries[1].ID != "2026/09/30/1" {
		t.Errorf("ids = %q, %q", entries[0].ID, entries[1].ID)
	}
	if entries[0].Size != 3 {
		t.Errorf("size = %d", entries[0].Size)
	}
}

func TestListOfMissingRootIsEmpty(t *testing.T) {
	s := Store{Root: filepath.Join(t.TempDir(), "nope")}
	entries, err := s.List()
	if err != nil || len(entries) != 0 {
		t.Fatalf("got %v, %v", entries, err)
	}
}

func TestResolveStaysUnderRoot(t *testing.T) {
	s := Store{Root: t.TempDir()}
	p, _ := s.Create(time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC), []byte("x"))

	got, err := s.Resolve(" 2026/09/30/1 ")
	if err != nil || got != p {
		t.Errorf("Resolve = %q, %v; want %q", got, err, p)
	}
	for _, bad := range []string{"../../etc", "/etc/passwd", "", ".", "2026/09/30/9"} {
		if _, err := s.Resolve(bad); err == nil {
			t.Errorf("Resolve(%q) succeeded", bad)
		}
	}
}

func TestRemoveClearsEmptyDirectory(t *testing.T) {
	s := Store{Root: t.TempDir()}
	p, _ := s.Create(time.Now(), []byte("x"))
	if err := Remove(p); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Dir(p)); !os.IsNotExist(err) {
		t.Errorf("directory still there: %v", err)
	}
	if err := Remove(p); err != nil {
		t.Errorf("second Remove: %v", err)
	}
}

// Two sessions that both read the highest number used to get the same N,
// and the second write replaced the first draft.
func TestCreateNeverReusesAClaimedDirectory(t *testing.T) {
	s := Store{Root: t.TempDir()}
	day := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	dayDir := filepath.Join(s.Root, "2026", "09", "30")
	if err := os.MkdirAll(filepath.Join(dayDir, "1"), 0700); err != nil {
		t.Fatal(err)
	}
	// A concurrent session claimed 2 after this one's nextCounter read 1.
	dir, err := claimDir(dayDir, 1)
	if err != nil || dir != filepath.Join(dayDir, "2") {
		t.Fatalf("claimDir = %q, %v", dir, err)
	}
	p, err := s.Create(day, []byte("mine"))
	if err != nil || p != filepath.Join(dayDir, "3", FileName) {
		t.Fatalf("Create = %q, %v; want slot 3", p, err)
	}
}

func TestWriteReplacesAtomicallyAndLeavesNoTemp(t *testing.T) {
	s := Store{Root: t.TempDir()}
	p, err := s.Create(time.Now(), []byte("first"))
	if err != nil {
		t.Fatal(err)
	}
	if err := Write(p, []byte("second")); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(p)
	if string(got) != "second" {
		t.Errorf("content = %q", got)
	}
	info, _ := os.Stat(p)
	if info.Mode().Perm() != 0600 {
		t.Errorf("mode = %o", info.Mode().Perm())
	}
	if left, _ := filepath.Glob(filepath.Join(filepath.Dir(p), ".*")); len(left) != 0 {
		t.Errorf("temporary files left: %v", left)
	}
}

// A write that cannot complete must leave the previous draft intact.
func TestWriteFailureKeepsPreviousContent(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	s := Store{Root: t.TempDir()}
	p, err := s.Create(time.Now(), []byte("keep me"))
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Dir(p)
	if err := os.Chmod(dir, 0500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(dir, 0700)
	if err := Write(p, []byte("lost")); err == nil {
		t.Fatal("Write into a read-only directory succeeded")
	}
	got, _ := os.ReadFile(p)
	if string(got) != "keep me" {
		t.Errorf("content = %q, want the previous draft", got)
	}
}

func TestRemoveClearsInterruptedWriteLeftovers(t *testing.T) {
	s := Store{Root: t.TempDir()}
	p, _ := s.Create(time.Now(), []byte("x"))
	if err := os.WriteFile(filepath.Join(filepath.Dir(p), "."+FileName+".tmp-123"), []byte("partial"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Remove(p); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Dir(p)); !os.IsNotExist(err) {
		t.Errorf("draft directory still there: %v", err)
	}
}

func TestResolveAcceptsDotPrefixedNamesInsideRoot(t *testing.T) {
	s := Store{Root: t.TempDir()}
	if err := os.MkdirAll(filepath.Join(s.Root, "..x"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.Root, "..x", FileName), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Resolve("..x"); err != nil {
		t.Errorf("Resolve(..x) = %v; it is inside the root", err)
	}
}

// Discarding a day's last draft removed its numbered directory and left the
// day, month and year behind; the root itself always stays, and a date with
// another draft in it is kept.
func TestRemoveClearsEmptyDateDirectories(t *testing.T) {
	root := t.TempDir()
	s := Store{Root: root}
	day := time.Date(2026, 10, 7, 12, 0, 0, 0, time.Local)
	other := time.Date(2026, 9, 1, 12, 0, 0, 0, time.Local)
	p, _ := s.Create(day, []byte("x"))
	keep, _ := s.Create(other, []byte("y"))

	if err := Remove(p); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "2026", "10")); !os.IsNotExist(err) {
		t.Errorf("the empty month is still there: %v", err)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Errorf("another day's draft went with it: %v", err)
	}

	if err := Remove(keep); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Errorf("root: %v entries, err %v; want it empty and still there", len(entries), err)
	}
}
