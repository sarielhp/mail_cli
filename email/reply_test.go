package email

import (
	"net/mail"
	"strings"
	"testing"
)

func TestFormatReplySubject(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", "Re: "},
		{"   ", "Re: "},
		{"Hello world", "Re: Hello world"},
		{"Re: Hello world", "Re: Hello world"},
		{"re: Hello world", "re: Hello world"},
		{"RE: Hello world", "RE: Hello world"},
		{"  Re: Already replied  ", "Re: Already replied"},
	}

	for i, tc := range tests {
		got := formatReplySubject(tc.input)
		if got == tc.want {
			continue
		}
		t.Errorf("case %d: formatReplySubject(%q) = %q; want %q", i, tc.input, got, tc.want)
	}
}

func TestBuildGroupReplyCC(t *testing.T) {
	raw := "From: Alice <alice@example.com>\r\n" +
		"To: Bob <bob@example.com>, Me <me@example.com>\r\n" +
		"Cc: Charlie <charlie@example.com>, Alice <alice@example.com>, David <david@example.com>\r\n" +
		"\r\nBody\r\n"

	msg, err := mail.ReadMessage(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("failed to parse test message: %v", err)
	}

	cc := BuildGroupReplyCC(msg, "alice@example.com", "me@example.com")
	expected := []string{"\"Bob\" <bob@example.com>", "\"Charlie\" <charlie@example.com>", "\"David\" <david@example.com>"}
	if len(cc) != len(expected) {
		t.Fatalf("BuildGroupReplyCC returned %d addresses (%v); want %d (%v)", len(cc), cc, len(expected), expected)
	}
	for i, want := range expected {
		if cc[i] != want {
			t.Errorf("cc[%d] = %q; want %q", i, cc[i], want)
		}
	}
}

func TestBuildReplyDetails_Single(t *testing.T) {
	raw := "From: Alice <alice@example.com>\r\n" +
		"Subject: Important Update\r\n" +
		"Message-ID: <msg123@example.com>\r\n" +
		"Date: Sun, 20 Sep 2026 12:00:00 +0000\r\n" +
		"Content-Type: text/plain\r\n" +
		"\r\n" +
		"Here is the plan for tomorrow.\nLet me know what you think.\n"

	msg, err := mail.ReadMessage(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("failed to parse message: %v", err)
	}

	details := BuildReplyDetails(msg, "me@example.com", false)

	if details.To != "Alice <alice@example.com>" {
		t.Errorf("To = %q; want Alice <alice@example.com>", details.To)
	}
	if len(details.Cc) != 0 {
		t.Errorf("Cc length = %d; want 0", len(details.Cc))
	}
	if details.Subject != "Re: Important Update" {
		t.Errorf("Subject = %q; want Re: Important Update", details.Subject)
	}
	if details.InReplyTo != "<msg123@example.com>" {
		t.Errorf("InReplyTo = %q; want <msg123@example.com>", details.InReplyTo)
	}
	if details.References != "<msg123@example.com>" {
		t.Errorf("References = %q; want <msg123@example.com>", details.References)
	}
	if !strings.Contains(details.QuotedBody, "On Sun, 20 Sep 2026 12:00:00 +0000, Alice <alice@example.com> wrote:") {
		t.Errorf("QuotedBody missing attribution header: %q", details.QuotedBody)
	}
	if !strings.Contains(details.QuotedBody, "> Here is the plan for tomorrow.") {
		t.Errorf("QuotedBody missing quoted line: %q", details.QuotedBody)
	}
}

func TestBuildReplyDetails_ReplyToAndReferences(t *testing.T) {
	raw := "From: Alice <alice@example.com>\r\n" +
		"Reply-To: Support Team <support@example.com>\r\n" +
		"Subject: Re: Existing Discussion\r\n" +
		"Message-ID: <msg456@example.com>\r\n" +
		"References: <root123@example.com>\r\n" +
		"Content-Type: text/plain\r\n" +
		"\r\n" +
		"Please reply to this ticket."

	msg, err := mail.ReadMessage(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("failed to parse message: %v", err)
	}

	details := BuildReplyDetails(msg, "me@example.com", false)

	if details.To != "Support Team <support@example.com>" {
		t.Errorf("To = %q; want Support Team <support@example.com>", details.To)
	}
	if details.Subject != "Re: Existing Discussion" {
		t.Errorf("Subject = %q; want Re: Existing Discussion", details.Subject)
	}
	if details.InReplyTo != "<msg456@example.com>" {
		t.Errorf("InReplyTo = %q; want <msg456@example.com>", details.InReplyTo)
	}
	if details.References != "<root123@example.com> <msg456@example.com>" {
		t.Errorf("References = %q; want '<root123@example.com> <msg456@example.com>'", details.References)
	}
}

func TestBuildReplyDetails_Nil(t *testing.T) {
	details := BuildReplyDetails(nil, "me@example.com", false)
	if details.To != "" || details.Subject != "" {
		t.Errorf("BuildReplyDetails(nil) returned non-empty details: %+v", details)
	}
}
