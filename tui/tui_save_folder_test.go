package tui

import (
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"strings"
	"testing"
	"time"

	"github.com/sarielhp/mail_cli/cache/folderindex"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/label"
	"github.com/sarielhp/mail_cli/mailclient"

	tea "github.com/charmbracelet/bubbletea"
)

func setupTestSaveFolderModel(t *testing.T) (*tuiModel, *mailclient.MockMailClient) {
	t.Helper()
	tempDir := t.TempDir()
	cfg := &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: tempDir}, AccountCache: cachedir.At(tempDir)}
	mock := &mailclient.MockMailClient{Cfg: cfg, InboxFolderVal: "INBOX"}
	m := newTuiModel(mock, "INBOX", nil)
	m.width = 80
	m.height = 24
	m.globalFolders = []label.Item{
		{Name: "INBOX", FullName: "INBOX"},
		{Name: "Archive", FullName: "Archive"},
		{Name: "Work", FullName: "Work"},
	}
	m.folders = m.globalFolders
	m.globalTree = label.BuildLabelTree(m.globalFolders)
	m.tree = m.globalTree
	m.rawEmails = []email.Email{
		{ID: "msg-1", Subject: "Important Project", EmailDate: time.Now()},
		{ID: "msg-2", Subject: "Weekly Update", EmailDate: time.Now()},
	}
	m.rebuildVisibleEmails()
	m.eIdx = 0
	return m, mock
}

func TestTUI_SaveMessageToFolder_IndexMode(t *testing.T) {
	m, mock := setupTestSaveFolderModel(t)

	// 1. Press 's' in Index mode
	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	tm := model.(*tuiModel)

	if !tm.treeOpen {
		t.Fatalf("expected treeOpen=true after 's'")
	}
	if !tm.savingMessageToFolder {
		t.Fatalf("expected savingMessageToFolder=true after 's'")
	}
	if tm.savingMessageID != "msg-1" {
		t.Fatalf("expected savingMessageID='msg-1', got %q", tm.savingMessageID)
	}

	// 2. Cancel with 'Esc'
	model, _ = tm.Update(tea.KeyMsg{Type: tea.KeyEsc})
	tm = model.(*tuiModel)
	if tm.treeOpen {
		t.Fatalf("expected treeOpen=false after Esc")
	}
	if tm.savingMessageToFolder {
		t.Fatalf("expected savingMessageToFolder=false after Esc")
	}

	// 3. Press 's' again, navigate to 'Archive', and press Enter
	model, _ = tm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	tm = model.(*tuiModel)

	// Find Archive in entries
	found := false
	for i, e := range tm.entries {
		if e.Node != nil && e.Node.FullName == "Archive" {
			tm.cursor = i
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("folder 'Archive' not found in entries: %+v", tm.entries)
	}

	model, cmd := tm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	tm = model.(*tuiModel)

	if tm.treeOpen {
		t.Fatalf("expected treeOpen=false after Enter")
	}
	if tm.savingMessageToFolder {
		t.Fatalf("expected savingMessageToFolder=false after Enter")
	}
	if cmd == nil {
		t.Fatalf("expected non-nil cmd from saveEmailToFolder")
	}

	// Execute cmd
	msg := cmd()
	doneMsg, ok := msg.(saveEmailDoneMsg)
	if !ok {
		t.Fatalf("expected saveEmailDoneMsg, got %T", msg)
	}
	if doneMsg.err != nil {
		t.Fatalf("unexpected error in saveEmailDoneMsg: %v", doneMsg.err)
	}
	if doneMsg.id != "msg-1" || doneMsg.folder != "Archive" {
		t.Fatalf("expected msg-1 to Archive, got %s to %s", doneMsg.id, doneMsg.folder)
	}

	// Verify MockMailClient recorded the copy
	if len(mock.CopiedEmails) != 1 {
		t.Fatalf("expected mock to record 1 copy, got: %+v", mock.CopiedEmails)
	}
	record := mock.CopiedEmails[0]
	if record.DestLabel != "Archive" || len(record.MessageIDs) != 1 || record.MessageIDs[0] != "msg-1" {
		t.Fatalf("expected copy of msg-1 to Archive, got: %+v", record)
	}

	// Verify update message handles status
	model, _ = tm.Update(doneMsg)
	tm = model.(*tuiModel)
	if !strings.Contains(tm.saveStatus, "Saved message to Archive") {
		t.Fatalf("expected saveStatus 'Saved message to Archive', got %q", tm.saveStatus)
	}
}

func TestTUI_SaveMessageToFolder_AlreadyInFolder(t *testing.T) {
	m, _ := setupTestSaveFolderModel(t)
	dir := m.Config().AccountCache

	// Pre-seed folder index with msg-1 already in Archive
	if err := folderindex.ReplaceAll(dir, "Archive", []email.ID{"msg-1"}); err != nil {
		t.Fatalf("seeding Archive index: %v", err)
	}

	// Press 's'
	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	tm := model.(*tuiModel)

	// Select Archive
	for i, e := range tm.entries {
		if e.Node != nil && e.Node.FullName == "Archive" {
			tm.cursor = i
			break
		}
	}

	model, cmd := tm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	tm = model.(*tuiModel)

	if cmd != nil {
		t.Fatalf("expected nil cmd when message already in folder, got %v", cmd)
	}
	if !strings.Contains(tm.saveStatus, "Message already in Archive") {
		t.Fatalf("expected saveStatus 'Message already in Archive', got %q", tm.saveStatus)
	}
}

func TestTUI_SaveMessageToFolder_DetailMode(t *testing.T) {
	m, mock := setupTestSaveFolderModel(t)
	m.mode = modeDetail
	m.selectedID = "msg-2"

	// Press 's' in Detail mode
	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	tm := model.(*tuiModel)

	if !tm.treeOpen || !tm.savingMessageToFolder {
		t.Fatalf("expected treeOpen and savingMessageToFolder true")
	}
	if tm.savingMessageID != "msg-2" {
		t.Fatalf("expected savingMessageID='msg-2', got %q", tm.savingMessageID)
	}

	// Select Work folder
	for i, e := range tm.entries {
		if e.Node != nil && e.Node.FullName == "Work" {
			tm.cursor = i
			break
		}
	}

	model, cmd := tm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	_ = model
	if cmd == nil {
		t.Fatalf("expected non-nil cmd")
	}

	msg := cmd()
	doneMsg := msg.(saveEmailDoneMsg)
	if doneMsg.err != nil {
		t.Fatalf("unexpected error: %v", doneMsg.err)
	}

	if len(mock.CopiedEmails) != 1 {
		t.Fatalf("expected 1 copy recorded, got %+v", mock.CopiedEmails)
	}
	record := mock.CopiedEmails[0]
	if record.DestLabel != "Work" || len(record.MessageIDs) != 1 || record.MessageIDs[0] != "msg-2" {
		t.Fatalf("expected msg-2 copied to Work, got: %+v", record)
	}
}
