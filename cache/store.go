package cache

import (
	"encoding/json"
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"os"
	"path/filepath"
	"time"

	"github.com/sarielhp/mail_cli/cache/dirlock"
	"github.com/sarielhp/mail_cli/cfg_acc"
)

// Store reads and writes a session's cached label listing. DiskCacheStore
// persists it under an account's cache directory; NoOpCacheStore is the stub
// used when no account is selected.
type Store interface {
	GetLabelItems() ([]cfg_acc.Item, error)
	SaveLabelItems(items []cfg_acc.Item) error
	CachedLabelItemsAge() (time.Duration, error)
	IsActive() bool
}

// DiskCacheStore persists a label listing as labels_cache.json inside one
// account's cache directory.
type DiskCacheStore struct {
	Dir cachedir.Account
}

func (d *DiskCacheStore) cachePath() string {
	return filepath.Join(d.Dir.Path(), "labels_cache.json")
}

// GetLabelItems returns the cached label listing. It reports os.ErrNotExist
// when the account has no cache directory or nothing has been saved yet.
func (d *DiskCacheStore) GetLabelItems() ([]cfg_acc.Item, error) {
	if d.Dir.IsZero() {
		return nil, os.ErrNotExist
	}
	data, err := os.ReadFile(d.cachePath())
	if err != nil {
		return nil, err
	}
	var items []cfg_acc.Item
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, err
	}
	return items, nil
}

// SaveLabelItems writes the listing to the account's cache atomically under the
// cache lock. It does nothing when the account has no cache directory.
func (d *DiskCacheStore) SaveLabelItems(items []cfg_acc.Item) error {
	if d.Dir.IsZero() {
		return nil
	}
	release := guard(d.Dir.Path())
	defer release()

	data, err := json.Marshal(items)
	if err != nil {
		return err
	}
	return dirlock.AtomicWrite(d.cachePath(), data, 0600)
}

// CachedLabelItemsAge returns how long ago the listing was saved, or
// os.ErrNotExist when there is no cache directory or no saved listing.
func (d *DiskCacheStore) CachedLabelItemsAge() (time.Duration, error) {
	if d.Dir.IsZero() {
		return 0, os.ErrNotExist
	}
	info, err := os.Stat(d.cachePath())
	if err != nil {
		return 0, err
	}
	return time.Since(info.ModTime()), nil
}

// NoOpCacheStore satisfies Store without touching disk, for when no account is
// selected: every read reports os.ErrNotExist and every write is discarded.
type NoOpCacheStore struct{}

// GetLabelItems always reports os.ErrNotExist: nothing is cached.
func (n *NoOpCacheStore) GetLabelItems() ([]cfg_acc.Item, error) {
	return nil, os.ErrNotExist
}

// SaveLabelItems discards the listing and always succeeds.
func (n *NoOpCacheStore) SaveLabelItems(_ []cfg_acc.Item) error {
	return nil
}

// CachedLabelItemsAge always reports os.ErrNotExist: nothing was ever saved.
func (n *NoOpCacheStore) CachedLabelItemsAge() (time.Duration, error) {
	return 0, os.ErrNotExist
}

// IsActive reports true: this store persists to disk.
func (d *DiskCacheStore) IsActive() bool { return true }

// IsActive reports false: this store keeps nothing.
func (n *NoOpCacheStore) IsActive() bool { return false }
