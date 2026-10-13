package tui

import (
	"errors"
	"strings"

	"github.com/sarielhp/mail_cli/mailclient"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *tuiModel) handleComposeModeKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.fileBrowserOpen {
		return m.kFileBrowser(k)
	}
	if m.attModalOpen {
		return m.kAttachmentModal(k)
	}
	if m.accountOverlayOpen {
		return m.kAccountOverlay(k)
	}
	if m.contactPick != nil {
		return m.kContactPicker(k)
	}
	return m.handleComposeKey(k)
}

// handleKeyWhileExiting runs while the program waits for a draft upload
// before exiting. Every key is swallowed: a second q or Esc used to quit at
// once and drop the upload. Two Ctrl+C in a row abandon the wait.
func (m *tuiModel) handleKeyWhileExiting(s string) (tea.Model, tea.Cmd) {
	if s != "ctrl+c" {
		m.abandonArmed = false
		return m, nil
	}
	if !m.abandonArmed {
		m.abandonArmed = true
		m.showLoad = "Uploading draft... (Ctrl+C again to abandon)"
		return m, nil
	}
	m.saveStatus = joinStatus(m.saveStatus, "server upload abandoned")
	return m, m.quitNow()
}

// ctrlCInCompose leaves a compose session keeping the draft: the field being
// typed is committed, overlays close, and the draft is saved and uploaded
// before the program exits. During a send it waits for the send instead: a
// draft saved then would be a copy of a message that went out.
func (m *tuiModel) ctrlCInCompose() (tea.Model, tea.Cmd) {
	if m.compose.sending {
		m.compose.quitAfterSend = true
		m.showLoad = "Sending email... (exits when sent)"
		return m, nil
	}
	m.contactPick = nil
	m.commitComposeField()
	m.fileBrowserOpen = false
	m.attModalOpen = false
	m.accountOverlayOpen = false
	m.compose.DiscardConfirm = false
	m.compose.WarnEmptySend = false
	return m.saveDraftAndLeave(true)
}

func (m *tuiModel) handleModalOverlayKey(k tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	switch {
	case m.menuOpen:
		model, cmd := m.kMenu(k)
		return model, cmd, true
	case m.spamMenuOpen:
		model, cmd := m.kSpamMenu(k)
		return model, cmd, true
	case m.showDiag:
		model, cmd := m.kDiag(k)
		return model, cmd, true
	case m.showSummary:
		model, cmd := m.kSummary(k)
		return model, cmd, true
	case m.showDeadlinesAgenda:
		model, cmd := m.kDeadlinesAgenda(k)
		return model, cmd, true
	case m.blockModalOpen:
		model, cmd := m.kBlockModal(k)
		return model, cmd, true
	case m.accountOverlayOpen:
		model, cmd := m.kAccountOverlay(k)
		return model, cmd, true
	default:
		return m, nil, false
	}
}

func (m *tuiModel) handleKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.inPipePrompt {
		return m.kPipePrompt(k)
	}
	if m.inTagPatternPrompt {
		return m.kTagPattern(k)
	}
	if m.inTagPrefix {
		return m.kTagPrefix(k)
	}
	if m.inRemoteSearch {
		return m.kRemoteSearchPrompt(k)
	}
	s := k.String()
	if m.exitPending {
		return m.handleKeyWhileExiting(s)
	}
	if s == "ctrl+c" {
		if m.mode == modeCompose && m.compose != nil {
			return m.ctrlCInCompose()
		}
		m.done = true
		return m, tea.Quit
	}
	if s == mouseToggleKey {
		return m.toggleMouse()
	}
	if m.showHelp {
		if s == "f1" || s == "esc" || s == "q" || s == "?" || s == "h" || k.Type == tea.KeyEsc {
			m.showHelp = false
			return m, nil
		}
		return m, nil
	}
	if model, cmd, handled := m.handleAuthErrorLogin(s); handled {
		return model, cmd
	}
	if m.mode == modeCompose {
		return m.handleComposeModeKey(k)
	}
	if model, cmd, ok := m.handleAttachmentOverlayKey(k, s); ok {
		return model, cmd
	}
	if s == "f1" {
		m.showHelp = !m.showHelp
		m.showDiag = false
		m.accountOverlayOpen = false
		m.treeOpen = false
		m.menuOpen = false
		m.spamMenuOpen = false
		m.showError = false
		return m, nil
	}
	if model, cmd, handled := m.handleModalOverlayKey(k); handled {
		return model, cmd
	}
	if s == "ctrl+d" {
		m.showDiag = true
		m.showHelp = false
		m.treeOpen = false
		m.accountOverlayOpen = false
		m.diagVp.GotoBottom()
		return m, nil
	}
	if m.inSearch {
		return m.kSearch(k)
	}
	if m.treeOpen && m.inFolderSearch {
		return m.kFolder(k)
	}
	if m.mode == modeDetail && m.rightPaneFocus {
		return m.kRightPane(k)
	}
	return m.kGlobalKey(k)
}

func (m *tuiModel) handleAttachmentOverlayKey(k tea.KeyMsg, s string) (tea.Model, tea.Cmd, bool) {
	if m.fileBrowserOpen {
		model, cmd := m.kFileBrowser(k)
		return model, cmd, true
	}
	if m.attachmentListOpen {
		model, cmd := m.handleAttachmentListKey(k)
		return model, cmd, true
	}
	if m.icsModalOpen {
		model, cmd := m.handleICSModalKey(k)
		return model, cmd, true
	}
	if m.attModalOpen {
		model, cmd := m.kAttachmentModal(k)
		return model, cmd, true
	}
	if m.attachmentHelpOpen {
		if s == "f1" || s == "esc" || s == "q" || k.Type == tea.KeyEsc {
			m.attachmentHelpOpen = false
		}
		return m, nil, true
	}
	if m.attachmentDrawer {
		model, cmd := m.kAttachmentDrawer(k)
		return model, cmd, true
	}
	return nil, nil, false
}

func (m *tuiModel) key(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	return m.handleKey(k)
}

func (m *tuiModel) kDiag(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := k.String()
	switch s {
	case "ctrl+d", "esc", "q":
		m.showDiag = false
	case "up", "k":
		m.diagVp.ScrollUp(1)
	case "down", "j":
		m.diagVp.ScrollDown(1)
	case "pageup", "pgup":
		m.diagVp.ScrollUp(m.diagVp.Height)
	case "pagedown", "pgdn", "pgdown":
		m.diagVp.ScrollDown(m.diagVp.Height)
	case "home":
		m.diagVp.GotoTop()
	case "end":
		m.diagVp.GotoBottom()
	}
	return m, nil
}

func (m *tuiModel) handleGlobalQuit() (tea.Model, tea.Cmd) {
	if m.treeOpen {
		m.treeOpen = false
		return m, nil
	}
	if m.mode == modeDetail {
		m.mode = modeIndex
		return m, nil
	}
	if m.mode == modeIndex && m.splitPreview {
		m.splitPreview = false
		return m, nil
	}
	m.done = true
	return m, tea.Quit
}

func (m *tuiModel) handleDetailEsc() (tea.Model, tea.Cmd) {
	if m.rightPaneFocus {
		m.rightPaneFocus = false
		return m, nil
	}
	if m.detailSplitOpen {
		m.detailSplitOpen = false
		m.attachmentDrawer = false
		m.detailVpDirty = true
		return m, nil
	}
	if m.attachmentDrawer {
		m.attachmentDrawer = false
		m.detailVpDirty = true
		return m, nil
	}
	m.mode = modeIndex
	return m, nil
}

func (m *tuiModel) handleIndexEsc() (tea.Model, tea.Cmd) {
	if m.searchQuery != "" {
		m.searchQuery = ""
		m.rebuildVisibleEmails()
		if len(m.emails) > 0 {
			m.eIdx = min(m.eIdx, len(m.emails)-1)
			m.selectedID = m.emails[m.eIdx].ID
		}
		return m, nil
	}
	if m.splitPreview {
		m.splitPreview = false
		return m, nil
	}
	m.done = true
	return m, tea.Quit
}

func (m *tuiModel) handleGlobalEsc() (tea.Model, tea.Cmd) {
	if m.attachmentListOpen {
		if len(m.attachmentDeleted) > 0 {
			m.applyAttachmentDeletions()
		}
		m.attachmentListOpen = false
		m.attachmentViewerOpen = false
		return m, nil
	}
	if m.showSummary {
		m.showSummary = false
		return m, nil
	}
	if m.showError {
		m.showError = false
		m.err = nil
		return m, nil
	}
	if m.showHelp {
		m.showHelp = false
		return m, nil
	}
	if m.blockModalOpen {
		m.blockModalOpen = false
		return m, nil
	}
	if m.accountOverlayOpen {
		m.accountOverlayOpen = false
		return m, nil
	}
	if m.treeOpen {
		m.treeOpen = false
		m.savingMessageToFolder = false
		m.bulkMovingToFolder = false
		m.savingMessageID = ""
		m.savingMessageSourceFolder = ""
		return m, nil
	}
	if m.mode == modeDetail {
		return m.handleDetailEsc()
	}
	if m.mode == modeIndex {
		return m.handleIndexEsc()
	}
	return m, nil
}

func (m *tuiModel) handleGlobalTab(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.treeOpen {
		return m.kFolder(k)
	}
	if m.mode == modeDetail {
		if len(m.attachments) > 0 && m.width >= 100 {
			m.rightPaneFocus = !m.rightPaneFocus
			return m, nil
		}
		m.mode = modeIndex
		return m, nil
	}
	if m.mode == modeIndex {
		if m.eIdx >= 0 && m.eIdx < len(m.emails) {
			em := m.emails[m.eIdx]
			if em.ThreadHasReplies || em.ThreadDepth > 0 {
				m.cycleEmailThreadSubtreeExpansion(em.ID)
				return m, nil
			}
		}
		if m.searchQuery != "" && !m.inSearch {
			m.inSearch = true
			return m, nil
		}
	}
	return m, nil
}

func (m *tuiModel) kGlobalKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := k.String()
	switch s {
	case "q":
		if m.standaloneCompose {
			return m, m.quitNow()
		}
		return m.handleGlobalQuit()
	case "f3", "F3":
		m.openAccountOverlay()
		return m, nil
	case "f4", "F4":
		return m.summarizeCurrentEmail()
	case "f2", "F2":
		return m.saveCurrentMessage()
	case "f", "F":
		if m.currentF != "" {
			if m.treeOpen {
				m.treeOpen = false
			} else {
				m.treeOpen = true
				m.showGlobalTree = true
				m.buildEntries()
			}
		}
		return m, nil
	case "h", "?":
		m.showHelp = !m.showHelp
		return m, nil
	case "tab":
		return m.handleGlobalTab(k)
	case "esc":
		return m.handleGlobalEsc()
	default:
		if m.showHelp {
			return m, nil
		}
		if m.treeOpen {
			return m.kFolder(k)
		}
		switch m.mode {
		case modeIndex:
			return m.kIndex(k)
		case modeDetail:
			return m.kDetail(k)
		case modeCompose:
			return m.kCompose(k)
		}
		return m, nil
	}
}

func (m *tuiModel) pageStep() int {
	if m.height > 2 {
		return m.height - 2
	}
	return 10
}

func (m *tuiModel) moveIndexSelection(delta int) {
	if len(m.emails) == 0 {
		return
	}
	m.eIdx = max(0, min(len(m.emails)-1, m.eIdx+delta))
	m.selectedID = m.emails[m.eIdx].ID
}

func (m *tuiModel) openPreviewCurrentEmail() tea.Cmd {
	if m.eIdx < 0 || m.eIdx >= len(m.emails) {
		return nil
	}
	m.selectedID = m.emails[m.eIdx].ID
	m.detailVpDirty = true
	m.vp.GotoTop()
	var cmds []tea.Cmd
	if !m.emails[m.eIdx].IsRead {
		m.emails[m.eIdx].IsRead = true
		m.isRead[m.emails[m.eIdx].ID] = true
		if cmd := m.markAsReadCmd(m.emails[m.eIdx].ID); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	m.detailH = ""
	m.detailCrypto = nil
	m.detail = "(Loading preview...)"
	cmds = append(cmds, m.loadDetailCmd(m.selectedID))
	return tea.Batch(cmds...)
}

func (m *tuiModel) navigateIndexPreview(delta int) (tea.Model, tea.Cmd) {
	next := m.eIdx + delta
	if next < 0 || next >= len(m.emails) {
		return m, nil
	}
	m.eIdx = next
	m.adjustScrollOffset()
	cmd := m.openPreviewCurrentEmail()
	return m, cmd
}

func (m *tuiModel) toggleSplitPreview() (tea.Model, tea.Cmd) {
	if len(m.emails) == 0 {
		return m, nil
	}
	m.splitPreview = !m.splitPreview
	if !m.splitPreview {
		return m, nil
	}
	cmd := m.openPreviewCurrentEmail()
	return m, cmd
}

func (m *tuiModel) kIndex(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.folderJustChanged = false
	s := k.String()
	switch {
	case m.splitPreview && (k.Type == tea.KeyUp || s == "up"):
		m.vp.ScrollUp(1)
	case m.splitPreview && (k.Type == tea.KeyDown || s == "down"):
		m.vp.ScrollDown(1)
	case m.splitPreview && (s == "pageup" || s == "pgup"):
		m.vp.ScrollUp(m.vp.Height)
	case m.splitPreview && (s == "pagedown" || s == "pgdown" || s == "pgdn"):
		m.vp.ScrollDown(m.vp.Height)
	case m.splitPreview && (k.Type == tea.KeyLeft || s == "left" || s == "k"):
		return m.navigateIndexPreview(-1)
	case m.splitPreview && (k.Type == tea.KeyRight || s == "right" || s == "j"):
		return m.navigateIndexPreview(1)
	case !m.splitPreview && (k.Type == tea.KeyUp || s == "k" || s == "up"):
		m.moveIndexSelection(-1)
	case !m.splitPreview && (k.Type == tea.KeyDown || s == "j" || s == "down"):
		m.moveIndexSelection(1)
	case !m.splitPreview && (s == "pageup" || s == "pgup"):
		m.moveIndexSelection(-m.pageStep())
	case !m.splitPreview && (s == "pagedown" || s == "pgdown" || s == "pgdn"):
		m.moveIndexSelection(m.pageStep())
	case s == "home":
		m.eIdx = 0
		if len(m.emails) > 0 {
			m.selectedID = m.emails[0].ID
		}
		if m.splitPreview {
			m.vp.GotoTop()
		}
	case s == "end":
		if len(m.emails) > 0 {
			m.eIdx = len(m.emails) - 1
			m.selectedID = m.emails[m.eIdx].ID
		}
		if m.splitPreview {
			m.vp.GotoBottom()
		}
	case k.Type == tea.KeyEnter || s == "e":
		return m.kIndexOpenDetail(k)
	case s == " ":
		return m.toggleSplitPreview()
	default:
		return m.kIndexActions(k)
	}
	return m, nil
}

// isAuthError reports whether err means the account must sign in again,
// which 'L' on the error banner does without leaving the TUI.
func isAuthError(err error) bool {
	return errors.Is(err, mailclient.ErrInteractiveAuthRequired) ||
		(err != nil && strings.Contains(err.Error(), "needs re-authentication"))
}

func (m *tuiModel) handleAuthErrorLogin(s string) (tea.Model, tea.Cmd, bool) {
	if !m.showError || (s != "l" && s != "L") {
		return m, nil, false
	}
	if !isAuthError(m.err) {
		return m, nil, false
	}
	m.showError = false
	m.err = nil
	accounts := m.getAccounts()
	idx := m.currentAccountIndex()
	if idx >= 0 && idx < len(accounts) {
		return m, m.loginAccountCmd(accounts[idx]), true
	}
	return m, nil, true
}
