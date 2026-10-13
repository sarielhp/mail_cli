package email

import (
	"bytes"
	"io"
	"mime"
	"net/mail"
	"os"
	"regexp"
	"strings"
	"time"
)

// ParseReader builds an Email from a raw message, stripping terminal escape
// sequences from the subject and sender. It returns nil when the message cannot
// be parsed, and falls back to path's modification time when the Date header is
// missing or invalid.
func ParseReader(r io.Reader, id ID, path string) *Email {
	msg, err := mail.ReadMessage(r)
	if err != nil {
		return nil
	}
	dec := new(mime.WordDecoder)
	// Subject and From/FromRaw are shown verbatim by every display path --
	// the TUI's message list, its detail headers, and the CLI's `show`
	// command -- so they are stripped of terminal escape sequences here,
	// once, for all of them: a malicious Subject or sender name could
	// otherwise manipulate the terminal (hide text, move the cursor, alter
	// the window title, or worse on terminals that honour dangerous OSC
	// codes).
	subject := StripEscapeSequences(DecodeHeader(dec, msg.Header.Get("Subject")))
	if subject == "" {
		subject = "(No Subject)"
	}
	from := StripEscapeSequences(ParseEmailAddress(msg.Header.Get("From")))
	if from == "" {
		from = "(Unknown Sender)"
	}
	fromRaw := StripEscapeSequences(strings.TrimSpace(msg.Header.Get("From")))
	if fromRaw == "" {
		fromRaw = "(Unknown Sender)"
	}
	dateStr := msg.Header.Get("Date")
	var t time.Time
	if dateStr != "" {
		if parsed, err := mail.ParseDate(dateStr); err == nil {
			t = parsed
		}
	}
	if t.IsZero() && path != "" {
		if info, err := os.Stat(path); err == nil {
			t = info.ModTime()
		}
	}
	hasAttachment, hasICS := DetectAttachments(msg.Header, msg.Body)
	return &Email{
		ID:            id,
		Subject:       subject,
		FromEmail:     from,
		FromRaw:       fromRaw,
		EmailDate:     t,
		FormattedDate: FormatEmailDate(t),
		HasAttachment: hasAttachment,
		HasICS:        hasICS,
		MessageID:     HeaderID(strings.TrimSpace(msg.Header.Get("Message-ID"))),
		InReplyTo:     strings.TrimSpace(msg.Header.Get("In-Reply-To")),
		References:    strings.TrimSpace(msg.Header.Get("References")),
	}
}

// CompilePattern creates a reusable matcher predicate for subject filtering.
// For patterns without wildcards, it performs fast case-insensitive substring search.
// For patterns with wildcards, it compiles a case-insensitive regular expression once.
func CompilePattern(pattern string) (func(string) bool, error) {
	if !strings.ContainsAny(pattern, "*?[") {
		re, err := regexp.Compile(`(?i:` + regexp.QuoteMeta(pattern) + `)`)
		if err != nil {
			return nil, err
		}
		return re.MatchString, nil
	}

	var buf strings.Builder
	buf.Grow(2*len(pattern) + len(`(?i)^$`))
	buf.WriteString(`(?i)^`)
	for _, r := range pattern {
		switch r {
		case '*':
			buf.WriteString(`.*`)
		case '?':
			buf.WriteByte('.')
		default:
			buf.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	buf.WriteByte('$')

	re, err := regexp.Compile(buf.String())
	if err != nil {
		return nil, err
	}
	return re.MatchString, nil
}

// ExtractDateOrNow returns the message's own Date header, falling back to the
// current time when the message has none or it cannot be parsed.
//
// The message cache lays files out under messages/YYYY/MM/DD taken from the
// date it is handed at store time, so this is what decides where a message
// lives on disk. Callers historically passed time.Now(), which filed a message
// by when it happened to be downloaded; passing the message's own date instead
// groups a mailbox by when its mail was actually sent. The fallback matters
// because a message with a broken Date header still needs a stable home.
func ExtractDateOrNow(raw []byte) time.Time {
	parsed, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return time.Now()
	}
	header := strings.TrimSpace(parsed.Header.Get("Date"))
	if header == "" {
		return time.Now()
	}
	t, err := mail.ParseDate(header)
	if err != nil || t.IsZero() {
		return time.Now()
	}
	return t
}
