package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sarielhp/mail_cli/calendar"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/tui/render"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type icsCheckStatus int

const (
	icsStatusChecking icsCheckStatus = iota
	icsStatusInCalendar
	icsStatusSimilarFound
	icsStatusNotInCalendar
	icsStatusAdding
	icsStatusAddedVerified
	icsStatusAddedWithDups
	icsStatusUpdatedVerified
	icsStatusError
	icsStatusNotConfigured
)

type icsCalCheckMsg struct {
	inCalendar  bool
	exactMatch  bool
	discrepancy string
	summary     string
	event       *calendar.ICSEvent
	err         error
}

type icsCalAddMsg struct {
	summary      string
	existed      bool
	verified     bool
	noDuplicates bool
	err          error
}

func (m *tuiModel) openICSViewer(att email.Attachment) (tea.Model, tea.Cmd) {
	m.icsModalOpen = true
	m.icsFilename = att.Filename
	m.icsData = att.Data
	m.icsInCalendar = false
	m.icsDiscrepancy = ""
	m.icsCheckErr = ""
	m.icsAddStatus = ""
	m.icsCheckStatus = icsStatusChecking
	m.icsEventIdx = 0

	boxW := min(78, max(45, m.width-8))
	contentW := max(20, boxW-6)
	m.icsVp = viewport.New(contentW, 6)

	if events, err := calendar.ParseAllICSEventsData(att.Data); err == nil && len(events) > 0 {
		m.icsEvents = events
		m.icsEvent = events[0]
		if m.icsEvent.Description != "" {
			m.icsVp.SetContent(email.WrapText(m.icsEvent.Description, contentW))
		} else {
			m.icsVp.SetContent("")
		}
	} else {
		m.icsEvents = nil
		m.icsEvent = nil
		m.icsCheckStatus = icsStatusError
		if err != nil {
			m.icsCheckErr = "Failed to parse ICS attachment: " + err.Error()
		} else {
			m.icsCheckErr = "No calendar events found in ICS"
		}
	}

	var cmd tea.Cmd
	if m.icsEvent != nil {
		cmd = m.cmdCheckICSCalendar(att.Data)
	}
	return m, cmd
}

func (m *tuiModel) updateICSEventSelection(newIdx int) {
	if len(m.icsEvents) == 0 {
		return
	}
	if newIdx < 0 {
		newIdx = len(m.icsEvents) - 1
	} else if newIdx >= len(m.icsEvents) {
		newIdx = 0
	}
	m.icsEventIdx = newIdx
	m.icsEvent = m.icsEvents[newIdx]

	boxW := min(78, max(45, m.width-8))
	contentW := max(20, boxW-6)
	if m.icsEvent.Description != "" {
		m.icsVp.SetContent(email.WrapText(m.icsEvent.Description, contentW))
	} else {
		m.icsVp.SetContent("")
	}
}

func (m *tuiModel) cmdCheckICSCalendar(data []byte) tea.Cmd {
	cfg := m.Config()
	if cfg == nil {
		cfg = m.cfg
	}
	bk := m.bk
	return func() tea.Msg {
		if verifier, ok := bk.verifyingCalendar(); ok {
			res, err := verifier.CheckICSDetailed(cfg, data)
			return icsCalCheckMsg{
				inCalendar:  res.InCalendar,
				exactMatch:  res.ExactMatch,
				discrepancy: res.Discrepancy,
				summary:     res.Summary,
				event:       res.Event,
				err:         err,
			}
		}
		if bk.hasCalendar() {
			inCal, summary, ev, err := bk.Calendar.CheckICS(cfg, data)
			return icsCalCheckMsg{
				inCalendar: inCal,
				exactMatch: inCal,
				summary:    summary,
				event:      ev,
				err:        err,
			}
		}
		return icsCalCheckMsg{
			err: fmt.Errorf("calendar service not available"),
		}
	}
}

func (m *tuiModel) cmdAddICSAsNew(data []byte) tea.Cmd {
	cfg := m.Config()
	if cfg == nil {
		cfg = m.cfg
	}
	bk := m.bk
	return func() tea.Msg {
		if verifier, ok := bk.verifyingCalendar(); ok {
			summary, err := verifier.AddICSAsNew(cfg, data)
			return icsCalAddMsg{
				summary:      summary,
				existed:      false,
				verified:     err == nil,
				noDuplicates: true,
				err:          err,
			}
		}
		return icsCalAddMsg{
			err: fmt.Errorf("calendar service not available"),
		}
	}
}

func (m *tuiModel) cmdAddICSCalendar(data []byte) tea.Cmd {
	cfg := m.Config()
	if cfg == nil {
		cfg = m.cfg
	}
	bk := m.bk
	return func() tea.Msg {
		if verifier, ok := bk.verifyingCalendar(); ok {
			summary, existed, verified, noDups, err := verifier.AddICSVerified(cfg, data)
			return icsCalAddMsg{
				summary:      summary,
				existed:      existed,
				verified:     verified,
				noDuplicates: noDups,
				err:          err,
			}
		}
		return icsCalAddMsg{
			err: fmt.Errorf("calendar service not available"),
		}
	}
}

func (m *tuiModel) saveICSAttachment() {
	att := email.Attachment{Filename: m.icsFilename, Data: m.icsData}
	savePattern := ""
	if m.Config() != nil {
		savePattern = m.Config().AttachmentSaveDir
	} else if m.cfg != nil {
		savePattern = m.cfg.AttachmentSaveDir
	}

	destDir, err := cfg_g.ExpandSavePath(savePattern, time.Now())
	if err != nil {
		m.icsAddStatus = "Error resolving path: " + err.Error()
		return
	}
	if err := os.MkdirAll(destDir, 0755); err != nil {
		m.icsAddStatus = "Error creating dir: " + err.Error()
		return
	}
	destPath := filepath.Join(destDir, att.Filename)
	if err := os.WriteFile(destPath, att.Data, 0644); err != nil {
		m.icsAddStatus = "Error saving: " + err.Error()
		return
	}
	m.icsAddStatus = "Saved to " + destPath
}

func (m *tuiModel) handleICSModalKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := k.String()
	switch s {
	case "esc", "q", "f1":
		m.icsModalOpen = false
		return m, nil
	case "tab", "n", "right":
		if len(m.icsEvents) > 1 {
			m.updateICSEventSelection(m.icsEventIdx + 1)
		} else if len(m.attachments) > 1 {
			return m.selectAttachmentByIndex(m.attCursor + 1)
		}
		return m, nil
	case "shift+tab", "p", "left":
		if len(m.icsEvents) > 1 {
			m.updateICSEventSelection(m.icsEventIdx - 1)
		} else if len(m.attachments) > 1 {
			return m.selectAttachmentByIndex(m.attCursor - 1)
		}
		return m, nil
	case "]", "}":
		if len(m.attachments) > 1 {
			return m.selectAttachmentByIndex(m.attCursor + 1)
		}
		return m, nil
	case "[", "{":
		if len(m.attachments) > 1 {
			return m.selectAttachmentByIndex(m.attCursor - 1)
		}
		return m, nil
	case "s":
		m.saveICSAttachment()
		return m, nil
	case "S", "b", "B":
		m.openAttachmentSaveDialog(email.Attachment{Filename: m.icsFilename, Data: m.icsData})
		m.icsModalOpen = false
		return m, nil
	case "c":
		if m.icsCheckStatus == icsStatusAdding {
			return m, nil
		}
		if m.icsCheckStatus == icsStatusSimilarFound {
			m.icsCheckStatus = icsStatusAdding
			m.icsAddStatus = "Adding as separate event to Google Calendar..."
			return m, m.cmdAddICSAsNew(m.icsData)
		}
		m.icsCheckStatus = icsStatusAdding
		m.icsAddStatus = "Adding to Google Calendar & verifying..."
		return m, m.cmdAddICSCalendar(m.icsData)
	case "a", "enter", "u":
		if m.icsCheckStatus == icsStatusAdding {
			return m, nil
		}
		m.icsCheckStatus = icsStatusAdding
		if m.icsInCalendar {
			m.icsAddStatus = "Updating event in Google Calendar & verifying..."
		} else {
			m.icsAddStatus = "Adding to Google Calendar & verifying..."
		}
		return m, m.cmdAddICSCalendar(m.icsData)
	case "o", "x":
		att := email.Attachment{Filename: m.icsFilename, Data: m.icsData}
		tmpFile, err := writeTempAttachment(att)
		if err == nil {
			go openSystemFile(tmpFile)
			m.icsAddStatus = "Opened in external program: " + m.icsFilename
		} else {
			m.icsAddStatus = "Error opening: " + err.Error()
		}
		return m, nil
	case "up", "k":
		m.icsVp.ScrollUp(1)
	case "down", "j":
		m.icsVp.ScrollDown(1)
	case "pgup":
		m.icsVp.HalfPageUp()
	case "pgdown":
		m.icsVp.HalfPageDown()
	}
	return m, nil
}

func renderICSEventFields(ev *calendar.ICSEvent, labelStyle, valStyle lipgloss.Style, bg lipgloss.Color) []string {
	var lines []string
	summary := ev.Summary
	if summary == "" {
		summary = "(Untitled Event)"
	}
	lines = append(lines, labelStyle.Render("  Title:      ")+valStyle.Bold(true).Render(summary))

	dateStr := formatICSEventTime(ev)
	lines = append(lines, labelStyle.Render("  When:       ")+valStyle.Render(dateStr))

	if ev.RRule != "" {
		rruleText := formatHumanRRule(ev.RRule)
		recurStyle := lipgloss.NewStyle().Background(bg).Foreground(lipgloss.Color("#79c0ff")).Bold(true)
		lines = append(lines, labelStyle.Render("  Repeats:    ")+recurStyle.Render(rruleText))
		if nextOccs := formatNextOccurrences(ev, 2); len(nextOccs) > 0 {
			lines = append(lines, labelStyle.Render("  Next Sched: ")+valStyle.Render(strings.Join(nextOccs, " | ")))
		}
	}

	if len(ev.RDates) > 0 {
		lines = append(lines, labelStyle.Render("  Extra Dates: ")+valStyle.Render(strings.Join(ev.RDates, ", ")))
	}
	if len(ev.ExDates) > 0 {
		lines = append(lines, labelStyle.Render("  Except:     ")+valStyle.Render(strings.Join(ev.ExDates, ", ")))
	}

	switch {
	case ev.ConferenceURL != "":
		lines = append(lines, labelStyle.Render("  Join:       ")+valStyle.Render(ev.ConferenceURL))
	case strings.HasPrefix(ev.Location, "http://") || strings.HasPrefix(ev.Location, "https://"):
		lines = append(lines, labelStyle.Render("  Join:       ")+valStyle.Render(ev.Location))
	case ev.Location != "":
		lines = append(lines, labelStyle.Render("  Location:   ")+valStyle.Render(ev.Location))
	}
	if ev.Organizer != "" {
		lines = append(lines, labelStyle.Render("  Organizer:  ")+valStyle.Render(ev.Organizer))
	}
	if ev.Status != "" {
		lines = append(lines, labelStyle.Render("  Status:     ")+valStyle.Render(ev.Status))
	}
	if len(ev.Attendees) > 0 {
		attStr := strings.Join(ev.Attendees, ", ")
		if len(ev.Attendees) > 3 {
			attStr = fmt.Sprintf("%s (+%d more)", strings.Join(ev.Attendees[:3], ", "), len(ev.Attendees)-3)
		}
		lines = append(lines, labelStyle.Render("  Attendees:  ")+valStyle.Render(attStr))
	}
	return lines
}

func renderICSModalOverlay(m *tuiModel) string {
	t := m.theme.Theme()
	borderColor := t.Get(render.ColorBorder)
	bg := lipgloss.Color("#1a1a35")
	boxW := min(78, max(45, m.width-8))

	headerTitle := "📅 CALENDAR EVENT (.ics)"
	if len(m.icsEvents) > 1 {
		headerTitle = fmt.Sprintf("📅 CALENDAR EVENTS (%d events in .ics)", len(m.icsEvents))
	}

	headerStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("#2a2a55")).
		Foreground(lipgloss.Color("#ffd700")).
		Bold(true).
		Padding(0, 1)

	header := headerStyle.Render(headerTitle)
	if m.icsFilename != "" {
		fileLabel := lipgloss.NewStyle().
			Background(bg).
			Foreground(lipgloss.Color("#8888aa")).
			Render(fmt.Sprintf(" File: %s", m.icsFilename))
		header = lipgloss.JoinHorizontal(lipgloss.Left, header, fileLabel)
	}

	labelStyle := lipgloss.NewStyle().
		Background(bg).
		Foreground(lipgloss.Color("#ffd700")).
		Bold(true)

	valStyle := lipgloss.NewStyle().
		Background(bg).
		Foreground(lipgloss.Color(t.Get(render.ColorFg)))

	divStyle := lipgloss.NewStyle().
		Background(bg).
		Foreground(lipgloss.Color(t.Get(render.ColorBorder)))

	var lines []string
	lines = append(lines, header)

	if len(m.icsEvents) > 1 {
		navText := fmt.Sprintf(" Event %d of %d  (Tab / n / p: Switch)", m.icsEventIdx+1, len(m.icsEvents))
		navBadge := lipgloss.NewStyle().
			Background(lipgloss.Color("#303060")).
			Foreground(lipgloss.Color("#ffffff")).
			Bold(true).
			Render(navText)
		lines = append(lines, "", "  "+navBadge)
	} else {
		lines = append(lines, "")
	}

	if m.icsEvent != nil {
		lines = append(lines, renderICSEventFields(m.icsEvent, labelStyle, valStyle, bg)...)
	}

	// Calendar Status
	calStatusBadge := renderICSCalendarStatus(m, bg)
	lines = append(lines, "", labelStyle.Render("  Calendar:   ")+calStatusBadge)

	if m.icsAddStatus != "" {
		addStatusStyle := lipgloss.NewStyle().Background(bg).Foreground(lipgloss.Color("#88c0d0"))
		lines = append(lines, "              "+addStatusStyle.Render(m.icsAddStatus))
	}

	// Multiple events schedule overview
	if len(m.icsEvents) > 1 {
		metaDivider := divStyle.Render(strings.Repeat("─", max(10, boxW-4)))
		lines = append(lines, "", metaDivider)
		lines = append(lines, labelStyle.Render("  Schedule in this attachment:"))
		for i, e := range m.icsEvents {
			marker := "  "
			itemStyle := valStyle
			if i == m.icsEventIdx {
				marker = "▶ "
				itemStyle = itemStyle.Bold(true).Foreground(lipgloss.Color("#ffd700"))
			}
			timeBrief := e.StartParsed.Format("2006-01-02 15:04")
			if e.IsAllDay {
				timeBrief = e.StartParsed.Format("2006-01-02") + " (All Day)"
			}
			lines = append(lines, rowZone(frameICS, i, 0, itemStyle.Render(fmt.Sprintf("    %s%d. %s — %s", marker, i+1, timeBrief, e.Summary))))
		}
	}

	// Description viewport
	if m.icsEvent != nil && m.icsEvent.Description != "" {
		metaDivider := divStyle.Render(strings.Repeat("─", max(10, boxW-4)))
		lines = append(lines, "", metaDivider)
		lines = append(lines, labelStyle.Render("  Description:"))
		lines = append(lines, m.icsVp.View())
	}

	metaDivider := divStyle.Render(strings.Repeat("─", max(10, boxW-4)))
	lines = append(lines, metaDivider)

	// Action key display footer
	footer := renderICSModalFooter(m, bg, t)
	lines = append(lines, footer)

	content := strings.Join(lines, "\n")
	box := lipgloss.NewStyle().
		Background(bg).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color(borderColor)).
		Padding(0, 1).
		Width(boxW).
		Render(content)

	return renderDimmedOverlay(m.width, m.height, m.renderBaseScreen(), box)
}
