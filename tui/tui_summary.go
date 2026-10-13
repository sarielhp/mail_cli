package tui

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/sarielhp/mail_cli/llm"
	"github.com/sarielhp/mail_cli/tui/render"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

func (m *tuiModel) setupSummaryViewport() {
	boxW := min(m.width-4, 88)
	boxH := min(m.height-12, 18)
	if boxW < 20 {
		boxW = 20
	}
	if boxH < 4 {
		boxH = 4
	}

	contentW := max(16, boxW-4)
	if m.summaryVp.Width == 0 {
		m.summaryVp = viewport.New(contentW, boxH)
	} else {
		m.summaryVp.Width = contentW
		m.summaryVp.Height = boxH
	}

	renderedContent := renderMarkdownSummary(m.summaryContent, contentW, m.theme.Theme())
	// Only new content goes back to the top. This runs on every frame, and
	// going to the top each time undid every scroll, by key or wheel.
	if renderedContent != m.summaryShown {
		m.summaryVp.SetContent(renderedContent)
		m.summaryVp.GotoTop()
		m.summaryShown = renderedContent
	}
}

func (m *tuiModel) kSummary(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := k.String()
	switch {
	case s == "f4" || s == "F4" || s == "esc" || s == "q" || s == "enter" || k.Type == tea.KeyEsc:
		m.showSummary = false
		return m, nil
	case k.Type == tea.KeyUp || s == "up" || s == "k":
		m.summaryVp.ScrollUp(1)
	case k.Type == tea.KeyDown || s == "down" || s == "j":
		m.summaryVp.ScrollDown(1)
	case k.Type == tea.KeyLeft || s == "left":
		m.summaryVp.ScrollUp(3)
	case k.Type == tea.KeyRight || s == "right":
		m.summaryVp.ScrollDown(3)
	case k.Type == tea.KeyPgUp || s == "pageup" || s == "pgup":
		m.summaryVp.ScrollUp(m.summaryVp.Height)
	case k.Type == tea.KeyPgDown || s == "pagedown" || s == "pgdn" || s == "pgdown" || s == " ":
		m.summaryVp.ScrollDown(m.summaryVp.Height)
	case k.Type == tea.KeyHome || s == "home":
		m.summaryVp.GotoTop()
	case k.Type == tea.KeyEnd || s == "end":
		m.summaryVp.GotoBottom()
	}
	return m, nil
}

func renderSummaryOverlay(m *tuiModel) string {
	t := m.theme.Theme()
	bg := lipgloss.Color("#16162a")
	borderColor := t.Get(render.ColorBorder)

	boxW := min(m.width-4, 88)
	if boxW < 20 {
		boxW = 20
	}

	m.setupSummaryViewport()

	modelName := "llama3.1"
	if m.Config() != nil && m.Config().LLM.Model != "" {
		modelName = m.Config().LLM.Model
	} else if m.cfg != nil && m.cfg.LLM.Model != "" {
		modelName = m.cfg.LLM.Model
	}

	header := lipgloss.NewStyle().
		Background(bg).
		Foreground(lipgloss.Color(t.Get(render.ColorHighlightFg))).
		Bold(true).
		Render(fmt.Sprintf(" 🤖 EMAIL SUMMARY (%s via Local LLM) ", modelName))

	// Parse summary metadata for badges
	parsed := llm.ParseSummary(m.summaryContent)

	// Top Metadata Header (Sender, Subject, Date)
	lblStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(t.Get(render.ColorHighlightFg)))
	fromStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#55ffff"))
	subjStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#ffdd44"))
	dateStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(t.Get(render.ColorDate)))
	divStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(borderColor))

	fromVal := m.summaryFrom
	if fromVal == "" {
		fromVal = "(Unknown Sender)"
	}
	subjVal := m.summarySubject
	if subjVal == "" {
		subjVal = "(No Subject)"
	}
	dateVal := m.summaryDate
	if dateVal == "" {
		dateVal = "(No Date)"
	}

	metaFrom := lblStyle.Render("From:    ") + fromStyle.Render(fromVal)
	metaSubj := lblStyle.Render("Subject: ") + subjStyle.Render(subjVal)
	metaDate := lblStyle.Render("Date:    ") + dateStyle.Render(dateVal)

	// Badges bar: Urgency, Reply Required
	var badges []string
	if parsed.Urgency != "" {
		uBg := "#455a64"
		if parsed.Urgency == "High" {
			uBg = "#b71c1c"
		} else if parsed.Urgency == "Medium" {
			uBg = "#f57f17"
		}
		badges = append(badges, lipgloss.NewStyle().Background(lipgloss.Color(uBg)).Foreground(lipgloss.Color("#ffffff")).Bold(true).Render(" Urgency: "+parsed.Urgency+" "))
	}

	if parsed.ReplyRequired {
		badges = append(badges, lipgloss.NewStyle().Background(lipgloss.Color("#d84315")).Foreground(lipgloss.Color("#ffffff")).Bold(true).Render(" ↩ Reply Required "))
	}

	badgesLine := strings.Join(badges, " ")

	metaDivider := divStyle.Render(strings.Repeat("─", max(10, boxW-4)))

	footer := lipgloss.NewStyle().
		Background(bg).
		Foreground(lipgloss.Color(t.Get(render.ColorDate))).
		Render(markHints(" [F4/Esc/q] Close | [↑/↓/j/k/PgUp/PgDn] Scroll ", hint{"[F4/Esc/q] Close", "esc"}))

	body := m.summaryVp.View()

	var parts []string
	parts = append(parts, header, "", metaFrom, metaSubj, metaDate)
	if badgesLine != "" {
		parts = append(parts, badgesLine)
	}
	parts = append(parts, metaDivider, "", body, "", footer)

	inner := lipgloss.JoinVertical(lipgloss.Left, parts...)

	box := lipgloss.NewStyle().
		Background(bg).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color(borderColor)).
		Padding(0, 1).
		Width(boxW).
		Render(inner)

	return renderDimmedOverlay(m.width, m.height, m.renderBaseScreen(), box)
}

var (
	reBold     = regexp.MustCompile(`\*\*(.*?)\*\*|__(.*?)__`)
	reItalic   = regexp.MustCompile(`\*(.*?)\*|_(.*?)_`)
	reCode     = regexp.MustCompile("`([^`]+)`")
	reSectionH = regexp.MustCompile(`^(?i)(\d+\.\s*\*\*(?:TL;DR|Action Items|Key Dates|Deadlines|Urgency)[^\*]*\*\*:?)(.*)$`)
)

func renderMarkdownSummary(raw string, wrapWidth int, _ *render.Theme) string {
	if wrapWidth <= 0 {
		wrapWidth = 72
	}
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	var out []string

	for _, line := range strings.Split(raw, "\n") {
		trimmed := strings.TrimRight(line, " \t\r")
		if strings.TrimSpace(trimmed) == "" {
			out = append(out, "")
			continue
		}
		if isClassificationLine(trimmed) {
			continue
		}
		out = append(out, renderMarkdownLine(trimmed, wrapWidth)...)
	}

	return strings.Join(out, "\n")
}

// isClassificationLine reports a summary's spam/political verdict line. The
// prompt no longer asks for one -- a weak local model labelled ordinary mail
// "Marketing / Political" -- but summaries cached before that still carry it.
func isClassificationLine(s string) bool {
	return strings.Contains(strings.ToLower(s), "**classification**")
}

func renderMarkdownLine(trimmed string, wrapWidth int) []string {
	if strings.HasPrefix(trimmed, "### ") || strings.HasPrefix(trimmed, "## ") || strings.HasPrefix(trimmed, "# ") {
		hText := strings.TrimLeft(trimmed, "# ")
		return []string{"\x1b[1;38;2;85;255;255m" + hText + "\x1b[0m"}
	}
	if match := reSectionH.FindStringSubmatch(trimmed); len(match) > 2 {
		return renderSectionHeaderLine(match[1], match[2], wrapWidth)
	}
	if isBulletLine(trimmed) {
		return renderBulletLine(trimmed, wrapWidth)
	}
	if isNumberedLine(trimmed) {
		if lines := renderNumberedLine(trimmed, wrapWidth); len(lines) > 0 {
			return lines
		}
	}
	wrapped := wrapPlainWords(trimmed, wrapWidth)
	var out []string
	for _, w := range wrapped {
		out = append(out, formatInlineMarkdown(w))
	}
	return out
}

func isBulletLine(s string) bool {
	return strings.HasPrefix(s, "- ") || strings.HasPrefix(s, "* ") || strings.HasPrefix(s, "• ") ||
		strings.HasPrefix(s, "  - ") || strings.HasPrefix(s, "  * ")
}

func renderBulletLine(trimmed string, wrapWidth int) []string {
	bulletPrefix := "  \x1b[1;38;2;85;255;255m•\x1b[0m "
	bulletContent := strings.TrimLeft(trimmed, " -*•\t")
	wrapped := wrapPlainWords(bulletContent, max(10, wrapWidth-4))
	var out []string
	for i, w := range wrapped {
		formatted := formatInlineMarkdown(w)
		if i == 0 {
			out = append(out, bulletPrefix+formatted)
		} else {
			out = append(out, "    "+formatted)
		}
	}
	return out
}

func renderSectionHeaderLine(secTitle, secRest string, wrapWidth int) []string {
	secRest = strings.TrimSpace(secRest)
	cleanTitle := strings.ReplaceAll(strings.ReplaceAll(secTitle, "**", ""), "__", "")
	styledTitle := "\x1b[1;38;2;255;221;68m" + cleanTitle + "\x1b[0m"
	if secRest == "" {
		return []string{styledTitle}
	}
	wrappedRest := wrapPlainWords(secRest, max(10, wrapWidth-runewidth.StringWidth(cleanTitle)-1))
	var out []string
	indent := strings.Repeat(" ", runewidth.StringWidth(cleanTitle)+1)
	for i, w := range wrappedRest {
		formatted := formatInlineMarkdown(w)
		if i == 0 {
			out = append(out, styledTitle+" "+formatted)
		} else {
			out = append(out, indent+formatted)
		}
	}
	return out
}

func isNumberedLine(s string) bool {
	return len(s) > 3 && (s[0] >= '0' && s[0] <= '9') && (s[1] == '.' || s[2] == '.')
}

func renderNumberedLine(trimmed string, wrapWidth int) []string {
	dotIdx := strings.Index(trimmed, ".")
	if dotIdx <= 0 || dotIdx >= 4 {
		return nil
	}
	numStr := trimmed[:dotIdx+1]
	restContent := strings.TrimSpace(trimmed[dotIdx+1:])
	styledNum := fmt.Sprintf("  \x1b[1;38;2;255;221;68m%s\x1b[0m ", numStr)
	numWidth := 2 + len(numStr) + 1
	wrapped := wrapPlainWords(restContent, max(10, wrapWidth-numWidth))
	var out []string
	indent := strings.Repeat(" ", numWidth)
	for i, w := range wrapped {
		formatted := formatInlineMarkdown(w)
		if i == 0 {
			out = append(out, styledNum+formatted)
		} else {
			out = append(out, indent+formatted)
		}
	}
	return out
}

func wrapPlainWords(text string, maxLen int) []string {
	if maxLen <= 0 {
		return []string{text}
	}
	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{""}
	}
	var res []string
	var line []string
	curLen := 0

	for _, w := range words {
		cleanWord := strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(w, "**", ""), "*", ""), "`", "")
		wLen := runewidth.StringWidth(cleanWord)

		if len(line) > 0 && curLen+1+wLen > maxLen {
			res = append(res, strings.Join(line, " "))
			line = []string{w}
			curLen = wLen
		} else {
			if len(line) > 0 {
				curLen += 1 + wLen
			} else {
				curLen = wLen
			}
			line = append(line, w)
		}
	}
	if len(line) > 0 {
		res = append(res, strings.Join(line, " "))
	}
	return res
}

func formatInlineMarkdown(line string) string {
	res := reCode.ReplaceAllString(line, "\x1b[48;2;40;40;60;38;2;130;220;255m $1 \x1b[0m")
	res = reBold.ReplaceAllStringFunc(res, func(s string) string {
		inner := strings.Trim(s, "*_")
		return "\x1b[1;38;2;255;221;68m" + inner + "\x1b[0m"
	})
	res = reItalic.ReplaceAllStringFunc(res, func(s string) string {
		inner := strings.Trim(s, "*_")
		return "\x1b[3m" + inner + "\x1b[23m"
	})
	return res
}
