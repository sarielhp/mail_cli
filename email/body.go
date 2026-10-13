package email

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"github.com/sarielhp/mail_cli/terminal"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"os/exec"
	"strings"
	"time"
)

func renderHTMLToText(htmlStr string) (string, error) {
	if htmlStr == "" {
		return "", nil
	}

	w3mPath, err := exec.LookPath("w3m")
	if err != nil {
		return "", fmt.Errorf("w3m not found (required for HTML email rendering); please install it (e.g. apt install w3m), or provide a text/plain part in your emails. (looked in $PATH for %q)", "w3m")
	}
	_ = w3mPath

	cols := terminal.Width()
	if cols > 6 {
		cols -= 4
	}
	if cols > 100 {
		cols = 100
	}
	if cols < 20 {
		cols = 70
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// display_link_number keeps every link target: a dump otherwise prints only
	// the anchor text, so a "Sign in" button or an image link lost its URL.
	// w3m numbers each link inline and lists the targets under References.
	// Keep URLs intact while w3m wraps the surrounding HTML into text.
	// Restore them before callers sanitize escapes and embed hyperlinks.
	htmlStr, restoreURLs := protectHTMLURLs(htmlStr)
	cmd := exec.CommandContext(ctx, "w3m", "-dump", "-cols", fmt.Sprintf("%d", cols), "-T", "text/html", "-no-cookie", "-o", "display_link_number=1")
	cmd.Stdin = strings.NewReader(htmlStr)
	var out bytes.Buffer
	cmd.Stdout = &out

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("w3m -dump timed out after 5s")
		}
		return "", fmt.Errorf("w3m -dump failed: %w", err)
	}

	result := strings.TrimSpace(out.String())
	if result == "" {
		return "", fmt.Errorf("w3m -dump produced empty output for %d bytes of HTML content", len(htmlStr))
	}
	return restoreURLs(result), nil
}

const (
	maxMIMEDepth     = 10
	maxMIMEPartBytes = 25 * 1024 * 1024 // 25MB ceiling per part payload
)

// DecodePartBody decodes base64 or quoted-printable body content bounded to maxMIMEPartBytes.
func DecodePartBody(r io.Reader, encoding string) ([]byte, error) {
	lr := io.LimitReader(r, maxMIMEPartBytes)
	encoding = strings.ToLower(strings.TrimSpace(encoding))
	if encoding == "base64" {
		dec := base64.NewDecoder(base64.StdEncoding, lr)
		return io.ReadAll(dec)
	}
	if encoding == "quoted-printable" {
		dec := quotedprintable.NewReader(lr)
		return io.ReadAll(dec)
	}
	return io.ReadAll(lr)
}

func renderHTMLWithFallback(htmlContent string) string {
	res, err := renderHTMLToText(htmlContent)
	if err == nil {
		return res
	}
	if fallback := StripHTML(htmlContent); fallback != "" {
		return fallback
	}
	return htmlContent
}

func appendPartContentDepth(part *multipart.Part, plainParts, htmlParts []string, depth int) ([]string, []string, error) {
	if depth > maxMIMEDepth {
		return plainParts, htmlParts, fmt.Errorf("mime parsing exceeded maximum nesting depth of %d", maxMIMEDepth)
	}
	partMediaType, _, err := mime.ParseMediaType(part.Header.Get("Content-Type"))
	if err != nil {
		return plainParts, htmlParts, nil
	}
	if strings.HasPrefix(partMediaType, "multipart/") {
		_, nestedParams, _ := mime.ParseMediaType(part.Header.Get("Content-Type"))
		nestedMR := multipart.NewReader(part, nestedParams["boundary"])
		nestedPlain, nestedHTML, nestedErr := extractNestedMultipartPartsDepth(nestedMR, depth+1)
		if nestedErr != nil {
			return plainParts, htmlParts, nestedErr
		}
		plainParts = append(plainParts, nestedPlain...)
		htmlParts = append(htmlParts, nestedHTML...)
		return plainParts, htmlParts, nil
	}
	partEncoding := part.Header.Get("Content-Transfer-Encoding")
	partBytes, err := DecodePartBody(part, partEncoding)
	if err != nil {
		return plainParts, htmlParts, nil
	}
	switch partMediaType {
	case "text/plain":
		plainParts = append(plainParts, string(partBytes))
	case "text/html":
		htmlParts = append(htmlParts, string(partBytes))
	}
	return plainParts, htmlParts, nil
}

func extractMultipartBody(body io.Reader, boundary string) (string, error) {
	mr := multipart.NewReader(body, boundary)
	plainTextParts, htmlParts, err := extractNestedMultipartPartsDepth(mr, 0)
	if err != nil {
		return "", err
	}

	if len(plainTextParts) > 0 {
		return strings.Join(plainTextParts, "\n"), nil
	}
	if len(htmlParts) > 0 {
		return renderHTMLWithFallback(strings.Join(htmlParts, "\n")), nil
	}
	return "", nil
}

// ExtractPlainBodyText extracts plain text from a parsed email message/part.
func ExtractPlainBodyText(msg *mail.Message) (string, error) {
	text, err := extractRawPlainBodyText(msg)
	if err != nil {
		return "", err
	}
	// Stripped here, before EmbedOSC8Hyperlinks (called separately by every
	// caller of this function) adds its own, legitimate OSC8 sequences
	// around detected URLs: a message body is otherwise shown verbatim in
	// the TUI detail pane and the CLI's `show` command, and a raw escape
	// sequence already present in it could manipulate the terminal, or a
	// raw OSC8 sequence could make a link's displayed text lie about its
	// real target -- bypassing this app's own URL-detection regex entirely,
	// since a pre-formed sequence is never "found" as a bare URL.
	return StripEscapeSequences(UnwrapURLDefenseInText(text)), nil
}

func extractRawPlainBodyText(msg *mail.Message) (string, error) {
	mediaType, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	if err != nil {
		bodyBytes, _ := io.ReadAll(io.LimitReader(msg.Body, maxMIMEPartBytes))
		return string(bodyBytes), nil
	}

	if strings.HasPrefix(mediaType, "multipart/") {
		return extractMultipartBody(msg.Body, params["boundary"])
	}

	encoding := msg.Header.Get("Content-Transfer-Encoding")
	bodyBytes, err := DecodePartBody(msg.Body, encoding)
	if err != nil {
		return "", err
	}

	if mediaType == "text/html" {
		return renderHTMLWithFallback(string(bodyBytes)), nil
	}
	return string(bodyBytes), nil
}

// ExtractNestedMultipartParts recursively extracts plain text and HTML from nested multipart parts.
func ExtractNestedMultipartParts(mr *multipart.Reader) ([]string, []string, error) {
	return extractNestedMultipartPartsDepth(mr, 0)
}

func extractNestedMultipartPartsDepth(mr *multipart.Reader, depth int) ([]string, []string, error) {
	if depth > maxMIMEDepth {
		return nil, nil, fmt.Errorf("mime parsing exceeded maximum nesting depth of %d", maxMIMEDepth)
	}

	var plainTextParts []string
	var htmlParts []string

	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return plainTextParts, htmlParts, err
		}
		plainTextParts, htmlParts, err = appendPartContentDepth(part, plainTextParts, htmlParts, depth)
		if err != nil {
			return plainTextParts, htmlParts, err
		}
	}
	return plainTextParts, htmlParts, nil
}

// ExtractAlternativeBodyParts extracts text/plain and text/html bodies from
// raw email bytes.
//
// Both returned parts are stripped of terminal escape sequences: the plain
// part is shown verbatim by callers, and the html part is exposed as a
// "message_body.html" pseudo-attachment a user can open -- so a malicious
// message could otherwise embed ANSI/OSC sequences in either to manipulate
// the terminal wherever they end up displayed.
func ExtractAlternativeBodyParts(data []byte) (string, string, error) {
	plain, html, err := extractAlternativeBodyPartsRaw(data)
	if err != nil {
		return "", "", err
	}
	return StripEscapeSequences(plain), StripEscapeSequences(html), nil
}

func extractAlternativeBodyPartsRaw(data []byte) (string, string, error) {
	msg, err := mail.ReadMessage(bytes.NewReader(data))
	if err != nil {
		return "", "", err
	}
	mediaType, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	if err != nil {
		bodyBytes, _ := io.ReadAll(io.LimitReader(msg.Body, maxMIMEPartBytes))
		return string(bodyBytes), "", nil
	}
	if strings.HasPrefix(mediaType, "multipart/") {
		mr := multipart.NewReader(msg.Body, params["boundary"])
		plainParts, htmlParts, extractErr := extractNestedMultipartPartsDepth(mr, 0)
		if extractErr != nil {
			return "", "", extractErr
		}
		return strings.Join(plainParts, "\n"), strings.Join(htmlParts, "\n"), nil
	}
	encoding := msg.Header.Get("Content-Transfer-Encoding")
	bodyBytes, err := DecodePartBody(msg.Body, encoding)
	if err != nil {
		return "", "", err
	}
	if mediaType == "text/html" {
		return "", string(bodyBytes), nil
	}
	return string(bodyBytes), "", nil
}
