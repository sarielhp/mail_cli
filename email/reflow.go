package email

import (
	"regexp"
	"strings"
)

// reReferenceLine matches an entry of the link list w3m appends under
// References ("[3] https://..."): one link per line, never joined.
var reReferenceLine = regexp.MustCompile(`^\[\d+\](\s|$)`)

func isBulletOrHeader(line string) bool {
	trimmed := strings.TrimLeft(line, " \t")
	if strings.HasPrefix(trimmed, "* ") || strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "+ ") || strings.HasPrefix(trimmed, "• ") {
		return true
	}
	for i, r := range trimmed {
		if r >= '0' && r <= '9' {
			continue
		}
		if i > 0 && (r == '.' || r == ')') && len(trimmed) > i+1 && (trimmed[i+1] == ' ' || trimmed[i+1] == '\t') {
			return true
		}
		break
	}
	if reReferenceLine.MatchString(trimmed) || IsASCIIDivider(line) {
		return true
	}
	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "from:") || strings.HasPrefix(lower, "to:") || strings.HasPrefix(lower, "date:") || strings.HasPrefix(lower, "subject:") || (strings.HasPrefix(lower, "on ") && strings.HasSuffix(trimmed, "wrote:")) || strings.HasPrefix(trimmed, "---") || strings.HasPrefix(trimmed, "___") {
		return true
	}
	return false
}

func getLeadingIndent(line string) int {
	indent := 0
loop:
	for _, r := range line {
		switch r {
		case ' ':
			indent++
		case '\t':
			indent += 4
		default:
			break loop
		}
	}
	return indent
}

func canJoinParagraphLines(prev, curr string) bool {
	tPrev := strings.TrimSpace(prev)
	tCurr := strings.TrimSpace(curr)
	if tPrev == "" || tCurr == "" {
		return false
	}
	if isBulletOrHeader(prev) || isBulletOrHeader(curr) {
		return false
	}
	if strings.HasSuffix(tPrev, ":") {
		return false
	}
	qPrev := ExtractQuotePrefix(prev)
	qCurr := ExtractQuotePrefix(curr)
	if qPrev != qCurr {
		return false
	}

	noQuotePrev := strings.TrimPrefix(prev, qPrev)
	noQuoteCurr := strings.TrimPrefix(curr, qCurr)

	indentPrev := getLeadingIndent(noQuotePrev)
	indentCurr := getLeadingIndent(noQuoteCurr)

	if indentPrev >= 4 || indentCurr >= 4 {
		return false
	}
	if indentPrev != indentCurr {
		return false
	}

	return true
}

// ReflowParagraphs connects short lines of the same paragraph if the next line
// starts at the same column / indentation in the original message.
// ReflowParagraphs rejoins hard-wrapped paragraphs so they can be rewrapped.
func ReflowParagraphs(text string) string {
	normalized := strings.ReplaceAll(text, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")
	if len(lines) <= 1 {
		return text
	}

	var result []string
	current := lines[0]

	for i := 1; i < len(lines); i++ {
		line := lines[i]
		if canJoinParagraphLines(current, line) {
			qPrefix := ExtractQuotePrefix(line)
			body := strings.TrimPrefix(strings.TrimLeft(line, " \t"), strings.TrimRight(qPrefix, " "))
			current = strings.TrimRight(current, " \t") + " " + strings.TrimLeft(body, " \t")
		} else {
			result = append(result, current)
			current = line
		}
	}
	result = append(result, current)
	return strings.Join(result, "\n")
}

var (
	reOnWrote     = regexp.MustCompile(`(?i)^On\s+.+wrote:\s*$`)
	reOriginalMsg = regexp.MustCompile(`(?i)^[-_]{2,}\s*(Original Message|Forwarded Message)\s*[-_]{2,}`)
	reOutlookFrom = regexp.MustCompile(`(?i)^From:\s+.+`)
	reDivider     = regexp.MustCompile(`^_{10,}`)
)

// isQuoteBlockToEnd reports whether every non-blank line from start onward is
// quote-prefixed. A client-inserted quoted reply always continues, aside from
// blank lines, through the end of the message; a single ">" line followed by
// the sender's own text (a shell command, a markdown blockquote) does not,
// and must not be mistaken for where quoting begins.
func isQuoteBlockToEnd(lines []string, start int) bool {
	for _, line := range lines[start:] {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if !strings.HasPrefix(trimmed, ">") {
			return false
		}
	}
	return true
}

// findOriginalMessageEnd finds the line index in rawBody where the original email body ends
// and quoted replies or signatures begin.
func findOriginalMessageEnd(rawBody string) int {
	lines := strings.Split(rawBody, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, ">") && isQuoteBlockToEnd(lines, i) {
			return i
		}
		if reOnWrote.MatchString(trimmed) {
			return i
		}
		if reOriginalMsg.MatchString(trimmed) {
			return i
		}
		if i > 0 && (strings.TrimSpace(lines[i-1]) == "" || reDivider.MatchString(lines[i-1])) && reOutlookFrom.MatchString(trimmed) {
			return i
		}
		if trimmed == "--" || trimmed == "-- " {
			return i
		}
	}
	return len(lines)
}

// SplitMessageAndQuotes splits rawBody into (originalBody, quotedOrSignature).
func SplitMessageAndQuotes(rawBody string) (string, string) {
	lines := strings.Split(rawBody, "\n")
	idx := findOriginalMessageEnd(rawBody)
	if idx <= 0 {
		return "", rawBody
	}
	if idx >= len(lines) {
		return rawBody, ""
	}
	original := strings.Join(lines[:idx], "\n")
	quoted := strings.Join(lines[idx:], "\n")
	return original, quoted
}
