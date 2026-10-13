package runner

import (
	"errors"
	"fmt"
	"io"

	"github.com/sarielhp/mail_cli/cache/dirlock"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/contacts"
	"github.com/sarielhp/mail_cli/mailclient"
	"github.com/sarielhp/mail_cli/mailops"
	"github.com/sarielhp/mail_cli/outbox"
	"github.com/sarielhp/mail_cli/report"
)

// OutboxService manages inspection and flushing of the offline outbox spool.
type OutboxService struct {
	Rep report.Reporter
	Out io.Writer
}

func (s OutboxService) rep() report.Reporter { return report.Or(s.Rep) }

// List returns all queued outbox messages for the configured cache root.
func (s OutboxService) List(config *cfg_g.Config) ([]outbox.Entry, error) {
	cacheRoot := ""
	if config != nil {
		cacheRoot = config.CacheRoot
	}
	return outbox.List(outbox.Dir(cacheRoot))
}

// Remove drops an outbox entry by ID from the cache root.
func (s OutboxService) Remove(config *cfg_g.Config, id string) error {
	cacheRoot := ""
	if config != nil {
		cacheRoot = config.CacheRoot
	}
	return outbox.Remove(outbox.Dir(cacheRoot), id)
}

// ErrFlushRefused is returned when the session forbids sending: a flush
// transmits mail, which --read-only and --dry-run both exist to prevent.
var ErrFlushRefused = errors.New("outbox flush sends mail; refused under --read-only and --dry-run")

// Flush attempts transmission of all queued messages in the outbox.
//
// It holds the outbox directory's lock for the whole run, so two flushes --
// say a cron job and an interactive one -- cannot both read the same entry and
// send it twice. The lock is the outbox directory's own: nothing on the
// SendEmail path takes it, so holding it across a send cannot deadlock against
// the account-cache lock the msgstore prefetch workers take.
func (s OutboxService) Flush(config *cfg_g.Config) ([]outbox.FlushResult, error) {
	if config != nil && (config.ReadOnly || config.DryRun) {
		return nil, ErrFlushRefused
	}
	cacheRoot := ""
	if config != nil {
		cacheRoot = config.CacheRoot
	}
	dir := outbox.Dir(cacheRoot)
	release, err := dirlock.Guard(dir)
	defer release()
	if err != nil {
		s.rep().Warnf("Outbox lock not taken (%v); a concurrent flush could send twice", err)
	}
	entries, err := outbox.List(dir)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, nil
	}

	var results []outbox.FlushResult
	for _, e := range entries {
		res := s.flushEntry(config, dir, e)
		results = append(results, res)
	}
	return results, nil
}

// recordFailure notes a failed attempt on the entry, except when the account
// itself refused because it is read-only: that attempt never happened.
func (s OutboxService) recordFailure(dir string, e outbox.Entry, account string, sendErr error) outbox.FlushResult {
	if !errors.Is(sendErr, mailclient.ErrReadOnlyOperationBlocked) {
		if err := outbox.RecordAttempt(dir, e.ID, sendErr); err != nil {
			s.rep().Warnf("Recording the failed attempt on outbox entry %s: %v", e.ID, err)
		}
	}
	return outbox.FlushResult{ID: e.ID, Account: account, Subject: e.Subject, Success: false, Err: sendErr.Error()}
}

func (s OutboxService) flushEntry(config *cfg_g.Config, dir string, e outbox.Entry) outbox.FlushResult {
	item, raw, err := outbox.Get(dir, e.ID)
	if err != nil {
		return outbox.FlushResult{ID: e.ID, Account: e.Account, Subject: e.Subject, Success: false, Err: err.Error()}
	}

	client, acc, err := mailops.GetClientForSending(config, item.Account, s.rep())
	if err != nil {
		return s.recordFailure(dir, e, e.Account, err)
	}

	if err := client.SendEmail(raw); err != nil {
		return s.recordFailure(dir, e, acc.Name, err)
	}

	contacts.RecordSentRecipients(config, raw)
	res := outbox.FlushResult{ID: e.ID, Account: acc.Name, Subject: e.Subject, Success: true}
	// A sent message left in the outbox is sent again by the next flush, so a
	// failed removal is reported, not dropped.
	if err := outbox.Remove(dir, e.ID); err != nil {
		res.Err = fmt.Sprintf("sent, but not removed from the outbox (remove %s before the next flush): %v", e.ID, err)
		s.rep().Warnf("Outbox message %s was sent but could not be removed: %v", e.ID, err)
		return res
	}
	s.rep().Successf("Outbox message %s delivered via %s", e.ID, acc.Name)
	return res
}
