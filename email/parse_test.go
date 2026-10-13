package email

import (
	"strings"
	"testing"
)

// TestSubjectMatching drives the one matcher every command now shares:
// `scan`, `splice` and `split`. The last of these used to compile its own
// always-anchored glob, so the cases marked below changed meaning when it
// was routed here -- that is the behaviour change, pinned.
func TestSubjectMatching(t *testing.T) {
	tests := []struct {
		subject string
		pattern string
		want    bool
	}{
		// Bracketed prefix patterns (Defect 4 regression)
		{"[JIRA] Task #101", "[JIRA]*", true},
		{"[jira] Task #101", "[JIRA]*", true},
		{"Java update", "[JIRA]*", false},
		{"Task [JIRA] #101", "[JIRA]*", false},
		{"Re: [PATCH] Fix bug", "*[PATCH]*", true},
		{"Unrelated email", "*[PATCH]*", false},
		{"[GitHub] PR #42", "[GitHub]*", true},
		{"Git hub update", "[GitHub]*", false},

		// Standard wildcard matching
		{"Urgent meeting today", "*urgent*", true},
		{"urgent!", "urgent?", true},
		{"urgent12", "urgent?", false},
		{"Quarterly Report", "*report", true},
		{"Report quarterly", "*report", false},

		// Plain substring fallback (no wildcards)
		{"Important announcement", "announcement", true},
		{"Unrelated subject", "announcement", false},

		// Escaped metacharacters
		{"file.txt attached", "file.*", true},
		{"filextxt attached", "file.*", false},
		{"price is $100", "*$100*", true},
		{"call (555) now", "*(555)*", true},

		// Absorbed from cli.matchPattern, which `split` used until the two
		// matchers were unified.
		{"Invoice #1234", "*invoice*", true},
		{"Billing department", "*invoice*", false},
		{"RE: urgent meeting request", "*urgent*", true},
		{"Exactly-this", "exactly-this", true},
		{"Subject with / slash", "*/*", true},
		// CHANGED by the unification: a pattern with no wildcard is a substring
		// test, not an equality test. Under `split`'s old anchored glob this was
		// false, so `split src invoice dst --do` silently moved nothing while
		// `splice --pattern invoice` on the same mailbox moved plenty.
		{"not exactly-this", "exactly-this", true},
	}

	for _, tt := range tests {
		matches, err := CompilePattern(tt.pattern)
		if err != nil {
			t.Errorf("CompilePattern(%q): %v", tt.pattern, err)
			continue
		}
		if got := matches(tt.subject); got != tt.want {
			t.Errorf("CompilePattern(%q) on %q = %v, want %v", tt.pattern, tt.subject, got, tt.want)
		}
	}
}

func TestCompilePattern(t *testing.T) {
	matcher, err := CompilePattern("invoice-????-*")
	if err != nil {
		t.Fatalf("CompilePattern failed: %v", err)
	}

	tests := []struct {
		subject string
		want    bool
	}{
		{"INVOICE-2026-FINAL", true},
		{"invoice-abcd-xyz", true},
		{"invoice-123-x", false},
		{"other subject", false},
	}

	for _, tt := range tests {
		if got := matcher(tt.subject); got != tt.want {
			t.Errorf("matcher(%q) = %v, want %v", tt.subject, got, tt.want)
		}
	}

	// Non-wildcard fast path
	plainMatcher, err := CompilePattern("payment")
	if err != nil {
		t.Fatalf("CompilePattern for plain substring failed: %v", err)
	}
	if !plainMatcher("Payment received for order 42") {
		t.Error("expected plainMatcher to match case-insensitively")
	}
	if plainMatcher("Receipt for order 42") {
		t.Error("expected plainMatcher not to match unrelated subject")
	}
}

func TestCompilePattern_LiteralCaseInsensitiveNoAlloc(t *testing.T) {
	matcher, err := CompilePattern("invoice")
	if err != nil {
		t.Fatalf("CompilePattern failed: %v", err)
	}

	tests := []struct {
		subject string
		want    bool
	}{
		{"URGENT INVOICE #123", true},
		{"Monthly Invoice Due", true},
		{"invOICE details", true},
		{"Receipt for purchase", false},
	}

	for _, tt := range tests {
		if got := matcher(tt.subject); got != tt.want {
			t.Errorf("matcher(%q) = %v, want %v", tt.subject, got, tt.want)
		}
	}

	allocs := testing.AllocsPerRun(100, func() {
		_ = matcher("URGENT INVOICE FOR PAYMENT")
	})
	if allocs > 0 {
		t.Errorf("expected 0 allocations per match run, got %f", allocs)
	}
}

// TestParseReader_StripsEscapeSequencesFromSubjectAndSender is a
// security-review finding: Subject and From/FromRaw are shown verbatim in
// the TUI's message list, detail headers, and the CLI's `show` command,
// with no escape-sequence stripping anywhere between parsing and the
// terminal. A malicious email could embed ANSI/OSC escape sequences in its
// Subject or sender name to manipulate the terminal display (hide text,
// move the cursor, alter the window title, or worse on terminals that
// support dangerous OSC codes). ParseReader is the single place every
// display path gets Email.Subject/FromRaw/FromEmail from, so stripping
// there closes it for all of them at once.
func TestParseReader_StripsEscapeSequencesFromSubjectAndSender(t *testing.T) {
	raw := "From: Attacker \x1b]0;pwned\x07<attacker@example.com>\r\n" +
		"Subject: Hello\x1b[2Jworld\r\n" +
		"Date: Mon, 02 Jan 2026 15:04:05 +0000\r\n" +
		"\r\n" +
		"body\r\n"

	got := ParseReader(strings.NewReader(raw), "id1", "")
	if got == nil {
		t.Fatal("ParseReader returned nil")
	}
	if strings.ContainsAny(got.Subject, "\x1b\x07") {
		t.Errorf("Subject still contains an escape byte: %q", got.Subject)
	}
	if strings.ContainsAny(got.FromRaw, "\x1b\x07") {
		t.Errorf("FromRaw still contains an escape byte: %q", got.FromRaw)
	}
	if strings.ContainsAny(got.FromEmail, "\x1b\x07") {
		t.Errorf("FromEmail still contains an escape byte: %q", got.FromEmail)
	}
}
