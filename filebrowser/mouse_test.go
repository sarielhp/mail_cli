package filebrowser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarielhp/mail_cli/filebrowser/zonetest"

	tea "github.com/charmbracelet/bubbletea"
	zone "github.com/lrstanley/bubblezone"
)

// tree makes root/{docs/,photos/,notes.txt} and returns root.
func tree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, d := range []string{"docs", "photos"} {
		if err := os.Mkdir(filepath.Join(root, d), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}

// The attachment picker opens on ".", and filepath.Dir(".") is ".": going up
// went nowhere, by key or by the ".." entry.
func TestGoingUpFromARelativeStart(t *testing.T) {
	root := tree(t)
	t.Chdir(filepath.Join(root, "docs"))
	m := NewFileSelector(".", 80, 24)

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if m.CurrentDir != root {
		t.Fatalf("Backspace from %q: in %q, want %q", "docs", m.CurrentDir, root)
	}
	// The cursor is on the directory just left, not at the top.
	if got := m.Filtered[m.Cursor].Name; got != "docs" {
		t.Errorf("cursor on %q after going up, want docs", got)
	}

	// The ".." entry goes up too.
	m.CurrentDir = filepath.Join(root, "photos")
	m.ReloadEntries()
	m.Cursor = 0 // ".."
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.CurrentDir != root || m.Filtered[m.Cursor].Name != "photos" {
		t.Errorf(`".." from photos: in %q on %q`, m.CurrentDir, m.Filtered[m.Cursor].Name)
	}
}

// click draws the browser and releases the left button over zone id.
func click(t *testing.T, m Model, id string) (Model, tea.Cmd) {
	t.Helper()
	z := zonetest.WaitForZone(t, func() { _ = zone.Scan(m.View()) }, id)
	return m.Mouse(tea.MouseMsg{X: z.StartX, Y: z.StartY, Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease})
}

func indexOf(t *testing.T, m Model, name string) int {
	t.Helper()
	for i, e := range m.Filtered {
		if e.Name == name {
			return i
		}
	}
	t.Fatalf("%q is not listed", name)
	return -1
}

func TestMouseSelectsThenOpens(t *testing.T) {
	root := tree(t)
	m := NewFileSelector(root, 80, 24)

	docs := indexOf(t, m, "docs")
	m, _ = click(t, m, rowZone(docs))
	if m.Cursor != docs || m.CurrentDir != root {
		t.Fatalf("first click: cursor %d in %q, want docs selected and not entered", m.Cursor, m.CurrentDir)
	}
	m, _ = click(t, m, rowZone(docs))
	if m.CurrentDir != filepath.Join(root, "docs") {
		t.Fatalf("second click: in %q, want docs entered", m.CurrentDir)
	}

	// The footer's Up is the key.
	m, _ = click(t, m, keyZone("backspace"))
	if m.CurrentDir != root {
		t.Fatalf("clicking Up: in %q", m.CurrentDir)
	}

	// Two clicks on a file choose it.
	notes := indexOf(t, m, "notes.txt")
	m, _ = click(t, m, rowZone(notes))
	m, cmd := click(t, m, rowZone(notes))
	if !m.Done || cmd == nil {
		t.Fatalf("two clicks on a file did not choose it")
	}
	if sel, ok := cmd().(SelectedMsg); !ok || sel.FullPath != filepath.Join(root, "notes.txt") {
		t.Errorf("chose %#v", cmd())
	}
}

func TestMouseCancelAndWheel(t *testing.T) {
	m := NewFileSelector(tree(t), 80, 24)
	m, _ = m.Mouse(tea.MouseMsg{Button: tea.MouseButtonWheelDown})
	if m.Cursor != 1 {
		t.Errorf("wheel down: cursor %d, want 1", m.Cursor)
	}
	m, _ = click(t, m, keyZone("esc"))
	if !m.Canceled {
		t.Errorf("clicking Cancel did not cancel")
	}
}

// A deep directory used to be cut at the right, hiding the one the user is in.
func TestDisplayDirKeepsTheEnd(t *testing.T) {
	home, _ := os.UserHomeDir()
	for _, tt := range []struct {
		dir   string
		width int
		want  string
	}{
		{home, 20, "~"},
		{filepath.Join(home, "docs", "talks"), 40, "~/docs/talks"},
		{"/very/long/path/to/the/folder/we/are/in", 16, "…older/we/are/in"},
		{"/short", 40, "/short"},
	} {
		if got := displayDir(tt.dir, tt.width); got != tt.want {
			t.Errorf("displayDir(%q, %d) = %q, want %q", tt.dir, tt.width, got, tt.want)
		}
	}
}

// Every footer row fits inside the box at any width, in both modes, and no
// hint is lost: a single row used to wrap through the middle of a hint.
func TestFooterFitsTheBox(t *testing.T) {
	for _, w := range []int{34, 50, 64, 100} {
		for _, m := range []Model{NewFileSelector(t.TempDir(), w, 30), New(t.TempDir(), "x.pdf", w, 30)} {
			plain := zone.Scan(m.renderFooter())
			for _, row := range strings.Split(plain, "\n") {
				if n := len([]rune(row)); n > m.contentWidth() {
					t.Errorf("width %d: footer row is %d columns, box has %d: %q", w, n, m.contentWidth(), row)
				}
			}
			for _, it := range append(m.footerItems(), jumpItems...) {
				if !strings.Contains(plain, it.label) {
					t.Errorf("width %d: %q is missing", w, it.label)
				}
			}
		}
	}
}

// The jump targets were a "[1-4]" hint; each is now clickable.
func TestMouseJump(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	m := NewFileSelector(tree(t), 100, 30)
	m, _ = click(t, m, keyZone("2"))
	if m.CurrentDir != home {
		t.Errorf("clicking [2] ~: in %q, want %q", m.CurrentDir, home)
	}
}
