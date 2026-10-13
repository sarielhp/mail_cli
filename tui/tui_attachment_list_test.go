package tui

import (
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sarielhp/mail_cli/cache/msg"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/mailclient"

	tea "github.com/charmbracelet/bubbletea"
)

func TestAttachmentList_NoAttachmentsNotice(t *testing.T) {
	mockClient := &mailclient.MockMailClient{Cfg: &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: t.TempDir()}, AccountCache: cachedir.At(t.TempDir())}}
	m := newTuiModel(mockClient, "INBOX", nil)
	m.width = 90
	m.height = 30
	m.emails = []email.Email{{ID: "msg-1", Subject: "Hello without attachments"}}
	m.eIdx = 0
	m.attachments = nil

	// Press 'a'
	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	tm := res.(*tuiModel)

	if tm.attachmentListOpen {
		t.Errorf("expected attachmentListOpen=false when no attachments")
	}
	if !strings.Contains(tm.saveStatus, "No attachments on this message") {
		t.Errorf("expected status 'No attachments on this message', got %q", tm.saveStatus)
	}
}

func TestAttachmentList_OpenAndNavigate(t *testing.T) {
	mockClient := &mailclient.MockMailClient{Cfg: &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: t.TempDir()}, AccountCache: cachedir.At(t.TempDir())}}
	m := newTuiModel(mockClient, "INBOX", nil)
	m.width = 90
	m.height = 30
	m.emails = []email.Email{{ID: "msg-1", Subject: "Project Specs"}}
	m.eIdx = 0
	m.attachments = []email.Attachment{
		{Filename: "spec.txt", ContentType: "text/plain", Data: []byte("Specs data")},
		{Filename: "diagram.png", ContentType: "image/png", Data: []byte("fake-png-data")},
	}

	// Press 'a' to open attachment list
	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	tm := res.(*tuiModel)

	if !tm.attachmentListOpen {
		t.Fatalf("expected attachmentListOpen=true")
	}
	if tm.attachmentListCursor != 0 {
		t.Errorf("expected cursor 0, got %d", tm.attachmentListCursor)
	}

	viewStr := tm.View()
	if !strings.Contains(viewStr, "ATTACHMENTS & PARTS (2)") || !strings.Contains(viewStr, "spec.txt") || !strings.Contains(viewStr, "diagram.png") {
		t.Errorf("expected view to contain attachment titles, got:\n%s", viewStr)
	}

	// Navigate down with 'j'
	res2, _ := tm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	tm2 := res2.(*tuiModel)
	if tm2.attachmentListCursor != 1 {
		t.Errorf("expected cursor 1 after 'j', got %d", tm2.attachmentListCursor)
	}

	// Navigate up with 'k'
	res3, _ := tm2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	tm3 := res3.(*tuiModel)
	if tm3.attachmentListCursor != 0 {
		t.Errorf("expected cursor 0 after 'k', got %d", tm3.attachmentListCursor)
	}
}

func TestAttachmentList_DeleteAndUndelete(t *testing.T) {
	mockClient := &mailclient.MockMailClient{Cfg: &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: t.TempDir()}, AccountCache: cachedir.At(t.TempDir())}}
	m := newTuiModel(mockClient, "INBOX", nil)
	m.width = 90
	m.height = 30
	m.emails = []email.Email{{ID: "msg-1", Subject: "Project Specs"}}
	m.eIdx = 0
	m.attachments = []email.Attachment{
		{Filename: "spec.txt", ContentType: "text/plain", Data: []byte("Specs data")},
	}
	m.attachmentListOpen = true
	m.attachmentDeleted = make(map[int]bool)

	// Press 'd' to mark for deletion
	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	tm := res.(*tuiModel)

	if !tm.attachmentDeleted[0] {
		t.Fatalf("expected attachmentDeleted[0]=true")
	}
	viewStr := tm.View()
	if !strings.Contains(viewStr, "[DELETED]") {
		t.Errorf("expected view to contain [DELETED] badge, got:\n%s", viewStr)
	}

	// Press 'u' to undelete
	res2, _ := tm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
	tm2 := res2.(*tuiModel)

	if tm2.attachmentDeleted[0] {
		t.Fatalf("expected attachmentDeleted[0]=false after undelete")
	}
}

func TestAttachmentList_LocalViewer(t *testing.T) {
	mockClient := &mailclient.MockMailClient{Cfg: &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: t.TempDir()}, AccountCache: cachedir.At(t.TempDir())}}
	m := newTuiModel(mockClient, "INBOX", nil)
	m.width = 90
	m.height = 30
	m.emails = []email.Email{{ID: "msg-1", Subject: "Code Review"}}
	m.eIdx = 0
	m.currentF = "INBOX"
	m.attachments = []email.Attachment{
		{Filename: "main.go", ContentType: "text/x-go", Data: []byte("package main\n\nfunc main() {}")},
	}
	m.attachmentListOpen = true
	m.attachmentDeleted = make(map[int]bool)

	// Press 'Enter' on locally viewable code file
	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	tm := res.(*tuiModel)

	if !tm.attachmentViewerOpen {
		t.Fatalf("expected attachmentViewerOpen=true")
	}

	viewStr := tm.View()
	if !strings.Contains(viewStr, "Attachment: main.go") {
		t.Errorf("expected header with filename, got:\n%s", viewStr)
	}
	if !strings.Contains(viewStr, "Folder: INBOX") {
		t.Errorf("expected header with folder, got:\n%s", viewStr)
	}

	// Press 'Esc' to exit viewer back to list
	res2, _ := tm.Update(tea.KeyMsg{Type: tea.KeyEsc})
	tm2 := res2.(*tuiModel)

	if tm2.attachmentViewerOpen {
		t.Errorf("expected attachmentViewerOpen=false after Esc")
	}
	if !tm2.attachmentListOpen {
		t.Errorf("expected attachmentListOpen=true after returning from viewer")
	}
}

func TestAttachmentList_SaveTrigger(t *testing.T) {
	tmpDir := t.TempDir()
	mockClient := &mailclient.MockMailClient{Cfg: &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: tmpDir}, AccountCache: cachedir.At(tmpDir)}}
	m := newTuiModel(mockClient, "INBOX", nil)
	m.width = 90
	m.height = 30
	m.emails = []email.Email{{ID: "msg-1", Subject: "Invoice"}}
	m.eIdx = 0
	m.attachments = []email.Attachment{
		{Filename: "invoice.pdf", ContentType: "application/pdf", Data: []byte("pdf-data")},
	}
	m.attachmentListOpen = true
	m.attachmentDeleted = make(map[int]bool)

	// Press 's' to save
	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	tm := res.(*tuiModel)

	if !tm.fileBrowserOpen {
		t.Errorf("expected fileBrowserOpen=true after pressing 's'")
	}
	if tm.pendingSaveAtt.Filename != "invoice.pdf" {
		t.Errorf("expected pendingSaveAtt=invoice.pdf, got %s", tm.pendingSaveAtt.Filename)
	}
}

func TestAttachmentList_AppliesDeletionToEML(t *testing.T) {
	tmpDir := t.TempDir()
	mockClient := &mailclient.MockMailClient{Cfg: &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: tmpDir}, AccountCache: cachedir.At(tmpDir)}}
	m := newTuiModel(mockClient, "INBOX", nil)
	m.width = 90
	m.height = 30

	rawEML := "From: alice@example.com\r\n" +
		"To: bob@example.com\r\n" +
		"Subject: Test Message\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: multipart/mixed; boundary=\"_BND_123_\"\r\n" +
		"\r\n" +
		"--_BND_123_\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\n" +
		"\r\n" +
		"Hello world body.\r\n" +
		"--_BND_123_\r\n" +
		"Content-Type: application/pdf; name=\"trash.pdf\"\r\n" +
		"Content-Disposition: attachment; filename=\"trash.pdf\"\r\n" +
		"\r\n" +
		"trash-data\r\n" +
		"--_BND_123_--\r\n"

	_ = msg.Store(cachedir.At(tmpDir), "msg-del-1", []byte(rawEML), time.Now())

	m.emails = []email.Email{{ID: "msg-del-1", Subject: "Test Message"}}
	m.eIdx = 0
	m.currentF = "INBOX"
	m.attachments = []email.Attachment{
		{Filename: "trash.pdf", ContentType: "application/pdf", Data: []byte("trash-data")},
	}
	m.attachmentListOpen = true
	m.attachmentDeleted = map[int]bool{0: true}

	// Press 'Esc' to exit and commit deletion
	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	tm := res.(*tuiModel)

	if tm.attachmentListOpen {
		t.Errorf("expected attachmentListOpen=false after Esc")
	}

	// Read cached EML
	cachedData, err := msg.Read(cachedir.At(tmpDir), "msg-del-1")
	if err != nil {
		t.Fatalf("failed to read cached msg: %v", err)
	}
	if !strings.Contains(string(cachedData), "[Attachment deleted: trash.pdf]") {
		t.Errorf("expected '[Attachment deleted: trash.pdf]' in cached message, got:\n%s", string(cachedData))
	}
}

func TestAttachmentList_QuickSave(t *testing.T) {
	tmpDir := t.TempDir()
	saveDir := filepath.Join(tmpDir, "downloads")
	cfg := &cfg_g.Config{
		Paths: cfg_g.Paths{
			CacheRoot: tmpDir,
		},
		Prefs: cfg_g.Prefs{
			AttachmentSaveDir: saveDir,
		},
	}
	mockClient := &mailclient.MockMailClient{Cfg: cfg}
	m := newTuiModel(mockClient, "INBOX", nil)
	m.cfg = cfg
	m.width = 90
	m.height = 30
	m.emails = []email.Email{{ID: "msg-1", Subject: "Invoice email"}}
	m.eIdx = 0
	m.attachments = []email.Attachment{
		{Filename: "invoice.pdf", ContentType: "application/pdf", Data: []byte("invoice-bytes-123")},
	}
	m.attachmentListOpen = true
	m.attachmentListCursor = 0

	// Press 'S' for Quick Save
	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'S'}})
	tm := res.(*tuiModel)

	if !strings.Contains(tm.saveStatus, "Quick-saved to") {
		t.Errorf("expected status to mention Quick-saved, got: %s", tm.saveStatus)
	}

	targetFile := filepath.Join(saveDir, "invoice.pdf")
	data, err := os.ReadFile(targetFile)
	if err != nil {
		t.Fatalf("expected file to exist at %s: %v", targetFile, err)
	}
	if string(data) != "invoice-bytes-123" {
		t.Errorf("expected file content 'invoice-bytes-123', got %q", string(data))
	}
}

func TestAttachmentList_Key_v(t *testing.T) {
	mockClient := &mailclient.MockMailClient{Cfg: &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: t.TempDir()}, AccountCache: cachedir.At(t.TempDir())}}
	m := newTuiModel(mockClient, "INBOX", nil)
	m.width = 90
	m.height = 30
	m.emails = []email.Email{{ID: "msg-1", Subject: "Testing v key"}}
	m.eIdx = 0
	m.attachments = []email.Attachment{
		{Filename: "notes.txt", ContentType: "text/plain", Data: []byte("Some notes")},
	}

	// Press 'v' to open attachment list
	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	tm := res.(*tuiModel)

	if !tm.attachmentListOpen {
		t.Fatalf("expected attachmentListOpen=true after pressing 'v'")
	}
}

func TestAttachmentList_AlternativeBodyParts_Protection(t *testing.T) {
	tmpDir := t.TempDir()
	emlData := "Content-Type: multipart/alternative; boundary=\"xyz\"\r\n\r\n" +
		"--xyz\r\nContent-Type: text/plain\r\n\r\nPlain Text Body\r\n" +
		"--xyz\r\nContent-Type: text/html\r\n\r\n<p>HTML Body</p>\r\n" +
		"--xyz--\r\n"

	_ = msg.Store(cachedir.At(tmpDir), "msg-html", []byte(emlData), time.Now())

	cfg := &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: tmpDir}, AccountCache: cachedir.At(tmpDir)}
	mockClient := &mailclient.MockMailClient{Cfg: cfg}
	m := newTuiModel(mockClient, "INBOX", nil)
	m.cfg = cfg
	m.width = 90
	m.height = 30
	m.emails = []email.Email{{ID: "msg-html", Subject: "HTML Email"}}
	m.eIdx = 0
	m.attachments = nil

	// Press 'v' -> loads attachments including alternative parts
	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	tm := res.(*tuiModel)

	if !tm.attachmentListOpen {
		t.Fatalf("expected attachmentListOpen=true")
	}
	if len(tm.attachments) == 0 {
		t.Fatalf("expected alternative body parts to be loaded into attachments")
	}

	hasHTML := false
	for _, att := range tm.attachments {
		if att.Filename == "message_body.html" {
			hasHTML = true
			break
		}
	}
	if !hasHTML {
		t.Errorf("expected message_body.html in attachments, got: %+v", tm.attachments)
	}

	// Attempt to delete message_body.html -> must be prevented
	tm.attachmentListCursor = 0
	res2, _ := tm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	tm2 := res2.(*tuiModel)
	if !strings.Contains(tm2.saveStatus, "Cannot delete message body part") {
		t.Errorf("expected deletion prevention message, got %q", tm2.saveStatus)
	}
}

// TestAttachmentDrawer_StaleAttCursorDoesNotPanic is a regression for a
// crash: kAttachmentDrawer's "enter"/"o"/"x", "v", "s", "S"/"b"/"B" and
// "c"/"C"/"u"/"U" handlers index m.attachments[m.attCursor] without checking
// m.attCursor is still in range, unlike the equivalent code in
// tui_keys_rightpane.go and tui_attachment_modal.go which both bound-check
// first.
//
// A stale, out-of-range attCursor is reachable in practice:
// updateAttachmentFocusOnScroll (tui_keys_actions.go) sets attCursor from
// viewport scroll position while the drawer auto-focuses, and
// applyAttachmentDeletions (tui_attachment_list.go) shrinks m.attachments
// after a deletion in the separate attachment-list overlay without ever
// touching attCursor -- so a cursor left pointing at the last of N
// attachments before a deletion is still pointing there, now out of range,
// after the list shrinks.
func TestAttachmentDrawer_StaleAttCursorDoesNotPanic(t *testing.T) {
	mockClient := &mailclient.MockMailClient{Cfg: &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: t.TempDir()}, AccountCache: cachedir.At(t.TempDir())}}

	keys := []string{"enter", "o", "x", "v", "s", "S", "b", "B", "c", "C", "u", "U"}
	for _, key := range keys {
		t.Run("key="+key, func(t *testing.T) {
			m := newTuiModel(mockClient, "INBOX", nil)
			m.mode = modeDetail
			m.width = 120
			m.height = 30
			// attCursor pointed at the last of 5 attachments (as
			// updateAttachmentFocusOnScroll would leave it while scrolling),
			// then a deletion elsewhere shrank the list to 2 without
			// touching attCursor.
			m.attachmentDrawer = true
			m.attCursor = 4
			m.attachments = []email.Attachment{
				{Filename: "a.txt", ContentType: "text/plain", Data: []byte("a")},
				{Filename: "b.txt", ContentType: "text/plain", Data: []byte("b")},
			}

			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("kAttachmentDrawer panicked on stale attCursor with key %q: %v", key, r)
				}
			}()
			var msg tea.KeyMsg
			if len(key) == 1 {
				msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
			} else {
				msg = tea.KeyMsg{Type: tea.KeyEnter}
			}
			m.Update(msg)
		})
	}
}

// Opening an attachment goes through the openSystemFile hook, which TestMain
// replaces. If a code path ever bypasses the hook, this fails to record and
// the real opener would have run.
func TestAttachmentDrawer_OpenGoesThroughHook(t *testing.T) {
	mockClient := &mailclient.MockMailClient{Cfg: &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: t.TempDir()}, AccountCache: cachedir.At(t.TempDir())}}
	m := newTuiModel(mockClient, "INBOX", nil)
	m.mode = modeDetail
	m.width, m.height = 120, 30
	m.attachmentDrawer = true
	m.attCursor = 0
	m.attachments = []email.Attachment{{Filename: "note.txt", ContentType: "text/plain", Data: []byte("x")}}

	openedFilesMu.Lock()
	before := len(openedFiles)
	openedFilesMu.Unlock()

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})

	// The opener is started on a goroutine; give it a moment.
	deadline := time.Now().Add(2 * time.Second)
	for {
		openedFilesMu.Lock()
		n := len(openedFiles)
		openedFilesMu.Unlock()
		if n > before {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("pressing o did not reach the openSystemFile hook")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
