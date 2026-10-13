package msg

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/sarielhp/mail_cli/cache/cachedir"
	"github.com/sarielhp/mail_cli/message_info"
)

// RepairReport is what Repair found in an account's index, and -- when it was
// asked to fix -- what it changed.
type RepairReport struct {
	// Records is the number of messages the index knows.
	Records int
	// Superseded counts records a later one replaced: every update appends.
	Superseded int
	// OldFormat counts version 1 and 2 records.
	OldFormat int
	// Damaged says the log ended in a record that could not be read.
	Damaged bool
	// Orphans are records with no message file; they are dropped.
	Orphans int
	// KeptOrphans are records with no message file that hold the user's
	// decision. They are kept, so a re-download gets the decision back.
	KeptOrphans int
	// Unindexed are message files with no record; they are indexed again.
	Unindexed int
	// Decided counts the messages kept that carry the user's decision.
	Decided int
	// Duplicates counts extra copies of a message filed under another date.
	// They are reported, not removed.
	Duplicates int
	// Misdated are records whose date names another directory than the one
	// the file is in, so the message could not be found.
	Misdated int
}

// NeedsRewrite says whether fixing would change the index file.
func (r RepairReport) NeedsRewrite() bool {
	return r.Superseded > 0 || r.OldFormat > 0 || r.Damaged ||
		r.Orphans > 0 || r.Unindexed > 0 || r.Misdated > 0
}

// Repair checks an account's index against the message files on disk and,
// when fix is set, rewrites it: one current-format record per message, a
// record for every file, and none for a missing file unless it holds the
// user's decision. Without fix it changes nothing.
func Repair(account cachedir.Account, fix bool) (RepairReport, error) {
	downloadDir := account.Path()
	defer guard(downloadDir)()
	dc := getDirCache(downloadDir)
	dc.mu.Lock()
	defer dc.mu.Unlock()

	binP := binPath(downloadDir)
	records, stats, err := message_info.ReadFileStats(binP)
	rep := RepairReport{Damaged: errors.Is(err, message_info.ErrDamaged)}
	if err != nil && !rep.Damaged {
		return rep, err
	}
	files, err := messageFiles(downloadDir)
	if err != nil {
		return rep, fmt.Errorf("listing cached messages: %w", err)
	}

	repaired := reconcile(records, files, &rep)
	rep.Records = len(records)
	rep.Superseded = stats.Records - len(records)
	rep.OldFormat = stats.Old
	if !fix || !rep.NeedsRewrite() {
		return rep, nil
	}

	if err := message_info.RewriteFile(binP, repaired); err != nil {
		return rep, err
	}
	dc.m = repaired
	dc.path = binP
	dc.loaded = true
	dc.entries = int64(len(repaired))
	if fi, err := os.Stat(binP); err == nil {
		dc.lastMod = fi.ModTime()
		dc.lastSize = fi.Size()
	}
	return rep, nil
}

// reconcile returns the index records should be, given the message files on
// disk, and counts what differs in rep.
func reconcile(records map[string]message_info.Compact, files map[string][]uint32, rep *RepairReport) map[string]message_info.Compact {
	repaired := make(map[string]message_info.Compact, len(files))
	for id, c := range records {
		dates, onDisk := files[id]
		switch {
		case onDisk && !slices.Contains(dates, c.Date):
			rep.Misdated++
			c.Date = dates[0]
		case onDisk:
		case c.Override != message_info.OverrideNone:
			rep.KeptOrphans++
		default:
			rep.Orphans++
			continue
		}
		if c.Override != message_info.OverrideNone {
			rep.Decided++
		}
		repaired[id] = c
	}
	for id, dates := range files {
		rep.Duplicates += len(dates) - 1
		if _, ok := records[id]; !ok {
			rep.Unindexed++
			repaired[id] = message_info.Compact{Date: dates[0]}
		}
	}
	return repaired
}

// messageFiles maps the ID of every message file under messages/YYYY/MM/DD to
// the dates its directories name: more than one when a copy was filed twice.
func messageFiles(downloadDir string) (map[string][]uint32, error) {
	files := make(map[string][]uint32)
	root := filepath.Join(downloadDir, "messages")
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) && path == root {
				return filepath.SkipDir
			}
			return err
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".eml") {
			return nil
		}
		id := strings.TrimSuffix(d.Name(), ".eml")
		if date, ok := dateFromPath(path); ok && id != "" {
			files[id] = append(files[id], date)
		}
		return nil
	})
	return files, err
}

// dateFromPath reads the date a message file's YYYY/MM/DD directories name,
// as message_info stores it.
func dateFromPath(path string) (uint32, bool) {
	dayDir := filepath.Dir(path)
	monthDir := filepath.Dir(dayDir)
	yearDir := filepath.Dir(monthDir)
	var y, m, d int
	spec := filepath.Base(yearDir) + " " + filepath.Base(monthDir) + " " + filepath.Base(dayDir)
	if _, err := fmt.Sscanf(spec, "%d %02d %02d", &y, &m, &d); err != nil {
		return 0, false
	}
	return uint32(y*10000 + m*100 + d), true
}
