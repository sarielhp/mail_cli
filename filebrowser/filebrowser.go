package filebrowser

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"
)

type entry struct {
	Name  string
	Path  string
	IsDir bool
	Size  int64
}

// Model is the bubbletea model for the file and directory picker: it holds the
// current directory, the visible entries and the cursor, and renders itself as
// a bordered box. Callers drive it with Update and learn the outcome from the
// command it returns or from Done, Canceled and SelectedDir.
type Model struct {
	CurrentDir      string
	Entries         []entry
	Filtered        []entry
	Cursor          int
	ScrollOffset    int
	Width           int
	Height          int
	Title           string
	TargetFileName  string
	ShowHidden      bool
	SearchQuery     string
	InSearch        bool
	EnteringPath    bool
	PathInput       string
	CreatingDir     bool
	NewDirName      string
	Err             error
	SelectedDir     string
	AllowFileSelect bool
	Done            bool
	Canceled        bool
}

// SelectedMsg reports a chosen file or directory to the caller, as the command
// Update returns. FullPath is the picked file, or the target name joined onto
// Dir when a save directory was chosen instead.
type SelectedMsg struct {
	Dir      string
	FullPath string
}

type canceledMsg struct{}

// New builds a picker starting in initialDir -- the user's Downloads folder
// when it is empty -- aimed at targetFileName for saving. A nonexistent
// initialDir falls back to its parent, then to the home directory.
func New(initialDir, targetFileName string, width, height int) Model {
	if initialDir == "" {
		if home, err := os.UserHomeDir(); err == nil {
			initialDir = filepath.Join(home, "Downloads")
		} else {
			initialDir = "."
		}
	}
	initialDir = filepath.Clean(initialDir)
	// Absolute, or there is no going up: filepath.Dir(".") is ".", so the
	// attachment picker, opened on ".", could never leave it.
	if abs, err := filepath.Abs(initialDir); err == nil {
		initialDir = abs
	}
	if _, err := os.Stat(initialDir); os.IsNotExist(err) {
		if parent := filepath.Dir(initialDir); parent != "" {
			if _, pErr := os.Stat(parent); pErr == nil {
				initialDir = parent
			} else if home, err := os.UserHomeDir(); err == nil {
				initialDir = home
			}
		}
	}

	m := Model{
		CurrentDir:     initialDir,
		TargetFileName: targetFileName,
		Width:          width,
		Height:         height,
		Title:          "SELECT SAVE DIRECTORY",
	}
	m.ReloadEntries()
	return m
}

// NewFileSelector creates a filebrowser configured for selecting an existing file.
func NewFileSelector(initialDir string, width, height int) Model {
	if initialDir == "" {
		initialDir = "."
	}
	m := New(initialDir, "", width, height)
	m.Title = "SELECT ATTACHMENT"
	m.AllowFileSelect = true
	return m
}

// ReloadEntries re-reads the current directory into Entries, with ".." first
// and directories before files, both sorted case-insensitively. Hidden entries
// are omitted unless ShowHidden is set; a read error is stored in Err and
// clears the listing.
func (m *Model) ReloadEntries() {
	m.Err = nil
	files, err := os.ReadDir(m.CurrentDir)
	if err != nil {
		m.Err = err
		m.Entries = nil
		m.Filtered = nil
		m.Cursor = 0
		return
	}

	var entries []entry
	// Add ".." entry if not root
	if m.CurrentDir != "/" && m.CurrentDir != filepath.VolumeName(m.CurrentDir)+"\\" {
		entries = append(entries, entry{
			Name:  "..",
			Path:  filepath.Dir(m.CurrentDir),
			IsDir: true,
		})
	}

	var dirs []entry
	var regularFiles []entry

	for _, f := range files {
		name := f.Name()
		if !m.ShowHidden && strings.HasPrefix(name, ".") {
			continue
		}
		info, err := f.Info()
		var size int64
		if err == nil {
			size = info.Size()
		}
		ent := entry{
			Name:  name,
			Path:  filepath.Join(m.CurrentDir, name),
			IsDir: f.IsDir(),
			Size:  size,
		}
		if f.IsDir() {
			dirs = append(dirs, ent)
		} else {
			regularFiles = append(regularFiles, ent)
		}
	}

	sort.Slice(dirs, func(i, j int) bool {
		return strings.ToLower(dirs[i].Name) < strings.ToLower(dirs[j].Name)
	})
	sort.Slice(regularFiles, func(i, j int) bool {
		return strings.ToLower(regularFiles[i].Name) < strings.ToLower(regularFiles[j].Name)
	})

	entries = append(entries, dirs...)
	entries = append(entries, regularFiles...)

	m.Entries = entries
	m.ApplyFilter()
}

// ApplyFilter recomputes Filtered from Entries using SearchQuery as a
// case-insensitive substring match, always keeping "..", and clamps Cursor
// into range.
func (m *Model) ApplyFilter() {
	if m.SearchQuery == "" {
		m.Filtered = m.Entries
	} else {
		var filtered []entry
		q := strings.ToLower(m.SearchQuery)
		for _, e := range m.Entries {
			if e.Name == ".." || strings.Contains(strings.ToLower(e.Name), q) {
				filtered = append(filtered, e)
			}
		}
		m.Filtered = filtered
	}
	if m.Cursor >= len(m.Filtered) {
		m.Cursor = max(0, len(m.Filtered)-1)
	}
}

// Update handles a key message by dispatching to whichever mode the picker is
// in -- naming a directory, entering a path, searching or navigating -- and
// returns the updated model and a command. Any non-key message is ignored.
func (m *Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return *m, nil
	}

	if m.CreatingDir {
		return m.updateCreatingDir(keyMsg)
	}
	if m.EnteringPath {
		return m.updateEnteringPath(keyMsg)
	}
	if m.InSearch {
		return m.updateInSearch(keyMsg)
	}

	return m.updateNavigation(keyMsg)
}

func (m *Model) updateCreatingDir(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.CreatingDir = false
		m.NewDirName = ""
	case tea.KeyEnter:
		dirName := strings.TrimSpace(m.NewDirName)
		if dirName != "" {
			newPath := filepath.Join(m.CurrentDir, dirName)
			if err := os.MkdirAll(newPath, 0755); err != nil {
				m.Err = err
			} else {
				m.CreatingDir = false
				m.NewDirName = ""
				m.CurrentDir = newPath
				m.ReloadEntries()
			}
		}
	case tea.KeyBackspace, tea.KeyCtrlH:
		if len(m.NewDirName) > 0 {
			m.NewDirName = m.NewDirName[:len(m.NewDirName)-1]
		}
	default:
		if len(msg.Runes) > 0 {
			m.NewDirName += string(msg.Runes)
		}
	}
	return *m, nil
}

func (m *Model) updateInSearch(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.InSearch = false
		m.SearchQuery = ""
		m.ApplyFilter()
	case tea.KeyEnter, tea.KeyTab:
		m.InSearch = false
	case tea.KeyBackspace, tea.KeyCtrlH:
		if len(m.SearchQuery) > 0 {
			m.SearchQuery = m.SearchQuery[:len(m.SearchQuery)-1]
			m.ApplyFilter()
		}
	default:
		if len(msg.Runes) > 0 {
			m.SearchQuery += string(msg.Runes)
			m.ApplyFilter()
		}
	}
	return *m, nil
}

func (m *Model) updateEnteringPath(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.EnteringPath = false
		m.PathInput = ""
	case tea.KeyEnter:
		if m.navigateToPath(m.PathInput) {
			m.EnteringPath = false
			m.PathInput = ""
		}
	case tea.KeyBackspace, tea.KeyCtrlH:
		if len(m.PathInput) > 0 {
			m.PathInput = m.PathInput[:len(m.PathInput)-1]
		}
	case tea.KeyTab:
		m.autocompletePath()
	default:
		if len(msg.Runes) > 0 {
			m.PathInput += string(msg.Runes)
		}
	}
	return *m, nil
}

func (m *Model) navigateToPath(rawPath string) bool {
	p := strings.TrimSpace(rawPath)
	if p == "" {
		return false
	}
	if strings.HasPrefix(p, "~") {
		if home, err := os.UserHomeDir(); err == nil {
			p = filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	p = filepath.Clean(p)
	st, err := os.Stat(p)
	if err != nil {
		m.Err = err
		return false
	}
	if !st.IsDir() {
		m.Err = fmt.Errorf("not a directory: %s", p)
		return false
	}
	m.CurrentDir = p
	m.ReloadEntries()
	m.Cursor = 0
	m.ScrollOffset = 0
	return true
}

func (m *Model) jumpToBookmark(num string) {
	home, _ := os.UserHomeDir()
	var target string
	switch num {
	case "1":
		if home != "" {
			target = filepath.Join(home, "Downloads")
		}
	case "2":
		target = home
	case "3":
		if home != "" {
			target = filepath.Join(home, "Documents")
		}
	case "4":
		target = os.TempDir()
	}
	if target != "" {
		m.navigateToPath(target)
	}
}

func (m *Model) autocompletePath() {
	raw := m.PathInput
	expanded := raw
	if strings.HasPrefix(raw, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return
		}
		expanded = filepath.Join(home, strings.TrimPrefix(raw, "~"))
	}
	dir, base := splitPathForCompletion(raw, expanded)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(strings.ToLower(e.Name()), strings.ToLower(base)) {
			continue
		}
		m.PathInput = formatCompletedPath(raw, base, e.Name())
		return
	}
}

func splitPathForCompletion(raw, expanded string) (string, string) {
	if strings.HasSuffix(raw, "/") || raw == "~" {
		return expanded, ""
	}
	return filepath.Dir(expanded), filepath.Base(expanded)
}

func formatCompletedPath(raw, base, matchedDir string) string {
	if strings.HasSuffix(raw, "/") || base == "" {
		return raw + matchedDir + "/"
	}
	idx := strings.LastIndex(raw, "/")
	if idx != -1 {
		return raw[:idx+1] + matchedDir + "/"
	}
	return matchedDir + "/"
}

func (m *Model) selectCurrentEntry() (Model, tea.Cmd) {
	if m.Cursor < 0 || m.Cursor >= len(m.Filtered) {
		return *m, nil
	}
	entry := m.Filtered[m.Cursor]
	if entry.Name == ".." {
		m.goUp()
		return *m, nil
	}
	if entry.IsDir {
		m.CurrentDir = filepath.Clean(entry.Path)
		m.ReloadEntries()
		m.Cursor = 0
		return *m, nil
	}
	if m.AllowFileSelect {
		m.Done = true
		m.SelectedDir = m.CurrentDir
		return *m, func() tea.Msg {
			return SelectedMsg{Dir: m.CurrentDir, FullPath: entry.Path}
		}
	}
	return *m, nil
}

// goUp moves to the parent directory with the cursor on the one just left,
// so that going up and back down is two keys, not a search.
func (m *Model) goUp() {
	child := m.CurrentDir
	parent := filepath.Dir(child)
	if parent == child {
		return // the root
	}
	m.CurrentDir = parent
	m.ReloadEntries()
	m.Cursor = 0
	for i, e := range m.Filtered {
		if e.Path == child {
			m.Cursor = i
			break
		}
	}
}

func (m *Model) updateNavigation(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.Canceled = true
		return *m, func() tea.Msg { return canceledMsg{} }

	case "up", "k":
		m.Cursor--
		if m.Cursor < 0 {
			m.Cursor = 0
		}

	case "down", "j":
		m.Cursor++
		if m.Cursor >= len(m.Filtered) {
			m.Cursor = max(0, len(m.Filtered)-1)
		}

	case "pageup", "pgup":
		m.Cursor -= 10
		if m.Cursor < 0 {
			m.Cursor = 0
		}

	case "pagedown", "pgdn", "pgdown":
		m.Cursor += 10
		if m.Cursor >= len(m.Filtered) {
			m.Cursor = max(0, len(m.Filtered)-1)
		}

	case "home":
		m.Cursor = 0

	case "end":
		m.Cursor = max(0, len(m.Filtered)-1)

	case "enter", "right", "l", " ":
		return m.selectCurrentEntry()

	case "backspace", "left", "h":
		m.goUp()

	// F2 is the TUI's save key, so pressing it again here saves at once.
	case "s", "S", "f2":
		if m.AllowFileSelect && m.Cursor >= 0 && m.Cursor < len(m.Filtered) && !m.Filtered[m.Cursor].IsDir {
			entry := m.Filtered[m.Cursor]
			m.Done = true
			m.SelectedDir = m.CurrentDir
			return *m, func() tea.Msg {
				return SelectedMsg{Dir: m.CurrentDir, FullPath: entry.Path}
			}
		}
		// Save in the current directory!
		m.Done = true
		m.SelectedDir = m.CurrentDir
		fullPath := filepath.Join(m.CurrentDir, m.TargetFileName)
		return *m, func() tea.Msg {
			return SelectedMsg{Dir: m.CurrentDir, FullPath: fullPath}
		}

	case "n", "N":
		m.CreatingDir = true
		m.NewDirName = ""

	case "/":
		m.InSearch = true
		m.SearchQuery = ""

	case ".", "H":
		m.ShowHidden = !m.ShowHidden
		m.ReloadEntries()

	case ":", "g":
		m.EnteringPath = true
		m.PathInput = ""

	case "1", "2", "3", "4":
		m.jumpToBookmark(msg.String())

	case "~":
		if home, err := os.UserHomeDir(); err == nil {
			m.CurrentDir = home
			m.ReloadEntries()
			m.Cursor = 0
		}
	}
	return *m, nil
}

// promptInputLine renders a prompt label followed by the current input value
// with a block cursor, in the picker's input colours.
func promptInputLine(label, value string) string {
	prompt := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#ffdd44")).Render(label)
	val := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#ffffff")).Render(value + "█")
	return prompt + val
}

// View renders the picker as a bordered box sized to the model. It also adjusts
// ScrollOffset so the cursor stays visible, so rendering is not free of side
// effects.
func (m *Model) View() string {
	boxW := min(m.Width-4, 86)
	boxH := min(m.Height-6, 24)
	if boxW < 30 {
		boxW = 30
	}
	if boxH < 10 {
		boxH = 10
	}

	bg := lipgloss.Color("#16162a")
	borderC := lipgloss.Color("#55ffff")

	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#55ffff")).
		Background(bg)

	pathStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#ffd700")).
		Background(bg)

	var lines []string
	lines = append(lines, headerStyle.Render(fmt.Sprintf(" 📁 %s ", m.Title)))
	lines = append(lines, "")

	if m.TargetFileName != "" {
		lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color("#aaaaaa")).Render(fmt.Sprintf(" Saving File: %s", m.TargetFileName)))
	}
	lines = append(lines, pathStyle.Render(" Directory:   "+displayDir(m.CurrentDir, boxW-17)))
	lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color("#334466")).Render(strings.Repeat("─", max(10, boxW-4))))

	if m.CreatingDir {
		lines = append(lines, promptInputLine(" New Folder Name: ", m.NewDirName))
		lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")).Render(" [Enter] Create  [Esc] Cancel"))
		lines = append(lines, "")
	}

	if m.InSearch {
		lines = append(lines, promptInputLine(" Filter: /", m.SearchQuery))
	}

	if m.EnteringPath {
		lines = append(lines, promptInputLine(" 📍 Go to Path: ", m.PathInput))
		lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")).Render(" [Enter] Go  [Tab] Complete  [Esc] Cancel"))
		lines = append(lines, "")
	}

	if m.Err != nil {
		lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color("#ff5555")).Bold(true).Render(fmt.Sprintf(" Error: %v", m.Err)))
	}

	// Listing area
	footerRows := strings.Count(m.renderFooter(), "\n") + 1
	// Below the list: a rule, the footer rows, and the border.
	availLines := boxH - len(lines) - 3 - footerRows
	if availLines < 3 {
		availLines = 3
	}

	if m.Cursor < m.ScrollOffset {
		m.ScrollOffset = m.Cursor
	} else if m.Cursor >= m.ScrollOffset+availLines {
		m.ScrollOffset = m.Cursor - availLines + 1
	}

	for i := m.ScrollOffset; i < len(m.Filtered) && i < m.ScrollOffset+availLines; i++ {
		e := m.Filtered[i]
		isSel := (i == m.Cursor)

		prefix := "   "
		if isSel {
			prefix = " > "
		}

		icon := "  "
		if e.IsDir {
			if e.Name == ".." {
				icon = "⬆ "
			} else {
				icon = "📁 "
			}
		} else {
			icon = "📄 "
		}

		name := e.Name
		maxNameW := boxW - 12
		if len(name) > maxNameW && maxNameW > 3 {
			name = name[:maxNameW-3] + "..."
		}

		var lineStr string
		if isSel {
			style := lipgloss.NewStyle().Bold(true).Background(lipgloss.Color("#333366")).Foreground(lipgloss.Color("#ffffff"))
			lineStr = style.Render(fmt.Sprintf("%s%s%-*s", prefix, icon, maxNameW, name))
		} else {
			fg := "#dddddd"
			if e.IsDir {
				fg = "#66ccff"
			}
			style := lipgloss.NewStyle().Foreground(lipgloss.Color(fg))
			lineStr = fmt.Sprintf("%s%s%s", prefix, icon, style.Render(name))
		}
		lines = append(lines, zone.Mark(rowZone(i), lineStr))
	}

	for len(lines) < boxH-2-footerRows {
		lines = append(lines, "")
	}

	lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color("#334466")).Render(strings.Repeat("─", max(10, boxW-4))))
	footer := lipgloss.NewStyle().Foreground(lipgloss.Color("#8888aa")).Render(m.renderFooter())
	lines = append(lines, footer)

	content := strings.Join(lines, "\n")

	box := lipgloss.NewStyle().
		Background(bg).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(borderC).
		Padding(0, 1).
		Width(boxW).
		Render(content)

	return box
}
