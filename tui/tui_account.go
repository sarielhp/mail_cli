package tui

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"

	"github.com/sarielhp/mail_cli/cache"
	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/label"
	"github.com/sarielhp/mail_cli/mailclient"
	"github.com/sarielhp/mail_cli/tui/render"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func (m *tuiModel) getAccounts() []cfg_acc.AccountConfig {
	if m.Config() != nil && len(m.Config().Accounts) > 0 {
		return m.Config().Accounts
	}
	if m.cfg != nil && len(m.cfg.Accounts) > 0 {
		return m.cfg.Accounts
	}
	return nil
}

func (m *tuiModel) currentAccountIndex() int {
	accounts := m.getAccounts()
	if len(accounts) == 0 {
		return 0
	}
	cfg := m.Config()
	if cfg == nil {
		cfg = m.cfg
	}
	for i, a := range accounts {
		if cfg.IsSelected(a) {
			return i
		}
	}
	return 0
}

// currentAccountLabel names the current account for a message, or "This
// account" when there is none to name.
func (m *tuiModel) currentAccountLabel() string {
	accounts := m.getAccounts()
	if idx := m.currentAccountIndex(); idx >= 0 && idx < len(accounts) {
		return accounts[idx].GetDisplayName()
	}
	return "This account"
}

func (m *tuiModel) openAccountOverlay() {
	accounts := m.getAccounts()
	if len(accounts) == 0 {
		return
	}
	m.accountOverlayOpen = true
	m.treeOpen = false
	m.showHelp = false
	m.showDiag = false
	m.showError = false
	m.accountCursor = m.activeAccountIndex()
}

// activeAccountIndex is the account the overlay should mark and start on:
// the sending account while composing, the browsed one otherwise.
func (m *tuiModel) activeAccountIndex() int {
	if m.mode == modeCompose && m.compose != nil {
		return m.compose.AccountIndex
	}
	return m.currentAccountIndex()
}

func (m *tuiModel) switchAccountByIndex(idx int) (tea.Model, tea.Cmd) {
	accounts := m.getAccounts()
	if len(accounts) == 0 {
		m.accountOverlayOpen = false
		return m, nil
	}
	if idx < 0 || idx >= len(accounts) {
		m.accountOverlayOpen = false
		return m, nil
	}
	m.accountOverlayOpen = false
	if m.mode == modeCompose && m.compose != nil {
		return m.switchComposeAccountTo(idx)
	}
	return m.switchAccount(accounts[idx])
}

func (m *tuiModel) resolveClientForSwitch(acc cfg_acc.AccountConfig) (mailclient.MailClient, error) {
	var cfgToUse *cfg_g.Config
	if m.cfg != nil {
		cfgToUse = m.cfg
	} else if m.Config() != nil {
		cfgToUse = m.Config()
	}
	if m.bk == nil || cfgToUse == nil {
		return nil, nil
	}
	return m.bk.clients().ForAccount(cfgToUse, acc)
}

func (m *tuiModel) initReadStateForAccount() {
	if m.client != nil && m.client.Config() != nil {
		m.isRead = cache.LoadReadState(m.client.Config().AccountCache)
		m.isReplied = cache.LoadRepliedState(m.client.Config().AccountCache)
	}
	if m.isRead == nil {
		m.isRead = make(map[email.ID]bool)
	}
	if m.isReplied == nil {
		m.isReplied = make(map[email.ID]bool)
	}
}

func (m *tuiModel) resetStateForAccountSwitch() {
	m.pendingEmails = make(map[email.ID]pendingOpType)
	m.expandedThreads = make(map[threadKey]bool)
	m.rawEmails = nil
	m.emails = nil
	m.eIdx = 0
	m.selectedID = ""
	m.indexStart = 0
	m.mode = modeIndex
	m.detail = ""
	m.detailH = ""
	m.detailCrypto = nil
	m.attachments = nil
	m.attachmentDrawer = false
	m.searchQuery = ""
	m.inSearch = false
	m.folderSearch = ""
	m.inFolderSearch = false
	m.treeOpen = false
	m.accountOverlayOpen = false
	m.showHelp = false
	m.showError = false
	m.err = nil
	m.showDiag = false
	m.menuOpen = false
}

func (m *tuiModel) loadAccountFolderCache() {
	// Through the store, not around it. This used to open
	// <account cache>/labels_cache.json and unmarshal it by hand -- the same file
	// name and the same format as DiskCacheStore.GetLabelItems, written out a
	// second time. Two statements of one fact drift, and this one would have
	// drifted silently: a changed file name or format leaves the TUI reading
	// nothing and showing an empty folder list, with no error to explain it.
	var cachedFolders []label.Item
	if m.client != nil && m.client.Config() != nil {
		if dir := m.client.Config().AccountCache; !dir.IsZero() {
			store := &cache.DiskCacheStore{Dir: dir}
			if items, err := store.GetLabelItems(); err == nil {
				cachedFolders = items
			}
		}
	}

	targetFolder := "INBOX"
	if m.client != nil {
		targetFolder = m.client.InboxFolder().String()
	}

	if len(cachedFolders) == 0 {
		m.globalFolders = nil
		m.globalTree = nil
		m.folders = nil
		m.tree = nil
		m.entries = nil
		m.currentF = targetFolder
		return
	}

	m.globalFolders = cachedFolders
	m.globalTree = label.BuildLabelTree(cachedFolders)
	m.folders = cachedFolders
	m.tree = label.BuildLabelTree(cachedFolders)
	m.buildEntries()
	m.currentF = label.ResolveFolder(cachedFolders, targetFolder, "INBOX")
}

func (m *tuiModel) switchAccount(acc cfg_acc.AccountConfig) (tea.Model, tea.Cmd) {
	slog.Info("switchAccount called", slog.String("accountName", acc.Name), slog.String("displayName", acc.GetDisplayName()))

	if m.Config() != nil {
		m.Config().SelectedAccount = cfg_g.NameOf(acc)
	}
	if m.cfg != nil {
		m.cfg.SelectedAccount = cfg_g.NameOf(acc)
	}

	newClient, err := m.resolveClientForSwitch(acc)
	if err != nil {
		m.err = err
		m.showError = true
		return m, nil
	}

	if newClient != nil {
		m.client = newClient
	}

	m.initReadStateForAccount()
	m.resetStateForAccountSwitch()
	m.loadAccountFolderCache()
	m.loadCachedEmailsForFolder(m.currentF)

	var cmds []tea.Cmd
	if m.client != nil {
		cmds = append(cmds, m.bgFetchFoldersCmd())
		if m.currentF != "" {
			if len(m.emails) == 0 {
				m.showLoad = "Downloading..."
			} else {
				m.showLoad = ""
			}
			cmds = append(cmds, m.loadEmailsCmd(m.currentF))
		}
	}
	cmds = append(cmds, tea.ClearScreen)

	return m, tea.Batch(cmds...)
}

func (m *tuiModel) kAccountOverlay(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	accounts := m.getAccounts()
	s := k.String()
	switch {
	case s == "esc" || s == "q" || s == "a" || s == "A":
		m.accountOverlayOpen = false
		return m, nil
	case s == "up" || s == "k":
		if len(accounts) > 0 {
			m.accountCursor--
			if m.accountCursor < 0 {
				m.accountCursor = len(accounts) - 1
			}
		}
	case s == "down" || s == "j" || s == "tab":
		if len(accounts) > 0 {
			m.accountCursor++
			if m.accountCursor >= len(accounts) {
				m.accountCursor = 0
			}
		}
	case s == "enter" || s == " ":
		return m.switchAccountByIndex(m.accountCursor)
	case s == "l" || s == "L":
		if m.accountCursor >= 0 && m.accountCursor < len(accounts) {
			targetAcc := accounts[m.accountCursor]
			return m, m.loginAccountCmd(targetAcc)
		}
	case len(s) == 1 && s[0] >= '1' && s[0] <= '9':
		num := int(s[0] - '0')
		if num-1 < len(accounts) {
			return m.switchAccountByIndex(num - 1)
		}
	}
	return m, nil
}

type accountLoginFinishedMsg struct {
	acc cfg_acc.AccountConfig
	err error
}

func (m *tuiModel) loginAccountCmd(acc cfg_acc.AccountConfig) tea.Cmd {
	bin, err := os.Executable()
	if err != nil || bin == "" {
		bin = os.Args[0]
	}
	c := exec.Command(bin, "account", "login", acc.Name)
	return tea.ExecProcess(c, func(execErr error) tea.Msg {
		return accountLoginFinishedMsg{
			acc: acc,
			err: execErr,
		}
	})
}

func renderAccountOverlay(m *tuiModel) string {
	t := m.theme.Theme()
	accounts := m.getAccounts()
	curIdx := m.activeAccountIndex()
	title := "       SWITCH ACCOUNT"
	if m.mode == modeCompose && m.compose != nil {
		title = "       SEND FROM ACCOUNT"
	}

	var lines []string
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(t.Get(render.ColorHighlightFg)))

	lines = append(lines, headerStyle.Render(title))
	lines = append(lines, "")

	if len(accounts) == 0 {
		lines = append(lines, "  No configured accounts found.")
	} else {
		for i, acc := range accounts {
			numStr := fmt.Sprintf("[%d]", i+1)
			name := acc.GetDisplayName()
			accType := strings.ToUpper(acc.Type)
			if accType == "" {
				accType = "MAIL"
			}
			user := acc.Username

			var prefix string
			var rowStyle lipgloss.Style
			if i == m.accountCursor {
				prefix = "> "
				rowStyle = lipgloss.NewStyle().
					Bold(true).
					Foreground(lipgloss.Color(t.Get(render.ColorHighlightFg)))
			} else {
				prefix = "  "
				rowStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color(t.Get(render.ColorFg)))
			}

			typeBadge := lipgloss.NewStyle().
				Foreground(lipgloss.Color(t.Get(render.ColorDate))).
				Render(fmt.Sprintf("[%s]", accType))

			roBadge := ""
			if acc.ReadOnly || (m.Config() != nil && m.Config().ReadOnly) || (m.cfg != nil && m.cfg.ReadOnly) {
				roBadge = " " + lipgloss.NewStyle().
					Foreground(lipgloss.Color(t.Get(render.ColorDate))).
					Render("[RO]")
			}

			activeBullet := ""
			if i == curIdx {
				activeBullet = " " + lipgloss.NewStyle().
					Foreground(lipgloss.Color(t.Get(render.ColorHighlightFg))).
					Render("●")
			}

			var desc string
			if user != "" && user != name {
				desc = fmt.Sprintf("%-4s %-14s %s%s  (%s)%s", numStr, name, typeBadge, roBadge, user, activeBullet)
			} else {
				desc = fmt.Sprintf("%-4s %-14s %s%s%s", numStr, name, typeBadge, roBadge, activeBullet)
			}

			row := prefix + rowStyle.Render(desc)
			lines = append(lines, rowZone(frameAccount, i, 0, row))
		}
	}

	lines = append(lines, "")
	dialogWidth := min(72, max(48, m.width-4))
	innerWidth := dialogWidth - 4 // accounting for Padding(1, 2)

	helpLeft := "Enter/1-9: Select  │  l: Login  │  j/k: Nav  │  Esc: Cancel"
	helpRight := "●=active"
	if innerWidth < len(helpLeft)+len(helpRight)+2 {
		helpLeft = "1-9: Select  │  l: Login  │  Esc: Cancel"
	}
	if innerWidth < len(helpLeft)+len(helpRight)+2 {
		helpLeft = "Enter: Select  │  l: Login  │  Esc: Cancel"
	}

	var footer string
	if gap := innerWidth - len(helpLeft) - len(helpRight); gap > 0 {
		footer = helpLeft + strings.Repeat(" ", gap) + helpRight
	} else {
		footer = helpLeft + "  " + helpRight
	}

	helpStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(t.Get(render.ColorDate)))
	lines = append(lines, helpStyle.Render(markHints(footer,
		hint{"Enter/1-9: Select", "enter"}, hint{"Enter: Select", "enter"}, hint{"l: Login", "l"}, hint{"Esc: Cancel", "esc"})))

	content := strings.Join(lines, "\n")
	dialog := lipgloss.NewStyle().
		Width(dialogWidth).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(t.Get(render.ColorBorder))).
		Padding(1, 2).
		Render(content)

	return lipgloss.NewStyle().
		Width(m.width).
		Height(max(10, m.height)).
		Align(lipgloss.Center, lipgloss.Center).
		Render(dialog)
}
