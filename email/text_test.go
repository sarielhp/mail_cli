package email

import (
	"mime"
	"strings"
	"testing"
)

func TestIsASCIIDivider(t *testing.T) {
	dividers := []string{
		"____________________________________________________",
		"----------------------------------------------------",
		"====================================================",
		"~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~",
		"-::~:~::~:~:~:~:~:~:~:~:~:~:~:~:~:~:~:~:~:~:~::~:~::-",
		"~~//~~",
	}

	for _, d := range dividers {
		if !IsASCIIDivider(d) {
			t.Errorf("expected IsASCIIDivider(%q) to be true", d)
		}
	}

	nonDividers := []string{
		"Hello world",
		"_italic text_",
		"--flag",
		"a = b + c",
	}

	for _, nd := range nonDividers {
		if IsASCIIDivider(nd) {
			t.Errorf("expected IsASCIIDivider(%q) to be false", nd)
		}
	}
}

func TestCleanMeetingText(t *testing.T) {
	input := "Join with Google Meet\nhttps://meet.google.com/abc-defg-hij"
	output := CleanMeetingText(input)
	if strings.Contains(output, "Join with Google Meet\n") {
		t.Errorf("expected newline to be removed after Join with Google Meet, got: %q", output)
	}
	if !strings.Contains(output, "Join with Google Meet: https://meet.google.com/abc-defg-hij") {
		t.Errorf("expected single line join URL, got: %q", output)
	}

	input2 := "Join:\nhttps://zoom.us/j/123456789"
	output2 := CleanMeetingText(input2)
	if strings.Contains(output2, "Join:\n") {
		t.Errorf("expected newline to be removed after Join:, got: %q", output2)
	}
	if !strings.Contains(output2, "Join: https://zoom.us/j/123456789") {
		t.Errorf("expected single line join URL, got: %q", output2)
	}
}

func TestWrapText_PreservesQuotePrefix(t *testing.T) {
	longQuote := "> This is a very long line of quoted reply text that must be reflowed and wrapped across multiple lines while preserving the quote prefix."
	wrapped := WrapText(longQuote, 30)

	lines := strings.Split(wrapped, "\n")
	if len(lines) < 2 {
		t.Fatalf("expected multiple wrapped lines, got %d lines: %v", len(lines), lines)
	}

	for i, l := range lines {
		if !strings.HasPrefix(l, ">") {
			t.Errorf("line %d does not preserve quote prefix: %q", i, l)
		}
	}
}

func TestReflowParagraphs(t *testing.T) {
	input := "This is line one of a short paragraph.\nThis is line two of the same paragraph starting in column 0.\n\nHere is a second paragraph.\nIt continues on the next line.\n\n* Bullet 1\n* Bullet 2\n\n    code block line 1\n    code block line 2"
	reflowed := ReflowParagraphs(input)

	if !strings.Contains(reflowed, "This is line one of a short paragraph. This is line two of the same paragraph") {
		t.Errorf("expected first paragraph lines to be connected, got:\n%s", reflowed)
	}
	if !strings.Contains(reflowed, "Here is a second paragraph. It continues on the next line.") {
		t.Errorf("expected second paragraph lines to be connected, got:\n%s", reflowed)
	}
	if !strings.Contains(reflowed, "* Bullet 1\n* Bullet 2") {
		t.Errorf("expected bullet items to remain separate, got:\n%s", reflowed)
	}
	if !strings.Contains(reflowed, "    code block line 1\n    code block line 2") {
		t.Errorf("expected indented code block to remain separate, got:\n%s", reflowed)
	}
}

func TestFindOriginalMessageEnd(t *testing.T) {
	msg := "Hello,\nThis is the body.\n\nOn Mon, Jan 1, 2024 at 10:00 AM Alice wrote:\n> Previous text"
	assertQuoteSplit(t, msg, quoteSplitExpectation{
		boundary:    3,
		origMarker:  "This is the body.",
		quoteMarker: "On Mon",
	})
}

func TestTruncateLines_IncrementalWidth(t *testing.T) {
	input := "the quick brown fox"
	res := truncateLines(input, 15)
	expected := "the quick brown\nfox"
	if res != expected {
		t.Errorf("expected %q, got %q", expected, res)
	}

	singleLine := "short text"
	if got := truncateLines(singleLine, 20); got != singleLine {
		t.Errorf("expected %q, got %q", singleLine, got)
	}

	// Long input to verify performance and consistency
	longInput := strings.Repeat("word ", 200)
	resLong := truncateLines(longInput, 30)
	if len(resLong) == 0 {
		t.Errorf("expected non-empty output for long input")
	}
}

func TestWrapQuotedLine_IncrementalWidth(t *testing.T) {
	// Standard wrapping with quote prefix
	line := "> the quick brown fox jumps over the lazy dog"
	lines := WrapQuotedLine(line, "> ", 20)
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d: %v", len(lines), lines)
	}
	for _, l := range lines {
		if !strings.HasPrefix(l, "> ") {
			t.Errorf("expected quote prefix '> ', got: %q", l)
		}
	}

	// Empty body part
	empty := WrapQuotedLine("> ", "> ", 20)
	if len(empty) != 0 {
		t.Errorf("expected empty result for empty body, got: %v", empty)
	}

	// A single word wider than the pane is broken, not left for the
	// viewport to crop.
	longWord := "> supercalifragilisticexpialidocious"
	resLong := WrapQuotedLine(longWord, "> ", 15)
	if strings.Join(resLong, "|") != "> supercalifrag|> ilisticexpial|> idocious" {
		t.Errorf("unexpected wrap for single long word: %v", resLong)
	}

	// Wide characters (CJK) with width 2
	cjk := "> 日本語 テスト 文章 です"
	cjkLines := WrapQuotedLine(cjk, "> ", 12)
	if len(cjkLines) < 2 {
		t.Errorf("expected CJK text to wrap into multiple lines, got %d: %v", len(cjkLines), cjkLines)
	}
	for _, l := range cjkLines {
		if !strings.HasPrefix(l, "> ") {
			t.Errorf("expected quote prefix in CJK wrap, got: %q", l)
		}
	}
}

func TestWrapText_OSC8Hyperlinks(t *testing.T) {
	link := FormatOSC8Link("https://example.com/very/long/destination/path/to/resource", "Short Link")
	line := "Check out " + link + " now!"

	wrapped := WrapText(line, 40)
	if strings.Contains(wrapped, "\n") {
		t.Errorf("expected single line wrap for OSC 8 link whose visible width fits in 40 columns, got:\n%q", wrapped)
	}

	if !strings.Contains(wrapped, "\x1b]8;;https://example.com/very/long/destination/path/to/resource\x1b\\Short Link\x1b]8;;\x1b\\") {
		t.Errorf("expected OSC 8 escape sequences to be preserved intact, got:\n%q", wrapped)
	}
}

// TestDecDecode_StripsEscapeSequences is a security-review finding:
// DecDecode is a second, independent RFC2047 header-decode path used
// directly by tui_model.go's detail-view header, tui_bg_summary.go's
// summary overlay, tui_compose.go's reply subject, and email/reply.go's
// reply composition -- all bypassing email.ParseReader (and the
// StripEscapeSequences it applies to Subject/FromRaw) entirely, since they
// decode the raw header themselves rather than going through
// email.Email's fields. A malicious Subject/From could still manipulate
// the terminal through any of these paths even after ParseReader was
// fixed.
func TestDecDecode_StripsEscapeSequences(t *testing.T) {
	dec := new(mime.WordDecoder)
	got := DecDecode(dec, "Evil\x1b[2JSubject")
	if strings.ContainsAny(got, "\x1b\x07") {
		t.Errorf("DecDecode(...) = %q, still contains an escape byte", got)
	}
	if got != "EvilSubject" {
		t.Errorf("DecDecode(...) = %q, want %q", got, "EvilSubject")
	}
}

func TestWrapQuotedLine_BreaksOverlongURL(t *testing.T) {
	url := "https://url8792.mail.anthropic.com/ls/click?upn=" + strings.Repeat("u001.rFcAm", 12)
	lines := WrapQuotedLine("[2] "+url, "", 40)
	if !strings.HasPrefix(lines[0], "[2] https://") {
		t.Errorf("marker separated from its URL: %q", lines)
	}
	for _, line := range lines {
		if VisibleStringWidth(line) > 40 {
			t.Errorf("line wider than the pane: %q", line)
		}
	}
	if joined := strings.TrimPrefix(strings.Join(lines, ""), "[2] "); joined != url {
		t.Errorf("URL lost text: got %q", joined)
	}
}

func TestWrapQuotedLine_KeepsLinkWords(t *testing.T) {
	link := FormatOSC8Link("https://example.com/"+strings.Repeat("x", 80), "https://example.com/x...")
	lines := WrapQuotedLine("see "+link, "", 20)
	if !strings.Contains(strings.Join(lines, "\n"), link) {
		t.Errorf("hyperlink was split: %q", lines)
	}
}
