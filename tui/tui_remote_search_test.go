package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sarielhp/mail_cli/cache/cachedir"
	"github.com/sarielhp/mail_cli/cache/virtual"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/mailclient"
)

func TestRemoteSearchPromptFlow(t *testing.T) {
	m := &tuiModel{
		mode:     modeIndex,
		currentF: "INBOX",
	}

	// Press 'ctrl+f' to open remote search prompt
	mMod, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlF})
	tm := mMod.(*tuiModel)
	if !tm.inRemoteSearch {
		t.Fatalf("expected inRemoteSearch=true after ctrl+f")
	}

	// Type "from:alice"
	for _, r := range "from:alice" {
		mMod, _ = tm.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		tm = mMod.(*tuiModel)
	}
	if tm.remoteSearchQuery != "from:alice" {
		t.Errorf("expected remoteSearchQuery='from:alice', got %q", tm.remoteSearchQuery)
	}

	// Backspace
	mMod, _ = tm.handleKey(tea.KeyMsg{Type: tea.KeyBackspace})
	tm = mMod.(*tuiModel)
	if tm.remoteSearchQuery != "from:alic" {
		t.Errorf("expected remoteSearchQuery='from:alic', got %q", tm.remoteSearchQuery)
	}

	// Escape cancels remote search prompt
	mMod, _ = tm.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	tm = mMod.(*tuiModel)
	if tm.inRemoteSearch {
		t.Errorf("expected inRemoteSearch=false after Esc")
	}
	if tm.remoteSearchQuery != "" {
		t.Errorf("expected cleared remoteSearchQuery, got %q", tm.remoteSearchQuery)
	}
}

func TestRemoteSearchDoneHandling(t *testing.T) {
	mockClient := &mailclient.MockMailClient{}
	m := &tuiModel{
		mode:   modeIndex,
		client: mockClient,
	}

	// Success
	emails := []*email.Email{
		{ID: "m1", Subject: "Remote Result 1"},
		{ID: "m2", Subject: "Remote Result 2"},
	}
	mMod, _ := m.Update(remoteSearchDoneMsg{query: "test", emails: emails})
	tm := mMod.(*tuiModel)

	if len(tm.emails) != 2 {
		t.Fatalf("expected 2 emails loaded, got %d", len(tm.emails))
	}
	if tm.currentF != "%search:test" {
		t.Errorf("expected currentF to be '%%search:test', got %q", tm.currentF)
	}
	if !strings.Contains(tm.saveStatus, "Found 2 remote message(s)") {
		t.Errorf("expected saveStatus to report 2 remote messages, got %q", tm.saveStatus)
	}

	// Error handling
	mMod, _ = m.Update(remoteSearchDoneMsg{query: "test", err: errTestFailure})
	tm = mMod.(*tuiModel)
	if !tm.showError || tm.err == nil {
		t.Errorf("expected showError=true with non-nil error")
	}
}

// A search that finishes after the user switched accounts belongs to the
// account it ran on: its result set is saved there, and the other account's
// view is left alone rather than filled with ids it cannot act on.
func TestRemoteSearchDoneAfterAccountSwitch(t *testing.T) {
	workDir := t.TempDir()
	searched := &mailclient.MockMailClient{Cfg: &cfg_g.Config{SelectedAccount: "Work", AccountCache: cachedir.At(workDir)}}
	active := &mailclient.MockMailClient{Cfg: &cfg_g.Config{SelectedAccount: "Personal", AccountCache: cachedir.At(t.TempDir())}}
	m := &tuiModel{mode: modeIndex, client: active, currentF: "INBOX",
		rawEmails: []email.Email{{ID: "p1"}}}
	m.rebuildVisibleEmails()

	mMod, _ := m.Update(remoteSearchDoneMsg{query: "q", client: searched,
		emails: []*email.Email{{ID: "w1"}}})
	tm := mMod.(*tuiModel)
	if tm.currentF != "INBOX" || len(tm.rawEmails) != 1 || tm.rawEmails[0].ID != "p1" {
		t.Errorf("the active account's view must be left alone, got folder %q rows %v", tm.currentF, tm.rawEmails)
	}
	vm, err := virtual.Load(cachedir.At(workDir), "search")
	if err != nil || len(vm.MessageIDs) != 1 || vm.MessageIDs[0] != "w1" {
		t.Errorf("results must be saved to the searched account: %+v %v", vm, err)
	}
}

// "%search:<q>" is not a server folder; a move from it used to send it as the
// source label.
func TestSearchViewRefusesMoves(t *testing.T) {
	cl := &mailclient.MockMailClient{Cfg: &cfg_g.Config{SelectedAccount: "Work"}}
	m := &tuiModel{mode: modeIndex, client: cl, currentF: searchFolderPrefix + "q",
		pendingEmails: map[email.ID]pendingOpType{}, rawEmails: []email.Email{{ID: "w1"}}}
	m.rebuildVisibleEmails()

	if _, cmd := m.archiveCurrentEmail(); cmd != nil || !m.showError {
		t.Error("archiving from a search view must be refused before any command runs")
	}
	m.showError = false
	m.taggedIDs = map[email.ID]bool{"w1": true}
	if _, cmd := m.bulkDeleteTagged(); cmd != nil || !m.showError {
		t.Error("a bulk delete from a search view must be refused")
	}
	if len(m.rawEmails) != 1 || len(cl.MovedEmails) != 0 {
		t.Errorf("a refused move must leave the row and the server alone")
	}
}
