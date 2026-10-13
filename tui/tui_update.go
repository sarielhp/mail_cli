package tui

import (
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/sarielhp/mail_cli/cache"
	"github.com/sarielhp/mail_cli/calendar"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/label"
	"github.com/sarielhp/mail_cli/llm"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *tuiModel) adjustScrollOffset() {
	availH := m.height - 2
	if m.showError {
		availH--
	}
	numLines := availH
	if m.splitPreview && len(m.emails) > 0 {
		numLines = max(3, (availH-1)/2)
	}
	if numLines <= 0 {
		numLines = 1
	}
	oldStart := m.indexStart
	m.indexStart = max(0, min(m.indexStart, len(m.emails)-numLines))
	if m.eIdx < m.indexStart {
		m.indexStart = m.eIdx
	} else if m.eIdx >= m.indexStart+numLines {
		m.indexStart = m.eIdx - numLines + 1
	}
	var emailDetails []string
	for _, em := range m.emails {
		emailDetails = append(emailDetails, fmt.Sprintf("%s(read=%t):%q", em.ID, em.IsRead, em.Subject))
	}
	slog.Info("adjustScrollOffset completed",
		slog.Int("height", m.height),
		slog.Int("numLines", numLines),
		slog.Int("emailsCount", len(m.emails)),
		slog.Any("emails", emailDetails),
		slog.Int("eIdx", m.eIdx),
		slog.Int("oldStart", oldStart),
		slog.Int("newStart", m.indexStart),
		slog.Int("mode", int(m.mode)),
	)
}

func (m *tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	prevMode := m.mode
	model, cmd := m.updateInner(msg)
	if tm, ok := model.(*tuiModel); ok {
		tm.adjustScrollOffset()
		if prevMode == modeDetail && tm.mode == modeIndex {
			if cmd == nil {
				cmd = tea.ClearScreen
			} else {
				cmd = tea.Batch(cmd, tea.ClearScreen)
			}
		}
		cmd = tm.retakeMouse(msg, cmd)
	}
	return model, cmd
}

func (m *tuiModel) handleWindowSize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.width = msg.Width
	m.height = msg.Height
	if m.mode == modeDetail {
		m.vp.Width = max(20, m.width-2)
		m.vp.Height = max(5, m.height-3)
	} else {
		m.vp.Width = max(20, m.width-2)
		m.vp.Height = max(5, m.height-2)
	}
	if m.compose != nil {
		m.compose.Vp.Width = max(20, m.width-6)
		m.compose.Vp.Height = max(5, m.height-14)
	}
	return m, nil
}

func (m *tuiModel) handleFoldersLoaded(msg foldersLoadedMsg) (tea.Model, tea.Cmd) {
	folders := msg.folders
	// The tree the folder pane shows is the global one, so the unified inbox
	// belongs there too; offered only in m.folders, it could not be opened.
	gFolders := append(m.unifiedInboxItems(), folders...)
	sort.Slice(gFolders, func(i, j int) bool {
		return strings.ToLower(gFolders[i].FullName) < strings.ToLower(gFolders[j].FullName)
	})
	m.globalFolders = gFolders
	newGlobalTree := label.BuildLabelTree(gFolders)
	transferExpandedStates(m.globalTree, newGlobalTree)
	m.globalTree = newGlobalTree
	if m.lpr != "" {
		lower := strings.ToLower(m.lpr)
		var filtered []label.Item
		for _, f := range folders {
			if strings.Contains(strings.ToLower(f.FullName), lower) {
				filtered = append(filtered, f)
			}
		}
		folders = filtered
		m.lpr = ""
	}
	folders = append(m.unifiedInboxItems(), folders...)
	sort.Slice(folders, func(i, j int) bool {
		return strings.ToLower(folders[i].FullName) < strings.ToLower(folders[j].FullName)
	})
	m.folders = folders
	newTree := label.BuildLabelTree(folders)
	transferExpandedStates(m.tree, newTree)
	m.tree = newTree
	m.buildEntries()
	targetFolder := "INBOX"
	if m.client != nil {
		targetFolder = m.client.InboxFolder().String()
	}
	resolved := label.ResolveFolder(folders, m.currentF, targetFolder)
	if isUnifiedInbox(m.currentF) {
		resolved = UnifiedInboxFolder
	}
	slog.Info("Update foldersLoadedMsg alignment", slog.String("oldCurrentF", m.currentF), slog.String("resolved", resolved), slog.String("targetFolder", targetFolder))
	if resolved != m.currentF {
		m.currentF = resolved
		m.selectedID = ""
		m.loadCachedEmailsForFolder(m.currentF)
		if len(m.emails) == 0 {
			m.showLoad = "Downloading..."
		} else {
			m.showLoad = ""
		}
		return m, m.loadEmailsCmd(m.currentF)
	}
	return m, nil
}

func (m *tuiModel) updateSelectionAfterEmailsLoaded() {
	if len(m.emails) == 0 {
		m.folderJustChanged = false
		m.eIdx = 0
		m.selectedID = ""
		m.indexStart = 0
		return
	}
	found := false
	if !m.folderJustChanged && m.selectedID != "" {
		for i, e := range m.emails {
			if e.ID == m.selectedID {
				m.eIdx = i
				found = true
				break
			}
		}
	}
	m.folderJustChanged = false
	if !found {
		m.eIdx = len(m.emails) - 1
		m.selectedID = m.emails[m.eIdx].ID
		m.adjustScrollOffset()
		return
	}
	m.indexStart = max(0, min(m.indexStart, len(m.emails)-1))
}

func (m *tuiModel) handleEmailsLoaded(msg emailsLoadedMsg) (tea.Model, tea.Cmd) {
	slog.Info("emailsLoadedMsg replacing rawEmails", slog.Int("incomingEmails", len(msg.emails)), slog.Int("prevRawEmails", len(m.rawEmails)))
	if msg.isRead != nil {
		m.isRead = msg.isRead
	}
	if msg.isReplied != nil {
		m.isReplied = msg.isReplied
	}
	var filtered []email.Email
	for _, e := range msg.emails {
		if _, pending := m.pendingEmails[e.ID]; !pending {
			filtered = append(filtered, e)
		}
	}
	m.rawEmails = filtered
	m.rebuildVisibleEmails()
	m.updateSelectionAfterEmailsLoaded()
	m.showLoad = ""
	slog.Info("emailsLoadedMsg DONE", slog.String("currentF", m.currentF), slog.Int("visibleEmails", len(m.emails)))

	var cmds []tea.Cmd
	if len(msg.classifyIDs) > 0 {
		cmds = append(cmds, m.bgClassifyCmd(msg.classifyIDs))
	}
	if bgCmd := m.startBackgroundSummaryQueue(); bgCmd != nil {
		cmds = append(cmds, bgCmd)
	}
	if len(cmds) > 0 {
		return m, tea.Batch(cmds...)
	}
	return m, nil
}

func (m *tuiModel) handleDetailLoaded(msg detailMsg) (tea.Model, tea.Cmd) {
	m.detailH = msg.headers
	m.detail = msg.body
	m.detailCrypto = msg.crypto
	m.attachments = msg.attachments
	m.attachmentDrawer = false
	m.detailSplitOpen = false
	m.rightPaneFocus = false
	m.attCursor = 0
	m.saveStatus = ""
	m.detailVpDirty = true
	m.icsModalOpen = false
	m.icsInCalendar = false
	m.icsCheckStatus = icsStatusChecking
	m.icsEvents = nil
	m.icsEvent = nil
	m.hideRightPane = false

	var cmds []tea.Cmd
	for _, a := range msg.attachments {
		if !email.IsICSAttachment(a) {
			continue
		}
		if events, err := calendar.ParseAllICSEventsData(a.Data); err == nil && len(events) > 0 {
			m.icsEvents = events
			m.icsEvent = events[0]
		}
		cmds = append(cmds, m.cmdCheckICSCalendar(a.Data))
		break
	}
	if len(cmds) > 0 {
		return m, tea.Batch(cmds...)
	}
	return m, nil
}

func (m *tuiModel) removeEmailFromList(id email.ID) {
	for i, e := range m.emails {
		if e.ID == id {
			m.emails = append(m.emails[:i], m.emails[i+1:]...)
			break
		}
	}
	m.removePending(id)
	if len(m.emails) > 0 {
		m.eIdx = min(m.eIdx, len(m.emails)-1)
		m.selectedID = m.emails[m.eIdx].ID
	} else {
		m.selectedID = ""
	}
}

func (m *tuiModel) handleDoneMsg(msg doneMsg) (tea.Model, tea.Cmd) {
	m.showLoad = ""
	m.removeEmailFromList(msg.id)
	return m, m.loadEmailsCmd(m.currentF)
}

func (m *tuiModel) handleSpamDone(msg spamDoneMsg) (tea.Model, tea.Cmd) {
	m.showLoad = ""
	m.removeEmailFromList(msg.id)
	return m, nil
}

// handleMarkedSpamDone drops the moved messages from the index: the server
// accepted each move, so they no longer belong in this folder.
func (m *tuiModel) handleMarkedSpamDone(msg markedSpamDoneMsg) (tea.Model, tea.Cmd) {
	m.showLoad = ""
	moved := 0
	for _, id := range msg.ids {
		if msg.failed[id] != nil {
			continue
		}
		m.removePending(id)
		m.removeEmailByID(id)
		moved++
	}
	m.saveStatus = fmt.Sprintf("Moved %d [SPAM] message(s) to Spam", moved)
	if n := len(msg.failed); n > 0 {
		m.saveStatus = fmt.Sprintf("Moved %d of %d [SPAM] message(s); %d failed", moved, len(msg.ids), n)
		m.err = fmt.Errorf("%d message(s) could not be moved to Spam: %w", n, firstError(msg.failed, msg.ids))
		m.showError = true
	}
	m.saveStatusTime = time.Now()
	return m, nil
}

// firstError returns the failure of the earliest id in list order, so the
// reason shown to the user is deterministic.
func firstError(failed map[email.ID]error, ids []email.ID) error {
	for _, id := range ids {
		if err := failed[id]; err != nil {
			return err
		}
	}
	return nil
}

// handleSpamClassified confirms the badge after the trainer records the
// message as spam. The badge is set optimistically, so a failure rolls it back
// and surfaces the error; success only reports the outcome.
func (m *tuiModel) handleSpamClassified(msg spamClassifiedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.setSpamBadge(msg.id, false)
		m.err = fmt.Errorf("could not learn message as spam: %w", msg.err)
		m.showError = true
		m.saveStatus = ""
		return m, nil
	}
	m.saveStatus = "Learned as spam; marked [SPAM]"
	m.saveStatusTime = time.Now()
	return m, nil
}

func (m *tuiModel) handleUnspamDone(msg unspamDoneMsg) (tea.Model, tea.Cmd) {
	m.showLoad = ""
	for i, e := range m.rawEmails {
		if e.ID == msg.id {
			m.rawEmails[i].IsSpam = false
			m.rawEmails[i].IsPolitical = false
			m.rawEmails[i].IsBlacklisted = false
		}
	}
	for i, e := range m.emails {
		if e.ID == msg.id {
			m.emails[i].IsSpam = false
			m.emails[i].IsPolitical = false
			m.emails[i].IsBlacklisted = false
		}
	}
	if !strings.EqualFold(m.currentF, "inbox") {
		for i, e := range m.emails {
			if e.ID == msg.id {
				m.emails = append(m.emails[:i], m.emails[i+1:]...)
				break
			}
		}
	}
	m.removePending(msg.id)
	if len(m.emails) > 0 {
		m.eIdx = min(m.eIdx, len(m.emails)-1)
		m.selectedID = m.emails[m.eIdx].ID
	} else {
		m.selectedID = ""
	}
	return m, nil
}

func (m *tuiModel) handleArchiveDone(msg archiveDoneMsg) (tea.Model, tea.Cmd) {
	m.showLoad = ""
	m.removeEmailFromList(msg.id)
	return m, nil
}

func (m *tuiModel) handleDeleteDone(msg deleteDoneMsg) (tea.Model, tea.Cmd) {
	m.showLoad = ""
	m.removeEmailFromList(msg.id)
	return m, nil
}

func (m *tuiModel) handleBlockDone(msg blockDoneMsg) (tea.Model, tea.Cmd) {
	m.showLoad = ""
	if msg.err != nil {
		m.err = msg.err
		m.showError = true
		return m, nil
	}
	m.removeEmailFromList(msg.id)
	return m, nil
}

// cacheSummary records a generated summary and its parsed form.
//
// The parsed classification is shown in the summary pane as the model's
// opinion and nothing more. It used to be written into the message's cached
// spam and political verdict, which the scan then trusts: a small local model
// routinely echoes the prompt's "Marketing / Political" choice for seminar
// announcements and the user's own replies, and those labels stuck to every
// message the TUI summarized, Sent included.
func (m *tuiModel) cacheSummary(msg summaryLoadedMsg) {
	m.summaryCache[msg.id] = msg.summary
	if m.summaryParsedCache == nil {
		m.summaryParsedCache = make(map[email.ID]llm.ParsedSummary)
	}
	m.summaryParsedCache[msg.id] = llm.ParseSummary(msg.summary)
}

func (m *tuiModel) handleSummaryLoaded(msg summaryLoadedMsg) (tea.Model, tea.Cmd) {
	m.summaryInFlight = false
	m.currentSummarizingID = ""
	if m.summaryCache == nil {
		m.summaryCache = make(map[email.ID]string)
	}
	if msg.err == nil && msg.summary != "" {
		m.cacheSummary(msg)
	}

	if m.waitingUserSummaryID == msg.id {
		m.showLoad = ""
		m.waitingUserSummaryID = ""
		if msg.err != nil {
			m.err = msg.err
			m.showError = true
		} else {
			m.showSummary = true
			m.summaryContent = msg.summary
			m.summaryEmailID = msg.id
			m.summaryFrom = msg.from
			m.summarySubject = msg.subject
			m.summaryDate = msg.date
			m.setupSummaryViewport()
		}
	}

	if nextCmd := m.processNextSummaryQueueItem(); nextCmd != nil {
		return m, nextCmd
	}
	return m, nil
}

func (m *tuiModel) handleEmailSent(msg emailSentMsg) (tea.Model, tea.Cmd) {
	m.showLoad = ""
	sameSession := m.compose != nil && m.compose.MessageID == msg.session
	if msg.err != nil {
		return m.handleSendFailed(msg, sameSession)
	}
	m.err = nil
	m.showError = false
	var cmd tea.Cmd
	if sameSession {
		quit := m.standaloneCompose || m.compose.quitAfterSend
		encrypted := m.compose.Encrypt
		m.mode = modeIndex
		m.compose = nil
		m.saveStatus = "Email sent successfully!"
		m.saveStatusTime = time.Now()
		// The server copy is now a draft of a sent message: retire it,
		// after any upload still in flight has landed.
		req := syncRequest{kind: syncRetire, client: m.client, encrypted: encrypted}
		if quit {
			cmd = m.requestQuitAfterSync(msg.session, req)
		} else {
			cmd = m.requestSync(msg.session, req)
		}
	}
	m.markReplied(msg.targetID)
	return m, cmd
}

// handleSendFailed keeps the session open for another try; a Ctrl+C that
// waited on the send now saves the draft and exits.
func (m *tuiModel) handleSendFailed(msg emailSentMsg, sameSession bool) (tea.Model, tea.Cmd) {
	m.err = msg.err
	m.showError = true
	if !sameSession {
		return m, nil
	}
	m.compose.sending = false
	if !m.compose.quitAfterSend {
		return m, nil
	}
	model, cmd := m.saveDraftAndLeave(true)
	m.saveStatus = joinStatus("Send failed: "+msg.err.Error(), m.saveStatus)
	if m.done {
		m.exitNotice = m.saveStatus
	}
	return model, cmd
}

func (m *tuiModel) markReplied(targetID email.ID) {
	if targetID == "" {
		return
	}
	m.isReplied[targetID] = true
	if m.client != nil && m.client.Config() != nil {
		_ = cache.MarkIDsReplied(m.client.Config().AccountCache, []email.ID{targetID}, m.isReplied)
	}
	for i := range m.rawEmails {
		if m.rawEmails[i].ID == targetID {
			m.rawEmails[i].IsReplied = true
		}
	}
	for i := range m.emails {
		if m.emails[i].ID == targetID {
			m.emails[i].IsReplied = true
		}
	}
}

func (m *tuiModel) handleICSCalCheck(msg icsCalCheckMsg) (tea.Model, tea.Cmd) {
	switch {
	case msg.err != nil:
		m.icsCheckStatus = icsStatusError
		m.icsCheckErr = msg.err.Error()
	case msg.inCalendar:
		if msg.discrepancy != "" {
			m.icsCheckStatus = icsStatusSimilarFound
			m.icsInCalendar = true
			m.icsDiscrepancy = msg.discrepancy
		} else {
			m.icsCheckStatus = icsStatusInCalendar
			m.icsInCalendar = true
			m.icsDiscrepancy = ""
		}
	default:
		m.icsCheckStatus = icsStatusNotInCalendar
		m.icsInCalendar = false
		m.icsDiscrepancy = ""
	}
	if msg.event != nil {
		m.icsEvent = msg.event
	}
	m.detailVpDirty = true
	return m, nil
}

func (m *tuiModel) handleICSCalAdd(msg icsCalAddMsg) (tea.Model, tea.Cmd) {
	switch {
	case msg.err != nil:
		m.icsCheckStatus = icsStatusError
		m.icsCheckErr = msg.err.Error()
		m.icsAddStatus = "Error: " + msg.err.Error()
	case msg.verified && msg.existed:
		m.icsInCalendar = true
		m.icsCheckStatus = icsStatusUpdatedVerified
		m.icsAddStatus = "✓ Event updated and verified in Calendar"
	case msg.verified && msg.noDuplicates:
		m.icsInCalendar = true
		m.icsCheckStatus = icsStatusAddedVerified
		m.icsAddStatus = "✓ Event added and verified in Calendar (no duplicates)"
	case msg.verified:
		m.icsInCalendar = true
		m.icsCheckStatus = icsStatusAddedWithDups
		m.icsAddStatus = "⚠ Event added, but duplicate entries detected in Calendar"
	case msg.existed:
		m.icsCheckStatus = icsStatusInCalendar
		m.icsInCalendar = true
		m.icsAddStatus = "Already present in Google Calendar"
	}
	m.detailVpDirty = true
	return m, nil
}

func (m *tuiModel) updateInner(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.handleWindowSize(msg)
	case tea.KeyMsg:
		return m.key(msg)
	case tea.MouseMsg:
		return m.handleMouse(msg)
	case foldersLoadedMsg:
		return m.handleFoldersLoaded(msg)
	case emailsLoadedMsg:
		return m.handleEmailsLoaded(msg)
	case classifyDoneMsg:
		slog.Info("classifyDoneMsg: reloading folder display after background classification")
		m.loadCachedEmailsForFolder(m.currentF)
		return m, nil
	case detailMsg:
		return m.handleDetailLoaded(msg)
	case doneMsg:
		return m.handleDoneMsg(msg)
	case spamDoneMsg:
		return m.handleSpamDone(msg)
	case unspamDoneMsg:
		return m.handleUnspamDone(msg)
	case markedSpamDoneMsg:
		return m.handleMarkedSpamDone(msg)
	case spamClassifiedMsg:
		return m.handleSpamClassified(msg)
	case archiveDoneMsg:
		return m.handleArchiveDone(msg)
	case deleteDoneMsg:
		return m.handleDeleteDone(msg)
	case blockDoneMsg:
		return m.handleBlockDone(msg)
	case saveEmailDoneMsg:
		m.reportActionOutcome(msg.err,
			fmt.Sprintf("failed to save message to %s", msg.folder),
			fmt.Sprintf("Saved message to %s", msg.folder))
		return m, nil
	case composeEditorFinishedMsg:
		return m.handleComposeEditorFinished(msg)
	case draftSyncedMsg:
		return m.handleDraftSynced(msg)
	case clearSpamUndoMsg:
		if time.Since(m.lastSpammedTime) >= 3*time.Second {
			m.undoSpamBanner = ""
		}
		return m, nil
	case messageReadyToSaveMsg:
		return m.handleMessageReadyToSave(msg)
	case noticeExpiredMsg:
		return m, nil
	case summaryLoadedMsg:
		return m.handleSummaryLoaded(msg)
	case emailSentMsg:
		return m.handleEmailSent(msg)
	case errMsg:
		m.showLoad = ""
		m.err = msg.err
		m.showError = true
		return m, nil
	case icsCalCheckMsg:
		return m.handleICSCalCheck(msg)
	case icsCalAddMsg:
		return m.handleICSCalAdd(msg)
	case ReportMsg:
		return m.handleReport(msg)
	case accountLoginFinishedMsg:
		m.accountOverlayOpen = false
		if msg.err != nil {
			m.err = fmt.Errorf("login failed for %q: %w", msg.acc.Name, msg.err)
			m.showError = true
			return m, nil
		}
		return m.switchAccount(msg.acc)
	case timeTickMsg:
		m.lastTime = time.Now()
		m.cleanupIsReadMap()
		return m, tea.Tick(time.Second*10, func(time.Time) tea.Msg { return timeTickMsg{} })
	case messageReadyForPipeMsg:
		return m.handleMessageReadyForPipe(msg)
	case pipeFinishedMsg:
		m.reportActionOutcome(msg.err,
			fmt.Sprintf("pipe to %q failed", msg.cmd),
			fmt.Sprintf("Piped message to %q", msg.cmd))
		return m, nil
	case bulkArchiveDoneMsg, bulkDeleteDoneMsg, bulkMarkReadDoneMsg, bulkSpamDoneMsg:
		return m.handleBulkDone(msg)
	case remoteSearchDoneMsg:
		return m.handleRemoteSearchDone(msg)
	default:
		return m, nil
	}
}

// reportActionOutcome records the result of a deferred background action: a
// failure surfaces errPrefix plus the error, a success shows okMsg in the
// status line.
func (m *tuiModel) reportActionOutcome(err error, errPrefix, okMsg string) {
	if err != nil {
		m.err = fmt.Errorf("%s: %w", errPrefix, err)
		m.showError = true
		return
	}
	m.saveStatus = okMsg
	m.saveStatusTime = time.Now()
}

// handleBulkDone reports a bulk action: the count is of the ids the servers
// accepted, and any failure is shown as an error beside it.
func (m *tuiModel) handleBulkDone(msg tea.Msg) (tea.Model, tea.Cmd) {
	var err error
	switch msg := msg.(type) {
	case bulkArchiveDoneMsg:
		m.saveStatus = fmt.Sprintf("Archived %d message(s)", len(msg.ids))
		err = msg.err
	case bulkDeleteDoneMsg:
		m.saveStatus = fmt.Sprintf("Deleted %d message(s)", len(msg.ids))
		err = msg.err
	case bulkMarkReadDoneMsg:
		m.saveStatus = fmt.Sprintf("Marked %d message(s) as read", len(msg.ids))
		if !msg.read {
			m.saveStatus = fmt.Sprintf("Marked %d message(s) as unread locally (server unchanged)", len(msg.ids))
		}
		err = msg.err
	case bulkSpamDoneMsg:
		m.saveStatus = fmt.Sprintf("Reported %d message(s) as spam", len(msg.ids))
		err = msg.err
	}
	m.saveStatusTime = time.Now()
	if err != nil {
		m.err = fmt.Errorf("bulk action failed: %w", err)
		m.showError = true
	}
	return m, nil
}
