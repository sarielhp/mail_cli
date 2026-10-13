package folderindex

import (
	"bufio"
	"encoding/json"
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"github.com/sarielhp/mail_cli/email"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/sarielhp/mail_cli/cache/dirlock"
	"github.com/sarielhp/mail_cli/cfg_g"
)

func idsPath(downloadDir, folder string) string {
	return filepath.Join(downloadDir, "indexes", cfg_g.SanitizeLabelForCache(folder)+".json")
}

func deltasPath(downloadDir, folder string) string {
	return filepath.Join(downloadDir, "indexes", cfg_g.SanitizeLabelForCache(folder)+".deltas.jsonl")
}

func readDeltas(path string) ([]email.ID, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var deltas []email.ID
	scanner := bufio.NewScanner(bufio.NewReaderSize(f, 64*1024))
	for scanner.Scan() {
		var id email.ID
		if err := json.Unmarshal(scanner.Bytes(), &id); err == nil && id != "" {
			deltas = append(deltas, id)
		}
	}
	return deltas, scanner.Err()
}

// IDs returns the message IDs indexed under folder, unioning the base index
// with the IDs appended since its last rewrite and dropping duplicates. It
// returns os.ErrNotExist when the folder has neither file.
func IDs(account cachedir.Account, folder string) ([]email.ID, error) {
	downloadDir := account.Path()
	p := idsPath(downloadDir, folder)
	data, err := os.ReadFile(p)
	var ids []email.ID
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if err == nil {
		if uErr := json.Unmarshal(data, &ids); uErr != nil {
			return nil, uErr
		}
	}

	deltas, dErr := readDeltas(deltasPath(downloadDir, folder))
	if dErr != nil {
		return nil, dErr
	}
	if err != nil && len(deltas) == 0 {
		return nil, os.ErrNotExist
	}
	if len(deltas) == 0 {
		return ids, nil
	}

	seen := make(map[email.ID]bool, len(ids)+len(deltas))
	for _, id := range ids {
		seen[id] = true
	}
	for _, id := range deltas {
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids, nil
}

// ReplaceAll replaces folder's whole index with msgIDs, discarding any pending
// deltas, and is a no-op when the account has no cache directory.
func ReplaceAll(account cachedir.Account, folder string, msgIDs []email.ID) error {
	downloadDir := account.Path()
	if downloadDir == "" {
		return nil
	}
	defer guard(downloadDir)()
	return replaceAllLocked(downloadDir, folder, msgIDs)
}

func replaceAllLocked(downloadDir, folder string, msgIDs []email.ID) error {
	if downloadDir == "" {
		return nil
	}
	data, err := json.Marshal(msgIDs)
	if err != nil {
		return err
	}
	if err := dirlock.AtomicWrite(idsPath(downloadDir, folder), data, 0600); err != nil {
		return err
	}
	_ = os.Remove(deltasPath(downloadDir, folder))
	return nil
}

// AppendBatch appends new message IDs to a folder's index without rewriting existing entries.
func AppendBatch(account cachedir.Account, folder string, msgIDs []email.ID) error {
	downloadDir := account.Path()
	if downloadDir == "" || len(msgIDs) == 0 {
		return nil
	}
	defer guard(downloadDir)()
	return appendBatchLocked(downloadDir, folder, msgIDs)
}

func appendBatchLocked(downloadDir, folder string, msgIDs []email.ID) error {
	p := idsPath(downloadDir, folder)
	if _, err := os.Stat(p); os.IsNotExist(err) {
		return replaceAllLocked(downloadDir, folder, msgIDs)
	}

	dp := deltasPath(downloadDir, folder)
	f, err := os.OpenFile(dp, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	for _, id := range msgIDs {
		if err := enc.Encode(id); err != nil {
			return err
		}
	}
	return f.Sync()
}

// Add records msgID under folder. Safe against concurrent callers in this
// process and against other mail_cli processes sharing downloadDir.
func Add(account cachedir.Account, folder string, msgID email.ID) error {
	downloadDir := account.Path()
	defer guard(downloadDir)()
	return addLocked(downloadDir, folder, msgID)
}

// Remove drops msgID from folder.
func Remove(account cachedir.Account, folder string, msgID email.ID) error {
	downloadDir := account.Path()
	defer guard(downloadDir)()
	return removeLocked(downloadDir, folder, msgID)
}

// Move relocates msgID from one folder index to another. The whole operation is
// held under one guard, so a concurrent reader never observes the message in
// neither folder.

// MoveBatch relocates multiple message IDs between folders in a single disk read/write cycle.
// MoveBatch is the only mover.
//
// There used to be a single-ID Move(downloadDir, msgID, fromFolder, toFolder)
// beside it: four adjacent string parameters with the ID in a different
// position from Add and Remove, which put it third. Nothing but care stopped a
// caller swapping the ID and the folder, and the compiler could not tell. The
// []string here distinguishes the ID argument by type, which care does not have
// to. A single move is MoveBatch(dir, []string{id}, from, to).
func MoveBatch(account cachedir.Account, msgIDs []email.ID, fromFolder, toFolder string) error {
	downloadDir := account.Path()
	if fromFolder == toFolder || len(msgIDs) == 0 {
		return nil
	}
	defer guard(downloadDir)()

	fromIDs, err := IDs(account, fromFolder)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	toIDs, err := IDs(account, toFolder)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	existingTo := make(map[email.ID]bool, len(toIDs)+len(msgIDs))
	for _, id := range toIDs {
		existingTo[id] = true
	}
	added := false
	for _, id := range msgIDs {
		if !existingTo[id] {
			toIDs = append(toIDs, id)
			existingTo[id] = true
			added = true
		}
	}

	moveSet := make(map[email.ID]bool, len(msgIDs))
	for _, id := range msgIDs {
		moveSet[id] = true
	}
	var filteredFrom []email.ID
	removed := false
	for _, id := range fromIDs {
		if moveSet[id] {
			removed = true
		} else {
			filteredFrom = append(filteredFrom, id)
		}
	}

	if added {
		if err := replaceAllLocked(downloadDir, toFolder, toIDs); err != nil {
			return err
		}
	}
	if len(fromIDs) > 0 && removed {
		if err := replaceAllLocked(downloadDir, fromFolder, filteredFrom); err != nil {
			return err
		}
	}
	return nil
}

// PurgeIDs removes the specified message IDs from all folder indexes,
// including any pending ".deltas.jsonl" entries appended since the folder's
// last full rewrite. Cleaning only the base ".json" file is not enough:
// IDs() unions the base with the deltas file on every read, so an ID purged
// there alone reappears the moment anything reads the folder again.
func PurgeIDs(account cachedir.Account, msgIDs []email.ID) error {
	downloadDir := account.Path()
	if downloadDir == "" || len(msgIDs) == 0 {
		return nil
	}
	defer guard(downloadDir)()

	idxDir := filepath.Join(downloadDir, "indexes")
	entries, err := os.ReadDir(idxDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	purgeSet := make(map[email.ID]bool, len(msgIDs))
	for _, id := range msgIDs {
		purgeSet[id] = true
	}

	// A folder index is split across "<key>.json" and an optional
	// "<key>.deltas.jsonl", both keyed by the folder's already-sanitized
	// name. Collect each key once so both halves purge together.
	keys := make(map[string]bool)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		switch {
		case strings.HasSuffix(e.Name(), ".deltas.jsonl"):
			keys[strings.TrimSuffix(e.Name(), ".deltas.jsonl")] = true
		case strings.HasSuffix(e.Name(), ".json"):
			keys[strings.TrimSuffix(e.Name(), ".json")] = true
		}
	}

	// One folder's malformed index must not abort every other folder's
	// purge (a regression the H5 rewrite itself introduced: the old
	// per-file loop silently skipped a bad file and kept going, which this
	// restores, now with a log line instead of full silence). cache prune's
	// caller only warns on PurgeIDs' own error rather than treating it as
	// fatal, on the same reasoning -- one folder's problem should cost that
	// folder's purge, not everyone else's.
	for key := range keys {
		if err := purgeIndexKey(idxDir, key, purgeSet); err != nil {
			slog.Warn("failed to purge folder index", slog.String("key", key), slog.Any("error", err))
		}
	}
	return nil
}

// purgeIndexKey removes purgeSet's IDs from one folder's base-plus-deltas
// index pair, keyed directly by its on-disk (already-sanitized) name so it
// does not risk re-sanitizing an already-escaped name (cfg_g.SanitizeLabelForCache
// is not its own inverse). It always folds a present deltas file back into
// the base and removes it, even when nothing in this folder was purged,
// since leaving it would let the very entries just read straight back in on
// the next append.
func purgeIndexKey(idxDir, key string, purgeSet map[email.ID]bool) error {
	basePath := filepath.Join(idxDir, key+".json")
	deltaPath := filepath.Join(idxDir, key+".deltas.jsonl")

	var ids []email.ID
	if data, err := os.ReadFile(basePath); err == nil {
		if uErr := json.Unmarshal(data, &ids); uErr != nil {
			return uErr
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	deltas, dErr := readDeltas(deltaPath)
	if dErr != nil {
		return dErr
	}
	hadDeltas := len(deltas) > 0
	if hadDeltas {
		seen := make(map[email.ID]bool, len(ids)+len(deltas))
		for _, id := range ids {
			seen[id] = true
		}
		for _, id := range deltas {
			if !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
	}

	var filtered []email.ID
	removed := false
	for _, id := range ids {
		if purgeSet[id] {
			removed = true
		} else {
			filtered = append(filtered, id)
		}
	}
	if !removed && !hadDeltas {
		return nil
	}

	newData, err := json.Marshal(filtered)
	if err != nil {
		return err
	}
	if err := dirlock.AtomicWrite(basePath, newData, 0600); err != nil {
		return err
	}
	if hadDeltas {
		_ = os.Remove(deltaPath)
	}
	return nil
}

// guard acquires the cache lock, returning the release function. A lock failure
// is logged and tolerated: the in-process mutex is still held, so this degrades
// to the previous behavior rather than failing the user's command.
func guard(downloadDir string) func() {
	release, err := dirlock.Guard(downloadDir)
	if err != nil {
		slog.Debug("cache lock unavailable, continuing with in-process lock only",
			slog.String("dir", downloadDir), slog.Any("error", err))
	}
	return release
}

func addLocked(downloadDir, folder string, msgID email.ID) error {
	ids, err := IDs(cachedir.At(downloadDir), folder)
	if err != nil {
		if os.IsNotExist(err) {
			return replaceAllLocked(downloadDir, folder, []email.ID{msgID})
		}
		return err
	}
	for _, id := range ids {
		if id == msgID {
			return nil
		}
	}
	return replaceAllLocked(downloadDir, folder, append(ids, msgID))
}

func removeLocked(downloadDir, folder string, msgID email.ID) error {
	ids, err := IDs(cachedir.At(downloadDir), folder)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for i, id := range ids {
		if id == msgID {
			return replaceAllLocked(downloadDir, folder, append(ids[:i:i], ids[i+1:]...))
		}
	}
	return nil
}

// HasStructure reports whether the account's cache already holds an indexes
// directory, which is what a populated folder index leaves behind.
func HasStructure(account cachedir.Account) bool {
	downloadDir := account.Path()
	_, err := os.Stat(filepath.Join(downloadDir, "indexes"))
	return err == nil
}
