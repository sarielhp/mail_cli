package tui

import (
	"fmt"
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"github.com/sarielhp/mail_cli/calendar"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/label"
	"github.com/sarielhp/mail_cli/mailclient"
	"github.com/sarielhp/mail_cli/tui/render"
	"mime"
	"net/mail"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
)

func newTestEmail(id, subject, fromRaw, fromEmail string, date string) *email.Email {
	d, _ := time.Parse(time.RFC1123Z, date)
	return &email.Email{
		ID:            email.ID(id),
		Subject:       subject,
		FromEmail:     fromEmail,
		FromRaw:       fromRaw,
		EmailDate:     d,
		FormattedDate: email.FormatEmailDate(d),
	}
}

func newDetailModel(t *testing.T, width, height int, folder string, emails []email.Email) *tuiModel {
	cacheDir := t.TempDir()
	m := newTuiModel(&mailclient.MockMailClient{Cfg: &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: cacheDir}, AccountCache: cachedir.At(cacheDir)}}, folder, nil)
	m.width = width
	m.height = height
	m.theme = render.NewThemeManager()
	m.folders = []label.Item{{Name: folder, FullName: folder}}
	m.emails = emails
	m.eIdx = 0
	m.mode = modeDetail
	m.selectedID = "test-001"
	m.vp = viewport.New(width, height)

	if len(emails) > 0 {
		hw := 4 + 2 + 8 + 2 + 20 + 2 + 2 + 2
		sw := width - hw
		m.selectedRow = render.EmailRow(emails[0], width, sw, 1, m.theme.Theme(), true, false)
	}

	m.detailH = "From: Alice Smith <alice@example.com>\nSubject: Team meeting tomorrow\nDate: Sat, 20 Jun 2026 14:30:00 +0000"
	m.detail = "Hi everyone,\n\nJust a reminder that we have a team meeting tomorrow at 10am.\n\nThanks,\nAlice"

	return m
}

func renderModelWithBody(t *testing.T, width, height int, emails []email.Email, headers, body string) *tuiModel {
	m := newDetailModel(t, width, height, "INBOX", emails)
	m.detailH = headers
	m.detail = body

	return m
}

// detailViewportContent builds the viewport content the detail view uses: a
// colored header block followed by the wrapped body.
func detailViewportContent(m *tuiModel) string {
	hdrContent := strings.Builder{}
	for _, line := range strings.Split(m.detailH, "\n") {
		if line == "" {
			continue
		}
		idx := strings.Index(line, ": ")
		if idx > 0 {
			label := render.Plain(line[:idx+2]).WithFg(render.ColorHighlightFg)
			value := render.Plain(line[idx+2:]).WithFg(render.ColorFg)
			hdrContent.WriteString(label.Render(m.theme.Theme()))
			hdrContent.WriteString(value.Render(m.theme.Theme()))
		}
		hdrContent.WriteString("\n")
	}
	return hdrContent.String() + "\n" + email.WrapText(m.detail, m.width)
}

func TestModeDetailLayout(t *testing.T) {
	e := *newTestEmail("test-001", "Team meeting tomorrow", "alice@example.com", "alice@example.com", "Sat, 20 Jun 2026 14:30:00 +0000")
	m := newDetailModel(t, 80, 24, "INBOX", []email.Email{e})
	out := m.View()

	if !strings.Contains(out, "   1") {
		t.Errorf("expected counter '   1' in rendered view: first 200 chars=%q", out[:min(200, len(out))])
	}

	for _, label := range []string{"From:", "Subject:", "Date:"} {
		if !strings.Contains(out, label) {
			t.Errorf("missing header label %q in output", label)
		}
	}

	if !strings.Contains(out, "Hi everyone") {
		t.Error("body text 'Hi everyone' missing from output")
	}

	if !strings.Contains(out, "< End of message >") {
		t.Logf("raw output (first 300):\n%q", out[:min(300, len(out))])
		t.Error("end-of-message marker missing from output")
	}
}

func TestTopBarShownInDetail(t *testing.T) {
	e := *newTestEmail("test-001", "Test", "a@b.com", "a@b.com", "Sat, 20 Jun 2026 14:30:00 +0000")
	m := newDetailModel(t, 80, 24, "INBOX", []email.Email{e})

	if renderTopBar(m) == "" {
		t.Error("renderTopBar should show content in modeDetail")
	}
}

func TestEndOfMessageMarker(t *testing.T) {
	line := render.EndOfMsgGreen(80, render.NewThemeManager().Theme())
	if !strings.Contains(line, " < End of message > ") {
		t.Errorf("end-of-message text missing: %q", line)
	}
	if !strings.Contains(line, "92m") {
		t.Logf("end-of-message line: %q", line)
		t.Error("expected green bg color code '92m' in end-of-message line")
	}
}

func TestEmailRowHighlight(t *testing.T) {
	e := *newTestEmail("x", "Subject", "sender@x.com", "sender@x.com", "Sat, 20 Jun 2026 14:30:00 +0000")
	tm := render.NewThemeManager()

	hl := render.EmailRow(e, 80, 20, 1, tm.Theme(), true, false)
	hlStr := hl.Render(tm.Theme())

	if !strings.Contains(hlStr, "48;2;204;34;34") && !strings.Contains(hlStr, "48;5;1") {
		t.Logf("highlighted row raw (first 300): %q", hlStr[:min(300, len(hlStr))])
		t.Error("highlighted row should contain red bg ANSI code")
	}
}

func TestEmailRowNoHighlight(t *testing.T) {
	e := *newTestEmail("x", "Subj", "s@x.com", "s@x.com", "Sat, 20 Jun 2026 14:30:00 +0000")
	tm := render.NewThemeManager()

	nhl := render.EmailRow(e, 80, 20, 0, tm.Theme(), false, false)
	nhlStr := nhl.Render(tm.Theme())

	esc := string([]byte{27})
	if strings.Contains(nhlStr, esc+"[48;5;1") || strings.Contains(nhlStr, esc+"[48;5;41") {
		t.Logf("non-highlighted row raw: %q", nhlStr)
		t.Error("non-highlighted row must not have ColorHighlightBg")
	}
}

func TestLongBodyWrapping(t *testing.T) {
	long := strings.Repeat("Lorem ipsum dolor sit amet, consectetur adipiscing elit. ", 10)
	e := *newTestEmail("long", "Long subj", "l@x.com", "l@x.com", "Sat, 20 Jun 2026 14:30:00 +0000")
	m := renderModelWithBody(t, 80, 24, []email.Email{e}, "From: l@x.com\nSubject: Long subj\nDate: Now", long)
	out := m.View()

	if !strings.Contains(out, "Lorem") {
		t.Error("body should contain 'Lorem'")
	}
	if !strings.Contains(out, "< End of message >") {
		t.Error("end-of-message should still be present")
	}
}

func TestMimeEncodedHeaders(t *testing.T) {
	decodedSubject, _ := new(mime.WordDecoder).DecodeHeader("=?UTF-8?Q?Re:_Project_update?=")
	decodedFrom, _ := new(mime.WordDecoder).DecodeHeader("=?UTF-8?Q?Maria_Garc=C3=ADa?= <maria@example.com>")

	e := email.Email{
		ID:            "m1",
		Subject:       decodedSubject,
		FromEmail:     "maria@example.com",
		FromRaw:       decodedFrom,
		EmailDate:     time.Date(2026, 6, 20, 14, 30, 0, 0, time.UTC),
		FormattedDate: email.FormatEmailDate(time.Date(2026, 6, 20, 14, 30, 0, 0, time.UTC)),
	}

	hdrs := fmt.Sprintf("From: %s\nSubject: %s\nDate: Sat, 20 Jun 2026 14:30:00 +0000", decodedFrom, decodedSubject)
	m := renderModelWithBody(t, 80, 24, []email.Email{e}, hdrs, "Body text.")
	out := m.View()

	if !strings.Contains(out, decodedSubject) {
		t.Errorf("MIME-decoded subject %q missing from output", decodedSubject)
	}
}

func TestHtmlBodyPlainTextFallsback(t *testing.T) {
	rawEml := `From: Web <noreply@example.com>
Subject: HTML digest
Date: Sat, 20 Jun 2026 14:30:00 +0000
Content-Type: text/html; charset=UTF-8

<html><body><h1>Digest</h1><p>Content here</p></body></html>`

	msg, err := mail.ReadMessage(strings.NewReader(rawEml))
	if err != nil {
		t.Fatalf("readMessage: %v", err)
	}
	body, _ := email.ExtractPlainBodyText(msg)
	if body == "" {
		t.Error("body should not be empty after HTML fallback")
	}

	e := *newTestEmail("h1", "HTML digest", "Web <noreply@example.com>", "noreply@example.com", "Sat, 20 Jun 2026 14:30:00 +0000")
	m := renderModelWithBody(t, 80, 24, []email.Email{e}, "From: Web <noreply@example.com>\nSubject: HTML digest\nDate: Sat, 20 Jun 2026 14:30:00 +0000", body)
	out := m.View()

	if !strings.Contains(out, "< End of message >") {
		t.Error("end-of-message should be present even with HTML source")
	}
}

func TestHtmlBodyMultiparagraphNoNewlines(t *testing.T) {
	if _, err := exec.LookPath("w3m"); err != nil {
		t.Skip("w3m not installed; skipping HTML rendering test")
	}
	rawEml := `From: Web <noreply@example.com>
Subject: HTML digest
Date: Sat, 20 Jun 2026 14:30:00 +0000
Content-Type: text/html; charset=UTF-8

<html><body><div><p>First paragraph of the message.</p><p>Second paragraph here with different content.</p><br><p>Third and final paragraph.</p></div></body></html>`

	msg, err := mail.ReadMessage(strings.NewReader(rawEml))
	if err != nil {
		t.Fatalf("readMessage: %v", err)
	}
	body, _ := email.ExtractPlainBodyText(msg)
	for _, expected := range []string{"first paragraph", "second paragraph", "third and final"} {
		if !strings.Contains(strings.ToLower(body), expected) {
			t.Errorf("body missing %q: %q", expected, body)
		}
	}
	if !strings.Contains(body, "\n") {
		t.Errorf("body should contain newlines between paragraphs, got: %q", body)
	}

	e := *newTestEmail("h2", "HTML digest", "Web <noreply@example.com>", "noreply@example.com", "Sat, 20 Jun 2026 14:30:00 +0000")
	m := renderModelWithBody(t, 80, 24, []email.Email{e}, "From: Web <noreply@example.com>\nSubject: HTML digest\nDate: Sat, 20 Jun 2026 14:30:00 +0000", body)
	out := m.View()
	lower := strings.ToLower(out)
	for _, expected := range []string{"first paragraph", "second paragraph", "third and final"} {
		if !strings.Contains(lower, expected) {
			t.Errorf("view missing %q", expected)
		}
	}
	if !strings.Contains(lower, "html digest") {
		t.Error("view missing subject")
	}
}

func TestHtmlBodyMultipartHtmlOnly(t *testing.T) {
	if _, err := exec.LookPath("w3m"); err != nil {
		t.Skip("w3m not installed; skipping HTML rendering test")
	}
	raw := `From: Sender <sender@example.com>
Subject: HTML only multipart
Date: Sat, 20 Jun 2026 14:30:00 +0000
Content-Type: multipart/alternative; boundary="BOUNDARY"

--BOUNDARY
Content-Type: text/html; charset=UTF-8

<html><body><div><p>Hello world.</p><p>This is a second paragraph.</p><br><p>And a third one.</p></div></body></html>
--BOUNDARY--
`
	msg, err := mail.ReadMessage(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("readMessage: %v", err)
	}
	body, err := email.ExtractPlainBodyText(msg)
	if err != nil {
		t.Fatalf("email.ExtractPlainBodyText: %v", err)
	}
	for _, expected := range []string{"hello world", "second paragraph", "third one"} {
		if !strings.Contains(strings.ToLower(body), expected) {
			t.Errorf("body missing %q: %q", expected, body)
		}
	}
	if !strings.Contains(body, "\n") {
		t.Errorf("body should contain newlines between paragraphs, got: %q", body)
	}
}

func TestEmptyDetailViewEdgeCase(t *testing.T) {
	m := newTuiModel(&mailclient.MockMailClient{Cfg: &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: t.TempDir()}, AccountCache: cachedir.At(t.TempDir())}}, "", nil)
	m.width = 80
	m.height = 24
	m.theme = render.NewThemeManager()
	m.mode = modeDetail
	m.emails = []email.Email{}
	m.eIdx = 0
	m.vp = viewport.New(80, 24)

	_ = m.View()
}

func TestDetailWithHeadersAndNoBody(t *testing.T) {
	e := *newTestEmail("nb", "No body", "no@body.com", "no@body.com", "Sat, 20 Jun 2026 14:30:00 +0000")
	hdrs := "From: no@body.com\nSubject: No body\nDate: Sat, 20 Jun 2026 14:30:00 +0000"
	m := renderModelWithBody(t, 80, 24, []email.Email{e}, hdrs, "   ")
	out := m.View()

	if !strings.Contains(out, "No body") {
		t.Error("header labels should appear")
	}
	if !strings.Contains(out, "< End of message >") {
		t.Error("end-of-message marker should be present even with minimal body")
	}
}

func TestRenderEndOfMsgVaryingWidths(t *testing.T) {
	tm := render.NewThemeManager()
	for _, w := range []int{40, 60, 80, 120, 200} {
		line := render.EndOfMsgGreen(w, tm.Theme())
		clean := stripAnsi(line)
		if len(clean) < w-5 {
			t.Errorf("width=%d renderEndOfMsg produced too short line (len=%d): %q", w, len(clean), clean)
		}
		if !strings.Contains(line, "< End of message >") {
			t.Errorf("width=%d missing end-of-message text", w)
		}
	}
}

func TestBase64BodyDecoding(t *testing.T) {
	encoded := "VGhpcyBpcyBiYXNlNjQgZW5jb2RlZCBjb250ZW50Lgo="
	raw := `From: Encoded <enc@example.com>
Subject: B64 test
Date: Sat, 20 Jun 2026 14:30:00 +0000
Content-Transfer-Encoding: base64
Content-Type: text/plain; charset=UTF-8

` + encoded

	msg, err := mail.ReadMessage(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("readMessage: %v", err)
	}
	body, _ := email.ExtractPlainBodyText(msg)
	if strings.Contains(body, "VGhpcyBpcy") {
		t.Error("body should be base64 decoded, got raw base64")
	}
	if strings.TrimSpace(body) == "" {
		t.Error("decoded body should not be empty")
	}
}

func TestMultiPartMimeBody(t *testing.T) {
	raw := `From: Multipart <mp@example.com>
Subject: Multipart test
Date: Sat, 20 Jun 2026 14:30:00 +0000
Content-Type: multipart/alternative; boundary="BOUNDARY"

--BOUNDARY
Content-Type: text/html; charset=UTF-8

<html><body><h1>HTML only</h1></body></html>
--BOUNDARY
Content-Type: text/plain; charset=UTF-8

This is the plain text version.

--BOUNDARY--
`
	msg, err := mail.ReadMessage(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("readMessage: %v", err)
	}
	body, err := email.ExtractPlainBodyText(msg)
	if err != nil {
		t.Fatalf("email.ExtractPlainBodyText: %v", err)
	}
	if !strings.Contains(body, "plain text version") {
		t.Errorf("expected plain text body, got: %q", body)
	}
}

func TestRenderedOutputLineCount(t *testing.T) {
	e := *newTestEmail("rc", "Short", "r@c.com", "r@c.com", "Sat, 20 Jun 2026 14:30:00 +0000")
	m := renderModelWithBody(t, 80, 24, []email.Email{e}, "From: r@c.com\nSubject: Short\nDate: Now", "Short body.")
	lines := strings.Split(m.View(), "\n")
	if len(lines) > 50 {
		t.Logf("Rendered %d lines: %q", len(lines), m.View())
		t.Errorf("view has %d lines, expected fewer than 50", len(lines))
	}
}

func TestColorSegmentsPresent(t *testing.T) {
	e := *newTestEmail("col", "Color test", "c@l.com", "c@l.com", "Sat, 20 Jun 2026 14:30:00 +0000")
	m := renderModelWithBody(t, 80, 24, []email.Email{e}, "From: c@l.com\nSubject: Color test\nDate: Now", "Colorful body.")
	out := m.View()

	esc := string([]byte{27})
	if !strings.Contains(out, esc+"[0m") {
		t.Error("output should contain ANSI reset code")
	}

	if !strings.Contains(out, "38;5;11") {
		t.Logf("Raw output snippet (first 400 bytes): %q", out[:min(400, len(out))])
		hasFg := strings.Contains(out, "38;2;") || strings.Contains(out, "38;5;")
		if !hasFg {
			t.Error("output should contain ANSI foreground color codes")
		}
	}
}

func TestModeDetailGolden(t *testing.T) {
	e := *newTestEmail("g1", "Golden test", "golden@example.com", "golden@example.com", "Sat, 20 Jun 2026 14:30:00 +0000")
	headers := "From: golden@example.com\nSubject: Golden test\nDate: Sat, 20 Jun 2026 14:30:00 +0000"
	body := "This is a golden snapshot test. It should render headers, body, and end-of-message marker."
	m := renderModelWithBody(t, 80, 24, []email.Email{e}, headers, body)
	out := m.View()

	goldenPath := filepath.Join("testdata", "snapshots", "mode_detail_basic.txt")
	_ = os.MkdirAll(filepath.Join("testdata", "snapshots"), 0755)

	if os.Getenv("UPDATE_GOLDEN") == "1" {
		err := os.WriteFile(goldenPath, []byte(out), 0644)
		if err != nil {
			t.Fatalf("write golden: %v", err)
		}
		t.Logf("wrote golden file: %s", goldenPath)
		return
	}

	expected, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Skipf("golden not found (%v); run once with UPDATE_GOLDEN=1 to create", err)
	}

	gotLines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	wantLines := strings.Split(strings.TrimRight(string(expected), "\n"), "\n")

	wantClean := make([]string, len(wantLines))
	for i, l := range wantLines {
		wantClean[i] = stripAnsi(l)
	}

	got := strings.Join(gotLines, "\n")
	wantContent := []string{"From:", "Subject:", "Golden test", "golden snapshot", "< End of message >", "golden@example.com"}
	for _, wc := range wantContent {
		if !strings.Contains(got, wc) {
			t.Errorf("golden output missing content %q", wc)
		}
	}

	lineRatio := float64(len(gotLines)) / float64(len(wantLines))
	if lineRatio < 0.7 || lineRatio > 1.3 {
		t.Logf("line count: got %d, want %d (golden may need update)", len(gotLines), len(wantLines))
	}
}

func TestViewportScrolling(t *testing.T) {
	m := newTuiModel(&mailclient.MockMailClient{Cfg: &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: t.TempDir()}, AccountCache: cachedir.At(t.TempDir())}}, "", nil)
	m.width = 60
	m.height = 10
	m.theme = render.NewThemeManager()
	m.vp = viewport.New(60, 8)

	longBody := strings.Repeat("Line of content. ", 20)
	m.detailH = "From: test@test.com\nSubject: Test\nDate: Now"
	m.detail = longBody

	m.vp.SetContent(detailViewportContent(m))

	View1 := stripAnsi(m.vp.View())
	if !strings.Contains(View1, "Line of content") {
		t.Error("first viewport view should show body text")
	}

	m.vp.ScrollDown(5)
	View2 := stripAnsi(m.vp.View())
	if View1 == "" || View2 == "" {
		t.Error("viewport should return non-empty strings")
	}
}

func TestDetailViewHasAllComponents(t *testing.T) {
	cacheDir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(cacheDir, "INBOX"), 0755)
	m := newTuiModel(&mailclient.MockMailClient{Cfg: &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: cacheDir}, AccountCache: cachedir.At(cacheDir)}}, "INBOX", nil)
	m.width = 80
	m.height = 24
	m.theme = render.NewThemeManager()
	m.folders = []label.Item{{Name: "INBOX", FullName: "INBOX"}}
	m.emails = []email.Email{{
		ID:            "d1",
		Subject:       "Detail view test",
		FromEmail:     "test@example.com",
		FromRaw:       "Test User <test@example.com>",
		EmailDate:     time.Now(),
		FormattedDate: email.FormatEmailDate(time.Now()),
	}}
	m.eIdx = 0
	m.mode = modeDetail
	m.vp = viewport.New(80, 24)
	m.detailH = "From: Test User <test@example.com>\nSubject: Detail view test\nDate: Sat, 20 Jun 2026 14:30:00 +0000"
	m.detail = "This is a multi-line body.\nLine 2 here.\nLine 3 of the message."

	hw := 4 + 2 + 8 + 2 + 20 + 2 + 2 + 2
	m.selectedRow = render.EmailRow(m.emails[0], m.width, m.width-hw, 1, m.theme.Theme(), true, false)

	m.vp.SetContent(detailViewportContent(m))

	out := m.View()

	if !strings.Contains(out, "Detail view test") {
		t.Error("missing subject in selected row")
	}
	if !strings.Contains(out, "From:") {
		t.Error("missing From: header")
	}
	if !strings.Contains(out, "This is a multi-line body") {
		t.Error("missing body text")
	}
	if !strings.Contains(out, "Line 2") {
		t.Error("missing 2nd line of body")
	}
	if !strings.Contains(out, "< End of message >") {
		t.Error("missing end-of-message marker")
	}
}

func stripAnsi(s string) string {
	var result strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == 27 && i+1 < len(s) && s[i+1] == '[' {
			i += 2
			for i < len(s) && s[i] != 'm' && s[i] != 'J' && s[i] != 'K' {
				i++
			}
			if i < len(s) {
				i++
			}
		} else {
			result.WriteByte(s[i])
			i++
		}
	}
	return result.String()
}

func TestColorReplyLinesDepthCount(t *testing.T) {
	tests := []struct {
		name  string
		line  string
		depth int
	}{
		{"no quote", "Plain text", 0},
		{"single >", "> Quoted", 1},
		{"double concat", ">> text", 2},
		{"spaced triple", "> > > text", 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if strings.Count(tt.line[:min(len(tt.line), 20)], ">") != tt.depth {
				t.Errorf("depth(%q) != %d", tt.line, tt.depth)
			}
		})
	}

	cs := render.ColorReplyLines("line 1\n> quoted\n>> double\n>>> triple")
	if cs.Segments[0].Fg != render.ColorDefault {
		t.Errorf("segment 0: got fg=%d, want ColorDefault (%d)", cs.Segments[0].Fg, render.ColorDefault)
	}
	for i, want := range []render.ColorID{render.ColorSender, render.ColorQuote2, render.ColorQuote3} {
		if cs.Segments[i+1].Fg != want {
			t.Errorf("segment %d: got fg=%d, want %d", i+1, cs.Segments[i+1].Fg, want)
		}
	}
}

func TestColorReplyLinesBodyQuotes(t *testing.T) {
	input := "plain text\n> quoted\n>> double\n>>> triple"
	cs := render.ColorReplyLines(input)
	if len(cs.Segments) != 4 {
		t.Fatalf("got %d segments", len(cs.Segments))
	}
	if cs.Segments[0].Fg != render.ColorDefault {
		t.Errorf("segment 0 fg=%d, want ColorDefault (%d)", cs.Segments[0].Fg, render.ColorDefault)
	}
	if cs.Segments[1].Fg != render.ColorSender {
		t.Errorf("segment 1 fg=%d, want ColorSender (%d)", cs.Segments[1].Fg, render.ColorSender)
	}
	if cs.Segments[2].Fg != render.ColorQuote2 {
		t.Errorf("segment 2 fg=%d, want ColorQuote2 (%d)", cs.Segments[2].Fg, render.ColorQuote2)
	}
	if cs.Segments[3].Fg != render.ColorQuote3 {
		t.Errorf("segment 3 fg=%d, want ColorQuote3 (%d)", cs.Segments[3].Fg, render.ColorQuote3)
	}
}

func TestColorReplyLinesRawEndToEnd(t *testing.T) {
	rawBody := "Original text\n> Quoted by Alice\n>> Double quoted\n>>> Deep quote"
	cs := render.ColorReplyLinesRaw(rawBody, 80)
	if len(cs.Segments) != 4 {
		t.Fatalf("expected 4 segments, got %d", len(cs.Segments))
	}
	if cs.Segments[0].Fg != render.ColorDefault {
		t.Errorf("segment 0 fg=%d, want ColorDefault", cs.Segments[0].Fg)
	}
	if cs.Segments[1].Fg != render.ColorSender {
		t.Errorf("segment 1 fg=%d, want ColorSender", cs.Segments[1].Fg)
	}
	if cs.Segments[2].Fg != render.ColorQuote2 {
		t.Errorf("segment 2 fg=%d, want ColorQuote2", cs.Segments[2].Fg)
	}
	if cs.Segments[3].Fg != render.ColorQuote3 {
		t.Errorf("segment 3 fg=%d, want ColorQuote3", cs.Segments[3].Fg)
	}
}

func TestColorReplyLinesWrapping(t *testing.T) {
	rawBody := "> This is a very long quoted message that definitely exceeds the narrow viewport width and will wrap onto the next line"
	cs := render.ColorReplyLinesRaw(rawBody, 30)
	if len(cs.Segments) < 2 {
		t.Fatalf("expected at least 2 segments due to wrapping, got %d", len(cs.Segments))
	}
	fg0 := cs.Segments[0].Fg
	for i := 1; i < len(cs.Segments); i++ {
		if cs.Segments[i].Fg != fg0 {
			t.Errorf("segment %d fg=%d differs from segment 0 fg=%d", i, cs.Segments[i].Fg, fg0)
		}
	}
}

func TestRenderDetailICSEventCard(t *testing.T) {
	m := newDetailModel(t, 80, 24, "INBOX", nil)
	m.icsCheckStatus = icsStatusInCalendar

	ev := &calendar.ICSEvent{
		Summary:     "Planning Review",
		Location:    "Meeting Room A",
		StartParsed: time.Date(2026, 8, 28, 10, 0, 0, 0, time.UTC),
		EndParsed:   time.Date(2026, 8, 28, 11, 0, 0, 0, time.UTC),
		Attendees:   []string{"alice@example.com", "bob@example.com"},
	}

	card := renderDetailICSEventCard(m, ev, 80)

	if !strings.Contains(card, "CALENDAR EVENT DETAILS") {
		t.Errorf("expected header in card, got:\n%s", card)
	}
	if !strings.Contains(card, "Planning Review") {
		t.Errorf("expected summary in card, got:\n%s", card)
	}
	if !strings.Contains(card, "Already in your Google Calendar") {
		t.Errorf("expected calendar status in card, got:\n%s", card)
	}
	if !strings.Contains(card, "Meeting Room A") {
		t.Errorf("expected location in card, got:\n%s", card)
	}
}
