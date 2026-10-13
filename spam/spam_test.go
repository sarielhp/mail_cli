package spam

import (
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"github.com/sarielhp/mail_cli/email"
	"testing"

	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/label"
	"github.com/sarielhp/mail_cli/report"
)

type mockSpamReporterClient struct {
	cfg             *cfg_g.Config
	reportSpamCalls int
	lastSourceLabel string
	lastIDs         []string
	err             error
}

func (m *mockSpamReporterClient) Config() *cfg_g.Config {
	return m.cfg
}

func (m *mockSpamReporterClient) ReportSpam(messageIDs []email.ID, source label.Folder) error {
	m.reportSpamCalls++
	m.lastIDs = email.IDStrings(messageIDs)
	m.lastSourceLabel = source.String()
	return m.err
}

func TestRelocateMarkedSpam_AlreadyInSpam(t *testing.T) {
	testCases := []struct {
		name       string
		spamFolder string
		folderName string
	}{
		{
			name:       "Gmail standard spam folder",
			spamFolder: "[Gmail]/Spam",
			folderName: "[Gmail]/Spam",
		},
		{
			name:       "Gmail spam folder case variation",
			spamFolder: "[Gmail]/Spam",
			folderName: "[gmail]/spam",
		},
		{
			name:       "Outlook junk email folder",
			spamFolder: "Junk Email",
			folderName: "Junk Email",
		},
		{
			name:       "Generic spam literal fallback",
			spamFolder: "[Gmail]/Spam",
			folderName: "spam",
		},
		{
			name:       "Generic spam literal fallback capitalized",
			spamFolder: "Junk Email",
			folderName: "Spam",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &cfg_g.Config{
				Policy: cfg_g.Policy{SpamFolder: tc.spamFolder},
				Paths:  cfg_g.Paths{CacheRoot: t.TempDir()}, AccountCache: cachedir.At(t.TempDir()),
			}
			client := &mockSpamReporterClient{cfg: cfg}
			rep := report.Discard()

			err := relocateMarkedSpam(rep, client, cfg, "target123", "msg123", tc.folderName)
			if err != nil {
				t.Fatalf("relocateMarkedSpam returned unexpected error: %v", err)
			}
			if client.reportSpamCalls != 0 {
				t.Errorf("expected 0 calls to ReportSpam for folder %q, got %d", tc.folderName, client.reportSpamCalls)
			}
		})
	}
}

func TestRelocateMarkedSpam_NotInSpam(t *testing.T) {
	cfg := &cfg_g.Config{
		Policy: cfg_g.Policy{SpamFolder: "[Gmail]/Spam"},
		Paths:  cfg_g.Paths{CacheRoot: t.TempDir()}, AccountCache: cachedir.At(t.TempDir()),
	}
	client := &mockSpamReporterClient{cfg: cfg}
	rep := report.Discard()

	err := relocateMarkedSpam(rep, client, cfg, "target123", "msg123", "INBOX")
	if err != nil {
		t.Fatalf("relocateMarkedSpam returned unexpected error: %v", err)
	}
	if client.reportSpamCalls != 1 {
		t.Fatalf("expected 1 call to ReportSpam, got %d", client.reportSpamCalls)
	}
	if client.lastSourceLabel != "INBOX" {
		t.Errorf("expected source label INBOX, got %q", client.lastSourceLabel)
	}
	if len(client.lastIDs) != 1 || client.lastIDs[0] != "msg123" {
		t.Errorf("expected message ID msg123, got %v", client.lastIDs)
	}
}

func TestRelocateMarkedSpam_ReadOnly(t *testing.T) {
	cfg := &cfg_g.Config{
		Policy: cfg_g.Policy{SpamFolder: "[Gmail]/Spam"},
		Paths:  cfg_g.Paths{CacheRoot: t.TempDir()}, AccountCache: cachedir.At(t.TempDir()),
		ReadOnly: true,
	}
	client := &mockSpamReporterClient{cfg: cfg}
	rep := report.Discard()

	err := relocateMarkedSpam(rep, client, cfg, "target123", "msg123", "INBOX")
	if err != nil {
		t.Fatalf("unexpected error in read only mode: %v", err)
	}
	if client.reportSpamCalls != 0 {
		t.Errorf("expected no server calls in read only mode, got %d", client.reportSpamCalls)
	}
}
