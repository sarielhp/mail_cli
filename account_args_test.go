package main

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/cli"

	"github.com/sarielhp/clihelp"
)

func TestNormalizeAccountArgs(t *testing.T) {
	for _, tt := range []struct {
		name string
		args []string
		want []string
	}{
		{"leading", []string{"-2", "scan", "%inbox"}, []string{"--account-index=2", "scan", "%inbox"}},
		{"trailing", []string{"scan", "%inbox", "-12"}, []string{"scan", "%inbox", "--account-index=12"}},
		{"terminator", []string{"scan", "--", "-2"}, []string{"scan", "--", "-2"}},
		{"long value", []string{"scan", "--pattern", "-2"}, []string{"scan", "--pattern", "-2"}},
		{"short value", []string{"-A", "-2", "scan"}, []string{"-A", "-2", "scan"}},
		{"attached value", []string{"-A-2", "scan"}, []string{"-A-2", "scan"}},
		{"cluster value", []string{"-vA", "-2", "scan"}, []string{"-vA", "-2", "scan"}},
		{"bool then index", []string{"-v", "-2", "scan"}, []string{"-v", "--account-index=2", "scan"}},
		{"optional value", []string{"scan", "-m", "-2"}, []string{"scan", "-m", "--account-index=2"}},
		{"canonical", []string{"--account-index", "2", "scan"}, []string{"--account-index", "2", "scan"}},
		{"completion", []string{"__complete", "-2", "tui", ""}, []string{"__complete", "--account-index=2", "tui", ""}},
		{"explanation", []string{"__explain", "-2", "scan"}, []string{"__explain", "--account-index=2", "scan"}},
		{"wrapper data", []string{"__clihelp", "wrapper", "mt", "-2", "tui"}, []string{"__clihelp", "wrapper", "mt", "-2", "tui"}},
		{"unknown flag", []string{"--bogus", "-2"}, []string{"--bogus", "-2"}},
		{"retired spelling", []string{"-wlist", "-2"}, []string{"-wlist", "-2"}},
		{"dry run", []string{"--dry-run", "-2", "scan"}, []string{"--dry-run", "--account-index=2", "scan"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			config := &cfg_g.Config{SelectedAccount: "unchanged"}
			cliApp := cli.InitCLI(&cli.Session{Config: config})
			original := append([]string{}, tt.args...)
			got, err := normalizeAccountArgs(cliApp, tt.args)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("normalized=%v want=%v", got, tt.want)
			}
			if config.SelectedAccount != "unchanged" || !reflect.DeepEqual(original, tt.args) {
				t.Fatal("normalization changed configuration or the original arguments")
			}
		})
	}
}

func TestAccountIndexSelection(t *testing.T) {
	for _, tt := range []struct {
		name  string
		args  []string
		want  string
		usage bool
	}{
		{"shortcut", []string{"-2"}, "second", false},
		{"canonical", []string{"--account-index", "2"}, "second", false},
		{"repeated", []string{"-2", "-1"}, "first", false},
		{"mixed", []string{"-2", "-A", "first"}, "", true},
		{"zero", []string{"--account-index=0"}, "", true},
		{"negative", []string{"--account-index=-1"}, "", true},
		{"out of range", []string{"-3"}, "", true},
		{"overflow", []string{"-999999999999999999999999999"}, "", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			config := &cfg_g.Config{Accounts: []cfg_acc.AccountConfig{{Name: "first"}, {Name: "second"}}}
			cliApp := cli.InitCLI(&cli.Session{Config: config})
			cliApp.Run = func(*clihelp.Context) error { return nil }
			args, err := normalizeAccountArgs(cliApp, tt.args)
			if err == nil {
				err = cliApp.Execute(args)
			}
			if tt.usage {
				if !clihelp.IsUsageError(err) || exitCodeFor(err) != exitUsageError {
					t.Fatalf("expected usage exit, got %v", err)
				}
				return
			}
			if err != nil || string(config.SelectedAccount) != tt.want {
				t.Fatalf("selected=%q error=%v want=%q", config.SelectedAccount, err, tt.want)
			}
		})
	}
}

func TestWrapperCompletionKeepsItsPresetCommand(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cliApp := cli.InitCLI(&cli.Session{Config: &cfg_g.Config{}})
	args, err := normalizeAccountArgs(cliApp, []string{"__complete", "-2", "tui", ""})
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	cliApp.Stdout = &out
	if err := cliApp.Execute(args); err != nil {
		t.Fatalf("__complete: %v", err)
	}
	words := map[string]bool{}
	for _, line := range strings.Split(out.String(), "\n") {
		words[strings.SplitN(line, "\t", 2)[0]] = true
	}
	if !words["write"] || words["scan"] {
		t.Errorf("completion lost tui's context:\n%s", out.String())
	}
}
