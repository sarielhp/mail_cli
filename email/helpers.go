package email

import (
	"fmt"
	"io"
	"mime"
	"net/mail"
	"regexp"
	"strings"
	"time"

	"golang.org/x/text/encoding/ianaindex"
)

// truncatedMarker stands in for a trailing encoded word that was cut off.
const truncatedMarker = " […]"

var (
	encodedWordRegexp           = regexp.MustCompile(`=\?[a-zA-Z0-9_-]+\?[qQbB]\?[^?]*\?=`)
	incompleteEncodedWordRegexp = regexp.MustCompile(`(?i)\s*=\?[a-z0-9_-]*(?:\?[a-z0-9_-]*(?:\?[^?]*)?)?$`)
)

func defaultCharsetReader(charset string, input io.Reader) (io.Reader, error) {
	enc, err := ianaindex.MIME.Encoding(charset)
	if err != nil || enc == nil {
		return input, nil
	}
	decoder := enc.NewDecoder()
	if decoder == nil {
		return input, nil
	}
	return decoder.Reader(input), nil
}

// DecodeHeader decodes MIME encoded-word headers using the provided decoder.
// It handles UTF-8, ISO-8859-1, and other character sets.
//
// The decoded value is stripped of terminal escape sequences: this is the
// decode path used directly (not through email.ParseReader) by the CLI's
// `show` command, cli/split.go, spam/spam.go and spam/unspam.go,
// scan/scan.go and scan/scanner.go, email/compose.go's reply/forward
// drafts, and every backend's notification building -- all of which would
// otherwise show a malicious Subject/From verbatim, able to manipulate the
// terminal purely through the header's own bytes.
func DecodeHeader(dec *mime.WordDecoder, val string) string {
	return StripEscapeSequences(decodeHeaderRaw(dec, val))
}

func decodeHeaderRaw(dec *mime.WordDecoder, val string) string {
	decoder := dec
	if decoder == nil {
		decoder = &mime.WordDecoder{
			CharsetReader: defaultCharsetReader,
		}
	}
	if decoder.CharsetReader == nil {
		cloned := *decoder
		cloned.CharsetReader = defaultCharsetReader
		decoder = &cloned
	}

	normalized := val
	normalized = strings.ReplaceAll(normalized, "=?UTF8?", "=?UTF-8?")
	normalized = strings.ReplaceAll(normalized, "=?utf8?", "=?utf-8?")

	// A truncated trailing encoded word cannot be decoded, but deleting it
	// silently shortened the subject with nothing to show for it -- a header cut
	// off mid-word rendered as though the sender had written it that way. Mark
	// the loss instead.
	normalized = incompleteEncodedWordRegexp.ReplaceAllString(normalized, truncatedMarker)

	locs := encodedWordRegexp.FindAllStringIndex(normalized, -1)
	if len(locs) == 0 {
		return normalized
	}

	type token struct {
		text      string
		isDecoded bool
	}

	var tokens []token
	lastEnd := 0

	for _, loc := range locs {
		start, end := loc[0], loc[1]
		if start > lastEnd {
			tokens = append(tokens, token{
				text:      normalized[lastEnd:start],
				isDecoded: false,
			})
		}
		word := normalized[start:end]
		decoded, err := decoder.Decode(word)
		if err == nil {
			tokens = append(tokens, token{
				text:      decoded,
				isDecoded: true,
			})
		} else {
			tokens = append(tokens, token{
				text:      word,
				isDecoded: false,
			})
		}
		lastEnd = end
	}
	if lastEnd < len(normalized) {
		tokens = append(tokens, token{
			text:      normalized[lastEnd:],
			isDecoded: false,
		})
	}

	// Filter out whitespace-only non-decoded tokens between two decoded tokens.
	// Use strings.Builder for efficient concatenation
	var sb strings.Builder
	for i := 0; i < len(tokens); i++ {
		if tokens[i].isDecoded && i+2 < len(tokens) && tokens[i+2].isDecoded {
			if !tokens[i+1].isDecoded && strings.TrimSpace(tokens[i+1].text) == "" {
				sb.WriteString(tokens[i].text)
				i++ // skip the middle whitespace token
				continue
			}
		}
		sb.WriteString(tokens[i].text)
	}

	return sb.String()
}

var emailValidationRegex = regexp.MustCompile(`^[a-zA-Z0-9.!#$%&'*+/=?^_` + "`" + `{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)+$`)

// IsValidEmail verifies that an email address has a valid syntax, contains '@',
// and has a valid domain with at least one dot.
func IsValidEmail(addr string) bool {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return false
	}
	if parsed, err := mail.ParseAddress(addr); err == nil && parsed != nil {
		addr = parsed.Address
	}
	return emailValidationRegex.MatchString(addr)
}

// IsValidEmailOrDomain checks if addr is a valid email address or a domain pattern (e.g. "@example.com").
func IsValidEmailOrDomain(addr string) bool {
	addr = strings.TrimSpace(addr)
	if strings.HasPrefix(addr, "@") {
		domain := addr[1:]
		if len(domain) < 3 || strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") || strings.Contains(domain, " ") || !strings.Contains(domain, ".") {
			return false
		}
		return true
	}
	return IsValidEmail(addr)
}

// ParseEmailAddress returns the address part of a From-style header, lowercased;
// it falls back to the text inside angle brackets, then to the whole header.
func ParseEmailAddress(fromHeader string) string {
	addr, err := mail.ParseAddress(fromHeader)
	if err == nil && addr != nil {
		return strings.ToLower(strings.TrimSpace(addr.Address))
	}
	if start := strings.Index(fromHeader, "<"); start != -1 {
		if end := strings.Index(fromHeader[start:], ">"); end != -1 {
			return strings.ToLower(strings.TrimSpace(fromHeader[start+1 : start+end]))
		}
	}
	return strings.ToLower(strings.TrimSpace(fromHeader))
}

// FormatEmailDate renders t as the space-padded month and day with a two-digit
// year, the short stamp used in listings, such as " 1/ 5/26".
func FormatEmailDate(t time.Time) string {
	return fmt.Sprintf("%2d/%2d/%02d", t.Month(), t.Day(), t.Year()%100)
}

func cleanSubject(s string) string {
	s = strings.ToLower(s)
	for {
		oldLen := len(s)
		s = strings.TrimSpace(s)
		s = strings.TrimPrefix(s, "re:")
		s = strings.TrimPrefix(s, "fwd:")
		s = strings.TrimPrefix(s, "fw:")
		s = strings.TrimPrefix(s, "aw:")
		s = strings.TrimPrefix(s, "reply:")
		if len(s) == oldLen {
			break
		}
	}
	return strings.TrimSpace(s)
}

// StripSubjectPrefix removes any run of Re:, Fwd:, Fw:, Aw: and Reply: prefixes
// from s, compared case-insensitively, and returns the trimmed remainder.
func StripSubjectPrefix(s string) string {
	for {
		trimmed := strings.TrimSpace(s)
		lower := strings.ToLower(trimmed)
		prefixes := []string{"re:", "fwd:", "fw:", "aw:", "reply:"}
		matched := false
		for _, p := range prefixes {
			if strings.HasPrefix(lower, p) {
				s = trimmed[len(p):]
				matched = true
				break
			}
		}
		if !matched {
			break
		}
	}
	return strings.TrimSpace(s)
}
