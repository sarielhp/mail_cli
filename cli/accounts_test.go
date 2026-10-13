package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/mailclient"
)

func TestAccountRename(t *testing.T) {
	// Create a temp directory for config and download dirs
	tempDir, err := os.MkdirTemp("", "mail_cli_accounts_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	configDir := filepath.Join(tempDir, "config")
	downloadDir := filepath.Join(tempDir, "download")
	err = os.MkdirAll(configDir, 0755)
	if err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}
	err = os.MkdirAll(downloadDir, 0755)
	if err != nil {
		t.Fatalf("failed to create download dir: %v", err)
	}

	// Create a mock config.json
	mockConfig := cfg_g.FileConfig{
		Accounts: &[]cfg_acc.AccountConfig{
			{Name: "acc1", Type: "gmail"},
			{Name: "acc2", Type: "jmap"},
		},
	}
	configBytes, err := json.Marshal(mockConfig)
	if err != nil {
		t.Fatalf("failed to marshal mock config: %v", err)
	}
	configPath := filepath.Join(configDir, "config.json")
	err = os.WriteFile(configPath, configBytes, 0644)
	if err != nil {
		t.Fatalf("failed to write mock config file: %v", err)
	}

	// Create fake cache directory and token file for acc1
	acc1Cache := filepath.Join(downloadDir, "acc1")
	err = os.MkdirAll(acc1Cache, 0755)
	if err != nil {
		t.Fatalf("failed to create fake cache dir: %v", err)
	}
	fakeEmailFile := filepath.Join(acc1Cache, "test_msg")
	err = os.WriteFile(fakeEmailFile, []byte("fake"), 0644)
	if err != nil {
		t.Fatalf("failed to write fake cache file: %v", err)
	}

	tokensDir := filepath.Join(configDir, "tokens")
	err = os.MkdirAll(tokensDir, 0755)
	if err != nil {
		t.Fatalf("failed to create tokens dir: %v", err)
	}
	acc1Token := filepath.Join(tokensDir, "token_acc1.json")
	err = os.WriteFile(acc1Token, []byte("fake token"), 0644)
	if err != nil {
		t.Fatalf("failed to write fake token file: %v", err)
	}

	session := &Session{
		Config: &cfg_g.Config{SelectedAccount: "acc1", Paths: cfg_g.Paths{ConfigDir: configDir, CacheRoot: downloadDir}},
	}

	cliApp := InitCLI(session)

	// 1. Rename nonexistent account (should error)
	err = cliApp.Execute([]string{"account", "rename", "%nonexistent", "acc3"})
	if err == nil {
		t.Error("expected error renaming nonexistent account, got nil")
	}

	// 2. Rename to existing account name (should error - collision)
	err = cliApp.Execute([]string{"account", "rename", "acc1", "acc2"})
	if err == nil {
		t.Error("expected error renaming to existing account name, got nil")
	}

	// 3. Rename to same name but different case (should succeed)
	err = cliApp.Execute([]string{"account", "rename", "acc1", "ACC1"})
	if err != nil {
		t.Errorf("unexpected error renaming to same name with case change: %v", err)
	}

	// 4. Normal rename (should succeed and update DisplayName)
	err = cliApp.Execute([]string{"account", "rename", "ACC1", "acc3"})
	if err != nil {
		t.Errorf("unexpected error renaming account: %v", err)
	}

	// Verify changes in config file
	savedConfig, err := cfg_g.LoadConfigFile(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if len(*savedConfig.Accounts) != 2 {
		t.Errorf("expected 2 accounts, got %d", len(*savedConfig.Accounts))
	}
	foundDisplay := false
	for _, acc := range *savedConfig.Accounts {
		if acc.DisplayName == "acc3" {
			foundDisplay = true
			if acc.Name != "acc1" {
				t.Errorf("expected internal name to remain 'acc1', got %q", acc.Name)
			}
		}
	}
	if !foundDisplay {
		t.Error("display name 'acc3' not found in saved config")
	}

	// 5. The outgoing sender name is set independently of the account label.
	err = cliApp.Execute([]string{"account", "user-name", "acc3", "Sariel Har-Peled"})
	if err != nil {
		t.Fatalf("unexpected error setting user name: %v", err)
	}
	savedConfig, err = cfg_g.LoadConfigFile(configPath)
	if err != nil {
		t.Fatalf("failed to reload config: %v", err)
	}
	foundUser := false
	for _, acc := range *savedConfig.Accounts {
		if acc.UserName == "Sariel Har-Peled" {
			foundUser = true
			if acc.DisplayName != "acc3" {
				t.Errorf("user-name must not change the display name, got %q", acc.DisplayName)
			}
		}
	}
	if !foundUser {
		t.Error("user name not found in saved config")
	}

	// 6. The send address is set independently of the login username.
	err = cliApp.Execute([]string{"account", "user-email", "acc3", "sariel@sarielhp.org"})
	if err != nil {
		t.Fatalf("unexpected error setting user email: %v", err)
	}
	savedConfig, err = cfg_g.LoadConfigFile(configPath)
	if err != nil {
		t.Fatalf("failed to reload config: %v", err)
	}
	foundEmail := false
	for _, acc := range *savedConfig.Accounts {
		if acc.UserEmail == "sariel@sarielhp.org" {
			foundEmail = true
		}
	}
	if !foundEmail {
		t.Error("user email not found in saved config")
	}
	if err = cliApp.Execute([]string{"account", "user-email", "acc3", "not-an-email"}); err == nil {
		t.Error("expected an error for a malformed user email, got nil")
	}

	// Verify cache directory and token file were NOT moved or deleted (structures stay unchanged)
	if _, err := os.Stat(filepath.Join(downloadDir, "acc1")); err != nil {
		t.Error("expected original cache directory 'acc1' to still exist")
	}
	if _, err := os.Stat(filepath.Join(downloadDir, "acc3")); !os.IsNotExist(err) {
		t.Error("expected cache directory 'acc3' to not be created")
	}

	if _, err := os.Stat(filepath.Join(tokensDir, "token_acc1.json")); err != nil {
		t.Error("expected original token file 'token_acc1.json' to still exist")
	}
	if _, err := os.Stat(filepath.Join(tokensDir, "token_acc3.json")); !os.IsNotExist(err) {
		t.Error("expected token file 'token_acc3.json' to not be created")
	}
}

func TestAccountListNumbers(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "mail_cli_list_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	configDir := filepath.Join(tempDir, "config")
	downloadDir := filepath.Join(tempDir, "download")
	_ = os.MkdirAll(configDir, 0755)
	_ = os.MkdirAll(downloadDir, 0755)

	mockConfig := cfg_g.FileConfig{
		Accounts: &[]cfg_acc.AccountConfig{
			{Name: "acc1", Type: "gmail"},
			{Name: "acc2", Type: "jmap"},
		},
	}
	configBytes, err := json.Marshal(mockConfig)
	if err != nil {
		t.Fatalf("failed to marshal mock config: %v", err)
	}
	configPath := filepath.Join(configDir, "config.json")
	if writeErr := os.WriteFile(configPath, configBytes, 0644); writeErr != nil {
		t.Fatalf("failed to write mock config file: %v", writeErr)
	}

	session := &Session{
		Config: &cfg_g.Config{SelectedAccount: "acc1", Paths: cfg_g.Paths{ConfigDir: configDir, CacheRoot: downloadDir}},
	}

	cliApp := InitCLI(session)

	r, w, _ := os.Pipe()
	oldStdout := os.Stdout
	os.Stdout = w

	err = cliApp.Execute([]string{"account", "list"})

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("account list failed: %v", err)
	}

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	output := buf.String()

	if !strings.Contains(output, "1. acc1 (gmail, type: regular) *") {
		t.Errorf("expected output to contain '1. acc1 (gmail, type: regular) *', got:\n%s", output)
	}
	if !strings.Contains(output, "2. acc2 (jmap, type: regular)") {
		t.Errorf("expected output to contain '2. acc2 (jmap, type: regular)', got:\n%s", output)
	}
}

// TestAccountTestHonorsArgument guards against the regression where
// 'account test <name>' parsed its argument, ignored it, validated the selected
// account instead, and reported success naming the account it never touched.
func TestAccountTestHonorsArgument(t *testing.T) {
	cfg := &cfg_g.Config{
		SelectedAccount: "personal",
		Accounts: []cfg_acc.AccountConfig{
			{Name: "personal", Type: "gmail"},
			{Name: "work-jmap", Type: "jmap", DisplayName: "Work"},
		},
	}

	var askedFor string
	session := &Session{
		Config: cfg,
		Deps: Deps{
			Clients: stubClients{
				ForSelectedFn: func(c *cfg_g.Config) (mailclient.MailClient, error) {
					askedFor = string(c.SelectedAccount)
					return &mailclient.MockMailClient{Cfg: c}, nil
				},
			},
		},
	}

	if err := InitCLI(session).Execute([]string{"account", "test", "work-jmap"}); err != nil {
		t.Fatalf("account test work-jmap: %v", err)
	}
	if askedFor != "work-jmap" {
		t.Errorf("expected the client to be built for %q, got %q", "work-jmap", askedFor)
	}

	// The display name should work too, matching what rename/delete accept.
	cfg.SelectedAccount = "personal"
	if err := InitCLI(session).Execute([]string{"account", "test", "Work"}); err != nil {
		t.Fatalf("account test Work: %v", err)
	}
	if askedFor != "work-jmap" {
		t.Errorf("expected display name %q to resolve to %q, got %q", "Work", "work-jmap", askedFor)
	}

	// An unknown account must error, not silently test something else.
	cfg.SelectedAccount = "personal"
	if err := InitCLI(session).Execute([]string{"account", "test", "nope"}); err == nil {
		t.Error("expected an error for an unknown account name")
	}
}

func TestRunAccountList_NilAccountsConfig(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	if err := os.WriteFile(configPath, []byte(`{"username": "test@example.com"}`), 0600); err != nil {
		t.Fatalf("write config failed: %v", err)
	}
	cfg := &cfg_g.Config{Paths: cfg_g.Paths{ConfigDir: tempDir}}
	session := &Session{Config: cfg}
	if err := runAccountList(session); err != nil {
		t.Fatalf("runAccountList failed: %v", err)
	}
}

func TestCleanupAccountToken_RemovesWhatAccountOpsNames(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	tokenPath := filepath.Join(tempDir, "token_my-gmail.json")
	if err := os.WriteFile(tokenPath, []byte("{}"), 0600); err != nil {
		t.Fatalf("write token: %v", err)
	}

	var askedDir string
	session := &Session{
		Config: &cfg_g.Config{Paths: cfg_g.Paths{ConfigDir: tempDir}},
		Deps: Deps{
			Accounts: stubAccounts{
				TokenPathFn: func(dir string, _ cfg_acc.AccountConfig) (string, error) {
					askedDir = dir
					return tokenPath, nil
				},
			},
		},
	}

	cleanupAccountToken(session, configPath, &cfg_acc.AccountConfig{Name: "my-gmail", Type: "gmail"})
	if askedDir != tempDir {
		t.Errorf("expected the token directory to be the config's directory, got %q", askedDir)
	}
	if _, err := os.Stat(tokenPath); !os.IsNotExist(err) {
		t.Errorf("expected token to be deleted, got err: %v", err)
	}
}

// A backend with no token must not be asked to delete one.
func TestCleanupAccountToken_NoTokenForBackend(t *testing.T) {
	tempDir := t.TempDir()
	session := &Session{
		Config: &cfg_g.Config{Paths: cfg_g.Paths{ConfigDir: tempDir}},
		Deps: Deps{
			Accounts: stubAccounts{
				TokenPathFn: func(string, cfg_acc.AccountConfig) (string, error) {
					return "", nil
				},
			},
		},
	}

	cleanupAccountToken(session, filepath.Join(tempDir, "config.json"),
		&cfg_acc.AccountConfig{Name: "local", Type: "mbox"})
}
