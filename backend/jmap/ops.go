package jmap

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/sarielhp/mail_cli/app"
	"github.com/sarielhp/mail_cli/backend/political"
	"github.com/sarielhp/mail_cli/cache/msg"
	"github.com/sarielhp/mail_cli/cfg_g"
	myme "github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/label"
	"github.com/sarielhp/mail_cli/mailclient"
	"github.com/sarielhp/mail_cli/report"
	"mime"
	"net/mail"
	"net/url"
	"strings"

	"git.sr.ht/~rockorager/go-jmap"
	"git.sr.ht/~rockorager/go-jmap/mail/email"
	"git.sr.ht/~rockorager/go-jmap/mail/emailsubmission"
)

func (c *client) DeleteAllSpam() error {
	if err := c.init(); err != nil {
		return err
	}

	spamFolder := c.account.SpamFolder
	if spamFolder == "" {
		spamFolder = "Spam"
	}

	spamID, err := c.getMailboxID(spamFolder)
	if err != nil {
		return err
	}

	destroyedCount, err := c.eachMailboxBatch(spamID, func(ids []jmap.ID) (int, error) {
		c.rep.Infof("Permanently destroying %d message(s) in JMAP Spam folder...\n", len(ids))

		reqDestroy := &jmap.Request{}
		reqDestroy.Invoke(&email.Set{
			Account: c.accID,
			Destroy: ids,
		})

		respDestroy, doErr := c.client.Do(reqDestroy)
		if doErr != nil {
			return 0, doErr
		}

		destroyed := 0
		for _, inv := range respDestroy.Responses {
			if r, ok := inv.Args.(*email.SetResponse); ok {
				destroyed += len(r.Destroyed)
				for id, errSet := range r.NotDestroyed {
					c.rep.Errorf("    Failed to destroy message %s: %s\n", id, safeStr(errSet.Description))
				}
			}
		}
		return destroyed, nil
	})
	if err != nil {
		return err
	}

	if destroyedCount == 0 {
		c.rep.Infof("JMAP Spam mailbox is already empty.\n")
		return nil
	}

	c.rep.Successf("Successfully purged %d spam emails permanently from JMAP server.\n", destroyedCount)
	return nil
}

func (c *client) MoveAllSpam(dest label.Folder) error {
	destLabel := dest.String()
	if err := c.init(); err != nil {
		return err
	}

	spamFolder := c.account.SpamFolder
	if spamFolder == "" {
		spamFolder = "Spam"
	}

	// Never escalate a move into a permanent purge (see gmail equivalent).
	if strings.EqualFold(spamFolder, destLabel) {
		return fmt.Errorf("spam_learn is set to %q, which is the spam folder itself; refusing to silently purge it. Run 'mail_cli spam del' to delete deliberately, or set spam_learn to a different folder", destLabel)
	}

	spamID, err := c.getMailboxID(spamFolder)
	if err != nil {
		return err
	}

	moved, err := c.eachMailboxBatch(spamID, func(batch []jmap.ID) (int, error) {
		ids := make([]myme.ID, 0, len(batch))
		for _, id := range batch {
			ids = append(ids, myme.ID(id))
		}
		c.rep.Infof("Moving %d message(s) from JMAP Spam folder to '%s'...\n", len(ids), destLabel)
		if moveErr := c.MoveEmail(ids, label.FromServer(spamFolder), dest); moveErr != nil {
			return len(ids) - failedCount(moveErr), moveErr
		}
		return len(ids), nil
	})
	if err != nil {
		return err
	}
	if moved == 0 {
		c.rep.Infof("JMAP Spam mailbox is already empty.\n")
	}
	return nil
}

// failedCount reports how many messages a move refused, so the batch walker can
// tell "nothing moved" from "most moved, some refused".
func failedCount(err error) int {
	var partial *mailclient.PartialMoveError
	if errors.As(err, &partial) {
		return len(partial.Failed)
	}
	return 0
}

type politicalEmailResult struct {
	id              string
	fromHeader      string
	subject         string
	score           float64
	triggered       []string
	listUnsubscribe string
	isBlacklisted   bool
	unsubSuccess    bool
}

func (c *client) ShowPoliticalSpam(act bool) error {
	ids, err := c.fetchSpamEmails()
	if err != nil || len(ids) == 0 {
		return err
	}

	politicalIDs := political.FilterIDs(c.config.AccountCache, ids, c.config.Whitelist)
	if len(politicalIDs) == 0 {
		return nil
	}

	printPoliticalHeader(c.rep)
	idsToDelete := c.processPoliticalEmails(myme.IDStrings(politicalIDs), act)
	c.deleteUnsubscribed(idsToDelete, act)
	return nil
}

func (c *client) fetchSpamEmails() ([]myme.ID, error) {
	if err := c.init(); err != nil {
		return nil, err
	}

	spamFolder := c.account.SpamFolder
	if spamFolder == "" {
		spamFolder = "Spam"
	}

	ids, err := c.FetchAndDownloadEmails(label.FromServer(spamFolder), "spam")
	if err != nil {
		return nil, err
	}

	if len(ids) == 0 {
		c.rep.Infof("JMAP Spam folder is empty.\n")
		return nil, nil
	}

	return ids, nil
}

func printPoliticalHeader(rep report.Reporter) {
	rep.Printf("\n")
	rep.Printf("%s\n", app.ColorBoldPurple.Sprint("======================================================================"))
	rep.Printf("%s\n", app.ColorBoldPurple.Sprint("           POLITICAL DONATION EMAILS DETECTED IN JMAP SPAM FOLDER     "))
	rep.Printf("%s\n", app.ColorBoldPurple.Sprint("======================================================================"))
}

func (c *client) processPoliticalEmails(politicalIDs []string, act bool) []string {
	dec := new(mime.WordDecoder)
	var idsToDelete []string

	for i, id := range politicalIDs {
		res := c.processSinglePoliticalEmail(myme.ID(id), act, dec)
		if res == nil {
			continue
		}
		printEmailResult(c.rep, i+1, res, c.config.Verbose)
		if res.unsubSuccess {
			idsToDelete = append(idsToDelete, id)
		}
	}

	return idsToDelete
}

func (c *client) processSinglePoliticalEmail(id myme.ID, act bool, dec *mime.WordDecoder) *politicalEmailResult {
	emailBytes, rErr := msg.Read(c.config.AccountCache, id)
	if rErr != nil {
		return nil
	}

	localEmail, errMail := mail.ReadMessage(bytes.NewReader(emailBytes))
	if errMail != nil {
		return nil
	}

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
	isBlacklisted := c.actSender(sender, listUnsubscribe, score, act)
	unsubSuccess := c.attemptUnsubscribe(listUnsubscribe, fromHeader, act)

	return &politicalEmailResult{
		id:              string(id),
		fromHeader:      fromHeader,
		subject:         subject,
		score:           score,
		triggered:       triggered,
		listUnsubscribe: listUnsubscribe,
		isBlacklisted:   isBlacklisted,
		unsubSuccess:    unsubSuccess,
	}
}

func (c *client) actSender(sender, listUnsubscribe string, score float64, act bool) bool {
	if !act || !myme.IsSafeToAutoBlacklist(sender, listUnsubscribe, score) {
		return false
	}

	if err := cfg_g.AutoBlacklistInternal(c.config, sender); err != nil {
		if c.config.Verbose {
			c.rep.Errorf("             Failed to auto-blacklist sender: %v\n", err)
		}
		return false
	}

	return true
}

// attemptUnsubscribe opts out of a mailing list. act is false for an audit,
// which must not send anything.
func (c *client) attemptUnsubscribe(listUnsubscribe, fromHeader string, act bool) bool {
	if !act || listUnsubscribe == "" || !c.config.AutoUnsubscribe || c.config.ReadOnly {
		return false
	}

	mailto, httpLink := myme.ParseListUnsubscribe(listUnsubscribe)
	unsubSuccess := false

	if mailto != "" {
		if c.config.Verbose {
			c.rep.Infof("             Attempting mailto unsubscribe...\n")
		}
		errUnsub := c.SendMail(mailto, "Unsubscribe", []byte("Please unsubscribe me."))
		configDir := ""
		if c.config != nil {
			configDir = c.config.ConfigDir
		}
		myme.LogUnsubscription(configDir, fromHeader, "mailto", mailto, errUnsub)
		if errUnsub != nil {
			if c.config.Verbose {
				c.rep.Errorf("             Mailto unsub failed: %v\n", errUnsub)
			}
		} else {
			if c.config.Verbose {
				c.rep.Successf("             Mailto unsub sent successfully!\n")
			}
			unsubSuccess = true
		}
	}

	if httpLink != "" {
		if c.config.Verbose {
			c.rep.Infof("             Attempting HTTP unsubscribe...\n")
		}
		errUnsub := myme.ExecuteHTTPUnsubscribe(httpLink)
		configDir := ""
		if c.config != nil {
			configDir = c.config.ConfigDir
		}
		myme.LogUnsubscription(configDir, fromHeader, "http", httpLink, errUnsub)
		if errUnsub != nil {
			if c.config.Verbose {
				c.rep.Errorf("             HTTP unsub failed: %v\n", errUnsub)
			}
		} else {
			if c.config.Verbose {
				c.rep.Successf("             HTTP unsub requested successfully!\n")
			}
			unsubSuccess = true
		}
	}

	return unsubSuccess
}

func printEmailResult(rep report.Reporter, polCount int, res *politicalEmailResult, verbose bool) {
	if verbose {
		rep.Printf("[%d] ID: ", polCount)
		rep.Printf("%s\n", app.ColorBold.Sprint(res.id))
		rep.Printf("    From:    ")
		rep.Printf("%s\n", app.ColorCyan.Sprint(res.fromHeader))
		rep.Printf("    Subject: ")
		rep.Printf("%s\n", app.ColorBold.Sprint(res.subject))

		scorePrinter := app.ColorYellow
		if res.score >= 20.0 {
			scorePrinter = app.ColorRed
		}
		rep.Printf("    Score:   ")
		scorePrinter.Printf("%.1f", res.score)
		rep.Printf("/10.0\n")

		rep.Printf("    Keys:    ")
		rep.Printf("%s\n", app.ColorYellow.Sprint(strings.Join(res.triggered, ", ")))

		if res.listUnsubscribe != "" {
			rep.Printf("    Unsub:   Found header: %s\n", res.listUnsubscribe)
			if res.unsubSuccess {
				rep.Successf("             Marked email for deletion from JMAP Spam folder.\n")
			}
		}
		rep.Printf("----------------------------------------------------------------------\n")
	} else {
		var actions []string
		if res.unsubSuccess {
			actions = append(actions, "Unsubscribed")
		}
		if res.isBlacklisted {
			actions = append(actions, "Blacklisted")
		}
		actionStr := ""
		if len(actions) > 0 {
			actionStr = " [" + strings.Join(actions, " & ") + "]"
		}
		scorePrinter := app.ColorYellow
		if res.score >= 20.0 {
			scorePrinter = app.ColorRed
		}
		scoreStr := scorePrinter.Sprintf("%.1f", res.score)
		rep.Printf("  - %d. [%s/10.0] From: %s | Subj: %s%s\n", polCount, scoreStr, res.fromHeader, res.subject, actionStr)
	}
}

// deleteUnsubscribed permanently removes messages that were unsubscribed from.
// act is false for an audit, which changes nothing.
func (c *client) deleteUnsubscribed(idsToDelete []string, act bool) {
	if !act {
		return
	}
	if len(idsToDelete) == 0 {
		return
	}
	if c.config.ReadOnly {
		c.rep.Printf("\n%s [DRY-RUN] Would permanently delete %d unsubscribed political spam email(s).\n", app.PrefixInfo, len(idsToDelete))
		return
	}

	c.rep.Printf("\n%s Automatically deleting %d unsubscribed political spam email(s) from JMAP server...\n", app.PrefixInfo, len(idsToDelete))
	reqDestroy := &jmap.Request{}
	var destroyIDs []jmap.ID
	for _, id := range idsToDelete {
		destroyIDs = append(destroyIDs, jmap.ID(id))
	}
	reqDestroy.Invoke(&email.Set{
		Account: c.accID,
		Destroy: destroyIDs,
	})
	resp, err := c.client.Do(reqDestroy)
	if err != nil {
		c.rep.Errorf("    Failed to delete unsubscribed messages: %v\n", err)
		return
	}
	forEachNotDestroyedEmail(resp, func(id jmap.ID, errSet *jmap.SetError) {
		c.rep.Errorf("    Failed to delete message %s: %s\n", id, safeStr(errSet.Description))
	})
}

func parseMailtoURL(to, subject string, body []byte) (string, string, []byte) {
	if !strings.HasPrefix(strings.ToLower(to), "mailto:") {
		return to, subject, body
	}
	u, err := url.Parse(to)
	if err != nil {
		return to, subject, body
	}
	toEmail := u.Path
	if toEmail == "" {
		toEmail = u.Opaque
	}
	mailSubject := subject
	mailBody := body
	if s := u.Query().Get("subject"); s != "" {
		mailSubject = s
	}
	if b := u.Query().Get("body"); b != "" {
		mailBody = []byte(b)
	}
	return toEmail, mailSubject, mailBody
}

func (c *client) getDraftsMailboxID() (jmap.ID, error) {
	draftsID, err := c.getMailboxID("Drafts")
	if err == nil {
		return draftsID, nil
	}
	draftsID, err = c.getMailboxID(c.inboxName())
	if err != nil {
		return "", fmt.Errorf("cannot find drafts or inbox mailbox: %w", err)
	}
	return draftsID, nil
}

func (c *client) createJMAPDraftEmail(draftsID jmap.ID, subject string, body []byte) (jmap.ID, error) {
	reqEmail := &jmap.Request{}
	reqEmail.Invoke(&email.Set{
		Account: c.accID,
		Create: map[jmap.ID]*email.Email{
			"draft1": {
				Subject:    subject,
				MailboxIDs: map[jmap.ID]bool{draftsID: true},
				BodyValues: map[string]*email.BodyValue{
					"body1": {Value: string(body)},
				},
				TextBody: []*email.BodyPart{
					{PartID: "body1", Type: "text/plain"},
				},
			},
		},
	})

	respEmail, err := c.client.Do(reqEmail)
	if err != nil {
		return "", err
	}

	for _, inv := range respEmail.Responses {
		if r, ok := inv.Args.(*email.SetResponse); ok {
			if errSet, failed := r.NotCreated["draft1"]; failed {
				return "", fmt.Errorf("failed to create JMAP draft email: %s", safeStr(errSet.Description))
			}
			for _, em := range r.Created {
				return em.ID, nil
			}
		}
	}
	return "", fmt.Errorf("failed to retrieve created draft email ID")
}

func (c *client) submitJMAPEmail(createdEmailID jmap.ID, toEmail string) error {
	identID, err := c.getIdentityID()
	if err != nil {
		return err
	}

	reqSub := &jmap.Request{}
	reqSub.Invoke(&emailsubmission.Set{
		Account: c.accID,
		Create: map[jmap.ID]*emailsubmission.EmailSubmission{
			"sub1": {
				IdentityID: identID,
				EmailID:    createdEmailID,
				Envelope: &emailsubmission.Envelope{
					MailFrom: &emailsubmission.Address{Email: c.account.GetSendAddress()},
					RcptTo:   []*emailsubmission.Address{{Email: toEmail}},
				},
			},
		},
	})

	respSub, err := c.client.Do(reqSub)
	if err != nil {
		return err
	}

	for _, inv := range respSub.Responses {
		if r, ok := inv.Args.(*emailsubmission.SetResponse); ok {
			if errSet, failed := r.NotCreated["sub1"]; failed {
				return fmt.Errorf("JMAP EmailSubmission failed: %s", safeStr(errSet.Description))
			}
		}
	}
	return nil
}

func (c *client) SendMail(to string, subject string, body []byte) error {
	if err := c.init(); err != nil {
		return err
	}

	toEmail, mailSubject, mailBody := parseMailtoURL(to, subject, body)

	draftsID, err := c.getDraftsMailboxID()
	if err != nil {
		return err
	}

	createdEmailID, err := c.createJMAPDraftEmail(draftsID, mailSubject, mailBody)
	if err != nil {
		return err
	}

	return c.submitJMAPEmail(createdEmailID, toEmail)
}
