package tui

import (
	"fmt"
	"strings"

	"github.com/sarielhp/mail_cli/tui/render"

	"github.com/charmbracelet/lipgloss"
)

type rgb struct {
	r, g, b int
}

func interpolateColor(x, width int) string {
	colors := []rgb{
		{255, 0, 0},   // Red
		{255, 127, 0}, // Orange
		{255, 255, 0}, // Yellow
		{0, 255, 0},   // Green
		{0, 191, 255}, // Sky Blue
		{0, 0, 255},   // Blue
		{139, 0, 255}, // Violet
	}

	if x <= 0 {
		return fmt.Sprintf("#%02x%02x%02x", colors[0].r, colors[0].g, colors[0].b)
	}
	if x >= width-1 {
		last := colors[len(colors)-1]
		return fmt.Sprintf("#%02x%02x%02x", last.r, last.g, last.b)
	}

	numSegments := len(colors) - 1
	segmentWidth := float64(width-1) / float64(numSegments)
	segment := int(float64(x) / segmentWidth)

	t := (float64(x) - float64(segment)*segmentWidth) / segmentWidth

	c1 := colors[segment]
	c2 := colors[segment+1]

	r := int(float64(c1.r) + t*float64(c2.r-c1.r))
	g := int(float64(c1.g) + t*float64(c2.g-c1.g))
	b := int(float64(c1.b) + t*float64(c2.b-c1.b))

	return fmt.Sprintf("#%02x%02x%02x", r, g, b)
}

func renderRainbowBannerEmptyState(width, numLines int, t *render.Theme) string {
	bannerLines := []string{
		"  ██    ███████  ███    ███  ██████   ████████  ██    ██     ██  ",
		" ██     ██       ████  ████  ██   ██     ██      ██  ██       ██ ",
		"██      █████    ██ ████ ██  ██████      ██       ████         ██",
		" ██     ██       ██  ██  ██  ██          ██        ██         ██ ",
		"  ██    ███████  ██      ██  ██          ██        ██        ██  ",
	}

	bannerWidth := 61
	bannerHeight := 5

	if width < bannerWidth+4 || numLines < bannerHeight+2 {
		msgText := " 📪 Mailbox Empty "
		runes := []rune(msgText)
		var sb strings.Builder
		for x, r := range runes {
			colorStr := interpolateColor(x, len(runes))
			sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(colorStr)).Render(string(r)))
		}

		emptyMsg := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(t.Get(render.ColorBorder))).
			Padding(1, 3).
			Render(sb.String())

		return lipgloss.NewStyle().
			Width(width).
			Height(numLines).
			Align(lipgloss.Center, lipgloss.Center).
			Render(emptyMsg)
	}

	var coloredLines []string
	for _, line := range bannerLines {
		runes := []rune(line)
		var sb strings.Builder
		for x, r := range runes {
			colorStr := interpolateColor(x, len(runes))
			sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(colorStr)).Render(string(r)))
		}
		coloredLines = append(coloredLines, sb.String())
	}
	bannerStr := strings.Join(coloredLines, "\n")

	return lipgloss.NewStyle().
		Width(width).
		Height(numLines).
		Align(lipgloss.Center, lipgloss.Center).
		Render(bannerStr)
}
