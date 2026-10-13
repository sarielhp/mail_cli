package cli

import (
	"bytes"
	"errors"
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarielhp/mail_cli/app"
	"github.com/sarielhp/mail_cli/cache"
	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/label"
	"github.com/sarielhp/mail_cli/mailclient"
	"github.com/sarielhp/mail_cli/scan"

	"github.com/sarielhp/clihelp"
)

func TestCmdExecutionStartedFlag(t *testing.T) {
	// Reset the flag
	app.CmdExecutionStarted = false

	session := &Session{
		Config: &cfg_g.Config{},
		Deps: Deps{
			Clients: stubClients{
				PreCheckFn: func(_ *cfg_g.Config) error {
					return errors.New("pre-check failed")
				},
			},
		},
	}

	cliApp := InitCLI(session)
	err := cliApp.Execute([]string{"scan", "%inbox"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !app.CmdExecutionStarted {
		t.Error("expected app.CmdExecutionStarted to be true after executing command")
	}

	// Reset again and test validation failure
	app.CmdExecutionStarted = false
	err = cliApp.Execute([]string{"scan", "%inbox", "extra_arg"})
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}

	if app.CmdExecutionStarted {
		t.Error("expected app.CmdExecutionStarted to be false when command validation fails")
	}
}

// `scan inbox` names the folder explicitly, which is what licenses clearing
// cached classifications; reaching the inbox by default does not. This used to
// be recorded in a package-level flag, so the test had to read a global.
func TestScanInboxExplicitFlag(t *testing.T) {
	var gotExplicit []bool

	session := &Session{
		Config: &cfg_g.Config{},
		Deps: Deps{
			Mail: stubMail{
				ScanFn: func(o scan.Options, _ *cfg_g.Config, _ string) (int, error) {
					gotExplicit = append(gotExplicit, o.ExplicitInbox)
					return 0, nil
				},
			},
		},
	}

	cliApp := InitCLI(session)
	if err := cliApp.Execute([]string{"scan", "%inbox"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(gotExplicit) != 1 || !gotExplicit[0] {
		t.Errorf("expected ExplicitInbox=true for `scan %%inbox`, got %v", gotExplicit)
	}

	if err := cliApp.Execute([]string{"scan", "%received"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(gotExplicit) != 2 || gotExplicit[1] {
		t.Errorf("expected ExplicitInbox=false for another folder, got %v", gotExplicit)
	}
}

func TestUnspamFolderCmd(t *testing.T) {
	unspamFolderCalled := false
	var targetFolder string

	session := &Session{
		Config: &cfg_g.Config{},
		Deps: Deps{
			Clients: stubClients{
				ForSelectedFn: func(c *cfg_g.Config) (mailclient.MailClient, error) {
					return &mailclient.MockMailClient{Cfg: c, Labels: []string{"INBOX", "Spam"}}, nil
				},
			},
			Spam: stubSpam{
				UnspamFolderFn: func(_ *cfg_g.Config, folder label.Folder) error {
					unspamFolderCalled = true
					targetFolder = folder.String()
					return nil
				},
			},
		},
	}

	cliApp := InitCLI(session)
	err := cliApp.Execute([]string{"spam", "unmark", "folder", "%spam"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !unspamFolderCalled {
		t.Error("expected session.RunUnspamFolder to be called")
	}
	if targetFolder != "Spam" {
		t.Errorf("expected targetFolder to be Spam, got %s", targetFolder)
	}
}

func TestLearningResetCmd(t *testing.T) {
	learningResetCalled := false

	session := &Session{
		Config: &cfg_g.Config{},
		Deps: Deps{
			Spam: stubSpam{
				ResetLearningFn: func(_ *cfg_g.Config) error {
					learningResetCalled = true
					return nil
				},
			},
		},
	}

	cliApp := InitCLI(session)
	err := cliApp.Execute([]string{"spam", "reset"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !learningResetCalled {
		t.Error("expected session.RunLearningReset to be called")
	}
}

func TestScanPatternFlag(t *testing.T) {
	flagScanPattern = ""

	session := &Session{
		Config: &cfg_g.Config{},
		Deps: Deps{
			Mail: stubMail{
				ScanFn: func(_ scan.Options, _ *cfg_g.Config, _ string) (int, error) {
					return 0, nil
				},
			},
		},
	}

	cliApp := InitCLI(session)
	err := cliApp.Execute([]string{"scan", "%received", "-p", "Urgent"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if flagScanPattern != "Urgent" {
		t.Errorf("expected flagScanPattern to be Urgent, got %q", flagScanPattern)
	}

	// A global flag placed before the command must reach the same variable.
	// Command resolution used to stop at the first token beginning with "-",
	// so anything before the command was lost and the command itself was
	// reported as unknown; clihelp v0.3.9 skips leading flags instead.
	flagScanPattern = ""
	cliApp = InitCLI(session)
	err = cliApp.Execute([]string{"-p", "Urgent", "scan", "%received"})
	if err != nil {
		t.Fatalf("unexpected error with the flag before the command: %v", err)
	}

	if flagScanPattern != "Urgent" {
		t.Errorf("expected flagScanPattern to be Urgent, got %q", flagScanPattern)
	}
}

// Global flags before a subcommand must not prevent the command, or its own
// flags, from being recognized.
func TestGlobalFlagsBeforeSubcommand(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "bool global then command", args: []string{"--read-only", "scan", "%received"}},
		{name: "valued global then command", args: []string{"-A", "acc1", "scan", "%received"}},
		{name: "inline valued global", args: []string{"--account=acc1", "scan", "%received"}},
		{name: "several globals", args: []string{"--read-only", "-v", "scan", "%received"}},
		{name: "global after command still works", args: []string{"scan", "%received", "--read-only"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			scanned := false
			session := &Session{
				Config: &cfg_g.Config{Accounts: []cfg_acc.AccountConfig{{Name: "acc1"}}},
				Deps: Deps{
					Mail: stubMail{
						ScanFn: func(scan.Options, *cfg_g.Config, string) (int, error) {
							scanned = true
							return 0, nil
						},
					},
				},
			}
			if err := InitCLI(session).Execute(tc.args); err != nil {
				t.Fatalf("Execute(%v): %v", tc.args, err)
			}
			if !scanned {
				t.Errorf("Execute(%v) did not reach the scan command", tc.args)
			}
		})
	}
}

// A command's own flag cannot precede its command, and an unrecognized leading
// flag is still an error rather than being silently skipped.
func TestLeadingFlagsThatMustStillFail(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "command flag before its command", args: []string{"-n", "2", "search", "invoice"}},
		{name: "unknown leading flag", args: []string{"--no-such-flag", "scan", "%received"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			session := &Session{
				Config: &cfg_g.Config{},
				Deps: Deps{
					Mail: stubMail{
						ScanFn: func(scan.Options, *cfg_g.Config, string) (int, error) { return 0, nil },
					},
				},
			}
			if err := InitCLI(session).Execute(tc.args); err == nil {
				t.Errorf("Execute(%v) succeeded, want an error", tc.args)
			}
		})
	}
}

func TestTuiPerfectSpamMatch(t *testing.T) {
	tmpDir := t.TempDir()
	config := &cfg_g.Config{Accounts: []cfg_acc.AccountConfig{
		{Name: "test-account", Type: "gmail", Username: "user@example.com", Password: "app-password", ReceivedFolder: "INBOX", SpamFolder: "Spam"},
	}, SelectedAccount: "test-account", Paths: cfg_g.Paths{ConfigDir: tmpDir, CacheRoot: tmpDir}}

	configJSON := `{
		"accounts": [
			{"name": "test-account", "type": "gmail", "username": "user@example.com", "password": "app-password", "received_folder": "INBOX", "spam_folder": "Spam"}
		]
	}`
	err := os.WriteFile(filepath.Join(tmpDir, "config.json"), []byte(configJSON), 0600)
	if err != nil {
		t.Fatalf("failed to write config.json: %v", err)
	}

	cacheDir := filepath.Join(tmpDir, "test-account")
	err = os.MkdirAll(cacheDir, 0700)
	if err != nil {
		t.Fatalf("failed to create cache dir: %v", err)
	}

	cs := &cache.DiskCacheStore{Dir: cachedir.At(cacheDir)}
	// Setup folders, one of which is a perfect case-insensitive match for "spam"
	items := []cfg_acc.Item{
		{Name: "Spam", FullName: "Spam"},
		{Name: "MySpam", FullName: "MySpam"},
		{Name: "ArchiveSpam", FullName: "ArchiveSpam"},
	}
	err = cs.SaveLabelItems(items)
	if err != nil {
		t.Fatalf("failed to save label items: %v", err)
	}

	var initTuiCalledWith string
	session := &Session{
		Config: config,
		Deps: Deps{
			TUI: stubTUI{
				RunFn: func(_ *cfg_g.Config, labelPrefix string, _ ...string) error {
					initTuiCalledWith = labelPrefix
					return nil
				},
			},
		},
	}

	cliApp := InitCLI(session)
	err = cliApp.Execute([]string{"tui", "%spam"})
	if err != nil {
		t.Fatalf("unexpected error executing tui command: %v", err)
	}

	if initTuiCalledWith != "Spam" {
		t.Errorf("expected InitTUI to be called with Spam, got %q", initTuiCalledWith)
	}
}

func TestAllCommandsNoPanic(t *testing.T) {
	session := &Session{
		Config: &cfg_g.Config{},
	}
	cliApp := InitCLI(session)
	for _, cmd := range cliApp.Commands {
		t.Run(cmd.Name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("command %q panicked during execution: %v", cmd.Name, r)
				}
			}()
			_ = cliApp.Execute([]string{cmd.Name, "--help"})
		})
	}
}

func TestOmitGlobalFlagsInSubcommands(t *testing.T) {
	session := &Session{Config: &cfg_g.Config{}}
	cliApp := InitCLI(session)
	if !cliApp.OmitGlobalFlagsInCommands {
		t.Error("expected OmitGlobalFlagsInCommands to be true on InitCLI")
	}
	var buf bytes.Buffer
	cliApp.RenderCommand(clihelp.Options{Writer: &buf}, "download")
	out := buf.String()
	if strings.Contains(out, "--logappend") {
		t.Errorf("subcommand help should omit global persistent options table, but found --logappend in:\n%s", out)
	}
}

func TestArchiveCmd_ErrorMentionsArchiveAll(t *testing.T) {
	mockClient := &mailclient.MockMailClient{}
	session := &Session{
		Config: &cfg_g.Config{},
		Deps: Deps{
			Clients: stubClients{
				ForSelectedFn: func(_ *cfg_g.Config) (mailclient.MailClient, error) {
					return mockClient, nil
				},
			},
		},
	}
	cliApp := InitCLI(session)
	err := cliApp.Execute([]string{"archive", "all", "label1", "extra_arg"})
	if err == nil {
		t.Fatal("expected error with too many args for archive all, got nil")
	}
	if !strings.Contains(err.Error(), "archive all") {
		t.Errorf("expected error message to mention 'archive all', got: %v", err)
	}
	if strings.Contains(err.Error(), "arc all") {
		t.Errorf("error message should not use non-canonical alias 'arc all': %v", err)
	}
}

// A label reference is unambiguous -- only a label starts with the sigil -- so
// naming one at the root is taken as a request to scan it. `mail_cli %` is the
// inbox.
func TestRootScansALabelReference(t *testing.T) {
	tests := []struct {
		args         []string
		wantPrefix   string
		wantExplicit bool
	}{
		{[]string{"%"}, "%", true},
		{[]string{"%inbox"}, "%inbox", true},
		{[]string{"%receipts"}, "%receipts", false},
		{[]string{"%work:receipts"}, "%work:receipts", false},
	}

	for _, tt := range tests {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			var gotPrefix string
			var gotExplicit bool
			session := &Session{
				Config: &cfg_g.Config{},
				Deps: Deps{
					Mail: stubMail{
						ScanFn: func(o scan.Options, _ *cfg_g.Config, prefix string) (int, error) {
							gotPrefix, gotExplicit = prefix, o.ExplicitInbox
							return 0, nil
						},
					},
				},
			}

			if err := InitCLI(session).Execute(tt.args); err != nil {
				t.Fatalf("Execute(%v): %v", tt.args, err)
			}
			if gotPrefix != tt.wantPrefix {
				t.Errorf("scanned %q, want %q", gotPrefix, tt.wantPrefix)
			}
			if gotExplicit != tt.wantExplicit {
				t.Errorf("ExplicitInbox = %v, want %v", gotExplicit, tt.wantExplicit)
			}
		})
	}
}

// The bare `%label` form runs scan, so it must accept scan's flags, not just
// -p. It once bound only -p while scanLabelRef still read -m, --move-from and
// --inbox-move, so `mail_cli % -m` died as an unknown flag.
func TestRootBareFormAcceptsScanFlags(t *testing.T) {
	t.Cleanup(func() {
		flagMoveSpam, flagMoveSpamFrom, flagMoveInboxStr = false, "", ""
	})

	rec := &sessionRecorder{}
	session := recordingSession(rec)

	args := []string{"%", "-m", "--move-from", "spammer@example.com", "--inbox-move", "friend@example.com"}
	if err := InitCLI(session).Execute(args); err != nil {
		t.Fatalf("Execute(%v): %v", args, err)
	}
	if !rec.opts.MoveSpam {
		t.Error("MoveSpam = false; -m was not bound at the root")
	}
	if rec.opts.MoveFrom != "spammer@example.com" {
		t.Errorf("MoveFrom = %q, want spammer@example.com", rec.opts.MoveFrom)
	}
	if rec.opts.MoveInbox != "friend@example.com" {
		t.Errorf("MoveInbox = %q, want friend@example.com", rec.opts.MoveInbox)
	}
}

// A zero-argument invocation must still not scan: it once ran across every
// configured account, which is why it prints help instead.
func TestRootWithNoArgumentsDoesNotScan(t *testing.T) {
	scanned := false
	var out bytes.Buffer
	session := &Session{
		Config: &cfg_g.Config{},
		Out:    &out,
		Deps: Deps{
			Mail: stubMail{
				ScanFn: func(scan.Options, *cfg_g.Config, string) (int, error) {
					scanned = true
					return 0, nil
				},
			},
		},
	}

	cliApp := InitCLI(session)
	cliApp.Stdout = &out
	if err := cliApp.Execute(nil); err != nil {
		t.Fatalf("Execute(): %v", err)
	}
	if scanned {
		t.Error("a bare invocation must not scan")
	}
	if !strings.Contains(out.String(), "Usage:") {
		t.Errorf("expected help, got %q", out.String())
	}
}
