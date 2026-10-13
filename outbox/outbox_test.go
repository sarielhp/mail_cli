package outbox

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestOutboxEnqueueAndList(t *testing.T) {
	tmpDir := t.TempDir()

	e1, err := Enqueue(tmpDir, "work", []string{"a@example.com"}, "First", []byte("RFC822 body 1"))
	if err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}
	time.Sleep(10 * time.Millisecond)
	e2, err := Enqueue(tmpDir, "personal", []string{"b@example.com", "c@example.com"}, "Second", []byte("RFC822 body 2"))
	if err != nil {
		t.Fatalf("Enqueue 2 failed: %v", err)
	}

	list, err := List(tmpDir)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 items, got %d", len(list))
	}
	if list[0].ID != e1.ID || list[1].ID != e2.ID {
		t.Errorf("expected list order [%s, %s], got [%s, %s]", e1.ID, e2.ID, list[0].ID, list[1].ID)
	}
	if list[0].Account != "work" || list[0].Subject != "First" {
		t.Errorf("entry 0 metadata mismatch: %+v", list[0])
	}
}

func TestOutboxGetAndRemove(t *testing.T) {
	tmpDir := t.TempDir()

	e, err := Enqueue(tmpDir, "work", []string{"a@example.com"}, "Test Subject", []byte("Payload data"))
	if err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}

	meta, raw, err := Get(tmpDir, e.ID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if meta.Subject != "Test Subject" {
		t.Errorf("subject mismatch: %q", meta.Subject)
	}
	if string(raw) != "Payload data" {
		t.Errorf("payload mismatch: %q", string(raw))
	}

	if attemptErr := RecordAttempt(tmpDir, e.ID, errors.New("network unreachable")); attemptErr != nil {
		t.Fatalf("RecordAttempt failed: %v", attemptErr)
	}
	metaUpdated, _, err := Get(tmpDir, e.ID)
	if err != nil {
		t.Fatalf("Get after attempt failed: %v", err)
	}
	if metaUpdated.Attempts != 1 || metaUpdated.LastError != "network unreachable" {
		t.Errorf("unexpected attempt info: %+v", metaUpdated)
	}

	if removeErr := Remove(tmpDir, e.ID); removeErr != nil {
		t.Fatalf("Remove failed: %v", removeErr)
	}

	listAfter, err := List(tmpDir)
	if err != nil {
		t.Fatalf("List after remove failed: %v", err)
	}
	if len(listAfter) != 0 {
		t.Errorf("expected 0 entries after remove, got %d", len(listAfter))
	}
}

func TestOutboxDirFallback(t *testing.T) {
	d := Dir("/custom/download")
	expected := filepath.Join("/custom/download", "outbox")
	if d != expected {
		t.Errorf("expected %q, got %q", expected, d)
	}

	dEmpty := Dir("")
	if dEmpty == "" {
		t.Error("expected non-empty default dir")
	}
}

// Two processes have separate sequence counters, so the same millisecond and
// sequence number used to give the same ID and the second write replaced the
// first message.
func TestReserveIDSurvivesACollidingSequence(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	seqCounter = 0
	first, err := reserveID(dir, now)
	if err != nil {
		t.Fatal(err)
	}
	seqCounter = 0 // a second process starting from the same counter
	second, err := reserveID(dir, now)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatalf("two reservations in the same millisecond share ID %s", first)
	}
}
