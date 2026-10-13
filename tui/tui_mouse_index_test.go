package tui

import (
	"strings"
	"testing"

	"github.com/sarielhp/mail_cli/label"
)

func TestMouseIndexSelectsThenOpens(t *testing.T) {
	m, _ := setupTestSpamMenuModel(t, false)
	m = clickOn(t, m, "idx:1")
	if m.eIdx != 1 || m.mode != modeIndex {
		t.Fatalf("first click: eIdx %d mode %v, want the second message selected, not opened", m.eIdx, m.mode)
	}
	m = clickOn(t, m, "idx:1")
	if m.mode != modeDetail || m.selectedID != "msg-2" {
		t.Errorf("second click: mode %v on %q, want the second message open", m.mode, m.selectedID)
	}
}

// With a dialog drawn over the index, the dimmed rows keep their zones, and a
// click on one must not act on a list the user cannot use.
func TestMouseIndexBehindADialogDoesNothing(t *testing.T) {
	m, _ := setupTestSpamMenuModel(t, false)
	_ = m.View() // record the rows' zones
	m.spamMenuOpen = true
	m = clickOn(t, m, "idx:1")
	if m.eIdx != 0 {
		t.Errorf("a click behind the dialog moved the selection to %d", m.eIdx)
	}
}

func TestMouseTreeSelectsThenOpens(t *testing.T) {
	m, _ := setupTestSpamMenuModel(t, false)
	m.treeOpen = true
	m.entries = []label.TreeEntry{
		{Node: &label.TreeNode{FullName: "INBOX"}, Text: "INBOX"},
		{Node: &label.TreeNode{FullName: "work"}, Text: "work"},
	}
	m.cursor = 0
	m = clickOn(t, m, "tree:1")
	if m.cursor != 1 || !m.treeOpen {
		t.Fatalf("first click: cursor %d open %v, want work selected and the tree still open", m.cursor, m.treeOpen)
	}
	m = clickOn(t, m, "tree:1")
	if m.treeOpen {
		t.Errorf("second click did not open the folder")
	}
}

func TestLinkAt(t *testing.T) {
	const url = "https://example.com/a"
	bel := "go \x1b]8;;" + url + "\x07here\x1b]8;;\x07 now"
	st := "\x1b[1mgo\x1b[0m \x1b]8;id=1;" + url + "\x1b\\here\x1b]8;;\x1b\\ now"
	wide := "日本 \x1b]8;;" + url + "\x07x\x1b]8;;\x07"
	for _, tt := range []struct {
		name, line string
		x          int
		want       string
	}{
		{"BEL-terminated, on the link", bel, 4, url},
		{"BEL-terminated, before it", bel, 1, ""},
		{"BEL-terminated, after it", bel, 8, ""},
		{"ST-terminated with params and SGR", st, 3, url},
		{"after wide characters", wide, 5, url},
		{"on a wide character", wide, 1, ""},
	} {
		frame := "first line\n" + tt.line
		if got := linkAt(frame, tt.x, 1); got != tt.want {
			t.Errorf("%s: linkAt(x=%d) = %q, want %q", tt.name, tt.x, got, tt.want)
		}
	}
}

// A message is written by a stranger: a click follows web and mail links, not
// a file: link or a desktop handler.
func TestOpenLinkRefusesOtherSchemes(t *testing.T) {
	m, _ := setupTestSpamMenuModel(t, false)
	for _, link := range []string{"file:///etc/passwd", "vscode://open?x", "javascript:alert(1)"} {
		model, _ := m.openLink(link)
		m = model.(*tuiModel)
		if !strings.HasPrefix(m.saveStatus, "Not opening") {
			t.Errorf("%s: status %q, want it refused", link, m.saveStatus)
		}
	}
}
