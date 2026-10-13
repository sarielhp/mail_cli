package tui

import (
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"net/mail"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/mailclient"

	tea "github.com/charmbracelet/bubbletea"
)

func setupTestComposeModel(t *testing.T, readOnly bool) (*tuiModel, *mailclient.MockMailClient, string) {
	t.Helper()
	tempDir := t.TempDir()

	cfg := &cfg_g.Config{ReadOnly: readOnly, SelectedAccount: "Personal", Accounts: []cfg_acc.AccountConfig{
		{
			Name:        "Personal",
			DisplayName: "Personal User",
			Type:        "gmail",
			Username:    "user@personal.com",
			ReadOnly:    readOnly,
		},
		{
			Name:        "Work",
			DisplayName: "Work User",
			Type:        "jmap",
			Username:    "user@work.com",
		},
	}, Credentials: cfg_g.Credentials{Username: "user@personal.com"}, Paths: cfg_g.Paths{CacheRoot: tempDir}, AccountCache: cachedir.At(tempDir)}

	mockClient := &mailclient.MockMailClient{
		Cfg:            cfg,
		BackendTypeVal: "gmail",
	}

	bk := &Backend{
		Clients: stubClients{
			ForAccountFn: func(c *cfg_g.Config, acc cfg_acc.AccountConfig) (mailclient.MailClient, error) {
				return &mailclient.MockMailClient{
					Cfg:            c,
					BackendTypeVal: acc.Type,
				}, nil
			},
		},
		Folders: stubFolders{
			DraftsFn: func(client mailclient.MailClient) (string, error) {
				if client.BackendType() == "gmail" {
					return "DRAFTS", nil
				}
				return "Drafts", nil
			},
		},
	}

	m := newTuiModel(mockClient, "INBOX", bk)
	m.cfg = cfg
	m.width = 100
	m.height = 30
	return m, mockClient, tempDir
}

func sendComposeKey(m *tuiModel, k tea.KeyMsg) (*tuiModel, tea.Cmd) {
	m2, cmd := m.kCompose(k)
	return m2.(*tuiModel), cmd
}

func TestTUI_ComposeCmd_NoRawHeadersInEditor(t *testing.T) {
	m, _, _ := setupTestComposeModel(t, false)

	// Trigger compose
	cmd := m.composeCmd()
	if cmd == nil {
		t.Fatalf("expected non-nil cmd from composeCmd")
	}

	// Verify template generation
	tempPath, _, err := m.createComposeBodyTemplate()
	if err != nil {
		t.Fatalf("createComposeBodyTemplate error: %v", err)
	}
	defer os.Remove(tempPath)

	data, err := os.ReadFile(tempPath)
	if err != nil {
		t.Fatalf("failed to read created template: %v", err)
	}

	content := string(data)
	headers := []string{"From:", "To:", "Cc:", "Bcc:", "Subject:", "In-Reply-To:", "References:", "MIME-Version:"}
	for _, h := range headers {
		if strings.Contains(content, h) {
			t.Errorf("forbidden raw header %q found in editor buffer file:\n%s", h, content)
		}
	}
}

func TestTUI_ReplyCmd_NoRawHeadersInEditor(t *testing.T) {
	m, _, _ := setupTestComposeModel(t, false)

	rawOrig := "From: Sender <sender@example.com>\r\n" +
		"To: user@personal.com\r\n" +
		"Subject: Important Discussion\r\n" +
		"Date: Sun, 30 Aug 2026 12:00:00 +0000\r\n" +
		"Message-ID: <orig-123@example.com>\r\n" +
		"\r\n" +
		"Hello world,\r\nPlease review this proposal.\r\n"

	origMsg, err := mail.ReadMessage(strings.NewReader(rawOrig))
	if err != nil {
		t.Fatalf("failed to read test message: %v", err)
	}

	tempPath, _, err := m.createReplyBodyTemplate(origMsg)
	if err != nil {
		t.Fatalf("createReplyBodyTemplate error: %v", err)
	}
	defer os.Remove(tempPath)

	data, err := os.ReadFile(tempPath)
	if err != nil {
		t.Fatalf("failed to read created reply template: %v", err)
	}

	content := string(data)
	forbiddenHeaders := []string{"From:", "To:", "Cc:", "Bcc:", "Subject:", "In-Reply-To:", "References:", "MIME-Version:"}
	for _, h := range forbiddenHeaders {
		if strings.Contains(content, h) {
			t.Errorf("forbidden raw header %q found in reply editor buffer:\n%s", h, content)
		}
	}

	// Verify quoted lines are present
	if !strings.Contains(content, "> Hello world,") || !strings.Contains(content, "> Please review this proposal.") {
		t.Errorf("expected quoted lines in reply editor buffer, got:\n%s", content)
	}
}

func TestTUI_SendScreen_Rendering(t *testing.T) {
	m, _, _ := setupTestComposeModel(t, false)
	m.mode = modeCompose
	m.compose = &composeState{
		AccountName: "Personal",
		From:        "Personal User <user@personal.com>",
		To:          "alice@example.com",
		Cc:          "bob@example.com",
		Bcc:         "charlie@example.com",
		Subject:     "Quarterly Report",
		Body:        "Hi Alice,\nHere is the report.",
		Attachments: []email.Attachment{
			{
				Filename: "report.pdf",
				Data:     make([]byte, 1024*25), // 25 KB
			},
		},
	}
	m.compose.Vp.SetContent(m.compose.Body)

	rendered := m.renderComposeScreen()

	// Verify title and account
	if !strings.Contains(rendered, "COMPOSE & SEND REVIEW") {
		t.Errorf("missing title in compose screen")
	}
	if !strings.Contains(rendered, "Personal") {
		t.Errorf("missing active account name in compose screen")
	}

	// Verify fields
	if !strings.Contains(rendered, "Personal User <user@personal.com>") {
		t.Errorf("missing From header")
	}
	if !strings.Contains(rendered, "alice@example.com") {
		t.Errorf("missing To header")
	}
	if !strings.Contains(rendered, "bob@example.com") {
		t.Errorf("missing Cc header")
	}
	if !strings.Contains(rendered, "charlie@example.com") {
		t.Errorf("missing Bcc header")
	}
	if !strings.Contains(rendered, "Quarterly Report") {
		t.Errorf("missing Subject header")
	}

	// Verify attachments with size
	if !strings.Contains(rendered, "report.pdf") || !strings.Contains(rendered, "25.0 KB") {
		t.Errorf("missing attachment or formatted size in compose screen: %s", rendered)
	}
}

func TestTUI_Compose_FieldEditing(t *testing.T) {
	m, _, _ := setupTestComposeModel(t, false)
	m.mode = modeCompose
	m.compose = &composeState{
		AccountName: "Personal",
		From:        "user@personal.com",
		To:          "initial@example.com",
		Subject:     "Initial Subj",
		Body:        "Body text",
	}

	// 1. Edit To field ('t')
	m, _ = sendComposeKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	if m.compose.FieldEditing != fieldTo || m.compose.FieldInput != "initial@example.com" {
		t.Errorf("expected FieldEditing=fieldTo, got %v, input %q", m.compose.FieldEditing, m.compose.FieldInput)
	}

	// Backspace to clear and type new address
	for i := 0; i < len("initial@example.com"); i++ {
		m, _ = sendComposeKey(m, tea.KeyMsg{Type: tea.KeyBackspace})
	}
	for _, r := range "new@example.com" {
		m, _ = sendComposeKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m, _ = sendComposeKey(m, tea.KeyMsg{Type: tea.KeyEnter})

	if m.compose.FieldEditing != fieldNone {
		t.Errorf("expected FieldEditing=fieldNone after Enter")
	}
	if m.compose.To != "new@example.com" {
		t.Errorf("expected To='new@example.com', got %q", m.compose.To)
	}

	// 2. Edit Subject ('s')
	m, _ = sendComposeKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	if m.compose.FieldEditing != fieldSubject {
		t.Errorf("expected FieldEditing=fieldSubject")
	}
	for i := 0; i < len("Initial Subj"); i++ {
		m, _ = sendComposeKey(m, tea.KeyMsg{Type: tea.KeyBackspace})
	}
	for _, r := range "Updated Subject" {
		m, _ = sendComposeKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m, _ = sendComposeKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.compose.Subject != "Updated Subject" {
		t.Errorf("expected Subject='Updated Subject', got %q", m.compose.Subject)
	}

	// 3. Edit Cc and Cancel with Esc
	m, _ = sendComposeKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	for _, r := range "cancelled@example.com" {
		m, _ = sendComposeKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m, _ = sendComposeKey(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.compose.Cc != "" {
		t.Errorf("expected Cc to remain empty on Esc, got %q", m.compose.Cc)
	}
}

func TestTUI_Compose_AttachmentAddAndDelete(t *testing.T) {
	m, _, _ := setupTestComposeModel(t, false)
	m.mode = modeCompose
	m.compose = &composeState{
		AccountName: "Personal",
		From:        "user@personal.com",
		To:          "dest@example.com",
		Subject:     "Attachments Test",
		Body:        "Body",
		Attachments: []email.Attachment{},
	}

	// 1. Open file browser with 'a'
	m, _ = sendComposeKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	if !m.fileBrowserOpen || !m.fileBrowser.AllowFileSelect {
		t.Fatalf("expected fileBrowserOpen=true and AllowFileSelect=true")
	}

	// Create dummy file and select it
	tmpFile := filepath.Join(t.TempDir(), "document.pdf")
	_ = os.WriteFile(tmpFile, []byte("%PDF-dummy"), 0644)

	m.fileBrowser.CurrentDir = filepath.Dir(tmpFile)
	m.fileBrowser.ReloadEntries()
	for i, e := range m.fileBrowser.Filtered {
		if e.Name == "document.pdf" {
			m.fileBrowser.Cursor = i
			break
		}
	}

	res, _ := m.kFileBrowser(tea.KeyMsg{Type: tea.KeyEnter})
	m = res.(*tuiModel)
	if m.fileBrowserOpen {
		t.Errorf("expected fileBrowserOpen=false after file selection")
	}
	if len(m.compose.Attachments) != 1 {
		t.Fatalf("expected 1 attachment added, got %d", len(m.compose.Attachments))
	}
	if m.compose.Attachments[0].Filename != "document.pdf" {
		t.Errorf("expected attachment filename 'document.pdf', got %q", m.compose.Attachments[0].Filename)
	}

	// 2. Delete attachment with 'd'
	m, _ = sendComposeKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if len(m.compose.Attachments) != 0 {
		t.Errorf("expected 0 attachments after 'd', got %d", len(m.compose.Attachments))
	}
}

func TestTUI_Compose_IdentitySwitching(t *testing.T) {
	m, _, _ := setupTestComposeModel(t, false)
	m.mode = modeCompose
	m.compose = &composeState{
		AccountIndex: 0,
		AccountName:  "Personal",
		From:         "Personal User <user@personal.com>",
		To:           "dest@example.com",
		Subject:      "Identity Test",
		Body:         "Hello",
	}

	// F3 opens the account picker; nothing changes until a row is chosen.
	m, _ = sendComposeKey(m, tea.KeyMsg{Type: tea.KeyF3})
	if !m.accountOverlayOpen || m.accountCursor != 0 {
		t.Fatalf("F3 should open the overlay on the sending account, got open=%v cursor=%d", m.accountOverlayOpen, m.accountCursor)
	}
	if m.compose.AccountIndex != 0 || m.client.BackendType() != "gmail" {
		t.Fatalf("opening the picker must not switch anything, got %d / %s", m.compose.AccountIndex, m.client.BackendType())
	}
	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	m = res.(*tuiModel)
	if m.accountOverlayOpen {
		t.Fatal("choosing a row should close the overlay")
	}

	if m.compose.AccountIndex != 1 {
		t.Errorf("expected AccountIndex=1, got %d", m.compose.AccountIndex)
	}
	if m.compose.AccountName != "Work" {
		t.Errorf("expected AccountName='Work', got %q", m.compose.AccountName)
	}
	if !strings.Contains(m.compose.From, "user@work.com") {
		t.Errorf("expected From containing user@work.com, got %q", m.compose.From)
	}
	if m.client.BackendType() != "jmap" {
		t.Errorf("expected active client backend switched to jmap, got %q", m.client.BackendType())
	}
}

func TestTUI_Compose_DraftSavingAndDiscard(t *testing.T) {
	m, _, _ := setupTestComposeModel(t, false)
	m.mode = modeCompose
	m.compose = &composeState{
		AccountName: "Personal",
		From:        "user@personal.com",
		To:          "draft@example.com",
		Subject:     "Draft Test",
		Body:        "Saved draft content",
	}

	// Save draft locally
	path, err := m.saveLocalDraft()
	if err != nil {
		t.Fatalf("saveLocalDraft failed: %v", err)
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatalf("expected draft file to exist on disk at %s", path)
	}

	// Verify draft content
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "Saved draft content") || !strings.Contains(string(data), "Subject: Draft Test") {
		t.Errorf("draft file missing expected RFC822 content:\n%s", string(data))
	}

	// Discard draft ('x' -> prompt -> 'y')
	m, _ = sendComposeKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if !m.compose.DiscardConfirm {
		t.Fatalf("expected DiscardConfirm=true after 'x'")
	}

	m, _ = sendComposeKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if m.mode != modeIndex {
		t.Errorf("expected modeIndex after confirming discard, got %v", m.mode)
	}
	if m.compose != nil {
		t.Errorf("expected compose state cleared after discard")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("expected draft file %s to be deleted after discard", path)
	}
}

func TestTUI_Compose_SendSuccessAndCleanup(t *testing.T) {
	m, mockClient, _ := setupTestComposeModel(t, false)
	m.mode = modeCompose
	m.compose = &composeState{
		AccountName: "Personal",
		From:        "user@personal.com",
		To:          "dest@example.com",
		Subject:     "Send Test",
		Body:        "Testing successful send",
	}

	path, _ := m.saveLocalDraft()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatalf("expected draft file at %s", path)
	}

	// Send message ('y')
	_, cmd := sendComposeKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if cmd == nil {
		t.Fatalf("expected cmd on send")
	}

	msg := cmd()
	sentMsg, ok := msg.(emailSentMsg)
	if !ok || sentMsg.err != nil {
		t.Fatalf("expected successful emailSentMsg, got %v", msg)
	}

	// Verify client received sent email
	if len(mockClient.SentRawEmails) != 1 {
		t.Errorf("expected 1 sent email on mockClient, got %d", len(mockClient.SentRawEmails))
	}

	// Verify draft cleaned up
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("expected local draft to be cleaned up after successful send")
	}
}

func TestTUI_Compose_SaveSentCopyConfiguration(t *testing.T) {
	cfg := &cfg_g.Config{
		SaveSentCopy:    true,
		SelectedAccount: "Acc1",
		Accounts: []cfg_acc.AccountConfig{
			{Name: "Acc1"},
			{Name: "Acc2", SaveSentCopy: new(bool)}, // false
		},
	}

	if !cfg.IsSaveSentCopyEnabled() {
		t.Errorf("expected SaveSentCopy enabled for Acc1 by default")
	}

	cfg.SelectedAccount = "Acc2"
	if cfg.IsSaveSentCopyEnabled() {
		t.Errorf("expected SaveSentCopy disabled for Acc2 via account config override")
	}
}

func TestTUI_Compose_DraftDirectoryPermissions(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	m, _, _ := setupTestComposeModel(t, false)
	m.compose = &composeState{
		To:      "test@example.com",
		Subject: "Permissions Test",
		Body:    "Test body",
	}

	path, err := m.saveLocalDraft()
	if err != nil {
		t.Fatalf("saveLocalDraft failed: %v", err)
	}
	defer cleanupLocalDraft(path)

	draftDir := filepath.Dir(path)
	info, err := os.Stat(draftDir)
	if err != nil {
		t.Fatalf("failed to stat draft directory %s: %v", draftDir, err)
	}
	if info.Mode().Perm()&0077 != 0 {
		t.Errorf("expected draft directory to have 0700 permissions, got %o", info.Mode().Perm())
	}
}

func TestTUI_Compose_StandaloneExitOnSent(t *testing.T) {
	m, _, _ := setupTestComposeModel(t, false)
	m.standaloneCompose = true
	m.mode = modeCompose
	m.compose = &composeState{
		To:      "user@example.com",
		Subject: "Test",
	}

	_, cmd := m.handleEmailSent(emailSentMsg{err: nil})
	if cmd == nil {
		t.Fatalf("expected tea.Quit cmd after successful email send in standalone mode")
	}
}

func TestTUI_Compose_StandaloneExitOnDiscard(t *testing.T) {
	m, _, _ := setupTestComposeModel(t, false)
	m.standaloneCompose = true
	m.mode = modeCompose
	m.compose = &composeState{
		DiscardConfirm: true,
		DraftPath:      filepath.Join(t.TempDir(), "draft.eml"),
	}

	_, cmd := m.handleComposeDiscardPrompt(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if cmd == nil {
		t.Fatalf("expected tea.Quit cmd after confirming discard in standalone mode")
	}
}

// The old single-key cycle on f/F/A/I switched the sender the moment a key
// was pressed, and a stray press changed the From line without the user
// noticing. Those keys now do nothing; only the F3 picker switches.
func TestTUI_Compose_AccountSwitch_OnlyThroughPicker(t *testing.T) {
	m, _, _ := setupTestComposeModel(t, false)
	m.mode = modeCompose
	m.compose = &composeState{
		AccountIndex: 0,
		AccountName:  "Personal",
		From:         "Personal User <user@personal.com>",
		To:           "dest@example.com",
		Subject:      "Identity Test",
		Body:         "Hello",
	}

	for _, r := range []rune{'f', 'F', 'A', 'I'} {
		m, _ = sendComposeKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		if m.compose.AccountIndex != 0 || m.accountOverlayOpen {
			t.Fatalf("%q must neither switch nor open the picker, got index %d open=%v", r, m.compose.AccountIndex, m.accountOverlayOpen)
		}
	}

	// Esc on the picker cancels without switching; Enter on a moved cursor switches.
	m, _ = sendComposeKey(m, tea.KeyMsg{Type: tea.KeyF3})
	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = res.(*tuiModel)
	if m.accountOverlayOpen || m.compose.AccountIndex != 0 {
		t.Fatalf("Esc should cancel, got open=%v index %d", m.accountOverlayOpen, m.compose.AccountIndex)
	}
	m, _ = sendComposeKey(m, tea.KeyMsg{Type: tea.KeyF3})
	for _, k := range []tea.KeyMsg{{Type: tea.KeyRunes, Runes: []rune{'j'}}, {Type: tea.KeyEnter}} {
		res, _ = m.Update(k)
		m = res.(*tuiModel)
	}
	if m.compose.AccountIndex != 1 || m.compose.AccountName != "Work" || m.client.BackendType() != "jmap" {
		t.Fatalf("expected switched to Work via picker, got %s (%d) client %s", m.compose.AccountName, m.compose.AccountIndex, m.client.BackendType())
	}
	if m.mode != modeCompose || m.compose == nil {
		t.Fatal("picking a sender must keep the compose session open")
	}
}

func TestTUI_Compose_Views_FromAndFooter(t *testing.T) {
	m, _, _ := setupTestComposeModel(t, false)
	m.mode = modeCompose
	m.compose = &composeState{
		AccountIndex: 0,
		AccountName:  "Personal",
		From:         "user@personal.com",
		To:           "to@example.com",
		Subject:      "Test Subj",
		Body:         "Body",
	}

	headers := renderComposeHeaderFields(m)
	headersJoined := strings.Join(headers, "\n")
	if !strings.Contains(headersJoined, "(F3 to change)") {
		t.Errorf("expected header fields to contain '(F3 to change)', got:\n%s", headersJoined)
	}

	footer := renderComposeFooter()
	if !strings.Contains(footer, "[F3]") || !strings.Contains(footer, "Account") {
		t.Errorf("expected footer to contain '[F3]' and 'Account', got:\n%s", footer)
	}
}
