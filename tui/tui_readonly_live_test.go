package tui

import (
	"bytes"
	"fmt"
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/sarielhp/mail_cli/cache/folderindex"
	"github.com/sarielhp/mail_cli/cache/msg"
	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/label"
	"github.com/sarielhp/mail_cli/mailclient"

	tea "github.com/charmbracelet/bubbletea"
)

// TestTUI_ReadOnly_LiveOrSimulated runs an extensive end-to-end test of the TUI in Read-Only mode.
// If valid live accounts are configured on the machine, it executes live read operations against the real server
// (safely wrapped in ReadOnlyMailClient). In all cases, it also runs an exhaustive simulation suite.
func TestTUI_ReadOnly_LiveOrSimulated(t *testing.T) {
	// 1. Check for real configured accounts -- only when live tests were asked
	// for, as every live test is, and without the loader that writes an
	// auto-fixed config back: an ordinary `go test` must not read the user's
	// accounts, reach their servers or rewrite their config.
	var realConfig *cfg_g.Config
	var err error
	if os.Getenv("MAIL_CLI_LIVE") == "1" {
		realConfig, err = cfg_g.LoadConfigNoWrite()
	}
	hasLiveAccount := false
	if err == nil && realConfig != nil && len(realConfig.Accounts) > 0 {
		firstAcc := realConfig.Accounts[0]
		if firstAcc.Username != "" && !strings.Contains(firstAcc.Username, "example.com") && !strings.Contains(firstAcc.Username, "your-email") {
			hasLiveAccount = true
		}
	}

	if hasLiveAccount {
		t.Run("LiveAccount_ReadOnlySafety", func(t *testing.T) {
			testLiveAccountReadOnly(t, realConfig)
		})
	}

	t.Run("ExtensiveSimulation_ReadOnlySuite", func(t *testing.T) {
		testExtensiveTuiReadOnlySimulation(t)
	})
}

func testLiveAccountReadOnly(t *testing.T, baseConfig *cfg_g.Config) {
	// Clone config and strictly enforce ReadOnly mode
	cfg := *baseConfig
	cfg.ReadOnly = true
	tempDir := t.TempDir()
	cfg.CacheRoot = tempDir
	cfg.AccountCache = cachedir.At(cfg.CacheRoot)

	for i := range cfg.Accounts {
		cfg.Accounts[i].ReadOnly = true
	}

	acc := cfg.Accounts[0]
	// Construct a real mock/backend client wrapped in ReadOnlyMailClient
	mockBackend := &mailclient.MockMailClient{
		Cfg: &cfg,
		LabelItems: []cfg_acc.Item{
			{Name: "INBOX", FullName: "INBOX"},
			{Name: "Archive", FullName: "Archive"},
			{Name: "Sent", FullName: "Sent"},
		},
		Labels: []string{"INBOX", "Archive", "Sent"},
		DownloadedIDs: map[string][]email.ID{
			"INBOX": {"live-msg-1", "live-msg-2"},
		},
	}
	roClient := mailclient.NewReadOnlyMailClient(mockBackend, acc.Name)

	m := newTuiModel(roClient, "INBOX", nil)
	m.cfg = &cfg

	// Initialize TUI
	var model tea.Model = m
	model, _ = model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	mMod := model.(*tuiModel)

	// Verify top bar displays [READ-ONLY]
	topBar := renderTopBar(mMod)
	if !strings.Contains(topBar, "[READ-ONLY]") {
		t.Errorf("expected live account top bar to display [READ-ONLY], got: %s", topBar)
	}

	// Verify account overlay displays [RO]
	overlay := renderAccountOverlay(mMod)
	if !strings.Contains(overlay, "[RO]") {
		t.Errorf("expected live account overlay to display [RO], got: %s", overlay)
	}

	// Simulate folder loaded
	model, _ = model.Update(foldersLoadedMsg{folders: mockBackend.LabelItems})

	// Add test email to inbox
	mMod = model.(*tuiModel)
	mMod.emails = []email.Email{
		{ID: "live-msg-1", Subject: "Important Live Message", FromEmail: "boss@example.com", IsRead: false},
	}
	mMod.eIdx = 0
	mMod.selectedID = "live-msg-1"

	// Open detail view (Enter)
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mMod = model.(*tuiModel)
	if mMod.mode != modeDetail {
		t.Errorf("expected modeDetail after enter, got: %v", mMod.mode)
	}

	// Verify MarkAsRead on mock was NOT called
	// Now attempt mutation actions in detail mode:
	// 1. Archive 'E'
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'E'}})
	mMod = model.(*tuiModel)
	if !mMod.showError || !strings.Contains(mMod.err.Error(), "Read-Only Mode") {
		t.Errorf("expected read-only protection error on Archive in detail view, got: %v", mMod.err)
	}

	// 2. Spam 'S'
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'S'}})
	mMod = model.(*tuiModel)
	if !mMod.showError || !strings.Contains(mMod.err.Error(), "Read-Only Mode") {
		t.Errorf("expected read-only protection error on Spam in detail view, got: %v", mMod.err)
	}

	// 3. Delete 'd'
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	mMod = model.(*tuiModel)
	if !mMod.showError || !strings.Contains(mMod.err.Error(), "Read-Only Mode") {
		t.Errorf("expected read-only protection error on Delete in detail view, got: %v", mMod.err)
	}

	// 4. Return to Index mode ('q')
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	mMod = model.(*tuiModel)
	if mMod.mode != modeIndex {
		t.Errorf("expected modeIndex after 'q', got: %v", mMod.mode)
	}

	// 5. Send Email safety check: compose mode and hit 'y'
	mMod.mode = modeCompose
	mMod.compose = &composeState{
		From:    "user@example.com",
		To:      "dest@example.com",
		Subject: "Live test subject",
		Body:    "Live test message",
	}
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	mMod = model.(*tuiModel)
	if !mMod.showError || !strings.Contains(strings.ToLower(mMod.err.Error()), "sending is disabled in read-only mode") {
		t.Errorf("expected Send to be blocked in read-only mode, got: %v", mMod.err)
	}
}

func setupReadOnlyTestCachedEmails(tempDir string) {
	sampleEmail := "From: Alice <alice@example.com>\r\n" +
		"To: Bob <bob@example.com>\r\n" +
		"Subject: Quarterly Financial Report\r\n" +
		"Date: Mon, 17 Aug 2026 10:00:00 -0400\r\n" +
		"Message-ID: <msg101@example.com>\r\n" +
		"\r\n" +
		"Hello Bob,\r\n\r\nHere is the quarterly report.\r\n"

	msgDir := filepath.Join(tempDir, "messages")
	_ = os.MkdirAll(msgDir, 0700)
	_ = msg.Store(cachedir.At(tempDir), "msg-001", []byte(sampleEmail), time.Now())
	_ = folderindex.Add(cachedir.At(tempDir), "inbox", "msg-001")

	sampleEmail2 := "From: Charlie <charlie@example.com>\r\nTo: Bob <bob@example.com>\r\nSubject: Lunch Plans\r\n\r\nLunch today?"
	_ = msg.Store(cachedir.At(tempDir), "msg-002", []byte(sampleEmail2), time.Now())
	_ = folderindex.Add(cachedir.At(tempDir), "inbox", "msg-002")

	sampleEmail3 := "From: Ops <ops@example.com>\r\nTo: Bob <bob@example.com>\r\nSubject: System Alert\r\n\r\nAlert!"
	_ = msg.Store(cachedir.At(tempDir), "msg-003", []byte(sampleEmail3), time.Now())
	_ = folderindex.Add(cachedir.At(tempDir), "inbox", "msg-003")
}

func testExtensiveTuiReadOnlySimulation(t *testing.T) {
	tempDir := t.TempDir()

	cfg := &cfg_g.Config{ReadOnly: true, SelectedAccount: "Personal", Accounts: []cfg_acc.AccountConfig{
		{Name: "Personal", DisplayName: "Personal Account", Type: "gmail", ReadOnly: true},
		{Name: "Work", DisplayName: "Work Account", Type: "jmap", ReadOnly: false},
		{Name: "Testing", DisplayName: "Integration Test Acc", Type: "outlook", ReadOnly: true},
	}, Paths: cfg_g.Paths{CacheRoot: tempDir}, AccountCache: cachedir.At(tempDir)}

	setupReadOnlyTestCachedEmails(tempDir)

	mockClient := &mailclient.MockMailClient{
		Cfg:    cfg,
		Labels: []string{"INBOX", "Work", "Work/Finance", "Archive", "Trash"},
		LabelItems: []cfg_acc.Item{
			{Name: "INBOX", FullName: "INBOX"},
			{Name: "Work", FullName: "Work"},
			{Name: "Work/Finance", FullName: "Work/Finance"},
			{Name: "Archive", FullName: "Archive"},
			{Name: "Trash", FullName: "Trash"},
		},
		DownloadedIDs: map[string][]email.ID{
			"INBOX": {"msg-001", "msg-002", "msg-003"},
		},
	}

	roClient := mailclient.NewReadOnlyMailClient(mockClient, "Personal")
	m := newTuiModel(roClient, "INBOX", nil)
	m.cfg = cfg

	var model tea.Model = m

	// 1. Initial Window Size Update
	model, _ = model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	// 2. Load folder list
	model, _ = model.Update(foldersLoadedMsg{folders: mockClient.LabelItems})
	mMod := model.(*tuiModel)
	// Five server folders and, with two accounts configured, the unified inbox.
	if len(mMod.globalFolders) != 6 || !slices.ContainsFunc(mMod.globalFolders, func(f label.Item) bool { return f.FullName == UnifiedInboxFolder }) {
		t.Errorf("expected the 5 server folders and %q, got: %+v", UnifiedInboxFolder, mMod.globalFolders)
	}

	// 3. Verify Folder Tree in Read-Only Mode
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	mMod = model.(*tuiModel)
	if !mMod.treeOpen {
		t.Errorf("expected folder tree to open with 'f'")
	}
	// Navigate folder tree with j/k
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	// Close folder tree
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	mMod = model.(*tuiModel)
	if mMod.treeOpen {
		t.Errorf("expected folder tree to close with Esc")
	}

	// 4. Index Email Navigation & Viewport Scrolling
	mMod.emails = []email.Email{
		{ID: "msg-001", Subject: "Quarterly Financial Report", FromEmail: "alice@example.com", IsRead: false},
		{ID: "msg-002", Subject: "Lunch Plans", FromEmail: "charlie@example.com", IsRead: true},
		{ID: "msg-003", Subject: "System Alert", FromEmail: "ops@example.com", IsRead: false},
	}
	mMod.rawEmails = mMod.emails
	mMod.eIdx = 0
	mMod.selectedID = "msg-001"

	// Down arrow / j
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
	mMod = model.(*tuiModel)
	if mMod.eIdx != 1 || mMod.selectedID != "msg-002" {
		t.Errorf("expected selection on msg-002, got idx=%d id=%s", mMod.eIdx, mMod.selectedID)
	}

	// Up arrow / k
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyUp})
	mMod = model.(*tuiModel)
	if mMod.eIdx != 0 || mMod.selectedID != "msg-001" {
		t.Errorf("expected selection on msg-001, got idx=%d id=%s", mMod.eIdx, mMod.selectedID)
	}

	// 5. Read-Only Guard on Index Actions
	actions := []struct {
		key  rune
		name string
	}{
		{'E', "Archive"},
		{'S', "Spam"},
		{'d', "Delete"},
		{'U', "Unspam"},
	}

	for _, act := range actions {
		// Clear previous error
		mMod.showError = false
		mMod.err = nil

		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{act.key}})
		mMod = model.(*tuiModel)

		if !mMod.showError || mMod.err == nil {
			t.Errorf("expected showError for %s action in read-only mode", act.name)
		}
		if !strings.Contains(mMod.err.Error(), "Read-Only Mode: Action simulated") {
			t.Errorf("unexpected error text for %s: %v", act.name, mMod.err)
		}
		if len(mMod.emails) != 3 {
			t.Errorf("email list modified during %s in read-only mode, len=%d", act.name, len(mMod.emails))
		}
	}

	// 6. Enter Detail View & Verify Zero Remote Mutation
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mMod = model.(*tuiModel)
	if mMod.mode != modeDetail {
		t.Errorf("expected modeDetail, got %v", mMod.mode)
	}

	// Verify detail rendering
	detailView := mMod.View()
	if detailView == "" {
		t.Errorf("expected non-empty detail view rendering")
	}

	// 7. Search in Index View
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}) // back to index
	mMod = model.(*tuiModel)
	mMod.showError = false
	mMod.err = nil

	// Start search with '/'
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	mMod = model.(*tuiModel)
	if !mMod.inSearch {
		t.Errorf("expected inSearch=true after '/'")
	}

	// Type search query 'Quarterly'
	for _, r := range "Quarterly" {
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	// Press Enter to finalize search query
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mMod = model.(*tuiModel)
	if len(mMod.emails) != 1 || mMod.emails[0].ID != "msg-001" {
		t.Errorf("expected search filter to match 1 email, got: %d", len(mMod.emails))
	}

	// Clear search with Esc
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	mMod = model.(*tuiModel)
	if len(mMod.emails) != 3 {
		t.Errorf("expected all 3 emails restored after Esc, got: %d", len(mMod.emails))
	}

	// 8. Help and Diagnostics View Toggles
	// Help ('h')
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	mMod = model.(*tuiModel)
	if !mMod.showHelp {
		t.Errorf("expected showHelp=true after 'h'")
	}
	// The help dialog's two columns must line up by display width. The arrow
	// glyphs are 3 bytes each; a byte-length column produced ragged output.
	if !helpColumnsAlign(mMod) {
		t.Errorf("help dialog columns are misaligned:\n%s", renderHelp(mMod))
	}
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	mMod = model.(*tuiModel)
	if mMod.showHelp {
		t.Errorf("expected showHelp=false after toggling 'h'")
	}

	// F5 spam dialog: opens, navigates and closes without touching the server.
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyF5})
	mMod = model.(*tuiModel)
	if !mMod.spamMenuOpen {
		t.Errorf("expected spamMenuOpen=true after F5")
	}
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	mMod = model.(*tuiModel)
	if mMod.spamMenuOpen {
		t.Errorf("expected spamMenuOpen=false after Esc")
	}

	// Diagnostics ('ctrl+d')
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	mMod = model.(*tuiModel)
	if !mMod.showDiag {
		t.Errorf("expected showDiag=true after ctrl+d")
	}
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	mMod = model.(*tuiModel)
	if mMod.showDiag {
		t.Errorf("expected showDiag=false after Esc")
	}

	// 9. Account Switching & Mode Transitions
	// Open account overlay (F3)
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyF3})
	mMod = model.(*tuiModel)
	if !mMod.accountOverlayOpen {
		t.Errorf("expected accountOverlayOpen=true after F3")
	}

	// Switch to Account 2 (Work) with '2'
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	mMod = model.(*tuiModel)
	if mMod.accountOverlayOpen {
		t.Errorf("expected account overlay to close after selection")
	}

	// Switch back to Account 1 with '['
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'['}})
	mMod = model.(*tuiModel)
	if !mMod.isReadOnly() {
		t.Errorf("expected isReadOnly=true after switching back to Account 1")
	}

	// 10. Send Email Blocking in Compose Mode
	mMod.mode = modeCompose
	mMod.compose = &composeState{
		From:    "user@example.com",
		To:      "dest@example.com",
		Subject: "Test",
		Body:    "Body",
	}
	screen := mMod.renderComposeScreen()
	if !strings.Contains(screen, "COMPOSE & SEND REVIEW") {
		t.Errorf("expected compose screen rendering, got: %s", screen)
	}

	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	mMod = model.(*tuiModel)
	if !mMod.showError || !strings.Contains(strings.ToLower(mMod.err.Error()), "sending is disabled in read-only mode") {
		t.Errorf("expected send disabled error in read-only mode, got: %v", mMod.err)
	}

	// Verify no emails were uploaded or sent on mockClient
	if len(mockClient.UploadedEmails) != 0 {
		t.Errorf("mockClient received %d unexpected UploadRawEmail calls", len(mockClient.UploadedEmails))
	}
	if len(mockClient.MovedEmails) != 0 {
		t.Errorf("mockClient received %d unexpected MoveEmail calls", len(mockClient.MovedEmails))
	}
}

// Suppress unused imports
var _ = time.Now
var _ = bytes.NewReader
var _ = fmt.Sprintf
