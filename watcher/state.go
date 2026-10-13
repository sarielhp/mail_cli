package watcher

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/sarielhp/mail_cli/cache/dirlock"
	"github.com/sarielhp/mail_cli/email"
)

// maxSeen bounds the per-account watermark. Each poll records the newest
// messages it fetched, so an old entry falls out of the window on its own; the
// cap only stops a state file growing without limit if a backend keeps
// returning an ever-longer list.
const maxSeen = 500

// State is the watcher's watermark: what it has already seen, per account.
type State struct {
	Accounts map[string]Account `json:"accounts"`
}

// Account is one account's watermark.
type Account struct {
	LastChecked time.Time `json:"last_checked_timestamp"`
	LastSeenIDs []string  `json:"last_seen_ids"`
}

// stateFile names the watermark beneath a download directory.
func stateFile(downloadDir string) string {
	return filepath.Join(downloadDir, "watcher", "state.json")
}

// LoadState reads the watermark, treating a missing file as an empty one. A
// file that exists but does not parse is an error: the watcher must not answer
// "nothing seen yet" to a damaged file, or its next poll would announce every
// message in the account.
func LoadState(downloadDir string) (*State, error) {
	data, err := os.ReadFile(stateFile(downloadDir))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &State{Accounts: map[string]Account{}}, nil
		}
		return nil, fmt.Errorf("read watcher state: %w", err)
	}
	state := &State{}
	if err := json.Unmarshal(data, state); err != nil {
		return nil, fmt.Errorf("parse watcher state %s: %w", stateFile(downloadDir), err)
	}
	if state.Accounts == nil {
		state.Accounts = map[string]Account{}
	}
	return state, nil
}

// Save writes the watermark atomically, so a crash mid-write cannot leave a
// half-written file for the next run to parse.
func (s *State) Save(downloadDir string) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("encode watcher state: %w", err)
	}
	data = append(data, '\n')
	if err := dirlock.AtomicWrite(stateFile(downloadDir), data, 0600); err != nil {
		return fmt.Errorf("write watcher state: %w", err)
	}
	return nil
}

// Known reports whether the account has a watermark yet. An account the watcher
// has never polled is on its first run, which records what it finds without
// announcing it.
func (s *State) Known(name string) bool {
	_, ok := s.Accounts[name]
	return ok
}

// Unseen returns the ids the account has not seen, in the order given.
func (s *State) Unseen(name string, ids []email.ID) []email.ID {
	account, ok := s.Accounts[name]
	if !ok {
		return ids
	}
	seen := make(map[string]bool, len(account.LastSeenIDs))
	for _, id := range account.LastSeenIDs {
		seen[id] = true
	}
	var unseen []email.ID
	for _, id := range ids {
		if !seen[string(id)] {
			unseen = append(unseen, id)
		}
	}
	return unseen
}

// MarkSeen records ids as seen and stamps the account's last-checked time. The
// newest ids are kept ahead of older ones, and the list is capped.
func (s *State) MarkSeen(name string, ids []email.ID, at time.Time) {
	if s.Accounts == nil {
		s.Accounts = map[string]Account{}
	}
	account := s.Accounts[name]

	seen := make([]string, 0, len(ids)+len(account.LastSeenIDs))
	have := make(map[string]bool, len(ids)+len(account.LastSeenIDs))
	for _, id := range ids {
		value := string(id)
		if !have[value] {
			have[value] = true
			seen = append(seen, value)
		}
	}
	for _, value := range account.LastSeenIDs {
		if !have[value] {
			have[value] = true
			seen = append(seen, value)
		}
	}
	if len(seen) > maxSeen {
		seen = seen[:maxSeen]
	}

	account.LastSeenIDs = seen
	account.LastChecked = at
	s.Accounts[name] = account
}
