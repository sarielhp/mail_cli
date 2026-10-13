package tui

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sarielhp/mail_cli/email"
)

type messageReadyForPipeMsg struct {
	id  email.ID
	raw []byte
	err error
	cmd string
}

type pipeFinishedMsg struct {
	err error
	cmd string
}

func (m *tuiModel) kPipePrompt(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	if cmdStr, ok := m.kPromptLine(k, &m.pipeCommand, &m.inPipePrompt); ok {
		return m.startPipeAction(cmdStr)
	}
	return m, nil
}

// kPromptLine applies one key to a single-line prompt buffer. On Enter it
// returns the trimmed, non-empty text with ok=true; Escape clears the buffer
// and deactivates the prompt. ok is false for every other key and empty submit.
func (m *tuiModel) kPromptLine(k tea.KeyMsg, query *string, active *bool) (string, bool) {
	s := k.String()
	switch {
	case k.Type == tea.KeyEsc:
		*active = false
		*query = ""
	case k.Type == tea.KeyEnter:
		*active = false
		q := strings.TrimSpace(*query)
		*query = ""
		if q == "" {
			return "", false
		}
		return q, true
	case k.Type == tea.KeyBackspace || s == "backspace" || s == "ctrl+h":
		if len(*query) > 0 {
			*query = (*query)[:len(*query)-1]
		}
	case k.Type == tea.KeySpace:
		*query += " "
	default:
		if len(k.Runes) > 0 {
			*query += string(k.Runes)
		}
	}
	return "", false
}

func (m *tuiModel) startPipeAction(cmdStr string) (tea.Model, tea.Cmd) {
	targetID := m.currentEmailID()
	if targetID == "" || m.client == nil {
		m.err = fmt.Errorf("no email selected to pipe")
		m.showError = true
		return m, nil
	}

	client := m.client
	return m, func() tea.Msg {
		raw, err := client.GetRawMessage(targetID)
		if err != nil {
			return messageReadyForPipeMsg{err: fmt.Errorf("retrieving message: %w", err), cmd: cmdStr}
		}
		return messageReadyForPipeMsg{id: targetID, raw: raw, cmd: cmdStr}
	}
}

func (m *tuiModel) handleMessageReadyForPipe(msg messageReadyForPipeMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.err = msg.err
		m.showError = true
		return m, nil
	}

	var c *exec.Cmd
	if strings.ContainsAny(msg.cmd, " \t\n|&;<>()$`\\\"'") {
		c = exec.Command("sh", "-c", msg.cmd)
	} else {
		parts := strings.Fields(msg.cmd)
		c = exec.Command(parts[0], parts[1:]...)
	}
	c.Stdin = bytes.NewReader(msg.raw)

	return m, tea.ExecProcess(c, func(err error) tea.Msg {
		return pipeFinishedMsg{err: err, cmd: msg.cmd}
	})
}
