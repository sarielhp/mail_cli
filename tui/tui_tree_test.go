package tui

import (
	"github.com/sarielhp/mail_cli/label"
	"strings"
	"testing"
)

func TestBuildFolderTree(t *testing.T) {
	items := []label.Item{
		{Name: "INBOX", FullName: "INBOX"},
		{Name: "Work", FullName: "Work"},
		{Name: "Work/Projects", FullName: "Work/Projects"},
		{Name: "Work/Projects/Beta", FullName: "Work/Projects/Beta"},
		{Name: "Personal", FullName: "Personal"},
	}
	tree := label.BuildLabelTree(items)
	if len(tree) != 3 {
		t.Errorf("expected 3 root nodes, got %d", len(tree))
	}
}

func TestFlattenTree(t *testing.T) {
	projects := &label.TreeNode{Name: "Projects", FullName: "Work/Projects"}
	projects.Expanded = true
	beta := &label.TreeNode{Name: "Beta", FullName: "Work/Projects/Beta"}
	projects.Children = []*label.TreeNode{beta}

	work := &label.TreeNode{Name: "Work", FullName: "Work"}
	work.Children = []*label.TreeNode{projects}
	work.Expanded = true

	inbox := &label.TreeNode{Name: "INBOX", FullName: "INBOX"}
	nodes := []*label.TreeNode{inbox, work}
	entries := label.FlattenTree(nodes)

	if len(entries) != 4 {
		t.Errorf("expected 4 entries, got %d", len(entries))
	}
}

func TestFlattenTreePrefixes(t *testing.T) {
	inbox := &label.TreeNode{Name: "INBOX", FullName: "INBOX"}

	projects := &label.TreeNode{Name: "Projects", FullName: "Work/Projects"}
	projects.Expanded = true
	beta := &label.TreeNode{Name: "Beta", FullName: "Work/Projects/Beta"}
	projects.Children = []*label.TreeNode{beta}

	work := &label.TreeNode{Name: "Work", FullName: "Work"}
	work.Children = []*label.TreeNode{projects}
	work.Expanded = true

	nodes := []*label.TreeNode{inbox, work}
	entries := label.FlattenTree(nodes)

	if entries[0].Prefix != "" {
		t.Errorf("expected entries[0] prefix to be empty, got %q", entries[0].Prefix)
	}
	if entries[1].Prefix != "" {
		t.Errorf("expected entries[1] prefix to be empty, got %q", entries[1].Prefix)
	}
	if entries[2].Prefix != "└── " {
		t.Errorf("expected entries[2] prefix to be '└── ', got %q", entries[2].Prefix)
	}
	if entries[3].Prefix != "    └── " {
		t.Errorf("expected entries[3] prefix to be '    └── ', got %q", entries[3].Prefix)
	}
}

func TestFlattenTreeCollapsed(t *testing.T) {
	work := &label.TreeNode{Name: "Work", FullName: "Work"}
	work.Children = []*label.TreeNode{{Name: "Child", FullName: "Work/Child"}}
	work.Expanded = false

	nodes := []*label.TreeNode{work}
	entries := label.FlattenTree(nodes)
	if len(entries) != 1 {
		t.Errorf("expected 1 entry when collapsed, got %d", len(entries))
	}
}

func TestTreeEntryDepth(t *testing.T) {
	projects := &label.TreeNode{Name: "Projects", FullName: "Work/Projects"}
	projects.Expanded = true
	deep := &label.TreeNode{Name: "Deep", FullName: "Work/Projects/Deep"}
	projects.Children = []*label.TreeNode{deep}

	work := &label.TreeNode{Name: "Work", FullName: "Work"}
	work.Children = []*label.TreeNode{projects}
	work.Expanded = true

	entries := label.FlattenTree([]*label.TreeNode{work})

	if len(entries) < 3 {
		t.Fatalf("not enough entries: %d", len(entries))
	}
	if entries[0].Depth != 0 {
		t.Errorf("expected depth 0, got %d", entries[0].Depth)
	}
	if entries[1].Depth != 1 {
		t.Errorf("expected depth 1, got %d", entries[1].Depth)
	}
	if entries[2].Depth != 2 {
		t.Errorf("expected depth 2, got %d", entries[2].Depth)
	}
}

func TestTuiModelFoldersLoaded(t *testing.T) {
	m := newTuiModel(nil, "", nil)
	folders := []label.Item{
		{Name: "INBOX", FullName: "INBOX"},
		{Name: "Work", FullName: "Work"},
		{Name: "Work/Projects", FullName: "Work/Projects"},
	}

	res, _ := m.Update(foldersLoadedMsg{folders: folders})
	updatedModel := res.(*tuiModel)
	if len(updatedModel.folders) != 3 {
		t.Errorf("expected 3 folders, got %d", len(updatedModel.folders))
	}
	if len(updatedModel.tree) != 2 {
		t.Errorf("expected 2 root tree nodes, got %d", len(updatedModel.tree))
	}
	if len(updatedModel.entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(updatedModel.entries))
	}
}

func TestFolderSearch(t *testing.T) {
	m := newTuiModel(nil, "", nil)
	folders := []label.Item{
		{Name: "INBOX", FullName: "INBOX"},
		{Name: "Work", FullName: "Work"},
		{Name: "Work/Projects", FullName: "Work/Projects"},
		{Name: "Personal", FullName: "Personal"},
	}

	res, _ := m.Update(foldersLoadedMsg{folders: folders})
	updatedModel := res.(*tuiModel)

	if len(updatedModel.entries) != 3 {
		t.Errorf("expected 3 entries, got %d", len(updatedModel.entries))
	}

	updatedModel.inFolderSearch = true
	updatedModel.folderSearch = "proj"
	updatedModel.buildEntries()

	foundProj := false
	for _, e := range updatedModel.entries {
		if strings.Contains(strings.ToLower(e.Node.FullName), "proj") {
			foundProj = true
		}
	}
	if !foundProj {
		t.Error("expected to find folder matching 'proj' in search results")
	}
}

func TestFolderSearchCursorJump(t *testing.T) {
	m := newTuiModel(nil, "", nil)
	folders := []label.Item{
		{Name: "Work", FullName: "Work"},
		{Name: "Work/Projects", FullName: "Work/Projects"},
		{Name: "Work/Projects/Sub", FullName: "Work/Projects/Sub"},
		{Name: "Other", FullName: "Other"},
	}

	res, _ := m.Update(foldersLoadedMsg{folders: folders})
	updatedModel := res.(*tuiModel)

	updatedModel.inFolderSearch = true
	updatedModel.folderSearch = "work"
	updatedModel.buildEntries()

	if updatedModel.cursor < 0 || updatedModel.cursor >= len(updatedModel.entries) {
		t.Fatalf("expected cursor to be valid, got %d (len: %d)", updatedModel.cursor, len(updatedModel.entries))
	}

	selectedNode := updatedModel.entries[updatedModel.cursor].Node
	if selectedNode.FullName != "Work/Projects/Sub" {
		t.Errorf("expected selected folder to be 'Work/Projects/Sub', got %q", selectedNode.FullName)
	}
}

func TestCountAllDescendants(t *testing.T) {
	grandchild := &label.TreeNode{Name: "Grandchild", FullName: "Work/Projects/Grandchild"}
	projects := &label.TreeNode{Name: "Projects", FullName: "Work/Projects", Children: []*label.TreeNode{grandchild}}
	work := &label.TreeNode{Name: "Work", FullName: "Work", Children: []*label.TreeNode{projects}}

	if count := label.CountAllDescendants(work); count != 2 {
		t.Errorf("expected Work to have 2 descendants, got %d", count)
	}
	if count := label.CountAllDescendants(projects); count != 1 {
		t.Errorf("expected Projects to have 1 descendant, got %d", count)
	}
	if count := label.CountAllDescendants(grandchild); count != 0 {
		t.Errorf("expected Grandchild to have 0 descendants, got %d", count)
	}
}
