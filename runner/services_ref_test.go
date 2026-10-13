package runner

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/sarielhp/mail_cli/cache/cachedir"
	"github.com/sarielhp/mail_cli/cache/folderindex"
	"github.com/sarielhp/mail_cli/cache/msg"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/contacts"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/label"
	"github.com/sarielhp/mail_cli/mailclient"
	"github.com/sarielhp/mail_cli/report"
)

// Archiving by the short ID a listing shows must still record the sender. The
// service passed the typed reference straight to the contact bookkeeping,
// which reads the message by exact ID: a short ID found nothing, and the
// archive succeeded without the sender ever being recorded.
func TestOrganizeByID_ShortIDRecordsSender(t *testing.T) {
	root := t.TempDir()
	account := cachedir.ForAccount(root, "acct")
	id := email.ID("full-server-id-1")
	raw := []byte("From: Alice <alice@example.com>\r\nSubject: hi\r\n\r\nbody\r\n")
	if err := msg.Store(account, id, raw, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := folderindex.Add(account, "INBOX", id); err != nil {
		t.Fatal(err)
	}
	config := &cfg_g.Config{SelectedAccount: "acct", AccountCache: account}
	config.CacheRoot = root
	client := &mailclient.MockMailClient{Cfg: config}

	short := string(email.ComputeShortID(id))
	if err := (OrganizeService{Rep: report.Discard()}).ByID(config, client, label.FromServer("Archive"), short); err != nil {
		t.Fatal(err)
	}

	hb, err := contacts.LoadHistory(filepath.Join(root, "suggested_contacts.json"))
	if err != nil {
		t.Fatalf("no contact history written: %v", err)
	}
	if len(hb.Entries) != 1 || hb.Entries[0].PrimaryEmail() != "alice@example.com" {
		t.Errorf("history = %+v, want alice@example.com", hb.Entries)
	}
}
