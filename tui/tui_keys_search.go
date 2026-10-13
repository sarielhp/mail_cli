package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

func (m *tuiModel) kSearch(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := k.String()
	switch {
	case k.Type == tea.KeyEsc:
		m.inSearch = false
		m.searchQuery = ""
		if m.fuzzySearch {
			m.fuzzySearch = false
			if m.fuzzyGlobal && m.savedRawEmails != nil {
				m.rawEmails = m.savedRawEmails
				m.savedRawEmails = nil
				m.currentF = m.savedCurrentF
			}
			m.fuzzyGlobal = false
		}
		m.rebuildVisibleEmails()
		if len(m.emails) > 0 {
			m.eIdx = min(m.eIdx, len(m.emails)-1)
			m.selectedID = m.emails[m.eIdx].ID
		} else {
			m.eIdx = 0
			m.selectedID = ""
		}
	case k.Type == tea.KeyTab || s == "tab" || k.Type == tea.KeyEnter:
		m.inSearch = false
		if len(m.emails) > 0 {
			m.eIdx = 0
			m.selectedID = m.emails[m.eIdx].ID
		} else {
			m.eIdx = 0
			m.selectedID = ""
		}
		return m, nil
	case k.Type == tea.KeyBackspace || s == "backspace" || s == "ctrl+h":
		if len(m.searchQuery) > 0 {
			m.searchQuery = m.searchQuery[:len(m.searchQuery)-1]
			m.rebuildVisibleEmails()
			if len(m.emails) > 0 {
				m.eIdx = min(m.eIdx, len(m.emails)-1)
				m.selectedID = m.emails[m.eIdx].ID
			} else {
				m.eIdx = 0
				m.selectedID = ""
			}
		}
	case k.Type == tea.KeySpace:
		m.searchQuery += " "
		m.rebuildVisibleEmails()
		if len(m.emails) > 0 {
			m.eIdx = min(m.eIdx, len(m.emails)-1)
			m.selectedID = m.emails[m.eIdx].ID
		} else {
			m.eIdx = 0
			m.selectedID = ""
		}
	default:
		if len(k.Runes) > 0 {
			m.searchQuery += string(k.Runes)
			m.rebuildVisibleEmails()
			if len(m.emails) > 0 {
				m.eIdx = min(m.eIdx, len(m.emails)-1)
				m.selectedID = m.emails[m.eIdx].ID
			} else {
				m.eIdx = 0
				m.selectedID = ""
			}
		}
	}
	return m, nil
}

func (m *tuiModel) kDetailSearch(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := k.String()
	switch {
	case k.Type == tea.KeyEsc:
		m.inDetailSearch = false
		m.detailSearch = ""
		m.detailVpDirty = true
		return m, nil
	case k.Type == tea.KeyEnter || s == "tab":
		m.inDetailSearch = false
		m.detailVpDirty = true
		return m, nil
	case k.Type == tea.KeyBackspace || s == "backspace" || s == "ctrl+h":
		if len(m.detailSearch) > 0 {
			m.detailSearch = m.detailSearch[:len(m.detailSearch)-1]
			m.detailVpDirty = true
		}
	case k.Type == tea.KeySpace:
		m.detailSearch += " "
		m.detailVpDirty = true
	default:
		if len(k.Runes) > 0 {
			m.detailSearch += string(k.Runes)
			m.detailVpDirty = true
		}
	}
	return m, nil
}
