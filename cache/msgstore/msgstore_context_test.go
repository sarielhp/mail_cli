package msgstore

import (
	"context"
	"errors"
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"github.com/sarielhp/mail_cli/email"
	"sync"
	"testing"
)

func TestPrefetchBatchedStreamingYieldsAndStores(t *testing.T) {
	dir := t.TempDir()
	b := &batchFetcher{fakeFetcher: newFetcher("b1", "b2")}
	s := New(cachedir.At(dir), b)

	if err := s.Prefetch([]email.ID{"b1", "b2"}); err != nil {
		t.Fatalf("Prefetch batched: %v", err)
	}
	if !s.isCached("b1") || !s.isCached("b2") {
		t.Errorf("expected b1 and b2 to be cached after streaming prefetch")
	}
}

type retryFetcher struct {
	mu        sync.Mutex
	callCount map[email.ID]int
	failOn    map[email.ID]int
}

func newRetryFetcher() *retryFetcher {
	return &retryFetcher{
		callCount: make(map[email.ID]int),
		failOn:    make(map[email.ID]int),
	}
}

func (r *retryFetcher) FetchRaw(_ context.Context, msgID email.ID) ([]byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.callCount[msgID]++
	if r.callCount[msgID] <= r.failOn[msgID] {
		return nil, errors.New("transient failure")
	}
	return []byte(rawFor(msgID)), nil
}

func TestRawBatchTransientPrefetchErrorRecoveredOnSecondPass(t *testing.T) {
	f := newRetryFetcher()
	f.failOn["t1"] = 1
	s := New(cachedir.At(t.TempDir()), f)

	got, err := s.RawBatch([]email.ID{"t1"})
	if err != nil {
		t.Fatalf("RawBatch failed despite second-pass success: %v", err)
	}
	if len(got) != 1 || string(got[0]) != rawFor("t1") {
		t.Fatalf("RawBatch returned invalid payload: %v", got)
	}
}

func TestSummariesTransientPrefetchErrorRecoveredOnSecondPass(t *testing.T) {
	f := newRetryFetcher()
	f.failOn["t1"] = 1
	s := New(cachedir.At(t.TempDir()), f)

	got, err := s.Summaries([]email.ID{"t1"})
	if err != nil {
		t.Fatalf("Summaries failed despite second-pass success: %v", err)
	}
	if len(got) != 1 || got[0] == nil || got[0].Subject != "message t1" {
		t.Fatalf("Summaries returned invalid summary: %v", got)
	}
}

func TestRawBatchNoCacheDirDoesNotDoubleFetch(t *testing.T) {
	f := newFetcher("m1", "m2")
	s := New(cachedir.At(""), f)

	got, err := s.RawBatch([]email.ID{"m1", "m2"})
	if err != nil {
		t.Fatalf("RawBatch: %v", err)
	}
	if len(got) != 2 || string(got[0]) != rawFor("m1") || string(got[1]) != rawFor("m2") {
		t.Fatalf("RawBatch unexpected result: %v", got)
	}
	if asked := f.asked(); len(asked) != 2 {
		t.Errorf("backend asked %d times for 2 messages without cache dir, want 2: %v", len(asked), asked)
	}
}

func TestSummariesNoCacheDirDoesNotDoubleFetch(t *testing.T) {
	f := newFetcher("s1", "s2")
	s := New(cachedir.At(""), f)

	got, err := s.Summaries([]email.ID{"s1", "s2"})
	if err != nil {
		t.Fatalf("Summaries: %v", err)
	}
	if len(got) != 2 || got[0] == nil || got[1] == nil {
		t.Fatalf("Summaries unexpected result: %v", got)
	}
	if asked := f.asked(); len(asked) != 2 {
		t.Errorf("backend asked %d times for 2 summaries without cache dir, want 2: %v", len(asked), asked)
	}
}

func TestRawContextCancellation(t *testing.T) {
	f := newFetcher("c1")
	s := New(cachedir.At(t.TempDir()), f)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := s.RawContext(ctx, "c1")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("RawContext = %v, want %v", err, context.Canceled)
	}
	if asked := f.asked(); len(asked) != 0 {
		t.Errorf("backend contacted despite cancelled context: %v", asked)
	}
}

// assertContextCancellationRejected runs a context-aware batch fetch, op, with
// an already-cancelled context and requires it to report cancellation rather
// than contact the backend.
func assertContextCancellationRejected[T any](t *testing.T, name string, op func(context.Context, []email.ID) (T, error)) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := op(ctx, []email.ID{"c1"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("%s = %v, want %v", name, err, context.Canceled)
	}
}

func TestRawBatchContextCancellation(t *testing.T) {
	s := New(cachedir.At(t.TempDir()), newFetcher("c1"))

	assertContextCancellationRejected(t, "RawBatchContext", s.RawBatchContext)
}

func TestSummaryContextCancellation(t *testing.T) {
	f := newFetcher("c1")
	s := New(cachedir.At(t.TempDir()), f)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := s.SummaryContext(ctx, "c1")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("SummaryContext = %v, want %v", err, context.Canceled)
	}
}

func TestSummariesContextCancellation(t *testing.T) {
	s := New(cachedir.At(t.TempDir()), newFetcher("c1"))

	assertContextCancellationRejected(t, "SummariesContext", s.SummariesContext)
}

type deadlineCheckingFetcher struct {
	mu          sync.Mutex
	hasDeadline bool
}

func (d *deadlineCheckingFetcher) FetchRaw(ctx context.Context, msgID email.ID) ([]byte, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, ok := ctx.Deadline()
	d.hasDeadline = ok
	return []byte(rawFor(msgID)), nil
}

func (d *deadlineCheckingFetcher) checked() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.hasDeadline
}

func TestLegacyMethodsEnforceDeadline(t *testing.T) {
	checkDeadline := func(name string, op func(s *Store)) {
		t.Helper()
		fetcher := &deadlineCheckingFetcher{}
		store := New(cachedir.At(t.TempDir()), fetcher)
		op(store)
		if !fetcher.checked() {
			t.Errorf("%s legacy method did not propagate a context with deadline", name)
		}
	}

	checkDeadline("Raw", func(s *Store) { _, _ = s.Raw("m1") })
	checkDeadline("RawBatch", func(s *Store) { _, _ = s.RawBatch([]email.ID{"m1"}) })
	checkDeadline("Summary", func(s *Store) { _, _ = s.Summary("m1") })
	checkDeadline("Summaries", func(s *Store) { _, _ = s.Summaries([]email.ID{"m1"}) })
	checkDeadline("Prefetch", func(s *Store) { _ = s.Prefetch([]email.ID{"m1"}) })
}
