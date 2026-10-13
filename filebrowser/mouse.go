package filebrowser

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	zone "github.com/lrstanley/bubblezone"
)

// The mouse, as the TUI handles it on its other screens: a click on a row
// selects it, and a click on the selected row opens it -- a directory is
// entered, a file chosen -- so two quick clicks open what was clicked. The
// wheel moves the cursor, and the footer's keys are clickable. A click is
// handed to Update as the key it stands for, so each control has one
// implementation.

// The zone manager is the TUI's; NewGlobal is idempotent, and calling it here
// keeps this package usable on its own, as its tests use it.
func init() { zone.NewGlobal() }

func rowZone(i int) string { return "fb:row:" + strconv.Itoa(i) }

func keyZone(k string) string { return "fb:key:" + k }

// footerItem is one clickable entry of the footer.
type footerItem struct {
	label, key string
}

// footerItems are the actions, by mode; jumpItems the directories 1-4 jump
// to. They are two rows: on one, the save picker's footer was 98 columns and
// wrapped in its box, and the jumps were a "[1-4]" hint nothing could click.
func (m *Model) footerItems() []footerItem {
	if m.AllowFileSelect {
		return []footerItem{
			{"[Enter] Open", "enter"}, {"[Bksp] Up", "backspace"}, {"[:] Path", ":"},
			{"[/] Search", "/"}, {"[Esc] Cancel", "esc"},
		}
	}
	return []footerItem{
		{"[s] Save here", "s"}, {"[Bksp] Up", "backspace"}, {"[:] Path", ":"},
		{"[/] Search", "/"}, {"[n] New folder", "n"}, {"[Esc] Cancel", "esc"},
	}
}

var jumpItems = []footerItem{
	{"[1] Downloads", "1"}, {"[2] ~", "2"}, {"[3] Documents", "3"}, {"[4] /tmp", "4"},
}

// renderFooter draws the two footer rows with each key marked for clicking.
func (m *Model) renderFooter() string {
	width := m.contentWidth()
	return strings.Join(append(packItems(m.footerItems(), " | ", " ", width), packItems(jumpItems, "  ", " Jump: ", width)...), "\n")
}

// packItems lays items out in rows no wider than width, the first row
// starting with lead, never splitting an item: lipgloss wrapped a narrow
// box's footer through the middle of a key hint, and a cut hint cannot be
// clicked.
func packItems(items []footerItem, sep, lead string, width int) []string {
	indent := strings.Repeat(" ", len([]rune(lead)))
	var rows []string
	row, plain, first := lead, lead, true // first: nothing on this row yet
	for _, it := range items {
		if !first && len([]rune(plain+sep+it.label)) > width {
			rows = append(rows, row)
			row, plain, first = indent, indent, true
		}
		if !first {
			row, plain = row+sep, plain+sep
		}
		row += zone.Mark(keyZone(it.key), it.label)
		plain += it.label
		first = false
	}
	return append(rows, row)
}

// contentWidth is the width inside the browser's box (see View).
func (m *Model) contentWidth() int {
	return max(30, min(m.Width-4, 86)) - 2
}

// Mouse handles a mouse event. While a name or a path is being typed it does
// nothing: a click there would leave half-typed input behind.
func (m *Model) Mouse(msg tea.MouseMsg) (Model, tea.Cmd) {
	if m.CreatingDir || m.InSearch || m.EnteringPath {
		return *m, nil
	}
	switch {
	case msg.Button == tea.MouseButtonWheelUp:
		return m.Update(tea.KeyMsg{Type: tea.KeyUp})
	case msg.Button == tea.MouseButtonWheelDown:
		return m.Update(tea.KeyMsg{Type: tea.KeyDown})
	case msg.Button != tea.MouseButtonLeft || msg.Action != tea.MouseActionRelease:
		return *m, nil
	}
	for i := range m.Filtered {
		if zone.Get(rowZone(i)).InBounds(msg) {
			if i == m.Cursor {
				return m.selectCurrentEntry()
			}
			m.Cursor = i
			return *m, nil
		}
	}
	for _, it := range append(m.footerItems(), jumpItems...) {
		if zone.Get(keyZone(it.key)).InBounds(msg) {
			return m.Update(keyMsg(it.key))
		}
	}
	return *m, nil
}

// keyMsg is the key event for k, as the keyboard would deliver it.
func keyMsg(k string) tea.KeyMsg {
	switch k {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
}

// displayDir shows dir in at most width columns, home as "~", and -- when it is
// still too long -- its end, which names the directory the user is in. Cut at
// the right, a deep path showed where it started and hid where it was.
func displayDir(dir string, width int) string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		if dir == home {
			dir = "~"
		} else if rest, ok := strings.CutPrefix(dir, home+string(filepath.Separator)); ok {
			dir = "~" + string(filepath.Separator) + rest
		}
	}
	r := []rune(dir)
	if width < 2 || len(r) <= width {
		return dir
	}
	return "…" + string(r[len(r)-(width-1):])
}
