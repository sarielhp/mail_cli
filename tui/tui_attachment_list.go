package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sarielhp/mail_cli/cache/msg"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/tui/render"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func (m *tuiModel) currentEmailSubject() string {
	if m.eIdx >= 0 && m.eIdx < len(m.emails) {
		return m.emails[m.eIdx].Subject
	}
	return ""
}

func (m *tuiModel) openAttachmentList() (tea.Model, tea.Cmd) {
	if len(m.emails) == 0 {
		m.openAccountOverlay()
		return m, nil
	}
	if len(m.attachments) == 0 {
		m.loadAttachmentsForCurrentEmail()
	}
	if len(m.attachments) == 0 {
		m.saveStatus = "No attachments on this message"
		m.saveStatusTime = time.Now()
		return m, nil
	}
	m.attachmentListOpen = true
	m.attachmentViewerOpen = false
	m.attachmentListCursor = 0
	m.attachmentDeleted = make(map[int]bool)
	m.saveStatus = ""
	return m, nil
}

func (m *tuiModel) loadAttachmentsForCurrentEmail() {
	cfg := m.Config()
	if cfg == nil {
		cfg = m.cfg
	}
	if cfg == nil {
		return
	}
	msgID := m.currentEmailID()
	if msgID == "" {
		return
	}
	data, err := msg.Read(cfg.AccountCache, msgID)
	if err != nil {
		return
	}
	atts, _ := email.ExtractAttachments(data)
	if plain, html, err := email.ExtractAlternativeBodyParts(data); err == nil {
		var parts []email.Attachment
		if strings.TrimSpace(html) != "" {
			parts = append(parts, email.Attachment{
				Filename:    "message_body.html",
				ContentType: "text/html",
				Data:        []byte(html),
			})
		}
		if strings.TrimSpace(plain) != "" && strings.TrimSpace(html) != "" {
			parts = append(parts, email.Attachment{
				Filename:    "message_body.txt",
				ContentType: "text/plain",
				Data:        []byte(plain),
			})
		}
		atts = append(parts, atts...)
	}
	m.attachments = atts
}

// isLocallyViewableAttachment reports whether the read-only text viewer renders
// this attachment. Calendar invites are not shown there: they open the calendar
// modal, which shows the parsed event and can add it to the calendar.
func isLocallyViewableAttachment(att email.Attachment) bool {
	if isPDFAttachment(att) || isImageAttachment(att) {
		return false
	}
	return !email.IsICSAttachment(att) && isTextAttachment(att)
}

func (m *tuiModel) handleAttachmentListKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.attachmentViewerOpen {
		return m.handleAttachmentViewerKey(k)
	}

	s := k.String()
	switch s {
	case "esc", "q":
		if len(m.attachmentDeleted) > 0 {
			m.applyAttachmentDeletions()
		}
		m.attachmentListOpen = false
		m.attachmentViewerOpen = false
		return m, nil
	case "up", "k":
		if m.attachmentListCursor > 0 {
			m.attachmentListCursor--
		}
		return m, nil
	case "down", "j":
		if m.attachmentListCursor < len(m.attachments)-1 {
			m.attachmentListCursor++
		}
		return m, nil
	case "d":
		if m.attachmentListCursor >= 0 && m.attachmentListCursor < len(m.attachments) {
			fn := m.attachments[m.attachmentListCursor].Filename
			if fn == "message_body.html" || fn == "message_body.txt" {
				m.saveStatus = "Cannot delete message body part"
				m.saveStatusTime = time.Now()
				return m, nil
			}
		}
		m.attachmentDeleted[m.attachmentListCursor] = true
		return m, nil
	case "u":
		delete(m.attachmentDeleted, m.attachmentListCursor)
		return m, nil
	case "s":
		return m.promptSaveAttachment(m.attachmentListCursor)
	case "S":
		return m.quickSaveAttachment(m.attachmentListCursor)
	case "o", "x":
		m.openAttachmentExternally(m.attachmentListCursor, "Opened in external program: ", "Error opening: ")
		return m, nil
	case "enter":
		return m.viewOrOpenAttachment(m.attachmentListCursor)
	case "c":
		return m.addICSFromAttachmentList()
	}
	return m, nil
}

// addICSFromAttachmentList is 'c' in the attachment list: add the message's
// invitation to the calendar. The list opens on its first entry, which is
// usually the message body, so the cursor's attachment is used only when it is
// an .ics; otherwise the message's first .ics is. It opens the calendar modal,
// which shows the event and the add's progress and verdict, and starts the
// add at once rather than the modal's read-only presence check, whose answer
// could otherwise land after the add's and overwrite it.
func (m *tuiModel) addICSFromAttachmentList() (tea.Model, tea.Cmd) {
	idx := m.icsAttachmentIndex()
	if idx < 0 {
		m.saveStatus = "No calendar (.ics) attachment on this message"
		m.saveStatusTime = time.Now()
		return m, nil
	}
	m.attachmentListCursor = idx
	m.viewOrOpenAttachment(idx)
	if m.icsEvent == nil {
		return m, nil
	}
	m.icsCheckStatus = icsStatusAdding
	m.icsAddStatus = "Adding to Google Calendar & verifying..."
	return m, m.cmdAddICSCalendar(m.icsData)
}

// icsAttachmentIndex is the attachment 'c' acts on: the one under the list's
// cursor if it is an .ics, else the first .ics, else -1.
func (m *tuiModel) icsAttachmentIndex() int {
	if c := m.attachmentListCursor; c >= 0 && c < len(m.attachments) && email.IsICSAttachment(m.attachments[c]) {
		return c
	}
	for i, att := range m.attachments {
		if email.IsICSAttachment(att) {
			return i
		}
	}
	return -1
}

func (m *tuiModel) handleAttachmentViewerKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := k.String()
	switch s {
	case "esc", "q":
		m.attachmentViewerOpen = false
		return m, nil
	case "up", "k":
		m.attachmentViewerVp.ScrollUp(1)
		return m, nil
	case "down", "j":
		m.attachmentViewerVp.ScrollDown(1)
		return m, nil
	case "pgup":
		m.attachmentViewerVp.HalfPageUp()
		return m, nil
	case "pgdown":
		m.attachmentViewerVp.HalfPageDown()
		return m, nil
	case "s":
		return m.promptSaveAttachment(m.attachmentListCursor)
	case "S":
		return m.quickSaveAttachment(m.attachmentListCursor)
	case "o", "x":
		m.openAttachmentExternally(m.attachmentListCursor, "Opened in external program: ", "Error opening: ")
		return m, nil
	}
	return m, nil
}

func (m *tuiModel) promptSaveAttachment(idx int) (tea.Model, tea.Cmd) {
	if idx < 0 || idx >= len(m.attachments) {
		return m, nil
	}
	m.openAttachmentSaveDialog(m.attachments[idx])
	return m, nil
}

func (m *tuiModel) quickSaveAttachment(idx int) (tea.Model, tea.Cmd) {
	if idx < 0 || idx >= len(m.attachments) {
		return m, nil
	}
	att := m.attachments[idx]
	savePattern := m.saveDirPattern()
	destDir, err := cfg_g.ExpandSavePath(savePattern, time.Now())
	if err != nil {
		m.saveStatus = "Error resolving path: " + err.Error()
		m.saveStatusTime = time.Now()
		return m, nil
	}
	if err := os.MkdirAll(destDir, 0755); err != nil {
		m.saveStatus = "Error creating dir: " + err.Error()
		m.saveStatusTime = time.Now()
		return m, nil
	}
	destPath := filepath.Join(destDir, att.Filename)
	if err := os.WriteFile(destPath, att.Data, 0644); err != nil {
		m.saveStatus = "Error saving: " + err.Error()
		m.saveStatusTime = time.Now()
		return m, nil
	}
	m.saveStatus = "Quick-saved to " + destPath
	m.saveStatusTime = time.Now()
	return m, nil
}

func (m *tuiModel) viewOrOpenAttachment(idx int) (tea.Model, tea.Cmd) {
	if idx < 0 || idx >= len(m.attachments) {
		return m, nil
	}
	att := m.attachments[idx]
	if email.IsICSAttachment(att) {
		// An .ics attachment opens the calendar modal, which shows the parsed
		// event and can add it to the calendar. That modal is a sibling overlay
		// to this list, and the list is consulted first by both the key and the
		// render dispatcher, so the list must close or the modal is unreachable
		// and 'c' never arrives.
		m.attachmentListOpen = false
		m.attachmentViewerOpen = false
		m.attCursor = idx
		return m.openICSViewer(att)
	}
	if isLocallyViewableAttachment(att) {
		m.attachmentViewerOpen = true
		vw := max(20, m.width-4)
		vh := max(5, m.height-6)
		m.attachmentViewerVp = viewport.New(vw, vh)
		m.attachmentViewerVp.SetContent(renderCodeAttachmentPreview(att, vw))
		return m, nil
	}

	m.openAttachmentExternally(idx, "Opened in external viewer: ", "Error opening: ")
	return m, nil
}

// openAttachmentExternally writes attachments[idx] to a temp file and hands it
// to the system opener. openedPrefix and errorPrefix label the status line.
func (m *tuiModel) openAttachmentExternally(idx int, openedPrefix, errorPrefix string) {
	if idx < 0 || idx >= len(m.attachments) {
		return
	}
	att := m.attachments[idx]
	tmpFile, err := writeTempAttachment(att)
	if err == nil {
		go openSystemFile(tmpFile)
		m.saveStatus = openedPrefix + att.Filename
	} else {
		m.saveStatus = errorPrefix + err.Error()
	}
	m.saveStatusTime = time.Now()
}

func (m *tuiModel) applyAttachmentDeletions() {
	if len(m.attachmentDeleted) == 0 {
		return
	}
	cfg := m.Config()
	if cfg == nil {
		cfg = m.cfg
	}
	if cfg == nil {
		return
	}
	msgID := m.currentEmailID()
	if msgID == "" {
		return
	}

	var filenamesToDelete []string
	for idx := range m.attachmentDeleted {
		if idx >= 0 && idx < len(m.attachments) {
			filenamesToDelete = append(filenamesToDelete, m.attachments[idx].Filename)
		}
	}
	if len(filenamesToDelete) == 0 {
		return
	}

	rawEML, err := msg.Read(cfg.AccountCache, msgID)
	if err != nil {
		return
	}
	newEML, stripped, err := email.StripAttachmentsFromEML(rawEML, filenamesToDelete)
	if err != nil || !stripped {
		return
	}

	info, _ := msg.GetInfo(cfg.AccountCache, msgID)
	receivedDate := time.Now()
	if info != nil && !info.Date.IsZero() {
		receivedDate = info.Date
	}
	if err := msg.Store(cfg.AccountCache, msgID, newEML, receivedDate); err == nil {
		m.saveStatus = fmt.Sprintf("Deleted %d attachment(s) from local cache", len(filenamesToDelete))
		m.saveStatusTime = time.Now()
		if atts, err := email.ExtractAttachments(newEML); err == nil {
			m.attachments = atts
		}
		m.detailVpDirty = true
	}
}

func (m *tuiModel) renderAttachmentListView() string {
	if m.attachmentViewerOpen {
		return m.renderAttachmentViewer()
	}
	return m.renderAttachmentListMenu()
}

func (m *tuiModel) renderAttachmentViewer() string {
	t := m.theme.Theme()
	bg := lipgloss.Color("#1a1a35")
	boxW := max(20, m.width)

	headerStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("#2a2a55")).
		Foreground(lipgloss.Color("#ffd700")).
		Bold(true).
		Padding(0, 1)

	attName := ""
	attSize := 0
	if m.attachmentListCursor >= 0 && m.attachmentListCursor < len(m.attachments) {
		attName = m.attachments[m.attachmentListCursor].Filename
		attSize = len(m.attachments[m.attachmentListCursor].Data)
	}

	subj := m.currentEmailSubject()
	if len(subj) > 30 {
		subj = subj[:27] + "..."
	}
	headerText := fmt.Sprintf("Folder: %s | Message: %s | Attachment: %s (%s)",
		m.currentF, subj, attName, formatFileSize(attSize))

	header := headerStyle.Render(headerText)
	div := lipgloss.NewStyle().Foreground(lipgloss.Color(t.Get(render.ColorBorder))).
		Render(strings.Repeat("─", boxW))

	dimStyle := lipgloss.NewStyle().Background(bg).Foreground(lipgloss.Color(t.Get(render.ColorDate)))
	keyStyle := lipgloss.NewStyle().Background(bg).Foreground(lipgloss.Color("#ffd700")).Bold(true)

	footer := fitHints(m.width-1, "  |  ", []string{
		keyStyle.Render("[Esc/q] Back to list"),
		keyStyle.Render("[s] Save to disk"),
		keyStyle.Render("[S] Quick save"),
		keyStyle.Render("[o] Open external"),
		dimStyle.Render("[j/k] Scroll"),
	})
	footer = markHints(footer, attachmentListHints...)

	var lines []string
	// The body gets what the header, the rules and the footer rows leave.
	m.attachmentViewerVp.Height = max(3, m.height-4-strings.Count(footer, "\n"))
	lines = append(lines, header, div, m.attachmentViewerVp.View(), div, footer)
	content := strings.Join(lines, "\n")
	return lipgloss.NewStyle().Background(bg).Width(m.width).Height(m.height).Render(content)
}

func (m *tuiModel) renderAttachmentListMenu() string {
	t := m.theme.Theme()
	bg := lipgloss.Color("#1a1a35")
	boxW := max(20, m.width)

	headerStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("#2a2a55")).
		Foreground(lipgloss.Color("#ffd700")).
		Bold(true).
		Padding(0, 1)

	subj := m.currentEmailSubject()
	if len(subj) > 40 {
		subj = subj[:37] + "..."
	}
	title := fmt.Sprintf("📎 ATTACHMENTS & PARTS (%d) — Message: %s", len(m.attachments), subj)
	header := headerStyle.Render(title)

	div := lipgloss.NewStyle().Foreground(lipgloss.Color(t.Get(render.ColorBorder))).
		Render(strings.Repeat("─", boxW))

	var lines []string
	lines = append(lines, header, div, "")

	availH := max(5, m.height-7)
	for i, att := range m.attachments {
		if i >= availH {
			lines = append(lines, fmt.Sprintf("  ... (+%d more attachments)", len(m.attachments)-availH))
			break
		}

		cursor := "  "
		isCursor := i == m.attachmentListCursor
		if isCursor {
			cursor = "▶ "
		}

		icon := getAttachmentIcon(att)
		sizeStr := formatFileSize(len(att.Data))
		mimeStr := att.ContentType
		if mimeStr == "" {
			mimeStr = "application/octet-stream"
		}

		delBadge := ""
		if m.attachmentDeleted[i] {
			delBadge = " " + lipgloss.NewStyle().Foreground(lipgloss.Color("#e06c75")).Bold(true).Render("[DELETED]")
		}

		rowText := fmt.Sprintf("%s%d. [%s] %-30s %10s   %-22s%s",
			cursor, i+1, strings.TrimSpace(icon), truncateString(att.Filename, 30), sizeStr, truncateString(mimeStr, 22), delBadge)

		if isCursor {
			rowText = lipgloss.NewStyle().
				Background(lipgloss.Color(t.Get(render.ColorHighlightBg))).
				Foreground(lipgloss.Color(t.Get(render.ColorHighlightFg))).
				Bold(true).
				Render(rowText)
		} else {
			rowText = lipgloss.NewStyle().
				Foreground(lipgloss.Color(t.Get(render.ColorFg))).
				Render(rowText)
		}

		lines = append(lines, rowZone(frameAttList, i, 0, "  "+rowText))
	}

	dimStyle := lipgloss.NewStyle().Background(bg).Foreground(lipgloss.Color(t.Get(render.ColorDate)))
	keyStyle := lipgloss.NewStyle().Background(bg).Foreground(lipgloss.Color("#ffd700")).Bold(true)

	footer := fitHints(m.width-1, "  |  ", []string{
		keyStyle.Render("[Enter] View"),
		keyStyle.Render("[o/x] Open ext"),
		keyStyle.Render("[c] Add to calendar"),
		keyStyle.Render("[s] Save to disk"),
		keyStyle.Render("[S] Quick save"),
		keyStyle.Render("[d] Delete"),
		keyStyle.Render("[u] Undelete"),
		dimStyle.Render("[j/k] Navigate"),
		dimStyle.Render("[Esc/q] Back"),
	})
	footer = markHints(footer, attachmentListHints...)

	for len(lines) < m.height-3-(strings.Count(footer, "\n")+1) {
		lines = append(lines, "")
	}

	if m.saveStatus != "" {
		statusStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#88c0d0")).Bold(true)
		lines = append(lines, "  "+statusStyle.Render(m.saveStatus))
	} else {
		lines = append(lines, "")
	}

	lines = append(lines, div)

	lines = append(lines, footer)
	content := strings.Join(lines, "\n")
	return lipgloss.NewStyle().Background(bg).Width(m.width).Height(m.height).Render(content)
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// attachmentListHints are the clickable hints of the attachment list and
// its viewer. The longer of two labels sharing a start comes first.
var attachmentListHints = []hint{
	{"[Enter] View", "enter"}, {"[o/x] Open ext", "o"}, {"[o] Open external", "o"},
	{"[c] Add to calendar", "c"},
	{"[s] Save to disk", "s"}, {"[S] Quick save", "S"}, {"[d] Delete", "d"}, {"[u] Undelete", "u"},
	{"[Esc/q] Back to list", "esc"}, {"[Esc/q] Back", "esc"},
}
