package watcher

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/sarielhp/mail_cli/cache/dirlock"
	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/mailclient"
	"github.com/sarielhp/mail_cli/report"
)

const (
	// DefaultInterval is how often the watcher polls when none is given.
	DefaultInterval = 120 * time.Second
	// MinInterval is the shortest interval a caller may ask for. Polling faster
	// than this is a way to be rate-limited, not a way to see mail sooner.
	MinInterval = 30 * time.Second
	// MaxBackoff caps the retry delay after a failed poll.
	MaxBackoff = 10 * time.Minute
	// DefaultLimit is how many recent messages each poll examines. It is the
	// window new mail is detected in, not a cap on how much is announced.
	DefaultLimit = 50
	// staleGrace is how far before the previous poll a message may be dated
	// and still be announced. An older message that appears in the window was
	// not delivered since: something above it was deleted, or (on Outlook,
	// whose IDs change on a move) it was filed. The slack covers sender clock
	// skew and delayed delivery.
	staleGrace = 6 * time.Hour
	// deliverTimeout bounds notify-send and the --on-mail hook, so one that
	// hangs cannot stop polling.
	deliverTimeout = 30 * time.Second
)

// Options is what a watch run was asked to do.
type Options struct {
	Interval time.Duration
	Account  string
	OnMail   string
	NoNotify bool
	NoFlush  bool
	Limit    int
}

func (o Options) normalized() Options {
	if o.Interval < MinInterval {
		o.Interval = DefaultInterval
	}
	if o.Limit <= 0 {
		o.Limit = DefaultLimit
	}
	return o
}

// Deps is what the watcher needs from the layers around it. Every field is
// supplied by the caller, so the package names no backend and prints nothing.
type Deps struct {
	Config   *cfg_g.Config
	Reporter report.Reporter

	// ClientFor builds a reader for one account. It is how the watcher reaches
	// a backend without importing one.
	ClientFor func(acc cfg_acc.AccountConfig) (mailclient.Reader, error)

	// Flush sends whatever the offline outbox holds. Nil skips it.
	Flush func() error

	// Notifier overrides the desktop notifier. Nil means notify-send, unless
	// Options.NoNotify was set, in which case alerts are logged.
	Notifier Notifier

	// Now is the clock. Nil means time.Now.
	Now func() time.Time
}

// Run polls every watched account until ctx is cancelled, then returns nil.
//
// It never panics and never exits the process: a network failure is a warning
// and a longer wait, and one account failing does not stop the others.
func Run(ctx context.Context, deps Deps, opts Options) error {
	opts = opts.normalized()
	rep := report.Or(deps.Reporter)
	now := deps.Now
	if now == nil {
		now = time.Now
	}
	notifier := deps.Notifier
	if notifier == nil && !opts.NoNotify {
		notifier = notifySendNotifier{}
	}

	accounts, err := watchedAccounts(deps.Config, opts.Account)
	if err != nil {
		return err
	}
	rep.Infof("watch: polling %d account(s) every %s", len(accounts), opts.Interval)

	wait := opts.Interval
	for {
		err := pollOnce(ctx, deps, opts, rep, notifier, accounts, now)
		if ctx.Err() != nil {
			return nil
		}
		if err != nil {
			rep.Warnf("watch: %v", err)
			wait = backoff(wait)
		} else {
			wait = opts.Interval
			flushOutbox(deps, opts, rep)
		}
		if !sleep(ctx, wait) {
			return nil
		}
	}
}

// watchedAccounts resolves which accounts to poll: the named one, or all of
// them.
func watchedAccounts(config *cfg_g.Config, name string) ([]cfg_acc.AccountConfig, error) {
	if config == nil || len(config.Accounts) == 0 {
		return nil, cfg_g.ErrNoAccounts
	}
	if name == "" {
		return config.Accounts, nil
	}
	account, err := cfg_g.Lookup(config.Accounts, name)
	if err != nil {
		return nil, err
	}
	return []cfg_acc.AccountConfig{*account}, nil
}

// pollOnce runs one cycle over every watched account. It reports the failures
// it met rather than stopping at the first, so one unreachable account does not
// silence the rest.
func pollOnce(ctx context.Context, deps Deps, opts Options, rep report.Reporter,
	notifier Notifier, accounts []cfg_acc.AccountConfig, now func() time.Time) error {
	var errs []error
	for _, account := range accounts {
		if ctx.Err() != nil {
			return nil
		}
		if err := checkAccount(ctx, deps, opts, rep, notifier, account, now); err != nil {
			rep.Warnf("watch: %s: %v", account.Name, err)
			errs = append(errs, fmt.Errorf("%s: %w", account.Name, err))
		}
	}
	return errors.Join(errs...)
}

// checkAccount polls one account: fetch the newest messages, work out which
// ones are new against the watermark, announce them, then record them.
func checkAccount(ctx context.Context, deps Deps, opts Options, rep report.Reporter,
	notifier Notifier, account cfg_acc.AccountConfig, now func() time.Time) error {
	client, err := deps.ClientFor(account)
	if err != nil {
		return err
	}
	refs, err := client.FetchLatestAccountEmails(opts.Limit)
	if err != nil {
		return fmt.Errorf("fetch latest: %w", err)
	}
	ids := make([]email.ID, 0, len(refs))
	for _, ref := range refs {
		ids = append(ids, ref.MessageID)
	}

	root := deps.Config.CacheRoot
	state, err := readState(root)
	if err != nil {
		return err
	}
	if !state.Known(account.Name) {
		if err := commitSeen(root, account.Name, ids, now()); err != nil {
			return err
		}
		rep.Infof("watch: %s: first run; recorded %d existing message(s) without notifying",
			account.Name, len(ids))
		return nil
	}

	since := state.Accounts[account.Name].LastChecked.Add(-staleGrace)
	fresh := state.Unseen(account.Name, announceable(refs, account))
	announce(ctx, opts, rep, notifier, account, client, fresh, since)
	return commitSeen(root, account.Name, ids, now())
}

// announce reports each new message, one at a time. It runs after the
// watermark has been read and before it is written, so a message whose summary
// cannot be read is still recorded as seen rather than announced again forever.
// A message dated before since is recorded without being announced; see
// staleGrace.
func announce(ctx context.Context, opts Options, rep report.Reporter, notifier Notifier,
	account cfg_acc.AccountConfig, client mailclient.Reader, ids []email.ID, since time.Time) {
	for _, id := range ids {
		if ctx.Err() != nil {
			return
		}
		summary, err := client.GetMessageSummary(id)
		if err != nil {
			rep.Warnf("watch: %s: cannot read %s: %v", account.Name, id, err)
			continue
		}
		if !summary.EmailDate.IsZero() && summary.EmailDate.Before(since) {
			continue
		}
		deliver(ctx, opts, rep, notifier, Notification{
			Account: account.Name,
			From:    email.FormatSenderVerbose(summary.FromRaw),
			Subject: summary.Subject,
			Date:    summary.FormattedDate,
			ID:      id,
		})
	}
}

// announceable returns the IDs of the refs filed where new mail is worth an
// alert. The account-wide listing covers every folder, so without this the
// watcher announced spam, the user's own sent mail and every draft autosave.
func announceable(refs []cfg_acc.MessageFolderRef, account cfg_acc.AccountConfig) []email.ID {
	ids := make([]email.ID, 0, len(refs))
	for _, ref := range refs {
		if !quietFolder(ref.Folder, account.SpamFolder) {
			ids = append(ids, ref.MessageID)
		}
	}
	return ids
}

// quietFolders names, by their last path element, the folders whose mail is
// never announced, in the Gmail, JMAP and Outlook spellings.
var quietFolders = map[string]bool{
	"spam": true, "junk": true, "junk email": true, "junk e-mail": true, "bulk mail": true,
	"trash": true, "deleted items": true, "deleted messages": true, "bin": true,
	"sent": true, "sent items": true, "sent mail": true, "sent messages": true,
	"draft": true, "drafts": true,
}

func quietFolder(folder, spamFolder string) bool {
	if folder == "" {
		return false
	}
	if spamFolder != "" && strings.EqualFold(folder, spamFolder) {
		return true
	}
	return quietFolders[strings.ToLower(path.Base(folder))]
}

// deliver shows one alert: the desktop notification, or a log line when there
// is none, and then the --on-mail hook.
func deliver(ctx context.Context, opts Options, rep report.Reporter, notifier Notifier, n Notification) {
	notified := false
	if notifier != nil {
		if err := notifier.Notify(ctx, n); err != nil {
			rep.Warnf("watch: desktop notification failed: %v", err)
		} else {
			notified = true
		}
	}
	if !notified {
		rep.Infof("watch: [%s] %s -- %s", n.Account, n.From, n.Subject)
	}
	if opts.OnMail == "" {
		return
	}
	if err := runHook(ctx, opts.OnMail, n); err != nil {
		rep.Warnf("watch: --on-mail %s failed: %v", opts.OnMail, err)
	}
}

// flushOutbox sends anything queued while offline, once a clean cycle has
// confirmed the network is reachable.
//
// Under --read-only or --dry-run it does nothing: flushing is a server
// mutation, and the client decorator would refuse each send anyway, leaving a
// failed-attempt record behind on every poll.
func flushOutbox(deps Deps, opts Options, rep report.Reporter) {
	if opts.NoFlush || deps.Flush == nil {
		return
	}
	if deps.Config != nil && (deps.Config.ReadOnly || deps.Config.DryRun) {
		return
	}
	if err := deps.Flush(); err != nil {
		rep.Warnf("watch: outbox flush failed: %v", err)
	}
}

// readState loads the watermark under the cache lock, so a second watcher
// sharing the directory reads a consistent file.
func readState(root string) (*State, error) {
	release, err := dirlock.Guard(root)
	if release != nil {
		defer release()
	}
	if err != nil {
		return nil, err
	}
	return LoadState(root)
}

// commitSeen records ids as seen under the cache lock.
func commitSeen(root, name string, ids []email.ID, at time.Time) error {
	release, err := dirlock.Guard(root)
	if release != nil {
		defer release()
	}
	if err != nil {
		return err
	}
	state, err := LoadState(root)
	if err != nil {
		return err
	}
	state.MarkSeen(name, ids, at)
	return state.Save(root)
}

// backoff doubles the wait after a failure, up to MaxBackoff.
func backoff(current time.Duration) time.Duration {
	next := current * 2
	switch {
	case next > MaxBackoff:
		return MaxBackoff
	case next < DefaultInterval:
		return DefaultInterval
	default:
		return next
	}
}

// sleep waits for d, or reports false if ctx was cancelled first.
func sleep(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
