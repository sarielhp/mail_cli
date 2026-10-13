package folderindex

import (
	"encoding/json"
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"github.com/sarielhp/mail_cli/email"
	"os"
	"path/filepath"
	"testing"

	"github.com/sarielhp/mail_cli/cfg_g"
)

// TestMigrateSanitizedNames_MovesBaseAndDeltasForward is H4's cache-layout
// regression for folder indexes: a folder cached under the pre-H4 sanitized
// key must still be found -- base file and any pending deltas both -- after
// the fix, instead of looking freshly empty because IDs() now looks under a
// different key that nothing was ever written to.
func TestMigrateSanitizedNames_MovesBaseAndDeltasForward(t *testing.T) {
	dir := t.TempDir()
	folder := "inbox.2026"
	oldKey := cfg_g.LegacySanitizeLabelForCache(folder)
	newKey := cfg_g.SanitizeLabelForCache(folder)
	if oldKey == newKey {
		t.Fatalf("test fixture invalid: %q sanitizes the same both ways", folder)
	}

	idxDir := filepath.Join(dir, "indexes")
	if err := os.MkdirAll(idxDir, 0700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	baseData, _ := json.Marshal([]string{"m1", "m2"})
	if err := os.WriteFile(filepath.Join(idxDir, oldKey+".json"), baseData, 0600); err != nil {
		t.Fatalf("seeding old base file: %v", err)
	}
	deltaLine, _ := json.Marshal("m3")
	if err := os.WriteFile(filepath.Join(idxDir, oldKey+".deltas.jsonl"), append(deltaLine, '\n'), 0600); err != nil {
		t.Fatalf("seeding old deltas file: %v", err)
	}

	moved, err := MigrateSanitizedNames(cachedir.At(dir), []string{folder})
	if err != nil {
		t.Fatalf("MigrateSanitizedNames: %v", err)
	}
	if moved != 1 {
		t.Errorf("expected 1 folder moved, got %d", moved)
	}

	ids, err := IDs(cachedir.At(dir), folder)
	if err != nil {
		t.Fatalf("IDs after migration: %v", err)
	}
	for _, want := range []string{"m1", "m2", "m3"} {
		if !has(ids, email.ID(want)) {
			t.Errorf("expected %s to survive migration, got %v", want, ids)
		}
	}

	if _, err := os.Stat(filepath.Join(idxDir, oldKey+".json")); !os.IsNotExist(err) {
		t.Errorf("expected old base file removed, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(idxDir, oldKey+".deltas.jsonl")); !os.IsNotExist(err) {
		t.Errorf("expected old deltas file removed, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(idxDir, newKey+".json")); err != nil {
		t.Errorf("expected new base file to exist: %v", err)
	}
}

func TestMigrateSanitizedNames_SkipsNamesNeedingNoMigration(t *testing.T) {
	dir := t.TempDir()
	if err := ReplaceAll(cachedir.At(dir), "INBOX", []email.ID{"m1"}); err != nil {
		t.Fatalf("ReplaceAll: %v", err)
	}

	moved, err := MigrateSanitizedNames(cachedir.At(dir), []string{"INBOX"})
	if err != nil {
		t.Fatalf("MigrateSanitizedNames: %v", err)
	}
	if moved != 0 {
		t.Errorf("expected 0 folders moved for a name needing no migration, got %d", moved)
	}

	ids, err := IDs(cachedir.At(dir), "INBOX")
	if err != nil || !has(ids, "m1") {
		t.Errorf("expected INBOX untouched, got %v, err %v", ids, err)
	}
}

func TestMigrateSanitizedNames_DoesNotOverwriteExistingNewIndex(t *testing.T) {
	dir := t.TempDir()
	folder := "inbox.2026"
	oldKey := cfg_g.LegacySanitizeLabelForCache(folder)

	idxDir := filepath.Join(dir, "indexes")
	if err := os.MkdirAll(idxDir, 0700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	oldData, _ := json.Marshal([]string{"stale"})
	if err := os.WriteFile(filepath.Join(idxDir, oldKey+".json"), oldData, 0600); err != nil {
		t.Fatalf("seeding old file: %v", err)
	}

	// A fresh, correctly named index already exists -- e.g. the account was
	// re-scanned since upgrading, before migration ever ran.
	if err := ReplaceAll(cachedir.At(dir), folder, []email.ID{"fresh"}); err != nil {
		t.Fatalf("ReplaceAll (fresh): %v", err)
	}

	if _, err := MigrateSanitizedNames(cachedir.At(dir), []string{folder}); err != nil {
		t.Fatalf("MigrateSanitizedNames: %v", err)
	}

	ids, err := IDs(cachedir.At(dir), folder)
	if err != nil {
		t.Fatalf("IDs: %v", err)
	}
	if !has(ids, "fresh") || has(ids, "stale") {
		t.Errorf("expected the fresh index to survive untouched by the stale old one, got %v", ids)
	}
}
