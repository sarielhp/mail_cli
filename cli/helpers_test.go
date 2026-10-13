package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarielhp/mail_cli/cache/cachedir"
	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/mailclient"
	"github.com/sarielhp/mail_cli/scan"
)

// Shared test builders, so a command test states only what it exercises and
// the shape of a Session or a MockMailClient is spelled once.

// sessionRecorder records how a command reached the pre-flight check and scan
// dependencies, so a test can assert it reached neither.
type sessionRecorder struct {
	opts       scan.Options
	scanned    bool
	prechecked bool
}

func (r *sessionRecorder) precheck(_ *cfg_g.Config) error {
	r.prechecked = true
	return nil
}

func (r *sessionRecorder) scan(o scan.Options, _ *cfg_g.Config, _ string) (int, error) {
	r.opts = o
	r.scanned = true
	return 0, nil
}

// recordingSession builds a Session whose pre-flight check and scan record into
// rec instead of touching an account.
func recordingSession(rec *sessionRecorder) *Session {
	return &Session{
		Config: &cfg_g.Config{},
		Deps: Deps{
			Clients: stubClients{PreCheckFn: rec.precheck},
			Mail:    stubMail{ScanFn: rec.scan},
		},
	}
}

// archiveMock builds a MockMailClient that serves ids as the whole contents of
// one label, caching under accDir.
func archiveMock(accDir, configDir, labelName string, ids ...email.ID) *mailclient.MockMailClient {
	return &mailclient.MockMailClient{
		Labels:        []string{labelName},
		DownloadedIDs: map[string][]email.ID{labelName: ids},
		Cfg:           &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: accDir, ConfigDir: configDir}, AccountCache: cachedir.At(accDir)},
	}
}

// singleAccountSession builds a one-account Session whose client provider
// always returns client.
func singleAccountSession(cacheRoot, configDir string, client mailclient.MailClient) *Session {
	return &Session{
		Config: &cfg_g.Config{Accounts: []cfg_acc.AccountConfig{
			{Name: "acc1", Type: "jmap"},
		}, Paths: cfg_g.Paths{CacheRoot: cacheRoot, ConfigDir: configDir}},
		Deps: Deps{
			Clients: stubClients{
				ForSelectedFn: func(_ *cfg_g.Config) (mailclient.MailClient, error) {
					return client, nil
				},
			},
		},
	}
}

// twoAccountSession builds an acc1/acc2 Session whose client provider returns
// client2 for account acc2 and client1 for any other account.
func twoAccountSession(cacheRoot, configDir string, client1, client2 mailclient.MailClient) *Session {
	return &Session{
		Config: &cfg_g.Config{Accounts: []cfg_acc.AccountConfig{
			{Name: "acc1", Type: "gmail"},
			{Name: "acc2", Type: "jmap"},
		}, Paths: cfg_g.Paths{CacheRoot: cacheRoot, ConfigDir: configDir}},
		Deps: Deps{
			Clients: stubClients{
				ForSelectedFn: func(cfg *cfg_g.Config) (mailclient.MailClient, error) {
					if strings.EqualFold(string(cfg.SelectedAccount), "acc2") {
						return client2, nil
					}
					return client1, nil
				},
			},
		},
	}
}

// dryRunSession builds a Session in dry-run mode for account, with no
// dependency wired yet.
func dryRunSession(account cfg_g.AccountName) *Session {
	return &Session{Config: &cfg_g.Config{SelectedAccount: account, DryRun: true}}
}

// assertInlineBody checks that resolve takes its body from the inline body flag
// rather than the include-file flag.
func assertInlineBody(t *testing.T, body, include *string, resolve func() (string, error), want string) {
	t.Helper()
	*body = want
	*include = ""
	defer func() { *body = "" }()

	got, err := resolve()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// assertBodyFromFile checks that resolve takes its body from the file named by
// the include flag.
func assertBodyFromFile(t *testing.T, body, include *string, resolve func() (string, error), content string) {
	t.Helper()
	*body = ""
	tmpFile := filepath.Join(t.TempDir(), "body.txt")
	if err := os.WriteFile(tmpFile, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write tmp file: %v", err)
	}
	*include = tmpFile
	defer func() { *include = "" }()

	got, err := resolve()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != content {
		t.Errorf("got %q, want %q", got, content)
	}
}
