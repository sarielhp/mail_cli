package cli

import (
	"strings"
	"testing"

	"github.com/sarielhp/clihelp"
)

// Words that mean one thing, everywhere.
//
// --force is reserved for "overwrite a conflicting remote object" and may never
// mean "yes, delete it": a user who learns that --force answers one prompt will
// reach for it at the next one, which may be the irreversible prompt. --yes is
// the only word for that.
//
// --do is not in the tree. Reversible work -- scan applying the routing rules
// you wrote -- does not ask permission, because a confirmation you always type
// is not a confirmation.
var reservedFlagMeanings = map[string]string{
	"--yes":   "confirms an irreversible action, and nothing else",
	"--force": "overwrites a conflicting remote object, and never means --yes",
}

func walkCommands(cmds []clihelp.Command, visit func(path string, c clihelp.Command)) {
	for _, c := range cmds {
		visit(c.Name, c)
		walkCommands(c.Subcommands, func(sub string, s clihelp.Command) {
			visit(c.Name+" "+sub, s)
		})
	}
}

// Every flag spelling in the tree, so a new command cannot quietly invent a
// third word for "I mean it".
func TestFlagSpellingsAreConsistent(t *testing.T) {
	app := InitCLI(&Session{})

	synonyms := map[string]string{
		"--confirm": "--yes", "--assume-yes": "--yes", "-f": "--force",
		"--really": "--yes", "--now": "--yes",
	}

	walkCommands(app.Commands, func(path string, c clihelp.Command) {
		for _, opt := range c.Options {
			for _, name := range strings.Split(opt.Flags, ",") {
				name = strings.TrimSpace(name)
				name = strings.SplitN(name, " ", 2)[0]
				if want, bad := synonyms[name]; bad && name != "-f" {
					t.Errorf("%s declares %q; use %q, which is the only word for it",
						path, name, want)
				}
			}
		}
	})
}

// The irreversible commands must all be reachable only through --yes.
func TestIrreversibleCommandsOfferYes(t *testing.T) {
	app := InitCLI(&Session{})

	irreversible := map[string]bool{
		"spam empty":     false,
		"cache clear":    false,
		"rule rm-all":    false,
		"account rm":     false,
		"label rm":       false,
		"rule remote rm": false,
	}

	walkCommands(app.Commands, func(path string, c clihelp.Command) {
		if _, watched := irreversible[path]; !watched {
			return
		}
		for _, opt := range c.Options {
			if strings.Contains(opt.Flags, "--yes") {
				irreversible[path] = true
			}
		}
	})

	for path, found := range irreversible {
		if !found {
			t.Errorf("%s deletes something that cannot be recovered and offers no --yes", path)
		}
	}
}

// --force may not be spelled on a command whose only destructive act is a
// deletion; that is --yes's job.
func TestForceIsNotAConfirmation(t *testing.T) {
	app := InitCLI(&Session{})

	walkCommands(app.Commands, func(path string, c clihelp.Command) {
		hasForce, hasYes := false, false
		for _, opt := range c.Options {
			if strings.Contains(opt.Flags, "--force") {
				hasForce = true
			}
			if strings.Contains(opt.Flags, "--yes") {
				hasYes = true
			}
		}
		if hasForce && hasYes {
			t.Errorf("%s offers both --force and --yes; %s", path, reservedFlagMeanings["--force"])
		}
	})
}

// Persistent option descriptions must stay at or below 45 characters to avoid
// line wrapping and preserve two-column alignment in root help.
func TestPersistentOptionsDescriptionLength(t *testing.T) {
	for _, opt := range cliPersistentOptions() {
		if len(opt.Description) > 45 {
			t.Errorf("persistent option %q description exceeds 45 chars (%d): %q",
				opt.Flags, len(opt.Description), opt.Description)
		}
	}
}
