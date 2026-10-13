package email

import (
	"mime"
	"sync"
	"testing"
)

func TestDecodeHeader(t *testing.T) {
	dec := new(mime.WordDecoder)

	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    "=?Windows-1252?Q?Welcome_to_MakerLab_Camp!_Minecraft_+_3D_Printing_(Jul_1?= =?Wi",
			expected: "Welcome to MakerLab Camp! Minecraft + 3D Printing (Jul 1 […]",
		},
		{
			// A trailing encoded word cut off mid-header cannot be decoded. It
			// used to vanish, so the subject came out shorter than it was with
			// nothing to say so; the marker says so.
			input:    "Hello =?UTF-8?Q?world?= =?UTF-8?Q?!?= and =?Wi",
			expected: "Hello world! and […]",
		},
		{
			input:    "=?UTF-8?B?SGVsbG8=?= =?UTF-8?B?IFdvcmxkIQ==?=",
			expected: "Hello World!",
		},
		{
			input:    "Plain text",
			expected: "Plain text",
		},
		{
			input:    "=?Unknown-Charset-1234?Q?Test_Subject?=",
			expected: "Test Subject",
		},
	}

	for _, tc := range tests {
		got := DecodeHeader(dec, tc.input)
		if got != tc.expected {
			t.Errorf("DecodeHeader(dec, %q) = %q; want %q", tc.input, got, tc.expected)
		}
	}

	// Test nil decoder pointer
	gotNilDec := DecodeHeader(nil, "=?Unknown-Charset-1234?Q?Test?=")
	if gotNilDec != "Test" {
		t.Errorf("DecodeHeader(nil, ...) = %q; want %q", gotNilDec, "Test")
	}
}

func TestDecodeHeader_ConcurrentSafety(t *testing.T) {
	dec := &mime.WordDecoder{} // CharsetReader is nil
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res := DecodeHeader(dec, "=?UTF-8?B?SGVsbG8=?= =?UTF-8?B?IFdvcmxkIQ==?=")
			if res != "Hello World!" {
				t.Errorf("got %q, want 'Hello World!'", res)
			}
		}()
	}
	wg.Wait()
}

// TestDecodeHeader_StripsEscapeSequences is a security-review finding:
// DecodeHeader is the RFC2047 decode path used directly (not through
// email.ParseReader) by show/show.go's CLI `show` command, cli/split.go,
// spam/spam.go and spam/unspam.go, scan/scan.go and scan/scanner.go,
// email/compose.go's reply/forward drafts, and every backend's
// spam/rule-notification building (backend/gmail, backend/jmap,
// backend/outlook) -- none of which strip escape sequences from the
// decoded value themselves. A malicious Subject/From header could
// manipulate the terminal through any of these paths.
func TestDecodeHeader_StripsEscapeSequences(t *testing.T) {
	dec := new(mime.WordDecoder)
	got := DecodeHeader(dec, "Evil\x1b[2JSubject")
	if got != "EvilSubject" {
		t.Errorf("DecodeHeader(dec, %q) = %q, want %q", "Evil\x1b[2JSubject", got, "EvilSubject")
	}
}

func TestStripSubjectPrefix(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Re: Hello World", "Hello World"},
		{"RE: FW: Important", "Important"},
		{"fwd: reply: Meeting", "Meeting"},
		{"No Prefix Here", "No Prefix Here"},
		{"  re:   Trim Spaces  ", "Trim Spaces"},
	}

	for _, tc := range tests {
		got := StripSubjectPrefix(tc.input)
		if got != tc.expected {
			t.Errorf("StripSubjectPrefix(%q) = %q; want %q", tc.input, got, tc.expected)
		}
	}
}

func TestIsValidEmail(t *testing.T) {
	validEmails := []string{
		"user@example.com",
		"alice.bob+tag@sub.domain.org",
		"support@mail-service.co.uk",
		"John Doe <john@example.com>",
	}

	for _, e := range validEmails {
		if !IsValidEmail(e) {
			t.Errorf("expected IsValidEmail(%q) to be true", e)
		}
		if !IsValidEmailOrDomain(e) {
			t.Errorf("expected IsValidEmailOrDomain(%q) to be true", e)
		}
	}

	validDomains := []string{
		"@example.com",
		"@sub.domain.org",
		"@github.com",
	}
	for _, d := range validDomains {
		if !IsValidEmailOrDomain(d) {
			t.Errorf("expected IsValidEmailOrDomain(%q) to be true", d)
		}
	}

	invalid := []string{
		"",
		"not-an-email",
		"user@",
		"@nodot",
		"user@.com",
		"user@domain.",
		"user name@domain.com",
		"12345",
	}
	for _, inv := range invalid {
		if IsValidEmail(inv) {
			t.Errorf("expected IsValidEmail(%q) to be false", inv)
		}
		if IsValidEmailOrDomain(inv) {
			t.Errorf("expected IsValidEmailOrDomain(%q) to be false", inv)
		}
	}
}
