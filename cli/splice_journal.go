package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/sarielhp/mail_cli/app"
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/label"
	"github.com/sarielhp/mail_cli/mailclient"
)

// SpliceJournalEntry tracks a message uploaded to destination but pending source deletion.
type SpliceJournalEntry struct {
	SourceID   string `json:"source_id"`
	DestID     string `json:"dest_id"`
	DestFolder string `json:"dest_folder"`
}

// SpliceJournal provides a crash-resilient write-ahead log for cross-account splice operations.
type SpliceJournal struct {
	mu           sync.Mutex
	filePath     string
	SourceFolder string                        `json:"source_folder"`
	Entries      map[string]SpliceJournalEntry `json:"entries"`
}

// OpenSpliceJournal loads or initializes the journal in the account's cache.
func OpenSpliceJournal(baseDir cachedir.Account, sourceFolder string) (*SpliceJournal, error) {
	fp := filepath.Join(baseDir.Path(), "splice_journal.json")
	j := &SpliceJournal{
		filePath:     fp,
		SourceFolder: sourceFolder,
		Entries:      make(map[string]SpliceJournalEntry),
	}
	data, err := os.ReadFile(fp)
	if err == nil {
		if unmarshalErr := json.Unmarshal(data, j); unmarshalErr != nil {
			return nil, fmt.Errorf("failed to parse existing splice journal %q: %w", fp, unmarshalErr)
		}
		if j.Entries == nil {
			j.Entries = make(map[string]SpliceJournalEntry)
		}
		return j, nil
	}
	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to read splice journal %q: %w", fp, err)
	}
	return j, nil
}

// RecordUpload logs that a source message was confirmed uploaded to the destination server.
func (j *SpliceJournal) RecordUpload(srcID, destID, destFolder string) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.Entries[srcID] = SpliceJournalEntry{
		SourceID:   srcID,
		DestID:     destID,
		DestFolder: destFolder,
	}
	return j.save()
}

// RecordDeleted removes an entry after successful deletion from the source server.
func (j *SpliceJournal) RecordDeleted(srcID string) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	delete(j.Entries, srcID)
	if len(j.Entries) == 0 {
		_ = os.Remove(j.filePath)
		return nil
	}
	return j.save()
}

func (j *SpliceJournal) save() error {
	data, err := json.MarshalIndent(j, "", "  ")
	if err != nil {
		return err
	}
	tmp := j.filePath + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, j.filePath)
}

// Close removes the journal file if all operations completed cleanly.
func (j *SpliceJournal) Close() error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if len(j.Entries) == 0 {
		_ = os.Remove(j.filePath)
	}
	return nil
}

// RecoverSpliceJournal resumes cleanup for any messages confirmed in destination but not yet deleted from source.
func RecoverSpliceJournal(journal *SpliceJournal, srcClient mailclient.MailClient, session *Session, source label.Folder) error {
	if journal == nil || len(journal.Entries) == 0 {
		return nil
	}
	fmt.Fprintf(session.errOut(), "%s Found %d uncompleted transfer(s) in journal from interrupted run. Resuming cleanup...\n", app.PrefixWarn, len(journal.Entries))
	for srcID, entry := range journal.Entries {
		fmt.Fprintf(session.out(), "   Recovering: source %s was confirmed at dest %s in %s; deleting from source...\n", srcID, entry.DestID, entry.DestFolder)
		if err := trashSpliceSource(session, srcClient, email.ID(srcID), source); err != nil {
			fmt.Fprintf(os.Stderr, "   Failed to delete %s from source during journal recovery: %v\n", srcID, err)
			continue
		}
		_ = journal.RecordDeleted(srcID)
	}
	return nil
}
