package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sarielhp/mail_cli/email"
)

func TestPipePromptFlow(t *testing.T) {
	m := &tuiModel{
		mode:     modeIndex,
		emails:   []email.Email{{ID: "msg-123", Subject: "Test email"}},
		eIdx:     0,
		currentF: "INBOX",
	}

	// Press '|' to enter pipe prompt
	mMod, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'|'}})
	tm := mMod.(*tuiModel)
	if cmd != nil {
		t.Errorf("expected nil cmd on pipe prompt open")
	}
	if !tm.inPipePrompt {
		t.Fatalf("expected inPipePrompt to be true")
	}
	if tm.pipeCommand != "" {
		t.Errorf("expected empty pipeCommand, got %q", tm.pipeCommand)
	}

	// Type 'cat'
	for _, r := range "cat" {
		mMod, _ = tm.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		tm = mMod.(*tuiModel)
	}
	if tm.pipeCommand != "cat" {
		t.Errorf("expected pipeCommand='cat', got %q", tm.pipeCommand)
	}

	// Backspace
	mMod, _ = tm.handleKey(tea.KeyMsg{Type: tea.KeyBackspace})
	tm = mMod.(*tuiModel)
	if tm.pipeCommand != "ca" {
		t.Errorf("expected pipeCommand='ca', got %q", tm.pipeCommand)
	}

	// Escape cancels pipe prompt
	mMod, _ = tm.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	tm = mMod.(*tuiModel)
	if tm.inPipePrompt {
		t.Errorf("expected inPipePrompt to be false after Esc")
	}
	if tm.pipeCommand != "" {
		t.Errorf("expected cleared pipeCommand after Esc, got %q", tm.pipeCommand)
	}
}

func TestPipeFinishedMsg(t *testing.T) {
	m := &tuiModel{
		mode: modeIndex,
	}

	// Success message
	mMod, _ := m.Update(pipeFinishedMsg{cmd: "xclip"})
	tm := mMod.(*tuiModel)
	if !strings.Contains(tm.saveStatus, "Piped message to \"xclip\"") {
		t.Errorf("expected saveStatus to mention piped command, got %q", tm.saveStatus)
	}

	// Error message
	mMod, _ = m.Update(pipeFinishedMsg{cmd: "badcmd", err: errTestFailure})
	tm = mMod.(*tuiModel)
	if !tm.showError || tm.err == nil {
		t.Errorf("expected showError=true with non-nil err, got show=%v err=%v", tm.showError, tm.err)
	}
}

var errTestFailure = &customTestErr{msg: "command failed"}

type customTestErr struct{ msg string }

func (e *customTestErr) Error() string { return e.msg }
