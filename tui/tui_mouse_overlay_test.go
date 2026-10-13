package tui

import (
	"strings"
	"testing"

	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/contacts"
	"github.com/sarielhp/mail_cli/email"

	tea "github.com/charmbracelet/bubbletea"
)

// Each overlay's rows select on a click and act on a second; its key hints
// are the keys. The tests click where the overlay drew the zone, the way
// clickOn does for the review screen.

func TestMouseSpamMenu(t *testing.T) {
	m, _ := setupTestSpamMenuModel(t, false)
	m.spamMenuOpen = true
	m = clickOn(t, m, rowZoneID(frameSpam, 1, 0))
	if m.spamMenuCursor != 1 || !m.spamMenuOpen {
		t.Fatalf("first click: cursor %d open %v, want the second entry selected", m.spamMenuCursor, m.spamMenuOpen)
	}
	m = clickOn(t, m, "hint:esc")
	if m.spamMenuOpen {
		t.Errorf("clicking \"Esc: close\" did not close the menu")
	}
}

func TestMouseDeadlinesAgenda(t *testing.T) {
	m, _ := setupTestSpamMenuModel(t, false)
	m.showDeadlinesAgenda = true
	m.deadlinesList = []deadlineEntry{
		{EmailID: "msg-1", Description: "Grant report", Sender: "a", Subject: "x"},
		{EmailID: "msg-2", Description: "Referee report", Sender: "b", Subject: "y"},
	}
	// The second line of an entry is part of it.
	m = clickOn(t, m, rowZoneID(frameDeadlines, 1, 1))
	if m.deadlinesCursor != 1 {
		t.Fatalf("click: cursor %d, want 1", m.deadlinesCursor)
	}
	m = clickOn(t, m, rowZoneID(frameDeadlines, 1, 0))
	if m.showDeadlinesAgenda || m.selectedID != "msg-2" {
		t.Errorf("second click: agenda open %v, selected %q; want it to jump to msg-2", m.showDeadlinesAgenda, m.selectedID)
	}
}

func TestMouseAttachmentList(t *testing.T) {
	m, _ := setupTestSpamMenuModel(t, false)
	m.attachments = []email.Attachment{
		{Filename: "a.txt", ContentType: "text/plain", Data: []byte("a")},
		{Filename: "b.txt", ContentType: "text/plain", Data: []byte("b")},
	}
	m.attachmentListOpen = true
	m.attachmentDeleted = map[int]bool{} // as opening the list does
	m = clickOn(t, m, rowZoneID(frameAttList, 1, 0))
	if m.attachmentListCursor != 1 {
		t.Fatalf("click: cursor %d, want 1", m.attachmentListCursor)
	}
	m = clickOn(t, m, "hint:d")
	if !m.attachmentDeleted[1] {
		t.Errorf("clicking [d] Delete did not mark the selected attachment")
	}
}

func TestMouseAccountPicker(t *testing.T) {
	m := newTestMultiAccountModel([]cfg_acc.AccountConfig{{Name: "home"}, {Name: "work"}}, "home")
	m.accountOverlayOpen = true
	m.accountCursor = 0
	m = clickOn(t, m, rowZoneID(frameAccount, 1, 0))
	if m.accountCursor != 1 || !m.accountOverlayOpen {
		t.Fatalf("click: cursor %d open %v, want work selected", m.accountCursor, m.accountOverlayOpen)
	}
	m = clickOn(t, m, "hint:esc")
	if m.accountOverlayOpen {
		t.Errorf("clicking \"Esc: Cancel\" did not close the picker")
	}
}

func TestMouseHelpDismiss(t *testing.T) {
	m, _ := setupTestSpamMenuModel(t, false)
	m.showHelp = true
	m = clickOn(t, m, "hint:esc")
	if m.showHelp {
		t.Errorf("clicking the dismiss line did not close help")
	}
}

func TestMouseMainMenu(t *testing.T) {
	m, _ := setupTestSpamMenuModel(t, false)
	m.menuOpen = true
	m.menuCursor = 0
	m = clickOn(t, m, rowZoneID(frameBase, 2, 0))
	if m.menuCursor != 2 || !m.menuOpen {
		t.Errorf("click: cursor %d open %v, want the third item selected", m.menuCursor, m.menuOpen)
	}
}

// In the contact picker a second click marks, so several can be picked; the
// "Enter: add" hint finishes.
func TestMouseContactPicker(t *testing.T) {
	m := mouseComposeModel(t)
	m.contactPick = &contactPicker{
		results: []contacts.ScoredContact{
			{Contact: contacts.Contact{Name: "Alice", Emails: []string{"alice@example.org"}}},
			{Contact: contacts.Contact{Name: "Bob", Emails: []string{"bob@example.org"}}},
		},
		marked: map[string]bool{},
		target: fieldCc,
	}
	m = clickOn(t, m, rowZoneID(frameContacts, 1, 0))
	if m.contactPick.cursor != 1 {
		t.Fatalf("click: cursor %d, want 1", m.contactPick.cursor)
	}
	m = clickOn(t, m, rowZoneID(frameContacts, 1, 0))
	if len(m.contactPick.marked) != 1 {
		t.Fatalf("second click marked %d contacts, want Bob", len(m.contactPick.marked))
	}
	m = clickOn(t, m, "hint:enter")
	if m.contactPick != nil || !strings.Contains(m.compose.Cc, "bob@example.org") {
		t.Errorf("clicking \"Enter: add\": picker %v, Cc %q", m.contactPick != nil, m.compose.Cc)
	}
}

// The summary was set back to its top on every frame, so no scroll -- key or
// wheel -- survived the next render.
func TestSummaryKeepsItsScroll(t *testing.T) {
	m, _ := setupTestSpamMenuModel(t, false)
	m.showSummary = true
	m.summaryContent = strings.Repeat("A line of the summary.\n\n", 80)
	_ = m.View()
	model, _ := m.Update(tea.MouseMsg{Button: tea.MouseButtonWheelDown})
	m = model.(*tuiModel)
	_ = m.View()
	if m.summaryVp.YOffset == 0 {
		t.Errorf("the wheel's scroll was undone by the next frame")
	}
}
