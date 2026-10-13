package outlook

import (
	"net/url"
	"strings"
	"testing"
)

// A Graph query string with an unescaped space is answered with a bare 400.
// That is how the account-wide query was broken, so the search URL is checked
// rather than assumed.
func TestBuildGraphSearchURLEscapesEverything(t *testing.T) {
	tests := []struct {
		name  string
		query string
	}{
		{name: "spaces", query: "quarterly report"},
		{name: "ampersand", query: "profit & loss"},
		{name: "quotes", query: `say "hello"`},
		{name: "plus and equals", query: "a+b=c"},
		{name: "unicode", query: "naïve café"},
		{name: "question mark and hash", query: "what? #tag"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			raw := buildGraphSearchURL(tc.query, 25)

			parsed, err := url.Parse(raw)
			if err != nil {
				t.Fatalf("URL does not parse: %v", err)
			}
			// A literal space anywhere in the query string is the failure mode.
			if strings.Contains(parsed.RawQuery, " ") {
				t.Errorf("RawQuery contains an unescaped space: %q", parsed.RawQuery)
			}

			values, err := url.ParseQuery(parsed.RawQuery)
			if err != nil {
				t.Fatalf("query string does not parse: %v", err)
			}
			search := values.Get("$search")
			if search == "" {
				t.Fatal("$search is missing")
			}
			// Graph requires the KQL string quoted.
			if !strings.HasPrefix(search, `"`) || !strings.HasSuffix(search, `"`) {
				t.Errorf("$search = %s, want it wrapped in quotes", search)
			}
			if !strings.Contains(search, strings.Split(tc.query, " ")[0]) {
				t.Errorf("$search = %s, does not contain the query", search)
			}
			if values.Get("$top") != "25" {
				t.Errorf("$top = %q, want 25", values.Get("$top"))
			}
		})
	}
}

// $search cannot be combined with $orderby; Graph rejects the pair.
func TestGraphSearchURLOmitsOrderBy(t *testing.T) {
	raw := buildGraphSearchURL("anything", 10)
	if strings.Contains(raw, "$orderby") {
		t.Errorf("search URL includes $orderby, which Graph refuses alongside $search: %s", raw)
	}
}

// $search carries a $top that bounds one Graph response, so ask for only what
// is left to find rather than a full page every time.
func TestSearchTop(t *testing.T) {
	tests := []struct {
		name  string
		limit int
		have  int
		want  int
	}{
		{name: "unbounded asks for a full page", limit: 0, have: 0, want: graphSearchPageSize},
		{name: "negative is unbounded", limit: -1, have: 0, want: graphSearchPageSize},
		{name: "small limit asks for only what is wanted", limit: 5, have: 0, want: 5},
		{name: "partial progress", limit: 120, have: 100, want: 20},
		{name: "large limit is capped at a page", limit: 500, have: 0, want: graphSearchPageSize},
	}
	for i := range tests {
		tc := tests[i]
		t.Run(tc.name, func(t *testing.T) {
			if got, want := searchTop(tc.limit, tc.have), tc.want; got != want {
				t.Errorf("searchTop(limit=%d, have=%d) = %d, want %d", tc.limit, tc.have, got, want)
			}
		})
	}
}
