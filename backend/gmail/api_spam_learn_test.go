package gmail

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarielhp/mail_cli/report"
)

func TestLoadTrainedSpamMap(t *testing.T) {
	rep := report.Discard()

	t.Run("non-existent file", func(t *testing.T) {
		tempDir := t.TempDir()
		dbPath := filepath.Join(tempDir, "trained_ids.json")
		m, err := loadTrainedSpamMap(rep, dbPath, false, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(m) != 0 {
			t.Fatalf("expected empty map, got %v", m)
		}
	})

	t.Run("valid file", func(t *testing.T) {
		tempDir := t.TempDir()
		dbPath := filepath.Join(tempDir, "trained_ids.json")
		if err := os.WriteFile(dbPath, []byte(`["id1", "id2"]`), 0600); err != nil {
			t.Fatal(err)
		}
		m, err := loadTrainedSpamMap(rep, dbPath, false, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(m) != 2 || !m["id1"] || !m["id2"] {
			t.Fatalf("unexpected map contents: %v", m)
		}
	})

	t.Run("corrupted file fails closed", func(t *testing.T) {
		tempDir := t.TempDir()
		dbPath := filepath.Join(tempDir, "trained_ids.json")
		if err := os.WriteFile(dbPath, []byte(`{invalid-json`), 0600); err != nil {
			t.Fatal(err)
		}
		_, err := loadTrainedSpamMap(rep, dbPath, false, false)
		if err == nil {
			t.Fatal("expected error on corrupted file, got nil")
		}
		if !strings.Contains(err.Error(), "delete the file to start the trained-spam record over") {
			t.Fatalf("expected remediation hint in error, got: %v", err)
		}
	})

	t.Run("corrupted file ignored when force is true", func(t *testing.T) {
		tempDir := t.TempDir()
		dbPath := filepath.Join(tempDir, "trained_ids.json")
		if err := os.WriteFile(dbPath, []byte(`{invalid-json`), 0600); err != nil {
			t.Fatal(err)
		}
		m, err := loadTrainedSpamMap(rep, dbPath, true, false)
		if err != nil {
			t.Fatalf("expected nil error with force=true, got: %v", err)
		}
		if len(m) != 0 {
			t.Fatalf("expected empty map, got %v", m)
		}
	})
}
