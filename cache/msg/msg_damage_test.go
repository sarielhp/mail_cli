package msg

import (
	"bytes"
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"os"
	"testing"
	"time"

	"github.com/sarielhp/mail_cli/message_info"
)

// TestDamagedIndexIsTruncatedOnLoadSoAppendsSurvive is H6's regression: a
// corrupted byte partway through message_info.bin must not silently drop
// every record appended after it. Loading the damaged file has to rewrite it
// from the salvaged records immediately, so the next append lands after a
// clean tail instead of after garbage that a later compaction would discard
// along with everything past it.
func TestDamagedIndexIsTruncatedOnLoadSoAppendsSurvive(t *testing.T) {
	dir := t.TempDir()
	binFile := binPath(dir)

	var buf bytes.Buffer
	if err := message_info.WriteOne(&buf, "good1", message_info.Compact{Date: 20260101, Flags: message_info.FlagScanned}); err != nil {
		t.Fatalf("WriteOne: %v", err)
	}
	buf.WriteByte(0xFF) // damage: not a valid version byte
	if err := os.WriteFile(binFile, buf.Bytes(), 0600); err != nil {
		t.Fatalf("seeding damaged index: %v", err)
	}

	// Loading must salvage "good1" despite the damage.
	info, err := GetInfo(cachedir.At(dir), "good1")
	if err != nil {
		t.Fatalf("GetInfo on damaged index: %v", err)
	}
	if info.Date.Year() != 2026 {
		t.Errorf("salvaged record has wrong date: %+v", info)
	}

	// The file on disk must now be clean -- re-reading it directly (a fresh
	// process, unaware of the in-memory cache) must not see the damage.
	reread, err := message_info.ReadFromFile(binFile)
	if err != nil {
		t.Fatalf("expected damaged index to be truncated on load, re-read failed: %v", err)
	}
	if len(reread) != 1 {
		t.Fatalf("expected exactly the salvaged record on disk, got %v", keysOfTest(reread))
	}

	// An append after the damaged load must be readable back: it landed after
	// a clean tail, not after the garbage byte.
	mustStore(t, dir, "good2", time.Date(2026, 2, 2, 0, 0, 0, 0, time.UTC))

	final, err := message_info.ReadFromFile(binFile)
	if err != nil {
		t.Fatalf("ReadFromFile after append: %v", err)
	}
	if _, ok := final["good1"]; !ok {
		t.Error("good1 lost after the damaged-index append")
	}
	if _, ok := final["good2"]; !ok {
		t.Error("good2 (appended after the damaged-index load) was not persisted")
	}
}

// TestRecoverDamagedMessageInfo_ReVerifiesUnderLockBeforeRewriting is a
// regression for a race the H6 fix itself introduced: the first, unlocked
// read that reports ErrDamaged can be seeing nothing more than another
// process's legitimate append still in flight, not real corruption. This
// checks the safety property that closes that race: recoverDamagedMessageInfo
// re-reads under the cross-process lock before deciding anything is really
// damaged, so a file that turns out to be clean on that second read is left
// untouched rather than truncated from a stale, incomplete salvage.
func TestRecoverDamagedMessageInfo_ReVerifiesUnderLockBeforeRewriting(t *testing.T) {
	dir := t.TempDir()
	binFile := binPath(dir)

	var buf bytes.Buffer
	if err := message_info.WriteOne(&buf, "good1", message_info.Compact{Date: 20260101}); err != nil {
		t.Fatalf("WriteOne: %v", err)
	}
	if err := message_info.WriteOne(&buf, "good2", message_info.Compact{Date: 20260202}); err != nil {
		t.Fatalf("WriteOne: %v", err)
	}
	// A clean file -- not damaged -- as if another process's in-flight
	// append had completed by the time the lock is acquired.
	clean := buf.Bytes()
	if err := os.WriteFile(binFile, clean, 0600); err != nil {
		t.Fatalf("seeding clean index: %v", err)
	}

	m, err := recoverDamagedMessageInfo(dir, binFile)
	if err != nil {
		t.Fatalf("recoverDamagedMessageInfo: %v", err)
	}
	if len(m) != 2 {
		t.Fatalf("expected both records from the clean re-read, got %v", keysOfTest(m))
	}

	after, err := os.ReadFile(binFile)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !bytes.Equal(after, clean) {
		t.Errorf("expected the clean file left untouched by recovery, got a different file on disk")
	}
}

func keysOfTest(m map[string]message_info.Compact) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
