package tui

import (
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"strings"
	"testing"

	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/mailclient"

	tea "github.com/charmbracelet/bubbletea"
)

func TestTUI_ReadOnly_TopBarAndAccountOverlay(t *testing.T) {
	cfg := &cfg_g.Config{ReadOnly: true, SelectedAccount: "Personal", Accounts: []cfg_acc.AccountConfig{
		{Name: "Personal", DisplayName: "Personal", Type: "gmail", ReadOnly: true},
		{Name: "Work", DisplayName: "Work", Type: "jmap", ReadOnly: false},
	}, Paths: cfg_g.Paths{CacheRoot: t.TempDir()}, AccountCache: cachedir.At(t.TempDir())}
	mockClient := &mailclient.MockMailClient{
		Cfg: cfg,
		LabelItems: []cfg_acc.Item{
			{Name: "INBOX", FullName: "INBOX"},
		},
	}

	m := newTuiModel(mockClient, "INBOX", nil)
	m.width = 80
	m.height = 24

	topBar := renderTopBar(m)
	if !strings.Contains(topBar, "[READ-ONLY]") {
		t.Errorf("expected top bar to contain [READ-ONLY], got: %q", topBar)
	}

	overlay := renderAccountOverlay(m)
	if !strings.Contains(overlay, "[RO]") {
		t.Errorf("expected account overlay to contain [RO], got: %q", overlay)
	}
	if strings.Contains(overlay, "● Active") {
		t.Errorf("expected account overlay not to contain '● Active', got: %q", overlay)
	}
	if !strings.Contains(overlay, "●=active") {
		t.Errorf("expected account overlay to contain '●=active', got: %q", overlay)
	}
}

func TestTUI_ReadOnly_ActionKeysIntercepted(t *testing.T) {
	cfg := &cfg_g.Config{ReadOnly: true, Accounts: []cfg_acc.AccountConfig{
		{Name: "Personal", Type: "gmail", ReadOnly: true},
	}, Paths: cfg_g.Paths{CacheRoot: t.TempDir()}, AccountCache: cachedir.At(t.TempDir())}
	mockClient := &mailclient.MockMailClient{
		Cfg: cfg,
	}

	m := newTuiModel(mockClient, "INBOX", nil)
	m.emails = []email.Email{
		{ID: "msg1", Subject: "Test Email", FromEmail: "sender@example.com"},
	}
	m.eIdx = 0
	m.selectedID = "msg1"

	// Archive key 'E'
	m2, _ := m.archiveCurrentEmail()
	mMod := m2.(*tuiModel)
	if !mMod.showError || mMod.err == nil || !strings.Contains(mMod.err.Error(), "Read-Only Mode: Action simulated") {
		t.Errorf("expected read-only error banner on archive, got showError=%v err=%v", mMod.showError, mMod.err)
	}

	// Spam key 's'
	m2, _ = m.spamCurrentEmail()
	mMod = m2.(*tuiModel)
	if !mMod.showError || mMod.err == nil || !strings.Contains(mMod.err.Error(), "Read-Only Mode: Action simulated") {
		t.Errorf("expected read-only error banner on spam, got showError=%v err=%v", mMod.showError, mMod.err)
	}

	// Delete key 'd'
	m2, _ = m.deleteCurrentEmail()
	mMod = m2.(*tuiModel)
	if !mMod.showError || mMod.err == nil || !strings.Contains(mMod.err.Error(), "Read-Only Mode: Action simulated") {
		t.Errorf("expected read-only error banner on delete, got showError=%v err=%v", mMod.showError, mMod.err)
	}

	// Unspam key 'U'
	m2, _ = m.unspamCurrentEmail()
	mMod = m2.(*tuiModel)
	if !mMod.showError || mMod.err == nil || !strings.Contains(mMod.err.Error(), "Read-Only Mode: Action simulated") {
		t.Errorf("expected read-only error banner on unspam, got showError=%v err=%v", mMod.showError, mMod.err)
	}
}

func TestTUI_ReadOnly_SendBlocked(t *testing.T) {
	cfg := &cfg_g.Config{ReadOnly: true, Paths: cfg_g.Paths{CacheRoot: t.TempDir()}, AccountCache: cachedir.At(t.TempDir())}
	mockClient := &mailclient.MockMailClient{Cfg: cfg}

	m := newTuiModel(mockClient, "INBOX", nil)
	m.mode = modeCompose
	m.compose = &composeState{
		From:    "user@example.com",
		To:      "dest@example.com",
		Subject: "Test",
		Body:    "Hello",
	}

	m2, _ := m.kCompose(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	mMod := m2.(*tuiModel)

	if !mMod.showError || mMod.err == nil || !strings.Contains(strings.ToLower(mMod.err.Error()), "sending is disabled in read-only mode") {
		t.Errorf("expected send disabled error, got: %v", mMod.err)
	}
}

func TestTUI_FolderReadOnly_TopBar(t *testing.T) {
	cfg := &cfg_g.Config{ReadOnly: false, SelectedAccount: "Work", Accounts: []cfg_acc.AccountConfig{
		{
			Name:            "Work",
			Type:            "jmap",
			ReadOnly:        false,
			ReadOnlyFolders: []string{"Archive", "Receipts"},
		},
	}, Paths: cfg_g.Paths{CacheRoot: t.TempDir()}, AccountCache: cachedir.At(t.TempDir())}
	mockClient := &mailclient.MockMailClient{
		Cfg: cfg,
		LabelItems: []cfg_acc.Item{
			{Name: "INBOX", FullName: "INBOX"},
			{Name: "Archive", FullName: "Archive"},
		},
	}

	// 1. INBOX: writable
	mInbox := newTuiModel(mockClient, "INBOX", nil)
	mInbox.width = 80
	mInbox.height = 24
	topInbox := renderTopBar(mInbox)
	if strings.Contains(topInbox, "[READ-ONLY]") || strings.Contains(topInbox, "[RO-FOLDER]") {
		t.Errorf("expected INBOX not to be marked read-only, got: %q", topInbox)
	}

	// 2. Archive: read-only folder
	mArchive := newTuiModel(mockClient, "Archive", nil)
	mArchive.width = 80
	mArchive.height = 24
	topArchive := renderTopBar(mArchive)
	if !strings.Contains(topArchive, "[RO-FOLDER]") {
		t.Errorf("expected Archive to contain [RO-FOLDER], got: %q", topArchive)
	}
	if strings.Contains(topArchive, "[READ-ONLY]") {
		t.Errorf("expected Archive not to be whole-account [READ-ONLY], got: %q", topArchive)
	}
}

func TestTUI_FolderReadOnly_LockKeyToggle(t *testing.T) {
	cfg := &cfg_g.Config{ReadOnly: false, Accounts: []cfg_acc.AccountConfig{
		{Name: "Work", Type: "jmap", ReadOnly: false},
	}, Paths: cfg_g.Paths{CacheRoot: t.TempDir()}, AccountCache: cachedir.At(t.TempDir())}
	mockClient := &mailclient.MockMailClient{Cfg: cfg}
	m := newTuiModel(mockClient, "INBOX", nil)
	m.emails = []email.Email{
		{ID: "msg1", Subject: "Test", FromEmail: "sender@example.com"},
	}
	m.eIdx = 0
	m.selectedID = "msg1"

	if m.isReadOnly() {
		t.Fatalf("expected folder initially writable")
	}

	// Press 'L' to lock
	m2, _ := m.kIndexActions(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'L'}})
	mMod := m2.(*tuiModel)
	if !mMod.isReadOnly() {
		t.Errorf("expected isReadOnly=true after pressing 'L'")
	}
	if !mMod.showError || mMod.err == nil || !strings.Contains(mMod.err.Error(), "locked as read-only") {
		t.Errorf("expected lock confirmation banner, got: %v", mMod.err)
	}

	// Action key 'd' must be blocked while locked
	m3, _ := mMod.deleteCurrentEmail()
	mMod3 := m3.(*tuiModel)
	if !mMod3.showError || mMod3.err == nil || !strings.Contains(mMod3.err.Error(), "Read-Only Mode: Action simulated") {
		t.Errorf("expected delete to be blocked while locked, got: %v", mMod3.err)
	}

	// Press 'L' again to unlock
	m4, _ := mMod.kIndexActions(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'L'}})
	mMod4 := m4.(*tuiModel)
	if mMod4.isReadOnly() {
		t.Errorf("expected isReadOnly=false after pressing 'L' again")
	}
	if !mMod4.showError || mMod4.err == nil || !strings.Contains(mMod4.err.Error(), "unlocked") {
		t.Errorf("expected unlock confirmation banner, got: %v", mMod4.err)
	}
}
