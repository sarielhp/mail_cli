package calendar

import (
	"testing"
	"time"
)

func TestNextOccurrences_NonRecurring(t *testing.T) {
	start := time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)
	ev := &ICSEvent{
		StartParsed: start,
	}

	fromBefore := time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC)
	occs := ev.NextOccurrences(fromBefore, 2)
	if len(occs) != 1 || !occs[0].Equal(start) {
		t.Fatalf("expected 1 occurrence equal to start, got %v", occs)
	}

	fromAfter := time.Date(2026, 9, 22, 0, 0, 0, 0, time.UTC)
	occsAfter := ev.NextOccurrences(fromAfter, 2)
	if len(occsAfter) != 0 {
		t.Fatalf("expected 0 occurrences after event date, got %v", occsAfter)
	}
}

func TestNextOccurrences_Recurring(t *testing.T) {
	start := time.Date(2026, 9, 1, 14, 0, 0, 0, time.UTC)
	ev := &ICSEvent{
		StartParsed: start,
		RRule:       "FREQ=WEEKLY;BYDAY=TU",
	}

	from := time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC)
	occs := ev.NextOccurrences(from, 2)
	if len(occs) != 2 {
		t.Fatalf("expected 2 occurrences, got %d", len(occs))
	}

	expectedFirst := time.Date(2026, 9, 22, 14, 0, 0, 0, time.UTC)
	expectedSecond := time.Date(2026, 9, 29, 14, 0, 0, 0, time.UTC)
	if !occs[0].Equal(expectedFirst) {
		t.Errorf("expected first occurrence %v, got %v", expectedFirst, occs[0])
	}
	if !occs[1].Equal(expectedSecond) {
		t.Errorf("expected second occurrence %v, got %v", expectedSecond, occs[1])
	}
}

func TestNextOccurrences_InvalidOrEmpty(t *testing.T) {
	var nilEv *ICSEvent
	if occs := nilEv.NextOccurrences(time.Now(), 2); occs != nil {
		t.Errorf("expected nil for nil event, got %v", occs)
	}

	emptyEv := &ICSEvent{}
	if occs := emptyEv.NextOccurrences(time.Now(), 2); occs != nil {
		t.Errorf("expected nil for empty event, got %v", occs)
	}

	validEv := &ICSEvent{StartParsed: time.Now()}
	if occs := validEv.NextOccurrences(time.Now(), 0); occs != nil {
		t.Errorf("expected nil for count=0, got %v", occs)
	}
}
