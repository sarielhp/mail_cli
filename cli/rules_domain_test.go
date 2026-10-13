package cli

import (
	"encoding/json"
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"github.com/sarielhp/mail_cli/email"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sarielhp/mail_cli/cache/folderindex"
	"github.com/sarielhp/mail_cli/cache/msg"
	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/mailclient"
)

func TestAddDomainRuleToConfig(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	downloadDir := filepath.Join(tempDir, "cache")
	if err := os.MkdirAll(downloadDir, 0755); err != nil {
		t.Fatalf("failed to create download dir: %v", err)
	}

	// Set up initial config
	fc := &cfg_g.FileConfig{
		Accounts: &[]cfg_acc.AccountConfig{
			{
				Name:           "default",
				Type:           "gmail",
				Username:       "user@example.com",
				Password:       "pass",
				SpamFolder:     "Spam",
				ReceivedFolder: "INBOX",
				SpamLearn:      "Spam",
				Rules:          []cfg_acc.Rule{},
			},
		},
	}
	data, err := json.Marshal(fc)
	if err != nil {
		t.Fatalf("failed to marshal config: %v", err)
	}
	if writeErr := os.WriteFile(configPath, data, 0600); writeErr != nil {
		t.Fatalf("failed to write config: %v", writeErr)
	}

	// Store a test email in cache and index it in INBOX
	msgID := "testmsg123"
	emailRaw := []byte("From: \"Alice Smith\" <alice@acme.org>\r\nSubject: Test Email\r\n\r\nHello World")
	if storeErr := msg.Store(cachedir.ForAccount(downloadDir, "default"), email.ID(msgID), emailRaw, time.Now()); storeErr != nil {
		t.Fatalf("failed to store cached email: %v", storeErr)
	}
	if indexErr := folderindex.Add(cachedir.ForAccount(downloadDir, "default"), "INBOX", email.ID(msgID)); indexErr != nil {
		t.Fatalf("failed to index cached email: %v", indexErr)
	}

	config := &cfg_g.Config{SelectedAccount: "default", Paths: cfg_g.Paths{ConfigDir: tempDir, CacheRoot: downloadDir}}

	session := &Session{
		Config: config,
		Deps: Deps{
			Clients: stubClients{
				ForSelectedFn: func(_ *cfg_g.Config) (mailclient.MailClient, error) {
					return &mockMailClient{}, nil
				},
			},
		},
	}

	// Run addDomainRuleToConfig with target label "Newsletters"
	if addErr := addDomainRuleToConfig(session, msgID, "Newsletters"); addErr != nil {
		t.Fatalf("addDomainRuleToConfig failed: %v", addErr)
	}

	// Read updated config.json and verify rule was added for @acme.org
	updatedFC, err := cfg_g.LoadConfigFile(configPath)
	if err != nil {
		t.Fatalf("failed to read updated config: %v", err)
	}

	accs := *updatedFC.Accounts
	if len(accs[0].Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(accs[0].Rules))
	}

	rule := accs[0].Rules[0]
	if rule.Sender != "@acme.org" {
		t.Errorf("expected rule sender '@acme.org', got %q", rule.Sender)
	}
	if rule.Label != "Newsletters" {
		t.Errorf("expected rule label 'Newsletters', got %q", rule.Label)
	}

	// Verify MatchRules matches emails from @acme.org
	matchedRule := cfg_acc.MatchRules(accs[0].Rules, "bob@acme.org", "Any Subject")
	if matchedRule == nil {
		t.Errorf("MatchRules failed to match bob@acme.org against rule @acme.org")
	}
}

func TestAddRuleFromMessageID(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	downloadDir := filepath.Join(tempDir, "cache")
	if err := os.MkdirAll(downloadDir, 0755); err != nil {
		t.Fatalf("failed to create download dir: %v", err)
	}

	fc := &cfg_g.FileConfig{
		Accounts: &[]cfg_acc.AccountConfig{
			{
				Name:           "default",
				Type:           "gmail",
				Username:       "user@example.com",
				Password:       "pass",
				SpamFolder:     "Spam",
				ReceivedFolder: "INBOX",
				SpamLearn:      "Spam",
				Rules:          []cfg_acc.Rule{},
			},
		},
	}
	data, _ := json.Marshal(fc)
	_ = os.WriteFile(configPath, data, 0600)

	msgID := "msg999"
	emailRaw := []byte("From: Bob Jones <bob@company.com>\r\nSubject: Invoice\r\n\r\nHello")
	_ = msg.Store(cachedir.ForAccount(downloadDir, "default"), email.ID(msgID), emailRaw, time.Now())
	_ = folderindex.Add(cachedir.ForAccount(downloadDir, "default"), "Finance", email.ID(msgID))

	config := &cfg_g.Config{SelectedAccount: "default", Paths: cfg_g.Paths{ConfigDir: tempDir, CacheRoot: downloadDir}}

	session := &Session{
		Config: config,
		Deps: Deps{
			Clients: stubClients{
				ForSelectedFn: func(_ *cfg_g.Config) (mailclient.MailClient, error) {
					return &mockMailClient{}, nil
				},
			},
		},
	}

	// 1. Add rule by message ID without specifying label (should default to Finance folder)
	if err := addRuleFromMessageID(session, msgID, ""); err != nil {
		t.Fatalf("addRuleFromMessageID failed: %v", err)
	}

	updatedFC, _ := cfg_g.LoadConfigFile(configPath)
	accs := *updatedFC.Accounts
	if len(accs[0].Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(accs[0].Rules))
	}
	if accs[0].Rules[0].Sender != "bob@company.com" {
		t.Errorf("expected sender 'bob@company.com', got %q", accs[0].Rules[0].Sender)
	}
	if !strings.EqualFold(accs[0].Rules[0].Label, "Finance") {
		t.Errorf("expected label 'Finance' (case-insensitive), got %q", accs[0].Rules[0].Label)
	}
}
