package calendar

import (
	"testing"
	"time"
)

func TestParseFirstICSEvent_Folded(t *testing.T) {
	folded := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nUID:fold-1\r\nSUMMARY:This is a very lo\r\n ng summary\r\nDTSTART:20231024T170000Z\r\nEND:VEVENT\r\nEND:VCALENDAR"
	event, err := ParseFirstICSEvent(folded)
	if err != nil {
		t.Fatalf("ParseFirstICSEvent failed: %v", err)
	}
	want := "This is a very long summary"
	if event.Summary != want {
		t.Errorf("event.Summary = %q, want %q", event.Summary, want)
	}
}

func TestParseFirstICSEvent_AllDay(t *testing.T) {
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nUID:ad-1\r\nSUMMARY:All Day Event\r\nDTSTART;VALUE=DATE:20231024\r\nEND:VEVENT\r\nEND:VCALENDAR"
	event, err := ParseFirstICSEvent(ics)
	if err != nil {
		t.Fatalf("ParseFirstICSEvent failed: %v", err)
	}
	if !event.IsAllDay {
		t.Errorf("expected IsAllDay=true")
	}
	wantT := time.Date(2023, 10, 24, 0, 0, 0, 0, time.UTC)
	if !event.StartParsed.Equal(wantT) {
		t.Errorf("StartParsed = %v, want %v", event.StartParsed, wantT)
	}
}

func TestParseFirstICSEvent(t *testing.T) {
	ics := `BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
UID:uid-1234
SUMMARY:Bastille Day Party
DESCRIPTION:Party time!
LOCATION:Paris
DTSTART:20230714T170000Z
DTEND:20230715T040000Z
END:VEVENT
END:VCALENDAR`

	event, err := ParseFirstICSEvent(ics)
	if err != nil {
		t.Fatalf("ParseFirstICSEvent failed: %v", err)
	}

	if event.UID != "uid-1234" {
		t.Errorf("got UID %q, want %q", event.UID, "uid-1234")
	}
	if event.Summary != "Bastille Day Party" {
		t.Errorf("got Summary %q, want %q", event.Summary, "Bastille Day Party")
	}
	if event.Description != "Party time!" {
		t.Errorf("got Description %q, want %q", event.Description, "Party time!")
	}
	if event.Location != "Paris" {
		t.Errorf("got Location %q, want %q", event.Location, "Paris")
	}
	if !event.StartParsed.Equal(time.Date(2023, 7, 14, 17, 0, 0, 0, time.UTC)) {
		t.Errorf("got Start %v, want %v", event.StartParsed, time.Date(2023, 7, 14, 17, 0, 0, 0, time.UTC))
	}
}

// A DTSTART carrying a TZID this system's tzdata does not know is read as UTC,
// because there is nothing better to read it as. The resulting instant is wrong
// by that zone's offset -- and it is the instant the Google Calendar
// integration files the event at, so the guess has to be visible rather than
// silent.
func TestUnknownTimeZoneIsRecordedRatherThanHidden(t *testing.T) {
	tests := []struct {
		name           string
		tzid           string
		value          string
		wantUnresolved bool
	}{
		{
			name:           "a zone tzdata does not have",
			tzid:           "Customer/Invented-Zone",
			value:          "20260701T090000",
			wantUnresolved: true,
		},
		{
			name:           "a real IANA zone resolves",
			tzid:           "America/Chicago",
			value:          "20260701T090000",
			wantUnresolved: false,
		},
		{
			name:           "an explicit UTC time needs no zone",
			tzid:           "",
			value:          "20260701T140000Z",
			wantUnresolved: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			events, err := ParseAllICSEventsData([]byte(icsWithStart(tc.tzid, tc.value)))
			if err != nil || len(events) != 1 {
				t.Fatalf("parsing: %v (%d events)", err, len(events))
			}
			if got := events[0].ZoneUnresolved; got != tc.wantUnresolved {
				t.Errorf("ZoneUnresolved = %v, want %v (start parsed as %v)",
					got, tc.wantUnresolved, events[0].StartParsed)
			}
		})
	}
}

func icsWithStart(tzid, value string) string {
	start := "DTSTART:" + value
	if tzid != "" {
		start = "DTSTART;TZID=" + tzid + ":" + value
	}
	return "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//test//EN\r\nBEGIN:VEVENT\r\n" +
		"UID:t1\r\nSUMMARY:Test\r\n" + start + "\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
}

// TestParseFirstICSEvent_StripsEscapeSequences is a security-review finding:
// SUMMARY, DESCRIPTION, LOCATION, and the ORGANIZER display name (CN
// parameter) are shown verbatim in the TUI's ICS event views
// (tui_attachment_modal.go, tui_attachment_graphic.go, tui_views_split.go,
// tui_ics.go), with no escape-sequence stripping anywhere in this parser --
// unlike email.ParseReader, which strips Subject/From for exactly this
// reason. A malicious .ics attachment could embed ANSI/OSC escape
// sequences in any of these fields to manipulate the terminal the same way
// a malicious email Subject could.
func TestParseFirstICSEvent_StripsEscapeSequences(t *testing.T) {
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\n" +
		"UID:uid-evil\r\n" +
		"SUMMARY:Evil\x1b[2JSummary\r\n" +
		"DESCRIPTION:Evil\x1b]0;pwned\x07Description\r\n" +
		"LOCATION:Evil\x1b[31mLocation\r\n" +
		"ORGANIZER;CN=Evil\x1b[2JOrganizer:mailto:evil@example.com\r\n" +
		"DTSTART:20231024T170000Z\r\n" +
		"END:VEVENT\r\nEND:VCALENDAR"

	event, err := ParseFirstICSEvent(ics)
	if err != nil {
		t.Fatalf("ParseFirstICSEvent failed: %v", err)
	}
	for name, val := range map[string]string{
		"Summary":     event.Summary,
		"Description": event.Description,
		"Location":    event.Location,
		"Organizer":   event.Organizer,
	} {
		if containsEscapeOrControlByte(val) {
			t.Errorf("event.%s still contains an escape/control byte: %q", name, val)
		}
	}
}

func containsEscapeOrControlByte(s string) bool {
	for _, r := range s {
		if r == 0x1b || r == 0x07 {
			return true
		}
	}
	return false
}
