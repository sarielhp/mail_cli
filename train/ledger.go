// Package train records which messages the spam filter has already been
// trained on.
//
// Training bogofilter twice on the same message skews the corpus, and undoing a
// misclassification means forgetting it was ever trained, so this bookkeeping
// decides whether training happens at all. It was previously three files in two
// incompatible JSON shapes -- a list of IDs in two of them, a map of ID to bool
// in the third -- with separate load, save and removal routines for each shape,
// and the paths open-coded across five packages.
//
// A Ledger reads either shape and writes one, so the formats converge as files
// are touched.
package train

import (
	"encoding/json"
	"fmt"
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/sarielhp/mail_cli/cache/dirlock"
)

// Ledger is the set of message IDs trained into one corpus. It is safe for
// concurrent use.
type Ledger struct {
	mu   sync.Mutex
	path string
	ids  map[string]bool
}

// Open reads the ledger at path. A file that does not exist is an empty
// ledger, which is the ordinary first run.
//
// A file that exists but cannot be read or parsed is an error, deliberately.
// The previous loader treated any parse failure as "nothing has been trained",
// so handing it the map-shaped file silently discarded the whole record and
// every message was trained again. Failing closed means a damaged ledger stops
// the caller rather than corrupting the corpus.
func Open(path string) (*Ledger, error) {
	ids, err := readIDsFromDisk(path)
	if err != nil {
		return nil, err
	}
	return &Ledger{path: path, ids: ids}, nil
}

// readIDsFromDisk reads and parses the ledger file directly, with no lock and
// no dependency on any in-memory Ledger state. It is the ground truth Open
// starts from and that saveWithMergeLocked re-reads immediately before every
// write, which is what lets a concurrent writer in another process be merged
// in rather than overwritten.
//
// A file that does not exist is an empty ledger; one that exists but cannot
// be read or parsed is an error, deliberately -- see Open's doc comment on
// why this fails closed instead of returning "nothing trained".
func readIDsFromDisk(path string) (map[string]bool, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return make(map[string]bool), nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading training ledger %s: %w", path, err)
	}
	if len(data) == 0 {
		return make(map[string]bool), nil
	}

	ids, err := parseIDs(data)
	if err != nil {
		return nil, fmt.Errorf("training ledger %s is unreadable: %w", path, err)
	}
	out := make(map[string]bool, len(ids))
	for _, id := range ids {
		out[id] = true
	}
	return out, nil
}

// parseIDs accepts both historical shapes: a JSON array of IDs, and a JSON
// object mapping ID to a boolean.
func parseIDs(data []byte) ([]string, error) {
	var list []string
	if err := json.Unmarshal(data, &list); err == nil {
		return list, nil
	}

	var m map[string]bool
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("not a list of IDs nor a map of them: %w", err)
	}
	out := make([]string, 0, len(m))
	for id, trained := range m {
		if trained {
			out = append(out, id)
		}
	}
	return out, nil
}

// OpenOrEmpty is Open with a damaged ledger reported and then ignored. It is
// for callers that must keep working on a broken file -- a cleanup pass that
// removes an ID has nothing to corrupt.
func OpenOrEmpty(path string) *Ledger {
	l, err := Open(path)
	if err != nil {
		return &Ledger{path: path, ids: make(map[string]bool)}
	}
	return l
}

// Has reports whether the message has already been trained.
func (l *Ledger) Has(id string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.ids[id]
}

// Len is how many messages the ledger records.
func (l *Ledger) Len() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.ids)
}

// IDs returns the recorded messages in a stable order.
func (l *Ledger) IDs() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]string, 0, len(l.ids))
	for id := range l.ids {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// Add records a message and saves, reporting whether it was new. Adding one
// that is already recorded does not rewrite the file.
//
// The mutation and the save happen under one lock. Releasing it in between let
// two concurrent adds each write the temp file and then race on the rename, so
// one of them failed and its message went unrecorded while the caller had
// already trained it.
func (l *Ledger) Add(id string) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if id == "" || l.ids[id] {
		return false, nil
	}
	if err := l.saveWithMergeLocked(func(ids map[string]bool) { ids[id] = true }); err != nil {
		return false, err
	}
	return true, nil
}

// AddAll records multiple messages and saves in a single batch write.
func (l *Ledger) AddAll(ids []string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	var toAdd []string
	for _, id := range ids {
		if id != "" && !l.ids[id] {
			toAdd = append(toAdd, id)
		}
	}
	if len(toAdd) == 0 {
		return nil
	}
	return l.saveWithMergeLocked(func(m map[string]bool) {
		for _, id := range toAdd {
			m[id] = true
		}
	})
}

// Remove forgets a message and saves, reporting whether it was there. This is
// what undoing a misclassification needs.
func (l *Ledger) Remove(id string) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if !l.ids[id] {
		return false, nil
	}
	if err := l.saveWithMergeLocked(func(m map[string]bool) { delete(m, id) }); err != nil {
		return false, err
	}
	return true, nil
}

// Save writes the ledger as a sorted JSON array, replacing whichever shape was
// there before.
func (l *Ledger) Save() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.saveWithMergeLocked(func(map[string]bool) {})
}

// saveWithMergeLocked requires the caller to hold l.mu. It closes the
// lost-update race a per-process in-memory map otherwise allows: two
// processes sharing one ledger (the documented TUI-training-spam-alongside-a-
// cron-scan pattern) each Open it, each add a different ID, and without this
// the later os.Rename simply replaces the earlier process's write wholesale,
// silently dropping whatever it recorded that this process's in-memory copy
// never saw (M8).
//
// Under the cross-process directory lock, it re-reads the ledger fresh from
// disk -- the ground truth, including anything another process committed
// since this Ledger was opened or last saved -- applies this call's own
// mutation to that fresh copy, writes it, and only then adopts it as l.ids.
// A failed write leaves l.ids exactly as it was, which is the rollback the
// callers above rely on; there is nothing to undo because nothing was mutated
// until the write already succeeded.
func (l *Ledger) saveWithMergeLocked(apply func(ids map[string]bool)) error {
	release, lockErr := dirlock.Guard(filepath.Dir(l.path))
	if lockErr != nil {
		slog.Debug("training ledger cross-process lock unavailable, continuing with in-process lock only",
			slog.String("path", l.path), slog.Any("error", lockErr))
	}
	defer release()

	onDisk, err := readIDsFromDisk(l.path)
	if err != nil {
		return err
	}
	apply(onDisk)

	ids := make([]string, 0, len(onDisk))
	for id := range onDisk {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	data, err := json.MarshalIndent(ids, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding training ledger: %w", err)
	}
	if dir := filepath.Dir(l.path); dir != "" {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return fmt.Errorf("creating %s: %w", dir, err)
		}
	}

	// Written through a temp file and renamed, with a name unique to this
	// call (not merely this process): a truncated ledger reads as "nothing
	// trained", which is the failure this package exists to avoid.
	if err := dirlock.AtomicWrite(l.path, data, 0600); err != nil {
		return fmt.Errorf("replacing training ledger %s: %w", l.path, err)
	}
	l.ids = onDisk
	return nil
}

// Path is where the ledger lives, for callers that report what they changed.
func (l *Ledger) Path() string { return l.path }

// MessageIDPath is where the global ledger of trained message IDs lives, beside
// the configuration rather than any one account's cache. See SpamUIDPath and
// HamPath for the two per-account ledgers.
func MessageIDPath(configDir string) string {
	return filepath.Join(configDir, "trained_message_ids.json")
}

// SpamUIDPath is where an account's ledger of messages trained as spam lives,
// beside that account's cache.
func SpamUIDPath(account cachedir.Account) string {
	downloadDir := account.Path()
	return filepath.Join(downloadDir, "trained_uids.json")
}

// HamPath is where an account's ledger of messages trained as ham lives, beside
// that account's cache.
func HamPath(account cachedir.Account) string {
	downloadDir := account.Path()
	return filepath.Join(downloadDir, "trained_uids_ham.json")
}
