package tui

import (
	"github.com/sarielhp/mail_cli/email"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestSpamAndArchiveImmediateRemoval(t *testing.T) {
	m := newTuiModel(nil, "", nil)
	m.mode = modeIndex

	baseTime := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	e0 := email.Email{ID: "e0", Subject: "Email 1", MessageID: "<e0>", FromRaw: "Alice <a@b.com>", EmailDate: baseTime}
	e1 := email.Email{ID: "e1", Subject: "Email 2", MessageID: "<e1>", FromRaw: "Bob <b@b.com>", EmailDate: baseTime.Add(time.Minute)}
	m.rawEmails = []email.Email{e0, e1}
	m.rebuildVisibleEmails()

	if len(m.emails) != 2 {
		t.Fatalf("expected 2 emails, got %d", len(m.emails))
	}

	m.eIdx = 0
	m.selectedID = "e0"

	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("S")})
	m = res.(*tuiModel)

	if len(m.emails) != 1 {
		t.Errorf("expected 1 email remaining in memory after pressing 'S', got %d", len(m.emails))
	}
	if m.emails[0].ID != "e1" {
		t.Errorf("expected email 'e1' to remain, got '%s'", m.emails[0].ID)
	}
	if m.selectedID != "e1" {
		t.Errorf("expected selection to move to 'e1', got '%s'", m.selectedID)
	}

	res, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("E")})
	m = res.(*tuiModel)

	if len(m.emails) != 0 {
		t.Errorf("expected 0 emails remaining in memory after pressing 'E', got %d", len(m.emails))
	}
	if m.selectedID != "" {
		t.Errorf("expected selectedID to be empty, got '%s'", m.selectedID)
	}
}

func TestUnspamImmediateRemoval(t *testing.T) {
	m := newTuiModel(nil, "", nil)
	m.mode = modeIndex
	m.currentF = "Spam"

	baseTime := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	e0 := email.Email{ID: "e0", Subject: "Spam Email 1", MessageID: "<e0>", FromRaw: "Spammer <s@b.com>", EmailDate: baseTime}
	e1 := email.Email{ID: "e1", Subject: "Spam Email 2", MessageID: "<e1>", FromRaw: "Spammer2 <s2@b.com>", EmailDate: baseTime.Add(time.Minute)}
	m.rawEmails = []email.Email{e0, e1}
	m.rebuildVisibleEmails()

	if len(m.emails) != 2 {
		t.Fatalf("expected 2 emails, got %d", len(m.emails))
	}

	m.eIdx = 0
	m.selectedID = "e0"

	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("U")})
	m = res.(*tuiModel)

	if len(m.emails) != 1 {
		t.Errorf("expected 1 email remaining in memory after pressing 'U', got %d", len(m.emails))
	}
	if m.emails[0].ID != "e1" {
		t.Errorf("expected email 'e1' to remain, got '%s'", m.emails[0].ID)
	}
	if m.selectedID != "e1" {
		t.Errorf("expected selection to move to 'e1', got '%s'", m.selectedID)
	}

	if m.pendingEmails["e0"] != pendingUnspam {
		t.Errorf("expected 'e0' to be marked as pendingUnspam, got %v", m.pendingEmails["e0"])
	}
}

func TestSpamUndo_Flow(t *testing.T) {
	m := newTuiModel(nil, "", nil)
	m.mode = modeIndex
	m.currentF = "INBOX"
	m.width = 80
	m.height = 24

	baseTime := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	e0 := email.Email{ID: "e0", Subject: "Important Email", MessageID: "<e0>", FromRaw: "Colleague <c@b.com>", EmailDate: baseTime}
	m.rawEmails = []email.Email{e0}
	m.rebuildVisibleEmails()

	m.eIdx = 0
	m.selectedID = "e0"

	// 1. Mark as spam with 'S'
	res, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("S")})
	tm := res.(*tuiModel)

	if cmd == nil {
		t.Fatalf("expected batch command from spam action")
	}
	if tm.lastSpammedID != "e0" {
		t.Errorf("expected lastSpammedID='e0', got '%s'", tm.lastSpammedID)
	}
	if tm.undoSpamBanner == "" {
		t.Errorf("expected undoSpamBanner to be set")
	}

	viewOutput := tm.View()
	if !strings.Contains(viewOutput, "Undo spam marking by pressing 'U'") {
		t.Errorf("expected View() to show undo banner, got: %s", viewOutput)
	}

	// 2. Press 'U' to undo spam
	res2, cmd2 := tm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("U")})
	tm2 := res2.(*tuiModel)

	if cmd2 == nil {
		t.Fatalf("expected unspam command from undo")
	}
	if tm2.undoSpamBanner != "" {
		t.Errorf("expected undoSpamBanner to be cleared after undo")
	}
	if tm2.pendingEmails["e0"] != pendingUnspam {
		t.Errorf("expected e0 in pending unspam, got %v", tm2.pendingEmails["e0"])
	}
}

func TestUnspamInInboxRetained(t *testing.T) {
	m := newTuiModel(nil, "", nil)
	m.mode = modeIndex
	m.currentF = "INBOX"

	baseTime := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	e0 := email.Email{ID: "e0", Subject: "Email 1", MessageID: "<e0>", FromRaw: "Alice <a@b.com>", EmailDate: baseTime}
	m.rawEmails = []email.Email{e0}
	m.rebuildVisibleEmails()

	if len(m.emails) != 1 {
		t.Fatalf("expected 1 email, got %d", len(m.emails))
	}

	m.eIdx = 0
	m.selectedID = "e0"

	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("U")})
	m = res.(*tuiModel)

	if len(m.emails) != 1 {
		t.Errorf("expected 1 email remaining in memory when unspamming inside INBOX, got %d", len(m.emails))
	}
	if m.emails[0].ID != "e0" {
		t.Errorf("expected email 'e0' to remain in index, got '%s'", m.emails[0].ID)
	}
	if m.pendingEmails["e0"] != pendingUnspam {
		t.Errorf("expected 'e0' to be marked as pendingUnspam, got %v", m.pendingEmails["e0"])
	}
}
