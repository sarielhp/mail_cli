package google

import (
	"fmt"
	"strings"
	"time"

	"google.golang.org/api/calendar/v3"
)

func detectEventDiscrepancies(existing *calendar.Event, ev *ICSEvent) []string {
	if existing == nil || ev == nil {
		return nil
	}
	var diffs []string
	if d := checkStartTimeDiff(existing, ev); d != "" {
		diffs = append(diffs, d)
	}
	if d := checkDurationDiff(existing, ev); d != "" {
		diffs = append(diffs, d)
	}
	if d := checkRecurrenceDiff(existing, ev); d != "" {
		diffs = append(diffs, d)
	}
	return diffs
}

func checkStartTimeDiff(existing *calendar.Event, ev *ICSEvent) string {
	if existing.Start == nil || ev.StartParsed.IsZero() {
		return ""
	}
	existStart := parseCalendarEventTime(existing.Start)
	if existStart.IsZero() {
		return ""
	}
	if ev.IsAllDay {
		if existStart.Format("2006-01-02") != ev.StartParsed.Format("2006-01-02") {
			return fmt.Sprintf("Calendar start date is %s; invite is %s",
				existStart.Format("Jan 02, 2006"), ev.StartParsed.Format("Jan 02, 2006"))
		}
		return ""
	}
	diff := existStart.Sub(ev.StartParsed)
	if diff > time.Minute || diff < -time.Minute {
		return fmt.Sprintf("Calendar start time is %s; invite is %s",
			existStart.Format("15:04 MST"), ev.StartParsed.Format("15:04 MST"))
	}
	return ""
}

func checkDurationDiff(existing *calendar.Event, ev *ICSEvent) string {
	if existing.Start == nil || existing.End == nil || ev.StartParsed.IsZero() || ev.EndParsed.IsZero() || ev.IsAllDay {
		return ""
	}
	existStart := parseCalendarEventTime(existing.Start)
	existEnd := parseCalendarEventTime(existing.End)
	if existStart.IsZero() || existEnd.IsZero() {
		return ""
	}
	existDur := existEnd.Sub(existStart)
	evDur := ev.EndParsed.Sub(ev.StartParsed)
	durDiff := existDur - evDur
	if durDiff > 5*time.Minute || durDiff < -5*time.Minute {
		return fmt.Sprintf("Calendar duration is %s; invite is %s",
			formatICSDuration(existDur), formatICSDuration(evDur))
	}
	return ""
}

func checkRecurrenceDiff(existing *calendar.Event, ev *ICSEvent) string {
	var existRRule string
	for _, r := range existing.Recurrence {
		if strings.HasPrefix(strings.ToUpper(r), "RRULE:") {
			existRRule = strings.TrimPrefix(r, "RRULE:")
			break
		}
	}
	evRRule := strings.TrimPrefix(ev.RRule, "RRULE:")

	existHas := strings.TrimSpace(existRRule) != ""
	evHas := strings.TrimSpace(evRRule) != ""

	if !existHas && evHas {
		return "Calendar event has no recurrence; invite repeats"
	}
	if existHas && !evHas {
		return "Calendar event is recurring; invite has no recurrence"
	}
	if existHas && evHas {
		if normalizeRRule(existRRule) != normalizeRRule(evRRule) {
			return fmt.Sprintf("Recurrence pattern differs (Calendar: %s; invite: %s)",
				existRRule, evRRule)
		}
	}
	return ""
}

func normalizeRRule(r string) string {
	parts := strings.Split(strings.ToUpper(strings.TrimSpace(r)), ";")
	return strings.Join(parts, ";")
}

func parseCalendarEventTime(dt *calendar.EventDateTime) time.Time {
	if dt == nil {
		return time.Time{}
	}
	if dt.DateTime != "" {
		if t, err := time.Parse(time.RFC3339, dt.DateTime); err == nil {
			return t
		}
	}
	if dt.Date != "" {
		if t, err := time.Parse("2006-01-02", dt.Date); err == nil {
			return t
		}
	}
	return time.Time{}
}

func formatICSDuration(d time.Duration) string {
	if d <= 0 {
		return "0m"
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	if h > 0 && m > 0 {
		return fmt.Sprintf("%dh %dm", h, m)
	}
	if h > 0 {
		return fmt.Sprintf("%dh", h)
	}
	return fmt.Sprintf("%dm", m)
}
