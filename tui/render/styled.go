package render

import (
	"fmt"
	"strings"

	"github.com/mattn/go-runewidth"
)

// Segment is one run of text together with the colours and strikethrough it is
// drawn with.
type Segment struct {
	Text   string
	Fg     ColorID
	Bg     ColorID
	Strike bool
}

// ColoredString is text split into styled segments; it renders to ANSI escapes
// or flattens to plain text.
type ColoredString struct {
	Segments []Segment
}

// Render returns the string with ANSI colour and strikethrough escapes applied;
// an empty ColoredString renders as "".
func (c ColoredString) Render(t *Theme) string {
	if len(c.Segments) == 0 {
		return ""
	}
	var result string
	for _, seg := range c.Segments {
		result += segmentRender(seg, t)
	}
	return result
}

func segmentRender(seg Segment, t *Theme) string {
	var sb strings.Builder
	esc := string([]byte{27})
	if seg.Fg >= 0 || seg.Bg >= 0 {
		sb.WriteString(esc + "[")
		var codes []string
		if seg.Fg >= 0 {
			codes = append(codes, fmtFgCode(t.Get(seg.Fg)))
		}
		if seg.Bg >= 0 {
			codes = append(codes, fmtBgCode(t.Get(seg.Bg)))
		}
		sb.WriteString(strings.Join(codes, ";"))
		sb.WriteString("m")
	}
	if seg.Strike {
		sb.WriteString(esc + "[9m")
	}
	sb.WriteString(seg.Text)
	if seg.Strike {
		sb.WriteString(esc + "[29m")
	}
	if seg.Fg >= 0 || seg.Bg >= 0 {
		sb.WriteString(esc + "[0m")
	}
	return sb.String()
}

// fmtColorCode formats color as an SGR parameter, using the truecolour form
// for a #rrggbb value and the 256-colour form otherwise. sel is the SGR
// selector: "38" for a foreground, "48" for a background.
func fmtColorCode(color, sel string) string {
	if strings.HasPrefix(color, "#") {
		h := color[1:]
		if len(h) == 6 {
			var r, g, b int
			if _, err := fmt.Sscanf(h, "%02x%02x%02x", &r, &g, &b); err == nil {
				return fmt.Sprintf("%s;2;%d;%d;%d", sel, r, g, b)
			}
		}
	}
	return fmt.Sprintf("%s;5;%s", sel, color)
}

func fmtFgCode(color string) string { return fmtColorCode(color, "38") }

func fmtBgCode(color string) string { return fmtColorCode(color, "48") }

// Plain returns the segment text with all styling removed.
func (c ColoredString) Plain() string {
	var sb strings.Builder
	for _, seg := range c.Segments {
		sb.WriteString(seg.Text)
	}
	return sb.String()
}

// Width returns the display width in terminal cells, counting wide and
// combining runes correctly.
func (c ColoredString) Width() int {
	w := 0
	for _, seg := range c.Segments {
		w += runewidth.StringWidth(seg.Text)
	}
	return w
}

// TruncateLeft shortens every segment to at most width display cells by
// dropping runes from its front; a non-positive width empties the segments.
func (c ColoredString) TruncateLeft(width int) ColoredString {
	if width <= 0 {
		for i := range c.Segments {
			c.Segments[i].Text = ""
		}
		return c
	}
	for i := range c.Segments {
		w := runewidth.StringWidth(c.Segments[i].Text)
		if w > width {
			runes := []rune(c.Segments[i].Text)
			for w > width {
				runes = runes[1:]
				w = runewidth.StringWidth(string(runes))
			}
			c.Segments[i].Text = string(runes)
		}
	}
	return c
}

// TruncateRight shortens every segment to at most width display cells by
// dropping runes from its end; a non-positive width empties the segments.
func (c ColoredString) TruncateRight(width int) ColoredString {
	if width <= 0 {
		for i := range c.Segments {
			c.Segments[i].Text = ""
		}
		return c
	}
	for i := range c.Segments {
		w := runewidth.StringWidth(c.Segments[i].Text)
		if w > width {
			runes := []rune(c.Segments[i].Text)
			for w > width {
				runes = runes[:len(runes)-1]
				w = runewidth.StringWidth(string(runes))
			}
			c.Segments[i].Text = string(runes)
		}
	}
	return c
}

// PadLeft prepends spaces, in the first segment's colours, to bring the string
// up to width cells; it is returned unchanged when already that wide.
func (c ColoredString) PadLeft(width int) ColoredString {
	curW := c.Width()
	if curW >= width {
		return c
	}
	count := width - curW
	var firstFG, firstBG ColorID
	if len(c.Segments) > 0 {
		first := c.Segments[0]
		firstFG = first.Fg
		firstBG = first.Bg
	}
	c.Segments = append([]Segment{{Text: strings.Repeat(" ", count), Fg: firstFG, Bg: firstBG}}, c.Segments...)
	return c
}

// PadRight appends spaces, in the last segment's colours, to bring the string
// up to width cells; it is returned unchanged when already that wide.
func (c ColoredString) PadRight(width int) ColoredString {
	curW := c.Width()
	if curW >= width {
		return c
	}
	count := width - curW
	var lastFG, lastBG ColorID
	if len(c.Segments) > 0 {
		last := c.Segments[len(c.Segments)-1]
		lastFG = last.Fg
		lastBG = last.Bg
	}
	c.Segments = append(c.Segments, Segment{Text: strings.Repeat(" ", count), Fg: lastFG, Bg: lastBG})
	return c
}

// Plain wraps s in a single default-coloured segment; an empty s yields an
// empty ColoredString.
func Plain(s string) ColoredString {
	if s == "" {
		return ColoredString{}
	}
	return ColoredString{Segments: []Segment{{Text: s, Fg: ColorDefault, Bg: ColorDefault}}}
}

// WithFg gives fg to the segments that have no foreground colour of their own,
// leaving explicitly coloured segments untouched.
func (c ColoredString) WithFg(fg ColorID) ColoredString {
	for i := range c.Segments {
		if c.Segments[i].Fg == ColorDefault {
			c.Segments[i].Fg = fg
		}
	}
	return c
}

// WithBg gives bg to the segments that have no background colour of their own,
// leaving explicitly coloured segments untouched.
func (c ColoredString) WithBg(bg ColorID) ColoredString {
	for i := range c.Segments {
		if c.Segments[i].Bg == ColorDefault {
			c.Segments[i].Bg = bg
		}
	}
	return c
}

// WithBgAll gives bg to the segments that have no background colour of their
// own; it performs the same operation as WithBg.
func (c ColoredString) WithBgAll(bg ColorID) ColoredString {
	for i := range c.Segments {
		if c.Segments[i].Bg == ColorDefault {
			c.Segments[i].Bg = bg
		}
	}
	return c
}

// Len returns the total byte length of the segment text, not its display width;
// use Width for terminal cells.
func (c ColoredString) Len() int {
	var w int
	for _, s := range c.Segments {
		w += len(s.Text)
	}
	return w
}
