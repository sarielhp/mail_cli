package email

import (
	"bytes"
	"fmt"
	"mime/multipart"
	"net/mail"
	"net/textproto"
	"os/exec"
	"strings"
	"testing"
)

func TestExtractNestedMultipartParts_DepthLimit(t *testing.T) {
	// Construct a deeply nested multipart MIME structure (> 10 levels)
	const nestingLevels = 15
	var buildNested func(level int) []byte
	buildNested = func(level int) []byte {
		if level >= nestingLevels {
			var b bytes.Buffer
			boundary := fmt.Sprintf("boundary_level_%d", level)
			w := multipart.NewWriter(&b)
			_ = w.SetBoundary(boundary)
			pw, _ := w.CreatePart(textproto.MIMEHeader{
				"Content-Type": []string{"text/plain; charset=utf-8"},
			})
			_, _ = pw.Write([]byte("deep inner content"))
			_ = w.Close()
			return b.Bytes()
		}

		var b bytes.Buffer
		boundary := fmt.Sprintf("boundary_level_%d", level)
		w := multipart.NewWriter(&b)
		_ = w.SetBoundary(boundary)
		nestedData := buildNested(level + 1)
		pw, _ := w.CreatePart(textproto.MIMEHeader{
			"Content-Type": []string{fmt.Sprintf("multipart/mixed; boundary=boundary_level_%d", level+1)},
		})
		_, _ = pw.Write(nestedData)
		_ = w.Close()
		return b.Bytes()
	}

	payload := buildNested(0)
	mr := multipart.NewReader(bytes.NewReader(payload), "boundary_level_0")

	// Must terminate safely and return recursion limit error, not panic from stack overflow
	plain, html, err := ExtractNestedMultipartParts(mr)
	if err == nil {
		t.Fatalf("expected error from recursion depth limit, got nil (plain=%d, html=%d)", len(plain), len(html))
	}
	if !strings.Contains(err.Error(), "exceeded maximum nesting depth") {
		t.Errorf("expected nesting depth error message, got: %v", err)
	}
}

func TestExtractNestedMultipartParts_Valid(t *testing.T) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.SetBoundary("root-boundary")

	// Part 1: plain text
	pw1, err := w.CreatePart(textproto.MIMEHeader{
		"Content-Type": []string{"text/plain; charset=utf-8"},
	})
	if err != nil {
		t.Fatalf("CreatePart 1 failed: %v", err)
	}
	_, _ = pw1.Write([]byte("Hello world"))

	// Part 2: nested multipart/alternative
	pw2, err := w.CreatePart(textproto.MIMEHeader{
		"Content-Type": []string{"multipart/alternative; boundary=sub-boundary"},
	})
	if err != nil {
		t.Fatalf("CreatePart 2 failed: %v", err)
	}
	var subBuf bytes.Buffer
	subW := multipart.NewWriter(&subBuf)
	_ = subW.SetBoundary("sub-boundary")
	subP, _ := subW.CreatePart(textproto.MIMEHeader{
		"Content-Type": []string{"text/html; charset=utf-8"},
	})
	_, _ = subP.Write([]byte("<p>Nested HTML</p>"))
	_ = subW.Close()
	_, _ = pw2.Write(subBuf.Bytes())

	_ = w.Close()

	mr := multipart.NewReader(bytes.NewReader(buf.Bytes()), "root-boundary")
	plain, html, err := ExtractNestedMultipartParts(mr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plain) != 1 || plain[0] != "Hello world" {
		t.Errorf("unexpected plain text: %v", plain)
	}
	if len(html) != 1 || html[0] != "<p>Nested HTML</p>" {
		t.Errorf("unexpected html text: %v", html)
	}
}

func TestDecodePartBody_Bounded(t *testing.T) {
	// Verify DecodePartBody does not read beyond maxMIMEPartBytes
	zeros := strings.Repeat("a", 1000)
	res, err := DecodePartBody(strings.NewReader(zeros), "")
	if err != nil {
		t.Fatalf("DecodePartBody failed: %v", err)
	}
	if len(res) != 1000 {
		t.Errorf("expected 1000 bytes, got %d", len(res))
	}
}

func TestRenderHTMLToText_Empty(t *testing.T) {
	res, err := renderHTMLToText("")
	if err != nil {
		t.Fatalf("unexpected error on empty string: %v", err)
	}
	if res != "" {
		t.Errorf("expected empty result, got %q", res)
	}
}

func TestRenderHTMLToText_ValidOrNotInstalled(t *testing.T) {
	res, err := renderHTMLToText("<p>test content</p>")
	if err != nil {
		if !strings.Contains(err.Error(), "w3m not found") {
			t.Errorf("unexpected error: %v", err)
		}
		return
	}
	if !strings.Contains(res, "test content") {
		t.Errorf("expected result to contain 'test content', got %q", res)
	}
}

func TestRenderHTMLToText_KeepsLinkTargets(t *testing.T) {
	if _, err := exec.LookPath("w3m"); err != nil {
		t.Skip("w3m not installed")
	}
	const button = "https://example.com/login?token=abc&next=%2Fhome"
	const icon = "https://example.com/social"
	html := `<p><a href="https://example.com/login?token=abc&amp;next=%2Fhome">Sign in</a></p>` +
		`<p><a href="` + icon + `"><img alt="" src="https://example.com/x.png"></a></p>`
	res, err := renderHTMLToText(html)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	// Each target must stand alone on its reference line: a placeholder w3m
	// took for a relative link once came back as file:///<cwd>/<url>.
	for _, want := range []string{"[1]Sign in", "\n[1] " + button + "\n", "\n[2] " + icon} {
		if !strings.Contains(res, want) {
			t.Errorf("rendered text lacks %q:\n%s", want, res)
		}
	}
}

func TestExtractPlainBodyText_UnwrapsURLDefense(t *testing.T) {
	raw := "From: test@example.com\r\n" +
		"Subject: URLDefense Test\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n" +
		"\r\n" +
		"Please review: https://urldefense.proofpoint.com/v2/url?u=https-3A__github.com_example_mail-5Fcli&d=DwIFAg\r\n"
	msg, err := mail.ReadMessage(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("mail.ReadMessage failed: %v", err)
	}

	plain, err := ExtractPlainBodyText(msg)
	if err != nil {
		t.Fatalf("ExtractPlainBodyText failed: %v", err)
	}
	if strings.Contains(plain, "urldefense.proofpoint.com") {
		t.Errorf("expected URLDefense link to be stripped, got: %s", plain)
	}
	if !strings.Contains(plain, "https://github.com/example/mail_cli") {
		t.Errorf("expected clean URL to be present, got: %s", plain)
	}
}

func TestExtractAlternativeBodyParts(t *testing.T) {
	multipartRaw := "Content-Type: multipart/alternative; boundary=\"alt-boundary\"\r\n\r\n" +
		"--alt-boundary\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n\r\n" +
		"Plain Hello\r\n" +
		"--alt-boundary\r\n" +
		"Content-Type: text/html; charset=utf-8\r\n\r\n" +
		"<b>HTML Hello</b>\r\n" +
		"--alt-boundary--\r\n"

	plain, html, err := ExtractAlternativeBodyParts([]byte(multipartRaw))
	if err != nil {
		t.Fatalf("ExtractAlternativeBodyParts failed: %v", err)
	}
	if !strings.Contains(plain, "Plain Hello") {
		t.Errorf("expected plain text 'Plain Hello', got %q", plain)
	}
	if !strings.Contains(html, "<b>HTML Hello</b>") {
		t.Errorf("expected html '<b>HTML Hello</b>', got %q", html)
	}

	htmlOnly := "Content-Type: text/html; charset=utf-8\r\n\r\n<h1>Title</h1>"
	_, hOnly, err := ExtractAlternativeBodyParts([]byte(htmlOnly))
	if err != nil || !strings.Contains(hOnly, "<h1>Title</h1>") {
		t.Errorf("expected htmlOnly to extract html, got err=%v html=%q", err, hOnly)
	}
}

// TestExtractPlainBodyText_StripsEscapeSequences is a security-review
// finding: the extracted plain body text is shown verbatim in the TUI
// detail pane and the CLI's `show` command (after EmbedOSC8Hyperlinks adds
// its own, legitimate OSC8 sequences around detected URLs), with no
// stripping of escape sequences that were already present in the message
// body itself. A malicious body could embed ANSI/OSC sequences to
// manipulate the terminal, or a raw OSC8 sequence to make a link's
// displayed text lie about its real target -- bypassing the app's own
// URL-detection regex entirely, since a pre-formed sequence is not "found"
// as a bare URL.
func TestExtractPlainBodyText_StripsEscapeSequences(t *testing.T) {
	raw := "Content-Type: text/plain; charset=utf-8\r\n\r\n" +
		"click \x1b]8;;http://evil.example\x1b\\here\x1b]8;;\x1b\\ for a prize\x1b[2Jerased"
	msg, err := mail.ReadMessage(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("mail.ReadMessage: %v", err)
	}
	got, err := ExtractPlainBodyText(msg)
	if err != nil {
		t.Fatalf("ExtractPlainBodyText: %v", err)
	}
	if strings.ContainsAny(got, "\x1b\x07") {
		t.Errorf("extracted body still contains an escape byte: %q", got)
	}
}

// TestExtractAlternativeBodyParts_StripsEscapeSequences covers the second
// body-extraction entry point, used when building the TUI's
// "message_body.txt"/"message_body.html" pseudo-attachments.
func TestExtractAlternativeBodyParts_StripsEscapeSequences(t *testing.T) {
	raw := "Content-Type: multipart/alternative; boundary=\"alt-boundary\"\r\n\r\n" +
		"--alt-boundary\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n\r\n" +
		"plain\x1b[31mtext\r\n" +
		"--alt-boundary\r\n" +
		"Content-Type: text/html; charset=utf-8\r\n\r\n" +
		"<b>html\x1b[31mtext</b>\r\n" +
		"--alt-boundary--\r\n"

	plain, html, err := ExtractAlternativeBodyParts([]byte(raw))
	if err != nil {
		t.Fatalf("ExtractAlternativeBodyParts: %v", err)
	}
	if strings.Contains(plain, "\x1b") {
		t.Errorf("plain part still contains an escape byte: %q", plain)
	}
	if strings.Contains(html, "\x1b") {
		t.Errorf("html part still contains an escape byte: %q", html)
	}
}
