package tui

import (
	"fmt"
	"strings"

	"github.com/sarielhp/mail_cli/cache/cachedir"
	"github.com/sarielhp/mail_cli/cache/msg"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/llm"
	"github.com/sarielhp/mail_cli/tui/render"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func (m *tuiModel) openDeadlinesAgenda() (tea.Model, tea.Cmd) {
	cfg := m.Config()
	if cfg == nil {
		cfg = m.cfg
	}
	var cacheDir cachedir.Account
	if cfg != nil {
		cacheDir = cfg.AccountCache
	}

	var list []deadlineEntry

	for _, em := range m.rawEmails {
		parsed, hasParsed := m.summaryParsedCache[em.ID]
		if !hasParsed && !cacheDir.IsZero() {
			if sumText, err := msg.ReadSummary(cacheDir, em.ID); err == nil && sumText != "" {
				parsed = llm.ParseSummary(sumText)
				if m.summaryParsedCache == nil {
					m.summaryParsedCache = make(map[email.ID]llm.ParsedSummary)
				}
				m.summaryParsedCache[em.ID] = parsed
				hasParsed = true
			}
		}

		if hasParsed && len(parsed.Deadlines) > 0 {
			for _, d := range parsed.Deadlines {
				list = append(list, deadlineEntry{
					EmailID:     em.ID,
					Sender:      em.FromRaw,
					Subject:     em.Subject,
					DateText:    d.Date,
					Description: d.Description,
					EmailDate:   em.EmailDate,
				})
			}
		}
	}

	m.deadlinesList = list
	m.deadlinesCursor = 0
	m.showDeadlinesAgenda = true
	return m, nil
}

func (m *tuiModel) kDeadlinesAgenda(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := k.String()
	switch {
	case s == "esc" || s == "q" || s == "D" || k.Type == tea.KeyEsc:
		m.showDeadlinesAgenda = false
		return m, nil

	case k.Type == tea.KeyUp || s == "up" || s == "k":
		m.deadlinesCursor--
		if m.deadlinesCursor < 0 {
			m.deadlinesCursor = 0
		}

	case k.Type == tea.KeyDown || s == "down" || s == "j":
		m.deadlinesCursor++
		if m.deadlinesCursor >= len(m.deadlinesList) {
			m.deadlinesCursor = max(0, len(m.deadlinesList)-1)
		}

	case k.Type == tea.KeyEnter:
		if len(m.deadlinesList) > 0 && m.deadlinesCursor < len(m.deadlinesList) {
			targetID := m.deadlinesList[m.deadlinesCursor].EmailID
			m.showDeadlinesAgenda = false
			for i, em := range m.emails {
				if em.ID == targetID {
					m.eIdx = i
					m.selectedID = targetID
					m.adjustScrollOffset()
					break
				}
			}
			return m, nil
		}
	}
	return m, nil
}

func renderDeadlinesAgenda(m *tuiModel) string {
	t := m.theme.Theme()
	bg := lipgloss.Color("#16162a")
	borderC := lipgloss.Color("#55ffff")

	boxW := min(m.width-4, 88)
	boxH := min(m.height-6, 24)
	if boxW < 30 {
		boxW = 30
	}
	if boxH < 10 {
		boxH = 10
	}

	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(t.Get(render.ColorHighlightFg))).
		Background(bg)

	var lines []string
	lines = append(lines, headerStyle.Render(" ⏰ UPCOMING ACTIONABLE DEADLINES & DATES "))
	lines = append(lines, "")

	if len(m.deadlinesList) == 0 {
		lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color("#aaaaaa")).Render("  No upcoming deadlines found in summarized emails."))
		lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color("#8888aa")).Render("  (Summaries are generated in the background as you read)"))
	} else {
		availItems := boxH - 7
		startIdx := 0
		if m.deadlinesCursor >= availItems {
			startIdx = m.deadlinesCursor - availItems + 1
		}

		for i := startIdx; i < len(m.deadlinesList) && i < startIdx+availItems; i++ {
			d := m.deadlinesList[i]
			isSel := (i == m.deadlinesCursor)

			prefix := "  "
			if isSel {
				prefix = "> "
			}

			desc := d.Description
			maxDescW := boxW - 8
			if len([]rune(desc)) > maxDescW {
				runes := []rune(desc)
				desc = string(runes[:maxDescW-3]) + "..."
			}

			senderSubj := fmt.Sprintf("    From: %s | Subject: %s", email.FormatSender(d.Sender), d.Subject)
			if len([]rune(senderSubj)) > maxDescW {
				runes := []rune(senderSubj)
				senderSubj = string(runes[:maxDescW-3]) + "..."
			}

			if isSel {
				descStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#ffffff")).Background(lipgloss.Color("#333366"))
				subStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#bbddff")).Background(lipgloss.Color("#333366"))
				lines = append(lines, rowZone(frameDeadlines, i, 0, descStyle.Render(fmt.Sprintf("%s• %s", prefix, desc))))
				lines = append(lines, rowZone(frameDeadlines, i, 1, subStyle.Render(senderSubj)))
			} else {
				descStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#ffd700"))
				subStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#8888aa"))
				lines = append(lines, rowZone(frameDeadlines, i, 0, descStyle.Render(fmt.Sprintf("%s• %s", prefix, desc))))
				lines = append(lines, rowZone(frameDeadlines, i, 1, subStyle.Render(senderSubj)))
			}
			lines = append(lines, "")
		}
	}

	for len(lines) < boxH-3 {
		lines = append(lines, "")
	}

	lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color("#334466")).Render(strings.Repeat("─", max(10, boxW-4))))
	footer := lipgloss.NewStyle().Foreground(lipgloss.Color("#8888aa")).Render(" [Enter] Jump to Email | [↑/↓/j/k] Navigate | [Esc/q/D] Close")
	lines = append(lines, markHints(footer, hint{"[Enter] Jump to Email", "enter"}, hint{"[Esc/q/D] Close", "esc"}))

	content := strings.Join(lines, "\n")

	box := lipgloss.NewStyle().
		Background(bg).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(borderC).
		Padding(0, 1).
		Width(boxW).
		Render(content)

	return renderDimmedOverlay(m.width, m.height, m.renderBaseScreen(), box)
}
