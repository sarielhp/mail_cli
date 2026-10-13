package contacts

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sarielhp/mail_cli/cfg_g"
)

func TestHarvestFromMessageStream_SentAndRecipients(t *testing.T) {
	rawMsg := "From: me@example.com\r\n" +
		"To: Colleague <colleague@example.com>\r\n" +
		"Cc: Boss <boss@example.com>\r\n" +
		"Date: Mon, 20 Sep 2026 10:00:00 -0500\r\n" +
		"\r\n" +
		"Hello team!\r\n"

	targetMap := make(map[string]*ScoredContact)
	blacklist := map[string]bool{}

	err := HarvestFromMessageStream(strings.NewReader(rawMsg), true, false, blacklist, "harvest:work", targetMap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(targetMap) != 2 {
		t.Fatalf("expected 2 recipients harvested from sent message, got %d", len(targetMap))
	}

	colleague := targetMap["colleague@example.com"]
	if colleague == nil || colleague.Name != "Colleague" || !colleague.IsSent {
		t.Errorf("expected colleague with IsSent=true, got %+v", colleague)
	}

	boss := targetMap["boss@example.com"]
	if boss == nil || boss.Name != "Boss" || !boss.IsSent {
		t.Errorf("expected boss with IsSent=true, got %+v", boss)
	}
}

func TestHarvestFromMessageStream_MailingListSuppressed(t *testing.T) {
	rawMsg := "From: Poster <poster@example.com>\r\n" +
		"To: algo-list@example.org\r\n" +
		"List-Id: <algo.example.org>\r\n" +
		"Date: Mon, 20 Sep 2026 10:00:00 -0500\r\n" +
		"\r\n" +
		"Digest message\r\n"

	targetMap := make(map[string]*ScoredContact)
	blacklist := map[string]bool{}

	err := HarvestFromMessageStream(strings.NewReader(rawMsg), false, false, blacklist, "harvest:work", targetMap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(targetMap) != 0 {
		t.Errorf("expected 0 contacts harvested from mailing list message, got %d", len(targetMap))
	}
}

func TestHarvestFromMessageStream_AutomatedAddressIgnored(t *testing.T) {
	rawMsg := "From: Service Notifications <no-reply@github.com>\r\n" +
		"To: me@example.com\r\n" +
		"Date: Mon, 20 Sep 2026 10:00:00 -0500\r\n" +
		"\r\n" +
		"New commit notification\r\n"

	targetMap := make(map[string]*ScoredContact)
	blacklist := map[string]bool{}

	err := HarvestFromMessageStream(strings.NewReader(rawMsg), false, false, blacklist, "harvest:work", targetMap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(targetMap) != 0 {
		t.Errorf("expected 0 contacts harvested for no-reply address, got %d", len(targetMap))
	}
}

func TestPurgeAddress(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "suggested_contacts.json")

	hb := &HistoryBook{
		Updated: time.Now().UTC(),
		Entries: []ScoredContact{
			{
				Contact: Contact{
					Name:   "Alice",
					Emails: []string{"alice@example.com"},
				},
			},
			{
				Contact: Contact{
					Name:   "Spammer",
					Emails: []string{"spammer@badsite.com"},
				},
			},
		},
	}

	if err := SaveHistory(path, hb); err != nil {
		t.Fatalf("SaveHistory failed: %v", err)
	}

	// Purge spammer
	if err := PurgeAddress(path, "spammer@badsite.com"); err != nil {
		t.Fatalf("PurgeAddress failed: %v", err)
	}

	loaded, err := LoadHistory(path)
	if err != nil {
		t.Fatalf("LoadHistory failed: %v", err)
	}

	if len(loaded.Entries) != 1 {
		t.Fatalf("expected 1 entry left after purge, got %d", len(loaded.Entries))
	}
	if loaded.Entries[0].PrimaryEmail() != "alice@example.com" {
		t.Errorf("expected alice@example.com, got %s", loaded.Entries[0].PrimaryEmail())
	}
}

func TestRecordSentRecipients(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &cfg_g.Config{
		Paths: cfg_g.Paths{
			CacheRoot: filepath.Join(tmpDir, "mail_cli"),
		},
	}

	raw := []byte("To: Bob Friend <bob@example.com>\r\nCc: Carol <carol@example.com>\r\n\r\nHi guys")
	RecordSentRecipients(cfg, raw)

	historyPath := DefaultHistoryPath(cfg)
	loaded, err := LoadHistory(historyPath)
	if err != nil {
		t.Fatalf("LoadHistory failed: %v", err)
	}

	if len(loaded.Entries) != 2 {
		t.Fatalf("expected 2 entries recorded, got %d", len(loaded.Entries))
	}
	for _, e := range loaded.Entries {
		if !e.IsSent {
			t.Errorf("expected IsSent=true for %s", e.PrimaryEmail())
		}
	}
}
