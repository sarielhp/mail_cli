package label

import (
	"testing"
)

// The label package orders and nests the folder listing every command prints,
// and resolves a folder name the user typed onto one the server has.

func items(names ...string) []Item {
	out := make([]Item, 0, len(names))
	for _, n := range names {
		out = append(out, Item{Name: n, FullName: n, IsLabel: true})
	}
	return out
}

// The standard folders sort into a familiar order rather than alphabetically,
// and each server spells them differently.
func TestStandardFolderRank(t *testing.T) {
	tests := []struct {
		name  string
		input []string
		want  []string
	}{
		{
			name:  "canonical order",
			input: []string{"Drafts", "Sent", "Trash", "Spam", "Archive", "INBOX"},
			want:  []string{"INBOX", "Archive", "Spam", "Trash", "Sent", "Drafts"},
		},
		{
			name:  "gmail spellings rank with their plain equivalents",
			input: []string{"[Gmail]/Drafts", "[Gmail]/Sent Mail", "[Gmail]/Spam", "[Gmail]/All Mail"},
			want:  []string{"[Gmail]/All Mail", "[Gmail]/Spam", "[Gmail]/Sent Mail", "[Gmail]/Drafts"},
		},
		{
			name:  "outlook spellings",
			input: []string{"Deleted Items", "Junk Email", "Sent Items", "Inbox"},
			want:  []string{"Inbox", "Junk Email", "Deleted Items", "Sent Items"},
		},
		{
			name:  "user folders come after the standard ones, alphabetically",
			input: []string{"zebra", "Receipts", "INBOX", "apple"},
			want:  []string{"INBOX", "apple", "Receipts", "zebra"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := sortedByRank(tc.input)
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Fatalf("got %v, want %v", got, tc.want)
				}
			}
		})
	}
}

func sortedByRank(names []string) []string {
	out := append([]string(nil), names...)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && compareFolderNames(out[j], out[j-1]); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

func TestBuildLabelTreeNestsByPath(t *testing.T) {
	tree := BuildLabelTree(items("INBOX", "research", "research/papers", "research/papers/2025", "receipts"))

	byName := map[string]*TreeNode{}
	for _, n := range tree {
		byName[n.FullName] = n
	}
	if _, ok := byName["research"]; !ok {
		t.Fatalf("research is not a root; roots are %v", rootNames(tree))
	}
	if len(byName["research"].Children) != 1 {
		t.Fatalf("research has %d children, want 1", len(byName["research"].Children))
	}
	papers := byName["research"].Children[0]
	if papers.FullName != "research/papers" {
		t.Errorf("child is %q, want research/papers", papers.FullName)
	}
	if len(papers.Children) != 1 || papers.Children[0].FullName != "research/papers/2025" {
		t.Errorf("research/papers does not hold 2025")
	}
	// A nested label must not also appear as a root.
	for _, n := range tree {
		if n.FullName == "research/papers" {
			t.Error("research/papers appears as a root as well as a child")
		}
	}
}

// A hierarchy whose intermediate level was never listed still nests, or a
// sublabel would vanish from the listing.
func TestBuildLabelTreeSynthesizesMissingParents(t *testing.T) {
	tree := BuildLabelTree(items("a/b/c"))
	if len(tree) != 1 || tree[0].Name != "a" {
		t.Fatalf("roots are %v, want a single synthesized 'a'", rootNames(tree))
	}
	if len(tree[0].Children) != 1 || tree[0].Children[0].Name != "b" {
		t.Fatal("the missing intermediate level was not synthesized")
	}
}

// A parent's count includes its descendants, so a collapsed folder still shows
// how much is under it.
func TestRecursiveCounts(t *testing.T) {
	in := []Item{
		{Name: "p", FullName: "p", MessagesTotal: 1, MessagesUnread: 1, IsLabel: true},
		{Name: "c", FullName: "p/c", MessagesTotal: 10, MessagesUnread: 2, IsLabel: true},
		{Name: "g", FullName: "p/c/g", MessagesTotal: 100, MessagesUnread: 3, IsLabel: true},
	}
	tree := BuildLabelTree(in)
	if len(tree) != 1 {
		t.Fatalf("roots are %v, want one", rootNames(tree))
	}
	// BuildLabelTree has already aggregated. computeRecursiveLabelCounts
	// assigns into the node it totals, so calling it again would add the
	// children back onto a parent that already includes them -- 111 becomes
	// 321. It is meant to run exactly once, from BuildLabelTree.
	if got := tree[0].MessagesTotal; got != 111 {
		t.Errorf("parent total = %d, want 111 including its descendants", got)
	}
	if got := CountAllDescendants(tree[0]); got != 2 {
		t.Errorf("descendants = %d, want 2", got)
	}
}

// FlattenTree descends only into nodes marked Expanded, which is what makes
// the tree collapsible.
func TestFlattenTreeShowsOnlyExpandedBranches(t *testing.T) {
	tree := BuildLabelTree(items("a", "a/b", "a/b/c"))

	collapsed := FlattenTree(tree)
	if len(collapsed) != 1 {
		t.Fatalf("a collapsed tree flattened to %d entries, want just the root", len(collapsed))
	}

	expandAll(tree)
	expanded := FlattenTree(tree)
	if len(expanded) != 3 {
		t.Fatalf("an expanded tree flattened to %d entries, want 3", len(expanded))
	}
	for i, want := range []int{0, 1, 2} {
		if expanded[i].Depth != want {
			t.Errorf("entry %d has depth %d, want %d", i, expanded[i].Depth, want)
		}
	}
	// The drawing prefix marks the last child of a branch differently.
	if expanded[2].Prefix == "" {
		t.Error("a nested entry has no tree prefix")
	}
}

func expandAll(nodes []*TreeNode) {
	for _, n := range nodes {
		n.Expanded = true
		expandAll(n.Children)
	}
}

func rootNames(nodes []*TreeNode) []string {
	var out []string
	for _, n := range nodes {
		out = append(out, n.FullName)
	}
	return out
}

func TestResolveFolder(t *testing.T) {
	folders := items("INBOX", "Archive", "research/papers", "Receipts")

	tests := []struct {
		name     string
		input    string
		fallback string
		want     string
	}{
		{name: "exact match", input: "Archive", want: "Archive"},
		{name: "case insensitive", input: "archive", want: "Archive"},
		{name: "substring match", input: "papers", want: "research/papers"},
		{name: "unknown falls back", input: "nonexistent", fallback: "Receipts", want: "Receipts"},
		{name: "unknown with no fallback lands on inbox", input: "nonexistent", want: "INBOX"},
		{name: "empty name uses the fallback", input: "", fallback: "Archive", want: "Archive"},
		{name: "empty name and no fallback lands on inbox", input: "", want: "INBOX"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ResolveFolder(folders, tc.input, tc.fallback); got != tc.want {
				t.Errorf("ResolveFolder(%q, %q) = %q, want %q", tc.input, tc.fallback, got, tc.want)
			}
		})
	}
}

// With no inbox and no match, the first folder is better than nothing, and an
// empty list returns what was asked for rather than an empty string.
func TestResolveFolderEdgeCases(t *testing.T) {
	if got := ResolveFolder(items("Other", "Second"), "nope", ""); got != "Other" {
		t.Errorf("got %q, want the first folder when there is no inbox", got)
	}
	if got := ResolveFolder(nil, "asked", ""); got != "asked" {
		t.Errorf("got %q, want the requested name back when there are no folders", got)
	}
}
