package tui

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/sarielhp/mail_cli/cache/msg"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/report"
	"github.com/sarielhp/mail_cli/spam"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// spam menu entries, in display order. The index into this slice is the
// cursor position; executeSpamMenu dispatches on the string so the two never
// drift when an entry is inserted.
const (
	spamMenuSpamCurrent   = "Spam current message"
	spamMenuUnspamCurrent = "Unspam current message"
	spamMenuFlushSpam     = "Move all [SPAM] to Spam folder"
)

func spamMenuItems() []string {
	return []string{spamMenuSpamCurrent, spamMenuUnspamCurrent, spamMenuFlushSpam}
}

// openSpamMenu opens the F5 dialog in the index view.
func (m *tuiModel) openSpamMenu() (tea.Model, tea.Cmd) {
	m.spamMenuOpen = true
	m.spamMenuCursor = 0
	return m, nil
}

// kSpamMenu drives the F5 dialog: up/down move, Enter executes, Esc/q close.
func (m *tuiModel) kSpamMenu(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	items := spamMenuItems()
	switch k.String() {
	case "up", "k":
		m.spamMenuCursor--
		if m.spamMenuCursor < 0 {
			m.spamMenuCursor = len(items) - 1
		}
	case "down", "j":
		m.spamMenuCursor++
		if m.spamMenuCursor >= len(items) {
			m.spamMenuCursor = 0
		}
	case "enter":
		return m.executeSpamMenu()
	case "esc", "q", "f5":
		m.spamMenuOpen = false
	}
	return m, nil
}

func (m *tuiModel) executeSpamMenu() (tea.Model, tea.Cmd) {
	items := spamMenuItems()
	if m.spamMenuCursor < 0 || m.spamMenuCursor >= len(items) {
		return m, nil
	}
	choice := items[m.spamMenuCursor]
	m.spamMenuOpen = false
	switch choice {
	case spamMenuSpamCurrent:
		return m.spamCurrentMessage()
	case spamMenuUnspamCurrent:
		return m.unspamCurrentEmail()
	case spamMenuFlushSpam:
		return m.moveAllSpam()
	}
	return m, nil
}

// spamCurrentMessage trains the current message as spam and records it as
// [SPAM] locally, without moving it. The badge appears in place; a later
// "move all [SPAM]" relocates every such message in one pass.
func (m *tuiModel) spamCurrentMessage() (tea.Model, tea.Cmd) {
	targetID := m.currentEmailID()
	if targetID == "" {
		return m, nil
	}
	if m.isReadOnly() {
		m.err = fmt.Errorf("🔒 Read-Only Mode: Action simulated (no server changes)")
		m.showError = true
		return m, nil
	}
	client := m.client
	if client == nil || client.Config() == nil {
		m.err = fmt.Errorf("mail client not configured")
		m.showError = true
		return m, nil
	}
	m.setSpamBadge(targetID, true)
	rep := report.Or(m.rep)
	cfg := client.Config()
	m.saveStatus = "Learned as spam; marked [SPAM]"
	m.saveStatusTime = time.Now()
	return m, func() tea.Msg {
		err := spam.ClassifyAsSpam(rep, client, cfg, string(targetID))
		return spamClassifiedMsg{id: targetID, err: err}
	}
}

// setSpamBadge sets the [SPAM] flag on a message in both the raw and the
// displayed lists.
func (m *tuiModel) setSpamBadge(id email.ID, on bool) {
	for i, e := range m.rawEmails {
		if e.ID == id {
			m.rawEmails[i].IsSpam = on
		}
	}
	for i, e := range m.emails {
		if e.ID == id {
			m.emails[i].IsSpam = on
		}
	}
}

// spamIDs returns the IDs of every message in the current folder carrying the
// [SPAM] classification, in list order so the move is deterministic. It is the
// set "Move all [SPAM]" acts on; there is no session-level marking.
func (m *tuiModel) spamIDs() []email.ID {
	var ids []email.ID
	for _, e := range m.rawEmails {
		if e.IsSpam {
			ids = append(ids, e.ID)
		}
	}
	return ids
}

func (m *tuiModel) spamCount() int {
	return len(m.spamIDs())
}

// moveAllSpam relocates every [SPAM]-classified message in the current folder
// to the spam folder. Bogofilter has already learned them, so this only moves.
func (m *tuiModel) moveAllSpam() (tea.Model, tea.Cmd) {
	ids := m.spamIDs()
	if len(ids) == 0 {
		m.saveStatus = "No [SPAM] messages in this folder"
		m.saveStatusTime = time.Now()
		return m, nil
	}
	if m.isReadOnly() {
		m.err = fmt.Errorf("🔒 Read-Only Mode: Action simulated (no server changes)")
		m.showError = true
		return m, nil
	}
	if m.client == nil || m.client.Config() == nil {
		m.err = fmt.Errorf("mail client not configured")
		m.showError = true
		return m, nil
	}
	m.showLoad = "Moving [SPAM] messages..."
	m.saveStatus = fmt.Sprintf("Moving %d [SPAM] message(s) to Spam...", len(ids))
	m.saveStatusTime = time.Now()

	client := m.client
	rep := report.Or(m.rep)
	return m, func() tea.Msg {
		cfg := client.Config()
		failed := map[email.ID]error{}
		for _, id := range ids {
			ok, err := msg.Exists(cfg.AccountCache, id)
			if err == nil && !ok {
				err = fmt.Errorf("message %s is not in the local cache", id)
			}
			if err == nil {
				err = spam.MoveToSpamByID(rep, client, cfg, string(id))
			}
			if err != nil {
				slog.Warn("moveAllSpam: move failed", slog.String("id", string(id)), slog.Any("error", err))
				failed[id] = err
			}
		}
		return markedSpamDoneMsg{ids: ids, failed: failed}
	}
}

// renderSpamMenu draws the F5 dialog over a grayed-out screen. Each entry is
// on its own line; the selected one carries a "▶ " marker and the flush entry
// shows how many messages are currently marked.
func renderSpamMenu(m *tuiModel) string {
	items := spamMenuItems()
	dialogWidth := min(76, max(52, m.width-4))
	inner := dialogWidth - 4 // accounting for Padding(1, 2)

	var content strings.Builder
	content.WriteString("\n")
	content.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#ffaa00")).
		Width(inner).Align(lipgloss.Center).Render("SPAM ACTIONS"))
	content.WriteString("\n\n")

	for i, item := range items {
		marked := ""
		if item == spamMenuFlushSpam {
			marked = fmt.Sprintf("  (%d [SPAM])", m.spamCount())
		}
		if i == m.spamMenuCursor {
			content.WriteString(rowZone(frameSpam, i, 0, lipgloss.NewStyle().
				Foreground(lipgloss.Color("#55ff55")).Bold(true).
				Render(" ▶ "+item+marked)))
			content.WriteString("\n")
		} else {
			content.WriteString(rowZone(frameSpam, i, 0, lipgloss.NewStyle().
				Foreground(lipgloss.Color("#cccccc")).
				Render("   "+item+marked)))
			content.WriteString("\n")
		}
	}

	content.WriteString("\n")
	content.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")).
		Render(markHints("  Enter: run    ↑/↓: select    Esc: close", hint{"Enter: run", "enter"}, hint{"Esc: close", "esc"})))

	dialogBox := lipgloss.NewStyle().
		Background(lipgloss.Color("#16162a")).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#ffaa00")).
		Padding(1, 2).
		Width(dialogWidth).
		Render(content.String())

	return renderDimmedOverlay(m.width, m.height, m.renderBaseScreen(), dialogBox)
}
