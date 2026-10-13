package virtual

import (
	"encoding/json"
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"github.com/sarielhp/mail_cli/email"
	"os"
	"path/filepath"
	"testing"

	"github.com/sarielhp/mail_cli/cfg_g"
)

// TestMigrateSanitizedNames_MovesOldFileForward is H4's cache-layout
// regression for virtual mailboxes: one saved under the pre-H4 sanitized
// file name must still be found by Load under its real name after the fix,
// with its message IDs and folder map intact, instead of looking like it
// was never saved.
func TestMigrateSanitizedNames_MovesOldFileForward(t *testing.T) {
	dir := t.TempDir()
	name := "last.2026"
	oldKey := cfg_g.LegacySanitizeLabelForCache(name)
	newKey := cfg_g.SanitizeLabelForCache(name)
	if oldKey == newKey {
		t.Fatalf("test fixture invalid: %q sanitizes the same both ways", name)
	}

	vDir := virtualDir(dir)
	if err := os.MkdirAll(vDir, 0700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	vm := Mailbox{
		Name:       name,
		MessageIDs: []email.ID{"m1", "m2"},
		FolderMap:  map[email.ID]string{"m1": "INBOX", "m2": "Archive"},
	}
	data, err := json.Marshal(vm)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if writeErr := os.WriteFile(filepath.Join(vDir, oldKey+".json"), data, 0600); writeErr != nil {
		t.Fatalf("seeding old file: %v", writeErr)
	}

	moved, err := MigrateSanitizedNames(cachedir.At(dir))
	if err != nil {
		t.Fatalf("MigrateSanitizedNames: %v", err)
	}
	if moved != 1 {
		t.Errorf("expected 1 mailbox moved, got %d", moved)
	}

	loaded, err := Load(cachedir.At(dir), name)
	if err != nil {
		t.Fatalf("Load after migration: %v", err)
	}
	if len(loaded.MessageIDs) != 2 || loaded.FolderMap["m2"] != "Archive" {
		t.Errorf("migrated mailbox content mismatch: %+v", loaded)
	}

	if _, err := os.Stat(filepath.Join(vDir, oldKey+".json")); !os.IsNotExist(err) {
		t.Errorf("expected old file removed, stat err = %v", err)
	}

	// The folder-index mirror must also be correct under the new name.
	folder := FindMessageFolder(cachedir.At(dir), "m1")
	if folder != "INBOX" {
		t.Errorf("FindMessageFolder(m1) = %q, want INBOX", folder)
	}
}

func TestMigrateSanitizedNames_SkipsNamesNeedingNoMigration(t *testing.T) {
	dir := t.TempDir()
	vm := &Mailbox{Name: "search", MessageIDs: []email.ID{"m1"}, FolderMap: map[email.ID]string{"m1": "INBOX"}}
	if err := Save(cachedir.At(dir), vm); err != nil {
		t.Fatalf("Save: %v", err)
	}

	moved, err := MigrateSanitizedNames(cachedir.At(dir))
	if err != nil {
		t.Fatalf("MigrateSanitizedNames: %v", err)
	}
	if moved != 0 {
		t.Errorf("expected 0 mailboxes moved for a name needing no migration, got %d", moved)
	}
}

func TestMigrateSanitizedNames_DoesNotOverwriteExistingNewFile(t *testing.T) {
	dir := t.TempDir()
	name := "last.2026"
	oldKey := cfg_g.LegacySanitizeLabelForCache(name)

	vDir := virtualDir(dir)
	if err := os.MkdirAll(vDir, 0700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	staleData, _ := json.Marshal(Mailbox{Name: name, MessageIDs: []email.ID{"stale"}})
	if err := os.WriteFile(filepath.Join(vDir, oldKey+".json"), staleData, 0600); err != nil {
		t.Fatalf("seeding old file: %v", err)
	}

	// A fresh mailbox already exists under the new name.
	fresh := &Mailbox{Name: name, MessageIDs: []email.ID{"fresh"}}
	if err := Save(cachedir.At(dir), fresh); err != nil {
		t.Fatalf("Save (fresh): %v", err)
	}

	if _, err := MigrateSanitizedNames(cachedir.At(dir)); err != nil {
		t.Fatalf("MigrateSanitizedNames: %v", err)
	}

	loaded, err := Load(cachedir.At(dir), name)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(loaded.MessageIDs) != 1 || loaded.MessageIDs[0] != "fresh" {
		t.Errorf("expected the fresh mailbox to survive untouched by the stale old one, got %+v", loaded)
	}
}
