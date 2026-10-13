package train

import (
	"encoding/json"
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	"github.com/sarielhp/mail_cli/cache/dirlock"
)

func ledgerPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "trained.json")
}

func TestOpenMissingFileIsEmpty(t *testing.T) {
	l, err := Open(ledgerPath(t))
	if err != nil {
		t.Fatalf("Open on a missing file = %v, want nil: a first run has no ledger", err)
	}
	if l.Len() != 0 {
		t.Errorf("Len = %d, want 0", l.Len())
	}
}

// Both historical shapes have to load, or upgrading loses the record of what
// has been trained and every message is trained a second time.
func TestOpenReadsBothLegacyShapes(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    []string
	}{
		{
			name:    "list shape",
			content: `["id1","id2","id3"]`,
			want:    []string{"id1", "id2", "id3"},
		},
		{
			name:    "map shape",
			content: `{"id1":true,"id2":true}`,
			want:    []string{"id1", "id2"},
		},
		{
			name:    "map shape with a false entry",
			content: `{"id1":true,"id2":false}`,
			want:    []string{"id1"},
		},
		{name: "empty list", content: `[]`, want: nil},
		{name: "empty object", content: `{}`, want: nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := ledgerPath(t)
			if err := os.WriteFile(path, []byte(tc.content), 0600); err != nil {
				t.Fatalf("seeding: %v", err)
			}

			l, err := Open(path)
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			got := l.IDs()
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for _, id := range tc.want {
				if !l.Has(id) {
					t.Errorf("%q is not recorded", id)
				}
			}
		})
	}
}

// A damaged ledger must stop the caller rather than read as "nothing trained".
// The previous loader returned an empty map on any parse failure, so handing it
// the map-shaped file silently discarded the record.
func TestOpenFailsClosedOnDamagedFile(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{name: "not json", content: "this is not json at all"},
		{name: "truncated", content: `["id1","id2`},
		{name: "wrong element type", content: `[1,2,3]`},
		{name: "nested", content: `{"a":{"b":true}}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := ledgerPath(t)
			if err := os.WriteFile(path, []byte(tc.content), 0600); err != nil {
				t.Fatalf("seeding: %v", err)
			}
			if l, err := Open(path); err == nil {
				t.Errorf("Open accepted a damaged ledger as %d entries; it must fail closed", l.Len())
			}
		})
	}
}

func TestOpenEmptyFileIsEmptyLedger(t *testing.T) {
	path := ledgerPath(t)
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatalf("seeding: %v", err)
	}
	l, err := Open(path)
	if err != nil {
		t.Fatalf("Open on an empty file = %v, want nil", err)
	}
	if l.Len() != 0 {
		t.Errorf("Len = %d, want 0", l.Len())
	}
}

func TestAddAndRemove(t *testing.T) {
	path := ledgerPath(t)
	l, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	added, err := l.Add("id1")
	if err != nil || !added {
		t.Fatalf("Add = %v, %v; want true, nil", added, err)
	}
	// Adding twice must not double-record or rewrite.
	if again, addErr := l.Add("id1"); addErr != nil || again {
		t.Errorf("second Add = %v, %v; want false, nil", again, addErr)
	}
	if !l.Has("id1") {
		t.Error("id1 is not recorded after Add")
	}

	removed, err := l.Remove("id1")
	if err != nil || !removed {
		t.Fatalf("Remove = %v, %v; want true, nil", removed, err)
	}
	if again, err := l.Remove("id1"); err != nil || again {
		t.Errorf("second Remove = %v, %v; want false, nil", again, err)
	}
	if l.Has("id1") {
		t.Error("id1 is still recorded after Remove")
	}
}

func TestAddIgnoresEmptyID(t *testing.T) {
	l, err := Open(ledgerPath(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if added, err := l.Add(""); err != nil || added {
		t.Errorf("Add(\"\") = %v, %v; want false, nil", added, err)
	}
}

func TestChangesPersist(t *testing.T) {
	path := ledgerPath(t)
	l, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	for _, id := range []string{"b", "a", "c"} {
		if _, addErr := l.Add(id); addErr != nil {
			t.Fatalf("Add(%s): %v", id, addErr)
		}
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("reopening: %v", err)
	}
	want := []string{"a", "b", "c"}
	if got := reopened.IDs(); !slices.Equal(got, want) {
		t.Errorf("reopened ledger holds %v, want %v in sorted order", got, want)
	}
}

// A map-shaped file is rewritten as a list once touched, so the two formats
// converge instead of needing two code paths forever.
func TestSaveNormalizesTheLegacyMapShape(t *testing.T) {
	path := ledgerPath(t)
	if err := os.WriteFile(path, []byte(`{"id1":true,"id2":true}`), 0600); err != nil {
		t.Fatalf("seeding: %v", err)
	}

	l, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, addErr := l.Add("id3"); addErr != nil {
		t.Fatalf("Add: %v", addErr)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading back: %v", err)
	}
	var list []string
	if err := json.Unmarshal(data, &list); err != nil {
		t.Fatalf("the saved ledger is not a list: %v\n%s", err, data)
	}
	if len(list) != 3 {
		t.Errorf("saved %v, want the two original IDs plus the new one", list)
	}
}

// Removing from a damaged ledger must not fail the cleanup it is part of.
func TestOpenOrEmptyToleratesDamage(t *testing.T) {
	path := ledgerPath(t)
	if err := os.WriteFile(path, []byte("garbage"), 0600); err != nil {
		t.Fatalf("seeding: %v", err)
	}

	l := OpenOrEmpty(path)
	if l == nil {
		t.Fatal("OpenOrEmpty returned nil")
	}
	if removed, err := l.Remove("anything"); err != nil || removed {
		t.Errorf("Remove on a damaged ledger = %v, %v; want false, nil", removed, err)
	}
}

// A truncated write would read back as "nothing trained", so the save is
// atomic and leaves no temp file behind. Add now also takes the
// cross-process directory lock (M8), which leaves its own persistent
// ".cache.lock" file behind by design -- dirlock.IsLockFile is how the rest
// of the codebase recognizes and ignores it, so this test does the same
// rather than special-casing the name itself.
func TestSaveIsAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "trained.json")

	l, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, addErr := l.Add("id1"); addErr != nil {
		t.Fatalf("Add: %v", addErr)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	var names []string
	for _, e := range entries {
		if dirlock.IsLockFile(e.Name()) {
			continue
		}
		names = append(names, e.Name())
	}
	if len(names) != 1 {
		t.Errorf("directory holds %v (besides the lock file), want only the ledger", names)
	}
}

func TestConcurrentAdds(t *testing.T) {
	path := ledgerPath(t)
	l, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	var wg sync.WaitGroup
	for w := range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range 10 {
				if _, err := l.Add(string(rune('a'+w)) + string(rune('0'+i))); err != nil {
					t.Errorf("Add: %v", err)
					return
				}
			}
		}()
	}
	wg.Wait()

	if l.Len() != 40 {
		t.Errorf("ledger holds %d IDs after concurrent adds, want 40", l.Len())
	}
}

// TestTwoProcessesAddingDifferentIDsBothSurvive is M8's regression: two
// *Ledger values over the same path, each standing in for a separate
// process (a long-running TUI training spam alongside a scheduled cron
// scan, the pattern the package doc names), each Open the ledger once and
// then each Add a different ID. Without merging under the cross-process
// lock, the later save's os.Rename replaces the earlier process's write
// wholesale and its ID is silently gone; both must survive here.
func TestTwoProcessesAddingDifferentIDsBothSurvive(t *testing.T) {
	path := ledgerPath(t)

	processA, err := Open(path)
	if err != nil {
		t.Fatalf("Open (process A): %v", err)
	}
	processB, err := Open(path)
	if err != nil {
		t.Fatalf("Open (process B): %v", err)
	}

	if _, addErrA := processA.Add("from-a"); addErrA != nil {
		t.Fatalf("process A Add: %v", addErrA)
	}
	// process B's in-memory copy was taken before A's write and still knows
	// nothing about "from-a"; its own Add must not discard it regardless.
	if _, addErrB := processB.Add("from-b"); addErrB != nil {
		t.Fatalf("process B Add: %v", addErrB)
	}

	reread, err := Open(path)
	if err != nil {
		t.Fatalf("re-open: %v", err)
	}
	if !reread.Has("from-a") {
		t.Error("process A's add was lost")
	}
	if !reread.Has("from-b") {
		t.Error("process B's add was lost")
	}
	if reread.Len() != 2 {
		t.Errorf("expected 2 IDs on disk after both processes added, got %d: %v", reread.Len(), reread.IDs())
	}
}

func TestLedgerPaths(t *testing.T) {
	if got := MessageIDPath("/cfg"); got != "/cfg/trained_message_ids.json" {
		t.Errorf("MessageIDPath = %q", got)
	}
	if got := SpamUIDPath(cachedir.At("/dl")); got != "/dl/trained_uids.json" {
		t.Errorf("SpamUIDPath = %q", got)
	}
	if got := HamPath(cachedir.At("/dl")); got != "/dl/trained_uids_ham.json" {
		t.Errorf("HamPath = %q", got)
	}
}

func TestAdd_RollbackOnSaveFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "trained.json")
	l, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	// Make directory read-only so CreateTemp in saveLocked fails
	if chmodErr := os.Chmod(dir, 0500); chmodErr != nil {
		t.Fatalf("Chmod 0500: %v", chmodErr)
	}
	defer func() { _ = os.Chmod(dir, 0700) }()

	added, err := l.Add("msg-err")
	if err == nil {
		t.Fatal("expected error on read-only directory, got nil")
	}
	if added {
		t.Error("expected added = false on save failure, got true")
	}
	if l.Has("msg-err") {
		t.Error("expected in-memory map to roll back on save failure")
	}

	// Restore permissions and retry
	if chmodErr := os.Chmod(dir, 0700); chmodErr != nil {
		t.Fatalf("Chmod 0700: %v", chmodErr)
	}

	added, err = l.Add("msg-err")
	if err != nil {
		t.Fatalf("retry Add failed: %v", err)
	}
	if !added {
		t.Error("expected retry Add to report added = true")
	}
	if !l.Has("msg-err") {
		t.Error("expected msg-err to be recorded after retry")
	}
}

func TestRemove_RollbackOnSaveFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "trained.json")
	l, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	if _, addErr := l.Add("msg-remove"); addErr != nil {
		t.Fatalf("Add: %v", addErr)
	}

	// Make directory read-only
	if chmodErr := os.Chmod(dir, 0500); chmodErr != nil {
		t.Fatalf("Chmod 0500: %v", chmodErr)
	}
	defer func() { _ = os.Chmod(dir, 0700) }()

	removed, err := l.Remove("msg-remove")
	if err == nil {
		t.Fatal("expected error on read-only directory, got nil")
	}
	if removed {
		t.Error("expected removed = false on save failure, got true")
	}
	if !l.Has("msg-remove") {
		t.Error("expected in-memory map to roll back and retain msg-remove on save failure")
	}

	// Restore permissions and retry
	if chmodErr := os.Chmod(dir, 0700); chmodErr != nil {
		t.Fatalf("Chmod 0700: %v", chmodErr)
	}

	removed, err = l.Remove("msg-remove")
	if err != nil {
		t.Fatalf("retry Remove failed: %v", err)
	}
	if !removed {
		t.Error("expected retry Remove to report removed = true")
	}
	if l.Has("msg-remove") {
		t.Error("expected msg-remove to be forgotten after retry")
	}
}

func TestAddAll_BatchInsertAndRollback(t *testing.T) {
	path := ledgerPath(t)
	l, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	// 1. Batch insert multiple IDs
	ids := []string{"id-1", "id-2", "id-3", "", "id-1"}
	if addAllErr := l.AddAll(ids); addAllErr != nil {
		t.Fatalf("AddAll: %v", addAllErr)
	}

	if l.Len() != 3 {
		t.Errorf("expected 3 IDs in ledger, got %d", l.Len())
	}
	for _, id := range []string{"id-1", "id-2", "id-3"} {
		if !l.Has(id) {
			t.Errorf("expected ledger to contain %s", id)
		}
	}

	// 2. Re-open and verify disk persistence
	l2, err := Open(path)
	if err != nil {
		t.Fatalf("re-open: %v", err)
	}
	if l2.Len() != 3 {
		t.Errorf("expected 3 IDs after re-open, got %d", l2.Len())
	}

	// 3. Calling AddAll with already present IDs is a no-op
	if err := l.AddAll([]string{"id-1", "id-2"}); err != nil {
		t.Fatalf("AddAll no-op: %v", err)
	}
	if l.Len() != 3 {
		t.Errorf("expected 3 IDs, got %d", l.Len())
	}
}
