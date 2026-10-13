package cli

import (
	"fmt"
	"github.com/sarielhp/mail_cli/mailclient"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/sarielhp/mail_cli/app"
	"github.com/sarielhp/mail_cli/cache"
	"github.com/sarielhp/mail_cli/label"
	"github.com/sarielhp/mail_cli/ui"

	"github.com/sarielhp/clihelp"
)

var (
	flagLabelsListExt     bool
	flagLabelsListNoZero  bool
	flagLabelsListFull    bool
	flagLabelsListReal    bool
	flagLabelsCountQuiet  bool
	flagLabelsCountUnread bool
)

// LabelsCmd returns the clihelp.Command for the labels command.
func LabelsCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:            "label",
		Aliases:         []string{"labels"},
		Group:           "Folders",
		Description:     "Manage labels: download, upload, sort, move.",
		LongDescription: "Subcommands include download (a folder to mbox) and upload (an mbox into a folder), sort and move (file mail into the keep/ hierarchy). Manage, inspect, and organize remote mail server labels and hierarchical folder trees across accounts, including cached folder statistics and structure repair.",
		UsageLine:       "mail_cli label <subcommand> [args...]",
		Subcommands: []clihelp.Command{
			labelsListCmd(session),
			labelsPrintCmd(session),
			labelsRenameCmd(session),
			labelsFixCmd(session),
			labelsDelCmd(session),
			labelsSearchCmd(session),
			labelsCacheCmd(session),
			labelsCreateCmd(session),
			labelsCountCmd(session),
			labelArchiveCmd(session),
			LabelDownloadCmd(session),
			LabelUploadCmd(session),
			LabelSortCmd(session),
			LabelMoveCmd(session),
		},
		Examples: []clihelp.Example{
			{Line: "mail_cli label list", Description: "List folders that hold messages"},
			{Line: "mail_cli label list --ext", Description: "Include message counts and hierarchy"},
			{Line: "mail_cli label count %inbox", Description: "How many messages are in the inbox"},
			{Line: "mail_cli label count %inbox -q", Description: "Just the number, for a script"},
			{Line: "mail_cli label search receipts", Description: "Find folders whose name contains a word"},
			{Line: `mail_cli label create "%Receipts/2026"`, Description: "Create a nested folder"},
			{Line: `mail_cli label rm "%Receipts/2025"`, Description: "Delete a folder"},
			{Line: `mail_cli label rename "%OldName" "%NewName"`, Description: "Rename a folder in place"},
			{Line: "mail_cli label repair", Description: "Create the missing parents of nested folders"},
		},
	}
}

func labelsListCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:            "list",
		Aliases:         []string{"ls"},
		Title:           "label list",
		Description:     "List all folders and labels in a tree view.",
		LongDescription: "List all folders and labels in a hierarchical tree view. Pass --full to show full paths, --ext to show unread and total message counts, --nozero to omit empty folders, or --real to query the server in real-time.",
		UsageLine:       "mail_cli label list [flags]",
		Options: []clihelp.Option{
			clihelp.Bool(&flagLabelsListAll, "-a, --all", false, "List all labels, including those with zero messages"),
			clihelp.Bool(&flagLabelsListNoZero, "-z, --nozero", false, "Only list folders that have at least one message in them"),
			clihelp.Bool(&flagLabelsListFull, "-f, --full", false, "Show full label names (without hierarchical view)"),
			clihelp.Bool(&flagLabelsListExt, "-e, --ext", false, "Show (unread/total) message counts"),
			clihelp.Bool(&flagLabelsListReal, "--real", false, "Fetch real-time counts from server (bypasses cache)"),
		},
		Run: func(ctx *clihelp.Context) error {
			args := ctx.Args
			isFull := flagLabelsListFull
			for _, a := range args {
				if strings.EqualFold(a, "full") {
					isFull = true
				}
			}
			if isFull {
				isExt, isNoZero := parseExtNoZero(args, flagLabelsListExt, flagLabelsListNoZero)
				realOnly := findArg(args, "real") || flagLabelsListReal
				return runLabelsListFull(session, isExt, isNoZero, realOnly)
			}
			client, err := GetValidatedClient(session)
			if err != nil {
				return err
			}
			_, isNoZero := parseExtNoZero(args, flagLabelsListExt, flagLabelsListNoZero)
			if isNoZero && client.Config() != nil {
				client.Config().HideZeroLabels = true
			}
			items, err := client.GetLabelItems()
			if err != nil {
				return err
			}
			ui.FormatLabelListing(session.out(), client.BackendType(), items, client.Config().HideZeroLabels)
			return nil
		},
		Examples: []clihelp.Example{
			{Line: "mail_cli label list"},
			{Line: "mail_cli label list --ext"},
			{Line: "mail_cli label list --full --ext"},
			{Line: "mail_cli label list --nozero"},
		},
	}
}

func labelsPrintCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:            "print",
		Aliases:         []string{"pr"},
		Description:     "Print all labels, one per line.",
		LongDescription: "Print all labels/folders, one per line (full path only).",
		UsageLine:       "mail_cli label print",
		Args:            clihelp.NoArgs,
		Run: func(_ *clihelp.Context) error {
			client, err := GetValidatedClient(session)
			if err != nil {
				return err
			}
			items, err := client.GetLabelItems()
			if err != nil {
				return err
			}
			var paths []string
			for _, item := range items {
				paths = append(paths, item.FullName)
			}
			sort.Strings(paths)
			for _, p := range paths {
				fmt.Fprintln(session.out(), p)
			}
			return nil
		},
		Examples: []clihelp.Example{
			{Line: "mail_cli label print"},
		},
	}
}

func labelsRenameCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:            "rename",
		Title:           "label rename <old_name> <new_name>",
		Description:     "Rename a label and update rules.",
		LongDescription: "Rename a label/folder and update all existing routing rules that reference it.",
		UsageLine:       "mail_cli label rename <old_name> <new_name>",
		Parameters: []clihelp.Param{
			{Name: "<old_name>", Description: "The full path of the label to rename.", Complete: CompleteFolderNames},
			{Name: "<new_name>", Description: "The new full path for the label."},
		},
		Args: clihelp.ExactArgs(2),
		Run: func(ctx *clihelp.Context) error {
			args := ctx.Args
			newFolder, err := label.ForCreate(args[1])
			if err != nil {
				return err
			}
			client, err := GetValidatedClient(session)
			if err != nil {
				return err
			}
			oldFolder, err := exactFolder(client, args[0])
			if err != nil {
				return err
			}
			if renameErr := client.RenameLabel(oldFolder, newFolder); renameErr != nil {
				return renameErr
			}
			oldName, newName := oldFolder.String(), newFolder.String()
			fc, targetAcc, _, configPath, err := cfgResolveAccount(session)
			if err != nil {
				return err
			}
			updateRuleLabels(session.out(), targetAcc, oldName, newName)
			if err := cfgSaveConfig(configPath, fc); err != nil {
				return err
			}
			return nil
		},
		Examples: []clihelp.Example{
			{Line: `mail_cli labels rename "Archive/2025" "Archive/2026"`},
		},
	}
}

func labelsFixCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:            "repair",
		Aliases:         []string{"fix"},
		Description:     "Ensure parent folders exist for nested paths.",
		LongDescription: "Ensure that all parent folders exist for every nested folder path in the account.",
		UsageLine:       "mail_cli label repair",
		Args:            clihelp.NoArgs,
		Run: func(_ *clihelp.Context) error {
			client, err := GetValidatedClient(session)
			if err != nil {
				return err
			}
			return client.FixLabels()
		},
		Examples: []clihelp.Example{
			{Line: "mail_cli label fix"},
		},
	}
}

func labelsDelCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:        "rm",
		NoAbbrev:    true,
		Aliases:     []string{"del", "delete"},
		Title:       "label rm %<name>",
		Description: "Delete a label or folder from the server.",
		UsageLine:   "mail_cli label rm %<name>",
		Parameters: []clihelp.Param{
			{Name: "<lbl_name>", Description: "The full path of the label to delete.", Complete: CompleteLabelRefs},
		},
		Options: []clihelp.Option{
			clihelp.Bool(&flagLabelRmYes, "-y, --yes", false, "Proceed without the confirmation prompt."),
		},
		Args: clihelp.ExactArgs(1),
		Run: func(ctx *clihelp.Context) error {
			// A label reference, like the usage line and the completion both
			// say. This used to hand ctx.Args[0] to the backend untouched, so
			// the % form named a label that does not exist and the bare form
			// deleted one without ever being parsed.
			client, name, err := session.ResolveClientAndLabel(ctx.Args[0])
			if err != nil {
				return err
			}
			folder, err := exactFolder(client, name)
			if err != nil {
				return err
			}
			// Deleting a server label cannot be undone, so it asks -- the row of
			// the confirmation matrix that had never been implemented.
			if err := session.Confirm(fmt.Sprintf("delete the label %q from the server", folder), flagLabelRmYes); err != nil {
				return declined(session, err)
			}
			return client.DeleteLabel(folder)
		},
		Examples: []clihelp.Example{
			{Line: "mail_cli label del %Receipts/Old"},
		},
	}
}

func labelsSearchCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:            "search",
		Title:           "label search <substring...>",
		Description:     "Search cached labels by pattern.",
		LongDescription: "Search cached labels by one or more substring patterns (all must match).",
		UsageLine:       "mail_cli label search <substring> [additional_substrings...]",
		Parameters: []clihelp.Param{
			{Name: "<substring>", Description: "Substring to search for in folder paths."},
			{Name: "[additional_substrings...]", Description: "Extra substrings; all must match.", Variadic: true},
		},
		Args: clihelp.MinimumNArgs(1),
		Run: func(ctx *clihelp.Context) error {
			args := ctx.Args
			cacheDir, err := ResolveCacheDir(session.Config)
			if err != nil {
				return err
			}
			cs := &cache.DiskCacheStore{Dir: cacheDir}
			items, cacheErr := cs.GetLabelItems()
			age, ageErr := cs.CachedLabelItemsAge()

			if cacheErr != nil || ageErr != nil {
				client, err := GetValidatedClient(session)
				if err != nil {
					return err
				}
				items, err = client.GetLabelItems()
				if err != nil {
					return err
				}
			} else if age >= 24*time.Hour {
				go asyncRefreshLabelsCache(session)
			}

			matches, _ := SearchLabels(session.Config, args)
			if matches == nil {
				var lowerPatterns []string
				for _, p := range args {
					lowerPatterns = append(lowerPatterns, strings.ToLower(p))
				}
				for _, item := range items {
					lowerName := strings.ToLower(item.FullName)
					allMatched := true
					for _, lp := range lowerPatterns {
						if !strings.Contains(lowerName, lp) {
							allMatched = false
							break
						}
					}
					if allMatched {
						matches = append(matches, item.FullName)
					}
				}
				sort.Strings(matches)
			}
			for _, m := range matches {
				fmt.Fprintln(session.out(), m)
			}
			return nil
		},
		Examples: []clihelp.Example{
			{Line: "mail_cli label search receipts"},
			{Line: "mail_cli label search archive 2026"},
		},
	}
}

func asyncRefreshLabelsCache(session *Session) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("panic in background labels refresh", slog.Any("recover", r))
		}
	}()
	client, err := GetValidatedClient(session)
	if err != nil {
		slog.Warn("async refresh: client creation failed", slog.String("error", err.Error()))
		return
	}
	if _, err := client.GetLabelItems(); err != nil {
		slog.Warn("background labels cache refresh failed", slog.String("error", err.Error()))
	}
}

func labelsCacheCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:            "cache",
		Title:           "label cache",
		Description:     "Manage the local labels cache.",
		LongDescription: "Manage the local labels cache: fetch and refresh cached label/folder data from the server.",
		UsageLine:       "mail_cli label cache <subcommand>",
		Subcommands: []clihelp.Command{
			labelsCacheUpdateCmd(session),
		},
		Examples: []clihelp.Example{
			{Line: "mail_cli label cache update"},
		},
	}
}

func labelsCacheUpdateCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:            "update",
		Aliases:         []string{"up", "sync"},
		Description:     "Refresh local cache from server.",
		LongDescription: "Fetch the latest labels and folder hierarchies from the server and refresh the local cache.",
		UsageLine:       "mail_cli label cache update",
		Args:            clihelp.NoArgs,
		Run: func(_ *clihelp.Context) error {
			client, err := GetValidatedClient(session)
			if err != nil {
				return err
			}
			_, err = client.GetLabelItems()
			if err != nil {
				return err
			}
			fmt.Fprintf(session.errOut(), "%s Labels cache updated\n", app.PrefixSuccess)
			return nil
		},
		Examples: []clihelp.Example{
			{Line: "mail_cli label cache update"},
		},
	}
}

func labelsCreateCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:        "create",
		Title:       "label create <lbl_name>",
		Description: "Create a label or folder on the server.",
		UsageLine:   "mail_cli label create <lbl_name>",
		Parameters: []clihelp.Param{
			{Name: "<lbl_name>", Description: "The name or path of the label/folder to create."},
		},
		Args: clihelp.ExactArgs(1),
		Run: func(ctx *clihelp.Context) error {
			targetLabel := ctx.Args[0]
			if strings.TrimSpace(targetLabel) == "" {
				return fmt.Errorf("label name cannot be empty")
			}
			folder, err := label.ForCreate(targetLabel)
			if err != nil {
				return err
			}
			client, err := GetValidatedClient(session)
			if err != nil {
				return err
			}

			labels, err := client.GetMatchingLabels("")
			if err != nil {
				return err
			}
			for _, l := range labels {
				if strings.EqualFold(l, targetLabel) {
					return fmt.Errorf("label %q already exists", targetLabel)
				}
			}

			if err := client.EnsureLabelExists(folder); err != nil {
				return err
			}
			fmt.Fprintf(session.errOut(), "%s Successfully created label %q\n", app.PrefixSuccess, targetLabel)
			return nil
		},
		Examples: []clihelp.Example{
			{Line: `mail_cli labels create "Work/Projects"`},
		},
	}
}

func labelsCountCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:        "count",
		Title:       "label count <label> [-q] [-u]",
		Description: "Count the messages in a label or folder.",
		UsageLine:   "mail_cli label count <label> [flags]",
		Parameters: []clihelp.Param{
			{Name: "<label>", Description: "Label or folder, optionally account:label", Complete: CompleteLabelRefs},
		},
		Options: []clihelp.Option{
			clihelp.Bool(&flagLabelsCountQuiet, "-q, --quiet", false, "Print only the number"),
			clihelp.Bool(&flagLabelsCountUnread, "-u, --unread", false, "Output unread count instead of total count"),
		},
		Args: clihelp.ExactArgs(1),
		Run: func(ctx *clihelp.Context) error {
			return runLabelsCount(session, ctx.Args[0], flagLabelsCountQuiet, flagLabelsCountUnread)
		},
		Examples: []clihelp.Example{
			{Line: "mail_cli label count inbox"},
			{Line: "mail_cli label count inbox -q"},
			{Line: "mail_cli label count FastMail:received_private -q"},
			{Line: "mail_cli label count work:Inbox -u -q"},
		},
	}
}

func runLabelsCount(session *Session, labelArg string, quiet, unread bool) error {
	client, targetLabel, err := session.ResolveClientAndLabel(labelArg)
	if err != nil {
		return err
	}
	if validateErr := client.Validate(); validateErr != nil {
		return validateErr
	}

	// Always query live from the server first, falling back to cache if offline
	items, err := getLabelItemsReal(client)
	fallbackUsed := false
	if err != nil {
		items, err = client.GetLabelItems()
		if err != nil {
			return fmt.Errorf("failed to fetch label counts: %w", err)
		}
		fallbackUsed = true
	}

	item, err := findLabelItem(items, targetLabel)
	if err != nil {
		return err
	}

	if wantJSON() {
		return session.emitJSON(map[string]any{
			"account": client.Config().SelectedAccount,
			"label":   item.FullName,
			"total":   item.MessagesTotal,
			"unread":  item.MessagesUnread,
			"cached":  fallbackUsed,
		})
	}

	if quiet {
		if unread {
			fmt.Fprintln(session.out(), item.MessagesUnread)
		} else {
			fmt.Fprintln(session.out(), item.MessagesTotal)
		}
		return nil
	}

	accName := string(client.Config().SelectedAccount)
	if accName == "" && len(session.Config.Accounts) > 0 {
		accName = session.Config.Accounts[0].Name
	}
	display := fmt.Sprintf("%s:%s", accName, item.FullName)
	// All three lines are the answer to `labels count`, so they are product
	// even though the first carries a marker. Only the fallback notice is
	// status.
	fmt.Fprintf(session.out(), "%s Folder:  %s\n", app.PrefixInfo, app.ColorBoldCyan.Sprint(display))
	fmt.Fprintf(session.out(), "   Total:   %s message(s)\n", app.ColorBoldYellow.Sprint(item.MessagesTotal))
	fmt.Fprintf(session.out(), "   Unread:  %s message(s)\n", app.ColorBoldYellow.Sprint(item.MessagesUnread))
	if fallbackUsed {
		fmt.Fprintf(session.errOut(), "%s (server query failed, showing cached count)\n", app.PrefixWarn)
	}
	return nil
}

// exactFolder names the folder called name, ignoring case, or fails. The
// destructive label commands use it rather than mailclient.ResolveOne, which
// also accepts a unique partial match: a folder is not renamed or deleted on
// a guess.
func exactFolder(client mailclient.LabelReader, name string) (label.Folder, error) {
	all, err := client.GetMatchingLabels("")
	if err != nil {
		return label.Folder{}, err
	}
	for _, l := range all {
		if strings.EqualFold(l, name) {
			return label.FromServer(l), nil
		}
	}
	return label.Folder{}, &mailclient.NotFoundError{Input: name}
}
