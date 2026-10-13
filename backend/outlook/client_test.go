package outlook

import (
	"strings"
	"testing"

	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/mailclient"
	"github.com/sarielhp/mail_cli/report"
)

func TestNewOutlookClient(t *testing.T) {
	acc := cfg_acc.AccountConfig{
		Name:     "test",
		Type:     "outlook",
		Username: "test@example.com",
	}
	config := &cfg_g.Config{}

	client := NewClient(acc, config, report.Discard())

	if client == nil {
		t.Fatal("NewClient returned nil")
	}

	// Verify it implements MailClient interface
	var _ mailclient.MailClient = client
}

func TestOutlookClient_Validate(t *testing.T) {
	acc := cfg_acc.AccountConfig{
		Name:           "test",
		Type:           "outlook",
		Username:       "test@example.com",
		SpamFolder:     "Junk Email",
		ReceivedFolder: "Archive",
	}
	config := &cfg_g.Config{
		SelectedAccount: "test",
	}

	client := NewClient(acc, config, report.Discard())

	// For testing, we skip the actual OAuth validation
	// The client is created successfully, validation would require real credentials
	// Just verify the client was created
	if client == nil {
		t.Fatal("NewClient returned nil")
	}
}

func TestOutlookClient_InboxFolder(t *testing.T) {
	acc := cfg_acc.AccountConfig{
		Name:     "test",
		Type:     "outlook",
		Username: "test@example.com",
	}
	config := &cfg_g.Config{}

	client := NewClient(acc, config, report.Discard())

	inbox := client.InboxFolder()
	if inbox.String() != "Inbox" {
		t.Errorf("InboxFolder() = %q, want %q", inbox, "Inbox")
	}
}

func TestOutlookClient_BackendType(t *testing.T) {
	acc := cfg_acc.AccountConfig{
		Name:     "test",
		Type:     "outlook",
		Username: "test@example.com",
	}
	config := &cfg_g.Config{}

	client := NewClient(acc, config, report.Discard())

	backendType := client.BackendType()
	if backendType != "outlook" {
		t.Errorf("BackendType() = %q, want %q", backendType, "outlook")
	}
}

func TestOutlookClient_Config(t *testing.T) {
	acc := cfg_acc.AccountConfig{
		Name:     "test",
		Type:     "outlook",
		Username: "test@example.com",
	}
	config := &cfg_g.Config{
		SelectedAccount: "test",
	}

	client := NewClient(acc, config, report.Discard())

	cfg := client.Config()
	if cfg == nil {
		t.Fatal("Config() returned nil")
	}
	if cfg.SelectedAccount != "test" {
		t.Errorf("Config().SelectedAccount = %q, want %q", cfg.SelectedAccount, "test")
	}
}

func TestReadErrorBody_Bounded(t *testing.T) {
	largeData := strings.Repeat("A", 10*1024)
	r := strings.NewReader(largeData)

	got := readErrorBody(r)
	if len(got) != maxErrorBodyBytes {
		t.Errorf("expected readErrorBody to truncate at %d bytes, got %d bytes", maxErrorBodyBytes, len(got))
	}
	if got != strings.Repeat("A", maxErrorBodyBytes) {
		t.Errorf("readErrorBody content mismatch")
	}

	shortData := "short error message"
	gotShort := readErrorBody(strings.NewReader(shortData))
	if gotShort != shortData {
		t.Errorf("expected %q, got %q", shortData, gotShort)
	}
}
