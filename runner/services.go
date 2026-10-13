package runner

import (
	"fmt"
	"github.com/sarielhp/mail_cli/cache"
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"io"
	"mime"
	"os"
	"path/filepath"

	"github.com/sarielhp/mail_cli/app"
	"github.com/sarielhp/mail_cli/calendar"
	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/cfghandler"
	"github.com/sarielhp/mail_cli/cli"
	"github.com/sarielhp/mail_cli/contacts"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/label"
	"github.com/sarielhp/mail_cli/last"
	"github.com/sarielhp/mail_cli/mailclient"
	"github.com/sarielhp/mail_cli/mailops"
	"github.com/sarielhp/mail_cli/organize"
	"github.com/sarielhp/mail_cli/report"
	"github.com/sarielhp/mail_cli/scan"
	"github.com/sarielhp/mail_cli/show"
	"github.com/sarielhp/mail_cli/sieve"
	"github.com/sarielhp/mail_cli/spam"
	"github.com/sarielhp/mail_cli/tui"
)

// The service types below implement the interfaces the CLI declares, one per
// command family. They are named nowhere in this package's imports: Go
// interfaces are structural, so mailops never has to import cli, and cli never
// has to import mailops. main binds the two.

// rep is Rep, or a discarding reporter when the service was built without one.
// Without this a service constructed as a bare struct literal -- which is easy
// to do and compiles -- passes nil into code that dereferences it.

func (s ClientService) rep() report.Reporter   { return report.Or(s.Rep) }
func (s MailService) rep() report.Reporter     { return report.Or(s.Rep) }
func (s SpamService) rep() report.Reporter     { return report.Or(s.Rep) }
func (s RuleService) rep() report.Reporter     { return report.Or(s.Rep) }
func (s OrganizeService) rep() report.Reporter { return report.Or(s.Rep) }
func (s CalendarService) rep() report.Reporter { return report.Or(s.Rep) }

// ClientService resolves backend clients for the CLI. Rep is where the clients
// it builds report their progress.
type ClientService struct {
	Rep report.Reporter
}

// ForSelected returns a client for the selected account, falling back to the
// first configured account when none is selected, and errors when no account
// is configured or the client cannot be built.
func (s ClientService) ForSelected(config *cfg_g.Config) (mailclient.MailClient, error) {
	return mailops.GetClientForSelected(config, s.rep())
}

// ForAccountIndex returns a client for the 1-based account index, and errors
// when the index falls outside the configured accounts.
func (s ClientService) ForAccountIndex(config *cfg_g.Config, index int) (mailclient.MailClient, error) {
	return mailops.GetClientForAccountIndex(config, index, s.rep())
}

// PreCheck verifies the host has the external tools the mail commands need --
// presently bogofilter -- and reports what to install when one is missing.
func (ClientService) PreCheck(_ *cfg_g.Config) error {
	return app.RunPreFlightCheck()
}

// MailService runs the account-spanning read commands.
//
// Out is where a command's product goes -- the rendered message, the listing --
// which is a different channel from Rep, where it says what it is doing. Only
// one of the two may be suppressed.
type MailService struct {
	Rep report.Reporter
	Out io.Writer
}

// Scan runs the spam scan over the folders named by labelPrefix, across every
// account it names, and returns how many messages the rules moved.
func (s MailService) Scan(opts scan.Options, config *cfg_g.Config, labelPrefix string) (int, error) {
	return RunScanOnAccounts(s.out(), opts, config, labelPrefix, s.rep())
}

// Show renders the message or folder listing named by labelPrefix and msgID,
// writing the product to Out. A reference without the label sigil is treated
// as a message ID.
func (s MailService) Show(opts show.Options, config *cfg_g.Config, labelPrefix, msgID string) error {
	return RunShowOnAccounts(opts, s.out(), config, labelPrefix, msgID, s.rep())
}

// Last prints the n most recent messages of the selected account.
func (s MailService) Last(opts last.Options, config *cfg_g.Config, n int) error {
	return RunLastOnAccounts(opts, s.out(), config, n, s.rep())
}

// Export writes the message or thread named by opts to an mbox or eml file.
func (s MailService) Export(config *cfg_g.Config, opts cli.ExportOptions) error {
	return RunExport(s.out(), config, opts, s.rep())
}

// Pipe feeds the message content selected by opts to an external command, with
// the command's standard output on Out and its standard error on standard error.
func (s MailService) Pipe(config *cfg_g.Config, opts cli.PipeOptions, cmdArgs []string) error {
	return RunPipe(s.out(), os.Stderr, config, opts, cmdArgs, s.rep())
}

// out is Out, defaulting to standard output.
func (s MailService) out() io.Writer {
	if s.Out == nil {
		return os.Stdout
	}
	return s.Out
}

// RunBackendTests exercises the selected account's Gmail REST surface, creating
// and deleting a label and a message to prove the credentials can write. It is
// refused under --read-only and --dry-run.
func (s MailService) RunBackendTests(config *cfg_g.Config) error {
	return mailops.RunBackendTests(s.rep(), config)
}

// SpamService classifies mail and maintains the training ledger.
type SpamService struct {
	Rep report.Reporter
}

// withClient resolves the selected account's client and hands it to fn. Every
// method below opens with the same two lines otherwise.
func (s SpamService) withClient(config *cfg_g.Config, fn func(mailclient.MailClient) error) error {
	client, err := mailops.GetClientForSelected(config, s.rep())
	if err != nil {
		return err
	}
	return fn(client)
}

// MarkByID reports the message ref names as spam and drops its sender from
// the contact suggestions.
func (s SpamService) MarkByID(config *cfg_g.Config, ref string) error {
	return s.withClient(config, func(c mailclient.MailClient) error {
		id := cachedID(c.Config().AccountCache, ref)
		err := spam.MarkByID(s.rep(), c, config, ref)
		if err == nil && id != "" {
			contacts.PurgeMessageSenders(config, c.Config().AccountCache, []email.ID{id})
		}
		return err
	})
}

// cachedID resolves ref, as the user typed it -- full or short -- to the cached
// message it names, or "" when the cache does not know it. The contact
// bookkeeping after a spam report or an archive read the message by ref
// itself, so with the short ID a listing shows it found nothing and silently
// did nothing.
func cachedID(account cachedir.Account, ref string) email.ID {
	id, _, err := cache.FindCachedEmailByID(account, ref)
	if err != nil {
		return ""
	}
	return id
}

// UnspamByID trains the message ref names as ham, clears its spam evidence and
// moves it back to the inbox; under --read-only it reports the move instead.
func (s SpamService) UnspamByID(config *cfg_g.Config, ref string) error {
	return s.withClient(config, func(c mailclient.MailClient) error {
		return spam.UnspamByID(s.rep(), c, config, ref)
	})
}

// UnspamFolder trains every message in folder as ham and moves the ones that
// trained back to the inbox; under --read-only it reports the move instead.
func (s SpamService) UnspamFolder(config *cfg_g.Config, folder label.Folder) error {
	return s.withClient(config, func(c mailclient.MailClient) error {
		return spam.UnspamFolder(s.rep(), c, config, folder)
	})
}

// LearnHam trains bogofilter on folder's messages as ham. force retrains the
// whole folder rather than only the messages the ledger has not seen.
func (s SpamService) LearnHam(config *cfg_g.Config, folder label.Folder, force bool) error {
	return s.withClient(config, func(c mailclient.MailClient) error {
		if force {
			config.ForceLearn = true
		}
		return spam.LearnHam(s.rep(), c, config, folder)
	})
}

// ResetLearning resets the selected account: its cache directory, not the
// root above every account, which holds no messages to reset.
func (s SpamService) ResetLearning(config *cfg_g.Config) error {
	return s.withClient(config, func(c mailclient.MailClient) error {
		return spam.ResetLearning(s.rep(), c.Config())
	})
}

// ForgetByID drops the user's spam/not-spam decision on each message refs
// names. It is local: the client only resolves the account's cache.
func (s SpamService) ForgetByID(config *cfg_g.Config, refs []string) error {
	return s.withClient(config, func(c mailclient.MailClient) error {
		return spam.ForgetByID(s.rep(), c.Config().AccountCache, refs, config.DryRun)
	})
}

// ForgetFolder drops the user's spam or ham decision on every message in
// folder, leaving what bogofilter learned alone. dryRun reports without
// changing anything, and a folder with no cached index is an error.
func (s SpamService) ForgetFolder(config *cfg_g.Config, folder label.Folder) error {
	return s.withClient(config, func(c mailclient.MailClient) error {
		return spam.ForgetFolder(s.rep(), c.Config().AccountCache, folder, config.DryRun)
	})
}

// RuleService moves routing rules in and out of the config.
type RuleService struct {
	Rep report.Reporter
}

// Export writes the selected account's routing rules to path as JSON.
func (s RuleService) Export(config *cfg_g.Config, path string) error {
	return ExportRulesToFile(s.rep(), config, path)
}

// Import reads routing rules from the JSON file at path into the selected
// account.
func (s RuleService) Import(config *cfg_g.Config, path string) error {
	return ImportRulesFromFile(s.rep(), config, path)
}

// ExportSieve writes the selected account's rules to path as a Sieve script.
func (s RuleService) ExportSieve(config *cfg_g.Config, path string) error {
	return sieve.ExportScript(s.rep(), config, path)
}

// ConfigService reads and edits the configuration file.
// ConfigService reads and edits the configuration file. Its output is the
// report the user asked for, so it goes to Out rather than through a reporter.
type ConfigService struct {
	Out io.Writer
}

func (s ConfigService) out() io.Writer {
	if s.Out == nil {
		return os.Stdout
	}
	return s.Out
}

// Show writes the resolved configuration to Out.
func (s ConfigService) Show(config *cfg_g.Config) error {
	return cfghandler.HandleConfigShow(s.out(), config)
}

// Validate checks the configuration and writes any problems it finds to Out.
func (s ConfigService) Validate(config *cfg_g.Config) error {
	return cfghandler.PerformConfigValidation(s.out(), config)
}

// Set writes key's new value to the config file and reports the result on Out.
func (s ConfigService) Set(config *cfg_g.Config, key, value string) error {
	return cfghandler.HandleConfigSet(s.out(), config, key, value)
}

// Reset clears key back to its default and reports the result on Out.
func (s ConfigService) Reset(config *cfg_g.Config, key string) error {
	return cfghandler.HandleConfigReset(s.out(), config, key)
}

// Get writes the value of key to Out.
func (s ConfigService) Get(config *cfg_g.Config, key string) error {
	return cfghandler.HandleConfigGet(s.out(), config, key)
}

// OrganizeService files mail into archive, trash, drafts and sent.
type OrganizeService struct {
	Rep report.Reporter
}

// All archives every message under sourcePrefix into target, skipping the
// folder that equals target; under --read-only it reports instead of moving.
func (s OrganizeService) All(config *cfg_g.Config, client mailclient.MailClient, sourcePrefix string, target label.Folder) error {
	return organize.All(s.rep(), config, client, sourcePrefix, target)
}

// ByID archives the message ref names and records its sender as a contact.
func (s OrganizeService) ByID(config *cfg_g.Config, client mailclient.MailClient, target label.Folder, ref string) error {
	id := cachedID(client.Config().AccountCache, ref)
	err := organize.ArchiveByID(s.rep(), config, client, target, ref)
	if err == nil {
		contacts.RecordArchivedSender(config, client.Config().AccountCache, id)
	}
	return err
}

// ResolveArchiveTarget names the folder archived mail is filed into, from the
// server's listing or the backend's well-known default.
func (s OrganizeService) ResolveArchiveTarget(client mailclient.MailClient) (label.Folder, error) {
	return organize.ResolveArchiveTarget(client)
}

// ResolveTrashTarget names the folder deleted mail is moved to, from the
// server's listing or the backend's well-known default.
func (s OrganizeService) ResolveTrashTarget(client mailclient.MailClient) (label.Folder, error) {
	return organize.ResolveTrashTarget(client)
}

// ResolveDraftsTarget names the folder drafts are saved in, from the server's
// listing or the backend's well-known default.
func (s OrganizeService) ResolveDraftsTarget(client mailclient.MailClient) (label.Folder, error) {
	return organize.ResolveDraftsTarget(client)
}

// ResolveSentTarget names the folder sent mail is kept in, from the server's
// listing or the backend's well-known default.
func (s OrganizeService) ResolveSentTarget(client mailclient.MailClient) (label.Folder, error) {
	return organize.ResolveSentTarget(client)
}

// ListServerDrafts names the Drafts folder and returns its messages, newest
// first; a message whose headers cannot be read comes back marked unreadable.
func (s OrganizeService) ListServerDrafts(client mailclient.MailClient) (label.Folder, []organize.ServerDraft, error) {
	return organize.ListServerDrafts(client)
}

// PlanServerPrune names the Drafts folder and returns how many drafts it holds
// and which ones a prune would trash, changing nothing. opts.Sent also consults
// the Sent folder.
func (s OrganizeService) PlanServerPrune(client mailclient.MailClient, opts organize.PruneOptions) (label.Folder, int, organize.PrunePlan, error) {
	return organize.PlanServerPrune(client, opts)
}

// FindServerDrafts names the Drafts folder and resolves names against its
// drafts, returning the matches and any name that matched nothing or more than
// one draft. It changes nothing.
func (s OrganizeService) FindServerDrafts(client mailclient.MailClient, names []string) (label.Folder, []organize.ServerDraft, []string, error) {
	return organize.FindServerDrafts(client, names)
}

// TrashServerDrafts moves ids out of folder into the account's trash and names
// that trash folder; empty ids is a no-op.
func (s OrganizeService) TrashServerDrafts(client mailclient.MailClient, folder label.Folder, ids []email.ID) (label.Folder, error) {
	return organize.TrashServerDrafts(client, folder, ids)
}

// CalendarService adds .ics attachments to Google Calendar.
type CalendarService struct {
	Rep report.Reporter
}

// Add extracts an .ics attachment from the message named by labelPrefix and
// msgID and inserts the event into Google Calendar.
func (s CalendarService) Add(config *cfg_g.Config, client mailclient.MailClient, labelPrefix, msgID string) error {
	return GoogleCalendar(s.rep()).PerformCalendarAdd(config, client, labelPrefix, msgID)
}

// AddAll scans the inbox for messages with .ics attachments and adds any events
// not already present to Google Calendar.
func (s CalendarService) AddAll(config *cfg_g.Config, client mailclient.MailClient) error {
	return GoogleCalendar(s.rep()).PerformCalAddAll(config, client)
}

// Week prints the calendar's events for the next days -- seven by default --
// starting today.
func (s CalendarService) Week(config *cfg_g.Config, days ...int) error {
	return GoogleCalendar(s.rep()).PerformCalendarWeek(config, days...)
}

// TUIService launches the interactive client.
type TUIService struct {
	Rep report.Reporter
}

// Run builds the TUI's own reporter first, so the backend it wires reports
// progress as bubbletea messages instead of writing over the display. The
// reporter has nowhere to send until the program exists, which is why the TUI
// fills in its Send once it has one.
func (s TUIService) Run(config *cfg_g.Config, labelPrefix string, initialNotice ...string) error {
	rep := tui.NewProgramReporter(nil)
	notice := ""
	if len(initialNotice) > 0 {
		notice = initialNotice[0]
	}
	return tui.InitTuiMode(config, labelPrefix, notice, NewTuiBackend(config, rep), rep)
}

// RunWithClient launches the interactive client over an already-built client, so
// a caller that has one can skip resolving the account again.
func (s TUIService) RunWithClient(config *cfg_g.Config, client mailclient.MailClient, labelPrefix string) error {
	rep := tui.NewProgramReporter(nil)
	return tui.InitTuiWithClient(config, client, labelPrefix, "", NewTuiBackend(config, rep), rep)
}

// Compose opens the interactive compose screen, resolving the sending account
// from opts and loading its attachments from disk first.
func (s TUIService) Compose(config *cfg_g.Config, opts cli.ComposeOptions) error {
	rep := tui.NewProgramReporter(nil)
	client, acc, err := mailops.GetClientForSending(config, opts.Account, rep)
	if err != nil {
		return err
	}

	attachments, err := loadComposeAttachments(opts.Attachments)
	if err != nil {
		return err
	}

	bk := NewTuiBackend(config, rep)
	tuiOpts := tui.ComposeOptions{
		To:          expandRecipients(config, opts.To),
		Cc:          expandRecipients(config, opts.Cc),
		Bcc:         expandRecipients(config, opts.Bcc),
		Subject:     opts.Subject,
		Body:        opts.Body,
		Attachments: attachments,
		InReplyTo:   opts.InReplyTo,
		References:  opts.References,
		OriginalID:  opts.OriginalID,
		DraftPath:   opts.DraftPath,
	}

	cfgCopy := *config
	cfgCopy.SelectedAccount = cfg_g.NameOf(*acc)

	return tui.InitTuiCompose(&cfgCopy, client, tuiOpts, bk, rep)
}

func loadComposeAttachments(paths []string) ([]email.Attachment, error) {
	var attachments []email.Attachment
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading attachment %s: %w", path, err)
		}
		fn := filepath.Base(path)
		ct := mime.TypeByExtension(filepath.Ext(fn))
		if ct == "" {
			ct = "application/octet-stream"
		}
		attachments = append(attachments, email.Attachment{
			Filename:    fn,
			ContentType: ct,
			Data:        data,
		})
	}
	return attachments, nil
}

// Services is every implementation, for a caller that wants them all.
func Services() (ClientService, MailService, SpamService, RuleService, ConfigService, OrganizeService, CalendarService, TUIService) {
	return ClientService{}, MailService{}, SpamService{}, RuleService{}, ConfigService{}, OrganizeService{}, CalendarService{}, TUIService{}
}

// tuiClientService is ClientService with the by-account lookup the TUI needs to
// switch accounts without going through the selected-account config field.
type tuiClientService struct {
	ClientService
}

func (s tuiClientService) ForAccount(config *cfg_g.Config, acc cfg_acc.AccountConfig) (mailclient.MailClient, error) {
	return mailops.NewMailClient(acc, config, s.rep())
}

// AddICS adds an .ics payload to Google Calendar.
func (s CalendarService) AddICS(config *cfg_g.Config, icsData []byte) (string, bool, error) {
	return GoogleCalendar(s.rep()).AddICSDataToCalendar(config, icsData)
}

// CheckICS reports whether the events in an .ics payload are already present.
func (s CalendarService) CheckICS(config *cfg_g.Config, icsData []byte) (bool, string, *calendar.ICSEvent, error) {
	return GoogleCalendar(s.rep()).CheckICSEventInCalendar(config, icsData)
}

// AddICSVerified adds the events and reads them back, so the TUI can say
// whether the write landed and whether it created duplicates.
func (s CalendarService) AddICSVerified(config *cfg_g.Config, icsData []byte) (string, bool, bool, bool, error) {
	return GoogleCalendar(s.rep()).AddICSDataWithVerification(config, icsData)
}

// AddICSAsNew inserts events as separate new events without overwriting existing ones.
func (s CalendarService) AddICSAsNew(config *cfg_g.Config, icsData []byte) (string, error) {
	return GoogleCalendar(s.rep()).AddICSDataAsNew(config, icsData)
}

// CheckICSDetailed reports whether the event is in Calendar, whether it is an exact match,
// and details any discrepancies (e.g. rescheduled start time, changed duration, recurrence).
func (s CalendarService) CheckICSDetailed(config *cfg_g.Config, icsData []byte) (tui.CalendarCheckResult, error) {
	res, err := GoogleCalendar(s.rep()).CheckICSEventDetailed(config, icsData)
	return tui.CalendarCheckResult{
		InCalendar:  res.InCalendar,
		ExactMatch:  res.ExactMatch,
		Discrepancy: res.Discrepancy,
		Summary:     res.Summary,
		Event:       res.Event,
	}, err
}

// AccountService answers where an account's token lives and how to open a local
// mbox. Both delegate straight to the factory and report nothing, so unlike the
// other services this one carries no reporter.
type AccountService struct{}

// TokenPath reports where the account's OAuth token lives, or "" for a backend
// that has none.
func (AccountService) TokenPath(configDir string, acc cfg_acc.AccountConfig) (string, error) {
	return mailops.TokenPath(configDir, acc)
}

// OpenMbox builds the read-only, caching client stack over a local mbox file.
func (AccountService) OpenMbox(config *cfg_g.Config, mboxPath string, cacheDir cachedir.Account) (mailclient.MailClient, error) {
	return mailops.OpenMbox(config, mboxPath, cacheDir)
}
