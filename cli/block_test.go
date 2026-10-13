package cli

import (
	"bytes"
	"encoding/json"
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sarielhp/mail_cli/cache"
	"github.com/sarielhp/mail_cli/cache/folderindex"
	"github.com/sarielhp/mail_cli/cache/msg"
	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/mailclient"
)

// setupTestBlockSession wires a session whose global Config.AccountCache and
// whose validated client's per-account Config.AccountCache are deliberately
// different directories, mirroring production (`<download_dir>/<account>/`,
// mailops/factory.go). msg block must resolve cached messages through the
// client's account directory, not the session's global one (H1); a test that
// pointed both at the same directory would pass whichever one the code used
// and could not catch that regression.
func setupTestBlockSession(t *testing.T) (session *Session, configPath, acctDir string) {
	t.Helper()
	tempDir := t.TempDir()
	configPath = filepath.Join(tempDir, "config.json")

	fc := &cfg_g.FileConfig{
		Accounts: &[]cfg_acc.AccountConfig{
			{
				Name:           "default",
				Type:           "gmail",
				Username:       "test@gmail.com",
				Password:       "pass",
				IMAPHost:       "imap.gmail.com",
				SpamFolder:     "Spam",
				ReceivedFolder: "INBOX",
				SpamLearn:      "SpamLearn",
				Rules:          []cfg_acc.Rule{},
				Blacklist:      []string{},
			},
		},
	}
	data, err := json.Marshal(fc)
	if err != nil {
		t.Fatalf("failed to marshal setup config: %v", err)
	}
	if err := os.WriteFile(configPath, data, 0600); err != nil {
		t.Fatalf("failed to write setup config: %v", err)
	}

	globalDir := filepath.Join(tempDir, "cache")
	acctDir = filepath.Join(globalDir, "default")
	if err := os.MkdirAll(acctDir, 0700); err != nil {
		t.Fatalf("failed to create account cache dir: %v", err)
	}

	config := &cfg_g.Config{
		SelectedAccount: "default",
		Paths:           cfg_g.Paths{ConfigDir: tempDir, CacheRoot: globalDir},
	}

	var out, status bytes.Buffer
	session = &Session{
		Config: config,
		Out:    &out,
		Err:    &status,
		Deps: Deps{
			Clients: stubClients{
				ForSelectedFn: func(c *cfg_g.Config) (mailclient.MailClient, error) {
					acctCfg := *c
					acctCfg.CacheRoot = acctDir
					acctCfg.AccountCache = cachedir.At(acctCfg.CacheRoot)
					// Wrapped in CheckingMailClient, matching what
					// mailops.NewMailClient always returns in production:
					// msg block's MoveEmail call reindexes through this
					// wrapper, not through any manual code in cli/msg.go
					// (see mailclient.TestCheckingMailClient_MoveEmail_ReindexesAndRespectsReadOnly).
					return mailclient.NewCheckingMailClient(&mailclient.MockMailClient{Cfg: &acctCfg}, "default", &cache.NoOpCacheStore{}), nil
				},
			},
			Contacts: stubContacts{
				PurgeFn: func(_ *cfg_g.Config, _ string) error {
					return nil
				},
			},
		},
	}
	return session, configPath, acctDir
}

func TestBlock_SpamDefault(t *testing.T) {
	session, configPath, _ := setupTestBlockSession(t)
	flagBlockTrash = false
	flagBlockPush = false

	err := whitelistOrBlacklist(session, "blacklistadd", []string{"spammer@example.com"})
	if err != nil {
		t.Fatalf("whitelistOrBlacklist failed: %v", err)
	}

	loadedFc, err := cfg_g.LoadConfigFile(configPath)
	if err != nil {
		t.Fatalf("load config failed: %v", err)
	}
	acc := (*loadedFc.Accounts)[0]

	// Must be in blacklist
	if len(acc.Blacklist) != 1 || acc.Blacklist[0] != "spammer@example.com" {
		t.Errorf("expected spammer in Blacklist, got %+v", acc.Blacklist)
	}

	// Must have SpamLearn rule
	if len(acc.Rules) != 1 || acc.Rules[0].Sender != "spammer@example.com" || acc.Rules[0].Label != "SpamLearn" {
		t.Errorf("expected SpamLearn rule, got %+v", acc.Rules)
	}
}

func TestBlock_TrashOption(t *testing.T) {
	session, configPath, _ := setupTestBlockSession(t)
	flagBlockTrash = true
	flagBlockPush = false
	defer func() { flagBlockTrash = false }()

	err := whitelistOrBlacklist(session, "blacklistadd", []string{"annoying@example.com"})
	if err != nil {
		t.Fatalf("whitelistOrBlacklist failed: %v", err)
	}

	loadedFc, err := cfg_g.LoadConfigFile(configPath)
	if err != nil {
		t.Fatalf("load config failed: %v", err)
	}
	acc := (*loadedFc.Accounts)[0]

	// Must NOT be in blacklist
	if len(acc.Blacklist) != 0 {
		t.Errorf("Trash-blocked sender must not be in Blacklist, got %+v", acc.Blacklist)
	}

	// Must have Trash rule
	if len(acc.Rules) != 1 || acc.Rules[0].Sender != "annoying@example.com" || acc.Rules[0].Label != "Trash" {
		t.Errorf("expected Trash rule, got %+v", acc.Rules)
	}

	// Now remove it
	err = whitelistOrBlacklist(session, "blacklistdel", []string{"annoying@example.com"})
	if err != nil {
		t.Fatalf("blacklistdel failed: %v", err)
	}

	loadedFc, err = cfg_g.LoadConfigFile(configPath)
	if err != nil {
		t.Fatalf("load config failed: %v", err)
	}
	acc = (*loadedFc.Accounts)[0]
	if len(acc.Rules) != 0 {
		t.Errorf("expected rule removed after del, got %+v", acc.Rules)
	}
}

func TestBlock_MsgBlockCommand(t *testing.T) {
	session, configPath, acctDir := setupTestBlockSession(t)

	raw := "From: Sales Bot <bot@vendor.com>\nSubject: Special Offer\nMessage-ID: <offer123@vendor.com>\n\nBuy now!"
	// The message is stored under the per-account directory the validated
	// client resolves to, never under session.Config.AccountCache (the global
	// base) -- that mismatch is exactly what H1 fixes.
	if err := msg.Store(cachedir.At(acctDir), "offer123", []byte(raw), time.Now()); err != nil {
		t.Fatalf("msg.Store: %v", err)
	}
	if err := folderindex.Add(cachedir.At(acctDir), "INBOX", "offer123"); err != nil {
		t.Fatalf("folderindex.Add: %v", err)
	}

	// Block by ID to Trash
	if err := blockByIDs(session, []string{"offer123"}, true, false); err != nil {
		t.Fatalf("blockByIDs failed: %v", err)
	}

	loadedFc, err := cfg_g.LoadConfigFile(configPath)
	if err != nil {
		t.Fatalf("load config failed: %v", err)
	}
	acc := (*loadedFc.Accounts)[0]

	// Must have Trash rule for bot@vendor.com
	if len(acc.Rules) != 1 || acc.Rules[0].Sender != "bot@vendor.com" || acc.Rules[0].Label != "Trash" {
		t.Errorf("expected Trash rule for bot@vendor.com, got %+v", acc.Rules)
	}

	// The move must be reflected in the account's own folder index, not the
	// global download dir.
	ids, err := folderindex.IDs(cachedir.At(acctDir), "Trash")
	if err != nil {
		t.Fatalf("folderindex.IDs: %v", err)
	}
	found := false
	for _, id := range ids {
		if id == "offer123" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected offer123 to be reindexed into Trash under the account dir, got %v", ids)
	}
}

// TestBlock_MsgBlockCommand_ReadOnlySkipsLocalReindex is H2's regression: a
// --read-only / --dry-run block must not simulate MoveEmail on the server
// (mock client does that verbatim) yet still leaves the local folder index
// untouched, since nothing actually moved.
func TestBlock_MsgBlockCommand_ReadOnlySkipsLocalReindex(t *testing.T) {
	session, _, acctDir := setupTestBlockSession(t)
	session.Config.ReadOnly = true

	raw := "From: Sales Bot <bot@vendor.com>\nSubject: Special Offer\nMessage-ID: <offer123@vendor.com>\n\nBuy now!"
	if err := msg.Store(cachedir.At(acctDir), "offer123", []byte(raw), time.Now()); err != nil {
		t.Fatalf("msg.Store: %v", err)
	}
	if err := folderindex.Add(cachedir.At(acctDir), "INBOX", "offer123"); err != nil {
		t.Fatalf("folderindex.Add: %v", err)
	}

	if err := blockByIDs(session, []string{"offer123"}, true, false); err != nil {
		t.Fatalf("blockByIDs failed: %v", err)
	}

	ids, err := folderindex.IDs(cachedir.At(acctDir), "INBOX")
	if err != nil {
		t.Fatalf("folderindex.IDs: %v", err)
	}
	found := false
	for _, id := range ids {
		if id == "offer123" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected offer123 to remain indexed under INBOX under --read-only, got %v", ids)
	}
}

// TestBlock_MsgBlockCommand_PerAccountReadOnlySkipsLocalReindex is a
// regression for a bug the H2 fix itself introduced: it gated the local
// reindex on session.Config.ReadOnly (the global --read-only/--dry-run
// flag), but an account can carry its own `read_only: true` without that
// global flag being set -- mailops.BuildAccountLocalConfig ORs it into the
// client's own Config().ReadOnly. That account's MoveEmail is still only
// simulated, so the gate has to read the client's config, not the session's,
// or it still records a move that never reached the server.
func TestBlock_MsgBlockCommand_PerAccountReadOnlySkipsLocalReindex(t *testing.T) {
	tempDir := t.TempDir()
	acctDir := filepath.Join(tempDir, "cache", "default")
	if err := os.MkdirAll(acctDir, 0700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	configPath := filepath.Join(tempDir, "config.json")
	fc := &cfg_g.FileConfig{
		Accounts: &[]cfg_acc.AccountConfig{{
			Name: "default", Type: "gmail", Username: "test@gmail.com", Password: "pass",
			IMAPHost: "imap.gmail.com", SpamFolder: "Spam", ReceivedFolder: "INBOX", SpamLearn: "SpamLearn",
		}},
	}
	data, err := json.Marshal(fc)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	if writeErr := os.WriteFile(configPath, data, 0600); writeErr != nil {
		t.Fatalf("write config: %v", writeErr)
	}

	var out, status bytes.Buffer
	session := &Session{
		// The session's own (global) ReadOnly is deliberately false: no
		// --read-only/--dry-run flag was passed.
		Config: &cfg_g.Config{SelectedAccount: "default", Paths: cfg_g.Paths{ConfigDir: tempDir}},
		Out:    &out,
		Err:    &status,
		Deps: Deps{
			Clients: stubClients{
				ForSelectedFn: func(c *cfg_g.Config) (mailclient.MailClient, error) {
					acctCfg := *c
					acctCfg.CacheRoot = acctDir
					acctCfg.AccountCache = cachedir.At(acctCfg.CacheRoot)
					// The account itself is read-only, independent of the
					// session's global flag.
					acctCfg.ReadOnly = true
					return mailclient.NewCheckingMailClient(&mailclient.MockMailClient{Cfg: &acctCfg}, "default", &cache.NoOpCacheStore{}), nil
				},
			},
			Contacts: stubContacts{PurgeFn: func(_ *cfg_g.Config, _ string) error { return nil }},
		},
	}

	raw := "From: Sales Bot <bot@vendor.com>\nSubject: Special Offer\nMessage-ID: <offer123@vendor.com>\n\nBuy now!"
	if storeErr := msg.Store(cachedir.At(acctDir), "offer123", []byte(raw), time.Now()); storeErr != nil {
		t.Fatalf("msg.Store: %v", storeErr)
	}
	if indexErr := folderindex.Add(cachedir.At(acctDir), "INBOX", "offer123"); indexErr != nil {
		t.Fatalf("folderindex.Add: %v", indexErr)
	}

	if blockErr := blockByIDs(session, []string{"offer123"}, true, false); blockErr != nil {
		t.Fatalf("blockByIDs failed: %v", blockErr)
	}

	ids, err := folderindex.IDs(cachedir.At(acctDir), "INBOX")
	if err != nil {
		t.Fatalf("folderindex.IDs: %v", err)
	}
	found := false
	for _, id := range ids {
		if id == "offer123" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected offer123 to remain indexed under INBOX for a per-account read-only account, got %v", ids)
	}
}
