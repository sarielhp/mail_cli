package render

import (
	"github.com/sarielhp/mail_cli/email"
	"strings"
)

// Colouring quoted reply levels is rendering, not text handling, so it moved
// here with the rest of the renderer while the plain text helpers went to the
// email package.

// ColorReplyLines colours each line of already-wrapped text by its quote depth
// and returns the bidi-ordered result; it does not reflow or re-wrap.
func ColorReplyLines(wrapped string) ColoredString {
	lines := strings.Split(wrapped, "\n")
	var segments []Segment
	for i, line := range lines {
		depth := email.CountQuoteDepth(line)
		var fg ColorID
		switch depth {
		case 0:
			fg = ColorDefault
		case 1:
			fg = ColorSender
		case 2:
			fg = ColorQuote2
		default:
			fg = ColorQuote3
		}
		displayLine := email.BidiDisplay(line)
		if i > 0 {
			displayLine = "\n" + displayLine
		}
		segments = append(segments, Segment{Text: displayLine, Fg: fg, Bg: ColorDefault})
	}
	return ColoredString{Segments: segments}
}

// ColorReplyLinesRaw strips meeting-invite boilerplate from body and colours
// the result with ColorReplyLinesWrap at wrapWidth.
func ColorReplyLinesRaw(body string, wrapWidth int) ColoredString {
	cleaned := email.CleanMeetingText(body)
	return ColorReplyLinesWrap(cleaned, wrapWidth)
}

// ColorReplyLinesWrap reflows rawBody, wraps it to wrapWidth, and colours each
// line by quote depth, drawing ASCII dividers as a horizontal rule.
func ColorReplyLinesWrap(rawBody string, wrapWidth int) ColoredString {
	rawBody = email.ReflowParagraphs(rawBody)
	var segments []Segment
	isFirst := true
	for _, line := range strings.Split(rawBody, "\n") {
		if email.IsASCIIDivider(line) {
			segments = appendDividerSegment(segments, wrapWidth, isFirst)
			isFirst = false
			continue
		}

		depth := email.CountQuoteDepth(line)
		var fg ColorID
		switch depth {
		case 0:
			fg = ColorDefault
		case 1:
			fg = ColorSender
		case 2:
			fg = ColorQuote2
		default:
			fg = ColorQuote3
		}
		if strings.TrimSpace(line) == "" {
			text := email.BidiDisplay(line)
			if !isFirst {
				text = "\n" + text
			}
			segments = append(segments, Segment{Text: text, Fg: ColorDefault, Bg: ColorDefault})
			isFirst = false
			continue
		}

		qPrefix := email.ExtractQuotePrefix(line)
		line = email.EmbedCompactOSC8Hyperlinks(line, min(60, max(10, wrapWidth-email.VisibleStringWidth(qPrefix))))
		for _, sub := range email.WrapQuotedLine(line, qPrefix, wrapWidth) {
			text := email.BidiDisplay(sub)
			if !isFirst {
				text = "\n" + text
			}
			segments = append(segments, Segment{Text: text, Fg: fg, Bg: ColorDefault})
			isFirst = false
		}
	}
	return ColoredString{Segments: segments}
}

// ColorQuotedReplyLinesRaw renders previous email / quoted content with distinctive quoted colors.
func ColorQuotedReplyLinesRaw(body string, wrapWidth int) ColoredString {
	cleaned := email.CleanMeetingText(body)
	cleaned = email.ReflowParagraphs(cleaned)
	var segments []Segment
	isFirst := true
	for _, line := range strings.Split(cleaned, "\n") {
		if email.IsASCIIDivider(line) {
			segments = appendDividerSegment(segments, wrapWidth, isFirst)
			isFirst = false
			continue
		}

		depth := email.CountQuoteDepth(line)
		var fg ColorID
		switch depth {
		case 0, 1:
			fg = ColorSender
		case 2:
			fg = ColorQuote2
		default:
			fg = ColorQuote3
		}

		if strings.TrimSpace(line) == "" {
			text := email.BidiDisplay(line)
			if !isFirst {
				text = "\n" + text
			}
			segments = append(segments, Segment{Text: text, Fg: ColorDefault, Bg: ColorDefault})
			isFirst = false
			continue
		}

		qPrefix := email.ExtractQuotePrefix(line)
		line = email.EmbedCompactOSC8Hyperlinks(line, min(60, max(10, wrapWidth-email.VisibleStringWidth(qPrefix))))
		for _, sub := range email.WrapQuotedLine(line, qPrefix, wrapWidth) {
			text := email.BidiDisplay(sub)
			if !isFirst {
				text = "\n" + text
			}
			segments = append(segments, Segment{Text: text, Fg: fg, Bg: ColorDefault})
			isFirst = false
		}
	}
	return ColoredString{Segments: segments}
}

// appendDividerSegment appends the horizontal rule that replaces an ASCII
// divider line, prefixed with a newline unless it opens the string.
func appendDividerSegment(segments []Segment, wrapWidth int, isFirst bool) []Segment {
	divLen := min(wrapWidth, 74)
	if divLen <= 0 {
		divLen = 60
	}
	divText := strings.Repeat("─", divLen)
	if !isFirst {
		divText = "\n" + divText
	}
	return append(segments, Segment{Text: divText, Fg: ColorBorder, Bg: ColorDefault})
}
