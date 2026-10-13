package gmail

import (
	"context"
	"fmt"

	gmailapi "google.golang.org/api/gmail/v1"

	"github.com/sarielhp/mail_cli/cache/msgstore"
	"github.com/sarielhp/mail_cli/email"
)

// rawFetcher retrieves raw messages over the Gmail REST API. It is the whole of
// what this backend has to provide for msgstore to handle cache lookups,
// deduplication, bounded concurrency and storage.
type rawFetcher struct {
	srv *gmailapi.Service
}

func (f rawFetcher) FetchRaw(ctx context.Context, msgID email.ID) ([]byte, error) {
	res, err := f.srv.Users.Messages.Get("me", string(msgID)).Format("raw").Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("download message %s: %w", msgID, err)
	}
	raw, err := DecodeGmailRaw(res.Raw)
	if err != nil {
		return nil, fmt.Errorf("decode message %s: %w", msgID, err)
	}
	return raw, nil
}

// newMessageStore returns a read-through store over this account's cache,
// downloading anything it does not already hold.
func newMessageStore(config *Config, srv *gmailapi.Service) *msgstore.Store {
	return msgstore.New(config.AccountCache, rawFetcher{srv: srv})
}

func (c *client) messageStoreContext(ctx context.Context) (*msgstore.Store, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	srv, err := GetGmailServiceContext(ctx, c.rep, c.config)
	if err != nil {
		return nil, err
	}
	return newMessageStore(c.config, srv), nil
}

func (c *client) GetRawMessageContext(ctx context.Context, msgID email.ID) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	store, err := c.messageStoreContext(ctx)
	if err != nil {
		return nil, err
	}
	return store.RawContext(ctx, msgID)
}

func (c *client) GetRawMessage(msgID email.ID) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), msgstore.DefaultFetchTimeout)
	defer cancel()
	return c.GetRawMessageContext(ctx, msgID)
}

func (c *client) GetRawMessagesBatchContext(ctx context.Context, msgIDs []email.ID) ([][]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	store, err := c.messageStoreContext(ctx)
	if err != nil {
		return nil, err
	}
	return store.RawBatchContext(ctx, msgIDs)
}

func (c *client) GetRawMessagesBatch(msgIDs []email.ID) ([][]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), msgstore.DefaultFetchTimeout)
	defer cancel()
	return c.GetRawMessagesBatchContext(ctx, msgIDs)
}

func (c *client) GetMessageSummaryContext(ctx context.Context, msgID email.ID) (*email.Email, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	store, err := c.messageStoreContext(ctx)
	if err != nil {
		return nil, err
	}
	return store.SummaryContext(ctx, msgID)
}

func (c *client) GetMessageSummary(msgID email.ID) (*email.Email, error) {
	ctx, cancel := context.WithTimeout(context.Background(), msgstore.DefaultFetchTimeout)
	defer cancel()
	return c.GetMessageSummaryContext(ctx, msgID)
}

func (c *client) GetMessageSummariesBatchContext(ctx context.Context, msgIDs []email.ID) ([]*email.Email, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	store, err := c.messageStoreContext(ctx)
	if err != nil {
		return nil, err
	}
	return store.SummariesContext(ctx, msgIDs)
}

func (c *client) GetMessageSummariesBatch(msgIDs []email.ID) ([]*email.Email, error) {
	ctx, cancel := context.WithTimeout(context.Background(), msgstore.DefaultFetchTimeout)
	defer cancel()
	return c.GetMessageSummariesBatchContext(ctx, msgIDs)
}

func (c *client) PrefetchMessagesContext(ctx context.Context, msgIDs []email.ID) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	store, err := c.messageStoreContext(ctx)
	if err != nil {
		return err
	}
	return store.PrefetchContext(ctx, msgIDs)
}

func (c *client) PrefetchMessages(msgIDs []email.ID) error {
	ctx, cancel := context.WithTimeout(context.Background(), msgstore.DefaultFetchTimeout)
	defer cancel()
	return c.PrefetchMessagesContext(ctx, msgIDs)
}
