package email

import (
	"bytes"
	"strings"
	"testing"
)

func TestStripAttachmentsFromEML_SingleAttachment(t *testing.T) {
	raw := "From: alice@example.com\r\n" +
		"To: bob@example.com\r\n" +
		"Subject: Test Message\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: multipart/mixed; boundary=\"_BOUNDARY_123_\"\r\n" +
		"\r\n" +
		"--_BOUNDARY_123_\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\n" +
		"\r\n" +
		"Here is the email text.\r\n" +
		"--_BOUNDARY_123_\r\n" +
		"Content-Type: application/pdf; name=\"invoice.pdf\"\r\n" +
		"Content-Disposition: attachment; filename=\"invoice.pdf\"\r\n" +
		"Content-Transfer-Encoding: base64\r\n" +
		"\r\n" +
		"c29tZS1wZGYtYnl0ZXM=\r\n" +
		"--_BOUNDARY_123_--\r\n"

	// Verify attachment initially extracted
	initialAtts, err := ExtractAttachments([]byte(raw))
	if err != nil || len(initialAtts) != 1 || initialAtts[0].Filename != "invoice.pdf" {
		t.Fatalf("expected 1 initial attachment, got %v, err=%v", initialAtts, err)
	}

	strippedEML, stripped, err := StripAttachmentsFromEML([]byte(raw), []string{"invoice.pdf"})
	if err != nil {
		t.Fatalf("unexpected error stripping: %v", err)
	}
	if !stripped {
		t.Fatalf("expected stripped=true")
	}

	// Verify modified EML has no attachments anymore
	postAtts, err := ExtractAttachments(strippedEML)
	if err != nil {
		t.Fatalf("error extracting attachments after strip: %v", err)
	}
	if len(postAtts) != 0 {
		t.Fatalf("expected 0 attachments after strip, got %v", postAtts)
	}

	// Verify text marker is present in new EML
	if !bytes.Contains(strippedEML, []byte("[Attachment deleted: invoice.pdf]")) {
		t.Fatalf("expected '[Attachment deleted: invoice.pdf]' in stripped EML, got:\n%s", string(strippedEML))
	}

	// Verify original body text preserved
	if !bytes.Contains(strippedEML, []byte("Here is the email text.")) {
		t.Fatalf("expected original body preserved, got:\n%s", string(strippedEML))
	}
}

func TestStripAttachmentsFromEML_NoMatch(t *testing.T) {
	raw := "From: alice@example.com\r\n" +
		"Subject: Non-multipart\r\n" +
		"\r\n" +
		"Hello world\r\n"

	out, stripped, err := StripAttachmentsFromEML([]byte(raw), []string{"unknown.pdf"})
	if err != nil || stripped {
		t.Fatalf("expected no strip, got stripped=%v, err=%v", stripped, err)
	}
	if string(out) != raw {
		t.Fatalf("expected unchanged output")
	}
}

func TestStripAttachmentsFromEML_CaseInsensitive(t *testing.T) {
	raw := "From: alice@example.com\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: multipart/mixed; boundary=\"BND\"\r\n" +
		"\r\n" +
		"--BND\r\n" +
		"Content-Type: text/plain\r\n" +
		"\r\n" +
		"Body\r\n" +
		"--BND\r\n" +
		"Content-Disposition: attachment; filename=\"Report.PDF\"\r\n" +
		"\r\n" +
		"data\r\n" +
		"--BND--\r\n"

	out, stripped, err := StripAttachmentsFromEML([]byte(raw), []string{"report.pdf"})
	if err != nil || !stripped {
		t.Fatalf("expected stripped=true, got err=%v", err)
	}
	if !strings.Contains(string(out), "[Attachment deleted: Report.PDF]") {
		t.Fatalf("expected deleted marker, got:\n%s", string(out))
	}
}
