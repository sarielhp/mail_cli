package tui

import (
	"fmt"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/label"
	"log/slog"
	"strings"
	"time"

	"github.com/sarielhp/mail_cli/cache"
	"github.com/sarielhp/mail_cli/cache/folderindex"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *tuiModel) kIndexOpenDetail(_ tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.eIdx >= 0 && m.eIdx < len(m.emails) {
		em := m.emails[m.eIdx]
		if em.ThreadHasReplies && !m.expandedThreads[threadKeyOf(&em)] {
			m.expandRecursively(em.ID)
			m.rebuildVisibleEmails()
			for i, e := range m.emails {
				if e.ID == m.selectedID {
					m.eIdx = i
					break
				}
			}
			return m, nil
		}
		m.mode = modeDetail
		m.selectedID = em.ID
		m.detailSplitOpen = false
		m.rightPaneFocus = false
		var cmds []tea.Cmd
		if !em.IsRead {
			m.emails[m.eIdx].IsRead = true
			m.isRead[em.ID] = true
			if cmd := m.markAsReadCmd(em.ID); cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
		h := 20
		if m.height > 0 {
			h = max(5, m.height-3)
		}
		if m.width > 0 {
			m.vp.Width = max(20, m.width-2)
			m.vp.Height = h
		}
		m.vp.SetContent("")
		m.detailH = ""
		m.detailCrypto = nil
		m.detail = "(Loading message...)"
		cmds = append(cmds, m.loadDetailCmd(m.selectedID))
		return m, tea.Batch(cmds...)
	}
	return m, nil
}

func (m *tuiModel) markAsReadCmd(msgID email.ID) tea.Cmd {
	if m.isReadOnly() || m.client == nil || m.client.Config() == nil {
		return nil
	}
	plan := m.planAction([]email.ID{msgID}, m.currentF)
	return func() tea.Msg {
		client, _, err := m.target(plan, msgID)
		if err != nil {
			slog.Error("markAsReadCmd: cannot resolve account", slog.String("msgID", string(msgID)), slog.Any("error", err))
			return nil
		}
		_ = cache.MarkIDsRead(client.Config().AccountCache, []email.ID{msgID}, nil)
		if err := client.MarkAsRead([]email.ID{msgID}); err != nil {
			slog.Error("markAsReadCmd: client.MarkAsRead failed", slog.String("msgID", string(msgID)), slog.Any("error", err))
		}
		return nil
	}
}

func (m *tuiModel) kIndexActions(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := k.String()
	switch {
	case s == "E":
		return m.archiveCurrentEmail()
	case s == "s":
		return m.promptSaveMessageToFolder()
	case s == "S":
		return m.spamCurrentEmail()
	case s == "B":
		return m.promptBlockSender()
	case s == "U" || s == "u":
		return m.unspamCurrentEmail()
	case s == "d":
		return m.deleteCurrentEmail()
	case s == "L":
		return m.toggleFolderLock()
	case s == "/":
		if m.fuzzySearch {
			m.fuzzySearch = false
			m.fuzzyGlobal = false
		}
		m.inSearch = true
		m.searchQuery = ""
		m.rebuildVisibleEmails()
		return m, nil
	case s == "~":
		m.loadAllCachedEmails()
		m.fuzzySearch = true
		m.fuzzyGlobal = true
		m.inSearch = true
		m.searchQuery = ""
		m.rebuildVisibleEmails()
		if len(m.emails) > 0 {
			m.eIdx = 0
			m.selectedID = m.emails[0].ID
		}
		return m, nil
	case s == "!":
		m.fuzzySearch = true
		m.fuzzyGlobal = false
		m.inSearch = true
		m.searchQuery = ""
		m.rebuildVisibleEmails()
		if len(m.emails) > 0 {
			m.eIdx = 0
			m.selectedID = m.emails[0].ID
		}
		return m, nil
	case s == "c":
		return m, m.composeCmd()
	case s == "r" || s == "g":
		if m.eIdx >= 0 && m.eIdx < len(m.emails) {
			targetID := m.emails[m.eIdx].ID
			groupReply := (s == "g")
			return m, m.replyCmd(targetID, groupReply)
		}
	case s == "R":
		m.showError = false
		m.err = nil
		m.showLoad = "Downloading..."
		return m, m.loadEmailsCmd(m.currentF)
	case s == "m" || s == "alt" || s == "alt+m" || k.Alt:
		m.menuOpen = true
		m.menuCursor = 0
		return m, nil
	case s == "a" || s == "v":
		return m.openAttachmentList()
	case s == "f3" || s == "F3" || k.Type == tea.KeyF3:
		m.openAccountOverlay()
		return m, nil
	case s == "f4" || s == "F4" || k.Type == tea.KeyF4:
		return m.summarizeCurrentEmail()
	case s == "f5" || s == "F5" || k.Type == tea.KeyF5:
		return m.openSpamMenu()
	case s == "D":
		return m.openDeadlinesAgenda()
	case s == "|":
		m.inPipePrompt = true
		m.pipeCommand = ""
		return m, nil
	case s == "t":
		return m.toggleTagAndAdvance()
	case s == "T":
		m.inTagPatternPrompt = true
		m.tagPatternQuery = ""
		return m, nil
	case s == "ctrl+a":
		return m.tagAllVisible()
	case s == "ctrl+u":
		return m.clearTags()
	case s == ";":
		m.inTagPrefix = true
		return m, nil
	case s == "ctrl+f":
		m.inRemoteSearch = true
		m.remoteSearchQuery = ""
		return m, nil
	default:
		return m, nil
	}
	return m, nil
}

func (m *tuiModel) currentEmailID() email.ID {
	if m.mode == modeIndex && m.eIdx >= 0 && m.eIdx < len(m.emails) {
		return m.emails[m.eIdx].ID
	}
	return m.selectedID
}

// beginEmailAction resolves the current email and refuses to act in read-only
// mode, reporting the simulated refusal. ok is false when there is nothing to
// act on or the session forbids it.
func (m *tuiModel) beginEmailAction() (email.ID, bool) {
	targetID := m.currentEmailID()
	if targetID == "" {
		return "", false
	}
	if m.isReadOnly() {
		m.err = fmt.Errorf("🔒 Read-Only Mode: Action simulated (no server changes)")
		m.showError = true
		return "", false
	}
	return targetID, true
}

// stageEmailRemoval ghosts a message for an action that moves it out of the
// folder: it expands the thread, drops the row and marks it pending.
func (m *tuiModel) stageEmailRemoval(targetID email.ID, pending pendingOpType) {
	m.expandWholeThreadForEmail(targetID)
	m.removeEmailByID(targetID)
	m.addPending(targetID, pending)
	m.mode = modeIndex
}

// removeCurrentEmail carries out an action that moves the current email out of
// the folder: it resolves the target, refuses in read-only mode, ghosts the row
// and returns the command that performs the move.
func (m *tuiModel) removeCurrentEmail(pending pendingOpType, cmd func(email.ID, actionPlan) tea.Cmd) (tea.Model, tea.Cmd) {
	if m.refuseInSearchView() {
		return m, nil
	}
	targetID, ok := m.beginEmailAction()
	if !ok {
		return m, nil
	}
	// The plan is taken before the row goes: afterwards nothing says which
	// account a unified-inbox message belongs to.
	plan := m.planAction([]email.ID{targetID}, m.currentF)
	m.stageEmailRemoval(targetID, pending)
	return m, cmd(targetID, plan)
}

func (m *tuiModel) archiveCurrentEmail() (tea.Model, tea.Cmd) {
	// m.currentF is captured here, on the Update goroutine, not inside the command.
	return m.removeCurrentEmail(pendingArchive, m.cmdArchive)
}

func (m *tuiModel) spamCurrentEmail() (tea.Model, tea.Cmd) {
	targetID, ok := m.beginEmailAction()
	if !ok {
		return m, nil
	}
	m.lastSpammedID = targetID
	m.lastSpammedTime = time.Now()
	m.undoSpamBanner = "Marked as spam. Undo spam marking by pressing 'U' (3s)"

	plan := m.planAction([]email.ID{targetID}, m.currentF)
	m.stageEmailRemoval(targetID, pendingSpam)

	var cmds []tea.Cmd
	cmds = append(cmds, m.cmdSpam(targetID, plan))
	cmds = append(cmds, tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
		return clearSpamUndoMsg{time: t}
	}))
	return m, tea.Batch(cmds...)
}

func (m *tuiModel) unspamCurrentEmail() (tea.Model, tea.Cmd) {
	if m.undoSpamBanner != "" && time.Since(m.lastSpammedTime) < 3*time.Second && m.lastSpammedID != "" {
		targetID := m.lastSpammedID
		m.undoSpamBanner = ""
		m.lastSpammedID = ""
		return m.unspamEmailByID(targetID)
	}
	targetID := m.currentEmailID()
	return m.unspamEmailByID(targetID)
}

func (m *tuiModel) unspamEmailByID(targetID email.ID) (tea.Model, tea.Cmd) {
	if targetID == "" {
		return m, nil
	}
	if m.isReadOnly() {
		m.err = fmt.Errorf("🔒 Read-Only Mode: Action simulated (no server changes)")
		m.showError = true
		return m, nil
	}
	if !strings.EqualFold(m.currentF, "inbox") {
		m.expandWholeThreadForEmail(targetID)
		m.removeEmailByID(targetID)
	}
	m.addPending(targetID, pendingUnspam)
	m.mode = modeIndex
	m.showLoad = "Unspamming..."
	return m, m.cmdUnspam(targetID)
}

func (m *tuiModel) deleteCurrentEmail() (tea.Model, tea.Cmd) {
	return m.removeCurrentEmail(pendingDelete, m.cmdDelete)
}

func (m *tuiModel) kDetail(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.rightPaneFocus {
		return m.kRightPane(k)
	}
	if m.attachmentDrawer {
		return m.kAttachmentDrawer(k)
	}
	if m.inDetailSearch {
		return m.kDetailSearch(k)
	}
	s := k.String()
	switch {
	case s == "tab" || s == "l":
		if m.resolveViewableAttachment() != nil && m.width >= 100 {
			m.rightPaneFocus = true
			return m, nil
		}
	case s == "p" || s == "P":
		if m.resolveViewableAttachment() != nil || m.hideRightPane {
			m.hideRightPane = !m.hideRightPane
			m.rightPaneFocus = false
			m.detailVpDirty = true
			return m, nil
		}
	case k.Type == tea.KeyUp || s == "k":
		m.vp.ScrollUp(1)
		m.updateAttachmentFocusOnScroll()
	case k.Type == tea.KeyDown || s == "j":
		m.vp.ScrollDown(1)
		m.updateAttachmentFocusOnScroll()
	case s == "pageup" || s == "pgup":
		m.vp.ScrollUp(m.vp.Height)
		m.updateAttachmentFocusOnScroll()
	case s == "pagedown" || s == "pgdown" || s == "pgdn":
		m.vp.ScrollDown(m.vp.Height)
		m.updateAttachmentFocusOnScroll()
	case s == "home":
		m.vp.GotoTop()
		m.updateAttachmentFocusOnScroll()
	case s == "end":
		m.vp.GotoBottom()
		m.updateAttachmentFocusOnScroll()
	case k.Type == tea.KeyLeft || s == "left":
		return m.navigateDetailEmail(-1)
	case k.Type == tea.KeyRight || s == "right":
		return m.navigateDetailEmail(1)
	case s == "E":
		return m.archiveCurrentEmail()
	case s == "s":
		return m.promptSaveMessageToFolder()
	case s == "S":
		return m.spamCurrentEmail()
	case s == "B":
		return m.promptBlockSender()
	case s == "U" || s == "u":
		return m.unspamCurrentEmail()
	case s == "d":
		return m.deleteCurrentEmail()
	case s == "L":
		return m.toggleFolderLock()
	case s == "c":
		return m, m.composeCmd()
	case s == "r" || s == "g":
		targetID := m.selectedID
		groupReply := (s == "g")
		return m, m.replyCmd(targetID, groupReply)
	case s == "i":
		m.toggleInvertColors()
		return m, nil
	case s == "a" || s == "v":
		return m.openAttachmentList()
	case s == "f4" || s == "F4" || k.Type == tea.KeyF4:
		return m.summarizeCurrentEmail()
	case s == "f5" || s == "F5" || k.Type == tea.KeyF5:
		return m.openSpamMenu()
	case s == "D":
		return m.openDeadlinesAgenda()
	case s == "m" || s == "alt" || s == "alt+m" || k.Alt:
		m.menuOpen = true
		m.menuCursor = 0
		return m, nil
	case s == "/":
		m.inDetailSearch = true
		m.detailSearch = ""
		return m, nil
	case s == "|":
		m.inPipePrompt = true
		m.pipeCommand = ""
		return m, nil
	case s == "ctrl+f":
		m.inRemoteSearch = true
		m.remoteSearchQuery = ""
		return m, nil
	default:
		return m, nil
	}
	return m, nil
}

func (m *tuiModel) navigateDetailEmail(delta int) (tea.Model, tea.Cmd) {
	if len(m.emails) == 0 {
		return m, nil
	}
	m.eIdx += delta
	if m.eIdx < 0 {
		m.eIdx = 0
	}
	if m.eIdx >= len(m.emails) {
		m.eIdx = len(m.emails) - 1
	}
	m.selectedID = m.emails[m.eIdx].ID
	var cmds []tea.Cmd
	if !m.emails[m.eIdx].IsRead {
		m.emails[m.eIdx].IsRead = true
		m.isRead[m.selectedID] = true
		if cmd := m.markAsReadCmd(m.selectedID); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	m.vp.GotoTop()
	m.vp.SetContent("")
	m.detailH = ""
	m.detailCrypto = nil
	m.detail = "(Loading message...)"
	cmds = append(cmds, m.loadDetailCmd(m.selectedID))
	return m, tea.Batch(cmds...)
}

func (m *tuiModel) updateAttachmentFocusOnScroll() {
	if len(m.attachments) == 0 || m.attachmentLineOffset <= 0 {
		return
	}
	middleLine := m.vp.YOffset + (m.vp.Height / 2)
	attStart := m.attachmentLineOffset
	attEnd := m.attachmentLineOffset + len(m.attachments)
	if middleLine >= attStart && middleLine < attEnd {
		idx := middleLine - attStart
		m.detailSplitOpen = true
		if !m.attachmentDrawer || m.attCursor != idx {
			m.attachmentDrawer = true
			m.attCursor = idx
			m.detailVpDirty = true
		}
	} else if m.attachmentDrawer {
		m.attachmentDrawer = false
		m.detailVpDirty = true
	}
}

func (m *tuiModel) toggleFolderLock() (tea.Model, tea.Cmd) {
	if m.lockedFolders == nil {
		m.lockedFolders = make(map[string]bool)
	}
	key := strings.ToLower(m.currentF)
	m.lockedFolders[key] = !m.lockedFolders[key]
	if m.lockedFolders[key] {
		m.err = fmt.Errorf("🔒 Folder %q locked as read-only", m.currentF)
		m.showError = true
		return m, nil
	}
	m.err = fmt.Errorf("🔓 Folder %q unlocked", m.currentF)
	m.showError = true
	return m, nil
}

type saveEmailDoneMsg struct {
	id     email.ID
	folder string
	err    error
}

func (m *tuiModel) promptSaveMessageToFolder() (tea.Model, tea.Cmd) {
	targetID := m.currentEmailID()
	if targetID == "" {
		return m, nil
	}
	m.expandWholeThreadForEmail(targetID)
	m.rebuildVisibleEmails()
	m.savingMessageToFolder = true
	m.savingMessageID = targetID
	m.savingMessageSourceFolder = m.currentF
	m.treeOpen = true
	m.showGlobalTree = true
	m.buildEntries()
	return m, nil
}

func (m *tuiModel) isMessageInFolder(targetID email.ID, folder string) bool {
	cfg := m.Config()
	if cfg == nil {
		cfg = m.cfg
	}
	if cfg == nil || cfg.AccountCache.IsZero() {
		return false
	}
	ids, err := folderindex.IDs(cfg.AccountCache, folder)
	if err != nil {
		return false
	}
	for _, id := range ids {
		if id == targetID {
			return true
		}
	}
	return false
}

func (m *tuiModel) saveEmailToFolder(targetID email.ID, srcFolder, destFolder string) (tea.Model, tea.Cmd) {
	if targetID == "" || destFolder == "" {
		return m, nil
	}
	if m.isReadOnly() {
		m.err = fmt.Errorf("🔒 Read-Only Mode: Action simulated (no server changes)")
		m.showError = true
		return m, nil
	}
	if m.client == nil {
		m.err = fmt.Errorf("mail client not configured")
		m.showError = true
		return m, nil
	}
	if m.isMessageInFolder(targetID, destFolder) {
		m.saveStatus = fmt.Sprintf("Message already in %s", destFolder)
		m.saveStatusTime = time.Now()
		return m, nil
	}

	m.saveStatus = fmt.Sprintf("Saving to %s...", destFolder)
	m.saveStatusTime = time.Now()

	client := m.client
	return m, func() tea.Msg {
		err := client.CopyEmail([]email.ID{targetID}, label.FromServer(srcFolder), label.FromServer(destFolder))
		return saveEmailDoneMsg{id: targetID, folder: destFolder, err: err}
	}
}
