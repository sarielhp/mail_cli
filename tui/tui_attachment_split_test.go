package tui

import (
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/mailclient"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

func TestTUI_DetailMode_VerticalSplitPDF(t *testing.T) {
	mockClient := &mailclient.MockMailClient{Cfg: &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: t.TempDir()}, AccountCache: cachedir.At(t.TempDir())}}
	m := newTuiModel(mockClient, "INBOX", nil)
	m.mode = modeDetail
	m.detailSplitOpen = true
	m.width = 120
	m.height = 30
	m.attachments = []email.Attachment{
		{Filename: "specification.pdf", ContentType: "application/pdf", Data: []byte("%PDF-1.4 sample data")},
	}
	m.detailH = "From: alice@example.com\nSubject: Project Specs"
	m.detail = "Please find the specifications attached."

	viewStr := m.View()
	if strings.Contains(viewStr, "PDF Document") {
		t.Errorf("PDF should not be displayed in right pane, got:\n%s", viewStr)
	}
	if !strings.Contains(viewStr, "Project Specs") {
		t.Errorf("expected view to contain left pane email subject 'Project Specs', got:\n%s", viewStr)
	}
}

func TestTUI_InlineYellowAttachments_Positioning(t *testing.T) {
	mockClient := &mailclient.MockMailClient{Cfg: &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: t.TempDir()}, AccountCache: cachedir.At(t.TempDir())}}
	m := newTuiModel(mockClient, "INBOX", nil)
	m.mode = modeDetail
	m.width = 100
	m.height = 30
	m.attachments = []email.Attachment{
		{Filename: "contract.pdf", ContentType: "application/pdf", Data: []byte("%PDF-1.4 data")},
	}
	m.detailH = "From: bob@example.com\nSubject: Contract"
	m.detail = "Hello,\nHere is the contract.\n\nOn Mon, Aug 12, 2026 at 3:15 PM Alice wrote:\n> Can you send the contract?"

	content := m.buildDetailContent(80)
	idxContract := strings.Index(content, "contract.pdf")
	idxQuote := strings.Index(content, "On Mon, Aug 12")
	if idxContract == -1 {
		t.Fatalf("expected content to contain 'contract.pdf'")
	}
	if idxQuote == -1 {
		t.Fatalf("expected content to contain quoted text 'On Mon, Aug 12'")
	}
	if idxContract > idxQuote {
		t.Errorf("expected attachments list to be positioned BEFORE quoted text, but got idxContract=%d, idxQuote=%d", idxContract, idxQuote)
	}
}

func TestTUI_DetailMode_PythonScriptSplitView(t *testing.T) {
	mockClient := &mailclient.MockMailClient{Cfg: &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: t.TempDir()}, AccountCache: cachedir.At(t.TempDir())}}
	m := newTuiModel(mockClient, "INBOX", nil)
	m.mode = modeDetail
	m.detailSplitOpen = true
	m.width = 120
	m.height = 30
	m.attachments = []email.Attachment{
		{
			Filename:    "solution.py",
			ContentType: "text/x-python",
			Data:        []byte("def solve():\n    return 42\n"),
		},
	}
	m.detailH = "From: developer@example.com\nSubject: Solution Script"
	m.detail = "Here is the python code."

	viewStr := m.View()
	// Scripts should not open right pane graphic preview
	if strings.Contains(viewStr, "Python Source") {
		t.Errorf("Python script should not be displayed in right pane, got:\n%s", viewStr)
	}
}

func TestTUI_RightPaneFocus_Navigation(t *testing.T) {
	mockClient := &mailclient.MockMailClient{Cfg: &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: t.TempDir()}, AccountCache: cachedir.At(t.TempDir())}}
	m := newTuiModel(mockClient, "INBOX", nil)
	m.mode = modeDetail
	m.width = 120
	m.height = 30
	m.attachments = []email.Attachment{
		{
			Filename:    "invite.ics",
			ContentType: "text/calendar",
			Data: []byte("BEGIN:VCALENDAR\nVERSION:2.0\nBEGIN:VEVENT\nUID:123\nSUMMARY:Meeting\n" +
				"DTSTART:20260920T100000Z\nDTEND:20260920T110000Z\nEND:VEVENT\nEND:VCALENDAR"),
		},
	}

	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	tm := res.(*tuiModel)
	if !tm.rightPaneFocus {
		t.Fatalf("expected rightPaneFocus=true after Tab")
	}

	res2, _ := tm.Update(tea.KeyMsg{Type: tea.KeyDown})
	tm2 := res2.(*tuiModel)
	if tm2.rightVp.YOffset < 0 {
		t.Errorf("unexpected negative YOffset")
	}

	// Test 'p' dismisses right pane
	res3, _ := tm2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	tm3 := res3.(*tuiModel)
	if tm3.rightPaneFocus {
		t.Fatalf("expected rightPaneFocus=false after 'p'")
	}
	if !tm3.hideRightPane {
		t.Fatalf("expected hideRightPane=true after 'p'")
	}
}

func TestTUI_DetailMode_ScrollEntersAttachmentHighlight(t *testing.T) {
	mockClient := &mailclient.MockMailClient{Cfg: &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: t.TempDir()}, AccountCache: cachedir.At(t.TempDir())}}
	m := newTuiModel(mockClient, "INBOX", nil)
	m.mode = modeDetail
	m.width = 100
	m.height = 20
	m.attachments = []email.Attachment{
		{Filename: "file1.pdf", ContentType: "application/pdf", Data: []byte("%PDF-1.4")},
		{Filename: "file2.pdf", ContentType: "application/pdf", Data: []byte("%PDF-1.4")},
	}
	m.attachmentLineOffset = 15
	m.vp = viewport.New(80, 20)
	m.vp.SetContent(strings.Repeat("Line content\n", 50))
	m.vp.SetYOffset(4) // middleLine = 4 + 10 = 14 (above attachment)

	// 1st Down: YOffset becomes 5, middleLine = 15 (att 0)
	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	tm := res.(*tuiModel)

	if !tm.attachmentDrawer {
		t.Fatalf("expected attachmentDrawer=true when scrolling into attachment area, got YOffset=%d", tm.vp.YOffset)
	}
	if tm.attCursor != 0 {
		t.Fatalf("expected attCursor=0, got %d", tm.attCursor)
	}

	// 2nd Down: YOffset becomes 6, middleLine = 16 (att 1)
	res2, _ := tm.Update(tea.KeyMsg{Type: tea.KeyDown})
	tm2 := res2.(*tuiModel)
	if !tm2.attachmentDrawer || tm2.attCursor != 1 {
		t.Fatalf("expected attachmentDrawer=true with attCursor=1, got drawer=%v, cursor=%d", tm2.attachmentDrawer, tm2.attCursor)
	}

	// 3rd Down: YOffset becomes 7, middleLine = 17 (exits past attachments)
	res3, _ := tm2.Update(tea.KeyMsg{Type: tea.KeyDown})
	tm3 := res3.(*tuiModel)
	if tm3.attachmentDrawer {
		t.Fatalf("expected attachmentDrawer=false after stepping past last attachment")
	}

	// Test Single Attachment: highlighted only once during scroll
	mSingle := newTuiModel(mockClient, "INBOX", nil)
	mSingle.mode = modeDetail
	mSingle.width = 100
	mSingle.height = 20
	mSingle.attachments = []email.Attachment{
		{Filename: "only.pdf", ContentType: "application/pdf", Data: []byte("%PDF-1.4")},
	}
	mSingle.attachmentLineOffset = 15
	mSingle.vp = viewport.New(80, 20)
	mSingle.vp.SetContent(strings.Repeat("Line content\n", 50))
	mSingle.vp.SetYOffset(4)

	// Before: YOffset becomes 5, middleLine = 15 (att 0)
	sRes1, _ := mSingle.Update(tea.KeyMsg{Type: tea.KeyDown})
	sTm1 := sRes1.(*tuiModel)
	if !sTm1.attachmentDrawer || sTm1.attCursor != 0 {
		t.Fatalf("expected attachmentDrawer=true on single attachment, got drawer=%v, cursor=%d, YOffset=%d", sTm1.attachmentDrawer, sTm1.attCursor, sTm1.vp.YOffset)
	}

	// Next Down: YOffset becomes 6, middleLine = 16 (exits immediately!)
	sRes2, _ := sTm1.Update(tea.KeyMsg{Type: tea.KeyDown})
	sTm2 := sRes2.(*tuiModel)
	if sTm2.attachmentDrawer {
		t.Fatalf("expected single attachment to be highlighted only once and exit on next scroll")
	}
}

func TestTUI_RightPane_AutoOpenAndEscape(t *testing.T) {
	mockClient := &mailclient.MockMailClient{Cfg: &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: t.TempDir()}, AccountCache: cachedir.At(t.TempDir())}}
	m := newTuiModel(mockClient, "INBOX", nil)
	m.mode = modeDetail
	m.width = 100
	m.height = 20
	m.attachments = []email.Attachment{
		{Filename: "doc.pdf", ContentType: "application/pdf", Data: []byte("%PDF-1.4")},
	}
	m.attachmentLineOffset = 15
	m.vp = viewport.New(80, 20)
	m.vp.SetContent(strings.Repeat("Line content\n", 50))
	m.vp.SetYOffset(4) // above attachment

	if m.detailSplitOpen {
		t.Fatalf("expected detailSplitOpen=false initially")
	}

	// 1. Scroll down to reach attachment -> detailSplitOpen becomes true
	res1, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	tm1 := res1.(*tuiModel)
	if !tm1.detailSplitOpen {
		t.Fatalf("expected detailSplitOpen=true after reaching attachment")
	}

	// 2. Scroll down past attachment -> detailSplitOpen stays true (no interface jump!)
	res2, _ := tm1.Update(tea.KeyMsg{Type: tea.KeyDown})
	tm2 := res2.(*tuiModel)
	if !tm2.detailSplitOpen {
		t.Fatalf("expected detailSplitOpen=true to stay open when scrolling past attachment")
	}

	// 3. Press Escape -> closes right pane (detailSplitOpen=false), remains in modeDetail
	res3, _ := tm2.Update(tea.KeyMsg{Type: tea.KeyEsc})
	tm3 := res3.(*tuiModel)
	if tm3.detailSplitOpen {
		t.Fatalf("expected detailSplitOpen=false after Escape")
	}
	if tm3.mode != modeDetail {
		t.Fatalf("expected mode=modeDetail after closing split pane, got %v", tm3.mode)
	}

	// 4. Press Escape again -> returns to modeIndex
	res4, _ := tm3.Update(tea.KeyMsg{Type: tea.KeyEsc})
	tm4 := res4.(*tuiModel)
	if tm4.mode != modeIndex {
		t.Fatalf("expected mode=modeIndex after second Escape, got %v", tm4.mode)
	}
}

func TestTUI_DetailMode_ICSSplitView(t *testing.T) {
	mockClient := &mailclient.MockMailClient{Cfg: &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: t.TempDir()}, AccountCache: cachedir.At(t.TempDir())}}
	m := newTuiModel(mockClient, "INBOX", nil)
	m.mode = modeDetail
	m.detailSplitOpen = true
	m.width = 120
	m.height = 30
	sampleICS := `BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
SUMMARY:Sprint Planning Meeting
DTSTART:20260901T150000Z
DTEND:20260901T160000Z
LOCATION:https://meet.google.com/abc-defg-hij
ORGANIZER;CN=Alice:mailto:alice@example.com
ATTENDEE;CN=Bob:mailto:bob@example.com
DESCRIPTION:Discuss Q4 roadmap and sprint backlog
END:VEVENT
END:VCALENDAR`
	m.attachments = []email.Attachment{
		{
			Filename:    "invite.ics",
			ContentType: "text/calendar",
			Data:        []byte(sampleICS),
		},
	}
	m.detailH = "From: alice@example.com\nSubject: Sprint Planning"
	m.detail = "Please see calendar invitation attached."

	viewStr := m.View()
	if !strings.Contains(viewStr, "invite.ics") {
		t.Errorf("expected view to contain 'invite.ics', got:\n%s", viewStr)
	}
	if !strings.Contains(viewStr, "Calendar Event") {
		t.Errorf("expected view to contain 'Calendar Event', got:\n%s", viewStr)
	}
	if !strings.Contains(viewStr, "Sprint Planning Meeting") {
		t.Errorf("expected view to contain 'Sprint Planning Meeting', got:\n%s", viewStr)
	}
	if !strings.Contains(viewStr, "https://meet.google.com/abc-defg-hij") {
		t.Errorf("expected view to contain meet link, got:\n%s", viewStr)
	}
	if !strings.Contains(viewStr, "Discuss Q4 roadmap") {
		t.Errorf("expected view to contain event description, got:\n%s", viewStr)
	}
}

func TestTUI_AttachmentInvertColors_Persistence(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	fc := &cfg_g.FileConfig{
		DownloadDir: tempDir,
	}
	if err := cfg_g.SaveConfigFile(configPath, fc); err != nil {
		t.Fatalf("failed to save mock config: %v", err)
	}

	cfg := &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: tempDir, ConfigDir: configPath}}
	mockClient := &mailclient.MockMailClient{Cfg: cfg}
	m := newTuiModel(mockClient, "INBOX", nil)

	if m.attInvertColors {
		t.Fatalf("expected attInvertColors=false by default")
	}

	// 1. Toggle inversion
	m.toggleInvertColors()
	if !m.attInvertColors {
		t.Fatalf("expected attInvertColors=true after toggle")
	}
	if !cfg.AttachmentInvertColors {
		t.Fatalf("expected cfg.AttachmentInvertColors=true")
	}

	// 2. Read from disk and verify persisted
	savedFC, err := cfg_g.LoadConfigFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config file: %v", err)
	}
	if savedFC.AttachmentInvertColors == nil || !*savedFC.AttachmentInvertColors {
		t.Fatalf("expected persisted AttachmentInvertColors=true in config.json")
	}

	// 3. Toggle back
	m.toggleInvertColors()
	if m.attInvertColors {
		t.Fatalf("expected attInvertColors=false after second toggle")
	}
	savedFC2, _ := cfg_g.LoadConfigFile(configPath)
	if savedFC2.AttachmentInvertColors == nil || *savedFC2.AttachmentInvertColors {
		t.Fatalf("expected persisted AttachmentInvertColors=false in config.json")
	}
}
