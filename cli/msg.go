package cli

import (
	"fmt"
	"github.com/sarielhp/mail_cli/label"
	"strings"

	"github.com/sarielhp/mail_cli/app"
	"github.com/sarielhp/mail_cli/cache"
	"github.com/sarielhp/mail_cli/cache/msg"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/mailclient"
	"github.com/sarielhp/mail_cli/spam"

	"github.com/sarielhp/clihelp"
)

// Commands that act on named messages.
//
// `archive` used to do two different things depending on whether its first
// argument was the word "all": archive one message by ID, or archive a whole
// folder. Those are different operations on different nouns, and naming them
// separately is the point of the split -- `msg archive <id>` and
// `label archive %folder`.

// MsgCmd groups the operations on individual messages.
func MsgCmd(session *Session) clihelp.Command {
	compose := rewriteCommandHelp(ComposeCmd(session), strings.NewReplacer("mail_cli write", "mail_cli msg compose"))
	compose.Name = "compose"
	compose.Hidden = true
	return clihelp.Command{
		Name:            "msg",
		Group:           "Messages",
		Description:     "Read, send, and manage messages.",
		LongDescription: "Use msg show to read a message or folder, msg send for scripts and pipes, and msg reply to continue a thread. msg drafts manages saved work, msg last lists recent mail, and msg export saves messages or threads. Write new mail with write. Archive and block operate on message IDs; label archive acts on a whole folder.",
		UsageLine:       "mail_cli msg <subcommand> [args...]",
		Subcommands: []clihelp.Command{
			ShowCmd(session),
			SendCmd(session),
			ReplyCmd(session),
			DraftsCmd(session),
			LastCmd(session),
			ExportCmd(session),
			PipeCmd(session),
			OutboxCmd(session),
			msgArchiveCmd(session),
			msgBlockCmd(session),
			compose,
		},
		Examples: []clihelp.Example{
			{Line: "mail_cli msg show abc123de", Description: "Read a message"},
			{Line: "mail_cli msg last 20", Description: "List recent messages across folders"},
			{Line: "mail_cli msg archive abc123de", Description: "Archive one message by ID"},
			{Line: "mail_cli msg block abc123de", Description: "Block sender of message to Spam"},
		},
	}
}

// msgArchiveCmd archives messages named by ID.
func msgArchiveCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:        "archive",
		Title:       "msg archive <message-id...>",
		Description: "Archive one or more messages by ID.",
		UsageLine:   "mail_cli msg archive <message-id...>",
		Parameters: []clihelp.Param{
			{Name: "<message-id...>", Description: "One or more message IDs (short 8-character or full).", Variadic: true},
		},
		Args: clihelp.MinimumNArgs(1),
		Run: func(ctx *clihelp.Context) error {
			return archiveByIDs(session, ctx.Args)
		},
		Examples: []clihelp.Example{
			{Line: "mail_cli msg archive abc123de", Description: "Archive one message"},
			{Line: "mail_cli msg archive abc123de 9f8e7d6c", Description: "Archive several at once"},
		},
	}
}

// labelArchiveCmd archives everything in a folder.
func labelArchiveCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:        "archive",
		Title:       "label archive [%label]",
		Description: "Archive every message in a folder.",
		UsageLine:   "mail_cli label archive [%label]",
		Parameters: []clihelp.Param{
			{Name: "[%label]", Description: "The folder to empty into the archive. Defaults to the inbox.", Complete: CompleteLabelRefs},
		},
		Args: clihelp.MaximumNArgs(1),
		Run: func(ctx *clihelp.Context) error {
			ref := ""
			if len(ctx.Args) > 0 {
				ref = ctx.Args[0]
			}
			return archiveFolder(session, ref)
		},
		Examples: []clihelp.Example{
			{Line: "mail_cli label archive", Description: "Archive everything in the inbox"},
			{Line: "mail_cli label archive %receipts", Description: "Archive everything in one folder"},
		},
	}
}

// archiveByIDs archives messages named by ID.
func archiveByIDs(session *Session, ids []string) error {
	client, err := GetValidatedClient(session)
	if err != nil {
		return err
	}
	target, err := session.organize().ResolveArchiveTarget(client)
	if err != nil {
		return err
	}

	for _, id := range ids {
		if err := session.organize().ByID(session.Config, client, target, id); err != nil {
			return err
		}
	}
	return nil
}

// archiveFolder archives every message in a folder, the inbox by default.
func archiveFolder(session *Session, ref string) error {
	client, err := GetValidatedClient(session)
	if err != nil {
		return err
	}

	// The source is a prefix: organize.All archives the folder and everything
	// under it, so it stays a name to match until All resolves it.
	source := client.InboxFolder().String()
	if ref != "" {
		c, prefix, resolveErr := session.ResolveClientAndLabel(ref)
		if resolveErr != nil {
			return resolveErr
		}
		if validateErr := c.Validate(); validateErr != nil {
			return validateErr
		}
		client, source = c, prefix
	}

	target, err := session.organize().ResolveArchiveTarget(client)
	if err != nil {
		return err
	}
	return session.organize().All(session.Config, client, source, target)
}

// msgBlockCmd blocks the sender of messages named by ID.
func msgBlockCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:        "block",
		Title:       "msg block [flags] <message-id...>",
		Description: "Block the senders of messages and move them.",
		UsageLine:   "mail_cli msg block [flags] <message-id...>",
		Parameters: []clihelp.Param{
			{Name: "<message-id...>", Description: "One or more message IDs (short 8-character or full).", Variadic: true},
		},
		Options: []clihelp.Option{
			clihelp.Bool(&flagBlockTrash, "--trash", false, "Route to Trash instead of Spam (no spam filter training)"),
			clihelp.Bool(&flagBlockPush, "--push", false, "Immediately push updated rules to the server"),
		},
		Args: clihelp.MinimumNArgs(1),
		Run: func(ctx *clihelp.Context) error {
			return blockByIDs(session, ctx.Args, flagBlockTrash, flagBlockPush)
		},
		Examples: []clihelp.Example{
			{Line: "mail_cli msg block abc123de", Description: "Block sender of one message to Spam"},
			{Line: "mail_cli msg block --trash abc123de", Description: "Block sender of message to Trash"},
		},
	}
}

func blockByIDs(session *Session, ids []string, toTrash, push bool) error {
	client, err := GetValidatedClient(session)
	if err != nil {
		return err
	}
	resolved, err := cfg_g.Resolve(session.Config)
	if err != nil {
		return err
	}

	for _, id := range ids {
		blockErr := blockSingleMessage(session, client, resolved, id, toTrash)
		if saveErr := cfg_g.SaveConfigFile(resolved.Path, resolved.File); saveErr != nil && blockErr == nil {
			blockErr = saveErr
		}
		if blockErr != nil {
			return blockErr
		}
	}

	if push {
		if err := client.ExportRules(); err != nil {
			return fmt.Errorf("exporting rules to server failed: %w", err)
		}
		fmt.Fprintf(session.out(), "%s Pushed updated rules to server.\n", app.PrefixSuccess)
	}
	return nil
}

func blockSingleMessage(session *Session, client mailclient.MailClient, resolved cfg_g.Resolved, id string, toTrash bool) error {
	dir := client.Config().AccountCache
	msgID, folder, err := cache.FindCachedEmailByID(dir, id)
	if err != nil {
		return fmt.Errorf("message %s: %w", id, err)
	}
	cached, err := msg.CachedEmail(dir, msgID)
	if err != nil || cached == nil {
		return fmt.Errorf("message %s not readable: %w", id, err)
	}
	sender := email.ParseEmailAddress(cached.FromRaw)
	if sender == "" {
		sender = email.ParseEmailAddress(cached.FromEmail)
	}
	if sender == "" {
		return fmt.Errorf("could not parse sender from message %s", id)
	}

	if toTrash {
		ensureRuleForTrash(session.out(), resolved.Account, sender)
		RemoveFromList(session.out(), resolved.Account, "blacklist", sender)
		trashFolder := label.FromServer("Trash")
		if tf, err := session.organize().ResolveTrashTarget(client); err == nil && !tf.IsZero() {
			trashFolder = tf
		}
		// No manual folderindex.MoveBatch/msg.ClearClassification here:
		// client, resolved through the normal client factory, is always a
		// *mailclient.CheckingMailClient, whose own MoveEmail already
		// reindexes via reindexMoved -- correctly gated on its own
		// Config().ReadOnly.
		if err := client.MoveEmail([]email.ID{msgID}, label.FromServer(folder), trashFolder); err != nil {
			return fmt.Errorf("moving message %s to %s failed: %w", id, trashFolder, err)
		}
		fmt.Fprintf(session.out(), "%s Blocked %s -> moved %s to %s\n", app.PrefixSuccess, sender, id, trashFolder)
	} else {
		AddToList(session.out(), resolved.Account, "blacklist", sender)
		ensureRuleForBlacklist(session.out(), resolved.Account, sender)
		if err := spam.MarkByID(session.status(), client, session.Config, string(msgID)); err != nil {
			return fmt.Errorf("marking message %s as spam failed: %w", id, err)
		}
		fmt.Fprintf(session.out(), "%s Blocked %s -> marked %s as spam\n", app.PrefixSuccess, sender, id)
	}
	_ = session.contacts().Purge(session.Config, sender)
	return nil
}
