package email

import (
	"sort"
	"time"
)

func initThreadNodes(emails []Email) ([]*ThreadNode, map[HeaderID]*ThreadNode) {
	nodeMap := make(map[HeaderID]*ThreadNode, len(emails))
	nodes := make([]*ThreadNode, len(emails))
	for i := range emails {
		node := &ThreadNode{
			Email: &emails[i],
		}
		nodes[i] = node
		if emails[i].MessageID != "" {
			nodeMap[emails[i].MessageID] = node
		}
	}
	return nodes, nodeMap
}

func findParentNode(node *ThreadNode, nodeMap map[HeaderID]*ThreadNode) *ThreadNode {
	if node.Email.InReplyTo != "" {
		for _, pid := range extractMessageIDs(node.Email.InReplyTo) {
			if p, ok := nodeMap[pid]; ok {
				return p
			}
		}
	}
	if node.Email.References != "" {
		parentIDs := extractMessageIDs(node.Email.References)
		for i := len(parentIDs) - 1; i >= 0; i-- {
			if p, ok := nodeMap[parentIDs[i]]; ok {
				return p
			}
		}
	}
	return nil
}

func groupRootsBySubject(orphanRoots []*ThreadNode) []*ThreadNode {
	subjectGroups := make(map[string][]*ThreadNode)
	var finalRoots []*ThreadNode

	for _, node := range orphanRoots {
		normSub := cleanSubject(node.Email.Subject)
		if normSub != "" && normSub != "no subject" && normSub != "(no subject)" {
			subjectGroups[normSub] = append(subjectGroups[normSub], node)
		} else {
			finalRoots = append(finalRoots, node)
		}
	}

	for _, group := range subjectGroups {
		if len(group) == 1 {
			finalRoots = append(finalRoots, group[0])
			continue
		}
		sort.Slice(group, func(i, j int) bool {
			return group[i].Email.EmailDate.Before(group[j].Email.EmailDate)
		})
		root := group[0]
		for i := 1; i < len(group); i++ {
			child := group[i]
			child.Parent = root
			root.Children = append(root.Children, child)
		}
		finalRoots = append(finalRoots, root)
	}
	return finalRoots
}

func sortThreadChildren(n *ThreadNode) {
	sort.Slice(n.Children, func(i, j int) bool {
		return n.Children[i].Email.EmailDate.Before(n.Children[j].Email.EmailDate)
	})
	for _, child := range n.Children {
		sortThreadChildren(child)
	}
}

func isAncestor(ancestor, child *ThreadNode) bool {
	for curr := child; curr != nil; curr = curr.Parent {
		if curr == ancestor {
			return true
		}
	}
	return false
}

// BuildThreadTree links emails by In-Reply-To and References, groups messages
// with no usable parent by normalized subject, and returns the roots with each
// node's children ordered oldest first.
func BuildThreadTree(emails []Email) []*ThreadNode {
	nodes, nodeMap := initThreadNodes(emails)

	var orphanRoots []*ThreadNode
	for _, node := range nodes {
		parentNode := findParentNode(node, nodeMap)
		if parentNode != nil && parentNode != node && !isAncestor(node, parentNode) {
			node.Parent = parentNode
			parentNode.Children = append(parentNode.Children, node)
		} else {
			orphanRoots = append(orphanRoots, node)
		}
	}

	finalRoots := groupRootsBySubject(orphanRoots)
	for _, root := range finalRoots {
		sortThreadChildren(root)
	}
	return finalRoots
}

// GetThreadNewestDate returns the latest EmailDate in n's subtree, including n
// itself.
func GetThreadNewestDate(n *ThreadNode) time.Time {
	newest := n.Email.EmailDate
	var traverse func(node *ThreadNode)
	traverse = func(node *ThreadNode) {
		if node.Email.EmailDate.After(newest) {
			newest = node.Email.EmailDate
		}
		for _, child := range node.Children {
			traverse(child)
		}
	}
	traverse(n)
	return newest
}

// CountThreadReplies returns the number of messages below n in the thread,
// counting all descendants but not n itself.
func CountThreadReplies(n *ThreadNode) int {
	count := 0
	var traverse func(node *ThreadNode)
	traverse = func(node *ThreadNode) {
		count += len(node.Children)
		for _, child := range node.Children {
			traverse(child)
		}
	}
	traverse(n)
	return count
}

func extractMessageIDs(header string) []HeaderID {
	var ids []HeaderID
	start := -1
	for i, r := range header {
		if r == '<' {
			start = i
		} else if r == '>' && start != -1 {
			ids = append(ids, HeaderID(header[start:i+1]))
			start = -1
		}
	}
	return ids
}
