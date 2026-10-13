package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/sarielhp/mail_cli/app"
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/label"
	"github.com/sarielhp/mail_cli/mailclient"
)

// splicePlan is one splice run: the folder messages come from, how each
// message's destination is named, and the client on each side.
//
// Folders are label.Folder throughout. The source came out of resolution;
// every destination is a keep/... name computeDest builds, validated by
// label.ForCreate before anything is moved, so a name the backends would
// refuse is reported per message instead of failing halfway through a run.
// Only the crash journal holds folder names as text, because it is a file.
type splicePlan struct {
	session    *Session
	src, dest  mailclient.MailClient
	source     label.Folder
	destBase   string
	yearSuffix bool
	suffix     bool
}

func (p *splicePlan) baseDir() cachedir.Account { return p.src.Config().AccountCache }

func (p *splicePlan) sameAccount() bool { return isSameAccount(p.src, p.dest) }

func (p *splicePlan) srcDisplay() string {
	return FormatAccountFolder(p.session, p.src, p.source.String())
}

func (p *splicePlan) destDisplay(dest label.Folder) string {
	return FormatAccountFolder(p.session, p.dest, dest.String())
}

// destFor names the destination of a message dated year/month.
func (p *splicePlan) destFor(year, month string) (label.Folder, error) {
	return label.ForCreate(computeDest(year, month, p.yearSuffix, p.suffix, p.destBase))
}

// isSource reports whether dest is the folder being spliced, which happens
// when the source is already a keep/ folder for that month.
func (p *splicePlan) isSource(dest label.Folder) bool {
	return p.sameAccount() && strings.EqualFold(p.source.String(), dest.String())
}

// spliceItem is one message loaded for splicing and the folder it goes to.
type spliceItem struct {
	id     email.ID
	parsed *email.Email
	raw    []byte
	dest   label.Folder
}

// load reads message i of total and names its destination, reporting a
// message that cannot be loaded or placed.
func (p *splicePlan) load(i, total int, id email.ID) (spliceItem, bool) {
	parsed, raw, year, month, err := parseEmailDate(p.baseDir(), id)
	if err == nil {
		var dest label.Folder
		if dest, err = p.destFor(year, month); err == nil {
			return spliceItem{id: id, parsed: parsed, raw: raw, dest: dest}, true
		}
	}
	fmt.Fprintf(os.Stderr, "  [%d/%d] Error loading %s: %v\n", i+1, total, id, err)
	return spliceItem{}, false
}

func (p *splicePlan) skipSelf(i, total int, it spliceItem) bool {
	if !p.isSource(it.dest) {
		return false
	}
	fmt.Fprintf(p.session.errOut(), "[%d/%d] %s %s | Subject: %q\n", i+1, total, app.PrefixWarn, strings.TrimSpace(string(it.id)), it.parsed.Subject)
	fmt.Fprintf(p.session.errOut(), "    %s\n", app.ColorYellow.Sprint("(source equals destination, skipping)"))
	return true
}

// fetchIDs lists the source folder, applies --pattern and -n, and says so
// when nothing is left.
func (p *splicePlan) fetchIDs(labelCache SanitizeLabelCache, limit int) ([]email.ID, error) {
	ids, err := p.src.FetchAndDownloadEmails(p.source, labelCache.Get(p.source.String()))
	if err != nil {
		return nil, fmt.Errorf("failed to fetch emails for %s: %w", p.srcDisplay(), err)
	}
	if flagSplicePattern != "" {
		ids = filterByPattern(ids, p.baseDir())
	}
	if len(ids) == 0 {
		if flagSplicePattern != "" {
			fmt.Fprintf(p.session.errOut(), "%s No messages in folder %s match pattern %q.\n", app.PrefixWarn, p.srcDisplay(), flagSplicePattern)
		} else {
			fmt.Fprintf(p.session.errOut(), "%s No messages found in folder %s.\n", app.PrefixWarn, p.srcDisplay())
		}
		return nil, nil
	}
	if limit > 0 && len(ids) > limit {
		ids = ids[:limit]
	}
	return ids, nil
}

// run splices ids in the mode the flags chose and returns every destination
// it touched.
func (p *splicePlan) run(ids []email.ID) (map[label.Folder]bool, error) {
	switch {
	case !flagSpliceCopy && !flagSpliceMove:
		return p.dryRun(ids), nil
	case flagSpliceCopy:
		return p.copyAll(ids), nil
	case p.sameAccount():
		return p.moveSameAccount(ids)
	default:
		return p.moveCrossAccount(ids)
	}
}

func (p *splicePlan) dryRun(ids []email.ID) map[label.Folder]bool {
	destCache := make(map[label.Folder]*destFolderIndex)
	touched := make(map[label.Folder]bool)
	for i, id := range ids {
		if i > 0 {
			fmt.Fprintln(p.session.out(), "---------------")
		}
		it, ok := p.load(i, len(ids), id)
		if !ok {
			continue
		}
		touched[it.dest] = true
		if p.skipSelf(i, len(ids), it) {
			continue
		}
		exists := isMessageInDest(p.dest, it.dest, it.id, it.parsed, destCache)
		printSpliceSuccess(p.session.out(), i, len(ids), it.id, it.parsed.Subject)
		fmt.Fprintf(p.session.out(), "    %s\n       →   %s\n", p.srcDisplay(), p.destDisplay(it.dest))
		note := app.ColorCyan.Sprint("(would move to destination)")
		if exists {
			note = app.ColorYellow.Sprint("(already exists in destination)")
		}
		fmt.Fprintf(p.session.out(), "    %s\n", note)
	}
	return touched
}

func (p *splicePlan) copyAll(ids []email.ID) map[label.Folder]bool {
	destCache := make(map[label.Folder]*destFolderIndex)
	ensured := make(map[label.Folder]bool)
	touched := make(map[label.Folder]bool)
	for i, id := range ids {
		if i > 0 {
			fmt.Fprintln(p.session.out(), "---------------")
		}
		it, ok := p.load(i, len(ids), id)
		if !ok {
			continue
		}
		touched[it.dest] = true
		if p.skipSelf(i, len(ids), it) {
			continue
		}
		p.copyOne(i, len(ids), it, destCache, ensured)
	}
	return touched
}

func (p *splicePlan) copyOne(i, total int, it spliceItem, destCache map[label.Folder]*destFolderIndex, ensured map[label.Folder]bool) {
	out := p.session.out()
	route := fmt.Sprintf("    %s\n       →   %s", p.srcDisplay(), p.destDisplay(it.dest))
	if isMessageInDest(p.dest, it.dest, it.id, it.parsed, destCache) {
		fmt.Fprintf(p.session.errOut(), "[%d/%d] %s %s | Subject: %q\n", i+1, total, app.PrefixInfo, strings.TrimSpace(string(it.id)), it.parsed.Subject)
		fmt.Fprintf(p.session.errOut(), "%s\n", route)
		fmt.Fprintf(out, "    %s\n", app.ColorYellow.Sprint("(already exists in destination, skipping copy)"))
		return
	}
	if err := p.ensureDest(it.dest, ensured); err != nil {
		printSpliceError(out, i, total, it.id, it.parsed.Subject)
		fmt.Fprintf(out, "%s (failed to create folder: %v)\n", route, err)
		return
	}
	destID, err := p.copyOrUpload(it)
	if err != nil {
		printSpliceError(out, i, total, it.id, it.parsed.Subject)
		what := "failed to upload to target account"
		if p.sameAccount() {
			what = "failed to copy"
		}
		fmt.Fprintf(out, "%s (%s: %v)\n", route, what, err)
		return
	}
	indexFor(destCache, it.dest).add(it.id, destID, it.parsed)
	printSpliceSuccess(out, i, total, it.id, it.parsed.Subject)
	fmt.Fprintf(out, "%s\n    %s\n", route, app.ColorGreen.Sprint("(copied to destination)"))
}

// moveSameAccount groups messages by destination and moves each group with
// server-side moves, 250 at a time.
func (p *splicePlan) moveSameAccount(ids []email.ID) (map[label.Folder]bool, error) {
	touched := make(map[label.Folder]bool)
	groups := make(map[label.Folder][]email.ID)
	subjects := make(map[email.ID]string)
	for i, id := range ids {
		it, ok := p.load(i, len(ids), id)
		if !ok || p.isSource(it.dest) {
			continue
		}
		groups[it.dest] = append(groups[it.dest], id)
		subjects[id] = it.parsed.Subject
		touched[it.dest] = true
	}

	done := 0
	for dest, group := range groups {
		if err := p.src.EnsureLabelExists(dest); err != nil {
			return touched, fmt.Errorf("failed to ensure destination folder %q: %w", dest, err)
		}
		for start := 0; start < len(group); start += 250 {
			chunk := group[start:min(start+250, len(group))]
			// No folderindex.MoveBatch here: the client is a CheckingMailClient,
			// whose MoveEmail reindexes the chunk itself, gated on its own
			// read-only setting and reconciled to what the server moved.
			if err := p.src.MoveEmail(chunk, p.source, dest); err != nil {
				return touched, fmt.Errorf("failed to move %d message(s) to %s: %w", len(chunk), dest, err)
			}
			p.reportMoved(chunk, dest, subjects, &done, len(ids))
		}
	}
	return touched, nil
}

func (p *splicePlan) reportMoved(chunk []email.ID, dest label.Folder, subjects map[email.ID]string, done *int, total int) {
	out := p.session.out()
	for idx, id := range chunk {
		*done++
		printSpliceSuccess(out, *done-1, total, id, subjects[id])
		fmt.Fprintf(out, "    %s\n       →   %s\n", p.srcDisplay(), p.destDisplay(dest))
		fmt.Fprintf(out, "    %s\n", app.ColorGreen.Sprint("(atomic server move)"))
		if idx < len(chunk)-1 {
			fmt.Fprintln(out, "---------------")
		}
	}
}

// crossMove is the state a cross-account move carries from message to
// message: what is known to be in each destination, which destinations exist,
// and the journal that lets an interrupted run finish its deletions.
type crossMove struct {
	destCache map[label.Folder]*destFolderIndex
	ensured   map[label.Folder]bool
	journal   *SpliceJournal
}

func (p *splicePlan) moveCrossAccount(ids []email.ID) (map[label.Folder]bool, error) {
	journal, err := OpenSpliceJournal(p.baseDir(), p.source.String())
	if err != nil {
		return nil, fmt.Errorf("cross-account move refusing to run without its crash journal: %w", err)
	}
	defer func() { _ = journal.Close() }()
	_ = RecoverSpliceJournal(journal, p.src, p.session, p.source)

	state := crossMove{
		destCache: make(map[label.Folder]*destFolderIndex),
		ensured:   make(map[label.Folder]bool),
		journal:   journal,
	}
	touched := make(map[label.Folder]bool)
	for i, id := range ids {
		if i > 0 {
			fmt.Fprintln(p.session.out(), "---------------")
		}
		it, ok := p.load(i, len(ids), id)
		if !ok {
			continue
		}
		if p.moveOneCrossAccount(i, len(ids), it, state) {
			touched[it.dest] = true
		}
	}
	return touched, nil
}

// moveOneCrossAccount uploads one message, confirms it arrived, and only then
// removes it from the source. It reports whether the message was moved.
func (p *splicePlan) moveOneCrossAccount(i, total int, it spliceItem, state crossMove) bool {
	out := p.session.out()
	route := fmt.Sprintf("    %s\n       →   %s", p.srcDisplay(), p.destDisplay(it.dest))
	fail := func(format string, err error) bool {
		printSpliceError(out, i, total, it.id, it.parsed.Subject)
		fmt.Fprintf(out, "%s ("+format+")\n", route, err)
		return false
	}
	if err := p.ensureDest(it.dest, state.ensured); err != nil {
		return fail("failed to create folder: %v", err)
	}

	exists := isMessageInDest(p.dest, it.dest, it.id, it.parsed, state.destCache)
	var destID email.ID
	if !exists {
		var err error
		if destID, err = p.copyOrUpload(it); err != nil {
			return fail("failed to upload to target account: %v", err)
		}
	}
	indexFor(state.destCache, it.dest).add(it.id, destID, it.parsed)

	if destID == "" && !exists {
		if confirmed, err := confirmMessageInDest(p.dest, it.dest, it.id, it.parsed); !confirmed || err != nil {
			return fail("unconfirmed in destination; source message NOT deleted: %v", err)
		}
	}

	_ = state.journal.RecordUpload(string(it.id), string(destID), it.dest.String())
	if err := trashSpliceSource(p.session, p.src, it.id, p.source); err != nil {
		printSpliceWarn(out, i, total, it.id, it.parsed.Subject)
		fmt.Fprintf(out, " (confirmed in destination, but failed to remove from source: %v)\n", err)
		return false
	}
	_ = state.journal.RecordDeleted(string(it.id))

	printSpliceSuccess(out, i, total, it.id, it.parsed.Subject)
	note := "(confirmed in destination, removed from source)"
	if exists {
		note = "(already existed in destination, confirmed and removed from source)"
	}
	fmt.Fprintf(out, "%s\n    %s\n", route, app.ColorGreen.Sprint(note))
	return true
}

// ensureDest creates dest on the destination side once per run.
func (p *splicePlan) ensureDest(dest label.Folder, ensured map[label.Folder]bool) error {
	if ensured[dest] {
		return nil
	}
	if err := p.dest.EnsureLabelExists(dest); err != nil {
		return err
	}
	ensured[dest] = true
	return nil
}

// copyOrUpload copies within an account, or uploads to another, returning the
// message's ID on the destination side.
func (p *splicePlan) copyOrUpload(it spliceItem) (email.ID, error) {
	if p.sameAccount() {
		return it.id, p.src.CopyEmail([]email.ID{it.id}, p.source, it.dest)
	}
	return p.dest.UploadRawEmail(it.raw, it.dest)
}

// trashSpliceSource removes a message that now lives in another account from
// the source folder, by moving it to the source account's trash.
func trashSpliceSource(session *Session, src mailclient.MailClient, id email.ID, source label.Folder) error {
	trash := label.FromServer("Trash")
	if t, err := session.organize().ResolveTrashTarget(src); err == nil && !t.IsZero() {
		trash = t
	}
	return src.MoveEmail([]email.ID{id}, source, trash)
}

func indexFor(destCache map[label.Folder]*destFolderIndex, dest label.Folder) *destFolderIndex {
	if destCache[dest] == nil {
		destCache[dest] = newDestFolderIndex(1)
	}
	return destCache[dest]
}

// printTotals prints the message counts of the source and every destination
// after the run.
func (p *splicePlan) printTotals(touched map[label.Folder]bool, labelCache SanitizeLabelCache) {
	srcIDs, _ := p.src.FetchAndDownloadEmails(p.source, labelCache.Get(p.source.String()))
	fmt.Fprintf(p.session.errOut(), "\n%s Folder totals after splice:\n", app.PrefixInfo)
	fmt.Fprintf(p.session.errOut(), "    %s: %d message(s)\n", p.srcDisplay(), len(srcIDs))
	for dest := range touched {
		destIDs, err := p.dest.FetchAndDownloadEmails(dest, labelCache.Get(dest.String()))
		if err == nil {
			fmt.Fprintf(p.session.out(), "    %s: %d message(s)\n", p.destDisplay(dest), len(destIDs))
		}
	}
}
