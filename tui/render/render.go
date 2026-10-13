package render

import (
	"fmt"
	"strings"

	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/label"

	"github.com/mattn/go-runewidth"
)

type styBuilder struct {
	segments []Segment
	curFg    ColorID
	curBg    ColorID
}

func newStyBuilder() *styBuilder {
	return &styBuilder{}
}

func (b *styBuilder) PushString(s string) *styBuilder {
	if s == "" {
		return b
	}
	if len(b.segments) > 0 {
		last := &b.segments[len(b.segments)-1]
		if last.Fg == b.curFg && last.Bg == b.curBg {
			last.Text += s
			return b
		}
	}
	b.segments = append(b.segments, Segment{Text: s, Fg: b.curFg, Bg: b.curBg})
	return b
}

func (b *styBuilder) PushSegment(s string, fg, bg ColorID) *styBuilder {
	b.curFg, b.curBg = fg, bg
	return b.PushString(s)
}

func (b *styBuilder) Build() ColoredString {
	return ColoredString{Segments: b.segments}
}

func joinCols(collections ...ColoredString) ColoredString {
	var all []Segment
	for _, cs := range collections {
		all = append(all, cs.Segments...)
	}
	return ColoredString{Segments: all}
}

func renderCounter(idx int, highlight bool) ColoredString {
	highlightStr := fmt.Sprintf("%4d", idx)
	if highlight {
		return Plain(highlightStr).WithFg(ColorHighlightFg).WithBg(ColorHighlightBg)
	}
	return Plain(highlightStr).WithFg(ColorDim)
}

func renderDate(date string, highlight bool) ColoredString {
	var dateCS ColoredString
	if highlight {
		dateCS = Plain(date).WithFg(ColorHighlightFg).WithBg(ColorHighlightBg)
	} else {
		dateCS = Plain(date).WithFg(ColorDate).WithBg(ColorDefault)
	}
	return dateCS.TruncateLeft(8).PadLeft(8)
}

func renderSender(e email.Email, highlight bool) ColoredString {
	from := email.BidiDisplay(email.FormatSender(e.FromRaw))
	if e.ThreadHasReplies && e.ThreadCollapsed && e.ThreadSenderSummary != "" {
		from = e.ThreadSenderSummary
	}
	if from == "" {
		from = "(Unknown Sender)"
	}
	var senderCS ColoredString
	if highlight {
		senderCS = Plain(from).WithFg(ColorHighlightFg).WithBg(ColorHighlightBg)
	} else {
		senderCS = Plain(from).WithFg(ColorSender).WithBg(ColorDefault)
	}
	return senderCS.TruncateRight(20).PadRight(20)
}

func renderThreadSubjectPrefix(e email.Email, highlight bool) ColoredString {
	var prefix string
	if e.ThreadPrefix != "" {
		prefix = e.ThreadPrefix
	} else if e.ThreadDepth > 0 {
		prefix = strings.Repeat("  ", e.ThreadDepth-1) + "└─> "
	}
	if prefix == "" {
		return ColoredString{}
	}
	if highlight {
		return Plain(prefix).WithFg(ColorHighlightFg).WithBg(ColorHighlightBg)
	}
	return Plain(prefix).WithFg(ColorHighlightFg).WithBg(ColorDefault)
}

func renderSubject(e email.Email, subjWidth int, highlight bool) ColoredString {
	subjText := e.Subject
	if e.ThreadDepth > 0 || e.ThreadPrefix != "" {
		if cleaned := email.StripSubjectPrefix(subjText); cleaned != "" {
			subjText = cleaned
		}
	}
	subjText = email.BidiDisplay(subjText)

	prefixCS := renderThreadSubjectPrefix(e, highlight)
	var textCS ColoredString
	if highlight {
		textCS = Plain(subjText).WithFg(ColorHighlightFg).WithBg(ColorHighlightBg)
	} else {
		fgColor := ColorSubject
		if e.IsSpam || e.IsPolitical || e.IsBlacklisted {
			fgColor = getBadgeFg(e)
		}
		textCS = Plain(subjText).WithFg(fgColor).WithBg(ColorDefault)
	}

	subjCS := textCS
	if len(prefixCS.Segments) > 0 {
		subjCS = joinCols(prefixCS, textCS)
	}

	if e.ThreadHasReplies && e.ThreadCollapsed {
		badgeText := fmt.Sprintf(" [+%d]", e.ThreadRepliesCount)
		var badgeCS ColoredString
		if highlight {
			badgeCS = Plain(badgeText).WithFg(ColorHighlightFg).WithBg(ColorHighlightBg)
		} else {
			badgeCS = Plain(badgeText).WithFg(ColorHighlightFg).WithBg(ColorDefault)
		}
		subjCS = joinCols(subjCS, badgeCS)
	}

	if e.IsSpam || e.IsPolitical || e.IsBlacklisted {
		badge := getBadgeText(e)
		var badgeCS, spaceCS ColoredString
		if highlight {
			badgeCS = Plain(badge).WithFg(getBadgeFg(e)).WithBg(ColorHighlightBg)
			spaceCS = Plain(" ").WithFg(ColorHighlightFg).WithBg(ColorHighlightBg)
		} else {
			badgeCS = Plain(badge).WithFg(getBadgeFg(e)).WithBg(ColorDefault)
			spaceCS = Plain(" ").WithFg(ColorDefault).WithBg(ColorDefault)
		}
		subjCS = joinCols(badgeCS, spaceCS, subjCS)
	}

	if e.Account != "" {
		// Padded to two cells, an emoji's width, so the subjects of rows with
		// a letter and rows with an emoji start in the same column.
		accBadge := runewidth.FillRight(AccountBadge(e), 2)
		var accCS, spaceCS ColoredString
		if highlight {
			accCS = Plain(accBadge).WithFg(AccountColor(e.AccountIndex)).WithBg(ColorHighlightBg)
			spaceCS = Plain(" ").WithFg(ColorHighlightFg).WithBg(ColorHighlightBg)
		} else {
			accCS = Plain(accBadge).WithFg(AccountColor(e.AccountIndex)).WithBg(ColorDefault)
			spaceCS = Plain(" ").WithFg(ColorDefault).WithBg(ColorDefault)
		}
		subjCS = joinCols(accCS, spaceCS, subjCS)
	}

	return subjCS.TruncateRight(subjWidth)
}

// AccountBadge is the tag a row shows for its account: the configured badge,
// or else the first letter of the account name, upper-cased. The colour, not
// the letter, is what tells two accounts with the same initial apart.
func AccountBadge(e email.Email) string {
	if e.AccountBadge != "" {
		return e.AccountBadge
	}
	for _, r := range e.Account {
		return strings.ToUpper(string(r))
	}
	return ""
}

func renderCount(e email.Email, highlight bool) ColoredString {
	showCount := (e.ThreadDepth == 0 && e.ThreadRepliesCount > 0)
	countStr := "  "
	if showCount {
		countStr = fmt.Sprintf("%2d", e.ThreadRepliesCount+1)
	}
	if highlight {
		return Plain(countStr).WithFg(ColorHighlightFg).WithBg(ColorHighlightBg)
	}
	if showCount {
		return Plain(countStr).WithFg(ColorBorder).WithBg(ColorDefault)
	}
	return Plain(countStr).WithFg(ColorDefault).WithBg(ColorDefault)
}

func renderStatusBadgesTagged(e email.Email, highlight, tagged bool) ColoredString {
	statusText := " "
	statusFg := ColorDefault
	switch {
	case tagged:
		statusText = "*"
		statusFg = ColorHighlightFg
	case e.IsReplied:
		statusText = "r"
		statusFg = ColorHighlightFg
	case !e.IsRead:
		statusText = "N"
		statusFg = ColorDate
	}

	var statusCS ColoredString
	if highlight {
		statusCS = Plain(statusText).WithFg(ColorHighlightFg).WithBg(ColorHighlightBg)
	} else {
		statusCS = Plain(statusText).WithFg(statusFg).WithBg(ColorDefault)
	}

	var icsCS ColoredString
	if e.HasICS {
		if highlight {
			icsCS = Plain("⏰").WithFg(ColorHighlightFg).WithBg(ColorHighlightBg)
		} else {
			icsCS = Plain("⏰").WithFg(ColorHighlightFg).WithBg(ColorDefault)
		}
	}

	var attCS ColoredString
	if e.HasAttachment {
		if highlight {
			attCS = Plain("📎").WithFg(ColorHighlightFg).WithBg(ColorHighlightBg)
		} else {
			attCS = Plain("📎").WithFg(ColorHighlightFg).WithBg(ColorDefault)
		}
	}

	nBadgeCS := joinCols(statusCS, icsCS, attCS).TruncateRight(5).PadRight(5)
	if highlight {
		for i := range nBadgeCS.Segments {
			nBadgeCS.Segments[i].Bg = ColorHighlightBg
		}
	}
	return nBadgeCS
}

// EmailRow renders one message-list row without the tagged marker; it is
// EmailRowTagged with tagged false.
func EmailRow(e email.Email, totalWidth, subjWidth, idx int, t *Theme, highlight bool, ghosted bool) ColoredString {
	return EmailRowTagged(e, totalWidth, subjWidth, idx, t, highlight, ghosted, false)
}

// EmailRowTagged renders one message-list row -- counter, status badges, date,
// sender, reply count and subject -- padding it to totalWidth when highlighted
// and striking it through when ghosted. The theme argument is unused: colours
// are resolved when the result is rendered.
func EmailRowTagged(e email.Email, totalWidth, subjWidth, idx int, _ *Theme, highlight, ghosted, tagged bool) ColoredString {
	counterCS := renderCounter(idx, highlight)
	dateCS := renderDate(e.FormattedDate, highlight)
	senderCS := renderSender(e, highlight)
	countCS := renderCount(e, highlight)
	nBadgeCS := renderStatusBadgesTagged(e, highlight, tagged)
	subjCS := renderSubject(e, subjWidth, highlight)

	var sepCS ColoredString
	if highlight {
		sepCS = Plain("  ").WithFg(ColorHighlightFg).WithBg(ColorHighlightBg)
	} else {
		sepCS = Plain("  ")
	}

	row := joinCols(counterCS, sepCS, nBadgeCS, sepCS, dateCS, sepCS, senderCS, sepCS, countCS, sepCS, subjCS)
	if ghosted {
		for i := range row.Segments {
			row.Segments[i].Strike = true
			row.Segments[i].Fg = ColorDim
		}
	}
	if highlight {
		row = row.PadRight(totalWidth)
	}
	return row
}

func getBadgeText(e email.Email) string {
	if e.IsBlacklisted {
		return "[BLACKLIST]"
	}
	if e.IsPolitical {
		return "[POLI]"
	}
	if e.IsSpam {
		return "[SPAM]"
	}
	return ""
}

func getBadgeFg(e email.Email) ColorID {
	if e.IsBlacklisted {
		return ColorSpam
	}
	if e.IsPolitical {
		return ColorPoli
	}
	if e.IsSpam {
		return ColorSpam
	}
	return ColorSubject
}

// EndOfMsgGreen returns the centred "End of message" bar, width cells wide, on
// the message-end background.
func EndOfMsgGreen(width int, t *Theme) string {
	const label = " < End of message > "
	b := newStyBuilder()
	pad := max(0, (width-len(label))/2)
	b.PushSegment(strings.Repeat(" ", pad), ColorDefault, ColorMsgEndBg)
	b.PushSegment(label, ColorDefault, ColorMsgEndBg)
	b.PushSegment(strings.Repeat(" ", width-len(label)-pad), ColorDefault, ColorMsgEndBg)
	return b.Build().Render(t)
}

// MakeLabelLine renders one folder-tree row with its frame, expand arrow, name
// and message counts, highlighting and padding it when it is the cursor row.
func MakeLabelLine(e label.TreeEntry, isCursor bool, t *Theme, width int) string {
	var segs []Segment
	if isCursor {
		segs = append(segs, Segment{Text: "  ", Fg: ColorHighlightFg, Bg: ColorHighlightBg})
	} else {
		segs = append(segs, Segment{Text: "  ", Fg: ColorDefault, Bg: ColorDefault})
	}
	frame := e.Prefix
	if frame != "" {
		var fg, bg ColorID = ColorBorder, ColorDefault
		if isCursor {
			fg, bg = ColorHighlightFg, ColorHighlightBg
		}
		segs = append(segs, Segment{Text: frame, Fg: fg, Bg: bg})
	}
	var arrow string
	if e.Node != nil && len(e.Node.Children) > 0 {
		if e.Node.Expanded {
			arrow = "[-] "
		} else {
			arrow = "[+] "
		}
	}
	if arrow != "" {
		if isCursor {
			segs = append(segs, Segment{Text: arrow, Fg: ColorHighlightFg, Bg: ColorHighlightBg})
		} else {
			segs = append(segs, Segment{Text: arrow, Fg: ColorDefault, Bg: ColorDefault})
		}
	}
	folderText := e.Text
	if e.Node != nil && len(e.Node.Children) > 0 {
		folderText += fmt.Sprintf(" [%d]", label.CountAllDescendants(e.Node))
	}
	if isCursor {
		segs = append(segs, Segment{Text: folderText, Fg: ColorHighlightFg, Bg: ColorHighlightBg})
	} else {
		segs = append(segs, Segment{Text: folderText, Fg: ColorDefault, Bg: ColorDefault})
	}
	var count string
	if e.Node != nil {
		if e.Node.MessagesUnread > 0 {
			count = fmt.Sprintf(" (%d/%d)", e.Node.MessagesUnread, e.Node.MessagesTotal)
		} else {
			count = fmt.Sprintf(" (%d)", e.Node.MessagesTotal)
		}
	}
	if count != "" {
		if isCursor {
			segs = append(segs, Segment{Text: count, Fg: ColorHighlightFg, Bg: ColorHighlightBg})
		} else {
			segs = append(segs, Segment{Text: count, Fg: ColorSubject, Bg: ColorDefault})
		}
	}
	row := ColoredString{Segments: segs}
	if isCursor {
		row = row.PadRight(max(0, width-2))
	}
	return row.Render(t)
}
