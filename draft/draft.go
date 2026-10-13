// Package draft is the local copy of every message being composed: one
// draft.eml per compose session under <root>/YYYY/MM/DD/N. It knows nothing
// about servers. The compose screen writes here on every edit and uploads to
// the account's Drafts folder separately, so a crash, a lost connection or a
// read-only session still leaves the text on disk.
package draft

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sarielhp/mail_cli/localdir"
)

// FileName is the one file in a draft directory.
const FileName = "draft.eml"

// Root is where drafts live for a download directory: <downloadDir>/drafts.
// With no download directory it falls back to ~/.cache/mail_cli/drafts, which
// is what the compose screen always used before -- derived from $HOME, so the
// test suite wrote hundreds of drafts into the developer's real cache.
func Root(downloadDir string) string {
	return localdir.Path(downloadDir, "drafts")
}

// Store is a drafts directory.
type Store struct {
	Root string
}

// Entry is one saved draft. ID is the path below the root without the file
// name ("2026/09/30/125"), which is how the drafts command names it.
type Entry struct {
	ID      string
	Path    string
	ModTime time.Time
	Size    int64
}

// Create allocates the next numbered directory for the day and writes raw
// into it, returning the file's path. The directory is claimed with an
// exclusive mkdir: two sessions reading the same highest number used to
// pick the same N, and the second write replaced the first session's draft.
func (s Store) Create(now time.Time, raw []byte) (string, error) {
	dayDir := filepath.Join(s.Root,
		fmt.Sprintf("%d", now.Year()),
		fmt.Sprintf("%02d", now.Month()),
		fmt.Sprintf("%02d", now.Day()),
	)
	if err := os.MkdirAll(dayDir, 0700); err != nil {
		return "", err
	}
	dir, err := claimDir(dayDir, nextCounter(dayDir))
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, FileName)
	if err := Write(path, raw); err != nil {
		_ = os.Remove(dir)
		return "", err
	}
	return path, nil
}

// claimDir creates dayDir/N for the first free N at or after start.
func claimDir(dayDir string, start int) (string, error) {
	for n := start; n < start+10000; n++ {
		dir := filepath.Join(dayDir, strconv.Itoa(n))
		err := os.Mkdir(dir, 0700)
		if err == nil {
			return dir, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return "", err
		}
	}
	return "", fmt.Errorf("no free draft slot under %s", dayDir)
}

// Write replaces the draft at path atomically: a temporary file in the same
// directory, synced, then renamed over it. A plain rewrite truncates first,
// so a crash halfway left an empty draft.eml -- the only local copy.
func Write(path string, raw []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+FileName+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once renamed
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0600); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func nextCounter(dir string) int {
	files, err := os.ReadDir(dir)
	if err != nil {
		return 1
	}
	maxNum := 0
	for _, f := range files {
		if !f.IsDir() {
			continue
		}
		if num, err := strconv.Atoi(f.Name()); err == nil && num > maxNum {
			maxNum = num
		}
	}
	return maxNum + 1
}

// List returns every draft, newest first. A missing root is an empty list.
func (s Store) List() ([]Entry, error) {
	var entries []Entry
	err := filepath.WalkDir(s.Root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if path == s.Root && errors.Is(err, os.ErrNotExist) {
				return filepath.SkipAll
			}
			return err
		}
		if d.IsDir() || d.Name() != FileName {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		entries = append(entries, Entry{
			ID:      s.idOf(path),
			Path:    path,
			ModTime: info.ModTime(),
			Size:    info.Size(),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].ModTime.After(entries[j].ModTime) })
	return entries, nil
}

func (s Store) idOf(path string) string {
	rel, err := filepath.Rel(s.Root, filepath.Dir(path))
	if err != nil {
		return path
	}
	return filepath.ToSlash(rel)
}

// Resolve maps an ID as List reports it to the draft's path. It refuses an
// ID that would leave the root, and reports a missing draft as an error.
func (s Store) Resolve(id string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(strings.TrimSpace(id)))
	if clean == "." || clean == "" || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid draft id %q", id)
	}
	path := filepath.Join(s.Root, clean, FileName)
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("draft %q: %w", id, err)
	}
	return path, nil
}

// Remove deletes a draft file and its directory when nothing else is in it.
// A path that is already gone is not an error.
func Remove(path string) error {
	if path == "" {
		return nil
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	parent := filepath.Dir(path)
	// A temporary file left by an interrupted Write is ours to clear.
	if leftovers, err := filepath.Glob(filepath.Join(parent, "."+FileName+".tmp-*")); err == nil {
		for _, l := range leftovers {
			_ = os.Remove(l)
		}
	}
	removeEmptyDateDirs(parent)
	return nil
}

// removeEmptyDateDirs removes dir -- a draft's numbered directory -- when it
// is empty, then the day, month and year directories above it while they
// are. Discarding the last draft of a day used to leave the day behind.
// Only directories named with digits are removed, so the climb stops at the
// drafts root whatever it is called.
func removeEmptyDateDirs(dir string) {
	for range 4 { // N, DD, MM, YYYY
		if !isDigits(filepath.Base(dir)) {
			return
		}
		if entries, err := os.ReadDir(dir); err != nil || len(entries) > 0 {
			return
		}
		if os.Remove(dir) != nil {
			return
		}
		dir = filepath.Dir(dir)
	}
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
