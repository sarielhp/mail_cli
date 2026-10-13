package jmap

import (
	"testing"
)

// JMAP has no query language, so a Gmail-style string has to be taken apart
// and mapped onto Email/query's structured filter fields.
func TestBuildJMAPFilter(t *testing.T) {
	tests := []struct {
		name        string
		query       string
		wantText    string
		wantFrom    string
		wantTo      string
		wantSubject string
		wantAttach  bool
	}{
		{
			name:     "plain text",
			query:    "quarterly report",
			wantText: "quarterly report",
		},
		{
			name:     "from qualifier only",
			query:    "from:alice@example.com",
			wantFrom: "alice@example.com",
		},
		{
			name:     "qualifier plus free text",
			query:    "from:alice invoice",
			wantFrom: "alice",
			wantText: "invoice",
		},
		{
			name:        "several qualifiers",
			query:       "from:alice to:bob subject:budget",
			wantFrom:    "alice",
			wantTo:      "bob",
			wantSubject: "budget",
		},
		{
			name:        "quoted value keeps its spaces",
			query:       `subject:"quarterly report"`,
			wantSubject: "quarterly report",
		},
		{
			name:       "has attachment",
			query:      "has:attachment taxes",
			wantAttach: true,
			wantText:   "taxes",
		},
		{
			name:     "unknown qualifier falls back to text",
			query:    "label:receipts",
			wantText: "label:receipts",
		},
		{
			name:     "has with an unknown value falls back to text",
			query:    "has:wings",
			wantText: "has:wings",
		},
		{
			name:     "bare colon is not a qualifier",
			query:    "ratio:",
			wantText: "ratio:",
		},
		{
			name:  "empty query",
			query: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := buildJMAPFilter(tc.query)
			if got.Text != tc.wantText {
				t.Errorf("Text = %q, want %q", got.Text, tc.wantText)
			}
			if got.From != tc.wantFrom {
				t.Errorf("From = %q, want %q", got.From, tc.wantFrom)
			}
			if got.To != tc.wantTo {
				t.Errorf("To = %q, want %q", got.To, tc.wantTo)
			}
			if got.Subject != tc.wantSubject {
				t.Errorf("Subject = %q, want %q", got.Subject, tc.wantSubject)
			}
			if got.HasAttachment != tc.wantAttach {
				t.Errorf("HasAttachment = %v, want %v", got.HasAttachment, tc.wantAttach)
			}
		})
	}
}

func TestSplitQueryTokens(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  []string
	}{
		{name: "plain words", query: "a b c", want: []string{"a", "b", "c"}},
		{name: "quoted run", query: `a "b c" d`, want: []string{"a", "b c", "d"}},
		{name: "quoted qualifier", query: `subject:"b c"`, want: []string{"subject:b c"}},
		{name: "repeated spaces", query: "a   b", want: []string{"a", "b"}},
		{name: "empty", query: "", want: nil},
		{name: "unterminated quote", query: `a "b c`, want: []string{"a", "b c"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := splitQueryTokens(tc.query)
			if len(got) != len(tc.want) {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("token %d = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}
