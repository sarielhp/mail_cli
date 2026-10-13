package mailops

import (
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"testing"

	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/report"
)

func TestBuildAccountLocalConfig(t *testing.T) {
	globalCfg := &cfg_g.Config{SelectedAccount: "acc1", Paths: cfg_g.Paths{CacheRoot: "/base/dir"}, AccountCache: cachedir.At("/base/dir")}
	acc := cfg_acc.AccountConfig{
		Name:           "acc1",
		Username:       "user@example.com",
		Password:       "pass123",
		IMAPHost:       "imap.example.com",
		SpamFolder:     "Junk",
		ReceivedFolder: "Inbox",
		AccountType:    "regular",
		Rules: []cfg_acc.Rule{
			{Sender: "test@example.com", Label: "Test"},
		},
		Whitelist: []string{"white@example.com"},
		Blacklist: []string{"black@example.com"},
	}

	local := BuildAccountLocalConfig(acc, globalCfg)
	if local.Username != "user@example.com" {
		t.Errorf("expected user@example.com, got %q", local.Username)
	}
	if local.SelectedAccount != "acc1" {
		t.Errorf("expected SelectedAccount acc1, got %q", local.SelectedAccount)
	}
	if len(local.Rules) != 1 {
		t.Errorf("expected 1 rule, got %d", len(local.Rules))
	}
}

func TestIsAccountSelected(t *testing.T) {
	cfg := &cfg_g.Config{
		SelectedAccount: "work",
		Accounts: []cfg_acc.AccountConfig{
			{Name: "personal"},
			{Name: "work"},
		},
	}
	if !IsAccountSelected(cfg.Accounts[1], cfg) {
		t.Errorf("expected work account to be selected")
	}
	if IsAccountSelected(cfg.Accounts[0], cfg) {
		t.Errorf("expected personal account not to be selected")
	}
}

func TestAccountNames(t *testing.T) {
	cfg := &cfg_g.Config{
		Accounts: []cfg_acc.AccountConfig{
			{Name: "acc1", DisplayName: "Account 1"},
			{Name: "acc2"},
		},
	}
	names := AccountNames(cfg)
	if len(names) != 2 {
		t.Fatalf("expected 2 names, got %d", len(names))
	}
	if names[0] != "Account 1 (acc1)" {
		t.Errorf("expected 'Account 1 (acc1)', got %q", names[0])
	}
	if names[1] != "acc2" {
		t.Errorf("expected 'acc2', got %q", names[1])
	}
}

func TestForEachAccount_NotFound(t *testing.T) {
	cfg := &cfg_g.Config{
		Accounts: []cfg_acc.AccountConfig{
			{Name: "acc1"},
		},
	}
	err := ForEachAccount(cfg, "nonexistent:folder", report.Discard(),
		func(_ cfg_acc.AccountConfig, _ string) (bool, error) {
			return false, nil
		})
	if err == nil {
		t.Errorf("expected error for nonexistent account")
	}
}

func TestBuildAccountLocalConfig_PropagatesReadOnly(t *testing.T) {
	globalCfg := &cfg_g.Config{ReadOnly: false, Paths: cfg_g.Paths{CacheRoot: "/base/dir"}, AccountCache: cachedir.At("/base/dir")}
	acc := cfg_acc.AccountConfig{
		Name:     "read-only-acc",
		ReadOnly: true,
	}

	local := BuildAccountLocalConfig(acc, globalCfg)
	if !local.ReadOnly {
		t.Errorf("expected localCfg.ReadOnly=true when acc.ReadOnly=true")
	}

	// Also verify global read-only remains true even if acc.ReadOnly=false
	globalReadOnlyCfg := &cfg_g.Config{ReadOnly: true, Paths: cfg_g.Paths{CacheRoot: "/base/dir"}, AccountCache: cachedir.At("/base/dir")}
	accWritable := cfg_acc.AccountConfig{
		Name:     "writable-acc",
		ReadOnly: false,
	}
	localWritable := BuildAccountLocalConfig(accWritable, globalReadOnlyCfg)
	if !localWritable.ReadOnly {
		t.Errorf("expected localCfg.ReadOnly=true when global ReadOnly=true")
	}
}

// Every per-account override in the config file has to reach the config the
// account's operations actually read, or the setting is one the user can write
// and nothing honours.
//
// SpamLearn and UnspamLearn were the two this function did not carry, so
// scan/scan.go moved blacklisted mail to the global folder while
// cfg_g/helpers.go filed the matching rule against the account's own — the two
// disagreeing about where one message belongs.
func TestBuildAccountLocalConfig_CarriesPerAccountSpamFolders(t *testing.T) {
	globalCfg := &cfg_g.Config{Policy: cfg_g.Policy{SpamLearn: "GlobalLearnSpam", UnspamLearn: "GlobalLearnHam"}}

	t.Run("the account's own folders win", func(t *testing.T) {
		local := BuildAccountLocalConfig(cfg_acc.AccountConfig{
			Name:        "work",
			SpamLearn:   "Work/LearnSpam",
			UnspamLearn: "Work/LearnHam",
		}, globalCfg)
		if local.SpamLearn != "Work/LearnSpam" {
			t.Errorf("SpamLearn = %q, want the account's own %q", local.SpamLearn, "Work/LearnSpam")
		}
		if local.UnspamLearn != "Work/LearnHam" {
			t.Errorf("UnspamLearn = %q, want the account's own %q", local.UnspamLearn, "Work/LearnHam")
		}
	})

	t.Run("an account that sets neither falls back to the global", func(t *testing.T) {
		local := BuildAccountLocalConfig(cfg_acc.AccountConfig{Name: "personal"}, globalCfg)
		if local.SpamLearn != "GlobalLearnSpam" {
			t.Errorf("SpamLearn = %q, want the global %q", local.SpamLearn, "GlobalLearnSpam")
		}
		if local.UnspamLearn != "GlobalLearnHam" {
			t.Errorf("UnspamLearn = %q, want the global %q", local.UnspamLearn, "GlobalLearnHam")
		}
	})

	t.Run("one set and one not", func(t *testing.T) {
		local := BuildAccountLocalConfig(cfg_acc.AccountConfig{
			Name: "mixed", SpamLearn: "Mixed/LearnSpam",
		}, globalCfg)
		if local.SpamLearn != "Mixed/LearnSpam" || local.UnspamLearn != "GlobalLearnHam" {
			t.Errorf("SpamLearn=%q UnspamLearn=%q, want the account's own and the global",
				local.SpamLearn, local.UnspamLearn)
		}
	})
}
