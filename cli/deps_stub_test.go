package cli

import (
	"context"

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

// Test stubs for the dependency interfaces. Each has a func field per method;
// an unset field behaves as "not configured", matching what a real unbound
// dependency does, so a test only has to supply the one call it exercises.
//
// They are separate types rather than one, because ConfigOps.Show and
// MailOps.Show share a name but not a signature.

type stubClients struct {
	ForSelectedFn     func(*cfg_g.Config) (mailclient.MailClient, error)
	ForAccountIndexFn func(*cfg_g.Config, int) (mailclient.MailClient, error)
	PreCheckFn        func(*cfg_g.Config) error
}

func (s stubClients) ForSelected(c *cfg_g.Config) (mailclient.MailClient, error) {
	if s.ForSelectedFn == nil {
		return nil, ErrNotConfigured
	}
	return s.ForSelectedFn(c)
}

func (s stubClients) ForAccountIndex(c *cfg_g.Config, i int) (mailclient.MailClient, error) {
	if s.ForAccountIndexFn == nil {
		return nil, ErrNotConfigured
	}
	return s.ForAccountIndexFn(c, i)
}

func (s stubClients) PreCheck(c *cfg_g.Config) error {
	if s.PreCheckFn == nil {
		return nil
	}
	return s.PreCheckFn(c)
}

type stubMail struct {
	ScanFn            func(scan.Options, *cfg_g.Config, string) (int, error)
	ShowFn            func(show.Options, *cfg_g.Config, string, string) error
	LastFn            func(last.Options, *cfg_g.Config, int) error
	ExportFn          func(*cfg_g.Config, ExportOptions) error
	PipeFn            func(*cfg_g.Config, PipeOptions, []string) error
	RunBackendTestsFn func(*cfg_g.Config) error
}

func (s stubMail) Scan(o scan.Options, c *cfg_g.Config, p string) (int, error) {
	if s.ScanFn == nil {
		return 0, ErrNotConfigured
	}
	return s.ScanFn(o, c, p)
}

func (s stubMail) Show(o show.Options, c *cfg_g.Config, p, id string) error {
	if s.ShowFn == nil {
		return ErrNotConfigured
	}
	return s.ShowFn(o, c, p, id)
}

func (s stubMail) Last(o last.Options, c *cfg_g.Config, n int) error {
	if s.LastFn == nil {
		return ErrNotConfigured
	}
	return s.LastFn(o, c, n)
}

func (s stubMail) Export(c *cfg_g.Config, o ExportOptions) error {
	if s.ExportFn == nil {
		return ErrNotConfigured
	}
	return s.ExportFn(c, o)
}

func (s stubMail) Pipe(c *cfg_g.Config, o PipeOptions, args []string) error {
	if s.PipeFn == nil {
		return ErrNotConfigured
	}
	return s.PipeFn(c, o, args)
}

func (s stubMail) RunBackendTests(c *cfg_g.Config) error {
	if s.RunBackendTestsFn == nil {
		return ErrNotConfigured
	}
	return s.RunBackendTestsFn(c)
}

type stubSpam struct {
	MarkByIDFn      func(*cfg_g.Config, string) error
	UnspamByIDFn    func(*cfg_g.Config, string) error
	UnspamFolderFn  func(*cfg_g.Config, label.Folder) error
	LearnHamFn      func(*cfg_g.Config, label.Folder, bool) error
	ResetLearningFn func(*cfg_g.Config) error
	ForgetByIDFn    func(*cfg_g.Config, []string) error
	ForgetFolderFn  func(*cfg_g.Config, label.Folder) error
}

func (s stubSpam) MarkByID(c *cfg_g.Config, id string) error {
	if s.MarkByIDFn == nil {
		return ErrNotConfigured
	}
	return s.MarkByIDFn(c, id)
}

func (s stubSpam) UnspamByID(c *cfg_g.Config, id string) error {
	if s.UnspamByIDFn == nil {
		return ErrNotConfigured
	}
	return s.UnspamByIDFn(c, id)
}

func (s stubSpam) UnspamFolder(c *cfg_g.Config, f label.Folder) error {
	if s.UnspamFolderFn == nil {
		return ErrNotConfigured
	}
	return s.UnspamFolderFn(c, f)
}

func (s stubSpam) LearnHam(c *cfg_g.Config, f label.Folder, force bool) error {
	if s.LearnHamFn == nil {
		return ErrNotConfigured
	}
	return s.LearnHamFn(c, f, force)
}

func (s stubSpam) ResetLearning(c *cfg_g.Config) error {
	if s.ResetLearningFn == nil {
		return ErrNotConfigured
	}
	return s.ResetLearningFn(c)
}

func (s stubSpam) ForgetByID(c *cfg_g.Config, refs []string) error {
	if s.ForgetByIDFn == nil {
		return ErrNotConfigured
	}
	return s.ForgetByIDFn(c, refs)
}

func (s stubSpam) ForgetFolder(c *cfg_g.Config, f label.Folder) error {
	if s.ForgetFolderFn == nil {
		return ErrNotConfigured
	}
	return s.ForgetFolderFn(c, f)
}

type stubConfig struct {
	ShowFn     func(*cfg_g.Config) error
	ValidateFn func(*cfg_g.Config) error
	SetFn      func(*cfg_g.Config, string, string) error
	ResetFn    func(*cfg_g.Config, string) error
	GetFn      func(*cfg_g.Config, string) error
}

func (s stubConfig) Show(c *cfg_g.Config) error {
	if s.ShowFn == nil {
		return ErrNotConfigured
	}
	return s.ShowFn(c)
}

func (s stubConfig) Validate(c *cfg_g.Config) error {
	if s.ValidateFn == nil {
		return ErrNotConfigured
	}
	return s.ValidateFn(c)
}

func (s stubConfig) Set(c *cfg_g.Config, k, v string) error {
	if s.SetFn == nil {
		return ErrNotConfigured
	}
	return s.SetFn(c, k, v)
}

func (s stubConfig) Reset(c *cfg_g.Config, k string) error {
	if s.ResetFn == nil {
		return ErrNotConfigured
	}
	return s.ResetFn(c, k)
}

func (s stubConfig) Get(c *cfg_g.Config, k string) error {
	if s.GetFn == nil {
		return ErrNotConfigured
	}
	return s.GetFn(c, k)
}

// stubOrganize's hooks speak strings, so tests can name folders as literals;
// the methods convert at the OrganizeOps boundary.
type stubOrganize struct {
	AllFn                  func(*cfg_g.Config, mailclient.MailClient, string, string) error
	ByIDFn                 func(*cfg_g.Config, mailclient.MailClient, string, string) error
	ResolveArchiveTargetFn func(mailclient.MailClient) (string, error)
	ResolveTrashTargetFn   func(mailclient.MailClient) (string, error)
	ResolveDraftsTargetFn  func(mailclient.MailClient) (string, error)
	ResolveSentTargetFn    func(mailclient.MailClient) (string, error)
	ListServerDraftsFn     func(mailclient.MailClient) (string, []organize.ServerDraft, error)
	PlanServerPruneFn      func(mailclient.MailClient, organize.PruneOptions) (string, int, organize.PrunePlan, error)
	FindServerDraftsFn     func(mailclient.MailClient, []string) (string, []organize.ServerDraft, []string, error)
	TrashServerDraftsFn    func(mailclient.MailClient, string, []email.ID) (string, error)
}

func (s stubOrganize) ListServerDrafts(m mailclient.MailClient) (label.Folder, []organize.ServerDraft, error) {
	if s.ListServerDraftsFn == nil {
		return label.Folder{}, nil, ErrNotConfigured
	}
	f, d, err := s.ListServerDraftsFn(m)
	return label.FromServer(f), d, err
}

func (s stubOrganize) PlanServerPrune(m mailclient.MailClient, opts organize.PruneOptions) (label.Folder, int, organize.PrunePlan, error) {
	if s.PlanServerPruneFn == nil {
		return label.Folder{}, 0, organize.PrunePlan{}, ErrNotConfigured
	}
	f, n, p, err := s.PlanServerPruneFn(m, opts)
	return label.FromServer(f), n, p, err
}

func (s stubOrganize) FindServerDrafts(m mailclient.MailClient, names []string) (label.Folder, []organize.ServerDraft, []string, error) {
	if s.FindServerDraftsFn == nil {
		return label.Folder{}, nil, nil, ErrNotConfigured
	}
	f, d, u, err := s.FindServerDraftsFn(m, names)
	return label.FromServer(f), d, u, err
}

func (s stubOrganize) TrashServerDrafts(m mailclient.MailClient, folder label.Folder, ids []email.ID) (label.Folder, error) {
	if s.TrashServerDraftsFn == nil {
		return label.Folder{}, ErrNotConfigured
	}
	trash, err := s.TrashServerDraftsFn(m, folder.String(), ids)
	return label.FromServer(trash), err
}

func (s stubOrganize) All(c *cfg_g.Config, m mailclient.MailClient, src string, t label.Folder) error {
	if s.AllFn == nil {
		return ErrNotConfigured
	}
	return s.AllFn(c, m, src, t.String())
}

func (s stubOrganize) ByID(c *cfg_g.Config, m mailclient.MailClient, t label.Folder, id string) error {
	if s.ByIDFn == nil {
		return ErrNotConfigured
	}
	return s.ByIDFn(c, m, t.String(), id)
}

func (s stubOrganize) ResolveArchiveTarget(m mailclient.MailClient) (label.Folder, error) {
	if s.ResolveArchiveTargetFn == nil {
		return label.Folder{}, ErrNotConfigured
	}
	f, err := s.ResolveArchiveTargetFn(m)
	return label.FromServer(f), err
}

func (s stubOrganize) ResolveTrashTarget(m mailclient.MailClient) (label.Folder, error) {
	if s.ResolveTrashTargetFn == nil {
		return label.Folder{}, ErrNotConfigured
	}
	f, err := s.ResolveTrashTargetFn(m)
	return label.FromServer(f), err
}

func (s stubOrganize) ResolveDraftsTarget(m mailclient.MailClient) (label.Folder, error) {
	if s.ResolveDraftsTargetFn == nil {
		return label.Folder{}, ErrNotConfigured
	}
	f, err := s.ResolveDraftsTargetFn(m)
	return label.FromServer(f), err
}

func (s stubOrganize) ResolveSentTarget(m mailclient.MailClient) (label.Folder, error) {
	if s.ResolveSentTargetFn == nil {
		return label.Folder{}, ErrNotConfigured
	}
	f, err := s.ResolveSentTargetFn(m)
	return label.FromServer(f), err
}

type stubCalendar struct {
	AddFn    func(*cfg_g.Config, mailclient.MailClient, string, string) error
	AddAllFn func(*cfg_g.Config, mailclient.MailClient) error
	WeekFn   func(*cfg_g.Config, ...int) error
}

func (s stubCalendar) Add(c *cfg_g.Config, m mailclient.MailClient, p, id string) error {
	if s.AddFn == nil {
		return ErrNotConfigured
	}
	return s.AddFn(c, m, p, id)
}

func (s stubCalendar) AddAll(c *cfg_g.Config, m mailclient.MailClient) error {
	if s.AddAllFn == nil {
		return ErrNotConfigured
	}
	return s.AddAllFn(c, m)
}

func (s stubCalendar) Week(c *cfg_g.Config, days ...int) error {
	if s.WeekFn == nil {
		return ErrNotConfigured
	}
	return s.WeekFn(c, days...)
}

type stubTUI struct {
	RunFn           func(*cfg_g.Config, string, ...string) error
	RunWithClientFn func(*cfg_g.Config, mailclient.MailClient, string) error
	ComposeFn       func(*cfg_g.Config, ComposeOptions) error
}

func (s stubTUI) Run(c *cfg_g.Config, p string, notices ...string) error {
	if s.RunFn == nil {
		return ErrNotConfigured
	}
	return s.RunFn(c, p, notices...)
}

func (s stubTUI) RunWithClient(c *cfg_g.Config, m mailclient.MailClient, p string) error {
	if s.RunWithClientFn == nil {
		return ErrNotConfigured
	}
	return s.RunWithClientFn(c, m, p)
}

func (s stubTUI) Compose(c *cfg_g.Config, opts ComposeOptions) error {
	if s.ComposeFn == nil {
		return ErrNotConfigured
	}
	return s.ComposeFn(c, opts)
}

type stubAccounts struct {
	TokenPathFn func(string, cfg_acc.AccountConfig) (string, error)
	OpenMboxFn  func(*cfg_g.Config, string, cachedir.Account) (mailclient.MailClient, error)
}

func (s stubAccounts) TokenPath(dir string, acc cfg_acc.AccountConfig) (string, error) {
	if s.TokenPathFn == nil {
		return "", ErrNotConfigured
	}
	return s.TokenPathFn(dir, acc)
}

func (s stubAccounts) OpenMbox(c *cfg_g.Config, path string, cacheDir cachedir.Account) (mailclient.MailClient, error) {
	if s.OpenMboxFn == nil {
		return nil, ErrNotConfigured
	}
	return s.OpenMboxFn(c, path, cacheDir)
}

type stubContacts struct {
	ListFn       func(*cfg_g.Config, string) error
	SearchFn     func(*cfg_g.Config, string) error
	ImportMuttFn func(*cfg_g.Config, string) error
	SyncGoogleFn func(*cfg_g.Config, string) error
	PathFn       func(*cfg_g.Config) error
	SuggestFn    func(*cfg_g.Config, string) error
	HarvestFn    func(*cfg_g.Config) error
	PurgeFn      func(*cfg_g.Config, string) error
}

func (s stubContacts) List(c *cfg_g.Config, f string) error {
	if s.ListFn == nil {
		return ErrNotConfigured
	}
	return s.ListFn(c, f)
}

func (s stubContacts) Search(c *cfg_g.Config, q string) error {
	if s.SearchFn == nil {
		return ErrNotConfigured
	}
	return s.SearchFn(c, q)
}

func (s stubContacts) ImportMutt(c *cfg_g.Config, p string) error {
	if s.ImportMuttFn == nil {
		return ErrNotConfigured
	}
	return s.ImportMuttFn(c, p)
}

func (s stubContacts) SyncGoogle(c *cfg_g.Config, a string) error {
	if s.SyncGoogleFn == nil {
		return ErrNotConfigured
	}
	return s.SyncGoogleFn(c, a)
}

func (s stubContacts) Path(c *cfg_g.Config) error {
	if s.PathFn == nil {
		return ErrNotConfigured
	}
	return s.PathFn(c)
}

func (s stubContacts) Suggest(c *cfg_g.Config, q string) error {
	if s.SuggestFn == nil {
		return ErrNotConfigured
	}
	return s.SuggestFn(c, q)
}

func (s stubContacts) Harvest(c *cfg_g.Config) error {
	if s.HarvestFn == nil {
		return ErrNotConfigured
	}
	return s.HarvestFn(c)
}

func (s stubContacts) Purge(c *cfg_g.Config, email string) error {
	if s.PurgeFn == nil {
		return ErrNotConfigured
	}
	return s.PurgeFn(c, email)
}

type stubSend struct {
	SendFn  func(*cfg_g.Config, SendOptions) error
	ReplyFn func(*cfg_g.Config, ReplyOptions) error
}

func (s stubSend) Send(c *cfg_g.Config, opts SendOptions) error {
	if s.SendFn == nil {
		return ErrNotConfigured
	}
	return s.SendFn(c, opts)
}

func (s stubSend) Reply(c *cfg_g.Config, opts ReplyOptions) error {
	if s.ReplyFn == nil {
		return ErrNotConfigured
	}
	return s.ReplyFn(c, opts)
}

type stubOutbox struct {
	ListFn   func(*cfg_g.Config) ([]outbox.Entry, error)
	FlushFn  func(*cfg_g.Config) ([]outbox.FlushResult, error)
	RemoveFn func(*cfg_g.Config, string) error
}

func (s stubOutbox) List(c *cfg_g.Config) ([]outbox.Entry, error) {
	if s.ListFn == nil {
		return nil, ErrNotConfigured
	}
	return s.ListFn(c)
}

func (s stubOutbox) Flush(c *cfg_g.Config) ([]outbox.FlushResult, error) {
	if s.FlushFn == nil {
		return nil, ErrNotConfigured
	}
	return s.FlushFn(c)
}

func (s stubOutbox) Remove(c *cfg_g.Config, id string) error {
	if s.RemoveFn == nil {
		return ErrNotConfigured
	}
	return s.RemoveFn(c, id)
}

type stubWatch struct {
	WatchFn func(context.Context, *cfg_g.Config, watcher.Options) error
}

func (s stubWatch) Watch(ctx context.Context, c *cfg_g.Config, opts watcher.Options) error {
	if s.WatchFn == nil {
		return ErrNotConfigured
	}
	return s.WatchFn(ctx, c, opts)
}
