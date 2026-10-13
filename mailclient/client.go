package mailclient

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/sarielhp/mail_cli/cache"
	"github.com/sarielhp/mail_cli/cache/folderindex"
	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/label"
)

// CheckingMailClient decorates a MailClient with the local label cache and the
// folder-index bookkeeping that has to follow a move.
//
// It embeds MailClient for the calls it has nothing to add to. Unlike
// ReadOnlyMailClient, silence is the safe default here: a method added to the
// interface and not handled below simply reaches the backend uncached, which is
// correct if slower. The one thing to watch is a new *mutating* method that
// ought to invalidate or reindex something -- MoveEmail and MoveToInbox are the
// existing examples -- because that will pass through unnoticed.
type CheckingMailClient struct {
	MailClient

	Delegate MailClient
	AccName  string
	Store    cache.Store
}

// NewCheckingMailClient wires the embedded MailClient and the Delegate to the
// same client. Building the struct literally is a mistake: leaving the embedded
// field nil makes every unhandled method panic on first use.
func NewCheckingMailClient(delegate MailClient, accName string, store cache.Store) *CheckingMailClient {
	return &CheckingMailClient{
		MailClient: delegate,
		Delegate:   delegate,
		AccName:    accName,
		Store:      store,
	}
}

// GetCacheStore returns the store the client was built with, substituting a
// no-op store when it is nil so callers never need a nil check.
func (c *CheckingMailClient) GetCacheStore() cache.Store {
	if c.Store == nil {
		return &cache.NoOpCacheStore{}
	}
	return c.Store
}

// Validate delegates to the backend, but first rejects a received_folder of
// "inbox" -- or repairs it on disk when FixConfig is set and the account is not
// read-only.
func (c *CheckingMailClient) Validate() error {
	if err := c.fixReceivedFolder(); err != nil {
		return err
	}
	return c.Delegate.Validate()
}

// fixReceivedFolder checks if received_folder is incorrectly set to "inbox"
// and auto-fixes it when FixConfig is enabled.
func (c *CheckingMailClient) fixReceivedFolder() error {
	config := c.Config()
	if config == nil {
		return nil
	}
	if !strings.EqualFold(config.ReceivedFolder, "inbox") {
		return nil
	}
	if config.FixConfig {
		// --read-only forbids server mutations and any local write that would
		// record something which did not happen. It does not forbid caching:
		// downloaded bodies, read state and folder indexes are all written
		// normally under --read-only, because they are facts the server
		// reported and are re-derivable by deleting the cache and running
		// again. Without them --read-only could not show a message at all.
		//
		// The config file is the other kind of write. It holds the user's
		// intent rather than an observation, so rewriting it under a flag whose
		// promise is "change nothing" is a change the user did not ask for.
		// This runs from Validate(), so a read-oriented command could reach it.
		if config.ReadOnly {
			slog.Info("[READ-ONLY] Skipped received_folder auto-fix")
			return nil
		}
		resolved, err := cfg_g.Resolve(config)
		if err != nil {
			return fmt.Errorf("failed to resolve account config for fixing: %w", err)
		}
		appropriate := "received"
		if strings.EqualFold(resolved.Account.Type, "outlook") {
			appropriate = "Archive"
		}
		resolved.Account.ReceivedFolder = appropriate
		if err := cfg_g.SaveConfigFile(resolved.Path, resolved.File); err != nil {
			return fmt.Errorf("failed to save config file after fix: %w", err)
		}
		config.ReceivedFolder = appropriate
		slog.Info("Automatically fixed received_folder in config", slog.String("account", resolved.Account.Name), slog.String("fixed_to", appropriate))
		return nil
	}
	accName := c.AccName
	if accName == "" && config.SelectedAccount != "" {
		accName = string(config.SelectedAccount)
	}
	if accName != "" {
		return fmt.Errorf("received_folder cannot be inbox for account %q; run 'mail_cli config check --fix' to repair it automatically, or edit received_folder in config.json", accName)
	}
	return fmt.Errorf("received_folder cannot be inbox; run 'mail_cli config check --fix' to repair it automatically, or edit received_folder in config.json")
}

// FetchAndDownloadEmails delegates the fetch and clears the folder's cached
// index when the backend reports it empty.
//
// Fetching is not gated on the cache store. It used to be: a store that does
// not write to disk made IsInactive() true and every fetch below returned
// (nil, nil) -- indistinguishable from an empty folder, with no error anywhere.
// A non-selected account got such a store, so reaching one rendered every folder
// empty and reported success. "Do not write to disk" and "do not talk to the
// server" are different instructions, and only the first was ever asked for.
func (c *CheckingMailClient) FetchAndDownloadEmails(folder label.Folder, cacheSubdir string) ([]email.ID, error) {
	ids, err := c.Delegate.FetchAndDownloadEmails(folder, cacheSubdir)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		if config := c.Config(); config != nil {
			if err := folderindex.ReplaceAll(config.AccountCache, folder.String(), nil); err != nil {
				slog.Error("CheckingMailClient.FetchAndDownloadEmails: ReplaceAll failed", slog.String("folder", folder.String()), slog.Any("error", err))
			}
		}
	}
	return ids, nil
}

// FetchLatestAccountEmails delegates to the backend; an account-wide listing
// needs no cache reconciliation.
func (c *CheckingMailClient) FetchLatestAccountEmails(limit int) ([]cfg_acc.MessageFolderRef, error) {
	return c.Delegate.FetchLatestAccountEmails(limit)
}

// CheckAndApplyRules delegates unchanged; the decorator adds no cache
// reconciliation for the rules a backend applies.
func (c *CheckingMailClient) CheckAndApplyRules(messageIDs []email.ID, source label.Folder, cacheSubdir string) ([]email.ID, error) {
	return c.Delegate.CheckAndApplyRules(messageIDs, source, cacheSubdir)
}

// MoveToInbox delegates the move, then reconciles the local folder index,
// including for messages the backend moved before it failed.
func (c *CheckingMailClient) MoveToInbox(messageIDs []email.ID, source label.Folder) error {
	dest := c.Delegate.InboxFolder()
	if err := c.Delegate.MoveToInbox(messageIDs, source); err != nil {
		c.reindexMoved(movedDespite(messageIDs, err), source, dest)
		return err
	}
	c.reindexMoved(messageIDs, source, dest)
	return nil
}

// MoveEmail reconciles whatever moved, including when the batch partly failed.
//
// A backend that can say which messages it refused returns a
// *PartialMoveError, and the rest are on the server's new side of the move: not
// reconciling them leaves them listed in a folder they have left. The error is
// still returned -- the operation did not fully succeed -- but the cache now
// describes what the server actually did.
func (c *CheckingMailClient) MoveEmail(messageIDs []email.ID, source, dest label.Folder) error {
	if err := c.Delegate.MoveEmail(messageIDs, source, dest); err != nil {
		c.reindexMoved(movedDespite(messageIDs, err), source, dest)
		return err
	}
	c.reindexMoved(messageIDs, source, dest)
	return nil
}

// DeleteMessages deletes on the server and then drops the messages from the
// folder's index, so a listing does not keep offering what is gone.
func (c *CheckingMailClient) DeleteMessages(messageIDs []email.ID, folder label.Folder) error {
	if err := c.Delegate.DeleteMessages(messageIDs, folder); err != nil {
		return err
	}
	cfg := c.Config()
	if cfg == nil || cfg.AccountCache.IsZero() || cfg.ReadOnly {
		return nil
	}
	for _, id := range messageIDs {
		if err := folderindex.Remove(cfg.AccountCache, folder.String(), id); err != nil {
			slog.Warn("cache reindex after delete failed", slog.String("id", string(id)), slog.Any("error", err))
		}
	}
	return nil
}

// CopyEmail reconciles copied messages into the destination folder index.
func (c *CheckingMailClient) CopyEmail(messageIDs []email.ID, source, dest label.Folder) error {
	if err := c.Delegate.CopyEmail(messageIDs, source, dest); err != nil {
		c.reindexCopied(movedDespite(messageIDs, err), dest)
		return err
	}
	c.reindexCopied(messageIDs, dest)
	return nil
}

func (c *CheckingMailClient) reindexCopied(messageIDs []email.ID, to label.Folder) {
	if len(messageIDs) == 0 {
		return
	}
	cfg := c.Config()
	if cfg == nil || cfg.AccountCache.IsZero() || cfg.ReadOnly {
		return
	}
	if err := folderindex.AppendBatch(cfg.AccountCache, to.String(), messageIDs); err != nil {
		slog.Warn("cache reindex after copy failed",
			slog.Int("count", len(messageIDs)), slog.String("to", to.String()), slog.Any("error", err))
	}
}

// reindexMoved brings the local folder index in line with a move that the
// server has already accepted. A message's spam evidence and the user's
// override belong to the message, not the folder, so a move leaves both alone.
//
// This lives here, in the decorator that owns the Store, because it was
// previously open-coded at eight call sites in five different shapes -- and
// cli/splice.go omitted it entirely, leaving the index listing messages that
// had been moved away.
//
// Cache maintenance never fails the operation: the server move already
// happened, so reporting failure would misdescribe what occurred.
func (c *CheckingMailClient) reindexMoved(messageIDs []email.ID, from, to label.Folder) {
	if from == to || len(messageIDs) == 0 {
		return
	}
	cfg := c.Config()
	if cfg == nil || cfg.AccountCache.IsZero() {
		return
	}
	// The read-only decorator sits *inside* this one, so under --read-only the
	// delegate returned nil having changed nothing on the server. Reconciling
	// here would record a move that never happened.
	if cfg.ReadOnly {
		return
	}
	if err := folderindex.MoveBatch(cfg.AccountCache, messageIDs, from.String(), to.String()); err != nil {
		slog.Warn("cache reindex after move failed",
			slog.Int("count", len(messageIDs)), slog.String("from", from.String()),
			slog.String("to", to.String()), slog.Any("error", err))
	}
}

// EnsureLabelExists is likewise ungated. Skipping it for a non-selected account
// meant the TUI's compose flow uploaded a draft to a "Drafts" folder it had just
// decided not to create. Under --read-only the ReadOnlyMailClient, which sits
// inside this decorator, still simulates it.
func (c *CheckingMailClient) EnsureLabelExists(name label.Folder) error {
	return c.Delegate.EnsureLabelExists(name)
}

const maxLabelCacheAge = 24 * time.Hour

// GetLabelItems delegates, caches a successful listing, and on failure falls
// back to the cached listing when it is younger than 24 hours.
func (c *CheckingMailClient) GetLabelItems() ([]cfg_acc.Item, error) {
	items, err := c.Delegate.GetLabelItems()
	if err == nil {
		slog.Info("GetLabelItems saving cache",
			slog.String("accName", c.AccName),
			slog.Int("items", len(items)))
		if saveErr := c.GetCacheStore().SaveLabelItems(items); saveErr != nil {
			slog.Warn("GetLabelItems: failed to persist cache fallback",
				slog.String("accName", c.AccName), slog.Any("error", saveErr))
		}
		return items, nil
	}

	slog.Warn("GetLabelItems: server fetch failed, checking cache", slog.Any("error", err))

	if age, ageErr := c.GetCacheStore().CachedLabelItemsAge(); ageErr == nil {
		if age > maxLabelCacheAge {
			slog.Warn("GetLabelItems: cache too old, not using fallback", slog.Duration("age", age))
			return nil, fmt.Errorf("label fetch failed and cache is %v old (max %v): %w", age, maxLabelCacheAge, err)
		}
		slog.Info("GetLabelItems: using cached data", slog.Duration("age", age))
	}

	if cachedItems, errRead := c.GetCacheStore().GetLabelItems(); errRead == nil {
		slog.Warn("GetLabelItems: using cached data (server fetch failed)", slog.Any("error", err))
		return cachedItems, nil
	}

	return nil, err
}

// BackendType reports the wrapped client's backend type -- "gmail", "jmap",
// "outlook" or "mbox".
func (c *CheckingMailClient) BackendType() string { return c.Delegate.BackendType() }

// The message accessors delegate: the backend owns the store, so there is one
// cache layer rather than one per decorator.
