package scan

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sarielhp/mail_cli/cache/cachedir"
	"github.com/sarielhp/mail_cli/cache/msg"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/mailclient"
	"github.com/sarielhp/mail_cli/report"
)

// Mail in Sent was written by the user: a scan must give it no verdict and
// clear any it had, even when the sender would otherwise be blacklisted. The
// sender check would not do -- spam forges the recipient's own address.
func TestPerform_SentFolderGetsNoVerdict(t *testing.T) {
	account := cachedir.At(filepath.Join(t.TempDir(), "acct"))
	raw := []byte("Date: Mon, 2 Jan 2026 15:04:05 -0700\nFrom: me@example.com\nSubject: Re: talk\n\nSee you.")
	if err := msg.Store(account, "s1", raw, time.Now()); err != nil {
		t.Fatal(err)
	}
	// A stale verdict, as an earlier classifier left on the user's own mail.
	if err := msg.SetEvidence(account, "s1", msg.Evidence{Checks: msg.CheckPolitical, BogoScore: 0.95}); err != nil {
		t.Fatal(err)
	}

	config := &cfg_g.Config{SelectedAccount: "acct", AccountCache: account}
	config.Blacklist = []string{"me@example.com"}
	client := &mailclient.MockMailClient{
		Cfg:           config,
		Labels:        []string{"INBOX", "Sent"},
		DownloadedIDs: map[string][]email.ID{"Sent": {"s1"}},
	}

	var out bytes.Buffer
	if _, _, err := Perform(&out, Options{}, report.Discard(), client, config, "Sent"); err != nil {
		t.Fatal(err)
	}
	info, err := msg.GetInfo(account, "s1")
	if err != nil {
		t.Fatal(err)
	}
	if info.Scanned || info.IsPolitical() || info.IsSpam() || info.IsBlacklisted() {
		t.Errorf("sent message kept or got a verdict: %+v", info)
	}
	if client.ReportSpamCalls != 0 || len(client.MovedEmails) != 0 {
		t.Errorf("sent message was acted on: reports=%d moved=%v", client.ReportSpamCalls, client.MovedEmails)
	}
	if strings.Contains(out.String(), "[POLI]") || strings.Contains(out.String(), "[SPAM]") {
		t.Errorf("listing labels the user's own mail:\n%s", out.String())
	}
}
