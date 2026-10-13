package tui

import (
	"strings"
	"testing"

	"github.com/sarielhp/mail_cli/label"

	tea "github.com/charmbracelet/bubbletea"
)

func TestFolderTree_TabCycleExpansion(t *testing.T) {
	// Build a 4-level test hierarchy:
	// Archive
	// └── 2026
	//     └── 08
	//         └── Inbox
	// Work
	// └── ProjectA
	folders := []label.Item{
		{FullName: "Archive"},
		{FullName: "Archive/2026"},
		{FullName: "Archive/2026/08"},
		{FullName: "Archive/2026/08/Inbox"},
		{FullName: "Work"},
		{FullName: "Work/ProjectA"},
	}

	m := newTuiModel(nil, "Archive", nil)
	m.folders = folders
	m.tree = label.BuildLabelTree(folders)
	m.globalFolders = folders
	m.globalTree = label.BuildLabelTree(folders)
	m.showGlobalTree = true
	m.treeOpen = true
	m.buildEntries()

	// Locate the "Archive" root node
	archiveIdx := -1
	for i, entry := range m.entries {
		if entry.Node != nil && entry.Node.FullName == "Archive" {
			archiveIdx = i
			break
		}
	}
	if archiveIdx == -1 {
		t.Fatal("expected Archive entry in folder entries")
	}
	m.cursor = archiveIdx
	archiveNode := m.entries[archiveIdx].Node

	// Initial State: All folded
	if archiveNode.Expanded {
		t.Error("expected Archive to start unexpanded")
	}

	// 1st Tab: Expand Level 0 (Archive node itself)
	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = res.(*tuiModel)
	if !archiveNode.Expanded {
		t.Error("expected Archive node to be expanded after 1st Tab")
	}
	// Level 1 child (2026) should still be unexpanded
	node2026 := archiveNode.Children[0]
	if node2026.Expanded {
		t.Error("expected Archive/2026 to remain unexpanded after 1st Tab")
	}

	// 2nd Tab: Expand Level 1 (Archive/2026)
	res, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = res.(*tuiModel)
	if !archiveNode.Expanded || !node2026.Expanded {
		t.Error("expected Archive and Archive/2026 to be expanded after 2nd Tab")
	}
	node08 := node2026.Children[0]
	if node08.Expanded {
		t.Error("expected Archive/2026/08 to remain unexpanded after 2nd Tab")
	}

	// 3rd Tab: Expand Level 2 (Archive/2026/08)
	res, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = res.(*tuiModel)
	if !node08.Expanded {
		t.Error("expected Archive/2026/08 to be expanded after 3rd Tab")
	}

	// 4th Tab: Exceeding maximum depth -> All should fold over
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if archiveNode.Expanded || node2026.Expanded || node08.Expanded {
		t.Errorf("expected entire Archive subtree to fold over after 4th Tab, got archive=%v, 2026=%v, 08=%v",
			archiveNode.Expanded, node2026.Expanded, node08.Expanded)
	}
}

func TestFolderTree_TabPartiallyUnfolded(t *testing.T) {
	folders := []label.Item{
		{FullName: "Parent"},
		{FullName: "Parent/Child1"},
		{FullName: "Parent/Child1/Grandchild"},
		{FullName: "Parent/Child1/Grandchild/GreatGrandchild"},
		{FullName: "Parent/Child2"},
		{FullName: "Parent/Child2/Grandchild"},
		{FullName: "Parent/Child2/Grandchild/GreatGrandchild"},
	}

	m := newTuiModel(nil, "Parent", nil)
	m.folders = folders
	m.tree = label.BuildLabelTree(folders)
	m.globalFolders = folders
	m.globalTree = label.BuildLabelTree(folders)
	m.showGlobalTree = true
	m.treeOpen = true
	m.buildEntries()

	parent := m.globalTree[0]
	// Manually unfold Parent (depth 0) and Child2 (depth 1), leaving Child1 unexpanded
	parent.Expanded = true
	parent.Children[1].Expanded = true // Child2
	m.buildEntries()

	// Focus on Parent
	for i, e := range m.entries {
		if e.Node != nil && e.Node.FullName == "Parent" {
			m.cursor = i
			break
		}
	}

	// Shallowest unexpanded node with children is Child1 (depth 1).
	// Tab should expand to depth 1, making Child1 expanded.
	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = res.(*tuiModel)

	if !parent.Children[0].Expanded {
		t.Error("expected Parent/Child1 to be expanded after Tab on partially unfolded tree")
	}

	// Next Tab: expands depth 2 (Grandchildren)
	res, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = res.(*tuiModel)

	if !parent.Children[0].Children[0].Expanded || !parent.Children[1].Children[0].Expanded {
		t.Error("expected Grandchildren to be expanded after subsequent Tab")
	}

	// Next Tab: folds everything in Parent subtree
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})

	if parent.Expanded || parent.Children[0].Expanded || parent.Children[1].Expanded {
		t.Error("expected entire Parent subtree to fold over")
	}
}

func TestFolderTree_StandardMailboxesOrder(t *testing.T) {
	folders := []label.Item{
		{FullName: "Work"},
		{FullName: "Trash"},
		{FullName: "Spam"},
		{FullName: "INBOX"},
		{FullName: "Sent"},
		{FullName: "Archive"},
		{FullName: "Personal"},
	}

	tree := label.BuildLabelTree(folders)
	entries := label.FlattenTree(tree)

	var names []string
	for _, e := range entries {
		if e.Node != nil && e.Depth == 0 {
			names = append(names, strings.ToLower(e.Node.Name))
		}
	}

	expectedOrder := []string{"inbox", "archive", "spam", "trash", "sent", "personal", "work"}
	if len(names) != len(expectedOrder) {
		t.Fatalf("expected %d entries, got %d: %v", len(expectedOrder), len(names), names)
	}

	for i, exp := range expectedOrder {
		if names[i] != exp {
			t.Errorf("at index %d: expected %q, got %q (full list: %v)", i, exp, names[i], names)
		}
	}
}

// F2 used to be a second key for the folder tree. It saves the message now:
// one action, one key, and the tree stays on f.
func TestFolderTree_FKeyOpensTreeAndF2DoesNot(t *testing.T) {
	m := newTuiModel(nil, "INBOX", nil)
	m.currentF = "INBOX"

	m.Update(tea.KeyMsg{Type: tea.KeyF2})
	if m.treeOpen {
		t.Error("F2 opened the folder tree")
	}

	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	if !res.(*tuiModel).treeOpen {
		t.Error("expected treeOpen=true after f")
	}
}
