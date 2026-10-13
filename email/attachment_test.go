package email

import (
	"io"
	"net/mail"
	"strings"
	"testing"
)

// messageFlagCase is a raw message plus the answer a header/body predicate
// such as hasAttachmentInMsg must give for it.
type messageFlagCase struct {
	name    string
	headers string
	body    string
	want    bool
}

// assertMessageFlag parses tc's headers and body into a message and checks
// that fn reports tc.want.
func assertMessageFlag(t *testing.T, tc messageFlagCase, fn func(mail.Header, io.Reader) bool) {
	t.Helper()
	msg, err := mail.ReadMessage(strings.NewReader(tc.headers + "\r\n" + tc.body))
	if err != nil {
		t.Fatalf("failed to parse test message: %v", err)
	}
	if got := fn(msg.Header, msg.Body); got != tc.want {
		t.Errorf("message flag = %v, want %v", got, tc.want)
	}
}

func TestHasICSAttachmentInMsg(t *testing.T) {
	tests := []messageFlagCase{
		{
			name:    "No attachments, plain text",
			headers: "Content-Type: text/plain; charset=UTF-8\r\n",
			body:    "Hello world",
			want:    false,
		},
		{
			name:    "Direct text/calendar content type",
			headers: "Content-Type: text/calendar; method=REQUEST; charset=UTF-8\r\n",
			body:    "BEGIN:VCALENDAR\r\nEND:VCALENDAR",
			want:    true,
		},
		{
			name:    "Direct application/ics content type",
			headers: "Content-Type: application/ics\r\n",
			body:    "BEGIN:VCALENDAR\r\nEND:VCALENDAR",
			want:    true,
		},
		{
			name:    "Content-Type name parameter has .ics extension",
			headers: "Content-Type: application/octet-stream; name=\"invite.ics\"\r\n",
			body:    "some ics binary/text",
			want:    true,
		},
		{
			name:    "Content-Disposition filename has .ics extension",
			headers: "Content-Type: application/octet-stream\r\nContent-Disposition: attachment; filename=\"meeting.ICS\"\r\n",
			body:    "some ics binary/text",
			want:    true,
		},
		{
			name:    "Multipart containing text/calendar",
			headers: "Content-Type: multipart/alternative; boundary=\"boundary123\"\r\n",
			body: "--boundary123\r\n" +
				"Content-Type: text/plain\r\n\r\n" +
				"Plain text\r\n" +
				"--boundary123\r\n" +
				"Content-Type: text/calendar; method=PUBLISH\r\n\r\n" +
				"BEGIN:VCALENDAR\r\nEND:VCALENDAR\r\n" +
				"--boundary123--\r\n",
			want: true,
		},
		{
			name:    "Multipart containing ics attachment file",
			headers: "Content-Type: multipart/mixed; boundary=\"boundary456\"\r\n",
			body: "--boundary456\r\n" +
				"Content-Type: text/plain\r\n\r\n" +
				"Hello\r\n" +
				"--boundary456\r\n" +
				"Content-Type: application/octet-stream; name=\"meeting.ics\"\r\n" +
				"Content-Disposition: attachment; filename=\"meeting.ics\"\r\n\r\n" +
				"BEGIN:VCALENDAR\r\nEND:VCALENDAR\r\n" +
				"--boundary456--\r\n",
			want: true,
		},
		{
			name:    "Multipart without ics",
			headers: "Content-Type: multipart/mixed; boundary=\"boundary789\"\r\n",
			body: "--boundary789\r\n" +
				"Content-Type: text/plain\r\n\r\n" +
				"Hello\r\n" +
				"--boundary789\r\n" +
				"Content-Type: image/png; name=\"logo.png\"\r\n" +
				"Content-Disposition: attachment; filename=\"logo.png\"\r\n\r\n" +
				"imagedata\r\n" +
				"--boundary789--\r\n",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertMessageFlag(t, tt, hasICSAttachmentInMsg)
		})
	}
}

func TestExtractAttachments(t *testing.T) {
	rawMsg := "Content-Type: multipart/mixed; boundary=\"boundary456\"\r\n\r\n" +
		"--boundary456\r\n" +
		"Content-Type: text/plain\r\n\r\n" +
		"Hello\r\n" +
		"--boundary456\r\n" +
		"Content-Type: application/octet-stream; name=\"meeting.ics\"\r\n" +
		"Content-Disposition: attachment; filename=\"meeting.ics\"\r\n\r\n" +
		"BEGIN:VCALENDAR\r\nEND:VCALENDAR\r\n" +
		"--boundary456\r\n" +
		"Content-Type: image/png\r\n" +
		"Content-Disposition: attachment; filename=\"logo.png\"\r\n" +
		"Content-Transfer-Encoding: base64\r\n\r\n" +
		"dGVzdGRhdGE=\r\n" +
		"--boundary456--\r\n"

	atts, err := ExtractAttachments([]byte(rawMsg))
	if err != nil {
		t.Fatalf("ExtractAttachments failed: %v", err)
	}

	if len(atts) != 2 {
		t.Fatalf("Expected 2 attachments, got %d", len(atts))
	}

	if atts[0].Filename != "meeting.ics" {
		t.Errorf("Expected first filename 'meeting.ics', got %q", atts[0].Filename)
	}
	if string(atts[0].Data) != "BEGIN:VCALENDAR\r\nEND:VCALENDAR" {
		t.Errorf("Expected first data 'BEGIN:VCALENDAR\r\nEND:VCALENDAR', got %q", string(atts[0].Data))
	}

	if atts[1].Filename != "logo.png" {
		t.Errorf("Expected second filename 'logo.png', got %q", atts[1].Filename)
	}
	if string(atts[1].Data) != "testdata" {
		t.Errorf("Expected second data 'testdata', got %q", string(atts[1].Data))
	}
}

func TestHasAttachmentInMsg(t *testing.T) {
	tests := []messageFlagCase{
		{
			name:    "Plain text, no attachment",
			headers: "Content-Type: text/plain; charset=UTF-8\r\n",
			body:    "Hello world",
			want:    false,
		},
		{
			name:    "Attachment via Content-Disposition",
			headers: "Content-Type: application/pdf\r\nContent-Disposition: attachment; filename=\"doc.pdf\"\r\n",
			body:    "pdf data",
			want:    true,
		},
		{
			name:    "Attachment via Content-Type name param",
			headers: "Content-Type: application/zip; name=\"archive.zip\"\r\n",
			body:    "zip data",
			want:    true,
		},
		{
			name:    "Multipart containing attachment",
			headers: "Content-Type: multipart/mixed; boundary=\"bound1\"\r\n",
			body: "--bound1\r\n" +
				"Content-Type: text/plain\r\n\r\n" +
				"Message body\r\n" +
				"--bound1\r\n" +
				"Content-Type: image/png\r\n" +
				"Content-Disposition: attachment; filename=\"pic.png\"\r\n\r\n" +
				"img data\r\n" +
				"--bound1--\r\n",
			want: true,
		},
		{
			name:    "Inline attachment with filename via Content-Disposition",
			headers: "Content-Type: application/pdf\r\nContent-Disposition: inline; filename=\"invoice.pdf\"\r\n",
			body:    "pdf data",
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertMessageFlag(t, tt, hasAttachmentInMsg)
		})
	}
}

func TestDetectAttachments(t *testing.T) {
	tests := []struct {
		name       string
		headers    string
		body       string
		wantHasAtt bool
		wantHasICS bool
	}{
		{
			name:    "Teams meeting invite (multipart/alternative with text/calendar)",
			headers: "Content-Type: multipart/alternative; boundary=\"_000_teams_\"\r\n",
			body: "--_000_teams_\r\n" +
				"Content-Type: text/plain; charset=\"iso-8859-1\"\r\n\r\n" +
				"Join Teams meeting\r\n" +
				"--_000_teams_\r\n" +
				"Content-Type: text/html; charset=\"iso-8859-1\"\r\n\r\n" +
				"<html>Join</html>\r\n" +
				"--_000_teams_\r\n" +
				"Content-Type: text/calendar; charset=\"utf-8\"; method=REQUEST\r\n\r\n" +
				"BEGIN:VCALENDAR\r\nSUMMARY:Sam Research Meeting\r\nEND:VCALENDAR\r\n" +
				"--_000_teams_--\r\n",
			wantHasAtt: false,
			wantHasICS: true,
		},
		{
			name:    "Meeting invite with regular PDF attachment",
			headers: "Content-Type: multipart/mixed; boundary=\"_mixed_1_\"\r\n",
			body: "--_mixed_1_\r\n" +
				"Content-Type: text/plain\r\n\r\n" +
				"Meeting info\r\n" +
				"--_mixed_1_\r\n" +
				"Content-Type: application/octet-stream; name=\"invite.ics\"\r\n" +
				"Content-Disposition: attachment; filename=\"invite.ics\"\r\n\r\n" +
				"BEGIN:VCALENDAR\r\nEND:VCALENDAR\r\n" +
				"--_mixed_1_\r\n" +
				"Content-Type: application/pdf; name=\"slides.pdf\"\r\n" +
				"Content-Disposition: attachment; filename=\"slides.pdf\"\r\n\r\n" +
				"pdf data\r\n" +
				"--_mixed_1_--\r\n",
			wantHasAtt: true,
			wantHasICS: true,
		},
		{
			name:    "Only PDF attachment",
			headers: "Content-Type: multipart/mixed; boundary=\"_mixed_2_\"\r\n",
			body: "--_mixed_2_\r\n" +
				"Content-Type: text/plain\r\n\r\n" +
				"Please see attachment\r\n" +
				"--_mixed_2_\r\n" +
				"Content-Type: application/pdf; name=\"report.pdf\"\r\n" +
				"Content-Disposition: attachment; filename=\"report.pdf\"\r\n\r\n" +
				"pdf data\r\n" +
				"--_mixed_2_--\r\n",
			wantHasAtt: true,
			wantHasICS: false,
		},
		{
			name:    "Inline PDF attachment with filename",
			headers: "Content-Type: multipart/mixed; boundary=\"_mixed_3_\"\r\n",
			body: "--_mixed_3_\r\n" +
				"Content-Type: text/plain\r\n\r\n" +
				"Please see invoice\r\n" +
				"--_mixed_3_\r\n" +
				"Content-Type: application/pdf\r\n" +
				"Content-Disposition: inline; filename=\"invoice.pdf\"\r\n\r\n" +
				"pdf data\r\n" +
				"--_mixed_3_--\r\n",
			wantHasAtt: true,
			wantHasICS: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := tt.headers + "\r\n" + tt.body
			msg, err := mail.ReadMessage(strings.NewReader(raw))
			if err != nil {
				t.Fatalf("failed to parse message: %v", err)
			}
			hasAtt, hasICS := DetectAttachments(msg.Header, msg.Body)
			if hasAtt != tt.wantHasAtt {
				t.Errorf("DetectAttachments() hasAtt = %v, want %v", hasAtt, tt.wantHasAtt)
			}
			if hasICS != tt.wantHasICS {
				t.Errorf("DetectAttachments() hasICS = %v, want %v", hasICS, tt.wantHasICS)
			}
		})
	}
}

func TestParseReader_TeamsMeetingInvite(t *testing.T) {
	raw := "From: \"Sam Smith\" <samuel@example.edu>\r\n" +
		"To: Alex Taylor <alex@example.org>\r\n" +
		"Subject: Team Research Meeting\r\n" +
		"Date: Wed, 26 Aug 2026 20:52:07 +0000\r\n" +
		"Content-Type: multipart/alternative; boundary=\"_boundary_teams_\"\r\n" +
		"MIME-Version: 1.0\r\n\r\n" +
		"--_boundary_teams_\r\n" +
		"Content-Type: text/plain; charset=\"iso-8859-1\"\r\n\r\n" +
		"Teams meeting link\r\n" +
		"--_boundary_teams_\r\n" +
		"Content-Type: text/html; charset=\"iso-8859-1\"\r\n\r\n" +
		"<html>Teams meeting link</html>\r\n" +
		"--_boundary_teams_\r\n" +
		"Content-Type: text/calendar; charset=\"utf-8\"; method=REQUEST\r\n" +
		"Content-Transfer-Encoding: base64\r\n\r\n" +
		"QkVHSU46VkNBTEVOREFSDQpTVU1NQVJZOlRlYW0gUmVzZWFyY2ggTWVldGluZw0KRU5EOlZDQUxFTkRBUg0K\r\n" +
		"--_boundary_teams_--\r\n"

	eml := ParseReader(strings.NewReader(raw), "test1234", "")
	if eml == nil {
		t.Fatal("expected non-nil Email from ParseReader")
	}

	if eml.Subject != "Team Research Meeting" {
		t.Errorf("expected Subject 'Team Research Meeting', got %q", eml.Subject)
	}
	if !eml.HasICS {
		t.Errorf("expected HasICS = true, got %v", eml.HasICS)
	}
	if eml.HasAttachment {
		t.Errorf("expected HasAttachment = false (only ICS present), got %v", eml.HasAttachment)
	}
}

// TestExtractAttachments_MalformedBase64IsNotSilentlyPresentedAsDecoded is a
// deep-review finding: decodeAttachmentData used to fall back to returning
// the raw, still-encoded bytes with a nil error when both a strict and a
// lenient base64 decode failed, so a corrupted attachment looked like it had
// decoded successfully while its Data was actually the undecoded base64
// text, not real content -- saving or opening it produced garbage with no
// indication anything had gone wrong.
//
// A message with two attachments, one well-formed and one with base64 that
// cannot be decoded by either attempt, must still return the good one; the
// bad one must not appear with corrupted content standing in for real data.
func TestExtractAttachments_MalformedBase64IsNotSilentlyPresentedAsDecoded(t *testing.T) {
	rawMsg := "Content-Type: multipart/mixed; boundary=\"boundary789\"\r\n\r\n" +
		"--boundary789\r\n" +
		"Content-Type: application/octet-stream; name=\"good.bin\"\r\n" +
		"Content-Disposition: attachment; filename=\"good.bin\"\r\n" +
		"Content-Transfer-Encoding: base64\r\n\r\n" +
		"dGVzdGRhdGE=\r\n" +
		"--boundary789\r\n" +
		"Content-Type: application/octet-stream; name=\"corrupt.bin\"\r\n" +
		"Content-Disposition: attachment; filename=\"corrupt.bin\"\r\n" +
		"Content-Transfer-Encoding: base64\r\n\r\n" +
		"!!!not-valid-base64!!!\r\n" +
		"--boundary789--\r\n"

	atts, err := ExtractAttachments([]byte(rawMsg))
	if err != nil {
		t.Fatalf("ExtractAttachments failed: %v", err)
	}

	for _, att := range atts {
		if att.Filename == "corrupt.bin" {
			t.Fatalf("corrupt.bin should not appear with corrupted content standing in for real data, got Data=%q", att.Data)
		}
	}

	found := false
	for _, att := range atts {
		if att.Filename == "good.bin" {
			found = true
			if string(att.Data) != "testdata" {
				t.Errorf("good.bin content corrupted: got %q, want %q", att.Data, "testdata")
			}
		}
	}
	if !found {
		t.Errorf("expected good.bin to still be extracted despite corrupt.bin failing, got %+v", atts)
	}
}
