package ui

import (
	"fmt"
	"github.com/sarielhp/mail_cli/app"
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"github.com/sarielhp/mail_cli/cache/msg"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/spam"
	"github.com/sarielhp/mail_cli/terminal"
	"io"
	"mime"
	"sort"
	"strings"
)

func loadFolderEmail(downloadDir cachedir.Account, id email.ID, _ *mime.WordDecoder, isSpam bool, res *spam.Response) email.Email {
	em, err := msg.CachedEmail(downloadDir, id)
	if err != nil {
		return PlaceholderEmail(id)
	}

	em.Subject = email.ReverseHebrewRuns(em.Subject)
	em.IsSpam = isSpam
	if res != nil {
		if _, hasPoli := res.Symbols["CUSTOM_POLITICAL_BLOCK"]; hasPoli {
			em.IsPolitical = true
		}
		if strings.Contains(res.Action, "Blacklisted Sender") {
			em.IsBlacklisted = true
		}
	}
	return *em
}

func sortFolderEmails(emails []email.Email) {
	isBadEmail := func(e email.Email) bool {
		return e.IsSpam || e.IsPolitical || e.IsBlacklisted
	}
	sort.Slice(emails, func(i, j int) bool {
		badI := isBadEmail(emails[i])
		badJ := isBadEmail(emails[j])
		if badI != badJ {
			return !badI
		}
		return emails[i].EmailDate.Before(emails[j].EmailDate)
	})
}

func emailState(eml email.Email) string {
	if eml.IsBlacklisted {
		return "blacklist"
	}
	if eml.IsPolitical {
		return "political"
	}
	if eml.IsSpam {
		return "spam"
	}
	return "normal"
}

func renderEmailRow(w io.Writer, eml email.Email, isEven bool, senderWidth, subjectWidth, idWidth int, verbose bool) {
	senderPlain := email.FormatSender(eml.FromRaw)
	if verbose {
		senderPlain = email.FormatSenderVerbose(eml.FromRaw)
	}
	senderPadded := padOrTruncate(senderPlain, senderWidth)
	senderState := emailState(eml)
	senderDisp := getCellStyle(senderState, isEven).Sprint(senderPadded)

	subjectPlain := eml.Subject
	switch {
	case eml.IsBlacklisted:
		subjectPlain = "[BLACKLIST] " + subjectPlain
	case eml.IsPolitical:
		subjectPlain = "[POLI] " + subjectPlain
	case eml.IsSpam:
		subjectPlain = "[SPAM] " + subjectPlain
	}
	subjectPadded := padOrTruncate(subjectPlain, subjectWidth)
	subjectDisp := getCellStyle(senderState, isEven).Sprint(subjectPadded)

	idPlain := string(email.ComputeShortID(eml.ID))
	idPadded := padOrTruncate(idPlain, idWidth)
	idDisp := getCellStyle("id", isEven).Sprint(idPadded)

	fmt.Fprintln(w, senderDisp+" "+subjectDisp+" "+idDisp)
}

func renderVerboseEmailDetails(w io.Writer, eml email.Email, res *spam.Response, isEven bool) {
	var details []string
	senderName, senderEmailAddr := email.ParseSenderInfo(eml.FromRaw)
	if senderEmailAddr == "" {
		senderEmailAddr = eml.FromEmail
	}
	if senderName != "" {
		details = append(details, fmt.Sprintf("  Sender Name: %s", senderName))
	} else {
		details = append(details, "  Sender Name: (none)")
	}
	details = append(details, fmt.Sprintf("  Sender Email: %s", senderEmailAddr))

	if res != nil {
		details = append(details, fmt.Sprintf("  [Bogofilter Result: Score=%.2f, Action=%s]", res.Score, res.Action))
		if len(res.Symbols) > 0 {
			details = append(details, "  Symbols:")
			var symNames []string
			for name := range res.Symbols {
				symNames = append(symNames, name)
			}
			sort.Strings(symNames)
			for _, name := range symNames {
				sym := res.Symbols[name]
				desc := sym.Description
				if desc == "" {
					desc = "(no description)"
				}
				details = append(details, fmt.Sprintf("    - %s (score=%.2f): %s", sym.Name, sym.Score, desc))
			}
		} else {
			details = append(details, "  Symbols: none")
		}
	} else {
		details = append(details, "  [Bogofilter Result: no Bogofilter data cached or fetched]")
	}

	for _, detailLine := range details {
		detailDisp := getCellStyle("id", isEven).Sprint(detailLine)
		fmt.Fprintln(w, detailDisp)
	}
}

func printFolderSummaryFooter(w io.Writer, totalMessages int, totalCountBeforePattern int, movedCount int, width int) {
	var countStr string
	switch {
	case totalCountBeforePattern > 0:
		countStr = fmt.Sprintf("==    matched %d/%d ", totalMessages, totalCountBeforePattern)
	case totalMessages == 1:
		countStr = fmt.Sprintf("==    %d message ", totalMessages)
	default:
		countStr = fmt.Sprintf("==    %d messages ", totalMessages)
	}
	if len(countStr) < width {
		countStr += strings.Repeat("=", width-len(countStr))
	}
	fmt.Fprintln(w, app.ColorBoldCyan.Sprint(countStr))
	if movedCount > 0 {
		fmt.Fprintf(w, "Messages moved by rules: %d\n", movedCount)
	}
	fmt.Fprintln(w)
}

// FormatFolderSummary prints a folder's messages as a summary table, reading
// each from the account's cache and sorting them. scanResults supplies cached
// spam verdicts for the verbose detail, and the footer reports movedCount and,
// when totalCountBeforePattern is set, how many matched out of the total.
func FormatFolderSummary(w io.Writer, folderTitle string, _ string, ids []email.ID, spamIDs []email.ID, scanResults map[email.ID]*spam.Response, config *cfg_g.Config, _ string, movedCount int, totalCountBeforePattern int) {
	width := terminal.Width()
	separator := strings.Repeat("=", width)

	fmt.Fprintln(w, app.ColorBoldCyan.Sprint(folderTitle))
	fmt.Fprintln(w, app.ColorBoldCyan.Sprint(separator))

	dec := new(mime.WordDecoder)
	isSpamMap := make(map[email.ID]bool, len(spamIDs))
	for _, id := range spamIDs {
		isSpamMap[id] = true
	}

	emails := make([]email.Email, 0, len(ids))
	for _, id := range ids {
		emails = append(emails, loadFolderEmail(config.AccountCache, id, dec, isSpamMap[id], scanResults[id]))
	}

	sortFolderEmails(emails)

	senderWidth := 20
	if width < 50 {
		senderWidth = 10
	}
	idWidth := 8
	subjectWidth := width - senderWidth - idWidth - 2
	if subjectWidth < 10 {
		subjectWidth = 10
	}

	for idx, eml := range emails {
		isEven := (idx%2 == 0)
		renderEmailRow(w, eml, isEven, senderWidth, subjectWidth, idWidth, config.Verbose)
		if config.Verbose {
			renderVerboseEmailDetails(w, eml, scanResults[eml.ID], isEven)
		}
	}

	printFolderSummaryFooter(w, len(ids), totalCountBeforePattern, movedCount, width)
}
