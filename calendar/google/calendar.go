package google

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/sarielhp/mail_cli/app"
	"github.com/sarielhp/mail_cli/cache/msg"
	cal "github.com/sarielhp/mail_cli/calendar"
	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/label"
	"github.com/sarielhp/mail_cli/mailclient"
	"github.com/sarielhp/mail_cli/report"
	"net/http"
	"net/mail"
	"time"

	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"
)

// ICSEvent represents a parsed calendar event from an ICS attachment.
type ICSEvent = cal.ICSEvent

// Service performs Google Calendar operations.
//
// NewHTTPClient supplies an OAuth-authorized HTTP client for whichever account
// the config currently selects. It is injected rather than implemented here so
// that this package does not depend on backend/gmail: the Google OAuth plumbing
// lives there, beside the Gmail client that shares the same token and scopes.
type Service struct {
	NewHTTPClient func(config *cfg_g.Config) (*http.Client, error)

	// rep is where the service says what it is doing. Adding an event prints a
	// summary and the day's schedule, which the CLI shows and the TUI turns
	// into messages.
	rep report.Reporter
}

// New builds a Service from an OAuth client provider. A nil rep discards.
func New(newHTTPClient func(config *cfg_g.Config) (*http.Client, error), rep report.Reporter) *Service {
	return &Service{NewHTTPClient: newHTTPClient, rep: report.Or(rep)}
}

var (
	parseAllICSEvents           = cal.ParseAllICSEvents
	extractICSAttachmentFromMsg = cal.ExtractICSAttachmentFromMsg
	detectSystemTimeZone        = cal.DetectSystemTimeZone
)

// findCalendarManagerAccount finds the configured account designated as calendar manager.
func findCalendarManagerAccount(config *cfg_g.Config) (*cfg_acc.AccountConfig, error) {
	for i, acc := range config.Accounts {
		if acc.CalendarManager {
			return &config.Accounts[i], nil
		}
	}
	return nil, fmt.Errorf("no calendar manager account designated. Use 'mail_cli account calendar <name>' to designate one")
}

// getCalendarService returns the authenticated Google Calendar service.
func (s *Service) getCalendarService(config *cfg_g.Config) (*calendar.Service, error) {
	if s == nil || s.NewHTTPClient == nil {
		return nil, errors.New("calendar service was built without an OAuth client provider")
	}

	calAcc, err := findCalendarManagerAccount(config)
	if err != nil {
		return nil, err
	}

	// The OAuth token is stored per account and the provider locates it from the
	// selected account, so the calendar manager has to be the selected account
	// for this call. A copy says so without disturbing the caller's config.
	client, err := s.NewHTTPClient(cfg_g.WithAccount(config, *calAcc))
	if err != nil {
		return nil, err
	}

	srv, err := calendar.NewService(context.Background(), option.WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve Calendar client: %w", err)
	}

	return srv, nil
}

// checkAndAddEvent verifies if event exists, adds or updates it, and returns status.
func checkAndAddEvent(srv *calendar.Service, event *ICSEvent) (*calendar.Event, bool, error) {
	existing, _ := findExistingGoogleCalendarEvent(srv, event)
	if existing != nil {
		updated := buildGoogleCalendarEvent(event)
		updated.Id = existing.Id
		res, err := srv.Events.Update("primary", existing.Id, updated).Do()
		if err != nil {
			return nil, true, fmt.Errorf("failed to update existing event %q: %w", event.Summary, err)
		}
		return res, true, nil
	}

	newEvent := buildGoogleCalendarEvent(event)
	created, err := srv.Events.Insert("primary", newEvent).Do()
	if err != nil {
		return nil, false, err
	}

	return created, false, nil
}

// listEventsForDay lists all events for the date of the given time.
func listEventsForDay(srv *calendar.Service, t time.Time) ([]*calendar.Event, error) {
	loc := t.Location()
	year, month, day := t.Date()
	dayStart := time.Date(year, month, day, 0, 0, 0, 0, loc)
	dayEnd := dayStart.AddDate(0, 0, 1)

	list, err := srv.Events.List("primary").
		TimeMin(dayStart.Format(time.RFC3339)).
		TimeMax(dayEnd.Format(time.RFC3339)).
		SingleEvents(true).
		OrderBy("startTime").
		Do()
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

// formatEventTimeSpan formats the start and end RFC3339 timestamps in the given location.
func formatEventTimeSpan(startStr, endStr string, loc *time.Location) string {
	st, err1 := time.Parse(time.RFC3339, startStr)
	et, err2 := time.Parse(time.RFC3339, endStr)
	if err1 != nil || err2 != nil {
		return "??:??"
	}
	if loc != nil {
		st = st.In(loc)
		et = et.In(loc)
	}
	return fmt.Sprintf("%s - %s", st.Format("15:04"), et.Format("15:04"))
}

// printDayEvents formats and outputs calendar events to stdout.
func printDayEvents(rep report.Reporter, events []*calendar.Event, dayStart time.Time, targetEvent *calendar.Event) {
	rep.Printf("%s\n", app.ColorBoldCyan.Sprint("\n======================================================================"))
	rep.Printf("%s", app.ColorBoldCyan.Sprintf("                     CALENDAR SCHEDULE FOR %s                  \n", dayStart.Format("2006-01-02")))
	rep.Printf("%s\n", app.ColorBoldCyan.Sprint("======================================================================"))

	if len(events) == 0 {
		rep.Printf("  No events scheduled for this day.\n")
	} else {
		for _, ev := range events {
			timeStr := ""
			if ev.Start.Date != "" {
				timeStr = app.ColorBoldYellow.Sprint("ALL DAY")
			} else {
				timeStr = formatEventTimeSpan(ev.Start.DateTime, ev.End.DateTime, dayStart.Location())
			}

			summary := ev.Summary
			if summary == "" {
				summary = "(No Title)"
			}

			if targetEvent != nil && (ev.Id == targetEvent.Id || (ev.ICalUID != "" && ev.ICalUID == targetEvent.ICalUID)) {
				summary = app.ColorBoldGreen.Sprint(summary) + " " + app.ColorBoldGreen.Sprint("[TARGET]")
			}

			locStr := ""
			if ev.Location != "" {
				locStr = fmt.Sprintf(" (%s)", ev.Location)
			}

			rep.Printf("  [%s] %s%s\n", timeStr, summary, locStr)
		}
	}
	rep.Printf("%s\n", app.ColorBoldCyan.Sprint("======================================================================"))
	rep.Printf("\n")
}

// printEventSummary prints an ICS event's summary, start time, and location to stdout.
func printEventSummary(rep report.Reporter, event *ICSEvent) {
	rep.Printf("%s", app.ColorBoldYellow.Sprintf("  Summary:     "))
	rep.Printf("%v\n", event.Summary)
	rep.Printf("%s", app.ColorBoldYellow.Sprintf("  Start:       "))
	if event.IsAllDay {
		rep.Printf("%v\n", event.StartParsed.Format("2006-01-02")+" (All Day)")
	} else {
		rep.Printf("%v\n", event.StartParsed.Format("2006-01-02 15:04:05 MST"))
	}
	// An unknown TZID means the wall-clock time was read as UTC for want of
	// anything better, so this instant is wrong by that zone's offset -- and it
	// is the instant the event is filed at. Saying so beats showing a guess in
	// the same typeface as a fact.
	if event.ZoneUnresolved {
		rep.Warnf("  The invitation names time zone %q, which this system does not know. "+
			"The time above was read as UTC and may be off by that zone's offset.\n", event.StartTimeZone)
	}
	if event.Location != "" {
		rep.Printf("%s", app.ColorBoldYellow.Sprintf("  Location:    "))
		rep.Printf("%v\n", event.Location)
	}
	if event.RRule != "" {
		rep.Printf("%s", app.ColorBoldYellow.Sprintf("  Repeats:     "))
		rep.Printf("%v\n", event.RRule)
	}
}

// findICSDataForMessage searches for an ICS attachment in a specific message across labels.
func findICSDataForMessage(client mailclient.Reader, localCfg *cfg_g.Config, matchedLabels []string, targetMsgID string) ([]byte, string, error) {
	for _, matchedLabel := range matchedLabels {
		cacheDirName := cfg_g.SanitizeLabelForCache(matchedLabel)

		downloadedIDs, err := client.FetchAndDownloadEmails(label.FromServer(matchedLabel), cacheDirName)
		if err != nil {
			return nil, "", fmt.Errorf("error downloading emails for label %s: %w", matchedLabel, err)
		}

		var matchedID email.ID
		for _, id := range downloadedIDs {
			if email.IDMatches(id, targetMsgID) {
				matchedID = id
				break
			}
		}

		if matchedID != "" {
			rawBytes, rErr := msg.Read(localCfg.AccountCache, email.ID(matchedID))
			if rErr != nil {
				continue
			}
			m, rErr := mail.ReadMessage(bytes.NewReader(rawBytes))
			if rErr != nil {
				continue
			}
			data, err := extractICSAttachmentFromMsg(m.Header, m.Body)
			if err != nil {
				return nil, "", fmt.Errorf("failed to extract ICS attachment from %s: %w", matchedID, err)
			}
			if data != nil {
				return data, matchedLabel, nil
			}
		}
	}
	return nil, "", fmt.Errorf("could not find any .ics attachment in message %s within labels matching %q", targetMsgID, matchedLabels)
}

// PerformCalendarAdd extracts an .ics file from a message and inserts it into the designated calendar manager's calendar.
func (s *Service) PerformCalendarAdd(config *cfg_g.Config, client mailclient.Reader, labelPrefix string, targetMsgID string) error {
	localCfg := client.Config()

	matchedLabels, err := client.GetMatchingLabels(labelPrefix)
	if err != nil {
		return err
	}
	if len(matchedLabels) == 0 {
		return fmt.Errorf("no labels found matching prefix %q", labelPrefix)
	}

	icsData, foundLabel, err := findICSDataForMessage(client, localCfg, matchedLabels, targetMsgID)
	if err != nil {
		return err
	}

	event, err := cal.ParseFirstICSEvent(string(icsData))
	if err != nil {
		return fmt.Errorf("failed to parse ICS event: %w", err)
	}

	s.rep.Infof("Found calendar event in message %s (folder %s):\n", targetMsgID, foundLabel)
	printEventSummary(s.rep, event)

	srv, err := s.getCalendarService(config)
	if err != nil {
		return fmt.Errorf("failed to load calendar service: %w", err)
	}

	calEvent, existed, err := checkAndAddEvent(srv, event)
	if err != nil {
		return fmt.Errorf("failed to process calendar event check/addition: %w", err)
	}

	if existed {
		s.rep.Successf("Successfully updated event in your calendar!\n")
	} else {
		s.rep.Successf("Successfully added event to your calendar!\n")
	}

	dayEvents, err := listEventsForDay(srv, event.StartParsed)
	if err != nil {
		return fmt.Errorf("failed to fetch schedule list for the event day: %w", err)
	}

	printDayEvents(s.rep, dayEvents, event.StartParsed, calEvent)
	return nil
}

// AddICSDataToCalendar parses an ICS payload and inserts the event into the designated calendar manager's calendar.
func (s *Service) AddICSDataToCalendar(config *cfg_g.Config, icsData []byte) (string, bool, error) {
	event, err := cal.ParseFirstICSEvent(string(icsData))
	if err != nil {
		return "", false, fmt.Errorf("failed to parse ICS event: %w", err)
	}

	srv, err := s.getCalendarService(config)
	if err != nil {
		return "", false, fmt.Errorf("failed to load calendar service: %w", err)
	}

	_, existed, err := checkAndAddEvent(srv, event)
	if err != nil {
		return "", false, fmt.Errorf("failed to process calendar event check/addition: %w", err)
	}

	summary := event.Summary
	if summary == "" {
		summary = "Untitled Event"
	}
	return summary, existed, nil
}

// processCalendarMessage processes a single cached message for calendar events.
func processCalendarMessage(rep report.Reporter, srv *calendar.Service, _ *cfg_g.Config, localCfg *cfg_g.Config, id email.ID) (processed bool, added bool, updated bool) {
	rawBytes, rErr := msg.Read(localCfg.AccountCache, id)
	if rErr != nil {
		return false, false, false
	}
	m, rErr := mail.ReadMessage(bytes.NewReader(rawBytes))
	if rErr != nil {
		return false, false, false
	}
	icsData, err := extractICSAttachmentFromMsg(m.Header, m.Body)
	if err != nil || len(icsData) == 0 {
		rep.Errorf("[Message %s] Failed to extract ICS: %v\n", email.ComputeShortID(id), err)
		return false, false, false
	}

	event, err := cal.ParseFirstICSEvent(string(icsData))
	if err != nil {
		rep.Errorf("[Message %s] Failed to parse ICS: %v\n", email.ComputeShortID(id), err)
		return false, false, false
	}

	rep.Infof("Found calendar event in message %s:\n", email.ComputeShortID(id))
	printEventSummary(rep, event)

	calEvent, existed, err := checkAndAddEvent(srv, event)
	if err != nil {
		rep.Errorf("[Message %s] Failed to process calendar: %v\n", email.ComputeShortID(id), err)
		return true, false, false
	}

	if existed {
		rep.Successf("Successfully updated event in your calendar!\n")
	} else {
		rep.Successf("Successfully added event to your calendar!\n")
	}

	dayEvents, err := listEventsForDay(srv, event.StartParsed)
	if err == nil {
		printDayEvents(rep, dayEvents, event.StartParsed, calEvent)
	} else {
		rep.Errorf("Failed to fetch daily schedule: %v\n", err)
	}

	return true, !existed, existed
}

// PerformCalAddAll scans the inbox for messages with .ics attachments and adds them if not already present.
func (s *Service) PerformCalAddAll(config *cfg_g.Config, client mailclient.Reader) error {
	localCfg := client.Config()

	matchedLabels, err := client.GetMatchingLabels(client.InboxFolder().String())
	if err != nil {
		return err
	}
	if len(matchedLabels) == 0 {
		return fmt.Errorf("no inbox label found")
	}

	srv, err := s.getCalendarService(config)
	if err != nil {
		return fmt.Errorf("failed to load calendar service: %w", err)
	}

	processedCount := 0
	addedCount := 0
	updatedCount := 0

	for _, matchedLabel := range matchedLabels {
		cacheDirName := cfg_g.SanitizeLabelForCache(matchedLabel)

		downloadedIDs, err := client.FetchAndDownloadEmails(label.FromServer(matchedLabel), cacheDirName)
		if err != nil {
			return fmt.Errorf("error downloading emails for label %s: %w", matchedLabel, err)
		}

		s.rep.Infof("Scanning label %q (%d messages)...\n", matchedLabel, len(downloadedIDs))

		for _, id := range downloadedIDs {
			processed, added, updated := processCalendarMessage(s.rep, srv, config, localCfg, id)
			if !processed {
				continue
			}
			processedCount++
			if added {
				addedCount++
			}
			if updated {
				updatedCount++
			}
		}
	}

	s.rep.Successf("Scan complete. Processed %d event(s). Added: %d, Updated: %d.\n", processedCount, addedCount, updatedCount)
	return nil
}
