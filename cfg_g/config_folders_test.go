package cfg_g

import (
	"testing"

	"github.com/sarielhp/mail_cli/cfg_acc"
)

// An account written without spam_folder or received_folder -- by an older
// version, or by hand -- was refused by ValidateAccountParams one missing field
// per attempt, so `account login` failed again after each fix. Loading now
// fills them with the type's defaults, the ones `account new` writes, and
// leaves anything already set alone.
func TestEvaluateAccountDefaultsFillsMissingFolders(t *testing.T) {
	accounts := []cfg_acc.AccountConfig{
		{Name: "bare", Type: "outlook", Username: "me@outlook.com"},
		{Name: "custom", Type: "jmap", SpamFolder: "Junk", ReceivedFolder: "Done"},
		{Name: "odd", Type: "imap"},
	}
	fc := &FileConfig{Accounts: &accounts}

	if !evaluateAccountDefaults(fc) {
		t.Fatal("filling folders must mark the config dirty so it is saved")
	}
	got := *fc.Accounts
	if got[0].SpamFolder != "Junk Email" || got[0].ReceivedFolder != "Archive" {
		t.Errorf("outlook defaults = %q/%q, want Junk Email/Archive", got[0].SpamFolder, got[0].ReceivedFolder)
	}
	if err := ValidateAccountParams(got[0]); err != nil {
		t.Errorf("a filled-in outlook account still fails validation: %v", err)
	}
	if got[1].SpamFolder != "Junk" || got[1].ReceivedFolder != "Done" {
		t.Errorf("configured folders were overwritten: %q/%q", got[1].SpamFolder, got[1].ReceivedFolder)
	}
	if got[2].SpamFolder != "" || got[2].ReceivedFolder != "" {
		t.Errorf("an unknown type was given folders: %q/%q", got[2].SpamFolder, got[2].ReceivedFolder)
	}
}
