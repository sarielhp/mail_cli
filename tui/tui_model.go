package tui

import (
	"bytes"
	"context"
	"fmt"
	"github.com/sarielhp/mail_cli/cache"
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"github.com/sarielhp/mail_cli/cache/folderindex"
	"github.com/sarielhp/mail_cli/cache/msg"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/label"
	"github.com/sarielhp/mail_cli/organize"
	"github.com/sarielhp/mail_cli/pgp"
	"github.com/sarielhp/mail_cli/report"
	"github.com/sarielhp/mail_cli/scan"
	"github.com/sarielhp/mail_cli/spam"
	"log/slog"
	"mime"
	"net/mail"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *tuiModel) Config() *cfg_g.Config {
	if m.client != nil {
		return m.client.Config()
	}
	return nil
}

func (m *tuiModel) isAccountReadOnly() bool {
	if m.Config() != nil && m.Config().ReadOnly {
		return true
	}
	if m.cfg != nil && m.cfg.ReadOnly {
		return true
	}
	accounts := m.getAccounts()
	idx := m.currentAccountIndex()
	if idx >= 0 && idx < len(accounts) {
		return accounts[idx].ReadOnly
	}
	return false
}

func (m *tuiModel) isCurrentFolderReadOnly() bool {
	if m.isAccountReadOnly() {
		return true
	}
	if m.lockedFolders != nil && m.lockedFolders[strings.ToLower(m.currentF)] {
		return true
	}
	accounts := m.getAccounts()
	idx := m.currentAccountIndex()
	if idx >= 0 && idx < len(accounts) {
		return accounts[idx].IsFolderReadOnly(m.currentF)
	}
	return false
}

func (m *tuiModel) isReadOnly() bool {
	return m.isCurrentFolderReadOnly()
}

func (m *tuiModel) Init() tea.Cmd {
	if m.standaloneCompose {
		return m.initStandaloneComposeCmd()
	}
	m.lastTime = time.Now()
	var cmds []tea.Cmd
	cmds = append(cmds, tea.Tick(time.Second*10, func(time.Time) tea.Msg { return timeTickMsg{} }))
	cmds = append(cmds, m.bgFetchFoldersCmd())
	if m.currentF != "" {
		if len(m.emails) == 0 {
			m.showLoad = "Downloading..."
		}
		cmds = append(cmds, m.loadEmailsCmd(m.currentF))
	} else {
		m.showLoad = "Downloading..."
		cmds = append(cmds, func() tea.Msg {
			items, err := m.client.GetLabelItems()
			if err != nil {
				return errMsg{err}
			}
			return foldersLoadedMsg{folders: items}
		})
	}
	return tea.Batch(cmds...)
}

func (m *tuiModel) bgFetchFoldersCmd() tea.Cmd {
	return func() tea.Msg {
		items, err := m.client.GetLabelItems()
		if err != nil {
			return errMsg{err}
		}
		return foldersLoadedMsg{folders: items}
	}
}

func (m *tuiModel) loadEmailsCmd(folder string) tea.Cmd {
	if isUnifiedInbox(folder) {
		return m.loadUnifiedEmailsCmd()
	}
	return func() tea.Msg {
		cacheDir := m.client.Config().AccountCache
		slog.Info("loadEmailsCmd: calling FetchAndDownloadEmails", slog.String("folder", folder), slog.String("cacheDir", cacheDir.Path()))
		ids, err := m.client.FetchAndDownloadEmails(label.FromServer(folder), "")
		if err != nil {
			slog.Error("loadEmailsCmd: FetchAndDownloadEmails failed", slog.String("folder", folder), slog.Any("error", err))
			return errMsg{err}
		}
		// Locals, not model fields: this runs on a tea.Cmd goroutine, and the key
		// handlers write these same maps from Update.
		isRead := cache.LoadReadState(cacheDir)
		isReplied := cache.LoadRepliedState(cacheDir)
		var msgs []email.Email
		var classifyIDs []email.ID
		// Sent and Drafts hold the user's own mail: no verdict, and any cached one is cleared.
		authored := organize.IsAuthoredFolder(m.client, label.FromServer(folder))
		stamp := scan.Stamp(m.client.Config())
		for _, id := range ids {
			if authored {
				_ = msg.ForgetEvidence(cacheDir, id)
			}
			// A message that will not load is left out rather than shown as an
			// error row: the TUI is a browser, not a listing.
			em, err := msg.CachedEmail(cacheDir, id)
			if err != nil {
				continue
			}
			em.IsRead = isRead[id]
			em.IsReplied = isReplied[id]
			needsClassify := !authored
			if info, infoErr := msg.GetInfo(cacheDir, id); infoErr == nil && !authored {
				needsClassify = info.NeedsScan(stamp)
			}
			if needsClassify {
				classifyIDs = append(classifyIDs, id)
			}
			msgs = append(msgs, *em)
		}
		sort.Slice(msgs, func(i, j int) bool {
			return msgs[i].EmailDate.After(msgs[j].EmailDate)
		})
		return emailsLoadedMsg{emails: msgs, classifyIDs: classifyIDs, isRead: isRead, isReplied: isReplied}
	}
}

func (m *tuiModel) bgClassifyCmd(ids []email.ID) tea.Cmd {
	return func() tea.Msg {
		if len(ids) == 0 {
			return classifyDoneMsg{}
		}
		slog.Info("Background classification starting", slog.Int("count", len(ids)))
		_, _, _, err := scan.Emails(report.Or(m.rep), ids, m.client.Config(), "")
		if err != nil {
			slog.Error("Background classification failed", slog.Any("error", err))
		} else {
			slog.Info("Background classification complete", slog.Int("count", len(ids)))
		}
		return classifyDoneMsg{}
	}
}

// loadDetailCmd plans on the Update goroutine which account holds id; the
// command reads neither m.currentF nor the rows.
func (m *tuiModel) loadDetailCmd(id email.ID) tea.Cmd {
	plan := m.planAction([]email.ID{id}, m.currentF)
	return func() tea.Msg {
		client, _, err := m.target(plan, id)
		if err != nil {
			return detailMsg{headers: "(Unavailable)", body: "(Could not resolve this message's account: " + err.Error() + ")"}
		}
		data, err := client.GetRawMessage(id)
		if err != nil {
			return detailMsg{headers: "(Unavailable)", body: "(Could not retrieve this message: " + err.Error() + ")"}
		}
		msg, err := mail.ReadMessage(bytes.NewReader(data))
		if err != nil {
			return detailMsg{headers: "(Parse error)", body: err.Error()}
		}
		dec := new(mime.WordDecoder)
		subj := email.DecDecode(dec, msg.Header.Get("Subject"))
		frm := email.DecDecode(dec, msg.Header.Get("From"))
		dS := msg.Header.Get("Date")
		hdrs := fmt.Sprintf("From: %s\nSubject: %s\nDate: %s", frm, subj, dS)

		// PGP runs before the body is read: for an encrypted message there is
		// no body to read until it has been decrypted, and the decrypted
		// entity -- not the multipart envelope -- is what carries the
		// attachments.
		processed := pgp.Process(context.Background(), data, pgp.Options{Verify: true, Decrypt: true})
		atts, _ := email.ExtractAttachments(processed.Raw)
		// The detail renderer wraps at the current viewport width after
		// compacting links; prewrapping here can damage URLs and spacing.
		return detailMsg{headers: hdrs, body: detailBody(processed), attachments: atts, crypto: processed.Status}
	}
}

// detailBody is the text of a message after the PGP layer: the plaintext when
// the message was decrypted or was an inline signature, and the message's own
// plain part otherwise.
func detailBody(processed pgp.Result) string {
	if processed.Body != "" {
		return email.EmbedOSC8Hyperlinks(email.StripEscapeSequences(processed.Body))
	}
	msg, err := mail.ReadMessage(bytes.NewReader(processed.Raw))
	if err != nil {
		return ""
	}
	body, _ := email.ExtractPlainBodyText(msg)
	return email.EmbedOSC8Hyperlinks(body)
}

// cmdArchive takes a plan made on the Update goroutine rather than reading
// m.currentF or the rows inside the closure. This runs on a tea.Cmd goroutine,
// so reading the field there picked up whatever folder the user had navigated
// to by the time the network call ran -- and that value is sent as the move's
// *source* label, which on Gmail removes the wrong label.
func (m *tuiModel) cmdArchive(id email.ID, plan actionPlan) tea.Cmd {
	return func() tea.Msg {
		client, realSrc, err := m.target(plan, id)
		if err != nil {
			return errMsg{err: err}
		}
		tgt, err := m.bk.folders().ResolveArchiveTarget(client)
		if err != nil {
			return errMsg{err: err}
		}
		// The index and the stale classification are reconciled by
		// CheckingMailClient, which wraps this client and owns that job for
		// every caller. Doing it again here was not merely redundant: the
		// decorator skips reconciliation under --read-only, because the move
		// was only simulated, and this copy did not -- so the cache recorded a
		// move the server never made.
		if err := client.MoveEmail([]email.ID{id}, label.FromServer(realSrc), tgt); err != nil {
			return errMsg{err: err}
		}
		return archiveDoneMsg{id: id}
	}
}

// cmdSpam marks a message as spam, through the same path as `spam mark`.
func (m *tuiModel) cmdSpam(id email.ID, plan actionPlan) tea.Cmd {
	return func() tea.Msg {
		client, _, err := m.target(plan, id)
		if err != nil {
			return errMsg{err: err}
		}
		cfg := client.Config()
		// Existence is all this needs; reading the body to find out wasted the
		// read.
		if ok, err := msg.Exists(cfg.AccountCache, id); err != nil || !ok {
			return spamDoneMsg{id: id}
		}
		if err := spam.MarkByID(report.Or(m.rep), client, cfg, string(id)); err != nil {
			slog.Warn("marking message as spam failed", slog.String("id", string(id)), slog.Any("error", err))
		}
		return spamDoneMsg{id: id}
	}
}

func (m *tuiModel) cmdUnspam(id email.ID) tea.Cmd {
	return func() tea.Msg {
		startTime := time.Now()
		cfg := m.client.Config()
		err := spam.UnspamByID(report.Or(m.rep), m.client, cfg, string(id))
		elapsed := time.Since(startTime)
		if elapsed < time.Second {
			time.Sleep(time.Second - elapsed)
		}
		if err != nil {
			return errMsg{err: err}
		}
		return unspamDoneMsg{id: id}
	}
}

// cmdDelete takes a plan; see cmdArchive.
func (m *tuiModel) cmdDelete(id email.ID, plan actionPlan) tea.Cmd {
	return func() tea.Msg {
		client, realSrc, err := m.target(plan, id)
		if err != nil {
			return errMsg{err: err}
		}
		cfg := client.Config()
		cd := cfg.AccountCache
		tgt, err := m.bk.folders().ResolveTrashTarget(client)
		moved := false
		if err == nil {
			// As in cmdArchive, the index is CheckingMailClient's job.
			moved = client.MoveEmail([]email.ID{id}, label.FromServer(realSrc), tgt) == nil
		}
		// The cached body goes only when the server accepted the move; dropping
		// it after a refusal would hide a message that is still in the folder.
		if moved {
			_ = msg.Delete(cd, id)
		}
		return deleteDoneMsg{id: id}
	}
}

func (m *tuiModel) loadCachedEmailsForFolder(folder string) {
	if isUnifiedInbox(folder) {
		m.loadUnifiedCachedEmails()
		return
	}
	if m.client == nil {
		slog.Warn("loadCachedEmailsForFolder: client is nil")
		return
	}
	if m.client.Config() == nil {
		slog.Warn("loadCachedEmailsForFolder: client config is nil")
		return
	}
	cacheDir := m.client.Config().AccountCache
	slog.Info("loadCachedEmailsForFolder starting", slog.String("folder", folder), slog.String("cacheDir", cacheDir.Path()))

	if !folderindex.HasStructure(cacheDir) {
		slog.Info("loadCachedEmailsForFolder: cache structure not found, skipping")
		return
	}

	ids, err := folderindex.IDs(cacheDir, folder)
	if err != nil {
		slog.Info("loadCachedEmailsForFolder: no folder index", slog.String("folder", folder))
		return
	}
	var cachedEmails []email.Email
	for _, id := range ids {
		if em, ok := m.loadCachedEmail(cacheDir, id); ok {
			cachedEmails = append(cachedEmails, em)
		}
	}

	prevSelectedID := m.selectedID
	prevFolder := m.currentF

	m.rawEmails = cachedEmails
	m.rebuildVisibleEmails()

	if prevFolder == folder && prevSelectedID != "" {
		found := false
		for i, e := range m.emails {
			if e.ID == prevSelectedID {
				m.eIdx = i
				m.selectedID = prevSelectedID
				found = true
				break
			}
		}
		if !found {
			if len(m.emails) > 0 {
				m.eIdx = min(m.eIdx, len(m.emails)-1)
				m.selectedID = m.emails[m.eIdx].ID
			} else {
				m.eIdx = 0
				m.selectedID = ""
			}
		}
	} else {
		m.folderJustChanged = true
		if len(m.emails) > 0 {
			m.eIdx = len(m.emails) - 1
			m.selectedID = m.emails[m.eIdx].ID
		} else {
			m.eIdx = 0
			m.selectedID = ""
		}
	}
	m.adjustScrollOffset()
}

// loadCachedEmail reads one message from the account cache and overlays the
// session's read/replied state and the stored spam classification. ok is false
// when the message is absent or unparseable.
func (m *tuiModel) loadCachedEmail(cacheDir cachedir.Account, id email.ID) (email.Email, bool) {
	data, err := msg.Read(cacheDir, id)
	if err != nil {
		return email.Email{}, false
	}
	em := email.ParseReader(bytes.NewReader(data), id, "")
	if em == nil {
		return email.Email{}, false
	}
	em.IsRead = m.isRead[id]
	em.IsReplied = m.isReplied[id]
	if info, err := msg.GetInfo(cacheDir, id); err == nil {
		em.IsSpam = info.IsSpam()
		em.IsPolitical = info.IsPolitical()
		em.IsBlacklisted = info.IsBlacklisted()
	}
	return *em, true
}

func (m *tuiModel) removeEmailByID(id email.ID) {
	newRaw := make([]email.Email, 0, len(m.rawEmails))
	for _, e := range m.rawEmails {
		if e.ID != id {
			newRaw = append(newRaw, e)
		}
	}
	m.rawEmails = newRaw
	m.rebuildVisibleEmails()
	if len(m.emails) > 0 {
		m.eIdx = min(m.eIdx, len(m.emails)-1)
		m.selectedID = m.emails[m.eIdx].ID
	} else {
		m.eIdx = 0
		m.selectedID = ""
	}
	m.adjustScrollOffset()
}

func (m *tuiModel) cleanupIsReadMap() {
	if len(m.isRead) > 10000 {
		slog.Info("Cleaning up isRead map", slog.Int("currentSize", len(m.isRead)))
		ids := make(map[email.ID]bool)
		for _, e := range m.rawEmails {
			ids[e.ID] = true
		}
		for id := range m.isRead {
			if !ids[id] {
				delete(m.isRead, id)
			}
		}
		slog.Info("Cleaned up isRead map", slog.Int("newSize", len(m.isRead)))
	}
}

func (m *tuiModel) loadAllCachedEmails() {
	if m.client == nil || m.client.Config() == nil {
		return
	}
	cacheDir := m.client.Config().AccountCache
	if !folderindex.HasStructure(cacheDir) {
		return
	}

	var allEmails []email.Email
	if len(m.folders) == 0 {
		return
	}

	for _, f := range m.folders {
		ids, err := folderindex.IDs(cacheDir, f.FullName)
		if err != nil {
			continue
		}
		for _, id := range ids {
			if em, ok := m.loadCachedEmail(cacheDir, id); ok {
				allEmails = append(allEmails, em)
			}
		}
	}

	sort.Slice(allEmails, func(i, j int) bool {
		return allEmails[i].EmailDate.After(allEmails[j].EmailDate)
	})

	m.savedRawEmails = m.rawEmails
	m.savedCurrentF = m.currentF
	m.rawEmails = allEmails
}
