package google

import (
	cal "github.com/sarielhp/mail_cli/calendar"
	"testing"
	"time"
)

func TestParseICSEventData_Full(t *testing.T) {
	icsSample := `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Example Corp.//EN
BEGIN:VEVENT
UID:uid-12345@example.com
SUMMARY:Project Kickoff
DESCRIPTION:Discuss project timeline\nand deliverables.
LOCATION:Meeting Room 3B
ORGANIZER;CN=Alice Boss:mailto:alice@example.com
STATUS:CONFIRMED
ATTENDEE;CN=Bob Developer:mailto:bob@example.com
ATTENDEE;CN=Charlie Designer:mailto:charlie@example.com
DTSTART:20260901T140000Z
DTEND:20260901T150000Z
END:VEVENT
END:VCALENDAR`

	ev, err := cal.ParseICSEventData([]byte(icsSample))
	if err != nil {
		t.Fatalf("ParseICSEventData failed: %v", err)
	}

	if ev.UID != "uid-12345@example.com" {
		t.Errorf("expected UID 'uid-12345@example.com', got %q", ev.UID)
	}
	if ev.Summary != "Project Kickoff" {
		t.Errorf("expected Summary 'Project Kickoff', got %q", ev.Summary)
	}
	if ev.Location != "Meeting Room 3B" {
		t.Errorf("expected Location 'Meeting Room 3B', got %q", ev.Location)
	}
	if ev.Organizer != "Alice Boss <alice@example.com>" {
		t.Errorf("expected Organizer 'Alice Boss <alice@example.com>', got %q", ev.Organizer)
	}
	if ev.Status != "CONFIRMED" {
		t.Errorf("expected Status 'CONFIRMED', got %q", ev.Status)
	}
	if len(ev.Attendees) != 2 {
		t.Fatalf("expected 2 attendees, got %d", len(ev.Attendees))
	}
	if ev.Attendees[0] != "Bob Developer <bob@example.com>" {
		t.Errorf("expected first attendee 'Bob Developer <bob@example.com>', got %q", ev.Attendees[0])
	}
	if ev.IsAllDay {
		t.Errorf("expected IsAllDay=false")
	}

	expectedStart := time.Date(2026, 9, 1, 14, 0, 0, 0, time.UTC)
	if !ev.StartParsed.Equal(expectedStart) {
		t.Errorf("expected start %v, got %v", expectedStart, ev.StartParsed)
	}
}

func TestParseICSEventData_AllDay(t *testing.T) {
	icsSample := `BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
UID:allday-999
SUMMARY:Company Holiday
DTSTART;VALUE=DATE:20261225
DTEND;VALUE=DATE:20261226
END:VEVENT
END:VCALENDAR`

	ev, err := cal.ParseICSEventData([]byte(icsSample))
	if err != nil {
		t.Fatalf("ParseICSEventData failed: %v", err)
	}

	if !ev.IsAllDay {
		t.Errorf("expected IsAllDay=true")
	}
	if ev.Summary != "Company Holiday" {
		t.Errorf("expected Summary 'Company Holiday', got %q", ev.Summary)
	}
}

func TestCheckICSEventInCalendar_InvalidData(t *testing.T) {
	_, _, _, err := (&Service{}).CheckICSEventInCalendar(nil, []byte("INVALID ICS DATA"))
	if err == nil {
		t.Errorf("expected error on invalid ICS data")
	}
}

func TestCheckICSEventInCalendar_NilConfig(t *testing.T) {
	icsSample := `BEGIN:VCALENDAR
BEGIN:VEVENT
UID:test-123
SUMMARY:Test Meeting
DTSTART:20260901T100000Z
END:VEVENT
END:VCALENDAR`

	inCal, summary, ev, err := (&Service{}).CheckICSEventInCalendar(nil, []byte(icsSample))
	if inCal {
		t.Errorf("expected inCal=false with nil config")
	}
	if summary != "Test Meeting" {
		t.Errorf("expected summary 'Test Meeting', got %q", summary)
	}
	if ev == nil {
		t.Errorf("expected ev to be parsed")
	}
	if err == nil {
		t.Errorf("expected error with nil config")
	}
}

func TestAddICSDataWithVerification_InvalidData(t *testing.T) {
	_, _, _, _, err := (&Service{}).AddICSDataWithVerification(nil, []byte("NOT ICS"))
	if err == nil {
		t.Errorf("expected error on invalid ICS")
	}
}

func TestParseAllICSEventsData_MultiEvent(t *testing.T) {
	multiICS := `BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
UID:sess-1
SUMMARY:Session 1: Introduction
DTSTART:20260901T100000Z
DTEND:20260901T110000Z
RRULE:FREQ=WEEKLY;BYDAY=MO,WE,FR;COUNT=6
END:VEVENT
BEGIN:VEVENT
UID:sess-2
SUMMARY:Session 2: Deep Dive
DTSTART:20260908T100000Z
DTEND:20260908T113000Z
RDATE;VALUE=DATE:20260910
EXDATE;VALUE=DATE:20260917
END:VEVENT
END:VCALENDAR`

	events, err := cal.ParseAllICSEventsData([]byte(multiICS))
	if err != nil {
		t.Fatalf("ParseAllICSEventsData failed: %v", err)
	}

	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}

	if events[0].Summary != "Session 1: Introduction" {
		t.Errorf("expected 'Session 1: Introduction', got %q", events[0].Summary)
	}
	if events[0].RRule != "FREQ=WEEKLY;BYDAY=MO,WE,FR;COUNT=6" {
		t.Errorf("expected RRule to be parsed, got %q", events[0].RRule)
	}

	if events[1].Summary != "Session 2: Deep Dive" {
		t.Errorf("expected 'Session 2: Deep Dive', got %q", events[1].Summary)
	}
	if len(events[1].RDates) != 1 || events[1].RDates[0] != "20260910" {
		t.Errorf("expected RDates to be parsed, got %v", events[1].RDates)
	}
	if len(events[1].ExDates) != 1 || events[1].ExDates[0] != "20260917" {
		t.Errorf("expected ExDates to be parsed, got %v", events[1].ExDates)
	}
}

func TestParseICSEventData_Timezone(t *testing.T) {
	icsData := `BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
UID:tz-event-1
SUMMARY:Weekly Sync
DTSTART;TZID=America/Chicago:20260915T110000
DTEND;TZID=America/Chicago:20260915T120000
RRULE:FREQ=WEEKLY;BYDAY=TU
END:VEVENT
END:VCALENDAR`

	ev, err := cal.ParseICSEventData([]byte(icsData))
	if err != nil {
		t.Fatalf("ParseICSEventData failed: %v", err)
	}

	if ev.StartTimeZone != "America/Chicago" {
		t.Errorf("expected StartTimeZone 'America/Chicago', got %q", ev.StartTimeZone)
	}
	if ev.EndTimeZone != "America/Chicago" {
		t.Errorf("expected EndTimeZone 'America/Chicago', got %q", ev.EndTimeZone)
	}
	if ev.RRule != "FREQ=WEEKLY;BYDAY=TU" {
		t.Errorf("expected RRule to be parsed, got %q", ev.RRule)
	}

	gEvent := buildGoogleCalendarEvent(ev)
	if gEvent.Start == nil || gEvent.Start.TimeZone == "" {
		t.Errorf("expected non-empty Start.TimeZone in Google Calendar event")
	}
	if gEvent.Start.TimeZone != "America/Chicago" {
		t.Errorf("expected Start.TimeZone 'America/Chicago', got %q", gEvent.Start.TimeZone)
	}
	if gEvent.End == nil || gEvent.End.TimeZone == "" {
		t.Errorf("expected non-empty End.TimeZone in Google Calendar event")
	}
	if len(gEvent.Recurrence) != 1 || gEvent.Recurrence[0] != "RRULE:FREQ=WEEKLY;BYDAY=TU" {
		t.Errorf("expected Recurrence rule populated, got %v", gEvent.Recurrence)
	}
}

func TestFormatEventTimeSpan(t *testing.T) {
	loc := time.FixedZone("EDT", -4*3600)

	tests := []struct {
		name     string
		startStr string
		endStr   string
		loc      *time.Location
		want     string
	}{
		{
			name:     "Converts UTC to local timezone",
			startStr: "2026-09-18T14:00:00Z",
			endStr:   "2026-09-18T15:30:00Z",
			loc:      loc,
			want:     "10:00 - 11:30",
		},
		{
			name:     "Nil location preserves original timezone",
			startStr: "2026-09-18T14:00:00Z",
			endStr:   "2026-09-18T15:30:00Z",
			loc:      nil,
			want:     "14:00 - 15:30",
		},
		{
			name:     "Invalid start time",
			startStr: "invalid",
			endStr:   "2026-09-18T15:30:00Z",
			loc:      loc,
			want:     "??:??",
		},
		{
			name:     "Invalid end time",
			startStr: "2026-09-18T14:00:00Z",
			endStr:   "invalid",
			loc:      loc,
			want:     "??:??",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatEventTimeSpan(tt.startStr, tt.endStr, tt.loc)
			if got != tt.want {
				t.Errorf("formatEventTimeSpan() = %q, want %q", got, tt.want)
			}
		})
	}
}
