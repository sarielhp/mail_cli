package mailclient

import (
	"context"
	"errors"

	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/label"
)

// ErrInteractiveAuthRequired reports that an account needs a sign-in that a
// non-interactive process cannot perform.
var ErrInteractiveAuthRequired = errors.New("account needs re-authentication")

// Role interfaces — consumers depend on the smallest relevant one.
//
// A folder is a label.Folder wherever one is passed in or handed back, so a
// name reaches a backend only after something resolved it against the server
// (label.FromServer) or checked it as a name to create (label.ForCreate). The
// listings stay strings: they are what resolution matches against.

// LabelReader is the read-only view of an account's folders: the names that
// match a prefix, the folders as listable items, and which folder is the inbox.
type LabelReader interface {
	GetMatchingLabels(prefix string) ([]string, error)
	GetLabelItems() ([]cfg_acc.Item, error)
	InboxFolder() label.Folder
}

type labelWriter interface {
	RenameLabel(oldName, newName label.Folder) error
	FixLabels() error
	DeleteLabel(name label.Folder) error
	EnsureLabelExists(name label.Folder) error
}

// EmailFetcher brings messages into the local cache: a whole folder's contents,
// or the newest messages across every folder of the account.
type EmailFetcher interface {
	FetchAndDownloadEmails(folder label.Folder, cacheSubdir string) ([]email.ID, error)
	FetchLatestAccountEmails(limit int) ([]cfg_acc.MessageFolderRef, error)
}

// MessageAccessor retrieves messages by ID without the caller knowing which
// folder holds them, whether they are cached, or how to download them.
//
// It is the consumer-facing half of the read-through cache: implementations
// read from the local cache when they can and fetch and store only what is
// missing. Callers no longer need a folder prefix to see a message, nor their
// own download loop -- which is what forced `show` to take a label argument and
// what led each backend to grow a private worker pool.
type MessageAccessor interface {
	// GetRawMessage returns the full RFC 5322 payload for a message ID.
	GetRawMessage(msgID email.ID) ([]byte, error)

	// GetRawMessagesBatch returns payloads for several IDs, in the order given.
	// An entry is nil when that message could not be retrieved, and the error
	// reports how many failed, so a partial result is still usable.
	GetRawMessagesBatch(msgIDs []email.ID) ([][]byte, error)

	// GetMessageSummary returns parsed header metadata for one message.
	GetMessageSummary(msgID email.ID) (*email.Email, error)

	// GetMessageSummariesBatch returns header metadata for several IDs, with
	// the same ordering and partial-result rules as GetRawMessagesBatch.
	GetMessageSummariesBatch(msgIDs []email.ID) ([]*email.Email, error)

	// PrefetchMessages makes sure every ID is cached, downloading only what is
	// missing. Unlike the batch getters it never holds more than one message in
	// memory, so it is what an unbounded set -- a folder sync, or an export of
	// every search match -- should use before reading the messages back one at
	// a time.
	PrefetchMessages(msgIDs []email.ID) error

	// GetRawMessageContext returns the full RFC 5322 payload for a message ID, respecting ctx.
	GetRawMessageContext(ctx context.Context, msgID email.ID) ([]byte, error)

	// GetRawMessagesBatchContext returns payloads for several IDs, respecting ctx.
	GetRawMessagesBatchContext(ctx context.Context, msgIDs []email.ID) ([][]byte, error)

	// GetMessageSummaryContext returns parsed header metadata for one message, respecting ctx.
	GetMessageSummaryContext(ctx context.Context, msgID email.ID) (*email.Email, error)

	// GetMessageSummariesBatchContext returns header metadata for several IDs, respecting ctx.
	GetMessageSummariesBatchContext(ctx context.Context, msgIDs []email.ID) ([]*email.Email, error)

	// PrefetchMessagesContext makes sure every ID is cached, respecting ctx.
	PrefetchMessagesContext(ctx context.Context, msgIDs []email.ID) error
}

// EmailSearcher runs a search on the server, across the account's whole
// history rather than the subset that happens to be cached locally.
//
// The server already maintains a full-text index over years of mail; querying
// it is both faster and more complete than anything that could be built over
// the local cache, which holds only what has been downloaded.
type EmailSearcher interface {
	// SearchEmails returns the IDs of matching messages, newest match first as
	// the backend ranks them. A limit of zero or less means every match, across
	// as many pages as the backend needs.
	SearchEmails(query string, limit int) ([]email.ID, error)
}

// EmailWriter is the mutating side of message handling: move, copy, mark read,
// return to the inbox, and upload a new message.
type EmailWriter interface {
	MoveEmail(messageIDs []email.ID, source, dest label.Folder) error
	CopyEmail(messageIDs []email.ID, source, dest label.Folder) error
	MarkAsRead(messageIDs []email.ID) error
	MoveToInbox(messageIDs []email.ID, source label.Folder) error
	UploadRawEmail(rawBytes []byte, target label.Folder) (email.ID, error)
	// DeleteMessages permanently deletes messages, bypassing the trash. It
	// is for a copy whose content must not survive anywhere -- a draft
	// uploaded in the clear before its session was encrypted -- and is not
	// a way to tidy up: a move to the trash is recoverable, this is not.
	DeleteMessages(messageIDs []email.ID, folder label.Folder) error
}

// SpamManager is the spam folder's mutating surface: report messages as spam,
// move or permanently delete the folder's contents, run the political-spam
// audit or action, and train the filter on what the folder holds.
type SpamManager interface {
	ReportSpam(messageIDs []email.ID, source label.Folder) error
	MoveAllSpam(dest label.Folder) error
	DeleteAllSpam() error
	// ShowPoliticalSpam reports political mail in the spam folder. act decides
	// whether it may also do something about it -- blacklist the sender, send
	// an opt-out, delete what it unsubscribed from. An audit passes false and
	// changes nothing, anywhere.
	ShowPoliticalSpam(act bool) error
	LearnSpam() error
}

// Filter is one server-side mail rule as a listing shows it. The strings are
// already human-readable: what a rule matches and what it does differ enough
// between backends that a structured form would be three shapes, not one.
type Filter struct {
	// ID identifies the rule to the user -- Gmail's filter ID, Outlook's
	// display name, an index for a local rule.
	ID string

	// RemoteID is the raw server identifier used for mutations/deletion
	// (e.g. Gmail filter ID, Outlook Graph rule ID).
	RemoteID string

	// Status is an optional qualifier shown beside the ID. Outlook reports
	// enabled or disabled, local rules report whether they have been exported,
	// and Gmail has no such notion and leaves it empty.
	Status string

	Match  string
	Action string

	// Structured criteria for matching against local rules.
	Sender      string
	Subject     string
	TargetLabel string
}

// FilterListing is a backend's answer to "what rules are in force".
type FilterListing struct {
	// Title heads the listing, naming the backend.
	Title string

	// Note explains something the user needs to know to read the list -- JMAP
	// servers have no server-side filters at all, so what is shown there is the
	// local routing rules instead.
	Note string

	Filters []Filter
}

// RuleReader inspects the rules in force without changing anything.
type RuleReader interface {
	GetFilters() (FilterListing, error)
	RewriteRuleLabelCasing(label string) string
}

// RuleWriter applies rules to mail, or pushes them to the server.
type RuleWriter interface {
	CheckAndApplyRules(messageIDs []email.ID, source label.Folder, cacheSubdir string) ([]email.ID, error)
	ExportRules() error
	DeleteFilter(id string) error
}

// RuleManager is the full server-rule surface, combining inspection with the
// ability to apply rules to mail or push them to the server.
type RuleManager interface {
	RuleReader
	RuleWriter
}

// ConfigProvider gives access to the global config.
type ConfigProvider interface {
	Config() *cfg_g.Config
}

// BackendInfo exposes backend identity.
type BackendInfo interface {
	BackendType() string
}

// Reader is everything that cannot change server state.
//
// The split exists for ReadOnlyMailClient. It embeds Reader, so a read method
// added here passes through for free, while a method added to Mutator fails to
// compile until the decorator decides whether to block or simulate it. That is
// the safe direction: forgetting to forward a read is an inconvenience,
// forgetting to block a write is a wrong answer to --read-only.
type Reader interface {
	Validate() error
	LabelReader
	EmailFetcher
	EmailSearcher
	MessageAccessor
	RuleReader
	ConfigProvider
	BackendInfo
}

// Mutator is everything that can change server state.
type Mutator interface {
	labelWriter
	EmailWriter
	SpamManager
	RuleWriter
	SendEmail(rawBytes []byte) error
}

// MailClient is the full union interface for all backend operations.
// Consumers should prefer the narrower role interfaces above.
type MailClient interface {
	Reader
	Mutator
}
