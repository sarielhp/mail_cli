package spam

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/sarielhp/mail_cli/cache/cachedir"
	"github.com/sarielhp/mail_cli/cache/folderindex"
	"github.com/sarielhp/mail_cli/cache/msg"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/label"
	"github.com/sarielhp/mail_cli/report"
)

// Forgetting drops only the user's decision: the scan's evidence stays, so a
// message the filter flagged is spam again, and one never scanned is judged
// on the next scan. A dry run changes nothing.
func TestForgetFolderDropsOnlyTheDecision(t *testing.T) {
	account := cachedir.At(filepath.Join(t.TempDir(), "acct"))
	raw := []byte("From: a@example.com\r\nSubject: hi\r\n\r\nbody\r\n")
	ids := []email.ID{"flagged", "unscanned", "undecided"}
	for _, id := range ids {
		if err := msg.Store(account, id, raw, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	_ = msg.SetEvidence(account, "flagged", msg.Evidence{Checks: msg.CheckBogofilter, Stamp: 9})
	_ = msg.SetOverride(account, "flagged", msg.OverrideHam)
	_ = msg.SetOverride(account, "unscanned", msg.OverrideSpam)
	if err := folderindex.ReplaceAll(account, "received", ids); err != nil {
		t.Fatal(err)
	}
	received := label.FromServer("received")

	if err := ForgetFolder(report.Discard(), account, received, true); err != nil {
		t.Fatal(err)
	}
	if info, _ := msg.GetInfo(account, "flagged"); info.Override != msg.OverrideHam {
		t.Fatalf("a dry run forgot the decision: %+v", info)
	}

	if err := ForgetFolder(report.Discard(), account, received, false); err != nil {
		t.Fatal(err)
	}
	if info, _ := msg.GetInfo(account, "flagged"); info.Override != msg.OverrideNone || !info.IsSpam() || info.Evidence.Stamp != 9 {
		t.Errorf("forgetting lost the evidence or kept the decision: %+v", info)
	}
	if info, _ := msg.GetInfo(account, "unscanned"); info.Override != msg.OverrideNone || !info.NeedsScan(9) {
		t.Errorf("an unscanned message is not handed back to the filter: %+v", info)
	}
}
