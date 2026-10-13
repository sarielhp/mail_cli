package contacts

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"github.com/sarielhp/mail_cli/email"
	"io"
	"net/mail"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sarielhp/mail_cli/app"
	"github.com/sarielhp/mail_cli/cache/folderindex"
	"github.com/sarielhp/mail_cli/cache/msg"
	"github.com/sarielhp/mail_cli/cfg_g"
)

// HistoryBook stores harvested communication history with interaction frequency and recency.
type HistoryBook struct {
	Updated time.Time       `json:"updated"`
	Entries []ScoredContact `json:"entries"`
}

// NewHistoryBook returns an empty HistoryBook.
func NewHistoryBook() *HistoryBook {
	return &HistoryBook{
		Updated: time.Now().UTC(),
		Entries: []ScoredContact{},
	}
}

// DefaultHistoryPath returns the file path to suggested_contacts.json in the cache directory.
func DefaultHistoryPath(config *cfg_g.Config) string {
	// CacheRoot is always the root. This used to guess, from the directory
	// names, whether it had been handed the root or an account's directory,
	// because both arrived in the same field.
	if config != nil && config.CacheRoot != "" {
		return filepath.Join(filepath.Clean(config.CacheRoot), "suggested_contacts.json")
	}
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".cache", app.AppName, "suggested_contacts.json")
	}
	return filepath.Join(cacheDir, app.AppName, "suggested_contacts.json")
}

// LoadHistory reads suggested_contacts.json, returning an empty HistoryBook if not found.
func LoadHistory(path string) (*HistoryBook, error) {
	return loadJSONFile(path, "history", NewHistoryBook)
}

// SaveHistory atomically writes suggested_contacts.json.
func SaveHistory(path string, hb *HistoryBook) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	hb.Updated = time.Now().UTC()
	data, err := json.MarshalIndent(hb, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to serialize history: %w", err)
	}

	tmpFile := fmt.Sprintf("%s.tmp.%d", path, time.Now().UnixNano())
	if err := os.WriteFile(tmpFile, data, 0o600); err != nil {
		return fmt.Errorf("failed to write temporary history file: %w", err)
	}

	if err := os.Rename(tmpFile, path); err != nil {
		_ = os.Remove(tmpFile)
		return fmt.Errorf("failed to commit history file %s: %w", path, err)
	}
	return nil
}

// PurgeAddress removes an email from suggested_contacts.json.
func PurgeAddress(path string, email string) error {
	hb, err := LoadHistory(path)
	if err != nil {
		return err
	}

	clean := strings.ToLower(strings.TrimSpace(email))
	var kept []ScoredContact
	removed := false

	for _, e := range hb.Entries {
		if strings.ToLower(e.PrimaryEmail()) == clean {
			removed = true
			continue
		}
		kept = append(kept, e)
	}

	if !removed {
		return nil
	}

	hb.Entries = kept
	return SaveHistory(path, hb)
}

// HarvestFromMessageStream parses email headers and updates candidate frequencies.
func HarvestFromMessageStream(r io.Reader, isSent, isArchived bool, blacklist map[string]bool, source string, targetMap map[string]*ScoredContact) error {
	msgParsed, err := mail.ReadMessage(r)
	if err != nil {
		return err
	}

	// Mailing list check: skip automated list traffic
	if isMailingList(msgParsed.Header) {
		return nil
	}

	msgDate, _ := msgParsed.Header.Date()

	var headerVals []string
	if isSent {
		if to := msgParsed.Header.Get("To"); to != "" {
			headerVals = append(headerVals, to)
		}
		if cc := msgParsed.Header.Get("Cc"); cc != "" {
			headerVals = append(headerVals, cc)
		}
	} else {
		if from := msgParsed.Header.Get("From"); from != "" {
			headerVals = append(headerVals, from)
		}
	}

	for _, h := range headerVals {
		addrs, err := mail.ParseAddressList(h)
		if err != nil {
			continue
		}
		for _, a := range addrs {
			recordAddress(a, isSent, isArchived, blacklist, source, msgDate, targetMap)
		}
	}

	return nil
}

func isMailingList(hdr mail.Header) bool {
	if hdr.Get("List-Id") != "" || hdr.Get("List-Unsubscribe") != "" || hdr.Get("List-Post") != "" {
		return true
	}
	precedence := strings.ToLower(hdr.Get("Precedence"))
	if precedence == "list" || precedence == "bulk" {
		return true
	}
	return hdr.Get("Mailing-List") != ""
}

func recordAddress(a *mail.Address, isSent, isArchived bool, blacklist map[string]bool, source string, msgDate time.Time, targetMap map[string]*ScoredContact) {
	if a == nil || a.Address == "" {
		return
	}
	email := strings.ToLower(strings.TrimSpace(a.Address))
	if isAutomatedAddress(email) || isBlacklisted(email, blacklist) {
		return
	}

	name := strings.TrimSpace(a.Name)
	existing, found := targetMap[email]
	if !found {
		existing = &ScoredContact{
			Contact: Contact{
				ID:     fmt.Sprintf("harvest:%s", email),
				Name:   name,
				Emails: []string{email},
				Source: source,
			},
			IsSent:     isSent,
			IsArchived: isArchived,
		}
		targetMap[email] = existing
	}

	if existing.Name == "" && name != "" {
		existing.Name = name
	}
	if isSent {
		existing.IsSent = true
	}
	if isArchived {
		existing.IsArchived = true
	}
	existing.Frequency++
	if !msgDate.IsZero() && msgDate.After(existing.LastSeen) {
		existing.LastSeen = msgDate
	}
}

func isAutomatedAddress(email string) bool {
	if !strings.Contains(email, "@") {
		return true
	}
	parts := strings.Split(email, "@")
	user := parts[0]

	ignoredPrefixes := []string{
		"no-reply", "noreply", "donotreply", "do-not-reply",
		"notifications", "notification", "mailer-daemon",
		"postmaster", "bounce", "bounces", "system", "automated",
	}

	for _, p := range ignoredPrefixes {
		if strings.HasPrefix(user, p) {
			return true
		}
	}
	return false
}

// HarvestAccountCache scans all cached messages in an account directory into the target map.
func HarvestAccountCache(account cachedir.Account, accountName string, blacklist []string, targetMap map[string]*ScoredContact) (int, error) {
	downloadDir := account
	if _, err := os.Stat(downloadDir.Path()); os.IsNotExist(err) {
		return 0, nil
	}

	bMap := make(map[string]bool, len(blacklist))
	for _, b := range blacklist {
		bMap[strings.ToLower(strings.TrimSpace(b))] = true
	}

	sentIDs := loadFolderIDs(downloadDir, []string{"[Gmail]/Sent Mail", "Sent", "Sent Items", "sent"})
	arcIDs := loadFolderIDs(downloadDir, []string{"[Gmail]/All Mail", "Archive", "archive"})
	source := fmt.Sprintf("harvest:%s", accountName)
	count := 0

	msg.ForEachID(downloadDir, func(id email.ID) bool {
		isSent := sentIDs[id]
		isArchived := arcIDs[id]
		err := msg.WithReader(downloadDir, id, func(r io.Reader) error {
			return HarvestFromMessageStream(r, isSent, isArchived, bMap, source, targetMap)
		})
		if err == nil {
			count++
		}
		return false
	})

	return count, nil
}

func loadFolderIDs(downloadDir cachedir.Account, candidateFolders []string) map[email.ID]bool {
	idMap := make(map[email.ID]bool)
	for _, folder := range candidateFolders {
		ids, err := folderindex.IDs(downloadDir, folder)
		if err == nil && len(ids) > 0 {
			for _, id := range ids {
				idMap[id] = true
			}
		}
	}
	return idMap
}

// RecordSentRecipients records recipients of an outbound email into suggestion history.
func RecordSentRecipients(config *cfg_g.Config, rawBytes []byte) {
	if len(rawBytes) == 0 {
		return
	}
	msgParsed, err := mail.ReadMessage(bytes.NewReader(rawBytes))
	if err != nil {
		return
	}

	var blacklist map[string]bool
	if config != nil {
		blacklist = make(map[string]bool, len(config.Policy.Blacklist))
		for _, b := range config.Policy.Blacklist {
			blacklist[strings.ToLower(strings.TrimSpace(b))] = true
		}
	}

	historyPath := DefaultHistoryPath(config)
	hb, err := LoadHistory(historyPath)
	if err != nil {
		hb = NewHistoryBook()
	}

	targetMap := make(map[string]*ScoredContact, len(hb.Entries))
	for i := range hb.Entries {
		targetMap[strings.ToLower(hb.Entries[i].PrimaryEmail())] = &hb.Entries[i]
	}

	var recipients []string
	if to := msgParsed.Header.Get("To"); to != "" {
		recipients = append(recipients, to)
	}
	if cc := msgParsed.Header.Get("Cc"); cc != "" {
		recipients = append(recipients, cc)
	}

	now := time.Now().UTC()
	updated := false
	for _, r := range recipients {
		addrs, err := mail.ParseAddressList(r)
		if err != nil {
			continue
		}
		for _, a := range addrs {
			recordAddress(a, true, false, blacklist, "outbound:sent", now, targetMap)
			updated = true
		}
	}

	if updated {
		rebuildAndSaveHistory(historyPath, hb, targetMap)
	}
}

// RecordArchivedSender records the sender of an archived message into suggestion history.
func RecordArchivedSender(config *cfg_g.Config, downloadDir cachedir.Account, messageID email.ID) {
	if messageID == "" {
		return
	}
	data, err := msg.Read(downloadDir, messageID)
	if err != nil {
		return
	}
	msgParsed, err := mail.ReadMessage(bytes.NewReader(data))
	if err != nil {
		return
	}
	if isMailingList(msgParsed.Header) {
		return
	}

	fromHdr := msgParsed.Header.Get("From")
	if fromHdr == "" {
		return
	}
	addr, err := mail.ParseAddress(fromHdr)
	if err != nil || addr == nil || addr.Address == "" {
		return
	}

	var blacklist map[string]bool
	if config != nil {
		blacklist = make(map[string]bool, len(config.Policy.Blacklist))
		for _, b := range config.Policy.Blacklist {
			blacklist[strings.ToLower(strings.TrimSpace(b))] = true
		}
	}

	historyPath := DefaultHistoryPath(config)
	hb, err := LoadHistory(historyPath)
	if err != nil {
		hb = NewHistoryBook()
	}

	targetMap := make(map[string]*ScoredContact, len(hb.Entries))
	for i := range hb.Entries {
		targetMap[strings.ToLower(hb.Entries[i].PrimaryEmail())] = &hb.Entries[i]
	}

	recordAddress(addr, false, true, blacklist, "archive", time.Now().UTC(), targetMap)
	rebuildAndSaveHistory(historyPath, hb, targetMap)
}

// PurgeMessageSenders purges the senders of the specified cached messages from suggestion history.
func PurgeMessageSenders(config *cfg_g.Config, downloadDir cachedir.Account, messageIDs []email.ID) {
	if len(messageIDs) == 0 {
		return
	}
	historyPath := DefaultHistoryPath(config)
	hb, err := LoadHistory(historyPath)
	if err != nil || len(hb.Entries) == 0 {
		return
	}

	purgedMap := make(map[string]bool)
	for _, id := range messageIDs {
		data, err := msg.Read(downloadDir, id)
		if err != nil {
			continue
		}
		parsed, err := mail.ReadMessage(bytes.NewReader(data))
		if err != nil {
			continue
		}
		addr, err := mail.ParseAddress(parsed.Header.Get("From"))
		if err == nil && addr != nil && addr.Address != "" {
			purgedMap[strings.ToLower(strings.TrimSpace(addr.Address))] = true
		}
	}

	if len(purgedMap) == 0 {
		return
	}

	var kept []ScoredContact
	removed := false
	for _, e := range hb.Entries {
		if purgedMap[strings.ToLower(e.PrimaryEmail())] {
			removed = true
			continue
		}
		kept = append(kept, e)
	}

	if removed {
		hb.Entries = kept
		_ = SaveHistory(historyPath, hb)
	}
}

func rebuildAndSaveHistory(path string, hb *HistoryBook, targetMap map[string]*ScoredContact) {
	hb.Entries = make([]ScoredContact, 0, len(targetMap))
	for _, e := range targetMap {
		hb.Entries = append(hb.Entries, *e)
	}
	_ = SaveHistory(path, hb)
}
