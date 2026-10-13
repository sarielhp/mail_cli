package tui

import (
	"bytes"
	"context"
	"fmt"
	"mime"
	"net/mail"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"github.com/sarielhp/mail_cli/app"
	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/pgp"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

// ComposeOptions configures direct launch into compose mode.
type ComposeOptions struct {
	To          []string
	Cc          []string
	Bcc         []string
	Subject     string
	Body        string
	Attachments []email.Attachment
	InReplyTo   string
	References  string
	OriginalID  email.ID
	// DraftPath resumes a locally saved draft: the review screen opens on its
	// contents and later saves overwrite the same file.
	DraftPath string
}

type composeField int

const (
	fieldNone composeField = iota
	fieldTo
	fieldCc
	fieldBcc
	fieldSubject
)

type composeState struct {
	OriginalID      email.ID
	IsGroupReply    bool
	AccountIndex    int
	AccountName     string
	From            string
	To              string
	Cc              string
	Bcc             string
	Subject         string
	InReplyTo       string
	References      string
	Body            string
	InitialTemplate string
	Attachments     []email.Attachment
	DraftPath       string
	// MessageID is fixed for the session so every draft copy and the sent
	// message carry the same one.
	MessageID string
	// sending is set while a send is in flight; quitAfterSend asks the
	// program to exit once it reports.
	sending         bool
	quitAfterSend   bool
	AttCursor       int
	Vp              viewport.Model
	FieldEditing    composeField
	FieldInput      string
	DiscardConfirm  bool
	WarnEmptySend   bool
	WarnEmptyReason string
	// SendConfirm asks before a clicked Send goes out; the y key needs no
	// question, a stray click does.
	SendConfirm       bool
	IsReEdit          bool
	SuggestCandidates []string
	SuggestIndex      int
	SuggestQuery      string
	// Sign and Encrypt are the session's PGP choices, toggled with p and P.
	Sign    bool
	Encrypt bool
	// PgpKeyID names the signing key for this session; empty falls back to the
	// account's key and then to gpg's default.
	PgpKeyID string
}

type composeEditorFinishedMsg struct {
	err          error
	path         string
	initialState *composeState
	isReEdit     bool
}

type emailSentMsg struct {
	err      error
	targetID email.ID
	session  string
}

// messageIDForCompose is the Message-ID a session keeps for every copy.
func messageIDForCompose(c *composeState) string {
	return email.MessageIDForSender(c.From)
}

func (m *tuiModel) composeCmd() tea.Cmd {
	cfg := m.Config()
	if cfg == nil {
		cfg = m.cfg
	}

	activeAcc, activeIdx := m.resolveActiveAccount()
	fromHeader := formatFromHeader(activeAcc, cfg)

	initialState := &composeState{
		AccountIndex: activeIdx,
		AccountName:  activeAcc.Name,
		From:         fromHeader,
		To:           "",
		Cc:           "",
		Bcc:          "",
		Subject:      "",
		Attachments:  []email.Attachment{},
	}

	tempPath, templateContent, err := m.createComposeBodyTemplate()
	if err != nil {
		return func() tea.Msg {
			return composeEditorFinishedMsg{err: fmt.Errorf("failed to create compose body template: %w", err)}
		}
	}
	initialState.InitialTemplate = templateContent

	return m.openEditorCmd(tempPath, initialState, false)
}

func (m *tuiModel) replyCmd(targetID email.ID, groupReply bool) tea.Cmd {
	cfg := m.Config()
	if cfg == nil {
		cfg = m.cfg
	}
	if cfg == nil {
		return func() tea.Msg {
			return composeEditorFinishedMsg{err: fmt.Errorf("configuration unavailable")}
		}
	}

	// Through the accessor: replying to a message that was never downloaded
	// used to fail outright.
	data, err := m.client.GetRawMessage(targetID)
	if err != nil {
		return func() tea.Msg {
			return composeEditorFinishedMsg{err: fmt.Errorf("could not retrieve the message being replied to: %w", err)}
		}
	}

	origMsg, err := mail.ReadMessage(bytes.NewReader(data))
	if err != nil {
		return func() tea.Msg {
			return composeEditorFinishedMsg{err: fmt.Errorf("failed to parse email: %w", err)}
		}
	}

	activeAcc, activeIdx := m.resolveActiveAccount()
	fromHeader := formatFromHeader(activeAcc, cfg)

	dec := new(mime.WordDecoder)
	toAddress := origMsg.Header.Get("Reply-To")
	if toAddress == "" {
		toAddress = origMsg.Header.Get("From")
	}

	subject := origMsg.Header.Get("Subject")
	subjDec := email.DecDecode(dec, subject)
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(subjDec)), "re:") {
		subjDec = "Re: " + subjDec
	}

	msgID := origMsg.Header.Get("Message-ID")
	origRefs := origMsg.Header.Get("References")
	inReplyTo := strings.TrimSpace(msgID)
	references := ""
	if origRefs != "" {
		references = strings.TrimSpace(origRefs) + " " + inReplyTo
	} else {
		references = inReplyTo
	}

	var ccAddresses []string
	if groupReply {
		ourEmail := cfg.Username
		if sendAddr := activeAcc.GetSendAddress(); sendAddr != "" {
			ourEmail = sendAddr
		}
		ccAddresses = buildGroupReplyCC(origMsg, toAddress, ourEmail)
	}

	initialState := &composeState{
		OriginalID:   targetID,
		IsGroupReply: groupReply,
		AccountIndex: activeIdx,
		AccountName:  activeAcc.Name,
		From:         fromHeader,
		To:           toAddress,
		Cc:           strings.Join(ccAddresses, ", "),
		Bcc:          "",
		Subject:      subjDec,
		InReplyTo:    inReplyTo,
		References:   references,
		Attachments:  []email.Attachment{},
	}

	tempPath, templateContent, err := m.createReplyBodyTemplate(origMsg)
	if err != nil {
		return func() tea.Msg {
			return composeEditorFinishedMsg{err: fmt.Errorf("failed to create reply body template: %w", err)}
		}
	}
	initialState.InitialTemplate = templateContent

	return m.openEditorCmd(tempPath, initialState, false)
}

func buildGroupReplyCC(origMsg *mail.Message, toAddress string, ourEmail string) []string {
	return email.BuildGroupReplyCC(origMsg, toAddress, ourEmail)
}

func (m *tuiModel) createComposeBodyTemplate() (string, string, error) {
	return m.createComposeBodyTemplateWithContent("")
}

func (m *tuiModel) createComposeBodyTemplateWithContent(initialBody string) (string, string, error) {
	tempDir, err := app.GetTempDir()
	if err != nil {
		return "", "", err
	}
	tempPath := filepath.Join(tempDir, fmt.Sprintf("compose_body_%d.txt", time.Now().UnixNano()))

	sig := readSignature()
	content := initialBody
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	if sig != "" {
		if content != "" {
			content += "\n" + sig
		} else {
			content = "\n" + sig
		}
	}

	if err := os.WriteFile(tempPath, []byte(content), 0600); err != nil {
		return "", "", err
	}
	baseline := ""
	if initialBody == "" {
		baseline = content
	}
	return tempPath, baseline, nil
}

func (m *tuiModel) initStandaloneComposeCmd() tea.Cmd {
	m.lastTime = time.Now()
	if m.composeOpts.DraftPath != "" {
		return m.resumeDraftCmd(m.composeOpts.DraftPath)
	}
	activeAcc, activeIdx := m.resolveActiveAccount()
	cfg := m.Config()
	if cfg == nil {
		cfg = m.cfg
	}
	fromHeader := formatFromHeader(activeAcc, cfg)

	initialState := &composeState{
		OriginalID:   m.composeOpts.OriginalID,
		AccountIndex: activeIdx,
		AccountName:  activeAcc.Name,
		From:         fromHeader,
		To:           strings.Join(m.composeOpts.To, ", "),
		Cc:           strings.Join(m.composeOpts.Cc, ", "),
		Bcc:          strings.Join(m.composeOpts.Bcc, ", "),
		Subject:      m.composeOpts.Subject,
		InReplyTo:    m.composeOpts.InReplyTo,
		References:   m.composeOpts.References,
		Attachments:  m.composeOpts.Attachments,
	}
	if initialState.Attachments == nil {
		initialState.Attachments = []email.Attachment{}
	}

	tempPath, templateContent, err := m.createComposeBodyTemplateWithContent(m.composeOpts.Body)
	if err != nil {
		return func() tea.Msg {
			return composeEditorFinishedMsg{err: fmt.Errorf("failed to create compose body template: %w", err)}
		}
	}
	initialState.InitialTemplate = templateContent

	return m.openEditorCmd(tempPath, initialState, false)
}

// resumeDraftCmd opens the review screen on a saved draft instead of the
// editor: the text is already written, and 'e' edits it. The draft file
// stays the session's local copy, so saves overwrite it rather than adding
// a second draft.
func (m *tuiModel) resumeDraftCmd(path string) tea.Cmd {
	raw, err := os.ReadFile(path)
	if err != nil {
		return func() tea.Msg { return composeEditorFinishedMsg{err: fmt.Errorf("cannot read draft: %w", err)} }
	}
	// An encrypted session's draft is encrypted to its own sender, so it has to
	// be decrypted before it can be read back. Process leaves a draft that was
	// never protected alone, so this is a no-op for an ordinary one.
	processed := pgp.Process(context.Background(), raw, pgp.Options{Decrypt: true, Verify: true})
	cm, err := email.ParseRFC822ToCompose(processed.Raw)
	if err != nil {
		return func() tea.Msg {
			return composeEditorFinishedMsg{err: fmt.Errorf("cannot parse draft %s: %w", path, err)}
		}
	}
	activeAcc, activeIdx := m.resolveActiveAccount()
	if idx, ok := m.accountForSender(cm.From); ok && idx != activeIdx {
		activeAcc, activeIdx = m.getAccounts()[idx], idx
		if c, err := m.resolveClientForSwitch(activeAcc); err == nil && c != nil {
			m.client = c
		}
	}
	cfg := m.Config()
	if cfg == nil {
		cfg = m.cfg
	}
	state := &composeState{
		AccountIndex: activeIdx,
		AccountName:  activeAcc.Name,
		From:         formatFromHeader(activeAcc, cfg),
		To:           cm.To,
		Cc:           cm.Cc,
		Bcc:          cm.Bcc,
		Subject:      cm.Subject,
		InReplyTo:    cm.InReplyTo,
		References:   cm.References,
		Body:         cm.Body,
		Attachments:  cm.Attachments,
		DraftPath:    path,
		MessageID:    cm.MessageID,
		// The draft's own form is the record of what the session was doing: a
		// draft that came back encrypted was written by an encrypted session.
		// A signed-only session leaves no such mark -- its draft is in the
		// clear -- so p has to be pressed again after a resume.
		Encrypt: processed.Status != nil && processed.Status.Encrypted,
	}
	if state.Attachments == nil {
		state.Attachments = []email.Attachment{}
	}
	state.Vp = viewport.New(max(20, m.width-6), max(5, m.height-14))
	state.Vp.SetContent(state.Body)
	m.compose = state
	m.mode = modeCompose
	return m.requestSync(m.composeSession(), syncRequest{kind: syncAdopt, client: m.client})
}

// accountForSender finds the configured account whose address is in from,
// so a resumed draft is sent from the account it was written from.
func (m *tuiModel) accountForSender(from string) (int, bool) {
	addr, err := mail.ParseAddress(from)
	if err != nil {
		return 0, false
	}
	for i, acc := range m.getAccounts() {
		if acc.Username != "" && strings.EqualFold(acc.Username, addr.Address) {
			return i, true
		}
		if acc.UserEmail != "" && strings.EqualFold(acc.UserEmail, addr.Address) {
			return i, true
		}
	}
	return 0, false
}

func (m *tuiModel) createReplyBodyTemplate(orig *mail.Message) (string, string, error) {
	tempDir, err := app.GetTempDir()
	if err != nil {
		return "", "", err
	}
	tempPath := filepath.Join(tempDir, fmt.Sprintf("reply_body_%d.txt", time.Now().UnixNano()))

	var buf bytes.Buffer
	buf.WriteString("\n")

	quote := email.FormatReplyQuote(orig)
	if quote != "" {
		buf.WriteString(quote)
	}

	sig := readSignature()
	if sig != "" {
		buf.WriteString(sig)
	}

	templateContent := buf.String()
	if err := os.WriteFile(tempPath, buf.Bytes(), 0600); err != nil {
		return "", "", err
	}
	return tempPath, templateContent, nil
}

func (m *tuiModel) openEditorCmd(tempPath string, state *composeState, isReEdit bool) tea.Cmd {
	cfg := m.Config()
	if cfg == nil {
		cfg = m.cfg
	}

	editor := ""
	if cfg != nil {
		editor = cfg.Editor
	}
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		editor = "emacs"
	}

	var args []string
	if cfg != nil && len(cfg.EditorArgs) > 0 {
		args = append(args, cfg.EditorArgs...)
	} else if editor == "emacs" {
		args = append(args, "-nw")
	}
	args = append(args, tempPath)

	c := exec.Command(editor, args...)
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return composeEditorFinishedMsg{
			err:          err,
			path:         tempPath,
			initialState: state,
			isReEdit:     isReEdit,
		}
	})
}

func (m *tuiModel) reEditBodyCmd() tea.Cmd {
	if m.compose == nil {
		return nil
	}
	tempDir, err := app.GetTempDir()
	if err != nil {
		m.err = err
		m.showError = true
		return nil
	}
	tempPath := filepath.Join(tempDir, fmt.Sprintf("edit_body_%d.txt", time.Now().UnixNano()))
	if err := os.WriteFile(tempPath, []byte(m.compose.Body), 0600); err != nil {
		m.err = err
		m.showError = true
		return nil
	}

	return m.openEditorCmd(tempPath, m.compose, true)
}

// leaveAfterEditorFailure ends a standalone compose whose editor failed,
// saying why and where any earlier copy of the draft is.
func (m *tuiModel) leaveAfterEditorFailure(_ composeEditorFinishedMsg) (tea.Model, tea.Cmd) {
	if !m.standaloneCompose {
		return m, nil
	}
	m.saveStatus = m.err.Error()
	if m.compose != nil && m.compose.DraftPath != "" {
		m.saveStatus = joinStatus(m.saveStatus, "draft kept at "+m.compose.DraftPath)
	}
	return m, m.quitNow()
}

func (m *tuiModel) handleComposeEditorFinished(msg composeEditorFinishedMsg) (tea.Model, tea.Cmd) {
	defer os.Remove(msg.path)

	if msg.err != nil {
		m.err = fmt.Errorf("editor session failed: %w", msg.err)
		m.showError = true
		return m.leaveAfterEditorFailure(msg)
	}

	bodyBytes, err := os.ReadFile(msg.path)
	if err != nil {
		m.err = fmt.Errorf("failed to read edited body: %w", err)
		m.showError = true
		return m.leaveAfterEditorFailure(msg)
	}

	// An editor that exits leaving an empty file -- killed, crashed, or quit
	// before its first save -- must not be taken as "the message is empty":
	// that is how a draft was once saved with no body. Keep what there was.
	if msg.isReEdit && m.compose != nil && strings.TrimSpace(string(bodyBytes)) == "" && strings.TrimSpace(m.compose.Body) != "" {
		m.saveStatus = "Editor returned an empty file; body kept."
		return m, nil
	}

	if !msg.isReEdit && msg.initialState != nil {
		editedText := strings.TrimSpace(string(bodyBytes))
		origText := strings.TrimSpace(msg.initialState.InitialTemplate)
		// An untouched or emptied template cancels, unless the command line
		// supplied a subject or attachments: those are a message worth keeping,
		// and the review screen warns before an empty body is sent.
		fromFlags := m.standaloneCompose && (m.composeOpts.Subject != "" || len(m.composeOpts.Attachments) > 0)
		if (editedText == origText || editedText == "") && !fromFlags {
			m.saveStatus = "Message not modified; compose cancelled."
			m.compose = nil
			m.mode = modeIndex
			if m.standaloneCompose {
				return m, m.quitNow()
			}
			return m, nil
		}
	}

	if msg.isReEdit && m.compose != nil {
		m.compose.Body = string(bodyBytes)
		m.compose.Vp.SetContent(m.compose.Body)
		return m, m.saveAndSyncDraftCmd()
	}

	state := msg.initialState
	if state == nil {
		state = &composeState{}
	}
	state.Body = string(bodyBytes)

	vpW := max(20, m.width-6)
	vpH := max(5, m.height-14)
	state.Vp = viewport.New(vpW, vpH)
	state.Vp.SetContent(state.Body)

	m.compose = state
	m.mode = modeCompose

	return m, m.saveAndSyncDraftCmd()
}

func (m *tuiModel) resolveActiveAccount() (cfg_acc.AccountConfig, int) {
	var activeAcc cfg_acc.AccountConfig
	activeIdx := 0
	cfg := m.Config()
	if cfg == nil {
		cfg = m.cfg
	}
	if cfg != nil {
		for i, acc := range cfg.Accounts {
			if cfg.IsSelected(acc) {
				activeAcc = acc
				activeIdx = i
				break
			}
		}
		if activeAcc.Name == "" && len(cfg.Accounts) > 0 {
			activeAcc = cfg.Accounts[0]
			activeIdx = 0
		}
	}
	return activeAcc, activeIdx
}

func formatFromHeader(acc cfg_acc.AccountConfig, cfg *cfg_g.Config) string {
	fullName := acc.GetSenderName()
	if fullName == "" {
		currUser, err := user.Current()
		if err == nil && currUser.Name != "" {
			fullName = currUser.Name
		}
	}

	emailAddr := acc.GetSendAddress()
	if emailAddr == "" && cfg != nil {
		emailAddr = cfg.Username
	}

	if emailAddr != "" {
		if fullName != "" {
			return fmt.Sprintf("%s <%s>", fullName, emailAddr)
		}
		return emailAddr
	}
	return ""
}

func (m *tuiModel) buildComposeRFC822() ([]byte, error) {
	built, err := m.composeBuiltMessage()
	if err != nil {
		return nil, err
	}
	recipients := m.composeRecipients()
	return pgp.Protect(context.Background(), built, pgp.OutboundOptions{
		Sign:             m.compose.Sign,
		Encrypt:          m.compose.Encrypt,
		KeyID:            m.composeSigningKey(),
		Recipients:       recipients.Visible,
		HiddenRecipients: recipients.Hidden,
	})
}

// buildDraftRFC822 is the message as it is kept between sessions.
//
// An encrypted session's draft is encrypted to the sender alone. A draft's
// recipients are not settled, and a draft left in the clear would be exactly
// the plaintext the session was turned on to avoid -- on disk and in the
// account's Drafts folder alike. Encrypting to the sender keeps its body and
// attachments resumable here and readable by no one else. Its routing headers
// -- From, To, Cc, Bcc and Subject -- stay in the clear, as on any PGP/MIME
// message, so the server still sees who the draft is to and what it is about.
// A copy uploaded before encryption was turned on is deleted outright, not
// trashed (retireServerDraft).
//
// A signed-only session's draft stays in the clear and unsigned: it is not a
// secret, and a signature over a draft that will be rebuilt when it is sent
// proves nothing -- while signing every autosave asks gpg-agent, and so
// possibly a smartcard or a pinentry, for each keystroke's save.
func (m *tuiModel) buildDraftRFC822() ([]byte, error) {
	built, err := m.composeBuiltMessage()
	if err != nil {
		return nil, err
	}
	if !m.compose.Encrypt {
		return built.Bytes(), nil
	}
	return pgp.Protect(context.Background(), built, pgp.OutboundOptions{
		Encrypt:    true,
		Recipients: []string{m.composeSenderAddress()},
	})
}

// composeBuiltMessage is the session's message split into routing headers and
// entity, before any protection is applied.
func (m *tuiModel) composeBuiltMessage() (email.BuiltMessage, error) {
	if m.compose == nil {
		return email.BuiltMessage{}, fmt.Errorf("no compose session in progress")
	}

	session := m.composeSession()
	comp := &email.ComposeMessage{
		MessageID:   session,
		From:        m.compose.From,
		To:          m.compose.To,
		Cc:          m.compose.Cc,
		Bcc:         m.compose.Bcc,
		Subject:     m.compose.Subject,
		InReplyTo:   m.compose.InReplyTo,
		References:  m.compose.References,
		Body:        m.compose.Body,
		Attachments: m.compose.Attachments,
	}

	return comp.Build()
}

// composeSigningKey is the session's key, then the account's, then gpg's.
func (m *tuiModel) composeSigningKey() string {
	if m.compose == nil {
		return ""
	}
	if m.compose.PgpKeyID != "" {
		return m.compose.PgpKeyID
	}
	accounts := m.getAccounts()
	if m.compose.AccountIndex >= 0 && m.compose.AccountIndex < len(accounts) {
		return accounts[m.compose.AccountIndex].PgpKeyID
	}
	return ""
}

// composeRecipients is everyone the message is addressed to, as bare addresses:
// a keyring is searched by address, not by the display name written beside it.
// Bcc is kept apart so its keys stay out of the ciphertext.
func (m *tuiModel) composeRecipients() pgp.Recipients {
	if m.compose == nil {
		return pgp.Recipients{}
	}
	return pgp.Recipients{
		Visible: recipientAddresses(m.compose.To, m.compose.Cc),
		Hidden:  recipientAddresses(m.compose.Bcc),
	}
}

func (m *tuiModel) composeSenderAddress() string {
	if m.compose == nil {
		return ""
	}
	if addr, err := mail.ParseAddress(m.compose.From); err == nil {
		return addr.Address
	}
	return strings.TrimSpace(m.compose.From)
}

// recipientAddresses splits comma-separated header fields into addresses,
// keeping anything that will not parse as written so a keyring lookup is still
// attempted rather than the recipient silently vanishing.
func recipientAddresses(fields ...string) []string {
	var out []string
	for _, field := range fields {
		for _, part := range strings.Split(field, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			if addr, err := mail.ParseAddress(part); err == nil {
				out = append(out, addr.Address)
				continue
			}
			out = append(out, part)
		}
	}
	return out
}
