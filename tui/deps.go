package tui

import (
	"errors"
	"github.com/sarielhp/mail_cli/label"

	"github.com/sarielhp/mail_cli/calendar"
	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/mailclient"
)

// What the TUI needs from the layers beneath it, declared here and bound in
// main's composition root. This replaces a struct of 15 nil-able function
// fields, three of which were never called.

// ErrNotConfigured reports a dependency that was never bound.
var ErrNotConfigured = errors.New("backend dependency not configured")

// ClientProvider resolves the backend client for an account.
type ClientProvider interface {
	ForSelected(config *cfg_g.Config) (mailclient.MailClient, error)
	ForAccount(config *cfg_g.Config, acc cfg_acc.AccountConfig) (mailclient.MailClient, error)
	ForAccountIndex(config *cfg_g.Config, index int) (mailclient.MailClient, error)
}

// FolderResolver names the folder a backend files archived, deleted and draft
// mail into, which differs per backend and per account.
type FolderResolver interface {
	ResolveArchiveTarget(client mailclient.MailClient) (label.Folder, error)
	ResolveTrashTarget(client mailclient.MailClient) (label.Folder, error)
	ResolveDraftsTarget(client mailclient.MailClient) (label.Folder, error)
}

// CalendarCheckResult details how an ICS event matches an existing Google Calendar event.
type CalendarCheckResult struct {
	InCalendar  bool
	ExactMatch  bool
	Discrepancy string
	Summary     string
	Event       *calendar.ICSEvent
}

// CalendarOps adds an .ics payload to the user's calendar.
type CalendarOps interface {
	AddICS(config *cfg_g.Config, icsData []byte) (summary string, existed bool, err error)
	CheckICS(config *cfg_g.Config, icsData []byte) (inCalendar bool, summary string, event *calendar.ICSEvent, err error)
}

// VerifyingCalendarOps is the richer form: a calendar that reads the event back
// after writing it and can say whether duplicates were created.
//
// It is an optional upgrade rather than part of CalendarOps because not every
// calendar can do it, and the TUI has a meaningfully different report for each
// case. Implementations that can verify satisfy it; the TUI type-asserts.
type VerifyingCalendarOps interface {
	CalendarOps
	AddICSVerified(config *cfg_g.Config, icsData []byte) (summary string, existed, verified, noDuplicates bool, err error)
	AddICSAsNew(config *cfg_g.Config, icsData []byte) (summary string, err error)
	CheckICSDetailed(config *cfg_g.Config, icsData []byte) (CalendarCheckResult, error)
}

// Backend is everything the TUI needs from below.
type Backend struct {
	Clients  ClientProvider
	Folders  FolderResolver
	Calendar CalendarOps
}

// Accessors tolerate both a nil Backend and a nil member, so call sites need no
// guards. cmdArchive and cmdDelete previously dereferenced m.bk unchecked.

func (b *Backend) clients() ClientProvider {
	if b == nil || b.Clients == nil {
		return noClients{}
	}
	return b.Clients
}

func (b *Backend) folders() FolderResolver {
	if b == nil || b.Folders == nil {
		return noFolders{}
	}
	return b.Folders
}

// hasCalendar reports whether calendar support was wired at all, which decides
// whether the TUI offers the action rather than failing it.
func (b *Backend) hasCalendar() bool {
	return b != nil && b.Calendar != nil
}

type noClients struct{}

func (noClients) ForSelected(*cfg_g.Config) (mailclient.MailClient, error) {
	return nil, ErrNotConfigured
}

func (noClients) ForAccount(*cfg_g.Config, cfg_acc.AccountConfig) (mailclient.MailClient, error) {
	return nil, ErrNotConfigured
}

func (noClients) ForAccountIndex(*cfg_g.Config, int) (mailclient.MailClient, error) {
	return nil, ErrNotConfigured
}

type noFolders struct{}

func (noFolders) ResolveArchiveTarget(mailclient.MailClient) (label.Folder, error) {
	return label.Folder{}, ErrNotConfigured
}
func (noFolders) ResolveTrashTarget(mailclient.MailClient) (label.Folder, error) {
	return label.Folder{}, ErrNotConfigured
}
func (noFolders) ResolveDraftsTarget(mailclient.MailClient) (label.Folder, error) {
	return label.Folder{}, ErrNotConfigured
}

// verifyingCalendar reports whether the wired calendar can confirm what it
// wrote. The TUI has a different, richer report when it can.
func (b *Backend) verifyingCalendar() (VerifyingCalendarOps, bool) {
	if !b.hasCalendar() {
		return nil, false
	}
	v, ok := b.Calendar.(VerifyingCalendarOps)
	return v, ok
}
