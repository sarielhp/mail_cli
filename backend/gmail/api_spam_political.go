package gmail

import (
	"bytes"
	"fmt"
	"github.com/sarielhp/mail_cli/app"
	"github.com/sarielhp/mail_cli/backend/political"
	"github.com/sarielhp/mail_cli/cache/msg"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/report"
	"log"
	"mime"
	"net/mail"
	"strings"

	gmailapi "google.golang.org/api/gmail/v1"
)

type politicalSpamInfo struct {
	id              string
	fromHeader      string
	sender          string
	subject         string
	listUnsubscribe string
	score           float64
	triggered       []string
	isBlacklisted   bool
	unsubSuccess    bool
}

func fetchAndDownloadSpamMessages(rep report.Reporter, srv *gmailapi.Service, config *Config) ([]email.ID, error) {
	rep.Infof("Fetching recent spam emails from Gmail Spam folder...\n")
	res, err := srv.Users.Messages.List("me").Q("label:SPAM").IncludeSpamTrash(true).MaxResults(int64(config.Limit)).Do()
	if err != nil {
		return nil, fmt.Errorf("failed to list spam: %w", err)
	}

	if len(res.Messages) == 0 {
		return nil, nil
	}

	var allIDs []email.ID
	var missingIDs []string
	for _, m := range res.Messages {
		allIDs = append(allIDs, email.ID(m.Id))
		exists, err := msg.Exists(config.AccountCache, email.ID(m.Id))
		if err != nil || !exists {
			missingIDs = append(missingIDs, m.Id)
		}
	}

	if err := downloadMissingSpamEmails(rep, srv, config, missingIDs, "political inspection"); err != nil {
		return nil, err
	}
	return allIDs, nil
}

// handlePoliticalUnsubscribe opts out of a mailing list. act is false for an
// audit, which must not reach here at all: sending an opt-out tells a sender
// the address is live, and it is irreversible.
func handlePoliticalUnsubscribe(rep report.Reporter, srv *gmailapi.Service, config *Config, listUnsubscribe, fromHeader string, act bool) bool {
	if !act || listUnsubscribe == "" || !config.AutoUnsubscribe || config.ReadOnly {
		return false
	}
	mailto, httpLink := email.ParseListUnsubscribe(listUnsubscribe)
	unsubSuccess := false

	if mailto != "" {
		if config.Verbose {
			rep.Infof("             Attempting mailto unsubscribe...\n")
		}
		errUnsub := executeMailtoUnsubscribeREST(srv, config, mailto)
		email.LogUnsubscription(config.ConfigDir, fromHeader, "mailto", mailto, errUnsub)
		if errUnsub == nil {
			if config.Verbose {
				rep.Successf("             Mailto unsub sent successfully via Gmail API!\n")
			}
			unsubSuccess = true
		} else if config.Verbose {
			rep.Errorf("             Mailto unsub failed: %v\n", errUnsub)
		}
	}

	if httpLink != "" {
		if config.Verbose {
			rep.Infof("             Attempting HTTP unsubscribe...\n")
		}
		errUnsub := email.ExecuteHTTPUnsubscribe(httpLink)
		email.LogUnsubscription(config.ConfigDir, fromHeader, "http", httpLink, errUnsub)
		if errUnsub == nil {
			if config.Verbose {
				rep.Successf("             HTTP unsub requested successfully!\n")
			}
			unsubSuccess = true
		} else if config.Verbose {
			rep.Errorf("             HTTP unsub failed: %v\n", errUnsub)
		}
	}
	return unsubSuccess
}

func checkAutoBlacklistPolitical(rep report.Reporter, config *Config, sender, listUnsubscribe string, score float64, act bool) bool {
	if !act || !email.IsSafeToAutoBlacklist(sender, listUnsubscribe, score) {
		return false
	}
	if errBlacklist := cfg_g.AutoBlacklistInternal(config, sender); errBlacklist != nil {
		if config.Verbose {
			rep.Errorf("             Failed to auto-blacklist sender: %v\n", errBlacklist)
		}
		return false
	}
	return true
}

func printPoliticalDetails(rep report.Reporter, idx int, info *politicalSpamInfo, verbose, autoUnsub bool) {
	if verbose {
		rep.Printf("[%d] ID: ", idx)
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

		if info.listUnsubscribe != "" {
			rep.Printf("    Unsub:   Found header: %s\n", info.listUnsubscribe)
			if autoUnsub && info.unsubSuccess {
				rep.Successf("             Marked email for deletion from Gmail Spam folder.\n")
			}
		}
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
	actionStr := ""
	if len(actions) > 0 {
		actionStr = " [" + strings.Join(actions, " & ") + "]"
	}
	scorePrinter := app.ColorYellow
	if info.score >= 20.0 {
		scorePrinter = app.ColorRed
	}
	scoreStr := scorePrinter.Sprintf("%.1f", info.score)
	rep.Printf("  - %d. [%s/10.0] From: %s | Subj: %s%s\n", idx, scoreStr, info.fromHeader, info.subject, actionStr)
}

// deleteUnsubscribedSpamMessages permanently removes messages that were just
// unsubscribed from. act is false for an audit, which reports and changes
// nothing.
func deleteUnsubscribedSpamMessages(rep report.Reporter, srv *gmailapi.Service, config *Config, idsToDelete []string, act bool) {
	if len(idsToDelete) == 0 || !act {
		return
	}
	if config.ReadOnly {
		rep.Infof("[DRY-RUN] Would delete %d unsubscribed email(s) from the Gmail Spam folder.\n", len(idsToDelete))
		return
	}
	rep.Infof("Deleting %d unsubscribed emails from Gmail Spam folder...\n", len(idsToDelete))
	err := srv.Users.Messages.BatchDelete("me", &gmailapi.BatchDeleteMessagesRequest{
		Ids: idsToDelete,
	}).Do()
	if err != nil {
		log.Printf("%s Failed to batch delete unsubscribed emails: %v", app.PrefixWarn, err)
		return
	}
	rep.Successf("Successfully deleted %d emails from Gmail Spam folder.\n", len(idsToDelete))
}

func inspectSinglePoliticalEmail(rep report.Reporter, srv *gmailapi.Service, config *Config, id email.ID, act bool) (*politicalSpamInfo, error) {
	emailBytes, err := msg.Read(config.AccountCache, id)
	if err != nil {
		return nil, err
	}
	localEmail, errMail := mail.ReadMessage(bytes.NewReader(emailBytes))
	if errMail != nil {
		return nil, errMail
	}

	dec := new(mime.WordDecoder)
	subject := email.DecodeHeader(dec, localEmail.Header.Get("Subject"))
	fromHeader := email.DecodeHeader(dec, localEmail.Header.Get("From"))
	bodyStr, _ := email.ExtractPlainBodyText(localEmail)
	bodyStr = email.StripHTML(bodyStr)
	if len(bodyStr) > 8192 {
		bodyStr = bodyStr[:8192]
	}

	listUnsubscribe := localEmail.Header.Get("List-Unsubscribe")
	_, score, triggered := email.DetectPolitical(subject, bodyStr)
	sender := email.ParseEmailAddress(fromHeader)
	isBlacklisted := checkAutoBlacklistPolitical(rep, config, sender, listUnsubscribe, score, act)
	unsubSuccess := handlePoliticalUnsubscribe(rep, srv, config, listUnsubscribe, fromHeader, act)

	return &politicalSpamInfo{
		id:              string(id),
		fromHeader:      fromHeader,
		sender:          sender,
		subject:         subject,
		listUnsubscribe: listUnsubscribe,
		score:           score,
		triggered:       triggered,
		isBlacklisted:   isBlacklisted,
		unsubSuccess:    unsubSuccess,
	}, nil
}

func showPoliticalSpamInGmailREST(rep report.Reporter, config *Config, act bool) error {
	srv, err := GetGmailService(rep, config)
	if err != nil {
		return err
	}

	allIDs, err := fetchAndDownloadSpamMessages(rep, srv, config)
	if err != nil {
		return err
	}
	if len(allIDs) == 0 {
		rep.Infof("No emails found in Gmail Spam folder.\n")
		return nil
	}

	rep.Infof("Inspecting %d spam emails for political content...\n", len(allIDs))
	politicalIDs := political.FilterIDs(config.AccountCache, allIDs, config.Whitelist)
	if len(politicalIDs) == 0 {
		return nil
	}

	rep.Printf("\n")
	rep.Printf("%s\n", app.ColorBoldPurple.Sprint("======================================================================"))
	rep.Printf("%s\n", app.ColorBoldPurple.Sprint("           POLITICAL DONATION EMAILS DETECTED IN SPAM FOLDER          "))
	rep.Printf("%s\n", app.ColorBoldPurple.Sprint("======================================================================"))

	var idsToDelete []string
	for idx, id := range politicalIDs {
		info, err := inspectSinglePoliticalEmail(rep, srv, config, email.ID(id), act)
		if err != nil {
			continue
		}
		if info.unsubSuccess {
			idsToDelete = append(idsToDelete, string(id))
		}
		printPoliticalDetails(rep, idx+1, info, config.Verbose, config.AutoUnsubscribe)
	}

	deleteUnsubscribedSpamMessages(rep, srv, config, idsToDelete, act)

	rep.Printf("Found ")
	rep.Printf("%s", app.ColorBoldPurple.Sprintf("%d", len(politicalIDs)))
	rep.Printf(" political email(s) out of %d spam emails analyzed.\n", len(allIDs))
	rep.Printf("%s\n", app.ColorBoldPurple.Sprint("======================================================================"))

	return nil
}
