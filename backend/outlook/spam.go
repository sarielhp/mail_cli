package outlook

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/sarielhp/mail_cli/app"
	"github.com/sarielhp/mail_cli/backend/political"
	"github.com/sarielhp/mail_cli/backend/spamlearn"
	"github.com/sarielhp/mail_cli/cache/msg"
	"github.com/sarielhp/mail_cli/cfg_g"
	myme "github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/label"
	"github.com/sarielhp/mail_cli/report"
	"github.com/sarielhp/mail_cli/spam"
	"github.com/sarielhp/mail_cli/train"
	"mime"
	"net/http"
	"net/mail"
	"strings"
)

func (c *client) ReportSpam(messageIDs []myme.ID, source label.Folder) error {
	return c.MoveEmail(messageIDs, source, label.FromServer("Junk Email"))
}

func (c *client) sendMail(to, subject string, body []byte) error {
	payload := map[string]interface{}{
		"message": map[string]interface{}{
			"subject": subject,
			"body": map[string]interface{}{
				"contentType": "Text",
				"content":     string(body),
			},
			"toRecipients": []map[string]interface{}{
				{
					"emailAddress": map[string]interface{}{
						"address": to,
					},
				},
			},
		},
	}
	bodyBytes, _ := json.Marshal(payload)
	resp, err := c.client.Post("https://graph.microsoft.com/v1.0/me/sendMail", "application/json", bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		b := readErrorBody(resp.Body)
		return fmt.Errorf("sendMail returned status %d: %s", resp.StatusCode, b)
	}
	return nil
}

type outlookPoliticalSpamInfo struct {
	id            string
	fromHeader    string
	sender        string
	subject       string
	score         float64
	triggered     []string
	isBlacklisted bool
	unsubSuccess  bool
}

// attemptOutlookUnsubscribe opts out of a mailing list. act is false for an
// audit, which must not send anything.
func (c *client) attemptOutlookUnsubscribe(listUnsubscribe, fromHeader string, act bool) bool {
	if !act || listUnsubscribe == "" || !c.config.AutoUnsubscribe || c.config.ReadOnly {
		return false
	}
	mailto, httpLink := myme.ParseListUnsubscribe(listUnsubscribe)
	if mailto != "" {
		errUnsub := c.sendMail(mailto, "Unsubscribe", []byte("Please unsubscribe me."))
		myme.LogUnsubscription(c.config.ConfigDir, fromHeader, "mailto", mailto, errUnsub)
		if errUnsub == nil {
			return true
		}
	}
	if httpLink != "" {
		errUnsub := myme.ExecuteHTTPUnsubscribe(httpLink)
		myme.LogUnsubscription(c.config.ConfigDir, fromHeader, "http", httpLink, errUnsub)
		if errUnsub == nil {
			return true
		}
	}
	return false
}

func (c *client) inspectPoliticalEmail(id myme.ID, act bool) (*outlookPoliticalSpamInfo, error) {
	emailBytes, rErr := msg.Read(c.config.AccountCache, id)
	if rErr != nil {
		return nil, rErr
	}
	localEmail, errMail := mail.ReadMessage(bytes.NewReader(emailBytes))
	if errMail != nil {
		return nil, errMail
	}
	dec := new(mime.WordDecoder)
	subject := myme.DecodeHeader(dec, localEmail.Header.Get("Subject"))
	fromHeader := myme.DecodeHeader(dec, localEmail.Header.Get("From"))
	bodyStr, _ := myme.ExtractPlainBodyText(localEmail)
	bodyStr = myme.StripHTML(bodyStr)
	if len(bodyStr) > 8192 {
		bodyStr = bodyStr[:8192]
	}
	listUnsubscribe := localEmail.Header.Get("List-Unsubscribe")
	_, score, triggered := myme.DetectPolitical(subject, bodyStr)
	sender := myme.ParseEmailAddress(fromHeader)

	isBlacklisted := false
	if act && myme.IsSafeToAutoBlacklist(sender, listUnsubscribe, score) {
		if errBlacklist := cfg_g.AutoBlacklistInternal(c.config, sender); errBlacklist == nil {
			isBlacklisted = true
		}
	}

	unsubSuccess := c.attemptOutlookUnsubscribe(listUnsubscribe, fromHeader, act)

	return &outlookPoliticalSpamInfo{
		id:            string(id),
		fromHeader:    fromHeader,
		sender:        sender,
		subject:       subject,
		score:         score,
		triggered:     triggered,
		isBlacklisted: isBlacklisted,
		unsubSuccess:  unsubSuccess,
	}, nil
}

func printOutlookPoliticalEmail(rep report.Reporter, count int, info *outlookPoliticalSpamInfo, verbose bool) {
	if verbose {
		rep.Printf("[%d] ID: ", count)
		rep.Printf("%s\n", app.ColorBold.Sprint(info.id))
		rep.Printf("    From:    ")
		rep.Printf("%s\n", app.ColorCyan.Sprint(info.fromHeader))
		rep.Printf("    Subject: ")
		rep.Printf("%s\n", app.ColorBold.Sprint(info.subject))
		scorePrinter := app.ColorYellow
		if info.score >= 20.0 {
			scorePrinter = app.ColorRed
		}
		rep.Printf("    Score:   ")
		scorePrinter.Printf("%.1f", info.score)
		rep.Printf("/10.0\n")
		rep.Printf("    Keys:    ")
		rep.Printf("%s\n", app.ColorYellow.Sprint(strings.Join(info.triggered, ", ")))
		rep.Printf("----------------------------------------------------------------------\n")
		return
	}

	var actions []string
	if info.unsubSuccess {
		actions = append(actions, "Unsubscribed")
	}
	if info.isBlacklisted {
		actions = append(actions, "Blacklisted")
	}
	actionStr := "None"
	if len(actions) > 0 {
		actionStr = strings.Join(actions, ", ")
	}
	rep.Printf("  Political Spam: %s -> %s\n", info.sender, actionStr)
}

func (c *client) ShowPoliticalSpam(act bool) error {
	if err := c.init(); err != nil {
		return err
	}
	spamFolder := c.account.SpamFolder
	if spamFolder == "" {
		spamFolder = "Junk Email"
	}
	ids, err := c.FetchAndDownloadEmails(label.FromServer(spamFolder), "spam")
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		c.rep.Infof("Outlook Junk folder is empty.\n")
		return nil
	}

	politicalIDs := political.FilterIDs(c.config.AccountCache, ids, c.config.Whitelist)
	if len(politicalIDs) == 0 {
		return nil
	}

	c.rep.Printf("\n")
	c.rep.Printf("%s\n", app.ColorBoldPurple.Sprint("======================================================================"))
	c.rep.Printf("%s\n", app.ColorBoldPurple.Sprint("         POLITICAL DONATION EMAILS DETECTED IN OUTLOOK JUNK FOLDER    "))
	c.rep.Printf("%s\n", app.ColorBoldPurple.Sprint("======================================================================"))

	var idsToDelete []string
	for i, id := range politicalIDs {
		info, err := c.inspectPoliticalEmail(id, act)
		if err != nil {
			continue
		}
		if info.unsubSuccess {
			idsToDelete = append(idsToDelete, string(id))
		}
		printOutlookPoliticalEmail(c.rep, i+1, info, c.config.Verbose)
	}

	if len(idsToDelete) > 0 {
		c.rep.Warnf("%d message(s) were unsubscribed but left in place: per-message deletion is not implemented for Outlook.\n", len(idsToDelete))
	}
	return nil
}

func (c *client) LearnSpam() error {
	if err := c.init(); err != nil {
		return err
	}
	spamFolder := c.account.SpamFolder
	if spamFolder == "" {
		spamFolder = "Junk Email"
	}
	ids, err := c.FetchAndDownloadEmails(label.FromServer(spamFolder), "spam")
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		c.rep.Infof("Outlook Junk folder is empty.\n")
		return nil
	}

	total := len(ids)

	if err := app.RunPreFlightCheck(); err != nil {
		return err
	}

	trainedUidsPath := train.SpamUIDPath(c.config.AccountCache)
	trainedUids := make(map[string]bool)
	if !c.config.ForceLearn {
		// A ledger that will not parse stops the run: carrying on would
		// retrain every message in the mailbox and skew the corpus.
		ledger, lErr := train.Open(trainedUidsPath)
		if lErr != nil {
			return fmt.Errorf("%w; delete the file to start the trained-spam record over", lErr)
		}
		for _, id := range ledger.IDs() {
			trainedUids[id] = true
		}
	}

	successCount := 0
	alreadyLearnedCount := 0
	ignoredCount := 0

	for i, id := range ids {
		if trainedUids[string(id)] {
			alreadyLearnedCount++
			continue
		}
		data, rErr := msg.Read(c.config.AccountCache, id)
		if rErr != nil {
			ignoredCount++
			continue
		}

		if errRun := spam.TrainSpam(data); errRun == nil {
			successCount++
			trainedUids[string(id)] = true
			_ = msg.ForgetEvidence(c.config.AccountCache, id)
		} else if c.config.Verbose {
			c.rep.Warnf("    Bogofilter training failed for ID %s: %v\n", id, errRun)
		}
		c.rep.Progress(i+1, total, "Training classifier")
	}

	spamlearn.RecordTrained(trainedUidsPath, trainedUids)

	c.rep.Successf("%s\n", app.TrainingSummary("spam message(s)", successCount, alreadyLearnedCount, ignoredCount))
	return nil
}
