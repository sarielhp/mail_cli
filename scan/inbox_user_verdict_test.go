package scan

import (
	"bytes"
	"path/filepath"
	"testing"
	"time"

	"github.com/sarielhp/mail_cli/cache/cachedir"
	"github.com/sarielhp/mail_cli/cache/msg"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/mailclient"
	"github.com/sarielhp/mail_cli/report"
)

// An explicit inbox scan re-labels what the scan decided, not what the user
// did. It cleared every verdict, so a message the user had trained as ham was
// relabelled spam by the language check each time the inbox was opened --
// `spam learn --ham %` appeared to do nothing.
func TestPerform_ExplicitInboxKeepsUserVerdict(t *testing.T) {
	hamBogofilter(t)
	account := cachedir.At(filepath.Join(t.TempDir(), "acct"))
	raw := []byte("Date: Mon, 2 Jan 2026 15:04:05 -0700\nFrom: colleague@example.org\nSubject: [vision] 已取消: talk\n\nCancelled.")
	for _, id := range []string{"ham", "scanned"} {
		if err := msg.Store(account, email.ID(id), raw, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	if err := msg.SetOverride(account, "ham", msg.OverrideHam); err != nil {
		t.Fatal(err)
	}
	if err := msg.SetEvidence(account, "scanned", msg.Evidence{}); err != nil {
		t.Fatal(err)
	}

	config := &cfg_g.Config{SelectedAccount: "acct", AccountCache: account}
	config.AllowedLanguages = []string{"english"}
	client := &mailclient.MockMailClient{
		Cfg:           config,
		Labels:        []string{"INBOX"},
		DownloadedIDs: map[string][]email.ID{"INBOX": {"ham", "scanned"}},
	}
	if _, _, err := Perform(&bytes.Buffer{}, Options{ExplicitInbox: true}, report.Discard(), client, config, "INBOX"); err != nil {
		t.Fatal(err)
	}

	if info, _ := msg.GetInfo(account, "ham"); info.IsSpam() || info.Override != msg.OverrideHam {
		t.Errorf("user's ham verdict was redone: %+v", info)
	}
	if info, _ := msg.GetInfo(account, "scanned"); !info.IsSpam() {
		t.Errorf("the scan's own verdict was not re-labelled: %+v", info)
	}
}
