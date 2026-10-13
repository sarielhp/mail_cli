package message_info

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"
)

// outlookID builds an ID shaped like a real Microsoft Graph one: 152
// characters, of which the first 87 are shared by every message in the same
// mailbox. Version 1 packed IDs into a fixed [36]byte, so all of these
// collapsed onto one key.
func outlookID(suffix string) string {
	const sharedPrefix = "AAMkADFlYzVjM2IwLTI1ZTAtNGYxYS1hMDQ3LTBjZDE3NThkNjhlMwBGAAAAAADfKW0_Zwj2SpBWkA7Zym1DBwCf"
	id := sharedPrefix + suffix
	for len(id) < 152 {
		id += "A"
	}
	return id
}

func TestLongIDsWithSharedPrefixStaySeparate(t *testing.T) {
	entries := map[string]Compact{
		outlookID("aaa"): {Date: 20250101, Flags: FlagScanned},
		outlookID("bbb"): {Date: 20250202, Flags: FlagScanned, Checks: 1},
		outlookID("ccc"): {Date: 20250303},
	}

	var buf bytes.Buffer
	for id, c := range entries {
		if err := WriteOne(&buf, id, c); err != nil {
			t.Fatalf("WriteOne: %v", err)
		}
	}

	got, err := ReadAll(&buf)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(got) != len(entries) {
		t.Fatalf("read back %d entries, want %d: long IDs sharing a prefix collided",
			len(got), len(entries))
	}
	for id, want := range entries {
		have, ok := got[id]
		if !ok {
			t.Errorf("ID ending %q is missing after a round trip", id[len(id)-10:])
			continue
		}
		if have.Date != want.Date || have.Flags != want.Flags {
			t.Errorf("ID ending %q: got %+v, want %+v", id[len(id)-10:], have, want)
		}
	}
}

func TestRoundTripPreservesIDsOfEveryLength(t *testing.T) {
	tests := []struct {
		name string
		id   string
	}{
		{name: "gmail hex", id: "19f3a8216d52dfc6"},
		{name: "jmap short", id: "Stmc-pdHF44-"},
		{name: "exactly the old 36-byte limit", id: "123456789012345678901234567890123456"},
		{name: "one past the old limit", id: "1234567890123456789012345678901234567"},
		{name: "outlook length", id: outlookID("zzz")},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			want := Compact{Date: 20240612, Flags: FlagScanned, Override: OverrideSpam, Checks: 5, BogoScore: 0.25, Stamp: 77}

			var buf bytes.Buffer
			if err := WriteOne(&buf, tc.id, want); err != nil {
				t.Fatalf("WriteOne: %v", err)
			}
			got, err := ReadAll(&buf)
			if err != nil {
				t.Fatalf("ReadAll: %v", err)
			}
			have, ok := got[tc.id]
			if !ok {
				t.Fatalf("ID did not survive the round trip; got keys %v", keysOf(got))
			}
			if have != want {
				t.Errorf("got %+v, want %+v", have, want)
			}
		})
	}
}

// The score is a float on disk now. Version 1 wrote uint32(score).
func TestBogoScoreKeepsItsFraction(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteOne(&buf, "m1", Compact{Date: 20250101, BogoScore: 6.75}); err != nil {
		t.Fatalf("WriteOne: %v", err)
	}
	got, err := ReadAll(&buf)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if got["m1"].BogoScore != 6.75 {
		t.Errorf("BogoScore = %v, want 6.75", got["m1"].BogoScore)
	}
}

// writeLegacyRecord produces a version 1 record, so migration can be tested
// without a stale file checked into the repository.
func writeLegacyRecord(buf *bytes.Buffer, id string, date uint32, flags uint8, score int32) {
	rec := make([]byte, legacyEntrySize)
	rec[0] = legacyVersion
	copy(rec[1:1+legacyIDSize], id)
	binary.BigEndian.PutUint32(rec[37:41], date)
	rec[41] = flags
	binary.BigEndian.PutUint32(rec[42:46], uint32(score))
	buf.Write(rec)
}

// An index written before the format change must still load, or every user
// silently loses their cache on upgrade.
func TestLegacyRecordsStillLoad(t *testing.T) {
	var buf bytes.Buffer
	writeLegacyRecord(&buf, "19f3a8216d52dfc6", 20230405, v2IsSpam|v2Classified, 4)
	writeLegacyRecord(&buf, "Stmc-pdHF44-", 20240506, 0, 0)

	got, err := ReadAll(&buf)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("read %d legacy entries, want 2: %v", len(got), keysOf(got))
	}
	first, ok := got["19f3a8216d52dfc6"]
	if !ok {
		t.Fatal("legacy Gmail ID did not survive; its zero padding was not trimmed")
	}
	// A version 1 verdict had no source, so it is dropped and re-scanned.
	if first.Date != 20230405 || first.Override != OverrideNone || first.Flags&FlagScanned != 0 {
		t.Errorf("legacy entry = %+v", first)
	}
	if got["Stmc-pdHF44-"].Date != 20240506 {
		t.Errorf("second legacy entry = %+v", got["Stmc-pdHF44-"])
	}
}

// Both versions can sit in one file: an existing index is appended to before it
// is ever compacted.
func TestMixedVersionsInOneFile(t *testing.T) {
	var buf bytes.Buffer
	writeLegacyRecord(&buf, "oldmsg1", 20220101, 0, 0)
	if err := WriteOne(&buf, outlookID("new"), Compact{Date: 20250707}); err != nil {
		t.Fatalf("WriteOne: %v", err)
	}
	writeLegacyRecord(&buf, "oldmsg2", 20220202, 0, 0)

	got, err := ReadAll(&buf)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("read %d entries, want 3: %v", len(got), keysOf(got))
	}
	if got[outlookID("new")].Date != 20250707 {
		t.Error("the version 2 record between two version 1 records was lost")
	}
	if got["oldmsg2"].Date != 20220202 {
		t.Error("the version 1 record after a version 2 record was lost")
	}
}

// A later write of the same ID wins, which is what makes the log append-only.
func TestLaterRecordWins(t *testing.T) {
	var buf bytes.Buffer
	for _, date := range []uint32{20200101, 20210202, 20220303} {
		if err := WriteOne(&buf, "m1", Compact{Date: date}); err != nil {
			t.Fatalf("WriteOne: %v", err)
		}
	}
	got, err := ReadAll(&buf)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if got["m1"].Date != 20220303 {
		t.Errorf("Date = %d, want the last write 20220303", got["m1"].Date)
	}
}

func TestWriteOneRejectsUnusableIDs(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteOne(&buf, "", Compact{}); err == nil {
		t.Error("WriteOne accepted an empty ID")
	}
	huge := make([]byte, maxIDLen+1)
	for i := range huge {
		huge[i] = 'x'
	}
	if err := WriteOne(&buf, string(huge), Compact{}); err == nil {
		t.Error("WriteOne accepted an ID past the sanity bound")
	}
}

// A truncated tail must not lose the records that came before it, but it
// must also be reported (ErrDamaged) rather than mistaken for a clean
// end-of-file -- see H6: a caller that cannot tell the difference has no
// chance to truncate the damage away before the next append lands after it.
func TestTruncatedFileKeepsEarlierRecords(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteOne(&buf, "good1", Compact{Date: 20250101}); err != nil {
		t.Fatalf("WriteOne: %v", err)
	}
	if err := WriteOne(&buf, "good2", Compact{Date: 20250202}); err != nil {
		t.Fatalf("WriteOne: %v", err)
	}
	full := buf.Bytes()
	truncated := full[:len(full)-5]

	got, err := ReadAll(bytes.NewReader(truncated))
	if err != ErrDamaged {
		t.Fatalf("ReadAll: expected ErrDamaged, got %v", err)
	}
	if _, ok := got["good1"]; !ok {
		t.Error("a complete record before the truncation was lost")
	}
}

func TestFileRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "message_info.bin")
	id := outlookID("file")

	if err := AppendToFile(path, id, Compact{Date: 20250808, BogoScore: 1.5}); err != nil {
		t.Fatalf("AppendToFile: %v", err)
	}
	got, err := ReadFromFile(path)
	if err != nil {
		t.Fatalf("ReadFromFile: %v", err)
	}
	if got[id].Date != 20250808 || got[id].BogoScore != 1.5 {
		t.Errorf("entry = %+v", got[id])
	}

	// RewriteFile replaces the log, and must be readable afterwards.
	if rewriteErr := RewriteFile(path, map[string]Compact{id: {Date: 20250909}}); rewriteErr != nil {
		t.Fatalf("RewriteFile: %v", rewriteErr)
	}
	got, err = ReadFromFile(path)
	if err != nil {
		t.Fatalf("ReadFromFile after rewrite: %v", err)
	}
	if len(got) != 1 || got[id].Date != 20250909 {
		t.Errorf("after rewrite: %v", got)
	}
}

func TestReadFromMissingFileIsEmpty(t *testing.T) {
	got, err := ReadFromFile(filepath.Join(t.TempDir(), "absent.bin"))
	if err != nil {
		t.Fatalf("ReadFromFile: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d entries for a missing file", len(got))
	}
}

func TestRewriteFileLeavesNoTempBehind(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "message_info.bin")
	if err := RewriteFile(path, map[string]Compact{"m1": {Date: 20250101}}); err != nil {
		t.Fatalf("RewriteFile: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("directory holds %d files, want only the index", len(entries))
	}
}

func TestMessageInfo_BufferedReadWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "message_info.bin")

	entries := make(map[string]Compact, 1000)
	for i := 0; i < 1000; i++ {
		id := fmt.Sprintf("msg_%04d", i)
		entries[id] = Compact{Date: uint32(20260100 + (i % 30) + 1), Flags: FlagScanned, BogoScore: float32(i) * 0.01}
	}

	if err := RewriteFile(path, entries); err != nil {
		t.Fatalf("RewriteFile: %v", err)
	}

	got, err := ReadFromFile(path)
	if err != nil {
		t.Fatalf("ReadFromFile: %v", err)
	}
	if len(got) != len(entries) {
		t.Fatalf("expected %d entries, got %d", len(entries), len(got))
	}
	for id, want := range entries {
		have, ok := got[id]
		if !ok {
			t.Fatalf("entry %s missing", id)
		}
		if have.Date != want.Date || have.Flags != want.Flags || math.Abs(float64(have.BogoScore-want.BogoScore)) > 0.0001 {
			t.Errorf("entry %s: got %+v, want %+v", id, have, want)
		}
	}
}

// TestReadAll_DamagedRecordIsReportedAndSalvaged is H6's regression: a byte
// that is not a valid version tag must be reported as ErrDamaged, not
// swallowed as a clean end-of-file, and every record before it must still
// come back so the caller can decide how to recover.
func TestReadAll_DamagedRecordIsReportedAndSalvaged(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteOne(&buf, "good-id", Compact{Date: 20260101, Flags: FlagScanned}); err != nil {
		t.Fatalf("WriteOne: %v", err)
	}
	buf.WriteByte(0xFF) // a byte that is neither Version nor legacyVersion

	got, err := ReadAll(&buf)
	if err == nil {
		t.Fatal("expected ErrDamaged, got nil")
	}
	if err != ErrDamaged {
		t.Fatalf("expected ErrDamaged, got %v", err)
	}
	if len(got) != 1 || got["good-id"].Date != 20260101 {
		t.Fatalf("expected the record before the damage to be salvaged, got %+v", got)
	}
}

// TestReadAll_TruncatedRecordIsReportedAndSalvaged covers a torn write: a
// record whose version byte and ID length are present but whose payload was
// cut short, as a crash mid-append would leave it.
func TestReadAll_TruncatedRecordIsReportedAndSalvaged(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteOne(&buf, "good-id", Compact{Date: 20260101, Flags: FlagScanned}); err != nil {
		t.Fatalf("WriteOne: %v", err)
	}

	var torn bytes.Buffer
	if err := WriteOne(&torn, "torn-id", Compact{Date: 20260202}); err != nil {
		t.Fatalf("WriteOne: %v", err)
	}
	buf.Write(torn.Bytes()[:torn.Len()-2]) // drop the last two payload bytes

	got, err := ReadAll(&buf)
	if err != ErrDamaged {
		t.Fatalf("expected ErrDamaged, got %v", err)
	}
	if len(got) != 1 || got["good-id"].Date != 20260101 {
		t.Fatalf("expected only the record before the truncation to be salvaged, got %+v", got)
	}
}

func keysOf(m map[string]Compact) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		if len(k) > 20 {
			k = k[:10] + "..." + k[len(k)-7:]
		}
		out = append(out, k)
	}
	return out
}

// Forgetting evidence must keep the user's override and the message's other
// flags.
func TestWithoutEvidenceKeepsOverrideAndFlags(t *testing.T) {
	c := Compact{Date: 7, Flags: flagIsRead | flagHasICS | FlagScanned, Override: OverrideHam, Checks: 4, BogoScore: 0.9, Stamp: 3}
	got := c.WithoutEvidence()
	want := Compact{Date: 7, Flags: flagIsRead | flagHasICS, Override: OverrideHam}
	if got != want {
		t.Errorf("WithoutEvidence() = %+v, want %+v", got, want)
	}
}

// writeV2Record produces a version 2 record: a length-prefixed ID, then date,
// flags and score.
func writeV2Record(buf *bytes.Buffer, id string, date uint32, flags uint8) {
	buf.WriteByte(v2Version)
	_ = binary.Write(buf, binary.BigEndian, uint16(len(id)))
	buf.WriteString(id)
	_ = binary.Write(buf, binary.BigEndian, date)
	buf.WriteByte(flags)
	_ = binary.Write(buf, binary.BigEndian, math.Float32bits(20))
}

// Version 2 kept one verdict. Only the user's -- source 3 in the top bits --
// becomes an override; a verdict the scan made is dropped so it is re-scanned
// under the new checks. Read state survives either way.
func TestV2RecordsKeepOnlyTheUsersVerdict(t *testing.T) {
	const user = 3 << 6
	var buf bytes.Buffer
	writeV2Record(&buf, "ham", 20250101, flagIsRead|v2Classified|user)
	writeV2Record(&buf, "spam", 20250101, v2Classified|v2IsSpam|user)
	writeV2Record(&buf, "scanned", 20250101, v2Classified|v2IsSpam|1<<6)

	got, err := ReadAll(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if c := got["ham"]; c.Override != OverrideHam || c.Flags != flagIsRead {
		t.Errorf("user ham = %+v", c)
	}
	if c := got["spam"]; c.Override != OverrideSpam {
		t.Errorf("user spam = %+v", c)
	}
	if c := got["scanned"]; c.Override != OverrideNone || c.Flags&FlagScanned != 0 {
		t.Errorf("a scan verdict survived conversion: %+v", c)
	}
}

// The stats count every record in the log, superseded and old-format ones
// included, which is what tells `cache repair` a rewrite would help.
func TestReadFileStatsCountsTheLog(t *testing.T) {
	var buf bytes.Buffer
	writeV2Record(&buf, "a", 20250404, 0)
	_ = WriteOne(&buf, "a", Compact{Date: 20250404, Override: OverrideHam})
	_ = WriteOne(&buf, "b", Compact{Date: 20250405})
	path := filepath.Join(t.TempDir(), "message_info.bin")
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}

	m, stats, err := ReadFileStats(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(m) != 2 || stats != (Stats{Records: 3, Old: 1}) {
		t.Errorf("got %d messages, %+v", len(m), stats)
	}
}
