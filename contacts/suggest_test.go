package contacts

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestSuggest_RankingAndAliases(t *testing.T) {
	book := &Book{
		Contacts: []Contact{
			{
				ID:        "mutt:jq",
				Name:      "John Quick",
				Emails:    []string{"jq@example.com"},
				Nicknames: []string{"jq"},
				Source:    "mutt",
			},
			{
				ID:        "google:work:c1",
				Name:      "John Normal",
				Emails:    []string{"john.normal@example.com"},
				Nicknames: []string{"john"},
				Source:    "google:work",
			},
		},
	}

	now := time.Now().UTC()
	history := &HistoryBook{
		Entries: []ScoredContact{
			{
				Contact: Contact{
					Name:   "Frequent Collaborator",
					Emails: []string{"collab@example.com"},
					Source: "harvest",
				},
				Frequency: 25,
				LastSeen:  now.Add(-2 * 24 * time.Hour),
				IsSent:    true,
			},
			{
				Contact: Contact{
					Name:   "Blocked Spammer",
					Emails: []string{"spammer@badsite.com"},
					Source: "harvest",
				},
				Frequency: 50,
			},
		},
	}

	blacklist := []string{"spammer@badsite.com"}

	// 1. Exact alias match "jq"
	jqResults := Suggest(book, history, blacklist, "jq", 10)
	if len(jqResults) == 0 {
		t.Fatalf("expected results for 'jq'")
	}
	if jqResults[0].PrimaryEmail() != "jq@example.com" {
		t.Errorf("expected jq@example.com first, got %s", jqResults[0].PrimaryEmail())
	}
	if jqResults[0].Score < 1500 {
		t.Errorf("expected alias score >= 1500, got %.1f", jqResults[0].Score)
	}

	// 2. Blacklisted address should never appear
	spamResults := Suggest(book, history, blacklist, "spammer", 10)
	if len(spamResults) != 0 {
		t.Errorf("expected 0 results for blacklisted spammer, got %v", spamResults)
	}

	// 3. History matching: "collab"
	collabResults := Suggest(book, history, blacklist, "collab", 10)
	if len(collabResults) != 1 || collabResults[0].PrimaryEmail() != "collab@example.com" {
		t.Fatalf("expected collab@example.com, got %v", collabResults)
	}
	if collabResults[0].Score <= 0 {
		t.Errorf("expected positive score for history candidate, got %.1f", collabResults[0].Score)
	}
}

func TestFormatSuggestTable(t *testing.T) {
	suggestions := []ScoredContact{
		{
			Contact: Contact{
				Name:      "Dave Rivers",
				Emails:    []string{"rivers@cs.example.edu"},
				Nicknames: []string{"dave"},
				Groups:    []string{"algo"},
				Source:    "mutt",
			},
			Score: 1550,
		},
	}

	var buf bytes.Buffer
	if err := FormatSuggestTable(&buf, suggestions); err != nil {
		t.Fatalf("FormatSuggestTable failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Dave Rivers") || !strings.Contains(out, "rivers@cs.example.edu") {
		t.Errorf("expected Dave Rivers in output, got:\n%s", out)
	}
	if !strings.Contains(out, "1550") {
		t.Errorf("expected score 1550 in output, got:\n%s", out)
	}
}
