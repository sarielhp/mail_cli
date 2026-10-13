package email

import (
	"strings"
	"testing"
)

func TestHeaderIDSameAndFolded(t *testing.T) {
	a, b := HeaderID("<Abc@Example.com>"), HeaderID("<abc@example.com>")
	if !a.Same(b) || a.Folded() != b.Folded() {
		t.Errorf("%q and %q should match ignoring case", a, b)
	}
	if a.Same("<other@example.com>") {
		t.Error("different identifiers matched")
	}
}

func TestExtractMessageIDsReturnsHeaderIDs(t *testing.T) {
	got := extractMessageIDs("<a@x> junk <b@y>")
	if len(got) != 2 || got[0] != "<a@x>" || got[1] != "<b@y>" {
		t.Errorf("extractMessageIDs = %q", got)
	}
}

func TestParseShortIDAndIDMatches(t *testing.T) {
	if s, ok := ParseShortID(" 1234abcd "); !ok || s != "1234ABCD" {
		t.Errorf("ParseShortID = %q, %v", s, ok)
	}
	for _, bad := range []string{"1234abc", "1234abcde", "1234abcz", "19f3a8216d52dfc6"} {
		if _, ok := ParseShortID(bad); ok {
			t.Errorf("ParseShortID(%q) accepted", bad)
		}
	}
	id := ID("19f3a8216d52dfc6")
	if !IDMatches(id, "19F3A8216D52DFC6") || !IDMatches(id, strings.ToLower(string(ComputeShortID(id)))) {
		t.Error("IDMatches must accept the full ID and the short ID in any case")
	}
	if IDMatches(id, "19f3a821") {
		t.Error("a prefix is not a short ID")
	}
}

func TestSubjectFileStem(t *testing.T) {
	for in, want := range map[string]string{
		"Re: Fwd: [list] Hello - world  (draft)": "Hello_world_draft",
		"RE[2]: AW: v1.2 release":                "v1.2_release",
		"":                                       "no_subject",
		"---":                                    "no_subject",
	} {
		if got := SubjectFileStem(in); got != want {
			t.Errorf("SubjectFileStem(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestShortStem(t *testing.T) {
	stem := "Reminder_quarterly_budget_review_meeting"
	if got := ShortStem(stem, 30, nil); got != "Reminder_quarterly_budget_revi" {
		t.Errorf("unique cut = %q", got)
	}
	// The cut grows past a name already in the directory, to the shortest
	// prefix nothing there starts with.
	if got := ShortStem(stem, 30, []string{"reminder_quarterly_budget_revi.eml"}); got != "Reminder_quarterly_budget_revie" {
		t.Errorf("extended cut = %q", got)
	}
	if got := ShortStem("short", 30, []string{"short.eml"}); got != "short" {
		t.Errorf("a short stem is left alone, got %q", got)
	}
}
