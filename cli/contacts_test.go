package cli

import (
	"bytes"
	"testing"

	"github.com/sarielhp/mail_cli/cfg_g"
)

// contactsCall records how a stubbed contacts op was invoked.
type contactsCall struct {
	ran bool
	arg string
}

// fn is the stub hook for the contacts ops that take one string argument.
func (c *contactsCall) fn(_ *cfg_g.Config, arg string) error {
	c.ran = true
	c.arg = arg
	return nil
}

// noArgFn is the stub hook for the contacts ops that take no argument.
func (c *contactsCall) noArgFn(_ *cfg_g.Config) error {
	c.ran = true
	return nil
}

// runContacts drives one contacts subcommand with a stubbed Contacts op, then
// checks the op ran and, when want is non-empty, recorded it as its argument.
func runContacts(t *testing.T, args []string, op ContactOps, call *contactsCall, want string) {
	t.Helper()
	session := &Session{
		Config: &cfg_g.Config{},
		Deps:   Deps{Contacts: op},
	}
	app := InitCLI(session)
	app.Stdout = &bytes.Buffer{}
	app.Stderr = &bytes.Buffer{}

	if err := app.Execute(args); err != nil {
		t.Fatalf("contacts %s failed: %v", args[1], err)
	}
	if !call.ran {
		t.Errorf("expected contacts %s to be called", args[1])
	}
	if want != "" && call.arg != want {
		t.Errorf("expected contacts %s argument %q, got %q", args[1], want, call.arg)
	}
}

func TestContacts_List(t *testing.T) {
	var call contactsCall
	runContacts(t, []string{"contacts", "list", "alice"}, stubContacts{ListFn: call.fn}, &call, "alice")
}

func TestContacts_Search(t *testing.T) {
	var call contactsCall
	runContacts(t, []string{"contacts", "search", "dave mount"}, stubContacts{SearchFn: call.fn}, &call, "dave mount")
}

func TestContacts_ImportMutt(t *testing.T) {
	var call contactsCall
	runContacts(t, []string{"contacts", "import-mutt", "/tmp/aliases"}, stubContacts{ImportMuttFn: call.fn}, &call, "/tmp/aliases")
}

func TestContacts_Sync(t *testing.T) {
	var call contactsCall
	runContacts(t, []string{"contacts", "sync", "work"}, stubContacts{SyncGoogleFn: call.fn}, &call, "work")
}

func TestContacts_Path(t *testing.T) {
	var call contactsCall
	runContacts(t, []string{"contacts", "path"}, stubContacts{PathFn: call.noArgFn}, &call, "")
}

func TestContacts_Suggest(t *testing.T) {
	var call contactsCall
	runContacts(t, []string{"contacts", "suggest", "alice"}, stubContacts{SuggestFn: call.fn}, &call, "alice")
}

func TestContacts_Harvest(t *testing.T) {
	var call contactsCall
	runContacts(t, []string{"contacts", "harvest"}, stubContacts{HarvestFn: call.noArgFn}, &call, "")
}
