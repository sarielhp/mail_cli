package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/sarielhp/clihelp"
	"github.com/spf13/pflag"
)

// normalizeAccountArgs preserves the application's numeric convenience spelling
// as an ordinary declared option. It never loads configuration or selects an
// account. Flag values and everything after -- remain data.
func normalizeAccountArgs(a *clihelp.App, args []string) ([]string, error) {
	if len(args) == 0 || args[0] == "__clihelp" {
		return args, nil
	}
	result := make([]string, 0, len(args))
	if args[0] == "__complete" || args[0] == "__explain" {
		result = append(result, args[0])
		args = args[1:]
	}
	cmds := append(append([]clihelp.Command{}, a.Commands...), a.Shortcuts...)
	inherited := append(append([]clihelp.Option{}, a.PersistentOptions...), a.GlobalFlags...)
	fs, err := accountNormalizationFlags(a, append(append([]clihelp.Option{}, inherited...), a.Options...))
	if err != nil {
		return nil, err
	}
	for len(args) > 0 {
		word := args[0]
		if word == "--" {
			return append(result, args...), nil
		}
		if numericAccountFlag(word) {
			result = append(result, "--account-index="+word[1:])
			args = args[1:]
			continue
		}
		if strings.HasPrefix(word, "-") && len(word) > 1 {
			count, known := accountFlagWords(fs, args)
			if !known {
				// Leave malformed input to clihelp rather than guessing its arity.
				return append(result, args...), nil
			}
			result = append(result, args[:count]...)
			args = args[count:]
			continue
		}
		cmd := accountArgumentCommand(cmds, word, a.AbbrevCommands)
		if cmd == nil {
			cmds = nil // a positional ends the command path
		} else {
			inherited = append(inherited, cmd.PersistentOptions...)
			fs, err = accountNormalizationFlags(a, append(append([]clihelp.Option{}, inherited...), cmd.Options...))
			if err != nil {
				return nil, err
			}
			cmds = cmd.Subcommands
		}
		result = append(result, word)
		args = args[1:]
	}
	return result, nil
}

func numericAccountFlag(word string) bool {
	if len(word) < 2 || word[0] != '-' {
		return false
	}
	for _, digit := range word[1:] {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	return true
}

// Binding occurs before Execute and before any parsed state exists. It uses the
// actual option declarations, including optional values and aliases, instead of
// maintaining another flag-name/arity table.
func accountNormalizationFlags(a *clihelp.App, opts []clihelp.Option) (*pflag.FlagSet, error) {
	fs := pflag.NewFlagSet(a.Name, pflag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.BoolP("help-concise", "h", false, "")
	fs.Bool("help", false, "")
	if a.ExtendedHelpFlag {
		fs.BoolP("help-extended", "H", false, "")
	}
	if a.EnableExamplesFlag {
		fs.BoolP("examples", "E", false, "")
	}
	for _, opt := range opts {
		if opt.Binder == nil {
			continue
		}
		if err := opt.Binder(fs); err != nil {
			return nil, fmt.Errorf("normalize account flags: %w", err)
		}
	}
	return fs, nil
}

func accountFlagWords(fs *pflag.FlagSet, args []string) (int, bool) {
	word := args[0]
	if strings.HasPrefix(word, "--") {
		name, _, attached := strings.Cut(word[2:], "=")
		flag := fs.Lookup(name)
		if flag == nil {
			return 0, false
		}
		if attached || flag.NoOptDefVal != "" || len(args) == 1 {
			return 1, true
		}
		return 2, true
	}
	for rest := word[1:]; rest != ""; rest = rest[1:] {
		flag := fs.ShorthandLookup(rest[:1])
		if flag == nil {
			return 0, false
		}
		if len(rest) > 1 && rest[1] == '=' {
			return 1, true
		}
		if flag.NoOptDefVal != "" {
			continue
		}
		if len(rest) > 1 || len(args) == 1 {
			return 1, true
		}
		return 2, true
	}
	return 1, true
}

func accountArgumentCommand(cmds []clihelp.Command, word string, abbrev bool) *clihelp.Command {
	for i := range cmds {
		for _, name := range append([]string{cmds[i].Name}, cmds[i].Aliases...) {
			if word == name {
				return &cmds[i]
			}
		}
	}
	if !abbrev {
		return nil
	}
	var matched *clihelp.Command
	for i := range cmds {
		if cmds[i].NoAbbrev {
			continue
		}
		for _, name := range append([]string{cmds[i].Name}, cmds[i].Aliases...) {
			if !strings.HasPrefix(name, word) {
				continue
			}
			if matched != nil && matched != &cmds[i] {
				return nil
			}
			matched = &cmds[i]
		}
	}
	return matched
}
