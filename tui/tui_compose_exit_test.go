package tui

import (
	"errors"
	"strings"
	"testing"

	"github.com/sarielhp/mail_cli/mailclient"

	tea "github.com/charmbracelet/bubbletea"
)

func composeForExit(t *testing.T, readOnly bool) (*tuiModel, *mailclient.MockMailClient) {
	t.Helper()
	m, mock, _ := setupTestComposeModel(t, readOnly)
	m.mode = modeCompose
	m.standaloneCompose = true
	m.compose = &composeState{
		AccountName: "Personal",
		From:        "user@personal.com",
		To:          "someone@example.com",
		Subject:     "Pending Email",
		Body:        "Don't lose this text",
	}
	return m, mock
}

// finishExit runs the command the exit returned and feeds its message back,
// the way the program would, returning the message that ends it.
func finishExit(t *testing.T, m *tuiModel, cmd tea.Cmd) tea.Msg {
	t.Helper()
	if cmd == nil {
		t.Fatal("exit returned no command; the upload was dropped")
	}
	msg := cmd()
	if _, quit := msg.(tea.QuitMsg); quit {
		return msg
	}
	_, next := m.Update(msg)
	if next == nil {
		t.Fatalf("no command after %T; the program would hang", msg)
	}
	return next()
}

// Standalone compose used to return tea.Quit beside the upload command, so
// the program exited and the server never saw the final edits.
func TestTUI_Compose_StandaloneQuitUploadsBeforeExit(t *testing.T) {
	m, rec := composeForExit(t, false)

	m2, cmd := m.kCompose(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	m = m2.(*tuiModel)
	if m.showLoad != "Uploading draft..." {
		t.Errorf("showLoad %q, want an upload notice while the exit waits", m.showLoad)
	}

	if _, ok := finishExit(t, m, cmd).(tea.QuitMsg); !ok {
		t.Fatal("the program did not quit after the upload")
	}
	if n := len(rec.UploadedEmails); n != 1 || rec.UploadedEmails[0].TargetLabel != "DRAFTS" {
		t.Fatalf("uploads %d to %v, want one to DRAFTS", n, rec.UploadedEmails)
	}
	if !strings.Contains(string(rec.UploadedEmails[0].RawBytes), "Don't lose this text") {
		t.Error("the uploaded draft lacks the body")
	}
	if !strings.Contains(m.saveStatus, "Draft saved to") || !strings.Contains(m.saveStatus, "uploaded to Personal/DRAFTS") {
		t.Errorf("saveStatus %q should name both copies", m.saveStatus)
	}
	if !m.done || m.exitNotice != m.saveStatus {
		t.Errorf("done=%v exitNotice=%q; the runner prints exitNotice after the screen is gone", m.done, m.exitNotice)
	}
}

func TestTUI_Compose_CtrlCSavesAndUploads(t *testing.T) {
	m, rec := composeForExit(t, false)
	m.standaloneCompose = false // the inbox TUI too: Ctrl+C must not throw the draft away

	m2, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlC})
	m = m2.(*tuiModel)
	if m.compose != nil {
		t.Fatal("compose session still open")
	}
	if _, ok := finishExit(t, m, cmd).(tea.QuitMsg); !ok {
		t.Fatal("Ctrl+C did not end the program after the upload")
	}
	if len(rec.UploadedEmails) != 1 {
		t.Fatalf("uploads %d, want 1", len(rec.UploadedEmails))
	}
}

func TestTUI_Compose_QuitStillExitsWhenUploadFails(t *testing.T) {
	m, rec := composeForExit(t, false)
	rec.FailOn = map[string]error{"UploadRawEmail": errors.New("server down")}

	m2, cmd := m.kCompose(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	m = m2.(*tuiModel)
	if _, ok := finishExit(t, m, cmd).(tea.QuitMsg); !ok {
		t.Fatal("a failed upload must not keep the program open")
	}
	if !strings.Contains(m.saveStatus, "Draft saved to") || !strings.Contains(m.saveStatus, "failed: server down") {
		t.Errorf("saveStatus %q should keep the local path and say the upload failed", m.saveStatus)
	}
}

func TestTUI_Compose_ReadOnlyQuitSkipsUpload(t *testing.T) {
	m, rec := composeForExit(t, true)

	m2, cmd := m.kCompose(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	m = m2.(*tuiModel)
	if _, ok := finishExit(t, m, cmd).(tea.QuitMsg); !ok {
		t.Fatal("read-only exit should quit at once")
	}
	if len(rec.UploadedEmails) != 0 {
		t.Fatalf("read-only uploaded %d drafts", len(rec.UploadedEmails))
	}
}
