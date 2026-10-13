package tui

import (
	zone "github.com/lrstanley/bubblezone"
	"log/slog"
	"strings"
	"time"

	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/tui/render"

	"github.com/charmbracelet/lipgloss"
)

func (m *tuiModel) withErrorBanner(content string) string {
	if m.showError {
		return content + "\n" + renderErrorBanner(m)
	}
	return content
}

func (m *tuiModel) renderLoadingOverlay() string {
	if m.showLoad == "Downloading..." {
		return renderDownloadingDialog(m.width, m.height)
	}
	if m.showLoad == "Unspamming..." {
		return renderUnspammingDialog(m.width, m.height)
	}
	return lipgloss.NewStyle().Width(m.width).Height(max(5, m.height/2)).
		Align(lipgloss.Center, lipgloss.Center).Bold(true).Render(m.showLoad)
}

// composeFrameKind is what renderComposeModal draws: a picker, or the screen.
func (m *tuiModel) composeFrameKind() string {
	switch {
	case m.contactPick != nil:
		return frameContacts
	case m.accountOverlayOpen:
		return frameAccount
	}
	return frameCompose
}

// renderComposeModal is the compose screen, or the picker drawn over it.
func (m *tuiModel) renderComposeModal() string {
	switch {
	case m.contactPick != nil:
		return renderContactPicker(m)
	case m.accountOverlayOpen:
		return renderAccountOverlay(m)
	}
	return m.renderComposeScreen()
}

func (m *tuiModel) tryRenderModalOverlay() (string, bool) {
	if m.showHelp {
		m.frameKind = frameHelp
		m.treeOpen = false
		m.accountOverlayOpen = false
		return renderDimmedOverlay(m.width, m.height, m.renderBaseScreen(), m.withErrorBanner(renderHelp(m))), true
	}
	if m.mode == modeCompose && !m.fileBrowserOpen && !m.attModalOpen {
		m.frameKind = m.composeFrameKind()
		return m.withErrorBanner(m.renderComposeModal()), true
	}
	if m.showDeadlinesAgenda {
		m.frameKind = frameDeadlines
		return m.withErrorBanner(renderDeadlinesAgenda(m)), true
	}
	if m.fileBrowserOpen {
		m.frameKind = frameFiles
		return m.withErrorBanner(renderDimmedOverlay(m.width, m.height, m.renderBaseScreen(), m.fileBrowser.View())), true
	}
	if m.attachmentListOpen {
		m.frameKind = frameAttList
		return m.withErrorBanner(m.renderAttachmentListView()), true
	}
	if m.icsModalOpen {
		m.frameKind = frameICS
		return m.withErrorBanner(renderICSModalOverlay(m)), true
	}
	if m.attModalOpen {
		m.frameKind = frameAttModal
		return m.withErrorBanner(renderAttachmentModalOverlay(m)), true
	}
	if m.attachmentHelpOpen {
		m.frameKind = frameAttHelp
		return renderDimmedOverlay(m.width, m.height, m.renderBaseScreen(), m.withErrorBanner(renderAttachmentHelp(m))), true
	}
	if m.showSummary {
		m.frameKind = frameSummary
		return m.withErrorBanner(renderSummaryOverlay(m)), true
	}
	if m.showDiag {
		m.frameKind = frameDiag
		return renderDiag(m), true
	}
	if m.accountOverlayOpen {
		m.frameKind = frameAccount
		return m.withErrorBanner(renderAccountOverlay(m)), true
	}
	if m.treeOpen && len(m.entries) > 0 {
		m.frameKind = frameTree
		return m.withErrorBanner(renderFolderOverlay(m, renderFolderTree(m))), true
	}
	if m.blockModalOpen {
		m.frameKind = frameBlock
		return m.withErrorBanner(renderBlockDialog(m)), true
	}
	if m.spamMenuOpen {
		m.frameKind = frameSpam
		return m.withErrorBanner(renderSpamMenu(m)), true
	}
	if m.showLoad != "" {
		m.frameKind = frameLoading
		return m.withErrorBanner(m.renderLoadingOverlay()), true
	}
	return "", false
}

func (m *tuiModel) resolveViewableAttachment() *email.Attachment {
	if m.hideRightPane {
		return nil
	}
	for i := range m.attachments {
		if email.IsICSAttachment(m.attachments[i]) {
			return &m.attachments[i]
		}
	}
	return nil
}

func (m *tuiModel) renderDetailView(topBar string) string {
	width := m.width
	if m.menuOpen {
		width = m.width - 19
	}
	viewableAtt := m.resolveViewableAttachment()
	isWideSplit := width >= 100 && viewableAtt != nil
	leftWidth := width
	rightWidth := 0
	if isWideSplit {
		leftWidth = width / 2
		rightWidth = width - leftWidth
	}

	var selectedLine string
	if m.eIdx >= 0 && m.eIdx < len(m.emails) {
		sjw := leftWidth - headerColTotal
		m.selectedRow = render.EmailRowTagged(m.emails[m.eIdx], leftWidth, sjw, m.eIdx+1, m.theme.Theme(), true, m.isPending(m.emails[m.eIdx].ID), m.isTagged(m.emails[m.eIdx].ID))
		selectedLine = m.selectedRow.Render(m.theme.Theme())
	}
	vh := max(5, m.height-3)
	if m.showError {
		vh = max(5, vh-1)
	}
	vw := max(20, leftWidth-2)
	m.refreshDetailViewport(vw, vh)
	viewportStr := m.vp.View()

	var bodyParts []string
	if selectedLine != "" {
		bodyParts = append(bodyParts, selectedLine)
	}
	bodyParts = append(bodyParts, viewportStr)
	leftBodyStr := strings.Join(bodyParts, "\n")

	bodyStr := leftBodyStr
	if isWideSplit {
		rightPane := renderRightGraphicPane(m, *viewableAtt, rightWidth, m.height-3)
		bodyStr = lipgloss.JoinHorizontal(lipgloss.Top, leftBodyStr, rightPane)
	}
	if m.menuOpen {
		bodyStr = lipgloss.JoinHorizontal(lipgloss.Top, renderMenu(m), bodyStr)
	}

	var parts []string
	if topBar != "" {
		parts = append(parts, topBar)
	}
	parts = append(parts, bodyStr)
	if m.undoSpamBanner != "" && time.Since(m.lastSpammedTime) < 3*time.Second {
		parts = append(parts, renderUndoSpamBanner(m))
	}
	if m.noticeVisible() {
		parts = append(parts, renderNotice(m))
	}
	if m.showError {
		parts = append(parts, renderErrorBanner(m))
	}
	return strings.Join(parts, "\n")
}

// refreshDetailViewport rebuilds the detail viewport when its size changed or
// its content is stale, so callers can render it unconditionally afterwards.
func (m *tuiModel) refreshDetailViewport(vw, vh int) {
	if !m.detailVpDirty && m.vp.Width == vw && m.vp.Height == vh {
		return
	}
	m.vp.SetContent(m.buildDetailContent(vw))
	m.vp.Width = vw
	m.vp.Height = vh
	m.detailVpDirty = false
}

func (m *tuiModel) renderIndexView(topBar string) string {
	var parts []string
	if topBar != "" {
		parts = append(parts, topBar)
	}
	var bodyStr string
	if m.splitPreview && len(m.emails) > 0 {
		bodyStr = renderSplitView(m)
	} else {
		bodyStr = renderIndex(m)
	}
	slog.Info("View renderIndex returned",
		slog.Int("width", m.width),
		slog.Int("height", m.height),
		slog.Int("len", len(bodyStr)),
		slog.String("content", bodyStr),
	)
	if m.attachmentDrawer {
		bodyStr = strings.Join([]string{bodyStr, renderAttachmentDrawer(m)}, "\n")
	}
	if m.menuOpen {
		bodyStr = lipgloss.JoinHorizontal(lipgloss.Top, renderMenu(m), bodyStr)
	}
	parts = append(parts, bodyStr)
	if m.undoSpamBanner != "" && time.Since(m.lastSpammedTime) < 3*time.Second {
		parts = append(parts, renderUndoSpamBanner(m))
	}
	if m.noticeVisible() {
		parts = append(parts, renderNotice(m))
	}
	if m.showError {
		parts = append(parts, renderErrorBanner(m))
	}
	return strings.Join(parts, "\n")
}

// View is the screen, with the clickable zones recorded and their markers
// removed; see tui_mouse.go.
func (m *tuiModel) View() string {
	m.lastFrame = zone.Scan(clipToHeight(m.view(), m.height))
	return m.lastFrame
}

// clipToHeight drops lines from the top of s until it fits h, which is what
// bubbletea's renderer does with a view taller than the terminal. Doing it
// first keeps the clickable zones where the text really is: recorded on the
// uncut view, every one of them was a row below its text.
func clipToHeight(s string, h int) string {
	if h <= 0 {
		return s
	}
	lines := strings.Split(s, "\n")
	if len(lines) <= h {
		return s
	}
	return strings.Join(lines[len(lines)-h:], "\n")
}

func (m *tuiModel) view() string {
	if !m.showSummary {
		m.summaryShown = "" // reopened, the summary starts at its top
	}
	if m.done {
		return ""
	}
	if overlay, ok := m.tryRenderModalOverlay(); ok {
		return overlay
	}
	m.frameKind = frameBase
	return m.renderBaseScreen()
}

// renderBaseScreen draws the non-modal screen: the index or detail view under
// its top bar. Modals dim this and composite themselves on top, so it must not
// consult any overlay state.
func (m *tuiModel) renderBaseScreen() string {
	topBar := renderTopBar(m)
	if m.mode == modeDetail {
		return m.renderDetailView(topBar)
	}
	return m.renderIndexView(topBar)
}
