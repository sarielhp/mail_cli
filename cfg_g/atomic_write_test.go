package cfg_g

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarielhp/mail_cli/cfg_acc"
)

func sampleFileConfig() *FileConfig {
	return &FileConfig{
		Accounts: &[]cfg_acc.AccountConfig{
			{
				Name:      "personal",
				Type:      "gmail",
				Whitelist: []string{"friend@example.com"},
				Blacklist: []string{"@spam.example"},
				Rules:     []cfg_acc.Rule{{Sender: "a@b.c", Label: "Inbox/A"}},
			},
		},
	}
}

func TestSaveConfigFile_RoundTrips(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	if err := SaveConfigFile(path, sampleFileConfig()); err != nil {
		t.Fatalf("SaveConfigFile: %v", err)
	}
	loaded, err := LoadConfigFile(path)
	if err != nil {
		t.Fatalf("LoadConfigFile: %v", err)
	}
	if loaded.Accounts == nil || len(*loaded.Accounts) != 1 {
		t.Fatalf("expected 1 account, got %#v", loaded.Accounts)
	}
	if got := (*loaded.Accounts)[0].Name; got != "personal" {
		t.Errorf("account name = %q, want personal", got)
	}
}

// TestSaveConfigFile_LeavesNoTempFiles ensures the atomic write cleans up after
// itself, so a config directory does not accumulate debris.
func TestSaveConfigFile_LeavesNoTempFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := SaveConfigFile(path, sampleFileConfig()); err != nil {
		t.Fatalf("SaveConfigFile: %v", err)
	}

	var stray []string
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.Contains(d.Name(), ".tmp-") {
			stray = append(stray, p)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if len(stray) != 0 {
		t.Errorf("temp files left behind: %v", stray)
	}
}

// TestSaveConfigFile_ReportsAccountWriteFailure covers the case that previously
// returned nil: the per-account files were written with `_ = os.WriteFile(...)`,
// so a full disk or a permissions problem silently lost a whitelist or rule
// update while the command reported success.
func TestSaveConfigFile_ReportsAccountWriteFailure(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: permission bits are not enforced")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	// Pre-create the accounts tree, then make it unwritable so the account
	// files cannot be created while config.json itself still could be.
	accountsDir := filepath.Join(dir, "accounts", "personal")
	if err := os.MkdirAll(accountsDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.Chmod(accountsDir, 0500); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(accountsDir, 0700) })

	if err := SaveConfigFile(path, sampleFileConfig()); err == nil {
		t.Error("expected an error when an account file cannot be written, got nil")
	}
}

// TestAtomicWriteFile_PreservesExistingOnFailure is the property that matters:
// a failed write must not damage the file that was already there.
func TestAtomicWriteFile_PreservesExistingOnFailure(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: permission bits are not enforced")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	original := []byte(`{"good":"original"}`)
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Make the directory unwritable: the temp file cannot be created, so the
	// write fails before the existing file is ever touched.
	if err := os.Chmod(dir, 0500); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0700) })

	if err := atomicWriteFile(path, []byte(`{"bad":`), 0600); err == nil {
		t.Fatal("expected atomicWriteFile to fail in an unwritable directory")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading back: %v", err)
	}
	if string(got) != string(original) {
		t.Errorf("existing file was damaged: got %q, want %q", got, original)
	}
}
