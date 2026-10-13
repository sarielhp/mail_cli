package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/sarielhp/mail_cli/app"
	"github.com/sarielhp/mail_cli/cfg_g"

	"github.com/sarielhp/clihelp"
)

// --read-only has one spelling now. `--ro` was a hidden alias for it and has
// been removed; `--dry-run` is a different contract with its own subtest below,
// not a synonym, so it does not belong in this loop -- it sets app.FlagDryRun
// and leaves app.FlagReadOnly alone.
func TestReadOnlyCanonicalSpelling(t *testing.T) {
	for _, flag := range []string{"--read-only"} {
		t.Run(flag, func(t *testing.T) {
			app.FlagReadOnly = false
			cfg := &cfg_g.Config{}
			session := &Session{
				Config: cfg,
			}
			cliApp := InitCLI(session)
			var buf bytes.Buffer
			cliApp.Stdout = &buf
			cliApp.Stderr = &buf

			err := cliApp.Execute([]string{flag})
			if err != nil {
				t.Fatalf("unexpected error executing with %s: %v", flag, err)
			}
			if !app.FlagReadOnly {
				t.Errorf("expected app.FlagReadOnly to be true for flag %s", flag)
			}
			if !cfg.ReadOnly {
				t.Errorf("expected session.Config.ReadOnly to be true for flag %s", flag)
			}
		})
	}

	// --dry-run was once a synonym for --read-only and was removed to leave one
	// spelling. It is back, meaning something wider: --read-only forbids server
	// mutations but still caches, and --dry-run forbids the local writes too.
	// A synonym retired and a distinct flag later added under the same name is
	// coherent only while help explains both, which is what the checks below
	// are for.
	t.Run("dry-run implies read-only and is wider", func(t *testing.T) {
		app.FlagReadOnly, app.FlagDryRun = false, false
		cfg := &cfg_g.Config{}
		session := &Session{Config: cfg}
		cliApp := InitCLI(session)
		var buf bytes.Buffer
		cliApp.Stdout, cliApp.Stderr = &buf, &buf

		if err := cliApp.Execute([]string{"--dry-run"}); err != nil {
			t.Fatalf("unexpected error executing with --dry-run: %v", err)
		}
		if !cfg.DryRun {
			t.Error("expected --dry-run to set Config.DryRun")
		}
		if !cfg.ReadOnly {
			t.Error("--dry-run must imply --read-only: it forbids strictly more")
		}

		// ...and not the reverse.
		app.FlagReadOnly, app.FlagDryRun = false, false
		plain := &cfg_g.Config{}
		plainApp := InitCLI(&Session{Config: plain})
		plainApp.Stdout, plainApp.Stderr = &buf, &buf
		if err := plainApp.Execute([]string{"--read-only"}); err != nil {
			t.Fatalf("unexpected error executing with --read-only: %v", err)
		}
		if plain.DryRun {
			t.Error("--read-only must not imply --dry-run: caching is permitted under it")
		}
	})

	// Verify help output contains only canonical --read-only, not --dry-run or --ro
	session := &Session{Config: &cfg_g.Config{}}
	cliApp := InitCLI(session)
	var helpBuf bytes.Buffer
	cliApp.RenderGlobal(clihelp.Options{Writer: &helpBuf})
	rendered := helpBuf.String()

	if !strings.Contains(rendered, "--read-only") {
		t.Errorf("expected global help to contain canonical flag '--read-only'")
	}
	// Both are advertised now, because they mean different things and a user
	// who cannot tell them apart will distrust both.
	if !strings.Contains(rendered, "--dry-run") {
		t.Errorf("expected global help to document '--dry-run' beside '--read-only'")
	}
	if strings.Contains(rendered, "--ro") {
		t.Errorf("expected global help NOT to advertise legacy alias '--ro'")
	}
}

func TestGlobalHelpDoesNotAdvertiseShortcuts(t *testing.T) {
	session := &Session{Config: &cfg_g.Config{}}
	cliApp := InitCLI(session)
	var helpBuf bytes.Buffer
	cliApp.RenderGlobal(clihelp.Options{Writer: &helpBuf})
	rendered := helpBuf.String()

	if strings.Contains(rendered, "Shortcut Commands:") {
		t.Errorf("expected global help NOT to contain 'Shortcut Commands:' section")
	}
	if strings.Contains(rendered, "Shortcut alias for:") {
		t.Errorf("expected global help NOT to contain 'Shortcut alias for:'")
	}
}

func TestTuiCommandDescriptionTiering(t *testing.T) {
	session := &Session{Config: &cfg_g.Config{}}
	cmd := TuiCmd(session)

	const maxDescLen = 45
	if len(cmd.Description) > maxDescLen {
		t.Errorf("TuiCmd description length %d exceeds limit of %d: %q", len(cmd.Description), maxDescLen, cmd.Description)
	}
	if cmd.Description == "" {
		t.Errorf("TuiCmd description must not be empty")
	}
	if cmd.LongDescription == "" {
		t.Errorf("TuiCmd LongDescription should contain detailed documentation")
	}
	if !strings.Contains(cmd.LongDescription, "label_prefix") {
		t.Errorf("TuiCmd LongDescription should explain label_prefix matching")
	}
}

// The suggestions come from clihelp now, not from a copy of its algorithm that
// lived in cli/suggest.go. The wording is the library's; what is asserted is the
// substance — which command it proposes, including a nested path.
func TestUnknownCommandSuggestions(t *testing.T) {
	session := &Session{Config: &cfg_g.Config{}}
	cliApp := InitCLI(session)

	tests := []struct {
		name        string
		args        []string
		wantContain string
	}{
		{
			name:        "typo suggests top-level command",
			args:        []string{"scn", "inbox"},
			wantContain: `unknown command "scn" for "mail_cli". Did you mean "scan"?`,
		},
		{
			name:        "exact subcommand suggests parent path",
			args:        []string{"clear"},
			wantContain: `unknown command "clear" for "mail_cli". Did you mean "utils cache clear"?`,
		},
		{
			name:        "nested subcommand suggests full path",
			args:        []string{"audit"},
			wantContain: `unknown command "audit" for "mail_cli". Did you mean "spam pol audit"?`,
		},
		{
			name:        "unrecognized input without close match hints at help",
			args:        []string{"foobarxyz"},
			wantContain: `unknown command "foobarxyz" for "mail_cli". Run "mail_cli -h" for a list of commands`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := cliApp.Execute(tc.args)
			if err == nil {
				t.Fatalf("expected error for args %v, got nil", tc.args)
			}
			if !strings.Contains(err.Error(), tc.wantContain) {
				t.Errorf("error %q does not contain expected text %q", err.Error(), tc.wantContain)
			}
		})
	}
}
