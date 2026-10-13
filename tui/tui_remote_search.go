package tui

import (
	"errors"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sarielhp/mail_cli/cache/virtual"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/mailclient"
)

// searchFolderPrefix names the TUI's view of remote search results. It is not
// a server folder.
const searchFolderPrefix = "%search:"

func isSearchView(folder string) bool { return strings.HasPrefix(folder, searchFolderPrefix) }

// errSearchViewAction refuses a move out of a remote search view. A search hit
// can sit in any folder and the server's summaries do not say which, while
// every move names a source folder; "%search:<q>" used to be sent as that
// source, which on Gmail is a label removal that matches nothing.
var errSearchViewAction = errors.New("remote search results do not record each message's folder; open its folder to move, delete or report it")

// refuseInSearchView reports, and returns true, when the current view is a
// remote search, where moves cannot name a source folder.
func (m *tuiModel) refuseInSearchView() bool {
	if !isSearchView(m.currentF) {
		return false
	}
	m.err = errSearchViewAction
	m.showError = true
	return true
}

type remoteSearchDoneMsg struct {
	query  string
	emails []*email.Email
	err    error
	// client is the one the search ran against, captured when it started.
	client mailclient.MailClient
}

func (m *tuiModel) kRemoteSearchPrompt(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	q, ok := m.kPromptLine(k, &m.remoteSearchQuery, &m.inRemoteSearch)
	if !ok {
		return m, nil
	}
	return m.startRemoteSearch(q)
}

func (m *tuiModel) startRemoteSearch(query string) (tea.Model, tea.Cmd) {
	if m.client == nil {
		m.err = fmt.Errorf("no mail client connected for remote search")
		m.showError = true
		return m, nil
	}

	m.showLoad = "Searching server..."
	client := m.client

	return m, func() tea.Msg {
		ids, err := client.SearchEmails(query, 0)
		if err != nil {
			return remoteSearchDoneMsg{query: query, err: err, client: client}
		}
		if len(ids) == 0 {
			return remoteSearchDoneMsg{query: query, emails: []*email.Email{}, client: client}
		}
		summaries, err := client.GetMessageSummariesBatch(ids)
		if err != nil {
			return remoteSearchDoneMsg{query: query, err: err, client: client}
		}
		return remoteSearchDoneMsg{query: query, emails: summaries, client: client}
	}
}

// sameAccount compares two clients by the account they serve, never by
// pointer: the decorators are built per call.
func sameAccount(a, b mailclient.MailClient) bool {
	if a == nil || b == nil || a.Config() == nil || b.Config() == nil {
		return false
	}
	return strings.EqualFold(string(a.Config().SelectedAccount), string(b.Config().SelectedAccount))
}

func (m *tuiModel) handleRemoteSearchDone(msg remoteSearchDoneMsg) (tea.Model, tea.Cmd) {
	m.showLoad = ""
	if msg.err != nil {
		m.err = fmt.Errorf("remote search failed: %w", msg.err)
		m.showError = true
		return m, nil
	}
	searched := msg.client
	if searched == nil {
		searched = m.client
	}

	rawList := make([]email.Email, 0, len(msg.emails))
	ids := make([]email.ID, 0, len(msg.emails))
	for _, em := range msg.emails {
		if em != nil {
			rawList = append(rawList, *em)
			ids = append(ids, em.ID)
		}
	}

	// The result set belongs to the account searched, which is not the
	// active one if the user switched while the search ran.
	if cfg := searched.Config(); cfg != nil && !cfg.AccountCache.IsZero() {
		vm := &virtual.Mailbox{
			Name:        "search",
			Description: msg.query,
			MessageIDs:  ids,
		}
		_ = virtual.Save(cfg.AccountCache, vm)
	}
	if msg.client != nil && !sameAccount(msg.client, m.client) {
		m.saveStatus = fmt.Sprintf("Search %q finished on %s (%d result(s)); switch back to view it", msg.query, accountLabel(searched), len(rawList))
		m.saveStatusTime = time.Now()
		return m, nil
	}

	m.rawEmails = rawList
	m.currentF = searchFolderPrefix + msg.query
	m.searchQuery = ""
	m.inSearch = false
	m.rebuildVisibleEmails()

	if len(m.emails) > 0 {
		m.eIdx = 0
		m.selectedID = m.emails[0].ID
	} else {
		m.eIdx = 0
		m.selectedID = ""
	}

	m.saveStatus = fmt.Sprintf("Found %d remote message(s)", len(rawList))
	m.saveStatusTime = time.Now()
	return m, nil
}
