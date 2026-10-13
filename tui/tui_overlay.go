package tui

import (
	"strconv"
	"strings"
)

// dimFactor scales every truecolor channel of the background screen so the
// dialog reads as the active layer. ~0.42 keeps the structure legible while
// clearly receding (the top bar, unread counts and row glyphs stay visible).
const dimFactor = 0.42

// dimLine darkens the 24-bit foreground and background colours in one rendered
// line, leaving every other escape sequence and all plain text untouched. It is
// deliberately narrow: only SGR truecolor sequences the render package emits
// are rewritten, so 256-colour and named colours pass through unmodified.
func dimLine(line string) string {
	var b strings.Builder
	for i := 0; i < len(line); {
		if line[i] == 0x1b && i+1 < len(line) && line[i+1] == '[' {
			if end, rewritten, ok := dimSGR(line, i); ok {
				b.WriteString(rewritten)
				i = end
				continue
			}
		}
		b.WriteByte(line[i])
		i++
	}
	return b.String()
}

// dimSGR rewrites the truecolor runs inside a single SGR sequence beginning at
// esc, tolerating any mix of other parameters around them. The render package
// joins a foreground and a background into one sequence -- "38;2;..;48;2;.." --
// so each run must be found in place rather than assuming the sequence is only
// one colour. It returns the index just past the terminating 'm' and whether the
// sequence was an SGR at all.
func dimSGR(line string, esc int) (end int, rewritten string, ok bool) {
	end = esc + 2
	for end < len(line) && line[end] != 'm' {
		end++
	}
	if end >= len(line) {
		return esc, "", false
	}
	params := strings.Split(line[esc+2:end], ";")
	for i := 0; i < len(params); {
		if (params[i] == "38" || params[i] == "48") && i+4 < len(params) && params[i+1] == "2" {
			if rgb, dimmed := dimTruecolor(params[i+2 : i+5]); dimmed {
				params[i+2], params[i+3], params[i+4] = rgb[0], rgb[1], rgb[2]
				i += 5
				continue
			}
		}
		i++
	}
	return end + 1, "\x1b[" + strings.Join(params, ";") + "m", true
}

// dimTruecolor parses r,g,b and returns their dimmed decimal forms. ok is false
// when any channel is not a valid integer, in which case the run is left alone.
func dimTruecolor(channels []string) (rgb [3]string, ok bool) {
	for i, c := range channels {
		v, err := strconv.Atoi(c)
		if err != nil {
			return rgb, false
		}
		rgb[i] = strconv.Itoa(dimChannel(v))
	}
	return rgb, true
}

func dimChannel(v int) int {
	return int(float64(v) * dimFactor)
}
