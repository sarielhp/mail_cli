package mailops

import (
	"encoding/json"
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"github.com/sarielhp/mail_cli/email"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/sarielhp/mail_cli/cache/folderindex"
	"github.com/sarielhp/mail_cli/cache/virtual"
	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/label"
	"github.com/sarielhp/mail_cli/report"
)

// TestNewMailClient_MigratesAccountCacheDirectory is H4's cache-layout
// regression at the account level: an account whose cache directory was
// written under the pre-H4 sanitized name must still be found -- not look
// freshly empty -- after NewMailClient resolves it under the fixed,
// injective name.
func TestNewMailClient_MigratesAccountCacheDirectory(t *testing.T) {
	base := t.TempDir()
	acc := cfg_acc.AccountConfig{Name: "corp.com", Type: "jmap", SessionURL: "https://jmap.example.com"}
	globalCfg := &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: base}, AccountCache: cachedir.At(base)}

	oldDir := filepath.Join(base, cfg_g.LegacySanitizeLabelForCache(acc.Name))
	newDir := filepath.Join(base, cfg_g.SanitizeLabelForCache(acc.Name))
	if oldDir == newDir {
		t.Fatalf("test fixture invalid: %q sanitizes the same both ways", acc.Name)
	}
	if err := os.MkdirAll(oldDir, 0700); err != nil {
		t.Fatalf("seeding old cache dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(oldDir, "message_info.bin"), []byte("keep-me"), 0600); err != nil {
		t.Fatalf("seeding marker file: %v", err)
	}

	if _, err := NewMailClient(acc, globalCfg, report.Or(nil)); err != nil {
		t.Fatalf("NewMailClient: %v", err)
	}

	if _, err := os.Stat(filepath.Join(newDir, "message_info.bin")); err != nil {
		t.Errorf("expected the account's cache content to be migrated to the new directory: %v", err)
	}
	if _, err := os.Stat(oldDir); !os.IsNotExist(err) {
		t.Errorf("expected the old cache directory to be gone after migration, stat err = %v", err)
	}
}

// TestMigrateAccountCacheLayout_MovesFolderIndexesAndVirtualMailboxes is
// H4's regression for what lives inside an account's cache directory: a
// folder index named in the account's own cached label listing, and a
// virtual mailbox, must both still be found under their pre-H4 sanitized
// names once this runs.
func TestMigrateAccountCacheLayout_MovesFolderIndexesAndVirtualMailboxes(t *testing.T) {
	dir := t.TempDir()
	folderName := "Sent.Items"
	oldFolderKey := cfg_g.LegacySanitizeLabelForCache(folderName)
	newFolderKey := cfg_g.SanitizeLabelForCache(folderName)
	if oldFolderKey == newFolderKey {
		t.Fatalf("test fixture invalid: %q sanitizes the same both ways", folderName)
	}

	// Seed the account's cached label listing so migrateAccountCacheLayout
	// knows folderName exists.
	items := []label.Item{{Name: folderName, FullName: folderName}}
	data, err := json.Marshal(items)
	if err != nil {
		t.Fatalf("Marshal label items: %v", err)
	}
	if writeErr := os.WriteFile(filepath.Join(dir, "labels_cache.json"), data, 0600); writeErr != nil {
		t.Fatalf("seeding labels_cache.json: %v", writeErr)
	}

	// Seed the old-sanitized folder index.
	idxDir := filepath.Join(dir, "indexes")
	if mkdirErr := os.MkdirAll(idxDir, 0700); mkdirErr != nil {
		t.Fatalf("MkdirAll indexes: %v", mkdirErr)
	}
	idData, _ := json.Marshal([]string{"m1"})
	if writeErr := os.WriteFile(filepath.Join(idxDir, oldFolderKey+".json"), idData, 0600); writeErr != nil {
		t.Fatalf("seeding old index: %v", writeErr)
	}

	// Seed an old-sanitized virtual mailbox.
	vName := "search.results"
	oldVKey := cfg_g.LegacySanitizeLabelForCache(vName)
	newVKey := cfg_g.SanitizeLabelForCache(vName)
	if oldVKey == newVKey {
		t.Fatalf("test fixture invalid: %q sanitizes the same both ways", vName)
	}
	vDir := filepath.Join(dir, "virtual")
	if mkdirErr := os.MkdirAll(vDir, 0700); mkdirErr != nil {
		t.Fatalf("MkdirAll virtual: %v", mkdirErr)
	}
	vmData, _ := json.Marshal(virtual.Mailbox{Name: vName, MessageIDs: []email.ID{"m2"}})
	if writeErr := os.WriteFile(filepath.Join(vDir, oldVKey+".json"), vmData, 0600); writeErr != nil {
		t.Fatalf("seeding old virtual mailbox: %v", writeErr)
	}

	migrateAccountCacheLayout(cachedir.At(dir))

	ids, err := folderindex.IDs(cachedir.At(dir), folderName)
	if err != nil || !has(email.IDStrings(ids), "m1") {
		t.Errorf("expected folder index migrated, IDs = %v, err = %v", ids, err)
	}
	if _, statErr := os.Stat(filepath.Join(idxDir, oldFolderKey+".json")); !os.IsNotExist(statErr) {
		t.Errorf("expected old folder index removed, stat err = %v", statErr)
	}

	vm, err := virtual.Load(cachedir.At(dir), vName)
	if err != nil {
		t.Fatalf("Load virtual mailbox after migration: %v", err)
	}
	if len(vm.MessageIDs) != 1 || vm.MessageIDs[0] != "m2" {
		t.Errorf("virtual mailbox content mismatch after migration: %+v", vm)
	}
	if _, err := os.Stat(filepath.Join(vDir, oldVKey+".json")); !os.IsNotExist(err) {
		t.Errorf("expected old virtual mailbox file removed, stat err = %v", err)
	}
}

func has(ids []string, want string) bool {
	return slices.Contains(ids, want)
}
