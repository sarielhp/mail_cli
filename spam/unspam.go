package spam

import (
	"bytes"
	"fmt"
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
	"mime"
	"net/mail"
	"os"
	"strconv"
	"strings"
)

func parseSpamicity(output string) float32 {
	if idx := strings.Index(output, "spamicity="); idx != -1 {
		sub := output[idx+10:]
		if endIdx := strings.IndexAny(sub, ", \r\n"); endIdx != -1 {
			sub = sub[:endIdx]
		}
		if val, errParse := strconv.ParseFloat(strings.TrimSpace(sub), 64); errParse == nil {
			return float32(val)
		}
	}
	return 0.0
}

func trainHamUntilMarked(rep report.Reporter, emailBytes []byte, msgID email.ID, config *cfg_g.Config) (float32, error) {
	var score float32
	for i := 0; i < 15; i++ {
		curScore, isSpam, _ := CheckSpamScore(emailBytes)
		score = curScore
		if !config.Quiet {
			rep.Infof("    Bogofilter score: %f\n", score)
		}

		if !isSpam {
			if !config.Quiet {
				rep.Successf("    Message %s is now classified as Ham locally (score: %f).\n", msgID, score)
			}
			return score, nil
		}

		if !config.Quiet {
			rep.Infof("    Training message %s as Ham (attempt %d/15, current score: %f)...\n", msgID, i+1, score)
		}
		if err := TrainHam(emailBytes); err != nil {
			return score, fmt.Errorf("bogofilter training failed: %w", err)
		}
	}

	curScore, isSpam, _ := CheckSpamScore(emailBytes)
	score = curScore
	if !isSpam {
		return score, nil
	}

	return score, fmt.Errorf("message %s is still not classified as Ham locally after training (score: %f)", msgID, score)
}

// clearTrainedDBs forgets a message across all three training records, so the
// filter can be retrained on it from scratch.
//
// There used to be two removal routines here, one for the files holding a JSON
// list and one for the file holding a JSON map. A Ledger reads either shape, so
// there is one path now.
func clearTrainedDBs(rep report.Reporter, downloadDir cachedir.Account, configDir string, msgID email.ID, quiet bool) {
	paths := []string{
		train.MessageIDPath(configDir),
		train.SpamUIDPath(downloadDir),
		train.HamPath(downloadDir),
	}
	for _, path := range paths {
		removed, err := train.OpenOrEmpty(path).Remove(string(msgID))
		if err != nil {
			if !quiet {
				rep.Warnf("    could not update %s: %v\n", path, err)
			}
			continue
		}
		if removed && !quiet {
			rep.Printf("%v\n", path)
		}
	}
}

// UnspamByID trains the message named by targetID as ham, records the user's
// ham override, and moves it back to the inbox. Under ReadOnly it stops after
// the local training and reports the move it would have made.
func UnspamByID(rep report.Reporter, client interface {
	Validate() error
	mailclient.ConfigProvider
	mailclient.EmailWriter
	mailclient.LabelReader
}, config *cfg_g.Config, targetID string) error {
	if err := client.Validate(); err != nil {
		return err
	}

	msgID, folderName, err := cache.FindCachedEmailByID(client.Config().AccountCache, targetID)
	if err != nil {
		return err
	}

	emailBytes, err := msg.Read(client.Config().AccountCache, msgID)
	if err != nil {
		return fmt.Errorf("failed to read cached email %s: %w", msgID, err)
	}

	subject := ""
	if data, rErr := msg.Read(client.Config().AccountCache, msgID); rErr == nil {
		if m, errParse := mail.ReadMessage(bytes.NewReader(data)); errParse == nil {
			dec := new(mime.WordDecoder)
			subject = email.DecodeHeader(dec, m.Header.Get("Subject"))
		}
	}
	if subject == "" {
		subject = "(No Subject)"
	}

	if !config.Quiet {
		rep.Infof("Training message %s (%q) as ham locally...\n", targetID, subject)
	}

	_, err = trainHamUntilMarked(rep, emailBytes, msgID, config)
	if err != nil {
		return err
	}

	clearTrainedDBs(rep, client.Config().AccountCache, config.ConfigDir, msgID, config.Quiet)

	_ = msg.SetOverride(client.Config().AccountCache, msgID, msg.OverrideHam)

	if config.ReadOnly {
		rep.Printf("[DRY-RUN] Would move message %s to %s\n", targetID, client.InboxFolder())
		return nil
	}

	if !config.Quiet {
		rep.Successf("Successfully learned message %s as ham locally.\n", targetID)
		rep.Infof("Moving message back to inbox on server...\n")
	}
	if err := client.MoveToInbox([]email.ID{msgID}, label.FromServer(folderName)); err != nil {
		return fmt.Errorf("failed to move message to inbox: %w", err)
	}
	_ = folderindex.MoveBatch(client.Config().AccountCache, []email.ID{msgID}, folderName, client.InboxFolder().String())
	if !config.Quiet {
		rep.Successf("Successfully moved message %s to inbox.\n", targetID)
	}

	return nil
}

// UnspamFolder trains every message in folder as ham, records each as a ham
// override, and moves them back to the inbox. Under ReadOnly it trains locally
// and reports the move it would make; an empty folder succeeds having done
// nothing.
func UnspamFolder(rep report.Reporter, client interface {
	Validate() error
	mailclient.ConfigProvider
	mailclient.EmailFetcher
	mailclient.EmailWriter
	mailclient.LabelReader
}, config *cfg_g.Config, folder label.Folder) error {
	if err := client.Validate(); err != nil {
		return err
	}
	folderName := folder.String()

	rep.Infof("Running in Folder Unspam Mode, targeting folder '%s'...\n", folderName)

	cacheSubdir := cfg_g.SanitizeLabelForCache(folderName)
	ids, err := client.FetchAndDownloadEmails(folder, cacheSubdir)
	if err != nil {
		return err
	}

	if len(ids) == 0 {
		rep.Infof("Folder %q is empty or no emails found. Nothing to unspam.\n", folderName)
		return nil
	}

	rep.Infof("Training Bogofilter on %d ham emails from folder %q...\n", len(ids), folderName)

	successIDs := []email.ID{}
	for _, id := range ids {
		exists, _ := msg.Exists(client.Config().AccountCache, id)
		if !exists {
			continue
		}

		emailBytes, err := msg.Read(client.Config().AccountCache, id)
		if err != nil {
			rep.Warnf("Error reading cached email ID %s: %v\n", id, err)
			continue
		}

		_, err = trainHamUntilMarked(rep, emailBytes, id, config)
		if err != nil {
			rep.Warnf("Bogofilter training failed for ID %s: %v\n", id, err)
			continue
		}

		clearTrainedDBs(rep, client.Config().AccountCache, config.ConfigDir, id, config.Quiet)
		_ = msg.SetOverride(client.Config().AccountCache, id, msg.OverrideHam)
		successIDs = append(successIDs, id)
	}

	if len(successIDs) > 0 {
		rep.Successf("Successfully trained Bogofilter on %d email(s) as ham.\n", len(successIDs))
		if config.ReadOnly {
			rep.Printf("[DRY-RUN] Would move %d message(s) to %s\n", len(successIDs), client.InboxFolder())
			return nil
		}
		rep.Infof("Moving %d email(s) back to inbox on server...\n", len(successIDs))
		if err := client.MoveToInbox(successIDs, folder); err != nil {
			return fmt.Errorf("failed to move messages to inbox: %w", err)
		}
		for _, id := range successIDs {
			_ = folderindex.MoveBatch(client.Config().AccountCache, []email.ID{email.ID(id)}, folderName, client.InboxFolder().String())
		}
		rep.Successf("Successfully moved %d email(s) to inbox.\n", len(successIDs))
	} else {
		rep.Warnf("No emails successfully trained as ham.\n")
	}

	return nil
}

// ResetLearning drops every message's scan evidence and deletes the three
// training ledgers, leaving the user's overrides intact. It is the "start over"
// path for a filter whose scores have been poisoned.
func ResetLearning(rep report.Reporter, config *cfg_g.Config) error {
	downloadDir := config.AccountCache
	configDir := config.ConfigDir

	if !config.Quiet {
		rep.Infof("Resetting all spam scores and classifications in cache for account...\n")
	}

	if err := msg.ForgetAllEvidence(downloadDir); err != nil {
		return fmt.Errorf("failed to reset classifications: %w", err)
	}

	for _, path := range []string{
		train.MessageIDPath(configDir),
		train.SpamUIDPath(downloadDir),
		train.HamPath(downloadDir),
	} {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			if !config.Quiet {
				rep.Warnf("    Warning: failed to remove %s: %v\n", path, err)
			}
		}
	}

	if !config.Quiet {
		rep.Successf("Successfully reset all cache scores and local training data.\n")
	}

	return nil
}
