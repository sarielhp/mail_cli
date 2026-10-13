package mailclient

import (
	"testing"
	"time"

	"github.com/sarielhp/mail_cli/cache/cachedir"
	"github.com/sarielhp/mail_cli/cache/msg"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/label"
)

// Unspamming a message in another folder records the user's ham verdict and
// then moves it to the inbox. The move cleared every verdict, so the inbox
// scan judged the message afresh -- the language check flagged its Chinese
// subject -- and it stayed [SPAM] however often it was unspammed. A move must
// leave both the user's decision and the scan's evidence alone.
func TestMoveToInboxKeepsUserVerdict(t *testing.T) {
	dir := t.TempDir()
	account := cachedir.At(dir)
	cfg := &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: dir}, AccountCache: account}
	client, _ := newChecking(t, cfg, true)

	raw := []byte("From: a@example.com\r\nSubject: hi\r\n\r\nbody\r\n")
	for _, id := range []email.ID{"user", "scan"} {
		if err := msg.Store(account, id, raw, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	_ = msg.SetOverride(account, "user", msg.OverrideHam)
	_ = msg.SetEvidence(account, "scan", msg.Evidence{Checks: msg.CheckBogofilter, BogoScore: 20})

	if err := client.MoveToInbox([]email.ID{"user", "scan"}, label.FromServer("Spam")); err != nil {
		t.Fatal(err)
	}

	if info, _ := msg.GetInfo(account, "user"); info.Override != msg.OverrideHam || info.IsSpam() {
		t.Errorf("the move cleared the user's verdict: %+v", info)
	}
	// Evidence belongs to the message, not the folder: a move leaves it too.
	if info, _ := msg.GetInfo(account, "scan"); !info.Scanned || !info.IsSpam() {
		t.Errorf("the move dropped the scan's evidence: %+v", info)
	}
}
