package cfg_g

import (
	"testing"

	"github.com/sarielhp/mail_cli/cfg_acc"
)

func accountsForNameTests() *Config {
	off := false
	return &Config{
		Accounts: []cfg_acc.AccountConfig{
			{Name: "personal"},
			{Name: "work", DisplayName: "Work Mail", Aliases: []string{"w"}, SaveSentCopy: &off},
		},
		SaveSentCopy: true,
	}
}

// SelectAccount stores the canonical name whatever identifier it was given.
func TestSelectAccountCanonicalizes(t *testing.T) {
	config := accountsForNameTests()
	for _, ident := range []string{"work", "WORK", "Work Mail", "w"} {
		scoped, err := SelectAccount(config, ident)
		if err != nil {
			t.Fatalf("SelectAccount(%q): %v", ident, err)
		}
		if scoped.SelectedAccount != "work" {
			t.Errorf("SelectAccount(%q) selected %q, want work", ident, scoped.SelectedAccount)
		}
	}
	if _, err := SelectAccount(config, "nonesuch"); err == nil {
		t.Error("an unknown account was selected")
	}
	if config.SelectedAccount != "" {
		t.Error("SelectAccount changed the caller's config")
	}
}

func TestIsSelected(t *testing.T) {
	config := accountsForNameTests()
	if !config.IsSelected(config.Accounts[0]) || config.IsSelected(config.Accounts[1]) {
		t.Error("no selection must mean the first account")
	}
	config.SelectedAccount = "work"
	if config.IsSelected(config.Accounts[0]) || !config.IsSelected(config.Accounts[1]) {
		t.Error("work is selected")
	}
}

// The per-account setting used to be missed when -A named the account by
// its display name, because the raw identifier was compared to acc.Name.
func TestIsSaveSentCopyEnabledUsesSelectedAccount(t *testing.T) {
	scoped, err := SelectAccount(accountsForNameTests(), "Work Mail")
	if err != nil {
		t.Fatal(err)
	}
	if scoped.IsSaveSentCopyEnabled() {
		t.Error("work disables save_sent_copy; the global default was used instead")
	}
}
