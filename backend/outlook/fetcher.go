package outlook

import (
	"context"

	"github.com/sarielhp/mail_cli/cache/msgstore"
	"github.com/sarielhp/mail_cli/email"
)

// rawFetcher retrieves raw messages from Microsoft Graph. Graph serves the MIME
// body directly by message ID, so the single-message form is all this backend
// needs; msgstore supplies the pooling the two hand-rolled copies used to do.
type rawFetcher struct {
	client *client
}

func (f rawFetcher) FetchRaw(ctx context.Context, msgID email.ID) ([]byte, error) {
	return f.client.downloadRawMessageContext(ctx, msgID)
}

// graphMailboxConcurrency is what Microsoft Graph allows in flight against one
// mailbox. Going past it does not slow requests down, it fails them with 429
// ApplicationThrottled, so this is a correctness bound rather than a tuning
// knob -- downloads were being lost at the shared default of 8.
const graphMailboxConcurrency = 4

// newMessageStore returns a read-through store over this account's cache.
func (c *client) newMessageStore() *msgstore.Store {
	store := msgstore.New(c.config.AccountCache, rawFetcher{client: c})
	store.Workers = graphMailboxConcurrency
	return store
}

func (c *client) GetRawMessageContext(ctx context.Context, msgID email.ID) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := c.init(); err != nil {
		return nil, err
	}
	return c.newMessageStore().RawContext(ctx, msgID)
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
	if err := c.init(); err != nil {
		return nil, err
	}
	return c.newMessageStore().RawBatchContext(ctx, msgIDs)
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
	if err := c.init(); err != nil {
		return nil, err
	}
	return c.newMessageStore().SummaryContext(ctx, msgID)
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
	if err := c.init(); err != nil {
		return nil, err
	}
	return c.newMessageStore().SummariesContext(ctx, msgIDs)
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
	if err := c.init(); err != nil {
		return err
	}
	return c.newMessageStore().PrefetchContext(ctx, msgIDs)
}

func (c *client) PrefetchMessages(msgIDs []email.ID) error {
	ctx, cancel := context.WithTimeout(context.Background(), msgstore.DefaultFetchTimeout)
	defer cancel()
	return c.PrefetchMessagesContext(ctx, msgIDs)
}
