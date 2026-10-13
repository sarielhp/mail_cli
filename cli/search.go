package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/sarielhp/mail_cli/app"
	"github.com/sarielhp/mail_cli/cache/msg"
	"github.com/sarielhp/mail_cli/cache/virtual"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/mailclient"
	"github.com/sarielhp/mail_cli/ui"

	"github.com/sarielhp/clihelp"
)

var (
	flagSearchOutput  string
	flagSearchLimit   int
	flagSearchAll     bool
	flagSearchReverse bool
)

// defaultSearchLimit keeps an unqualified search from pulling a decade of mail.
const defaultSearchLimit = 50

// SearchCmd returns the clihelp.Command for the search command.
func SearchCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:        "search",
		Group:       "Daily",
		Description: "Search email history or export to mbox.",
		UsageLine:   "mail_cli search <query> [flags]",
		Parameters: []clihelp.Param{
			{Name: "<query>", Description: "Query string (free text or search filters)."},
		},
		Options: []clihelp.Option{
			clihelp.String(&flagSearchOutput, "-o, --output <file.mbox>", "", "Write matches to an mbox file"),
			clihelp.Int(&flagSearchLimit, "-n, --limit <num>", defaultSearchLimit, "Maximum number of matches to retrieve"),
			clihelp.Bool(&flagSearchAll, "--all", false, "Retrieve every match, however many there are"),
			clihelp.Bool(&flagSearchReverse, "-r, --reverse", false, "List newest first instead of oldest first"),
		},
		Examples: []clihelp.Example{
			{Line: "mail_cli search \"taxes 2025\""},
			{Line: "mail_cli search from:alice@example.com -n 25"},
			{Line: "mail_cli -A work search \"from:manager has:attachment\""},
			{Line: "mail_cli search from:lawyer@firm.com --all -o legal.mbox", Description: "Export every match to an archive"},
		},
		Args: clihelp.ExactArgs(1),
		Run: func(ctx *clihelp.Context) error {
			c := ctx.Context
			if c == nil {
				c = context.Background()
			}
			return runSearchContext(c, session, ctx.Args[0])
		},
	}
}

func runSearch(session *Session, query string) error {
	return runSearchContext(context.Background(), session, query)
}

func runSearchContext(ctx context.Context, session *Session, query string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	client, err := GetValidatedClient(session)
	if err != nil {
		return err
	}

	limit := flagSearchLimit
	if flagSearchAll {
		limit = 0
	}

	ids, err := client.SearchEmails(query, limit)
	if err != nil {
		return fmt.Errorf("search %q: %w", query, err)
	}
	if len(ids) == 0 {
		if wantJSON() {
			return session.emitJSON([]email.SummaryJSON{})
		}
		fmt.Fprintf(session.errOut(), "%s No messages match %q.\n", app.PrefixInfo, query)
		return nil
	}

	if flagSearchOutput != "" {
		return exportSearchResults(ctx, session.out(), client, ids, query)
	}
	return listSearchResults(ctx, session, client, ids, query)
}

// listSearchResults prints the matches as a table, oldest first so the newest
// match ends up next to the prompt rather than scrolled off the top.
func listSearchResults(ctx context.Context, session *Session, client mailclient.Reader, ids []email.ID, query string) error {
	out := session.out()
	status := session.errOut()
	summaries, err := client.GetMessageSummariesBatchContext(ctx, ids)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		// Partial results are still worth showing; the messages that did come
		// back are the ones the user asked for.
		fmt.Fprintf(status, "%s %v\n", app.PrefixWarn, err)
	}

	emails := sortedSearchEmails(summaries)
	if len(emails) == 0 {
		return fmt.Errorf("found %d match(es) for %q but none could be retrieved", len(ids), query)
	}

	saveSearchMailbox(client, emails, query)

	if wantJSON() {
		return session.emitJSON(email.ToSummariesJSON(emails, nil))
	}

	title := fmt.Sprintf("SEARCH: %s (%d match(es))", query, len(emails))
	ui.FormatEmailsWithFolders(out, title, emails, nil, client.Config())

	if len(emails) < len(ids) {
		fmt.Fprintf(status, "%s %d of %d match(es) could not be retrieved.\n",
			app.PrefixWarn, len(ids)-len(emails), len(ids))
	}
	return nil
}

// sortedSearchEmails drops the misses and orders what remains chronologically,
// newest last.
func sortedSearchEmails(summaries []*email.Email) []email.Email {
	emails := make([]email.Email, 0, len(summaries))
	for _, s := range summaries {
		if s != nil {
			emails = append(emails, *s)
		}
	}
	sortSearchEmails(emails)
	return emails
}

// sortSearchEmails orders matches oldest first, so the newest lands at the
// bottom of a listing next to the prompt and an archive reads forwards in time.
// --reverse flips a listing for the cases where the newest match is the point.
func sortSearchEmails(emails []email.Email) {
	sort.SliceStable(emails, func(i, j int) bool {
		if flagSearchReverse {
			return emails[i].EmailDate.After(emails[j].EmailDate)
		}
		return emails[i].EmailDate.Before(emails[j].EmailDate)
	})
}

// saveSearchMailbox records the result set so `show <id>` and the TUI can walk
// it afterwards, the same way `last` does.
func saveSearchMailbox(client mailclient.Reader, emails []email.Email, query string) {
	cfg := client.Config()
	if cfg == nil || cfg.AccountCache.IsZero() {
		return
	}
	vm := &virtual.Mailbox{
		Name:        "search",
		Description: fmt.Sprintf("Results for %q", query),
		MessageIDs:  make([]email.ID, len(emails)),
		FolderMap:   make(map[email.ID]string, len(emails)),
	}
	for i, em := range emails {
		vm.MessageIDs[i] = em.ID
	}
	if err := virtual.Save(cfg.AccountCache, vm); err != nil {
		fmt.Fprintf(os.Stderr, "%s could not save the search result set: %v\n", app.PrefixWarn, err)
	}
}

// exportSearchResults streams the matches into an mbox file.
//
// The messages are cached first and then read back one at a time rather than
// held together in memory: --all can match thousands of messages, and some of
// them carry large attachments.
func exportSearchResults(ctx context.Context, w io.Writer, client mailclient.Reader, ids []email.ID, query string) error {
	fmt.Fprintf(w, "%s Retrieving %d match(es) for %q...\n", app.PrefixInfo, len(ids), query)

	ordered, err := cacheAndOrderForExport(ctx, client, ids)
	if err != nil {
		return err
	}
	if len(ordered) == 0 {
		return fmt.Errorf("none of the %d match(es) could be retrieved", len(ids))
	}

	written, err := writeSearchMbox(client.Config().AccountCache, flagSearchOutput, ordered)
	if err != nil {
		return err
	}

	fmt.Fprintf(w, "%s Wrote %d message(s) to %s\n", app.PrefixSuccess, written, flagSearchOutput)
	if written < len(ids) {
		fmt.Fprintf(w, "%s %d match(es) could not be retrieved.\n", app.PrefixWarn, len(ids)-written)
	}
	return nil
}

// cacheAndOrderForExport downloads whatever is missing and returns the IDs in
// the order they belong in the archive: oldest first, so the file reads as a
// timeline.
//
// GetMessageSummariesBatch caches every match and reads them back one at a
// time, so this stays bounded however many matches --all turns up; the bodies
// are then streamed out of the cache by writeSearchMbox.
func cacheAndOrderForExport(ctx context.Context, client mailclient.Reader, ids []email.ID) ([]email.ID, error) {
	summaries, err := client.GetMessageSummariesBatchContext(ctx, ids)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		fmt.Fprintf(os.Stderr, "%s %v\n", app.PrefixWarn, err)
	}

	emails := sortedSearchEmails(summaries)
	ordered := make([]email.ID, 0, len(emails))
	for _, em := range emails {
		ordered = append(ordered, em.ID)
	}
	return ordered, nil
}

func writeSearchMbox(downloadDir cachedir.Account, destPath string, ids []email.ID) (int, error) {
	if dir := filepath.Dir(destPath); dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return 0, fmt.Errorf("creating %q: %w", dir, err)
		}
	}

	tmpPath := destPath + ".tmp"
	tmpFile, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return 0, fmt.Errorf("creating temporary archive: %w", err)
	}
	abandon := func(cause error) (int, error) {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
		return 0, cause
	}

	bw := bufio.NewWriterSize(tmpFile, 64*1024)
	written := 0
	for _, id := range ids {
		raw, rErr := msg.Read(downloadDir, id)
		if rErr != nil {
			continue
		}
		fromAddr, dateVal := extractEnvelopeFromDate(raw)
		if err := writeToMbox(bw, raw, fromAddr, dateVal); err != nil {
			return abandon(err)
		}
		written++
	}

	if err := bw.Flush(); err != nil {
		return abandon(err)
	}
	if err := tmpFile.Sync(); err != nil {
		return abandon(err)
	}
	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return 0, err
	}
	if err := os.Rename(tmpPath, destPath); err != nil {
		_ = os.Remove(tmpPath)
		return 0, fmt.Errorf("finalizing %q: %w", destPath, err)
	}
	return written, nil
}
