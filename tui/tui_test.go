package tui

import (
	"errors"
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sarielhp/mail_cli/cache/folderindex"
	"github.com/sarielhp/mail_cli/cache/msg"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/label"
	"github.com/sarielhp/mail_cli/mailclient"

	tea "github.com/charmbracelet/bubbletea"
)

func createTestEmailDir(t *testing.T) string {
	return t.TempDir()
}

// assertThreadShape checks one visible row's collapsed-thread depth and reply
// count.
func assertThreadShape(t *testing.T, row string, e email.Email, wantDepth int, wantHasReplies bool, wantReplies int) {
	t.Helper()
	if e.ThreadDepth != wantDepth || e.ThreadHasReplies != wantHasReplies || e.ThreadRepliesCount != wantReplies {
		t.Errorf("%s thread properties unexpected: depth=%d, hasReplies=%v, count=%d",
			row, e.ThreadDepth, e.ThreadHasReplies, e.ThreadRepliesCount)
	}
}

func TestDecDecode(t *testing.T) {
	dec := new(mime.WordDecoder)
	encoded := "=?UTF-8?Q?Hello_World?="
	result := email.DecDecode(dec, encoded)
	if result != "Hello World" {
		t.Errorf("DecDecode(%q) = %q, want %q", encoded, result, "Hello World")
	}
}

func TestEmailThreadingAndCollapsing(t *testing.T) {
	m := newTuiModel(nil, "", nil)
	m.mode = modeIndex

	baseTime := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	e0 := email.Email{ID: "e0", Subject: "Thread Root", MessageID: "<e0>", FromRaw: "Alice <a@b.com>", EmailDate: baseTime}
	e1 := email.Email{ID: "e1", Subject: "Re: Thread Root", MessageID: "<e1>", InReplyTo: "<e0>", FromRaw: "Bob <b@b.com>", EmailDate: baseTime.Add(time.Minute)}
	e2 := email.Email{ID: "e2", Subject: "Re: Thread Root", MessageID: "<e2>", InReplyTo: "<e1>", FromRaw: "Charlie <c@b.com>", EmailDate: baseTime.Add(2 * time.Minute)}
	e3 := email.Email{ID: "e3", Subject: "Unrelated Subject", MessageID: "<e3>", FromRaw: "Dave <d@b.com>", EmailDate: baseTime.Add(3 * time.Minute)}

	m.rawEmails = []email.Email{e0, e1, e2, e3}
	m.rebuildVisibleEmails()

	if len(m.emails) != 2 {
		t.Fatalf("expected 2 visible emails initially (collapsed by default), got %d", len(m.emails))
	}
	if m.emails[0].ID != "e0" || m.emails[1].ID != "e3" {
		t.Errorf("expected visible emails to be e0 and e3, got %s and %s", m.emails[0].ID, m.emails[1].ID)
	}
	if !m.emails[0].ThreadCollapsed {
		t.Error("expected e0 to be collapsed by default")
	}

	m.expandedThreads[threadKey{header: "<e0>"}] = true
	m.expandedThreads[threadKey{header: "<e1>"}] = true
	m.rebuildVisibleEmails()

	if len(m.emails) != 4 {
		t.Fatalf("expected 4 visible emails after expanding e0, got %d", len(m.emails))
	}

	assertThreadShape(t, "e0", m.emails[0], 0, true, 2)
	assertThreadShape(t, "e1", m.emails[1], 1, true, 1)
	if m.emails[2].ThreadDepth != 2 || m.emails[2].ThreadHasReplies {
		t.Errorf("e2 thread properties unexpected: depth=%d, hasReplies=%v",
			m.emails[2].ThreadDepth, m.emails[2].ThreadHasReplies)
	}

	if !strings.Contains(m.emails[0].ThreadSenderSummary, "Alice") || !strings.Contains(m.emails[0].ThreadSenderSummary, "Bob") {
		t.Errorf("expected sender summary to contain Alice and Bob, got %q", m.emails[0].ThreadSenderSummary)
	}

	m.expandedThreads[threadKey{header: "<e0>"}] = false
	m.rebuildVisibleEmails()

	if len(m.emails) != 2 {
		t.Fatalf("expected 2 visible emails after collapsing e0, got %d", len(m.emails))
	}

	m.eIdx = 0
	m.selectedID = "e0"
	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	m = res.(*tuiModel)
	if !m.splitPreview {
		t.Error("expected Space key to toggle split preview")
	}

	res, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	m = res.(*tuiModel)
	if m.splitPreview {
		t.Error("expected second Space key to toggle split preview off")
	}

	m.eIdx = 0
	m.selectedID = "e0"
	res, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = res.(*tuiModel)
	if !m.expandedThreads[threadKey{header: "<e0>"}] {
		t.Error("expected Enter key on collapsed line to expand it")
	}
	if len(m.emails) != 4 {
		t.Errorf("expected 4 visible emails after Enter expand, got %d", len(m.emails))
	}
	if m.mode != modeIndex {
		t.Errorf("expected mode to remain modeIndex on expand Enter, got %d", m.mode)
	}

	res, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = res.(*tuiModel)
	if m.mode != modeDetail {
		t.Errorf("expected mode to become modeDetail on second Enter, got %d", m.mode)
	}
}

func TestNewTuiImprovements(t *testing.T) {
	m := newTuiModel(nil, "", nil)
	m.mode = modeIndex

	baseTime := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	e0 := email.Email{ID: "e0", Subject: "Thread Root", MessageID: "<e0>", FromRaw: "Alice <a@b.com>", EmailDate: baseTime}
	e1 := email.Email{ID: "e1", Subject: "Re: Thread Root", MessageID: "<e1>", InReplyTo: "<e0>", FromRaw: "Bob <b@b.com>", EmailDate: baseTime.Add(time.Minute)}
	e2 := email.Email{ID: "e2", Subject: "Re: Thread Root", MessageID: "<e2>", InReplyTo: "<e1>", FromRaw: "Charlie <c@b.com>", EmailDate: baseTime.Add(2 * time.Minute)}
	e3 := email.Email{ID: "e3", Subject: "Unrelated", MessageID: "<e3>", FromRaw: "Dave <d@b.com>", EmailDate: baseTime.Add(3 * time.Minute)}

	m.rawEmails = []email.Email{e0, e1, e2, e3}
	m.rebuildVisibleEmails()

	if len(m.emails) != 2 {
		t.Fatalf("expected 2 visible emails, got %d", len(m.emails))
	}
	if m.emails[0].ThreadIndex != 1 || m.emails[1].ThreadIndex != 4 {
		t.Errorf("expected stable indices 1 and 4, got %d and %d", m.emails[0].ThreadIndex, m.emails[1].ThreadIndex)
	}

	m.expandRecursively("e0")
	if !m.expandedThreads[threadKey{header: "<e0>"}] || !m.expandedThreads[threadKey{header: "<e1>"}] {
		t.Error("expected recursive expansion to expand both e0 and e1")
	}

	m.rebuildVisibleEmails()
	if len(m.emails) != 4 {
		t.Fatalf("expected all 4 emails visible after recursive expand, got %d", len(m.emails))
	}

	if m.emails[1].ThreadPrefix != "└── " {
		t.Errorf("expected e1 ThreadPrefix to be '└── ', got %q", m.emails[1].ThreadPrefix)
	}
	if m.emails[2].ThreadPrefix != "  └── " {
		t.Errorf("expected e2 ThreadPrefix to be '  └── ', got %q", m.emails[2].ThreadPrefix)
	}

	m.treeOpen = true
	m.height = 10
	m.entries = []label.TreeEntry{
		{Node: &label.TreeNode{FullName: "1"}},
		{Node: &label.TreeNode{FullName: "2"}},
		{Node: &label.TreeNode{FullName: "3"}},
		{Node: &label.TreeNode{FullName: "4"}},
		{Node: &label.TreeNode{FullName: "5"}},
		{Node: &label.TreeNode{FullName: "6"}},
		{Node: &label.TreeNode{FullName: "7"}},
		{Node: &label.TreeNode{FullName: "8"}},
		{Node: &label.TreeNode{FullName: "9"}},
		{Node: &label.TreeNode{FullName: "10"}},
	}
	m.cursor = 0

	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("pgdown")})
	m = res.(*tuiModel)
	if m.cursor != 8 {
		t.Errorf("expected folder cursor to be 8 after pagedown, got %d", m.cursor)
	}

	res, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("pageup")})
	m = res.(*tuiModel)
	if m.cursor != 0 {
		t.Errorf("expected folder cursor to be 0 after pageup, got %d", m.cursor)
	}

	m.treeOpen = false
	m.showGlobalTree = false
	m.currentF = "INBOX"

	res, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("F")})
	m = res.(*tuiModel)
	if !m.treeOpen || !m.showGlobalTree {
		t.Errorf("expected treeOpen=true, showGlobalTree=true after F key, got treeOpen=%v, showGlobalTree=%v", m.treeOpen, m.showGlobalTree)
	}

	res, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	m = res.(*tuiModel)
	if m.treeOpen {
		t.Error("expected treeOpen=false after toggling with f")
	}
}

func TestHelpToggle(t *testing.T) {
	m := newTuiModel(nil, "", nil)
	if m.showHelp {
		t.Error("expected showHelp=false by default")
	}

	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	m = res.(*tuiModel)
	if !m.showHelp {
		t.Error("expected showHelp=true after pressing '?'")
	}

	res, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	m = res.(*tuiModel)
	if m.showHelp {
		t.Error("expected showHelp=false after pressing '?' again")
	}

	res, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	m = res.(*tuiModel)
	if !m.showHelp {
		t.Error("expected showHelp=true after pressing 'h'")
	}
}

func TestHelpInComposeMode(t *testing.T) {
	m, _, _ := setupTestComposeModel(t, false)
	m.mode = modeCompose
	m.compose = &composeState{
		To:      "user@example.com",
		Subject: "Testing Help",
	}

	if m.showHelp {
		t.Fatal("expected showHelp=false initially")
	}

	// Press '?' in Compose mode
	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	m = res.(*tuiModel)
	if !m.showHelp {
		t.Fatal("expected showHelp=true after pressing '?' in Compose mode")
	}

	// Verify renderHelp renders compose keybindings
	helpView := renderHelp(m)
	if !strings.Contains(helpView, "COMPOSE KEYBINDINGS") {
		t.Errorf("expected COMPOSE KEYBINDINGS in compose help, got:\n%s", helpView)
	}

	// Dismiss help with 'q'
	res, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	m = res.(*tuiModel)
	if m.showHelp {
		t.Errorf("expected showHelp=false after pressing 'q' on help modal")
	}
	// Compose state should remain intact
	if m.mode != modeCompose || m.compose == nil {
		t.Errorf("expected to remain in modeCompose with compose state intact")
	}
}

func TestMenuNavigation(t *testing.T) {
	m := newTuiModel(nil, "", nil)
	m.mode = modeIndex
	m.currentF = "INBOX"

	baseTime := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	e0 := email.Email{ID: "e0", Subject: "Email 1", MessageID: "<e0>", FromRaw: "Alice <a@b.com>", EmailDate: baseTime}
	m.rawEmails = []email.Email{e0}
	m.rebuildVisibleEmails()

	if m.menuOpen {
		t.Error("expected menuOpen=false by default")
	}

	// 1. Press 'm' to open the menu
	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
	m = res.(*tuiModel)
	if !m.menuOpen {
		t.Error("expected menuOpen=true after pressing 'm'")
	}
	if m.menuCursor != 0 {
		t.Errorf("expected menuCursor=0 initially, got %d", m.menuCursor)
	}

	// 2. Press 'j' to move cursor down to "Exit"
	res, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = res.(*tuiModel)
	if m.menuCursor != 1 {
		t.Errorf("expected menuCursor=1 after pressing 'j', got %d", m.menuCursor)
	}

	// 3. Press 'k' to move cursor back up to "Move to Spam"
	res, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	m = res.(*tuiModel)
	if m.menuCursor != 0 {
		t.Errorf("expected menuCursor=0 after pressing 'k', got %d", m.menuCursor)
	}

	// 4. Press 'Enter' on "Move to Spam"
	res, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = res.(*tuiModel)
	if m.menuOpen {
		t.Error("expected menuOpen=false after selecting an action")
	}
	if len(m.emails) != 0 {
		t.Errorf("expected email to be removed from index, got %d remaining", len(m.emails))
	}
	if m.pendingEmails["e0"] != pendingSpam {
		t.Errorf("expected 'e0' to be marked as pendingSpam, got %v", m.pendingEmails["e0"])
	}
}

func TestSingleEmailBackFromViewer(t *testing.T) {
	m := newTuiModel(nil, "", nil)
	m.mode = modeIndex
	m.height = 20
	m.width = 80

	baseTime := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	e0 := email.Email{ID: "e0", Subject: "Single Email", MessageID: "<e0>", FromRaw: "Alice <a@b.com>", EmailDate: baseTime}
	m.rawEmails = []email.Email{e0}
	m.rebuildVisibleEmails()

	if len(m.emails) != 1 {
		t.Fatalf("expected 1 email, got %d", len(m.emails))
	}

	// View the email (press Enter)
	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = res.(*tuiModel)
	if m.mode != modeDetail {
		t.Errorf("expected modeDetail, got %d", m.mode)
	}

	// Go back to the index (press q)
	res, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	m = res.(*tuiModel)
	if m.mode != modeIndex {
		t.Errorf("expected modeIndex, got %d", m.mode)
	}

	// Render view and check if the single email is listed
	viewStr := m.View()
	if !strings.Contains(viewStr, "Single Email") {
		t.Errorf("expected email subject 'Single Email' to be rendered, but view output was:\n%s", viewStr)
	}
}

func TestSingleEmailBgClassifyWithMockClient(t *testing.T) {
	tempDir := t.TempDir()

	// Create cache dir structure
	if err := os.MkdirAll(filepath.Join(tempDir, "indexes"), 0700); err != nil {
		t.Fatal(err)
	}

	// Write mock email into the msg cache
	baseTime := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	e0 := email.Email{ID: "e0", Subject: "Single Email", MessageID: "<e0>", FromRaw: "Alice <a@b.com>", EmailDate: baseTime}
	emailContent := []byte("Subject: Single Email\nMessage-ID: <e0>\nFrom: Alice <a@b.com>\nDate: Sat, 20 Jun 2026 12:00:00 +0000\n\nBody")
	if err := msg.Store(cachedir.At(tempDir), "e0", emailContent, baseTime); err != nil {
		t.Fatal(err)
	}

	// Write the index file for INBOX
	if err := folderindex.ReplaceAll(cachedir.At(tempDir), "INBOX", []email.ID{"e0"}); err != nil {
		t.Fatal(err)
	}

	cfg := &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: tempDir}, AccountCache: cachedir.At(tempDir)}
	cl := &mailclient.MockMailClient{Cfg: cfg}
	m := newTuiModel(cl, "INBOX", nil)
	m.height = 20
	m.width = 80

	// Initially loaded from cache
	if len(m.emails) != 1 {
		t.Fatalf("expected 1 email loaded from cache, got %d", len(m.emails))
	}

	// Simulate emailsLoadedMsg
	res, _ := m.Update(emailsLoadedMsg{emails: []email.Email{e0}})
	m = res.(*tuiModel)
	if len(m.emails) != 1 {
		t.Fatalf("expected 1 email after emailsLoadedMsg, got %d", len(m.emails))
	}

	// View the email (press Enter)
	res, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = res.(*tuiModel)
	if m.mode != modeDetail {
		t.Fatalf("expected modeDetail, got %d", m.mode)
	}

	// While in Detail mode, receive classifyDoneMsg
	res, _ = m.Update(classifyDoneMsg{})
	m = res.(*tuiModel)

	// Go back to the index (press tab)
	res, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = res.(*tuiModel)
	if m.mode != modeIndex {
		t.Fatalf("expected modeIndex, got %d", m.mode)
	}

	// Render view and check if the single email is listed
	viewStr := m.View()
	if !strings.Contains(viewStr, "Single Email") {
		t.Errorf("expected email subject 'Single Email' to be rendered, but view output was:\n%s", viewStr)
	}
}

func TestHelpGrayoutOverlay(t *testing.T) {
	m := newTuiModel(nil, "", nil)
	m.width = 80
	m.height = 40

	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	m = res.(*tuiModel)
	if !m.showHelp {
		t.Fatal("expected showHelp=true after pressing '?'")
	}

	viewStr := m.View()

	viewLines := strings.Split(viewStr, "\n")

	if !strings.Contains(viewStr, "Move down") {
		t.Error("expected help content 'Move down' in view output")
	}
	if !strings.Contains(viewStr, "KEYBINDINGS") {
		t.Error("expected 'KEYBINDINGS' header in view output")
	}

	if len(viewLines) != m.height {
		t.Errorf("expected output to be exactly %d lines (terminal height), got %d", m.height, len(viewLines))
	}

	firstContentIdx := -1
	for i, line := range viewLines {
		if strings.Contains(line, "KEYBINDINGS") {
			firstContentIdx = i
			break
		}
	}
	if firstContentIdx > 0 {
		t.Logf("grayout top padding: %d lines", firstContentIdx)
	} else {
		t.Error("expected grayout padding above help dialog")
	}

	lastContentIdx := -1
	for i := len(viewLines) - 1; i >= 0; i-- {
		if strings.Contains(viewLines[i], "dismiss") {
			lastContentIdx = i
			break
		}
	}
	bottomPad := len(viewLines) - lastContentIdx - 1
	if bottomPad > 0 {
		t.Logf("grayout bottom padding: %d lines", bottomPad)
	} else {
		t.Error("expected grayout padding below help dialog")
	}

	grayoutAnsi := "48;2;26;26;26"
	if !strings.Contains(viewStr, grayoutAnsi) {
		t.Error("expected grayout background color (48;2;26;26;26) in help overlay")
	}

	if !strings.Contains(viewStr, "┌") {
		t.Error("expected dialog top border in help overlay")
	}
	if !strings.Contains(viewStr, "└") {
		t.Error("expected dialog bottom border in help overlay")
	}
	if !strings.Contains(viewStr, "│") {
		t.Error("expected dialog vertical borders in help overlay")
	}
}

func TestTUI_InitialNoticeBanner(t *testing.T) {
	m := newTuiModel(nil, "", nil)
	m.width = 80
	m.height = 24
	notice := "Labels cache not available; opening TUI without folder filter \"work\""
	m.err = errors.New(notice)
	m.showError = true

	banner := renderErrorBanner(m)
	if !strings.Contains(banner, notice) {
		t.Errorf("expected banner to contain %q, got %q", notice, banner)
	}

	view := m.renderIndexView("")
	if !strings.Contains(view, notice) {
		t.Errorf("expected index view to contain %q, got %q", notice, view)
	}
}
