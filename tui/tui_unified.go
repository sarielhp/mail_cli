package tui

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/sarielhp/mail_cli/cache/folderindex"
	"github.com/sarielhp/mail_cli/cache/msg"
	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/label"
	"github.com/sarielhp/mail_cli/mailclient"

	tea "github.com/charmbracelet/bubbletea"
)

const (
	// UnifiedInboxFolder is the visible display name for the aggregated inbox.
	UnifiedInboxFolder = "All Inboxes"
	// UnifiedInboxPrefix is the label sigil naming all inboxes.
	UnifiedInboxPrefix = "%all:inbox"
)

func isUnifiedInbox(folder string) bool {
	return strings.EqualFold(folder, UnifiedInboxFolder) ||
		strings.EqualFold(folder, UnifiedInboxPrefix) ||
		strings.EqualFold(folder, "%all")
}

// unifiedInboxItems is the folder entry for the unified inbox, offered when
// more than one account is configured.
func (m *tuiModel) unifiedInboxItems() []label.Item {
	if len(m.getAccounts()) < 2 {
		return nil
	}
	return []label.Item{{Name: UnifiedInboxFolder, FullName: UnifiedInboxFolder, IsSystem: true}}
}

// stampAccount marks a row with the account it came from: the name that
// routes actions to it, and the badge and colour that show it.
func stampAccount(em *email.Email, idx int, acc cfg_acc.AccountConfig) {
	em.Account = acc.Name
	em.AccountBadge = acc.Badge
	em.AccountIndex = idx
}

func (m *tuiModel) findEmailByID(id email.ID) *email.Email {
	for i := range m.emails {
		if m.emails[i].ID == id {
			return &m.emails[i]
		}
	}
	for i := range m.rawEmails {
		if m.rawEmails[i].ID == id {
			return &m.rawEmails[i]
		}
	}
	return nil
}

func (m *tuiModel) loadUnifiedCachedEmails() {
	var allEmails []email.Email
	accounts := m.getAccounts()
	cfg := m.Config()
	if cfg == nil {
		cfg = m.cfg
	}

	for idx, acc := range accounts {
		if cfg == nil {
			continue
		}
		accCache := cfg.AccountCacheFor(acc)
		if accCache.IsZero() || !folderindex.HasStructure(accCache) {
			continue
		}

		inboxName := "INBOX"
		ids, err := folderindex.IDs(accCache, inboxName)
		if err != nil {
			continue
		}

		for _, id := range ids {
			data, err := msg.Read(accCache, id)
			if err != nil {
				continue
			}
			em := email.ParseReader(bytes.NewReader(data), id, "")
			if em == nil {
				continue
			}
			stampAccount(em, idx, acc)
			em.IsRead = m.isRead[id]
			em.IsReplied = m.isReplied[id]
			if info, err := msg.GetInfo(accCache, id); err == nil {
				em.IsSpam = info.IsSpam()
				em.IsPolitical = info.IsPolitical()
				em.IsBlacklisted = info.IsBlacklisted()
			}
			allEmails = append(allEmails, *em)
		}
	}

	sort.Slice(allEmails, func(i, j int) bool {
		return allEmails[i].EmailDate.After(allEmails[j].EmailDate)
	})

	m.rawEmails = allEmails
	m.rebuildVisibleEmails()
	if len(m.emails) > 0 {
		m.eIdx = 0
		m.selectedID = m.emails[0].ID
	} else {
		m.selectedID = ""
	}
	m.folderJustChanged = true
}

func (m *tuiModel) loadUnifiedEmailsCmd() tea.Cmd {
	return func() tea.Msg {
		var allEmails []email.Email
		accounts := m.getAccounts()
		cfg := m.Config()
		if cfg == nil {
			cfg = m.cfg
		}

		for idx, acc := range accounts {
			cl, err := m.resolveClientForSwitch(acc)
			if err != nil || cl == nil {
				continue
			}
			inboxFolder := cl.InboxFolder()
			ids, err := cl.FetchAndDownloadEmails(inboxFolder, "")
			if err != nil {
				continue
			}
			accCache := cfg.AccountCacheFor(acc)
			for _, id := range ids {
				data, err := msg.Read(accCache, id)
				if err != nil {
					continue
				}
				em := email.ParseReader(bytes.NewReader(data), id, "")
				if em == nil {
					continue
				}
				stampAccount(em, idx, acc)
				if info, err := msg.GetInfo(accCache, id); err == nil {
					em.IsSpam = info.IsSpam()
					em.IsPolitical = info.IsPolitical()
					em.IsBlacklisted = info.IsBlacklisted()
				}
				allEmails = append(allEmails, *em)
			}
		}

		sort.Slice(allEmails, func(i, j int) bool {
			return allEmails[i].EmailDate.After(allEmails[j].EmailDate)
		})
		return emailsLoadedMsg{emails: allEmails}
	}
}

// actionPlan is what an action on messages needs to know about where they
// live, captured on the Update goroutine before their rows are removed. The
// command that performs the action runs later on its own goroutine, where the
// rows are gone and m.emails may be changing under it; resolving an id there
// found nothing, so unified-inbox actions fell back to the active account with
// "All Inboxes" as the source folder.
type actionPlan struct {
	// active is the client the session had when the action began; it takes
	// every id outside a unified inbox.
	active mailclient.MailClient
	// folder is the folder the action was taken in.
	folder string
	// accounts names the owning account of each id; set only in a unified
	// inbox, where each message belongs to a different account.
	accounts map[email.ID]string
}

// planAction captures the session's client, folder and, in a unified inbox,
// each id's account. Call it on the Update goroutine, before removing rows.
func (m *tuiModel) planAction(ids []email.ID, folder string) actionPlan {
	p := actionPlan{active: m.client, folder: folder}
	if !isUnifiedInbox(folder) {
		return p
	}
	p.accounts = make(map[email.ID]string, len(ids))
	for _, id := range ids {
		if em := m.findEmailByID(id); em != nil {
			p.accounts[id] = em.Account
		}
	}
	return p
}

func (p actionPlan) unified() bool { return isUnifiedInbox(p.folder) }

// clientForAccountName returns the client for the named account. It never
// substitutes another account's client: acting on the wrong server with the
// wrong folder is worse than a refusal.
func (m *tuiModel) clientForAccountName(active mailclient.MailClient, name string) (mailclient.MailClient, error) {
	if name == "" {
		return nil, errors.New("message has no known account")
	}
	if active != nil && active.Config() != nil && strings.EqualFold(string(active.Config().SelectedAccount), name) {
		return active, nil
	}
	for _, acc := range m.getAccounts() {
		if !strings.EqualFold(acc.Name, name) {
			continue
		}
		cl, err := m.resolveClientForSwitch(acc)
		if err != nil {
			return nil, fmt.Errorf("account %s: %w", name, err)
		}
		if cl == nil {
			return nil, fmt.Errorf("account %s: no client", name)
		}
		return cl, nil
	}
	return nil, fmt.Errorf("account %s is not configured", name)
}

// target is the client and source folder an action on id must use: the
// active client and folder, or in a unified inbox the message's own account
// and that account's inbox.
func (m *tuiModel) target(p actionPlan, id email.ID) (mailclient.MailClient, string, error) {
	if !p.unified() {
		if p.active == nil {
			return nil, "", errors.New("no active account")
		}
		return p.active, p.folder, nil
	}
	cl, err := m.clientForAccountName(p.active, p.accounts[id])
	if err != nil {
		return nil, "", err
	}
	return cl, cl.InboxFolder().String(), nil
}

// accountBatch is the ids of one account in a bulk action, with the client
// and source folder that account needs.
type accountBatch struct {
	client mailclient.MailClient
	folder string
	ids    []email.ID
}

// batches groups ids by the client that must act on them. An id whose
// account cannot be resolved is reported in the error and left out.
func (m *tuiModel) batches(p actionPlan, ids []email.ID) ([]accountBatch, error) {
	if !p.unified() {
		cl, folder, err := m.target(p, "")
		if err != nil {
			return nil, err
		}
		return []accountBatch{{client: cl, folder: folder, ids: ids}}, nil
	}
	byName := make(map[string][]email.ID)
	for _, id := range ids {
		byName[p.accounts[id]] = append(byName[p.accounts[id]], id)
	}
	var out []accountBatch
	var errs []error
	for _, name := range sortedKeys(byName) {
		cl, err := m.clientForAccountName(p.active, name)
		if err != nil {
			errs = append(errs, fmt.Errorf("%d message(s): %w", len(byName[name]), err))
			continue
		}
		out = append(out, accountBatch{client: cl, folder: cl.InboxFolder().String(), ids: byName[name]})
	}
	return out, errors.Join(errs...)
}

func sortedKeys(groups map[string][]email.ID) []string {
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// bulkOutcome is what a bulk action achieved: the ids the servers accepted
// and every failure, so the status line never counts a refused id as done.
type bulkOutcome struct {
	done []email.ID
	err  error
}

// eachBatch runs act on every account's batch and collects the outcome.
func (m *tuiModel) eachBatch(p actionPlan, ids []email.ID, act func(accountBatch) error) bulkOutcome {
	batches, err := m.batches(p, ids)
	errs := []error{err}
	var done []email.ID
	for _, b := range batches {
		if err := act(b); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", accountLabel(b.client), err))
			continue
		}
		done = append(done, b.ids...)
	}
	return bulkOutcome{done: done, err: errors.Join(errs...)}
}

func accountLabel(cl mailclient.MailClient) string {
	if cl == nil || cl.Config() == nil || cl.Config().SelectedAccount == "" {
		return "account"
	}
	return string(cl.Config().SelectedAccount)
}

func (m *tuiModel) dispatchBulkArchive(p actionPlan, ids []email.ID) bulkOutcome {
	return m.dispatchBulkMove(p, ids, m.bk.folders().ResolveArchiveTarget)
}

// folderTargetResolver names the folder a bulk move files mail into, per client.
type folderTargetResolver func(mailclient.MailClient) (label.Folder, error)

// dispatchBulkMove moves ids out of their folder. In a unified inbox each
// message's own account resolves the destination and receives its own ids;
// otherwise the active client takes the whole batch.
func (m *tuiModel) dispatchBulkMove(p actionPlan, ids []email.ID, resolve folderTargetResolver) bulkOutcome {
	return m.eachBatch(p, ids, func(b accountBatch) error {
		tgt, err := resolve(b.client)
		if err != nil {
			return err
		}
		return b.client.MoveEmail(b.ids, label.FromServer(b.folder), tgt)
	})
}

func (m *tuiModel) dispatchBulkDelete(p actionPlan, ids []email.ID) bulkOutcome {
	return m.dispatchBulkMove(p, ids, m.bk.folders().ResolveTrashTarget)
}

// dispatchBulkMarkRead marks ids read on their servers.
func (m *tuiModel) dispatchBulkMarkRead(p actionPlan, ids []email.ID) bulkOutcome {
	return m.eachBatch(p, ids, func(b accountBatch) error {
		return b.client.MarkAsRead(b.ids)
	})
}

func (m *tuiModel) dispatchBulkSpam(p actionPlan, ids []email.ID) bulkOutcome {
	return m.eachBatch(p, ids, func(b accountBatch) error {
		return b.client.ReportSpam(b.ids, label.FromServer(b.folder))
	})
}
