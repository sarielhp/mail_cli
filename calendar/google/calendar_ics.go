package google

import (
	"fmt"
	"github.com/sarielhp/mail_cli/cfg_g"
	"strings"
	"time"

	"google.golang.org/api/calendar/v3"
)

// CalendarCheckResult details the match status between an ICS event and Google Calendar.
type CalendarCheckResult struct {
	InCalendar  bool
	ExactMatch  bool
	Discrepancy string
	Summary     string
	Event       *ICSEvent
}

// CheckICSEventInCalendar checks if ICS event(s) are already present in Google Calendar.
func (s *Service) CheckICSEventInCalendar(config *cfg_g.Config, icsData []byte) (bool, string, *ICSEvent, error) {
	res, err := s.CheckICSEventDetailed(config, icsData)
	return res.InCalendar && res.ExactMatch, res.Summary, res.Event, err
}

// CheckICSEventDetailed checks whether events are in Google Calendar, whether they match exactly,
// and records any discrepancy (e.g. rescheduled time, duration difference, or changed recurrence).
func (s *Service) CheckICSEventDetailed(config *cfg_g.Config, icsData []byte) (CalendarCheckResult, error) {
	events, err := parseAllICSEvents(string(icsData))
	if err != nil {
		return CalendarCheckResult{}, fmt.Errorf("failed to parse ICS events: %w", err)
	}
	if len(events) == 0 {
		return CalendarCheckResult{}, fmt.Errorf("no calendar events found in ICS")
	}

	primaryEvent := events[0]
	summary := primaryEvent.Summary
	if summary == "" {
		summary = "Untitled Event"
	}
	if len(events) > 1 {
		summary = fmt.Sprintf("%s (+%d more events)", summary, len(events)-1)
	}

	res := CalendarCheckResult{
		Summary: summary,
		Event:   primaryEvent,
	}

	if config == nil {
		return res, fmt.Errorf("configuration not available")
	}

	srv, err := s.getCalendarService(config)
	if err != nil {
		return res, fmt.Errorf("calendar service unavailable: %w", err)
	}

	allFound := true
	allExact := true
	var firstDiscrepancy string

	for _, ev := range events {
		existingItem, count := findExistingGoogleCalendarEvent(srv, ev)
		if count == 0 {
			allFound = false
			allExact = false
			break
		}
		discrepancies := detectEventDiscrepancies(existingItem, ev)
		if len(discrepancies) > 0 {
			allExact = false
			if firstDiscrepancy == "" {
				firstDiscrepancy = strings.Join(discrepancies, "; ")
			}
		}
	}

	res.InCalendar = allFound
	res.ExactMatch = allFound && allExact
	res.Discrepancy = firstDiscrepancy
	return res, nil
}

// buildGoogleCalendarEvent constructs a Google Calendar event struct from an ICSEvent.
func buildGoogleCalendarEvent(ev *ICSEvent) *calendar.Event {
	newEvent := &calendar.Event{
		Summary:     ev.Summary,
		Description: ev.Description,
		Location:    ev.Location,
		ICalUID:     ev.UID,
	}

	tz := ev.StartTimeZone
	if tz == "" {
		tz = detectSystemTimeZone()
	}
	endTz := ev.EndTimeZone
	if endTz == "" {
		endTz = tz
	}

	if ev.IsAllDay {
		newEvent.Start = &calendar.EventDateTime{
			Date:     ev.StartParsed.Format("2006-01-02"),
			TimeZone: tz,
		}
		newEvent.End = &calendar.EventDateTime{
			Date:     ev.EndParsed.Format("2006-01-02"),
			TimeZone: endTz,
		}
	} else {
		newEvent.Start = &calendar.EventDateTime{
			DateTime: ev.StartParsed.Format(time.RFC3339),
			TimeZone: tz,
		}
		newEvent.End = &calendar.EventDateTime{
			DateTime: ev.EndParsed.Format(time.RFC3339),
			TimeZone: endTz,
		}
	}

	var recurrence []string
	if ev.RRule != "" {
		rrule := ev.RRule
		if !strings.HasPrefix(strings.ToUpper(rrule), "RRULE:") {
			rrule = "RRULE:" + rrule
		}
		recurrence = append(recurrence, rrule)
	}
	for _, rd := range ev.RDates {
		rdate := rd
		if !strings.HasPrefix(strings.ToUpper(rdate), "RDATE") {
			rdate = "RDATE:" + rdate
		}
		recurrence = append(recurrence, rdate)
	}
	for _, ex := range ev.ExDates {
		exdate := ex
		if !strings.HasPrefix(strings.ToUpper(exdate), "EXDATE") {
			exdate = "EXDATE:" + exdate
		}
		recurrence = append(recurrence, exdate)
	}
	if len(recurrence) > 0 {
		newEvent.Recurrence = recurrence
	}

	return newEvent
}

func findExistingGoogleCalendarEvent(srv *calendar.Service, ev *ICSEvent) (*calendar.Event, int) {
	if ev.UID != "" {
		list, err := srv.Events.List("primary").ICalUID(ev.UID).Do()
		if err == nil && len(list.Items) > 0 {
			return list.Items[0], len(list.Items)
		}
	}
	if ev.StartParsed.IsZero() {
		return nil, 0
	}

	timeMin := ev.StartParsed.Add(-1 * time.Hour).Format(time.RFC3339)
	timeMax := ev.StartParsed.Add(1 * time.Hour).Format(time.RFC3339)
	list, err := srv.Events.List("primary").
		TimeMin(timeMin).
		TimeMax(timeMax).
		SingleEvents(true).
		Do()
	if err != nil {
		return nil, 0
	}

	var existingItem *calendar.Event
	existingCount := 0
	for _, item := range list.Items {
		if strings.EqualFold(item.Summary, ev.Summary) {
			existingCount++
			if existingItem == nil {
				existingItem = item
			}
		}
	}
	return existingItem, existingCount
}

func insertNewICSEvent(srv *calendar.Service, ev *ICSEvent) error {
	eventObj := buildGoogleCalendarEvent(ev)
	created, err := srv.Events.Insert("primary", eventObj).Do()
	if err != nil {
		return fmt.Errorf("failed to insert event %q: %w", ev.Summary, err)
	}
	if created == nil || created.Id == "" {
		return fmt.Errorf("calendar API returned empty event for %q", ev.Summary)
	}
	fetched, err := srv.Events.Get("primary", created.Id).Do()
	if err != nil || fetched == nil {
		return fmt.Errorf("event %q inserted but failed verification query: %w", ev.Summary, err)
	}
	return nil
}

func saveOrUpdateICSEvent(srv *calendar.Service, ev *ICSEvent, existing *calendar.Event) error {
	if existing == nil {
		return insertNewICSEvent(srv, ev)
	}
	eventObj := buildGoogleCalendarEvent(ev)
	eventObj.Id = existing.Id
	updated, err := srv.Events.Update("primary", existing.Id, eventObj).Do()
	if err != nil {
		return fmt.Errorf("failed to update event %q: %w", ev.Summary, err)
	}
	if updated == nil || updated.Id == "" {
		return fmt.Errorf("calendar API returned empty event after updating %q", ev.Summary)
	}
	fetched, err := srv.Events.Get("primary", updated.Id).Do()
	if err != nil || fetched == nil {
		return fmt.Errorf("event %q updated but failed verification query: %w", ev.Summary, err)
	}
	return nil
}

// AddICSDataAsNew inserts event(s) from ICS data as separate new events without overwriting existing ones.
func (s *Service) AddICSDataAsNew(config *cfg_g.Config, icsData []byte) (string, error) {
	events, err := parseAllICSEvents(string(icsData))
	if err != nil {
		return "", fmt.Errorf("failed to parse ICS events: %w", err)
	}
	if len(events) == 0 {
		return "", fmt.Errorf("no calendar events found in ICS")
	}

	firstSummary := events[0].Summary
	if firstSummary == "" {
		firstSummary = "Untitled Event"
	}
	summary := firstSummary
	if len(events) > 1 {
		summary = fmt.Sprintf("%s (%d events)", firstSummary, len(events))
	}

	srv, err := s.getCalendarService(config)
	if err != nil {
		return summary, fmt.Errorf("failed to load calendar service: %w", err)
	}

	for _, ev := range events {
		if err := insertNewICSEvent(srv, ev); err != nil {
			return summary, err
		}
	}
	return summary, nil
}

// AddICSDataWithVerification inserts or updates event(s) from ICS data, verifies their addition/update, and verifies there are no duplicates.
func (s *Service) AddICSDataWithVerification(config *cfg_g.Config, icsData []byte) (summary string, existed bool, verified bool, noDuplicates bool, err error) {
	events, err := parseAllICSEvents(string(icsData))
	if err != nil {
		return "", false, false, false, fmt.Errorf("failed to parse ICS events: %w", err)
	}
	if len(events) == 0 {
		return "", false, false, false, fmt.Errorf("no calendar events found in ICS")
	}

	firstSummary := events[0].Summary
	if firstSummary == "" {
		firstSummary = "Untitled Event"
	}
	summary = firstSummary
	if len(events) > 1 {
		summary = fmt.Sprintf("%s (%d events)", firstSummary, len(events))
	}

	srv, err := s.getCalendarService(config)
	if err != nil {
		return summary, false, false, false, fmt.Errorf("failed to load calendar service: %w", err)
	}

	allAlreadyExisted := true
	anyDuplicates := false

	for _, ev := range events {
		existingItem, existingCount := findExistingGoogleCalendarEvent(srv, ev)
		if existingCount > 1 {
			anyDuplicates = true
		}
		if existingItem == nil {
			allAlreadyExisted = false
		}
		if err := saveOrUpdateICSEvent(srv, ev, existingItem); err != nil {
			return summary, existingItem != nil, false, false, err
		}
	}

	return summary, allAlreadyExisted, true, !anyDuplicates, nil
}
