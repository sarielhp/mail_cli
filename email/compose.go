package email

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"net/textproto"
	"path/filepath"
	"strings"
	"time"
)

// ComposeMessage holds all the parts of an email being composed or saved as a draft.
type ComposeMessage struct {
	From        string
	To          string
	Cc          string
	Bcc         string
	Subject     string
	InReplyTo   string
	References  string
	Body        string
	Attachments []Attachment
	Date        time.Time
	MessageID   string
}

// GenerateMessageID creates a compliant Message-ID header value.
func GenerateMessageID(domain string) string {
	if domain == "" {
		domain = "mailcli.local"
	}
	domain = strings.TrimPrefix(domain, "@")
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return fmt.Sprintf("<%d.%s@%s>", time.Now().UnixNano(), hex.EncodeToString(b), domain)
}

// MessageIDForSender generates a Message-ID under the sender's domain. A
// compose session calls it once and keeps the result, so every draft copy and
// the message finally sent share one ID and can be told to be the same message.
func MessageIDForSender(from string) string {
	domain := "mailcli.local"
	if from != "" {
		if addr, err := mail.ParseAddress(from); err == nil && addr != nil {
			parts := strings.Split(addr.Address, "@")
			if len(parts) == 2 && parts[1] != "" {
				domain = parts[1]
			}
		}
	}
	return GenerateMessageID(domain)
}

// SanitizeHeaderValue strips CR and LF from a header value.
//
// Header values reach BuildRFC822 from untrusted mail: replying to a message
// carries its RFC 2047-decoded Subject straight into the reply, and a decoded
// encoded-word can contain any byte sequence including CRLF. Without this, a
// hostile Subject such as
//
//	=?UTF-8?Q?Hi=0D=0ABcc:_attacker@example.com?=
//
// injects a real Bcc header into the message the user sends. CR and LF are
// never valid inside a single unfolded header value, so removing them cannot
// affect legitimate mail.
func SanitizeHeaderValue(s string) string {
	if !strings.ContainsAny(s, "\r\n") {
		return s
	}
	return strings.NewReplacer("\r", "", "\n", "").Replace(s)
}

// writeHeader appends one sanitized header line, skipping empty values.
func writeHeader(buf *bytes.Buffer, name, value string) {
	if value == "" {
		return
	}
	buf.WriteString(name)
	buf.WriteString(": ")
	buf.WriteString(SanitizeHeaderValue(value))
	buf.WriteString("\r\n")
}

// BuiltMessage is a composed message split into the two pieces PGP/MIME needs
// separately: the routing headers, which have to stay in the clear so the
// message can be delivered at all, and the MIME entity, which is what a
// signature covers and what encryption protects.
type BuiltMessage struct {
	Headers []byte
	Entity  []byte
}

// Bytes is the message as it goes on the wire.
func (m BuiltMessage) Bytes() []byte {
	out := make([]byte, 0, len(m.Headers)+len(m.Entity))
	out = append(out, m.Headers...)
	return append(out, m.Entity...)
}

// BuildRFC822 constructs a fully compliant RFC822 MIME byte slice.
func (c *ComposeMessage) BuildRFC822() ([]byte, error) {
	built, err := c.Build()
	if err != nil {
		return nil, err
	}
	return built.Bytes(), nil
}

// Build constructs the message as two byte slices: the routing headers and the
// MIME entity.
//
// The split exists because RFC 3156 signs and encrypts the entity, not the
// message. Signing everything would cover From and To, which the transport is
// entitled to rewrite; signing only the body text would leave Content-Type --
// and so the meaning of the bytes -- outside the signature.
func (c *ComposeMessage) Build() (BuiltMessage, error) {
	entity, err := c.buildEntity()
	if err != nil {
		return BuiltMessage{}, err
	}
	return BuiltMessage{Headers: c.buildHeaders(), Entity: entity}, nil
}

// buildHeaders writes the headers that route and identify the message. The
// entity's own Content-Type is not among them: it belongs to the entity.
func (c *ComposeMessage) buildHeaders() []byte {
	var buf bytes.Buffer
	writeHeader(&buf, "From", c.From)
	writeHeader(&buf, "To", c.To)
	writeHeader(&buf, "Cc", c.Cc)
	writeHeader(&buf, "Bcc", c.Bcc)

	subj := SanitizeHeaderValue(c.Subject)
	if isNonASCII(subj) {
		subj = mime.QEncoding.Encode("utf-8", subj)
	}
	writeHeader(&buf, "Subject", subj)

	msgDate := c.Date
	if msgDate.IsZero() {
		msgDate = time.Now()
	}
	buf.WriteString(fmt.Sprintf("Date: %s\r\n", msgDate.Format(time.RFC1123Z)))

	msgID := c.MessageID
	if msgID == "" {
		msgID = MessageIDForSender(c.From)
	}
	writeHeader(&buf, "Message-ID", msgID)

	writeHeader(&buf, "In-Reply-To", c.InReplyTo)
	writeHeader(&buf, "References", c.References)

	buf.WriteString("MIME-Version: 1.0\r\n")
	return buf.Bytes()
}

// buildEntity writes the MIME entity: its Content-Type, the body, and any
// attachments.
func (c *ComposeMessage) buildEntity() ([]byte, error) {
	var buf bytes.Buffer
	normBody := normalizeLineEndings(c.Body)

	cte, encodedBody := textTransferEncoding(normBody)
	if len(c.Attachments) == 0 {
		buf.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
		buf.WriteString("Content-Transfer-Encoding: " + cte + "\r\n\r\n")
		buf.WriteString(encodedBody)
		return buf.Bytes(), nil
	}

	// Multipart MIME with attachments
	mpWriter := multipart.NewWriter(&buf)
	buf.WriteString(fmt.Sprintf("Content-Type: multipart/mixed; boundary=%s\r\n\r\n", mpWriter.Boundary()))

	// Body Part
	textHeader := make(textproto.MIMEHeader)
	textHeader.Set("Content-Type", "text/plain; charset=utf-8")
	textHeader.Set("Content-Transfer-Encoding", cte)
	textPart, err := mpWriter.CreatePart(textHeader)
	if err != nil {
		return nil, fmt.Errorf("failed to create text MIME part: %w", err)
	}
	if _, err := textPart.Write([]byte(encodedBody)); err != nil {
		return nil, fmt.Errorf("failed to write body text: %w", err)
	}

	// Attachment Parts
	for _, att := range c.Attachments {
		ct := att.ContentType
		if ct == "" {
			ct = mime.TypeByExtension(filepath.Ext(att.Filename))
		}
		if ct == "" {
			ct = "application/octet-stream"
		}

		attHeader := make(textproto.MIMEHeader)
		attHeader.Set("Content-Type", fmt.Sprintf("%s; name=%q", ct, att.Filename))
		attHeader.Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", att.Filename))
		attHeader.Set("Content-Transfer-Encoding", "base64")

		part, err := mpWriter.CreatePart(attHeader)
		if err != nil {
			return nil, fmt.Errorf("failed to create attachment part %q: %w", att.Filename, err)
		}

		b64Data := encodeBase64Wrapped(att.Data, 76)
		if _, err := part.Write([]byte(b64Data)); err != nil {
			return nil, fmt.Errorf("failed to write attachment data for %q: %w", att.Filename, err)
		}
	}

	if err := mpWriter.Close(); err != nil {
		return nil, fmt.Errorf("failed to close multipart writer: %w", err)
	}

	return buf.Bytes(), nil
}

// textTransferEncoding picks the body's transfer encoding and encodes it.
//
// The entity is what a PGP signature covers, and RFC 3156 requires it to be
// 7-bit safe: a relay that downgrades 8bit, strips trailing whitespace or
// rewraps a long line breaks the signature. Plain short ASCII goes as 7bit;
// anything else as quoted-printable, which survives all three.
func textTransferEncoding(body string) (string, string) {
	if sevenBitSafe(body) {
		return "7bit", body
	}
	var buf bytes.Buffer
	qp := quotedprintable.NewWriter(&buf)
	_, _ = qp.Write([]byte(body))
	_ = qp.Close()
	return "quoted-printable", buf.String()
}

// sevenBitSafe reports whether CRLF text can travel unencoded: ASCII without
// NUL, lines of at most 76 characters, no trailing whitespace, and no line a
// transport rewrites ("From " in an mbox, a lone "." in SMTP).
func sevenBitSafe(body string) bool {
	for _, line := range strings.Split(body, "\r\n") {
		if len(line) > 76 || strings.HasPrefix(line, "From ") || strings.HasPrefix(line, ".") {
			return false
		}
		if strings.HasSuffix(line, " ") || strings.HasSuffix(line, "\t") || strings.Contains(line, "\r") {
			return false
		}
		for i := 0; i < len(line); i++ {
			if line[i] == 0 || line[i] > 127 {
				return false
			}
		}
	}
	return true
}

// normalizeLineEndings rewrites a body to CRLF, which is what a MIME entity is
// defined in and what a signature is computed over.
func normalizeLineEndings(body string) string {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	body = strings.ReplaceAll(body, "\r", "\n")
	return strings.ReplaceAll(body, "\n", "\r\n")
}

// ParseRFC822ToCompose converts raw RFC822 message bytes into a ComposeMessage struct.
func ParseRFC822ToCompose(rawBytes []byte) (*ComposeMessage, error) {
	msg, err := mail.ReadMessage(bytes.NewReader(rawBytes))
	if err != nil {
		return nil, err
	}

	dec := new(mime.WordDecoder)
	subject := DecodeHeader(dec, msg.Header.Get("Subject"))
	from := DecodeHeader(dec, msg.Header.Get("From"))
	to := DecodeHeader(dec, msg.Header.Get("To"))
	cc := DecodeHeader(dec, msg.Header.Get("Cc"))
	bcc := DecodeHeader(dec, msg.Header.Get("Bcc"))
	inReplyTo := msg.Header.Get("In-Reply-To")
	references := msg.Header.Get("References")
	msgID := msg.Header.Get("Message-ID")

	var date time.Time
	if dateStr := msg.Header.Get("Date"); dateStr != "" {
		if parsedDate, err := mail.ParseDate(dateStr); err == nil {
			date = parsedDate
		}
	}

	bodyText, _ := extractPlainBodyTextFromMsg(msg.Header, msg.Body)
	bodyText = strings.ReplaceAll(bodyText, "\r\n", "\n")
	bodyText = strings.ReplaceAll(bodyText, "\r", "\n")
	atts, _ := ExtractAttachments(rawBytes)

	return &ComposeMessage{
		From:        from,
		To:          to,
		Cc:          cc,
		Bcc:         bcc,
		Subject:     subject,
		InReplyTo:   inReplyTo,
		References:  references,
		Body:        bodyText,
		Attachments: atts,
		Date:        date,
		MessageID:   msgID,
	}, nil
}

func extractPlainBodyTextFromMsg(header mail.Header, body io.Reader) (string, error) {
	mediaType, params, err := mime.ParseMediaType(header.Get("Content-Type"))
	if err != nil || mediaType == "" {
		mediaType = "text/plain"
	}

	if strings.HasPrefix(mediaType, "text/plain") {
		data, readErr := io.ReadAll(body)
		if readErr != nil {
			return "", readErr
		}
		decoded, decodeErr := DecodePartBody(bytes.NewReader(data), header.Get("Content-Transfer-Encoding"))
		if decodeErr != nil {
			return string(data), nil
		}
		return string(decoded), nil
	}

	if strings.HasPrefix(mediaType, "multipart/") {
		if text, ok := extractMultipartPlainBody(body, params["boundary"]); ok {
			return text, nil
		}
	}

	data, err := io.ReadAll(body)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func isNonASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] > 127 {
			return true
		}
	}
	return false
}

func encodeBase64Wrapped(data []byte, lineLen int) string {
	b64 := base64.StdEncoding.EncodeToString(data)
	if len(b64) <= lineLen {
		return b64 + "\r\n"
	}
	var sb strings.Builder
	for len(b64) > lineLen {
		sb.WriteString(b64[:lineLen])
		sb.WriteString("\r\n")
		b64 = b64[lineLen:]
	}
	if len(b64) > 0 {
		sb.WriteString(b64)
		sb.WriteString("\r\n")
	}
	return sb.String()
}

func extractMultipartPlainBody(body io.Reader, boundary string) (string, bool) {
	if boundary == "" {
		return "", false
	}
	mr := multipart.NewReader(body, boundary)
	for {
		part, err := mr.NextPart()
		if err != nil {
			break
		}
		partHeader := mail.Header(part.Header)
		partMT, _, _ := mime.ParseMediaType(partHeader.Get("Content-Type"))
		if !strings.HasPrefix(partMT, "text/plain") {
			continue
		}
		data, err := io.ReadAll(part)
		if err == nil {
			return string(data), true
		}
	}
	return "", false
}
