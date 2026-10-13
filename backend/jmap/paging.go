package jmap

import (
	"github.com/sarielhp/mail_cli/backend/paging"

	"git.sr.ht/~rockorager/go-jmap"
	"git.sr.ht/~rockorager/go-jmap/mail/email"
)

// batchLimit is how many messages one Email/query asks for at a time.
const batchLimit = 500

// eachMailboxBatch hands every message in a mailbox to fn, a batch at a time.
//
// The bulk spam operations used to issue a single Email/query with Limit 1000
// and no loop, then report "Successfully purged N" -- so a spam mailbox holding
// more than that was partly processed and wholly reported. The residue then fed
// back into every later run.
//
// The loop that walks it to the end is paging.Converge, which owns the exit
// conditions: it re-queries from position zero rather than advancing a cursor,
// because fn removes messages from the mailbox and the positions behind a
// cursor shift under it -- a page boundary is exactly where that loses
// messages. Go's type inference binds Converge's ID parameter to jmap.ID, so
// the loop needs no wrapper here.
func (c *client) eachMailboxBatch(mailboxID jmap.ID, fn func(ids []jmap.ID) (int, error)) (int, error) {
	next := func() ([]jmap.ID, error) { return c.queryMailboxPage(mailboxID) }
	return paging.Converge(next, fn)
}

// queryMailboxPage returns up to batchLimit message IDs from a mailbox.
func (c *client) queryMailboxPage(mailboxID jmap.ID) ([]jmap.ID, error) {
	req := &jmap.Request{}
	req.Invoke(&email.Query{
		Account: c.accID,
		Filter:  &email.FilterCondition{InMailbox: mailboxID},
		Limit:   batchLimit,
	})

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	for _, inv := range resp.Responses {
		if qr, ok := inv.Args.(*email.QueryResponse); ok {
			return qr.IDs, nil
		}
	}
	return nil, nil
}
