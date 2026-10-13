package email

import (
	"mime"
	"net/mail"
	"path/filepath"
	"strings"
	"testing"
)

// TestBuildRFC822_RejectsHeaderInjection reproduces the original exploit: a
// hostile inbound Subject, RFC 2047-encoded with CRLF, carried into a reply.
func TestBuildRFC822_RejectsHeaderInjection(t *testing.T) {
	hostile := "=?UTF-8?Q?Hi=0D=0ABcc:_attacker@example.com?="
	decoded, err := new(mime.WordDecoder).DecodeHeader(hostile)
	if err != nil {
		t.Fatalf("decoding the hostile header failed: %v", err)
	}
	if !strings.Contains(decoded, "\r\n") {
		t.Fatalf("test premise broken: decoded value has no CRLF: %q", decoded)
	}

	cm := &ComposeMessage{
		From:    "me@example.com",
		To:      "someone@example.com",
		Subject: "Re: " + decoded,
		Body:    "reply text",
	}
	raw, err := cm.BuildRFC822()
	if err != nil {
		t.Fatalf("BuildRFC822: %v", err)
	}

	msg, err := mail.ReadMessage(strings.NewReader(string(raw)))
	if err != nil {
		t.Fatalf("generated message does not parse: %v", err)
	}
	if got := msg.Header.Get("Bcc"); got != "" {
		t.Errorf("header injection: unexpected Bcc %q", got)
	}
	if n := len(msg.Header["Subject"]); n != 1 {
		t.Errorf("expected exactly one Subject header, got %d", n)
	}
}

// TestBuildRFC822_SanitizesEveryHeader covers the address and threading headers,
// which received no encoding or sanitization at all before.
func TestBuildRFC822_SanitizesEveryHeader(t *testing.T) {
	evil := "x@example.com\r\nBcc: attacker@example.com"
	for _, tc := range []struct {
		name string
		cm   *ComposeMessage
	}{
		{"From", &ComposeMessage{From: evil, To: "a@b.c"}},
		{"To", &ComposeMessage{From: "a@b.c", To: evil}},
		{"Cc", &ComposeMessage{From: "a@b.c", To: "a@b.c", Cc: evil}},
		{"Bcc", &ComposeMessage{From: "a@b.c", To: "a@b.c", Bcc: evil}},
		{"InReplyTo", &ComposeMessage{From: "a@b.c", To: "a@b.c", InReplyTo: evil}},
		{"References", &ComposeMessage{From: "a@b.c", To: "a@b.c", References: evil}},
	} {
		raw, err := tc.cm.BuildRFC822()
		if err != nil {
			t.Fatalf("%s: BuildRFC822: %v", tc.name, err)
		}
		headerBlock, _, _ := strings.Cut(string(raw), "\r\n\r\n")
		for _, line := range strings.Split(headerBlock, "\r\n") {
			if strings.HasPrefix(line, "Bcc: attacker@") {
				t.Errorf("%s: injected header survived: %q", tc.name, line)
			}
		}
	}
}

func TestSanitizeFilename(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"report.pdf", "report.pdf"},
		{"../../../../../../../../etc/passwd", "passwd"},
		{"../../.ssh/authorized_keys", "authorized_keys"},
		{"/etc/shadow", "shadow"},
		{`..\..\windows\system32\evil.dll`, "evil.dll"},
		{"", "attachment"},
		{"..", "attachment"},
		{".", "attachment"},
		{"/", "attachment"},
		{"  spaced.txt  ", "spaced.txt"},
		{"invite.ics", "invite.ics"},
	} {
		if got := SanitizeFilename(tc.in); got != tc.want {
			t.Errorf("SanitizeFilename(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestExtractAttachments_RejectsPathTraversal proves the traversal cannot
// survive parsing, which is where every save and open path picks the name up.
func TestExtractAttachments_RejectsPathTraversal(t *testing.T) {
	raw := "From: attacker@example.com\r\n" +
		"Subject: invoice\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: multipart/mixed; boundary=BOUND\r\n" +
		"\r\n" +
		"--BOUND\r\n" +
		"Content-Type: application/octet-stream\r\n" +
		`Content-Disposition: attachment; filename="../../../../../../tmp/pwned"` + "\r\n" +
		"\r\n" +
		"payload\r\n" +
		"--BOUND--\r\n"

	atts, err := ExtractAttachments([]byte(raw))
	if err != nil {
		t.Fatalf("ExtractAttachments: %v", err)
	}
	if len(atts) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(atts))
	}
	name := atts[0].Filename
	if strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		t.Fatalf("filename still contains path components: %q", name)
	}
	// The joined path must stay inside the target directory.
	dir := t.TempDir()
	joined := filepath.Join(dir, name)
	if !strings.HasPrefix(filepath.Clean(joined), filepath.Clean(dir)+string(filepath.Separator)) {
		t.Errorf("joined path escapes target dir: %q not under %q", joined, dir)
	}
}

// TestSanitizeFilename_StripsEscapeSequences is a security-review finding:
// SanitizeFilename confined a Content-Disposition filename to a path-safe
// basename but did not strip terminal escape sequences or control
// characters, so a name like "evil\x1b[2Jname.txt" survived intact and was
// later shown verbatim in TUI attachment lists/badges -- able to manipulate
// the terminal display purely through the filename, independent of path
// traversal.
func TestSanitizeFilename_StripsEscapeSequences(t *testing.T) {
	in := "evil\x1b[2Jname.txt"
	got := SanitizeFilename(in)
	if strings.Contains(got, "\x1b") {
		t.Errorf("SanitizeFilename(%q) = %q, still contains an escape byte", in, got)
	}
	if got != "evilname.txt" {
		t.Errorf("SanitizeFilename(%q) = %q, want %q", in, got, "evilname.txt")
	}
}
