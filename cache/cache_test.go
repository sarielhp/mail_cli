package cache

import (
	"fmt"
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"github.com/sarielhp/mail_cli/email"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/sarielhp/mail_cli/cache/msg"
	"github.com/sarielhp/mail_cli/cfg_acc"
)

// The top-level cache holds read and replied state and the label-item cache.
// It had no tests, and show <id> reaches FindCachedEmailByID on every
// single-argument invocation.

const sampleRaw = "From: alice@example.com\r\n" +
	"Subject: hello\r\n" +
	"Date: Tue, 02 Sep 2025 10:04:05 +0000\r\n\r\nbody\r\n"

func TestReadStateRoundTrip(t *testing.T) {
	dir := t.TempDir()

	if got := LoadReadState(cachedir.At(dir)); len(got) != 0 {
		t.Errorf("a fresh directory reports %d read messages, want none", len(got))
	}

	state := map[email.ID]bool{"a": true, "b": false, "c": true}
	if err := saveReadState(dir, state); err != nil {
		t.Fatalf("saveReadState: %v", err)
	}

	got := LoadReadState(cachedir.At(dir))
	if len(got) != len(state) {
		t.Fatalf("loaded %d entries, want %d", len(got), len(state))
	}
	for id, want := range state {
		if got[id] != want {
			t.Errorf("read state for %q = %v, want %v", id, got[id], want)
		}
	}
}

// Unreadable or absent state must come back as an empty map rather than nil,
// so callers can assign into it without checking.
func TestLoadReadStateOfMissingDirIsUsable(t *testing.T) {
	got := LoadReadState(cachedir.At(filepath.Join(t.TempDir(), "nonexistent")))
	if got == nil {
		t.Fatal("LoadReadState returned nil; callers write into the result")
	}
	got["x"] = true
}

func TestMarkIDsRead(t *testing.T) {
	dir := t.TempDir()
	state := map[email.ID]bool{"old": true}

	if err := MarkIDsRead(cachedir.At(dir), []email.ID{"n1", "n2"}, state); err != nil {
		t.Fatalf("MarkIDsRead: %v", err)
	}

	reloaded := LoadReadState(cachedir.At(dir))
	for _, id := range []email.ID{"old", "n1", "n2"} {
		if !reloaded[id] {
			t.Errorf("%q is not marked read after MarkIDsRead", id)
		}
	}
}

func TestRepliedStateIsSeparateFromReadState(t *testing.T) {
	dir := t.TempDir()

	if err := saveReadState(dir, map[email.ID]bool{"m1": true}); err != nil {
		t.Fatalf("saveReadState: %v", err)
	}
	if err := saveRepliedState(dir, map[email.ID]bool{"m2": true}); err != nil {
		t.Fatalf("saveRepliedState: %v", err)
	}

	// A message being read says nothing about whether it was replied to.
	if LoadRepliedState(cachedir.At(dir))["m1"] {
		t.Error("read state leaked into replied state")
	}
	if LoadReadState(cachedir.At(dir))["m2"] {
		t.Error("replied state leaked into read state")
	}
	if !LoadRepliedState(cachedir.At(dir))["m2"] {
		t.Error("replied state did not persist")
	}
}

func TestMarkIDsReplied(t *testing.T) {
	dir := t.TempDir()
	if err := MarkIDsReplied(cachedir.At(dir), []email.ID{"r1"}, map[email.ID]bool{}); err != nil {
		t.Fatalf("MarkIDsReplied: %v", err)
	}
	if !LoadRepliedState(cachedir.At(dir))["r1"] {
		t.Error("r1 is not marked replied")
	}
}

// Two writers share one state file; this is what the directory lock is for.
func TestConcurrentReadStateWrites(t *testing.T) {
	dir := t.TempDir()

	var wg sync.WaitGroup
	for w := range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range 10 {
				id := string(rune('a'+w)) + string(rune('0'+i))
				if err := MarkIDsRead(cachedir.At(dir), []email.ID{email.ID(id)}, LoadReadState(cachedir.At(dir))); err != nil {
					t.Errorf("MarkIDsRead: %v", err)
					return
				}
			}
		}()
	}
	wg.Wait()

	if len(LoadReadState(cachedir.At(dir))) == 0 {
		t.Error("no read state survived concurrent writes")
	}
}

func TestFindCachedEmailByID(t *testing.T) {
	dir := t.TempDir()
	const id = "19f3a8216d52dfc6"
	if err := msg.Store(cachedir.At(dir), id, []byte(sampleRaw), time.Now()); err != nil {
		t.Fatalf("seeding cache: %v", err)
	}

	got, _, err := FindCachedEmailByID(cachedir.At(dir), id)
	if err != nil {
		t.Fatalf("FindCachedEmailByID: %v", err)
	}
	if got != id {
		t.Errorf("found %q, want %q", got, id)
	}
}

// Listings print a short ID, so that has to be a way back to the message.
func TestFindCachedEmailByShortID(t *testing.T) {
	dir := t.TempDir()
	const id = "19f3a8216d52dfc6"
	if err := msg.Store(cachedir.At(dir), id, []byte(sampleRaw), time.Now()); err != nil {
		t.Fatalf("seeding cache: %v", err)
	}

	short := shortIDOf(id)
	got, _, err := FindCachedEmailByID(cachedir.At(dir), short)
	if err != nil {
		t.Fatalf("FindCachedEmailByID(%q): %v", short, err)
	}
	if got != id {
		t.Errorf("short ID %q resolved to %q, want %q", short, got, id)
	}
}

func TestFindCachedEmailByIDReportsTheFolder(t *testing.T) {
	dir := t.TempDir()
	const id = "abc123def456"
	if err := msg.Store(cachedir.At(dir), id, []byte(sampleRaw), time.Now()); err != nil {
		t.Fatalf("seeding cache: %v", err)
	}

	idxDir := filepath.Join(dir, "indexes")
	if err := os.MkdirAll(idxDir, 0700); err != nil {
		t.Fatalf("creating index dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(idxDir, "Receipts.json"), []byte(`["`+id+`"]`), 0600); err != nil {
		t.Fatalf("writing folder index: %v", err)
	}

	got, folder, err := FindCachedEmailByID(cachedir.At(dir), id)
	if err != nil {
		t.Fatalf("FindCachedEmailByID: %v", err)
	}
	if got != id {
		t.Errorf("found %q, want %q", got, id)
	}
	if folder != "Receipts" {
		t.Errorf("folder = %q, want Receipts", folder)
	}
}

func TestFindCachedEmailByIDUnknown(t *testing.T) {
	if _, _, err := FindCachedEmailByID(cachedir.At(t.TempDir()), "nosuchmessage"); err == nil {
		t.Error("FindCachedEmailByID found a message that was never cached")
	}
}

func TestDiskCacheStoreLabelItems(t *testing.T) {
	store := &DiskCacheStore{Dir: cachedir.At(t.TempDir())}

	if _, err := store.GetLabelItems(); err == nil {
		t.Error("GetLabelItems succeeded before anything was saved")
	}

	items := []cfg_acc.Item{
		{Name: "INBOX", FullName: "INBOX"},
		{Name: "Receipts", FullName: "Receipts"},
	}
	if err := store.SaveLabelItems(items); err != nil {
		t.Fatalf("SaveLabelItems: %v", err)
	}

	got, err := store.GetLabelItems()
	if err != nil {
		t.Fatalf("GetLabelItems: %v", err)
	}
	if len(got) != 2 || got[1].FullName != "Receipts" {
		t.Errorf("got %+v, want the two saved items", got)
	}

	// The age decides whether a stale cache may stand in for a failed fetch.
	age, err := store.CachedLabelItemsAge()
	if err != nil {
		t.Fatalf("CachedLabelItemsAge: %v", err)
	}
	if age < 0 || age > time.Minute {
		t.Errorf("age = %v, want a few moments", age)
	}
}

func TestDiskCacheStoreIsActive(t *testing.T) {
	if !(&DiskCacheStore{Dir: cachedir.At(t.TempDir())}).IsActive() {
		t.Error("a disk cache store reports itself inactive")
	}
}

// The no-op store stands in for an account whose cache must not be written.
// Every method has to be safe and say nothing is there.
func TestNoOpCacheStore(t *testing.T) {
	var store NoOpCacheStore

	if store.IsActive() {
		t.Error("the no-op store reports itself active")
	}
	if err := store.SaveLabelItems([]cfg_acc.Item{{Name: "X"}}); err != nil {
		t.Errorf("SaveLabelItems = %v, want nil", err)
	}
	if _, err := store.GetLabelItems(); err == nil {
		t.Error("GetLabelItems succeeded on the no-op store")
	}
	if _, err := store.CachedLabelItemsAge(); err == nil {
		t.Error("CachedLabelItemsAge succeeded on the no-op store")
	}
}

func TestMarkIDsReadStaleSnapshotPreservesCommitted(t *testing.T) {
	dir := t.TempDir()

	// Initial state loaded by process A
	staleSnapshot := map[email.ID]bool{"m1": true}
	if err := saveReadState(dir, staleSnapshot); err != nil {
		t.Fatalf("saveReadState: %v", err)
	}

	// Process B commits an update while A has the stale snapshot
	if err := MarkIDsRead(cachedir.At(dir), []email.ID{"m2"}, nil); err != nil {
		t.Fatalf("MarkIDsRead m2: %v", err)
	}

	// Process A commits its update with the old snapshot
	if err := MarkIDsRead(cachedir.At(dir), []email.ID{"m3"}, staleSnapshot); err != nil {
		t.Fatalf("MarkIDsRead m3: %v", err)
	}

	final := LoadReadState(cachedir.At(dir))
	for _, id := range []email.ID{"m1", "m2", "m3"} {
		if !final[id] {
			t.Errorf("expected %q in read state, but was missing (stale snapshot overwrite)", id)
		}
	}
}

// assertCorruptStatePreserved seeds filename with corruptData, runs op against
// it, and checks the call failed without overwriting the corrupt bytes.
func assertCorruptStatePreserved(t *testing.T, filename string, corruptData []byte, op func(cachedir.Account) error) {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, filename)
	if err := os.WriteFile(p, corruptData, 0600); err != nil {
		t.Fatalf("seeding corrupt file: %v", err)
	}

	if err := op(cachedir.At(dir)); err == nil {
		t.Fatalf("operation succeeded on corrupt %s, expected error", filename)
	}

	data, readErr := os.ReadFile(p)
	if readErr != nil {
		t.Fatalf("reading file after failed operation: %v", readErr)
	}
	if string(data) != string(corruptData) {
		t.Errorf("corrupt %s was overwritten! got %q, want %q", filename, data, corruptData)
	}
}

func TestMarkIDsReadCorruptFileFailsWithoutOverwriting(t *testing.T) {
	assertCorruptStatePreserved(t, "seen_cache.json", []byte(`{"m1": true, "corrupt":`),
		func(acc cachedir.Account) error { return MarkIDsRead(acc, []email.ID{"new_msg"}, nil) })
}

func TestMarkIDsRepliedCorruptFileFailsWithoutOverwriting(t *testing.T) {
	assertCorruptStatePreserved(t, "replied_cache.json", []byte(`{"r1": true, "broken":`),
		func(acc cachedir.Account) error { return MarkIDsReplied(acc, []email.ID{"new_msg"}, nil) })
}

func TestMarkIDsReadLockFailure(t *testing.T) {
	f := filepath.Join(t.TempDir(), "blocking_file")
	if err := os.WriteFile(f, []byte("x"), 0600); err != nil {
		t.Fatalf("seeding: %v", err)
	}
	invalidDir := filepath.Join(f, "sub")

	err := MarkIDsRead(cachedir.At(invalidDir), []email.ID{"m1"}, nil)
	if err == nil {
		t.Fatal("MarkIDsRead succeeded on invalid lock path, want error")
	}
}

func TestUpdateReadStateCorruptFilePreserved(t *testing.T) {
	assertCorruptStatePreserved(t, "seen_cache.json", []byte(`{"m1": true, "corrupt":`),
		readState.mark("new_msg"))
}

func TestUpdateRepliedStateCorruptFilePreserved(t *testing.T) {
	assertCorruptStatePreserved(t, "replied_cache.json", []byte(`{"r1": true, "broken":`),
		repliedState.mark("new_reply"))
}

func TestUpdateReadStateConcurrent(t *testing.T) {
	dir := t.TempDir()
	var wg sync.WaitGroup
	for w := range 4 {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := range 10 {
				id := email.ID(fmt.Sprintf("w%d_msg%d", worker, i))
				if err := UpdateReadState(cachedir.At(dir), func(state map[email.ID]bool) {
					state[id] = true
				}); err != nil {
					t.Errorf("UpdateReadState: %v", err)
					return
				}
			}
		}(w)
	}
	wg.Wait()

	final := LoadReadState(cachedir.At(dir))
	for w := range 4 {
		for i := range 10 {
			id := email.ID(fmt.Sprintf("w%d_msg%d", workerKey(w), i))
			if !final[id] {
				t.Errorf("missing key %q after concurrent UpdateReadState", id)
			}
		}
	}
}

func workerKey(w int) int {
	return w
}

// stateCache pairs the update and load functions of one boolean state cache.
// The read-marks and replied-to marks live in two files of the same shape, so a
// shared assertion drives both.
type stateCache struct {
	update func(cachedir.Account, func(map[email.ID]bool)) error
	load   func(cachedir.Account) map[email.ID]bool
}

var (
	readState    = stateCache{update: UpdateReadState, load: LoadReadState}
	repliedState = stateCache{update: UpdateRepliedState, load: LoadRepliedState}
)

// mark returns an op that records key through this cache's update function.
func (sc stateCache) mark(key email.ID) func(cachedir.Account) error {
	return func(acc cachedir.Account) error {
		return sc.update(acc, func(state map[email.ID]bool) { state[key] = true })
	}
}

// blockCacheLock returns a cache directory whose lock path is occupied by a
// directory, so acquiring the advisory lock fails.
func blockCacheLock(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".cache.lock"), 0700); err != nil {
		t.Fatalf("seeding blocking lock dir: %v", err)
	}
	return dir
}

// assertUpdatePersists applies an update marking key, then checks key survived
// into what the matching load returns.
func assertUpdatePersists(t *testing.T, dir string, sc stateCache, key email.ID) {
	t.Helper()
	acc := cachedir.At(dir)
	if err := sc.update(acc, func(state map[email.ID]bool) {
		state[key] = true
	}); err != nil {
		t.Fatalf("update %q: %v", key, err)
	}
	state := sc.load(acc)
	if !state[key] {
		t.Errorf("expected %q to be persisted, got %v", key, state)
	}
}

func TestUpdateReadStateSuccess(t *testing.T) {
	assertUpdatePersists(t, t.TempDir(), readState, "msg1")
}

func TestUpdateRepliedStateSuccess(t *testing.T) {
	assertUpdatePersists(t, t.TempDir(), repliedState, "reply1")
}

func TestUpdateReadStateDegradesGracefullyOnLockFailure(t *testing.T) {
	assertUpdatePersists(t, blockCacheLock(t), readState, "graceful_msg")
}

func TestUpdateRepliedStateDegradesGracefullyOnLockFailure(t *testing.T) {
	assertUpdatePersists(t, blockCacheLock(t), repliedState, "graceful_reply")
}

func TestDiskCacheStoreConcurrentSaveLabelItems(t *testing.T) {
	dir := t.TempDir()
	store := &DiskCacheStore{Dir: cachedir.At(dir)}

	var wg sync.WaitGroup
	for w := range 5 {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := range 8 {
				items := []cfg_acc.Item{
					{Name: fmt.Sprintf("W%d_%d", worker, i), FullName: fmt.Sprintf("W%d_%d", worker, i)},
				}
				if err := store.SaveLabelItems(items); err != nil {
					t.Errorf("SaveLabelItems worker %d iteration %d: %v", worker, i, err)
					return
				}
			}
		}(w)
	}
	wg.Wait()

	items, err := store.GetLabelItems()
	if err != nil {
		t.Fatalf("GetLabelItems after concurrent saves: %v", err)
	}
	if len(items) != 1 {
		t.Errorf("expected 1 label item from last write, got %d", len(items))
	}
}
