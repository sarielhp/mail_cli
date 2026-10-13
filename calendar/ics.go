package calendar

import (
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"os"
	"strings"
	"time"

	"encoding/base64"

	"github.com/sarielhp/mail_cli/email"

	"github.com/emersion/go-ical"
)

// ICSEvent represents a parsed calendar event from an ICS attachment.
type ICSEvent struct {
	UID           string
	Summary       string
	Description   string
	Location      string
	StartStr      string
	EndStr        string
	StartParsed   time.Time
	EndParsed     time.Time
	StartTimeZone string
	EndTimeZone   string

	// ZoneUnresolved records that DTSTART carried a TZID this system's tzdata
	// does not know, so the wall-clock time was read as UTC because there was
	// nothing better to read it as.
	//
	// The instant in StartParsed is then wrong by whatever that zone's offset
	// is. It matters beyond display: this time is what the Google Calendar
	// integration files the event at, so a silent guess puts a real event in
	// the user's real calendar at the wrong hour. Callers that act on the time
	// say so rather than presenting it as fact.
	ZoneUnresolved bool
	IsAllDay       bool
	Organizer      string
	Status         string
	Attendees      []string
	RRule          string
	RDates         []string
	ExDates        []string
	RecurrenceID   string
	ConferenceURL  string
}

func cleanTZID(tz string) string {
	tz = strings.Trim(tz, "\"")
	tz = strings.TrimPrefix(tz, "/")
	if idx := strings.Index(tz, "zoneinfo/"); idx != -1 {
		tz = tz[idx+len("zoneinfo/"):]
	}
	return tz
}

// DetectSystemTimeZone detects the host system time zone.
func DetectSystemTimeZone() string {
	if tz := os.Getenv("TZ"); tz != "" {
		if _, err := time.LoadLocation(tz); err == nil {
			return tz
		}
	}
	if target, err := os.Readlink("/etc/localtime"); err == nil {
		if idx := strings.Index(target, "zoneinfo/"); idx != -1 {
			tz := target[idx+len("zoneinfo/"):]
			if _, err := time.LoadLocation(tz); err == nil {
				return tz
			}
		}
	}
	if data, err := os.ReadFile("/etc/timezone"); err == nil {
		tz := strings.TrimSpace(string(data))
		if _, err := time.LoadLocation(tz); err == nil {
			return tz
		}
	}
	locStr := time.Now().Location().String()
	if locStr != "" && locStr != "Local" {
		if _, err := time.LoadLocation(locStr); err == nil {
			return locStr
		}
	}
	return "UTC"
}

// parseICSTime parses a trimmed DTSTART/DTEND value into a time, honouring a
// trailing "Z" as UTC and otherwise interpreting the wall clock in zone. It
// reports whether the value parsed; an unresolvable zone or a malformed value
// leaves the caller to try its own fallbacks.
func parseICSTime(trimmedVal, zone string) (time.Time, bool) {
	if strings.HasSuffix(trimmedVal, "Z") {
		t, err := time.Parse("20060102T150405Z", trimmedVal)
		return t, err == nil
	}
	if zone == "" {
		return time.Time{}, false
	}
	loc, err := time.LoadLocation(zone)
	if err != nil {
		return time.Time{}, false
	}
	t, err := time.ParseInLocation("20060102T150405", trimmedVal, loc)
	return t, err == nil
}

func parseICSStartTime(startProp *ical.Prop, ev *ICSEvent, defaultCalTz string) {
	if startProp == nil {
		return
	}
	ev.StartStr = startProp.Value
	startTz := startProp.Params.Get("TZID")
	if startTz == "" {
		startTz = defaultCalTz
	}
	ev.StartTimeZone = cleanTZID(startTz)

	valType := startProp.Params.Get("VALUE")
	trimmedVal := strings.TrimSpace(startProp.Value)
	if strings.EqualFold(valType, "DATE") || len(trimmedVal) == 8 {
		ev.IsAllDay = true
		if t, err := time.Parse("20060102", trimmedVal); err == nil {
			ev.StartParsed = t
		}
		return
	}

	if strings.HasSuffix(trimmedVal, "Z") {
		ev.StartTimeZone = "UTC"
	}
	if t, ok := parseICSTime(trimmedVal, ev.StartTimeZone); ok {
		ev.StartParsed = t
	}
	if ev.StartParsed.IsZero() {
		// A TZID was given and could not be resolved -- everything below reads
		// the wall clock as UTC, which is wrong by that zone's offset.
		if ev.StartTimeZone != "" && !strings.HasSuffix(trimmedVal, "Z") {
			ev.ZoneUnresolved = true
		}
		if t, err := startProp.DateTime(time.UTC); err == nil {
			ev.StartParsed = t
		} else if t, err := time.Parse("20060102T150405Z", trimmedVal); err == nil {
			ev.StartParsed = t
		} else if t, err := time.Parse("20060102T150405", trimmedVal); err == nil {
			ev.StartParsed = t
		}
	}
}

func parseICSEndTime(endProp *ical.Prop, ev *ICSEvent) {
	if endProp == nil {
		ev.EndTimeZone = ev.StartTimeZone
		if ev.IsAllDay {
			ev.EndParsed = ev.StartParsed.AddDate(0, 0, 1)
		} else {
			ev.EndParsed = ev.StartParsed.Add(1 * time.Hour)
		}
		return
	}

	ev.EndStr = endProp.Value
	endTz := endProp.Params.Get("TZID")
	if endTz == "" {
		endTz = ev.StartTimeZone
	}
	ev.EndTimeZone = cleanTZID(endTz)

	trimmedEnd := strings.TrimSpace(endProp.Value)
	if ev.IsAllDay {
		if t, err := time.Parse("20060102", trimmedEnd); err == nil {
			ev.EndParsed = t
		} else {
			ev.EndParsed = ev.StartParsed.AddDate(0, 0, 1)
		}
		return
	}

	if strings.HasSuffix(trimmedEnd, "Z") {
		ev.EndTimeZone = "UTC"
	}
	if t, ok := parseICSTime(trimmedEnd, ev.EndTimeZone); ok {
		ev.EndParsed = t
	}
	if ev.EndParsed.IsZero() {
		if t, err := endProp.DateTime(time.UTC); err == nil {
			ev.EndParsed = t
		} else if t, err := time.Parse("20060102T150405Z", trimmedEnd); err == nil {
			ev.EndParsed = t
		} else if t, err := time.Parse("20060102T150405", trimmedEnd); err == nil {
			ev.EndParsed = t
		} else {
			ev.EndParsed = ev.StartParsed.Add(1 * time.Hour)
		}
	}
}

func parseICSEventTimes(child *ical.Component, ev *ICSEvent, defaultCalTz string) {
	parseICSStartTime(child.Props.Get(ical.PropDateTimeStart), ev, defaultCalTz)
	if ev.StartTimeZone == "" {
		ev.StartTimeZone = DetectSystemTimeZone()
	}
	parseICSEndTime(child.Props.Get(ical.PropDateTimeEnd), ev)
}

func parseICSConferenceURL(child *ical.Component, location string) (string, string) {
	confURL := ""
	for _, propName := range []string{"X-GOOGLE-CONFERENCE", "X-MICROSOFT-SKYPETEAMSMEETINGURL", ical.PropURL} {
		if prop := child.Props.Get(propName); prop != nil && prop.Value != "" {
			confURL = email.StripEscapeSequences(prop.Value)
			break
		}
	}
	if confURL == "" && (strings.HasPrefix(location, "http://") || strings.HasPrefix(location, "https://")) {
		return location, ""
	}
	return confURL, location
}

func formatICSPerson(prop *ical.Prop) string {
	if prop == nil {
		return ""
	}
	cn := email.StripEscapeSequences(prop.Params.Get("CN"))
	mailAddr := email.StripEscapeSequences(strings.TrimPrefix(prop.Value, "mailto:"))
	if cn != "" {
		return fmt.Sprintf("%s <%s>", cn, mailAddr)
	}
	return mailAddr
}

func parseICSParticipants(child *ical.Component, ev *ICSEvent) {
	if orgProp := child.Props.Get(ical.PropOrganizer); orgProp != nil {
		ev.Organizer = formatICSPerson(orgProp)
	}
	for _, attProp := range child.Props[ical.PropAttendee] {
		ev.Attendees = append(ev.Attendees, formatICSPerson(&attProp))
	}
}

func parseICSRecurrence(child *ical.Component, ev *ICSEvent) {
	if prop := child.Props.Get(ical.PropRecurrenceID); prop != nil {
		ev.RecurrenceID = prop.Value
	}
	if prop := child.Props.Get(ical.PropRecurrenceRule); prop != nil {
		ev.RRule = prop.Value
	}
	for _, prop := range child.Props["RDATE"] {
		ev.RDates = append(ev.RDates, prop.Value)
	}
	for _, prop := range child.Props["EXDATE"] {
		ev.ExDates = append(ev.ExDates, prop.Value)
	}
}

// parseSingleICSEvent reads one VEVENT into an ICSEvent.
//
// Summary/Description/Location/Organizer/Attendees are shown verbatim by
// every ICS-rendering view in the TUI, the same way email.Email's
// Subject/From are shown verbatim by the message list and detail views --
// so they get the same email.StripEscapeSequences treatment
// email.ParseReader already applies, or a malicious .ics attachment could
// manipulate the terminal purely through its own property values.
func parseSingleICSEvent(child *ical.Component, defaultCalTz string) *ICSEvent {
	ev := &ICSEvent{}
	if prop := child.Props.Get(ical.PropUID); prop != nil {
		ev.UID = prop.Value
	}
	if prop := child.Props.Get(ical.PropSummary); prop != nil {
		ev.Summary = email.StripEscapeSequences(prop.Value)
	}
	if prop := child.Props.Get(ical.PropDescription); prop != nil {
		ev.Description = email.StripEscapeSequences(prop.Value)
	}
	if prop := child.Props.Get(ical.PropLocation); prop != nil {
		ev.Location = email.StripEscapeSequences(prop.Value)
	}
	ev.ConferenceURL, ev.Location = parseICSConferenceURL(child, ev.Location)
	if prop := child.Props.Get(ical.PropStatus); prop != nil {
		ev.Status = prop.Value
	}

	parseICSRecurrence(child, ev)
	parseICSParticipants(child, ev)
	parseICSEventTimes(child, ev, defaultCalTz)
	if ev.StartParsed.IsZero() {
		return nil
	}
	return ev
}

// ParseAllICSEvents extracts and parses all VEVENT blocks from an ICS string using go-ical.
func ParseAllICSEvents(icsContent string) ([]*ICSEvent, error) {
	dec := ical.NewDecoder(strings.NewReader(icsContent))
	cal, err := dec.Decode()
	if err != nil {
		return nil, fmt.Errorf("failed to decode ICS calendar: %w", err)
	}

	defaultCalTz := ""
	for _, child := range cal.Children {
		if child.Name == ical.CompTimezone {
			if prop := child.Props.Get("TZID"); prop != nil {
				defaultCalTz = cleanTZID(prop.Value)
				break
			}
		}
	}

	var events []*ICSEvent
	for _, child := range cal.Children {
		if child.Name != ical.CompEvent {
			continue
		}
		if ev := parseSingleICSEvent(child, defaultCalTz); ev != nil {
			events = append(events, ev)
		}
	}

	if len(events) == 0 {
		return nil, fmt.Errorf("no VEVENT blocks found in ICS")
	}
	return events, nil
}

// ParseFirstICSEvent extracts and parses the first VEVENT block from an ICS string.
func ParseFirstICSEvent(icsContent string) (*ICSEvent, error) {
	events, err := ParseAllICSEvents(icsContent)
	if err != nil {
		return nil, err
	}
	return events[0], nil
}

// ParseICSEventData parses raw ICS data and returns the first ICSEvent.
func ParseICSEventData(icsData []byte) (*ICSEvent, error) {
	return ParseFirstICSEvent(string(icsData))
}

// ParseAllICSEventsData parses raw ICS data and returns all ICSEvents in the file.
func ParseAllICSEventsData(icsData []byte) ([]*ICSEvent, error) {
	return ParseAllICSEvents(string(icsData))
}

// ExtractICSAttachmentFromMsg parses MIME structure recursively to find an ICS attachment.
func ExtractICSAttachmentFromMsg(header mail.Header, body io.Reader) ([]byte, error) {
	mediaType, params, err := mime.ParseMediaType(header.Get("Content-Type"))
	if err != nil {
		ct := strings.ToLower(header.Get("Content-Type"))
		if strings.Contains(ct, "text/calendar") || strings.Contains(ct, "application/ics") || strings.Contains(ct, ".ics") {
			encoding := header.Get("Content-Transfer-Encoding")
			return DecodePartBodyReader(body, encoding)
		}
		return nil, nil
	}

	if name, ok := params["name"]; ok && strings.HasSuffix(strings.ToLower(name), ".ics") {
		encoding := header.Get("Content-Transfer-Encoding")
		return DecodePartBodyReader(body, encoding)
	}

	cdHeader := header.Get("Content-Disposition")
	if cdHeader != "" {
		cdType, cdParams, err := mime.ParseMediaType(cdHeader)
		if err == nil {
			if cdType == "attachment" {
				if filename, ok := cdParams["filename"]; ok && strings.HasSuffix(strings.ToLower(filename), ".ics") {
					encoding := header.Get("Content-Transfer-Encoding")
					return DecodePartBodyReader(body, encoding)
				}
			}
		}
	}

	if mediaType == "text/calendar" || mediaType == "application/ics" || mediaType == "text/x-vcalendar" {
		encoding := header.Get("Content-Transfer-Encoding")
		return DecodePartBodyReader(body, encoding)
	}

	if strings.HasPrefix(mediaType, "multipart/") {
		boundary := params["boundary"]
		if boundary == "" {
			return nil, nil
		}
		mr := multipart.NewReader(body, boundary)
		for {
			part, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				break
			}
			partHeader := mail.Header(part.Header)
			data, err := ExtractICSAttachmentFromMsg(partHeader, part)
			if err == nil && data != nil {
				return data, nil
			}
		}
	}

	return nil, nil
}

// DecodePartBodyReader handles body decoding based on transfer encoding header.
func DecodePartBodyReader(r io.Reader, encoding string) ([]byte, error) {
	encoding = strings.ToLower(strings.TrimSpace(encoding))
	if encoding == "base64" {
		dec := base64.NewDecoder(base64.StdEncoding, r)
		return io.ReadAll(dec)
	}
	if encoding == "quoted-printable" {
		dec := quotedprintable.NewReader(r)
		return io.ReadAll(dec)
	}
	return io.ReadAll(r)
}
