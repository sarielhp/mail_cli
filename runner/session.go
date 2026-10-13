package runner

import (
	gcal "github.com/sarielhp/mail_cli/calendar/google"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/mailops"
	"github.com/sarielhp/mail_cli/report"
	"github.com/sarielhp/mail_cli/tui"
	"net/http"
)

// GoogleCalendar builds the Google Calendar service, injecting the Gmail
// backend's OAuth client provider. Gmail and Calendar share one token, and this
// is the composition root -- calendar/google knows nothing about the mail backend,
// and the provider itself is named behind the factory.
func GoogleCalendar(rep report.Reporter) *gcal.Service {
	return gcal.New(func(config *cfg_g.Config) (*http.Client, error) {
		return mailops.NewGoogleOAuthClient(rep, config)
	}, rep)
}

// NewTuiBackend binds the TUI's declared dependencies to their implementations.
//
// rep is the TUI's own reporter, so backend progress arrives as bubbletea
// messages rather than being written over the display.
func NewTuiBackend(_ *cfg_g.Config, rep report.Reporter) *tui.Backend {
	return &tui.Backend{
		Clients:  tuiClientService{ClientService: ClientService{Rep: rep}},
		Folders:  OrganizeService{},
		Calendar: CalendarService{Rep: rep},
	}
}
