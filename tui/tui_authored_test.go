package tui

import (
	"testing"
	"time"

	"github.com/sarielhp/mail_cli/cache/cachedir"
	"github.com/sarielhp/mail_cli/cache/msg"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/mailclient"
)

// Browsing Sent must neither queue the user's own mail for classification nor
// show a verdict left on it from before.
func TestLoadEmails_SentIsNotClassified(t *testing.T) {
	account := cachedir.At(t.TempDir())
	raw := []byte("Date: Mon, 2 Jan 2026 15:04:05 -0700\r\nFrom: me@example.com\r\nSubject: Re: talk\r\n\r\nSee you.")
	if err := msg.Store(account, "s1", raw, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := msg.SetEvidence(account, "s1", msg.Evidence{Checks: msg.CheckPolitical, BogoScore: 0.95}); err != nil {
		t.Fatal(err)
	}
	cfg := &cfg_g.Config{AccountCache: account}
	client := &mailclient.MockMailClient{Cfg: cfg, Labels: []string{"INBOX", "Sent"}, DownloadedIDs: map[string][]email.ID{"Sent": {"s1"}}}
	m := newTuiModel(client, "Sent", nil)

	loaded, ok := m.loadEmailsCmd("Sent")().(emailsLoadedMsg)
	if !ok {
		t.Fatal("loadEmailsCmd did not return emailsLoadedMsg")
	}
	if len(loaded.classifyIDs) != 0 {
		t.Errorf("sent mail queued for classification: %v", loaded.classifyIDs)
	}
	if len(loaded.emails) != 1 || loaded.emails[0].IsPolitical || loaded.emails[0].IsSpam {
		t.Errorf("sent mail listed with a verdict: %+v", loaded.emails)
	}
}
