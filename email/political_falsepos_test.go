package email

import "testing"

// TestPoliticalFalsePositives covers ordinary mail that must never be labelled
// political. "congress", "election", "campaign", "ballot" and "deadline" are
// everyday vocabulary, especially on a .edu mailbox; with the vocabulary scorer
// removed they no longer contribute, and none of these carries an instant block.
func TestPoliticalFalsePositives(t *testing.T) {
	for _, tc := range []struct{ name, subject, body string }{
		{
			"university committee election",
			"Graduate Student Congress: Committee Election Campaign — Deadline Friday",
			"Please submit your ballot for the committee election before the deadline.",
		},
		{
			"marketing campaign",
			"Our new ad campaign launches Monday",
			"The campaign includes polling data and a deadline of the 15th.",
		},
		{
			"a colleague named Harris",
			"Meeting with Dr. Harris",
			"Prof. Harris will join the senate subcommittee on curriculum. Deadline Friday.",
		},
		{
			"conference proceedings",
			"World Congress on Applied Mathematics",
			"The congress program and the election of new officers are attached.",
		},
		{
			"donation vocabulary without an instant block",
			"Help us flip the Senate",
			"Please donate today to support our campaign. Your contribution to the election matters.",
		},
	} {
		isPolitical, score, triggered := DetectPolitical(tc.subject, tc.body)
		if isPolitical {
			t.Errorf("%s: classified as political (score %.1f, triggered %v)\n  subject: %q",
				tc.name, score, triggered, tc.subject)
		}
	}
}

// TestPoliticalTruePositives pins the detection that must keep working, so the
// false-positive fix above is not achieved by disabling the feature.
func TestPoliticalTruePositives(t *testing.T) {
	for _, tc := range []struct{ name, subject, body string }{
		{
			"actblue donation appeal",
			"We need your help before the deadline",
			"Chip in $5 now: https://actblue.com/donate/campaign2026 — every contribution counts.",
		},
		{
			"pac disclosure",
			"Urgent: final hours",
			"Paid for by the Committee to Elect. Contributions are not tax deductible.",
		},
		{
			"explicit donation plus context",
			"Help us flip the Senate",
			"Give now at winred.com to support our campaign. Your contribution to the election matters.",
		},
	} {
		isPolitical, score, triggered := DetectPolitical(tc.subject, tc.body)
		if !isPolitical {
			t.Errorf("%s: NOT classified as political (score %.1f, triggered %v)\n  subject: %q",
				tc.name, score, triggered, tc.subject)
		}
	}
}

// TestWordBoundaryMatching: substring matching made any word containing a
// keyword a hit.
func TestWordBoundaryMatching(t *testing.T) {
	// "gop" inside "gopher", "dnc" inside "cadence"-like tokens.
	if isPolitical, _, triggered := DetectPolitical("Gopher protocol notes", "The gopher server and its cadence."); isPolitical {
		t.Errorf("substring match leaked: triggered %v", triggered)
	}
	if !containsPhrase("the GOP said", "gop") {
		t.Error("containsPhrase should match a standalone word case-insensitively")
	}
	if containsPhrase("gopher", "gop") {
		t.Error("containsPhrase must not match inside a longer word")
	}
	if !containsPhrase("Paid For By the committee", "paid for by") {
		t.Error("containsPhrase should match multi-word phrases")
	}
	if !containsPhrase("visit https://actblue.com/donate now", "actblue.com") {
		t.Error("containsPhrase should match a domain surrounded by punctuation")
	}
}
