package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/sarielhp/mail_cli/calendar"
	"github.com/sarielhp/mail_cli/tui/render"

	"github.com/charmbracelet/lipgloss"
)

func formatRRuleFrequency(freq string, interval int) (string, bool) {
	switch freq {
	case "DAILY":
		if interval == 1 {
			return "Daily", true
		}
		return fmt.Sprintf("Every %d days", interval), true
	case "WEEKLY":
		if interval == 1 {
			return "Weekly", true
		}
		return fmt.Sprintf("Every %d weeks", interval), true
	case "MONTHLY":
		if interval == 1 {
			return "Monthly", true
		}
		return fmt.Sprintf("Every %d months", interval), true
	case "YEARLY":
		if interval == 1 {
			return "Yearly", true
		}
		return fmt.Sprintf("Every %d years", interval), true
	default:
		return "", false
	}
}

func formatRRuleByDay(byDay string) string {
	if byDay == "" {
		return ""
	}
	if byDay == "MO,TU,WE,TH,FR" {
		return " on weekdays"
	}
	if byDay == "SA,SU" {
		return " on weekends"
	}
	dayMap := map[string]string{
		"MO": "Mon", "TU": "Tue", "WE": "Wed", "TH": "Thu", "FR": "Fri", "SA": "Sat", "SU": "Sun",
	}
	var namedDays []string
	for _, d := range strings.Split(byDay, ",") {
		clean := strings.TrimSpace(d)
		if name, ok := dayMap[clean]; ok {
			namedDays = append(namedDays, name)
			continue
		}
		namedDays = append(namedDays, clean)
	}
	return " on " + strings.Join(namedDays, ", ")
}

func formatRRuleLimit(count int, until string) string {
	if count > 0 {
		return fmt.Sprintf(" (%d occurrences)", count)
	}
	if until == "" {
		return ""
	}
	if t, err := time.Parse("20060102T150405Z", until); err == nil {
		return " until " + t.Format("Jan 02, 2006")
	}
	if t, err := time.Parse("20060102", until); err == nil {
		return " until " + t.Format("Jan 02, 2006")
	}
	return ""
}

func formatHumanRRule(raw string) string {
	raw = strings.TrimPrefix(raw, "RRULE:")
	parts := strings.Split(raw, ";")
	params := make(map[string]string)
	for _, p := range parts {
		kv := strings.SplitN(p, "=", 2)
		if len(kv) == 2 {
			params[strings.ToUpper(strings.TrimSpace(kv[0]))] = strings.TrimSpace(kv[1])
		}
	}

	interval, _ := strconv.Atoi(params["INTERVAL"])
	if interval <= 0 {
		interval = 1
	}

	freqStr, ok := formatRRuleFrequency(params["FREQ"], interval)
	if !ok {
		return raw
	}

	freqStr += formatRRuleByDay(params["BYDAY"])

	count, _ := strconv.Atoi(params["COUNT"])
	freqStr += formatRRuleLimit(count, params["UNTIL"])

	return freqStr
}

func formatICSEventTime(ev *calendar.ICSEvent) string {
	if ev.IsAllDay {
		return ev.StartParsed.Format("Monday, Jan 02, 2006") + " (All Day)"
	}
	if ev.StartParsed.Format("2006-01-02") == ev.EndParsed.Format("2006-01-02") {
		dur := ev.EndParsed.Sub(ev.StartParsed)
		durStr := formatICSDuration(dur)
		return fmt.Sprintf("%s • %s - %s (%s)",
			ev.StartParsed.Format("Monday, Jan 02, 2006"),
			ev.StartParsed.Format("15:04"),
			ev.EndParsed.Format("15:04 MST"),
			durStr,
		)
	}
	return fmt.Sprintf("%s - %s",
		ev.StartParsed.Format("Jan 02, 2006 15:04"),
		ev.EndParsed.Format("Jan 02, 2006 15:04 MST"),
	)
}

func formatICSDuration(d time.Duration) string {
	if d <= 0 {
		return "0m"
	}
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	var parts []string
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%dh", hours))
	}
	if minutes > 0 {
		parts = append(parts, fmt.Sprintf("%dm", minutes))
	}
	if len(parts) == 0 {
		return "0m"
	}
	return strings.Join(parts, " ")
}

func formatNextOccurrences(ev *calendar.ICSEvent, count int) []string {
	if ev == nil || ev.RRule == "" || ev.StartParsed.IsZero() {
		return nil
	}
	loc := ev.StartParsed.Location()
	if loc == nil {
		loc = time.Local
	}
	now := time.Now().In(loc)
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	occs := ev.NextOccurrences(todayStart, count)
	if len(occs) == 0 {
		return nil
	}
	var res []string
	dur := ev.EndParsed.Sub(ev.StartParsed)
	for i, occ := range occs {
		if ev.IsAllDay {
			res = append(res, fmt.Sprintf("%d. %s (All Day)", i+1, occ.Format("Mon, Jan 02, 2006")))
			continue
		}
		occEnd := occ.Add(dur)
		res = append(res, fmt.Sprintf("%d. %s • %s - %s",
			i+1,
			occ.Format("Mon, Jan 02, 2006"),
			occ.Format("15:04"),
			occEnd.Format("15:04 MST"),
		))
	}
	return res
}

func renderICSCalendarStatus(m *tuiModel, bg lipgloss.Color) string {
	switch m.icsCheckStatus {
	case icsStatusChecking:
		return lipgloss.NewStyle().Background(bg).Foreground(lipgloss.Color("#e5c07b")).Render("⏳ Checking Google Calendar...")
	case icsStatusInCalendar:
		return lipgloss.NewStyle().Background(bg).Foreground(lipgloss.Color("#98c379")).Bold(true).Render("✔ Already in your Google Calendar (Exact match)")
	case icsStatusSimilarFound:
		warn := "⚠ Similar event found in Google Calendar"
		if m.icsDiscrepancy != "" {
			warn += ": " + m.icsDiscrepancy
		}
		return lipgloss.NewStyle().Background(bg).Foreground(lipgloss.Color("#e5c07b")).Bold(true).Render(warn)
	case icsStatusNotInCalendar:
		return lipgloss.NewStyle().Background(bg).Foreground(lipgloss.Color("#e06c75")).Bold(true).Render("✗ Not in your Google Calendar")
	case icsStatusAdding:
		return lipgloss.NewStyle().Background(bg).Foreground(lipgloss.Color("#61afef")).Render("⏳ Adding event & verifying with Google Calendar...")
	case icsStatusAddedVerified:
		return lipgloss.NewStyle().Background(bg).Foreground(lipgloss.Color("#98c379")).Bold(true).Render("✓ Added and verified in Calendar (no duplicates)")
	case icsStatusAddedWithDups:
		return lipgloss.NewStyle().Background(bg).Foreground(lipgloss.Color("#e5c07b")).Bold(true).Render("⚠ Added, but duplicate entries detected in Calendar")
	case icsStatusUpdatedVerified:
		return lipgloss.NewStyle().Background(bg).Foreground(lipgloss.Color("#98c379")).Bold(true).Render("✓ Updated and verified in Calendar")
	case icsStatusError:
		msg := m.icsCheckErr
		if msg == "" {
			msg = "Calendar check failed"
		}
		return lipgloss.NewStyle().Background(bg).Foreground(lipgloss.Color("#e06c75")).Render("⚠ " + msg)
	case icsStatusNotConfigured:
		return lipgloss.NewStyle().Background(bg).Foreground(lipgloss.Color("#8888aa")).Italic(true).Render("⚠ Google Calendar not configured")
	default:
		return ""
	}
}

func renderICSModalFooter(m *tuiModel, bg lipgloss.Color, t *render.Theme) string {
	dimStyle := lipgloss.NewStyle().Background(bg).Foreground(lipgloss.Color(t.Get(render.ColorDate)))
	keyStyle := lipgloss.NewStyle().Background(bg).Foreground(lipgloss.Color("#ffd700")).Bold(true)
	greenKeyStyle := lipgloss.NewStyle().Background(bg).Foreground(lipgloss.Color("#98c379")).Bold(true)

	var parts []string
	if len(m.icsEvents) > 1 {
		parts = append(parts, keyStyle.Render("[Tab/n] Next event"))
	}

	switch {
	case m.icsCheckStatus == icsStatusSimilarFound:
		parts = append(parts,
			keyStyle.Render("[u / Enter] Update event"),
			keyStyle.Render("[c] Add as new event"),
		)
	case !m.icsInCalendar && m.icsCheckStatus != icsStatusAdding:
		actionText := "[a / c / Enter] Add to Calendar"
		if len(m.icsEvents) > 1 {
			actionText = fmt.Sprintf("[a / c / Enter] Add all %d events", len(m.icsEvents))
		}
		parts = append(parts, keyStyle.Render(actionText))
	case m.icsInCalendar && m.icsCheckStatus != icsStatusAdding:
		parts = append(parts, greenKeyStyle.Render("[✓ In Calendar]"), keyStyle.Render("[u / Enter] Update event"))
	case m.icsCheckStatus == icsStatusAdding:
		parts = append(parts, lipgloss.NewStyle().Background(bg).Foreground(lipgloss.Color("#61afef")).Render("[⏳ Updating...]"))
	}

	parts = append(parts, keyStyle.Render("[s] Save"))
	parts = append(parts, dimStyle.Render("[o] Open in app"))
	if m.icsEvent != nil && m.icsEvent.Description != "" {
		parts = append(parts, dimStyle.Render("[j/k] Scroll"))
	}
	parts = append(parts, dimStyle.Render("[Esc/q] Close"))

	return markHints(fitHints(min(78, max(45, m.width-8))-4, "  |  ", parts),
		hint{"[Tab/n] Next event", "tab"}, hint{"[u / Enter] Update event", "u"}, hint{"[c] Add as new event", "c"},
		hint{"[a / c / Enter] Add", "a"}, hint{"[s] Save", "s"}, hint{"[o] Open in app", "o"}, hint{"[Esc/q] Close", "esc"})
}
