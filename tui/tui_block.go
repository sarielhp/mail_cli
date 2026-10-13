package tui

import (
	"errors"
	"fmt"
	"github.com/sarielhp/mail_cli/label"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/sarielhp/mail_cli/cache/msg"
	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/contacts"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/mailclient"
	"github.com/sarielhp/mail_cli/organize"
	"github.com/sarielhp/mail_cli/report"
	"github.com/sarielhp/mail_cli/spam"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// BlockDestination specifies where blocked emails and future messages should go.
type BlockDestination int

const (
	blockDestSpam BlockDestination = iota
	blockDestTrash
)

type blockDoneMsg struct {
	id          email.ID
	senderEmail string
	dest        BlockDestination
	err         error
}

func (m *tuiModel) promptBlockSender() (tea.Model, tea.Cmd) {
	targetID := m.currentEmailID()
	if targetID == "" {
		return m, nil
	}
	if m.isReadOnly() {
		m.err = fmt.Errorf("🔒 Read-Only Mode: Action simulated (no server changes)")
		m.showError = true
		return m, nil
	}

	name, emailAddr := m.resolveCurrentSender(targetID)
	if emailAddr == "" {
		m.err = fmt.Errorf("cannot block sender: email address not found for message %s", targetID)
		m.showError = true
		return m, nil
	}
	if name == "" {
		name = emailAddr
	}

	m.blockSenderName = name
	m.blockSenderEmail = emailAddr
	m.blockMessageID = targetID
	m.blockModalOpen = true
	return m, nil
}

func (m *tuiModel) resolveCurrentSender(targetID email.ID) (string, string) {
	for i := range m.emails {
		if m.emails[i].ID == targetID {
			return extractSender(&m.emails[i])
		}
	}
	for i := range m.rawEmails {
		if m.rawEmails[i].ID == targetID {
			return extractSender(&m.rawEmails[i])
		}
	}
	if cfg := m.Config(); cfg != nil {
		if cached, err := msg.CachedEmail(cfg.AccountCache, targetID); err == nil && cached != nil {
			return extractSender(cached)
		}
	}
	return "", ""
}

func extractSender(e *email.Email) (string, string) {
	name, addr := email.ParseSenderInfo(e.FromRaw)
	if addr == "" {
		addr = email.ParseEmailAddress(e.FromEmail)
	}
	if addr == "" {
		addr = email.ParseEmailAddress(e.FromRaw)
	}
	return name, addr
}

func renderBlockDialog(m *tuiModel) string {
	dialogWidth := 54
	dialogHeight := 9

	var content strings.Builder
	content.WriteString("\n")
	title := "BLOCK SENDER"
	content.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#ff5555")).Render("               "+title+"               ") + "\n\n")

	senderDesc := m.blockSenderEmail
	if m.blockSenderName != "" && m.blockSenderName != m.blockSenderEmail {
		senderDesc = fmt.Sprintf("%s <%s>", m.blockSenderName, m.blockSenderEmail)
	}
	if len(senderDesc) > 46 {
		senderDesc = senderDesc[:43] + "..."
	}

	content.WriteString(fmt.Sprintf("  Block %s?\n", senderDesc))
	content.WriteString("  Choose destination for this and future mail:\n\n")
	content.WriteString(
		lipgloss.NewStyle().Foreground(lipgloss.Color("#ffaa00")).Bold(true).Render(markHints("  [s] Spam", hint{"[s] Spam", "s"})) + "   " +
			lipgloss.NewStyle().Foreground(lipgloss.Color("#ff5555")).Bold(true).Render(markHints("  [t] Trash", hint{"[t] Trash", "t"})) + "   " +
			lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")).Render(markHints("  [c/Esc] Cancel", hint{"[c/Esc] Cancel", "esc"})),
	)

	dialogBox := lipgloss.NewStyle().
		Background(lipgloss.Color("#16162a")).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#ff5555")).
		Padding(1, 2).
		Width(dialogWidth).
		Height(dialogHeight).
		Render(content.String())

	return renderDimmedOverlay(m.width, m.height, m.renderBaseScreen(), dialogBox)
}

func (m *tuiModel) kBlockModal(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := k.String()
	switch s {
	case "s", "S":
		return m.executeBlock(blockDestSpam)
	case "t", "T":
		return m.executeBlock(blockDestTrash)
	case "esc", "c", "C", "q", "Q", "n", "N":
		m.blockModalOpen = false
		return m, nil
	}
	return m, nil
}

func (m *tuiModel) executeBlock(dest BlockDestination) (tea.Model, tea.Cmd) {
	m.blockModalOpen = false
	targetID := m.blockMessageID
	senderEmail := m.blockSenderEmail
	if targetID == "" || senderEmail == "" {
		return m, nil
	}

	m.expandWholeThreadForEmail(targetID)
	m.removeEmailByID(targetID)
	if dest == blockDestSpam {
		m.addPending(targetID, pendingSpam)
		m.saveStatus = fmt.Sprintf("Blocked %s (Spam)", senderEmail)
	} else {
		m.addPending(targetID, pendingDelete)
		m.saveStatus = fmt.Sprintf("Blocked %s (Trash)", senderEmail)
	}
	m.saveStatusTime = time.Now()
	m.mode = modeIndex

	return m, m.cmdBlockSender(targetID, senderEmail, dest, m.currentF)
}

func (m *tuiModel) cmdBlockSender(id email.ID, senderEmail string, dest BlockDestination, srcFolder string) tea.Cmd {
	return func() tea.Msg {
		cfg := m.client.Config()
		if cfg == nil {
			return blockDoneMsg{id: id, senderEmail: senderEmail, dest: dest, err: errors.New("missing config")}
		}

		trashFolder := resolveBlockTrashFolder(m.client)
		if err := applyBlockToConfig(cfg, senderEmail, dest, trashFolder.String()); err != nil {
			slog.Warn("cmdBlockSender: failed to update config", slog.String("email", senderEmail), slog.Any("error", err))
		}

		if err := m.client.ExportRules(); err != nil {
			slog.Warn("cmdBlockSender: ExportRules failed", slog.Any("error", err))
		}

		if dest == blockDestSpam {
			if ok, err := msg.Exists(cfg.AccountCache, id); err == nil && ok {
				if err := spam.MarkByID(report.Or(m.rep), m.client, cfg, string(id)); err != nil {
					slog.Warn("cmdBlockSender: MarkByID failed", slog.String("id", string(id)), slog.Any("error", err))
				}
			}
		} else {
			if err := m.client.MoveEmail([]email.ID{id}, label.FromServer(srcFolder), trashFolder); err != nil {
				slog.Warn("cmdBlockSender: MoveEmail to trash failed", slog.String("id", string(id)), slog.Any("error", err))
			}
		}

		return blockDoneMsg{id: id, senderEmail: senderEmail, dest: dest}
	}
}

func resolveBlockTrashFolder(client mailclient.MailClient) label.Folder {
	if client != nil {
		if tf, err := organize.ResolveTrashTarget(client); err == nil && !tf.IsZero() {
			return tf
		}
	}
	return label.FromServer("Trash")
}

func applyBlockToConfig(cfg *cfg_g.Config, senderEmail string, dest BlockDestination, trashTarget string) error {
	resolved, err := cfg_g.Resolve(cfg)
	if err != nil {
		return err
	}
	emailLower := strings.ToLower(strings.TrimSpace(senderEmail))

	if dest == blockDestSpam {
		applyBlockSpam(resolved.Account, emailLower)
	} else {
		applyBlockTrash(resolved.Account, emailLower, trashTarget)
	}

	if cfg.IsSelected(*resolved.Account) {
		cfg.Blacklist = resolved.Account.Blacklist
		cfg.Rules = resolved.Account.Rules
	}

	// The contacts history lives at the cache root, not in the account's
	// directory -- where this used to look, so a blocked sender stayed in it.
	if cfg.CacheRoot != "" {
		_ = contacts.PurgeAddress(filepath.Join(cfg.CacheRoot, "suggested_contacts.json"), emailLower)
	}

	return cfg_g.SaveConfigFile(resolved.Path, resolved.File)
}

func applyBlockSpam(acc *cfg_acc.AccountConfig, emailLower string) {
	found := false
	for _, b := range acc.Blacklist {
		if strings.EqualFold(b, emailLower) {
			found = true
			break
		}
	}
	if !found {
		acc.Blacklist = append(acc.Blacklist, emailLower)
	}

	spamTarget := acc.SpamLearn
	if spamTarget == "" {
		spamTarget = acc.SpamFolder
	}
	if spamTarget == "" {
		spamTarget = "Spam"
	}

	for i, r := range acc.Rules {
		if strings.EqualFold(r.Sender, emailLower) {
			acc.Rules[i].Label = spamTarget
			acc.Rules[i].Exported = false
			return
		}
	}
	acc.Rules = append(acc.Rules, cfg_acc.Rule{
		Sender:   emailLower,
		Label:    spamTarget,
		Exported: false,
	})
}

func applyBlockTrash(acc *cfg_acc.AccountConfig, emailLower string, trashTarget string) {
	var newBlist []string
	for _, b := range acc.Blacklist {
		if !strings.EqualFold(b, emailLower) {
			newBlist = append(newBlist, b)
		}
	}
	acc.Blacklist = newBlist

	if trashTarget == "" {
		trashTarget = "Trash"
	}

	for i, r := range acc.Rules {
		if strings.EqualFold(r.Sender, emailLower) {
			acc.Rules[i].Label = trashTarget
			acc.Rules[i].Exported = false
			return
		}
	}
	acc.Rules = append(acc.Rules, cfg_acc.Rule{
		Sender:   emailLower,
		Label:    trashTarget,
		Exported: false,
	})
}
