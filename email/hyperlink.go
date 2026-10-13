package email

import (
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/mattn/go-runewidth"
)

var (
	// StripEscapeSequences originally covered only plain SGR colour codes
	// and the app's own OSC8 hyperlink shape. Neither is the actual
	// escape-sequence grammar, which matters once this function's job is
	// keeping attacker-controlled email content (Subject, From, body text,
	// attachment filenames) from manipulating the terminal, not just
	// computing display width for wrapping.
	//
	// cscRegex matches a full CSI (Control Sequence Introducer) sequence per
	// ECMA-48: ESC [, parameter bytes 0x30-0x3F, intermediate bytes
	// 0x20-0x2F, one final byte 0x40-0x7E. The old ansiRegex's
	// "[0-9;]*[a-zA-Z]" was narrower than this -- it did not match a
	// private-mode sequence like the cursor-hide "\x1b[?25l", since '?'
	// falls outside "[0-9;]".
	cscRegex = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)
	// oscAnyRegex matches any OSC (Operating System Command) sequence, not
	// only the OSC8 hyperlink form: ESC ] ... terminated by BEL or ST
	// (ESC \). This also covers OSC 52 (clipboard write) and OSC 0/1/2
	// (window/tab title).
	oscAnyRegex = regexp.MustCompile(`\x1b\][^\x07\x1b]*(\x07|\x1b\\)`)
	// otherEscRegex matches the two-character escapes that are neither CSI
	// nor OSC (cursor save/restore, RIS full reset, and similar).
	otherEscRegex = regexp.MustCompile(`\x1b[0-9A-Za-z=><]`)
	// controlCharsRegex matches remaining C0 control bytes and DEL --
	// backspace, BEL, and the like sent on their own, outside any escape
	// sequence -- excluding tab, newline and carriage return, which are
	// legitimate in displayed text.
	controlCharsRegex = regexp.MustCompile(`[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]`)
)

// SupportsOSC8Hyperlinks checks whether the current terminal emulator supports OSC 8 hyperlinks.
func SupportsOSC8Hyperlinks() bool {
	if vte := os.Getenv("VTE_VERSION"); vte != "" {
		if ver, err := strconv.Atoi(vte); err == nil && ver >= 5000 {
			return true
		}
	}
	termProg := strings.ToLower(os.Getenv("TERM_PROGRAM"))
	switch termProg {
	case "iterm.app", "vscode", "wezterm", "ghostty", "warp", "hyper", "alacritty", "contour":
		return true
	}
	if os.Getenv("WT_SESSION") != "" || os.Getenv("KITTY_WINDOW_ID") != "" {
		return true
	}
	term := strings.ToLower(os.Getenv("TERM"))
	if strings.Contains(term, "kitty") || strings.Contains(term, "alacritty") || strings.Contains(term, "wezterm") {
		return true
	}
	return false
}

// FormatOSC8Link formats a URL and display text as an OSC 8 terminal hyperlink:
// \x1b]8;;TargetURL\x1b\DisplayText\x1b]8;;\x1b\
func FormatOSC8Link(targetURL, displayText string) string {
	return "\x1b]8;;" + targetURL + "\x1b\\" + displayText + "\x1b]8;;\x1b\\"
}

// StripEscapeSequences removes OSC 8 and ANSI escape sequences from a string.
func StripEscapeSequences(s string) string {
	s = oscAnyRegex.ReplaceAllString(s, "")
	s = cscRegex.ReplaceAllString(s, "")
	s = otherEscRegex.ReplaceAllString(s, "")
	s = controlCharsRegex.ReplaceAllString(s, "")
	return s
}

// VisibleStringWidth calculates visual display width ignoring OSC 8 and ANSI escape sequences.
func VisibleStringWidth(s string) int {
	clean := StripEscapeSequences(s)
	return runewidth.StringWidth(clean)
}

var linkOrOSC8Regex = regexp.MustCompile(`(\x1b\]8;;[^\x1b\x07]*(?:\x1b\\|\x07).*?\x1b\]8;;(?:\x1b\\|\x07))|(https?://[^\s<>"'\)\]}]+)`)

var osc8LinkPartsRegex = regexp.MustCompile(`^\x1b\]8;;([^\x1b\x07]*)(?:\x1b\\|\x07)(.*?)\x1b\]8;;(?:\x1b\\|\x07)$`)

func cleanTrailingPunct(raw string) (string, string) {
	cut := len(raw)
	for cut > 0 {
		last := raw[cut-1]
		if last == '.' || last == ',' || last == ';' || last == ':' || last == '!' || last == '?' {
			cut--
			continue
		}
		break
	}
	return raw[:cut], raw[cut:]
}

// EmbedOSC8Hyperlinks scans text for URLs and converts them to OSC 8 hyperlinks
// if the terminal supports them.
func EmbedOSC8Hyperlinks(text string) string {
	if !SupportsOSC8Hyperlinks() {
		return text
	}
	return EmbedOSC8HyperlinksAlways(text)
}

// EmbedOSC8HyperlinksAlways converts raw URLs in text into OSC 8 hyperlinks unconditionally.
func EmbedOSC8HyperlinksAlways(text string) string {
	return linkOrOSC8Regex.ReplaceAllStringFunc(text, func(m string) string {
		if strings.HasPrefix(m, "\x1b]8;;") {
			return m
		}
		cleanURL, punct := cleanTrailingPunct(m)
		if cleanURL == "" {
			return m
		}
		return FormatOSC8Link(cleanURL, cleanURL) + punct
	})
}

// EmbedCompactOSC8Hyperlinks shortens the visible URL in place while keeping
// its complete target. Descriptive link labels and unsupported terminals keep
// their original text, so shortening never removes the only usable destination.
func EmbedCompactOSC8Hyperlinks(text string, maxWidth int) string {
	if maxWidth < 4 || !SupportsOSC8Hyperlinks() {
		return text
	}
	return linkOrOSC8Regex.ReplaceAllStringFunc(text, func(link string) string {
		target, punct := cleanTrailingPunct(link)
		if strings.HasPrefix(link, "\x1b]8;;") {
			parts := osc8LinkPartsRegex.FindStringSubmatch(link)
			if len(parts) != 3 || parts[1] != parts[2] {
				return link
			}
			target, punct = parts[1], ""
		}
		if target == "" {
			return link
		}
		display := target
		if VisibleStringWidth(display) > maxWidth {
			display = truncateWord(display, maxWidth-3) + "..."
		}
		return FormatOSC8Link(target, display) + punct
	})
}
