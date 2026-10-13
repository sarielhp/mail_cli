package mailclient

import (
	"context"
	"errors"
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"github.com/sarielhp/mail_cli/email"
	"testing"

	"github.com/sarielhp/mail_cli/cache"
	"github.com/sarielhp/mail_cli/cfg_g"
)

// The accessors are what let a caller ask for a message by ID alone. These
// check that the decorator chain a real client is built from -- Checking over
// ReadOnly over the backend -- passes them through rather than swallowing them,
// since both decorators short-circuit other methods.

func accessorChain(t *testing.T, backend *MockMailClient) *CheckingMailClient {
	t.Helper()
	return NewCheckingMailClient(
		NewReadOnlyMailClient(backend, "test-account"),
		"test-account",
		&cache.DiskCacheStore{Dir: cachedir.At(t.TempDir())},
	)
}

func mockWithMessages() *MockMailClient {
	return &MockMailClient{
		Cfg: &cfg_g.Config{},
		RawMessages: map[email.ID][]byte{
			"m1": []byte("From: a@example.com\r\nSubject: first\r\nDate: Tue, 02 Sep 2025 10:04:05 +0000\r\n\r\nbody\r\n"),
			"m2": []byte("From: b@example.com\r\nSubject: second\r\nDate: Wed, 03 Sep 2025 11:00:00 +0000\r\n\r\nbody\r\n"),
		},
	}
}

func TestGetRawMessageThroughDecorators(t *testing.T) {
	client := accessorChain(t, mockWithMessages())

	raw, err := client.GetRawMessage("m1")
	if err != nil {
		t.Fatalf("GetRawMessage: %v", err)
	}
	if len(raw) == 0 {
		t.Fatal("GetRawMessage returned no bytes")
	}
}

// Reading a message is not a mutation, so read-only must not block it. This is
// the property `show <id>` depends on under --read-only.
func TestReadOnlyPermitsMessageAccess(t *testing.T) {
	backend := mockWithMessages()
	ro := NewReadOnlyMailClient(backend, "test-account")

	if _, err := ro.GetRawMessage("m1"); err != nil {
		t.Errorf("GetRawMessage under read-only = %v, want nil", err)
	}
	if _, err := ro.GetMessageSummary("m1"); err != nil {
		t.Errorf("GetMessageSummary under read-only = %v, want nil", err)
	}
	if _, err := ro.GetRawMessagesBatch([]email.ID{"m1", "m2"}); err != nil {
		t.Errorf("GetRawMessagesBatch under read-only = %v, want nil", err)
	}
}

func TestGetMessageSummaryParsesHeaders(t *testing.T) {
	client := accessorChain(t, mockWithMessages())

	summary, err := client.GetMessageSummary("m2")
	if err != nil {
		t.Fatalf("GetMessageSummary: %v", err)
	}
	if summary.Subject != "second" {
		t.Errorf("Subject = %q, want %q", summary.Subject, "second")
	}
	if summary.FromEmail != "b@example.com" {
		t.Errorf("FromEmail = %q, want b@example.com", summary.FromEmail)
	}
	if summary.ID != "m2" {
		t.Errorf("ID = %q, want m2", summary.ID)
	}
}

func TestBatchAccessorsPreserveOrder(t *testing.T) {
	client := accessorChain(t, mockWithMessages())
	ids := []email.ID{"m2", "m1"}

	raws, err := client.GetRawMessagesBatch(ids)
	if err != nil {
		t.Fatalf("GetRawMessagesBatch: %v", err)
	}
	if len(raws) != 2 {
		t.Fatalf("got %d entries, want 2", len(raws))
	}
	summaries, err := client.GetMessageSummariesBatch(ids)
	if err != nil {
		t.Fatalf("GetMessageSummariesBatch: %v", err)
	}
	if summaries[0].Subject != "second" || summaries[1].Subject != "first" {
		t.Errorf("summaries came back as %q, %q; input order was not preserved",
			summaries[0].Subject, summaries[1].Subject)
	}
}

// A missing message yields a nil hole and an error, not a lost batch.
func TestBatchAccessorsReturnPartialResults(t *testing.T) {
	client := accessorChain(t, mockWithMessages())

	raws, err := client.GetRawMessagesBatch([]email.ID{"m1", "absent", "m2"})
	if err == nil {
		t.Fatal("GetRawMessagesBatch returned nil error for a missing message")
	}
	if len(raws) != 3 {
		t.Fatalf("got %d entries, want 3", len(raws))
	}
	if raws[0] == nil || raws[2] == nil {
		t.Error("present messages were dropped alongside the missing one")
	}
	if raws[1] != nil {
		t.Error("missing message should be a nil entry")
	}
}

func TestAccessorFailureIsReported(t *testing.T) {
	backend := mockWithMessages()
	backend.FailOn = map[string]error{"GetRawMessage": errBackend}
	client := accessorChain(t, backend)

	if _, err := client.GetRawMessage("m1"); !errors.Is(err, errBackend) {
		t.Errorf("GetRawMessage error = %v, want %v", err, errBackend)
	}
}

func TestMessageAccessor_ContextCancellation(t *testing.T) {
	client := accessorChain(t, mockWithMessages())
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-canceled

	if _, err := client.GetRawMessageContext(ctx, "m1"); !errors.Is(err, context.Canceled) {
		t.Errorf("GetRawMessageContext error = %v, want context.Canceled", err)
	}
	if _, err := client.GetRawMessagesBatchContext(ctx, []email.ID{"m1", "m2"}); !errors.Is(err, context.Canceled) {
		t.Errorf("GetRawMessagesBatchContext error = %v, want context.Canceled", err)
	}
	if _, err := client.GetMessageSummaryContext(ctx, "m1"); !errors.Is(err, context.Canceled) {
		t.Errorf("GetMessageSummaryContext error = %v, want context.Canceled", err)
	}
	if _, err := client.GetMessageSummariesBatchContext(ctx, []email.ID{"m1", "m2"}); !errors.Is(err, context.Canceled) {
		t.Errorf("GetMessageSummariesBatchContext error = %v, want context.Canceled", err)
	}
	if err := client.PrefetchMessagesContext(ctx, []email.ID{"m1", "m2"}); !errors.Is(err, context.Canceled) {
		t.Errorf("PrefetchMessagesContext error = %v, want context.Canceled", err)
	}
}
