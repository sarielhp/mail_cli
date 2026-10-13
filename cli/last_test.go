package cli

import (
	"testing"

	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/last"
)

func TestLastCmd_Validation(t *testing.T) {
	called := false
	var calledN int

	session := &Session{
		Config: &cfg_g.Config{},
		Deps: Deps{
			Mail: stubMail{
				LastFn: func(_ last.Options, _ *cfg_g.Config, n int) error {
					called = true
					calledN = n
					return nil
				},
			},
		},
	}

	cliApp := InitCLI(session)

	// Test valid integer
	err := cliApp.Execute([]string{"msg", "last", "15"})
	if err != nil {
		t.Fatalf("expected no error for 'last 15', got %v", err)
	}
	if !called || calledN != 15 {
		t.Errorf("expected RunLast to be called with 15, called=%v, calledN=%d", called, calledN)
	}

	// Test invalid non-integer argument
	err = cliApp.Execute([]string{"msg", "last", "abc"})
	if err == nil {
		t.Errorf("expected error for 'last abc', got nil")
	}

	// Test invalid 0 or negative argument
	err = cliApp.Execute([]string{"msg", "last", "0"})
	if err == nil {
		t.Errorf("expected error for 'last 0', got nil")
	}
}

func TestLastCmd_JSONFlag(t *testing.T) {
	var gotOpts last.Options
	session := &Session{
		Config: &cfg_g.Config{},
		Deps: Deps{
			Mail: stubMail{
				LastFn: func(opts last.Options, _ *cfg_g.Config, _ int) error {
					gotOpts = opts
					return nil
				},
			},
		},
	}
	cliApp := InitCLI(session)
	err := cliApp.Execute([]string{"msg", "last", "--json", "5"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !gotOpts.JSON {
		t.Errorf("expected gotOpts.JSON to be true")
	}
}
