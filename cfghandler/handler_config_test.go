package cfghandler

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"
)

func TestHandleConfigSetAndReset_LLM(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	fc := &cfg_g.FileConfig{
		Accounts: &[]cfg_acc.AccountConfig{
			{Name: "default", Type: "gmail", Username: "u@gmail.com", Password: "p", SpamFolder: "Spam", ReceivedFolder: "Inbox"},
		},
	}
	if err := cfg_g.SaveConfigFile(configPath, fc); err != nil {
		t.Fatalf("failed to save test config: %v", err)
	}

	config := &cfg_g.Config{Accounts: *fc.Accounts, Paths: cfg_g.Paths{ConfigDir: tmpDir}}

	// 1. Set llm_endpoint
	if err := HandleConfigSet(io.Discard, config, "llm_endpoint", "http://localhost:4000/v1/chat/completions"); err != nil {
		t.Fatalf("HandleConfigSet llm_endpoint failed: %v", err)
	}
	if config.LLM.Endpoint != "http://localhost:4000/v1/chat/completions" {
		t.Errorf("expected endpoint http://localhost:4000/v1/chat/completions, got %s", config.LLM.Endpoint)
	}

	// 2. Set llm_api_key
	if err := HandleConfigSet(io.Discard, config, "llm_api_key", "sk-secret-key"); err != nil {
		t.Fatalf("HandleConfigSet llm_api_key failed: %v", err)
	}
	if config.LLM.APIKey != "sk-secret-key" {
		t.Errorf("expected api_key sk-secret-key, got %s", config.LLM.APIKey)
	}

	// 3. Set llm_temperature
	if err := HandleConfigSet(io.Discard, config, "llm_temperature", "0.35"); err != nil {
		t.Fatalf("HandleConfigSet llm_temperature failed: %v", err)
	}
	if config.LLM.Temperature == nil || *config.LLM.Temperature != 0.35 {
		t.Errorf("expected temperature 0.35, got %v", config.LLM.Temperature)
	}

	// 4. Set llm_max_tokens
	if err := HandleConfigSet(io.Discard, config, "llm_max_tokens", "500"); err != nil {
		t.Fatalf("HandleConfigSet llm_max_tokens failed: %v", err)
	}
	if config.LLM.MaxTokens == nil || *config.LLM.MaxTokens != 500 {
		t.Errorf("expected max_tokens 500, got %v", config.LLM.MaxTokens)
	}

	// 5. Verify persistence by reading config back from disk
	loadedFc, err := cfg_g.LoadConfigFile(configPath)
	if err != nil {
		t.Fatalf("failed to reload config: %v", err)
	}
	if loadedFc.LLM == nil {
		t.Fatalf("expected loadedFc.LLM to be non-nil")
	}
	if loadedFc.LLM.Endpoint != "http://localhost:4000/v1/chat/completions" {
		t.Errorf("expected saved endpoint http://localhost:4000/v1/chat/completions, got %s", loadedFc.LLM.Endpoint)
	}
	if loadedFc.LLM.APIKey != "sk-secret-key" {
		t.Errorf("expected saved api_key sk-secret-key, got %s", loadedFc.LLM.APIKey)
	}

	// 6. Test reset
	if resetErr := HandleConfigReset(io.Discard, config, "llm"); resetErr != nil {
		t.Fatalf("HandleConfigReset llm failed: %v", resetErr)
	}
	if config.LLM.Endpoint != "" || config.LLM.APIKey != "" {
		t.Errorf("expected empty LLM config after reset, got %+v", config.LLM)
	}

	loadedFcReset, err := cfg_g.LoadConfigFile(configPath)
	if err != nil {
		t.Fatalf("failed to reload config after reset: %v", err)
	}
	if loadedFcReset.LLM != nil {
		t.Errorf("expected loadedFcReset.LLM to be nil after reset, got %+v", loadedFcReset.LLM)
	}
}

func TestHandleConfigShow_And_Validate(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	fc := &cfg_g.FileConfig{
		DownloadDir: tmpDir,
		Accounts: &[]cfg_acc.AccountConfig{
			{Name: "default", Type: "gmail", Username: "user@gmail.com", Password: "pwd", IMAPHost: "imap.gmail.com:993", SpamFolder: "[Gmail]/Spam", ReceivedFolder: "Inbox"},
		},
		LLM: &cfg_g.LLMConfig{
			Endpoint: "http://127.0.0.1:4000/v1",
			APIKey:   "sk-1234567890",
			Model:    "llama3.1",
		},
	}
	if err := cfg_g.SaveConfigFile(configPath, fc); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	config := &cfg_g.Config{Accounts: *fc.Accounts, LLM: *fc.LLM, Paths: cfg_g.Paths{ConfigDir: tmpDir, CacheRoot: tmpDir}}

	if err := HandleConfigShow(io.Discard, config); err != nil {
		t.Errorf("HandleConfigShow failed: %v", err)
	}

	// Skip bogofilter failure in test environments if not installed
	_ = os.Setenv("PATH", os.Getenv("PATH"))
	_ = PerformConfigValidation(io.Discard, config)
}

func TestHandleConfigSetAndReset_MuttAliases(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	fc := &cfg_g.FileConfig{
		Accounts: &[]cfg_acc.AccountConfig{
			{Name: "default", Type: "gmail", Username: "u@gmail.com", Password: "p", SpamFolder: "Spam", ReceivedFolder: "Inbox"},
		},
	}
	if err := cfg_g.SaveConfigFile(configPath, fc); err != nil {
		t.Fatalf("failed to save test config: %v", err)
	}

	config := &cfg_g.Config{Accounts: *fc.Accounts, Paths: cfg_g.Paths{ConfigDir: tmpDir}}

	// 1. Set mutt_aliases
	aliasFile := "/path/to/mutt_aliases"
	if err := HandleConfigSet(io.Discard, config, "mutt_aliases", aliasFile); err != nil {
		t.Fatalf("HandleConfigSet mutt_aliases failed: %v", err)
	}
	if config.MuttAliases != aliasFile {
		t.Errorf("expected %s, got %s", aliasFile, config.MuttAliases)
	}

	// 2. Verify disk persistence
	reloaded, err := cfg_g.LoadConfigFile(configPath)
	if err != nil {
		t.Fatalf("failed to reload config: %v", err)
	}
	if reloaded.MuttAliases != aliasFile {
		t.Errorf("expected reloaded %s, got %s", aliasFile, reloaded.MuttAliases)
	}

	// 3. Reset mutt_aliases
	if err := HandleConfigReset(io.Discard, config, "mutt_aliases"); err != nil {
		t.Fatalf("HandleConfigReset mutt_aliases failed: %v", err)
	}
	if config.MuttAliases != "" {
		t.Errorf("expected empty mutt_aliases after reset, got %s", config.MuttAliases)
	}

	// 4. Set using mutt_alias_file key
	if err := HandleConfigSet(io.Discard, config, "mutt_alias_file", aliasFile); err != nil {
		t.Fatalf("HandleConfigSet mutt_alias_file failed: %v", err)
	}
	if config.MuttAliases != aliasFile {
		t.Errorf("expected %s, got %s", aliasFile, config.MuttAliases)
	}

	// 5. Get using mutt_alias_file
	var getBuf bytes.Buffer
	if err := HandleConfigGet(&getBuf, config, "mutt_alias_file"); err != nil {
		t.Fatalf("HandleConfigGet mutt_alias_file failed: %v", err)
	}
	if strings.TrimSpace(getBuf.String()) != aliasFile {
		t.Errorf("expected %s, got %s", aliasFile, strings.TrimSpace(getBuf.String()))
	}

	// 6. Show includes Mutt Aliases File
	var showBuf bytes.Buffer
	if err := HandleConfigShow(&showBuf, config); err != nil {
		t.Fatalf("HandleConfigShow failed: %v", err)
	}
	if !strings.Contains(showBuf.String(), "Mutt Aliases File:   "+aliasFile) {
		t.Errorf("expected Mutt Aliases File in show output, got:\n%s", showBuf.String())
	}

	// 7. Reset using mutt_alias_file
	if err := HandleConfigReset(io.Discard, config, "mutt_alias_file"); err != nil {
		t.Fatalf("HandleConfigReset mutt_alias_file failed: %v", err)
	}
	if config.MuttAliases != "" {
		t.Errorf("expected empty mutt_aliases after reset, got %s", config.MuttAliases)
	}
}

func TestHandleConfigSetGetReset_DefaultSendAccount(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	accounts := []cfg_acc.AccountConfig{
		{Name: "work", Username: "work@example.com"},
		{Name: "personal", Username: "personal@example.com"},
	}
	initialFC := cfg_g.FileConfig{
		Accounts: &accounts,
	}
	if err := cfg_g.SaveConfigFile(cfgPath, &initialFC); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	config := &cfg_g.Config{
		Paths:    cfg_g.Paths{ConfigDir: dir},
		Accounts: accounts,
	}

	// 1. Setting nonexistent account should fail validation
	if err := HandleConfigSet(io.Discard, config, "default_send_account", "nonexistent"); err == nil {
		t.Fatalf("expected error setting nonexistent account, got nil")
	}

	// 2. Setting valid account
	if err := HandleConfigSet(io.Discard, config, "default_send_account", "work"); err != nil {
		t.Fatalf("HandleConfigSet default_send_account failed: %v", err)
	}
	if config.DefaultSendAccount != "work" {
		t.Errorf("expected work, got %s", config.DefaultSendAccount)
	}

	// 3. Get default_send_account
	var getBuf bytes.Buffer
	if err := HandleConfigGet(&getBuf, config, "default_send_account"); err != nil {
		t.Fatalf("HandleConfigGet default_send_account failed: %v", err)
	}
	if strings.TrimSpace(getBuf.String()) != "work" {
		t.Errorf("expected work, got %s", strings.TrimSpace(getBuf.String()))
	}

	// 4. Show includes Default Send Account
	var showBuf bytes.Buffer
	if err := HandleConfigShow(&showBuf, config); err != nil {
		t.Fatalf("HandleConfigShow failed: %v", err)
	}
	if !strings.Contains(showBuf.String(), "Default Send Account: work") {
		t.Errorf("expected Default Send Account: work in show output, got:\n%s", showBuf.String())
	}

	// 5. Reset default_send_account
	if err := HandleConfigReset(io.Discard, config, "default_send_account"); err != nil {
		t.Fatalf("HandleConfigReset default_send_account failed: %v", err)
	}
	if config.DefaultSendAccount != "" {
		t.Errorf("expected empty DefaultSendAccount after reset, got %s", config.DefaultSendAccount)
	}
}

// badge is an account key: it lands on the account -A selected and nowhere
// else, and survives the round trip through the file.
func TestHandleConfigSetBadgeWritesTheSelectedAccount(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	fc := &cfg_g.FileConfig{
		Accounts: &[]cfg_acc.AccountConfig{
			{Name: "home", Type: "gmail", Username: "h@example.com", Password: "p"},
			{Name: "work", Type: "jmap", Username: "w@example.com", Password: "p"},
		},
	}
	if err := cfg_g.SaveConfigFile(configPath, fc); err != nil {
		t.Fatal(err)
	}
	config := &cfg_g.Config{Accounts: *fc.Accounts, SelectedAccount: "work", Paths: cfg_g.Paths{ConfigDir: tmpDir}}

	if err := HandleConfigSet(io.Discard, config, "badge", "💼"); err != nil {
		t.Fatalf("set badge: %v", err)
	}
	saved, err := cfg_g.LoadConfigFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, acc := range *saved.Accounts {
		want := ""
		if acc.Name == "work" {
			want = "💼"
		}
		if acc.Badge != want {
			t.Errorf("account %s badge = %q, want %q", acc.Name, acc.Badge, want)
		}
	}
}

// username is settable per account, so an account created without one can be
// repaired without hand-editing its account.json; an empty value is refused.
func TestHandleConfigSetUsername(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	fc := &cfg_g.FileConfig{Accounts: &[]cfg_acc.AccountConfig{{Name: "work", Type: "outlook"}}}
	if err := cfg_g.SaveConfigFile(configPath, fc); err != nil {
		t.Fatal(err)
	}
	config := &cfg_g.Config{Accounts: *fc.Accounts, SelectedAccount: "work", Paths: cfg_g.Paths{ConfigDir: tmpDir}}

	if err := HandleConfigSet(io.Discard, config, "username", "  "); err == nil {
		t.Error("an empty username was accepted")
	}
	if err := HandleConfigSet(io.Discard, config, "username", "me@example.com"); err != nil {
		t.Fatalf("set username: %v", err)
	}
	saved, err := cfg_g.LoadConfigFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := (*saved.Accounts)[0].Username; got != "me@example.com" {
		t.Errorf("username = %q, want me@example.com", got)
	}
}
