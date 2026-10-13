package gmail

import "testing"

// A search for five matches should not pull a hundred.
func TestSearchPageSize(t *testing.T) {
	tests := []struct {
		name  string
		limit int
		have  int
		want  int
		why   string
	}{
		{name: "unbounded asks for a full page", limit: 0, have: 0, want: gmailSearchPageSize, why: "limit 0 is no limit, so the page cap applies"},
		{name: "negative is unbounded", limit: -5, have: 0, want: gmailSearchPageSize, why: "any limit <= 0 is no limit"},
		{name: "small limit asks for only what is wanted", limit: 5, have: 0, want: 5, why: "the wanted count is below the page cap"},
		{name: "partial progress", limit: 150, have: 100, want: 50, why: "only what is still missing is requested"},
		{name: "large limit is capped at a page", limit: 1000, have: 0, want: gmailSearchPageSize, why: "Gmail returns at most one page per call"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := searchPageSize(tc.limit, tc.have); got != tc.want {
				t.Errorf("searchPageSize(%d, %d) = %d, want %d: %s", tc.limit, tc.have, got, tc.want, tc.why)
			}
		})
	}
}
