package cli

import (
	"bytes"
	"fmt"
	"github.com/sarielhp/mail_cli/label"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/sarielhp/mail_cli/app"
	"github.com/sarielhp/mail_cli/cache/msg"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/mailclient"

	"github.com/sarielhp/clihelp"
)

// isEmailMatch checks if two parsed emails represent the same message.
// If both emails have a Message-ID header, that is authoritative: mismatch immediately returns false.
// Subject and non-zero date comparison is only used as a fallback when Message-ID is missing.
func isEmailMatch(src, dest *email.Email) bool {
	if src == nil || dest == nil {
		return false
	}
	if src.MessageID != "" && dest.MessageID != "" {
		return src.MessageID.Same(dest.MessageID)
	}
	// Fallback only if at least one side lacks Message-ID, subjects match, and dates are valid & equal
	if src.Subject != "" && dest.Subject == src.Subject && !src.EmailDate.IsZero() && src.EmailDate.Equal(dest.EmailDate) {
		return true
	}
	return false
}

// messageIdentityIndex indexes the identity of messages without reference to
// any folder: each Message-ID, plus the Subject+Date fallback for messages
// that lack one. mbox deduplication and splice's destination check both match
// on exactly these, so the matching core lives here once.
type messageIdentityIndex struct {
	messageIDs            map[email.HeaderID]struct{}
	subjectDatesWithID    map[string]struct{}
	subjectDatesWithoutID map[string]struct{}
}

func newMessageIdentityIndex(capacity int) *messageIdentityIndex {
	return &messageIdentityIndex{
		messageIDs:            make(map[email.HeaderID]struct{}, capacity),
		subjectDatesWithID:    make(map[string]struct{}, capacity),
		subjectDatesWithoutID: make(map[string]struct{}, capacity),
	}
}

func (idx *messageIdentityIndex) add(em *email.Email) {
	if em == nil {
		return
	}
	if em.MessageID != "" {
		idx.messageIDs[em.MessageID.Folded()] = struct{}{}
	}
	if em.Subject != "" && !em.EmailDate.IsZero() {
		k := subjectDateKey(em)
		if em.MessageID != "" {
			idx.subjectDatesWithID[k] = struct{}{}
			return
		}
		idx.subjectDatesWithoutID[k] = struct{}{}
	}
}

func (idx *messageIdentityIndex) contains(em *email.Email) bool {
	if idx == nil || em == nil {
		return false
	}
	if em.MessageID != "" {
		if _, ok := idx.messageIDs[em.MessageID.Folded()]; ok {
			return true
		}
		if em.Subject != "" && !em.EmailDate.IsZero() {
			_, ok := idx.subjectDatesWithoutID[subjectDateKey(em)]
			return ok
		}
		return false
	}
	if em.Subject != "" && !em.EmailDate.IsZero() {
		k := subjectDateKey(em)
		if _, ok := idx.subjectDatesWithoutID[k]; ok {
			return true
		}
		_, ok := idx.subjectDatesWithID[k]
		return ok
	}
	return false
}

// subjectDateKey is the fallback identity of a message without a Message-ID:
// its subject together with its send time.
func subjectDateKey(em *email.Email) string {
	return fmt.Sprintf("%s|%d", em.Subject, em.EmailDate.Unix())
}

// destFolderIndex is a messageIdentityIndex plus the server and cache IDs seen
// in one destination folder.
type destFolderIndex struct {
	ids map[email.ID]bool
	*messageIdentityIndex
}

func newDestFolderIndex(size int) *destFolderIndex {
	return &destFolderIndex{
		ids:                  make(map[email.ID]bool, size),
		messageIdentityIndex: newMessageIdentityIndex(size),
	}
}

func (idx *destFolderIndex) add(msgID, serverID email.ID, em *email.Email) {
	if msgID != "" {
		idx.ids[msgID] = true
	}
	if serverID != "" {
		idx.ids[serverID] = true
	}
	idx.messageIdentityIndex.add(em)
}

func (idx *destFolderIndex) contains(msgID email.ID, srcEmail *email.Email) bool {
	if idx == nil {
		return false
	}
	if idx.ids[msgID] {
		return true
	}
	return idx.messageIdentityIndex.contains(srcEmail)
}

// destCacheSubdir is the cache subdirectory a destination folder is listed
// into.
func destCacheSubdir(folder label.Folder) string {
	if sub := cfg_g.SanitizeLabelForCache(folder.String()); sub != "" {
		return sub
	}
	return folder.String()
}

func buildDestFolderIndex(destClient mailclient.Reader, destFolder label.Folder) *destFolderIndex {
	targetIDs, err := destClient.FetchAndDownloadEmails(destFolder, destCacheSubdir(destFolder))
	if err != nil {
		slog.Warn("buildDestFolderIndex: failed to fetch destination folder", slog.String("folder", destFolder.String()), slog.Any("error", err))
		return newDestFolderIndex(0)
	}

	idx := newDestFolderIndex(len(targetIDs))
	destDir := destClient.Config().AccountCache
	for _, eID := range targetIDs {
		idx.ids[eID] = true
		rawBytes, err := msg.Read(destDir, eID)
		if err != nil {
			slog.Warn("buildDestFolderIndex: read error for cached email", slog.String("msgID", string(eID)), slog.Any("error", err))
			continue
		}
		destEmail := email.ParseReader(bytes.NewReader(rawBytes), eID, "")
		if destEmail != nil {
			idx.add(eID, "", destEmail)
		}
	}
	return idx
}

// isMessageInDest checks whether a message already exists in the destination folder.
// Results are cached per folder in destCache.
func isMessageInDest(destClient mailclient.MailClient, destFolder label.Folder, msgID email.ID, srcEmail *email.Email, destCache map[label.Folder]*destFolderIndex) bool {
	idx, ok := destCache[destFolder]
	if !ok {
		idx = buildDestFolderIndex(destClient, destFolder)
		destCache[destFolder] = idx
	}
	return idx.contains(msgID, srcEmail)
}

// confirmMessageInDest verifies a message exists on the destination server by fetching
// from the server (not cache) and checking by ID, Message-ID, or Subject+Date match.
// Retries with exponential backoff to account for server eventual consistency.
func confirmMessageInDest(destClient mailclient.Reader, destFolder label.Folder, msgID email.ID, srcEmail *email.Email) (bool, error) {
	cacheSubdir := destCacheSubdir(destFolder)

	delay := confirmRetryDelay
	for attempt := 1; attempt <= maxConfirmRetries; attempt++ {
		targetIDs, err := destClient.FetchAndDownloadEmails(destFolder, cacheSubdir)
		if err != nil {
			if attempt < maxConfirmRetries {
				slog.Debug("confirmMessageInDest: retry after fetch error", slog.Int("attempt", attempt), slog.Any("error", err))
				time.Sleep(delay)
				delay *= 2
				continue
			}
			return false, fmt.Errorf("failed to fetch destination folder %q from server after %d attempts: %w", destFolder, attempt, err)
		}

		destDir := destClient.Config().AccountCache
		for _, eID := range targetIDs {
			if eID == msgID {
				return true, nil
			}
			if srcEmail != nil {
				rawBytes, err := msg.Read(destDir, eID)
				if err != nil {
					continue
				}
				destEmail := email.ParseReader(bytes.NewReader(rawBytes), eID, "")
				if isEmailMatch(srcEmail, destEmail) {
					return true, nil
				}
			}
		}

		if attempt < maxConfirmRetries {
			slog.Debug("confirmMessageInDest: message not found yet, retrying", slog.Int("attempt", attempt), slog.String("msgID", string(msgID)), slog.String("folder", destFolder.String()))
			time.Sleep(delay)
			delay *= 2
		}
	}

	return false, fmt.Errorf("message %s not confirmed on server in destination folder %q after %d attempts", msgID, destFolder, maxConfirmRetries)
}

// ValidateSpliceArgs validates folder naming and message count constraints for splice.
func ValidateSpliceArgs(folder string, numMessages int, allowKeep bool) error {
	if !allowKeep && (strings.EqualFold(folder, "keep") || strings.HasPrefix(strings.ToLower(folder), "keep/")) {
		return fmt.Errorf("source folder %q must not be \"keep\" or start with \"keep/\" unless --allow is specified", folder)
	}
	if numMessages <= 0 {
		return fmt.Errorf("flag -n must be at least 1")
	}
	return nil
}

func isSameAccount(c1, c2 mailclient.Reader) bool {
	if c1 == nil || c2 == nil {
		return false
	}
	// Use backend type + config identity rather than pointer comparison,
	// since CheckingMailClient wrappers may wrap the same backend.
	if c1.BackendType() != c2.BackendType() {
		return false
	}
	cfg1 := c1.Config()
	cfg2 := c2.Config()
	if cfg1 == nil || cfg2 == nil {
		return false
	}
	if !cfg1.AccountCache.IsZero() && !cfg2.AccountCache.IsZero() {
		return cfg1.AccountCache == cfg2.AccountCache
	}
	if cfg1.SelectedAccount != "" && cfg2.SelectedAccount != "" {
		return strings.EqualFold(string(cfg1.SelectedAccount), string(cfg2.SelectedAccount))
	}
	if cfg1.Username != "" && cfg2.Username != "" {
		return strings.EqualFold(cfg1.Username, cfg2.Username)
	}
	return false
}

type spliceExampleItem struct {
	title       string
	command     string
	explanation string
	srcLabel    string
	destLabel   string
	mode        string
}

// SpliceExamplesCmd returns the clihelp.Command for "mail_cli splice examples".
func SpliceExamplesCmd() clihelp.Command {
	return clihelp.Command{
		Name:            "examples",
		Title:           "splice examples",
		Description:     "Show archive mapping patterns and examples.",
		LongDescription: "Display structured examples with detailed explanations showing concrete source to destination label mappings.",
		UsageLine:       "mail_cli splice examples",
		Args:            clihelp.NoArgs,
		Run: func(ctx *clihelp.Context) error {
			printSpliceExamples(ctxStdout(ctx))
			return nil
		},
	}
}

func formatExampleLabel(label string) string {
	parts := strings.SplitN(label, ":", 2)
	if len(parts) == 2 {
		return parts[0] + ":" + app.ColorBlue.Sprint(parts[1])
	}
	return app.ColorBlue.Sprint(label)
}

func printSpliceExamples(w io.Writer) {
	fmt.Fprintf(w, "%s Structured Splice Examples & Mapping Patterns:\n\n", app.PrefixInfo)
	examples := getSpliceExampleList()
	for i, ex := range examples {
		fmt.Fprintf(w, "%d. %s\n", i+1, app.ColorBold.Sprint(ex.title))
		fmt.Fprintf(w, "   Command:     %s\n", app.ColorGreen.Sprint(ex.command))
		fmt.Fprintf(w, "   Explanation: %s\n", ex.explanation)
		fmt.Fprintf(w, "   Mapping:     %s -> %s\n", formatExampleLabel(ex.srcLabel), formatExampleLabel(ex.destLabel))
		if ex.mode != "" {
			fmt.Fprintf(w, "   Mode:        %s\n", ex.mode)
		}
		fmt.Fprintln(w)
	}
}

func getSpliceExampleList() []spliceExampleItem {
	return []spliceExampleItem{
		{
			title:       "Basic Date Splice (Dry Run)",
			command:     "mail_cli splice %GMail:receipts",
			explanation: "Preview organizing messages from folder 'receipts' into monthly archive folders based on email dates without modifying messages.",
			srcLabel:    "GMail:receipts",
			destLabel:   "GMail:keep/2026/09/receipts",
			mode:        "Dry run (default; no changes made)",
		},
		{
			title:       "Live Message Move (--move)",
			command:     "mail_cli splice %GMail:receipts --move",
			explanation: "Actually moves messages into destination archive folders; verifies each message on the destination server before removing from source.",
			srcLabel:    "GMail:receipts",
			destLabel:   "GMail:keep/2026/09/receipts",
			mode:        "Live move (copies to destination, confirms, removes from source)",
		},
		{
			title:       "Custom Destination Folder Name (-f, --folder)",
			command:     "mail_cli splice %GMail:inbox -f Archive --move",
			explanation: "Moves messages into a custom named folder under keep/YYYY/MM/ instead of using the source folder name.",
			srcLabel:    "GMail:inbox",
			destLabel:   "GMail:keep/2026/09/Archive",
		},
		{
			title:       "Folder with Year-Month Suffix (-F, --folder-suffix)",
			command:     "mail_cli splice %GMail:services -F services --move",
			explanation: "Appends the message date suffix to the folder name under keep/YYYY/MM/<name>-YYYY-MM.",
			srcLabel:    "GMail:services",
			destLabel:   "GMail:keep/2026/09/services-2026-09",
		},
		{
			title:       "Year-Only Directory Hierarchy (-Y, --folder-year)",
			command:     "mail_cli splice %GMail:services -Y services --move",
			explanation: "Places messages directly under keep/YYYY/ with year-month suffix attached (omitting the MM subdirectory).",
			srcLabel:    "GMail:services",
			destLabel:   "GMail:keep/2026/services-2026-09",
		},
		{
			title:       "Cross-Account Splicing (GMail to FastMail)",
			command:     "mail_cli splice %GMail:received -Y %FastMail:services --move",
			explanation: "Migrates messages from account 'GMail' to 'FastMail' under keep/YYYY/services-YYYY-MM and trashes from source once verified.",
			srcLabel:    "GMail:received",
			destLabel:   "FastMail:keep/2026/services-2026-09",
			mode:        "Cross-account transfer (uploads to target, verifies, trashes source)",
		},
		{
			title:       "Copying Messages Without Deletion (--copy)",
			command:     "mail_cli splice %GMail:receipts --copy -n 50",
			explanation: "Copies up to 50 messages to destination archive folders without deleting them from the source folder.",
			srcLabel:    "GMail:receipts",
			destLabel:   "GMail:keep/2026/09/receipts",
			mode:        "Copy (duplicates into destination archive)",
		},
		{
			title:       "Subject Pattern Filtering (-p, --pattern)",
			command:     `mail_cli splice %GMail:inbox -p "*order*" -f Orders --move`,
			explanation: "Processes only messages whose subject matches the wildcard pattern '*order*' (* and ? wildcards supported).",
			srcLabel:    "GMail:inbox",
			destLabel:   "GMail:keep/2026/09/Orders",
		},
	}
}
