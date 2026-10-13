package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/sarielhp/mail_cli/cache/folderindex"
	"github.com/sarielhp/mail_cli/cache/msg"
	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/label"
	"github.com/sarielhp/mail_cli/mailclient"
)

func TestIsUnifiedInbox(t *testing.T) {
	tests := []struct {
		folder string
		want   bool
	}{
		{"All Inboxes", true},
		{"all inboxes", true},
		{"%all:inbox", true},
		{"%all", true},
		{"INBOX", false},
		{"Archive", false},
		{"", false},
	}

	for _, tc := range tests {
		if got := isUnifiedInbox(tc.folder); got != tc.want {
			t.Errorf("isUnifiedInbox(%q) = %v, want %v", tc.folder, got, tc.want)
		}
	}
}

func TestUnifiedFoldersInjectedWhenMultipleAccounts(t *testing.T) {
	m := &tuiModel{
		cfg: &cfg_g.Config{
			Accounts: []cfg_acc.AccountConfig{
				{Name: "Personal", Type: "gmail"},
				{Name: "Work", Type: "jmap"},
			},
		},
		currentF: "INBOX",
	}

	rawFolders := []label.Item{
		{Name: "INBOX", FullName: "INBOX", IsSystem: true},
		{Name: "Sent", FullName: "Sent", IsSystem: true},
	}

	mUpdated, _ := m.handleFoldersLoaded(foldersLoadedMsg{folders: rawFolders})
	tm := mUpdated.(*tuiModel)

	foundUnified := false
	for _, f := range tm.folders {
		if f.FullName == UnifiedInboxFolder {
			foundUnified = true
			break
		}
	}
	if !foundUnified {
		t.Errorf("expected %q folder in model folders when multiple accounts exist", UnifiedInboxFolder)
	}
}

func TestLoadUnifiedCachedEmails(t *testing.T) {
	tmpDir := t.TempDir()

	acc1 := cfg_acc.AccountConfig{Name: "Personal", Type: "gmail"}
	acc2 := cfg_acc.AccountConfig{Name: "Work", Type: "jmap"}

	cfg := &cfg_g.Config{
		Paths: cfg_g.Paths{
			ConfigDir: tmpDir,
			CacheRoot: tmpDir,
		},
		Accounts: []cfg_acc.AccountConfig{acc1, acc2},
	}

	dir1 := cfg.AccountCacheFor(acc1)
	dir2 := cfg.AccountCacheFor(acc2)

	now := time.Now()
	// Seed cache for acc1
	_ = folderindex.ReplaceAll(dir1, "INBOX", []email.ID{"p1"})
	_ = msg.Store(dir1, "p1", []byte("Date: Sun, 11 Oct 2026 10:00:00 +0000\r\nSubject: Personal Note\r\nFrom: friend@example.com\r\n\r\nHi!"), now)

	// Seed cache for acc2
	_ = folderindex.ReplaceAll(dir2, "INBOX", []email.ID{"w1"})
	_ = msg.Store(dir2, "w1", []byte("Date: Sun, 11 Oct 2026 12:00:00 +0000\r\nSubject: Work Memo\r\nFrom: boss@example.com\r\n\r\nMeeting!"), now)

	m := &tuiModel{
		cfg:      cfg,
		currentF: UnifiedInboxFolder,
		isRead:   make(map[email.ID]bool),
	}

	m.loadUnifiedCachedEmails()

	if len(m.rawEmails) != 2 {
		t.Fatalf("expected 2 emails loaded, got %d", len(m.rawEmails))
	}

	// Should be sorted newest first (Work Memo at 12:00 then Personal Note at 10:00)
	if m.rawEmails[0].ID != "w1" || m.rawEmails[0].Account != "Work" {
		t.Errorf("expected w1 (Work) first, got: %+v", m.rawEmails[0])
	}
	if m.rawEmails[1].ID != "p1" || m.rawEmails[1].Account != "Personal" {
		t.Errorf("expected p1 (Personal) second, got: %+v", m.rawEmails[1])
	}
}

// unifiedFixture is a unified inbox over two accounts: Personal is active,
// Work is reached through the client provider, and missing is an account the
// provider cannot build.
func unifiedFixture() (*tuiModel, *mailclient.MockMailClient, *mailclient.MockMailClient) {
	personal := &mailclient.MockMailClient{Cfg: &cfg_g.Config{SelectedAccount: "Personal"}, InboxFolderVal: "INBOX"}
	work := &mailclient.MockMailClient{Cfg: &cfg_g.Config{SelectedAccount: "Work"}, InboxFolderVal: "Inbox"}
	m := &tuiModel{
		client:        personal,
		currentF:      UnifiedInboxFolder,
		isRead:        map[email.ID]bool{},
		pendingEmails: map[email.ID]pendingOpType{},
		cfg: &cfg_g.Config{Accounts: []cfg_acc.AccountConfig{
			{Name: "Personal", Type: "gmail"},
			{Name: "Work", Type: "jmap"},
			{Name: "Broken", Type: "jmap"},
		}},
		bk: &Backend{
			Clients: stubClients{ForAccountFn: func(_ *cfg_g.Config, acc cfg_acc.AccountConfig) (mailclient.MailClient, error) {
				if acc.Name == "Work" {
					return work, nil
				}
				return nil, errors.New("no credentials")
			}},
			Folders: stubFolders{ArchiveFn: func(mailclient.MailClient) (string, error) { return "Archive", nil }},
		},
	}
	m.rawEmails = []email.Email{
		{ID: "w1", Account: "Work", Subject: "Job"},
		{ID: "p1", Account: "Personal", Subject: "Chat"},
		{ID: "b1", Account: "Broken", Subject: "Lost"},
	}
	m.rebuildVisibleEmails()
	return m, personal, work
}

func TestPlanTargetsOwnAccount(t *testing.T) {
	m, personal, work := unifiedFixture()
	plan := m.planAction([]email.ID{"p1", "w1", "b1"}, UnifiedInboxFolder)
	// The rows go before the command runs; the plan must not need them.
	m.rawEmails, m.emails = nil, nil

	cl, f, err := m.target(plan, "w1")
	if err != nil || cl != mailclient.MailClient(work) || f != "Inbox" {
		t.Errorf("w1: got (%v, %q, %v), want Work's client and its Inbox", cl, f, err)
	}
	cl, f, err = m.target(plan, "p1")
	if err != nil || cl != mailclient.MailClient(personal) || f != "INBOX" {
		t.Errorf("p1: got (%v, %q, %v), want Personal's client and INBOX", cl, f, err)
	}
	if cl, _, err = m.target(plan, "b1"); err == nil {
		t.Errorf("b1: an account whose client cannot be built must fail, got %v", cl)
	}

	other := m.planAction([]email.ID{"w1"}, "Archive")
	cl, f, err = m.target(other, "w1")
	if err != nil || cl != mailclient.MailClient(personal) || f != "Archive" {
		t.Errorf("outside the unified inbox the active client and folder act, got (%v, %q, %v)", cl, f, err)
	}
}

func TestBulkArchiveUnifiedRoutesEachAccount(t *testing.T) {
	m, personal, work := unifiedFixture()
	m.taggedIDs = map[email.ID]bool{"w1": true, "p1": true, "b1": true}

	_, cmd := m.bulkArchiveTagged()
	if cmd == nil {
		t.Fatal("expected a command")
	}
	done, ok := cmd().(bulkArchiveDoneMsg)
	if !ok {
		t.Fatalf("expected bulkArchiveDoneMsg")
	}
	if len(work.MovedEmails) != 1 || work.MovedEmails[0].SourceLabel != "Inbox" || work.MovedEmails[0].MessageIDs[0] != "w1" {
		t.Errorf("Work should move w1 out of its own Inbox, got %+v", work.MovedEmails)
	}
	if len(personal.MovedEmails) != 1 || personal.MovedEmails[0].SourceLabel != "INBOX" || personal.MovedEmails[0].MessageIDs[0] != "p1" {
		t.Errorf("Personal should move only p1 out of INBOX, got %+v", personal.MovedEmails)
	}
	if len(done.ids) != 2 {
		t.Errorf("only the two accepted ids are done, got %v", done.ids)
	}
	if done.err == nil || !strings.Contains(done.err.Error(), "Broken") {
		t.Errorf("the unresolvable account must be reported, got %v", done.err)
	}

	mMod, _ := m.Update(done)
	if tm := mMod.(*tuiModel); !tm.showError || tm.saveStatus != "Archived 2 message(s)" {
		t.Errorf("status %q showError %v: want the accepted count and the error", tm.saveStatus, tm.showError)
	}
}

func TestBulkMoveReportsServerFailure(t *testing.T) {
	m, _, work := unifiedFixture()
	work.FailOn = map[string]error{"MoveEmail": errors.New("server said no")}
	m.taggedIDs = map[email.ID]bool{"w1": true}

	_, cmd := m.bulkArchiveTagged()
	done := cmd().(bulkArchiveDoneMsg)
	if len(done.ids) != 0 || done.err == nil {
		t.Errorf("a refused move must not count as done: ids %v err %v", done.ids, done.err)
	}
}

func TestBulkMarkUnreadSaysItIsLocal(t *testing.T) {
	m, _, _ := unifiedFixture()
	m.taggedIDs = map[email.ID]bool{"w1": true}
	_, cmd := m.bulkMarkReadTagged(false)
	mMod, _ := m.Update(cmd())
	if got := mMod.(*tuiModel).saveStatus; !strings.Contains(got, "locally") {
		t.Errorf("marking unread changes no server and must say so, got %q", got)
	}
}

// The folder pane shows the global tree, so the unified inbox has to be in it:
// offered only in m.folders, which nothing draws, it could not be opened.
func TestUnifiedInboxInFolderPane(t *testing.T) {
	m := &tuiModel{
		cfg: &cfg_g.Config{Accounts: []cfg_acc.AccountConfig{
			{Name: "Personal", Type: "gmail"},
			{Name: "Work", Type: "jmap"},
		}},
		currentF:       "INBOX",
		showGlobalTree: true,
	}
	res, _ := m.handleFoldersLoaded(foldersLoadedMsg{folders: []label.Item{
		{Name: "INBOX", FullName: "INBOX", IsSystem: true},
		{Name: "Archive", FullName: "Archive"},
	}})
	tm := res.(*tuiModel)
	tm.treeOpen = true
	tm.buildEntries()
	idx := -1
	for i, e := range tm.entries {
		if e.Node != nil && e.Node.FullName == UnifiedInboxFolder {
			idx = i
		}
	}
	if idx < 0 {
		t.Fatalf("folder pane lacks %q", UnifiedInboxFolder)
	}

	// Nothing can be filed into it: the move picker ignores it.
	tm.cursor = idx
	tm.bulkMovingToFolder = true
	tm.taggedIDs = map[email.ID]bool{"x": true}
	res, cmd := tm.selectFolderEntry()
	tm = res.(*tuiModel)
	if cmd != nil || !tm.bulkMovingToFolder || !tm.treeOpen {
		t.Errorf("bulk move accepted %q as a destination", UnifiedInboxFolder)
	}

	// Selecting it opens it.
	tm.bulkMovingToFolder = false
	res, _ = tm.selectFolderEntry()
	if got := res.(*tuiModel).currentF; got != UnifiedInboxFolder {
		t.Errorf("selecting %q opened %q", UnifiedInboxFolder, got)
	}
}

// With one account there is nothing to unify.
func TestUnifiedInboxNotOfferedForOneAccount(t *testing.T) {
	m := &tuiModel{cfg: &cfg_g.Config{Accounts: []cfg_acc.AccountConfig{{Name: "Personal"}}}}
	if items := m.unifiedInboxItems(); items != nil {
		t.Errorf("one account offered %v", items)
	}
}
