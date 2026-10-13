package tui

import (
	"strings"
	"testing"

	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/filebrowser/zonetest"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// mouseComposeModel is the review screen with two attachments, drawn once at
// a known size so its clickable zones are recorded.
func mouseComposeModel(t *testing.T) *tuiModel {
	t.Helper()
	m, _, _ := setupTestComposeModel(t, false)
	m.mode = modeCompose
	m.width, m.height = 100, 40
	m.compose = &composeState{
		AccountName: "Personal",
		From:        "user@personal.com",
		To:          "someone@example.com",
		Subject:     "Clicks",
		Body:        "body",
		Attachments: []email.Attachment{
			{Filename: "a.txt", ContentType: "text/plain", Data: []byte("a")},
			{Filename: "b.txt", ContentType: "text/plain", Data: []byte("b")},
		},
	}
	return m
}

// clickOn draws the screen and releases the left button over the zone id.
// Zones are recorded by a worker goroutine after Scan, so it waits for them.
func clickOn(t *testing.T, m *tuiModel, id string) *tuiModel {
	t.Helper()
	z := zonetest.WaitForZone(t, func() { _ = m.View() }, id)
	model, _ := m.Update(tea.MouseMsg{X: z.StartX, Y: z.StartY, Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease})
	return model.(*tuiModel)
}

func TestMouseF9Toggles(t *testing.T) {
	m := mouseComposeModel(t)
	model, cmd := m.Update(tea.KeyMsg{Type: tea.KeyF9})
	m = model.(*tuiModel)
	if !m.mouseOff || cmd == nil {
		t.Fatalf("F9 did not let the mouse go: off=%v cmd=%v", m.mouseOff, cmd)
	}
	if !strings.Contains(ansi.Strip(m.View()), mouseOffHint) {
		t.Errorf("the review screen does not say the mouse is off")
	}
	// While it is off, a click does nothing.
	m = clickOn(t, m, keyZoneID("t", zoneRow))
	if m.compose.FieldEditing != fieldNone {
		t.Errorf("a click acted while the mouse was off")
	}
	model, cmd = m.Update(tea.KeyMsg{Type: tea.KeyF9})
	if m = model.(*tuiModel); m.mouseOff || cmd == nil {
		t.Fatalf("F9 did not take the mouse back: off=%v", m.mouseOff)
	}
}

func TestMouseClickEditsAField(t *testing.T) {
	m := clickOn(t, mouseComposeModel(t), keyZoneID("s", zoneRow))
	if m.compose.FieldEditing != fieldSubject {
		t.Fatalf("clicking Subject: editing %v, want the subject", m.compose.FieldEditing)
	}
	// Clicking another field saves the one being typed, as Enter would.
	m.compose.FieldInput = "Typed subject"
	m = clickOn(t, m, keyZoneID("t", zoneRow))
	if m.compose.Subject != "Typed subject" || m.compose.FieldEditing != fieldTo {
		t.Errorf("moving on by click: subject %q, editing %v", m.compose.Subject, m.compose.FieldEditing)
	}
}

func TestMouseClickSelectsThenOpensAnAttachment(t *testing.T) {
	m := clickOn(t, mouseComposeModel(t), "att:1")
	if m.compose.AttCursor != 1 {
		t.Fatalf("first click: cursor %d, want 1", m.compose.AttCursor)
	}
	m = clickOn(t, m, "att:1")
	if !m.attModalOpen {
		t.Errorf("second click on the selected attachment did not open it")
	}
}

// A stray click must not send: Send asks, and only y answers it.
func TestMouseClickedSendAsksFirst(t *testing.T) {
	m := clickOn(t, mouseComposeModel(t), keyZoneID(composeSendZone, zoneFooter))
	if !m.compose.SendConfirm || m.compose.sending {
		t.Fatalf("clicked Send: confirm=%v sending=%v; want the question and nothing sent", m.compose.SendConfirm, m.compose.sending)
	}
	m = clickOn(t, m, keyZoneID("n", zoneDialog))
	if m.compose.SendConfirm || m.compose.sending {
		t.Errorf("Cancel: confirm=%v sending=%v", m.compose.SendConfirm, m.compose.sending)
	}
}

// A zone must sit where its text is drawn. The rows and the footer used to
// share zone names (Subject and [s] were both "s"), the footer's replaced the
// row's, and a click on a header row did nothing -- which clicking the zone's
// own coordinates, as the tests above do, cannot notice.
func TestMouseZonesSitOnTheirText(t *testing.T) {
	m := mouseComposeModel(t)
	lines := strings.Split(ansi.Strip(m.View()), "\n")
	for _, c := range []struct{ id, text string }{
		{keyZoneID("f3", zoneRow), "From:"},
		{keyZoneID("t", zoneRow), "To:"},
		{keyZoneID("s", zoneRow), "Subject:"},
		{keyZoneID("a", zoneRow), "Attach:"},
		{keyZoneID("s", zoneFooter), "[s] Subj"},
		{"att:1", "[2] b.txt"},
	} {
		z := zonetest.WaitForZone(t, func() { _ = m.View() }, c.id)
		if z.StartY >= len(lines) || !strings.Contains(lines[z.StartY], c.text) {
			t.Errorf("zone %s is on line %d, which does not show %q", c.id, z.StartY, c.text)
		}
	}
}

// The review screen drew one line more than the terminal had; the renderer cut
// its top line, and every click landed a row off. It fills the terminal
// exactly, title first, and draws no frame along the terminal's edges.
func TestComposeScreenFitsTheTerminal(t *testing.T) {
	for _, h := range []int{24, 34, 40, 60} {
		m := mouseComposeModel(t)
		m.width, m.height = 100, h
		lines := strings.Split(ansi.Strip(m.View()), "\n")
		if len(lines) != h {
			t.Errorf("height %d: the review screen is %d lines", h, len(lines))
		}
		if !strings.Contains(lines[0], "COMPOSE & SEND REVIEW") {
			t.Errorf("height %d: the first line is %q, not the title", h, lines[0])
		}
		for _, edge := range []string{lines[0], lines[len(lines)-1]} {
			if strings.ContainsAny(edge, "┌┐└┘") {
				t.Errorf("height %d: a frame corner on the screen's edge: %q", h, edge)
			}
		}
	}
}

// Whatever a screen draws, zones are recorded on what fits, so a view that
// overflows cannot put them a row off.
func TestClipToHeightKeepsTheBottom(t *testing.T) {
	if got := clipToHeight("a\nb\nc\nd", 2); got != "c\nd" {
		t.Errorf("clipToHeight = %q, want the last two lines", got)
	}
	if got := clipToHeight("a\nb", 5); got != "a\nb" {
		t.Errorf("clipToHeight changed a view that fits: %q", got)
	}
}
