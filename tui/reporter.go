package tui

import (
	"fmt"
	"time"

	"github.com/sarielhp/mail_cli/report"

	tea "github.com/charmbracelet/bubbletea"
)

// ReportMsg is progress from the layers below, delivered as a bubbletea message
// so the TUI can show it instead of having it written over the display.
type ReportMsg struct {
	Level ReportLevel
	Text  string

	// Current, Total and Label are set for Progress messages; Total is 0
	// otherwise.
	Current int
	Total   int
	Label   string
}

// ReportLevel is how prominent a reported line is.
type ReportLevel int

const (
	reportInfo ReportLevel = iota
	reportSuccess
	reportWarn
	reportError
	reportPlain
	reportProgress
)

// ProgramReporter turns reports into bubbletea messages.
//
// This is the point of the whole exercise: the same backend call that prints a
// progress bar on the CLI now feeds the TUI's status line, where before it
// either corrupted the display or was suppressed by a global flag.
type ProgramReporter struct {
	Send func(msg any)
}

// NewProgramReporter sends through send, which is typically a Program's Send.
func NewProgramReporter(send func(msg any)) *ProgramReporter {
	return &ProgramReporter{Send: send}
}

func (p *ProgramReporter) emit(level ReportLevel, format string, a ...any) {
	if p == nil || p.Send == nil {
		return
	}
	p.Send(ReportMsg{Level: level, Text: fmt.Sprintf(format, a...)})
}

// Infof reports an informational line to the TUI's status area; it is a no-op
// when the reporter's Send is nil.
func (p *ProgramReporter) Infof(format string, a ...any) { p.emit(reportInfo, format, a...) }

// Successf reports a success line to the TUI's status area.
func (p *ProgramReporter) Successf(format string, a ...any) { p.emit(reportSuccess, format, a...) }

// Warnf reports a warning line to the TUI's status area.
func (p *ProgramReporter) Warnf(format string, a ...any) { p.emit(reportWarn, format, a...) }

// Errorf reports an error line to the TUI's status area.
func (p *ProgramReporter) Errorf(format string, a ...any) { p.emit(reportError, format, a...) }

// Printf reports a plain line, with no status prefix, to the TUI's status area.
func (p *ProgramReporter) Printf(format string, a ...any) { p.emit(reportPlain, format, a...) }

// Progress reports a progress update with its current and total counts to the
// TUI; it is a no-op when the reporter's Send is nil.
func (p *ProgramReporter) Progress(current, total int, label string) {
	if p == nil || p.Send == nil {
		return
	}
	p.Send(ReportMsg{
		Level:   reportProgress,
		Current: current,
		Total:   total,
		Label:   label,
		Text:    fmt.Sprintf("%s (%d/%d)", label, current, total),
	})
}

var _ report.Reporter = (*ProgramReporter)(nil)

// handleReport puts a reported line in the status area. Backends used to write
// these straight to the terminal, which is why the TUI had to mute them.
func (m *tuiModel) handleReport(msg ReportMsg) (tea.Model, tea.Cmd) {
	if msg.Text == "" {
		return m, nil
	}
	m.saveStatus = msg.Text
	m.saveStatusTime = time.Now()
	return m, nil
}
