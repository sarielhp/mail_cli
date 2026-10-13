package tui

import (
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/sarielhp/mail_cli/browser"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	zone "github.com/lrstanley/bubblezone"
)

// Clicks on the index, the folder tree and the reader. As on the review
// screen, a click on a row selects it and a click on the selected row opens
// it -- the same thing Enter does, through the same code -- so two quick
// clicks open what was clicked.

// indexZone marks the index row of message i.
func indexZone(i int, rendered string) string { return zone.Mark("idx:"+strconv.Itoa(i), rendered) }

// treeZone marks the folder tree row of entry i.
func treeZone(i int, rendered string) string { return zone.Mark("tree:"+strconv.Itoa(i), rendered) }

// clickTree selects the clicked folder, or opens the selected one.
func (m *tuiModel) clickTree(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	for i := range m.entries {
		if !zone.Get("tree:" + strconv.Itoa(i)).InBounds(msg) {
			continue
		}
		if i == m.cursor {
			return m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
		}
		m.cursor = i
		return m, nil
	}
	return m, nil
}

// clickBase acts on the index or the reader with nothing drawn over them: a
// message row, or else a link.
func (m *tuiModel) clickBase(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	// The main menu is drawn beside the index, not over it.
	if m.menuOpen {
		for i := range m.menuItems {
			if zone.Get(rowZoneID(frameBase, i, 0)).InBounds(msg) {
				return m.clickOverlayRow(frameBase, i)
			}
		}
	}
	for i := range m.emails {
		if zone.Get("idx:" + strconv.Itoa(i)).InBounds(msg) {
			return m.clickMessage(i)
		}
	}
	if link := linkAt(m.lastFrame, msg.X, msg.Y); link != "" {
		return m.openLink(link)
	}
	return m, nil
}

// clickMessage selects message i, or opens it when it is already selected.
// In the reader the rows above the body are the neighbouring messages, and a
// click on one reads it.
func (m *tuiModel) clickMessage(i int) (tea.Model, tea.Cmd) {
	switch {
	case i == m.eIdx && m.mode == modeDetail:
		return m, nil
	case i == m.eIdx:
		return m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	case m.mode == modeDetail:
		m.eIdx = i
		m.selectedID = m.emails[i].ID
		return m.kIndexOpenDetail(tea.KeyMsg{})
	case m.splitPreview:
		return m.navigateIndexPreview(i - m.eIdx)
	}
	m.moveIndexSelection(i - m.eIdx)
	return m, nil
}

// openLink opens a link from a message in the browser. A message is written
// by a stranger, so only the schemes a link in a mail is for are followed:
// anything else -- file:, a desktop handler -- is not opened on a click.
func (m *tuiModel) openLink(link string) (tea.Model, tea.Cmd) {
	m.saveStatusTime = time.Now()
	u, err := url.Parse(link)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "mailto") {
		m.saveStatus = "Not opening a link of that kind: " + link
		return m, nil
	}
	if cfg := m.Config(); cfg != nil && cfg.Browser != "" {
		err = browser.OpenWith(cfg.Browser, link)
	} else {
		err = browser.Open(link)
	}
	if err != nil {
		m.saveStatus = "Could not open the link: " + err.Error()
		return m, nil
	}
	m.saveStatus = "Opened " + link
	return m, nil
}

// linkAt returns the target of the OSC 8 hyperlink drawn at cell (x, y) of
// frame, or "". The reader draws links that way, and while the TUI holds the
// mouse the terminal hands their clicks to it instead of opening them.
func linkAt(frame string, x, y int) string {
	lines := strings.Split(frame, "\n")
	if y < 0 || y >= len(lines) {
		return ""
	}
	line := lines[y]
	target, col := "", 0
	for i := 0; i < len(line); {
		if line[i] == 0x1b && i+1 < len(line) {
			next, osc := skipEscape(line, i)
			if t, ok := strings.CutPrefix(osc, "8;"); ok {
				_, target, _ = strings.Cut(t, ";")
			}
			i = next
			continue
		}
		r, size := utf8.DecodeRuneInString(line[i:])
		w := ansi.StringWidth(string(r))
		if x >= col && x < col+w {
			return target
		}
		col += w
		i += size
	}
	return ""
}

// skipEscape returns where the escape sequence at line[i] ends, and its body
// when it is an OSC (ESC ] ... BEL, or ESC ] ... ESC \).
func skipEscape(line string, i int) (next int, osc string) {
	switch line[i+1] {
	case ']':
		for j := i + 2; j < len(line); j++ {
			if line[j] == 0x07 {
				return j + 1, line[i+2 : j]
			}
			if line[j] == 0x1b && j+1 < len(line) && line[j+1] == '\\' {
				return j + 2, line[i+2 : j]
			}
		}
		return len(line), ""
	case '[':
		j := i + 2
		for j < len(line) && (line[j] < 0x40 || line[j] > 0x7e) {
			j++
		}
		return j + 1, ""
	}
	return i + 2, ""
}
