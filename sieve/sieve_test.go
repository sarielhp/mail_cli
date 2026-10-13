package sieve

import (
	"github.com/sarielhp/mail_cli/cfg_acc"
	"strings"
	"testing"
)

func TestSanitizeForSieve(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"no escaping needed", "test@example.com", "test@example.com"},
		{"backslash escaped", `test\"quote`, `test\\\"quote`},
		{"double quote escaped", `test"quote`, `test\"quote`},
		{"both escaped", `test\"both`, `test\\\"both`},
		{"empty string", "", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitizeForSieve(tc.input)
			if got != tc.want {
				t.Fatalf("sanitizeForSieve(%q) = %q, want %q", tc.input, got, tc.want)
			}
			// A quoted string in a Sieve script ends at the line it opened
			// on, so an escaped value must carry no raw line break.
			if strings.ContainsAny(got, "\r\n") {
				t.Errorf("sanitizeForSieve(%q) left a line break: %q", tc.input, got)
			}
		})
	}
}

func TestGenerateSieveScript(t *testing.T) {
	rules := []cfg_acc.Rule{
		{Sender: "spam@example.com", Label: "Spam"},
		{Sender: "news@example.com", Label: "News"},
		{Subject: "Alert", Label: "Alerts"},
	}
	script := generateSieveScript(rules)

	if !strings.Contains(script, "require [\"fileinto\"];") {
		t.Error("generateSieveScript should include fileinto require")
	}
	if !strings.Contains(script, "spam@example.com") {
		t.Error("generateSieveScript should include sender filter")
	}
	if !strings.Contains(script, "News") {
		t.Error("generateSieveScript should include News label")
	}
	if !strings.Contains(script, "header :matches \"Subject\" \"Alert*\"") {
		t.Error("generateSieveScript should include subject matches filter")
	}
	if !strings.Contains(script, "stop") {
		t.Error("generateSieveScript should include stop directive")
	}
}

func TestGenerateSieveScriptEmpty(t *testing.T) {
	script := generateSieveScript([]cfg_acc.Rule{})
	if !strings.Contains(script, "require") {
		t.Error("generateSieveScript should always include require even with empty rules")
	}
}

// A Sieve quoted string cannot contain a raw newline, and a comment ends at
// one. A rule value carrying either used to end the literal mid-line and leave
// the rest as script text -- in a file the user is told to upload to their mail
// server. Rule values are not hand-typed only: they are derived from message
// senders and subjects, and a JSON string in config.json may hold a newline.
func TestGeneratedScriptCannotBeBrokenOutOf(t *testing.T) {
	hostile := []cfg_acc.Rule{
		{Sender: "a@b.c\n}\ndiscard;\nif true {", Label: "Inbox"},
		{Sender: "ok@b.c", Label: "Good\"\n}\ndiscard;\nif true {"},
		{Subject: "hi\n}\ndiscard;\nif true {", Label: "Inbox"},
		{Sender: "tab\there@b.c", Label: "In\rbox"},
	}

	script := generateSieveScript(hostile)

	for i, line := range strings.Split(script, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "", strings.HasPrefix(trimmed, "#"):
			continue
		}
		if trimmed == "discard;" {
			t.Errorf("line %d injected a bare `discard;` into the script:\n%s", i+1, script)
		}
	}

	// Every quoted string must close on the line it opens on, which is the
	// property a raw newline destroys. Comments are exempt: a quote inside one
	// is just text, and the newline that would have ended the comment early is
	// already gone.
	for i, line := range strings.Split(script, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		if unescaped := strings.Count(line, `"`) - strings.Count(line, `\"`); unescaped%2 != 0 {
			t.Errorf("line %d leaves a quoted string open: %q", i+1, line)
		}
	}
}

func TestSanitizeForSieveEscapesInOrder(t *testing.T) {
	// Backslash first, then quote: the other order double-escapes.
	if got := sanitizeForSieve(`a\"b`); got != `a\\\"b` {
		t.Errorf("sanitizeForSieve(%q) = %q", `a\"b`, got)
	}
	if got := sanitizeForSieve("a\nb"); got != "a b" {
		t.Errorf("a newline must not survive into a quoted string, got %q", got)
	}
}
