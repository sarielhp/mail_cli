package email

import (
	"strings"
	"testing"
)

// TestFindOriginalMessageEnd_IgnoresInlineGreaterThan is a correctness
// regression: findOriginalMessageEnd treated any line starting with ">" as
// the start of a quoted reply, even when the sender's own original message
// legitimately contains such a line -- a shell command shown in a how-to
// email, a markdown blockquote, or a comparison written at the start of a
// wrapped line. A single "> git log --oneline" line in the middle of the
// body caused SplitMessageAndQuotes to cut the original body short there,
// so everything the sender wrote afterward was hidden as if it were a
// quoted/signature tail.
//
// A genuine client-inserted quote always continues, possibly after blank
// lines, all the way to the end of the message; an inline ">" reference does
// not. That distinction is what the fix checks.
func TestFindOriginalMessageEnd_IgnoresInlineGreaterThan(t *testing.T) {
	body := "Please run this command:\n> git log --oneline\n\nLet me know if it works."
	idx := findOriginalMessageEnd(body)
	lines := strings.Split(body, "\n")
	if idx != len(lines) {
		t.Errorf("expected no quote boundary (whole body is original), got line %d: %q", idx, lines[idx])
	}

	orig, quotes := SplitMessageAndQuotes(body)
	if orig != body {
		t.Errorf("expected the entire body to be treated as original, got orig=%q quotes=%q", orig, quotes)
	}
	if quotes != "" {
		t.Errorf("expected no quoted tail, got %q", quotes)
	}
}

// quoteSplitExpectation is the expected outcome of splitting a body at its
// quoted-reply boundary.
type quoteSplitExpectation struct {
	boundary    int
	origMarker  string
	quoteMarker string
}

// assertQuoteSplit checks that findOriginalMessageEnd and SplitMessageAndQuotes
// agree on where body's quoted reply begins, and that each half still carries
// its marker text.
func assertQuoteSplit(t *testing.T, body string, want quoteSplitExpectation) {
	t.Helper()
	if idx := findOriginalMessageEnd(body); idx != want.boundary {
		t.Errorf("findOriginalMessageEnd(%q) = %d, want %d", body, idx, want.boundary)
	}

	orig, quotes := SplitMessageAndQuotes(body)
	if !strings.Contains(orig, want.origMarker) {
		t.Errorf("original body missing %q, got %q", want.origMarker, orig)
	}
	if !strings.Contains(quotes, want.quoteMarker) {
		t.Errorf("quoted tail missing %q, got %q", want.quoteMarker, quotes)
	}
}

// TestFindOriginalMessageEnd_StillSplitsRealQuoteBlock guards against an
// overcorrection: a genuine quoted reply, where every remaining non-blank
// line is quote-prefixed through to the end of the message, must still be
// recognized and split off.
func TestFindOriginalMessageEnd_StillSplitsRealQuoteBlock(t *testing.T) {
	body := "Sounds good, thanks!\n\n> On Mon, Jan 1, 2024 Alice wrote:\n> Are you free to meet?"
	assertQuoteSplit(t, body, quoteSplitExpectation{
		boundary:    2,
		origMarker:  "Sounds good, thanks!",
		quoteMarker: "Are you free to meet?",
	})
}

func TestReflowParagraphs_KeepsReferenceLines(t *testing.T) {
	in := "References:\n\n[1] https://a.example/x\n[2] https://b.example/y\n[10] https://c.example/z"
	if got := ReflowParagraphs(in); got != in {
		t.Errorf("reference lines were joined:\n%s", got)
	}
}
