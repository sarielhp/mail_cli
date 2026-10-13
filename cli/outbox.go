package cli

import (
	"fmt"
	"strings"

	"github.com/sarielhp/mail_cli/outbox"

	"github.com/sarielhp/clihelp"
)

// OutboxCmd creates the outbox parent command.
func OutboxCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:            "outbox",
		Group:           "Messages",
		Description:     "Manage offline outbound email queue.",
		LongDescription: "Inspect, transmit, or delete messages spooled to the local outbox when offline or queued via msg send --queue.",
		UsageLine:       "mail_cli msg outbox <ls | flush | rm <id>...>",
		Subcommands: []clihelp.Command{
			outboxListCmd(session),
			outboxFlushCmd(session),
			outboxRemoveCmd(session),
		},
		Examples: []clihelp.Example{
			{Line: "mail_cli msg outbox ls", Description: "List all spooled messages"},
			{Line: "mail_cli msg outbox flush", Description: "Transmit all queued messages"},
			{Line: "mail_cli msg outbox rm 20261011-134500-0001", Description: "Delete a queued message"},
		},
	}
}

func outboxListCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:        "ls",
		Aliases:     []string{"list"},
		Description: "List queued messages in outbox.",
		UsageLine:   "mail_cli msg outbox ls",
		Run: func(*clihelp.Context) error {
			return runOutboxList(session)
		},
	}
}

func outboxFlushCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:        "flush",
		Description: "Transmit all queued messages in outbox.",
		UsageLine:   "mail_cli msg outbox flush",
		Run: func(*clihelp.Context) error {
			return runOutboxFlush(session)
		},
	}
}

func outboxRemoveCmd(session *Session) clihelp.Command {
	return newRemoveByIDCmd("Delete queued messages from outbox.", "mail_cli msg outbox rm <id>...", func(ids []string) error {
		return runOutboxRemove(session, ids)
	})
}

func runOutboxList(session *Session) error {
	entries, err := session.outbox().List(sessionCfg(session))
	if err != nil {
		return err
	}
	if wantJSON() {
		return writeJSONList(session, entries)
	}
	if len(entries) == 0 {
		fmt.Fprintln(session.out(), "Outbox is empty.")
		return nil
	}
	for _, e := range entries {
		toStr := strings.Join(e.To, ", ")
		fmt.Fprintf(session.out(), "%-24s  %-12s  %s  %-20s  %s\n",
			e.ID, e.Account, e.CreatedAt.Format("2006-01-02 15:04"), truncateField(toStr, 20), e.Subject)
	}
	return nil
}

func runOutboxFlush(session *Session) error {
	cfg := sessionCfg(session)
	if cfg != nil && cfg.DryRun {
		return previewOutboxFlush(session)
	}
	results, err := session.outbox().Flush(cfg)
	if err != nil {
		return err
	}
	if wantJSON() {
		if err := writeJSONList(session, results); err != nil {
			return err
		}
		return flushFailure(results)
	}
	if len(results) == 0 {
		fmt.Fprintln(session.out(), "Outbox is empty.")
		return nil
	}
	successes := 0
	for _, r := range results {
		switch {
		case r.Success && r.Err != "":
			successes++
			fmt.Fprintf(session.out(), "✔ Sent [%s] %s (%s), but: %s\n", r.Account, r.Subject, r.ID, r.Err)
		case r.Success:
			successes++
			fmt.Fprintf(session.out(), "✔ Sent [%s] %s (%s)\n", r.Account, r.Subject, r.ID)
		default:
			fmt.Fprintf(session.out(), "✗ Failed [%s] %s (%s): %s\n", r.Account, r.Subject, r.ID, r.Err)
		}
	}
	fmt.Fprintf(session.out(), "Flushed %d message(s): %d sent, %d failed.\n", len(results), successes, len(results)-successes)
	return flushFailure(results)
}

// flushFailure is the error a flush exits with: any unsent message, or a sent
// one still in the outbox (which the next flush would send again), fails the
// command, so a script or cron job sees it.
func flushFailure(results []outbox.FlushResult) error {
	bad := 0
	for _, r := range results {
		if !r.Success || r.Err != "" {
			bad++
		}
	}
	if bad == 0 {
		return nil
	}
	return fmt.Errorf("%d of %d outbox message(s) did not flush cleanly", bad, len(results))
}

// previewOutboxFlush is flush under --dry-run: it names what would be sent
// and sends nothing, nor records any attempt.
func previewOutboxFlush(session *Session) error {
	entries, err := session.outbox().List(sessionCfg(session))
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		fmt.Fprintln(session.out(), "Outbox is empty.")
		return nil
	}
	for _, e := range entries {
		fmt.Fprintf(session.out(), "[DRY-RUN] Would send [%s] %s (%s)\n", e.Account, e.Subject, e.ID)
	}
	return nil
}

func runOutboxRemove(session *Session, ids []string) error {
	for _, id := range ids {
		if err := session.outbox().Remove(sessionCfg(session), id); err != nil {
			return err
		}
		fmt.Fprintf(session.out(), "Removed outbox message %s\n", id)
	}
	return nil
}
