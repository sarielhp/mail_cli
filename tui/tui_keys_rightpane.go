package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

func (m *tuiModel) kRightPane(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := k.String()
	switch {
	case k.Type == tea.KeyUp || s == "k":
		m.rightVp.ScrollUp(1)
	case k.Type == tea.KeyDown || s == "j":
		m.rightVp.ScrollDown(1)
	case s == "pageup" || s == "pgup":
		m.rightVp.ScrollUp(m.rightVp.Height)
	case s == "pagedown" || s == "pgdown" || s == "pgdn":
		m.rightVp.ScrollDown(m.rightVp.Height)
	case s == "home":
		m.rightVp.GotoTop()
	case s == "end":
		m.rightVp.GotoBottom()
	case s == "esc" || s == "q" || s == "p" || s == "P":
		m.rightPaneFocus = false
		m.hideRightPane = true
		m.detailVpDirty = true
		return m, nil
	case s == "h" || s == "left" || s == "tab":
		m.rightPaneFocus = false
		return m, nil
	case s == "i":
		m.toggleInvertColors()
		return m, nil
	case s == "enter" || s == "o" || s == "x":
		m.openAttachmentExternally(m.attCursor, "Opened: ", "Error: ")
		return m, nil
	case s == "a" || s == "v":
		m.rightPaneFocus = false
		m.attachmentDrawer = true
		m.attCursor = 0
		m.detailVpDirty = true
		if m.attachmentLineOffset > 0 {
			m.vp.SetYOffset(max(0, m.attachmentLineOffset-2))
		}
		return m, nil
	}
	return m, nil
}
