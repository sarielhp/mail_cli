package tui

import (
	"bytes"
	"errors"
	"fmt"
	"mime"
	"net/mail"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/filebrowser"
)

// noticeTTL is how long a notice stays on screen.
const noticeTTL = 4 * time.Second

// messageReadyToSaveMsg carries a message F2 fetched, to be saved where the
// user chooses.
type messageReadyToSaveMsg struct {
	stem string
	raw  []byte
	err  error
}

// noticeExpiredMsg redraws the screen once a notice has timed out.
type noticeExpiredMsg struct{}

// saveDirPattern is the configured attachment save directory.
func (m *tuiModel) saveDirPattern() string {
	if cfg := m.Config(); cfg != nil {
		return cfg.AttachmentSaveDir
	}
	if m.cfg != nil {
		return m.cfg.AttachmentSaveDir
	}
	return ""
}

// saveCurrentMessage fetches the selected message for F2 -- in the
// background, because it may have to be downloaded first -- and then asks
// where to save it.
//
// Writing a file the user asked for is not a server mutation, so this is
// allowed under --read-only.
func (m *tuiModel) saveCurrentMessage() (tea.Model, tea.Cmd) {
	id := m.currentEmailID()
	if id == "" || m.client == nil {
		return m, nil
	}
	client := m.client
	return m, func() tea.Msg {
		raw, err := client.GetRawMessage(id)
		if err != nil {
			return messageReadyToSaveMsg{err: fmt.Errorf("could not retrieve the message: %w", err)}
		}
		return messageReadyToSaveMsg{stem: messageFileStem(raw), raw: raw}
	}
}

// handleMessageReadyToSave opens the save dialog -- the one the attachment
// drawer uses -- in the directory the program was started from. F2 used to
// write straight into the attachment save directory, which from inside the
// TUI looked like a file going somewhere arbitrary.
func (m *tuiModel) handleMessageReadyToSave(msg messageReadyToSaveMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.err = fmt.Errorf("saving message: %w", msg.err)
		m.showError = true
		return m, nil
	}
	dir, err := os.Getwd()
	if err != nil {
		dir, _ = os.UserHomeDir()
	}
	// The name shown is the one this directory would get; the save
	// recomputes it for whichever directory is finally chosen.
	name := messageFileName(dir, msg.stem)
	m.openSaveDialog(dir, email.Attachment{Filename: name, Data: msg.raw})
	m.pendingSaveStem = msg.stem
	return m, nil
}

// openAttachmentSaveDialog opens the save dialog for an attachment, in the
// configured attachment directory.
func (m *tuiModel) openAttachmentSaveDialog(att email.Attachment) {
	dir, _ := cfg_g.ExpandSavePath(m.saveDirPattern(), time.Now())
	m.openSaveDialog(dir, att)
}

// openSaveDialog is the one way the save dialog opens, so no earlier save
// can leave state behind for the next.
func (m *tuiModel) openSaveDialog(dir string, att email.Attachment) {
	m.fileBrowser = filebrowser.New(dir, att.Filename, m.width, m.height)
	m.fileBrowserOpen = true
	m.pendingSaveAtt = att
	m.pendingSaveStem = ""
	m.saveStatus = ""
}

// messageFileName names a saved message for dir: its subject stem, cut to
// email.FileStemLimit when that is unique among the names already there.
func messageFileName(dir, stem string) string {
	var taken []string
	if entries, err := os.ReadDir(dir); err == nil {
		for _, e := range entries {
			taken = append(taken, e.Name())
		}
	}
	return email.ShortStem(stem, email.FileStemLimit, taken) + ".eml"
}

// messageFileStem names the file after the message's subject.
func messageFileStem(raw []byte) string {
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return email.SubjectFileStem("")
	}
	subj := email.DecDecode(new(mime.WordDecoder), msg.Header.Get("Subject"))
	return email.SubjectFileStem(subj)
}

// writeNewFile creates name in dir, or name_2, name_3 and so on before the
// extension when that exists: a save never overwrites an earlier file.
func writeNewFile(dir, name string, data []byte) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("creating %s: %w", dir, err)
	}
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	for n := 1; n < 1000; n++ {
		candidate := name
		if n > 1 {
			candidate = fmt.Sprintf("%s_%d%s", stem, n, ext)
		}
		path := filepath.Join(dir, candidate)
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		_, werr := f.Write(data)
		if cerr := f.Close(); werr == nil {
			werr = cerr
		}
		if werr != nil {
			return "", fmt.Errorf("writing %s: %w", path, werr)
		}
		return path, nil
	}
	return "", fmt.Errorf("too many files named %s in %s", name, dir)
}

// showNotice puts a transient confirmation under the index and detail views.
func (m *tuiModel) showNotice(text string) tea.Cmd {
	m.notice = text
	m.noticeTime = time.Now()
	return tea.Tick(noticeTTL, func(time.Time) tea.Msg { return noticeExpiredMsg{} })
}

// noticeVisible reports whether a notice is still on screen.
func (m *tuiModel) noticeVisible() bool {
	return m.notice != "" && time.Since(m.noticeTime) < noticeTTL
}

func renderNotice(m *tuiModel) string {
	return lipgloss.NewStyle().
		Width(m.width).
		Background(lipgloss.Color("#2e7d32")).
		Foreground(lipgloss.Color("#ffffff")).
		Bold(true).
		Render(" ✔ " + m.notice + " ")
}
