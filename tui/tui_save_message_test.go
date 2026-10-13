package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/mailclient"
)

// pressF2 sends F2 through the full key dispatch and feeds the fetched
// message back, which opens the save dialog.
func pressF2(t *testing.T, m *tuiModel) {
	t.Helper()
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyF2})
	if cmd == nil {
		t.Fatal("F2 returned no command")
	}
	ready, ok := cmd().(messageReadyToSaveMsg)
	if !ok {
		t.Fatal("F2's command did not fetch the message")
	}
	if ready.err != nil {
		t.Fatalf("fetch failed: %v", ready.err)
	}
	m.Update(ready)
}

// confirmSaveIn navigates the open dialog to dir and presses its save key.
func confirmSaveIn(m *tuiModel, dir string) {
	m.fileBrowser.CurrentDir = dir
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
}

// F2 asks where to save, starting in the working directory, and never
// overwrites an earlier save.
func TestF2SavesMessageWhereTheUserChooses(t *testing.T) {
	raw := []byte("From: a@example.com\r\nSubject: Quarterly report: Q3\r\n\r\nbody\r\n")
	cwd, _ := os.Getwd()
	for _, mode := range []int{modeDetail, modeIndex} {
		m := newDetailModel(t, 80, 30, "INBOX", nil)
		m.mode = mode
		m.client.(*mailclient.MockMailClient).RawMessages = map[email.ID][]byte{"test-001": raw}

		pressF2(t, m)
		if !m.fileBrowserOpen {
			t.Fatalf("mode %v: F2 did not open the save dialog", mode)
		}
		if m.fileBrowser.CurrentDir != cwd {
			t.Errorf("mode %v: dialog starts in %q, want the working directory %q", mode, m.fileBrowser.CurrentDir, cwd)
		}
		if m.pendingSaveAtt.Filename != "Quarterly_report_Q3.eml" {
			t.Errorf("mode %v: proposed name %q", mode, m.pendingSaveAtt.Filename)
		}

		// The user picks a directory and confirms.
		dir := t.TempDir()
		confirmSaveIn(m, dir)
		first := filepath.Join(dir, "Quarterly_report_Q3.eml")
		if got, err := os.ReadFile(first); err != nil || string(got) != string(raw) {
			t.Fatalf("mode %v: saved file %q: %v", mode, got, err)
		}
		if m.notice != "Saved to "+first || !strings.Contains(m.View(), "Saved to") {
			t.Errorf("mode %v: no confirmation on screen", mode)
		}

		pressF2(t, m)
		confirmSaveIn(m, dir)
		if _, err := os.Stat(filepath.Join(dir, "Quarterly_report_Q3_2.eml")); err != nil {
			t.Errorf("mode %v: a second save did not get its own name: %v", mode, err)
		}
	}
}

// Esc in the dialog saves nothing.
func TestF2DialogCancel(t *testing.T) {
	m := newDetailModel(t, 80, 30, "INBOX", nil)
	m.client.(*mailclient.MockMailClient).RawMessages = map[email.ID][]byte{"test-001": []byte("Subject: x\r\n\r\nb")}
	pressF2(t, m)
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.fileBrowserOpen {
		t.Error("Esc did not close the save dialog")
	}
}

// F2 inside the dialog saves at once, so F2 F2 saves in the working
// directory, and a long subject is cut to a prefix unique in that directory.
func TestF2F2SavesImmediatelyWithShortUniqueName(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	subject := "Re: Fwd: [team-list] Reminder - quarterly budget review meeting on Thursday"
	stem := "Reminder_quarterly_budget_review_meeting_on_Thursday"
	// Something already there shares the first 30 characters.
	if err := os.WriteFile(filepath.Join(dir, stem[:30]+"_old.eml"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	m := newDetailModel(t, 80, 30, "INBOX", nil)
	m.client.(*mailclient.MockMailClient).RawMessages = map[email.ID][]byte{"test-001": []byte("Subject: " + subject + "\r\n\r\nb")}
	pressF2(t, m)
	m.Update(tea.KeyMsg{Type: tea.KeyF2})

	if m.fileBrowserOpen {
		t.Fatal("F2 in the dialog did not save")
	}
	want := filepath.Join(dir, stem[:31]+".eml")
	if _, err := os.Stat(want); err != nil {
		t.Errorf("expected %s (the shortest prefix unique in the directory): %v", want, err)
	}
}
