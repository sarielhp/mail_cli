package tui

import (
	"strconv"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	zone "github.com/lrstanley/bubblezone"
)

// The mouse.
//
// It is captured from the start, and F9 lets it go: while the TUI holds the
// mouse the terminal cannot select text with a plain drag (Shift-drag still
// can, in most terminals). F9 because F1–F5 are taken, F10 opens the menu bar
// of several terminals, F11 is full screen and F12 drops down Guake and
// Yakuake.
//
// A click does what a key does. Clickable text is marked with the key it
// stands for (keyZone), and a click on it is handed to handleKey as that key,
// so the behaviour of every control has one implementation. The exceptions
// are what a key cannot say: which attachment row was clicked, and Send,
// which a stray click must not do without asking.

func init() { zone.NewGlobal() }

// terminalReturned marks the message that reports an external program --
// the editor, "account login" -- has handed the terminal back. bubbletea
// restores the alternate screen and bracketed paste then, but not mouse
// reporting, so every return from the editor left the mouse dead until F9
// was pressed twice. Update takes the mouse back on any such message.
type terminalReturned interface{ terminalReturned() }

func (composeEditorFinishedMsg) terminalReturned() {}
func (accountLoginFinishedMsg) terminalReturned()  {}

// retakeMouse adds taking the mouse back to cmd, after an external program,
// unless F9 has let it go.
func (m *tuiModel) retakeMouse(msg tea.Msg, cmd tea.Cmd) tea.Cmd {
	if _, ok := msg.(terminalReturned); !ok || m.mouseOff {
		return cmd
	}
	return tea.Batch(cmd, tea.EnableMouseCellMotion)
}

const mouseToggleKey = "f9"

// mouseOffHint is shown, while the mouse is off, where the screen says what
// state it is in: off is the state that can puzzle, and on is the default.
const mouseOffHint = "mouse off (F9)"

// toggleMouse lets the mouse go, or takes it back.
func (m *tuiModel) toggleMouse() (tea.Model, tea.Cmd) {
	m.mouseOff = !m.mouseOff
	m.saveStatusTime = time.Now()
	if m.mouseOff {
		m.saveStatus = "Mouse off: the terminal selects text again (F9 turns it back on)"
		return m, tea.DisableMouse
	}
	m.saveStatus = "Mouse on (F9 turns it off; Shift-drag still selects text)"
	return m, tea.EnableMouseCellMotion
}

// keyZone marks rendered text at place as standing for the key k. The place
// keeps zone names unique: the Subject row and the footer's [s] are both s,
// and a zone manager keeps one zone per name -- the footer, drawn later, used
// to replace the row, so clicking the row did nothing.
func keyZone(k, place, rendered string) string { return zone.Mark(keyZoneID(k, place), rendered) }

func keyZoneID(k, place string) string { return "key:" + k + "@" + place }

// Where keyZone marks text.
const (
	zoneRow    = "row"
	zoneFooter = "footer"
	zoneDialog = "dialog"
)

var zonePlaces = []string{zoneRow, zoneFooter, zoneDialog}

// attZone marks the row of attachment i.
func attZone(i int, rendered string) string { return zone.Mark("att:"+strconv.Itoa(i), rendered) }

// composeSendZone is the Send control, which asks before sending.
const composeSendZone = "send"

// composeKeyZones are the keys the review screen and its dialogs mark.
var composeKeyZones = []string{"t", "c", "b", "s", "e", "a", "d", "v", "f3", "f5", "q", "x", "y", "n", composeSendZone}

// handleMouse turns a mouse event into what it stands for: the wheel scrolls,
// a left click on marked text acts. Clicks act on release, as buttons do.
func (m *tuiModel) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.mouseOff {
		return m, nil
	}
	if m.fileBrowserOpen && !m.showHelp {
		newFb, cmd := m.fileBrowser.Mouse(msg)
		return m.afterFileBrowser(newFb, cmd)
	}
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		return m.wheel(-1)
	case tea.MouseButtonWheelDown:
		return m.wheel(1)
	case tea.MouseButtonLeft:
		if msg.Action == tea.MouseActionRelease {
			return m.click(msg)
		}
	}
	return m, nil
}

// wheel scrolls what is under the eye: the message body on the review
// screen, and otherwise whatever the arrow keys move.
func (m *tuiModel) wheel(dir int) (tea.Model, tea.Cmd) {
	if m.composeScreenShowing() {
		if dir < 0 {
			m.compose.Vp.ScrollUp(3)
		} else {
			m.compose.Vp.ScrollDown(3)
		}
		return m, nil
	}
	if dir < 0 {
		return m.handleKey(tea.KeyMsg{Type: tea.KeyUp})
	}
	return m.handleKey(tea.KeyMsg{Type: tea.KeyDown})
}

// click acts on what is under a left click: marked text on the review
// screen, a folder in the tree, a message in the index, or a link.
func (m *tuiModel) click(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	switch m.frameKind {
	case frameCompose:
		if m.composeScreenShowing() {
			return m.clickCompose(msg)
		}
		return m, nil
	case frameTree:
		return m.clickTree(msg)
	case frameBase:
		return m.clickBase(msg)
	}
	return m.clickOverlay(m.frameKind, msg)
}

func (m *tuiModel) clickCompose(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	for i := range m.compose.Attachments {
		if zone.Get("att:" + strconv.Itoa(i)).InBounds(msg) {
			return m.clickAttachment(i)
		}
	}
	for _, k := range composeKeyZones {
		for _, place := range zonePlaces {
			if zone.Get(keyZoneID(k, place)).InBounds(msg) {
				return m.clickKey(k)
			}
		}
	}
	return m, nil
}

// clickKey presses k. A header field being typed into is saved first, as
// Enter would save it: the click is the user moving on.
func (m *tuiModel) clickKey(k string) (tea.Model, tea.Cmd) {
	if m.compose.FieldEditing != fieldNone {
		m.commitComposeField()
	}
	if k == composeSendZone {
		m.compose.SendConfirm = true
		return m, nil
	}
	return m.handleKey(keyMsg(k))
}

// clickAttachment selects attachment i; a click on the selected one opens it.
func (m *tuiModel) clickAttachment(i int) (tea.Model, tea.Cmd) {
	if m.compose.FieldEditing != fieldNone {
		m.commitComposeField()
	}
	if m.compose.AttCursor == i {
		return m.handleKey(keyMsg("v"))
	}
	m.compose.AttCursor = i
	return m, nil
}

// composeScreenShowing reports whether the review screen, or one of its
// own dialogs, is what is on screen: nothing drawn over it, so a click lands
// on what the user sees.
func (m *tuiModel) composeScreenShowing() bool {
	return m.mode == modeCompose && m.compose != nil && !m.compose.sending &&
		!m.showHelp && !m.fileBrowserOpen && !m.attModalOpen &&
		m.contactPick == nil && !m.accountOverlayOpen
}

// keyMsg is the key event for k, as the keyboard would deliver it.
func keyMsg(k string) tea.KeyMsg {
	switch k {
	case "f3":
		return tea.KeyMsg{Type: tea.KeyF3}
	case "f5":
		return tea.KeyMsg{Type: tea.KeyF5}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
}
