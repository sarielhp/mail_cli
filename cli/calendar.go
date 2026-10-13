package cli

import (
	"strconv"

	"github.com/sarielhp/clihelp"
)

func calendarAddSubcmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:            "add",
		Title:           "calendar add [label_prefix] <message_id>",
		Description:     "Add event from email's .ics attachment.",
		LongDescription: "Add a calendar event from an .ics attachment in a specific email.",
		UsageLine:       "mail_cli calendar add [label_prefix] <message_id>",
		Parameters: []clihelp.Param{
			{Name: "[label_prefix]", Description: "Label prefix to locate the message (defaults to inbox).", Complete: CompleteLabelRefs},
			{Name: "<message_id>", Description: "Message ID or prefix containing the .ics attachment."},
		},
		Examples: []clihelp.Example{
			{Line: "mail_cli calendar add 1234abcd"},
			{Line: "mail_cli calendar add %receipts 5678efgh"},
		},
		Args: clihelp.RangeArgs(1, 2),
		Run: func(ctx *clihelp.Context) error {
			args := ctx.Args
			if len(args) == 1 {
				client, err := session.clients().ForSelected(session.Config)
				if err != nil {
					return err
				}
				return session.calendar().Add(session.Config, client, client.InboxFolder().String(), args[0])
			}
			// The label is a prefix the service matches against, so it is
			// parsed for its account but not narrowed to one folder.
			client, labelPrefix, err := session.ResolveClientAndLabel(args[0])
			if err != nil {
				return err
			}
			return session.calendar().Add(session.Config, client, labelPrefix, args[1])
		},
	}
}

func calendarWeekSubcmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:            "week",
		Title:           "calendar week [days]",
		Description:     "Show upcoming calendar events.",
		LongDescription: "Show all calendar events in the upcoming week or specified number of days.",
		UsageLine:       "mail_cli calendar week [days]",
		Parameters: []clihelp.Param{
			{Name: "[days]", Description: "Number of days to display (defaults to 7)."},
		},
		Examples: []clihelp.Example{
			{Line: "mail_cli calendar week"},
			{Line: "mail_cli calendar week 14"},
		},
		Args: clihelp.RangeArgs(0, 1),
		Run: func(ctx *clihelp.Context) error {
			days := 7
			if len(ctx.Args) > 0 {
				if d, err := strconv.Atoi(ctx.Args[0]); err == nil && d > 0 {
					days = d
				}
			}
			return session.calendar().Week(session.Config, days)
		},
	}
}

func calendarAddAllSubcmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:            "add-all",
		Aliases:         []string{"addall"},
		Title:           "calendar add-all",
		Description:     "Scan inbox and add all .ics events.",
		LongDescription: "Scan inbox for .ics attachments and add them to calendar.",
		UsageLine:       "mail_cli calendar add-all",
		Examples: []clihelp.Example{
			{Line: "mail_cli calendar add-all"},
		},
		Args: clihelp.NoArgs,
		Run: func(_ *clihelp.Context) error {
			client, err := session.clients().ForSelected(session.Config)
			if err != nil {
				return err
			}
			return session.calendar().AddAll(session.Config, client)
		},
	}
}

// CalendarCmd returns the clihelp.Command for the calendar command.
func CalendarCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:            "calendar",
		Group:           "Calendar",
		Description:     "Manage calendar events from attachments.",
		LongDescription: "Parse iCalendar (.ics) attachments from emails and synchronize events with Google Calendar or inspect upcoming agenda schedules.",
		UsageLine:       "mail_cli calendar <subcommand> [args...]",
		Subcommands: []clihelp.Command{
			calendarAddSubcmd(session),
			calendarWeekSubcmd(session),
			calendarAddAllSubcmd(session),
			CalendarSyncCmd(session),
		},
		Examples: []clihelp.Example{
			{Line: "mail_cli calendar week"},
			{Line: "mail_cli calendar add 1234abcd"},
			{Line: "mail_cli calendar add-all"},
		},
	}
}

// CalAddCmd returns the clihelp.Command for the caladd shortcut command.
func calAddCmd(session *Session, name, path string) clihelp.Command {
	return clihelp.Command{
		Name:            name,
		Group:           "Calendar",
		Description:     "Scan inbox and sync all .ics events.",
		LongDescription: "Scan all messages in the inbox for .ics attachments, extract meeting invites, and synchronize them into the configured calendar account.",
		UsageLine:       "mail_cli " + path,
		Examples: []clihelp.Example{
			{Line: "mail_cli " + path, Description: "Add every .ics attachment in the inbox to the calendar"},
		},
		Args: clihelp.NoArgs,
		Run: func(_ *clihelp.Context) error {
			client, err := session.clients().ForSelected(session.Config)
			if err != nil {
				return err
			}
			return session.calendar().AddAll(session.Config, client)
		},
	}
}

// CalendarSyncCmd adds every .ics attachment it finds, under the noun it acts
// on. "caladd" abbreviated nothing useful and squatted a plausible root name.
func CalendarSyncCmd(session *Session) clihelp.Command {
	return calAddCmd(session, "sync", "calendar sync")
}
