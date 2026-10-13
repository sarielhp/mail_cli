package jmap

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	jmapemail "git.sr.ht/~rockorager/go-jmap/mail/email"

	"github.com/sarielhp/mail_cli/cache/msgstore"
	mailemail "github.com/sarielhp/mail_cli/email"
)

// blobWorkers bounds concurrent blob downloads inside one batch. JMAP resolves
// every blob ID in a single Email/get, but each body is a separate HTTP
// download, so the transfers are still worth overlapping.
const blobWorkers = 5

// rawFetcher retrieves raw messages from a JMAP server.
//
// It implements msgstore.BatchRawFetcher as well as the single-message form
// because JMAP cannot address a message body by message ID: a body lives at a
// blob ID that only Email/get can supply. Fetching one message therefore costs
// two round trips, while one Email/get resolves a whole batch, so the batch
// path is the one that matters here.
type rawFetcher struct {
	client *client
}

func (f rawFetcher) FetchRaw(ctx context.Context, msgID mailemail.ID) ([]byte, error) {
	var raw []byte
	err := f.FetchRawBatch(ctx, []mailemail.ID{msgID}, func(id mailemail.ID, data []byte) error {
		if id == msgID {
			raw = data
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return nil, fmt.Errorf("message %s not returned by server", msgID)
	}
	return raw, nil
}

func (f rawFetcher) FetchRawBatch(
	ctx context.Context,
	msgIDs []mailemail.ID,
	yield func(id mailemail.ID, raw []byte) error,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	details, err := f.client.fetchEmailDetailsContext(ctx, toJmapIDs(msgIDs), []string{"id", "blobId"})
	if err != nil {
		return fmt.Errorf("resolve blob IDs for %d message(s): %w", len(msgIDs), err)
	}
	return f.downloadBlobs(ctx, details, yield)
}

// downloadBlobs fetches each message body concurrently and streams it to yield.
// A message that fails is left out; msgstore reports omissions against the IDs
// it asked for, so a single bad blob does not fail the batch.
func (f rawFetcher) downloadBlobs(
	ctx context.Context,
	details []*jmapemail.Email,
	yield func(id mailemail.ID, raw []byte) error,
) error {
	if len(details) == 0 {
		return nil
	}
	tasks := make(chan *jmapemail.Email, len(details))
	for _, em := range details {
		if em != nil && em.BlobID != "" {
			tasks <- em
		}
	}
	close(tasks)

	var wg sync.WaitGroup
	var (
		yieldMu  sync.Mutex
		firstErr error
	)
	setErr := func(err error) {
		yieldMu.Lock()
		if firstErr == nil {
			firstErr = err
		}
		yieldMu.Unlock()
	}

	workers := min(blobWorkers, len(details))
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					slog.Error("panic downloading JMAP blob", slog.Any("recover", r))
				}
			}()
			for em := range tasks {
				if ctx.Err() != nil {
					setErr(ctx.Err())
					return
				}
				yieldMu.Lock()
				stopped := firstErr != nil
				yieldMu.Unlock()
				if stopped {
					return
				}

				idStr := string(em.ID)
				raw, err := f.client.downloadBlobContext(ctx, string(f.client.accID), string(em.BlobID), idStr+".eml", "message/rfc822")
				if err != nil {
					if ctx.Err() != nil {
						setErr(ctx.Err())
						return
					}
					slog.Warn("failed to download JMAP blob", slog.String("id", idStr), slog.Any("error", err))
					continue
				}
				if len(raw) == 0 {
					slog.Warn("JMAP blob download returned empty body", slog.String("id", idStr))
					continue
				}

				yieldMu.Lock()
				if firstErr == nil {
					if yErr := yield(mailemail.ID(idStr), raw); yErr != nil {
						firstErr = yErr
					}
				}
				yieldMu.Unlock()
			}
		}()
	}
	wg.Wait()
	return firstErr
}

// newMessageStore returns a read-through store over this account's cache.
func (c *client) newMessageStore() *msgstore.Store {
	store := msgstore.New(c.config.AccountCache, rawFetcher{client: c})
	store.Workers = blobWorkers
	return store
}

var _ msgstore.BatchRawFetcher = rawFetcher{}

func (c *client) GetRawMessageContext(ctx context.Context, msgID mailemail.ID) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := c.initContext(ctx); err != nil {
		return nil, err
	}
	return c.newMessageStore().RawContext(ctx, msgID)
}

func (c *client) GetRawMessage(msgID mailemail.ID) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), msgstore.DefaultFetchTimeout)
	defer cancel()
	return c.GetRawMessageContext(ctx, msgID)
}

func (c *client) GetRawMessagesBatchContext(ctx context.Context, msgIDs []mailemail.ID) ([][]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := c.initContext(ctx); err != nil {
		return nil, err
	}
	return c.newMessageStore().RawBatchContext(ctx, msgIDs)
}

func (c *client) GetRawMessagesBatch(msgIDs []mailemail.ID) ([][]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), msgstore.DefaultFetchTimeout)
	defer cancel()
	return c.GetRawMessagesBatchContext(ctx, msgIDs)
}

func (c *client) GetMessageSummaryContext(ctx context.Context, msgID mailemail.ID) (*mailemail.Email, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := c.initContext(ctx); err != nil {
		return nil, err
	}
	return c.newMessageStore().SummaryContext(ctx, msgID)
}

func (c *client) GetMessageSummary(msgID mailemail.ID) (*mailemail.Email, error) {
	ctx, cancel := context.WithTimeout(context.Background(), msgstore.DefaultFetchTimeout)
	defer cancel()
	return c.GetMessageSummaryContext(ctx, msgID)
}

func (c *client) GetMessageSummariesBatchContext(ctx context.Context, msgIDs []mailemail.ID) ([]*mailemail.Email, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := c.initContext(ctx); err != nil {
		return nil, err
	}
	return c.newMessageStore().SummariesContext(ctx, msgIDs)
}

func (c *client) GetMessageSummariesBatch(msgIDs []mailemail.ID) ([]*mailemail.Email, error) {
	ctx, cancel := context.WithTimeout(context.Background(), msgstore.DefaultFetchTimeout)
	defer cancel()
	return c.GetMessageSummariesBatchContext(ctx, msgIDs)
}

func (c *client) PrefetchMessagesContext(ctx context.Context, msgIDs []mailemail.ID) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := c.initContext(ctx); err != nil {
		return err
	}
	return c.newMessageStore().PrefetchContext(ctx, msgIDs)
}

func (c *client) PrefetchMessages(msgIDs []mailemail.ID) error {
	ctx, cancel := context.WithTimeout(context.Background(), msgstore.DefaultFetchTimeout)
	defer cancel()
	return c.PrefetchMessagesContext(ctx, msgIDs)
}
