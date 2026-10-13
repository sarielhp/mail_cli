package msg

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sarielhp/mail_cli/cache/cachedir"
	"github.com/sarielhp/mail_cli/email"
)

func storeOne(t *testing.T, dir string, id email.ID) cachedir.Account {
	t.Helper()
	mustStore(t, dir, id, time.Date(2025, 4, 4, 0, 0, 0, 0, time.UTC))
	return cachedir.At(dir)
}

// The scan and the user each write their own record, so neither can undo the
// other. One shared verdict is how a move after an unspam -- clearing the
// verdict for the scan's sake -- relabelled mail the user had called ham.
func TestEvidenceAndOverrideDoNotOverwriteEachOther(t *testing.T) {
	acc := storeOne(t, t.TempDir(), "m")
	if err := SetOverride(acc, "m", OverrideHam); err != nil {
		t.Fatal(err)
	}
	if err := SetEvidence(acc, "m", Evidence{Checks: CheckLanguage, BogoScore: 0.01, Stamp: 7}); err != nil {
		t.Fatal(err)
	}
	info, err := GetInfo(acc, "m")
	if err != nil {
		t.Fatal(err)
	}
	if info.Override != OverrideHam || info.IsSpam() {
		t.Errorf("a scan undid the user's ham: %+v", info)
	}
	if !info.Scanned || info.Evidence.Checks != CheckLanguage || info.Evidence.Stamp != 7 {
		t.Errorf("the evidence was not kept: %+v", info)
	}

	if err := ForgetEvidence(acc, "m"); err != nil {
		t.Fatal(err)
	}
	info, _ = GetInfo(acc, "m")
	if info.Scanned || info.Override != OverrideHam {
		t.Errorf("forgetting evidence lost the override: %+v", info)
	}
}

func TestVerdictDerivation(t *testing.T) {
	scanned := func(c Checks) Info { return Info{Scanned: true, Evidence: Evidence{Checks: c}} }
	cases := []struct {
		name                  string
		info                  Info
		spam, political, list bool
	}{
		{"unscanned", Info{}, false, false, false},
		{"clean", scanned(0), false, false, false},
		{"bogofilter", scanned(CheckBogofilter), true, false, false},
		{"language", scanned(CheckLanguage), true, false, false},
		{"political", scanned(CheckPolitical), true, true, false},
		{"blacklisted", scanned(CheckBlacklist), true, false, true},
		{"whitelist outranks checks", scanned(CheckWhitelist | CheckLanguage), false, false, false},
		{"user ham outranks checks", Info{Scanned: true, Evidence: Evidence{Checks: CheckPolitical}, Override: OverrideHam}, false, false, false},
		{"user spam without evidence", Info{Override: OverrideSpam}, true, false, false},
	}
	for _, tc := range cases {
		if got := tc.info.IsSpam(); got != tc.spam {
			t.Errorf("%s: IsSpam = %v, want %v", tc.name, got, tc.spam)
		}
		if got := tc.info.IsPolitical(); got != tc.political {
			t.Errorf("%s: IsPolitical = %v, want %v", tc.name, got, tc.political)
		}
		if got := tc.info.IsBlacklisted(); got != tc.list {
			t.Errorf("%s: IsBlacklisted = %v, want %v", tc.name, got, tc.list)
		}
	}
}

// A message is judged again when its evidence is missing or was computed
// against another filter or other settings -- never when the user decided.
func TestNeedsScan(t *testing.T) {
	cases := []struct {
		name string
		info Info
		want bool
	}{
		{"never scanned", Info{}, true},
		{"current", Info{Scanned: true, Evidence: Evidence{Stamp: 5}}, false},
		{"stale", Info{Scanned: true, Evidence: Evidence{Stamp: 4}}, true},
		{"user decided", Info{Override: OverrideHam}, false},
	}
	for _, tc := range cases {
		if got := tc.info.NeedsScan(5); got != tc.want {
			t.Errorf("%s: NeedsScan = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// Both records survive a reload, so a later process still knows them.
func TestEvidenceAndOverrideSurviveReload(t *testing.T) {
	dir := t.TempDir()
	acc := storeOne(t, dir, "m")
	_ = SetEvidence(acc, "m", Evidence{Checks: CheckBogofilter | CheckPolitical, BogoScore: 0.97, Stamp: 42})
	_ = SetOverride(acc, "m", OverrideSpam)

	dst := filepath.Join(t.TempDir(), "copy")
	if err := os.CopyFS(dst, os.DirFS(dir)); err != nil {
		t.Fatal(err)
	}
	info, err := GetInfo(cachedir.At(dst), "m")
	if err != nil {
		t.Fatal(err)
	}
	want := Evidence{Checks: CheckBogofilter | CheckPolitical, BogoScore: 0.97, Stamp: 42}
	if info.Evidence != want || info.Override != OverrideSpam || !info.Scanned {
		t.Errorf("after reload: %+v", info)
	}
}

// Rewriting the same record changes nothing on disk.
func TestSetOverrideIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	acc := storeOne(t, dir, "m")
	_ = SetOverride(acc, "m", OverrideHam)
	before, _ := os.Stat(filepath.Join(dir, "message_info.bin"))
	_ = SetOverride(acc, "m", OverrideHam)
	after, _ := os.Stat(filepath.Join(dir, "message_info.bin"))
	if before.Size() != after.Size() {
		t.Errorf("an unchanged override was appended again: %d -> %d bytes", before.Size(), after.Size())
	}
}

// Storing a body again -- a re-download -- keeps what is known about the
// message. It reset the record, dropping the user's override with it.
func TestStoreAgainKeepsOverrideAndEvidence(t *testing.T) {
	dir := t.TempDir()
	acc := storeOne(t, dir, "m")
	_ = SetOverride(acc, "m", OverrideHam)
	_ = SetEvidence(acc, "m", Evidence{Checks: CheckLanguage, Stamp: 3})

	if err := Store(acc, "m", []byte(sampleRaw), time.Date(2025, 4, 4, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	batch, err := NewStoreBatch(acc)
	if err != nil {
		t.Fatal(err)
	}
	if storeErr := batch.Store("m", []byte(sampleRaw), time.Date(2025, 4, 4, 0, 0, 0, 0, time.UTC)); storeErr != nil {
		t.Fatal(storeErr)
	}
	if closeErr := batch.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}

	info, err := GetInfo(acc, "m")
	if err != nil {
		t.Fatal(err)
	}
	if info.Override != OverrideHam || !info.Scanned || info.Evidence.Stamp != 3 {
		t.Errorf("storing the body again lost the record: %+v", info)
	}
}
