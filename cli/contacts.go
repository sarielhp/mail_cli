package cli

import (
	"github.com/sarielhp/mail_cli/contacts"

	"github.com/sarielhp/clihelp"
)

// ContactsCmd returns the clihelp.Command for managing contacts and aliases.
func ContactsCmd(session *Session) clihelp.Command {
	listRun := func(ctx *clihelp.Context) error {
		return runContactsList(session, ctx.Args)
	}

	return clihelp.Command{
		Name:            "contacts",
		Group:           "Contacts",
		Description:     "Manage contacts and aliases.",
		LongDescription: "View and manage email contacts and aliases from Google and Mutt.",
		UsageLine:       "mail_cli contacts [subcommand]",
		Run:             listRun,
		Subcommands: []clihelp.Command{
			contactsListSubcmd(session),
			contactsSearchSubcmd(session),
			contactsSuggestSubcmd(session),
			contactsHarvestSubcmd(session),
			contactsImportMuttSubcmd(session),
			contactsSyncSubcmd(session),
			contactsPathSubcmd(session),
		},
		Examples: []clihelp.Example{
			{Line: "mail_cli contacts", Description: "List all contacts and aliases"},
			{Line: "mail_cli contacts search algo", Description: "Search contacts and groups"},
			{Line: "mail_cli contacts suggest alice", Description: "Suggest ranked email recipients"},
			{Line: "mail_cli contacts harvest", Description: "Harvest suggested recipients from cached mail"},
			{Line: "mail_cli contacts sync", Description: "Sync contacts from Google Contacts"},
			{Line: "mail_cli contacts import-mutt", Description: "Import Mutt aliases configured in config.json"},
			{Line: "mail_cli contacts import-mutt ~/.mutt/aliases", Description: "Import Mutt aliases from a specific file"},
		},
	}
}

func contactsListSubcmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:            "list",
		Title:           "contacts list [filter]",
		Description:     "List stored contacts and aliases.",
		LongDescription: "List stored contacts and aliases with an optional substring filter.",
		UsageLine:       "mail_cli contacts list [filter]",
		Parameters: []clihelp.Param{
			{Name: "[filter]", Description: "Optional substring to filter by name, email, or alias."},
		},
		Examples: []clihelp.Example{
			{Line: "mail_cli contacts list"},
			{Line: "mail_cli contacts list john"},
		},
		Args: clihelp.RangeArgs(0, 1),
		Run: func(ctx *clihelp.Context) error {
			return runContactsList(session, ctx.Args)
		},
	}
}

// runContactsList lists stored contacts, applying an optional substring filter.
// It backs both the bare `contacts` command and `contacts list`, which are the
// same listing.
func runContactsList(session *Session, args []string) error {
	filter := ""
	if len(args) > 0 {
		filter = args[0]
	}
	if wantJSON() {
		path := contacts.DefaultPath(session.Config)
		book, err := contacts.LoadBook(path)
		if err != nil {
			return err
		}
		return session.emitJSON(contacts.Filter(book, filter))
	}
	return session.contacts().List(session.Config, filter)
}

func contactsSearchSubcmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:            "search",
		Title:           "contacts search <query>",
		Description:     "Search contacts by name, email, or note.",
		LongDescription: "Search contacts across names, emails, nicknames, groups, and notes.",
		UsageLine:       "mail_cli contacts search <query>",
		Parameters: []clihelp.Param{
			{Name: "<query>", Description: "Search query words."},
		},
		Examples: []clihelp.Example{
			{Line: "mail_cli contacts search algo"},
			{Line: "mail_cli contacts search \"dave mount\""},
		},
		Args: clihelp.ExactArgs(1),
		Run: func(ctx *clihelp.Context) error {
			if wantJSON() {
				path := contacts.DefaultPath(session.Config)
				book, err := contacts.LoadBook(path)
				if err != nil {
					return err
				}
				return session.emitJSON(contacts.Search(book, ctx.Args[0]))
			}
			return session.contacts().Search(session.Config, ctx.Args[0])
		},
	}
}

func contactsImportMuttSubcmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:            "import-mutt",
		Title:           "contacts import-mutt [file_path]",
		Description:     "Import contacts from a Mutt alias file.",
		LongDescription: "Import contacts and distribution lists from a Mutt alias file.",
		UsageLine:       "mail_cli contacts import-mutt [file_path]",
		Parameters: []clihelp.Param{
			{Name: "[file_path]", Description: "Mutt alias file (default: from config)"},
		},
		Examples: []clihelp.Example{
			{Line: "mail_cli contacts import-mutt"},
			{Line: "mail_cli contacts import-mutt ~/.mutt/aliases"},
		},
		Args: clihelp.RangeArgs(0, 1),
		Run: func(ctx *clihelp.Context) error {
			path := ""
			if len(ctx.Args) > 0 {
				path = ctx.Args[0]
			}
			return session.contacts().ImportMutt(session.Config, path)
		},
	}
}

func contactsSyncSubcmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:            "sync",
		Title:           "contacts sync [account]",
		Description:     "Sync contacts from Google People API.",
		LongDescription: "Fetch and update contacts from Google People API into local contacts.json.",
		UsageLine:       "mail_cli contacts sync [account]",
		Parameters: []clihelp.Param{
			{Name: "[account]", Description: "Name of the Google account to sync (defaults to selected account).", Complete: CompleteAccountNames},
		},
		Examples: []clihelp.Example{
			{Line: "mail_cli contacts sync"},
			{Line: "mail_cli contacts sync work"},
		},
		Args: clihelp.RangeArgs(0, 1),
		Run: func(ctx *clihelp.Context) error {
			account := ""
			if len(ctx.Args) > 0 {
				account = ctx.Args[0]
			}
			return session.contacts().SyncGoogle(session.Config, account)
		},
	}
}

func contactsPathSubcmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:        "path",
		Title:       "contacts path",
		Description: "Print the file path to local contacts store.",
		UsageLine:   "mail_cli contacts path",
		Args:        clihelp.NoArgs,
		Run: func(_ *clihelp.Context) error {
			return session.contacts().Path(session.Config)
		},
	}
}

func contactsSuggestSubcmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:            "suggest",
		Title:           "contacts suggest [query]",
		Description:     "Suggest ranked recipients from history.",
		LongDescription: "Suggest ranked email recipients combining curated contacts and interaction history.",
		UsageLine:       "mail_cli contacts suggest [query]",
		Parameters: []clihelp.Param{
			{Name: "[query]", Description: "Optional prefix or search string to match recipients."},
		},
		Examples: []clihelp.Example{
			{Line: "mail_cli contacts suggest"},
			{Line: "mail_cli contacts suggest alice"},
			{Line: "mail_cli contacts suggest algo"},
		},
		Args: clihelp.RangeArgs(0, 1),
		Run: func(ctx *clihelp.Context) error {
			query := ""
			if len(ctx.Args) > 0 {
				query = ctx.Args[0]
			}
			if wantJSON() {
				path := contacts.DefaultPath(session.Config)
				book, _ := contacts.LoadBook(path)
				historyPath := contacts.DefaultHistoryPath(session.Config)
				history, _ := contacts.LoadHistory(historyPath)
				var blacklist []string
				if session.Config != nil {
					blacklist = session.Config.Policy.Blacklist
				}
				results := contacts.Suggest(book, history, blacklist, query, 15)
				return session.emitJSON(results)
			}
			return session.contacts().Suggest(session.Config, query)
		},
	}
}

func contactsHarvestSubcmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:            "harvest",
		Title:           "contacts harvest",
		Description:     "Build suggestion index from cached mail.",
		LongDescription: "Scan cached sent and archived messages across all accounts to build the recipient suggestion index.",
		UsageLine:       "mail_cli contacts harvest",
		Args:            clihelp.NoArgs,
		Examples: []clihelp.Example{
			{Line: "mail_cli contacts harvest"},
		},
		Run: func(_ *clihelp.Context) error {
			return session.contacts().Harvest(session.Config)
		},
	}
}
