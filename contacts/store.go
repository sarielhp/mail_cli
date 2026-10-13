package contacts

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sarielhp/mail_cli/app"
	"github.com/sarielhp/mail_cli/cache/dirlock"
	"github.com/sarielhp/mail_cli/cfg_g"
)

// DefaultPath returns the path to the global contacts.json file.
func DefaultPath(config *cfg_g.Config) string {
	if config != nil && config.ConfigDir != "" {
		return filepath.Join(config.ConfigDir, "contacts.json")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "contacts.json"
	}
	return filepath.Join(home, ".config", app.AppName, "contacts.json")
}

// LoadBook reads the Book from the specified JSON file, or returns an empty Book if not found.
func LoadBook(path string) (*Book, error) {
	return loadJSONFile(path, "contacts", NewBook)
}

// loadJSONFile reads a JSON document into a new T, returning an empty one when
// the file does not exist. The noun names the document in error messages.
func loadJSONFile[T any](path, noun string, empty func() *T) (*T, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return empty(), nil
		}
		return nil, fmt.Errorf("failed to read %s file %s: %w", noun, path, err)
	}

	var v T
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, fmt.Errorf("failed to parse %s file %s: %w", noun, path, err)
	}
	return &v, nil
}

// SaveBook writes the Book to the specified JSON file with directory creation and atomic rename.
func SaveBook(path string, book *Book) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	book.Updated = time.Now().UTC()
	data, err := json.MarshalIndent(book, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to serialize contacts: %w", err)
	}

	if err := dirlock.AtomicWrite(path, data, 0o600); err != nil {
		return fmt.Errorf("failed to commit contacts file %s: %w", path, err)
	}
	return nil
}

// UpdateSource replaces contacts matching sourcePrefix with the given newContacts,
// leaving all contacts from other sources intact.
func UpdateSource(book *Book, sourcePrefix string, newContacts []Contact) {
	var kept []Contact
	for _, c := range book.Contacts {
		if !strings.HasPrefix(c.Source, sourcePrefix) {
			kept = append(kept, c)
		}
	}
	kept = append(kept, newContacts...)
	book.Contacts = kept
	book.Updated = time.Now().UTC()
}

// Filter returns contacts matching a single case-insensitive substring across Name, Email, or Nickname.
func Filter(book *Book, term string) []Contact {
	if book == nil {
		return nil
	}
	clean := strings.TrimSpace(strings.ToLower(term))
	if clean == "" {
		return book.Contacts
	}

	var results []Contact
	for _, c := range book.Contacts {
		if matchContact(c, clean) {
			results = append(results, c)
		}
	}
	return results
}

// Search returns contacts where all space-separated terms in query match at least one field.
func Search(book *Book, query string) []Contact {
	if book == nil {
		return nil
	}
	terms := strings.Fields(strings.ToLower(query))
	if len(terms) == 0 {
		return book.Contacts
	}

	var results []Contact
	for _, c := range book.Contacts {
		allMatched := true
		for _, term := range terms {
			if !matchContact(c, term) {
				allMatched = false
				break
			}
		}
		if allMatched {
			results = append(results, c)
		}
	}
	return results
}

func matchContact(c Contact, term string) bool {
	if strings.Contains(strings.ToLower(c.Name), term) {
		return true
	}
	for _, e := range c.Emails {
		if strings.Contains(strings.ToLower(e), term) {
			return true
		}
	}
	for _, n := range c.Nicknames {
		if strings.Contains(strings.ToLower(n), term) {
			return true
		}
	}
	for _, g := range c.Groups {
		if strings.Contains(strings.ToLower(g), term) {
			return true
		}
	}
	if strings.Contains(strings.ToLower(c.Source), term) {
		return true
	}
	return strings.Contains(strings.ToLower(c.Notes), term)
}
