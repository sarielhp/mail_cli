package runner

import (
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/mailclient"
)

// The per-backend token layout moved here with AccountService, from a cli test
// that had to import gmail and outlook to assert on it.
func TestAccountServiceTokenPath(t *testing.T) {
	dir := t.TempDir()

	tests := []struct {
		name       string
		acc        cfg_acc.AccountConfig
		wantEmpty  bool
		wantPrefix string
		wantSubstr string
	}{
		{
			name:       "gmail names the account in the file",
			acc:        cfg_acc.AccountConfig{Name: "my-gmail", Type: "gmail"},
			wantPrefix: dir,
			wantSubstr: "my-gmail",
		},
		{
			name:       "outlook names the account and a hash",
			acc:        cfg_acc.AccountConfig{Name: "work-outlook", Type: "outlook"},
			wantPrefix: dir,
			wantSubstr: "work-outlook",
		},
		{
			name:      "a backend without OAuth has no token",
			acc:       cfg_acc.AccountConfig{Name: "local", Type: "mbox"},
			wantEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := AccountService{}.TokenPath(dir, tt.acc)
			if err != nil {
				t.Fatalf("TokenPath: %v", err)
			}
			if tt.wantEmpty {
				if got != "" {
					t.Errorf("expected no token path, got %q", got)
				}
				return
			}
			if filepath.Dir(got) != tt.wantPrefix {
				t.Errorf("expected token under %q, got %q", tt.wantPrefix, got)
			}
			if !strings.Contains(got, tt.wantSubstr) {
				t.Errorf("expected %q in token path, got %q", tt.wantSubstr, got)
			}
		})
	}
}

// OpenMbox must hand back a client that cannot write to the file it is viewing.
func TestAccountServiceOpenMboxIsReadOnly(t *testing.T) {
	dir := t.TempDir()
	mboxPath := filepath.Join(dir, "view.mbox")
	if err := os.WriteFile(mboxPath, []byte("From x@y Mon Jan  1 00:00:00 2024\nSubject: hi\n\nbody\n"), 0600); err != nil {
		t.Fatalf("write mbox: %v", err)
	}

	config := &cfg_g.Config{Accounts: []cfg_acc.AccountConfig{
		{Name: "mbox", Type: "mbox", ReadOnly: true},
	}, Paths: cfg_g.Paths{CacheRoot: dir}, AccountCache: cachedir.At(dir)}

	client, err := AccountService{}.OpenMbox(config, mboxPath, cachedir.At(dir))
	if err != nil {
		t.Fatalf("OpenMbox: %v", err)
	}

	checking, ok := client.(*mailclient.CheckingMailClient)
	if !ok {
		t.Fatalf("expected a CheckingMailClient, got %T", client)
	}
	if _, ok := checking.Delegate.(*mailclient.ReadOnlyMailClient); !ok {
		t.Errorf("expected the delegate to be a ReadOnlyMailClient, got %T", checking.Delegate)
	}
	if client.BackendType() != "mbox" {
		t.Errorf("expected backend type mbox, got %q", client.BackendType())
	}
}

// An mbox view without its synthetic account has nothing to build a client from.
func TestAccountServiceOpenMboxRequiresAccount(t *testing.T) {
	if _, err := (AccountService{}).OpenMbox(&cfg_g.Config{}, "x.mbox", cachedir.At(t.TempDir())); err == nil {
		t.Error("expected an error when the config carries no account")
	}
}
