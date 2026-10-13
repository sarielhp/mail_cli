package tui

import (
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarielhp/mail_cli/contacts"
	"github.com/sarielhp/mail_cli/email"

	tea "github.com/charmbracelet/bubbletea"
)

func TestTUI_Compose_SaveDraftAndExit(t *testing.T) {
	m, _, _ := setupTestComposeModel(t, false)
	m.mode = modeCompose
	m.compose = &composeState{
		AccountName: "Personal",
		From:        "user@personal.com",
		To:          "someone@example.com",
		Subject:     "Pending Email",
		Body:        "Don't lose this text",
	}

	// Exit with 'q'
	m2, cmd := m.kCompose(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	m = m2.(*tuiModel)

	if m.mode != modeIndex {
		t.Errorf("expected modeIndex after 'q', got %v", m.mode)
	}
	if m.compose != nil {
		t.Errorf("expected compose state cleared after exit")
	}
	if !strings.Contains(m.saveStatus, "Draft saved to") {
		t.Errorf("expected saveStatus to indicate draft saved, got %q", m.saveStatus)
	}

	// Verify sync command was returned
	if cmd != nil {
		_ = cmd() // background sync command runs cleanly
	}
}

func TestTUI_Compose_PreviewAttachment(t *testing.T) {
	m, _, _ := setupTestComposeModel(t, false)
	m.mode = modeCompose
	m.compose = &composeState{
		AccountName: "Personal",
		From:        "user@personal.com",
		To:          "someone@example.com",
		Subject:     "Attachment Preview",
		Body:        "Here is the attachment",
		Attachments: []email.Attachment{
			{
				Filename:    "info.txt",
				ContentType: "text/plain",
				Data:        []byte("Important text preview data"),
			},
		},
		AttCursor: 0,
	}

	// View attachment ('v')
	m2, _ := m.kCompose(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	m = m2.(*tuiModel)

	if !m.attModalOpen {
		t.Fatalf("expected attModalOpen=true after 'v'")
	}
	if m.attPreviewAtt.Filename != "info.txt" {
		t.Errorf("expected preview filename info.txt, got %q", m.attPreviewAtt.Filename)
	}

	// Close modal with Esc
	m2, _ = m.kAttachmentModal(tea.KeyMsg{Type: tea.KeyEsc})
	m = m2.(*tuiModel)
	if m.attModalOpen {
		t.Errorf("expected attModalOpen=false after Esc")
	}
	if m.mode != modeCompose {
		t.Errorf("expected to remain in modeCompose after closing attachment modal")
	}
}

func TestTUI_Compose_ReEditBodyCmd(t *testing.T) {
	m, _, _ := setupTestComposeModel(t, false)
	m.mode = modeCompose
	m.compose = &composeState{
		AccountName: "Personal",
		From:        "user@personal.com",
		To:          "user@example.com",
		Subject:     "Edit test",
		Body:        "Initial body text",
	}

	cmd := m.reEditBodyCmd()
	if cmd == nil {
		t.Fatalf("expected non-nil cmd from reEditBodyCmd")
	}

	// Simulate editor finished with updated text
	tempFile, _ := os.CreateTemp("", "reedit_*.txt")
	_, _ = tempFile.WriteString("Updated body text from editor")
	_ = tempFile.Close()

	res, _ := m.handleComposeEditorFinished(composeEditorFinishedMsg{
		err:      nil,
		path:     tempFile.Name(),
		isReEdit: true,
	})
	m = res.(*tuiModel)

	if m.compose.Body != "Updated body text from editor" {
		t.Errorf("expected updated body text, got %q", m.compose.Body)
	}
}

func TestTUI_Compose_BackgroundDraftSync(t *testing.T) {
	m, mockClient, _ := setupTestComposeModel(t, false)
	m.compose = &composeState{From: "user@personal.com", To: "test@example.com", Subject: "Draft", Body: "Draft Body"}

	rawDraft := []byte("From: user@personal.com\r\nTo: test@example.com\r\nSubject: Draft\r\n\r\nDraft Body")
	syncCmd := m.requestSync(m.composeSession(), syncRequest{kind: syncUpload, raw: rawDraft, client: mockClient})
	if syncCmd == nil {
		t.Fatalf("expected non-nil syncCmd")
	}

	res, ok := syncCmd().(draftSyncedMsg)
	if !ok || res.err != nil || res.draft.Folder.String() != "DRAFTS" || res.draft.Account != "Personal" || res.draft.ID == "" || res.session != m.compose.MessageID {
		t.Errorf("expected a clean draftSyncedMsg for Personal/DRAFTS in this session, got %+v (ok=%v)", res, ok)
	}

	if len(mockClient.UploadedEmails) != 1 {
		t.Fatalf("expected 1 uploaded email to Drafts, got %d", len(mockClient.UploadedEmails))
	}
	if mockClient.UploadedEmails[0].TargetLabel != "DRAFTS" {
		t.Errorf("expected target label DRAFTS, got %q", mockClient.UploadedEmails[0].TargetLabel)
	}
}

func TestTUI_Compose_KeysInIndexAndDetailModes(t *testing.T) {
	m, _, _ := setupTestComposeModel(t, false)

	// modeIndex: 'c' triggers compose
	m.mode = modeIndex
	_, cmd := m.kIndexActions(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	if cmd == nil {
		t.Errorf("expected cmd from 'c' in modeIndex")
	}

	// modeDetail: 'c' triggers compose
	m.mode = modeDetail
	m.selectedID = "msg1"
	_, cmdDetail := m.kDetail(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	if cmdDetail == nil {
		t.Errorf("expected cmd from 'c' in modeDetail")
	}
}

func TestTUI_Compose_UnmodifiedAborts(t *testing.T) {
	m, _, _ := setupTestComposeModel(t, false)

	// Create initial state with template
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "body.txt")
	templateText := "\n-- \nMy Signature\n"
	_ = os.WriteFile(filePath, []byte(templateText), 0600)

	initialState := &composeState{
		From:            "user@personal.com",
		To:              "alice@example.com",
		Subject:         "Test Subject",
		InitialTemplate: templateText,
	}

	resModel, _ := m.handleComposeEditorFinished(composeEditorFinishedMsg{
		path:         filePath,
		initialState: initialState,
		isReEdit:     false,
	})
	mMod := resModel.(*tuiModel)

	if mMod.mode != modeIndex {
		t.Errorf("expected modeIndex after unmodified compose, got %v", mMod.mode)
	}
	if mMod.compose != nil {
		t.Errorf("expected compose session to be nil after unmodified compose")
	}
	if !strings.Contains(mMod.saveStatus, "Message not modified") {
		t.Errorf("expected 'Message not modified' in saveStatus, got %q", mMod.saveStatus)
	}
}

func TestTUI_Compose_WarnEmptySubjectOrBody(t *testing.T) {
	m, mockClient, _ := setupTestComposeModel(t, false)
	m.mode = modeCompose
	m.compose = &composeState{
		AccountName: "Personal",
		From:        "user@personal.com",
		To:          "alice@example.com",
		Subject:     "", // empty subject
		Body:        "Hello there",
	}

	// 1. Press 'y' to send -> should trigger empty subject warning
	m2, cmd := sendComposeKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if cmd != nil {
		t.Errorf("expected no immediate send command when subject is empty")
	}
	if !m2.compose.WarnEmptySend {
		t.Errorf("expected WarnEmptySend to be true")
	}
	if !strings.Contains(m2.compose.WarnEmptyReason, "Subject is empty") {
		t.Errorf("expected 'Subject is empty' in reason, got %q", m2.compose.WarnEmptyReason)
	}

	// View rendering should show warning dialog
	rendered := m2.renderComposeScreen()
	if !strings.Contains(rendered, "WARNING: EMPTY EMAIL FIELDS") {
		t.Errorf("expected warning dialog in rendered view, got:\n%s", rendered)
	}

	// 2. Press 'n' to cancel warning and return to edit
	m3, _ := sendComposeKey(m2, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if m3.compose.WarnEmptySend {
		t.Errorf("expected WarnEmptySend to be cleared after 'n'")
	}
	if len(mockClient.SentRawEmails) != 0 {
		t.Errorf("expected no sent emails on cancel")
	}

	// 3. Trigger warning again, then confirm with 'y' -> sends email
	m4, _ := sendComposeKey(m3, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if !m4.compose.WarnEmptySend {
		t.Errorf("expected WarnEmptySend to be true again")
	}
	m5, sendCmd := sendComposeKey(m4, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if sendCmd == nil {
		t.Fatalf("expected sendCmd on confirming send anyway")
	}
	_ = sendCmd()
	if len(mockClient.SentRawEmails) != 1 {
		t.Errorf("expected 1 sent email after confirming warning, got %d", len(mockClient.SentRawEmails))
	}
	_ = m5
}

func TestTUI_Compose_TabCompletion(t *testing.T) {
	m, _, _ := setupTestComposeModel(t, false)
	tmpDir := t.TempDir()
	m.cfg.Paths.ConfigDir = tmpDir
	m.cfg.Paths.CacheRoot = tmpDir
	m.cfg.AccountCache = cachedir.At(m.cfg.Paths.CacheRoot)

	book := contacts.NewBook()
	book.Contacts = append(book.Contacts, contacts.Contact{
		Name:      "Alice Smith",
		Emails:    []string{"alice@example.com"},
		Nicknames: []string{"al"},
	})
	if err := contacts.SaveBook(contacts.DefaultPath(m.cfg), book); err != nil {
		t.Fatalf("SaveBook failed: %v", err)
	}

	m.mode = modeCompose
	m.compose = &composeState{
		AccountName:  "Personal",
		From:         "user@personal.com",
		FieldEditing: fieldTo,
		FieldInput:   "al",
	}

	// Press Tab
	m2, _ := m.kComposeFieldEdit(tea.KeyMsg{Type: tea.KeyTab})
	m = m2.(*tuiModel)

	expected := "Alice Smith <alice@example.com>"
	if m.compose.FieldInput != expected {
		t.Errorf("expected FieldInput %q after Tab, got %q", expected, m.compose.FieldInput)
	}

	// Press Enter to commit
	m3, _ := m.kComposeFieldEdit(tea.KeyMsg{Type: tea.KeyEnter})
	m = m3.(*tuiModel)

	if m.compose.To != expected {
		t.Errorf("expected To %q after Enter, got %q", expected, m.compose.To)
	}
	if m.compose.FieldEditing != fieldNone {
		t.Errorf("expected FieldEditing None after Enter, got %v", m.compose.FieldEditing)
	}
}
