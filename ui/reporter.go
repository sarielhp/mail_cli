package ui

import (
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/sarielhp/mail_cli/app"
	"github.com/sarielhp/mail_cli/report"
)

// TerminalReporter writes progress to a terminal, with the markers and colours
// the CLI has always used, so output is unchanged from when the backends
// printed for themselves.
type TerminalReporter struct {
	w io.Writer
}

// NewTerminalReporter writes to w. A nil w means os.Stdout.
func NewTerminalReporter(w io.Writer) *TerminalReporter {
	if w == nil {
		w = os.Stdout
	}
	return &TerminalReporter{w: w}
}

func (t *TerminalReporter) line(prefix, format string, a ...any) {
	fmt.Fprintf(t.w, "%s %s\n", prefix, strings.TrimRight(fmt.Sprintf(format, a...), "\n"))
}

// Infof writes an informational line, prefixed with the "[*]" marker.
func (t *TerminalReporter) Infof(format string, a ...any) {
	t.line(app.PrefixInfo, format, a...)
}

// Successf writes a success line, prefixed with the "[+]" marker.
func (t *TerminalReporter) Successf(format string, a ...any) {
	t.line(app.PrefixSuccess, format, a...)
}

// Warnf writes a warning line, prefixed with the "[!]" marker.
func (t *TerminalReporter) Warnf(format string, a ...any) {
	t.line(app.PrefixWarn, format, a...)
}

// Errorf writes an error line, prefixed with the "[!]" marker.
func (t *TerminalReporter) Errorf(format string, a ...any) {
	t.line(app.PrefixError, format, a...)
}

// Printf writes format verbatim, adding no marker and no trailing line break.
func (t *TerminalReporter) Printf(format string, a ...any) {
	fmt.Fprintf(t.w, format, a...)
}

// CanPrompt reports whether this reporter writes directly to an interactive terminal
// where a user can view multi-line prompts and respond to browser sign-in flows.
func (t *TerminalReporter) CanPrompt() bool {
	if t == nil || t.w == nil {
		return false
	}
	if f, ok := t.w.(*os.File); ok {
		return term.IsTerminal(int(f.Fd()))
	}
	return false
}

// progressWidth is the bar's inner width in cells.
const progressWidth = 30

// Progress draws a bar in place, returning to the start of the line each time
// and breaking the line only once the job is done.
func (t *TerminalReporter) Progress(current, total int, label string) {
	if total <= 0 {
		return
	}

	percent := float64(current) / float64(total)
	filled := int(float64(progressWidth) * percent)

	var sb strings.Builder
	sb.WriteString("\r" + label + " [")
	sb.WriteString(strings.Repeat("█", filled))
	sb.WriteString(strings.Repeat("░", progressWidth-filled))
	fmt.Fprintf(&sb, "] %.0f%% (%d/%d)", percent*100, current, total)
	if current >= total {
		sb.WriteString("\n")
	}

	fmt.Fprint(t.w, sb.String())
	if f, ok := t.w.(*os.File); ok {
		_ = f.Sync()
	}
}

// quietReporter passes warnings and errors through and drops everything else.
// `--quiet` used to be checked at each call site; this puts it in one place.
type quietReporter struct {
	Inner report.Reporter
}

func (q quietReporter) Infof(string, ...any)      {}
func (q quietReporter) Successf(string, ...any)   {}
func (q quietReporter) Printf(string, ...any)     {}
func (q quietReporter) Progress(int, int, string) {}

func (q quietReporter) Warnf(format string, a ...any) {
	report.Or(q.Inner).Warnf(format, a...)
}

func (q quietReporter) Errorf(format string, a ...any) {
	report.Or(q.Inner).Errorf(format, a...)
}
