package cli

import (
	"bytes"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"

	"github.com/sarielhp/clihelp"
)

func TestCompactRootCommandInventory(t *testing.T) {
	app := InitCLI(&Session{Config: &cfg_g.Config{}})
	var visible []string
	for _, cmd := range app.Commands {
		if !cmd.Hidden {
			visible = append(visible, cmd.Name)
		}
	}
	want := []string{"scan", "search", "write", "tui", "watch", "msg", "label", "mbox", "spam", "rule", "calendar", "contacts", "account", "config", "utils"}
	if !reflect.DeepEqual(visible, want) {
		t.Fatalf("visible root commands = %v, want %v", visible, want)
	}

	var out bytes.Buffer
	app.RenderGlobal(clihelp.Options{Writer: &out, Width: 80, Concise: true})
	for _, name := range want {
		if !strings.Contains(out.String(), "\n  "+name+" ") {
			t.Errorf("%q is missing from the first screen of concise help:\n%s", name, out.String())
		}
	}
	out.Reset()
	app.Stdout = &out
	if err := app.Execute([]string{"__complete", ""}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"compose", "tools", "send", "show", "reply", "drafts", "last", "export", "setup", "migrate", "llm", "cache", "color", "doctor", "completion", "manpage"} {
		if strings.Contains(out.String(), name+"\t") {
			t.Errorf("root completion still advertises moved command %q", name)
		}
	}
}

// Check every descendant, including hidden aliases and default actions. A new
// tree must not silently drop a feature, flag, completer, or argument validator.
func TestMovedCommandsPreserveTheirEntireSurface(t *testing.T) {
	app := InitCLI(&Session{Config: &cfg_g.Config{}})
	tests := []struct{ old, path string }{
		{"send", "msg send"}, {"show", "msg show"},
		{"compose", "write"}, {"msg compose", "write"}, {"tools", "utils"},
		{"reply", "msg reply"}, {"drafts", "msg drafts"},
		{"last", "msg last"}, {"export", "msg export"},
		{"setup", "account setup"}, {"migrate", "account migrate"},
		{"llm", "config llm"}, {"cache", "utils cache"},
		{"color", "utils color"}, {"doctor", "utils doctor"},
		{"completion", "utils completion"}, {"manpage", "utils manpage"},
	}
	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			old := app.LookupCommand(strings.Fields(tc.old)...)
			current := app.LookupCommand(strings.Fields(tc.path)...)
			if old == nil || current == nil {
				t.Fatalf("missing compatibility or canonical command: %s -> %s", tc.old, tc.path)
			}
			if !old.Hidden || current.Hidden {
				t.Error("only the compatibility spelling should be hidden")
			}
			assertCommandSurface(t, old, current)
			var out bytes.Buffer
			app.RenderCommand(clihelp.Options{Writer: &out, Width: 80}, strings.Fields(tc.path)...)
			if !strings.Contains(out.String(), "mail_cli "+tc.path) {
				t.Errorf("help does not teach the canonical path:\n%s", out.String())
			}
		})
	}
}

func assertCommandSurface(t *testing.T, old, current *clihelp.Command) {
	t.Helper()
	for _, pair := range [][2]any{
		{old.Run, current.Run},
		{old.PreRun, current.PreRun}, {old.PostRun, current.PostRun},
		{old.OptionsValidator, current.OptionsValidator},
	} {
		if reflect.ValueOf(pair[0]).IsNil() != reflect.ValueOf(pair[1]).IsNil() {
			t.Errorf("%s: handler or validator missing", current.Name)
		}
	}
	assertArgumentSurface(t, old.Args, current.Args)
	for _, pair := range [][2][]clihelp.Option{{old.Options, current.Options}, {old.PersistentOptions, current.PersistentOptions}} {
		if len(pair[0]) != len(pair[1]) {
			t.Fatalf("%s: option count changed", current.Name)
		}
		for i, opt := range pair[0] {
			newOpt := pair[1][i]
			changed := opt.Flags != newOpt.Flags || opt.Required != newOpt.Required
			assertSurfaceEntry(t, fmt.Sprintf("%s: option %q", current.Name, opt.Flags), changed, opt.Complete, newOpt.Complete)
		}
	}
	if !reflect.DeepEqual(old.Parameters, current.Parameters) {
		// DeepEqual treats non-nil functions as unequal; inspect positional hooks.
		if len(old.Parameters) != len(current.Parameters) {
			t.Fatalf("%s: parameter count changed", current.Name)
		}
		for i, param := range old.Parameters {
			newParam := current.Parameters[i]
			changed := param.Name != newParam.Name || param.Variadic != newParam.Variadic
			assertSurfaceEntry(t, fmt.Sprintf("%s: parameter %q", current.Name, param.Name), changed, param.Complete, newParam.Complete)
		}
	}
	if len(old.Subcommands) != len(current.Subcommands) {
		t.Fatalf("%s: subcommand count changed", current.Name)
	}
	for i := range old.Subcommands {
		a, b := &old.Subcommands[i], &current.Subcommands[i]
		if a.Name != b.Name || !reflect.DeepEqual(a.Aliases, b.Aliases) {
			t.Errorf("%s: descendant name or aliases changed", a.Name)
		}
		assertCommandSurface(t, a, b)
	}
}

// assertSurfaceEntry fails when a clihelp option or parameter differs between a
// compatibility command and its canonical spelling: either its identity fields
// changed, or its completion hook is no longer the same function.
func assertSurfaceEntry(t *testing.T, label string, fieldsDiffer bool, oldHook, newHook any) {
	t.Helper()
	if fieldsDiffer || reflect.ValueOf(oldHook).Pointer() != reflect.ValueOf(newHook).Pointer() {
		t.Errorf("%s changed", label)
	}
}

func assertArgumentSurface(t *testing.T, old, current clihelp.ArgsValidator) {
	t.Helper()
	if old == nil || current == nil {
		if (old == nil) != (current == nil) {
			t.Error("argument validator missing")
		}
		return
	}
	oldMin, oldMax := old.Arity()
	newMin, newMax := current.Arity()
	if oldMin != newMin || oldMax != newMax {
		t.Errorf("argument range changed: (%d, %d) -> (%d, %d)", oldMin, oldMax, newMin, newMax)
	}
	args := []string{"a", "b", "c", "d"}
	for n := 0; n <= len(args); n++ {
		if (old.ValidateArgs(args[:n]) == nil) != (current.ValidateArgs(args[:n]) == nil) {
			t.Errorf("argument validation changed for %d args", n)
		}
	}
}

func TestNestedCompletionAndParentHelp(t *testing.T) {
	app := InitCLI(&Session{Config: &cfg_g.Config{}})
	for _, tc := range []struct {
		path string
		want []string
	}{
		{"tui", []string{"write"}},
		{"msg", []string{"show", "send", "reply", "drafts", "last", "export", "archive", "block"}},
		{"account", []string{"setup", "migrate", "login", "new"}},
		{"config", []string{"show", "set", "check", "llm"}},
		{"utils", []string{"cache", "color", "doctor", "completion", "manpage"}},
		{"msg drafts server", []string{"ls", "rm", "prune"}},
	} {
		t.Run(tc.path, func(t *testing.T) {
			var out bytes.Buffer
			app.Stdout = &out
			args := append([]string{"__complete"}, strings.Fields(tc.path)...)
			if err := app.Execute(append(args, "")); err != nil {
				t.Fatal(err)
			}
			for _, name := range tc.want {
				if !strings.Contains(out.String(), name+"\t") {
					t.Errorf("completion missing %q:\n%s", name, out.String())
				}
			}
			out.Reset()
			if err := app.Execute(append(strings.Fields(tc.path), "-h")); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out.String(), "Subcommands:") {
				t.Errorf("parent help missing subcommands:\n%s", out.String())
			}
		})
	}
}

func TestCanonicalComposeReceivesFlagsAndAccount(t *testing.T) {
	for _, path := range [][]string{{"write"}, {"tui", "write"}} {
		t.Run(strings.Join(path, " "), func(t *testing.T) {
			assertComposeInvocation(t, path)
		})
	}
}

func assertComposeInvocation(t *testing.T, path []string) {
	t.Helper()
	var got ComposeOptions
	// -A by display name: the command must see the canonical name.
	session := &Session{
		Config: &cfg_g.Config{Accounts: []cfg_acc.AccountConfig{{Name: "personal"}, {Name: "work", DisplayName: "Work Mail"}}},
		Deps: Deps{TUI: stubTUI{ComposeFn: func(_ *cfg_g.Config, opts ComposeOptions) error {
			got = opts
			return nil
		}}},
	}
	t.Cleanup(func() { flagComposeBody, flagComposeSubject = "", "" })
	args := append([]string{"-A", "Work Mail"}, path...)
	args = append(args, "boss@example.com", "-s", "Status", "--body", "Update")
	if err := InitCLI(session).Execute(args); err != nil {
		t.Fatal(err)
	}
	if got.Account != "work" || got.Subject != "Status" || got.Body != "Update" ||
		!reflect.DeepEqual(got.To, []string{"boss@example.com"}) {
		t.Errorf("compose options = %+v", got)
	}
}

func TestTuiWritePreservesWriteSurface(t *testing.T) {
	app := InitCLI(&Session{Config: &cfg_g.Config{}})
	write := app.LookupCommand("write")
	alias := app.LookupCommand("tui", "write")
	if write == nil || alias == nil {
		t.Fatal("write and tui write must both resolve")
	}
	assertCommandSurface(t, write, alias)
	var out bytes.Buffer
	app.RenderCommand(clihelp.Options{Writer: &out, Width: 80}, "tui", "write")
	if !strings.Contains(out.String(), "mail_cli tui write") {
		t.Errorf("alias help should use its own path:\n%s", out.String())
	}
}
