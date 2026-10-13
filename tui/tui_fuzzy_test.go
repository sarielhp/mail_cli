package tui

import (
	"github.com/sarielhp/mail_cli/email"
	"testing"
	"time"
)

// fuzzyCase is one fuzzyScore expectation: a query that should (or should not)
// match a target.
type fuzzyCase struct {
	query   string
	target  string
	wantPos bool
}

// assertFuzzyScores checks each case: a wanted match must score positive and an
// unwanted one must score zero.
func assertFuzzyScores(t *testing.T, tests []fuzzyCase) {
	t.Helper()
	for _, tt := range tests {
		score := fuzzyScore(tt.query, tt.target)
		if tt.wantPos && score <= 0 {
			t.Errorf("fuzzyScore(%q, %q) = %d, want > 0", tt.query, tt.target, score)
		}
		if !tt.wantPos && score != 0 {
			t.Errorf("fuzzyScore(%q, %q) = %d, want 0", tt.query, tt.target, score)
		}
	}
}

func TestFuzzyScore_ASCII(t *testing.T) {
	tests := []fuzzyCase{
		{"", "hello world", false},
		{"hw", "hello world", true},
		{"xyz", "hello world", false},
		{"hello", "hello world", true},
		{"WORLD", "hello world", true},
		{"toolongqueryforthis", "short", false},
	}

	assertFuzzyScores(t, tests)

	// Prefix match should score higher than substring without word boundary
	scorePrefix := fuzzyScore("hel", "hello world")
	scoreMiddle := fuzzyScore("llo", "hello world")
	if scorePrefix <= scoreMiddle {
		t.Errorf("expected prefix match score (%d) > middle score (%d)", scorePrefix, scoreMiddle)
	}
}

func TestFuzzyScore_Unicode(t *testing.T) {
	// Multi-byte UTF-8 matching
	tests := []fuzzyCase{
		{"ü", "München", true},
		{"Ü", "münchen", true},
		{"mün", "München", true},
		{"שלום", "שלום עולם", true},
		{"עולם", "שלום עולם", true},
		{"café", "Café au lait", true},
		{"CAFÉ", "café au lait", true},
		{"тест", "Тестовое сообщение", true},
		{"äöü", "abcdef", false},
	}

	assertFuzzyScores(t, tests)
}

func TestFuzzyFilterEmails(t *testing.T) {
	t1 := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC)

	emails := []email.Email{
		{ID: "id1", Subject: "Announcement from boss", FromRaw: "Boss <boss@corp.com>", EmailDate: t1},
		{ID: "id2", Subject: "Team lunch", FromRaw: "Alice <alice@corp.com>", EmailDate: t2},
		{ID: "id3", Subject: "Urgent: security announcement", FromRaw: "Security <sec@corp.com>", EmailDate: t2},
		{ID: "id4", Subject: "Café meetup", FromRaw: "Bob <bob@corp.com>", EmailDate: t1},
	}

	// Empty query returns original list unmodified
	if res := fuzzyFilterEmails("", emails); len(res) != len(emails) {
		t.Errorf("empty query returned %d emails, want %d", len(res), len(emails))
	}

	// Match query "announcement" -> should match id1 and id3
	ann := fuzzyFilterEmails("announcement", emails)
	if len(ann) != 2 {
		t.Fatalf("expected 2 emails matching 'announcement', got %d", len(ann))
	}
	// Prefix match on "announcement" in id1 should score higher than middle match in id3
	if ann[0].ID != "id1" {
		t.Errorf("expected id1 ranked first for 'announcement', got %s", ann[0].ID)
	}

	// Match unicode query "café"
	cafe := fuzzyFilterEmails("café", emails)
	if len(cafe) != 1 || cafe[0].ID != "id4" {
		t.Fatalf("expected id4 for 'café', got %v", cafe)
	}

	// Match by sender
	boss := fuzzyFilterEmails("boss", emails)
	if len(boss) != 1 || boss[0].ID != "id1" {
		t.Fatalf("expected id1 for sender 'boss', got %v", boss)
	}

	// No match
	none := fuzzyFilterEmails("nonexistentquery", emails)
	if len(none) != 0 {
		t.Errorf("expected 0 matches, got %d", len(none))
	}
}
