package cfg_g

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sarielhp/mail_cli/cfg_acc"
)

func TestParseAccountLabelSpec(t *testing.T) {
	tests := []struct {
		input     string
		wantName  string
		wantLabel string
		wantErr   bool
	}{
		{"1:sort/mailing_list/wuna", "", "", true},
		{"2:wuna", "", "", true},
		{"10:keep/2026/08/wuna", "", "", true},
		{"wuna", "", "wuna", false},
		{"keep/2026/08/wuna", "", "keep/2026/08/wuna", false},
		{"0:wuna", "", "0:wuna", false},
		{"-1:wuna", "", "-1:wuna", false},
		{"1:", "", "", true},
		{"", "", "", false},
		{"GMail:inbox", "GMail", "inbox", false},
		{"FastMail:sort/news", "FastMail", "sort/news", false},
		{"Work-GMail:Spam", "Work-GMail", "Spam", false},
	}

	for _, tt := range tests {
		spec, err := ParseAccountLabelSpec(tt.input)
		if tt.wantErr {
			if err == nil {
				t.Errorf("ParseAccountLabelSpec(%q) expected error but got %+v", tt.input, spec)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseAccountLabelSpec(%q) unexpected error: %v", tt.input, err)
			continue
		}
		if spec.AccountName != tt.wantName || spec.Label != tt.wantLabel {
			t.Errorf("ParseAccountLabelSpec(%q) = {Name: %q, Label: %q}, want {Name: %q, Label: %q}",
				tt.input, spec.AccountName, spec.Label, tt.wantName, tt.wantLabel)
		}
	}
}

func TestExpandSavePath(t *testing.T) {
	refTime := time.Date(2026, 8, 23, 14, 30, 45, 0, time.UTC)

	// 1. Pattern "~/info/misc/%YY/%MM/%DD/"
	p1, err := ExpandSavePath("~/info/misc/%YY/%MM/%DD/", refTime)
	if err != nil {
		t.Fatalf("ExpandSavePath failed: %v", err)
	}
	if !strings.HasSuffix(p1, "info/misc/26/08/23") {
		t.Errorf("expected path ending in 'info/misc/26/08/23', got: %s", p1)
	}

	// 2. Full 4-digit year "%YYYY/%MM/%DD"
	p2, err := ExpandSavePath("/tmp/archive/%YYYY/%MM/%DD", refTime)
	if err != nil {
		t.Fatalf("ExpandSavePath failed: %v", err)
	}
	if p2 != "/tmp/archive/2026/08/23" {
		t.Errorf("expected '/tmp/archive/2026/08/23', got: %s", p2)
	}

	// 3. Default empty -> Downloads
	p3, err := ExpandSavePath("", refTime)
	if err != nil {
		t.Fatalf("ExpandSavePath failed: %v", err)
	}
	if !strings.HasSuffix(p3, "Downloads") {
		t.Errorf("expected default ending in 'Downloads', got: %s", p3)
	}
}

func TestLoadConfigFile_NilAccounts(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.json")
	if err := os.WriteFile(cfgPath, []byte(`{"username": "user@example.com"}`), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	fc, err := LoadConfigFile(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfigFile failed: %v", err)
	}
	if fc.Accounts == nil {
		t.Fatalf("expected fc.Accounts to be initialized, got nil")
	}
	if len(*fc.Accounts) != 0 {
		t.Errorf("expected 0 accounts, got %d", len(*fc.Accounts))
	}

	// Verify appending and dereferencing does not panic
	for _, acc := range *fc.Accounts {
		_ = acc.Name
	}
}

func TestLoadAccountFromDisk_PreservesReadOnlyFoldersAndSaveSentCopy(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.json")
	mainCfg := `{"accounts": [{"name": "work", "type": "jmap"}]}`
	if err := os.WriteFile(cfgPath, []byte(mainCfg), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	accDir := filepath.Join(tmpDir, "accounts", "work")
	if err := os.MkdirAll(accDir, 0700); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}

	accJSON := `{
		"name": "work",
		"type": "jmap",
		"read_only": true,
		"read_only_folders": ["Archive/2019", "Legal"],
		"save_sent_copy": false
	}`
	if err := os.WriteFile(filepath.Join(accDir, "account.json"), []byte(accJSON), 0600); err != nil {
		t.Fatalf("WriteFile account.json failed: %v", err)
	}

	fc, err := LoadConfigFile(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfigFile failed: %v", err)
	}
	if len(*fc.Accounts) != 1 {
		t.Fatalf("expected 1 account, got %d", len(*fc.Accounts))
	}

	acc := (*fc.Accounts)[0]
	if !acc.ReadOnly {
		t.Errorf("expected ReadOnly=true")
	}
	if len(acc.ReadOnlyFolders) != 2 || acc.ReadOnlyFolders[0] != "Archive/2019" || acc.ReadOnlyFolders[1] != "Legal" {
		t.Errorf("expected ReadOnlyFolders [Archive/2019 Legal], got %v", acc.ReadOnlyFolders)
	}
	if acc.SaveSentCopy == nil || *acc.SaveSentCopy != false {
		t.Errorf("expected SaveSentCopy=false, got %v", acc.SaveSentCopy)
	}

	// Verify SaveConfigFile preserves these fields on re-save
	if saveErr := SaveConfigFile(cfgPath, fc); saveErr != nil {
		t.Fatalf("SaveConfigFile failed: %v", saveErr)
	}

	fcReloaded, err := LoadConfigFile(cfgPath)
	if err != nil {
		t.Fatalf("reload LoadConfigFile failed: %v", err)
	}
	reloadedAcc := (*fcReloaded.Accounts)[0]
	if len(reloadedAcc.ReadOnlyFolders) != 2 || reloadedAcc.ReadOnlyFolders[1] != "Legal" {
		t.Errorf("reloaded ReadOnlyFolders corrupted: %v", reloadedAcc.ReadOnlyFolders)
	}
	if reloadedAcc.SaveSentCopy == nil || *reloadedAcc.SaveSentCopy != false {
		t.Errorf("reloaded SaveSentCopy corrupted: %v", reloadedAcc.SaveSentCopy)
	}
}

func TestAutoBlacklistInternal_HonorsDomainWhitelist(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.json")
	mainCfg := `{"accounts": [{"name": "work", "type": "jmap"}]}`
	if err := os.WriteFile(cfgPath, []byte(mainCfg), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	accDir := filepath.Join(tmpDir, "accounts", "work")
	if err := os.MkdirAll(accDir, 0700); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}

	accJSON := `{
		"name": "work",
		"type": "jmap",
		"whitelist": ["@example.edu"]
	}`
	if err := os.WriteFile(filepath.Join(accDir, "account.json"), []byte(accJSON), 0600); err != nil {
		t.Fatalf("WriteFile account.json failed: %v", err)
	}
	whitelistJSON := `["@example.edu"]`
	if err := os.WriteFile(filepath.Join(accDir, "whitelist.json"), []byte(whitelistJSON), 0600); err != nil {
		t.Fatalf("WriteFile whitelist.json failed: %v", err)
	}

	cfg := &Config{SelectedAccount: "work", Paths: Paths{ConfigDir: tmpDir}}

	// 1. Sender matching domain whitelist should be skipped without rule creation
	if err := AutoBlacklistInternal(cfg, "announce@example.edu"); err != nil {
		t.Fatalf("AutoBlacklistInternal failed: %v", err)
	}

	fc, err := LoadConfigFile(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfigFile failed: %v", err)
	}
	acc := (*fc.Accounts)[0]
	for _, r := range acc.Rules {
		if r.Sender == "announce@example.edu" {
			t.Errorf("expected whitelisted domain sender not to be blacklisted")
		}
	}

	// 2. Non-whitelisted sender should have internal rule created
	if blacklistErr := AutoBlacklistInternal(cfg, "campaign@unsolicited.org"); blacklistErr != nil {
		t.Fatalf("AutoBlacklistInternal failed: %v", blacklistErr)
	}

	fc2, err := LoadConfigFile(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfigFile 2 failed: %v", err)
	}
	acc2 := (*fc2.Accounts)[0]
	found := false
	for _, r := range acc2.Rules {
		if r.Sender == "campaign@unsolicited.org" && r.Internal {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected internal blacklist rule for campaign@unsolicited.org")
	}
}

// A freshly created config.json is seeded with one placeholder account, so a
// first run is not literally account-less -- NeedsOnboarding is what tells it
// apart from a configured one.
func TestNeedsOnboarding(t *testing.T) {
	placeholderGmail := cfg_acc.AccountConfig{
		Name: "default", Type: "gmail",
		Username: "your-email@gmail.com", Password: "your-app-password",
	}
	placeholderJMAP := cfg_acc.AccountConfig{
		Name: "fastmail", Type: "jmap",
		Username: "user@fastmail.com", Password: "your-jmap-api-token",
	}
	realGmail := cfg_acc.AccountConfig{
		Name: "work", Type: "gmail",
		Username: "user@gmail.com", Password: "app-password",
	}
	realOutlook := cfg_acc.AccountConfig{
		Name: "work", Type: "outlook",
		Username: "user@outlook.com",
	}

	tests := []struct {
		name   string
		config *Config
		want   bool
	}{
		{"nil config", nil, true},
		{"no accounts", &Config{}, true},
		{"only placeholder gmail", &Config{Accounts: []cfg_acc.AccountConfig{placeholderGmail}}, true},
		{"placeholder token as password", &Config{Accounts: []cfg_acc.AccountConfig{placeholderJMAP}}, true},
		{"real gmail", &Config{Accounts: []cfg_acc.AccountConfig{realGmail}}, false},
		{"outlook needs no password", &Config{Accounts: []cfg_acc.AccountConfig{realOutlook}}, false},
		{"one real among placeholders", &Config{Accounts: []cfg_acc.AccountConfig{placeholderGmail, realGmail}}, false},
	}

	for _, tt := range tests {
		if got := NeedsOnboarding(tt.config); got != tt.want {
			t.Errorf("%s: NeedsOnboarding = %v, want %v", tt.name, got, tt.want)
		}
	}
}
