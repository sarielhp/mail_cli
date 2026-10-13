package email

import (
	"os"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestCompactHyperlinksPreserveTargetsAndContext(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "wezterm")
	url := "https://example.com/report?token=" + strings.Repeat("a", 120)
	for _, input := range []string{url, FormatOSC8Link(url, url)} {
		got := EmbedCompactOSC8Hyperlinks("Read "+input+", then reply.", 32)
		parts := osc8LinkPartsRegex.FindStringSubmatch(linkOrOSC8Regex.FindString(got))
		if len(parts) != 3 || parts[1] != url {
			t.Fatalf("original destination was lost: %q", got)
		}
		if VisibleStringWidth(parts[2]) != 32 || !strings.HasSuffix(parts[2], "...") {
			t.Errorf("display should be shortened to 32 columns: %q", parts[2])
		}
		if plain := StripEscapeSequences(got); plain != "Read "+parts[2]+", then reply." {
			t.Errorf("surrounding context changed: %q", plain)
		}
		if again := EmbedCompactOSC8Hyperlinks(got, 32); again != got {
			t.Error("rendering a compact link again changed it")
		}
	}
	short := "https://example.com"
	if got := StripEscapeSequences(EmbedCompactOSC8Hyperlinks(short, 32)); got != short {
		t.Errorf("short link changed: %q", got)
	}
	labelled := FormatOSC8Link(url, "View the report")
	if got := EmbedCompactOSC8Hyperlinks(labelled, 32); got != labelled {
		t.Error("descriptive link label changed")
	}
	unicodeURL := "https://example.com/" + strings.Repeat("文件", 50)
	got := EmbedCompactOSC8Hyperlinks(unicodeURL, 32)
	if !utf8.ValidString(got) || VisibleStringWidth(got) > 32 || !strings.Contains(got, unicodeURL) {
		t.Errorf("Unicode URL or display width damaged: %q", got)
	}
}

func TestCompactHyperlinksKeepFullURLsWithoutTerminalSupport(t *testing.T) {
	for _, key := range []string{"TERM_PROGRAM", "VTE_VERSION", "WT_SESSION", "KITTY_WINDOW_ID"} {
		t.Setenv(key, "")
	}
	t.Setenv("TERM", "dumb")
	input := "Read https://example.com/?token=" + strings.Repeat("x", 100)
	if got := EmbedCompactOSC8Hyperlinks(input, 32); got != input {
		t.Errorf("unsupported terminal lost the original URL: %q", got)
	}
}

func TestFormatOSC8Link(t *testing.T) {
	url := "https://example.com"
	text := "Example"
	got := FormatOSC8Link(url, text)
	want := "\x1b]8;;https://example.com\x1b\\Example\x1b]8;;\x1b\\"
	if got != want {
		t.Errorf("FormatOSC8Link(%q, %q) = %q, want %q", url, text, got, want)
	}
}

func TestStripEscapeSequences(t *testing.T) {
	link := "\x1b[1mBold\x1b[0m \x1b]8;;https://example.com\x1b\\Click Here\x1b]8;;\x1b\\"
	got := StripEscapeSequences(link)
	want := "Bold Click Here"
	if got != want {
		t.Errorf("StripEscapeSequences(%q) = %q, want %q", link, got, want)
	}
}

func TestVisibleStringWidth(t *testing.T) {
	plain := "Click here for info"
	link := FormatOSC8Link("https://example.com/very/long/url/that/would/otherwise/inflate/width", plain)
	if w := VisibleStringWidth(link); w != len(plain) {
		t.Errorf("VisibleStringWidth(%q) = %d, want %d", link, w, len(plain))
	}
}

func TestEmbedOSC8HyperlinksAlways(t *testing.T) {
	input := "Visit https://google.com for search and https://github.com/test, okay?"
	got := EmbedOSC8HyperlinksAlways(input)

	if !strings.Contains(got, "\x1b]8;;https://google.com\x1b\\https://google.com\x1b]8;;\x1b\\") {
		t.Errorf("expected embedded google link, got: %q", got)
	}
	if !strings.Contains(got, "\x1b]8;;https://github.com/test\x1b\\https://github.com/test\x1b]8;;\x1b\\,") {
		t.Errorf("expected embedded github link with trailing comma preserved, got: %q", got)
	}

	// Idempotency: formatting an already embedded link should not duplicate escape sequences
	double := EmbedOSC8HyperlinksAlways(got)
	if double != got {
		t.Errorf("expected idempotency for already embedded links, got %q, want %q", double, got)
	}
}

func TestSupportsOSC8Hyperlinks(t *testing.T) {
	origTerm := os.Getenv("TERM_PROGRAM")
	defer os.Setenv("TERM_PROGRAM", origTerm)

	os.Setenv("TERM_PROGRAM", "iTerm.app")
	if !SupportsOSC8Hyperlinks() {
		t.Errorf("expected SupportsOSC8Hyperlinks to be true for iTerm.app")
	}

	os.Setenv("TERM_PROGRAM", "dumb")
	origVTE := os.Getenv("VTE_VERSION")
	origWT := os.Getenv("WT_SESSION")
	origKitty := os.Getenv("KITTY_WINDOW_ID")
	origT := os.Getenv("TERM")
	defer func() {
		os.Setenv("VTE_VERSION", origVTE)
		os.Setenv("WT_SESSION", origWT)
		os.Setenv("KITTY_WINDOW_ID", origKitty)
		os.Setenv("TERM", origT)
	}()
	os.Unsetenv("VTE_VERSION")
	os.Unsetenv("WT_SESSION")
	os.Unsetenv("KITTY_WINDOW_ID")
	os.Setenv("TERM", "dumb")

	if SupportsOSC8Hyperlinks() {
		t.Errorf("expected SupportsOSC8Hyperlinks to be false for dumb terminal")
	}
}

// assertEscapeStripped checks that StripEscapeSequences strips the escape
// grammar from in and returns want.
func assertEscapeStripped(t *testing.T, in, want string) {
	t.Helper()
	if got := StripEscapeSequences(in); got != want {
		t.Errorf("StripEscapeSequences(%q) = %q, want %q", in, got, want)
	}
}

// TestStripEscapeSequences_CoversPrivateCSIAndOSC52AndControlChars is a
// security-review finding: the old regexes (`\x1b\[[0-9;]*[a-zA-Z]` for CSI,
// `\x1b\]8;;...` for OSC only the 8-hyperlink form) missed a private-mode CSI
// sequence like the cursor-hide "\x1b[?25l" (the "?" falls outside
// "[0-9;]"), any other OSC command such as 52 (clipboard write) or 0/1/2
// (window/tab title), and bare C0 control bytes like backspace or BEL sent
// on their own. This is the function every terminal-facing render path is
// supposed to rely on to keep attacker-controlled email content
// (Subject, From, body, attachment filenames) from manipulating the
// terminal, so it has to cover the actual escape-sequence grammar, not just
// the common colour-code shape.
func TestStripEscapeSequences_CoversPrivateCSIAndOSC52AndControlChars(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain ANSI color (already covered)", "hello\x1b[31mworld\x1b[0m", "helloworld"},
		{"private CSI (cursor hide)", "hide\x1b[?25lcursor", "hidecursor"},
		{"OSC set window title", "title\x1b]0;evil title\x07here", "titlehere"},
		{"OSC 52 clipboard write", "clip\x1b]52;c;ZXZpbA==\x07steal", "clipsteal"},
		{"OSC8 hyperlink (already covered)", "link" + FormatOSC8Link("http://evil.example", "click") + "here", "linkclickhere"},
		{"bare backspace characters", "back\x08\x08\x08space", "backspace"},
		{"bare bell character", "bell\x07sound", "bellsound"},
		{"legitimate whitespace preserved", "normal text\twith\ttabs\nand\nnewlines", "normal text\twith\ttabs\nand\nnewlines"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assertEscapeStripped(t, tc.in, tc.want)
		})
	}
}
