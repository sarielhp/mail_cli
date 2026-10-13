package tui

import (
	"fmt"
	"strings"

	"github.com/sarielhp/mail_cli/calendar"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/tui/render"

	"github.com/charmbracelet/lipgloss"
)

func (m *tuiModel) buildDetailHeaders() string {
	var hdrs strings.Builder
	if m.eIdx >= 0 && m.eIdx < len(m.emails) {
		em := m.emails[m.eIdx]
		if em.IsSpam || em.IsPolitical || em.IsBlacklisted {
			badge := "[SPAM]"
			if em.IsPolitical {
				badge = "[POLI]"
			}
			if em.IsBlacklisted {
				badge = "[BLACKLIST]"
			}
			hdrs.WriteString(render.Plain(badge).WithFg(render.ColorSpam).Render(m.theme.Theme()) + "\n")
		}
	}
	if pill, color := securityPill(m.detailCrypto); pill != "" {
		hdrs.WriteString(render.Plain(pill).WithFg(color).Render(m.theme.Theme()) + "\n")
	}
	if len(m.attachments) > 0 {
		var names []string
		hasICS := false
		for _, a := range m.attachments {
			names = append(names, a.Filename)
			if email.IsICSAttachment(a) {
				hasICS = true
			}
		}
		if !hasICS && m.eIdx >= 0 && m.eIdx < len(m.emails) && m.emails[m.eIdx].HasICS {
			hasICS = true
		}
		prefix := "📎"
		if hasICS {
			prefix = "⏰📎"
		}
		badge := fmt.Sprintf("%s Attachments (%d): %s (Press 'v'/'a' to manage)", prefix, len(m.attachments), strings.Join(names, ", "))
		hdrs.WriteString(render.Plain(badge).WithFg(render.ColorDate).Render(m.theme.Theme()) + "\n")
	}
	for _, line := range strings.Split(m.detailH, "\n") {
		if line == "" {
			continue
		}
		idx := strings.Index(line, ": ")
		if idx > 0 {
			hdrs.WriteString(render.Plain(line[:idx+2]).WithFg(render.ColorHighlightFg).Render(m.theme.Theme()))
			hdrs.WriteString(render.Plain(email.BidiDisplay(line[idx+2:])).WithFg(render.ColorFg).Render(m.theme.Theme()))
		} else {
			hdrs.WriteString(render.Plain(email.BidiDisplay(line)).WithFg(render.ColorFg).Render(m.theme.Theme()))
		}
		hdrs.WriteString("\n")
	}
	return hdrs.String()
}

// buildDetailICSHeader draws one card per event the message carries. An
// invitation from Google or Outlook carries the same event twice -- an inline
// text/calendar part beside the text and HTML bodies, and an attached
// invite.ics -- so events are drawn once each, by identity, not once per part.
func (m *tuiModel) buildDetailICSHeader(vw int) string {
	var icsHeader strings.Builder
	seen := map[string]bool{}
	for _, a := range m.attachments {
		if !email.IsICSAttachment(a) {
			continue
		}
		events, err := calendar.ParseAllICSEventsData(a.Data)
		if err != nil || len(events) == 0 {
			continue
		}
		for _, ev := range events {
			key := icsEventKey(ev)
			if seen[key] {
				continue
			}
			seen[key] = true
			icsHeader.WriteString(renderDetailICSEventCard(m, ev, vw))
		}
	}
	return icsHeader.String()
}

// icsEventKey identifies an event across the copies of one invitation: its
// UID, and the occurrence a RECURRENCE-ID picks out of a series. An event
// without a UID, which RFC 5545 does not allow but senders produce, falls back
// to what a reader would call the same event: its summary and start.
func icsEventKey(ev *calendar.ICSEvent) string {
	if ev.UID != "" {
		return "uid\x00" + ev.UID + "\x00" + ev.RecurrenceID
	}
	return "event\x00" + ev.Summary + "\x00" + ev.StartStr
}

func (m *tuiModel) buildDetailSearchBody(vw int) (string, string) {
	cursorChar := ""
	if m.inDetailSearch {
		cursorChar = "█"
	}
	searchLabel := fmt.Sprintf("\n /%s%s \n", m.detailSearch, cursorChar)
	renderedSearchLabel := render.Plain(searchLabel).
		WithFg(render.ColorHighlightFg).WithBg(render.ColorBg).Render(m.theme.Theme())

	if m.detailSearch == "" {
		return renderedSearchLabel, ""
	}

	var matchedLines []string
	lowerQuery := strings.ToLower(m.detailSearch)
	for _, line := range strings.Split(m.detail, "\n") {
		if strings.Contains(strings.ToLower(line), lowerQuery) {
			matchedLines = append(matchedLines, line)
		}
	}
	if len(matchedLines) == 0 {
		return renderedSearchLabel, render.Plain("(no matches)").WithFg(render.ColorDim).Render(m.theme.Theme())
	}
	result := strings.Join(matchedLines, "\n")
	bodyCS := render.ColorReplyLinesRaw(result, vw)
	return renderedSearchLabel, bodyCS.Render(m.theme.Theme())
}

func (m *tuiModel) buildDetailBodyWithAttachments(prefixLines int, vw int) string {
	origText, quotedText := email.SplitMessageAndQuotes(m.detail)
	origCS := render.ColorReplyLinesWrap(origText, vw)
	origRendered := origCS.Render(m.theme.Theme())

	origLines := len(strings.Split(origRendered, "\n"))
	m.attachmentLineOffset = prefixLines + origLines

	attBlock := m.renderInlineAttachments(vw)
	if quotedText == "" {
		return origRendered + "\n" + attBlock
	}
	quotedCS := render.ColorQuotedReplyLinesRaw(quotedText, vw)
	quotedRendered := quotedCS.Render(m.theme.Theme())
	return origRendered + "\n" + attBlock + "\n" + quotedRendered
}

func (m *tuiModel) buildDetailContent(vw int) string {
	if vw <= 0 {
		vw = 78
	}
	hdrs := m.buildDetailHeaders()
	icsHeader := m.buildDetailICSHeader(vw)

	var bodyText string
	switch {
	case m.inDetailSearch || m.detailSearch != "":
		searchLabel, searchBody := m.buildDetailSearchBody(vw)
		hdrs += searchLabel
		bodyText = searchBody
	case len(m.attachments) > 0:
		prefixLines := len(strings.Split(hdrs, "\n")) + len(strings.Split(icsHeader, "\n"))
		bodyText = m.buildDetailBodyWithAttachments(prefixLines, vw)
	default:
		bodyCS := render.ColorReplyLinesWrap(m.detail, vw)
		bodyText = bodyCS.Render(m.theme.Theme())
	}

	endBar := render.EndOfMsgGreen(vw, m.theme.Theme())
	return hdrs + "\n" + icsHeader + bodyText + "\n\n" + endBar
}

func (m *tuiModel) renderInlineAttachments(vw int) string {
	if len(m.attachments) == 0 {
		return ""
	}
	yellowBold := lipgloss.NewStyle().Foreground(lipgloss.Color("#ffd700")).Bold(true)
	yellowNormal := lipgloss.NewStyle().Foreground(lipgloss.Color("#f1fa8c"))
	activeHighlight := lipgloss.NewStyle().Foreground(lipgloss.Color("#1e1e2e")).Background(lipgloss.Color("#ffd700")).Bold(true)

	divLen := min(vw, 74)
	if divLen <= 0 {
		divLen = 60
	}
	topTitle := fmt.Sprintf("── Attachments (%d) ── [v/a: focus, Enter: open] ", len(m.attachments))
	remLen := max(2, divLen-len([]rune(topTitle)))
	topBar := yellowBold.Render(topTitle + strings.Repeat("─", remLen))
	bottomBar := yellowBold.Render(strings.Repeat("─", divLen))

	var lines []string
	lines = append(lines, "", topBar)
	for i, att := range m.attachments {
		icon := getAttachmentIcon(att)
		itemStr := fmt.Sprintf(" %d. %s%s (%s)", i+1, icon, att.Filename, formatFileSize(len(att.Data)))
		if m.attachmentDrawer && i == m.attCursor {
			lines = append(lines, activeHighlight.Render(" >"+itemStr))
		} else {
			lines = append(lines, yellowNormal.Render("  "+itemStr))
		}
	}
	lines = append(lines, bottomBar, "")
	return strings.Join(lines, "\n")
}

func renderSplitView(m *tuiModel) string {
	width := m.width
	if m.menuOpen {
		width = m.width - 19
	}
	t := m.theme.Theme()

	availH := m.height - 2
	if m.showError {
		availH--
	}
	if availH < 6 {
		availH = 6
	}

	topLines := max(3, (availH-1)/2)
	bottomLines := max(3, availH-topLines-1)

	headerWidth := 4 + 2 + 8 + 2 + 20 + 2 + 2 + 2 + 5 + 2
	subjWidth := width - headerWidth

	start := m.indexStart
	end := start + topLines
	if end > len(m.emails) {
		end = len(m.emails)
	}

	indexRows := m.emailRows(start, end, width, subjWidth)
	for len(indexRows) < topLines {
		indexRows = append(indexRows, "")
	}
	topIndexStr := strings.Join(indexRows, "\n")

	// Divider line: Neomutt style
	var dividerTitle string
	if m.eIdx >= 0 && m.eIdx < len(m.emails) {
		em := m.emails[m.eIdx]
		dividerTitle = fmt.Sprintf("── [Preview: %s] ──", em.Subject)
	} else {
		dividerTitle = "── [Preview] ──"
	}
	if len([]rune(dividerTitle)) > width {
		runes := []rune(dividerTitle)
		dividerTitle = string(runes[:max(4, width-1)])
	}
	divPad := max(0, width-len([]rune(dividerTitle)))
	fullDivider := dividerTitle + strings.Repeat("─", divPad)
	dividerStr := lipgloss.NewStyle().
		Foreground(lipgloss.Color(t.Get(render.ColorBorder))).
		Bold(true).
		Render(fullDivider)

	// Bottom Viewport (Message Preview)
	vw := max(20, width-2)
	vh := bottomLines
	m.refreshDetailViewport(vw, vh)
	bottomPreviewStr := m.vp.View()

	return lipgloss.JoinVertical(lipgloss.Top, topIndexStr, dividerStr, bottomPreviewStr)
}

func renderDetailICSEventCard(m *tuiModel, ev *calendar.ICSEvent, vw int) string {
	t := m.theme.Theme()
	borderColor := t.Get(render.ColorBorder)
	boxW := min(74, max(45, vw-4))

	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#ffd700"))
	labelStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(t.Get(render.ColorHighlightFg)))
	valStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(t.Get(render.ColorFg)))

	var lines []string
	lines = append(lines, headerStyle.Render("📅 CALENDAR EVENT DETAILS"))

	summary := ev.Summary
	if summary == "" {
		summary = "(Untitled Event)"
	}
	lines = append(lines, labelStyle.Render("Title:       ")+valStyle.Bold(true).Render(email.BidiDisplay(summary)))
	lines = append(lines, labelStyle.Render("When:        ")+valStyle.Render(formatICSEventTime(ev)))

	if ev.RRule != "" {
		recurStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#79c0ff")).Bold(true)
		lines = append(lines, labelStyle.Render("Repeats:     ")+recurStyle.Render(formatHumanRRule(ev.RRule)))
		if nextOccs := formatNextOccurrences(ev, 2); len(nextOccs) > 0 {
			lines = append(lines, labelStyle.Render("Next Sched:  ")+valStyle.Render(strings.Join(nextOccs, " | ")))
		}
	}
	if ev.ConferenceURL != "" {
		lines = append(lines, labelStyle.Render("Join:        ")+valStyle.Render(ev.ConferenceURL))
	} else if strings.HasPrefix(ev.Location, "http://") || strings.HasPrefix(ev.Location, "https://") {
		lines = append(lines, labelStyle.Render("Join:        ")+valStyle.Render(ev.Location))
	}
	if ev.Location != "" && !strings.HasPrefix(ev.Location, "http://") && !strings.HasPrefix(ev.Location, "https://") {
		lines = append(lines, labelStyle.Render("Location:    ")+valStyle.Render(email.BidiDisplay(ev.Location)))
	}
	if ev.Organizer != "" {
		lines = append(lines, labelStyle.Render("Organizer:   ")+valStyle.Render(email.BidiDisplay(ev.Organizer)))
	}
	if len(ev.Attendees) > 0 {
		attStr := strings.Join(ev.Attendees, ", ")
		if len(attStr) > 45 {
			attStr = attStr[:45] + "..."
		}
		lines = append(lines, labelStyle.Render("Attendees:   ")+valStyle.Render(email.BidiDisplay(attStr)))
	}

	// Calendar status display
	var calStatus string
	switch m.icsCheckStatus {
	case icsStatusInCalendar:
		calStatus = lipgloss.NewStyle().Foreground(lipgloss.Color("#98c379")).Bold(true).Render("✔ Already in your Google Calendar (Exact match)")
	case icsStatusSimilarFound:
		warn := "⚠ Similar event found in Calendar"
		if m.icsDiscrepancy != "" {
			warn += " (" + m.icsDiscrepancy + ")"
		}
		warn += " — press 'a' → 'u' to update or 'c' to add new"
		calStatus = lipgloss.NewStyle().Foreground(lipgloss.Color("#e5c07b")).Bold(true).Render(warn)
	case icsStatusNotInCalendar:
		calStatus = lipgloss.NewStyle().Foreground(lipgloss.Color("#e06c75")).Bold(true).Render("✗ Not in your Google Calendar (press 'a' → 'c' to add)")
	case icsStatusChecking:
		calStatus = lipgloss.NewStyle().Foreground(lipgloss.Color("#e5c07b")).Render("⏳ Checking Google Calendar...")
	case icsStatusAddedVerified:
		calStatus = lipgloss.NewStyle().Foreground(lipgloss.Color("#98c379")).Bold(true).Render("✓ Added and verified in Calendar")
	case icsStatusUpdatedVerified:
		calStatus = lipgloss.NewStyle().Foreground(lipgloss.Color("#98c379")).Bold(true).Render("✓ Updated and verified in Calendar")
	case icsStatusAddedWithDups:
		calStatus = lipgloss.NewStyle().Foreground(lipgloss.Color("#e5c07b")).Bold(true).Render("⚠ Added (duplicate detected in Calendar)")
	case icsStatusError:
		if m.icsCheckErr != "" {
			calStatus = lipgloss.NewStyle().Foreground(lipgloss.Color("#e06c75")).Render("⚠ " + m.icsCheckErr)
		}
	}
	if calStatus != "" {
		lines = append(lines, labelStyle.Render("Calendar:    ")+calStatus)
	}

	box := lipgloss.NewStyle().
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color(borderColor)).
		Padding(0, 1).
		Width(boxW).
		Render(strings.Join(lines, "\n"))

	return box + "\n\n"
}
