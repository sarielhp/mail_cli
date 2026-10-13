package msg

import (
	"errors"
	"fmt"
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"github.com/sarielhp/mail_cli/email"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/sarielhp/mail_cli/cache/dirlock"
	"github.com/sarielhp/mail_cli/message_info"
)

// Info is what the cache knows about a message: its date, the evidence the
// last scan recorded, and the user's override. Whether it is spam is derived
// from the last two (see verdict.go), never stored.
type Info struct {
	Date     time.Time
	Scanned  bool
	Evidence Evidence
	Override Override
}

type dirCache struct {
	mu       sync.RWMutex
	m        map[string]message_info.Compact
	path     string
	loaded   bool
	entries  int64
	lastMod  time.Time
	lastSize int64
}

var dirCaches sync.Map

func getDirCache(downloadDir string) *dirCache {
	v, _ := dirCaches.LoadOrStore(downloadDir, &dirCache{})
	return v.(*dirCache)
}

func binPath(downloadDir string) string {
	return filepath.Join(downloadDir, "message_info.bin")
}

func ensureLoaded(downloadDir string) {
	dc := getDirCache(downloadDir)
	binP := binPath(downloadDir)

	dc.mu.RLock()
	if dc.loaded && dc.path == binP {
		fi, err := os.Stat(binP)
		if err == nil && dc.lastMod.Equal(fi.ModTime()) && dc.lastSize == fi.Size() {
			dc.mu.RUnlock()
			return
		}
		if os.IsNotExist(err) && dc.lastSize == 0 {
			dc.mu.RUnlock()
			return
		}
	}
	dc.mu.RUnlock()

	dc.mu.Lock()
	defer dc.mu.Unlock()
	loadDirCacheLocked(dc, downloadDir, binP)
}

func loadDirCacheLocked(dc *dirCache, downloadDir, binP string) {
	fi, err := os.Stat(binP)
	if err != nil {
		if os.IsNotExist(err) && !dc.loaded {
			dc.m = make(map[string]message_info.Compact)
			dc.path = binP
			dc.loaded = true
			dc.entries = 0
			dc.lastMod = time.Time{}
			dc.lastSize = 0
		}
		return
	}

	if dc.loaded && dc.path == binP && dc.lastMod.Equal(fi.ModTime()) && dc.lastSize == fi.Size() {
		return
	}

	m, err := message_info.ReadFromFile(binP)
	if errors.Is(err, message_info.ErrDamaged) {
		m, err = recoverDamagedMessageInfo(downloadDir, binP)
		if err == nil {
			if fi2, statErr := os.Stat(binP); statErr == nil {
				fi = fi2
			}
		}
	}
	if err != nil {
		slog.Error("Failed to read message_info", slog.String("path", binP), slog.Any("error", err))
		if !dc.loaded {
			dc.m = make(map[string]message_info.Compact)
			dc.path = binP
			dc.loaded = true
		}
		return
	}
	if len(m) == 0 && !dc.loaded {
		migrateOldCache(downloadDir, binP)
		m2, err2 := message_info.ReadFromFile(binP)
		if err2 == nil {
			m = m2
			if fi2, statErr := os.Stat(binP); statErr == nil {
				fi = fi2
			}
		}
	}
	dc.m = m
	dc.path = binP
	dc.loaded = true
	dc.lastMod = fi.ModTime()
	dc.lastSize = fi.Size()
	dc.entries = int64(len(m))
}

// recoverDamagedMessageInfo re-reads binP under the cross-process directory
// lock and, if it is still damaged there, truncates it to what that fresh
// read salvages.
//
// ensureLoaded's own read of binP takes no lock at all -- reads never have,
// since they only had to tolerate an occasional torn read before H6 added a
// write to this path. Re-reading under the lock here, rather than trusting
// that first unlocked read, matters because the "damage" it saw can be
// nothing more than another process's append still in flight: a read
// landing mid-write can look exactly like corruption. By the time this lock
// is acquired, that write may have completed and the file may no longer be
// damaged at all -- rewriting from the earlier, stale, incomplete salvage in
// that case would permanently discard whatever the other process just
// legitimately appended, racing a real writer to overwrite its own recent
// work.
func recoverDamagedMessageInfo(downloadDir, binP string) (map[string]message_info.Compact, error) {
	defer guard(downloadDir)()

	m, err := message_info.ReadFromFile(binP)
	if !errors.Is(err, message_info.ErrDamaged) {
		return m, err
	}

	slog.Error("message_info damaged; truncating to last valid record",
		slog.String("path", binP), slog.Int("recordsSalvaged", len(m)))
	if rwErr := message_info.RewriteFile(binP, m); rwErr != nil {
		slog.Error("failed to rewrite damaged message_info", slog.String("path", binP), slog.Any("error", rwErr))
	}
	return m, nil
}

func migrateOldCache(downloadDir, binP string) {
	oldIndex := filepath.Join(downloadDir, "message_index.json")
	if _, err := os.Stat(oldIndex); os.IsNotExist(err) {
		return
	}
	slog.Info("Migrating old cache to message_info.bin", slog.String("downloadDir", downloadDir))

	if err := os.MkdirAll(filepath.Dir(binP), 0700); err != nil {
		slog.Warn("failed to create migration directory", slog.Any("error", err))
	}

	entries := make(map[string]message_info.Compact)
	var entryCount int

	walkFn := func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || filepath.Ext(info.Name()) != ".eml" {
			return nil
		}
		id := info.Name()[:len(info.Name())-4]
		if id == "" {
			return nil
		}
		date, ok := dateFromPath(path)
		if !ok {
			return nil
		}
		entries[id] = message_info.Compact{Date: date}
		entryCount++
		return nil
	}

	if err := filepath.Walk(filepath.Join(downloadDir, "messages"), walkFn); err != nil {
		slog.Warn("failed to walk messages directory during migration", slog.Any("error", err))
	}

	if entryCount > 0 {
		if err := message_info.RewriteFile(binP, entries); err != nil {
			slog.Error("Failed to write migrated message_info", slog.Any("error", err))
			return
		}
	}

	os.Remove(filepath.Join(downloadDir, "message_index.json"))
	os.RemoveAll(filepath.Join(downloadDir, "classifications"))
	if files, err := os.ReadDir(downloadDir); err == nil {
		for _, f := range files {
			if !f.IsDir() && filepath.Ext(f.Name()) == ".json" {
				if f.Name() != "labels_cache.json" && f.Name() != "seen_cache.json" && f.Name() != "folder_index.json" {
					os.Remove(filepath.Join(downloadDir, f.Name()))
				}
			}
		}
	} else {
		os.Remove(filepath.Join(downloadDir, "trained_uids.json"))
		os.Remove(filepath.Join(downloadDir, "trained_uids_ham.json"))
		os.Remove(filepath.Join(downloadDir, "trained_message_ids.json"))
	}

	slog.Info("Migration complete", slog.Int("entries", entryCount))
}

func lookupPath(downloadDir string, id email.ID) (string, error) {
	info := getCompact(downloadDir, id)
	if info == nil {
		return "", fmt.Errorf("message %s not found in message_info", id)
	}
	y := info.Date / 10000
	m := (info.Date / 100) % 100
	d := info.Date % 100
	datePath := fmt.Sprintf("%04d/%02d/%02d", y, m, d)
	return filepath.Join(downloadDir, "messages", datePath, string(id)+".eml"), nil
}

func getCompact(downloadDir string, id email.ID) *message_info.Compact {
	ensureLoaded(downloadDir)
	dc := getDirCache(downloadDir)
	dc.mu.RLock()
	defer dc.mu.RUnlock()
	if c, ok := dc.m[string(id)]; ok {
		return &c
	}
	return nil
}

func setCompact(downloadDir string, id email.ID, info message_info.Compact) error {
	defer guard(downloadDir)()
	ensureLoaded(downloadDir)
	dc := getDirCache(downloadDir)
	dc.mu.Lock()
	defer dc.mu.Unlock()

	dc.m[string(id)] = info
	dc.entries++
	ratio := float64(dc.entries-int64(len(dc.m))) / float64(dc.entries)
	needsCompact := dc.entries > 1000 && ratio > 0.25

	p := binPath(downloadDir)
	if err := message_info.AppendToFile(p, string(id), info); err != nil {
		return fmt.Errorf("append message_info: %w", err)
	}
	if fi, err := os.Stat(p); err == nil {
		dc.lastMod = fi.ModTime()
		dc.lastSize = fi.Size()
	}

	if needsCompact {
		if err := compactMapLocked(downloadDir, dc); err != nil {
			slog.Error("Failed to compact message_info", slog.Any("error", err))
		}
	}
	return nil
}

// guard acquires the cross-process cache lock. A failure is tolerated: the
// per-directory mutex still applies within this process.
func guard(downloadDir string) func() {
	release, err := dirlock.Guard(downloadDir)
	if err != nil {
		slog.Debug("cache lock unavailable, continuing with in-process lock only",
			slog.String("dir", downloadDir), slog.Any("error", err))
	}
	return release
}

// compactMapLocked requires the caller to hold dc.mu.
func compactMapLocked(downloadDir string, dc *dirCache) error {
	p := binPath(downloadDir)
	if err := message_info.RewriteFile(p, dc.m); err != nil {
		return fmt.Errorf("compact message_info: %w", err)
	}
	dc.entries = int64(len(dc.m))
	if fi, err := os.Stat(p); err == nil {
		dc.lastMod = fi.ModTime()
		dc.lastSize = fi.Size()
	}
	slog.Info("Compacted message_info", slog.Int64("entries", dc.entries))
	return nil
}

// Read returns the cached body of msgID. It errors when the message has no
// cache entry or its file cannot be read.
func Read(account cachedir.Account, msgID email.ID) ([]byte, error) {
	downloadDir := account.Path()
	p, err := lookupPath(downloadDir, msgID)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(p)
}

// WithReader opens a cached message for streaming read. The file is closed
// when fn returns.
func WithReader(account cachedir.Account, msgID email.ID, fn func(io.Reader) error) error {
	downloadDir := account.Path()
	p, err := lookupPath(downloadDir, msgID)
	if err != nil {
		return err
	}
	f, err := os.Open(p)
	if err != nil {
		return err
	}
	defer f.Close()
	return fn(f)
}

// Exists reports whether msgID has a cached body. A message with no cache entry
// reports false with no error; a stat failure other than "not exist" is
// returned.
func Exists(account cachedir.Account, msgID email.ID) (bool, error) {
	downloadDir := account.Path()
	p, err := lookupPath(downloadDir, msgID)
	if err != nil {
		return false, nil
	}
	_, err = os.Stat(p)
	if os.IsNotExist(err) {
		return false, nil
	}
	return err == nil, err
}

// restored is the record for a message whose body is being stored: a new
// one, or -- when the message is already indexed -- the old one with the new
// date. Storing a body again, as a re-download does, says nothing about the
// message: it used to reset the record and so dropped the user's override and
// the scan's evidence along with it.
func restored(old *message_info.Compact, date uint32) message_info.Compact {
	if old == nil {
		return message_info.Compact{Date: date}
	}
	c := *old
	c.Date = date
	return c
}

// Store writes a message body to the cache under the cache lock and records its
// received date, preserving the classification already indexed for it. Any
// cached parsed summary is dropped, since the body it described has been
// replaced.
func Store(account cachedir.Account, msgID email.ID, rawBytes []byte, receivedDate time.Time) error {
	downloadDir := account.Path()
	dateVal := uint32(receivedDate.Year()*10000 + int(receivedDate.Month())*100 + receivedDate.Day())
	if err := setCompact(downloadDir, msgID, restored(getCompact(downloadDir, msgID), dateVal)); err != nil {
		return err
	}
	p, err := lookupPath(downloadDir, msgID)
	if err != nil {
		return err
	}
	// setCompact already reentrantly takes and releases this same guard for
	// the index write; holding it across the body write too closes the
	// window StoreBatch.Store already closes -- two writers of the same
	// message (a TUI action racing a scheduled scan against a shared
	// account cache is the documented pattern) no longer share one unlocked
	// ".tmp" name and interleave into a truncated or mixed body (M6).
	defer guard(downloadDir)()
	if err := dirlock.AtomicWrite(p, rawBytes, 0600); err != nil {
		return err
	}
	if psp, err := parsedSummaryPath(downloadDir, msgID); err == nil {
		_ = os.Remove(psp)
	}
	return nil
}

func deleteCachedArtifacts(base string) bool {
	if base == "" {
		return false
	}
	emlPath := base + ".eml"
	removed := false
	if err := os.Remove(emlPath); err == nil {
		removed = true
	} else if !os.IsNotExist(err) {
		slog.Warn("failed to remove cached message", slog.String("path", emlPath), slog.Any("error", err))
	}
	_ = os.Remove(base + ".summary.json")
	_ = os.Remove(base + ".llm_summary.md")
	_ = os.Remove(base + ".llm_summary")
	return removed
}

// Delete removes msgID's cached body, its summaries and its index record. It is
// a no-op when nothing is cached.
func Delete(account cachedir.Account, msgID email.ID) error {
	_, err := DeleteBatch(account, []email.ID{msgID})
	return err
}

// DeleteBatch removes cached messages and their auxiliary summaries, updating message_info.
// It returns the number of successfully removed cached messages.
func DeleteBatch(account cachedir.Account, msgIDs []email.ID) (int, error) {
	downloadDir := account.Path()
	if downloadDir == "" || len(msgIDs) == 0 {
		return 0, nil
	}

	defer guard(downloadDir)()
	ensureLoaded(downloadDir)
	dc := getDirCache(downloadDir)
	dc.mu.Lock()
	defer dc.mu.Unlock()

	var deletedCount int
	changed := false
	for _, id := range msgIDs {
		c, ok := dc.m[string(id)]
		if ok {
			delete(dc.m, string(id))
			changed = true
		}
		var base string
		if ok {
			y := c.Date / 10000
			m := (c.Date / 100) % 100
			d := c.Date % 100
			datePath := fmt.Sprintf("%04d/%02d/%02d", y, m, d)
			base = filepath.Join(downloadDir, "messages", datePath, string(id))
		} else {
			base = filepath.Join(downloadDir, string(id))
		}
		if deleteCachedArtifacts(base) {
			deletedCount++
		}
	}
	if changed {
		if err := compactMapLocked(downloadDir, dc); err != nil {
			slog.Warn("failed to compact message_info after delete", slog.Any("error", err))
		}
	}
	return deletedCount, nil
}

// StoreBatch batches message body writes and amortizes directory creation,
// directory locking, and message_info.bin append/compaction operations.
type StoreBatch struct {
	downloadDir string
	releaseLock func()
	dirsCreated map[string]bool
	entries     []message_info.Entry
	closed      bool
}

// NewStoreBatch initializes a bulk cache writer that acquires the cache lock
// and batches metadata index updates across multiple stored messages.
func NewStoreBatch(account cachedir.Account) (*StoreBatch, error) {
	downloadDir := account.Path()
	if downloadDir == "" {
		return nil, errors.New("downloadDir cannot be empty")
	}
	return &StoreBatch{
		downloadDir: downloadDir,
		releaseLock: guard(downloadDir),
		dirsCreated: make(map[string]bool),
	}, nil
}

// Store atomically writes a message body to disk and buffers its metadata entry.
func (b *StoreBatch) Store(msgID email.ID, rawBytes []byte, receivedDate time.Time) error {
	if b.closed {
		return errors.New("store batch is closed")
	}
	dateVal := uint32(receivedDate.Year()*10000 + int(receivedDate.Month())*100 + receivedDate.Day())
	dateDir := fmt.Sprintf("%04d/%02d/%02d", receivedDate.Year(), int(receivedDate.Month()), receivedDate.Day())
	dirPath := filepath.Join(b.downloadDir, "messages", dateDir)
	if !b.dirsCreated[dirPath] {
		if err := os.MkdirAll(dirPath, 0700); err != nil {
			return err
		}
		b.dirsCreated[dirPath] = true
	}
	// A unique CreateTemp-based name (M6), not a fixed emlPath+".tmp": Store
	// can write to this same emlPath from a concurrent goroutine or process,
	// and two writers sharing one fixed temp name would interleave into a
	// truncated or mixed body.
	emlPath := filepath.Join(dirPath, string(msgID)+".eml")
	if err := dirlock.AtomicWrite(emlPath, rawBytes, 0600); err != nil {
		return err
	}
	_ = os.Remove(filepath.Join(dirPath, string(msgID)+".summary.json"))
	b.entries = append(b.entries, message_info.Entry{
		ID:      string(msgID),
		Compact: message_info.Compact{Date: dateVal},
	})
	return nil
}

// Close flushes all buffered metadata entries to message_info.bin, updates the
// in-memory directory cache, and releases the directory lock.
func (b *StoreBatch) Close() error {
	if b.closed {
		return nil
	}
	b.closed = true
	defer b.releaseLock()

	if len(b.entries) == 0 {
		return nil
	}

	ensureLoaded(b.downloadDir)
	dc := getDirCache(b.downloadDir)
	dc.mu.Lock()
	defer dc.mu.Unlock()

	for i, e := range b.entries {
		if old, ok := dc.m[e.ID]; ok {
			b.entries[i].Compact = restored(&old, e.Compact.Date)
		}
	}
	p := binPath(b.downloadDir)
	if err := message_info.AppendBatchToFile(p, b.entries); err != nil {
		return fmt.Errorf("append message_info batch: %w", err)
	}

	for _, e := range b.entries {
		dc.m[e.ID] = e.Compact
		dc.entries++
	}
	if fi, err := os.Stat(p); err == nil {
		dc.lastMod = fi.ModTime()
		dc.lastSize = fi.Size()
	}

	ratio := float64(dc.entries-int64(len(dc.m))) / float64(dc.entries)
	if dc.entries > 1000 && ratio > 0.25 {
		if err := compactMapLocked(b.downloadDir, dc); err != nil {
			slog.Error("Failed to compact message_info in batch", slog.Any("error", err))
		}
	}
	return nil
}

func summaryPath(downloadDir string, msgID email.ID) (string, error) {
	emlPath, err := lookupPath(downloadDir, msgID)
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(emlPath, ".eml") + ".llm_summary.md", nil
}

// HasSummary reports whether msgID has a cached LLM summary, accepting the
// legacy file name without the ".md" suffix too.
func HasSummary(account cachedir.Account, msgID email.ID) bool {
	downloadDir := account.Path()
	p, err := summaryPath(downloadDir, msgID)
	if err != nil {
		return false
	}
	if _, statErr := os.Stat(p); statErr == nil {
		return true
	}
	// Fallback to legacy path without .md
	legacy := strings.TrimSuffix(p, ".md")
	_, err = os.Stat(legacy)
	return err == nil
}

// ReadSummary returns msgID's cached LLM summary, falling back to the legacy
// file name. It errors when neither exists.
func ReadSummary(account cachedir.Account, msgID email.ID) (string, error) {
	downloadDir := account.Path()
	p, err := summaryPath(downloadDir, msgID)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(p)
	if err == nil {
		return string(data), nil
	}
	// Fallback to legacy .llm_summary
	legacy := strings.TrimSuffix(p, ".md")
	dataLegacy, errLegacy := os.ReadFile(legacy)
	if errLegacy != nil {
		return "", err
	}
	return string(dataLegacy), nil
}

// StoreSummary writes msgID's LLM summary into the cache, creating the
// directory if needed.
func StoreSummary(account cachedir.Account, msgID email.ID, summary string) error {
	downloadDir := account.Path()
	p, err := summaryPath(downloadDir, msgID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		return err
	}
	return os.WriteFile(p, []byte(summary), 0600)
}

func parsedSummaryPath(downloadDir string, msgID email.ID) (string, error) {
	emlPath, err := lookupPath(downloadDir, msgID)
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(emlPath, ".eml") + ".summary.json", nil
}

// ReadParsedSummary reads cached parsed summary bytes for the message if present.
func ReadParsedSummary(account cachedir.Account, msgID email.ID) ([]byte, error) {
	downloadDir := account.Path()
	p, err := parsedSummaryPath(downloadDir, msgID)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(p)
}

// StoreParsedSummary writes cached parsed summary bytes atomically for the message.
func StoreParsedSummary(account cachedir.Account, msgID email.ID, data []byte) error {
	downloadDir := account.Path()
	p, err := parsedSummaryPath(downloadDir, msgID)
	if err != nil {
		return err
	}
	tmpPath := fmt.Sprintf("%s.tmp-%d", p, os.Getpid())
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmpPath, p)
}

func infoFromCompact(c message_info.Compact) *Info {
	y := c.Date / 10000
	m := (c.Date / 100) % 100
	d := c.Date % 100
	return &Info{
		Date:     time.Date(int(y), time.Month(m), int(d), 0, 0, 0, 0, time.UTC),
		Scanned:  c.Flags&message_info.FlagScanned != 0,
		Evidence: Evidence{Checks: Checks(c.Checks), BogoScore: c.BogoScore, Stamp: c.Stamp},
		Override: Override(c.Override),
	}
}

func parsedSummaryPathFromCompact(downloadDir string, msgID email.ID, c message_info.Compact) string {
	y := c.Date / 10000
	m := (c.Date / 100) % 100
	d := c.Date % 100
	datePath := fmt.Sprintf("%04d/%02d/%02d", y, m, d)
	return filepath.Join(downloadDir, "messages", datePath, string(msgID)+".summary.json")
}

// ReadParsedSummaryAndInfo reads cached parsed summary bytes and metadata Info
// from a single index snapshot without redundant disk stats.
func ReadParsedSummaryAndInfo(account cachedir.Account, msgID email.ID) ([]byte, *Info, error) {
	downloadDir := account.Path()
	c := getCompact(downloadDir, msgID)
	if c == nil {
		return nil, nil, fmt.Errorf("message %s not in cache", msgID)
	}
	p := parsedSummaryPathFromCompact(downloadDir, msgID, *c)
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, nil, err
	}
	return data, infoFromCompact(*c), nil
}

// GetInfo returns msgID's classification from the index. It errors when the
// message is not in the cache.
func GetInfo(account cachedir.Account, msgID email.ID) (*Info, error) {
	downloadDir := account.Path()
	c := getCompact(downloadDir, msgID)
	if c == nil {
		return nil, fmt.Errorf("message %s not in cache", msgID)
	}
	return infoFromCompact(*c), nil
}

// ForEachID calls fn for every cached message ID until fn returns true. The
// order is unspecified.
func ForEachID(account cachedir.Account, fn func(id email.ID) bool) {
	downloadDir := account.Path()
	ensureLoaded(downloadDir)
	dc := getDirCache(downloadDir)
	dc.mu.RLock()
	defer dc.mu.RUnlock()
	for id := range dc.m {
		if fn(email.ID(id)) {
			return
		}
	}
}

// ForgetAllEvidence drops every message's scan evidence, keeping the user's
// overrides, so the next scan judges each message afresh.
func ForgetAllEvidence(account cachedir.Account) error {
	downloadDir := account.Path()
	defer guard(downloadDir)()
	ensureLoaded(downloadDir)
	dc := getDirCache(downloadDir)
	dc.mu.Lock()
	defer dc.mu.Unlock()
	for id, c := range dc.m {
		dc.m[id] = c.WithoutEvidence()
	}
	dc.entries = int64(len(dc.m))

	// The lock stays held through the rewrite: releasing it here handed dc.m to
	// RewriteFile for iteration while another goroutine could be writing to it,
	// which is a data race and can abort the process with
	// "concurrent map iteration and map write".
	p := binPath(downloadDir)
	err := message_info.RewriteFile(p, dc.m)
	if fi, sErr := os.Stat(p); sErr == nil {
		dc.lastMod = fi.ModTime()
		dc.lastSize = fi.Size()
	}
	return err
}
