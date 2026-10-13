package tui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/mailclient"

	tea "github.com/charmbracelet/bubbletea"
)

func composeWithTrash(t *testing.T) (*tuiModel, *mailclient.MockMailClient) {
	t.Helper()
	m, mock, tempDir := setupTestComposeModel(t, false)
	m.cfg.ConfigDir = tempDir
	m.bk.Folders = stubFolders{
		DraftsFn: func(mailclient.MailClient) (string, error) { return "DRAFTS", nil },
		TrashFn:  func(mailclient.MailClient) (string, error) { return "TRASH", nil },
	}
	m.mode = modeCompose
	m.compose = &composeState{
		AccountName: "Personal",
		From:        "user@personal.com",
		To:          "someone@example.com",
		Subject:     "Keep one copy",
		Body:        "first",
	}
	return m, mock
}

// liveDrafts is what is left in DRAFTS: every upload minus what was moved out.
func liveDrafts(mock *mailclient.MockMailClient) []string {
	moved := map[string]bool{}
	for _, mv := range mock.MovedEmails {
		if mv.SourceLabel == "DRAFTS" {
			for _, id := range mv.MessageIDs {
				moved[string(id)] = true
			}
		}
	}
	var live []string
	for i := range mock.UploadedEmails {
		id := uploadID(i)
		if !moved[id] {
			live = append(live, id)
		}
	}
	return live
}

func uploadID(i int) string {
	return "upload_msg_" + string(rune('1'+i))
}

// stubWorkAccount makes every per-account client lookup resolve to work, and
// stamps each resolved config with the account it was looked up for.
func stubWorkAccount(m *tuiModel, work *mailclient.MockMailClient) {
	m.bk.Clients = stubClients{
		ForAccountFn: func(c *cfg_g.Config, acc cfg_acc.AccountConfig) (mailclient.MailClient, error) {
			cfg := *c
			cfg.SelectedAccount = cfg_g.NameOf(acc)
			work.Cfg = &cfg
			return work, nil
		},
	}
}

// drain runs cmd and feeds every message back through Update, the way the
// program would, until nothing more is returned. It reports whether the
// program quit.
func drain(t *testing.T, m *tuiModel, cmd tea.Cmd) bool {
	t.Helper()
	for i := 0; cmd != nil && i < 20; i++ {
		msg := cmd()
		if _, ok := msg.(tea.QuitMsg); ok {
			return true
		}
		if msg == nil {
			return false
		}
		_, cmd = m.Update(msg)
	}
	return false
}

func key(r string) tea.KeyMsg {
	switch r {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "ctrl+c":
		return tea.KeyMsg{Type: tea.KeyCtrlC}
	case "f3":
		return tea.KeyMsg{Type: tea.KeyF3}
	case "f5":
		return tea.KeyMsg{Type: tea.KeyF5}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(r)}
}

func press(m *tuiModel, k string) tea.Cmd {
	_, cmd := m.Update(key(k))
	return cmd
}

// Every edit used to upload a fresh copy and forget the previous one.
func TestDraftSync_RetiresPreviousServerCopy(t *testing.T) {
	m, mock := composeWithTrash(t)
	session := m.composeSession()

	drain(t, m, m.saveAndSyncDraftCmd())
	m.compose.Body = "second"
	drain(t, m, m.saveAndSyncDraftCmd())

	if live := liveDrafts(mock); len(live) != 1 || live[0] != "upload_msg_2" {
		t.Fatalf("live drafts = %v, want only the newest", live)
	}
	if got := m.draftSyncs[session].server.ID; got != "upload_msg_2" {
		t.Errorf("server copy = %q", got)
	}
}

func TestDraftSync_SerializesUploads(t *testing.T) {
	m, mock := composeWithTrash(t)

	inFlight := m.saveAndSyncDraftCmd()
	m.compose.Body = "edited meanwhile"
	if cmd := m.saveAndSyncDraftCmd(); cmd != nil {
		t.Fatal("a second edit must wait for the upload in flight")
	}
	drain(t, m, inFlight)

	if len(mock.UploadedEmails) != 2 || len(liveDrafts(mock)) != 1 {
		t.Fatalf("uploads %d live %v", len(mock.UploadedEmails), liveDrafts(mock))
	}
	if !strings.Contains(string(mock.UploadedEmails[1].RawBytes), "edited meanwhile") {
		t.Error("the queued upload did not carry the latest edit")
	}
}

// Quitting while an upload ran used to start a second upload beside it:
// two copies stayed in Drafts.
func TestDraftSync_QuitDuringUploadLeavesOneCopy(t *testing.T) {
	m, mock := composeWithTrash(t)
	m.standaloneCompose = true

	inFlight := m.saveAndSyncDraftCmd()
	m.compose.Body = "final text"
	quitCmd := press(m, "q")
	if quitCmd != nil {
		t.Fatal("the exit must queue behind the upload in flight")
	}
	if !drain(t, m, inFlight) {
		t.Fatal("the program did not quit once the upload finished")
	}
	live := liveDrafts(mock)
	if len(live) != 1 || !strings.Contains(string(mock.UploadedEmails[len(mock.UploadedEmails)-1].RawBytes), "final text") {
		t.Fatalf("live drafts = %v; want exactly the final copy", live)
	}
	if !strings.Contains(m.exitNotice, "uploaded to Personal/DRAFTS") {
		t.Errorf("exitNotice = %q", m.exitNotice)
	}
}

func TestDraftSync_DiscardDuringUploadLeavesNothing(t *testing.T) {
	m, mock := composeWithTrash(t)
	m.standaloneCompose = true

	inFlight := m.saveAndSyncDraftCmd()
	press(m, "x")
	if cmd := press(m, "y"); cmd != nil {
		t.Fatal("the discard must queue behind the upload in flight")
	}
	if !drain(t, m, inFlight) {
		t.Fatal("no quit after discard")
	}
	if live := liveDrafts(mock); len(live) != 0 {
		t.Fatalf("live drafts = %v after discard", live)
	}
	if m.exitNotice != "Draft discarded" {
		t.Errorf("exitNotice = %q", m.exitNotice)
	}
}

func TestDraftSync_DiscardPromptDoesNotTakeEnter(t *testing.T) {
	m, _ := composeWithTrash(t)
	press(m, "x")
	press(m, "enter")
	if m.compose == nil {
		t.Fatal("Enter discarded the draft; only y may")
	}
}

// Sending while an upload ran left the new copy in Drafts after the
// message had gone out.
func TestDraftSync_SendDuringUploadLeavesNothing(t *testing.T) {
	m, mock := composeWithTrash(t)
	m.standaloneCompose = true

	inFlight := m.saveAndSyncDraftCmd()
	send := press(m, "y")
	drain(t, m, inFlight)
	if !drain(t, m, send) {
		t.Fatal("no quit after send")
	}
	if len(mock.SentRawEmails) != 1 {
		t.Fatalf("sent %d times", len(mock.SentRawEmails))
	}
	if live := liveDrafts(mock); len(live) != 0 {
		t.Fatalf("live drafts = %v after send", live)
	}
	if !strings.Contains(m.exitNotice, "sent") {
		t.Errorf("exitNotice = %q", m.exitNotice)
	}
}

// A second q, Esc or Ctrl+C while "Uploading draft..." showed used to quit
// at once and drop the upload.
func TestDraftSync_KeysDuringExitWaitAreSwallowed(t *testing.T) {
	m, mock := composeWithTrash(t)
	m.standaloneCompose = true

	upload := press(m, "q")
	if !m.exitPending {
		t.Fatal("exit is not waiting for the upload")
	}
	for _, k := range []string{"q", "esc", "enter"} {
		if cmd := press(m, k); cmd != nil || m.done {
			t.Fatalf("%q during the wait returned a command or quit", k)
		}
	}
	if !drain(t, m, upload) || len(mock.UploadedEmails) != 1 {
		t.Fatalf("quit after upload: uploads %d", len(mock.UploadedEmails))
	}
}

func TestDraftSync_TwoCtrlCAbandonTheWait(t *testing.T) {
	m, _ := composeWithTrash(t)
	m.standaloneCompose = true

	press(m, "q")
	if cmd := press(m, "ctrl+c"); cmd != nil || m.done {
		t.Fatal("one Ctrl+C must only arm the abandon")
	}
	cmd := press(m, "ctrl+c")
	if cmd == nil || !m.done || !strings.Contains(m.exitNotice, "abandoned") || !strings.Contains(m.exitNotice, "Draft saved to") {
		t.Fatalf("done=%v notice=%q", m.done, m.exitNotice)
	}
}

// Enter twice during "Sending email..." used to send twice.
func TestDraftSync_SendIsNotRepeated(t *testing.T) {
	m, mock := composeWithTrash(t)
	first := press(m, "y")
	if second := press(m, "enter"); second != nil {
		t.Fatal("a second send was dispatched while the first was in flight")
	}
	drain(t, m, first)
	if len(mock.SentRawEmails) != 1 {
		t.Fatalf("sent %d times", len(mock.SentRawEmails))
	}
}

// Ctrl+C mid-send used to save and upload a draft of a message going out.
func TestDraftSync_CtrlCDuringSendWaitsForIt(t *testing.T) {
	m, mock := composeWithTrash(t)
	send := press(m, "y")
	if cmd := press(m, "ctrl+c"); cmd != nil || m.done {
		t.Fatal("Ctrl+C during a send must wait for it")
	}
	if !drain(t, m, send) {
		t.Fatal("did not exit after the send")
	}
	if len(mock.UploadedEmails) != 0 || len(mock.SentRawEmails) != 1 {
		t.Fatalf("uploads %d sends %d; want no draft of a sent message", len(mock.UploadedEmails), len(mock.SentRawEmails))
	}
}

func TestDraftSync_CtrlCAfterFailedSendKeepsDraft(t *testing.T) {
	m, mock := composeWithTrash(t)
	mock.FailOn = map[string]error{"SendEmail": errors.New("smtp down")}
	send := press(m, "y")
	press(m, "ctrl+c")
	if !drain(t, m, send) {
		t.Fatal("did not exit")
	}
	if len(liveDrafts(mock)) != 1 || !strings.Contains(m.exitNotice, "Send failed: smtp down") || !strings.Contains(m.exitNotice, "Draft saved to") {
		t.Fatalf("live %v notice %q", liveDrafts(mock), m.exitNotice)
	}
}

// A result from a finished session used to land in the next one, which then
// retired the first session's draft.
func TestDraftSync_StaleResultDoesNotReachNextSession(t *testing.T) {
	m, mock := composeWithTrash(t)
	leaving := press(m, "q") // inbox TUI: leave, upload in the background
	if m.compose != nil {
		t.Fatal("still composing")
	}

	m.mode = modeCompose
	m.compose = &composeState{From: "user@personal.com", To: "b@example.com", Subject: "Next", Body: "b"}
	drain(t, m, leaving) // session A's result arrives during session B

	drain(t, m, m.saveAndSyncDraftCmd())
	if live := liveDrafts(mock); len(live) != 2 {
		t.Fatalf("live drafts = %v; session A's copy must survive session B's upload", live)
	}
	if _, ok := m.draftSyncs[strings.TrimSpace("")]; ok {
		t.Error("an empty session key was recorded")
	}
}

// F3 used to leave the copy on the previous account for good.
func TestDraftSync_AccountSwitchRetiresOldCopyThroughOldClient(t *testing.T) {
	m, personal := composeWithTrash(t)
	work := &mailclient.MockMailClient{BackendTypeVal: "jmap"}
	stubWorkAccount(m, work)

	drain(t, m, m.saveAndSyncDraftCmd())
	_, cmd := m.switchComposeAccountTo(1)
	drain(t, m, cmd)

	if len(personal.MovedEmails) != 1 || personal.MovedEmails[0].MessageIDs[0] != "upload_msg_1" || personal.MovedEmails[0].DestLabel != "TRASH" {
		t.Fatalf("personal moves = %+v; the old copy must be retired through the old client", personal.MovedEmails)
	}
	if len(work.UploadedEmails) != 1 {
		t.Fatalf("work uploads = %d", len(work.UploadedEmails))
	}
}

// Resume used to recompute From from the selected account and forget the
// copy the earlier session left on the server.
func TestDraftSync_ResumeKeepsSenderAndAdoptsServerCopy(t *testing.T) {
	m, _ := composeWithTrash(t)
	m.compose = nil
	m.standaloneCompose = true
	raw, err := (&email.ComposeMessage{From: "Work User <user@work.com>", To: "x@example.com", Subject: "Resumed", Body: "text", MessageID: "<r1@work.com>"}).BuildRFC822()
	if err != nil {
		t.Fatal(err)
	}
	path, err := m.draftStore().Create(time.Now(), raw)
	if err != nil {
		t.Fatal(err)
	}
	work := &mailclient.MockMailClient{
		BackendTypeVal: "jmap",
		Labels:         []string{"DRAFTS"},
		DownloadedIDs:  map[string][]email.ID{"DRAFTS": {"srv-old"}},
		RawMessages:    map[email.ID][]byte{"srv-old": raw},
	}
	stubWorkAccount(m, work)
	m.composeOpts.DraftPath = path

	drain(t, m, m.initStandaloneComposeCmd())
	if m.compose == nil || m.compose.AccountName != "Work" || !strings.Contains(m.compose.From, "user@work.com") {
		t.Fatalf("resumed as %+v; want the Work sender", m.compose)
	}
	if m.compose.MessageID != "<r1@work.com>" {
		t.Errorf("Message-ID = %q", m.compose.MessageID)
	}
	drain(t, m, m.saveAndSyncDraftCmd())
	if len(work.MovedEmails) != 1 || work.MovedEmails[0].MessageIDs[0] != "srv-old" {
		t.Fatalf("work moves = %+v; the earlier session's copy must be retired", work.MovedEmails)
	}
}

// Ctrl+C while a header field was being typed dropped the typed text.
func TestDraftSync_CtrlCCommitsTheFieldBeingEdited(t *testing.T) {
	m, mock := composeWithTrash(t)
	m.standaloneCompose = true
	m.compose.FieldEditing = fieldSubject
	m.compose.FieldInput = "Typed but not committed"

	drain(t, m, press(m, "ctrl+c"))
	if len(mock.UploadedEmails) != 1 || !strings.Contains(string(mock.UploadedEmails[0].RawBytes), "Typed but not committed") {
		t.Fatal("the field being edited was not saved")
	}
}

// A local save that failed on exit used to vanish from the notice.
func TestDraftSync_ExitNoticeReportsLocalFailure(t *testing.T) {
	m, _ := composeWithTrash(t)
	m.standaloneCompose = true
	if err := os.WriteFile(filepath.Join(m.cfg.AccountCache.Path(), "drafts"), []byte("not a dir"), 0600); err != nil {
		t.Fatal(err)
	}
	drain(t, m, press(m, "q"))
	if !strings.HasPrefix(m.exitNotice, "Draft NOT saved locally") || !strings.Contains(m.exitNotice, "uploaded to") {
		t.Fatalf("exitNotice = %q", m.exitNotice)
	}
}

func TestDraftSync_EmptyExitStillSaysSo(t *testing.T) {
	m, mock := composeWithTrash(t)
	m.standaloneCompose = true
	m.compose.Subject, m.compose.Body = "", ""
	drain(t, m, press(m, "q"))
	if m.exitNotice != "Empty message; no draft saved." || len(mock.UploadedEmails) != 0 {
		t.Fatalf("notice %q uploads %d", m.exitNotice, len(mock.UploadedEmails))
	}
}

// An edit that empties the message must not upload the empty version.
func TestDraftSync_EmptiedMessageIsNotUploaded(t *testing.T) {
	m, mock := composeWithTrash(t)
	m.compose.Subject, m.compose.Body = "", ""
	if cmd := m.saveAndSyncDraftCmd(); cmd != nil || len(mock.UploadedEmails) != 0 {
		t.Fatal("an empty draft was uploaded")
	}
}

// A failed rewrite used to fork the draft into a second numbered file.
func TestDraftSync_FailedRewriteDoesNotForkTheDraft(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores permissions")
	}
	m, _ := composeWithTrash(t)
	path, err := m.saveLocalDraft()
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Dir(path)
	if err := os.Chmod(dir, 0500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(dir, 0700)

	m.compose.Body = "changed"
	if _, err := m.saveLocalDraft(); err == nil {
		t.Fatal("the failed rewrite was not reported")
	}
	if entries, _ := m.draftStore().List(); len(entries) != 1 || m.compose.DraftPath != path {
		t.Fatalf("drafts %d, path %q; want the one original", len(entries), m.compose.DraftPath)
	}
}

// A subject from -s with an untouched body used to be cancelled as "not
// modified", dropping the subject and any -a attachments.
func TestDraftSync_FlagSubjectSurvivesAnUntouchedBody(t *testing.T) {
	m, _ := composeWithTrash(t)
	m.compose = nil
	m.standaloneCompose = true
	m.composeOpts.Subject = "From the command line"
	tmp := filepath.Join(t.TempDir(), "body.txt")
	if err := os.WriteFile(tmp, []byte("\n-- \nsig\n"), 0600); err != nil {
		t.Fatal(err)
	}
	state := &composeState{Subject: "From the command line", InitialTemplate: "\n-- \nsig\n"}
	m.handleComposeEditorFinished(composeEditorFinishedMsg{path: tmp, initialState: state})
	if m.compose == nil || m.mode != modeCompose {
		t.Fatal("compose was cancelled although -s supplied a subject")
	}
}
