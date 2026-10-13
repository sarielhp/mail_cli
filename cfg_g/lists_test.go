package cfg_g

import "testing"

// TestDomainPatternsMatch guards against the regression where the CLI validated
// and stored "@domain.com" entries that the matcher could never match, so a
// whitelisted domain got no protection and a blacklisted domain was never
// blocked.
func TestDomainPatternsMatch(t *testing.T) {
	for _, tc := range []struct {
		name   string
		sender string
		list   []string
		want   bool
	}{
		{"domain entry matches sender in that domain", "someone@example.edu", []string{"@example.edu"}, true},
		{"domain entry is case-insensitive", "SomeOne@Example.EDU", []string{"@example.edu"}, true},
		{"domain entry does not match another domain", "someone@example.com", []string{"@example.edu"}, false},
		{"domain entry does not match a suffix", "someone@notexample.edu", []string{"@example.edu"}, false},
		{"exact address still matches", "a@b.c", []string{"a@b.c"}, true},
		{"exact address does not match a different one", "a@b.c", []string{"d@b.c"}, false},
		{"mixed list", "x@spam.example", []string{"keep@me.com", "@spam.example"}, true},
		{"empty entries are skipped", "a@b.c", []string{"", "  "}, false},
		{"sender without a domain does not match a domain entry", "malformed", []string{"@b.c"}, false},
	} {
		if got := listContains(tc.sender, tc.list); got != tc.want {
			t.Errorf("%s: listContains(%q, %v) = %v, want %v", tc.name, tc.sender, tc.list, got, tc.want)
		}
	}
}

func TestIsWhitelistedAndBlacklistedUseDomains(t *testing.T) {
	if !IsWhitelisted("prof@example.edu", []string{"@example.edu"}) {
		t.Error("IsWhitelisted should honor a domain pattern")
	}
	if !IsBlacklisted("bot@spam.example", []string{"@spam.example"}) {
		t.Error("IsBlacklisted should honor a domain pattern")
	}
}
