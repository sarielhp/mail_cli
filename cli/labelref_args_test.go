package cli

import (
	"errors"
	"testing"

	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/label"
	"github.com/sarielhp/mail_cli/mailclient"
)

// Every command that takes a label argument completes it as `%label`, so
// every one must parse it. These used to hand the argument to the backend or
// the config unparsed, so the sigil reached the server as part of the name.

// labelSession is a session whose selected client knows the given folders and
// which records the account each client was built for.
func labelSession(mock *mailclient.MockMailClient, accounts *[]string) *Session {
	cfg := &cfg_g.Config{}
	mock.Cfg = cfg
	return &Session{
		Config: cfg,
		Deps: Deps{
			Clients: stubClients{
				ForSelectedFn: func(c *cfg_g.Config) (mailclient.MailClient, error) {
					if accounts != nil {
						*accounts = append(*accounts, string(c.SelectedAccount))
					}
					return mock, nil
				},
			},
		},
	}
}

func TestSpamLearnHam_AccountRefScopesConfig(t *testing.T) {
	var accounts []string
	session := labelSession(&mailclient.MockMailClient{Labels: []string{"Archive"}}, &accounts)
	session.Config.Accounts = []cfg_acc.AccountConfig{{Name: "default"}, {Name: "Work", DisplayName: "Work Mail"}}
	var gotAccount string
	session.Deps.Spam = stubSpam{LearnHamFn: func(c *cfg_g.Config, _ label.Folder, _ bool) error {
		gotAccount = string(c.SelectedAccount)
		return nil
	}}

	if err := InitCLI(session).Execute([]string{"spam", "learn", "--ham", "%work:archive"}); err != nil {
		t.Fatalf("spam learn --ham returned error: %v", err)
	}
	// Resolved to the canonical name, not left as typed.
	if gotAccount != "Work" {
		t.Errorf("LearnHam ran against account %q, want %q", gotAccount, "Work")
	}
}

func TestSpamLearnHam_UnknownFolderIsAnError(t *testing.T) {
	session := labelSession(&mailclient.MockMailClient{Labels: []string{"Archive"}}, nil)
	called := false
	session.Deps.Spam = stubSpam{LearnHamFn: func(*cfg_g.Config, label.Folder, bool) error {
		called = true
		return nil
	}}

	err := InitCLI(session).Execute([]string{"spam", "learn", "--ham", "%nosuch"})
	var notFound *mailclient.NotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("err = %v, want a NotFoundError", err)
	}
	if called {
		t.Error("LearnHam ran on a folder that does not exist")
	}
}

func TestSpamLearn_DestResolvesExistingFolder(t *testing.T) {
	mock := &mailclient.MockMailClient{Labels: []string{"Archive/Spam"}}
	session := labelSession(mock, nil)

	if err := InitCLI(session).Execute([]string{"spam", "learn", "%archive/spam"}); err != nil {
		t.Fatalf("spam learn returned error: %v", err)
	}
	if mock.LastMoveAllSpamDest != "Archive/Spam" {
		t.Errorf("MoveAllSpam dest = %q, want %q", mock.LastMoveAllSpamDest, "Archive/Spam")
	}
}

func TestSpamUnmark_SigilArgumentIsAFolder(t *testing.T) {
	session := labelSession(&mailclient.MockMailClient{Labels: []string{"Archive"}}, nil)
	var folder string
	byID := false
	session.Deps.Spam = stubSpam{
		UnspamFolderFn: func(_ *cfg_g.Config, f label.Folder) error {
			folder = f.String()
			return nil
		},
		UnspamByIDFn: func(*cfg_g.Config, string) error {
			byID = true
			return nil
		},
	}

	if err := InitCLI(session).Execute([]string{"spam", "unmark", "%archive"}); err != nil {
		t.Fatalf("spam unmark returned error: %v", err)
	}
	if byID {
		t.Error("%archive was treated as a message ID")
	}
	if folder != "Archive" {
		t.Errorf("UnspamFolder folder = %q, want %q", folder, "Archive")
	}
}

func TestRuleLabelArg_ResolvesAndRefusesOtherAccount(t *testing.T) {
	session := labelSession(&mailclient.MockMailClient{Labels: []string{"Dev/GitHub"}}, nil)

	got, err := ruleLabelArg(session, []string{"x", "%dev/github"}, 1)
	if err != nil || got != "Dev/GitHub" {
		t.Errorf("ruleLabelArg = %q, %v; want %q", got, err, "Dev/GitHub")
	}

	// A folder that does not exist yet is taken as written, without the sigil.
	if got, err := ruleLabelArg(session, []string{"x", "%New/Folder"}, 1); err != nil || got != "New/Folder" {
		t.Errorf("ruleLabelArg(new) = %q, %v; want %q", got, err, "New/Folder")
	}

	if _, err := ruleLabelArg(session, []string{"x", "%work:Dev"}, 1); err == nil {
		t.Error("a rule label naming another account was accepted")
	}
}

func TestCalendarAdd_ParsesLabelPrefix(t *testing.T) {
	session := labelSession(&mailclient.MockMailClient{}, nil)
	var prefix string
	session.Deps.Calendar = stubCalendar{AddFn: func(_ *cfg_g.Config, _ mailclient.MailClient, p, _ string) error {
		prefix = p
		return nil
	}}

	if err := InitCLI(session).Execute([]string{"calendar", "add", "%receipts", "1234abcd"}); err != nil {
		t.Fatalf("calendar add returned error: %v", err)
	}
	if prefix != "receipts" {
		t.Errorf("calendar Add prefix = %q, want %q", prefix, "receipts")
	}
}

func TestLabelCreate_RefusesSigilName(t *testing.T) {
	session := labelSession(&mailclient.MockMailClient{}, nil)

	err := InitCLI(session).Execute([]string{"label", "create", "%oops"})
	if !errors.Is(err, label.ErrReservedName) {
		t.Fatalf("err = %v, want ErrReservedName", err)
	}
}

// "%%foo" names a folder literally called "%foo". Reaching an existing one is
// allowed; asking for a new one must not slip past label.ForCreate.
func TestResolveDestFolder_EscapedSigilIsNotCreatable(t *testing.T) {
	session := labelSession(&mailclient.MockMailClient{Labels: []string{"%legacy"}}, nil)

	if got, err := session.ResolveDestFolder("%%legacy"); err != nil || got.Folder.String() != "%legacy" {
		t.Errorf("existing %%legacy: %q, %v", got.Folder, err)
	}
	if _, err := session.ResolveDestFolder("%%fresh"); !errors.Is(err, label.ErrReservedName) {
		t.Errorf("new %%fresh: err = %v, want ErrReservedName", err)
	}
}
