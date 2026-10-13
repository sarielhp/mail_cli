package tui

import (
	"sort"
	"strings"

	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/label"
)

func transferExpandedStates(oldNodes, newNodes []*label.TreeNode) {
	oldStates := make(map[string]bool)
	var collect func(nodes []*label.TreeNode)
	collect = func(nodes []*label.TreeNode) {
		for _, n := range nodes {
			oldStates[n.FullName] = n.Expanded
			collect(n.Children)
		}
	}
	collect(oldNodes)
	var apply func(nodes []*label.TreeNode)
	apply = func(nodes []*label.TreeNode) {
		for _, n := range nodes {
			if exp, ok := oldStates[n.FullName]; ok {
				n.Expanded = exp
			}
			apply(n.Children)
		}
	}
	apply(newNodes)
}

func (m *tuiModel) buildEntries() {
	var tree []*label.TreeNode
	if m.inFolderSearch && m.folderSearch != "" {
		var filtered []label.Item
		lowerSearch := strings.ToLower(m.folderSearch)
		sourceFolders := m.folders
		if m.showGlobalTree {
			sourceFolders = m.globalFolders
		}
		for _, f := range sourceFolders {
			if strings.Contains(strings.ToLower(f.FullName), lowerSearch) {
				filtered = append(filtered, f)
			}
		}
		tree = label.BuildLabelTree(filtered)
		var expandAll func(nodes []*label.TreeNode)
		expandAll = func(nodes []*label.TreeNode) {
			for _, n := range nodes {
				n.Expanded = true
				expandAll(n.Children)
			}
		}
		expandAll(tree)
	} else {
		if m.showGlobalTree {
			tree = m.globalTree
		} else {
			tree = m.tree
		}
	}
	m.entries = label.FlattenTree(tree)
	if m.inFolderSearch && m.folderSearch != "" {
		found := false
		for i, entry := range m.entries {
			if entry.Node != nil && len(entry.Node.Children) == 0 {
				m.cursor = i
				found = true
				break
			}
		}
		if !found {
			m.cursor = min(m.cursor, max(0, len(m.entries)-1))
		}
	} else {
		m.cursor = min(m.cursor, max(0, len(m.entries)-1))
	}
}

func sortThreadRoots(roots []*email.ThreadNode) {
	isBadNode := func(n *email.ThreadNode) bool {
		return n.Email.IsSpam || n.Email.IsPolitical || n.Email.IsBlacklisted
	}
	sort.Slice(roots, func(i, j int) bool {
		badI := isBadNode(roots[i])
		badJ := isBadNode(roots[j])
		if badI != badJ {
			return !badI
		}
		dateI := email.GetThreadNewestDate(roots[i])
		dateJ := email.GetThreadNewestDate(roots[j])
		return dateI.Before(dateJ)
	})
}

func buildThreadIDOrder(roots []*email.ThreadNode) map[email.ID]int {
	var expandedOrder []email.ID
	var getExpandedOrder func(n *email.ThreadNode)
	getExpandedOrder = func(n *email.ThreadNode) {
		expandedOrder = append(expandedOrder, n.Email.ID)
		for _, child := range n.Children {
			getExpandedOrder(child)
		}
	}
	for _, root := range roots {
		getExpandedOrder(root)
	}
	idToNumber := make(map[email.ID]int, len(expandedOrder))
	for i, id := range expandedOrder {
		idToNumber[id] = i + 1
	}
	return idToNumber
}

func computeThreadPrefix(depth int, isLastSlice []bool, hasReplies bool, isCollapsed bool) string {
	if depth == 0 {
		if !hasReplies {
			return ""
		}
		if isCollapsed {
			return "━━━ "
		}
		return "┌── "
	}
	var sb strings.Builder
	for j := 0; j < depth-1; j++ {
		if isLastSlice[j] {
			sb.WriteString("  ")
		} else {
			sb.WriteString("│ ")
		}
	}
	if isLastSlice[depth-1] {
		sb.WriteString("└── ")
	} else {
		sb.WriteString("├── ")
	}
	return sb.String()
}

func collectThreadSenders(n *email.ThreadNode) string {
	var senders []string
	seen := make(map[string]bool)
	var collect func(node *email.ThreadNode)
	collect = func(node *email.ThreadNode) {
		name := email.FormatSender(node.Email.FromRaw)
		if !seen[name] {
			seen[name] = true
			senders = append(senders, name)
		}
		for _, child := range node.Children {
			collect(child)
		}
	}
	collect(n)
	return strings.Join(senders, ", ")
}

func filterVisibleEmails(emails []email.Email, query string, fuzzy bool) []email.Email {
	if query == "" {
		return emails
	}
	if fuzzy {
		return fuzzyFilterEmails(query, emails)
	}
	var filtered []email.Email
	lowerQuery := strings.ToLower(query)
	for _, em := range emails {
		if strings.Contains(strings.ToLower(em.Subject), lowerQuery) ||
			strings.Contains(strings.ToLower(em.FromRaw), lowerQuery) ||
			strings.Contains(strings.ToLower(em.FromEmail), lowerQuery) {
			filtered = append(filtered, em)
		}
	}
	return filtered
}

func (m *tuiModel) rebuildVisibleEmails() {
	if len(m.rawEmails) == 0 {
		m.emails = nil
		return
	}
	roots := email.BuildThreadTree(m.rawEmails)
	sortThreadRoots(roots)
	idToNumber := buildThreadIDOrder(roots)

	var visible []email.Email
	var flatten func(n *email.ThreadNode, depth int, isAncestorCollapsed bool, isLastSlice []bool)
	flatten = func(n *email.ThreadNode, depth int, isAncestorCollapsed bool, isLastSlice []bool) {
		if isAncestorCollapsed {
			for i, child := range n.Children {
				flatten(child, depth+1, true, append(isLastSlice, i == len(n.Children)-1))
			}
			return
		}
		em := *n.Email
		em.ThreadDepth = depth
		em.ThreadHasReplies = len(n.Children) > 0
		em.ThreadRepliesCount = email.CountThreadReplies(n)
		em.ThreadIndex = idToNumber[em.ID]
		isCollapsed := !m.expandedThreads[threadKeyOf(&em)]
		em.ThreadCollapsed = isCollapsed
		em.ThreadPrefix = computeThreadPrefix(depth, isLastSlice, em.ThreadHasReplies, isCollapsed)
		if em.ThreadHasReplies {
			em.ThreadSenderSummary = collectThreadSenders(n)
		}
		visible = append(visible, em)
		for i, child := range n.Children {
			flatten(child, depth+1, isCollapsed, append(isLastSlice, i == len(n.Children)-1))
		}
	}
	for _, root := range roots {
		flatten(root, 0, false, []bool{})
	}
	m.emails = filterVisibleEmails(visible, m.searchQuery, m.fuzzySearch)
}

func (m *tuiModel) expandRecursively(emailID email.ID) {
	target := findThreadNode(email.BuildThreadTree(m.rawEmails), emailID)
	if target == nil {
		return
	}
	var markExpanded func(n *email.ThreadNode)
	markExpanded = func(n *email.ThreadNode) {
		m.expandedThreads[threadNodeKey(n)] = true
		for _, child := range n.Children {
			markExpanded(child)
		}
	}
	markExpanded(target)
}

func (m *tuiModel) expandWholeThreadForEmail(emailID email.ID) {
	if emailID == "" || len(m.rawEmails) == 0 {
		return
	}
	if m.expandedThreads == nil {
		m.expandedThreads = make(map[threadKey]bool)
	}
	roots := email.BuildThreadTree(m.rawEmails)
	var findInTree func(n *email.ThreadNode) bool
	findInTree = func(n *email.ThreadNode) bool {
		if n.Email.ID == emailID {
			return true
		}
		for _, child := range n.Children {
			if findInTree(child) {
				return true
			}
		}
		return false
	}
	var targetRoot *email.ThreadNode
	for _, root := range roots {
		if findInTree(root) {
			targetRoot = root
			break
		}
	}
	if targetRoot == nil {
		return
	}
	var markExpanded func(n *email.ThreadNode)
	markExpanded = func(n *email.ThreadNode) {
		m.expandedThreads[threadNodeKey(n)] = true
		for _, child := range n.Children {
			markExpanded(child)
		}
	}
	markExpanded(targetRoot)
}

func (m *tuiModel) cycleTreeSubtreeExpansion(root *label.TreeNode) {
	if root == nil || len(root.Children) == 0 {
		return
	}

	// Find the shallowest depth of any node with children in root's subtree that is not yet expanded.
	minUnexpandedDepth := -1
	var findShallowest func(n *label.TreeNode, depth int)
	findShallowest = func(n *label.TreeNode, depth int) {
		if len(n.Children) > 0 {
			if !n.Expanded {
				if minUnexpandedDepth == -1 || depth < minUnexpandedDepth {
					minUnexpandedDepth = depth
				}
			}
			for _, child := range n.Children {
				findShallowest(child, depth+1)
			}
		}
	}
	findShallowest(root, 0)

	// If all nodes with children in the subtree are already expanded, fold/collapse the whole subtree.
	if minUnexpandedDepth == -1 {
		var collapseSubtree func(n *label.TreeNode)
		collapseSubtree = func(n *label.TreeNode) {
			n.Expanded = false
			for _, child := range n.Children {
				collapseSubtree(child)
			}
		}
		collapseSubtree(root)
		return
	}

	// Otherwise, expand all nodes with children in the subtree at relative depth <= minUnexpandedDepth.
	var expandToDepth func(n *label.TreeNode, depth int)
	expandToDepth = func(n *label.TreeNode, depth int) {
		if len(n.Children) > 0 {
			if depth <= minUnexpandedDepth {
				n.Expanded = true
			}
			for _, child := range n.Children {
				expandToDepth(child, depth+1)
			}
		}
	}
	expandToDepth(root, 0)
}

// threadKey names a message in the thread-expansion state: by its Message-ID
// header when it has one, so the state survives a refresh that renumbers
// backend IDs, and by its backend ID otherwise. The map used to hold both
// kinds as bare strings, so a header value and a backend ID shared one
// namespace; the struct keeps them apart.
type threadKey struct {
	header email.HeaderID
	id     email.ID
}

func threadKeyOf(em *email.Email) threadKey {
	if em.MessageID != "" {
		return threadKey{header: em.MessageID}
	}
	return threadKey{id: em.ID}
}

func threadNodeKey(n *email.ThreadNode) threadKey { return threadKeyOf(n.Email) }

// findThreadNode finds a message by its backend ID, which is unique within
// the folder being threaded.
func findThreadNode(nodes []*email.ThreadNode, emailID email.ID) *email.ThreadNode {
	for _, n := range nodes {
		if n.Email.ID == emailID {
			return n
		}
		if found := findThreadNode(n.Children, emailID); found != nil {
			return found
		}
	}
	return nil
}

func findShallowestUnexpandedDepth(n *email.ThreadNode, expanded map[threadKey]bool, depth int, minDepth *int) {
	if len(n.Children) == 0 {
		return
	}
	if !expanded[threadNodeKey(n)] {
		if *minDepth == -1 || depth < *minDepth {
			*minDepth = depth
		}
	}
	for _, child := range n.Children {
		findShallowestUnexpandedDepth(child, expanded, depth+1, minDepth)
	}
}

func collapseThreadSubtree(n *email.ThreadNode, expanded map[threadKey]bool) {
	delete(expanded, threadNodeKey(n))
	for _, child := range n.Children {
		collapseThreadSubtree(child, expanded)
	}
}

func expandThreadSubtreeToDepth(n *email.ThreadNode, expanded map[threadKey]bool, depth, maxDepth int) {
	if len(n.Children) == 0 {
		return
	}
	if depth <= maxDepth {
		expanded[threadNodeKey(n)] = true
	}
	for _, child := range n.Children {
		expandThreadSubtreeToDepth(child, expanded, depth+1, maxDepth)
	}
}

func (m *tuiModel) cycleEmailThreadSubtreeExpansion(emailID email.ID) {
	roots := email.BuildThreadTree(m.rawEmails)
	target := findThreadNode(roots, emailID)
	if target == nil || len(target.Children) == 0 {
		return
	}

	minUnexpandedDepth := -1
	findShallowestUnexpandedDepth(target, m.expandedThreads, 0, &minUnexpandedDepth)

	if minUnexpandedDepth == -1 {
		collapseThreadSubtree(target, m.expandedThreads)
	} else {
		expandThreadSubtreeToDepth(target, m.expandedThreads, 0, minUnexpandedDepth)
	}

	m.rebuildVisibleEmails()
	for i, e := range m.emails {
		if e.ID == emailID {
			m.eIdx = i
			break
		}
	}
}
