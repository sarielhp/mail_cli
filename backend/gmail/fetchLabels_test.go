package gmail

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/report"

	gmailapi "google.golang.org/api/gmail/v1"
)

func TestFetchLabelDetailsConcurrently_PanicRecovery(_ *testing.T) {
	// Nil srv causes srv.Users.Labels.Get to panic
	labels := []*gmailapi.Label{
		{Id: "L1", Name: "Test1"},
		{Id: "L2", Name: "Test2"},
	}

	// Should not crash due to recovery boundary
	fetchLabelDetailsConcurrently(context.Background(), nil, labels)
}

func TestBackfillZeroCountLabels_PanicRecovery(_ *testing.T) {
	// Nil srv causes srv.Users.Messages.List to panic
	labels := []*gmailapi.Label{
		{Id: "L1", Name: "Test1", MessagesTotal: 0},
	}

	// Should not crash due to recovery boundary
	backfillZeroCountLabels(context.Background(), nil, labels)
}

func TestSortLabelsAlphabetically(t *testing.T) {
	labels := []*gmailapi.Label{
		{Name: "Zebra"},
		{Name: "apple"},
		{Name: "Banana"},
		{Name: "cherry"},
		{Name: "APRICOT"},
	}
	sortLabelsAlphabetically(labels)

	expected := []string{"apple", "APRICOT", "Banana", "cherry", "Zebra"}
	for i, exp := range expected {
		if labels[i].Name != exp {
			t.Errorf("at index %d: expected %s, got %s", i, exp, labels[i].Name)
		}
	}

	// Empty and single-item slices should not panic
	sortLabelsAlphabetically(nil)
	single := []*gmailapi.Label{{Name: "One"}}
	sortLabelsAlphabetically(single)
	if single[0].Name != "One" {
		t.Errorf("single element altered: %s", single[0].Name)
	}
}

func TestFetchLabelDetailsConcurrently_Retry(t *testing.T) {
	origDelay := gmailListRetryBaseDelay
	gmailListRetryBaseDelay = time.Millisecond
	defer func() { gmailListRetryBaseDelay = origDelay }()

	attempts := 0
	ts := retry429Server(&attempts, `{"id": "L1", "name": "Test1", "messagesTotal": 50, "messagesUnread": 12}`)
	defer ts.Close()

	srv := newGmailTestService(t, ts)

	labels := []*gmailapi.Label{
		{Id: "L1", Name: "Test1"},
	}

	fetchLabelDetailsConcurrently(context.Background(), srv, labels)

	if labels[0].MessagesTotal != 50 || labels[0].MessagesUnread != 12 {
		t.Errorf("expected 50 total and 12 unread, got %d total and %d unread",
			labels[0].MessagesTotal, labels[0].MessagesUnread)
	}
}

func TestUpdateLabelMessageCounts_Retry(t *testing.T) {
	origDelay := gmailListRetryBaseDelay
	gmailListRetryBaseDelay = time.Millisecond
	defer func() { gmailListRetryBaseDelay = origDelay }()

	attempts := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		if attempts%2 == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"error": {"code": 503, "message": "unavailable"}}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"messages": [{"id": "m1"}, {"id": "m2"}], "resultSizeEstimate": 2}`))
	}))
	defer ts.Close()

	srv := newGmailTestService(t, ts)

	label := &gmailapi.Label{Id: "L1", Name: "TestLabel"}
	var mu sync.Mutex

	updateLabelMessageCounts(context.Background(), srv, label, &mu)

	if label.MessagesTotal != 2 {
		t.Errorf("expected MessagesTotal = 2, got %d", label.MessagesTotal)
	}
	if label.MessagesUnread != 2 {
		t.Errorf("expected MessagesUnread = 2, got %d", label.MessagesUnread)
	}
}

func TestGetMatchingLabels_MissingCredentials(t *testing.T) {
	cfg := &Config{
		Paths: cfg_g.Paths{
			ConfigDir: t.TempDir(),
		},
	}
	_, err := GetMatchingLabels(report.Discard(), cfg, "Inbox")
	if err == nil {
		t.Fatal("expected error with missing credentials, got nil")
	}
}
