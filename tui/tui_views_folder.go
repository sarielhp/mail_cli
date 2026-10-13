package tui

import (
	"fmt"
	"strings"

	"github.com/sarielhp/mail_cli/tui/render"

	"github.com/charmbracelet/lipgloss"
)

// folderVisibleHeight is the rows the tree gets: everything above its key bar.
func folderVisibleHeight(m *tuiModel) int {
	return max(1, m.height-1)
}

func renderFolderTree(m *tuiModel) string {
	if len(m.entries) == 0 {
		return "(no folders)"
	}
	vis := folderVisibleHeight(m)
	start := m.cursor - vis/2
	if start < 0 {
		start = 0
	}
	end := start + vis
	if end > len(m.entries) {
		end = len(m.entries)
		start = end - vis
		if start < 0 {
			start = 0
		}
	}
	var lines []string
	for i := start; i < end; i++ {
		lines = append(lines, treeZone(i, render.MakeLabelLine(m.entries[i], m.cursor == i, m.theme.Theme(), m.width)))
	}
	return strings.Join(lines, "\n")
}

func renderFolderOverlay(m *tuiModel, content string) string {
	// No frame: it ran along the terminal's edges, and at Width(m.width) its
	// right side fell past the last column. The key bar marks the bottom.
	panel := lipgloss.NewStyle().
		Width(m.width).
		Height(folderVisibleHeight(m)).
		Render(content)
	var barText string
	switch {
	case m.savingMessageToFolder && m.inFolderSearch:
		barText = fmt.Sprintf(" Save to Folder Search: %s█  (Esc:Exit search, Enter:Save to folder)", m.folderSearch)
	case m.savingMessageToFolder:
		barText = " [Enter] Save to Folder  [Esc] Cancel  [/] Search  [j/k] Nav"
	case m.inFolderSearch:
		barText = fmt.Sprintf(" Search: %s█  (Esc:Exit search, Arrow keys:Nav, Enter:Select)", m.folderSearch)
	default:
		barText = " F:Close  /:Search  Tab:Subtree  Enter:Select  j/k:Nav  q:Quit"
	}
	bar := lipgloss.NewStyle().Width(m.width).Render(barText)
	return lipgloss.JoinVertical(lipgloss.Bottom, panel, bar)
}

func renderDownloadingDialog(width, height int) string {
	return renderStatusDialog(width, height, " Downloading... ")
}

func renderUnspammingDialog(width, height int) string {
	return renderStatusDialog(width, height, " Unspamming... ")
}

// renderStatusDialog centres a bordered one-line status message over the screen.
func renderStatusDialog(width, height int, message string) string {
	dialog := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#4a9e6e")).
		Padding(1, 3).
		Bold(true).
		Foreground(lipgloss.Color("#ffdd44")).
		Background(lipgloss.Color("#2b2b2b")).
		Render(message)
	return lipgloss.NewStyle().
		Width(width).
		Height(height).
		Align(lipgloss.Center, lipgloss.Center).
		Render(dialog)
}
