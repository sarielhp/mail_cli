package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/sarielhp/mail_cli/draft"
	"github.com/sarielhp/mail_cli/email"

	"github.com/sarielhp/clihelp"
)

// draftStore is the local drafts directory for this session's config.
func draftStore(session *Session) draft.Store {
	dir := ""
	if session != nil && session.Config != nil {
		dir = session.Config.CacheRoot
	}
	return draft.Store{Root: draft.Root(dir)}
}

// DraftsCmd lists and removes the local copies compose keeps.
func DraftsCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:        "drafts",
		Group:       "Messages",
		Description: "List or remove locally saved drafts.",
		UsageLine:   "mail_cli msg drafts <ls | rm <id>... | server ...>",
		LongDescription: "Every compose session keeps a local copy of the message under the " +
			"cache's drafts directory, rewritten on each edit and on exit, beside the copy " +
			"it uploads to the account's Drafts folder. The listing names each local copy " +
			"by an ID that 'write --resume <id>' reopens on the review screen. Sending a " +
			"message removes its local copy; leaving with q or Esc keeps it, and 'rm' " +
			"deletes it. The 'server' subcommand is the counterpart for the account's " +
			"Drafts folder: it lists, trashes and prunes the copies uploaded there.",
		Subcommands: []clihelp.Command{
			draftsListCmd(session),
			draftsRemoveCmd(session),
			draftsServerCmd(session),
		},
		Examples: []clihelp.Example{
			{Line: "mail_cli msg drafts ls", Description: "List saved drafts, newest first"},
			{Line: "mail_cli write --resume 2026/09/30/125", Description: "Reopen a draft on the review screen"},
			{Line: "mail_cli msg drafts rm 2026/09/30/125", Description: "Delete a draft"},
			{Line: "mail_cli msg drafts server prune --sent", Description: "Trash duplicate and already-sent server drafts"},
		},
	}
}

func draftsListCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:        "ls",
		Description: "List saved drafts, newest first.",
		UsageLine:   "mail_cli msg drafts ls",
		Args:        clihelp.MinimumNArgs(0),
		Run: func(*clihelp.Context) error {
			return runDraftsList(session)
		},
	}
}

func draftsRemoveCmd(session *Session) clihelp.Command {
	return newRemoveByIDCmd("Delete saved drafts by ID.", "mail_cli msg drafts rm <id>...", func(ids []string) error {
		return runDraftsRemove(session, ids)
	})
}

func runDraftsList(session *Session) error {
	store := draftStore(session)
	entries, err := store.List()
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		fmt.Fprintf(session.out(), "No saved drafts under %s.\n", store.Root)
		return nil
	}
	fmt.Fprintf(session.out(), "%-16s %-16s %-30s %s\n", "ID", "MODIFIED", "TO", "SUBJECT")
	for _, e := range entries {
		to, subject := draftSummary(e.Path)
		fmt.Fprintf(session.out(), "%-16s %-16s %-30s %s\n",
			e.ID, e.ModTime.Format("2006-01-02 15:04"), truncateField(to, 30), subject)
	}
	return nil
}

// draftSummary reads the To and Subject headers; a draft that will not parse
// is still listed, so it can be removed.
func draftSummary(path string) (string, string) {
	raw, err := readDraftFile(path)
	if err != nil {
		return "", "(unreadable)"
	}
	cm, err := email.ParseRFC822ToCompose(raw)
	if err != nil {
		return "", "(unreadable)"
	}
	subject := strings.TrimSpace(cm.Subject)
	if subject == "" {
		subject = "(no subject)"
	}
	return strings.TrimSpace(cm.To), subject
}

func readDraftFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

func truncateField(s string, width int) string {
	if len(s) <= width {
		return s
	}
	return s[:width-1] + "…"
}

func runDraftsRemove(session *Session, ids []string) error {
	store := draftStore(session)
	for _, id := range ids {
		path, err := store.Resolve(id)
		if err != nil {
			return err
		}
		if err := draft.Remove(path); err != nil {
			return err
		}
		fmt.Fprintf(session.out(), "Removed draft %s\n", id)
	}
	return nil
}
