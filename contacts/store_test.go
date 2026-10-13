package contacts

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStore_SaveLoadAndFilter(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "contacts.json")

	book, err := LoadBook(path)
	if err != nil {
		t.Fatalf("expected empty book, got error: %v", err)
	}
	if len(book.Contacts) != 0 {
		t.Fatalf("expected 0 contacts, got %d", len(book.Contacts))
	}

	book.Contacts = []Contact{
		{
			ID:        "mutt:dave",
			Name:      "Dave Rivers",
			Emails:    []string{"rivers@cs.example.edu"},
			Nicknames: []string{"dave", "rivers"},
			Groups:    []string{"algo"},
			Source:    "mutt",
		},
		{
			ID:        "google:work:c1",
			Name:      "Dave Rivers",
			Emails:    []string{"rivers@cs.example.edu"},
			Nicknames: []string{"dave"},
			Groups:    []string{"algo"},
			Source:    "google:work",
		},
		{
			ID:        "mutt:alice",
			Name:      "Alice Smith",
			Emails:    []string{"alice@example.com"},
			Nicknames: []string{"alice"},
			Source:    "mutt",
		},
	}

	if saveErr := SaveBook(path, book); saveErr != nil {
		t.Fatalf("SaveBook failed: %v", saveErr)
	}

	loaded, err := LoadBook(path)
	if err != nil {
		t.Fatalf("LoadBook failed: %v", err)
	}
	if len(loaded.Contacts) != 3 {
		t.Fatalf("expected 3 contacts, got %d", len(loaded.Contacts))
	}

	// Filter
	daves := Filter(loaded, "rivers")
	if len(daves) != 2 {
		t.Errorf("expected 2 daves matching 'rivers', got %d", len(daves))
	}

	// Search
	results := Search(loaded, "dave google")
	if len(results) != 1 {
		t.Errorf("expected 1 result matching 'dave google', got %d", len(results))
	} else if results[0].Source != "google:work" {
		t.Errorf("expected google:work source, got %q", results[0].Source)
	}

	// UpdateSource mutt
	newMutt := []Contact{
		{
			ID:        "mutt:bob",
			Name:      "Bob Jones",
			Emails:    []string{"bob@example.com"},
			Nicknames: []string{"bob"},
			Source:    "mutt",
		},
	}
	UpdateSource(loaded, "mutt", newMutt)

	// Google contact should still be there, mutt replaced
	if len(loaded.Contacts) != 2 {
		t.Errorf("expected 2 contacts after UpdateSource, got %d", len(loaded.Contacts))
	}
	hasGoogle := false
	hasBob := false
	for _, c := range loaded.Contacts {
		if c.Source == "google:work" {
			hasGoogle = true
		}
		if c.DisplayAlias() == "bob" {
			hasBob = true
		}
	}
	if !hasGoogle || !hasBob {
		t.Errorf("expected google:work and bob, got %v", loaded.Contacts)
	}
}

func TestSaveBook_AtomicAndDurable(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "nested", "contacts.json")

	book := &Book{
		Contacts: []Contact{
			{
				ID:     "c1",
				Name:   "Test Contact",
				Emails: []string{"test@example.com"},
				Source: "manual",
			},
		},
	}

	if err := SaveBook(path, book); err != nil {
		t.Fatalf("SaveBook in nested directory failed: %v", err)
	}

	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat contacts file failed: %v", err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("file mode = %v, want 0600", fi.Mode().Perm())
	}

	loaded, err := LoadBook(path)
	if err != nil {
		t.Fatalf("LoadBook failed: %v", err)
	}
	if len(loaded.Contacts) != 1 || loaded.Contacts[0].Name != "Test Contact" {
		t.Errorf("loaded contacts mismatch: %+v", loaded.Contacts)
	}

	// Repeated overwrite
	book.Contacts = append(book.Contacts, Contact{ID: "c2", Name: "Second Contact", Source: "manual"})
	if saveErr := SaveBook(path, book); saveErr != nil {
		t.Fatalf("repeated SaveBook failed: %v", saveErr)
	}

	loaded2, err := LoadBook(path)
	if err != nil {
		t.Fatalf("LoadBook after update failed: %v", err)
	}
	if len(loaded2.Contacts) != 2 {
		t.Errorf("expected 2 contacts, got %d", len(loaded2.Contacts))
	}
}
