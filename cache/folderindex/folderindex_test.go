package folderindex

import (
	"fmt"
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"github.com/sarielhp/mail_cli/email"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func has(ids []email.ID, want email.ID) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}

func TestAddRemoveMoveRoundTrip(t *testing.T) {
	dir := t.TempDir()

	if err := Add(cachedir.At(dir), "INBOX", "m1"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := Add(cachedir.At(dir), "INBOX", "m1"); err != nil { // idempotent
		t.Fatalf("Add twice: %v", err)
	}
	ids, _ := IDs(cachedir.At(dir), "INBOX")
	if len(ids) != 1 {
		t.Fatalf("expected 1 id after adding the same id twice, got %v", ids)
	}

	if err := MoveBatch(cachedir.At(dir), []email.ID{"m1"}, "INBOX", "Archive"); err != nil {
		t.Fatalf("Move: %v", err)
	}
	inbox, _ := IDs(cachedir.At(dir), "INBOX")
	archive, _ := IDs(cachedir.At(dir), "Archive")
	if has(inbox, "m1") {
		t.Errorf("m1 still in INBOX: %v", inbox)
	}
	if !has(archive, "m1") {
		t.Errorf("m1 not in Archive: %v", archive)
	}

	// Move is idempotent: repeating it must not resurrect or duplicate.
	if err := MoveBatch(cachedir.At(dir), []email.ID{"m1"}, "INBOX", "Archive"); err != nil {
		t.Fatalf("Move again: %v", err)
	}
	archive, _ = IDs(cachedir.At(dir), "Archive")
	if len(archive) != 1 {
		t.Errorf("expected Archive to hold exactly 1 id, got %v", archive)
	}

	if err := Remove(cachedir.At(dir), "Archive", "m1"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	archive, _ = IDs(cachedir.At(dir), "Archive")
	if has(archive, "m1") {
		t.Errorf("m1 not removed: %v", archive)
	}
	// Removing something absent is not an error.
	if err := Remove(cachedir.At(dir), "Archive", "nope"); err != nil {
		t.Errorf("Remove of an absent id should succeed, got %v", err)
	}
}

// TestConcurrentAddDoesNotLoseUpdates is the regression guard for the missing
// lock: Add is a read-modify-write over a whole-file list, so without
// serialization two writers each read the pre-update list and the second write
// silently discards the first. Each TUI action runs on its own goroutine, so
// this was reachable by archiving messages in quick succession.
func TestConcurrentAddDoesNotLoseUpdates(t *testing.T) {
	dir := t.TempDir()
	const n = 60

	if err := ReplaceAll(cachedir.At(dir), "INBOX", []email.ID{}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if err := Add(cachedir.At(dir), "INBOX", email.ID(fmt.Sprintf("m%03d", i))); err != nil {
				t.Errorf("Add(m%03d): %v", i, err)
			}
		}(i)
	}
	wg.Wait()

	ids, err := IDs(cachedir.At(dir), "INBOX")
	if err != nil {
		t.Fatalf("IDs: %v", err)
	}
	if len(ids) != n {
		t.Errorf("lost updates: expected %d ids, got %d", n, len(ids))
	}
	for i := 0; i < n; i++ {
		if !has(ids, email.ID(fmt.Sprintf("m%03d", i))) {
			t.Errorf("m%03d missing from the index", i)
		}
	}
}

// TestConcurrentMoveKeepsEveryMessage checks that a message is never lost
// between folders: Move is Remove-then-Add, and without a guard held across
// both a concurrent reader (or a crash) could see it in neither.
func TestConcurrentMoveKeepsEveryMessage(t *testing.T) {
	dir := t.TempDir()
	const n = 40

	seed := make([]string, n)
	for i := range seed {
		seed[i] = fmt.Sprintf("m%03d", i)
	}
	if err := ReplaceAll(cachedir.At(dir), "src", email.IDs(seed)); err != nil {
		t.Fatalf("seed: %v", err)
	}

	var wg sync.WaitGroup
	for _, id := range seed {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			if err := MoveBatch(cachedir.At(dir), []email.ID{email.ID(id)}, "src", "dst"); err != nil {
				t.Errorf("Move(%s): %v", id, err)
			}
		}(id)
	}
	wg.Wait()

	src, _ := IDs(cachedir.At(dir), "src")
	dst, _ := IDs(cachedir.At(dir), "dst")
	if len(src) != 0 {
		t.Errorf("expected the source index to be empty, got %d: %v", len(src), src)
	}
	if len(dst) != n {
		t.Errorf("messages lost in transit: expected %d in dst, got %d", n, len(dst))
	}
}

func TestConcurrentReplaceAllAndMove(t *testing.T) {
	dir := t.TempDir()
	const n = 30

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(3)
		msgID := email.ID(fmt.Sprintf("m%03d", i))
		go func() {
			defer wg.Done()
			_ = Add(cachedir.At(dir), "FolderA", msgID)
		}()
		go func() {
			defer wg.Done()
			_ = MoveBatch(cachedir.At(dir), []email.ID{msgID}, "FolderA", "FolderB")
		}()
		go func(id string) {
			defer wg.Done()
			_ = ReplaceAll(cachedir.At(dir), "FolderC", []email.ID{email.ID(id)})
		}(string(msgID))
	}
	wg.Wait()
}

func TestMoveBatch(t *testing.T) {
	dir := t.TempDir()

	initial := []string{"m1", "m2", "m3", "m4", "m5"}
	if err := ReplaceAll(cachedir.At(dir), "INBOX", email.IDs(initial)); err != nil {
		t.Fatalf("ReplaceAll INBOX: %v", err)
	}

	// Move subset m2, m3, m4 plus nonexistent m99
	toMove := []string{"m2", "m3", "m4", "m99"}
	if err := MoveBatch(cachedir.At(dir), email.IDs(toMove), "INBOX", "Archive"); err != nil {
		t.Fatalf("MoveBatch: %v", err)
	}

	inbox, err := IDs(cachedir.At(dir), "INBOX")
	if err != nil {
		t.Fatalf("IDs INBOX: %v", err)
	}
	if len(inbox) != 2 || !has(inbox, "m1") || !has(inbox, "m5") {
		t.Errorf("expected INBOX to have [m1, m5], got %v", inbox)
	}

	archive, err := IDs(cachedir.At(dir), "Archive")
	if err != nil {
		t.Fatalf("IDs Archive: %v", err)
	}
	if len(archive) != 4 {
		t.Fatalf("expected Archive to have 4 items, got %d: %v", len(archive), archive)
	}
	for _, id := range toMove {
		if !has(archive, email.ID(id)) {
			t.Errorf("expected Archive to have %s", id)
		}
	}

	// MoveBatch idempotent: re-running does not duplicate
	if err := MoveBatch(cachedir.At(dir), email.IDs(toMove), "INBOX", "Archive"); err != nil {
		t.Fatalf("MoveBatch duplicate: %v", err)
	}
	archiveAfter, _ := IDs(cachedir.At(dir), "Archive")
	if len(archiveAfter) != 4 {
		t.Errorf("expected Archive to still have 4 items, got %d: %v", len(archiveAfter), archiveAfter)
	}

	// Same folder or empty slice is a no-op
	if err := MoveBatch(cachedir.At(dir), email.IDs(toMove), "Archive", "Archive"); err != nil {
		t.Errorf("same folder MoveBatch: %v", err)
	}
	if err := MoveBatch(cachedir.At(dir), nil, "Archive", "Trash"); err != nil {
		t.Errorf("empty MoveBatch: %v", err)
	}
}

func TestMoveBatch_DestinationWriteFailureLeavesSourceIntact(t *testing.T) {
	dir := t.TempDir()

	if err := ReplaceAll(cachedir.At(dir), "INBOX", []email.ID{"m1", "m2"}); err != nil {
		t.Fatalf("ReplaceAll: %v", err)
	}

	// Block writes to Archive folder index by creating a non-empty directory at its target path.
	destPath := idsPath(dir, "Archive")
	if err := os.MkdirAll(destPath, 0700); err != nil {
		t.Fatalf("MkdirAll destPath: %v", err)
	}
	if err := os.WriteFile(filepath.Join(destPath, "blocker"), []byte("data"), 0600); err != nil {
		t.Fatalf("WriteFile blocker: %v", err)
	}

	// Move m1 from INBOX to Archive. Must fail because destination cannot be written.
	err := MoveBatch(cachedir.At(dir), []email.ID{"m1"}, "INBOX", "Archive")
	if err == nil {
		t.Fatal("expected MoveBatch to fail when destination cannot be written, got nil")
	}

	// Verify INBOX still contains m1 and m2 (never lost due to destination failure).
	inbox, err := IDs(cachedir.At(dir), "INBOX")
	if err != nil {
		t.Fatalf("IDs INBOX: %v", err)
	}
	if !has(inbox, "m1") || !has(inbox, "m2") {
		t.Fatalf("source index was modified despite destination failure: %v", inbox)
	}
}

func TestPurgeIDs(t *testing.T) {
	dir := t.TempDir()

	if err := ReplaceAll(cachedir.At(dir), "INBOX", []email.ID{"m1", "m2", "m3"}); err != nil {
		t.Fatalf("ReplaceAll INBOX: %v", err)
	}
	if err := ReplaceAll(cachedir.At(dir), "Archive", []email.ID{"m2", "m4"}); err != nil {
		t.Fatalf("ReplaceAll Archive: %v", err)
	}

	if err := PurgeIDs(cachedir.At(dir), []email.ID{"m2", "m3"}); err != nil {
		t.Fatalf("PurgeIDs: %v", err)
	}

	inbox, err := IDs(cachedir.At(dir), "INBOX")
	if err != nil {
		t.Fatalf("IDs INBOX: %v", err)
	}
	if len(inbox) != 1 || inbox[0] != "m1" {
		t.Errorf("expected INBOX to have [m1], got %v", inbox)
	}

	archive, err := IDs(cachedir.At(dir), "Archive")
	if err != nil {
		t.Fatalf("IDs Archive: %v", err)
	}
	if len(archive) != 1 || archive[0] != "m4" {
		t.Errorf("expected Archive to have [m4], got %v", archive)
	}

	// Purge with empty slice or nonexistent folder is a no-op
	if err := PurgeIDs(cachedir.At(dir), nil); err != nil {
		t.Errorf("PurgeIDs nil: %v", err)
	}
	if err := PurgeIDs(cachedir.At(t.TempDir()), []email.ID{"m1"}); err != nil {
		t.Errorf("PurgeIDs nonexistent dir: %v", err)
	}
}

// TestPurgeIDs_RemovesDeltaEntries is H5's regression: an ID appended via
// AppendBatch (and so living only in "<folder>.deltas.jsonl") must not
// resurrect once purged, or cache prune reports a deletion that IDs() then
// silently undoes on the very next read.
func TestPurgeIDs_RemovesDeltaEntries(t *testing.T) {
	dir := t.TempDir()

	if err := ReplaceAll(cachedir.At(dir), "INBOX", []email.ID{"m1", "m2"}); err != nil {
		t.Fatalf("ReplaceAll: %v", err)
	}
	if err := AppendBatch(cachedir.At(dir), "INBOX", []email.ID{"m3", "m4"}); err != nil {
		t.Fatalf("AppendBatch: %v", err)
	}

	before, err := IDs(cachedir.At(dir), "INBOX")
	if err != nil {
		t.Fatalf("IDs before purge: %v", err)
	}
	if !has(before, "m3") || !has(before, "m4") {
		t.Fatalf("expected delta entries visible before purge, got %v", before)
	}

	if purgeErr := PurgeIDs(cachedir.At(dir), []email.ID{"m3", "m4"}); purgeErr != nil {
		t.Fatalf("PurgeIDs: %v", purgeErr)
	}

	after, err := IDs(cachedir.At(dir), "INBOX")
	if err != nil {
		t.Fatalf("IDs after purge: %v", err)
	}
	if has(after, "m3") || has(after, "m4") {
		t.Errorf("purged delta-only IDs resurrected after purge: %v", after)
	}
	if !has(after, "m1") || !has(after, "m2") {
		t.Errorf("purge must not touch untouched IDs, got %v", after)
	}

	// The deltas file must be folded away so a later reader can't reopen it
	// and union the purged IDs back in.
	if _, err := os.Stat(deltasPath(dir, "INBOX")); !os.IsNotExist(err) {
		t.Errorf("expected deltas file removed after purge, stat err = %v", err)
	}
}

// TestPurgeIDs_OneMalformedFolderDoesNotBlockTheOthers is a regression for a
// bug the H5 rewrite itself introduced: the old per-file loop silently
// skipped a folder whose index it could not parse and kept purging the
// rest; the rewrite's per-key loop returned on the first such error and
// left every other folder purged only by luck of map-iteration order.
func TestPurgeIDs_OneMalformedFolderDoesNotBlockTheOthers(t *testing.T) {
	dir := t.TempDir()

	if err := ReplaceAll(cachedir.At(dir), "INBOX", []email.ID{"m1", "m2"}); err != nil {
		t.Fatalf("ReplaceAll INBOX: %v", err)
	}
	if err := ReplaceAll(cachedir.At(dir), "Archive", []email.ID{"m1", "m3"}); err != nil {
		t.Fatalf("ReplaceAll Archive: %v", err)
	}
	// A folder whose base index is not valid JSON.
	if err := os.WriteFile(idsPath(dir, "Corrupt"), []byte("not valid json"), 0600); err != nil {
		t.Fatalf("seeding corrupt index: %v", err)
	}

	if err := PurgeIDs(cachedir.At(dir), []email.ID{"m1"}); err != nil {
		t.Fatalf("PurgeIDs: %v", err)
	}

	inbox, err := IDs(cachedir.At(dir), "INBOX")
	if err != nil || has(inbox, "m1") || !has(inbox, "m2") {
		t.Errorf("expected INBOX purged despite Corrupt's error, got %v (err %v)", inbox, err)
	}
	archive, err := IDs(cachedir.At(dir), "Archive")
	if err != nil || has(archive, "m1") || !has(archive, "m3") {
		t.Errorf("expected Archive purged despite Corrupt's error, got %v (err %v)", archive, err)
	}
}

func TestAppendBatch(t *testing.T) {
	dir := t.TempDir()

	// 1. AppendBatch to nonexistent index creates it
	if err := AppendBatch(cachedir.At(dir), "INBOX", []email.ID{"m1", "m2"}); err != nil {
		t.Fatalf("AppendBatch initial: %v", err)
	}
	ids, err := IDs(cachedir.At(dir), "INBOX")
	if err != nil {
		t.Fatalf("IDs INBOX: %v", err)
	}
	if len(ids) != 2 || ids[0] != "m1" || ids[1] != "m2" {
		t.Fatalf("expected [m1, m2], got %v", ids)
	}

	// 2. Incremental AppendBatch appends without modifying existing entries
	if appendErr := AppendBatch(cachedir.At(dir), "INBOX", []email.ID{"m3", "m4"}); appendErr != nil {
		t.Fatalf("AppendBatch incremental: %v", appendErr)
	}
	ids, err = IDs(cachedir.At(dir), "INBOX")
	if err != nil {
		t.Fatalf("IDs INBOX after append: %v", err)
	}
	expected := []email.ID{"m1", "m2", "m3", "m4"}
	if len(ids) != len(expected) {
		t.Fatalf("expected %d entries, got %d", len(expected), len(ids))
	}
	for i, exp := range expected {
		if ids[i] != exp {
			t.Errorf("entry [%d]: expected %q, got %q", i, exp, ids[i])
		}
	}

	// 3. Verify deltas file was created and is read by IDs
	dp := deltasPath(dir, "INBOX")
	if _, statErr := os.Stat(dp); statErr != nil {
		t.Errorf("expected deltas file %s to exist, err: %v", dp, statErr)
	}

	// 4. ReplaceAll atomically resets index and clears deltas
	if replaceErr := ReplaceAll(cachedir.At(dir), "INBOX", []email.ID{"m1"}); replaceErr != nil {
		t.Fatalf("ReplaceAll: %v", replaceErr)
	}
	if _, statErr := os.Stat(dp); !os.IsNotExist(statErr) {
		t.Errorf("expected deltas file to be removed after ReplaceAll, got err: %v", statErr)
	}
	ids, err = IDs(cachedir.At(dir), "INBOX")
	if err != nil || len(ids) != 1 || ids[0] != "m1" {
		t.Fatalf("IDs after ReplaceAll: %v, err: %v", ids, err)
	}

	// 5. AppendBatch with empty inputs is no-op
	if err := AppendBatch(cachedir.At(dir), "INBOX", nil); err != nil {
		t.Errorf("AppendBatch nil: %v", err)
	}
	if err := AppendBatch(cachedir.At(""), "INBOX", []email.ID{"m5"}); err != nil {
		t.Errorf("AppendBatch empty dir: %v", err)
	}
}
