package msg

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sarielhp/mail_cli/cache/cachedir"
	"github.com/sarielhp/mail_cli/email"
)

// A cache whose index has drifted from its files: an update that superseded a
// record, a missing file with and without the user's decision, a file with no
// record, and a file under another date than its record names.
func driftedCache(t *testing.T) (string, cachedir.Account) {
	t.Helper()
	dir := t.TempDir()
	acc := cachedir.At(dir)
	day := time.Date(2025, 4, 4, 0, 0, 0, 0, time.UTC)
	for _, id := range []string{"kept", "gone", "decided-gone", "misdated"} {
		mustStore(t, dir, email.ID(id), day)
	}
	_ = SetOverride(acc, "kept", OverrideHam)
	_ = SetOverride(acc, "decided-gone", OverrideSpam)

	msgs := filepath.Join(dir, "messages")
	for _, id := range []string{"gone", "decided-gone"} {
		if err := os.Remove(filepath.Join(msgs, "2025/04/04", id+".eml")); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(msgs, "2025/04/05"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(msgs, "2025/04/04/misdated.eml"), filepath.Join(msgs, "2025/04/05/misdated.eml")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(msgs, "2025/04/05/stray.eml"), []byte(sampleRaw), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir, acc
}

func TestRepairReportsWithoutChanging(t *testing.T) {
	dir, acc := driftedCache(t)
	before, _ := os.ReadFile(filepath.Join(dir, "message_info.bin"))

	rep, err := Repair(acc, false)
	if err != nil {
		t.Fatal(err)
	}
	want := RepairReport{Records: 4, Superseded: 2, Orphans: 1, KeptOrphans: 1, Unindexed: 1, Misdated: 1, Decided: 2}
	if rep != want {
		t.Errorf("report = %+v, want %+v", rep, want)
	}
	after, _ := os.ReadFile(filepath.Join(dir, "message_info.bin"))
	if !bytes.Equal(before, after) {
		t.Error("a report-only repair changed the index")
	}
}

func TestRepairRewritesTheIndex(t *testing.T) {
	_, acc := driftedCache(t)
	if _, err := Repair(acc, true); err != nil {
		t.Fatal(err)
	}

	if _, err := GetInfo(acc, "gone"); err == nil {
		t.Error("a record with no file and no decision was kept")
	}
	if info, err := GetInfo(acc, "decided-gone"); err != nil || info.Override != OverrideSpam {
		t.Errorf("the user's decision on a missing file was dropped: %+v, %v", info, err)
	}
	if info, _ := GetInfo(acc, "kept"); info == nil || info.Override != OverrideHam {
		t.Errorf("a healthy record lost its decision: %+v", info)
	}
	for _, id := range []string{"misdated", "stray"} {
		if _, err := Read(acc, email.ID(id)); err != nil {
			t.Errorf("%s is still unreachable: %v", id, err)
		}
	}

	again, err := Repair(acc, false)
	if err != nil {
		t.Fatal(err)
	}
	if again.NeedsRewrite() {
		t.Errorf("a repaired index still needs repair: %+v", again)
	}
}

// A message filed under two dates keeps the record that points at one of its
// copies; the other copy is counted, not removed.
func TestRepairKeepsTheRecordedCopyOfADuplicate(t *testing.T) {
	dir := t.TempDir()
	acc := cachedir.At(dir)
	mustStore(t, dir, "twice", time.Date(2025, 4, 4, 0, 0, 0, 0, time.UTC))
	other := filepath.Join(dir, "messages/2025/01/01")
	if err := os.MkdirAll(other, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(other, "twice.eml"), []byte(sampleRaw), 0o600); err != nil {
		t.Fatal(err)
	}

	rep, err := Repair(acc, true)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Duplicates != 1 || rep.Misdated != 0 || rep.NeedsRewrite() {
		t.Errorf("report = %+v", rep)
	}
	if info, _ := GetInfo(acc, "twice"); info == nil || info.Date.Day() != 4 {
		t.Errorf("the record moved to the other copy: %+v", info)
	}
}
