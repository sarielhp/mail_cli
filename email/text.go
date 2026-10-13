package email

import (
	"mime"
	"net/mail"
	"regexp"
	"strings"

	"github.com/mattn/go-runewidth"
	"golang.org/x/text/unicode/bidi"
)

func hasRTL(s string) bool {
	for _, r := range s {
		if r >= 0x0590 && r <= 0x06FF {
			return true
		}
	}
	return false
}

// BidiDisplay reorders the runs of an RTL (Hebrew or Arabic) string into visual
// order for a terminal, and returns s unchanged when it holds no RTL characters
// or cannot be processed.
func BidiDisplay(s string) string {
	if s == "" || !hasRTL(s) {
		return s
	}
	var p bidi.Paragraph
	if _, err := p.SetString(s, bidi.DefaultDirection(bidi.RightToLeft)); err != nil {
		return s
	}
	o, err := p.Order()
	if err != nil {
		return s
	}
	var sb strings.Builder
	for i := 0; i < o.NumRuns(); i++ {
		r := o.Run(i)
		sb.WriteString(r.String())
	}
	return sb.String()
}

// ReverseHebrewRuns reverses each run of Hebrew characters, keeping any
// trailing neutral characters in place, so the run reads correctly in a
// left-to-right terminal.
func ReverseHebrewRuns(s string) string {
	var result []rune
	var run []rune
	inRun := false

	isHebrew := func(r rune) bool {
		return r >= 0x0590 && r <= 0x05FF
	}

	isNeutral := func(r rune) bool {
		return r == ' ' || r == '-' || r == '_' || r == ':' || r == ',' || r == '.' || r == '!' || r == '?' || (r >= '0' && r <= '9')
	}

	flushRun := func() {
		if len(run) > 0 {
			hasHebrew := false
			for _, r := range run {
				if isHebrew(r) {
					hasHebrew = true
					break
				}
			}

			if hasHebrew {
				trimIdx := len(run)
				for trimIdx > 0 && isNeutral(run[trimIdx-1]) {
					trimIdx--
				}

				for i := trimIdx - 1; i >= 0; i-- {
					result = append(result, run[i])
				}
				for i := trimIdx; i < len(run); i++ {
					result = append(result, run[i])
				}
			} else {
				result = append(result, run...)
			}
			run = nil
		}
		inRun = false
	}

	for _, r := range s {
		if isHebrew(r) || (inRun && isNeutral(r)) {
			inRun = true
			run = append(run, r)
		} else {
			flushRun()
			result = append(result, r)
		}
	}
	flushRun()

	return string(result)
}

// DecDecode is a second, independent RFC2047 header-decode path from
// DecodeHeader (email/helpers.go), used directly by the TUI's detail view,
// its LLM summary overlay, its reply-compose subject, and email/reply.go's
// reply composition -- all of which decode a raw header themselves instead
// of going through email.ParseReader's Email.Subject/FromRaw fields, and so
// would otherwise bypass the escape-sequence stripping ParseReader applies
// there.
func DecDecode(dec *mime.WordDecoder, v string) string {
	d, err := dec.DecodeHeader(v)
	if err != nil {
		return StripEscapeSequences(v)
	}
	return StripEscapeSequences(d)
}

// FormatSender returns the display name from a From header, or the bare address
// when the header has no name.
func FormatSender(fromRaw string) string {
	name, email := parseSenderInfo(fromRaw)
	if name != "" {
		return name
	}
	return email
}

// FormatSenderVerbose returns "Name <address>" from a From header, or just the
// address when the header has no name.
func FormatSenderVerbose(fromRaw string) string {
	name, email := parseSenderInfo(fromRaw)
	if name != "" {
		return name + " <" + email + ">"
	}
	return email
}

func parseSenderInfo(fromHeader string) (name string, email string) {
	addr, err := mail.ParseAddress(fromHeader)
	if err == nil && addr != nil {
		return strings.TrimSpace(addr.Name), strings.ToLower(strings.TrimSpace(addr.Address))
	}
	return "", strings.ToLower(strings.TrimSpace(fromHeader))
}

// ParseSenderInfo splits a From header into its display name and its lowercased
// address, returning an empty name when the header has none.
func ParseSenderInfo(fromHeader string) (name string, email string) {
	return parseSenderInfo(fromHeader)
}

// WrapText reflows text and wraps it to width columns, keeping each line's quote
// prefix; a non-positive width returns text unchanged.
func WrapText(text string, width int) string {
	if width <= 0 {
		return text
	}
	text = ReflowParagraphs(text)
	var lines []string
	normalized := strings.ReplaceAll(text, "\r\n", "\n")
	for _, l := range strings.Split(normalized, "\n") {
		trimmed := strings.TrimRight(l, " \t\r")
		if strings.TrimSpace(trimmed) == "" {
			lines = append(lines, "")
			continue
		}
		qPrefix := ExtractQuotePrefix(trimmed)
		if qPrefix != "" {
			lines = append(lines, WrapQuotedLine(trimmed, qPrefix, width)...)
			continue
		}
		lines = append(lines, truncateLines(trimmed, width))
	}
	return strings.Join(lines, "\n")
}

func truncateWord(word string, maxLen int) string {
	if strings.Contains(word, "\x1b") {
		return word
	}
	var truncated []rune
	w := 0
	for _, r := range word {
		rw := runewidth.RuneWidth(r)
		if w+rw > maxLen {
			break
		}
		truncated = append(truncated, r)
		w += rw
	}
	return string(truncated)
}

func truncateLines(s string, maxLen int) string {
	var out strings.Builder
	words := strings.Fields(s)
	var line []rune
	lineWidth := 0
	for _, word := range words {
		wordWidth := VisibleStringWidth(word)
		if len(line) > 0 && lineWidth+1+wordWidth > maxLen {
			out.WriteString(string(line) + "\n")
			line = nil
			lineWidth = 0
			if wordWidth > maxLen {
				out.WriteString(truncateWord(word, maxLen))
			} else {
				out.WriteString(word)
			}
			continue
		}
		if len(line) > 0 {
			line = append(line, ' ')
			lineWidth++
		}
		line = append(line, []rune(word)...)
		lineWidth += wordWidth
	}
	if len(line) > 0 {
		out.WriteString(string(line))
	}
	return out.String()
}

// CountQuoteDepth returns how many '>' quoting markers appear in the first 21
// runes of a line.
func CountQuoteDepth(line string) int {
	runes := []rune(line)
	count := 0
	for i, r := range runes {
		if i > 20 {
			break
		}
		if r == '>' {
			count++
		}
	}
	return count
}

// IsASCIIDivider checks if a line is an ASCII separator line made of underscores, dashes, equals, etc.
func IsASCIIDivider(line string) bool {
	trimmed := strings.TrimSpace(line)
	if len(trimmed) < 4 {
		return false
	}
	runes := []rune(trimmed)
	first := runes[0]
	if first == '_' || first == '-' || first == '=' || first == '~' || first == '*' || first == '─' {
		isAllSame := true
		for _, r := range runes {
			if r != first && r != ' ' {
				isAllSame = false
				break
			}
		}
		if isAllSame {
			return true
		}
	}
	if strings.HasPrefix(trimmed, "-::~") || strings.HasPrefix(trimmed, "~~//~~") || strings.HasPrefix(trimmed, "__//__") {
		return true
	}
	return false
}

var reJoinMeeting = regexp.MustCompile(`(?i)(Join(?:\s+with\s+Google\s+Meet|\s+Zoom\s+Meeting|\s+Microsoft\s+Teams\s+Meeting)?)\s*(:?)\s*\n\s*(https?://\S+)`)

// CleanMeetingText collapses redundant newlines between 'Join' labels and URLs.
// CleanMeetingText strips the boilerplate a meeting invite wraps its body in.
func CleanMeetingText(rawBody string) string {
	return reJoinMeeting.ReplaceAllString(rawBody, "$1: $3")
}

// ExtractQuotePrefix extracts the leading quote markers (e.g. "> ", ">> ", "> > ") from a line.
func ExtractQuotePrefix(line string) string {
	trimmed := strings.TrimLeft(line, " \t")
	if !strings.HasPrefix(trimmed, ">") {
		return ""
	}
	var sb strings.Builder
	runes := []rune(trimmed)
	i := 0
	for i < len(runes) && (runes[i] == '>' || runes[i] == ' ' || runes[i] == '\t') {
		sb.WriteRune(runes[i])
		i++
	}
	p := sb.String()
	if !strings.HasSuffix(p, " ") {
		p += " "
	}
	return p
}

// WrapQuotedLine wraps one quoted line, repeating its quote prefix.
func WrapQuotedLine(trimmed, qPrefix string, width int) []string {
	bodyPart := strings.TrimPrefix(strings.TrimLeft(trimmed, " \t"), strings.TrimRight(qPrefix, " "))
	words := strings.Fields(bodyPart)
	prefixW := VisibleStringWidth(qPrefix)
	effectiveWidth := max(10, width-prefixW)
	var lines []string
	var sb strings.Builder
	curW := 0

	for _, word := range words {
		wLen := VisibleStringWidth(word)
		// A word wider than the pane -- a URL where the terminal has no
		// hyperlinks -- is broken across lines; the viewport would otherwise
		// crop it and lose its end. A word carrying an escape is a link
		// already shortened to fit, and splitting it would break the link.
		if wLen > effectiveWidth && !strings.Contains(word, "\x1b") {
			room := 0
			if sb.Len() > 0 {
				room = effectiveWidth - curW - 1
			}
			pieces := splitByWidth(word, room, effectiveWidth)
			if pieces[0] != "" {
				sb.WriteByte(' ')
				sb.WriteString(pieces[0])
			}
			for _, piece := range pieces[1:] {
				if sb.Len() > 0 {
					lines = append(lines, qPrefix+sb.String())
					sb.Reset()
				}
				sb.WriteString(piece)
			}
			curW = VisibleStringWidth(sb.String())
			continue
		}
		if sb.Len() == 0 {
			sb.WriteString(word)
			curW = wLen
			continue
		}
		if curW+1+wLen > effectiveWidth {
			lines = append(lines, qPrefix+sb.String())
			sb.Reset()
			sb.WriteString(word)
			curW = wLen
			continue
		}
		sb.WriteByte(' ')
		sb.WriteString(word)
		curW += 1 + wLen
	}
	if sb.Len() > 0 {
		lines = append(lines, qPrefix+sb.String())
	}
	return lines
}

// splitByWidth cuts word into pieces no wider than width, except the first,
// which fills the room left on the current line. Less room than a few
// characters is not worth using, so the first piece is then empty.
func splitByWidth(word string, room, width int) []string {
	var pieces []string
	limit := room
	if limit < 8 {
		pieces = append(pieces, "")
		limit = width
	}
	var cur strings.Builder
	curW := 0
	for _, r := range word {
		w := VisibleStringWidth(string(r))
		if curW+w > limit && cur.Len() > 0 {
			pieces = append(pieces, cur.String())
			cur.Reset()
			curW, limit = 0, width
		}
		cur.WriteRune(r)
		curW += w
	}
	return append(pieces, cur.String())
}
