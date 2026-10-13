package tui

import (
	"github.com/sarielhp/mail_cli/calendar"
	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/label"
	"github.com/sarielhp/mail_cli/mailclient"
)

// Test stubs for the TUI's dependency interfaces. An unset func field behaves
// as "not configured", so a test supplies only the call it exercises.

type stubClients struct {
	ForSelectedFn     func(*cfg_g.Config) (mailclient.MailClient, error)
	ForAccountFn      func(*cfg_g.Config, cfg_acc.AccountConfig) (mailclient.MailClient, error)
	ForAccountIndexFn func(*cfg_g.Config, int) (mailclient.MailClient, error)
}

func (s stubClients) ForSelected(c *cfg_g.Config) (mailclient.MailClient, error) {
	if s.ForSelectedFn == nil {
		return nil, ErrNotConfigured
	}
	return s.ForSelectedFn(c)
}

func (s stubClients) ForAccount(c *cfg_g.Config, a cfg_acc.AccountConfig) (mailclient.MailClient, error) {
	if s.ForAccountFn == nil {
		return nil, ErrNotConfigured
	}
	return s.ForAccountFn(c, a)
}

func (s stubClients) ForAccountIndex(c *cfg_g.Config, i int) (mailclient.MailClient, error) {
	if s.ForAccountIndexFn == nil {
		return nil, ErrNotConfigured
	}
	return s.ForAccountIndexFn(c, i)
}

type stubFolders struct {
	ArchiveFn func(mailclient.MailClient) (string, error)
	TrashFn   func(mailclient.MailClient) (string, error)
	DraftsFn  func(mailclient.MailClient) (string, error)
}

func (s stubFolders) ResolveArchiveTarget(c mailclient.MailClient) (label.Folder, error) {
	if s.ArchiveFn == nil {
		return label.Folder{}, ErrNotConfigured
	}
	f, err := s.ArchiveFn(c)
	return label.FromServer(f), err
}

func (s stubFolders) ResolveTrashTarget(c mailclient.MailClient) (label.Folder, error) {
	if s.TrashFn == nil {
		return label.Folder{}, ErrNotConfigured
	}
	f, err := s.TrashFn(c)
	return label.FromServer(f), err
}

func (s stubFolders) ResolveDraftsTarget(c mailclient.MailClient) (label.Folder, error) {
	if s.DraftsFn == nil {
		return label.Folder{}, ErrNotConfigured
	}
	f, err := s.DraftsFn(c)
	return label.FromServer(f), err
}

// stubCalendar implements CalendarOps only. Use stubVerifyingCalendar to
// exercise the richer path.
type stubCalendar struct {
	AddICSFn   func(*cfg_g.Config, []byte) (string, bool, error)
	CheckICSFn func(*cfg_g.Config, []byte) (bool, string, *calendar.ICSEvent, error)
}

func (s stubCalendar) AddICS(c *cfg_g.Config, data []byte) (string, bool, error) {
	if s.AddICSFn == nil {
		return "", false, ErrNotConfigured
	}
	return s.AddICSFn(c, data)
}

func (s stubCalendar) CheckICS(c *cfg_g.Config, data []byte) (bool, string, *calendar.ICSEvent, error) {
	if s.CheckICSFn == nil {
		return false, "", nil, ErrNotConfigured
	}
	return s.CheckICSFn(c, data)
}

type stubVerifyingCalendar struct {
	stubCalendar
	AddICSVerifiedFn   func(*cfg_g.Config, []byte) (string, bool, bool, bool, error)
	AddICSAsNewFn      func(*cfg_g.Config, []byte) (string, error)
	CheckICSDetailedFn func(*cfg_g.Config, []byte) (CalendarCheckResult, error)
}

func (s stubVerifyingCalendar) AddICSVerified(c *cfg_g.Config, data []byte) (string, bool, bool, bool, error) {
	if s.AddICSVerifiedFn == nil {
		return "", false, false, false, ErrNotConfigured
	}
	return s.AddICSVerifiedFn(c, data)
}

func (s stubVerifyingCalendar) AddICSAsNew(c *cfg_g.Config, data []byte) (string, error) {
	if s.AddICSAsNewFn == nil {
		return "", ErrNotConfigured
	}
	return s.AddICSAsNewFn(c, data)
}

func (s stubVerifyingCalendar) CheckICSDetailed(c *cfg_g.Config, data []byte) (CalendarCheckResult, error) {
	if s.CheckICSDetailedFn == nil {
		return CalendarCheckResult{}, ErrNotConfigured
	}
	return s.CheckICSDetailedFn(c, data)
}
