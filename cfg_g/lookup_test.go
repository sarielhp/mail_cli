package cfg_g

import (
	"errors"
	"strings"
	"testing"

	"github.com/sarielhp/mail_cli/cfg_acc"
)

func accounts() []cfg_acc.AccountConfig {
	return []cfg_acc.AccountConfig{
		{Name: "default", DisplayName: "GMail", Type: "gmail"},
		{Name: "fastmail", DisplayName: "FastMail", Type: "jmap", Aliases: []string{"fm"}},
		{Name: "work", DisplayName: "Work", Type: "outlook"},
		{Name: "test_acct", Type: "outlook"},
	}
}

func TestLookupByEachKindOfIdentifier(t *testing.T) {
	tests := []struct {
		ident string
		want  string
	}{
		{"work", "work"},
		{"WORK", "work"},
		{"Work", "work"}, // display name
		{"FastMail", "fastmail"},
		{"fm", "fastmail"},   // alias
		{"jmap", "fastmail"}, // type, only one candidate
		{"", "default"},      // no identifier means the first account
	}

	for _, tt := range tests {
		t.Run(tt.ident, func(t *testing.T) {
			got, err := Lookup(accounts(), tt.ident)
			if err != nil {
				t.Fatalf("Lookup(%q): %v", tt.ident, err)
			}
			if got.Name != tt.want {
				t.Errorf("Lookup(%q) = %q, want %q", tt.ident, got.Name, tt.want)
			}
		})
	}
}

// A type names every account using that backend. Resolving to whichever comes
// first in the file is a silent wrong answer.
func TestLookupReportsAmbiguity(t *testing.T) {
	_, err := Lookup(accounts(), "outlook")

	var ambiguous *AmbiguousAccountError
	if !errors.As(err, &ambiguous) {
		t.Fatalf("expected an ambiguity error, got %v", err)
	}
	if len(ambiguous.Candidates) != 2 {
		t.Errorf("expected both Outlook accounts named, got %v", ambiguous.Candidates)
	}
	for _, want := range []string{"work", "test_acct"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not name %q: %s", want, err)
		}
	}
}

// A more specific match wins outright, so an account stays reachable by its own
// name even when another account's display name collides with it.
func TestLookupPrefersTheCanonicalName(t *testing.T) {
	accs := []cfg_acc.AccountConfig{
		{Name: "archive", Type: "gmail"},
		{Name: "work", DisplayName: "archive", Type: "outlook"},
	}

	got, err := Lookup(accs, "archive")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if got.Name != "archive" {
		t.Errorf("expected the account named archive, got %q", got.Name)
	}
}

func TestLookupRejectsAnUnknownIdentifier(t *testing.T) {
	_, err := Lookup(accounts(), "nonesuch")

	var unknown *UnknownAccountError
	if !errors.As(err, &unknown) {
		t.Fatalf("expected an unknown-account error, got %v", err)
	}
	if !strings.Contains(err.Error(), "default") {
		t.Errorf("error should list what is configured: %s", err)
	}
}

func TestLookupWithNoAccounts(t *testing.T) {
	if _, err := Lookup(nil, "work"); !errors.Is(err, ErrNoAccounts) {
		t.Errorf("expected ErrNoAccounts, got %v", err)
	}
}
