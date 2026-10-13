package jmap

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/sarielhp/mail_cli/label"

	"git.sr.ht/~rockorager/go-jmap"
	"git.sr.ht/~rockorager/go-jmap/mail/mailbox"
)

// createMailboxLevel creates one level of a mailbox hierarchy and returns
// its ID, retrying on a transient failure the same way
// executeJMAPOpWithRetry does for every other JMAP call in this package --
// except that here the operation being retried is not safe to simply
// resend. A timeout or 502/503/504 is exactly the ambiguous case where the
// server may have already created the mailbox and only the response was
// lost; every other retried call in this package (loading mailboxes,
// marking read, querying) is either a pure read or naturally idempotent, so
// blindly resending it on retry is harmless. Resending mailbox.Set{Create:
// ...} is not: JMAP gives the created object a client-chosen local ID
// ("new-mb") but nothing dedupes by Name, so a retry after a lost response
// creates a second, duplicate mailbox with the identical name.
//
// Before each retry, this re-checks whether currentPath now exists and uses
// that instead of creating again.
func (c *client) createMailboxLevel(ctx context.Context, currentPath, localName string, parentID jmap.ID) (jmap.ID, error) {
	if err := label.ValidateNewName(currentPath); err != nil {
		return "", err
	}
	var lastErr error
	for attempt := 0; attempt < maxJMAPAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return "", err
		}

		id, err := c.attemptCreateMailbox(ctx, currentPath, localName, parentID)
		if err == nil {
			return id, nil
		}
		lastErr = err

		if !isRetryableJMAPError(err) || attempt == maxJMAPAttempts-1 {
			return "", err
		}

		if id, ok := c.recheckMailboxExists(currentPath); ok {
			return id, nil
		}

		sleepDur := computeJMAPRetryDelay(attempt)
		slog.Debug("JMAP create mailbox throttled, retrying",
			slog.String("path", currentPath),
			slog.Int("attempt", attempt+1),
			slog.Duration("wait", sleepDur),
			slog.Any("error", err))
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(sleepDur):
		}
	}
	return "", lastErr
}

// recheckMailboxExists refreshes the mailbox list from the server and
// reports whether currentPath now resolves -- the check createMailboxLevel
// makes before every retry, so an ambiguous failure whose Create actually
// landed is discovered instead of duplicated.
func (c *client) recheckMailboxExists(currentPath string) (jmap.ID, bool) {
	if err := c.loadMailboxes(); err != nil {
		return "", false
	}
	return c.findMailboxID(currentPath)
}

func (c *client) attemptCreateMailbox(ctx context.Context, currentPath, localName string, parentID jmap.ID) (jmap.ID, error) {
	mbCreate := &mailbox.Mailbox{Name: localName}
	if parentID != "" {
		mbCreate.ParentID = parentID
	}
	reqCreate := &jmap.Request{Context: ctx}
	reqCreate.Invoke(&mailbox.Set{
		Account: c.accID,
		Create: map[jmap.ID]*mailbox.Mailbox{
			"new-mb": mbCreate,
		},
	})

	respCreate, err := c.client.Do(reqCreate)
	if err != nil {
		return "", err
	}
	return extractCreatedMailboxID(respCreate, currentPath)
}

func extractCreatedMailboxID(respCreate *jmap.Response, currentPath string) (jmap.ID, error) {
	var createdID jmap.ID
	for _, inv := range respCreate.Responses {
		if r, ok := inv.Args.(*mailbox.SetResponse); ok {
			if errSet, failed := r.NotCreated["new-mb"]; failed {
				return "", fmt.Errorf("failed to create mailbox %s: %s", currentPath, safeStr(errSet.Description))
			}
			for _, mb := range r.Created {
				createdID = mb.ID
			}
		}
	}
	if createdID == "" {
		return "", fmt.Errorf("failed to retrieve ID of created mailbox: %s", currentPath)
	}
	return createdID, nil
}
