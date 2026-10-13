package msg

import (
	"fmt"
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"github.com/sarielhp/mail_cli/email"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sarielhp/mail_cli/message_info"
)

// These are characterization tests: they pin down what the message cache
// actually does today, including the places where that differs from what a
// reader would assume. The read-through message store is being built directly
// on top of this package, so its current behavior needs to be checked rather
// than inferred.

const sampleRaw = "From: alice@example.com\r\n" +
	"To: bob@example.com\r\n" +
	"Subject: hello\r\n" +
	"Date: Tue, 02 Sep 2025 10:04:05 +0000\r\n" +
	"Message-ID: <abc@example.com>\r\n\r\n" +
	"body text\r\n"

func mustStore(t *testing.T, dir string, id email.ID, date time.Time) {
	t.Helper()
	if err := Store(cachedir.At(dir), id, []byte(sampleRaw), date); err != nil {
		t.Fatalf("Store(%s): %v", id, err)
	}
}

func TestStoreReadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	mustStore(t, dir, "msg001", time.Date(2025, 9, 2, 10, 4, 5, 0, time.UTC))

	got, err := Read(cachedir.At(dir), "msg001")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if string(got) != sampleRaw {
		t.Errorf("Read returned %q, want %q", got, sampleRaw)
	}
}

// The on-disk layout is derived from the date passed to Store, not from the
// message's own Date header. Callers currently pass time.Now(), so a message's
// path reflects when it was downloaded.
func TestStorePathIsDerivedFromSuppliedDate(t *testing.T) {
	dir := t.TempDir()
	mustStore(t, dir, "msg002", time.Date(2024, 3, 7, 23, 59, 0, 0, time.UTC))

	want := filepath.Join(dir, "messages", "2024", "03", "07", "msg002.eml")
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("expected message at %s: %v", want, err)
	}
}

func TestExists(t *testing.T) {
	dir := t.TempDir()
	mustStore(t, dir, "present", time.Now())

	tests := []struct {
		name string
		id   string
		want bool
	}{
		{name: "stored message", id: "present", want: true},
		{name: "never stored", id: "absent", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Exists(cachedir.At(dir), email.ID(tc.id))
			if err != nil {
				t.Fatalf("Exists: %v", err)
			}
			if got != tc.want {
				t.Errorf("Exists(%q) = %v, want %v", tc.id, got, tc.want)
			}
		})
	}
}

// An ID absent from the index reports (false, nil) rather than an error, so a
// caller cannot distinguish "not cached" from "index unreadable".
func TestExistsReportsNoErrorForUnknownID(t *testing.T) {
	_, err := Exists(cachedir.At(t.TempDir()), "never-seen")
	if err != nil {
		t.Errorf("Exists on unknown ID returned error %v, want nil", err)
	}
}

// The index entry and the .eml file can disagree: Exists consults both, but
// Read fails with the os error rather than the index error.
func TestExistsFalseWhenIndexedFileIsMissing(t *testing.T) {
	dir := t.TempDir()
	mustStore(t, dir, "vanishing", time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC))

	emlPath := filepath.Join(dir, "messages", "2025", "01", "02", "vanishing.eml")
	if err := os.Remove(emlPath); err != nil {
		t.Fatalf("removing cached file: %v", err)
	}

	got, err := Exists(cachedir.At(dir), "vanishing")
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if got {
		t.Error("Exists = true after the .eml was deleted, want false")
	}
	if _, err := Read(cachedir.At(dir), "vanishing"); err == nil {
		t.Error("Read succeeded for a message whose file was deleted")
	}
}

func TestReadUnknownIDFails(t *testing.T) {
	if _, err := Read(cachedir.At(t.TempDir()), "nope"); err == nil {
		t.Error("Read of an unindexed ID returned nil error")
	}
}

func TestDelete(t *testing.T) {
	dir := t.TempDir()
	mustStore(t, dir, "doomed", time.Now())

	if err := Delete(cachedir.At(dir), "doomed"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if exists, _ := Exists(cachedir.At(dir), "doomed"); exists {
		t.Error("Exists = true after Delete")
	}
	if _, err := Read(cachedir.At(dir), "doomed"); err == nil {
		t.Error("Read succeeded after Delete")
	}
}

func TestDeleteBatch(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	mustStore(t, dir, "b1", now)
	mustStore(t, dir, "b2", now)
	mustStore(t, dir, "b3", now)

	deleted, err := DeleteBatch(cachedir.At(dir), []email.ID{"b1", "b3"})
	if err != nil {
		t.Fatalf("DeleteBatch: %v", err)
	}
	if deleted != 2 {
		t.Errorf("DeleteBatch count = %d, want 2", deleted)
	}

	for _, id := range []email.ID{"b1", "b3"} {
		if exists, _ := Exists(cachedir.At(dir), id); exists {
			t.Errorf("Exists(%q) = true after DeleteBatch", id)
		}
		if _, err := Read(cachedir.At(dir), id); err == nil {
			t.Errorf("Read(%q) succeeded after DeleteBatch", id)
		}
	}

	if exists, _ := Exists(cachedir.At(dir), "b2"); !exists {
		t.Error("Exists(\"b2\") = false, but it was not deleted")
	}

	// Empty and nil batches are no-ops
	if count, err := DeleteBatch(cachedir.At(dir), nil); err != nil || count != 0 {
		t.Errorf("DeleteBatch(nil) = (%d, %v), want (0, nil)", count, err)
	}
	if count, err := DeleteBatch(cachedir.At(""), []email.ID{"b2"}); err != nil || count != 0 {
		t.Errorf("DeleteBatch(\"\") = (%d, %v), want (0, nil)", count, err)
	}
}

// Deleting something that was never cached is not an error.
func TestDeleteUnknownIDIsNoOp(t *testing.T) {
	if err := Delete(cachedir.At(t.TempDir()), "never-existed"); err != nil {
		t.Errorf("Delete of unknown ID = %v, want nil", err)
	}
}

func TestDelete_PersistenceAcrossReload(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	mustStore(t, dir, "msg-keep", now)
	mustStore(t, dir, "msg-delete", now)

	if err := Delete(cachedir.At(dir), "msg-delete"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Invalidate in-memory dirCache to simulate process restart
	dirCaches.Delete(dir)

	// Verify msg-delete was not resurrected from message_info.bin
	if info := getCompact(dir, "msg-delete"); info != nil {
		t.Errorf("msg-delete resurrected in cache after reload: %+v", info)
	}
	if info := getCompact(dir, "msg-keep"); info == nil {
		t.Errorf("msg-keep missing from cache after reload")
	}

	var foundIDs []string
	ForEachID(cachedir.At(dir), func(id email.ID) bool {
		foundIDs = append(foundIDs, string(id))
		return true
	})

	if len(foundIDs) != 1 || foundIDs[0] != "msg-keep" {
		t.Errorf("expected only [msg-keep] in ForEachID, got: %v", foundIDs)
	}
}

func TestGetInfoDate(t *testing.T) {
	dir := t.TempDir()
	mustStore(t, dir, "dated", time.Date(2023, 11, 30, 14, 22, 9, 0, time.UTC))

	info, err := GetInfo(cachedir.At(dir), "dated")
	if err != nil {
		t.Fatalf("GetInfo: %v", err)
	}
	// Only the calendar day survives the round trip; the clock time is dropped.
	want := time.Date(2023, 11, 30, 0, 0, 0, 0, time.UTC)
	if !info.Date.Equal(want) {
		t.Errorf("GetInfo().Date = %v, want %v", info.Date, want)
	}
}

func TestGetInfoUnknownIDFails(t *testing.T) {
	if _, err := GetInfo(cachedir.At(t.TempDir()), "nope"); err == nil {
		t.Error("GetInfo of an unindexed ID returned nil error")
	}
}

func TestSetAndForgetEvidence(t *testing.T) {
	dir := t.TempDir()
	stored := time.Date(2025, 5, 5, 8, 0, 0, 0, time.UTC)
	mustStore(t, dir, "suspect", stored)

	if err := SetEvidence(cachedir.At(dir), "suspect", Evidence{Checks: CheckBogofilter | CheckPolitical, BogoScore: 7}); err != nil {
		t.Fatalf("SetEvidence: %v", err)
	}

	info, err := GetInfo(cachedir.At(dir), "suspect")
	if err != nil {
		t.Fatalf("GetInfo: %v", err)
	}
	if !info.Scanned || !info.IsSpam() || !info.IsPolitical() || info.IsBlacklisted() {
		t.Errorf("after SetEvidence: %+v", info)
	}
	// Classification must not disturb the date, which is what locates the file.
	if info.Date.Day() != stored.Day() || info.Date.Month() != stored.Month() {
		t.Errorf("SetEvidence changed the stored date to %v", info.Date)
	}
	if _, readErr := Read(cachedir.At(dir), "suspect"); readErr != nil {
		t.Errorf("message unreadable after classification: %v", readErr)
	}

	if forgetErr := ForgetEvidence(cachedir.At(dir), "suspect"); forgetErr != nil {
		t.Fatalf("ClearClassification: %v", forgetErr)
	}
	cleared, err := GetInfo(cachedir.At(dir), "suspect")
	if err != nil {
		t.Fatalf("GetInfo after clear: %v", err)
	}
	if cleared.Scanned || cleared.IsSpam() || cleared.IsPolitical() {
		t.Errorf("flags survived ClearClassification: %+v", cleared)
	}
}

// The spam score is a float and must stay one across a restart. The version 1
// record wrote uint32(score), so 6.75 came back as 6 once the index reloaded
// while the in-memory value still read 6.75 -- a value that changed depending
// only on whether the process had been restarted.
func TestSpamScoreSurvivesReload(t *testing.T) {
	dir := t.TempDir()
	mustStore(t, dir, "scored", time.Date(2025, 4, 4, 0, 0, 0, 0, time.UTC))

	if err := SetEvidence(cachedir.At(dir), "scored", Evidence{Checks: CheckBogofilter, BogoScore: 6.75}); err != nil {
		t.Fatalf("SetEvidence: %v", err)
	}
	info, err := GetInfo(cachedir.At(dir), "scored")
	if err != nil {
		t.Fatalf("GetInfo: %v", err)
	}
	if info.Evidence.BogoScore != 6.75 {
		t.Errorf("in-memory SpamScore = %v, want 6.75", info.Evidence.BogoScore)
	}

	dst := filepath.Join(t.TempDir(), "copy")
	if copyErr := os.CopyFS(dst, os.DirFS(dir)); copyErr != nil {
		t.Fatalf("copying cache: %v", copyErr)
	}
	reloaded, err := GetInfo(cachedir.At(dst), "scored")
	if err != nil {
		t.Fatalf("GetInfo after reload: %v", err)
	}
	if reloaded.Evidence.BogoScore != 6.75 {
		t.Errorf("SpamScore after reload = %v, want 6.75", reloaded.Evidence.BogoScore)
	}
}

func TestSetVerdictUnknownIDFails(t *testing.T) {
	if err := SetEvidence(cachedir.At(t.TempDir()), "nope", Evidence{Checks: CheckBogofilter, BogoScore: 1}); err == nil {
		t.Error("SetVerdict on an unindexed ID returned nil error")
	}
}

func TestClearClassificationUnknownIDIsNoOp(t *testing.T) {
	if err := ForgetEvidence(cachedir.At(t.TempDir()), "nope"); err != nil {
		t.Errorf("ClearClassification of unknown ID = %v, want nil", err)
	}
}

func TestForEachIDVisitsEveryStoredMessage(t *testing.T) {
	dir := t.TempDir()
	want := map[string]bool{"aa01": true, "bb02": true, "cc03": true}
	for id := range want {
		mustStore(t, dir, email.ID(id), time.Now())
	}

	seen := make(map[string]bool)
	ForEachID(cachedir.At(dir), func(id email.ID) bool {
		seen[string(id)] = true
		return false
	})

	if len(seen) != len(want) {
		t.Fatalf("ForEachID visited %d IDs (%v), want %d", len(seen), seen, len(want))
	}
	for id := range want {
		if !seen[id] {
			t.Errorf("ForEachID never visited %q", id)
		}
	}
}

// Returning true stops the walk early.
func TestForEachIDStopsWhenCallbackReturnsTrue(t *testing.T) {
	dir := t.TempDir()
	for i := range 5 {
		mustStore(t, dir, email.ID(fmt.Sprintf("id%02d", i)), time.Now())
	}

	count := 0
	ForEachID(cachedir.At(dir), func(email.ID) bool {
		count++
		return true
	})
	if count != 1 {
		t.Errorf("ForEachID made %d calls after the first returned true, want 1", count)
	}
}

func TestSummaryRoundTrip(t *testing.T) {
	dir := t.TempDir()
	mustStore(t, dir, "summarized", time.Now())

	if HasSummary(cachedir.At(dir), "summarized") {
		t.Error("HasSummary = true before any summary was stored")
	}
	if err := StoreSummary(cachedir.At(dir), "summarized", "a short summary"); err != nil {
		t.Fatalf("StoreSummary: %v", err)
	}
	if !HasSummary(cachedir.At(dir), "summarized") {
		t.Error("HasSummary = false after StoreSummary")
	}
	got, err := ReadSummary(cachedir.At(dir), "summarized")
	if err != nil {
		t.Fatalf("ReadSummary: %v", err)
	}
	if got != "a short summary" {
		t.Errorf("ReadSummary = %q, want %q", got, "a short summary")
	}
}

// Summaries written before the .md suffix was adopted are still found.
func TestReadSummaryFallsBackToLegacyPath(t *testing.T) {
	dir := t.TempDir()
	mustStore(t, dir, "legacy", time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC))

	legacyPath := filepath.Join(dir, "messages", "2025", "06", "01", "legacy.llm_summary")
	if err := os.WriteFile(legacyPath, []byte("old format"), 0600); err != nil {
		t.Fatalf("writing legacy summary: %v", err)
	}

	if !HasSummary(cachedir.At(dir), "legacy") {
		t.Error("HasSummary = false for a legacy summary file")
	}
	got, err := ReadSummary(cachedir.At(dir), "legacy")
	if err != nil {
		t.Fatalf("ReadSummary: %v", err)
	}
	if got != "old format" {
		t.Errorf("ReadSummary = %q, want %q", got, "old format")
	}
}

func TestDeleteRemovesSummary(t *testing.T) {
	dir := t.TempDir()
	mustStore(t, dir, "withsummary", time.Now())
	if err := StoreSummary(cachedir.At(dir), "withsummary", "text"); err != nil {
		t.Fatalf("StoreSummary: %v", err)
	}
	if err := Delete(cachedir.At(dir), "withsummary"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if HasSummary(cachedir.At(dir), "withsummary") {
		t.Error("summary survived Delete")
	}
}

func TestForgetAllEvidence(t *testing.T) {
	dir := t.TempDir()
	ids := []email.ID{"r1", "r2", "r3"}
	for _, id := range ids {
		mustStore(t, dir, id, time.Now())
		if err := SetEvidence(cachedir.At(dir), id, Evidence{Checks: CheckBogofilter | CheckPolitical | CheckBlacklist, BogoScore: 9}); err != nil {
			t.Fatalf("SetEvidence(%s): %v", id, err)
		}
	}

	if err := ForgetAllEvidence(cachedir.At(dir)); err != nil {
		t.Fatalf("ResetAllClassifications: %v", err)
	}

	for _, id := range ids {
		info, err := GetInfo(cachedir.At(dir), id)
		if err != nil {
			t.Fatalf("GetInfo(%s): %v", id, err)
		}
		if info.Scanned || info.IsSpam() || info.IsPolitical() || info.IsBlacklisted() {
			t.Errorf("%s still classified after reset: %+v", id, info)
		}
		if _, err := Read(cachedir.At(dir), id); err != nil {
			t.Errorf("%s unreadable after reset: %v", id, err)
		}
	}
}

// The index is reloaded from message_info.bin when a directory is seen for the
// first time. Copying a populated cache to a fresh path exercises that path,
// since the in-memory index is memoized per directory.
func TestIndexReloadsFromDisk(t *testing.T) {
	src := t.TempDir()
	mustStore(t, src, "persisted", time.Date(2022, 12, 25, 0, 0, 0, 0, time.UTC))
	if err := SetEvidence(cachedir.At(src), "persisted", Evidence{Checks: CheckBogofilter | CheckBlacklist, BogoScore: 4}); err != nil {
		t.Fatalf("SetEvidence: %v", err)
	}

	dst := filepath.Join(t.TempDir(), "copy")
	if err := os.CopyFS(dst, os.DirFS(src)); err != nil {
		t.Fatalf("copying cache: %v", err)
	}

	info, err := GetInfo(cachedir.At(dst), "persisted")
	if err != nil {
		t.Fatalf("GetInfo after reload: %v", err)
	}
	if !info.IsSpam() || !info.IsBlacklisted() || !info.Scanned {
		t.Errorf("flags did not survive reload: %+v", info)
	}
	if info.Date.Year() != 2022 || info.Date.Day() != 25 {
		t.Errorf("date did not survive reload: %v", info.Date)
	}
	got, err := Read(cachedir.At(dst), "persisted")
	if err != nil {
		t.Fatalf("Read after reload: %v", err)
	}
	if string(got) != sampleRaw {
		t.Error("message body did not survive reload")
	}
}

// The index file is an append-only log that is rewritten once stale entries
// exceed a quarter of it, and only past 1000 appends. Compaction must leave
// exactly one record per live ID.
func TestIndexCompactsWhenStaleEntriesAccumulate(t *testing.T) {
	if testing.Short() {
		t.Skip("compaction test writes >1000 cache entries")
	}
	dir := t.TempDir()
	const total = 1001
	for i := range total {
		mustStore(t, dir, email.ID(fmt.Sprintf("c%05d", i)), time.Now())
	}

	binFile := filepath.Join(dir, "message_info.bin")
	before := fileSize(t, binFile)

	// Re-classifying rewrites entries in place, pushing the stale ratio over
	// the threshold without adding IDs.
	const reclassified = 400
	for i := range reclassified {
		if err := SetEvidence(cachedir.At(dir), email.ID(fmt.Sprintf("c%05d", i)), Evidence{Checks: CheckBogofilter, BogoScore: 1}); err != nil {
			t.Fatalf("SetEvidence: %v", err)
		}
	}

	// Compaction runs partway through the loop, as soon as the stale ratio
	// first crosses the threshold; the appends after that point are still
	// appended. So the file is smaller than an uncompacted run would leave it,
	// but larger than one record per ID.
	after := fileSize(t, binFile)
	perRecord := before / int64(total)
	uncompacted := before + int64(reclassified)*perRecord
	if after >= uncompacted {
		t.Errorf("index is %d bytes, uncompacted would be about %d; compaction did not run", after, uncompacted)
	}
	if after < before {
		t.Errorf("index shrank to %d bytes, below one record per ID (%d)", after, before)
	}

	// Compaction must not lose anything.
	info, err := GetInfo(cachedir.At(dir), "c00000")
	if err != nil {
		t.Fatalf("GetInfo after compaction: %v", err)
	}
	if !info.IsSpam() {
		t.Error("classification lost during compaction")
	}
	if _, err := Read(cachedir.At(dir), "c01000"); err != nil {
		t.Errorf("message unreadable after compaction: %v", err)
	}
}

// See msg_damage_test.go for H6's damage-recovery regression tests
// (TestDamagedIndexIsTruncatedOnLoadSoAppendsSurvive,
// TestRecoverDamagedMessageInfo_ReVerifiesUnderLockBeforeRewriting) and the
// keysOfTest helper they share.

func fileSize(t *testing.T, path string) int64 {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	return fi.Size()
}

// Concurrent writers share one index; this is the case the in-process mutex and
// the directory lock exist for. Run with -race.
// Microsoft Graph message IDs run to 152 characters and share an 87-character
// prefix within a mailbox. The index key used to be a 36-byte truncation, so
// every Outlook message in an account addressed the same entry: storing one
// overwrote the last, and Exists then answered false for all but the most
// recent. This is that failure, at the level a caller sees it.
func TestLongIDsSharingAPrefixDoNotCollide(t *testing.T) {
	const sharedPrefix = "AAMkADFlYzVjM2IwLTI1ZTAtNGYxYS1hMDQ3LTBjZDE3NThkNjhlMwBGAAAAAADfKW0_Zwj2SpBWkA7Zym1DBwCf"

	ids := []email.ID{sharedPrefix + "AAA=", sharedPrefix + "BBB=", sharedPrefix + "CCC="}
	dates := []time.Time{
		time.Date(2025, 1, 5, 0, 0, 0, 0, time.UTC),
		time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC),
		time.Date(2025, 11, 25, 0, 0, 0, 0, time.UTC),
	}

	dir := t.TempDir()
	for i, id := range ids {
		if err := Store(cachedir.At(dir), id, []byte(sampleRaw), dates[i]); err != nil {
			t.Fatalf("Store(%s): %v", id[len(id)-6:], err)
		}
	}

	for i, id := range ids {
		exists, err := Exists(cachedir.At(dir), id)
		if err != nil {
			t.Fatalf("Exists: %v", err)
		}
		if !exists {
			t.Errorf("message ending %q is not cached; its entry was overwritten by another long ID",
				id[len(id)-6:])
			continue
		}
		info, err := GetInfo(cachedir.At(dir), id)
		if err != nil {
			t.Errorf("GetInfo(%q): %v", id[len(id)-6:], err)
			continue
		}
		// Each must keep its own date, or lookups resolve to another message.
		if info.Date.Month() != dates[i].Month() || info.Date.Day() != dates[i].Day() {
			t.Errorf("message ending %q has date %v, want %v: entries were merged",
				id[len(id)-6:], info.Date, dates[i])
		}
		if _, err := Read(cachedir.At(dir), id); err != nil {
			t.Errorf("message ending %q is unreadable: %v", id[len(id)-6:], err)
		}
	}

	var count int
	ForEachID(cachedir.At(dir), func(email.ID) bool {
		count++
		return false
	})
	if count != len(ids) {
		t.Errorf("index holds %d IDs, want %d", count, len(ids))
	}
}

func TestExternalDiskAppendPreservedDuringDeleteAndCompaction(t *testing.T) {
	dir := t.TempDir()
	d1 := time.Date(2026, 9, 18, 10, 0, 0, 0, time.UTC)
	mustStore(t, dir, "msg001", d1)

	// Ensure msg001 is loaded in cache
	ex, err := Exists(cachedir.At(dir), "msg001")
	if err != nil || !ex {
		t.Fatalf("Exists msg001 failed: %v", err)
	}

	// Simulate external process writing msg002 directly to disk
	d2 := time.Date(2026, 9, 18, 11, 0, 0, 0, time.UTC)
	msg2Dir := filepath.Join(dir, "messages", "2026", "09", "18")
	if mkdirErr := os.MkdirAll(msg2Dir, 0700); mkdirErr != nil {
		t.Fatalf("mkdir failed: %v", mkdirErr)
	}
	msg2Path := filepath.Join(msg2Dir, "msg002.eml")
	if writeErr := os.WriteFile(msg2Path, []byte("external msg2 content"), 0600); writeErr != nil {
		t.Fatalf("write msg002.eml failed: %v", writeErr)
	}

	binP := binPath(dir)
	y, m, d := d2.Date()
	c := message_info.Compact{
		Date:  uint32(y*10000 + int(m)*100 + d),
		Flags: 0,
	}
	if appendErr := message_info.AppendToFile(binP, "msg002", c); appendErr != nil {
		t.Fatalf("external AppendToFile failed: %v", appendErr)
	}

	// Invalidation check: Exists and Read must detect msg002 from updated message_info.bin
	ex2, err := Exists(cachedir.At(dir), "msg002")
	if err != nil || !ex2 {
		t.Fatalf("Exists msg002 failed to detect external append: %v", err)
	}
	raw, err := Read(cachedir.At(dir), "msg002")
	if err != nil || string(raw) != "external msg2 content" {
		t.Fatalf("Read msg002 returned %q, error: %v", string(raw), err)
	}

	// Mutating operation: Delete msg001 (triggers compaction via compactMapLocked)
	if deleteErr := Delete(cachedir.At(dir), "msg001"); deleteErr != nil {
		t.Fatalf("Delete msg001 failed: %v", deleteErr)
	}

	// Crucial check: msg002 must NOT be clobbered by compaction
	exAfter, err := Exists(cachedir.At(dir), "msg002")
	if err != nil || !exAfter {
		t.Fatalf("msg002 was clobbered by compaction after delete: %v", err)
	}
	rawAfter, err := Read(cachedir.At(dir), "msg002")
	if err != nil || string(rawAfter) != "external msg2 content" {
		t.Fatalf("msg002 payload unreadable after compaction: %v", err)
	}
}

func TestStoreBatch_BulkIngestion(t *testing.T) {
	dir := t.TempDir()
	batch, err := NewStoreBatch(cachedir.At(dir))
	if err != nil {
		t.Fatalf("NewStoreBatch failed: %v", err)
	}

	d1 := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	d2 := time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC)

	if err := batch.Store("batch-msg-1", []byte("batch body 1"), d1); err != nil {
		t.Fatalf("batch.Store 1 failed: %v", err)
	}
	if err := batch.Store("batch-msg-2", []byte("batch body 2"), d2); err != nil {
		t.Fatalf("batch.Store 2 failed: %v", err)
	}
	if err := batch.Close(); err != nil {
		t.Fatalf("batch.Close failed: %v", err)
	}

	// Verify both exist and are readable
	for _, id := range []email.ID{"batch-msg-1", "batch-msg-2"} {
		exists, err := Exists(cachedir.At(dir), id)
		if err != nil || !exists {
			t.Errorf("Exists(%q) failed: %v", id, err)
		}
		raw, err := Read(cachedir.At(dir), id)
		if err != nil || len(raw) == 0 {
			t.Errorf("Read(%q) failed: %v", id, err)
		}
		info, err := GetInfo(cachedir.At(dir), id)
		if err != nil || info == nil {
			t.Errorf("GetInfo(%q) failed: %v", id, err)
		}
	}
}

func TestReadParsedSummaryAndInfo(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	if err := Store(cachedir.At(dir), "summary-test-1", []byte("From: test\n\nbody"), now); err != nil {
		t.Fatalf("Store failed: %v", err)
	}
	summaryJSON := []byte(`{"id":"summary-test-1","subject":"Test Subject"}`)
	if err := StoreParsedSummary(cachedir.At(dir), "summary-test-1", summaryJSON); err != nil {
		t.Fatalf("StoreParsedSummary failed: %v", err)
	}
	if err := SetEvidence(cachedir.At(dir), "summary-test-1", Evidence{Checks: CheckBogofilter, BogoScore: 9.5}); err != nil {
		t.Fatalf("SetEvidence failed: %v", err)
	}

	data, info, err := ReadParsedSummaryAndInfo(cachedir.At(dir), "summary-test-1")
	if err != nil {
		t.Fatalf("ReadParsedSummaryAndInfo failed: %v", err)
	}
	if string(data) != string(summaryJSON) {
		t.Errorf("expected summary JSON %s, got %s", string(summaryJSON), string(data))
	}
	if info == nil || !info.IsSpam() || info.Evidence.BogoScore != 9.5 {
		t.Errorf("unexpected info: %+v", info)
	}

	// Non-existent message returns error
	if _, _, err := ReadParsedSummaryAndInfo(cachedir.At(dir), "missing-id"); err == nil {
		t.Error("expected error for missing ID, got nil")
	}
}
