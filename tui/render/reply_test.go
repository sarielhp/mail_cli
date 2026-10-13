package render

import (
	"github.com/sarielhp/mail_cli/email"
	"net/mail"
	"os/exec"
	"strings"
	"testing"
)

func TestLongURLsStayClickableWithinMessageAndQuotedText(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "wezterm")
	url := "https://example.com/report?token=" + strings.Repeat("a", 120)
	for _, renderBody := range []func(string, int) ColoredString{ColorReplyLinesWrap, ColorQuotedReplyLinesRaw} {
		body := renderBody("Report: "+email.FormatOSC8Link(url, url)+"\n> Original: "+url, 36)
		var text strings.Builder
		for _, segment := range body.Segments {
			text.WriteString(segment.Text)
		}
		got := text.String()
		if strings.Count(got, "\x1b]8;;"+url+"\x1b\\") != 2 {
			t.Errorf("full link targets missing: %q", got)
		}
		plain := email.StripEscapeSequences(got)
		if !strings.Contains(plain, "Report:") || !strings.Contains(plain, "> Original:") || strings.Count(plain, "...") != 2 {
			t.Errorf("link context or shortening missing: %q", plain)
		}
		for _, line := range strings.Split(got, "\n") {
			if email.VisibleStringWidth(line) > 36 {
				t.Errorf("link overflowed the pane: %q", line)
			}
		}
	}
}

func TestColorReplyLinesRawGraphicalDivider(t *testing.T) {
	body := "Header text\n____________________________________________________\nFooter text"
	cs := ColorReplyLinesRaw(body, 80)

	foundGraphicalDivider := false
	for _, seg := range cs.Segments {
		if strings.Contains(seg.Text, "──") {
			foundGraphicalDivider = true
			if seg.Fg != ColorBorder {
				t.Errorf("expected graphical divider to have ColorBorder, got: %v", seg.Fg)
			}
		}
		if strings.Contains(seg.Text, "____") {
			t.Errorf("found raw underscores in output: %q", seg.Text)
		}
	}

	if !foundGraphicalDivider {
		t.Errorf("expected graphical divider segment in output")
	}
}

func TestColorQuotedReplyLinesRaw(t *testing.T) {
	quoted := "On Monday, Alice wrote:\nHere is the original text."
	cs := ColorQuotedReplyLinesRaw(quoted, 80)

	for _, seg := range cs.Segments {
		if strings.TrimSpace(seg.Text) != "" && seg.Fg == ColorDefault {
			t.Errorf("expected quoted reply line to use non-default quote color, got ColorDefault on segment: %q", seg.Text)
		}
	}
}

// TestHTMLLinkTargetsReachTheDetailPane follows an HTML message the way the
// detail pane does: w3m's dump, then EmbedOSC8Hyperlinks, then the renderer.
// The button's target and a long tracking link must both arrive as complete
// OSC 8 targets, each beside its [n] marker, without overflowing the pane.
func TestHTMLLinkTargetsReachTheDetailPane(t *testing.T) {
	if _, err := exec.LookPath("w3m"); err != nil {
		t.Skip("w3m not installed")
	}
	t.Setenv("TERM_PROGRAM", "")
	t.Setenv("KITTY_WINDOW_ID", "1")
	button := "https://claude.ai/magic-link#0a3ce7a9:c2FyaWVs"
	tracker := "https://url8792.mail.anthropic.com/ls/click?upn=" + strings.Repeat("u001.rFcAm-2B", 15)
	raw := "From: a@example.com\r\nSubject: s\r\nContent-Type: text/html; charset=utf-8\r\n\r\n" +
		`<p>Click below.</p><p><a href="` + button + `">Sign in</a></p>` +
		`<p>Need a hand? <a href="` + tracker + `">Support</a></p>`
	msg, err := mail.ReadMessage(strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	body, err := email.ExtractPlainBodyText(msg)
	if err != nil {
		t.Fatal(err)
	}
	const width = 80
	var text strings.Builder
	for _, segment := range ColorReplyLinesRaw(email.EmbedOSC8Hyperlinks(body), width).Segments {
		text.WriteString(segment.Text)
	}
	got := text.String()
	for _, target := range []string{button, tracker} {
		if !strings.Contains(got, "\x1b]8;;"+target+"\x1b\\") {
			t.Errorf("no complete OSC 8 target for %s:\n%q", target, got)
		}
	}
	plain := email.StripEscapeSequences(got)
	for _, want := range []string{"[1]Sign in", "\n[1] https://claude.ai/", "\n[2] https://url8792."} {
		if !strings.Contains(plain, want) {
			t.Errorf("rendered text lacks %q:\n%s", want, plain)
		}
	}
	for _, line := range strings.Split(got, "\n") {
		if email.VisibleStringWidth(line) > width {
			t.Errorf("line overflowed the pane: %q", email.StripEscapeSequences(line))
		}
	}
}
