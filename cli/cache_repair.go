package cli

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/sarielhp/mail_cli/app"
	"github.com/sarielhp/mail_cli/cache/msg"
	"github.com/sarielhp/mail_cli/train"

	"github.com/sarielhp/clihelp"
)

// runCacheRepair checks the selected account's index against its message
// files and the training ledgers, and rewrites the index unless --dry-run or
// --read-only asked for a report. It touches only local files.
func runCacheRepair(session *Session) error {
	account := resolveAccountCacheDir(session)
	if account.Path() == "" {
		return fmt.Errorf("%w: no account selected", app.ErrUsage)
	}
	fix := !session.Config.DryRun && !session.Config.ReadOnly
	rep, err := msg.Repair(account, fix)
	if err != nil {
		return fmt.Errorf("repairing %s: %w", account, err)
	}

	w := session.out()
	fmt.Fprintf(w, "Index of %s\n", account)
	printRepairReport(w, rep)
	printLedgerHealth(w, []string{
		train.SpamUIDPath(account),
		train.HamPath(account),
		train.MessageIDPath(session.Config.ConfigDir),
	})

	switch {
	case !rep.NeedsRewrite():
		fmt.Fprintln(w, "Nothing to repair.")
	case fix:
		fmt.Fprintf(w, "Rewrote the index: %d record(s).\n", rep.Records-rep.Orphans+rep.Unindexed)
	default:
		fmt.Fprintln(w, "Report only (--dry-run or --read-only); run without either to rewrite the index.")
	}
	return nil
}

func printRepairReport(w io.Writer, rep msg.RepairReport) {
	fmt.Fprintf(w, "  %d message(s), %d carrying your spam/not-spam decision\n", rep.Records, rep.Decided)
	fmt.Fprintf(w, "  %d record(s) in the log: %d superseded, %d in an older format\n",
		rep.Records+rep.Superseded, rep.Superseded, rep.OldFormat)
	if rep.Damaged {
		fmt.Fprintln(w, "  the index ends in a damaged record")
	}
	fmt.Fprintf(w, "  %d record(s) with no message file", rep.Orphans+rep.KeptOrphans)
	if rep.KeptOrphans > 0 {
		fmt.Fprintf(w, " (%d kept: they hold your decision)", rep.KeptOrphans)
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "  %d message file(s) with no record, %d filed under the wrong date\n", rep.Unindexed, rep.Misdated)
	if rep.Duplicates > 0 {
		fmt.Fprintf(w, "  %d duplicate message file(s) under another date, left in place\n", rep.Duplicates)
	}
}

// printLedgerHealth reports a damaged training ledger without touching it:
// guessing its contents would retrain bogofilter on what it already learned.
func printLedgerHealth(w io.Writer, paths []string) {
	for _, p := range paths {
		if _, err := train.Open(p); err != nil {
			fmt.Fprintf(w, "  %s is damaged and was left alone: %v\n", filepath.Base(p), err)
		}
	}
}

func newCacheRepairCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:        "repair",
		Description: "Check and rewrite this account's index.",
		LongDescription: "Check the account's message index against the cached message files and rewrite it: " +
			"one current-format record per message, a record for every file, and none for a missing file " +
			"unless it holds your spam/not-spam decision, which a re-download then gets back. " +
			"Damaged training ledgers are reported but never rewritten. Only local files are touched.",
		UsageLine: "mail_cli utils cache repair",
		Args:      clihelp.NoArgs,
		Notes: []clihelp.Note{
			{Heading: "Reporting only", Text: "With --dry-run or --read-only it reports what it would change and changes nothing."},
		},
		Run: func(_ *clihelp.Context) error {
			return runCacheRepair(session)
		},
		Examples: []clihelp.Example{
			{Line: "mail_cli --dry-run utils cache repair", Description: "Report what a repair would change"},
			{Line: "mail_cli utils cache repair"},
		},
	}
}
