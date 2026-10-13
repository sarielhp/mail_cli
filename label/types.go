package label

import "github.com/sarielhp/mail_cli/email"

// Item is one folder or label as the server reports it: its display name, full
// path, message counts and whether it is a label or a server folder.
type Item struct {
	Name           string
	FullName       string
	MessagesTotal  int64
	MessagesUnread int64
	IsLabel        bool

	// IsSystem marks a folder the server provides rather than one the user
	// made -- Gmail's INBOX, SENT and the rest. A listing shows these apart
	// from the user's own hierarchy, which was previously done by each backend
	// printing its own two-section output instead of saying which was which.
	IsSystem bool
}

// TreeNode is one node of a label hierarchy, holding its own counts and its
// children in display order.
type TreeNode struct {
	Name           string
	FullName       string
	MessagesTotal  int64
	MessagesUnread int64
	IsLabel        bool
	Children       []*TreeNode
	Expanded       bool
}

// TreeEntry is one flattened tree row for rendering: the node, its depth, its
// text and the prefix that draws its place in the hierarchy.
type TreeEntry struct {
	Node   *TreeNode
	Depth  int
	Text   string
	Prefix string
}

// MessageFolderRef pairs a message ID with its associated folder name.
type MessageFolderRef struct {
	MessageID email.ID
	Folder    string
}
