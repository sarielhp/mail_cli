package email

import (
	"bytes"
	"net/mail"
	"strings"
	"testing"
	"time"
)

// The entity is what a PGP signature covers, and RFC 3156 requires it 7-bit
// safe: a body with non-ASCII text, trailing whitespace or a long line is sent
// quoted-printable, and reading it back gives the text that was written.
func TestBuildEntityIsSevenBitSafe(t *testing.T) {
	body := "Grüße aus Köln \nFrom the start of a line\n" + strings.Repeat("x", 120) + "\n"
	for _, atts := range [][]Attachment{nil, {{Filename: "a.txt", ContentType: "text/plain", Data: []byte("a")}}} {
		comp := &ComposeMessage{From: "a@example.com", To: "b@example.com", Subject: "s", Body: body, Attachments: atts}
		built, err := comp.Build()
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(built.Entity, []byte("8bit")) || !bytes.Contains(built.Entity, []byte("quoted-printable")) {
			t.Errorf("entity = %q, want a quoted-printable text part", built.Entity)
		}
		for i, b := range built.Entity {
			if b > 127 {
				t.Fatalf("entity byte %d is %#x, not 7-bit", i, b)
			}
		}
		for _, line := range strings.Split(string(built.Entity), "\r\n") {
			if len(line) > 998 || strings.HasSuffix(line, " ") {
				t.Fatalf("entity line %q would not survive a relay", line)
			}
		}
		parsed, err := ParseRFC822ToCompose(built.Bytes())
		if err != nil {
			t.Fatal(err)
		}
		if parsed.Body != body {
			t.Errorf("body read back = %q, want %q", parsed.Body, body)
		}
	}
}

func TestBuildPlainASCIIStaysUnencoded(t *testing.T) {
	built, err := (&ComposeMessage{From: "a@example.com", Body: "short plain text\n"}).Build()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(built.Entity, []byte("Content-Transfer-Encoding: 7bit\r\n\r\nshort plain text\r\n")) {
		t.Errorf("entity = %q, want plain ASCII left as 7bit", built.Entity)
	}
}

func TestBuildAndParseRFC822_Plain(t *testing.T) {
	comp := &ComposeMessage{
		From:       "Alice <alice@example.com>",
		To:         "Bob <bob@example.com>",
		Cc:         "Charlie <charlie@example.com>",
		Subject:    "Hello World",
		InReplyTo:  "<orig-123@example.com>",
		References: "<orig-123@example.com>",
		Body:       "This is the message body.\nSecond line.\n",
		Date:       time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC),
		MessageID:  "<test-msg-id@example.com>",
	}

	raw, err := comp.BuildRFC822()
	if err != nil {
		t.Fatalf("BuildRFC822 failed: %v", err)
	}

	parsed, err := ParseRFC822ToCompose(raw)
	if err != nil {
		t.Fatalf("ParseRFC822ToCompose failed: %v", err)
	}

	if parsed.From != comp.From {
		t.Errorf("From = %q, want %q", parsed.From, comp.From)
	}
	if parsed.To != comp.To {
		t.Errorf("To = %q, want %q", parsed.To, comp.To)
	}
	if parsed.Cc != comp.Cc {
		t.Errorf("Cc = %q, want %q", parsed.Cc, comp.Cc)
	}
	if parsed.Subject != comp.Subject {
		t.Errorf("Subject = %q, want %q", parsed.Subject, comp.Subject)
	}
	if parsed.InReplyTo != comp.InReplyTo {
		t.Errorf("InReplyTo = %q, want %q", parsed.InReplyTo, comp.InReplyTo)
	}
	if strings.TrimSpace(parsed.Body) != strings.TrimSpace(comp.Body) {
		t.Errorf("Body = %q, want %q", parsed.Body, comp.Body)
	}
	if len(parsed.Attachments) != 0 {
		t.Errorf("Attachments count = %d, want 0", len(parsed.Attachments))
	}
}

func TestBuildAndParseRFC822_WithAttachments(t *testing.T) {
	comp := &ComposeMessage{
		From:    "Sender <sender@example.com>",
		To:      "Recipient <recipient@example.com>",
		Subject: "Email with Files",
		Body:    "Please find attached the report.\n",
		Attachments: []Attachment{
			{
				Filename:    "report.pdf",
				ContentType: "application/pdf",
				Data:        []byte("%PDF-1.4 mock pdf content"),
			},
			{
				Filename:    "notes.txt",
				ContentType: "text/plain",
				Data:        []byte("Meeting notes go here"),
			},
		},
	}

	raw, err := comp.BuildRFC822()
	if err != nil {
		t.Fatalf("BuildRFC822 with attachments failed: %v", err)
	}

	parsed, err := ParseRFC822ToCompose(raw)
	if err != nil {
		t.Fatalf("ParseRFC822ToCompose failed: %v", err)
	}

	if len(parsed.Attachments) != 2 {
		t.Fatalf("Attachments count = %d, want 2", len(parsed.Attachments))
	}

	if parsed.Attachments[0].Filename != "report.pdf" {
		t.Errorf("Attachment 0 filename = %q, want report.pdf", parsed.Attachments[0].Filename)
	}
	if !bytes.Equal(parsed.Attachments[0].Data, []byte("%PDF-1.4 mock pdf content")) {
		t.Errorf("Attachment 0 data mismatch")
	}

	if parsed.Attachments[1].Filename != "notes.txt" {
		t.Errorf("Attachment 1 filename = %q, want notes.txt", parsed.Attachments[1].Filename)
	}
	if string(parsed.Attachments[1].Data) != "Meeting notes go here" {
		t.Errorf("Attachment 1 data mismatch: %q", string(parsed.Attachments[1].Data))
	}
}

func TestBuildRFC822_NonASCIIHeaders(t *testing.T) {
	comp := &ComposeMessage{
		From:    "Sender <sender@example.com>",
		To:      "Bob <bob@example.com>",
		Subject: "שלום עולם — Café & Crème",
		Body:    "Testing UTF-8 subject encoding",
	}

	raw, err := comp.BuildRFC822()
	if err != nil {
		t.Fatalf("BuildRFC822 failed: %v", err)
	}

	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("ReadMessage failed: %v", err)
	}

	rawSubj := msg.Header.Get("Subject")
	if !strings.Contains(rawSubj, "=?utf-8?q?") && !strings.Contains(rawSubj, "=?UTF-8?q?") {
		t.Errorf("expected Q-encoded subject, got %q", rawSubj)
	}

	parsed, err := ParseRFC822ToCompose(raw)
	if err != nil {
		t.Fatalf("ParseRFC822ToCompose failed: %v", err)
	}

	if parsed.Subject != comp.Subject {
		t.Errorf("Parsed subject = %q, want %q", parsed.Subject, comp.Subject)
	}
}

func TestMessageIDForSender(t *testing.T) {
	cases := map[string]string{
		"":                     "@mailcli.local>",
		"Name <a@example.org>": "@example.org>",
		"b@host.test":          "@host.test>",
		"not an address":       "@mailcli.local>",
	}
	for from, suffix := range cases {
		id := MessageIDForSender(from)
		if !strings.HasPrefix(id, "<") || !strings.HasSuffix(id, suffix) {
			t.Errorf("MessageIDForSender(%q) = %q, want suffix %q", from, id, suffix)
		}
	}
	if first, second := MessageIDForSender("a@b.c"), MessageIDForSender("a@b.c"); first == second {
		t.Error("two calls returned the same Message-ID")
	}
}
