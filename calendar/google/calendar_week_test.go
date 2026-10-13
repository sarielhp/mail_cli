package google

import (
	"testing"
	"time"

	"google.golang.org/api/calendar/v3"
)

func TestGroupEventsByDay_AllDayTimezoneOffset(t *testing.T) {
	// Negative offset timezone: CDT (UTC-5)
	loc := time.FixedZone("CDT", -5*3600)

	events := []*calendar.Event{
		{
			Id:      "allday-1",
			Summary: "All-Day Conference",
			Start: &calendar.EventDateTime{
				Date: "2026-09-18",
			},
		},
		{
			Id:      "timed-1",
			Summary: "Morning Standup",
			Start: &calendar.EventDateTime{
				DateTime: "2026-09-18T14:00:00Z", // 09:00 CDT
			},
		},
	}

	groups := groupEventsByDay(events, loc)
	if len(groups) != 1 {
		t.Fatalf("expected 1 date group, got %d", len(groups))
	}

	expectedDate := "Friday, 2026-09-18"
	if groups[0].dateStr != expectedDate {
		t.Errorf("group dateStr = %q, want %q", groups[0].dateStr, expectedDate)
	}

	if len(groups[0].items) != 2 {
		t.Errorf("expected 2 events in group, got %d", len(groups[0].items))
	}
}

func TestGroupEventsByDay_MultipleDays(t *testing.T) {
	loc := time.FixedZone("EDT", -4*3600)

	events := []*calendar.Event{
		{
			Id:      "ev-friday",
			Summary: "Friday All-Day",
			Start: &calendar.EventDateTime{
				Date: "2026-09-18",
			},
		},
		{
			Id:      "ev-saturday",
			Summary: "Saturday All-Day",
			Start: &calendar.EventDateTime{
				Date: "2026-09-19",
			},
		},
	}

	groups := groupEventsByDay(events, loc)
	if len(groups) != 2 {
		t.Fatalf("expected 2 date groups, got %d", len(groups))
	}

	if groups[0].dateStr != "Friday, 2026-09-18" {
		t.Errorf("group[0] dateStr = %q, want %q", groups[0].dateStr, "Friday, 2026-09-18")
	}
	if groups[1].dateStr != "Saturday, 2026-09-19" {
		t.Errorf("group[1] dateStr = %q, want %q", groups[1].dateStr, "Saturday, 2026-09-19")
	}
}
