package runner

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/contacts"
)

func TestContactService_Path(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &cfg_g.Config{
		Paths: cfg_g.Paths{ConfigDir: tmpDir},
	}

	var buf bytes.Buffer
	svc := ContactService{Out: &buf}
	if err := svc.Path(cfg); err != nil {
		t.Fatalf("Path failed: %v", err)
	}

	expected := filepath.Join(tmpDir, "contacts.json")
	if strings.TrimSpace(buf.String()) != expected {
		t.Errorf("expected %q, got %q", expected, buf.String())
	}
}

func TestContactService_ImportMuttAndListSearch(t *testing.T) {
	tmpDir := t.TempDir()
	muttFile := filepath.Join(tmpDir, "aliases")
	muttContent := `
alias alice Alice Smith <alice@example.com>
alias bob Bob Jones <bob@example.com>
alias algo alice bob
`
	if err := os.WriteFile(muttFile, []byte(muttContent), 0o600); err != nil {
		t.Fatalf("failed to write mutt file: %v", err)
	}

	cfg := &cfg_g.Config{
		Paths: cfg_g.Paths{
			ConfigDir:   tmpDir,
			MuttAliases: muttFile,
		},
	}

	var out bytes.Buffer
	svc := ContactService{Out: &out}

	// 1. Import using configured path
	if err := svc.ImportMutt(cfg, ""); err != nil {
		t.Fatalf("ImportMutt failed: %v", err)
	}

	// Verify contacts.json was created
	bookPath := contacts.DefaultPath(cfg)
	book, err := contacts.LoadBook(bookPath)
	if err != nil {
		t.Fatalf("failed to load book: %v", err)
	}
	if len(book.Contacts) != 3 {
		t.Fatalf("expected 3 contacts in book, got %d", len(book.Contacts))
	}

	// 2. List
	out.Reset()
	if err := svc.List(cfg, "alice"); err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if !strings.Contains(out.String(), "Alice Smith") || !strings.Contains(out.String(), "alice@example.com") {
		t.Errorf("expected Alice in list output, got:\n%s", out.String())
	}

	// 3. Search
	out.Reset()
	if err := svc.Search(cfg, "algo"); err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if !strings.Contains(out.String(), "Group: algo") {
		t.Errorf("expected Group: algo in search output, got:\n%s", out.String())
	}
}

func TestContactService_SyncGoogle_Validation(t *testing.T) {
	cfg := &cfg_g.Config{
		Accounts: []cfg_acc.AccountConfig{
			{Name: "jmap_acc", Type: "jmap"},
		},
	}

	svc := ContactService{}

	// Account not found
	if err := svc.SyncGoogle(cfg, "missing"); err == nil {
		t.Errorf("expected error for missing account, got nil")
	}

	// Non-gmail account
	if err := svc.SyncGoogle(cfg, "jmap_acc"); err == nil {
		t.Errorf("expected error for non-gmail account, got nil")
	}
}

func TestContactService_SuggestAndPurge(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &cfg_g.Config{
		Paths: cfg_g.Paths{
			ConfigDir: tmpDir,
			CacheRoot: tmpDir,
		},
	}

	book := contacts.NewBook()
	book.Contacts = append(book.Contacts, contacts.Contact{
		Name:      "Alice Smith",
		Emails:    []string{"alice@example.com"},
		Nicknames: []string{"al"},
	})
	if err := contacts.SaveBook(contacts.DefaultPath(cfg), book); err != nil {
		t.Fatalf("SaveBook failed: %v", err)
	}

	hist := contacts.NewHistoryBook()
	hist.Entries = append(hist.Entries, contacts.ScoredContact{
		Contact: contacts.Contact{
			Name:   "Bob Spammer",
			Emails: []string{"spammer@example.com"},
		},
		Frequency: 10,
	})
	if err := contacts.SaveHistory(contacts.DefaultHistoryPath(cfg), hist); err != nil {
		t.Fatalf("SaveHistory failed: %v", err)
	}

	var out bytes.Buffer
	svc := ContactService{Out: &out}

	if err := svc.Suggest(cfg, "al"); err != nil {
		t.Fatalf("Suggest failed: %v", err)
	}
	if !strings.Contains(out.String(), "Alice Smith") {
		t.Errorf("expected Alice in suggest output, got:\n%s", out.String())
	}

	if err := svc.Purge(cfg, "spammer@example.com"); err != nil {
		t.Fatalf("Purge failed: %v", err)
	}

	out.Reset()
	if err := svc.Suggest(cfg, "spammer"); err != nil {
		t.Fatalf("Suggest failed: %v", err)
	}
	if strings.Contains(out.String(), "Bob Spammer") {
		t.Errorf("expected Bob Spammer to be purged, got:\n%s", out.String())
	}
}
