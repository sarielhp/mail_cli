package cli

import (
	"testing"

	"github.com/sarielhp/mail_cli/cfg_g"
)

func TestCalendarCmd_Structure(t *testing.T) {
	session := &Session{Config: &cfg_g.Config{}}
	cmd := CalendarCmd(session)
	if cmd.Name != "calendar" {
		t.Errorf("expected name 'calendar', got %q", cmd.Name)
	}

	expectedSubcmds := map[string]bool{"add": false, "week": false, "add-all": false}
	for _, sub := range cmd.Subcommands {
		if _, ok := expectedSubcmds[sub.Name]; ok {
			expectedSubcmds[sub.Name] = true
		}
	}
	for name, found := range expectedSubcmds {
		if !found {
			t.Errorf("expected subcommand %q in CalendarCmd, not found", name)
		}
	}
}
