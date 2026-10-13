package scan

import (
	"bytes"
	"fmt"
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"github.com/sarielhp/mail_cli/cache/msg"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/label"
	"github.com/sarielhp/mail_cli/mailclient"
	"github.com/sarielhp/mail_cli/organize"
	"github.com/sarielhp/mail_cli/report"
	"github.com/sarielhp/mail_cli/ui"
	"io"
	"mime"
	"net/mail"
	"strings"
)

// Client is the mail client surface a scan needs: reading labels and messages,
// writing labels and messages, managing spam and rules, and reading the config.
type Client interface {
	mailclient.LabelReader
	mailclient.EmailFetcher
	mailclient.EmailWriter
	mailclient.SpamManager
	mailclient.RuleManager
	mailclient.ConfigProvider
	mailclient.BackendInfo
	Validate() error
}

func resolveTargetLabels(client mailclient.LabelReader, labelPrefix string, isMoveSpam bool) ([]label.Folder, error) {
	matchedLabels, err := client.GetMatchingLabels(labelPrefix)
	if err != nil {
		return nil, err
	}
	if len(matchedLabels) == 0 {
		return nil, fmt.Errorf("no labels found matching prefix %q", labelPrefix)
	}

	for _, l := range matchedLabels {
		if strings.EqualFold(l, labelPrefix) {
			matchedLabels = []string{l}
			break
		}
	}

	if isMoveSpam {
		for _, l := range matchedLabels {
			if strings.EqualFold(l, "SPAM") {
				return nil, fmt.Errorf("the -m (move) flag is not allowed when scanning the system spam folder itself (%s)", l)
			}
		}
	}
	return label.FromServerAll(matchedLabels), nil
}

func fetchAndFilterEmails(opts Options, rep report.Reporter, client Client, config *cfg_g.Config, matchedLabel label.Folder, cacheDirName string) ([]email.ID, int, error) {
	if config.ReadOnly {
		rep.Infof("[READ-ONLY / DRY RUN] Scanning label: %s...\n", matchedLabel)
	} else if config.Verbose {
		rep.Infof("Scanning label: %s...\n", matchedLabel)
	}

	downloadedIDs, err := client.FetchAndDownloadEmails(matchedLabel, cacheDirName)
	if err != nil {
		return nil, 0, fmt.Errorf("error downloading emails for label %s: %w", matchedLabel, err)
	}

	totalBeforePattern := 0
	if opts.Pattern == "" {
		return downloadedIDs, totalBeforePattern, nil
	}

	matcher, err := email.CompilePattern(opts.Pattern)
	if err != nil {
		return nil, 0, fmt.Errorf("invalid scan pattern %q: %w", opts.Pattern, err)
	}

	totalBeforePattern = len(downloadedIDs)
	var filteredIDs []email.ID
	baseDir := client.Config().AccountCache
	for _, id := range downloadedIDs {
		rawBytes, rErr := msg.Read(baseDir, id)
		if rErr != nil {
			continue
		}
		parsed := email.ParseReader(bytes.NewReader(rawBytes), id, "")
		if parsed != nil && matcher(parsed.Subject) {
			filteredIDs = append(filteredIDs, id)
		}
	}
	return filteredIDs, totalBeforePattern, nil
}

func findMatchingIDsBySender(downloadDir cachedir.Account, ids []email.ID, targetSender string) []email.ID {
	var matching []email.ID
	target := strings.ToLower(strings.TrimSpace(targetSender))
	for _, id := range ids {
		emailBytes, rErr := msg.Read(downloadDir, id)
		if rErr != nil {
			continue
		}
		localEmail, errMail := mail.ReadMessage(bytes.NewReader(emailBytes))
		if errMail == nil {
			sender := strings.ToLower(email.ParseEmailAddress(localEmail.Header.Get("From")))
			if sender == target {
				matching = append(matching, id)
			}
		}
	}
	return matching
}

func readMessageSubject(downloadDir cachedir.Account, id email.ID) string {
	data, err := msg.Read(downloadDir, id)
	if err != nil {
		return "(No Subject)"
	}
	m, errParse := mail.ReadMessage(bytes.NewReader(data))
	if errParse != nil {
		return "(No Subject)"
	}
	dec := new(mime.WordDecoder)
	s := email.DecodeHeader(dec, m.Header.Get("Subject"))
	if s == "" {
		return "(No Subject)"
	}
	return s
}

func processMoveInbox(rep report.Reporter, client Client, config *cfg_g.Config, downloadedIDs []email.ID, matchedLabel label.Folder, moveInbox string) ([]email.ID, int, error) {
	if moveInbox == "" {
		return downloadedIDs, 0, nil
	}
	matchingIDs := findMatchingIDsBySender(config.AccountCache, downloadedIDs, moveInbox)
	if len(matchingIDs) == 0 {
		if config.Verbose {
			rep.Infof("No messages from '%s' found in label %s.\n", moveInbox, matchedLabel)
		}
		return downloadedIDs, 0, nil
	}

	if config.ReadOnly {
		rep.Printf("[DRY-RUN] Would move %d message(s) to inbox\n", len(matchingIDs))
	} else {
		rep.Infof("Found %d message(s) from '%s' in label %s. Moving to Inbox...\n", len(matchingIDs), moveInbox, matchedLabel)
		if err := client.MoveToInbox(matchingIDs, matchedLabel); err != nil {
			return downloadedIDs, 0, fmt.Errorf("failed to move messages to inbox: %w", err)
		}
		rep.Successf("Successfully moved %d message(s) to inbox.\n", len(matchingIDs))
	}

	movedMap := make(map[email.ID]bool, len(matchingIDs))
	for _, id := range matchingIDs {
		movedMap[id] = true
	}
	var remainingIDs []email.ID
	for _, id := range downloadedIDs {
		if !movedMap[id] {
			remainingIDs = append(remainingIDs, id)
		}
	}
	movedCount := 0
	if !config.ReadOnly {
		movedCount = len(matchingIDs)
	}
	return remainingIDs, movedCount, nil
}

func processMoveSpam(rep report.Reporter, client Client, config *cfg_g.Config, downloadedIDs []email.ID, matchedLabel label.Folder, moveSpam string) ([]email.ID, int, error) {
	if moveSpam == "" {
		return downloadedIDs, 0, nil
	}
	matchingIDs := findMatchingIDsBySender(config.AccountCache, downloadedIDs, moveSpam)
	if len(matchingIDs) == 0 {
		if config.Verbose {
			rep.Infof("No messages from '%s' found in label %s.\n", moveSpam, matchedLabel)
		}
		return downloadedIDs, 0, nil
	}

	if len(matchingIDs) > 1 {
		rep.Warnf("Found %d messages from '%s' in label %s (not unique, skipping move).\n", len(matchingIDs), moveSpam, matchedLabel)
		return downloadedIDs, 0, nil
	}

	uniqueID := matchingIDs[0]
	subject := readMessageSubject(config.AccountCache, uniqueID)

	if config.ReadOnly {
		rep.Printf("[DRY-RUN] Would move message %s to spam\n", uniqueID)
	} else {
		rep.Infof("Found unique message from '%s' (Subject: %q, ID: %s). Moving to Spam...\n", moveSpam, subject, uniqueID)
		if err := client.ReportSpam([]email.ID{uniqueID}, matchedLabel); err != nil {
			return downloadedIDs, 0, fmt.Errorf("failed to move unique message to spam: %w", err)
		}
		rep.Successf("Successfully moved unique message to spam.\n")
	}

	var remainingIDs []email.ID
	for _, id := range downloadedIDs {
		if id != uniqueID {
			remainingIDs = append(remainingIDs, id)
		}
	}
	movedCount := 0
	if !config.ReadOnly {
		movedCount = 1
	}
	return remainingIDs, movedCount, nil
}

// applySenderMoves runs the two sender-addressed moves -- back to Inbox, then
// the lone message to Spam -- in the order the flags imply, returning the
// messages neither move claimed and how many left. It is one function because
// the scan needs the same two steps and the same "stop once nothing is left"
// short-circuit twice; processMoveSpam is a no-op on an empty list, so the
// second move can simply follow the first.
func applySenderMoves(rep report.Reporter, client Client, config *cfg_g.Config, ids []email.ID, matchedLabel label.Folder, moveInbox, moveFrom string) ([]email.ID, int, error) {
	ids, movedInbox, err := processMoveInbox(rep, client, config, ids, matchedLabel, moveInbox)
	if err != nil {
		return ids, movedInbox, err
	}
	ids, movedSpam, err := processMoveSpam(rep, client, config, ids, matchedLabel, moveFrom)
	if err != nil {
		return ids, movedInbox + movedSpam, err
	}
	return ids, movedInbox + movedSpam, nil
}

// clearExplicitInboxEvidence drops the cached classifications of an inbox the
// user named on purpose, so the scan re-judges it. It is a separate function
// because the guard is three comparisons that would otherwise sit in the
// middle of the scan's happy path.
func clearExplicitInboxEvidence(rep report.Reporter, config *cfg_g.Config, matchedLabel label.Folder, ids []email.ID) {
	isInbox := strings.EqualFold(matchedLabel.String(), "inbox") || strings.EqualFold(matchedLabel.String(), config.ReceivedFolder)
	if !isInbox {
		return
	}
	rep.Infof("    Explicit 'scan inbox' detected. Clearing cached classifications for %d email(s) to force re-labeling...\n", len(ids))
	for _, id := range ids {
		_ = msg.ForgetEvidence(config.AccountCache, id)
	}
}

func reportAndMoveSpam(rep report.Reporter, client Client, config *cfg_g.Config, spamIDs []email.ID, matchedLabel label.Folder, isMoveSpam bool) (int, error) {
	if !isMoveSpam || len(spamIDs) == 0 {
		return 0, nil
	}
	if config.ReadOnly {
		rep.Printf("[DRY-RUN] Would move %d message(s) to %s\n", len(spamIDs), config.SpamFolder)
		return 0, nil
	}
	if config.Verbose {
		rep.Infof("Reporting %d spam emails to mail server...\n", len(spamIDs))
	}
	if err := client.ReportSpam(spamIDs, matchedLabel); err != nil {
		return 0, fmt.Errorf("error reporting spam to mail server for label %s: %w", matchedLabel, err)
	}
	rep.Successf("Successfully reported and moved %d spam emails.\n", len(spamIDs))
	return len(spamIDs), nil
}

func moveBlacklistedEmails(rep report.Reporter, client Client, config *cfg_g.Config, blacklistedIDs []email.ID, matchedLabel label.Folder) (int, error) {
	if len(blacklistedIDs) == 0 || config.SpamLearn == "" {
		return 0, nil
	}
	if config.ReadOnly {
		rep.Printf("[DRY-RUN] Would move %d blacklisted message(s) from %s to %s\n", len(blacklistedIDs), matchedLabel, config.SpamLearn)
		return 0, nil
	}
	if config.Verbose {
		rep.Infof("Moving %d blacklisted email(s) from %s to %s...\n", len(blacklistedIDs), matchedLabel, config.SpamLearn)
	}
	if err := client.MoveEmail(blacklistedIDs, matchedLabel, label.FromServer(config.SpamLearn)); err != nil {
		return 0, fmt.Errorf("failed to move blacklisted emails from %s to %s: %w", matchedLabel, config.SpamLearn, err)
	}
	rep.Successf("Moved %d blacklisted message(s) from %s to %s.\n", len(blacklistedIDs), matchedLabel, config.SpamLearn)
	return len(blacklistedIDs), nil
}

func printScanFolderAdvisories(rep report.Reporter, config *cfg_g.Config, matchedLabel label.Folder, downloadedIDs, spamIDs, blacklistedIDs []email.ID, isMoveSpam bool) {
	isSpamFolder := strings.EqualFold(matchedLabel.String(), config.SpamFolder) || strings.EqualFold(matchedLabel.String(), "spam") || strings.EqualFold(matchedLabel.String(), "[gmail]/spam")
	if !isMoveSpam && len(spamIDs) > 0 && !isSpamFolder {
		rep.Infof("Run the scan with the -m flag to report and move these messages as spam.\n")
	}
	if len(blacklistedIDs) > 0 && config.SpamLearn == "" {
		rep.Warnf("Warning: %d blacklisted email(s) detected but no SpamLearn folder configured.\n", len(blacklistedIDs))
	}
	if isSpamFolder && len(downloadedIDs) > len(spamIDs) {
		rep.Infof("Run 'spam learn force' to train the classifier and mark these emails as spam.\n")
	}
}

func scanSingleLabel(out io.Writer, opts Options, rep report.Reporter, client Client, config *cfg_g.Config, matchedLabel label.Folder) (int, *FolderReport, error) {
	cacheDirName := cfg_g.SanitizeLabelForCache(matchedLabel.String())
	downloadedIDs, totalBeforePattern, err := fetchAndFilterEmails(opts, rep, client, config, matchedLabel, cacheDirName)
	if err != nil {
		return 0, nil, err
	}

	folderReport := &FolderReport{
		Account: config.GetSelectedAccountDisplayName(),
		Folder:  matchedLabel.String(),
		Total:   len(downloadedIDs),
	}
	if len(downloadedIDs) == 0 {
		if opts.Pattern != "" {
			rep.Infof("No emails in label %s match pattern %q.\n", matchedLabel, opts.Pattern)
		} else {
			rep.Infof("No emails found in label %s.\n", matchedLabel)
		}
		return 0, folderReport, nil
	}

	if opts.ExplicitInbox {
		clearExplicitInboxEvidence(rep, config, matchedLabel, downloadedIDs)
	}

	origCount := len(downloadedIDs)
	downloadedIDs, err = client.CheckAndApplyRules(downloadedIDs, matchedLabel, cacheDirName)
	if err != nil {
		return 0, folderReport, fmt.Errorf("error applying rules for label %s: %w", matchedLabel, err)
	}
	rulesMoved := origCount - len(downloadedIDs)
	folderReport.Total = origCount
	folderReport.MovedByRules = rulesMoved
	if len(downloadedIDs) == 0 {
		rep.Successf("All emails in label %s were processed and labeled by rules.\n", matchedLabel)
		if rulesMoved > 0 {
			rep.Printf("Messages moved by rules: %d\n", rulesMoved)
		}
		return 0, folderReport, nil
	}

	downloadedIDs, movedCount, err := applySenderMoves(rep, client, config, downloadedIDs, matchedLabel, opts.MoveInbox, opts.MoveFrom)
	if err != nil {
		return movedCount, folderReport, err
	}
	if len(downloadedIDs) == 0 {
		folderReport.Moved = movedCount
		return movedCount, folderReport, nil
	}

	spamIDs, blacklistedIDs, scanResults, err := classifyFolder(rep, client, config, matchedLabel, downloadedIDs)
	if err != nil {
		return movedCount, folderReport, err
	}
	folderReport.Spam = len(spamIDs)
	folderReport.Blacklisted = len(blacklistedIDs)
	folderReport.Ham = len(downloadedIDs) - len(spamIDs)
	folderReport.Messages = buildMessageReports(config, opts.JSON, downloadedIDs, spamIDs, blacklistedIDs, scanResults)

	movedSpamCount, err := reportAndMoveSpam(rep, client, config, spamIDs, matchedLabel, opts.MoveSpam)
	if err != nil {
		return movedCount, folderReport, err
	}
	movedCount += movedSpamCount

	movedBlacklistCount, err := moveBlacklistedEmails(rep, client, config, blacklistedIDs, matchedLabel)
	if err != nil {
		return movedCount, folderReport, err
	}
	movedCount += movedBlacklistCount
	folderReport.Moved = movedCount

	if !opts.JSON {
		folderTitle := fmt.Sprintf("Folder: %s:%s (scan).", config.GetSelectedAccountDisplayName(), matchedLabel)
		ui.FormatFolderSummary(out, folderTitle, "", downloadedIDs, spamIDs, scanResults, config, cacheDirName, rulesMoved, totalBeforePattern)
	}
	printScanFolderAdvisories(rep, config, matchedLabel, downloadedIDs, spamIDs, blacklistedIDs, opts.MoveSpam)
	return movedCount, folderReport, nil
}

// classifyFolder gives each message in folder a spam verdict. Mail in the
// account's Sent or Drafts folder was written by the user and gets none: any
// verdict cached for it is cleared, and nothing is reported as spam.
func classifyFolder(rep report.Reporter, client Client, config *cfg_g.Config, folder label.Folder, ids []email.ID) ([]email.ID, []email.ID, map[email.ID]*Response, error) {
	if organize.IsAuthoredFolder(client, folder) {
		for _, id := range ids {
			_ = msg.ForgetEvidence(config.AccountCache, id)
		}
		return nil, nil, map[email.ID]*Response{}, nil
	}
	spamIDs, blacklistedIDs, results, err := Emails(rep, ids, config, cfg_g.SanitizeLabelForCache(folder.String()))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("error scanning emails with Bogofilter for label %s: %w", folder, err)
	}
	return spamIDs, blacklistedIDs, results, nil
}

// Perform scans every folder the prefix names and returns the total number of
// messages moved. When opts.JSON is set it also returns one FolderReport per
// scanned folder, which the caller serializes -- the scan package does not
// write JSON itself, because a single invocation may span several accounts and
// the caller is the one that can emit them as one document.
func Perform(out io.Writer, opts Options, rep report.Reporter, client Client, config *cfg_g.Config, labelPrefix string) (int, []FolderReport, error) {
	matchedLabels, err := resolveTargetLabels(client, labelPrefix, opts.MoveSpam)
	if err != nil {
		return 0, nil, err
	}

	var reports []FolderReport
	movedCountTotal := 0
	for _, matchedLabel := range matchedLabels {
		moved, folderReport, err := scanSingleLabel(out, opts, rep, client, config, matchedLabel)
		movedCountTotal += moved
		if folderReport != nil {
			reports = append(reports, *folderReport)
		}
		if err != nil {
			return movedCountTotal, reports, err
		}
	}
	return movedCountTotal, reports, nil
}
