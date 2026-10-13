package tui

import (
	"github.com/sarielhp/mail_cli/label"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *tuiModel) kFolder(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.inFolderSearch {
		return m.kFolderSearch(k)
	}
	s := k.String()
	switch {
	case k.Type == tea.KeyUp || s == "k":
		m.cursor--
		if m.cursor < 0 {
			m.cursor = 0
		}
	case k.Type == tea.KeyDown || s == "j":
		m.cursor++
		if m.cursor >= len(m.entries) {
			m.cursor = len(m.entries) - 1
		}
	case s == "pageup" || s == "pgup":
		m.cursor -= m.pageStep()
		if m.cursor < 0 {
			m.cursor = 0
		}
	case s == "pagedown" || s == "pgdown" || s == "pgdn":
		m.cursor += m.pageStep()
		if m.cursor >= len(m.entries) {
			m.cursor = len(m.entries) - 1
		}
	case s == "home":
		m.cursor = 0
	case s == "end":
		m.cursor = len(m.entries) - 1
	case k.Type == tea.KeyRight || s == "+":
		m.setFolderExpanded(true)
	case k.Type == tea.KeyLeft || s == "-":
		m.setFolderExpanded(false)
	case s == " ":
		m.toggleFolderExpanded()
	case k.Type == tea.KeyTab || s == "tab":
		m.cycleFolderExpansion()
	case k.Type == tea.KeyEnter:
		return m.selectFolderEntry()
	case s == "/":
		m.inFolderSearch = true
		m.folderSearch = ""
		m.buildEntries()
	case s == "f" || s == "esc":
		m.treeOpen = false
		m.savingMessageToFolder = false
		m.bulkMovingToFolder = false
		m.savingMessageID = ""
		m.savingMessageSourceFolder = ""
	default:
		return m, nil
	}
	return m, nil
}

func (m *tuiModel) kFolderSearch(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := k.String()
	switch {
	case k.Type == tea.KeyEsc:
		m.inFolderSearch = false
		m.folderSearch = ""
		m.buildEntries()
	case k.Type == tea.KeyEnter:
		return m.selectFolderEntry()
	case k.Type == tea.KeyUp:
		m.cursor--
		if m.cursor < 0 {
			m.cursor = 0
		}
	case k.Type == tea.KeyDown:
		m.cursor++
		if m.cursor >= len(m.entries) {
			m.cursor = len(m.entries) - 1
		}
	case k.Type == tea.KeyBackspace || s == "backspace" || s == "ctrl+h":
		if len(m.folderSearch) > 0 {
			m.folderSearch = m.folderSearch[:len(m.folderSearch)-1]
			m.buildEntries()
		}
	case k.Type == tea.KeySpace:
		m.folderSearch += " "
		m.buildEntries()
	default:
		if len(k.Runes) > 0 {
			m.folderSearch += string(k.Runes)
			m.buildEntries()
		}
	}
	return m, nil
}

func (m *tuiModel) selectFolderEntry() (tea.Model, tea.Cmd) {
	if m.cursor < 0 || m.cursor >= len(m.entries) {
		return m, nil
	}
	n := m.entries[m.cursor]
	if n.Node == nil {
		return m, nil
	}
	// The unified inbox is a view, not a folder: nothing can be filed into it.
	if (m.bulkMovingToFolder || m.savingMessageToFolder) && isUnifiedInbox(n.Node.FullName) {
		return m, nil
	}
	if m.bulkMovingToFolder {
		destFolder := n.Node.FullName
		m.bulkMovingToFolder = false
		m.treeOpen = false
		m.inFolderSearch = false
		m.folderSearch = ""
		return m.executeBulkMoveToFolder(label.FromServer(destFolder))
	}
	if m.savingMessageToFolder {
		destFolder := n.Node.FullName
		targetID := m.savingMessageID
		srcFolder := m.savingMessageSourceFolder
		m.savingMessageToFolder = false
		m.savingMessageID = ""
		m.savingMessageSourceFolder = ""
		m.treeOpen = false
		m.inFolderSearch = false
		m.folderSearch = ""
		return m.saveEmailToFolder(targetID, srcFolder, destFolder)
	}
	m.currentF = n.Node.FullName
	m.treeOpen = false
	m.inFolderSearch = false
	m.folderSearch = ""
	m.mode = modeIndex
	m.loadCachedEmailsForFolder(n.Node.FullName)
	if len(m.emails) == 0 {
		m.showLoad = "Downloading..."
	} else {
		m.showLoad = ""
	}
	return m, m.loadEmailsCmd(n.Node.FullName)
}

func (m *tuiModel) cycleFolderExpansion() {
	if m.cursor < 0 || m.cursor >= len(m.entries) {
		return
	}
	n := m.entries[m.cursor]
	if n.Node == nil {
		return
	}
	targetName := n.Node.FullName
	m.cycleTreeSubtreeExpansion(n.Node)
	m.buildEntries()
	for i, e := range m.entries {
		if e.Node != nil && e.Node.FullName == targetName {
			m.cursor = i
			break
		}
	}
}

func (m *tuiModel) setFolderExpanded(expanded bool) {
	if m.cursor >= 0 && m.cursor < len(m.entries) {
		if n := m.entries[m.cursor]; n.Node != nil {
			n.Node.Expanded = expanded
		}
	}
	m.buildEntries()
}

func (m *tuiModel) toggleFolderExpanded() {
	if m.cursor >= 0 && m.cursor < len(m.entries) {
		if n := m.entries[m.cursor]; n.Node != nil {
			n.Node.Expanded = !n.Node.Expanded
		}
	}
	m.buildEntries()
}
