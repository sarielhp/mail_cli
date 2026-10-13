package cli

import (
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/sarielhp/mail_cli/app"

	"github.com/sarielhp/clihelp"
	"github.com/xo/terminfo"
)

type colorSpec struct {
	name string
	fg   int
	bg   int
}

var themeColorSpecs = []colorSpec{
	{name: "color0", fg: 0, bg: 0},
	{name: "color8", fg: 8, bg: 8},
	{name: "color7", fg: 7, bg: 7},
	{name: "color15", fg: 15, bg: 15},
	{name: "colorGrey", fg: 242, bg: 242},
	{name: "red", fg: 196, bg: 52},
	{name: "green", fg: 46, bg: 22},
	{name: "yellow", fg: 226, bg: 100},
	{name: "blue", fg: 33, bg: 19},
	{name: "magenta", fg: 201, bg: 90},
	{name: "cyan", fg: 51, bg: 30},
	{name: "white", fg: 255, bg: 255},
	{name: "orange", fg: 214, bg: 94},
	{name: "pink", fg: 213, bg: 96},
	{name: "brown", fg: 130, bg: 58},
	{name: "purple", fg: 129, bg: 55},
	{name: "teal", fg: 43, bg: 23},
}

// ColorCmd returns the clihelp.Command for the color command.
func ColorCmd(_ *Session) clihelp.Command {
	return clihelp.Command{
		Name:        "color",
		Group:       "Other",
		Aliases:     []string{"test-color"},
		Description: "Test the terminal's true-color support.",
		UsageLine:   "mail_cli utils color",
		Examples: []clihelp.Example{
			{Line: "mail_cli utils color"},
		},
		Args: clihelp.NoArgs,
		Run: func(_ *clihelp.Context) error {
			return runColorTest()
		},
	}
}

func runColorTest() error {
	out := os.Stdout
	colored, err := terminfo.ColorLevelFromEnv()
	if err != nil {
		return fmt.Errorf("cannot detect color capability: %w", err)
	}

	fmt.Fprintln(out, "╔══════════════════════════════════╗")
	fmt.Fprintln(out, "║      Terminal Color Test         ║")
	fmt.Fprintln(out, "╚══════════════════════════════════╝")
	fmt.Fprintln(out)
	fmt.Fprintf(out, "Terminal: %s\n", os.Getenv("TERM"))
	fmt.Fprintf(out, "Color Level: %s\n", colored)
	fmt.Fprintln(out)

	fmt.Fprintf(out, "%s 24-bit True Color Test:\n\n", app.ColorBoldYellow.Sprint("▸"))
	esc := string([]byte{27})
	for _, spec := range themeColorSpecs {
		renderColoredBox(out, fmt.Sprintf("%s[38;5;%dm", esc, spec.fg), fmt.Sprintf("%s[48;5;%dm", esc, spec.bg), spec.name)
	}
	fmt.Fprintln(out)
	renderHighlightTest(out, func(fg, bg, text string) string {
		return esc + "[38;5;" + fg + "m" + esc + "[48;5;" + bg + "m" + text + esc + "[0m"
	})
	fmt.Fprintln(out)
	renderHighlightTest256(out, func(fg, bg, text string) string {
		return esc + "[38;5;" + fg + "m" + esc + "[48;5;" + bg + "m" + text + esc + "[0m"
	})
	return nil
}

func renderColoredBox(w io.Writer, fgCode, bgCode, label string) {
	esc := string([]byte{27})
	fmt.Fprintf(w, "%s%s %-10s %s\n", fgCode, bgCode, label, esc+"[0m")
}

func renderHighlightTest(w io.Writer, format func(fg, bg, text string) string) {
	fmt.Fprintf(w, "%s 256-Color Named Highlight Test (%d shades):\n\n", app.ColorBoldYellow.Sprint("▸"), len(themeColorSpecs))
	esc := string([]byte{27})
	for _, spec := range themeColorSpecs {
		fgStr := strconv.Itoa(spec.fg)
		bgStr := strconv.Itoa(spec.bg)
		fmt.Fprintf(w, "  %s %-10s %s  ", format(fgStr, bgStr, "Hello"), spec.name, esc+"[0m")
	}
	fmt.Fprintln(w)
}

func renderHighlightTest256(w io.Writer, format func(fg, bg, text string) string) {
	fmt.Fprintf(w, "%s 256-Color 6x6x6 Cube Test:\n\n", app.ColorBoldYellow.Sprint("▸"))
	for g := 0; g < 6; g++ {
		for r := 0; r < 6; r++ {
			for b := 0; b < 6; b++ {
				code := 16 + (36*r + 6*g + b)
				fg := "15"
				bg := strconv.Itoa(code)
				if r > 3 || g > 3 || b > 2 {
					fg = "0"
				}
				fmt.Fprintf(w, "%s ", format(fg, bg, "▉"))
			}
		}
		fmt.Fprintln(w)
	}
	fmt.Fprintln(w)
}
