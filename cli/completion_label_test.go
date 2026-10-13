package cli

import (
	"strings"
	"testing"

	"github.com/sarielhp/clihelp"
)

// A shell replaces the whole word being completed, so a candidate without the
// sigil would eat the % the user already typed.
func TestCompleteFolderNamesKeepsTheSigil(t *testing.T) {
	tests := []struct {
		toComplete string
		wantPrefix string
	}{
		{"%IN", "%INBOX"},
		{"%in", "%INBOX"},
		{"%work:IN", "%work:INBOX"},
		{"IN", "INBOX"}, // the bare form still completes, during the window
	}

	for _, tt := range tests {
		t.Run(tt.toComplete, func(t *testing.T) {
			got := CompleteFolderNames(tt.toComplete)
			if len(got) == 0 {
				t.Fatalf("no candidates for %q", tt.toComplete)
			}
			first := strings.SplitN(got[0], "\t", 2)[0]
			if first != tt.wantPrefix {
				t.Errorf("CompleteFolderNames(%q)[0] = %q, want %q", tt.toComplete, first, tt.wantPrefix)
			}
		})
	}
}

// With nothing typed there is no spelling to preserve, so completion offers the
// form the program is moving to. Offering bare names there would have taught the
// deprecated spelling and attached a migration notice to every candidate it
// suggested -- found by running the binary, not by a test.
func TestCompleteLabelRefsOffersTheSigilWhenNothingIsTyped(t *testing.T) {
	got := CompleteLabelRefs("")
	if len(got) == 0 {
		t.Fatal("no candidates for an empty word")
	}
	for _, c := range got {
		cand := strings.SplitN(c, "\t", 2)[0]
		if !strings.HasPrefix(cand, "%") {
			t.Errorf("CompleteLabelRefs(\"\") offered %q, want the %% sigil", cand)
		}
	}
}

// A typed sigil is kept, with any account prefix, and never doubled. A bare
// word gets no labels, only a hint naming the sigil: that spelling is
// deprecated, and in a position that also takes a subcommand the folders
// buried it ("mt w" listed every w-folder beside "write").
func TestCompleteLabelRefsCompletesOnlyTheSigilForm(t *testing.T) {
	for _, tc := range []struct{ in, wantPrefix string }{
		{"%IN", "%INBOX"},
		{"%in", "%INBOX"},
		{"%work:IN", "%work:INBOX"},
		{"IN", ""},
		{"work:IN", ""},
	} {
		got := CompleteLabelRefs(tc.in)
		if tc.wantPrefix == "" {
			want := clihelp.CompletionHint("folder names start with %, as in %" + tc.in)
			if len(got) != 1 || got[0] != want {
				t.Errorf("CompleteLabelRefs(%q) = %q, want only the hint %q", tc.in, got, want)
			}
			continue
		}
		if len(got) == 0 {
			t.Fatalf("no candidates for %q", tc.in)
		}
		if first := strings.SplitN(got[0], "\t", 2)[0]; first != tc.wantPrefix {
			t.Errorf("CompleteLabelRefs(%q)[0] = %q, want %q", tc.in, first, tc.wantPrefix)
		}
	}
}

func TestSplitLabelToComplete(t *testing.T) {
	tests := []struct {
		in        string
		wantKeep  string
		wantLabel string
		wantFound bool
	}{
		{"%rec", "%", "rec", true},
		{"%work:rec", "%work:", "rec", true},
		{"%", "%", "", true},
		{"rec", "", "rec", false},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			keep, label, found := splitLabelToComplete(tt.in)
			if keep != tt.wantKeep || label != tt.wantLabel || found != tt.wantFound {
				t.Errorf("splitLabelToComplete(%q) = (%q, %q, %v), want (%q, %q, %v)",
					tt.in, keep, label, found, tt.wantKeep, tt.wantLabel, tt.wantFound)
			}
		})
	}
}
