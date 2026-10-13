package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sarielhp/mail_cli/email"
)

func TestTaggingOperations(t *testing.T) {
	m := &tuiModel{
		mode: modeIndex,
		emails: []email.Email{
			{ID: "msg-1", Subject: "Important Invoice", FromRaw: "billing@acme.com"},
			{ID: "msg-2", Subject: "Weekly Newsletter", FromRaw: "news@example.com"},
			{ID: "msg-3", Subject: "Invoice follow-up", FromRaw: "billing@acme.com"},
		},
		eIdx:     0,
		currentF: "INBOX",
	}

	if m.countTagged() != 0 {
		t.Errorf("expected 0 tagged initially, got %d", m.countTagged())
	}

	// Toggle tag on msg-1 and advance
	m.toggleTagAndAdvance()
	if !m.isTagged("msg-1") {
		t.Errorf("expected msg-1 to be tagged")
	}
	if m.eIdx != 1 {
		t.Errorf("expected selection to advance to index 1, got %d", m.eIdx)
	}

	// Tag by pattern "invoice"
	m.tagByPattern("invoice")
	if !m.isTagged("msg-3") {
		t.Errorf("expected msg-3 to be tagged by pattern")
	}
	if m.isTagged("msg-2") {
		t.Errorf("expected msg-2 to not be tagged")
	}
	if m.countTagged() != 2 {
		t.Errorf("expected 2 tagged, got %d", m.countTagged())
	}

	// Tag all visible
	m.tagAllVisible()
	if m.countTagged() != 3 {
		t.Errorf("expected 3 tagged after tagAllVisible, got %d", m.countTagged())
	}

	// Clear tags
	m.clearTags()
	if m.countTagged() != 0 {
		t.Errorf("expected 0 tagged after clearTags, got %d", m.countTagged())
	}
}

func TestTagKeybindingsFlow(t *testing.T) {
	m := &tuiModel{
		mode: modeIndex,
		emails: []email.Email{
			{ID: "msg-1", Subject: "Important Invoice", FromRaw: "billing@acme.com"},
			{ID: "msg-2", Subject: "Weekly Newsletter", FromRaw: "news@example.com"},
		},
		eIdx:     0,
		currentF: "INBOX",
	}

	// 't' toggles tag
	mMod, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	tm := mMod.(*tuiModel)
	if !tm.isTagged("msg-1") {
		t.Errorf("expected msg-1 to be tagged after pressing 't'")
	}

	// ';' arms tag prefix
	mMod, _ = tm.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{';'}})
	tm = mMod.(*tuiModel)
	if !tm.inTagPrefix {
		t.Fatalf("expected inTagPrefix to be true after ';'")
	}

	// Esc cancels tag prefix
	mMod, _ = tm.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	tm = mMod.(*tuiModel)
	if tm.inTagPrefix {
		t.Errorf("expected inTagPrefix to be false after Esc")
	}

	// 'ctrl+u' clears tags
	mMod, _ = tm.handleKey(tea.KeyMsg{Type: tea.KeyCtrlU})
	tm = mMod.(*tuiModel)
	if tm.countTagged() != 0 {
		t.Errorf("expected 0 tagged after ctrl+u, got %d", tm.countTagged())
	}
}

func TestBulkActionMessages(t *testing.T) {
	m := &tuiModel{
		mode: modeIndex,
	}

	// Bulk archive message
	mMod, _ := m.Update(bulkArchiveDoneMsg{ids: []email.ID{"m1", "m2"}})
	tm := mMod.(*tuiModel)
	if tm.saveStatus != "Archived 2 message(s)" {
		t.Errorf("expected saveStatus 'Archived 2 message(s)', got %q", tm.saveStatus)
	}

	// Bulk delete message
	mMod, _ = m.Update(bulkDeleteDoneMsg{ids: []email.ID{"m1"}})
	tm = mMod.(*tuiModel)
	if tm.saveStatus != "Deleted 1 message(s)" {
		t.Errorf("expected saveStatus 'Deleted 1 message(s)', got %q", tm.saveStatus)
	}

	// Bulk mark read message
	mMod, _ = m.Update(bulkMarkReadDoneMsg{ids: []email.ID{"m1"}, read: true})
	tm = mMod.(*tuiModel)
	if tm.saveStatus != "Marked 1 message(s) as read" {
		t.Errorf("expected saveStatus 'Marked 1 message(s) as read', got %q", tm.saveStatus)
	}
}
