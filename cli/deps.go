package cli

import (
	"context"
	"errors"

	"github.com/sarielhp/mail_cli/cache/cachedir"
	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/label"
	"github.com/sarielhp/mail_cli/last"
	"github.com/sarielhp/mail_cli/mailclient"
	"github.com/sarielhp/mail_cli/organize"
	"github.com/sarielhp/mail_cli/outbox"
	"github.com/sarielhp/mail_cli/scan"
	"github.com/sarielhp/mail_cli/show"
	"github.com/sarielhp/mail_cli/watcher"
)

// The CLI declares here what it needs from the layers beneath it, grouped by
// command family. Implementations live in mailops and the domain packages and
// are bound in main's composition root; because Go interfaces are structural,
// neither side imports the other.
//
// This replaces a struct of 30 nil-able function fields, which meant every
// handler had to check its dependency was wired before using it and no
// mis-wiring was caught until the command ran.

// ErrNotConfigured reports a dependency that was never bound. Reaching it is a
// wiring bug, not a user error.
var ErrNotConfigured = errors.New("command dependency not configured")

// ClientProvider resolves the backend client for an account.
type ClientProvider interface {
	ForSelected(config *cfg_g.Config) (mailclient.MailClient, error)
	ForAccountIndex(config *cfg_g.Config, index int) (mailclient.MailClient, error)
	PreCheck(config *cfg_g.Config) error
}

// ExportOptions contains parameters for exporting emails or conversation threads.
type ExportOptions struct {
	MessageID string
	Account   string
	Output    string
	Format    string // "mbox" or "eml"
	Single    bool
	DryRun    bool
}

// PipeOptions contains parameters for piping an email to an external command.
type PipeOptions struct {
	MessageID   string
	Account     string
	BodyOnly    bool
	HeadersOnly bool
}

// MailOps runs the commands that read and triage mail across accounts.
type MailOps interface {
	Scan(opts scan.Options, config *cfg_g.Config, labelPrefix string) (int, error)
	Show(opts show.Options, config *cfg_g.Config, labelPrefix, msgID string) error
	Last(opts last.Options, config *cfg_g.Config, n int) error
	Export(config *cfg_g.Config, opts ExportOptions) error
	Pipe(config *cfg_g.Config, opts PipeOptions, cmdArgs []string) error
	RunBackendTests(config *cfg_g.Config) error
}

// SpamOps covers spam classification and the training ledger.
type SpamOps interface {
	MarkByID(config *cfg_g.Config, id string) error
	UnspamByID(config *cfg_g.Config, id string) error
	UnspamFolder(config *cfg_g.Config, folder label.Folder) error
	LearnHam(config *cfg_g.Config, folder label.Folder, force bool) error
	ResetLearning(config *cfg_g.Config) error
	ForgetByID(config *cfg_g.Config, refs []string) error
	ForgetFolder(config *cfg_g.Config, folder label.Folder) error
}

// RuleOps moves routing rules between the config and the outside world.
type RuleOps interface {
	Export(config *cfg_g.Config, path string) error
	Import(config *cfg_g.Config, path string) error
	ExportSieve(config *cfg_g.Config, path string) error
}

// ConfigOps reads and edits the configuration file.
type ConfigOps interface {
	Show(config *cfg_g.Config) error
	Validate(config *cfg_g.Config) error
	Set(config *cfg_g.Config, key, value string) error
	Reset(config *cfg_g.Config, key string) error
	Get(config *cfg_g.Config, key string) error
}

// OrganizeOps files mail into archive, trash, drafts and sent folders.
type OrganizeOps interface {
	All(config *cfg_g.Config, client mailclient.MailClient, sourcePrefix string, target label.Folder) error
	ByID(config *cfg_g.Config, client mailclient.MailClient, target label.Folder, id string) error
	ResolveArchiveTarget(client mailclient.MailClient) (label.Folder, error)
	ResolveTrashTarget(client mailclient.MailClient) (label.Folder, error)
	ResolveDraftsTarget(client mailclient.MailClient) (label.Folder, error)
	ResolveSentTarget(client mailclient.MailClient) (label.Folder, error)
	// ListServerDrafts lists the account's Drafts folder and names it.
	ListServerDrafts(client mailclient.MailClient) (folder label.Folder, drafts []organize.ServerDraft, err error)
	// PlanServerPrune says what a prune of the Drafts folder would trash,
	// and how many drafts the folder holds. It changes nothing.
	PlanServerPrune(client mailclient.MailClient, opts organize.PruneOptions) (folder label.Folder, total int, plan organize.PrunePlan, err error)
	// FindServerDrafts resolves draft IDs as the listing shows them; unknown
	// holds names that match nothing or more than one draft.
	FindServerDrafts(client mailclient.MailClient, names []string) (folder label.Folder, matched []organize.ServerDraft, unknown []string, err error)
	// TrashServerDrafts moves drafts to the trash and names the trash folder.
	TrashServerDrafts(client mailclient.MailClient, folder label.Folder, ids []email.ID) (trash label.Folder, err error)
}

// CalendarOps adds .ics attachments to a calendar and reports the week.
type CalendarOps interface {
	Add(config *cfg_g.Config, client mailclient.MailClient, labelPrefix, msgID string) error
	AddAll(config *cfg_g.Config, client mailclient.MailClient) error
	Week(config *cfg_g.Config, days ...int) error
}

// AccountOps covers the two places a command has to know which backend an
// account uses: where that backend keeps its OAuth token, and how to open a
// local mbox file as a client. Both are factory decisions, so they belong
// behind this interface rather than in an import of gmail, outlook and mbox.
type AccountOps interface {
	// TokenPath is where the account's OAuth token lives, or "" for a backend
	// that has none.
	TokenPath(configDir string, acc cfg_acc.AccountConfig) (string, error)

	// OpenMbox builds a read-only client over a local mbox file, caching into
	// cacheDir.
	OpenMbox(config *cfg_g.Config, mboxPath string, cacheDir cachedir.Account) (mailclient.MailClient, error)
}

// ComposeOptions configures an interactive compose session.
type ComposeOptions struct {
	To          []string
	Cc          []string
	Bcc         []string
	Subject     string
	Body        string
	Attachments []string
	Account     string
	InReplyTo   string
	References  string
	OriginalID  email.ID
	// DraftPath resumes a saved local draft instead of starting empty.
	DraftPath string
}

// TUILauncher starts the interactive client.
type TUILauncher interface {
	Run(config *cfg_g.Config, labelPrefix string, initialNotice ...string) error
	RunWithClient(config *cfg_g.Config, client mailclient.MailClient, labelPrefix string) error
	Compose(config *cfg_g.Config, opts ComposeOptions) error
}

// ContactOps manages importing, syncing, listing, searching, and suggesting contacts and aliases.
type ContactOps interface {
	List(config *cfg_g.Config, filter string) error
	Search(config *cfg_g.Config, query string) error
	ImportMutt(config *cfg_g.Config, customPath string) error
	SyncGoogle(config *cfg_g.Config, accountName string) error
	Path(config *cfg_g.Config) error
	Suggest(config *cfg_g.Config, query string) error
	Harvest(config *cfg_g.Config) error
	Purge(config *cfg_g.Config, email string) error
}

// SendOptions contains parameters for an outbound email send.
type SendOptions struct {
	To          []string
	Cc          []string
	Bcc         []string
	Subject     string
	Body        string
	Attachments []string
	Account     string
	DryRun      bool
	Raw         bool
	RawBytes    []byte
	Queue       bool
	// Sign asks for the message body to be PGP-signed.
	Sign bool
	// Encrypt asks for the message body to be PGP-encrypted to the recipients.
	Encrypt bool
	// PgpKeyID names the signing key; empty falls back to the account's key
	// and then to gpg's default.
	PgpKeyID string
}

// ReplyOptions contains parameters for replying to an email.
type ReplyOptions struct {
	MessageID   string
	Account     string
	Group       bool
	Subject     string
	Cc          []string
	Bcc         []string
	Attachments []string
	Body        string
	IncludeFile string
	Headless    bool
	NoQuote     bool
	DryRun      bool
}

// SendOps delivers outbound mail.
type SendOps interface {
	Send(config *cfg_g.Config, opts SendOptions) error
	Reply(config *cfg_g.Config, opts ReplyOptions) error
}

// OutboxOps manages the offline send spool.
type OutboxOps interface {
	List(config *cfg_g.Config) ([]outbox.Entry, error)
	Flush(config *cfg_g.Config) ([]outbox.FlushResult, error)
	Remove(config *cfg_g.Config, id string) error
}

// WatchOps runs the background mail watcher.
type WatchOps interface {
	Watch(ctx context.Context, config *cfg_g.Config, opts watcher.Options) error
}

// Deps is everything the CLI needs from below. A nil member is not a crash: the
// accessors on Session substitute a stub that reports ErrNotConfigured, so a
// handler can call its dependency without first checking it exists.
type Deps struct {
	Clients  ClientProvider
	Mail     MailOps
	Spam     SpamOps
	Rules    RuleOps
	Config   ConfigOps
	Organize OrganizeOps
	Calendar CalendarOps
	Accounts AccountOps
	TUI      TUILauncher
	Contacts ContactOps
	Send     SendOps
	Outbox   OutboxOps
	Watch    WatchOps
}

// One stub per interface, rather than one type implementing all of them:
// ConfigOps.Show and MailOps.Show share a name but not a signature, and a
// single stub cannot have both.

type noClients struct{}

func (noClients) ForSelected(*cfg_g.Config) (mailclient.MailClient, error) {
	return nil, ErrNotConfigured
}

func (noClients) ForAccountIndex(*cfg_g.Config, int) (mailclient.MailClient, error) {
	return nil, ErrNotConfigured
}

// PreCheck succeeds when unbound. The callback it replaces was optional --
// handlers ran it only `if session.PreCheck != nil` -- so "no pre-flight check
// configured" has always meant "nothing to check", not "fail".
func (noClients) PreCheck(*cfg_g.Config) error { return nil }

type noMail struct{}

func (noMail) Scan(scan.Options, *cfg_g.Config, string) (int, error) {
	return 0, ErrNotConfigured
}
func (noMail) Show(show.Options, *cfg_g.Config, string, string) error { return ErrNotConfigured }
func (noMail) Last(last.Options, *cfg_g.Config, int) error            { return ErrNotConfigured }
func (noMail) Export(*cfg_g.Config, ExportOptions) error              { return ErrNotConfigured }
func (noMail) Pipe(*cfg_g.Config, PipeOptions, []string) error        { return ErrNotConfigured }
func (noMail) RunBackendTests(*cfg_g.Config) error                    { return ErrNotConfigured }

type noSpam struct{}

func (noSpam) MarkByID(*cfg_g.Config, string) error             { return ErrNotConfigured }
func (noSpam) UnspamByID(*cfg_g.Config, string) error           { return ErrNotConfigured }
func (noSpam) UnspamFolder(*cfg_g.Config, label.Folder) error   { return ErrNotConfigured }
func (noSpam) LearnHam(*cfg_g.Config, label.Folder, bool) error { return ErrNotConfigured }
func (noSpam) ResetLearning(*cfg_g.Config) error                { return ErrNotConfigured }
func (noSpam) ForgetByID(*cfg_g.Config, []string) error         { return ErrNotConfigured }
func (noSpam) ForgetFolder(*cfg_g.Config, label.Folder) error   { return ErrNotConfigured }

type noRules struct{}

func (noRules) Export(*cfg_g.Config, string) error      { return ErrNotConfigured }
func (noRules) Import(*cfg_g.Config, string) error      { return ErrNotConfigured }
func (noRules) ExportSieve(*cfg_g.Config, string) error { return ErrNotConfigured }

type noConfig struct{}

func (noConfig) Show(*cfg_g.Config) error                { return ErrNotConfigured }
func (noConfig) Validate(*cfg_g.Config) error            { return ErrNotConfigured }
func (noConfig) Set(*cfg_g.Config, string, string) error { return ErrNotConfigured }
func (noConfig) Reset(*cfg_g.Config, string) error       { return ErrNotConfigured }
func (noConfig) Get(*cfg_g.Config, string) error         { return ErrNotConfigured }

type noOrganize struct{}

func (noOrganize) All(*cfg_g.Config, mailclient.MailClient, string, label.Folder) error {
	return ErrNotConfigured
}

func (noOrganize) ByID(*cfg_g.Config, mailclient.MailClient, label.Folder, string) error {
	return ErrNotConfigured
}

func (noOrganize) ResolveArchiveTarget(mailclient.MailClient) (label.Folder, error) {
	return label.Folder{}, ErrNotConfigured
}
func (noOrganize) ResolveTrashTarget(mailclient.MailClient) (label.Folder, error) {
	return label.Folder{}, ErrNotConfigured
}
func (noOrganize) ResolveDraftsTarget(mailclient.MailClient) (label.Folder, error) {
	return label.Folder{}, ErrNotConfigured
}
func (noOrganize) ResolveSentTarget(mailclient.MailClient) (label.Folder, error) {
	return label.Folder{}, ErrNotConfigured
}

func (noOrganize) ListServerDrafts(mailclient.MailClient) (label.Folder, []organize.ServerDraft, error) {
	return label.Folder{}, nil, ErrNotConfigured
}

func (noOrganize) PlanServerPrune(mailclient.MailClient, organize.PruneOptions) (label.Folder, int, organize.PrunePlan, error) {
	return label.Folder{}, 0, organize.PrunePlan{}, ErrNotConfigured
}

func (noOrganize) FindServerDrafts(mailclient.MailClient, []string) (label.Folder, []organize.ServerDraft, []string, error) {
	return label.Folder{}, nil, nil, ErrNotConfigured
}

func (noOrganize) TrashServerDrafts(mailclient.MailClient, label.Folder, []email.ID) (label.Folder, error) {
	return label.Folder{}, ErrNotConfigured
}

type noCalendar struct{}

func (noCalendar) Add(*cfg_g.Config, mailclient.MailClient, string, string) error {
	return ErrNotConfigured
}
func (noCalendar) AddAll(*cfg_g.Config, mailclient.MailClient) error { return ErrNotConfigured }
func (noCalendar) Week(*cfg_g.Config, ...int) error                  { return ErrNotConfigured }

type noAccounts struct{}

func (noAccounts) TokenPath(string, cfg_acc.AccountConfig) (string, error) {
	return "", ErrNotConfigured
}

func (noAccounts) OpenMbox(*cfg_g.Config, string, cachedir.Account) (mailclient.MailClient, error) {
	return nil, ErrNotConfigured
}

type noTUI struct{}

func (noTUI) Run(*cfg_g.Config, string, ...string) error { return ErrNotConfigured }

func (noTUI) RunWithClient(*cfg_g.Config, mailclient.MailClient, string) error {
	return ErrNotConfigured
}

func (noTUI) Compose(*cfg_g.Config, ComposeOptions) error {
	return ErrNotConfigured
}

type noContacts struct{}

func (noContacts) List(*cfg_g.Config, string) error       { return ErrNotConfigured }
func (noContacts) Search(*cfg_g.Config, string) error     { return ErrNotConfigured }
func (noContacts) ImportMutt(*cfg_g.Config, string) error { return ErrNotConfigured }
func (noContacts) SyncGoogle(*cfg_g.Config, string) error { return ErrNotConfigured }
func (noContacts) Path(*cfg_g.Config) error               { return ErrNotConfigured }
func (noContacts) Suggest(*cfg_g.Config, string) error    { return ErrNotConfigured }
func (noContacts) Harvest(*cfg_g.Config) error            { return ErrNotConfigured }
func (noContacts) Purge(*cfg_g.Config, string) error      { return ErrNotConfigured }

// Accessors. Each substitutes the stub for an unbound dependency, so the 41
// scattered nil checks this replaces live in one place.

func (s *Session) clients() ClientProvider {
	if s.Deps.Clients == nil {
		return noClients{}
	}
	return s.Deps.Clients
}

func (s *Session) mail() MailOps {
	if s.Deps.Mail == nil {
		return noMail{}
	}
	return s.Deps.Mail
}

func (s *Session) spam() SpamOps {
	if s.Deps.Spam == nil {
		return noSpam{}
	}
	return s.Deps.Spam
}

func (s *Session) rules() RuleOps {
	if s.Deps.Rules == nil {
		return noRules{}
	}
	return s.Deps.Rules
}

func (s *Session) configOps() ConfigOps {
	if s.Deps.Config == nil {
		return noConfig{}
	}
	return s.Deps.Config
}

func (s *Session) organize() OrganizeOps {
	if s.Deps.Organize == nil {
		return noOrganize{}
	}
	return s.Deps.Organize
}

func (s *Session) calendar() CalendarOps {
	if s.Deps.Calendar == nil {
		return noCalendar{}
	}
	return s.Deps.Calendar
}

func (s *Session) accounts() AccountOps {
	if s.Deps.Accounts == nil {
		return noAccounts{}
	}
	return s.Deps.Accounts
}

func (s *Session) tui() TUILauncher {
	if s.Deps.TUI == nil {
		return noTUI{}
	}
	return s.Deps.TUI
}

func (s *Session) contacts() ContactOps {
	if s.Deps.Contacts == nil {
		return noContacts{}
	}
	return s.Deps.Contacts
}

type noSend struct{}

func (noSend) Send(*cfg_g.Config, SendOptions) error   { return ErrNotConfigured }
func (noSend) Reply(*cfg_g.Config, ReplyOptions) error { return ErrNotConfigured }

func (s *Session) send() SendOps {
	if s.Deps.Send == nil {
		return noSend{}
	}
	return s.Deps.Send
}

type noOutbox struct{}

func (noOutbox) List(*cfg_g.Config) ([]outbox.Entry, error)        { return nil, ErrNotConfigured }
func (noOutbox) Flush(*cfg_g.Config) ([]outbox.FlushResult, error) { return nil, ErrNotConfigured }
func (noOutbox) Remove(*cfg_g.Config, string) error                { return ErrNotConfigured }

type noWatch struct{}

func (noWatch) Watch(context.Context, *cfg_g.Config, watcher.Options) error {
	return ErrNotConfigured
}

func (s *Session) outbox() OutboxOps {
	if s.Deps.Outbox == nil {
		return noOutbox{}
	}
	return s.Deps.Outbox
}

func (s *Session) watch() WatchOps {
	if s.Deps.Watch == nil {
		return noWatch{}
	}
	return s.Deps.Watch
}
