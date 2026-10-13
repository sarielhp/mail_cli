package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/sarielhp/mail_cli/app"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/draft"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/mailclient"
	"github.com/sarielhp/mail_cli/organize"
)

func draftsSession(t *testing.T, org stubOrganize) (*Session, *bytes.Buffer) {
	t.Helper()
	out := &bytes.Buffer{}
	s := &Session{
		Config: &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: t.TempDir()}},
		Out:    out,
		Deps: Deps{
			Clients:  stubClients{ForSelectedFn: func(*cfg_g.Config) (mailclient.MailClient, error) { return &mailclient.MockMailClient{}, nil }},
			Organize: org,
		},
	}
	return s, out
}

func TestDraftsListAndRemove(t *testing.T) {
	s, out := draftsSession(t, stubOrganize{})
	store := draftStore(s)
	p, err := store.Create(time.Date(2026, 9, 30, 9, 0, 0, 0, time.Local), []byte("To: a@example.com\r\nSubject: Hello\r\n\r\nbody\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	if err := runDraftsList(s); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "2026/09/30/1") || !strings.Contains(out.String(), "Hello") {
		t.Fatalf("listing:\n%s", out)
	}
	if err := runDraftsRemove(s, []string{"2026/09/30/1"}); err != nil {
		t.Fatal(err)
	}
	if entries, _ := store.List(); len(entries) != 0 {
		t.Errorf("draft %s still listed", p)
	}
	if err := runDraftsRemove(s, []string{"../../etc"}); err == nil {
		t.Error("an id outside the store was accepted")
	}
}

// --resume used to silently ignore recipients and every other flag.
func TestComposeResumeRefusesOtherContent(t *testing.T) {
	s, _ := draftsSession(t, stubOrganize{})
	called := false
	s.Deps.TUI = stubTUI{ComposeFn: func(*cfg_g.Config, ComposeOptions) error { called = true; return nil }}
	flagComposeResume, flagComposeSubject = "2026/09/30/1", "new"
	defer func() { flagComposeResume, flagComposeSubject = "", "" }()

	err := runCompose(s, nil)
	if !errors.Is(err, app.ErrUsage) || called {
		t.Fatalf("err = %v, compose called = %v", err, called)
	}
	flagComposeSubject = ""
	if err := runCompose(s, []string{"bob@example.com"}); !errors.Is(err, app.ErrUsage) {
		t.Fatalf("recipients with --resume: err = %v", err)
	}
}

func TestComposeResumeOpensTheDraft(t *testing.T) {
	s, _ := draftsSession(t, stubOrganize{})
	p, _ := draftStore(s).Create(time.Date(2026, 9, 30, 9, 0, 0, 0, time.Local), []byte("x"))
	var got ComposeOptions
	s.Deps.TUI = stubTUI{ComposeFn: func(_ *cfg_g.Config, o ComposeOptions) error { got = o; return nil }}
	flagComposeResume = "2026/09/30/1"
	defer func() { flagComposeResume = "" }()
	if err := runCompose(s, nil); err != nil || got.DraftPath != p {
		t.Fatalf("err %v, DraftPath %q want %q", err, got.DraftPath, p)
	}
}

func TestServerDraftsRemoveRefusesUnknownIDs(t *testing.T) {
	trashed := 0
	s, _ := draftsSession(t, stubOrganize{
		FindServerDraftsFn: func(mailclient.MailClient, []string) (string, []organize.ServerDraft, []string, error) {
			return "Drafts", []organize.ServerDraft{{ID: "a"}}, []string{"nope"}, nil
		},
		TrashServerDraftsFn: func(mailclient.MailClient, string, []email.ID) (string, error) { trashed++; return "Trash", nil },
	})
	err := runServerDraftsRemove(s, []string{"a", "nope"})
	if !errors.Is(err, app.ErrUsage) || trashed != 0 {
		t.Fatalf("err %v, trashed %d; an unknown id must stop the whole command", err, trashed)
	}
}

func TestServerDraftsPrune(t *testing.T) {
	plan := organize.PrunePlan{
		Duplicates: []organize.ServerDraft{{ID: "d1", ShortID: "D1", Subject: "Re: x"}},
		Sent:       []organize.ServerDraft{{ID: "d1", ShortID: "D1"}, {ID: "s1", ShortID: "S1"}},
	}
	cases := []struct {
		name      string
		dryRun    bool
		readOnly  bool
		plan      organize.PrunePlan
		wantOut   []string
		wantTrash []string
	}{
		{name: "nothing to prune", wantOut: []string{"Nothing to prune in Drafts (4 drafts)."}},
		{name: "acts", plan: plan, wantOut: []string{"Trashing from Drafts:", "older copy", "already sent", "Moved 2 draft(s)"}, wantTrash: []string{"d1", "s1"}},
		{name: "dry run", dryRun: true, readOnly: true, plan: plan, wantOut: []string{"Would trash from Drafts:", "[dry-run] would move 2"}, wantTrash: []string{"d1", "s1"}},
		{name: "read-only", readOnly: true, plan: plan, wantOut: []string{"Would trash", "[read-only] would move 2"}, wantTrash: []string{"d1", "s1"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotOpts organize.PruneOptions
			var gotTrash []string
			s, out := draftsSession(t, stubOrganize{
				PlanServerPruneFn: func(_ mailclient.MailClient, o organize.PruneOptions) (string, int, organize.PrunePlan, error) {
					gotOpts = o
					return "Drafts", 4, tc.plan, nil
				},
				TrashServerDraftsFn: func(_ mailclient.MailClient, _ string, ids []email.ID) (string, error) {
					gotTrash = email.IDStrings(ids)
					return "Trash", nil
				},
			})
			s.Config.DryRun, s.Config.ReadOnly = tc.dryRun, tc.readOnly
			if err := runServerDraftsPrune(s, organize.PruneOptions{Sent: true}); err != nil {
				t.Fatal(err)
			}
			if !gotOpts.Sent || gotOpts.BySubject {
				t.Errorf("options passed = %+v", gotOpts)
			}
			for _, w := range tc.wantOut {
				if !strings.Contains(out.String(), w) {
					t.Errorf("output lacks %q:\n%s", w, out)
				}
			}
			if strings.Join(gotTrash, ",") != strings.Join(tc.wantTrash, ",") {
				t.Errorf("trashed %v, want %v", gotTrash, tc.wantTrash)
			}
		})
	}
}

// draftStore must follow the session's download directory.
func TestDraftStoreFollowsCacheRoot(t *testing.T) {
	s, _ := draftsSession(t, stubOrganize{})
	if got := draftStore(s).Root; got != draft.Root(s.Config.CacheRoot) {
		t.Errorf("root = %q", got)
	}
}

func TestServerDraftsRemoveAll(t *testing.T) {
	all := []organize.ServerDraft{
		{ID: "a", ShortID: "AAAA0001", Subject: "One"},
		{ID: "b", ShortID: "BBBB0002", Subject: "(unreadable)", Unreadable: true},
	}
	var trashed []string
	s, out := draftsSession(t, stubOrganize{
		ListServerDraftsFn: func(mailclient.MailClient) (string, []organize.ServerDraft, error) { return "Drafts", all, nil },
		TrashServerDraftsFn: func(_ mailclient.MailClient, folder string, ids []email.ID) (string, error) {
			if folder != "Drafts" {
				t.Errorf("folder = %q", folder)
			}
			trashed = email.IDStrings(ids)
			return "Trash", nil
		},
	})
	if err := runServerDraftsRemove(s, []string{"ALL"}); err != nil {
		t.Fatal(err)
	}
	if strings.Join(trashed, ",") != "a,b" {
		t.Errorf("trashed %v, want every draft including the unreadable one", trashed)
	}
	for _, w := range []string{"Trashing all 2 drafts from Drafts:", "AAAA0001", "BBBB0002", "Moved 2 draft(s) from Drafts to Trash"} {
		if !strings.Contains(out.String(), w) {
			t.Errorf("output lacks %q:\n%s", w, out)
		}
	}
}

func TestServerDraftsRemoveAllOnEmptyFolderTrashesNothing(t *testing.T) {
	called := false
	s, out := draftsSession(t, stubOrganize{
		ListServerDraftsFn:  func(mailclient.MailClient) (string, []organize.ServerDraft, error) { return "Drafts", nil, nil },
		TrashServerDraftsFn: func(mailclient.MailClient, string, []email.ID) (string, error) { called = true; return "Trash", nil },
	})
	if err := runServerDraftsRemove(s, []string{"all"}); err != nil || called || !strings.Contains(out.String(), "No drafts in Drafts.") {
		t.Fatalf("err %v called %v out %q", err, called, out)
	}
}

func TestServerDraftsRemoveAllRefusesExtraIDs(t *testing.T) {
	s, _ := draftsSession(t, stubOrganize{})
	if err := runServerDraftsRemove(s, []string{"all", "AAAA0001"}); !errors.Is(err, app.ErrUsage) {
		t.Fatalf("err = %v", err)
	}
}
