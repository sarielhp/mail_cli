package cfg_g

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sarielhp/mail_cli/cfg_acc"
)

func TestMigrateDirSanitization_RenamesOldDirForward(t *testing.T) {
	base := t.TempDir()
	name := "corp.com"
	oldKey := LegacySanitizeLabelForCache(name)
	newKey := SanitizeLabelForCache(name)
	if oldKey == newKey {
		t.Fatalf("test fixture invalid: %q sanitizes the same both ways (%q)", name, oldKey)
	}

	oldDir := filepath.Join(base, oldKey)
	if err := os.MkdirAll(oldDir, 0700); err != nil {
		t.Fatalf("seeding old dir: %v", err)
	}
	marker := filepath.Join(oldDir, "account.json")
	if err := os.WriteFile(marker, []byte(`{"name":"corp.com"}`), 0600); err != nil {
		t.Fatalf("seeding marker file: %v", err)
	}

	MigrateDirSanitization(base, name)

	newDir := filepath.Join(base, newKey)
	if _, err := os.Stat(newDir); err != nil {
		t.Fatalf("expected the new directory to exist after migration: %v", err)
	}
	if _, err := os.Stat(filepath.Join(newDir, "account.json")); err != nil {
		t.Errorf("expected the migrated content to be present under the new path: %v", err)
	}
	if _, err := os.Stat(oldDir); !os.IsNotExist(err) {
		t.Errorf("expected the old directory to be gone after migration, stat err = %v", err)
	}
}

func TestMigrateDirSanitization_NoOpWhenNewDirAlreadyExists(t *testing.T) {
	base := t.TempDir()
	name := "corp.com"
	oldKey := LegacySanitizeLabelForCache(name)
	newKey := SanitizeLabelForCache(name)

	oldDir := filepath.Join(base, oldKey)
	newDir := filepath.Join(base, newKey)
	if err := os.MkdirAll(oldDir, 0700); err != nil {
		t.Fatalf("seeding old dir: %v", err)
	}
	if err := os.MkdirAll(newDir, 0700); err != nil {
		t.Fatalf("seeding new dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(newDir, "marker"), []byte("keep-me"), 0600); err != nil {
		t.Fatalf("seeding marker: %v", err)
	}

	MigrateDirSanitization(base, name)

	// Both directories must survive untouched: migrating would silently
	// clobber whatever is already at the new path.
	if _, err := os.Stat(oldDir); err != nil {
		t.Errorf("expected old dir to be left alone when new dir already exists: %v", err)
	}
	if _, err := os.Stat(filepath.Join(newDir, "marker")); err != nil {
		t.Errorf("expected the existing new-dir content to survive untouched: %v", err)
	}
}

func TestMigrateDirSanitization_NoOpWhenNothingToMigrate(t *testing.T) {
	base := t.TempDir()
	// "inbox" sanitizes identically both ways -- nothing should happen, and
	// in particular no directory should be created.
	MigrateDirSanitization(base, "inbox")

	entries, err := os.ReadDir(base)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected no directories created for a name needing no migration, got %v", entries)
	}
}

// TestLoadAccountFromDisk_MigratesOldAccountDirectory is H4's cache-layout
// regression: an account whose name was cached under the pre-H4 sanitized
// directory must still be found -- with its blacklist and rules intact --
// after the fix, not silently reset to empty because the new sanitization
// looks under a path nothing was ever written to.
func TestLoadAccountFromDisk_MigratesOldAccountDirectory(t *testing.T) {
	configDir := t.TempDir()
	acc := &cfg_acc.AccountConfig{Name: "corp.com", Type: "gmail"}

	// Save under the account's own directory using saveAccountToDisk, then
	// relocate that directory back to where the old, pre-H4 sanitization
	// would have put it -- reproducing what an upgrading user's disk
	// actually looks like, without hand-rolling the on-disk JSON shape.
	acc.Blacklist = []string{"spammer@example.com"}
	acc.Rules = []cfg_acc.Rule{{Sender: "spammer@example.com", Label: "Trash"}}
	if err := saveAccountToDisk(configDir, acc); err != nil {
		t.Fatalf("saveAccountToDisk: %v", err)
	}

	newDir := filepath.Join(configDir, "accounts", SanitizeLabelForCache(acc.Name))
	oldDir := filepath.Join(configDir, "accounts", LegacySanitizeLabelForCache(acc.Name))
	if newDir == oldDir {
		t.Fatalf("test fixture invalid: account name sanitizes the same both ways")
	}
	if err := os.Rename(newDir, oldDir); err != nil {
		t.Fatalf("relocating to the legacy path: %v", err)
	}

	loaded := &cfg_acc.AccountConfig{Name: "corp.com"}
	loadAccountFromDisk(configDir, loaded)

	if len(loaded.Blacklist) != 1 || loaded.Blacklist[0] != "spammer@example.com" {
		t.Errorf("blacklist not recovered from the legacy directory: %+v", loaded.Blacklist)
	}
	if len(loaded.Rules) != 1 || loaded.Rules[0].Sender != "spammer@example.com" {
		t.Errorf("rules not recovered from the legacy directory: %+v", loaded.Rules)
	}
	if _, err := os.Stat(newDir); err != nil {
		t.Errorf("expected the account directory to now live at the new sanitized path: %v", err)
	}
}
