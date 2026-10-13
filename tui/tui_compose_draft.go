package tui

import (
	"errors"
	"fmt"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/label"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sarielhp/mail_cli/draft"
	"github.com/sarielhp/mail_cli/mailclient"
	"github.com/sarielhp/mail_cli/organize"

	tea "github.com/charmbracelet/bubbletea"
)

// A compose session keeps two copies of its message: a local draft.eml,
// rewritten on every edit, and one copy in the account's Drafts folder. The
// server side is a queue per session (draftSync), keyed by the session's
// Message-ID and held on the model rather than on composeState, because the
// work outlives the session: leaving, discarding and sending all end the
// session while an upload may still be running, and each used to race that
// upload -- leaving two copies, an orphan, or a stale result applied to the
// next session.

// serverDraft is the copy of a session's message in a Drafts folder, with
// the client that put it there. Retiring it goes through that client, so a
// copy survives neither an account switch nor a new client for the same
// account.
type serverDraft struct {
	ID      email.ID
	Account string
	Folder  label.Folder
	client  mailclient.MailClient
	// encrypted says the copy was uploaded encrypted. A copy found on the
	// server (resume) is not known to be, and counts as clear.
	encrypted bool
}

type syncKind int

const (
	syncUpload syncKind = iota // upload raw, then retire the previous copy
	syncRetire                 // retire the current copy (discard, after send)
	syncAdopt                  // find the session's copy already on the server (resume)
)

// syncRequest is one piece of server work for a session. Only the latest
// pending request runs: an upload replaces an older upload, and a retire
// replaces an upload the user no longer wants.
type syncRequest struct {
	kind   syncKind
	raw    []byte
	client mailclient.MailClient
	// encrypted is whether the session was encrypted when this was asked
	// for. A clear copy retired by an encrypted session is deleted outright.
	encrypted bool
}

// draftSync is one session's server-side state.
type draftSync struct {
	inFlight bool
	server   serverDraft
	pending  *syncRequest
	// quit is set when the program should exit once this session's work is
	// done; lastUpload says how the last upload went, for the exit notice.
	quit       bool
	lastUpload string
}

// draftSyncedMsg reports one finished server operation.
type draftSyncedMsg struct {
	session string
	kind    syncKind
	err     error
	draft   serverDraft
}

// draftStore is the local drafts directory: under the root config's download
// directory, so a test model with a temporary one never touches the real cache.
func (m *tuiModel) draftStore() draft.Store {
	dir := ""
	if m.cfg != nil {
		dir = m.cfg.CacheRoot
	}
	if dir == "" {
		if c := m.Config(); c != nil {
			dir = c.CacheRoot
		}
	}
	return draft.Store{Root: draft.Root(dir)}
}

// errDraftEmpty is a message with no subject, body or attachment: nothing
// worth a file, and never worth overwriting a saved one with.
var errDraftEmpty = errors.New("draft is empty")

func (m *tuiModel) isDraftEmpty() bool {
	return strings.TrimSpace(m.compose.Body) == "" && strings.TrimSpace(m.compose.Subject) == "" && len(m.compose.Attachments) == 0
}

// saveLocalDraft writes the session's draft.eml, creating it on first save.
// A rewrite that fails is reported, not papered over: falling back to a new
// file used to fork the draft into two, the older one silently stale. Only a
// file removed from under us (drafts rm) is recreated.
func (m *tuiModel) saveLocalDraft() (string, error) {
	path, _, err := m.saveDraftCopy()
	return path, err
}

// saveDraftCopy is saveLocalDraft returning the bytes it wrote as well, nil
// when nothing was built. They are the bytes the server copy must hold: the
// draft form, never the outgoing one, which is encrypted to the recipients
// rather than to the sender and may carry a signature.
func (m *tuiModel) saveDraftCopy() (string, []byte, error) {
	if m.compose == nil {
		return "", nil, fmt.Errorf("no compose message active")
	}
	if m.isDraftEmpty() {
		return m.compose.DraftPath, nil, errDraftEmpty
	}

	rawBytes, err := m.buildDraftRFC822()
	if err != nil {
		return "", nil, fmt.Errorf("failed to build draft RFC822: %w", err)
	}
	path, err := m.writeLocalDraft(rawBytes)
	return path, rawBytes, err
}

// writeLocalDraft stores bytes already built for this session.
//
// It exists because building a draft is not free -- an encrypted one runs gpg
// -- and saveAndSyncDraftCmd needs the same bytes twice, for the file and for
// the upload. Building twice would run gpg twice and, worse, produce two
// different ciphertexts for one draft.
func (m *tuiModel) writeLocalDraft(rawBytes []byte) (string, error) {
	if m.compose.DraftPath != "" {
		err := draft.Write(m.compose.DraftPath, rawBytes)
		if err == nil {
			return m.compose.DraftPath, nil
		}
		if _, statErr := os.Stat(filepath.Dir(m.compose.DraftPath)); !errors.Is(statErr, os.ErrNotExist) {
			return m.compose.DraftPath, err
		}
	}

	path, err := m.draftStore().Create(time.Now(), rawBytes)
	if err != nil {
		return "", err
	}
	m.compose.DraftPath = path
	return path, nil
}

func cleanupLocalDraft(path string) error {
	return draft.Remove(path)
}

// composeSession is the open session's key, assigned on first use. The
// Message-ID doubles as the key because it is already fixed per session.
func (m *tuiModel) composeSession() string {
	if m.compose.MessageID == "" {
		m.compose.MessageID = messageIDForCompose(m.compose)
	}
	return m.compose.MessageID
}

func (m *tuiModel) syncFor(session string) *draftSync {
	if m.draftSyncs == nil {
		m.draftSyncs = map[string]*draftSync{}
	}
	st := m.draftSyncs[session]
	if st == nil {
		st = &draftSync{}
		m.draftSyncs[session] = st
	}
	return st
}

// saveAndSyncDraftCmd is what every edit calls: write the local copy now and
// queue an upload of it. A local failure is shown on the status line.
func (m *tuiModel) saveAndSyncDraftCmd() tea.Cmd {
	if m.compose == nil || m.isDraftEmpty() {
		return nil
	}
	raw, err := m.buildDraftRFC822()
	if err != nil {
		m.saveStatus = "Local draft not saved: " + err.Error()
		m.saveStatusTime = time.Now()
		return nil
	}
	if _, err := m.writeLocalDraft(raw); err != nil {
		m.saveStatus = "Local draft not saved: " + err.Error()
		m.saveStatusTime = time.Now()
	}
	return m.requestSync(m.composeSession(), syncRequest{kind: syncUpload, raw: raw, client: m.client, encrypted: m.compose.Encrypt})
}

// canSync reports whether a request may reach the server at all.
func (m *tuiModel) canSync(client mailclient.MailClient) bool {
	return client != nil && !m.isAccountReadOnly()
}

// requestSync queues req for the session, or starts it when nothing is
// running. A request with nothing to do (read-only, no copy to retire)
// starts nothing; if the session was asked to quit, the program quits now.
func (m *tuiModel) requestSync(session string, req syncRequest) tea.Cmd {
	st := m.syncFor(session)
	if !m.canSync(req.client) || (req.kind == syncRetire && st.server.ID == "" && !st.inFlight) {
		return m.settleSession(session, st)
	}
	if st.inFlight {
		st.pending = &req
		return nil
	}
	return m.startSync(session, st, req)
}

// requestQuitAfterSync marks the session so the program exits once its
// server work is done, and swallows keys until then (see handleKey).
func (m *tuiModel) requestQuitAfterSync(session string, req syncRequest) tea.Cmd {
	st := m.syncFor(session)
	st.quit = true
	cmd := m.requestSync(session, req)
	if m.done {
		return cmd
	}
	m.exitPending = true
	m.showLoad = "Uploading draft..."
	if req.kind == syncRetire {
		m.showLoad = "Removing server draft..."
	}
	return cmd
}

func (m *tuiModel) startSync(session string, st *draftSync, req syncRequest) tea.Cmd {
	st.inFlight = true
	prev := st.server
	folders := m.bk.folders()
	switch req.kind {
	case syncRetire:
		return func() tea.Msg {
			retireServerDraft(folders, prev, req.encrypted)
			return draftSyncedMsg{session: session, kind: syncRetire}
		}
	case syncAdopt:
		return func() tea.Msg {
			found, err := findServerCopy(req.client, session)
			return draftSyncedMsg{session: session, kind: syncAdopt, err: err, draft: found}
		}
	}
	return func() tea.Msg {
		account := clientAccountName(req.client)
		folder := resolveDraftsFolder(folders, req.client)
		id, err := req.client.UploadRawEmail(req.raw, folder)
		if err != nil {
			slog.Warn("draft upload failed", slog.String("folder", folder.String()), slog.Any("error", err))
			return draftSyncedMsg{session: session, err: err, draft: serverDraft{Account: account, Folder: folder}}
		}
		slog.Info("draft upload succeeded", slog.String("folder", folder.String()), slog.String("id", string(id)))
		// Upload first, retire second: a failure leaves the older copy, never none.
		retireServerDraft(folders, prev, req.encrypted)
		return draftSyncedMsg{session: session, draft: serverDraft{ID: id, Account: account, Folder: folder,
			client: req.client, encrypted: req.encrypted}}
	}
}

// handleDraftSynced records what a finished operation left on the server,
// starts the request that queued behind it, and finishes an exit that was
// waiting. A message for a session that is gone and idle is dropped.
func (m *tuiModel) handleDraftSynced(msg draftSyncedMsg) (tea.Model, tea.Cmd) {
	st := m.draftSyncs[msg.session]
	if st == nil {
		slog.Info("draft sync result for an unknown session", slog.String("session", msg.session))
		return m, nil
	}
	st.inFlight = false
	switch {
	case msg.kind == syncRetire:
		// The copy is gone; an earlier upload is no longer news.
		st.server = serverDraft{}
		st.lastUpload = ""
	case msg.kind == syncAdopt:
		if msg.err == nil && msg.draft.ID != "" && st.server.ID == "" {
			st.server = msg.draft
		}
	case msg.err != nil:
		st.lastUpload = fmt.Sprintf("upload to %s failed: %v", where(msg.draft), msg.err)
	default:
		st.server = msg.draft
		st.lastUpload = "uploaded to " + where(msg.draft)
	}
	if st.pending != nil {
		req := *st.pending
		st.pending = nil
		return m, m.startSync(msg.session, st, req)
	}
	return m, m.settleSession(msg.session, st)
}

// settleSession runs when a session has nothing in flight: it finishes a
// waiting exit, and forgets a session that has ended.
func (m *tuiModel) settleSession(session string, st *draftSync) tea.Cmd {
	if st.inFlight {
		return nil
	}
	if st.quit {
		m.saveStatus = joinStatus(m.saveStatus, st.lastUpload)
		return m.quitNow()
	}
	if m.compose == nil || m.compose.MessageID != session {
		delete(m.draftSyncs, session)
	}
	return nil
}

// quitNow ends the program, leaving the status line as the exit notice.
func (m *tuiModel) quitNow() tea.Cmd {
	m.showLoad = ""
	m.exitPending = false
	m.exitNotice = m.saveStatus
	m.done = true
	return tea.Quit
}

func where(d serverDraft) string {
	if d.Account == "" {
		return d.Folder.String()
	}
	return d.Account + "/" + d.Folder.String()
}

func joinStatus(parts ...string) string {
	var kept []string
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, "; ")
}

func clientAccountName(client mailclient.MailClient) string {
	if cfg := client.Config(); cfg != nil {
		return string(cfg.SelectedAccount)
	}
	return ""
}

// The injected resolver is the one to ask; organize is the fallback for a
// model built without a Backend, which the tests do.
func resolveDraftsFolder(folders FolderResolver, client mailclient.MailClient) label.Folder {
	if target, err := folders.ResolveDraftsTarget(client); err == nil && !target.IsZero() {
		return target
	}
	if target, err := organize.ResolveDraftsTarget(client); err == nil && !target.IsZero() {
		return target
	}
	return label.FromServer("Drafts")
}

func resolveTrashFolder(folders FolderResolver, client mailclient.MailClient) label.Folder {
	if target, err := folders.ResolveTrashTarget(client); err == nil && !target.IsZero() {
		return target
	}
	if target, err := organize.ResolveTrashTarget(client); err == nil && !target.IsZero() {
		return target
	}
	return label.Folder{}
}

// retireServerDraft removes a superseded copy through the client that
// uploaded it. Usually it goes to the trash, which expires on its own; what
// matters is that Drafts holds one copy.
//
// A copy uploaded in the clear by a session that is now encrypted is deleted
// permanently instead: moved to the trash, it would keep the plaintext the
// session was encrypted to protect on the server for another month. If the
// delete fails it still goes to the trash rather than staying in Drafts.
func retireServerDraft(folders FolderResolver, prev serverDraft, sessionEncrypted bool) {
	if prev.ID == "" || prev.client == nil {
		return
	}
	if sessionEncrypted && !prev.encrypted {
		err := prev.client.DeleteMessages([]email.ID{prev.ID}, prev.Folder)
		if err == nil {
			slog.Info("deleted the clear-text server draft", slog.String("id", string(prev.ID)))
			return
		}
		slog.Warn("deleting the clear-text server draft failed; moving it to the trash",
			slog.String("id", string(prev.ID)), slog.Any("error", err))
	}
	trash := resolveTrashFolder(folders, prev.client)
	if trash.IsZero() || trash == prev.Folder {
		return
	}
	if err := prev.client.MoveEmail([]email.ID{prev.ID}, prev.Folder, trash); err != nil {
		slog.Warn("retiring the previous server draft failed", slog.String("id", string(prev.ID)), slog.Any("error", err))
		return
	}
	slog.Info("retired previous server draft", slog.String("id", string(prev.ID)), slog.String("to", trash.String()))
}

// findServerCopy looks for the resumed session's copy in Drafts by
// Message-ID, so the first upload after a resume retires it instead of
// leaving it beside the new one.
func findServerCopy(client mailclient.MailClient, messageID string) (serverDraft, error) {
	folder, drafts, err := organize.ListServerDrafts(client)
	if err != nil {
		return serverDraft{}, err
	}
	for _, d := range drafts {
		if d.MessageID != "" && d.MessageID == messageID {
			return serverDraft{ID: d.ID, Account: clientAccountName(client), Folder: folder, client: client}, nil
		}
	}
	return serverDraft{}, nil
}

func readSignature() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	sigPath := filepath.Join(home, ".signature")
	data, err := os.ReadFile(sigPath)
	if err != nil {
		return ""
	}
	sigText := string(data)
	if sigText != "" {
		return "\n-- \n" + strings.TrimSuffix(sigText, "\n") + "\n"
	}
	return ""
}
