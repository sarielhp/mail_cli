package label

import (
	"sort"
	"strings"
)

func standardFolderRank(name string) int {
	lower := strings.ToLower(strings.TrimSpace(name))
	lower = strings.TrimPrefix(lower, "[gmail]/")
	lower = strings.TrimPrefix(lower, "[google mail]/")
	switch lower {
	case "inbox":
		return 1
	case "archive", "all mail":
		return 2
	case "spam", "junk", "junk email":
		return 3
	case "trash", "bin", "deleted", "deleted items", "deleted messages":
		return 4
	case "sent", "sent items", "sent messages", "sent mail":
		return 5
	case "drafts":
		return 6
	default:
		return 100
	}
}

func compareFolderNames(a, b string) bool {
	rankA := standardFolderRank(a)
	rankB := standardFolderRank(b)
	if rankA != rankB {
		return rankA < rankB
	}
	return strings.ToLower(a) < strings.ToLower(b)
}

func compareFolderParts(fullA, fullB string) bool {
	partsA := strings.Split(fullA, "/")
	partsB := strings.Split(fullB, "/")
	minLen := min(len(partsA), len(partsB))
	for i := 0; i < minLen; i++ {
		if !strings.EqualFold(partsA[i], partsB[i]) {
			return compareFolderNames(partsA[i], partsB[i])
		}
	}
	return len(partsA) < len(partsB)
}

// BuildLabelTree builds a sorted label hierarchy from items, sorting items in
// place, and rolls each node's message totals up from its descendants. The
// returned top-level nodes start collapsed; FlattenTree descends only into
// nodes whose Expanded is set.
func BuildLabelTree(items []Item) []*TreeNode {
	// Sort items once at the top level
	sort.Slice(items, func(i, j int) bool {
		return compareFolderParts(items[i].FullName, items[j].FullName)
	})

	root := &TreeNode{Name: "root"}

	// Build tree in a single pass
	for _, item := range items {
		parts := strings.Split(item.FullName, "/")
		current := root
		for i, part := range parts {
			var found *TreeNode
			// Use binary search for sorted children
			children := current.Children
			left, right := 0, len(children)
			for left < right {
				mid := (left + right) / 2
				if compareFolderNames(children[mid].Name, part) {
					left = mid + 1
				} else {
					right = mid
				}
			}
			if left < len(children) && strings.EqualFold(children[left].Name, part) {
				found = children[left]
			}
			if found == nil {
				fullName := strings.Join(parts[:i+1], "/")
				found = &TreeNode{
					Name:     part,
					FullName: fullName,
				}
				// Insert at sorted position
				current.Children = append(current.Children, nil)
				copy(current.Children[left+1:], current.Children[left:])
				current.Children[left] = found
			}
			current = found
			if i == len(parts)-1 {
				current.MessagesTotal = item.MessagesTotal
				current.MessagesUnread = item.MessagesUnread
				current.IsLabel = item.IsLabel
			}
		}
	}

	// Compute counts in a single pass (no sorting needed)
	for _, child := range root.Children {
		computeRecursiveLabelCounts(child)
	}

	return root.Children
}

func computeRecursiveLabelCounts(n *TreeNode) int64 {
	total := int64(0)
	if n.IsLabel {
		total += n.MessagesTotal
	}
	for _, child := range n.Children {
		total += computeRecursiveLabelCounts(child)
	}
	n.MessagesTotal = total
	return total
}

// FlattenTree walks nodes depth-first into render rows, descending only into
// nodes marked Expanded, and gives each row the box-drawing prefix for its
// position.
func FlattenTree(nodes []*TreeNode) []TreeEntry {
	var lines []TreeEntry
	var walk func(n *TreeNode, depth int, isLastSlice []bool)
	walk = func(n *TreeNode, depth int, isLastSlice []bool) {
		var prefix string
		if depth > 0 {
			var sb strings.Builder
			for j := 0; j < depth-1; j++ {
				if isLastSlice[j] {
					sb.WriteString("    ")
				} else {
					sb.WriteString("│   ")
				}
			}
			if isLastSlice[depth-1] {
				sb.WriteString("└── ")
			} else {
				sb.WriteString("├── ")
			}
			prefix = sb.String()
		}
		lines = append(lines, TreeEntry{
			Node:   n,
			Depth:  depth,
			Text:   n.Name,
			Prefix: prefix,
		})
		if n.Expanded {
			for i, c := range n.Children {
				walk(c, depth+1, append(isLastSlice, i == len(n.Children)-1))
			}
		}
	}
	for _, n := range nodes {
		walk(n, 0, []bool{})
	}
	return lines
}

// CountAllDescendants returns how many nodes lie below n in the tree; it is
// zero for a nil node.
func CountAllDescendants(n *TreeNode) int {
	if n == nil {
		return 0
	}
	count := len(n.Children)
	for _, child := range n.Children {
		count += CountAllDescendants(child)
	}
	return count
}
