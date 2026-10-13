package email

import (
	"strings"
	"time"
)

// Email is the summary of one message: its identity, decoded display headers,
// read/reply/spam flags and its position in a thread.
type Email struct {
	ID      ID
	Account string
	// AccountBadge and AccountIndex are display-only: the tag a listing
	// that mixes accounts shows for Account, and the account's position in
	// the configuration, which gives it a stable colour.
	AccountBadge        string
	AccountIndex        int
	Subject             string
	FromEmail           string
	FromRaw             string
	EmailDate           time.Time
	FormattedDate       string
	IsSpam              bool
	IsPolitical         bool
	IsBlacklisted       bool
	IsRead              bool
	IsReplied           bool
	HasICS              bool
	HasAttachment       bool
	MessageID           HeaderID
	InReplyTo           string
	References          string
	ThreadDepth         int
	ThreadHasReplies    bool
	ThreadCollapsed     bool
	ThreadRepliesCount  int
	ThreadSenderSummary string
	ThreadPrefix        string
	ThreadIndex         int
}

// ThreadNode is one message in a thread tree, together with its parent and its
// replies.
type ThreadNode struct {
	Email    *Email
	Children []*ThreadNode
	Parent   *ThreadNode
}

// HeaderID is one RFC 5322 message identifier, `<local@domain>`, as found in
// Message-ID and listed in In-Reply-To and References. It is not Email.ID,
// the backend's identifier for the stored message: the two are both strings,
// and code that compared one against the other matched by accident or not at
// all. Keeping them distinct types makes that comparison a compile error.
type HeaderID string

// Same reports whether two identifiers name the same message, ignoring case
// as every dedup comparison here always has.
func (h HeaderID) Same(o HeaderID) bool { return strings.EqualFold(string(h), string(o)) }

// Folded is h in the form Same compares, for use as a map key.
func (h HeaderID) Folded() HeaderID { return HeaderID(strings.ToLower(string(h))) }
