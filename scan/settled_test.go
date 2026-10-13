package scan

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sarielhp/mail_cli/bogofiltertest"
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"github.com/sarielhp/mail_cli/cache/msg"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/report"
)

// hamBogofilter puts a bogofilter on PATH that calls everything ham, so a test
// can tell a fresh classification from a reused one without the real filter.
func hamBogofilter(t *testing.T) {
	t.Helper()
	bogofiltertest.Install(t, "#!/bin/sh\ncat >/dev/null\necho 'X-Bogosity: Ham, tests=bogofilter, spamicity=0.010000'\nexit 1\n")
}

// Evidence computed against the current filter and settings is reused;
// evidence with another stamp is judged again; a message the user decided is
// never scanned. Training changes the stamp, which is how a `spam learn`
// re-judges mail scanned before it.
func TestScanEmails_RescansOnlyStaleEvidence(t *testing.T) {
	hamBogofilter(t)
	t.Setenv("BOGOFILTER_DIR", t.TempDir())
	account := cachedir.At(filepath.Join(t.TempDir(), "acct"))
	raw := []byte("Date: Mon, 2 Jan 2026 15:04:05 -0700\nFrom: colleague@example.org\nSubject: Seminar\n\nTalk on Friday.")
	config := &cfg_g.Config{SelectedAccount: "acct", AccountCache: account}
	config.AllowedLanguages = []string{"english"}
	stamp := Stamp(config)

	for _, id := range []email.ID{"stale", "current", "decided"} {
		if err := msg.Store(account, id, raw, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	spamEvidence := msg.Evidence{Checks: msg.CheckBogofilter, BogoScore: 0.99}
	spamEvidence.Stamp = stamp + 1
	_ = msg.SetEvidence(account, "stale", spamEvidence)
	spamEvidence.Stamp = stamp
	_ = msg.SetEvidence(account, "current", spamEvidence)
	_ = msg.SetOverride(account, "decided", msg.OverrideSpam)

	if _, _, _, err := Emails(report.Discard(), []email.ID{"stale", "current", "decided"}, config, ""); err != nil {
		t.Fatal(err)
	}

	if info, _ := msg.GetInfo(account, "stale"); info.IsSpam() || info.Evidence.Stamp != stamp {
		t.Errorf("stale evidence was reused: %+v", info)
	}
	if info, _ := msg.GetInfo(account, "current"); !info.IsSpam() {
		t.Errorf("current evidence was not reused: %+v", info)
	}
	if info, _ := msg.GetInfo(account, "decided"); !info.IsSpam() || info.Scanned {
		t.Errorf("a message the user decided was scanned: %+v", info)
	}
}

// The stamp moves with the filter database and with every setting a check
// reads, and with nothing else.
func TestStampTracksFilterAndSettings(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BOGOFILTER_DIR", dir)
	db := filepath.Join(dir, "wordlist.db")
	if err := os.WriteFile(db, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	config := &cfg_g.Config{}
	config.AllowedLanguages = []string{"English", "hebrew"}
	base := Stamp(config)

	reordered := &cfg_g.Config{}
	reordered.AllowedLanguages = []string{"hebrew", "english"}
	if Stamp(reordered) != base {
		t.Error("the order or case of allowed_languages changed the stamp")
	}

	config.BlockPolitical = true
	if Stamp(config) == base {
		t.Error("block_political did not change the stamp")
	}
	config.BlockPolitical = false

	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(db, later, later); err != nil {
		t.Fatal(err)
	}
	if Stamp(config) == base {
		t.Error("training the filter did not change the stamp")
	}
}
