package spam

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
	"github.com/sarielhp/mail_cli/label"
	"github.com/sarielhp/mail_cli/mailclient"
	"github.com/sarielhp/mail_cli/report"
	"github.com/sarielhp/mail_cli/train"
)

// fakeBogofilter puts a bogofilter on PATH that accepts anything, so the test
// never touches the user's real training database.
func fakeBogofilter(t *testing.T) {
	t.Helper()
	bogofiltertest.Install(t, "#!/bin/sh\ncat >/dev/null\nexit 0\n")
}

// LearnHam must read and record in the account's cache. It used the root
// download directory, which holds no messages: every message was "not found",
// nothing was trained, and all of them were recorded as trained anyway -- so
// later runs skipped them, and the spam and political labels a scan had
// cached stayed on mail the user had just called ham.
func TestLearnHam_TrainsFromAccountCacheAndClearsLabels(t *testing.T) {
	fakeBogofilter(t)
	root := t.TempDir()
	accountDir := filepath.Join(root, "fastmail")
	raw := []byte("Subject: hello\r\n\r\nbody\r\n")
	for _, id := range []string{"m1", "m2"} {
		if err := msg.Store(cachedir.At(accountDir), email.ID(id), raw, time.Now()); err != nil {
			t.Fatal(err)
		}
		// A verdict an earlier scan cached.
		if err := msg.SetEvidence(cachedir.At(accountDir), email.ID(id), msg.Evidence{Checks: msg.CheckBogofilter | msg.CheckPolitical, BogoScore: 20}); err != nil {
			t.Fatal(err)
		}
	}

	// The session config knows only the root; the client's names the account.
	config := &cfg_g.Config{}
	config.CacheRoot = root
	accountCfg := *config
	accountCfg.AccountCache = cachedir.At(accountDir)
	client := &mailclient.MockMailClient{Cfg: &accountCfg, DownloadedIDs: map[string][]email.ID{"Archive": {"m1", "m2", "missing"}}}

	if err := LearnHam(report.Discard(), client, config, label.FromServer("Archive")); err != nil {
		t.Fatal(err)
	}

	ledger, err := train.Open(train.HamPath(cachedir.At(accountDir)))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, id := range ledger.IDs() {
		got[id] = true
	}
	if !got["m1"] || !got["m2"] {
		t.Errorf("trained messages not recorded in the account's ledger: %v", ledger.IDs())
	}
	if got["missing"] {
		t.Error("a message that was never read was recorded as trained")
	}
	if _, err := os.Stat(train.HamPath(cachedir.At(root))); err == nil {
		t.Error("the ledger was written at the cache root")
	}
	for _, id := range []string{"m1", "m2"} {
		info, err := msg.GetInfo(cachedir.At(accountDir), email.ID(id))
		if err != nil {
			t.Fatal(err)
		}
		if info.IsSpam() || info.IsPolitical() {
			t.Errorf("%s still labelled spam=%v political=%v after ham training", id, info.IsSpam(), info.IsPolitical())
		}
	}
}
