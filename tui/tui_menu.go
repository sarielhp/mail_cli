package tui

import (
	"fmt"
	"mime"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/sarielhp/mail_cli/app"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/filebrowser"
	"github.com/sarielhp/mail_cli/tui/render"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func (m *tuiModel) handleAttachmentSave(att email.Attachment) {
	savePattern := ""
	if m.Config() != nil {
		savePattern = m.Config().AttachmentSaveDir
	} else if m.cfg != nil {
		savePattern = m.cfg.AttachmentSaveDir
	}

	destDir, err := cfg_g.ExpandSavePath(savePattern, time.Now())
	if err != nil {
		m.saveStatus = "Error resolving path: " + err.Error()
		m.saveStatusTime = time.Now()
		return
	}
	if err := os.MkdirAll(destDir, 0755); err != nil {
		m.saveStatus = "Error creating dir: " + err.Error()
		m.saveStatusTime = time.Now()
		return
	}
	destPath := filepath.Join(destDir, att.Filename)
	if err := os.WriteFile(destPath, att.Data, 0644); err == nil {
		m.saveStatus = "Saved to " + destPath
	} else {
		m.saveStatus = "Error saving: " + err.Error()
	}
	m.saveStatusTime = time.Now()
}

func (m *tuiModel) handleAttachmentCalendarAction(att email.Attachment) {
	if !email.IsICSAttachment(att) {
		m.saveStatus = "Not a calendar (.ics) attachment"
		m.saveStatusTime = time.Now()
		return
	}
	if !m.bk.hasCalendar() {
		m.saveStatus = "Calendar service not configured"
		m.saveStatusTime = time.Now()
		return
	}
	if verifier, ok := m.bk.verifyingCalendar(); ok {
		summary, existed, verified, noDups, err := verifier.AddICSVerified(m.Config(), att.Data)
		switch {
		case err != nil:
			m.saveStatus = "Calendar error: " + err.Error()
			m.icsCheckStatus = icsStatusError
			m.icsCheckErr = err.Error()
			m.icsAddStatus = "Error: " + err.Error()
		case existed && verified:
			m.saveStatus = "Updated in calendar: " + summary
			m.icsCheckStatus = icsStatusUpdatedVerified
			m.icsInCalendar = true
			m.icsAddStatus = "✓ Event updated and verified in Calendar"
		case existed:
			m.saveStatus = "Already in calendar: " + summary
			m.icsCheckStatus = icsStatusInCalendar
			m.icsInCalendar = true
			m.icsAddStatus = "Already present in Google Calendar"
		case verified && noDups:
			m.saveStatus = "Added & verified (no duplicates): " + summary
			m.icsCheckStatus = icsStatusAddedVerified
			m.icsInCalendar = true
			m.icsAddStatus = "✓ Event added and verified in Calendar (no duplicates)"
		case verified:
			m.saveStatus = "Added (warning: duplicate entries): " + summary
			m.icsCheckStatus = icsStatusAddedWithDups
			m.icsInCalendar = true
			m.icsAddStatus = "⚠ Event added, but duplicate entries detected in Calendar"
		default:
			m.saveStatus = "Added to calendar: " + summary
			m.icsCheckStatus = icsStatusAddedVerified
			m.icsInCalendar = true
			m.icsAddStatus = "✓ Event added to Calendar"
		}
		m.detailVpDirty = true
	} else {
		summary, existed, err := m.bk.Calendar.AddICS(m.Config(), att.Data)
		switch {
		case err != nil:
			m.saveStatus = "Calendar error: " + err.Error()
		case existed:
			m.saveStatus = "Updated in calendar: " + summary
			m.icsCheckStatus = icsStatusUpdatedVerified
			m.icsInCalendar = true
			m.detailVpDirty = true
		default:
			m.saveStatus = "Added to calendar: " + summary
			m.icsCheckStatus = icsStatusAddedVerified
			m.icsInCalendar = true
			m.detailVpDirty = true
		}
	}
	m.saveStatusTime = time.Now()
}

func (m *tuiModel) kAttachmentDrawer(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := k.String()
	if len(m.attachments) == 0 {
		m.attachmentDrawer = false
		return m, nil
	}
	// A deletion in the separate attachment-list overlay
	// (applyAttachmentDeletions) shrinks m.attachments without touching
	// attCursor, which updateAttachmentFocusOnScroll can independently have
	// left pointing at an attachment near the end of the *previous*, longer
	// list. Clamp once here so every case below that indexes
	// m.attachments[m.attCursor] is safe, instead of bounds-checking each
	// one separately the way tui_keys_rightpane.go and
	// tui_attachment_modal.go already have to.
	if m.attCursor < 0 || m.attCursor >= len(m.attachments) {
		m.attCursor = len(m.attachments) - 1
	}
	switch s {
	case "f1":
		m.attachmentHelpOpen = !m.attachmentHelpOpen
		return m, nil
	case "tab", "l", "right":
		if m.width >= 100 && len(m.attachments) > 0 {
			m.rightPaneFocus = true
			return m, nil
		}
	case "a", "esc", "q":
		m.attachmentDrawer = false
		m.saveStatus = ""
		m.detailVpDirty = true
		return m, nil
	case "up", "k":
		m.vp.ScrollUp(1)
		m.updateAttachmentFocusOnScroll()
		return m, nil
	case "down", "j":
		m.vp.ScrollDown(1)
		m.updateAttachmentFocusOnScroll()
		return m, nil
	case "i":
		m.toggleInvertColors()
		return m, nil
	case "enter", "o", "x":
		att := m.attachments[m.attCursor]
		if email.IsICSAttachment(att) && s == "enter" {
			return m.openICSViewer(att)
		}
		tmpFile, err := writeTempAttachment(att)
		if err == nil {
			go openSystemFile(tmpFile)
			m.saveStatus = "Opened: " + att.Filename
		} else {
			m.saveStatus = "Error: " + err.Error()
		}
		m.saveStatusTime = time.Now()
		return m, nil
	case "v":
		att := m.attachments[m.attCursor]
		if email.IsICSAttachment(att) {
			return m.openICSViewer(att)
		}
		return m.openAttachmentViewer(att)
	case "s":
		m.handleAttachmentSave(m.attachments[m.attCursor])
	case "S", "b", "B":
		att := m.attachments[m.attCursor]
		savePattern := ""
		if m.Config() != nil {
			savePattern = m.Config().AttachmentSaveDir
		} else if m.cfg != nil {
			savePattern = m.cfg.AttachmentSaveDir
		}
		initialDir, _ := cfg_g.ExpandSavePath(savePattern, time.Now())
		m.fileBrowser = filebrowser.New(initialDir, att.Filename, m.width, m.height)
		m.fileBrowserOpen = true
		m.pendingSaveAtt = att
		m.saveStatus = ""
	case "c", "C", "u", "U":
		m.handleAttachmentCalendarAction(m.attachments[m.attCursor])
	}
	return m, nil
}

func (m *tuiModel) kFileBrowser(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	newFb, cmd := m.fileBrowser.Update(k)
	return m.afterFileBrowser(newFb, cmd)
}

// afterFileBrowser takes the browser's new state after a key or a click, and
// acts on a choice or a cancel.
func (m *tuiModel) afterFileBrowser(newFb filebrowser.Model, cmd tea.Cmd) (tea.Model, tea.Cmd) {
	m.fileBrowser = newFb

	if m.fileBrowser.Canceled {
		m.fileBrowserOpen = false
		m.pendingSaveStem = ""
		return m, nil
	}

	if !m.fileBrowser.Done {
		return m, cmd
	}

	m.fileBrowserOpen = false
	if m.mode == modeCompose && m.compose != nil && m.fileBrowser.AllowFileSelect {
		return m, m.attachBrowserFile(cmd)
	}

	return m, m.savePendingAttachment()
}

func (m *tuiModel) attachBrowserFile(cmd tea.Cmd) tea.Cmd {
	if cmd == nil {
		return nil
	}
	selMsg, ok := cmd().(filebrowser.SelectedMsg)
	if !ok || selMsg.FullPath == "" {
		return nil
	}
	data, err := os.ReadFile(selMsg.FullPath)
	if err != nil {
		return nil
	}
	fn := filepath.Base(selMsg.FullPath)
	ct := mime.TypeByExtension(filepath.Ext(fn))
	if ct == "" {
		ct = "application/octet-stream"
	}
	m.compose.Attachments = append(m.compose.Attachments, email.Attachment{
		Filename:    fn,
		ContentType: ct,
		Data:        data,
	})
	return m.saveAndSyncDraftCmd()
}

// savePendingAttachment writes what the save dialog was opened for -- an
// attachment, or a whole message from F2 -- into the directory chosen. It
// never overwrites: an existing name gets a -2, -3 suffix. The result shows
// in the attachment drawer when that is open, and as a notice otherwise.
func (m *tuiModel) savePendingAttachment() tea.Cmd {
	name := m.pendingSaveAtt.Filename
	if m.pendingSaveStem != "" {
		name = messageFileName(m.fileBrowser.SelectedDir, m.pendingSaveStem)
	}
	m.pendingSaveStem = ""
	destPath, err := writeNewFile(m.fileBrowser.SelectedDir, name, m.pendingSaveAtt.Data)
	m.saveStatusTime = time.Now()
	if err != nil {
		m.saveStatus = "Error saving: " + err.Error()
		if !m.attachmentDrawer {
			m.err = fmt.Errorf("saving %s: %w", m.pendingSaveAtt.Filename, err)
			m.showError = true
		}
		return nil
	}
	m.saveStatus = "Saved to " + destPath
	if m.attachmentDrawer {
		return nil
	}
	return m.showNotice("Saved to " + destPath)
}

func renderAttachmentHelp(m *tuiModel) string {
	t := m.theme.Theme()
	borderColor := t.Get(render.ColorBorder)
	helpBg := lipgloss.Color("#1a1a35")

	leftCol := []string{
		"  Enter / v    Preview attachment in popup window (.ics / text / media)",
		"  o / x        Open in default system application",
		"  s            Quick save to default directory",
		"  S / b        Open File Browser to choose directory",
		"  c            Add event to Google Calendar (.ics)",
		"  j / ↓        Next attachment",
		"  k / ↑        Previous attachment",
		"  F1           Toggle this help",
		"  Esc / q / a  Close attachment drawer",
	}

	savePattern := "(default: ~/Downloads)"
	if m.Config() != nil && m.Config().AttachmentSaveDir != "" {
		savePattern = m.Config().AttachmentSaveDir
	} else if m.cfg != nil && m.cfg.AttachmentSaveDir != "" {
		savePattern = m.cfg.AttachmentSaveDir
	}

	var lines []string
	lines = append(lines, lipgloss.NewStyle().Background(helpBg).Render("  📎 ATTACHMENT DRAWER HELP  "))
	lines = append(lines, "")
	lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color("#ffd700")).Background(helpBg).Render(fmt.Sprintf("  Save Directory Pattern: %s", savePattern)))
	lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color("#8888aa")).Background(helpBg).Render("  Supported Pattern Tags: %YYYY, %YY, %MM, %DD, %hh, %mm"))
	lines = append(lines, "")

	for _, s := range leftCol {
		lines = append(lines, lipgloss.NewStyle().Background(helpBg).Render(s))
	}

	lines = append(lines, "")
	lines = append(lines, lipgloss.NewStyle().Background(helpBg).Render(markHints("  Press F1 or Esc to dismiss  ", hint{"Press F1 or Esc to dismiss", "esc"})))

	content := strings.Join(lines, "\n")

	return lipgloss.NewStyle().
		Background(helpBg).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color(borderColor)).
		Padding(0, 2).
		Render(content)
}

func writeTempAttachment(att email.Attachment) (string, error) {
	tmpDir, err := app.GetTempDir()
	if err != nil {
		return "", err
	}
	tmpPath := filepath.Join(tmpDir, att.Filename)
	err = os.WriteFile(tmpPath, att.Data, 0600)
	return tmpPath, err
}

// openSystemFile hands a file to the desktop's default opener. It is a
// variable so the test suite can intercept it: a drawer test that presses
// "o" on a text attachment used to reach the real opener, which on one
// machine meant an emacs window popping up on every test run.
var openSystemFile = launchSystemOpener

func launchSystemOpener(path string) {
	switch runtime.GOOS {
	case "darwin":
		_ = exec.Command("open", path).Start()
	case "windows":
		_ = buildWindowsOpenFileCmd(path).Start()
	default:
		for _, launcher := range []string{"xdg-open", "gio", "gnome-open", "kde-open", "open"} {
			if p, err := exec.LookPath(launcher); err == nil {
				if launcher == "gio" {
					_ = exec.Command(p, "open", path).Start()
				} else {
					_ = exec.Command(p, path).Start()
				}
				return
			}
		}
		_ = exec.Command("xdg-open", path).Start()
	}
}

func buildWindowsOpenFileCmd(path string) *exec.Cmd {
	return exec.Command("rundll32", "url.dll,FileProtocolHandler", path)
}

func (m *tuiModel) kMenu(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := k.String()
	switch s {
	case "up", "k":
		m.menuCursor--
		if m.menuCursor < 0 {
			m.menuCursor = len(m.menuItems) - 1
		}
	case "down", "j":
		m.menuCursor++
		if m.menuCursor >= len(m.menuItems) {
			m.menuCursor = 0
		}
	case "enter":
		m.menuOpen = false
		return m.handleMenuSelection()
	case "esc", "q", "m", "alt", "alt+m":
		m.menuOpen = false
	default:
		if k.Alt {
			m.menuOpen = false
		}
	}
	return m, nil
}

func (m *tuiModel) handleMenuSelection() (tea.Model, tea.Cmd) {
	if m.menuCursor >= len(m.menuItems) {
		return m, nil
	}
	switch m.menuItems[m.menuCursor] {
	case "Move to Spam":
		targetID := m.resolveMenuTargetID()
		if targetID != "" {
			plan := m.planAction([]email.ID{targetID}, m.currentF)
			m.removeEmailByID(targetID)
			m.addPending(targetID, pendingSpam)
			m.mode = modeIndex
			return m, m.cmdSpam(targetID, plan)
		}
	case "Switch Account":
		m.openAccountOverlay()
		return m, nil
	case "Exit":
		m.done = true
		return m, tea.Quit
	}
	return m, nil
}

func (m *tuiModel) resolveMenuTargetID() email.ID {
	if m.mode == modeIndex && m.eIdx >= 0 && m.eIdx < len(m.emails) {
		return m.emails[m.eIdx].ID
	}
	if m.mode == modeDetail {
		return m.selectedID
	}
	return ""
}
