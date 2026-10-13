package tui

import (
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"
)

// Clicks on the overlays: the dialogs, menus and pickers drawn over the
// index, the reader or the review screen. They follow the rest of the TUI:
//
//   - a row of a list selects on a click and acts on a click on the selected
//     row, as Enter does -- except where Enter commits something a stray
//     click must not (adding a calendar event), and in the contact picker,
//     where a second click marks the contact so several can be picked;
//   - a key hint ("[s] Save", "Esc: close") is that key.
//
// View records which overlay it drew (frameKind), and a click is read against
// that: an overlay drawn over the index leaves the index's rows marked
// underneath, and they must not take the click.

// Overlay kinds, as tryRenderModalOverlay records them in frameKind.
const (
	frameBase      = "base"
	frameCompose   = "compose"
	frameHelp      = "help"
	frameDeadlines = "deadlines"
	frameFiles     = "files"
	frameAttList   = "attlist"
	frameICS       = "ics"
	frameAttModal  = "attmodal"
	frameAttHelp   = "atthelp"
	frameSummary   = "summary"
	frameDiag      = "diag"
	frameAccount   = "account"
	frameContacts  = "contacts"
	frameTree      = "tree"
	frameBlock     = "block"
	frameSpam      = "spam"
	frameLoading   = "loading"
)

// rowZone marks line part (0, 1, ...) of row i of the list in overlay kind. A
// row drawn on several lines marks each of them.
func rowZone(kind string, i, part int, rendered string) string {
	return zone.Mark(rowZoneID(kind, i, part), rendered)
}

func rowZoneID(kind string, i, part int) string {
	return "row:" + kind + ":" + strconv.Itoa(i) + ":" + strconv.Itoa(part)
}

// hintKeyZone marks a key hint as the key k.
func hintKeyZone(k, rendered string) string { return zone.Mark("hint:"+k, rendered) }

// hintKeys are every key a hint stands for, in the order they are looked up.
var hintKeys = []string{
	"enter", "esc", "space", "tab", "f4", "f5",
	"a", "b", "c", "d", "i", "l", "n", "o", "q", "s", "S", "t", "u", "x",
}

// clickOverlay acts on a click on the overlay of kind: a row, or a key hint.
func (m *tuiModel) clickOverlay(kind string, msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	for i := range m.overlayRowCount(kind) {
		for part := range 2 {
			if zone.Get(rowZoneID(kind, i, part)).InBounds(msg) {
				return m.clickOverlayRow(kind, i)
			}
		}
	}
	for _, k := range hintKeys {
		if zone.Get("hint:" + k).InBounds(msg) {
			return m.handleKey(overlayKeyMsg(k))
		}
	}
	return m, nil
}

// overlayRowCount is how many rows the list in overlay kind has.
func (m *tuiModel) overlayRowCount(kind string) int {
	switch kind {
	case frameDeadlines:
		return len(m.deadlinesList)
	case frameAttList:
		return len(m.attachments)
	case frameICS:
		return len(m.icsEvents)
	case frameAccount:
		return len(m.getAccounts())
	case frameSpam:
		return len(spamMenuItems())
	case frameContacts:
		if m.contactPick != nil {
			return len(m.contactPick.results)
		}
	case frameBase:
		if m.menuOpen {
			return len(m.menuItems)
		}
	}
	return 0
}

// clickOverlayRow selects row i of the list in overlay kind, or acts on it
// when it is already selected.
func (m *tuiModel) clickOverlayRow(kind string, i int) (tea.Model, tea.Cmd) {
	enter := tea.KeyMsg{Type: tea.KeyEnter}
	switch kind {
	case frameDeadlines:
		return m.selectOrPress(&m.deadlinesCursor, i, enter)
	case frameAttList:
		if m.attachmentViewerOpen {
			return m, nil
		}
		return m.selectOrPress(&m.attachmentListCursor, i, enter)
	case frameICS:
		// Enter adds the event to the calendar; a click only shows it.
		if i != m.icsEventIdx {
			m.updateICSEventSelection(i)
		}
		return m, nil
	case frameAccount:
		return m.selectOrPress(&m.accountCursor, i, enter)
	case frameSpam:
		return m.selectOrPress(&m.spamMenuCursor, i, enter)
	case frameContacts:
		// Space marks and moves on, so that a second click on each of
		// several contacts picks them all; Enter (its hint) adds them.
		return m.selectOrPress(&m.contactPick.cursor, i, tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}})
	case frameBase:
		return m.selectOrPress(&m.menuCursor, i, enter)
	}
	return m, nil
}

// selectOrPress moves *cursor to i, or presses k when it is already there.
func (m *tuiModel) selectOrPress(cursor *int, i int, k tea.KeyMsg) (tea.Model, tea.Cmd) {
	if *cursor == i {
		return m.handleKey(k)
	}
	*cursor = i
	return m, nil
}

// overlayKeyMsg is the key event for a hint's key.
func overlayKeyMsg(k string) tea.KeyMsg {
	switch k {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "space":
		return tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "f4":
		return tea.KeyMsg{Type: tea.KeyF4}
	case "f5":
		return tea.KeyMsg{Type: tea.KeyF5}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
}

// hint is a key hint's label and the key it stands for.
type hint struct{ label, key string }

// markHints marks, in an overlay's finished hint line s, the first
// occurrence of each label as its key. Working on the finished line leaves
// each overlay's own layout -- widths, truncation, styling -- as it was; a
// label that was cut off is simply not found.
func markHints(s string, hints ...hint) string {
	for _, h := range hints {
		if i := strings.Index(s, h.label); i >= 0 {
			s = s[:i] + hintKeyZone(h.key, h.label) + s[i+len(h.label):]
		}
	}
	return s
}

// fitHints lays hint parts out in rows no wider than width, joined by sep,
// never splitting a hint. A hint line wider than its screen was cut off or
// wrapped through a hint -- at 80 columns the attachment list lost
// everything after "[S] Quick save" -- and a hint not drawn whole cannot
// be read or clicked.
func fitHints(width int, sep string, parts []string) string {
	var rows []string
	row := ""
	for _, p := range parts {
		switch {
		case row == "":
			row = " " + p
		case lipgloss.Width(row+sep+p) <= width:
			row += sep + p
		default:
			rows = append(rows, row)
			row = " " + p
		}
	}
	if row != "" {
		rows = append(rows, row)
	}
	return strings.Join(rows, "\n")
}
