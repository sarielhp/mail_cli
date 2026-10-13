package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/mailclient"
)

func TestSaveNewSetupAccount_FirstAccountDefaults(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")

	fc := &cfg_g.FileConfig{
		Accounts: &[]cfg_acc.AccountConfig{},
	}
	if err := cfg_g.SaveConfigFile(configPath, fc); err != nil {
		t.Fatalf("failed to create dummy config: %v", err)
	}

	session := &Session{
		Config: &cfg_g.Config{
			Accounts: []cfg_acc.AccountConfig{},
		},
	}

	newAcc := cfg_acc.AccountConfig{
		Name:     "fastmail",
		Type:     "jmap",
		Username: "user@fastmail.com",
	}

	err := saveNewSetupAccount(session, configPath, fc, newAcc)
	if err != nil {
		t.Fatalf("saveNewSetupAccount failed: %v", err)
	}

	if fc.DefaultSendAccount != "fastmail" {
		t.Errorf("expected DefaultSendAccount='fastmail', got %q", fc.DefaultSendAccount)
	}
	if len(*fc.Accounts) != 1 || (*fc.Accounts)[0].Name != "fastmail" {
		t.Errorf("expected 1 account in fc.Accounts, got %v", *fc.Accounts)
	}

	// Verify session was updated
	if session.Config.DefaultSendAccount != "fastmail" {
		t.Errorf("session DefaultSendAccount = %q, want 'fastmail'", session.Config.DefaultSendAccount)
	}
	if session.Config.SelectedAccount != "fastmail" {
		t.Errorf("session SelectedAccount = %q, want 'fastmail'", session.Config.SelectedAccount)
	}
}

func TestSaveNewSetupAccount_SecondAccountPreservesDefaults(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")

	fc := &cfg_g.FileConfig{
		DefaultSendAccount: "work",
		Accounts: &[]cfg_acc.AccountConfig{
			{Name: "work", Type: "outlook", Username: "user@work.com"},
		},
	}
	if err := cfg_g.SaveConfigFile(configPath, fc); err != nil {
		t.Fatalf("failed to create dummy config: %v", err)
	}

	session := &Session{
		Config: &cfg_g.Config{
			DefaultSendAccount: "work",
			SelectedAccount:    "work",
			Accounts: []cfg_acc.AccountConfig{
				{Name: "work", Type: "outlook", Username: "user@work.com"},
			},
		},
	}

	secondAcc := cfg_acc.AccountConfig{
		Name:     "personal",
		Type:     "gmail",
		Username: "user@gmail.com",
	}

	err := saveNewSetupAccount(session, configPath, fc, secondAcc)
	if err != nil {
		t.Fatalf("saveNewSetupAccount failed: %v", err)
	}

	if fc.DefaultSendAccount != "work" {
		t.Errorf("expected DefaultSendAccount to remain 'work', got %q", fc.DefaultSendAccount)
	}
	if session.Config.SelectedAccount != "work" {
		t.Errorf("expected SelectedAccount to remain 'work', got %q", session.Config.SelectedAccount)
	}
	if len(*fc.Accounts) != 2 {
		t.Errorf("expected 2 accounts, got %d", len(*fc.Accounts))
	}
}

// TestVerifySetupAccount_VerifiesTheAccountJustAdded is M4's regression: with
// more than one account already configured, saveNewSetupAccount leaves
// session.Config.SelectedAccount on the old account (see
// TestSaveNewSetupAccount_SecondAccountPreservesDefaults above), so
// verifySetupAccount must not resolve its client from session.Config as-is --
// that would verify the old account's credentials and report success without
// ever exercising the new one.
func TestVerifySetupAccount_VerifiesTheAccountJustAdded(t *testing.T) {
	oldStdin := os.Stdin
	defer func() { os.Stdin = oldStdin }()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdin = r
	go func() {
		defer w.Close()
		_, _ = w.WriteString("y\n")
	}()

	var requestedAccount string
	session := &Session{
		Config: &cfg_g.Config{
			SelectedAccount: "work",
			Accounts: []cfg_acc.AccountConfig{
				{Name: "work", Type: "outlook"},
				{Name: "personal", Type: "jmap", SessionURL: "https://jmap.example.com"},
			},
		},
		Deps: Deps{
			Clients: stubClients{
				ForSelectedFn: func(cfg *cfg_g.Config) (mailclient.MailClient, error) {
					requestedAccount = string(cfg.SelectedAccount)
					return &mailclient.MockMailClient{Cfg: cfg}, nil
				},
			},
		},
	}

	newAcc := cfg_acc.AccountConfig{Name: "personal", Type: "jmap", SessionURL: "https://jmap.example.com"}
	if err := verifySetupAccount(session, newAcc); err != nil {
		t.Fatalf("verifySetupAccount failed: %v", err)
	}

	if requestedAccount != "personal" {
		t.Errorf("verifySetupAccount built its client for %q, want the newly added account %q", requestedAccount, "personal")
	}
	// The session's own selected account must be untouched by verification.
	if session.Config.SelectedAccount != "work" {
		t.Errorf("verifySetupAccount must not mutate session.Config.SelectedAccount, got %q", session.Config.SelectedAccount)
	}
}

// The loader seeds a placeholder "default" gmail account on a first run.
// Setup used to append the real account after it, so later runs selected the
// placeholder; it must be replaced.
func TestSaveNewSetupAccount_ReplacesSeededPlaceholder(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	placeholder := cfg_acc.AccountConfig{Name: "default", Type: "gmail", Username: "your-email@gmail.com", Password: "your-app-password"}
	fc := &cfg_g.FileConfig{DefaultSendAccount: "default", Accounts: &[]cfg_acc.AccountConfig{placeholder}}
	session := &Session{Config: &cfg_g.Config{SelectedAccount: "default", Accounts: []cfg_acc.AccountConfig{placeholder}}}

	newAcc := cfg_acc.AccountConfig{Name: "fastmail", Type: "jmap", Username: "user@fastmail.com", Password: "tok"}
	if err := saveNewSetupAccount(session, configPath, fc, newAcc); err != nil {
		t.Fatal(err)
	}
	if len(*fc.Accounts) != 1 || (*fc.Accounts)[0].Name != "fastmail" {
		t.Fatalf("placeholder not replaced: %+v", *fc.Accounts)
	}
	if fc.DefaultSendAccount != "fastmail" || session.Config.SelectedAccount != "fastmail" {
		t.Errorf("default sender %q, selected %q: want the new account", fc.DefaultSendAccount, session.Config.SelectedAccount)
	}
}

// A test account exists for the mutating live tests and must never become
// the default sender, even as the first account.
func TestSaveNewSetupAccount_TestAccountIsNotDefaultSender(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	fc := &cfg_g.FileConfig{Accounts: &[]cfg_acc.AccountConfig{}}
	session := &Session{Config: &cfg_g.Config{}}

	acc := cfg_acc.AccountConfig{Name: "scratch", Type: "jmap", AccountType: "test", Username: "t@example.com", Password: "tok"}
	if err := saveNewSetupAccount(session, configPath, fc, acc); err != nil {
		t.Fatal(err)
	}
	if fc.DefaultSendAccount != "" || session.Config.DefaultSendAccount != "" {
		t.Errorf("test account became the default sender: %q", fc.DefaultSendAccount)
	}
}
