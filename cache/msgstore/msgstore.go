// Package msgstore serves raw messages by ID, reading whatever the local cache
// already holds and downloading only what is missing.
//
// It sits between cache/msg, which is a purely local reader that returns
// os.ErrNotExist on a miss and knows nothing about servers, and the backends,
// which know how to talk to a server but had each grown a private copy of the
// same "check the cache, pool the misses, store the results" loop. There were
// five such copies across gmail, jmap and outlook, differing only in worker
// count and in how they reported failures. A Store is that loop written once,
// with each backend reduced to a single FetchRaw method.
//
// The package is deliberately a leaf: it depends on cache/msg and email and
// nothing else, so both the backends and the mailclient decorator that wraps
// them can use it without an import cycle.
package msgstore

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"log/slog"
	"sync"
	"time"

	"github.com/sarielhp/mail_cli/cache/msg"
	"github.com/sarielhp/mail_cli/email"
)

// DefaultWorkers bounds concurrent downloads when a backend has no batch API.
//
// This is only a default. Servers differ in what they tolerate, and exceeding
// it is not a slow request but a hard rejection: Microsoft Graph answers 429
// ApplicationThrottled once a mailbox has more than a handful of requests in
// flight. Each backend sets Workers to what its server actually allows.
const DefaultWorkers = 8

// DefaultBatchSize bounds how many IDs are handed to a batch-capable backend at
// once, so a large result set does not become one enormous request.
const DefaultBatchSize = 100

// RawFetcher retrieves one raw RFC 5322 payload from a remote server. This is
// the whole of what a backend must implement to gain caching, batching and
// bounded concurrency.
type RawFetcher interface {
	FetchRaw(ctx context.Context, msgID email.ID) ([]byte, error)
}

// BatchRawFetcher is an optional optimization for backends that can retrieve
// several messages per round trip. JMAP is the reason it exists: fetching one
// message there means an Email/get to resolve a blob ID followed by a blob
// download, so one-at-a-time retrieval costs two round trips per message,
// while a single Email/get can resolve a hundred.
//
// A backend that implements it has FetchRawBatch called instead of the worker
// pool. Payloads are streamed to yield as they arrive, so memory usage is
// bounded by the backend's worker concurrency rather than the batch size.
type BatchRawFetcher interface {
	FetchRawBatch(
		ctx context.Context,
		msgIDs []email.ID,
		yield func(id email.ID, raw []byte) error,
	) error
}

// Store reads messages from a cache directory, downloading misses through
// Fetch. The zero value is not usable; call New.
type Store struct {
	// Dir is the account's download directory. When empty the store keeps
	// nothing and every call goes straight to Fetch, which is what an account
	// with no local cache needs.
	Dir cachedir.Account

	// Fetch retrieves messages the cache does not hold. When nil the store is
	// read-only and a miss is an error rather than a download.
	Fetch RawFetcher

	// Workers bounds concurrent downloads; zero means DefaultWorkers.
	Workers int

	// BatchSize bounds a single batch request; zero means DefaultBatchSize.
	BatchSize int
}

// New returns a Store over dir, downloading misses through fetch. Either may be
// zero: an empty dir disables caching, a nil fetch disables downloading.
func New(account cachedir.Account, fetch RawFetcher) *Store {
	return &Store{Dir: account, Fetch: fetch}
}

func (s *Store) workers() int {
	if s.Workers > 0 {
		return s.Workers
	}
	return DefaultWorkers
}

func (s *Store) batchSize() int {
	if s.BatchSize > 0 {
		return s.BatchSize
	}
	return DefaultBatchSize
}

// ErrNoFetcher is returned when a message is not cached and the store has no
// way to retrieve it.
var ErrNoFetcher = errors.New("message not cached and no fetcher configured")

// DefaultFetchTimeout is the maximum duration legacy non-context operations
// are allowed to run before being cancelled.
const DefaultFetchTimeout = 60 * time.Second

// RawContext returns the full raw payload for one message, reading from the cache when
// possible and downloading and caching it when not, respecting ctx cancellation.
func (s *Store) RawContext(ctx context.Context, msgID email.ID) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return s.readOrFetch(ctx, msgID)
}

// Raw returns the full raw payload for one message, reading from the cache when
// possible and downloading and caching it when not, imposing DefaultFetchTimeout.
func (s *Store) Raw(msgID email.ID) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultFetchTimeout)
	defer cancel()
	return s.RawContext(ctx, msgID)
}

func (s *Store) fetchOne(ctx context.Context, msgID email.ID) ([]byte, error) {
	if s.Fetch == nil {
		return nil, fmt.Errorf("%w: %s", ErrNoFetcher, msgID)
	}
	data, err := s.Fetch.FetchRaw(ctx, msgID)
	if err != nil {
		return nil, fmt.Errorf("fetch message %s: %w", msgID, err)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("fetch message %s: backend returned no data", msgID)
	}
	return data, nil
}

// store writes a downloaded message to the cache. A cache write failure is
// logged rather than returned: the caller asked for the message, and it has the
// message, so failing here would misreport what happened.
func (s *Store) store(msgID email.ID, raw []byte) {
	if s.Dir.IsZero() || len(raw) == 0 {
		return
	}
	if err := msg.Store(s.Dir, msgID, raw, email.ExtractDateOrNow(raw)); err != nil {
		slog.Warn("failed to cache fetched message",
			slog.String("id", string(msgID)), slog.Any("error", err))
	}
}

// Prefetch makes sure every ID is present in the cache, downloading only the
// ones that are missing, imposing DefaultFetchTimeout.
func (s *Store) Prefetch(msgIDs []email.ID) error {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultFetchTimeout)
	defer cancel()
	return s.PrefetchContext(ctx, msgIDs)
}

// PrefetchContext makes sure every ID is present in the cache, downloading only the
// ones that are missing, while respecting context cancellation.
func (s *Store) PrefetchContext(ctx context.Context, msgIDs []email.ID) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	missing := s.missing(msgIDs)
	if len(missing) == 0 {
		return nil
	}
	if s.Fetch == nil {
		return fmt.Errorf("%w: %d message(s)", ErrNoFetcher, len(missing))
	}
	if batcher, ok := s.Fetch.(BatchRawFetcher); ok {
		return s.prefetchBatched(ctx, batcher, missing)
	}
	return s.prefetchPooled(ctx, missing)
}

func (s *Store) isCached(id email.ID) bool {
	if s.Dir.IsZero() {
		return false
	}
	exists, err := msg.Exists(s.Dir, id)
	return err == nil && exists
}

// missing returns the deduplicated IDs that are not already cached, in input
// order. With no cache directory nothing is cached, so everything is missing.
func (s *Store) missing(msgIDs []email.ID) []email.ID {
	seen := make(map[email.ID]bool, len(msgIDs))
	var out []email.ID
	for _, id := range msgIDs {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		if s.isCached(id) {
			continue
		}
		out = append(out, id)
	}
	return out
}

func (s *Store) prefetchBatched(ctx context.Context, batcher BatchRawFetcher, missing []email.ID) error {
	var errs []error
	for chunk := range slicesChunk(missing, s.batchSize()) {
		if err := ctx.Err(); err != nil {
			errs = append(errs, err)
			break
		}
		seen := make(map[email.ID]bool, len(chunk))
		err := batcher.FetchRawBatch(ctx, chunk, func(id email.ID, raw []byte) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			if len(raw) == 0 {
				return fmt.Errorf("fetch message %s: backend returned no data", id)
			}
			seen[id] = true
			s.store(id, raw)
			return nil
		})
		if err != nil {
			errs = append(errs, fmt.Errorf("fetch batch of %d: %w", len(chunk), err))
			continue
		}
		for _, id := range chunk {
			if !seen[id] {
				errs = append(errs, fmt.Errorf("fetch message %s: not returned by backend", id))
			}
		}
	}
	return joinFetchErrors(errs, len(missing))
}

func (s *Store) prefetchPooled(ctx context.Context, missing []email.ID) error {
	workers := min(s.workers(), len(missing))
	tasks := make(chan email.ID, len(missing))
	errCh := make(chan error, len(missing))

	for _, id := range missing {
		tasks <- id
	}
	close(tasks)

	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for id := range tasks {
				if ctx.Err() != nil {
					return
				}
				raw, err := s.fetchOne(ctx, id)
				if err != nil {
					errCh <- err
					continue
				}
				if ctx.Err() != nil {
					return
				}
				s.store(id, raw)
			}
		}()
	}

	wg.Wait()
	close(errCh)

	if err := ctx.Err(); err != nil {
		return err
	}

	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}
	return joinFetchErrors(errs, len(missing))
}

// joinFetchErrors summarizes per-message failures. The count leads because a
// caller usually wants to know how much of a batch survived before it wants the
// individual reasons.
func joinFetchErrors(errs []error, attempted int) error {
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("%d of %d message(s) could not be retrieved: %w",
		len(errs), attempted, errors.Join(errs...))
}

// slicesChunk yields successive chunks of at most size elements.
func slicesChunk(ids []email.ID, size int) func(func([]email.ID) bool) {
	return func(yield func([]email.ID) bool) {
		for start := 0; start < len(ids); start += size {
			end := min(start+size, len(ids))
			if !yield(ids[start:end]) {
				return
			}
		}
	}
}

// RawBatchContext returns raw payloads for every ID, in the order given,
// respecting ctx cancellation. Cached messages are read from disk and the rest
// are downloaded first, if a cache directory is configured.
//
// An entry is nil when that message could not be retrieved, and the error
// describes how many failed. Partial results are returned deliberately: an
// export of fifty messages should not be lost because one of them is gone from
// the server.
//
// Every message is held in memory at once, so use Prefetch for unbounded sets.
func (s *Store) RawBatchContext(ctx context.Context, msgIDs []email.ID) ([][]byte, error) {
	if len(msgIDs) == 0 {
		return nil, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !s.Dir.IsZero() {
		_ = s.PrefetchContext(ctx, msgIDs)
	}

	out := make([][]byte, len(msgIDs))
	var finalErrs []error
	for i, id := range msgIDs {
		if err := ctx.Err(); err != nil {
			finalErrs = append(finalErrs, err)
			break
		}
		data, err := s.readOrFetch(ctx, id)
		if err != nil {
			finalErrs = append(finalErrs, fmt.Errorf("%s: %w", id, err))
			continue
		}
		out[i] = data
	}
	return out, joinFetchErrors(finalErrs, len(msgIDs))
}

// RawBatch returns raw payloads for every ID, in the order given, imposing DefaultFetchTimeout.
// Cached messages are read from disk and the rest are downloaded first.
func (s *Store) RawBatch(msgIDs []email.ID) ([][]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultFetchTimeout)
	defer cancel()
	return s.RawBatchContext(ctx, msgIDs)
}

// readOrFetch prefers the cache, falling back to a direct fetch when the cache
// is unconfigured or when a cached message payload is unreadable.
func (s *Store) readOrFetch(ctx context.Context, msgID email.ID) ([]byte, error) {
	if !s.Dir.IsZero() {
		if exists, _ := msg.Exists(s.Dir, msgID); exists {
			data, err := msg.Read(s.Dir, msgID)
			if err == nil && len(data) > 0 {
				return data, nil
			}
			slog.Debug("cached message unreadable, falling back to fetch",
				slog.String("id", string(msgID)), slog.Any("error", err))
		}
	}
	data, err := s.fetchOne(ctx, msgID)
	if err != nil {
		return nil, err
	}
	s.store(msgID, data)
	return data, nil
}

// SummaryContext returns parsed header metadata for one message, respecting ctx cancellation:
// subject, sender, date, and attachment flags, without the caller touching MIME.
func (s *Store) SummaryContext(ctx context.Context, msgID email.ID) (*email.Email, error) {
	raw, err := s.RawContext(ctx, msgID)
	if err != nil {
		return nil, err
	}
	return summarize(msgID, raw)
}

// Summary returns parsed header metadata for one message, imposing DefaultFetchTimeout:
// subject, sender, date, and attachment flags, without the caller touching MIME.
func (s *Store) Summary(msgID email.ID) (*email.Email, error) {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultFetchTimeout)
	defer cancel()
	return s.SummaryContext(ctx, msgID)
}

// SummariesContext returns header metadata for many messages, in the order given, respecting ctx cancellation.
// Messages that could not be retrieved or parsed are nil, matching RawBatchContext.
func (s *Store) SummariesContext(ctx context.Context, msgIDs []email.ID) ([]*email.Email, error) {
	if len(msgIDs) == 0 {
		return nil, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// Prefetch and then read each message back in turn, rather than going
	// through RawBatch. A summary is a few hundred bytes; RawBatch would hold
	// every full message, attachments included, in memory at once just to
	// produce them -- which an unbounded search would turn into gigabytes.
	if !s.Dir.IsZero() {
		_ = s.PrefetchContext(ctx, msgIDs)
	}

	out := make([]*email.Email, len(msgIDs))
	var finalErrs []error
	for i, id := range msgIDs {
		if err := ctx.Err(); err != nil {
			finalErrs = append(finalErrs, err)
			break
		}
		raw, err := s.readOrFetch(ctx, id)
		if err != nil {
			finalErrs = append(finalErrs, fmt.Errorf("%s: %w", id, err))
			continue
		}
		parsed, pErr := summarize(id, raw)
		if pErr != nil {
			finalErrs = append(finalErrs, fmt.Errorf("%s: %w", id, pErr))
			continue
		}
		out[i] = parsed
	}

	return out, joinFetchErrors(finalErrs, len(msgIDs))
}

// Summaries returns header metadata for many messages, in the order given, imposing DefaultFetchTimeout.
// Messages that could not be retrieved or parsed are nil, matching RawBatch.
func (s *Store) Summaries(msgIDs []email.ID) ([]*email.Email, error) {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultFetchTimeout)
	defer cancel()
	return s.SummariesContext(ctx, msgIDs)
}

func summarize(msgID email.ID, raw []byte) (*email.Email, error) {
	parsed := email.ParseReader(bytes.NewReader(raw), msgID, "")
	if parsed == nil {
		return nil, fmt.Errorf("parse message %s: malformed MIME", msgID)
	}
	if parsed.EmailDate.IsZero() {
		parsed.EmailDate = email.ExtractDateOrNow(raw)
		parsed.FormattedDate = email.FormatEmailDate(parsed.EmailDate)
	}
	return parsed, nil
}
