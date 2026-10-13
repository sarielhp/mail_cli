package email

import (
	"testing"
	"time"
)

func TestToSummaryJSON(t *testing.T) {
	d := time.Date(2026, 3, 15, 10, 30, 0, 0, time.UTC)
	e := Email{
		ID:            ID("msg-123"),
		Subject:       "Test subject",
		FromRaw:       "Sender <sender@example.com>",
		FromEmail:     "sender@example.com",
		EmailDate:     d,
		FormattedDate: "15 Mar",
		IsRead:        true,
		IsReplied:     false,
		HasAttachment: true,
		HasICS:        false,
		IsSpam:        false,
		IsPolitical:   false,
	}

	summary := ToSummaryJSON(e, "INBOX")
	if summary.ID != "msg-123" {
		t.Errorf("expected ID msg-123, got %q", summary.ID)
	}
	if summary.Folder != "INBOX" {
		t.Errorf("expected Folder INBOX, got %q", summary.Folder)
	}
	if summary.Subject != "Test subject" {
		t.Errorf("expected Subject 'Test subject', got %q", summary.Subject)
	}
	if !summary.IsRead || !summary.HasAttachment {
		t.Errorf("expected IsRead=true and HasAttachment=true, got read=%v attach=%v", summary.IsRead, summary.HasAttachment)
	}

	summaries := ToSummariesJSON([]Email{e}, map[ID]string{ID("msg-123"): "Archive"})
	if len(summaries) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(summaries))
	}
	if summaries[0].Folder != "Archive" {
		t.Errorf("expected Folder Archive from map, got %q", summaries[0].Folder)
	}
}
