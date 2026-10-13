package cli

import (
	"bytes"
	"fmt"
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"github.com/sarielhp/mail_cli/label"
	"io"
	"mime"
	"net/mail"

	"github.com/sarielhp/mail_cli/app"
	"github.com/sarielhp/mail_cli/cache/msg"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/mailclient"

	"github.com/sarielhp/clihelp"
)

var flagSplitDo bool

func resolveSplitEndpoint(session *Session, labelName string) (mailclient.MailClient, label.Folder, error) {
	client, folder, err := session.ResolveClientAndLabel(labelName)
	if err != nil {
		return nil, label.Folder{}, err
	}
	if validateErr := client.Validate(); validateErr != nil {
		return nil, label.Folder{}, validateErr
	}
	unique, err := resolveUniqueLabel(client, folder)
	if err != nil {
		display := FormatAccountFolder(session, client, folder)
		return nil, label.Folder{}, fmt.Errorf("failed to resolve label %s: %w", display, err)
	}
	return client, unique, nil
}

// filterMessagesBySubjectPattern selects the messages whose subject matches
// pattern, using the same matcher as `splice` and `scan`.
//
// It used to compile its own glob, anchored at both ends, so `--pattern invoice`
// matched only a subject equal to "invoice" while the identical flag on `splice`
// matched any subject containing it. One flag, registered once, meant two things
// depending on which command you typed.
func filterMessagesBySubjectPattern(downloadDir cachedir.Account, messageIDs []email.ID, pattern string) ([]email.ID, error) {
	matches, err := email.CompilePattern(pattern)
	if err != nil {
		return nil, err
	}
	var matchedMsgs []email.ID
	dec := new(mime.WordDecoder)
	for _, msgID := range messageIDs {
		rawBytes, rErr := msg.Read(downloadDir, msgID)
		if rErr != nil {
			continue
		}
		m, rErr := mail.ReadMessage(bytes.NewReader(rawBytes))
		if rErr != nil {
			continue
		}
		subject := email.DecodeHeader(dec, m.Header.Get("Subject"))
		if subject == "" {
			subject = "(No Subject)"
		}
		if matches(subject) {
			matchedMsgs = append(matchedMsgs, msgID)
		}
	}
	return matchedMsgs, nil
}

func displaySplitDryRun(w io.Writer, downloadDir cachedir.Account, matchedMsgs []email.ID, existingTargetIDs map[email.ID]bool, pattern string, srcDisplay string, targetDisplay string) {
	dec := new(mime.WordDecoder)
	fmt.Fprintf(w, "%s [Dry-Run] Would move/remove %d message(s) from %s to %s (pattern: %q):\n", app.PrefixInfo, len(matchedMsgs), srcDisplay, targetDisplay, pattern)
	for _, msgID := range matchedMsgs {
		rawBytes, _ := msg.Read(downloadDir, msgID)
		var subject string
		if m, err := mail.ReadMessage(bytes.NewReader(rawBytes)); err == nil {
			subject = email.DecodeHeader(dec, m.Header.Get("Subject"))
		}
		status := ""
		if existingTargetIDs[msgID] {
			status = " (already in target, will only remove from source)"
		}
		fmt.Fprintf(w, "  - %s: %s%s\n", msgID[:8], subject, status)
	}
	fmt.Fprintf(w, "\nUse --do flag to perform the actual move.\n")
}

func executeSplitMove(session *Session, srcClient mailclient.MailClient, targetClient mailclient.MailClient, matchedMsgs []email.ID, sourceUnique label.Folder, targetUnique label.Folder, downloadDir cachedir.Account, srcDisplay string, targetDisplay string) error {
	fmt.Fprintf(session.errOut(), "%s Moving/removing %d message(s) from %s to %s on server...\n", app.PrefixInfo, len(matchedMsgs), srcDisplay, targetDisplay)
	if isSameAccount(srcClient, targetClient) {
		// No manual folderindex.MoveBatch/msg.ClearClassification here:
		// srcClient, resolved through the normal client factory, is always
		// a *mailclient.CheckingMailClient, whose own MoveEmail already
		// reindexes the whole batch in one call via reindexMoved --
		// correctly gated on its own Config().ReadOnly, and reconciling a
		// partial failure to exactly the messages the server actually
		// moved, which this function's own error handling does not attempt.
		if err := srcClient.MoveEmail(matchedMsgs, sourceUnique, targetUnique); err != nil {
			return fmt.Errorf("failed to move emails: %w", err)
		}
		fmt.Fprintf(session.errOut(), "%s Successfully moved/removed %d message(s) from %s to %s.\n", app.PrefixSuccess, len(matchedMsgs), srcDisplay, targetDisplay)
		return nil
	}

	trashTarget := label.FromServer("Trash")
	if t, err := session.organize().ResolveTrashTarget(srcClient); err == nil && !t.IsZero() {
		trashTarget = t
	}
	moved := 0
	for _, msgID := range matchedMsgs {
		rawBytes, rErr := msg.Read(downloadDir, msgID)
		if rErr != nil {
			continue
		}
		if _, err := targetClient.UploadRawEmail(rawBytes, targetUnique); err != nil {
			return fmt.Errorf("failed to upload message %s to target account: %w", msgID, err)
		}
		if err := srcClient.MoveEmail([]email.ID{msgID}, sourceUnique, trashTarget); err != nil {
			return fmt.Errorf("uploaded %s to %s but failed to remove it from source %s: %w", msgID, targetDisplay, srcDisplay, err)
		}
		moved++
	}
	fmt.Fprintf(session.errOut(), "%s Successfully moved/removed %d message(s) from %s to %s.\n", app.PrefixSuccess, moved, srcDisplay, targetDisplay)
	return nil
}

func runSplit(session *Session, sourceLabel, pattern, targetLabel string, doMove bool) error {
	srcClient, sourceUnique, err := resolveSplitEndpoint(session, sourceLabel)
	if err != nil {
		return err
	}
	targetClient, targetUnique, err := resolveSplitEndpoint(session, targetLabel)
	if err != nil {
		return err
	}

	targetCacheDirName := cfg_g.SanitizeLabelForCache(targetUnique.String())
	if targetCacheDirName == "" {
		targetCacheDirName = targetUnique.String()
	}
	targetMessageIDs, err := targetClient.FetchAndDownloadEmails(targetUnique, targetCacheDirName)
	if err != nil {
		return fmt.Errorf("failed to fetch target folder emails: %w", err)
	}
	existingTargetIDs := make(map[email.ID]bool, len(targetMessageIDs))
	for _, id := range targetMessageIDs {
		existingTargetIDs[id] = true
	}

	cacheDirName := cfg_g.SanitizeLabelForCache(sourceUnique.String())
	if cacheDirName == "" {
		cacheDirName = sourceUnique.String()
	}
	messageIDs, err := srcClient.FetchAndDownloadEmails(sourceUnique, cacheDirName)
	if err != nil {
		return err
	}
	if len(messageIDs) == 0 {
		fmt.Fprintf(session.errOut(), "%s Source label %q has no messages to scan.\n", app.PrefixInfo, sourceUnique)
		return nil
	}

	downloadDir := srcClient.Config().AccountCache
	matchedMsgs, err := filterMessagesBySubjectPattern(downloadDir, messageIDs, pattern)
	if err != nil {
		return err
	}
	if len(matchedMsgs) == 0 {
		fmt.Fprintf(session.errOut(), "%s No messages in %q matched pattern %q.\n", app.PrefixInfo, sourceUnique, pattern)
		return nil
	}

	srcDisplay := FormatAccountLabel(session, srcClient, sourceUnique.String())
	targetDisplay := FormatAccountLabel(session, targetClient, targetUnique.String())
	fmt.Fprintf(session.errOut(), "%s Found %d messages matching pattern %q in %q:\n", app.PrefixInfo, len(matchedMsgs), pattern, sourceUnique)

	if !doMove {
		displaySplitDryRun(session.out(), downloadDir, matchedMsgs, existingTargetIDs, pattern, srcDisplay, targetDisplay)
		return nil
	}
	return executeSplitMove(session, srcClient, targetClient, matchedMsgs, sourceUnique, targetUnique, downloadDir, srcDisplay, targetDisplay)
}

// SplitCmd returns the clihelp.Command for the split command.
func splitCmd(session *Session, name, path string) clihelp.Command {
	return clihelp.Command{
		Name:        name,
		Group:       "Folders",
		Description: "Move emails matching pattern to target label.",
		UsageLine:   "mail_cli " + path + " <source_label> <pattern> <target_label> [--do]",
		Parameters: []clihelp.Param{
			{Name: "<source_label>", Description: "Source folder/label containing messages to split.", Complete: CompleteLabelRefs},
			{Name: "<pattern>", Description: "Subject pattern with wildcards (*, ?) to match."},
			{Name: "<target_label>", Description: "Destination folder/label to move matched messages to.", Complete: CompleteLabelRefs},
		},
		Options: []clihelp.Option{
			clihelp.Bool(&flagSplitDo, "--do", false, "Perform the actual move operations"),
		},
		Examples: []clihelp.Example{
			{Line: "mail_cli " + path + ` Inbox "*invoice*" Receipts`},
			{Line: "mail_cli " + path + ` Inbox "*invoice*" Receipts --do`},
		},
		Args: clihelp.ExactArgs(3),
		Run: func(ctx *clihelp.Context) error {
			return runSplit(session, ctx.Args[0], ctx.Args[1], ctx.Args[2], flagSplitDo)
		},
	}
}

// resolveUniqueLabel names one folder or fails, via the single resolver in
// mailclient. It is kept as a name because three call sites read better for it.
func resolveUniqueLabel(client mailclient.MailClient, name string) (label.Folder, error) {
	return mailclient.ResolveOne(client, name)
}

// LabelMoveCmd moves what matches a pattern from one folder to one other:
// point to point, where label sort fans out.
//
// The argument shape is unchanged. Unifying the two pattern matchers is finding
// H5 and roadmap step 20, which the review asks be staged over two releases
// because it changes what a pattern means.
func LabelMoveCmd(session *Session) clihelp.Command {
	return splitCmd(session, "move", "label move")
}
