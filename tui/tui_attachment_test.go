package tui

import (
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/mailclient"

	tea "github.com/charmbracelet/bubbletea"
)

func TestTUI_AttachmentDrawer_HelpToggle(t *testing.T) {
	mockClient := &mailclient.MockMailClient{Cfg: &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: t.TempDir()}, AccountCache: cachedir.At(t.TempDir())}}
	m := newTuiModel(mockClient, "INBOX", nil)
	m.width = 80
	m.height = 24
	m.attachments = []email.Attachment{
		{Filename: "document.pdf", Data: []byte("%PDF-1.4...")},
	}
	m.attachmentDrawer = true

	// Press F1 inside drawer -> should toggle attachmentHelpOpen
	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyF1})
	tm := res.(*tuiModel)
	if !tm.attachmentHelpOpen {
		t.Errorf("expected attachmentHelpOpen=true after F1 in drawer")
	}

	viewStr := tm.View()
	if !strings.Contains(viewStr, "ATTACHMENT DRAWER HELP") {
		t.Errorf("expected view to contain 'ATTACHMENT DRAWER HELP', got:\n%s", viewStr)
	}

	// Press F1 again -> should close attachmentHelpOpen
	res2, _ := tm.Update(tea.KeyMsg{Type: tea.KeyF1})
	tm2 := res2.(*tuiModel)
	if tm2.attachmentHelpOpen {
		t.Errorf("expected attachmentHelpOpen=false after second F1")
	}
}

func TestTUI_AttachmentDrawer_QuickSavePattern(t *testing.T) {
	tmpDir := t.TempDir()
	savePattern := filepath.Join(tmpDir, "custom", "%YY", "%MM", "%DD")

	cfg := &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: tmpDir}, AccountCache: cachedir.At(tmpDir), Prefs: cfg_g.Prefs{AttachmentSaveDir: savePattern}}
	mockClient := &mailclient.MockMailClient{Cfg: cfg}
	m := newTuiModel(mockClient, "INBOX", nil)
	m.cfg = cfg
	m.width = 80
	m.height = 24
	m.attachments = []email.Attachment{
		{Filename: "report.pdf", Data: []byte("PDF content 123")},
	}
	m.attachmentDrawer = true
	m.attCursor = 0

	// Press 's' to quick save
	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	tm := res.(*tuiModel)

	if !strings.Contains(tm.saveStatus, "Saved to") {
		t.Fatalf("expected saveStatus 'Saved to', got: %s", tm.saveStatus)
	}

	// Verify file on disk
	savedPath := strings.TrimPrefix(tm.saveStatus, "Saved to ")
	data, err := os.ReadFile(savedPath)
	if err != nil {
		t.Fatalf("failed to read saved file %s: %v", savedPath, err)
	}
	if string(data) != "PDF content 123" {
		t.Errorf("content mismatch: got %q", string(data))
	}
}

func TestTUI_AttachmentDrawer_FileBrowserSave(t *testing.T) {
	tmpDir := t.TempDir()
	targetFolder := filepath.Join(tmpDir, "my_docs")
	_ = os.MkdirAll(targetFolder, 0755)

	mockClient := &mailclient.MockMailClient{Cfg: &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: tmpDir}, AccountCache: cachedir.At(tmpDir)}}
	m := newTuiModel(mockClient, "INBOX", nil)
	m.width = 80
	m.height = 24
	m.attachments = []email.Attachment{
		{Filename: "contract.pdf", Data: []byte("Contract Data")},
	}
	m.attachmentDrawer = true
	m.attCursor = 0

	// 1. Press 'S' to open File Browser
	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("S")})
	tm := res.(*tuiModel)
	if !tm.fileBrowserOpen {
		t.Fatalf("expected fileBrowserOpen=true")
	}

	// View should render file browser
	viewStr := tm.View()
	if !strings.Contains(viewStr, "SELECT SAVE DIRECTORY") {
		t.Errorf("expected 'SELECT SAVE DIRECTORY' in view, got: %s", viewStr)
	}

	// Set browser to targetFolder and select with 's'
	tm.fileBrowser.CurrentDir = targetFolder
	res2, _ := tm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	tm2 := res2.(*tuiModel)

	if tm2.fileBrowserOpen {
		t.Errorf("expected fileBrowserOpen=false after save")
	}
	if !strings.Contains(tm2.saveStatus, "Saved to") {
		t.Errorf("expected saveStatus 'Saved to', got: %s", tm2.saveStatus)
	}

	savedPath := filepath.Join(targetFolder, "contract.pdf")
	data, err := os.ReadFile(savedPath)
	if err != nil {
		t.Fatalf("failed to read saved file from file browser: %v", err)
	}
	if string(data) != "Contract Data" {
		t.Errorf("saved content mismatch: got %q", string(data))
	}
}

func TestTUI_DetailView_ICSAttachmentBadge(t *testing.T) {
	mockClient := &mailclient.MockMailClient{Cfg: &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: t.TempDir()}, AccountCache: cachedir.At(t.TempDir())}}
	m := newTuiModel(mockClient, "INBOX", nil)
	m.width = 80
	m.height = 24
	m.attachments = []email.Attachment{
		{Filename: "invite.ics", ContentType: "text/calendar", Data: []byte("BEGIN:VCALENDAR\nEND:VCALENDAR")},
	}
	m.detail = "Meeting invitation body"

	content := m.buildDetailContent(80)
	if !strings.Contains(content, "⏰📎 Attachments (1): invite.ics") {
		t.Errorf("expected detail content to contain '⏰📎 Attachments (1): invite.ics', got:\n%s", content)
	}
}

func TestTUI_DetailView_RegularAttachmentBadge(t *testing.T) {
	mockClient := &mailclient.MockMailClient{Cfg: &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: t.TempDir()}, AccountCache: cachedir.At(t.TempDir())}}
	m := newTuiModel(mockClient, "INBOX", nil)
	m.width = 80
	m.height = 24
	m.attachments = []email.Attachment{
		{Filename: "document.pdf", ContentType: "application/pdf", Data: []byte("%PDF...")},
	}
	m.detail = "Document email body"

	content := m.buildDetailContent(80)
	if !strings.Contains(content, "📎 Attachments (1): document.pdf") {
		t.Errorf("expected detail content to contain '📎 Attachments (1): document.pdf', got:\n%s", content)
	}
	if strings.Contains(content, "⏰📎") {
		t.Errorf("did not expect '⏰📎' in detail content for regular attachment, got:\n%s", content)
	}
}

func TestTUI_AttachmentDrawer_ICSIcon(t *testing.T) {
	mockClient := &mailclient.MockMailClient{Cfg: &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: t.TempDir()}, AccountCache: cachedir.At(t.TempDir())}}
	m := newTuiModel(mockClient, "INBOX", nil)
	m.width = 80
	m.height = 24
	m.attachments = []email.Attachment{
		{Filename: "document.pdf", ContentType: "application/pdf", Data: []byte("%PDF...")},
		{Filename: "meeting.ics", ContentType: "text/calendar", Data: []byte("BEGIN:VCALENDAR\nEND:VCALENDAR")},
	}
	m.attachmentDrawer = true

	drawerView := renderAttachmentDrawer(m)
	if !strings.Contains(drawerView, "📕") {
		t.Errorf("expected drawer to contain '📕' for PDF, got:\n%s", drawerView)
	}
	if !strings.Contains(drawerView, "⏰") {
		t.Errorf("expected drawer to contain '⏰' for ICS, got:\n%s", drawerView)
	}
}

func TestTUI_AttachmentDrawer_AddCalendarEvent(t *testing.T) {
	var calledWithData []byte
	bk := &Backend{
		Calendar: stubCalendar{
			AddICSFn: func(_ *cfg_g.Config, icsData []byte) (string, bool, error) {
				calledWithData = icsData
				return "Team Standup", false, nil
			},
		},
	}
	mockClient := &mailclient.MockMailClient{Cfg: &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: t.TempDir()}, AccountCache: cachedir.At(t.TempDir())}}
	m := newTuiModel(mockClient, "INBOX", bk)
	m.width = 80
	m.height = 24
	m.attachments = []email.Attachment{
		{Filename: "meeting.ics", ContentType: "text/calendar", Data: []byte("BEGIN:VCALENDAR\nSUMMARY:Team Standup\nEND:VCALENDAR")},
	}
	m.attachmentDrawer = true
	m.attCursor = 0

	// Press 'c' to import to calendar
	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	tm := res.(*tuiModel)

	if string(calledWithData) != string(m.attachments[0].Data) {
		t.Errorf("expected AddICSEventToCalendar to be called with ICS data, got: %s", string(calledWithData))
	}
	if !strings.Contains(tm.saveStatus, "Added to calendar: Team Standup") {
		t.Errorf("expected saveStatus 'Added to calendar: Team Standup', got: %s", tm.saveStatus)
	}
}

func TestTUI_AttachmentDrawer_AddCalendarNonICS(t *testing.T) {
	bk := &Backend{
		Calendar: stubCalendar{
			AddICSFn: func(_ *cfg_g.Config, _ []byte) (string, bool, error) {
				t.Errorf("should not be called for non-ICS attachment")
				return "", false, nil
			},
		},
	}
	mockClient := &mailclient.MockMailClient{Cfg: &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: t.TempDir()}, AccountCache: cachedir.At(t.TempDir())}}
	m := newTuiModel(mockClient, "INBOX", bk)
	m.width = 80
	m.height = 24
	m.attachments = []email.Attachment{
		{Filename: "document.pdf", ContentType: "application/pdf", Data: []byte("%PDF...")},
	}
	m.attachmentDrawer = true
	m.attCursor = 0

	// Press 'c' on non-ICS attachment
	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	tm := res.(*tuiModel)

	if !strings.Contains(tm.saveStatus, "Not a calendar (.ics) attachment") {
		t.Errorf("expected status 'Not a calendar (.ics) attachment', got: %s", tm.saveStatus)
	}
}

func TestTUI_AttachmentModal_TextPreview(t *testing.T) {
	mockClient := &mailclient.MockMailClient{Cfg: &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: t.TempDir()}, AccountCache: cachedir.At(t.TempDir())}}
	m := newTuiModel(mockClient, "INBOX", nil)
	m.width = 80
	m.height = 24
	m.attachments = []email.Attachment{
		{Filename: "notes.txt", ContentType: "text/plain", Data: []byte("Meeting notes content here\nLine 2")},
	}
	m.attachmentDrawer = true
	m.attCursor = 0

	// Press 'v' to open attachment preview modal
	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	tm := res.(*tuiModel)

	if !tm.attModalOpen {
		t.Fatalf("expected attModalOpen=true after 'v' on text attachment")
	}

	viewStr := tm.View()
	if !strings.Contains(viewStr, "ATTACHMENT PREVIEW") {
		t.Errorf("expected view to contain 'ATTACHMENT PREVIEW', got:\n%s", viewStr)
	}
	if !strings.Contains(viewStr, "notes.txt") {
		t.Errorf("expected view to contain filename 'notes.txt', got:\n%s", viewStr)
	}
	if !strings.Contains(viewStr, "Meeting notes content here") {
		t.Errorf("expected view to contain content 'Meeting notes content here', got:\n%s", viewStr)
	}

	// Press Esc to close
	res2, _ := tm.Update(tea.KeyMsg{Type: tea.KeyEsc})
	tm2 := res2.(*tuiModel)
	if tm2.attModalOpen {
		t.Errorf("expected attModalOpen=false after Esc")
	}
}

func TestTUI_AttachmentModal_NavigationAndSave(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: tmpDir}, AccountCache: cachedir.At(tmpDir), Prefs: cfg_g.Prefs{AttachmentSaveDir: tmpDir}}
	mockClient := &mailclient.MockMailClient{Cfg: cfg}
	m := newTuiModel(mockClient, "INBOX", nil)
	m.cfg = cfg
	m.width = 80
	m.height = 24
	m.attachments = []email.Attachment{
		{Filename: "data.csv", ContentType: "text/csv", Data: []byte("id,name\n1,Alice")},
		{Filename: "image.png", ContentType: "image/png", Data: []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")},
	}
	m.attachmentDrawer = true
	m.attCursor = 0

	// 1. Open preview on first attachment
	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")})
	tm := res.(*tuiModel)
	if !tm.attModalOpen || tm.attPreviewAtt.Filename != "data.csv" {
		t.Fatalf("expected modal open on data.csv")
	}

	// 2. Press 's' to quick save from within modal
	res2, _ := tm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	tm2 := res2.(*tuiModel)
	if !strings.Contains(tm2.attPreviewStatus, "Saved to") {
		t.Errorf("expected attPreviewStatus 'Saved to', got: %s", tm2.attPreviewStatus)
	}

	// 3. Press 'Tab' to switch to next attachment (image.png)
	res3, _ := tm2.Update(tea.KeyMsg{Type: tea.KeyTab})
	tm3 := res3.(*tuiModel)
	if tm3.attPreviewAtt.Filename != "image.png" {
		t.Errorf("expected switched to image.png, got: %s", tm3.attPreviewAtt.Filename)
	}

	viewStr := tm3.View()
	if !strings.Contains(viewStr, "Binary Attachment: image.png") {
		t.Errorf("expected binary preview for image.png, got:\n%s", viewStr)
	}
	if !strings.Contains(viewStr, "SHA-256") {
		t.Errorf("expected SHA-256 in binary preview, got:\n%s", viewStr)
	}
}

func TestTUI_AttachmentDrawer_PDFPreview(t *testing.T) {
	mockClient := &mailclient.MockMailClient{Cfg: &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: t.TempDir()}, AccountCache: cachedir.At(t.TempDir())}}
	m := newTuiModel(mockClient, "INBOX", nil)
	m.width = 100
	m.height = 24
	m.attachments = []email.Attachment{
		{Filename: "invoice.pdf", ContentType: "application/pdf", Data: []byte("%PDF-1.4 sample data")},
	}
	m.attachmentDrawer = true
	m.attCursor = 0

	drawerView := renderAttachmentDrawer(m)
	if !strings.Contains(drawerView, "PDF Preview") && !strings.Contains(drawerView, "invoice.pdf") {
		t.Errorf("expected drawer to contain PDF preview / filename, got:\n%s", drawerView)
	}
	if !strings.Contains(drawerView, "📕") {
		t.Errorf("expected drawer to contain PDF icon 📕, got:\n%s", drawerView)
	}
}

func TestTUI_AttachmentDrawer_ImagePreview(t *testing.T) {
	mockClient := &mailclient.MockMailClient{Cfg: &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: t.TempDir()}, AccountCache: cachedir.At(t.TempDir())}}
	m := newTuiModel(mockClient, "INBOX", nil)
	m.width = 100
	m.height = 24
	m.attachments = []email.Attachment{
		{Filename: "photo.jpg", ContentType: "image/jpeg", Data: []byte("\xff\xd8\xff\xe0\x00\x10JFIF\x00")},
		{Filename: "diagram.png", ContentType: "image/png", Data: []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")},
	}
	m.attachmentDrawer = true
	m.attCursor = 0

	drawerView := renderAttachmentDrawer(m)
	if !strings.Contains(drawerView, "photo.jpg") {
		t.Errorf("expected drawer to contain 'photo.jpg', got:\n%s", drawerView)
	}
	if !strings.Contains(drawerView, "🖼️") {
		t.Errorf("expected drawer to contain image icon 🖼️, got:\n%s", drawerView)
	}

	// Move cursor to second image (diagram.png)
	m.attCursor = 1
	drawerView2 := renderAttachmentDrawer(m)
	if !strings.Contains(drawerView2, "diagram.png") {
		t.Errorf("expected drawer to contain 'diagram.png', got:\n%s", drawerView2)
	}
}

func TestExtractPDFPreview_EmptyAndMalformed(t *testing.T) {
	emptyResult := extractPDFPreview(nil, 10)
	if emptyResult != "(Empty PDF file)" {
		t.Errorf("expected '(Empty PDF file)', got: %q", emptyResult)
	}

	malformedResult := extractPDFPreview([]byte("not a real pdf"), 10)
	if !strings.Contains(malformedResult, "PDF Document") {
		t.Errorf("expected fallback message, got: %q", malformedResult)
	}
}

func TestRenderCodeAttachmentPreview_Empty(t *testing.T) {
	att := email.Attachment{Data: nil}
	res := renderCodeAttachmentPreview(att, 80)
	if res != "(Empty file)" {
		t.Errorf("expected '(Empty file)', got: %q", res)
	}
}
