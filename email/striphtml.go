package email

import (
	"html"
	"regexp"
	"strings"
)

var htmlTagRegex = regexp.MustCompile(`<[^>]+>|<![^>]*>`)

// htmlBlockRegex removes the elements whose text is not body text.
//
// script and style run to the closing tag *or to the end of the input*. Without
// that fallback an unclosed <script> matched nothing here, the opening tag alone
// was removed as an ordinary tag, and the script body went through as prose --
// into the reading pane, and into the text the spam classifier scores. Truncated
// and malformed HTML is exactly what hostile mail sends.
//
// title and head deliberately do not get the fallback. Their content is merely
// uninteresting rather than dangerous, and a document that opens <head> and
// never closes it would otherwise lose its whole body.
//
// RE2 has no backreferences, so each element is spelled out; it is also
// linear-time, which is what makes the open-ended alternative safe on input this
// size.
var htmlBlockRegex = regexp.MustCompile(`(?i)` +
	`<script[^>]*>[\s\S]*?(?:</script\s*>|$)` +
	`|<style[^>]*>[\s\S]*?(?:</style\s*>|$)` +
	`|<title[^>]*>[\s\S]*?</title\s*>` +
	`|<head[^>]*>[\s\S]*?</head\s*>`)

// StripHTML reduces an HTML body to the words it displays.
//
// It is used for display fallback and, more importantly, as the text the
// classifier scores -- so an entity that survives undecoded is a word the
// classifier cannot see. It used to decode six named entities by hand and let
// everything else through literally, which meant "&#118;&#105;agra" read as
// exactly that: keyword obfuscation that worked by doing nothing clever.
// html.UnescapeString covers the named and numeric forms alike.
func StripHTML(text string) string {
	clean := htmlBlockRegex.ReplaceAllString(text, " ")
	clean = htmlTagRegex.ReplaceAllString(clean, "")
	// After tag removal, so that an escaped "&lt;script&gt;" becomes visible
	// text rather than a tag this function has already decided not to strip.
	clean = html.UnescapeString(clean)
	// Fields splits on unicode.IsSpace, which includes the U+00A0 that &nbsp;
	// decodes to, so the old hand-written &nbsp; case is covered.
	return strings.Join(strings.Fields(clean), " ")
}
