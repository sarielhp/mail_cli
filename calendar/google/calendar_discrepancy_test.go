package google

import (
	"strings"
	"testing"
	"time"

	"google.golang.org/api/calendar/v3"
)

func TestDetectEventDiscrepancies_ExactMatch(t *testing.T) {
	start := time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)
	end := time.Date(2026, 9, 21, 11, 0, 0, 0, time.UTC)
	ev := &ICSEvent{
		Summary:     "Team Sync",
		StartParsed: start,
		EndParsed:   end,
		RRule:       "FREQ=WEEKLY;BYDAY=MO",
	}

	existing := &calendar.Event{
		Summary: "Team Sync",
		Start: &calendar.EventDateTime{
			DateTime: start.Format(time.RFC3339),
		},
		End: &calendar.EventDateTime{
			DateTime: end.Format(time.RFC3339),
		},
		Recurrence: []string{"RRULE:FREQ=WEEKLY;BYDAY=MO"},
	}

	diffs := detectEventDiscrepancies(existing, ev)
	if len(diffs) != 0 {
		t.Fatalf("expected 0 discrepancies for exact match, got %v", diffs)
	}
}

func TestDetectEventDiscrepancies_StartTimeDiff(t *testing.T) {
	evStart := time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)
	calStart := time.Date(2026, 9, 21, 11, 0, 0, 0, time.UTC)
	ev := &ICSEvent{
		StartParsed: evStart,
		EndParsed:   evStart.Add(time.Hour),
	}

	existing := &calendar.Event{
		Start: &calendar.EventDateTime{
			DateTime: calStart.Format(time.RFC3339),
		},
		End: &calendar.EventDateTime{
			DateTime: calStart.Add(time.Hour).Format(time.RFC3339),
		},
	}

	diffs := detectEventDiscrepancies(existing, ev)
	if len(diffs) == 0 {
		t.Fatalf("expected start time discrepancy, got none")
	}
	if !strings.Contains(diffs[0], "start time") {
		t.Errorf("expected start time diff message, got: %s", diffs[0])
	}
}

func TestDetectEventDiscrepancies_DurationDiff(t *testing.T) {
	start := time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)
	ev := &ICSEvent{
		StartParsed: start,
		EndParsed:   start.Add(30 * time.Minute),
	}

	existing := &calendar.Event{
		Start: &calendar.EventDateTime{
			DateTime: start.Format(time.RFC3339),
		},
		End: &calendar.EventDateTime{
			DateTime: start.Add(60 * time.Minute).Format(time.RFC3339),
		},
	}

	diffs := detectEventDiscrepancies(existing, ev)
	if len(diffs) == 0 {
		t.Fatalf("expected duration discrepancy, got none")
	}
	if !strings.Contains(diffs[0], "duration") {
		t.Errorf("expected duration diff message, got: %s", diffs[0])
	}
}

func TestDetectEventDiscrepancies_RecurrenceDiff(t *testing.T) {
	start := time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)
	ev := &ICSEvent{
		StartParsed: start,
		EndParsed:   start.Add(time.Hour),
		RRule:       "FREQ=WEEKLY;BYDAY=TU",
	}

	// Existing has no recurrence
	existing := &calendar.Event{
		Start: &calendar.EventDateTime{
			DateTime: start.Format(time.RFC3339),
		},
		End: &calendar.EventDateTime{
			DateTime: start.Add(time.Hour).Format(time.RFC3339),
		},
	}

	diffs := detectEventDiscrepancies(existing, ev)
	if len(diffs) == 0 {
		t.Fatalf("expected recurrence discrepancy, got none")
	}
	if !strings.Contains(diffs[0], "recurrence") {
		t.Errorf("expected recurrence diff message, got: %s", diffs[0])
	}
}
