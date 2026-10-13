package cli

import (
	"fmt"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/label"
	"strings"

	"github.com/sarielhp/mail_cli/app"
	"github.com/sarielhp/mail_cli/mailclient"
	"github.com/sarielhp/mail_cli/organize"

	"github.com/sarielhp/clihelp"
)

// The server side of drafts: the copies the compose screen uploads to the
// account's Drafts folder. Nothing here deletes; a draft leaves Drafts by
// moving to the trash, which the mail client can do on every backend. The
// decisions -- which drafts a name means, which a prune would trash -- are
// made below, in organize; this file only asks and renders.

func draftsServerCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:        "server",
		Description: "List, trash or prune drafts on the server.",
		UsageLine:   "mail_cli msg drafts server <ls | rm <id>... | prune [--sent] [--by-subject]>",
		LongDescription: "Works on the selected account's Drafts folder. 'ls' shows each " +
			"draft with the short ID 'rm' takes; a draft that cannot be read is listed as " +
			"unreadable so it can still be removed. 'prune' trashes older copies of a " +
			"message, recognised by a shared Message-ID, which compose gives every copy it " +
			"uploads in one session. With --sent it also trashes drafts whose Message-ID is " +
			"in the Sent folder.",
		Notes: []clihelp.Note{
			{
				Heading: "Matching by subject",
				Text: "--by-subject also treats drafts with the same recipients and subject as copies " +
					"when one of them has no Message-ID, and with --sent matches such a draft to a " +
					"message sent after it. Two messages that both carry Message-IDs are the same only " +
					"if the IDs agree, so it cannot merge copies that compose uploaded before " +
					"2026-09-30, when each upload got a fresh ID: remove those with 'rm' after 'ls'.",
			},
			{
				Heading: "What it reads and changes",
				Text: "Sent is read up to the account's fetch limit, and --sent downloads it into the " +
					"cache. Under --dry-run or --read-only every move is reported and none is made.",
			},
		},
		Subcommands: []clihelp.Command{
			{
				Name:        "ls",
				Description: "List the drafts on the server.",
				UsageLine:   "mail_cli msg drafts server ls",
				Args:        clihelp.MinimumNArgs(0),
				Run:         func(*clihelp.Context) error { return runServerDraftsList(session) },
			},
			{
				Name:        "rm",
				NoAbbrev:    true,
				Description: "Trash server drafts by ID, or all of them.",
				UsageLine:   "mail_cli msg drafts server rm <id>... | all",
				LongDescription: "'all' moves every draft in the Drafts folder to the trash, " +
					"unreadable ones included, and lists each one it moves. The trash keeps them " +
					"until the server expires it, so nothing is lost at once; run with --dry-run " +
					"to see the list first.",
				Args: clihelp.MinimumNArgs(1),
				Run:  func(ctx *clihelp.Context) error { return runServerDraftsRemove(session, ctx.Args) },
			},
			{
				Name:        "prune",
				Description: "Trash superseded and already-sent drafts.",
				UsageLine:   "mail_cli msg drafts server prune [--sent] [--by-subject]",
				Args:        clihelp.MinimumNArgs(0),
				Options: []clihelp.Option{
					clihelp.Bool(&flagDraftsPruneSent, "--sent", false, "Also trash drafts whose message is in the Sent folder."),
					clihelp.Bool(&flagDraftsPruneBySubject, "--by-subject", false, "Match by recipients and subject when a draft has no Message-ID."),
				},
				Run: func(*clihelp.Context) error {
					return runServerDraftsPrune(session, organize.PruneOptions{Sent: flagDraftsPruneSent, BySubject: flagDraftsPruneBySubject})
				},
			},
		},
		Examples: []clihelp.Example{
			{Line: "mail_cli msg drafts server ls"},
			{Line: "mail_cli msg drafts server rm 141C4C3B 6C5A8047"},
			{Line: "mail_cli --dry-run msg drafts server rm all", Description: "Show every draft rm all would trash"},
			{Line: "mail_cli --dry-run msg drafts server prune --sent", Description: "Show what a prune would trash"},
		},
	}
}

func runServerDraftsList(session *Session) error {
	client, err := session.clients().ForSelected(session.Config)
	if err != nil {
		return err
	}
	folder, drafts, err := session.organize().ListServerDrafts(client)
	if err != nil {
		return err
	}
	if len(drafts) == 0 {
		fmt.Fprintf(session.out(), "No drafts in %s.\n", folder)
		return nil
	}
	fmt.Fprintf(session.out(), "%s (%d)\n", folder, len(drafts))
	fmt.Fprintf(session.out(), "%-9s %-16s %-30s %s\n", "ID", "DATE", "TO", "SUBJECT")
	for _, d := range drafts {
		date := ""
		if !d.Date.IsZero() {
			date = d.Date.Format("2006-01-02 15:04")
		}
		fmt.Fprintf(session.out(), "%-9s %-16s %-30s %s\n", d.ShortID, date, truncateField(d.To, 30), subjectOrNone(d.Subject))
	}
	return nil
}

func subjectOrNone(s string) string {
	if strings.TrimSpace(s) == "" {
		return "(no subject)"
	}
	return s
}

func runServerDraftsRemove(session *Session, names []string) error {
	client, err := session.clients().ForSelected(session.Config)
	if err != nil {
		return err
	}
	if len(names) == 1 && strings.EqualFold(names[0], "all") {
		return runServerDraftsRemoveAll(session, client)
	}
	for _, n := range names {
		if strings.EqualFold(n, "all") {
			return fmt.Errorf("%w: 'all' cannot be combined with draft IDs", app.ErrUsage)
		}
	}
	folder, matched, unknown, err := session.organize().FindServerDrafts(client, names)
	if err != nil {
		return err
	}
	if len(unknown) > 0 {
		return fmt.Errorf("%w: no single draft in %s matches %s; use the full ID for an ambiguous short one",
			app.ErrUsage, folder, strings.Join(unknown, ", "))
	}
	return trashServerDrafts(session, client, folder, matched)
}

// runServerDraftsRemoveAll trashes the whole Drafts folder, listing each
// draft first so the output says exactly what moved.
func runServerDraftsRemoveAll(session *Session, client mailclient.MailClient) error {
	folder, drafts, err := session.organize().ListServerDrafts(client)
	if err != nil {
		return err
	}
	if len(drafts) == 0 {
		fmt.Fprintf(session.out(), "No drafts in %s.\n", folder)
		return nil
	}
	verb := "Trashing"
	if simulating(session) {
		verb = "Would trash"
	}
	fmt.Fprintf(session.out(), "%s all %d drafts from %s:\n", verb, len(drafts), folder)
	for _, d := range drafts {
		fmt.Fprintf(session.out(), "  %-9s %-30s %s\n", d.ShortID, truncateField(d.To, 30), subjectOrNone(d.Subject))
	}
	return trashServerDrafts(session, client, folder, drafts)
}

func runServerDraftsPrune(session *Session, opts organize.PruneOptions) error {
	client, err := session.clients().ForSelected(session.Config)
	if err != nil {
		return err
	}
	folder, total, plan, err := session.organize().PlanServerPrune(client, opts)
	if err != nil {
		return err
	}
	if len(plan.IDs()) == 0 {
		fmt.Fprintf(session.out(), "Nothing to prune in %s (%d drafts).\n", folder, total)
		return nil
	}
	verb := "Trashing"
	if simulating(session) {
		verb = "Would trash"
	}
	fmt.Fprintf(session.out(), "%s from %s:\n", verb, folder)
	for _, d := range plan.Duplicates {
		fmt.Fprintf(session.out(), "  %-9s older copy    %-30s %s\n", d.ShortID, truncateField(d.To, 30), subjectOrNone(d.Subject))
	}
	for _, d := range plan.Sent {
		fmt.Fprintf(session.out(), "  %-9s already sent  %-30s %s\n", d.ShortID, truncateField(d.To, 30), subjectOrNone(d.Subject))
	}
	return trashServerDrafts(session, client, folder, append(plan.Duplicates, plan.Sent...))
}

// simulating reports whether the client will only pretend to move: under
// --read-only, which --dry-run implies, the read-only decorator returns nil
// from every mutator, so the outcome cannot be read from the error.
func simulating(session *Session) bool {
	return session.Config != nil && (session.Config.DryRun || session.Config.ReadOnly)
}

func trashServerDrafts(session *Session, client mailclient.MailClient, folder label.Folder, drafts []organize.ServerDraft) error {
	ids := make([]email.ID, 0, len(drafts))
	seen := map[string]bool{}
	for _, d := range drafts {
		if !seen[string(d.ID)] {
			seen[string(d.ID)] = true
			ids = append(ids, d.ID)
		}
	}
	trash, err := session.organize().TrashServerDrafts(client, folder, ids)
	if err != nil {
		return err
	}
	switch {
	case session.Config != nil && session.Config.DryRun:
		fmt.Fprintf(session.out(), "[dry-run] would move %d draft(s) from %s to %s\n", len(ids), folder, trash)
	case simulating(session):
		fmt.Fprintf(session.out(), "[read-only] would move %d draft(s) from %s to %s\n", len(ids), folder, trash)
	default:
		fmt.Fprintf(session.out(), "Moved %d draft(s) from %s to %s\n", len(ids), folder, trash)
	}
	return nil
}
