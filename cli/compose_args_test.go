package cli

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/contacts"
)

// touch creates empty files under dir.
func touch(t *testing.T, dir string, names ...string) {
	t.Helper()
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(dir, n), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestExpandAttachments(t *testing.T) {
	dir := t.TempDir()
	touch(t, dir, "a.pdf", "b.pdf", "notes.txt", "report[1].txt", "report1.txt")
	home := t.TempDir()
	t.Setenv("HOME", home)
	touch(t, home, "cv.pdf")
	pdfs := []string{filepath.Join(dir, "a.pdf"), filepath.Join(dir, "b.pdf")}

	for _, tt := range []struct {
		name    string
		in      []string
		want    []string
		errText string
	}{
		{"a quoted pattern is expanded", []string{filepath.Join(dir, "*.pdf")}, pdfs, ""},
		{"a literal path is kept, even if missing", []string{"spec.pdf"}, []string{"spec.pdf"}, ""},
		// A script naming a file with wildcard characters means that file.
		{"an existing file is not a pattern", []string{filepath.Join(dir, "report[1].txt")}, []string{filepath.Join(dir, "report[1].txt")}, ""},
		{"a match named twice is attached once", []string{filepath.Join(dir, "*.pdf"), pdfs[0]}, pdfs, ""},
		{"a quoted ~/ is the home directory", []string{"~/*.pdf"}, []string{filepath.Join(home, "cv.pdf")}, ""},
		{"a pattern that matches nothing is refused", []string{filepath.Join(dir, "*.doc")}, nil, "matches no file"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := expandAttachments(tt.in)
			if tt.errText != "" {
				if err == nil || !strings.Contains(err.Error(), tt.errText) {
					t.Fatalf("err = %v, want %q", err, tt.errText)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("expandAttachments(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// `write -a *.pdf boss@x` unquoted: the shell expands the pattern, -a keeps the
// first match, and the rest arrive as recipients.
func TestCheckRecipientsAreNotFiles(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	touch(t, dir, "b.pdf", "bob")
	book := &contacts.Book{Contacts: []contacts.Contact{
		{Name: "Bob Smith", Emails: []string{"bob@example.com"}, Nicknames: []string{"bob"}},
	}}
	for _, tt := range []struct {
		name       string
		recipients []string
		bad        string
	}{
		{"a file left by an unquoted pattern", []string{"boss@example.com", "b.pdf"}, "b.pdf"},
		{"inside a comma list", []string{"boss@example.com,b.pdf"}, "b.pdf"},
		{"a contact named like a file is a contact", []string{"bob"}, ""},
		{"an address", []string{"b.pdf@example.com"}, ""},
		{"a name that is no file", []string{"carol"}, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := checkRecipientsAreNotFiles(book, tt.recipients)
			if tt.bad == "" {
				if err != nil {
					t.Errorf("refused %q: %v", tt.recipients, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.bad) || !strings.Contains(err.Error(), `-a "*.pdf"`) {
				t.Errorf("err = %v, want one naming %q and how to quote", err, tt.bad)
			}
		})
	}
}

// recipientSession is a session whose contacts book holds two contacts and
// whose send history is empty.
func recipientSession(t *testing.T) *Session {
	t.Helper()
	cfgDir, cacheDir := t.TempDir(), t.TempDir()
	config := &cfg_g.Config{Paths: cfg_g.Paths{ConfigDir: cfgDir, CacheRoot: cacheDir}}
	book := &contacts.Book{Contacts: []contacts.Contact{
		{ID: "1", Name: "Bob Smith", Emails: []string{"bob@example.com"}, Nicknames: []string{"bobby"}},
		{ID: "2", Name: "Alice Jones", Emails: []string{"alice@example.org"}},
	}}
	if err := contacts.SaveBook(contacts.DefaultPath(config), book); err != nil {
		t.Fatal(err)
	}
	return &Session{Config: config}
}

func TestCompleteRecipients(t *testing.T) {
	complete := completeRecipients(recipientSession(t))
	for _, tt := range []struct {
		in, want string
	}{
		{"bob", "bobby\tBob Smith <bob@example.com>"},
		{"bob@", "bob@example.com\tBob Smith <bob@example.com>"},
		{"ali", "alice@example.org\tAlice Jones <alice@example.org>"},
		{"carol@x.org,bob", "carol@x.org,bobby\tBob Smith <bob@example.com>"},
		// Nothing starts with "smith", so the match inside the name is offered.
		{"smith", "bob@example.com\tBob Smith <bob@example.com>"},
	} {
		got := complete(tt.in)
		if len(got) == 0 || got[0] != tt.want {
			t.Errorf("completeRecipients(%q) = %q, want first %q", tt.in, got, tt.want)
		}
	}

	// "a" is inside "Bob Smith" too, but alice starts with it: matches inside a
	// word are not offered beside ones a prefix-matching shell can use.
	if got := complete("a"); len(got) != 1 || !strings.HasPrefix(got[0], "alice@") {
		t.Errorf("completeRecipients(\"a\") = %q, want only alice", got)
	}
}

func TestCompleteAttachments(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	touch(t, dir, "a.pdf", "b.pdf", ".hidden")
	if err := os.Mkdir(filepath.Join(dir, "docs"), 0o700); err != nil {
		t.Fatal(err)
	}
	touch(t, filepath.Join(dir, "docs"), "c.pdf")

	for _, tt := range []struct {
		in   string
		want []string
	}{
		// Without a comma the shell's own file completion answers.
		{"a.p", nil},
		{"a.pdf,b", []string{"a.pdf,b.pdf"}},
		{"a.pdf,d", []string{"a.pdf,docs/"}},
		{"a.pdf,docs/", []string{"a.pdf,docs/c.pdf"}},
		{"a.pdf,.h", []string{"a.pdf,.hidden"}},
	} {
		if got := completeAttachments(tt.in); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("completeAttachments(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// The whole path: a quoted pattern given to write reaches the composer as the
// files it matches.
func TestRunComposeExpandsAttachmentPatterns(t *testing.T) {
	dir := t.TempDir()
	touch(t, dir, "a.pdf", "b.pdf")
	var got ComposeOptions
	session := &Session{Config: &cfg_g.Config{Paths: cfg_g.Paths{ConfigDir: t.TempDir()}}, Deps: Deps{TUI: stubTUI{
		ComposeFn: func(_ *cfg_g.Config, opts ComposeOptions) error { got = opts; return nil },
	}}}
	flagComposeAttach = []string{filepath.Join(dir, "*.pdf")}
	defer func() { flagComposeAttach = nil }()

	if err := runCompose(session, []string{"boss@example.com"}); err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Join(dir, "a.pdf"), filepath.Join(dir, "b.pdf")}
	if !reflect.DeepEqual(got.Attachments, want) {
		t.Errorf("Attachments = %q, want %q", got.Attachments, want)
	}
}

// Shell completion runs before BeforeRun, so the session holds the empty
// bootstrap Config. The completer must load the real one rather than read the
// contacts and history from the default locations.
func TestCompleteRecipientsLoadsConfigBeforeBootstrap(t *testing.T) {
	loaded := recipientSession(t).Config
	loaded.Policy.Blacklist = []string{"alice@example.org"}
	orig := loadCompletionConfig
	loadCompletionConfig = func() (*cfg_g.Config, error) { return loaded, nil }
	t.Cleanup(func() { loadCompletionConfig = orig })

	complete := completeRecipients(&Session{Config: &cfg_g.Config{}})
	if got := complete("bob"); len(got) == 0 || got[0] != "bobby\tBob Smith <bob@example.com>" {
		t.Errorf("completion should read the configured contacts, got %q", got)
	}
	if got := complete("ali"); len(got) != 0 {
		t.Errorf("the configured blacklist should apply, got %q", got)
	}
}
