package google

import (
	"fmt"
	"github.com/sarielhp/mail_cli/app"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/report"
	"time"

	"google.golang.org/api/calendar/v3"
)

// dayEventGroup groups calendar events by date string.
type dayEventGroup struct {
	dateStr string
	items   []*calendar.Event
}

// fetchCalendarEvents fetches all events between start and end times.
func fetchCalendarEvents(srv *calendar.Service, start, end time.Time) ([]*calendar.Event, error) {
	list, err := srv.Events.List("primary").
		TimeMin(start.Format(time.RFC3339)).
		TimeMax(end.Format(time.RFC3339)).
		SingleEvents(true).
		OrderBy("startTime").
		Do()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch calendar events: %w", err)
	}
	return list.Items, nil
}

// groupEventsByDay groups calendar events by their date string.
func groupEventsByDay(events []*calendar.Event, loc *time.Location) []dayEventGroup {
	var grouped []dayEventGroup
	for _, ev := range events {
		var evTime time.Time
		if ev.Start.Date != "" {
			evTime, _ = time.ParseInLocation("2006-01-02", ev.Start.Date, loc)
		} else {
			evTime, _ = time.Parse(time.RFC3339, ev.Start.DateTime)
			evTime = evTime.In(loc)
		}
		dateStr := evTime.Format("Monday, 2006-01-02")

		foundIdx := -1
		for i, g := range grouped {
			if g.dateStr == dateStr {
				foundIdx = i
				break
			}
		}

		if foundIdx == -1 {
			grouped = append(grouped, dayEventGroup{
				dateStr: dateStr,
				items:   []*calendar.Event{ev},
			})
		} else {
			grouped[foundIdx].items = append(grouped[foundIdx].items, ev)
		}
	}
	return grouped
}

// printSingleWeekEvent prints a single event in the week view format.
func printSingleWeekEvent(rep report.Reporter, ev *calendar.Event, loc *time.Location) {
	timeStr := ""
	if ev.Start.Date != "" {
		timeStr = app.ColorBoldYellow.Sprint("ALL DAY")
	} else {
		st, err1 := time.Parse(time.RFC3339, ev.Start.DateTime)
		et, err2 := time.Parse(time.RFC3339, ev.End.DateTime)
		if err1 == nil && err2 == nil {
			st = st.In(loc)
			et = et.In(loc)
			timeStr = fmt.Sprintf("%s - %s", st.Format("15:04"), et.Format("15:04"))
		} else {
			timeStr = "??:??"
		}
	}

	summary := ev.Summary
	if summary == "" {
		summary = "(No Title)"
	}

	locStr := ""
	if ev.Location != "" {
		locStr = fmt.Sprintf(" (%s)", ev.Location)
	}

	recurringStr := ""
	if ev.RecurringEventId != "" {
		recurringStr = " [repeats]"
	}

	rep.Printf("    [%s] %s%s%s\n", timeStr, summary, recurringStr, locStr)
}

// printWeekHeader prints the week view header.
func printWeekHeader(rep report.Reporter, start, end time.Time) {
	rep.Printf("%s\n", app.ColorBoldCyan.Sprint("\n======================================================================"))
	rep.Printf("%s", app.ColorBoldCyan.Sprintf("                CALENDAR EVENTS FOR %s to %s              \n", start.Format("2006-01-02"), end.AddDate(0, 0, -1).Format("2006-01-02")))
	rep.Printf("%s\n", app.ColorBoldCyan.Sprint("======================================================================"))
}

// printWeekFooter prints the week view footer.
func printWeekFooter(rep report.Reporter) {
	rep.Printf("%s\n", app.ColorBoldCyan.Sprint("\n======================================================================"))
	rep.Printf("\n")
}

// PerformCalendarWeek displays events for the upcoming days (default 7) in the default calendar.
func (s *Service) PerformCalendarWeek(config *cfg_g.Config, days ...int) error {
	srv, err := s.getCalendarService(config)
	if err != nil {
		return fmt.Errorf("failed to load calendar service: %w", err)
	}

	spanDays := 7
	if len(days) > 0 && days[0] > 0 {
		spanDays = days[0]
	}

	now := time.Now()
	loc := now.Location()
	year, month, day := now.Date()
	start := time.Date(year, month, day, 0, 0, 0, 0, loc)
	end := start.AddDate(0, 0, spanDays)

	events, err := fetchCalendarEvents(srv, start, end)
	if err != nil {
		return err
	}

	printWeekHeader(s.rep, start, end)

	if len(events) == 0 {
		s.rep.Printf("  No events scheduled for the selected period.\n")
	} else {
		grouped := groupEventsByDay(events, loc)
		for _, g := range grouped {
			s.rep.Printf("%s", app.ColorBoldYellow.Sprintf("\n  %s:\n", g.dateStr))
			for _, ev := range g.items {
				printSingleWeekEvent(s.rep, ev, loc)
			}
		}
	}

	printWeekFooter(s.rep)
	return nil
}
