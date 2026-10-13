package spam

import (
	"bytes"
	"fmt"
	"github.com/sarielhp/mail_cli/app"
	"github.com/sarielhp/mail_cli/cache"
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"github.com/sarielhp/mail_cli/cache/folderindex"
	"github.com/sarielhp/mail_cli/cache/msg"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/label"
	"github.com/sarielhp/mail_cli/mailclient"
	"github.com/sarielhp/mail_cli/report"
	"github.com/sarielhp/mail_cli/train"
	"log/slog"
	"mime"
	"net/mail"
	"os/exec"
	"strings"
	"time"
)

func trainSingleHamEmail(rep report.Reporter, downloadDir cachedir.Account, id email.ID, verbose bool) (bool, bool) {
	exists, exErr := msg.Exists(downloadDir, email.ID(id))
	if !exists {
		if verbose {
			rep.Warnf("Warning: Cache file not found for ID %s: %v\n", id, exErr)
		}
		return false, true
	}

	emailBytes, err := msg.Read(downloadDir, email.ID(id))
	if err != nil {
		rep.Warnf("Error reading cached ham email ID %s: %v\n", id, err)
		return false, false
	}

	if err := TrainHam(emailBytes); err != nil {
		rep.Warnf("Bogofilter training failed for ID %s: %v\n", id, err)
		return false, false
	}
	return true, false
}

// markHam records that the user called a message ham, replacing whatever
// verdict a scan cached for it. Without this a folder trained as ham kept
// showing the spam and political labels from before, because scan reuses a
// cached verdict rather than classifying again.
func markHam(downloadDir cachedir.Account, id email.ID) {
	_ = msg.SetOverride(downloadDir, id, msg.OverrideHam)
}

func trainHamEmails(rep report.Reporter, downloadDir cachedir.Account, ids []email.ID, trainedMap map[string]bool, verbose bool) (int, int, int, bool) {
	successCount, alreadyLearnedCount, ignoredCount := 0, 0, 0
	dbUpdated := false

	for idx, id := range ids {
		if trainedMap[string(id)] {
			alreadyLearnedCount++
			markHam(downloadDir, id)
			continue
		}
		if idx > 0 {
			time.Sleep(100 * time.Millisecond)
		}
		trained, ignored := trainSingleHamEmail(rep, downloadDir, id, verbose)
		if ignored {
			// Not recorded: a message that was never read was never
			// trained. Recording it made every later run skip it as
			// "already trained".
			ignoredCount++
			continue
		}
		if trained {
			successCount++
			trainedMap[string(id)] = true
			dbUpdated = true
			markHam(downloadDir, id)
		}
	}
	return successCount, alreadyLearnedCount, ignoredCount, dbUpdated
}

func printHamLearningSummary(rep report.Reporter, successCount, alreadyCount, ignoredCount int) {
	rep.Successf("%s\n", app.TrainingSummary("ham message(s)", successCount, alreadyCount, ignoredCount))
}

// LearnHam trains bogofilter to treat the messages in folder as ham, recording
// the IDs it trained in the account's ham ledger. It errors when bogofilter is
// not on PATH, and an empty folder succeeds having trained nothing.
func LearnHam(rep report.Reporter, client interface {
	mailclient.EmailFetcher
	mailclient.ConfigProvider
}, config *cfg_g.Config, folder label.Folder) error {
	folderName := folder.String()
	// The account's own cache: config.DownloadDir is the root above every
	// account, where no message is stored, so reading from it found nothing,
	// trained nothing and recorded every message as trained anyway.
	downloadDir := client.Config().AccountCache
	if _, err := exec.LookPath("bogofilter"); err != nil {
		return fmt.Errorf("bogofilter executable not found in PATH. Please install bogofilter on your system.\n" +
			"On Debian/Ubuntu/Kali Linux, execute:\n  sudo apt-get install bogofilter")
	}

	rep.Infof("Running in Ham Learning Mode, targeting folder '%s'...\n", folderName)

	cacheSubdir := cfg_g.SanitizeLabelForCache(folderName)
	ids, err := client.FetchAndDownloadEmails(folder, cacheSubdir)
	if err != nil {
		return err
	}

	if len(ids) == 0 {
		rep.Infof("Folder %q is empty or no emails found. Nothing to learn.\n", folderName)
		return nil
	}

	ledger, err := openHamLedger(downloadDir)
	if err != nil {
		return err
	}
	trainedMap := ledgerAsMap(rep, ledger, config.ForceLearn, config.Verbose)

	rep.Infof("Training Bogofilter on %d ham emails...\n", len(ids))
	successCount, alreadyCount, ignoredCount, dbUpdated := trainHamEmails(rep, downloadDir, ids, trainedMap, config.Verbose)

	if dbUpdated {
		if err := saveHamLedger(downloadDir, trainedMap); err != nil {
			return err
		}
	}

	printHamLearningSummary(rep, successCount, alreadyCount, ignoredCount)
	return nil
}

func trainSpamUntilMarked(rep report.Reporter, emailBytes []byte, msgID email.ID, config *cfg_g.Config) (float32, error) {
	var score float32
	for i := 0; i < 15; i++ {
		curScore, isSpam, _ := CheckSpamScore(emailBytes)
		score = curScore
		if !config.Quiet {
			rep.Infof("    Bogofilter score: %f\n", score)
		}

		if isSpam {
			if !config.Quiet {
				rep.Successf("    Message %s is now classified as Spam locally (score: %f).\n", msgID, score)
			}
			return score, nil
		}

		if !config.Quiet {
			rep.Infof("    Training message %s as Spam (attempt %d/15, current score: %f)...\n", msgID, i+1, score)
		}
		if err := TrainSpam(emailBytes); err != nil {
			return score, fmt.Errorf("bogofilter training failed: %w", err)
		}
	}

	curScore, isSpam, _ := CheckSpamScore(emailBytes)
	score = curScore
	if isSpam {
		return score, nil
	}

	return score, fmt.Errorf("message %s is still not classified as Spam locally after training (score: %f)", msgID, score)
}

func extractSubjectFromEmail(emailBytes []byte) string {
	if m, errParse := mail.ReadMessage(bytes.NewReader(emailBytes)); errParse == nil {
		dec := new(mime.WordDecoder)
		if s := email.DecodeHeader(dec, m.Header.Get("Subject")); s != "" {
			return s
		}
	}
	return "(No Subject)"
}

// openHamLedger reads the ham training record. A damaged ledger stops the run:
// carrying on would retrain every message in the folder and skew the corpus.
func openHamLedger(downloadDir cachedir.Account) (*train.Ledger, error) {
	ledger, err := train.Open(train.HamPath(downloadDir))
	if err != nil {
		return nil, fmt.Errorf("%w; delete the file to start the ham record over", err)
	}
	return ledger, nil
}

// ledgerAsMap adapts the ledger to the map trainHamEmails works with.
// --force-learn starts from nothing, retraining the whole folder.
func ledgerAsMap(rep report.Reporter, ledger *train.Ledger, force, verbose bool) map[string]bool {
	out := make(map[string]bool)
	if force {
		return out
	}
	for _, id := range ledger.IDs() {
		out[id] = true
	}
	if verbose && len(out) > 0 {
		rep.Infof("Loaded %d Message IDs from local trained ham database\n", len(out))
	}
	return out
}

func saveHamLedger(downloadDir cachedir.Account, trained map[string]bool) error {
	ledger, err := train.Open(train.HamPath(downloadDir))
	if err != nil {
		return err
	}
	var ids []string
	for id, done := range trained {
		if done {
			ids = append(ids, id)
		}
	}
	if err := ledger.AddAll(ids); err != nil {
		return fmt.Errorf("recording trained ham: %w", err)
	}
	return nil
}

// recordTrainedMessageID notes that a message has been trained as spam, so a
// later run does not train it again.
func recordTrainedMessageID(configDir string, msgID email.ID) {
	ledger := train.OpenOrEmpty(train.MessageIDPath(configDir))
	if _, err := ledger.Add(string(msgID)); err != nil {
		slog.Warn("could not record trained message",
			slog.String("id", string(msgID)), slog.Any("error", err))
	}
}

func relocateMarkedSpam(rep report.Reporter, client interface {
	mailclient.ConfigProvider
	ReportSpam(messageIDs []email.ID, source label.Folder) error
}, config *cfg_g.Config, targetID string, msgID email.ID, folderName string) error {
	isAlreadySpam := (config.SpamFolder != "" && strings.EqualFold(folderName, config.SpamFolder)) ||
		strings.EqualFold(folderName, "spam")

	if isAlreadySpam {
		rep.Successf("Successfully learned message %s locally (already in '%s' folder).\n", targetID, folderName)
		return nil
	}

	if config.ReadOnly {
		rep.Printf("[DRY-RUN] Would mark message %s as spam and move to %s\n", targetID, config.SpamFolder)
		return nil
	}

	rep.Infof("Moving message to spam filter on server...\n")
	if err := client.ReportSpam([]email.ID{email.ID(msgID)}, label.FromServer(folderName)); err != nil {
		return fmt.Errorf("failed to move message to spam: %w", err)
	}
	_ = msg.Delete(client.Config().AccountCache, email.ID(msgID))
	_ = folderindex.Remove(client.Config().AccountCache, folderName, email.ID(msgID))
	rep.Successf("Successfully learned and moved message %s to '%s' folder.\n", targetID, config.SpamFolder)
	return nil
}

// spamClassifier is the client surface MarkByID and MoveToSpamByID need: the
// account config, validation, and the ability to report a message as spam.
type spamClassifier interface {
	mailclient.ConfigProvider
	Validate() error
	ReportSpam(messageIDs []email.ID, source label.Folder) error
}

// MarkByID trains the message named by targetID as spam locally and then moves
// it to the account's spam folder on the server.
func MarkByID(rep report.Reporter, client spamClassifier, config *cfg_g.Config, targetID string) error {
	if err := ClassifyAsSpam(rep, client, config, targetID); err != nil {
		return err
	}
	return MoveToSpamByID(rep, client, config, targetID)
}

// ClassifyAsSpam trains the message with bogofilter and records it as spam in
// the local cache, without moving it. This is what "spam the current message"
// means in the TUI: the message is learned and gains the [SPAM] badge in place,
// and a later "move all" relocates it.
func ClassifyAsSpam(rep report.Reporter, client interface {
	mailclient.ConfigProvider
	Validate() error
}, config *cfg_g.Config, targetID string) error {
	if err := client.Validate(); err != nil {
		return err
	}

	msgID, _, err := cache.FindCachedEmailByID(client.Config().AccountCache, targetID)
	if err != nil {
		return err
	}

	emailBytes, err := msg.Read(client.Config().AccountCache, msgID)
	if err != nil {
		return fmt.Errorf("failed to read cached email %s: %w", msgID, err)
	}

	rep.Infof("Training message %s (%q) as spam locally...\n", targetID, extractSubjectFromEmail(emailBytes))

	_, err = trainSpamUntilMarked(rep, emailBytes, msgID, config)
	if err != nil {
		return err
	}

	recordTrainedMessageID(config.ConfigDir, msgID)
	// The training succeeded; a failure to persist the badge is not fatal, and
	// the previous MarkByID swallowed it too.
	_ = msg.SetOverride(client.Config().AccountCache, msgID, msg.OverrideSpam)
	return nil
}

// MoveToSpamByID relocates an already-learned message to the spam folder. It
// trains nothing: the [SPAM] classification is what marks a message for this
// move, and bogofilter has already seen it.
func MoveToSpamByID(rep report.Reporter, client spamClassifier, config *cfg_g.Config, targetID string) error {
	if err := client.Validate(); err != nil {
		return err
	}
	msgID, folderName, err := cache.FindCachedEmailByID(client.Config().AccountCache, targetID)
	if err != nil {
		return err
	}
	return relocateMarkedSpam(rep, client, config, targetID, msgID, folderName)
}
