package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m *tuiModel) renderComposeScreen() string {
	if m.compose == nil {
		return ""
	}

	if m.compose.DiscardConfirm {
		return renderDiscardConfirmDialog(m)
	}

	if m.compose.WarnEmptySend {
		return renderWarnEmptySendDialog(m)
	}

	if m.compose.SendConfirm {
		return renderSendConfirmDialog(m)
	}

	bg := lipgloss.Color("#16162a")
	// The screen is the whole terminal, with no frame: a border along the
	// terminal's own edge separates nothing, takes two rows and four columns,
	// and lands in every Shift-drag selection. The rules between the headers,
	// the body and the footer stay, since they do separate something.
	boxW := max(40, m.width)

	var lines []string

	// 1. Top Header Banner
	accLabel := m.compose.AccountName
	if accLabel == "" {
		accLabel = "default"
	}
	topTitle := fmt.Sprintf(" ✉ COMPOSE & SEND REVIEW  [%s] ", accLabel)
	if badge := m.composePgpBadge(); badge != "" {
		topTitle += " " + badge + " "
	}
	if m.mouseOff {
		topTitle += " " + mouseOffHint + " "
	}
	topStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#ffd700")).
		Background(lipgloss.Color("#2a2a4e")).
		Padding(0, 1)
	lines = append(lines, topStyle.Render(topTitle), "")

	// 2. Header Fields
	lines = append(lines, renderComposeHeaderFields(m)...)
	lines = append(lines, "")

	// 3. Attachments List
	lines = append(lines, renderComposeAttachments(m)...)
	lines = append(lines, "")

	// 4. Body Viewport
	divStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#445588"))
	divider := divStyle.Render(strings.Repeat("─", max(10, boxW-2)))
	bodyHeader := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#55ffff")).Render(" Message Body:")
	lines = append(lines, divider, bodyHeader)

	// Below the body: a divider and the two footer rows.
	usedLines := len(lines) + 3
	availHeight := max(4, m.height-usedLines)
	if m.compose.Vp.Height != availHeight || m.compose.Vp.Width != boxW-2 {
		m.compose.Vp.Height = availHeight
		m.compose.Vp.Width = boxW - 2
		m.compose.Vp.SetContent(m.compose.Body)
	}
	lines = append(lines, keyZone("e", zoneRow, m.compose.Vp.View()), divider)

	// 5. Footer Menu / Keybindings
	lines = append(lines, renderComposeFooter())

	content := strings.Join(lines, "\n")
	return lipgloss.NewStyle().
		Background(bg).
		Padding(0, 1).
		Width(boxW).
		Height(m.height).
		Render(content)
}

func renderComposeHeaderFields(m *tuiModel) []string {
	var lines []string
	labelStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#ffdd44"))
	valStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#ffffff"))
	editStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#55ffff")).Background(lipgloss.Color("#333366"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))

	// From
	fromVal := m.compose.From
	if fromVal == "" {
		fromVal = "(none)"
	}
	lines = append(lines, fmt.Sprintf(" %-9s %s %s", labelStyle.Render("From:"), valStyle.Render(fromVal), dimStyle.Render("(F3 to change)")))

	// To, Cc and Bcc are the same row behind a different field.
	addrRows := []struct {
		label string
		field composeField
		value string
		hint  string
	}{
		{"To:", fieldTo, m.compose.To, "(none — press 't' to edit)"},
		{"Cc:", fieldCc, m.compose.Cc, "(none — press 'c' to edit)"},
		{"Bcc:", fieldBcc, m.compose.Bcc, "(none — press 'b' to edit)"},
	}
	for _, row := range addrRows {
		if m.compose.FieldEditing == row.field {
			lines = append(lines, fmt.Sprintf(" %-9s %s %s", labelStyle.Render(row.label), editStyle.Render(m.compose.FieldInput+"█"), dimStyle.Render("[Enter: Save, Tab: Complete, F5: Contacts, Esc: Cancel]")))
			continue
		}
		val := row.value
		if val == "" {
			val = dimStyle.Render(row.hint)
		} else {
			val = valStyle.Render(val)
		}
		lines = append(lines, fmt.Sprintf(" %-9s %s", labelStyle.Render(row.label), val))
	}

	// Subject
	if m.compose.FieldEditing == fieldSubject {
		lines = append(lines, fmt.Sprintf(" %-9s %s %s", labelStyle.Render("Subject:"), editStyle.Render(m.compose.FieldInput+"█"), dimStyle.Render("[Enter: Save, Esc: Cancel]")))
	} else {
		subjVal := m.compose.Subject
		if subjVal == "" {
			subjVal = dimStyle.Render("(no subject — press 's' to edit)")
		} else {
			subjVal = valStyle.Bold(true).Render(subjVal)
		}
		lines = append(lines, fmt.Sprintf(" %-9s %s", labelStyle.Render("Subject:"), subjVal))
	}

	// Each row is clickable as the key that edits it.
	for i, k := range []string{"f3", "t", "c", "b", "s"} {
		lines[i] = keyZone(k, zoneRow, lines[i])
	}
	return lines
}

func renderComposeAttachments(m *tuiModel) []string {
	var lines []string
	labelStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#ffdd44"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))

	if len(m.compose.Attachments) == 0 {
		lines = append(lines, keyZone("a", zoneRow, fmt.Sprintf(" %-9s %s", labelStyle.Render("Attach:"), dimStyle.Render("(none — press 'a' to attach a file)"))))
		return lines
	}

	lines = append(lines, keyZone("a", zoneRow, fmt.Sprintf(" %-9s %s", labelStyle.Render("Attach:"), lipgloss.NewStyle().Foreground(lipgloss.Color("#55ffff")).Render(fmt.Sprintf("%d file(s)", len(m.compose.Attachments))))))

	for i, att := range m.compose.Attachments {
		isSel := (i == m.compose.AttCursor)
		prefix := "    "
		if isSel {
			prefix = "  > "
		}
		attInfo := fmt.Sprintf("[%d] %s (%s)", i+1, att.Filename, formatFileSize(len(att.Data)))
		if isSel {
			style := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#ffffff")).Background(lipgloss.Color("#333366"))
			lines = append(lines, attZone(i, prefix+style.Render(attInfo)))
		} else {
			style := lipgloss.NewStyle().Foreground(lipgloss.Color("#cccccc"))
			lines = append(lines, attZone(i, prefix+style.Render(attInfo)))
		}
	}
	return lines
}

func renderComposeFooter() string {
	keyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#55ffff")).Bold(true)
	descStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#aaaaaa"))
	sendStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#55ff55")).Bold(true)
	exitStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#ff5555")).Bold(true)

	// Each entry is clickable as its key; Send asks first when clicked.
	item := func(zoneKey string, style lipgloss.Style, label, desc string) string {
		return keyZone(zoneKey, zoneFooter, style.Render(label)+" "+descStyle.Render(desc))
	}
	row1 := " " + strings.Join([]string{
		item(composeSendZone, sendStyle, "[y/Enter]", "Send"),
		item("e", keyStyle, "[e]", "Edit Body"),
		item("t", keyStyle, "[t]", "To"),
		item("c", keyStyle, "[c]", "Cc"),
		item("b", keyStyle, "[b]", "Bcc"),
		item("s", keyStyle, "[s]", "Subj"),
		item("f5", keyStyle, "[F5]", "Contacts"),
	}, "  ")
	row2 := " " + strings.Join([]string{
		item("a", keyStyle, "[a]", "Add Att"),
		item("d", keyStyle, "[d]", "Del Att"),
		item("v", keyStyle, "[v]", "View Att"),
		item("p", keyStyle, "[p/P]", "PGP"),
		item("f3", keyStyle, "[F3]", "Account"),
		item("q", exitStyle, "[q/Esc]", "Save & Exit"),
		item("x", exitStyle, "[x]", "Discard"),
	}, "  ")
	return row1 + "\n" + row2
}

// dialogActionRow renders a dialog's confirm/cancel row: a yes button, spacing,
// and a no button, each clickable as its key.
func dialogActionRow(yLabel, nLabel string, yStyle, nStyle lipgloss.Style) string {
	return keyZone("y", zoneDialog, yStyle.Render("  "+yLabel)) + "    " +
		keyZone("n", zoneDialog, nStyle.Render("  "+nLabel))
}

func renderDiscardConfirmDialog(m *tuiModel) string {
	dialogWidth := 50
	dialogHeight := 8

	var content strings.Builder
	content.WriteString("\n")
	content.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#ff5555")).Render("         DISCARD DRAFT?         ") + "\n\n")
	content.WriteString("  Are you sure you want to discard this draft?\n")
	content.WriteString("  The local draft file will be deleted.\n\n")
	content.WriteString(dialogActionRow("[y] Yes, Discard", "[n/Esc] Cancel",
		lipgloss.NewStyle().Foreground(lipgloss.Color("#ff5555")).Bold(true),
		lipgloss.NewStyle().Foreground(lipgloss.Color("#55ff55"))))

	dialogBox := lipgloss.NewStyle().
		Background(lipgloss.Color("#16162a")).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#ff5555")).
		Padding(1, 2).
		Width(dialogWidth).
		Height(dialogHeight).
		Render(content.String())

	return lipgloss.NewStyle().
		Width(m.width).
		Height(m.height).
		Align(lipgloss.Center, lipgloss.Center).
		Render(dialogBox)
}

func renderWarnEmptySendDialog(m *tuiModel) string {
	dialogWidth := 56
	dialogHeight := 9

	reason := m.compose.WarnEmptyReason
	if reason == "" {
		reason = "Message has empty fields!"
	}

	var content strings.Builder
	content.WriteString("\n")
	content.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#ffd700")).Render("      ⚠ WARNING: EMPTY EMAIL FIELDS      ") + "\n\n")
	content.WriteString(fmt.Sprintf("  %s\n", lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#ffaa00")).Render(reason)))
	content.WriteString("  It is strongly advised to provide a subject and body.\n\n")
	content.WriteString(dialogActionRow("[y] Send Anyway", "[n/Esc] Return to Edit",
		lipgloss.NewStyle().Foreground(lipgloss.Color("#ff5555")),
		lipgloss.NewStyle().Foreground(lipgloss.Color("#55ff55")).Bold(true)))

	dialogBox := lipgloss.NewStyle().
		Background(lipgloss.Color("#16162a")).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#ffd700")).
		Padding(1, 2).
		Width(dialogWidth).
		Height(dialogHeight).
		Render(content.String())

	return lipgloss.NewStyle().
		Width(m.width).
		Height(m.height).
		Align(lipgloss.Center, lipgloss.Center).
		Render(dialogBox)
}

// renderSendConfirmDialog asks before a clicked Send goes out, naming where it
// goes so the answer is an informed one.
func renderSendConfirmDialog(m *tuiModel) string {
	to := m.compose.To
	if to == "" {
		to = "(no recipient)"
	}
	var content strings.Builder
	content.WriteString("\n")
	content.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#55ff55")).Render("          SEND THIS EMAIL?          ") + "\n\n")
	content.WriteString(fmt.Sprintf("  To: %s\n", truncateTo(to, 44)))
	content.WriteString(fmt.Sprintf("  %d attachment(s)\n\n", len(m.compose.Attachments)))
	content.WriteString(dialogActionRow("[y] Send", "[n/Esc] Cancel",
		lipgloss.NewStyle().Foreground(lipgloss.Color("#55ff55")).Bold(true),
		lipgloss.NewStyle().Foreground(lipgloss.Color("#ff5555"))))

	dialogBox := lipgloss.NewStyle().
		Background(lipgloss.Color("#16162a")).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#55ff55")).
		Padding(1, 2).
		Width(54).
		Height(8).
		Render(content.String())

	return lipgloss.NewStyle().
		Width(m.width).
		Height(m.height).
		Align(lipgloss.Center, lipgloss.Center).
		Render(dialogBox)
}
