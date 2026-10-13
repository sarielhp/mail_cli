package app

import (
	"fmt"

	"github.com/fatih/color"
)

// The palette the CLI prints with: the plain colours for normal output and the
// bold variants for emphasis. They are fatih/color values whose Sprint applies
// the escape codes, and sharing them keeps every command's colours the same.
var (
	ColorCyan      = color.New(color.FgCyan)
	ColorBlue      = color.New(color.FgBlue)
	ColorGreen     = color.New(color.FgGreen)
	ColorYellow    = color.New(color.FgYellow)
	ColorRed       = color.New(color.FgRed, color.Bold)
	ColorPoli      = color.RGB(255, 127, 127).Add(color.Bold)
	ColorPurple    = color.New(color.FgMagenta)
	ColorBlacklist = color.New(color.FgRed, color.Bold)
	ColorWhite     = color.New(color.FgWhite)
	ColorBold      = color.New(color.Bold)

	ColorBoldCyan   = color.New(color.FgCyan, color.Bold)
	ColorBoldBlue   = color.New(color.FgBlue, color.Bold)
	ColorBoldGreen  = color.New(color.FgGreen, color.Bold)
	ColorBoldYellow = color.New(color.FgYellow, color.Bold)
	ColorBoldRed    = color.New(color.FgRed, color.Bold)
	ColorBoldPurple = color.New(color.FgMagenta, color.Bold)
)

// TrainingSummary is the one-line result of a bogofilter training run, e.g.
// "Successfully trained Bogofilter on 2 new ham message(s). 0 already trained. 0 ignored."
// It is built as one string because the reporter ends every call with a line
// break: printed in pieces, the counts landed on a line of their own.
func TrainingSummary(noun string, trained, already, ignored int) string {
	return fmt.Sprintf("Successfully trained Bogofilter on %s new %s. %s already trained. %s ignored.",
		ColorBoldGreen.Sprintf("%d", trained), noun,
		ColorBoldYellow.Sprintf("%d", already),
		ColorBoldRed.Sprintf("%d", ignored))
}
