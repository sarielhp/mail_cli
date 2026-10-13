package cli

import (
	"bytes"
	"sort"
	"strings"
	"testing"

	"github.com/sarielhp/mail_cli/cfg_g"

	"github.com/sarielhp/clihelp"
)

// topLevelNames returns every name the root accepts, aliases included.
func topLevelNames(t *testing.T) map[string]bool {
	t.Helper()
	app := InitCLI(&Session{Config: &cfg_g.Config{}})

	names := map[string]bool{}
	for _, cmd := range app.Commands {
		names[cmd.Name] = true
		for _, a := range cmd.Aliases {
			names[a] = true
		}
	}
	return names
}

// The retired spellings are gone, and must stay gone.
//
// Each was a hidden duplicate that warned and pointed at its replacement --
// the deprecation window that 0.8.0 closes. A duplicate is cheap to add back by
// accident and expensive to notice: it is hidden from help by construction, so
// nothing but this test would say it had returned.
func TestRetiredSpellingsAreGone(t *testing.T) {
	names := topLevelNames(t)

	retired := []string{
		"splice", "split",
		"unspam", "learning", "learn-ham", "test",
		"filter", "caladd", "whitelist", "blacklist",
	}

	for _, name := range retired {
		if names[name] {
			t.Errorf("%q is a retired spelling and has come back; it belongs to its canonical command now", name)
		}
	}
}

// What replaced them must exist, or the test above passes by deleting the
// feature rather than by renaming it.
func TestCanonicalPathsExist(t *testing.T) {
	app := InitCLI(&Session{Config: &cfg_g.Config{}})

	for _, path := range [][]string{
		{"label", "sort"}, {"label", "move"},
		{"label", "download"}, {"label", "upload"},
		{"spam", "unmark"}, {"spam", "allow"}, {"spam", "block"},
		{"rule", "remote"}, {"calendar", "sync"}, {"doctor"},
	} {
		t.Run(strings.Join(path, " "), func(t *testing.T) {
			if cmd := app.LookupCommand(path...); cmd == nil {
				t.Errorf("%q does not resolve; the retired spelling was deleted without its replacement",
					strings.Join(path, " "))
			}
		})
	}
}

// The shortcuts that were kept are real commands, not argv rewrites and not
// deprecated duplicates: help and completion can see a command, and neither can
// see a rewrite. They are hidden because they are shorthand, not because they
// are on their way out.
func TestKeptShortcutsAreRealHiddenCommands(t *testing.T) {
	app := InitCLI(&Session{Config: &cfg_g.Config{}})

	for _, name := range []string{"ss", "sb", "wlist", "blist"} {
		t.Run(name, func(t *testing.T) {
			cmd := app.LookupCommand(name)
			if cmd == nil {
				t.Fatalf("%q is muscle memory and was promised to keep working", name)
			}
			if !cmd.Hidden {
				t.Errorf("%q is shorthand and should not crowd the help listing", name)
			}
			if cmd.Run == nil && len(cmd.Subcommands) == 0 {
				t.Errorf("%q resolves to nothing runnable", name)
			}
		})
	}
}

// Nothing in the tree may warn about its own name any more. The mechanism that
// printed those notices is deleted; this catches a hand-rolled revival.
func TestNoCommandAnnouncesItsOwnDeprecation(t *testing.T) {
	app := InitCLI(&Session{Config: &cfg_g.Config{}})

	var offenders []string
	_ = app.Walk(func(path []string, cmd *clihelp.Command) error {
		text := cmd.Description + " " + cmd.LongDescription
		if strings.Contains(strings.ToLower(text), "deprecat") {
			offenders = append(offenders, strings.Join(path, " "))
		}
		return nil
	})

	sort.Strings(offenders)
	if len(offenders) != 0 {
		t.Errorf("commands still describe themselves as deprecated: %v", offenders)
	}
}

// The confirmation matrix, asserted rather than described.
//
// Every one of these deletes something the server cannot give back, so each
// prompts on a terminal and refuses without --yes when stdin is not one. The
// matrix listed five rows and only four were implemented: `label rm` deleted a
// server label with no prompt at all.
func TestIrreversibleCommandsRefuseWithoutATerminal(t *testing.T) {
	commands := [][]string{
		{"rule", "rm-all"},
		{"cache", "clear"},
	}

	for _, argv := range commands {
		t.Run(strings.Join(argv[:2], " "), func(t *testing.T) {
			// A pipe, not a terminal -- which is the condition under test.
			session := &Session{Config: &cfg_g.Config{}, In: strings.NewReader("")}

			err := InitCLI(session).Execute(argv)
			if err == nil {
				t.Fatalf("%v proceeded with no terminal to ask; it destroys something", argv)
			}
			if !strings.Contains(err.Error(), "--yes") {
				t.Errorf("the refusal should name --yes, got: %v", err)
			}
		})
	}
}

// LongDescription must provide genuine additional value over Description,
// never a tautological copy or trivial rephrase.
func TestLongDescriptionAddsSubstance(t *testing.T) {
	app := InitCLI(&Session{Config: &cfg_g.Config{}})

	_ = app.Walk(func(path []string, cmd *clihelp.Command) error {
		if cmd.LongDescription == "" {
			return nil
		}
		cleanShort := strings.TrimSpace(cmd.Description)
		cleanLong := strings.TrimSpace(cmd.LongDescription)

		if strings.EqualFold(cleanShort, cleanLong) {
			t.Errorf("%s: LongDescription is an identical duplicate of Description", strings.Join(path, " "))
		}
		if len(cleanLong) < len(cleanShort)+20 {
			t.Errorf("%s: LongDescription (%d chars) does not meaningfully expand on Description (%d chars)",
				strings.Join(path, " "), len(cleanLong), len(cleanShort))
		}
		return nil
	})
}

// Subcommands under rule remote must be compact single-line entries fitting within 80 columns.
func TestRuleRemoteSubcommandsAreSingleLine(t *testing.T) {
	app := InitCLI(&Session{Config: &cfg_g.Config{}})
	var buf bytes.Buffer
	app.RenderCommand(clihelp.Options{Writer: &buf, Width: 80, Concise: true}, "rule", "remote")
	rendered := buf.String()

	subSection := false
	var subLines []string
	for _, line := range strings.Split(rendered, "\n") {
		if strings.HasPrefix(line, "Subcommands:") {
			subSection = true
			continue
		}
		if subSection {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				break
			}
			subLines = append(subLines, line)
		}
	}

	cmd := app.LookupCommand("rule", "remote")
	if cmd == nil {
		t.Fatalf("could not find rule remote command")
	}
	if len(subLines) != len(cmd.Subcommands) {
		t.Errorf("expected %d single-line subcommands, got %d lines:\n%v",
			len(cmd.Subcommands), len(subLines), subLines)
	}
	for _, line := range subLines {
		if len(line) > 80 {
			t.Errorf("rule remote subcommand line exceeds 80 columns (%d chars): %q", len(line), line)
		}
	}
}

// All commands and subcommands must be concise single-line entries fitting within 80 columns.
func TestAllCommandsAndSubcommandsAreSingleLineAt80Cols(t *testing.T) {
	app := InitCLI(&Session{Config: &cfg_g.Config{}})

	checkGroup := func(name string, cmds []clihelp.Command, path ...string) {
		t.Run(name, func(t *testing.T) {
			var visible []clihelp.Command
			for _, c := range cmds {
				if !c.Hidden {
					visible = append(visible, c)
				}
			}
			if len(visible) == 0 {
				return
			}

			var buf bytes.Buffer
			opts := clihelp.Options{Writer: &buf, Width: 80, Concise: true, ConciseMaxLines: -1}
			if len(path) == 0 {
				app.RenderGlobal(opts)
			} else {
				app.RenderCommand(opts, path...)
			}
			rendered := buf.String()
			t.Logf("RENDERED [%s]:\n%s", name, rendered)

			lines := strings.Split(rendered, "\n")
			inSection := false
			var cmdLines []string
			var contLines []string

			for _, line := range lines {
				trimmed := strings.TrimSpace(line)
				if strings.HasPrefix(line, "Commands:") || strings.HasPrefix(line, "Subcommands:") {
					inSection = true
					continue
				}
				if !inSection {
					continue
				}
				if strings.HasPrefix(line, "Flags:") || strings.HasPrefix(line, "Global Flags:") ||
					strings.HasPrefix(line, "Parameters:") || strings.HasPrefix(line, "Options:") ||
					strings.HasPrefix(line, "Examples:") || strings.HasPrefix(line, "Extended Help:") {
					break
				}
				if trimmed == "" {
					continue
				}
				if strings.HasSuffix(trimmed, ":") && !strings.HasPrefix(line, "  ") {
					continue
				}
				if strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "    ") {
					cmdLines = append(cmdLines, line)
					if len(line) > 80 {
						t.Errorf("line exceeds 80 columns (%d chars): %q", len(line), line)
					}
				} else if strings.HasPrefix(line, "    ") {
					contLines = append(contLines, line)
				}
			}

			if len(contLines) > 0 {
				t.Errorf("found %d wrapped continuation lines (expected single-line per command):\n%s",
					len(contLines), strings.Join(contLines, "\n"))
			}
			if len(cmdLines) != len(visible) {
				t.Errorf("expected %d single-line commands, got %d command lines", len(visible), len(cmdLines))
			}
		})
	}

	// 1. Root commands
	checkGroup("root", app.Commands)

	// 2. All commands with subcommands
	_ = app.Walk(func(path []string, cmd *clihelp.Command) error {
		if len(cmd.Subcommands) > 0 {
			if path[0] == "completion" || path[0] == "manpage" ||
				(len(path) > 1 && (path[0] == "utils" || path[0] == "tools") && (path[1] == "completion" || path[1] == "manpage")) {
				return nil
			}
			checkGroup(strings.Join(path, " "), cmd.Subcommands, path...)
		}
		return nil
	})
}
