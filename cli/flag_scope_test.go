package cli

import (
	"strings"
	"testing"

	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/scan"
)

// Scan-only flags used to be persistent, so every command in the tree accepted
// them, advertised them on its help page, and ignored them. `config show -p x
// --fix` parsed cleanly and did nothing with either -- the user got no output
// they asked for and no error saying why.
//
// The charter allows five persistent flags: -A, -v, --dry-run, --json and
// --logappend. A command that does not read a flag must not accept it.
func TestScanOnlyFlagsAreNotPersistent(t *testing.T) {
	for _, arg := range []string{"-p", "--pattern", "--fix"} {
		t.Run(arg, func(t *testing.T) {
			session := &Session{
				Config: &cfg_g.Config{},
				Deps: Deps{
					Config: stubConfig{ShowFn: func(*cfg_g.Config) error { return nil }},
				},
			}

			argv := []string{"config", "show", arg}
			if arg != "--fix" {
				argv = append(argv, "x")
			}

			err := InitCLI(session).Execute(argv)
			if err == nil {
				t.Fatalf("%v was accepted by `config show`, which does not read it", argv)
			}
			if !strings.Contains(err.Error(), "unknown") {
				t.Errorf("expected an unknown-flag error for %q, got: %v", arg, err)
			}
		})
	}
}

// The commands that do read the pattern still take it: `scan`, the bare form
// that runs scan, and `label sort`. The bare form is why the root declares it
// as its own option rather than inheriting one.
func TestPatternFlagReachesTheCommandsThatReadIt(t *testing.T) {
	tests := []struct {
		name string
		argv []string
	}{
		{name: "scan", argv: []string{"scan", "%inbox", "-p", "invoice"}},
		{name: "bare scan form", argv: []string{"%inbox", "-p", "invoice"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got string
			session := &Session{
				Config: &cfg_g.Config{},
				Deps: Deps{
					Mail: stubMail{
						ScanFn: func(o scan.Options, _ *cfg_g.Config, _ string) (int, error) {
							got = o.Pattern
							return 0, nil
						},
					},
				},
			}

			if err := InitCLI(session).Execute(tc.argv); err != nil {
				t.Fatalf("Execute(%v): %v", tc.argv, err)
			}
			if got != "invoice" {
				t.Errorf("scan received Pattern %q, want %q", got, "invoice")
			}
		})
	}
}

// --fix repairs the config file, and it used to be reachable from every
// command because Validate() runs on the way into most of them. Only the
// command whose job is repair may turn it on.
func TestFixIsScopedToConfigCheck(t *testing.T) {
	var sawFix bool
	session := &Session{
		Config: &cfg_g.Config{},
		Deps: Deps{
			Config: stubConfig{ValidateFn: func(c *cfg_g.Config) error {
				sawFix = c.FixConfig
				return nil
			}},
		},
	}

	cliApp := InitCLI(session)
	if err := cliApp.Execute([]string{"config", "check"}); err != nil {
		t.Fatalf("config check: %v", err)
	}
	if sawFix {
		t.Error("config check without --fix set FixConfig")
	}

	if err := cliApp.Execute([]string{"config", "check", "--fix"}); err != nil {
		t.Fatalf("config check --fix: %v", err)
	}
	if !sawFix {
		t.Error("config check --fix did not set FixConfig")
	}
}

// `config set` took two arguments or three. The third selected an
// account-specific override, was threaded through three layers, and was read
// nowhere -- so the promise was false and any stray word triggered it silently.
// The account is chosen with -A, so two arguments is the whole grammar.
func TestConfigSetRefusesAStrayThirdArgument(t *testing.T) {
	var calls int
	newSession := func() *Session {
		return &Session{
			Config: &cfg_g.Config{},
			Deps: Deps{
				Config: stubConfig{SetFn: func(*cfg_g.Config, string, string) error {
					calls++
					return nil
				}},
			},
		}
	}

	if err := InitCLI(newSession()).Execute([]string{"config", "set", "spam_folder", "Junk"}); err != nil {
		t.Fatalf("two arguments should be accepted: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected the set to reach the service once, got %d", calls)
	}

	err := InitCLI(newSession()).Execute([]string{"config", "set", "spam_folder", "Junk", "stray"})
	if err == nil {
		t.Fatal("a third argument was accepted; it used to vanish silently")
	}
	if calls != 1 {
		t.Errorf("the rejected command still reached the service: %d calls", calls)
	}
}

func TestConfig_GetCommand(t *testing.T) {
	var gotKey string
	session := &Session{
		Config: &cfg_g.Config{},
		Deps: Deps{
			Config: stubConfig{GetFn: func(_ *cfg_g.Config, key string) error {
				gotKey = key
				return nil
			}},
		},
	}

	cliApp := InitCLI(session)
	if err := cliApp.Execute([]string{"config", "get", "mutt_alias_file"}); err != nil {
		t.Fatalf("config get failed: %v", err)
	}
	if gotKey != "mutt_alias_file" {
		t.Errorf("expected key mutt_alias_file, got %s", gotKey)
	}
}
