package ui

import (
	"fmt"
	"io"
	"strings"

	"github.com/sarielhp/mail_cli/app"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/terminal"

	"github.com/fatih/color"
	"github.com/mattn/go-runewidth"
)

// padOrTruncate pads or truncates string s to the given terminal width.
func padOrTruncate(s string, width int) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\t", " ")
	currentWidth := runewidth.StringWidth(s)
	if currentWidth == width {
		return s
	}
	if currentWidth > width {
		var sb strings.Builder
		w := 0
		for _, r := range s {
			rw := runewidth.RuneWidth(r)
			if w+rw > width {
				break
			}
			sb.WriteRune(r)
			w += rw
		}
		remainder := width - w
		if remainder > 0 {
			sb.WriteString(strings.Repeat(" ", remainder))
		}
		return sb.String()
	}
	return s + strings.Repeat(" ", width-currentWidth)
}

// getCellStyle returns the terminal color style based on the email state and row parity.
func getCellStyle(state string, isEven bool) *color.Color {
	var style *color.Color
	switch state {
	case "blacklist":
		style = color.New(color.FgHiYellow, color.Bold, color.CrossedOut)
	case "political":
		style = color.RGB(255, 127, 127).Add(color.Bold)
	case "spam":
		style = color.New(color.FgHiYellow, color.Bold)
	case "id":
		style = color.New(color.FgHiBlack)
	case "folder":
		style = color.New(color.FgHiBlue)
	default:
		if !isEven {
			style = color.RGB(255, 255, 170)
		} else {
			style = color.New(color.FgWhite)
		}
	}
	return style
}

// summaryColumns is the width of each column in a message listing, and whether
// the folder column is present at all.
type summaryColumns struct {
	showFolders bool
	folder      int
	sender      int
	subject     int
	id          int
}

// summaryColumnsFor divides the terminal width between the columns, giving
// whatever is left to the subject.
func summaryColumnsFor(width int, showFolders bool) summaryColumns {
	cols := summaryColumns{showFolders: showFolders, folder: 15, sender: 20, id: 8}
	if width < 70 {
		cols.folder = 10
	}
	if width < 50 {
		cols.sender = 10
	}

	gaps := 3
	if !showFolders {
		cols.folder = 0
		gaps = 2
	}

	cols.subject = width - cols.folder - cols.sender - cols.id - gaps
	if cols.subject < 10 {
		cols.subject = 10
	}
	return cols
}

// summaryRowState names the styling state a message should be shown in, and the
// marker its subject carries.
func summaryRowState(eml email.Email) (state, subjectPrefix string) {
	switch {
	case eml.IsBlacklisted:
		return "blacklist", "[BLACKLIST] "
	case eml.IsPolitical:
		return "political", "[POLI] "
	case eml.IsSpam:
		return "spam", "[SPAM] "
	default:
		return "normal", ""
	}
}

// renderSummaryFolderCell returns the folder column including its trailing gap, or an
// empty string when the listing has no folder column.
func renderSummaryFolderCell(_ io.Writer, eml email.Email, folderMap map[email.ID]string, cols summaryColumns, isEven bool) string {
	if !cols.showFolders {
		return ""
	}
	name := folderMap[eml.ID]
	if name == "" {
		name = "(Unknown)"
	}
	return getCellStyle("folder", isEven).Sprint(padOrTruncate(name, cols.folder)) + " "
}

// renderSummaryRow formats one message as a line of the listing.
func renderSummaryRow(w io.Writer, eml email.Email, folderMap map[email.ID]string, cols summaryColumns, isEven, verbose bool) string {
	state, subjectPrefix := summaryRowState(eml)

	senderPlain := email.FormatSender(eml.FromRaw)
	if verbose {
		senderPlain = email.FormatSenderVerbose(eml.FromRaw)
	}
	senderDisp := getCellStyle(state, isEven).Sprint(padOrTruncate(senderPlain, cols.sender))
	subjectDisp := getCellStyle(state, isEven).Sprint(padOrTruncate(subjectPrefix+eml.Subject, cols.subject))
	idDisp := getCellStyle("id", isEven).Sprint(padOrTruncate(string(email.ComputeShortID(eml.ID)), cols.id))

	return renderSummaryFolderCell(w, eml, folderMap, cols, isEven) + senderDisp + " " + subjectDisp + " " + idDisp
}

// summaryFooter is the closing rule, carrying the message count.
func summaryFooter(total, width int) string {
	countStr := fmt.Sprintf("==    %d messages ", total)
	if total == 1 {
		countStr = fmt.Sprintf("==    %d message ", total)
	}
	if len(countStr) < width {
		countStr += strings.Repeat("=", width-len(countStr))
	}
	return countStr
}

// FormatEmailsWithFolders prints a summary of emails along with their source
// folders.
//
// An empty folderMap drops the folder column rather than filling every row with
// "(Unknown)", and gives the space to the subject. Search results have no
// folder -- a server-side query returns message IDs and nothing about where
// they live -- and the folder is not why the user searched.
func FormatEmailsWithFolders(w io.Writer, title string, emails []email.Email, folderMap map[email.ID]string, config *cfg_g.Config) {
	width := terminal.Width()

	fmt.Fprintln(w, app.ColorBoldCyan.Sprint(title))
	fmt.Fprintln(w, app.ColorBoldCyan.Sprint(strings.Repeat("=", width)))

	cols := summaryColumnsFor(width, len(folderMap) > 0)
	for idx, eml := range emails {
		fmt.Fprintln(w, renderSummaryRow(w, eml, folderMap, cols, idx%2 == 0, config.Verbose))
	}

	fmt.Fprintln(w, app.ColorBoldCyan.Sprint(summaryFooter(len(emails), width)))
	fmt.Fprintln(w)
}
