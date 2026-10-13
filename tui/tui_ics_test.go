package tui

import (
	"bytes"
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"strings"
	"testing"
	"time"

	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/mailclient"

	tea "github.com/charmbracelet/bubbletea"
)

const sampleICSData = `BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
UID:meet-12345@example.com
SUMMARY:Architecture Design Review
DESCRIPTION:Review new service architecture and database migration plans.
LOCATION:Building 4 Room 201
ORGANIZER;CN=Lead Architect:mailto:architect@example.com
STATUS:CONFIRMED
DTSTART:20260915T140000Z
DTEND:20260915T153000Z
END:VEVENT
END:VCALENDAR`

func TestTUI_ICS_OpenViewer_OnEnter(t *testing.T) {
	mockClient := &mailclient.MockMailClient{Cfg: &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: t.TempDir()}, AccountCache: cachedir.At(t.TempDir())}}
	m := newTuiModel(mockClient, "INBOX", nil)
	m.width = 90
	m.height = 30
	m.attachments = []email.Attachment{
		{Filename: "design_review.ics", ContentType: "text/calendar", Data: []byte(sampleICSData)},
	}
	m.attachmentDrawer = true
	m.attCursor = 0

	// Press Enter on the ICS attachment -> opens the ICS modal popup
	res, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	tm := res.(*tuiModel)

	if !tm.icsModalOpen {
		t.Fatalf("expected icsModalOpen=true after Enter on ICS attachment")
	}
	if cmd == nil {
		t.Fatalf("expected background calendar check cmd to be returned")
	}
	if tm.icsEvent == nil {
		t.Fatalf("expected icsEvent to be parsed")
	}
	if tm.icsEvent.Summary != "Architecture Design Review" {
		t.Errorf("expected summary 'Architecture Design Review', got %q", tm.icsEvent.Summary)
	}

	viewStr := tm.View()
	if !strings.Contains(viewStr, "CALENDAR EVENT (.ics)") {
		t.Errorf("expected view to contain 'CALENDAR EVENT (.ics)', got:\n%s", viewStr)
	}
	if !strings.Contains(viewStr, "Architecture Design Review") {
		t.Errorf("expected view to contain 'Architecture Design Review', got:\n%s", viewStr)
	}
	if !strings.Contains(viewStr, "Building 4 Room 201") {
		t.Errorf("expected view to contain Location, got:\n%s", viewStr)
	}
	if !strings.Contains(viewStr, "Lead Architect") {
		t.Errorf("expected view to contain Organizer, got:\n%s", viewStr)
	}
	if !strings.Contains(viewStr, "Checking Google Calendar") {
		t.Errorf("expected view to show checking status initially, got:\n%s", viewStr)
	}
}

func TestTUI_ICS_OpenViewer_OnKeyV(t *testing.T) {
	mockClient := &mailclient.MockMailClient{Cfg: &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: t.TempDir()}, AccountCache: cachedir.At(t.TempDir())}}
	m := newTuiModel(mockClient, "INBOX", nil)
	m.width = 90
	m.height = 30
	m.attachments = []email.Attachment{
		{Filename: "invite.ics", ContentType: "text/calendar", Data: []byte(sampleICSData)},
	}
	m.attachmentDrawer = true
	m.attCursor = 0

	// Press 'v' to view attachment
	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")})
	tm := res.(*tuiModel)

	if !tm.icsModalOpen {
		t.Fatalf("expected icsModalOpen=true after 'v' on ICS attachment")
	}
}

func TestTUI_ICS_CalendarCheckMsg_InCalendar(t *testing.T) {
	mockClient := &mailclient.MockMailClient{Cfg: &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: t.TempDir()}, AccountCache: cachedir.At(t.TempDir())}}
	m := newTuiModel(mockClient, "INBOX", nil)
	m.width = 90
	m.height = 30
	m.openICSViewer(email.Attachment{Filename: "invite.ics", Data: []byte(sampleICSData)})

	// Simulate calendar check message indicating event already exists
	res, _ := m.Update(icsCalCheckMsg{
		inCalendar: true,
		summary:    "Architecture Design Review",
		event:      m.icsEvent,
	})
	tm := res.(*tuiModel)

	if !tm.icsInCalendar {
		t.Errorf("expected icsInCalendar=true")
	}
	if tm.icsCheckStatus != icsStatusInCalendar {
		t.Errorf("expected icsStatusInCalendar, got %v", tm.icsCheckStatus)
	}

	viewStr := tm.View()
	if !strings.Contains(viewStr, "Already in your Google Calendar") {
		t.Errorf("expected view to contain 'Already in your Google Calendar', got:\n%s", viewStr)
	}
	if !strings.Contains(viewStr, "[✓ In Calendar]") {
		t.Errorf("expected view to contain '[✓ In Calendar]' in footer, got:\n%s", viewStr)
	}
}

func TestTUI_ICS_CalendarCheckMsg_NotInCalendar(t *testing.T) {
	mockClient := &mailclient.MockMailClient{Cfg: &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: t.TempDir()}, AccountCache: cachedir.At(t.TempDir())}}
	m := newTuiModel(mockClient, "INBOX", nil)
	m.width = 90
	m.height = 30
	m.openICSViewer(email.Attachment{Filename: "invite.ics", Data: []byte(sampleICSData)})

	// Simulate calendar check message indicating event NOT in calendar
	res, _ := m.Update(icsCalCheckMsg{
		inCalendar: false,
		summary:    "Architecture Design Review",
		event:      m.icsEvent,
	})
	tm := res.(*tuiModel)

	if tm.icsInCalendar {
		t.Errorf("expected icsInCalendar=false")
	}
	if tm.icsCheckStatus != icsStatusNotInCalendar {
		t.Errorf("expected icsStatusNotInCalendar, got %v", tm.icsCheckStatus)
	}

	viewStr := tm.View()
	if !strings.Contains(viewStr, "Not in your Google Calendar") {
		t.Errorf("expected view to contain 'Not in your Google Calendar', got:\n%s", viewStr)
	}
	if !strings.Contains(viewStr, "[a / c / Enter] Add to Calendar") {
		t.Errorf("expected view to contain '[a / c / Enter] Add to Calendar', got:\n%s", viewStr)
	}
}

func TestTUI_ICS_AddCalendar_VerificationAndNoDuplicates(t *testing.T) {
	calledAdd := false
	bk := &Backend{
		Calendar: stubVerifyingCalendar{
			AddICSVerifiedFn: func(_ *cfg_g.Config, _ []byte) (string, bool, bool, bool, error) {
				calledAdd = true
				return "Architecture Design Review", false, true, true, nil
			},
		},
	}
	mockClient := &mailclient.MockMailClient{Cfg: &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: t.TempDir()}, AccountCache: cachedir.At(t.TempDir())}}
	m := newTuiModel(mockClient, "INBOX", bk)
	m.width = 90
	m.height = 30
	m.openICSViewer(email.Attachment{Filename: "invite.ics", Data: []byte(sampleICSData)})
	m.icsCheckStatus = icsStatusNotInCalendar
	m.icsInCalendar = false

	// Press 'a' to add to calendar
	res, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	tm := res.(*tuiModel)

	if tm.icsCheckStatus != icsStatusAdding {
		t.Errorf("expected status icsStatusAdding, got %v", tm.icsCheckStatus)
	}
	if cmd == nil {
		t.Fatalf("expected cmd to execute addition")
	}

	// Execute cmd
	msg := cmd()
	addMsg, ok := msg.(icsCalAddMsg)
	if !ok {
		t.Fatalf("expected icsCalAddMsg from cmd, got %T", msg)
	}
	if !calledAdd {
		t.Errorf("expected AddICSDataWithVerification to be called")
	}
	if !addMsg.verified || !addMsg.noDuplicates {
		t.Errorf("expected verified=true and noDuplicates=true")
	}

	// Deliver add message to model
	res2, _ := tm.Update(addMsg)
	tm2 := res2.(*tuiModel)

	if !tm2.icsInCalendar {
		t.Errorf("expected icsInCalendar=true after add")
	}
	if tm2.icsCheckStatus != icsStatusAddedVerified {
		t.Errorf("expected icsStatusAddedVerified, got %v", tm2.icsCheckStatus)
	}

	viewStr := tm2.View()
	if !strings.Contains(viewStr, "Added and verified in Calendar (no duplicates)") {
		t.Errorf("expected view to show verified addition with no duplicates, got:\n%s", viewStr)
	}
}

func TestTUI_ICS_AddCalendar_DuplicateDetected(t *testing.T) {
	mockClient := &mailclient.MockMailClient{Cfg: &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: t.TempDir()}, AccountCache: cachedir.At(t.TempDir())}}
	m := newTuiModel(mockClient, "INBOX", nil)
	m.width = 90
	m.height = 30
	m.openICSViewer(email.Attachment{Filename: "invite.ics", Data: []byte(sampleICSData)})

	// Deliver add message where duplicate is detected
	res, _ := m.Update(icsCalAddMsg{
		summary:      "Architecture Design Review",
		existed:      false,
		verified:     true,
		noDuplicates: false,
	})
	tm := res.(*tuiModel)

	if tm.icsCheckStatus != icsStatusAddedWithDups {
		t.Errorf("expected icsStatusAddedWithDups, got %v", tm.icsCheckStatus)
	}

	viewStr := tm.View()
	if !strings.Contains(viewStr, "duplicate entries detected") {
		t.Errorf("expected view to contain duplicate warning, got:\n%s", viewStr)
	}
}

func TestTUI_ICS_CloseModal(t *testing.T) {
	mockClient := &mailclient.MockMailClient{Cfg: &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: t.TempDir()}, AccountCache: cachedir.At(t.TempDir())}}
	m := newTuiModel(mockClient, "INBOX", nil)
	m.width = 90
	m.height = 30
	m.openICSViewer(email.Attachment{Filename: "invite.ics", Data: []byte(sampleICSData)})

	if !m.icsModalOpen {
		t.Fatalf("expected modal open")
	}

	// Press Esc to close modal
	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	tm := res.(*tuiModel)

	if tm.icsModalOpen {
		t.Errorf("expected icsModalOpen=false after Esc")
	}
}

func TestTUI_ICS_Scrolling(t *testing.T) {
	longICS := `BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
UID:long-123
SUMMARY:Long Agenda Meeting
DESCRIPTION:Line 1\nLine 2\nLine 3\nLine 4\nLine 5\nLine 6\nLine 7\nLine 8\nLine 9\nLine 10
DTSTART:20260915T140000Z
DTEND:20260915T150000Z
END:VEVENT
END:VCALENDAR`

	mockClient := &mailclient.MockMailClient{Cfg: &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: t.TempDir()}, AccountCache: cachedir.At(t.TempDir())}}
	m := newTuiModel(mockClient, "INBOX", nil)
	m.width = 90
	m.height = 30
	m.openICSViewer(email.Attachment{Filename: "long.ics", Data: []byte(longICS)})

	initialYOffset := m.icsVp.YOffset

	// Press 'j' / down to scroll
	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	tm := res.(*tuiModel)

	if tm.icsVp.YOffset <= initialYOffset && tm.icsVp.TotalLineCount() > tm.icsVp.Height {
		t.Errorf("expected YOffset to change after scroll down")
	}
}

func TestTUI_ICS_DurationFormatting(t *testing.T) {
	if d := formatICSDuration(1 * time.Hour); d != "1h" {
		t.Errorf("expected '1h', got %q", d)
	}
	if d := formatICSDuration(90 * time.Minute); d != "1h 30m" {
		t.Errorf("expected '1h 30m', got %q", d)
	}
	if d := formatICSDuration(45 * time.Minute); d != "45m" {
		t.Errorf("expected '45m', got %q", d)
	}
}

func TestTUI_ICS_MultiEvent_Navigation(t *testing.T) {
	multiICS := `BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
UID:sess-1
SUMMARY:Keynote Session
DTSTART:20260901T090000Z
DTEND:20260901T100000Z
END:VEVENT
BEGIN:VEVENT
UID:sess-2
SUMMARY:Workshop Part 1
DTSTART:20260901T103000Z
DTEND:20260901T120000Z
END:VEVENT
BEGIN:VEVENT
UID:sess-3
SUMMARY:Workshop Part 2
DTSTART:20260901T133000Z
DTEND:20260901T150000Z
END:VEVENT
END:VCALENDAR`

	mockClient := &mailclient.MockMailClient{Cfg: &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: t.TempDir()}, AccountCache: cachedir.At(t.TempDir())}}
	m := newTuiModel(mockClient, "INBOX", nil)
	m.width = 90
	m.height = 30
	m.openICSViewer(email.Attachment{Filename: "conference.ics", Data: []byte(multiICS)})

	if len(m.icsEvents) != 3 {
		t.Fatalf("expected 3 events, got %d", len(m.icsEvents))
	}
	if m.icsEventIdx != 0 {
		t.Errorf("expected initial icsEventIdx=0, got %d", m.icsEventIdx)
	}

	view1 := m.View()
	if !strings.Contains(view1, "Event 1 of 3") {
		t.Errorf("expected 'Event 1 of 3' in view, got:\n%s", view1)
	}
	if !strings.Contains(view1, "Keynote Session") {
		t.Errorf("expected 'Keynote Session' in view, got:\n%s", view1)
	}
	if !strings.Contains(view1, "Schedule in this attachment:") {
		t.Errorf("expected 'Schedule in this attachment:' in view, got:\n%s", view1)
	}

	// Press 'n' / tab to switch to next event
	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	tm := res.(*tuiModel)

	if tm.icsEventIdx != 1 {
		t.Errorf("expected icsEventIdx=1 after 'n', got %d", tm.icsEventIdx)
	}
	if tm.icsEvent.Summary != "Workshop Part 1" {
		t.Errorf("expected 'Workshop Part 1', got %q", tm.icsEvent.Summary)
	}

	view2 := tm.View()
	if !strings.Contains(view2, "Event 2 of 3") {
		t.Errorf("expected 'Event 2 of 3' in view, got:\n%s", view2)
	}

	// Press 'p' to switch back
	res2, _ := tm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	tm2 := res2.(*tuiModel)

	if tm2.icsEventIdx != 0 {
		t.Errorf("expected icsEventIdx=0 after 'p', got %d", tm2.icsEventIdx)
	}
}

func TestTUI_ICS_RRuleFormatting(t *testing.T) {
	cases := []struct {
		rrule    string
		expected string
	}{
		{"FREQ=DAILY", "Daily"},
		{"FREQ=DAILY;INTERVAL=3;COUNT=5", "Every 3 days (5 occurrences)"},
		{"FREQ=WEEKLY;BYDAY=MO,WE,FR", "Weekly on Mon, Wed, Fri"},
		{"FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR", "Weekly on weekdays"},
		{"FREQ=WEEKLY;BYDAY=SA,SU", "Weekly on weekends"},
		{"FREQ=MONTHLY;INTERVAL=2", "Every 2 months"},
		{"FREQ=YEARLY", "Yearly"},
	}

	for _, c := range cases {
		got := formatHumanRRule(c.rrule)
		if !strings.Contains(got, c.expected) {
			t.Errorf("for %q expected to contain %q, got %q", c.rrule, c.expected, got)
		}
	}
}

func TestTUI_ICS_InlineEventDetails(t *testing.T) {
	raw := "From: \"Sam Smith\" <samuel@example.edu>\r\n" +
		"To: Alex Taylor <alex@example.org>\r\n" +
		"Subject: Team Research Meeting\r\n" +
		"Date: Wed, 26 Aug 2026 20:52:07 +0000\r\n" +
		"Content-Type: multipart/alternative; boundary=\"_boundary_teams_\"\r\n" +
		"MIME-Version: 1.0\r\n\r\n" +
		"--_boundary_teams_\r\n" +
		"Content-Type: text/plain; charset=\"utf-8\"\r\n\r\n" +
		"Join Microsoft Teams Meeting\r\n" +
		"--_boundary_teams_\r\n" +
		"Content-Type: text/calendar; charset=\"utf-8\"; method=REQUEST\r\n\r\n" +
		"BEGIN:VCALENDAR\r\n" +
		"VERSION:2.0\r\n" +
		"PRODID:-//Example//EN\r\n" +
		"BEGIN:VEVENT\r\n" +
		"SUMMARY:Team Research Meeting\r\n" +
		"DTSTART:20260826T210000Z\r\n" +
		"DTEND:20260826T220000Z\r\n" +
		"DESCRIPTION:Meeting\r\n" +
		"END:VEVENT\r\n" +
		"END:VCALENDAR\r\n" +
		"--_boundary_teams_--\r\n"
	data := []byte(raw)

	atts, err := email.ExtractAttachments(data)
	if err != nil {
		t.Fatalf("failed to extract attachments: %v", err)
	}

	em := email.ParseReader(bytes.NewReader(data), "msg-ics-test", "")
	if em == nil {
		t.Fatalf("failed to parse email")
	}

	mockClient := &mailclient.MockMailClient{Cfg: &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: t.TempDir()}, AccountCache: cachedir.At(t.TempDir())}}
	m := newTuiModel(mockClient, "INBOX", nil)
	m.width = 90
	m.height = 30
	m.emails = []email.Email{*em}
	m.eIdx = 0
	m.selectedID = em.ID
	m.attachments = atts
	m.detailH = "From: Sam Smith <samuel@example.edu>\nSubject: Team Research Meeting\nDate: Wed, 26 Aug 2026 20:52:07 +0000"
	m.detail = "Join Microsoft Teams Meeting"

	content := m.buildDetailContent(80)
	if !strings.Contains(content, "CALENDAR EVENT DETAILS") {
		t.Errorf("expected detail content to contain inline calendar event details header, got:\n%s", content)
	}
	if !strings.Contains(content, "Team Research Meeting") {
		t.Errorf("expected detail content to contain 'Team Research Meeting', got:\n%s", content)
	}
	if !strings.Contains(content, "Microsoft Teams Meeting") {
		t.Errorf("expected detail content to contain 'Microsoft Teams Meeting', got:\n%s", content)
	}
}

func TestTUI_ICS_UpdateCalendar_WhenAlreadyInCalendar(t *testing.T) {
	calledUpdate := false
	bk := &Backend{
		Calendar: stubVerifyingCalendar{
			AddICSVerifiedFn: func(_ *cfg_g.Config, _ []byte) (string, bool, bool, bool, error) {
				calledUpdate = true
				return "Team Standup", true, true, true, nil
			},
		},
	}
	mockClient := &mailclient.MockMailClient{Cfg: &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: t.TempDir()}, AccountCache: cachedir.At(t.TempDir())}}
	m := newTuiModel(mockClient, "INBOX", bk)
	m.width = 90
	m.height = 30
	m.openICSViewer(email.Attachment{Filename: "invite.ics", Data: []byte(sampleICSData)})
	m.icsCheckStatus = icsStatusInCalendar
	m.icsInCalendar = true

	viewStr := m.View()
	if !strings.Contains(viewStr, "[u / Enter] Update event") {
		t.Errorf("expected view footer to show '[u / Enter] Update event', got:\n%s", viewStr)
	}

	// Press 'u' to update calendar event
	res, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("u")})
	tm := res.(*tuiModel)

	if tm.icsCheckStatus != icsStatusAdding {
		t.Errorf("expected status icsStatusAdding, got %v", tm.icsCheckStatus)
	}
	if cmd == nil {
		t.Fatalf("expected cmd to execute update")
	}

	// Execute cmd
	msg := cmd()
	addMsg, ok := msg.(icsCalAddMsg)
	if !ok {
		t.Fatalf("expected icsCalAddMsg from cmd, got %T", msg)
	}
	if !calledUpdate {
		t.Errorf("expected AddICSDataWithVerification to be called")
	}
	if !addMsg.existed || !addMsg.verified {
		t.Errorf("expected existed=true and verified=true")
	}

	// Deliver addMsg to model
	res2, _ := tm.Update(addMsg)
	tm2 := res2.(*tuiModel)

	if tm2.icsCheckStatus != icsStatusUpdatedVerified {
		t.Errorf("expected icsStatusUpdatedVerified, got %v", tm2.icsCheckStatus)
	}
	viewAfter := tm2.View()
	if !strings.Contains(viewAfter, "Updated and verified in Calendar") {
		t.Errorf("expected view to show 'Updated and verified in Calendar', got:\n%s", viewAfter)
	}
}

func TestTUI_ICS_SimilarEventWarningAndAddAsNew(t *testing.T) {
	mockClient := &mailclient.MockMailClient{Cfg: &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: t.TempDir()}, AccountCache: cachedir.At(t.TempDir())}}
	calledAddAsNew := false
	stubBk := &Backend{
		Calendar: stubVerifyingCalendar{
			AddICSAsNewFn: func(_ *cfg_g.Config, _ []byte) (string, error) {
				calledAddAsNew = true
				return "Design Review", nil
			},
		},
	}
	m := newTuiModel(mockClient, "INBOX", stubBk)
	m.width = 90
	m.height = 30
	m.openICSViewer(email.Attachment{Filename: "invite.ics", Data: []byte(sampleICSData)})

	// Deliver message indicating a similar event was found with discrepancy
	res, _ := m.Update(icsCalCheckMsg{
		inCalendar:  true,
		discrepancy: "Calendar start time is 11:00 AM; invite is 10:00 AM",
		summary:     "Design Review",
		event:       m.icsEvent,
	})
	tm := res.(*tuiModel)

	if tm.icsCheckStatus != icsStatusSimilarFound {
		t.Fatalf("expected icsStatusSimilarFound, got %v", tm.icsCheckStatus)
	}

	viewStr := tm.View()
	if !strings.Contains(viewStr, "Similar event found in Google Calendar") {
		t.Errorf("expected view to contain warning, got:\n%s", viewStr)
	}
	if !strings.Contains(viewStr, "[c] Add as new event") {
		t.Errorf("expected view to contain '[c] Add as new event', got:\n%s", viewStr)
	}

	// Press 'c' to add as new separate event
	res2, cmd := tm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	tm2 := res2.(*tuiModel)
	if cmd == nil {
		t.Fatalf("expected cmd from pressing 'c'")
	}
	msg := cmd()
	addMsg, ok := msg.(icsCalAddMsg)
	if !ok || !calledAddAsNew || !addMsg.verified {
		t.Errorf("expected AddICSAsNew to be called and verified")
	}
	_ = tm2
}

func TestTUI_ICS_RecurringEventNextOccurrences(t *testing.T) {
	recurringICS := `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Example Corp.//EN
BEGIN:VEVENT
UID:recur-12345
DTSTAMP:20260101T000000Z
DTSTART:20260101T140000Z
DTEND:20260101T150000Z
RRULE:FREQ=DAILY;INTERVAL=1
SUMMARY:Daily Standup
END:VEVENT
END:VCALENDAR`

	mockClient := &mailclient.MockMailClient{Cfg: &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: t.TempDir()}, AccountCache: cachedir.At(t.TempDir())}}
	m := newTuiModel(mockClient, "INBOX", nil)
	m.width = 90
	m.height = 30
	m.openICSViewer(email.Attachment{Filename: "standup.ics", Data: []byte(recurringICS)})

	viewStr := m.View()
	if !strings.Contains(viewStr, "Repeats:") || !strings.Contains(viewStr, "Daily") {
		t.Errorf("expected repeats daily in view, got:\n%s", viewStr)
	}
	if !strings.Contains(viewStr, "Next Sched:") {
		t.Errorf("expected Next Sched in view, got:\n%s", viewStr)
	}
}

// A calendar invite reached through 'a' must open the calendar modal, not the
// read-only text preview: the modal is where 'c' adds the event, and the
// attachment list is dispatched before it.
func TestTUI_ICS_OpenFromAttachmentList(t *testing.T) {
	mockClient := &mailclient.MockMailClient{Cfg: &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: t.TempDir()}, AccountCache: cachedir.At(t.TempDir())}}
	m := newTuiModel(mockClient, "INBOX", nil)
	m.width = 90
	m.height = 30
	m.emails = []email.Email{{ID: "msg-1", Subject: "Team Sync"}}
	m.eIdx = 0
	m.attachments = []email.Attachment{
		{Filename: "agenda.txt", ContentType: "text/plain", Data: []byte("agenda")},
		{Filename: "invite.ics", ContentType: "text/calendar", Data: []byte(sampleICSData)},
	}

	// 'a' opens the attachment list, 'j' moves to the invite.
	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	tm := res.(*tuiModel)
	if !tm.attachmentListOpen {
		t.Fatalf("expected attachmentListOpen=true after 'a'")
	}
	res, _ = tm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	tm = res.(*tuiModel)

	res, cmd := tm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	tm = res.(*tuiModel)

	if !tm.icsModalOpen {
		t.Fatalf("expected Enter on the invite to open the calendar modal")
	}
	if tm.attachmentListOpen {
		t.Errorf("expected the attachment list to close so the calendar modal receives keys")
	}
	if tm.attachmentViewerOpen {
		t.Errorf("expected the invite not to open the read-only text viewer")
	}
	if cmd == nil {
		t.Fatalf("expected the background calendar check to be dispatched")
	}
	if tm.icsEvent == nil || tm.icsEvent.Summary != "Architecture Design Review" {
		t.Fatalf("expected the invite to be parsed, got %+v", tm.icsEvent)
	}
	if tm.attCursor != 1 {
		t.Errorf("expected attCursor=1 for attachment navigation, got %d", tm.attCursor)
	}
}

func TestTUI_ICS_AddToCalendarFromAttachmentList(t *testing.T) {
	mockClient := &mailclient.MockMailClient{Cfg: &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: t.TempDir()}, AccountCache: cachedir.At(t.TempDir())}}
	m := newTuiModel(mockClient, "INBOX", nil)
	m.width = 90
	m.height = 30
	m.attachments = []email.Attachment{
		{Filename: "invite.ics", ContentType: "text/calendar", Data: []byte(sampleICSData)},
	}
	m.attachmentListOpen = true

	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	tm := res.(*tuiModel)
	if !tm.icsModalOpen {
		t.Fatalf("expected the calendar modal to be open")
	}

	// 'c' adds the event to the calendar: it must reach the modal rather than
	// be swallowed by the attachment list.
	res, cmd := tm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	tm = res.(*tuiModel)

	if tm.icsCheckStatus != icsStatusAdding {
		t.Fatalf("expected icsStatusAdding after 'c', got %v", tm.icsCheckStatus)
	}
	if cmd == nil {
		t.Fatalf("expected an add-to-calendar command after 'c'")
	}
	if !strings.Contains(tm.icsAddStatus, "Adding") {
		t.Errorf("expected an add status, got %q", tm.icsAddStatus)
	}
}

// The path a reader takes: 'a' opens the attachment list, whose cursor starts
// on the message body, and 'c' adds the invitation. The list had no 'c', so
// the key was dropped and nothing happened.
func TestTUI_ICS_AttachmentListCAddsTheInvitation(t *testing.T) {
	var added []byte
	bk := &Backend{
		Calendar: stubVerifyingCalendar{
			AddICSVerifiedFn: func(_ *cfg_g.Config, data []byte) (string, bool, bool, bool, error) {
				added = data
				return "Architecture Design Review", false, true, true, nil
			},
		},
	}
	mockClient := &mailclient.MockMailClient{Cfg: &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: t.TempDir()}, AccountCache: cachedir.At(t.TempDir())}}
	m := newTuiModel(mockClient, "INBOX", bk)
	m.width, m.height = 90, 30
	m.emails = []email.Email{{ID: "msg-1", Subject: "Invitation"}}
	m.eIdx = 0
	m.attachments = []email.Attachment{
		{Filename: "message_body.html", ContentType: "text/html", Data: []byte("<p>Join us</p>")},
		{Filename: "invite.ics", ContentType: "text/calendar", Data: []byte(sampleICSData)},
	}

	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	tm := res.(*tuiModel)
	if !tm.attachmentListOpen || tm.attachmentListCursor != 0 {
		t.Fatalf("'a' should open the list on its first entry")
	}
	res, cmd := tm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	tm = res.(*tuiModel)
	if cmd == nil {
		t.Fatal("'c' in the attachment list did nothing")
	}
	if tm.attachmentListOpen || !tm.icsModalOpen || tm.icsCheckStatus != icsStatusAdding {
		t.Fatalf("want the calendar modal open and adding; list=%v modal=%v status=%v",
			tm.attachmentListOpen, tm.icsModalOpen, tm.icsCheckStatus)
	}
	res, _ = tm.Update(cmd())
	tm = res.(*tuiModel)
	if string(added) != sampleICSData {
		t.Errorf("the calendar was given %q, want the .ics attachment", added)
	}
	if tm.icsCheckStatus != icsStatusAddedVerified || !tm.icsInCalendar {
		t.Errorf("after the add: status=%v inCalendar=%v", tm.icsCheckStatus, tm.icsInCalendar)
	}
}

// With no invitation on the message, 'c' says so instead of doing nothing.
func TestTUI_ICS_AttachmentListCWithoutInvitation(t *testing.T) {
	mockClient := &mailclient.MockMailClient{Cfg: &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: t.TempDir()}, AccountCache: cachedir.At(t.TempDir())}}
	m := newTuiModel(mockClient, "INBOX", nil)
	m.width, m.height = 90, 30
	m.emails = []email.Email{{ID: "msg-1", Subject: "Report"}}
	m.eIdx = 0
	m.attachments = []email.Attachment{{Filename: "report.pdf", ContentType: "application/pdf", Data: []byte("%PDF")}}

	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	res, cmd := res.(*tuiModel).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	tm := res.(*tuiModel)
	if cmd != nil || tm.icsModalOpen {
		t.Fatal("'c' without an .ics should not start an add")
	}
	if !strings.Contains(tm.saveStatus, "No calendar") {
		t.Errorf("status = %q, want a no-invitation notice", tm.saveStatus)
	}
}

// A Google or Outlook invitation carries its event twice: inline as
// text/calendar and attached as invite.ics. The message view drew a card for
// each, so every invitation showed its event twice.
func TestDetailICSHeaderDrawsEachEventOnce(t *testing.T) {
	mockClient := &mailclient.MockMailClient{Cfg: &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: t.TempDir()}, AccountCache: cachedir.At(t.TempDir())}}
	m := newTuiModel(mockClient, "INBOX", nil)
	m.width, m.height = 90, 30
	other := strings.NewReplacer("meet-12345", "meet-67890", "Architecture Design Review", "Budget Planning").Replace(sampleICSData)
	m.attachments = []email.Attachment{
		{Filename: "invite.ics", ContentType: "text/calendar", Data: []byte(sampleICSData)},
		{Filename: "invite.ics", ContentType: "application/ics", Data: []byte(sampleICSData)},
		{Filename: "other.ics", ContentType: "text/calendar", Data: []byte(other)},
	}

	header := m.buildDetailICSHeader(80)
	if n := strings.Count(header, "Architecture Design Review"); n != 1 {
		t.Errorf("the invitation's event is drawn %d times, want once:\n%s", n, header)
	}
	if !strings.Contains(header, "Budget Planning") {
		t.Errorf("a different event in the same message was dropped:\n%s", header)
	}
}
