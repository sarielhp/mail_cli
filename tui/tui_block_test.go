package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/mailclient"

	tea "github.com/charmbracelet/bubbletea"
)

func TestBlockModal_PromptAndCancel(t *testing.T) {
	m := newTuiModel(nil, "", nil)
	m.mode = modeIndex

	baseTime := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	e0 := email.Email{ID: "e0", Subject: "Email 1", MessageID: "<e0>", FromRaw: "Bogi Shogie <bogi@example.com>", EmailDate: baseTime}
	m.rawEmails = []email.Email{e0}
	m.rebuildVisibleEmails()

	m.eIdx = 0
	m.selectedID = "e0"

	// Press 'B' -> should open block modal
	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("B")})
	m = res.(*tuiModel)

	if !m.blockModalOpen {
		t.Fatalf("expected blockModalOpen=true after 'B'")
	}
	if m.blockSenderEmail != "bogi@example.com" {
		t.Errorf("expected blockSenderEmail='bogi@example.com', got %q", m.blockSenderEmail)
	}
	if m.blockSenderName != "Bogi Shogie" {
		t.Errorf("expected blockSenderName='Bogi Shogie', got %q", m.blockSenderName)
	}

	// Press 'c' -> should cancel and close modal
	res, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	m = res.(*tuiModel)

	if m.blockModalOpen {
		t.Fatalf("expected blockModalOpen=false after 'c'")
	}
	if len(m.emails) != 1 {
		t.Errorf("expected email to remain after cancel, got len %d", len(m.emails))
	}
}

// runBlockModalExecute opens the block modal on a single message from the given
// sender, presses key to execute the block, and checks that the message left the
// index and that the status names the chosen destination.
func runBlockModalExecute(t *testing.T, from, key, wantDest string) {
	t.Helper()
	mockClient := &mailclient.MockMailClient{}
	m := newTuiModel(mockClient, "", nil)
	m.mode = modeIndex

	baseTime := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	e0 := email.Email{ID: "e0", Subject: "Email 1", MessageID: "<e0>", FromRaw: from, EmailDate: baseTime}
	m.rawEmails = []email.Email{e0}
	m.rebuildVisibleEmails()

	m.eIdx = 0
	m.selectedID = "e0"

	// Press 'B' -> open modal
	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("B")})
	m = res.(*tuiModel)
	if !m.blockModalOpen {
		t.Fatalf("expected blockModalOpen=true")
	}

	// Execute the block to the chosen destination
	res, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
	m = res.(*tuiModel)

	if m.blockModalOpen {
		t.Errorf("expected blockModalOpen=false after %q", key)
	}
	if len(m.emails) != 0 {
		t.Errorf("expected email removed from index after block, got len %d", len(m.emails))
	}
	if !strings.Contains(m.saveStatus, "Blocked") || !strings.Contains(m.saveStatus, wantDest) {
		t.Errorf("expected saveStatus to mention Blocked and %s, got %q", wantDest, m.saveStatus)
	}
}

func TestBlockModal_ExecuteSpam(t *testing.T) {
	runBlockModalExecute(t, "Spammer <spammer@bad.com>", "s", "Spam")
}

func TestBlockModal_ExecuteTrash(t *testing.T) {
	runBlockModalExecute(t, "Annoying <annoying@bad.com>", "t", "Trash")
}

func TestBlockModal_ReadOnly(t *testing.T) {
	cfg := &cfg_g.Config{
		ReadOnly: true,
	}
	m := newTuiModel(nil, "", nil)
	m.cfg = cfg
	m.mode = modeIndex

	baseTime := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	e0 := email.Email{ID: "e0", Subject: "Email 1", MessageID: "<e0>", FromRaw: "Sender <s@bad.com>", EmailDate: baseTime}
	m.rawEmails = []email.Email{e0}
	m.rebuildVisibleEmails()

	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("B")})
	m = res.(*tuiModel)

	if m.blockModalOpen {
		t.Errorf("block modal must not open in read-only mode")
	}
	if !m.showError {
		t.Errorf("expected showError=true in read-only mode")
	}
}

func TestRenderBlockDialog(t *testing.T) {
	m := newTuiModel(nil, "", nil)
	m.width = 80
	m.height = 24
	m.blockSenderName = "Bogi Shogie"
	m.blockSenderEmail = "bogi@example.com"

	dialog := renderBlockDialog(m)
	if !strings.Contains(dialog, "BLOCK SENDER") {
		t.Errorf("expected dialog to contain 'BLOCK SENDER', got %s", dialog)
	}
	if !strings.Contains(dialog, "bogi@example.com") {
		t.Errorf("expected dialog to contain sender email, got %s", dialog)
	}
	if !strings.Contains(dialog, "[s] Spam") || !strings.Contains(dialog, "[t] Trash") {
		t.Errorf("expected dialog to offer [s] Spam and [t] Trash, got %s", dialog)
	}
}
