package msgstore

import (
	"context"
	"errors"
	"fmt"
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"github.com/sarielhp/mail_cli/email"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sarielhp/mail_cli/cache/msg"
)

// fakeFetcher stands in for a backend. It records what was asked for, so a test
// can prove the store did not go to the network for something it already had.
type fakeFetcher struct {
	mu        sync.Mutex
	messages  map[email.ID][]byte
	requested []email.ID
	failWith  map[email.ID]error
	err       error

	// maxConcurrent records the high-water mark of simultaneous calls, which is
	// how the worker bound is checked.
	inFlight      int
	maxConcurrent int
	delay         time.Duration
}

func newFetcher(ids ...string) *fakeFetcher {
	f := &fakeFetcher{messages: make(map[email.ID][]byte), failWith: make(map[email.ID]error)}
	for _, id := range ids {
		f.messages[email.ID(id)] = []byte(rawFor(email.ID(id)))
	}
	return f
}

func rawFor(id email.ID) string {
	s := string(id)
	return "From: alice@example.com\r\n" +
		"To: bob@example.com\r\n" +
		"Subject: message " + s + "\r\n" +
		"Date: Tue, 02 Sep 2025 10:04:05 +0000\r\n" +
		"Message-ID: <" + s + "@example.com>\r\n\r\n" +
		"body of " + s + "\r\n"
}

func (f *fakeFetcher) FetchRaw(ctx context.Context, msgID email.ID) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f.mu.Lock()
	f.requested = append(f.requested, msgID)
	f.inFlight++
	if f.inFlight > f.maxConcurrent {
		f.maxConcurrent = f.inFlight
	}
	f.mu.Unlock()

	defer func() {
		f.mu.Lock()
		f.inFlight--
		f.mu.Unlock()
	}()

	if f.delay > 0 {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(f.delay):
		}
	}

	if f.err != nil {
		return nil, f.err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if err, bad := f.failWith[msgID]; bad {
		return nil, err
	}
	data, ok := f.messages[msgID]
	if !ok {
		return nil, fmt.Errorf("no such message %q", msgID)
	}
	return data, nil
}

func (f *fakeFetcher) asked() []email.ID {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]email.ID(nil), f.requested...)
}

// batchFetcher also implements BatchRawFetcher, standing in for JMAP.
type batchFetcher struct {
	*fakeFetcher
	mu        sync.Mutex
	batchSize []int
	calls     int
}

func (b *batchFetcher) FetchRawBatch(
	ctx context.Context,
	msgIDs []email.ID,
	yield func(id email.ID, raw []byte) error,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	b.mu.Lock()
	b.calls++
	b.batchSize = append(b.batchSize, len(msgIDs))
	b.mu.Unlock()

	if b.fakeFetcher.err != nil {
		return b.fakeFetcher.err
	}
	for _, id := range msgIDs {
		if _, bad := b.failWith[id]; bad {
			continue
		}
		if data, ok := b.messages[id]; ok {
			if err := yield(id, data); err != nil {
				return err
			}
		}
	}
	return nil
}

var errRemote = errors.New("remote refused")

func TestRawReadsFromCacheWithoutFetching(t *testing.T) {
	dir := t.TempDir()
	if err := msg.Store(cachedir.At(dir), "cached1", []byte(rawFor("cached1")), time.Now()); err != nil {
		t.Fatalf("seeding cache: %v", err)
	}
	f := newFetcher("cached1")
	s := New(cachedir.At(dir), f)

	got, err := s.Raw("cached1")
	if err != nil {
		t.Fatalf("Raw: %v", err)
	}
	if string(got) != rawFor("cached1") {
		t.Errorf("Raw returned unexpected bytes")
	}
	if asked := f.asked(); len(asked) != 0 {
		t.Errorf("fetched %v from the backend for an already-cached message", asked)
	}
}

func TestRawFetchesAndCachesOnMiss(t *testing.T) {
	dir := t.TempDir()
	f := newFetcher("remote1")
	s := New(cachedir.At(dir), f)

	got, err := s.Raw("remote1")
	if err != nil {
		t.Fatalf("Raw: %v", err)
	}
	if string(got) != rawFor("remote1") {
		t.Error("Raw returned unexpected bytes")
	}
	if asked := f.asked(); len(asked) != 1 || asked[0] != "remote1" {
		t.Errorf("backend asked for %v, want [remote1]", asked)
	}
	// The point of the store: a second read must not hit the network.
	if _, err := s.Raw("remote1"); err != nil {
		t.Fatalf("second Raw: %v", err)
	}
	if asked := f.asked(); len(asked) != 1 {
		t.Errorf("backend asked %d times, want 1 — the fetched message was not cached", len(asked))
	}
	if exists, _ := msg.Exists(cachedir.At(dir), "remote1"); !exists {
		t.Error("fetched message was not written to the cache")
	}
}

// A downloaded message is filed under its own Date header, not the download
// time, so a mailbox groups by when its mail was actually sent.
func TestFetchedMessageIsFiledUnderItsOwnDate(t *testing.T) {
	dir := t.TempDir()
	s := New(cachedir.At(dir), newFetcher("dated1"))

	if _, err := s.Raw("dated1"); err != nil {
		t.Fatalf("Raw: %v", err)
	}
	info, err := msg.GetInfo(cachedir.At(dir), "dated1")
	if err != nil {
		t.Fatalf("GetInfo: %v", err)
	}
	if info.Date.Year() != 2025 || info.Date.Month() != time.September || info.Date.Day() != 2 {
		t.Errorf("stored date = %v, want 2025-09-02 from the Date header", info.Date)
	}
}

func TestRawPropagatesFetchError(t *testing.T) {
	f := newFetcher()
	f.err = errRemote
	s := New(cachedir.At(t.TempDir()), f)

	if _, err := s.Raw("nope"); !errors.Is(err, errRemote) {
		t.Errorf("Raw error = %v, want %v", err, errRemote)
	}
}

func TestRawWithoutFetcherFailsOnMiss(t *testing.T) {
	s := New(cachedir.At(t.TempDir()), nil)
	_, err := s.Raw("absent")
	if !errors.Is(err, ErrNoFetcher) {
		t.Errorf("Raw error = %v, want ErrNoFetcher", err)
	}
}

// With no cache directory the store is a pass-through, which is what an
// account whose cache is disabled needs.
func TestRawWithoutCacheDirAlwaysFetches(t *testing.T) {
	f := newFetcher("x1")
	s := New(cachedir.At(""), f)

	for range 2 {
		if _, err := s.Raw("x1"); err != nil {
			t.Fatalf("Raw: %v", err)
		}
	}
	if asked := f.asked(); len(asked) != 2 {
		t.Errorf("backend asked %d times, want 2 with caching disabled", len(asked))
	}
}

// An indexed message whose .eml has been deleted is a miss, not a hard failure.
func TestRawRefetchesWhenCachedFileIsGone(t *testing.T) {
	dir := t.TempDir()
	if err := msg.Store(cachedir.At(dir), "gone1", []byte(rawFor("gone1")), time.Now()); err != nil {
		t.Fatalf("seeding cache: %v", err)
	}
	if err := msg.Delete(cachedir.At(dir), "gone1"); err != nil {
		t.Fatalf("deleting cached file: %v", err)
	}

	f := newFetcher("gone1")
	s := New(cachedir.At(dir), f)
	got, err := s.Raw("gone1")
	if err != nil {
		t.Fatalf("Raw: %v", err)
	}
	if string(got) != rawFor("gone1") {
		t.Error("Raw returned unexpected bytes after refetch")
	}
}

func TestPrefetchOnlyDownloadsMissing(t *testing.T) {
	dir := t.TempDir()
	if err := msg.Store(cachedir.At(dir), "have1", []byte(rawFor("have1")), time.Now()); err != nil {
		t.Fatalf("seeding cache: %v", err)
	}
	f := newFetcher("have1", "want1", "want2")
	s := New(cachedir.At(dir), f)

	if err := s.Prefetch([]email.ID{"have1", "want1", "want2"}); err != nil {
		t.Fatalf("Prefetch: %v", err)
	}
	asked := f.asked()
	if len(asked) != 2 {
		t.Fatalf("backend asked for %v, want only the two missing IDs", asked)
	}
	for _, id := range asked {
		if id == "have1" {
			t.Error("backend was asked for an already-cached message")
		}
	}
}

func TestPrefetchDeduplicatesIDs(t *testing.T) {
	f := newFetcher("dup1")
	s := New(cachedir.At(t.TempDir()), f)

	if err := s.Prefetch([]email.ID{"dup1", "dup1", "", "dup1"}); err != nil {
		t.Fatalf("Prefetch: %v", err)
	}
	if asked := f.asked(); len(asked) != 1 {
		t.Errorf("backend asked %d times for a repeated ID, want 1 (%v)", len(asked), asked)
	}
}

func TestPrefetchEmptyInputIsNoOp(t *testing.T) {
	f := newFetcher()
	s := New(cachedir.At(t.TempDir()), f)
	if err := s.Prefetch(nil); err != nil {
		t.Errorf("Prefetch(nil) = %v, want nil", err)
	}
	if len(f.asked()) != 0 {
		t.Error("empty Prefetch contacted the backend")
	}
}

// One bad message must not lose the rest of the batch.
func TestPrefetchReportsPartialFailure(t *testing.T) {
	dir := t.TempDir()
	f := newFetcher("g1", "g2", "g3")
	f.failWith["bad1"] = errRemote
	s := New(cachedir.At(dir), f)
	s.Workers = 2

	err := s.Prefetch([]email.ID{"g1", "bad1", "g2", "g3"})
	if err == nil {
		t.Fatal("Prefetch returned nil despite a failed message")
	}
	if !errors.Is(err, errRemote) {
		t.Errorf("error %v does not wrap the backend error", err)
	}
	if !strings.Contains(err.Error(), "1 of 4") {
		t.Errorf("error %q should say how many of how many failed", err)
	}
	for _, id := range []email.ID{"g1", "g2", "g3"} {
		if exists, _ := msg.Exists(cachedir.At(dir), id); !exists {
			t.Errorf("%s was not cached; one failure lost the rest of the batch", id)
		}
	}
}

func TestPrefetchWithoutFetcherFails(t *testing.T) {
	s := New(cachedir.At(t.TempDir()), nil)
	err := s.Prefetch([]email.ID{"a", "b"})
	if !errors.Is(err, ErrNoFetcher) {
		t.Errorf("Prefetch error = %v, want ErrNoFetcher", err)
	}
}

func TestPrefetchRespectsWorkerBound(t *testing.T) {
	var ids []email.ID
	f := newFetcher()
	for i := range 20 {
		id := email.ID(fmt.Sprintf("m%02d", i))
		ids = append(ids, id)
		f.messages[id] = []byte(rawFor(id))
	}
	f.delay = 2 * time.Millisecond

	s := New(cachedir.At(t.TempDir()), f)
	s.Workers = 3
	if err := s.Prefetch(email.IDs(email.IDStrings(ids))); err != nil {
		t.Fatalf("Prefetch: %v", err)
	}

	f.mu.Lock()
	peak := f.maxConcurrent
	f.mu.Unlock()
	if peak > 3 {
		t.Errorf("peak concurrency was %d, want at most the configured 3", peak)
	}
	if peak < 2 {
		t.Errorf("peak concurrency was %d; downloads did not run in parallel", peak)
	}
}

func TestRawBatchPreservesInputOrder(t *testing.T) {
	dir := t.TempDir()
	if err := msg.Store(cachedir.At(dir), "b2", []byte(rawFor("b2")), time.Now()); err != nil {
		t.Fatalf("seeding cache: %v", err)
	}
	s := New(cachedir.At(dir), newFetcher("b1", "b2", "b3"))

	ids := []email.ID{"b3", "b1", "b2"}
	got, err := s.RawBatch(ids)
	if err != nil {
		t.Fatalf("RawBatch: %v", err)
	}
	if len(got) != len(ids) {
		t.Fatalf("returned %d entries, want %d", len(got), len(ids))
	}
	for i, id := range ids {
		if string(got[i]) != rawFor(id) {
			t.Errorf("entry %d is not %s; order was not preserved", i, id)
		}
	}
}

func TestRawBatchReturnsPartialResultsWithError(t *testing.T) {
	f := newFetcher("p1", "p3")
	f.failWith["p2"] = errRemote
	s := New(cachedir.At(t.TempDir()), f)

	got, err := s.RawBatch([]email.ID{"p1", "p2", "p3"})
	if err == nil {
		t.Fatal("RawBatch returned nil error despite a failed message")
	}
	if len(got) != 3 {
		t.Fatalf("returned %d entries, want 3 with a nil hole", len(got))
	}
	if string(got[0]) != rawFor("p1") || string(got[2]) != rawFor("p3") {
		t.Error("surviving messages were not returned alongside the error")
	}
	if got[1] != nil {
		t.Error("failed message should be a nil entry")
	}
}

func TestRawBatchEmptyInput(t *testing.T) {
	s := New(cachedir.At(t.TempDir()), newFetcher())
	got, err := s.RawBatch(nil)
	if err != nil || got != nil {
		t.Errorf("RawBatch(nil) = %v, %v; want nil, nil", got, err)
	}
}

func TestBatchCapableBackendIsUsedInsteadOfThePool(t *testing.T) {
	f := newFetcher("j1", "j2", "j3")
	b := &batchFetcher{fakeFetcher: f}
	s := New(cachedir.At(t.TempDir()), b)

	if err := s.Prefetch([]email.ID{"j1", "j2", "j3"}); err != nil {
		t.Fatalf("Prefetch: %v", err)
	}
	b.mu.Lock()
	calls, sizes := b.calls, b.batchSize
	b.mu.Unlock()

	if calls != 1 {
		t.Errorf("made %d batch calls, want 1", calls)
	}
	if len(sizes) != 1 || sizes[0] != 3 {
		t.Errorf("batch sizes %v, want one call of 3", sizes)
	}
	if asked := f.asked(); len(asked) != 0 {
		t.Errorf("per-message FetchRaw was used %d times despite a batch-capable backend", len(asked))
	}
}

func TestBatchRequestsAreChunked(t *testing.T) {
	f := newFetcher()
	var ids []email.ID
	for i := range 250 {
		id := email.ID(fmt.Sprintf("c%03d", i))
		ids = append(ids, id)
		f.messages[id] = []byte(rawFor(id))
	}
	b := &batchFetcher{fakeFetcher: f}
	s := New(cachedir.At(t.TempDir()), b)
	s.BatchSize = 100

	if err := s.Prefetch(email.IDs(email.IDStrings(ids))); err != nil {
		t.Fatalf("Prefetch: %v", err)
	}
	b.mu.Lock()
	sizes := b.batchSize
	b.mu.Unlock()

	want := []int{100, 100, 50}
	if len(sizes) != len(want) {
		t.Fatalf("made %d batch calls (%v), want %d", len(sizes), sizes, len(want))
	}
	for i, n := range want {
		if sizes[i] != n {
			t.Errorf("batch %d had %d IDs, want %d", i, sizes[i], n)
		}
	}
}

// A batch backend that silently omits a message must be reported, not treated
// as success.
func TestBatchOmissionIsReported(t *testing.T) {
	f := newFetcher("k1", "k3")
	f.failWith["k2"] = errRemote
	b := &batchFetcher{fakeFetcher: f}
	s := New(cachedir.At(t.TempDir()), b)

	err := s.Prefetch([]email.ID{"k1", "k2", "k3"})
	if err == nil {
		t.Fatal("Prefetch returned nil despite an omitted message")
	}
	if !strings.Contains(err.Error(), "k2") {
		t.Errorf("error %q should name the omitted message", err)
	}
}

func TestSummaryParsesHeaders(t *testing.T) {
	s := New(cachedir.At(t.TempDir()), newFetcher("s1"))

	got, err := s.Summary("s1")
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if got.ID != "s1" {
		t.Errorf("ID = %q, want s1", got.ID)
	}
	if got.Subject != "message s1" {
		t.Errorf("Subject = %q, want %q", got.Subject, "message s1")
	}
	if got.FromEmail != "alice@example.com" {
		t.Errorf("FromEmail = %q, want alice@example.com", got.FromEmail)
	}
	if got.EmailDate.Year() != 2025 || got.EmailDate.Month() != time.September {
		t.Errorf("EmailDate = %v, want September 2025", got.EmailDate)
	}
	if got.MessageID != "<s1@example.com>" {
		t.Errorf("MessageID = %q", got.MessageID)
	}
}

func TestSummariesPreserveOrderAndHoleOnFailure(t *testing.T) {
	f := newFetcher("t1", "t3")
	f.failWith["t2"] = errRemote
	s := New(cachedir.At(t.TempDir()), f)

	got, err := s.Summaries([]email.ID{"t1", "t2", "t3"})
	if err == nil {
		t.Fatal("Summaries returned nil error despite a failed message")
	}
	if len(got) != 3 {
		t.Fatalf("returned %d summaries, want 3", len(got))
	}
	if got[0] == nil || got[0].Subject != "message t1" {
		t.Errorf("entry 0 = %+v, want the t1 summary", got[0])
	}
	if got[1] != nil {
		t.Error("failed message should be a nil summary")
	}
	if got[2] == nil || got[2].Subject != "message t3" {
		t.Errorf("entry 2 = %+v, want the t3 summary", got[2])
	}
}

func TestSummariesReadFromCacheWithoutFetching(t *testing.T) {
	dir := t.TempDir()
	for _, id := range []email.ID{"u1", "u2"} {
		if err := msg.Store(cachedir.At(dir), id, []byte(rawFor(id)), time.Now()); err != nil {
			t.Fatalf("seeding cache: %v", err)
		}
	}
	f := newFetcher("u1", "u2")
	s := New(cachedir.At(dir), f)

	got, err := s.Summaries([]email.ID{"u1", "u2"})
	if err != nil {
		t.Fatalf("Summaries: %v", err)
	}
	if len(got) != 2 || got[0] == nil || got[1] == nil {
		t.Fatalf("got %+v, want two summaries", got)
	}
	if asked := f.asked(); len(asked) != 0 {
		t.Errorf("contacted the backend for %v despite both being cached", asked)
	}
}

// Concurrent callers sharing one store must not corrupt the cache index.
func TestConcurrentPrefetch(t *testing.T) {
	f := newFetcher()
	var ids []email.ID
	for i := range 40 {
		id := email.ID(fmt.Sprintf("n%03d", i))
		ids = append(ids, id)
		f.messages[id] = []byte(rawFor(id))
	}
	s := New(cachedir.At(t.TempDir()), f)

	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := s.Prefetch(email.IDs(email.IDStrings(ids))); err != nil {
				t.Errorf("Prefetch: %v", err)
			}
		}()
	}
	wg.Wait()

	for _, id := range ids {
		if exists, _ := msg.Exists(s.Dir, id); !exists {
			t.Fatalf("%s missing after concurrent prefetch", id)
		}
	}
}

func TestBulkRetrievalRefetchesUnreadableCachedMessage(t *testing.T) {
	dir := t.TempDir()
	id := "corrupt1"
	if err := msg.Store(cachedir.At(dir), email.ID(id), []byte(rawFor(email.ID(id))), time.Now()); err != nil {
		t.Fatalf("seeding cache: %v", err)
	}

	// Corrupt the cached payload by truncating it to 0 bytes
	now := time.Now()
	datePath := fmt.Sprintf("%04d/%02d/%02d", now.Year(), now.Month(), now.Day())
	emlPath := filepath.Join(dir, "messages", datePath, id+".eml")
	if err := os.WriteFile(emlPath, []byte(""), 0600); err != nil {
		t.Fatalf("corrupting payload: %v", err)
	}

	f := newFetcher(id)
	s := New(cachedir.At(dir), f)

	// RawBatch must notice the unreadable file, fetch it, and return the data
	rawBatch, err := s.RawBatch([]email.ID{email.ID(id)})
	if err != nil {
		t.Fatalf("RawBatch: %v", err)
	}
	if len(rawBatch) != 1 || len(rawBatch[0]) == 0 {
		t.Fatalf("RawBatch returned empty for recoverable message: %+v", rawBatch)
	}
	if string(rawBatch[0]) != rawFor(email.ID(id)) {
		t.Errorf("got %q, want %q", string(rawBatch[0]), rawFor(email.ID(id)))
	}
}

func TestPrefetchContextCancellation(t *testing.T) {
	dir := t.TempDir()
	f := newFetcher("c1", "c2", "c3")
	f.delay = 50 * time.Millisecond
	s := New(cachedir.At(dir), f)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := s.PrefetchContext(ctx, []email.ID{"c1", "c2", "c3"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("PrefetchContext = %v, want %v", err, context.Canceled)
	}
}

func TestPrefetchContextInFlightCancellation(t *testing.T) {
	dir := t.TempDir()
	f := newFetcher("c1", "c2", "c3")
	f.delay = 100 * time.Millisecond
	s := New(cachedir.At(dir), f)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	err := s.PrefetchContext(ctx, []email.ID{"c1", "c2", "c3"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("PrefetchContext = %v, want %v", err, context.Canceled)
	}

	f.mu.Lock()
	inFlight := f.inFlight
	f.mu.Unlock()
	if inFlight != 0 {
		t.Fatalf("expected 0 in-flight workers after PrefetchContext returned, got %d", inFlight)
	}

	for _, id := range []email.ID{"c1", "c2", "c3"} {
		if s.isCached(id) {
			t.Errorf("message %s was cached after cancellation", id)
		}
	}
}

func TestPrefetchBatchedContextCancellation(t *testing.T) {
	dir := t.TempDir()
	b := &batchFetcher{fakeFetcher: newFetcher("b1", "b2")}
	b.delay = 50 * time.Millisecond
	s := New(cachedir.At(dir), b)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := s.PrefetchContext(ctx, []email.ID{"b1", "b2"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("PrefetchContext batched = %v, want %v", err, context.Canceled)
	}
}
