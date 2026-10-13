package gmail

import (
	"bytes"
	"fmt"
	"github.com/sarielhp/mail_cli/app"
	"github.com/sarielhp/mail_cli/backend/spamlearn"
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"github.com/sarielhp/mail_cli/cache/msg"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/report"
	"github.com/sarielhp/mail_cli/spam"
	"github.com/sarielhp/mail_cli/train"
	"log"
	"net/mail"
	"os"
	"path/filepath"
	"time"

	gmailapi "google.golang.org/api/gmail/v1"
)

func fetchSpamIDsToTrain(rep report.Reporter, srv *gmailapi.Service, limit int, downloadDir cachedir.Account) ([]email.ID, []string, error) {
	rep.Infof("Searching for recent spam emails to train...\n")
	res, err := srv.Users.Messages.List("me").Q("label:SPAM").IncludeSpamTrash(true).MaxResults(int64(limit)).Do()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list spam: %w", err)
	}

	var allIDs []email.ID
	var missingIDs []string
	for _, m := range res.Messages {
		allIDs = append(allIDs, email.ID(m.Id))
		exists, err := msg.Exists(downloadDir, email.ID(m.Id))
		if err != nil || !exists {
			missingIDs = append(missingIDs, m.Id)
		}
	}
	return allIDs, missingIDs, nil
}

func resolveTrainedSpamDBPath(configDir string) string {
	if configDir == "" {
		homeDir, _ := os.UserHomeDir()
		configDir = filepath.Join(homeDir, ".config", app.AppName)
	}
	return train.MessageIDPath(configDir)
}

// loadTrainedSpamMap adapts the training ledger to the map this file works
// with. A damaged ledger stops the run: carrying on would retrain every
// message in the folder and skew the corpus.
func loadTrainedSpamMap(rep report.Reporter, dbPath string, force, verbose bool) (map[string]bool, error) {
	out := make(map[string]bool)
	if force {
		return out, nil
	}
	ledger, err := train.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("%w; delete the file to start the trained-spam record over", err)
	}
	for _, id := range ledger.IDs() {
		out[id] = true
	}
	if verbose && len(out) > 0 {
		rep.Infof("Loaded %d Message IDs from local trained database\n", len(out))
	}
	return out, nil
}

func isSpamSenderBlacklisted(rep report.Reporter, emailBytes []byte, blacklist []string, verbose bool, id email.ID) bool {
	localEmail, errMail := mail.ReadMessage(bytes.NewReader(emailBytes))
	if errMail != nil {
		return false
	}
	sender := email.ParseEmailAddress(localEmail.Header.Get("From"))
	if cfg_g.IsBlacklisted(sender, blacklist) {
		if verbose {
			rep.Infof("    Skipping Bogofilter training for Message ID %s: sender '%s' is blacklisted.\n", id, sender)
		}
		return true
	}
	return false
}

func trainSingleSpamEmail(rep report.Reporter, downloadDir cachedir.Account, id email.ID, blacklist []string, verbose bool) (bool, bool) {
	exists, err := msg.Exists(downloadDir, id)
	if err != nil || !exists {
		return false, true
	}

	emailBytes, err := msg.Read(downloadDir, id)
	if err != nil {
		log.Printf("%s Error reading cached spam email ID %s: %v", app.PrefixWarn, id, err)
		return false, false
	}

	if isSpamSenderBlacklisted(rep, emailBytes, blacklist, verbose, id) {
		return false, true
	}

	if err := spam.TrainSpam(emailBytes); err != nil {
		log.Printf("%s Bogofilter training failed for ID %s: %v", app.PrefixWarn, id, err)
		return false, false
	}

	if info, _ := msg.GetInfo(downloadDir, id); info != nil {
		_ = msg.ForgetEvidence(downloadDir, id)
	}
	return true, false
}

func trainSpamEmails(rep report.Reporter, downloadDir cachedir.Account, allIDs []string, trainedMap map[string]bool, blacklist []string, verbose bool) (int, int, int, bool) {
	successCount, alreadyLearnedCount, ignoredCount := 0, 0, 0
	dbUpdated := false

	for idx, id := range allIDs {
		if trainedMap[id] {
			alreadyLearnedCount++
			continue
		}

		if idx > 0 {
			time.Sleep(100 * time.Millisecond)
		}

		trained, ignored := trainSingleSpamEmail(rep, downloadDir, email.ID(id), blacklist, verbose)
		if ignored {
			ignoredCount++
			trainedMap[id] = true
			dbUpdated = true
			continue
		}
		if trained {
			successCount++
			trainedMap[id] = true
			dbUpdated = true
		}
	}
	return successCount, alreadyLearnedCount, ignoredCount, dbUpdated
}

func printSpamLearningSummary(rep report.Reporter, successCount, alreadyLearnedCount, ignoredCount int) {
	rep.Successf("%s\n", app.TrainingSummary("spam message(s)", successCount, alreadyLearnedCount, ignoredCount))
}

// learnSpamFromGmailREST searches recent spam and trains Bogofilter.
func learnSpamFromGmailREST(rep report.Reporter, config *Config) error {
	srv, err := GetGmailService(rep, config)
	if err != nil {
		return err
	}

	allIDs, missingIDs, err := fetchSpamIDsToTrain(rep, srv, config.Limit, config.AccountCache)
	if err != nil {
		return err
	}
	if len(allIDs) == 0 {
		rep.Infof("No emails found in the spam folder. Nothing to learn.\n")
		return nil
	}

	if downloadErr := downloadMissingSpamEmails(rep, srv, config, missingIDs, "training"); downloadErr != nil {
		return downloadErr
	}

	dbPath := resolveTrainedSpamDBPath(config.ConfigDir)
	trainedMap, err := loadTrainedSpamMap(rep, dbPath, config.ForceLearn, config.Verbose)
	if err != nil {
		return err
	}

	rep.Infof("Training Bogofilter on %d spam emails...\n", len(allIDs))
	successCount, alreadyLearnedCount, ignoredCount, dbUpdated := trainSpamEmails(rep, config.AccountCache, email.IDStrings(allIDs), trainedMap, config.Blacklist, config.Verbose)

	if dbUpdated {
		spamlearn.RecordTrained(dbPath, trainedMap)
	}

	printSpamLearningSummary(rep, successCount, alreadyLearnedCount, ignoredCount)
	return nil
}
