package tui

import (
	"fmt"
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"github.com/sarielhp/mail_cli/cache/folderindex"
	"github.com/sarielhp/mail_cli/cache/msg"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/label"
	"github.com/sarielhp/mail_cli/mailclient"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestIndexModeScrolling(t *testing.T) {
	m := newTuiModel(nil, "", nil)
	m.height = 10

	var emails []email.Email
	for i := 0; i < 20; i++ {
		emails = append(emails, email.Email{
			ID:            email.ID(fmt.Sprintf("id-%d", i)),
			Subject:       fmt.Sprintf("Subject %d", i),
			FromEmail:     "a@b.com",
			FormattedDate: " 6/20/26",
		})
	}
	m.emails = emails

	m.adjustScrollOffset()
	renderIndex(m)
	if m.indexStart != 0 {
		t.Errorf("expected indexStart to be 0 at start, got %d", m.indexStart)
	}

	m.eIdx = 8
	m.adjustScrollOffset()
	renderIndex(m)
	if m.indexStart != 1 {
		t.Errorf("expected indexStart to scroll to 1 when eIdx is 8, got %d", m.indexStart)
	}

	m.eIdx = 19
	m.adjustScrollOffset()
	renderIndex(m)
	if m.indexStart != 12 {
		t.Errorf("expected indexStart to be 12 at the end, got %d", m.indexStart)
	}
}

func TestLargeFolderIndexScrollingUp(t *testing.T) {
	m := newTuiModel(nil, "", nil)
	m.height = 20
	m.width = 80

	var emails []email.Email
	for i := 0; i < 100; i++ {
		emails = append(emails, email.Email{
			ID:            email.ID(fmt.Sprintf("id-%d", i)),
			Subject:       fmt.Sprintf("Subject %d", i),
			FromEmail:     "a@b.com",
			FormattedDate: " 6/20/26",
		})
	}
	m.emails = emails

	// Scroll to the bottom
	m.eIdx = 99
	m.adjustScrollOffset()

	// Press Up arrow 50 times
	for i := 0; i < 50; i++ {
		msg := tea.KeyMsg{Type: tea.KeyUp}
		m.Update(msg)
	}

	// Get the rendered view
	viewStr := m.View()
	lines := strings.Split(viewStr, "\n")

	// Check line count: should not exceed m.height!
	if len(lines) > m.height {
		t.Errorf("View returned %d lines, expected at most %d", len(lines), m.height)
	}

	// Check if the top line is the topBar
	if len(lines) > 0 {
		topLine := lines[0]
		if !strings.Contains(topLine, "default") {
			t.Errorf("Top line does not contain topBar content, got: %q", topLine)
		}
	}
}

func TestDetailModeArrowNavigation(t *testing.T) {
	m := newTuiModel(nil, "", nil)
	m.mode = modeDetail

	var emails []email.Email
	for i := 0; i < 3; i++ {
		emails = append(emails, email.Email{
			ID:            email.ID(fmt.Sprintf("id-%d", i)),
			Subject:       fmt.Sprintf("Subject %d", i),
			FromEmail:     "a@b.com",
			FormattedDate: " 6/20/26",
		})
	}
	m.emails = emails
	m.eIdx = 1
	m.selectedID = emails[1].ID

	{
		res, _ := m.Update(tea.KeyMsg{Type: tea.KeyLeft})
		newM := res.(*tuiModel)
		if newM.eIdx != 0 {
			t.Errorf("expected eIdx to be 0 after KeyLeft, got %d", newM.eIdx)
		}
		if newM.selectedID != emails[0].ID {
			t.Errorf("expected selectedID to be %q after KeyLeft, got %q", emails[0].ID, newM.selectedID)
		}
		if newM.detail != "(Loading message...)" {
			t.Errorf("expected detail body to reset, got %q", newM.detail)
		}
	}

	{
		res, _ := m.Update(tea.KeyMsg{Type: tea.KeyLeft})
		newM := res.(*tuiModel)
		if newM.eIdx != 0 {
			t.Errorf("expected eIdx to remain 0 after KeyLeft at boundary, got %d", newM.eIdx)
		}
	}

	{
		res, _ := m.Update(tea.KeyMsg{Type: tea.KeyRight})
		newM := res.(*tuiModel)
		if newM.eIdx != 1 {
			t.Errorf("expected eIdx to be 1 after KeyRight, got %d", newM.eIdx)
		}
		if newM.selectedID != emails[1].ID {
			t.Errorf("expected selectedID to be %q after KeyRight, got %q", emails[1].ID, newM.selectedID)
		}
	}

	{
		res, _ := m.Update(tea.KeyMsg{Type: tea.KeyRight})
		newM := res.(*tuiModel)
		if newM.eIdx != 2 {
			t.Errorf("expected eIdx to be 2, got %d", newM.eIdx)
		}

		res2, _ := newM.Update(tea.KeyMsg{Type: tea.KeyRight})
		newM2 := res2.(*tuiModel)
		if newM2.eIdx != 2 {
			t.Errorf("expected eIdx to remain 2, got %d", newM2.eIdx)
		}
	}
}

func TestLoadCachedEmailsForFolder(t *testing.T) {
	tempDir := createTestEmailDir(t)

	today := time.Now()
	yy, mm, dd := today.Date()

	emlContent := "From: test@example.com\nSubject: Test Subject\nDate: Mon, 22 Jun 2026 12:00:00 +0000\n\nBody text"
	if err := msg.Store(cachedir.At(tempDir), "msg123", []byte(emlContent), time.Date(yy, mm, dd, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}

	if err := folderindex.ReplaceAll(cachedir.At(tempDir), "Archive", []email.ID{"msg123"}); err != nil {
		t.Fatal(err)
	}

	cfg := &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: tempDir}, AccountCache: cachedir.At(tempDir)}
	cl := &mailclient.MockMailClient{Cfg: cfg}
	m := newTuiModel(cl, "", nil)

	m.loadCachedEmailsForFolder("Archive")

	if len(m.emails) != 1 {
		t.Fatalf("expected 1 cached email, got %d", len(m.emails))
	}
	if m.emails[0].ID != "msg123" {
		t.Errorf("expected email ID to be 'msg123', got %q", m.emails[0].ID)
	}
	if m.emails[0].Subject != "Test Subject" {
		t.Errorf("expected subject to be 'Test Subject', got %q", m.emails[0].Subject)
	}
}

func TestDefaultFolderResolution(t *testing.T) {
	cfg1 := &cfg_g.Config{Policy: cfg_g.Policy{ReceivedFolder: "received"}}
	mc1 := &mailclient.MockMailClient{
		Cfg: cfg1,
	}
	m1 := newTuiModel(mc1, "", nil)
	if m1.currentF != "received" {
		t.Errorf("expected targetFolder to be %q, got %q", "received", m1.currentF)
	}

	cfg2 := &cfg_g.Config{Policy: cfg_g.Policy{ReceivedFolder: "Inbox"}}
	mc2 := &mailclient.MockMailClient{
		Cfg: cfg2,
	}
	m2 := newTuiModel(mc2, "", nil)
	if m2.currentF != "Inbox" {
		t.Errorf("expected targetFolder to be %q, got %q", "Inbox", m2.currentF)
	}

	folders := []label.Item{
		{Name: "Inbox", FullName: "Inbox"},
		{Name: "Work", FullName: "Work"},
	}
	m2.currentF = ""
	res, _ := m2.Update(foldersLoadedMsg{folders: folders})
	updatedModel := res.(*tuiModel)
	if updatedModel.currentF != "Inbox" {
		t.Errorf("expected foldersLoadedMsg fallback to target %q, got %q", "Inbox", updatedModel.currentF)
	}
}

func TestNewNavigationKeys(t *testing.T) {
	m := newTuiModel(nil, "", nil)
	m.height = 10

	var emails []email.Email
	for i := 0; i < 20; i++ {
		emails = append(emails, email.Email{
			ID:        email.ID(fmt.Sprintf("id-%d", i)),
			Subject:   fmt.Sprintf("Subject %d", i),
			FromEmail: "a@b.com",
		})
	}
	m.emails = emails

	{
		m.eIdx = 5
		res, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("home")})
		newM := res.(*tuiModel)
		if newM.eIdx != 0 {
			t.Errorf("expected eIdx to be 0 after 'home', got %d", newM.eIdx)
		}
		if newM.selectedID != emails[0].ID {
			t.Errorf("expected selectedID to be %q after 'home', got %q", emails[0].ID, newM.selectedID)
		}

		res2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("end")})
		newM2 := res2.(*tuiModel)
		if newM2.eIdx != 19 {
			t.Errorf("expected eIdx to be 19 after 'end', got %d", newM2.eIdx)
		}
		if newM2.selectedID != emails[19].ID {
			t.Errorf("expected selectedID to be %q after 'end', got %q", emails[19].ID, newM2.selectedID)
		}
	}

	{
		m.eIdx = 15
		res, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("pageup")})
		newM := res.(*tuiModel)
		if newM.eIdx != 7 {
			t.Errorf("expected eIdx to be 7 after 'pageup', got %d", newM.eIdx)
		}

		res2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("pgdown")})
		newM2 := res2.(*tuiModel)
		if newM2.eIdx != 15 {
			t.Errorf("expected eIdx to be 15 after 'pgdown', got %d", newM2.eIdx)
		}
	}
}

func TestEscapeToQuit(t *testing.T) {
	m := newTuiModel(nil, "", nil)
	m.mode = modeIndex

	res, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	newM := res.(*tuiModel)
	if !newM.done {
		t.Error("expected done to be true after esc in index view")
	}
	if cmd == nil {
		t.Error("expected a tea.Quit command after esc in index view")
	}
}
