package email

import (
	"fmt"
	"net/mail"
	"strings"
	"testing"
)

// assertStripHTML checks that StripHTML renders input as want.
func assertStripHTML(t *testing.T, input, want string) {
	t.Helper()
	if got := StripHTML(input); got != want {
		t.Errorf("StripHTML(%q) = %q, want %q", input, got, want)
	}
}

// stripHTMLCase is one HTML fragment and the text StripHTML must render from
// it. group names the boundary test that owns the case.
type stripHTMLCase struct {
	group string
	name  string
	input string
	want  string
}

// stripHTMLCases is the shared corpus for the two StripHTML boundary tests,
// keeping their scenarios in one table.
var stripHTMLCases = []stripHTMLCase{
	{
		// An unclosed <script> used to leak its body as prose: the opening tag
		// was removed as an ordinary tag and the script source after it went
		// into the reading pane and the text the classifier scores.
		group: "markup",
		name:  "unclosed script",
		input: "Hello <script>var x = 'buy pills now';",
		want:  "Hello",
	},
	{
		group: "markup",
		name:  "unclosed style",
		input: "Hello <style>body { content: 'buy pills now'; }",
		want:  "Hello",
	},
	{
		group: "markup",
		name:  "closed script still works",
		input: "Hello <script>var x = 1;</script> World",
		want:  "Hello World",
	},
	{
		group: "markup",
		name:  "closing tag with trailing space",
		input: "Hello <script>var x = 1;</script > World",
		want:  "Hello World",
	},
	{
		// head and title keep requiring their closing tag: their content is
		// uninteresting rather than dangerous, and swallowing to end-of-input
		// would lose the body of a document that never closes <head>.
		group: "markup",
		name:  "unclosed head keeps the body",
		input: "<head><meta><body>Hello World",
		want:  "Hello World",
	},
	{
		// Numeric character references used to pass through as literal text, so
		// a keyword spelled with them was invisible to the classifier while
		// rendering normally in any mail reader.
		group: "entity",
		name:  "decimal",
		input: "&#118;&#105;agra",
		want:  "viagra",
	},
	{group: "entity", name: "hex", input: "&#x76;&#x69;agra", want: "viagra"},
	{group: "entity", name: "named beyond the old six", input: "caf&eacute; &mdash; open", want: "café — open"},
	{group: "entity", name: "the old six still work", input: "a &amp; b &lt; c &gt; d &quot;e&quot; &#39;f&#39;", want: `a & b < c > d "e" 'f'`},
	{group: "entity", name: "nbsp collapses as whitespace", input: "a&nbsp;&nbsp;b", want: "a b"},
}

// stripHTMLCasesFor returns the corpus entries a boundary test owns.
func stripHTMLCasesFor(group string) []stripHTMLCase {
	var out []stripHTMLCase
	for _, tc := range stripHTMLCases {
		if tc.group == group {
			out = append(out, tc)
		}
	}
	return out
}

// TestStripHTMLDropsUnclosedScriptAndStyle: an unclosed <script> or <style>
// must not leak its body as prose into the reading pane.
func TestStripHTMLDropsUnclosedScriptAndStyle(t *testing.T) {
	for _, tc := range stripHTMLCasesFor("markup") {
		t.Run(tc.name, func(t *testing.T) {
			assertStripHTML(t, tc.input, tc.want)
		})
	}
}

// TestStripHTMLDecodesNumericAndNamedEntities: numeric and named character
// references must decode rather than reach the classifier as literal text.
func TestStripHTMLDecodesNumericAndNamedEntities(t *testing.T) {
	for _, tc := range stripHTMLCasesFor("entity") {
		t.Run(tc.name, func(t *testing.T) {
			assertStripHTML(t, tc.input, tc.want)
		})
	}
}

// An escaped tag is text, and must stay text: unescaping happens after tag
// removal so that "&lt;script&gt;" cannot become a tag this function has
// already finished stripping.
func TestStripHTMLDoesNotResurrectEscapedTags(t *testing.T) {
	const in = "Code sample: &lt;script&gt;alert(1)&lt;/script&gt;"
	const want = "Code sample: <script>alert(1)</script>"

	if got := StripHTML(in); got != want {
		t.Errorf("StripHTML(%q) = %q, want %q", in, got, want)
	}
}

// nestedMultipart builds a message whose multipart/mixed parts nest `depth`
// levels deep, with a single named attachment at the bottom.
func nestedMultipart(depth int) string {
	var b strings.Builder
	b.WriteString("Content-Type: multipart/mixed; boundary=b0\r\n\r\n")
	for i := range depth {
		fmt.Fprintf(&b, "--b%d\r\nContent-Type: multipart/mixed; boundary=b%d\r\n\r\n", i, i+1)
	}
	fmt.Fprintf(&b, "--b%d\r\nContent-Type: text/plain; name=\"deep.txt\"\r\n"+
		"Content-Disposition: attachment; filename=\"deep.txt\"\r\n\r\nhello\r\n--b%d--\r\n", depth, depth)
	for i := depth - 1; i >= 0; i-- {
		fmt.Fprintf(&b, "--b%d--\r\n", i)
	}
	return b.String()
}

// Multipart nesting is followed only as far as maxMIMEDepth.
//
// The walk runs automatically whenever a message is opened or scanned, on input
// the sender chooses, and it had no limit at all -- the body walk in body.go
// already capped itself, and these two did not. Real mail is two to four levels
// deep, so the cap costs nothing a genuine message would have carried.
func TestMultipartNestingStopsAtTheDepthCap(t *testing.T) {
	tests := []struct {
		name  string
		depth int
		want  bool
	}{
		{name: "shallow, as real mail is", depth: 3, want: true},
		{name: "just inside the cap", depth: maxMIMEDepth - 2, want: true},
		{name: "past the cap", depth: maxMIMEDepth + 5, want: false},
		{name: "far past the cap", depth: 2000, want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			msg, err := mail.ReadMessage(strings.NewReader(nestedMultipart(tc.depth)))
			if err != nil {
				t.Fatalf("building a %d-deep message: %v", tc.depth, err)
			}

			hasAtt, _ := DetectAttachments(msg.Header, msg.Body)
			if hasAtt != tc.want {
				t.Errorf("DetectAttachments at depth %d = %v, want %v", tc.depth, hasAtt, tc.want)
			}
		})
	}
}

// ExtractAttachments walks the same tree by a different pair of functions, and
// they were uncapped together.
func TestExtractAttachmentsStopsAtTheDepthCap(t *testing.T) {
	if atts, err := ExtractAttachments([]byte(nestedMultipart(2000))); err != nil || len(atts) != 0 {
		t.Errorf("got %d attachments (err %v) from past the cap, want none", len(atts), err)
	}

	atts, err := ExtractAttachments([]byte(nestedMultipart(3)))
	if err != nil {
		t.Fatalf("ExtractAttachments: %v", err)
	}
	if len(atts) != 1 || atts[0].Filename != "deep.txt" {
		t.Errorf("got %v, want the one attachment real nesting depth carries", atts)
	}
}
