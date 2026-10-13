package outlook

import (
	"testing"

	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/report"
)

// The TUI's compose account switch builds a brand-new client and hands
// it straight to the background draft sync, which asks it for the Drafts
// folder through GetMatchingLabels before anything has called Validate. The
// Graph HTTP client is built lazily by init, so a method that reaches Graph
// without running init first dereferences a nil *http.Client -- which is how
// pressing 'f' on the compose screen panicked. With no token on disk and no
// terminal to sign in from, the call must come back with an error instead.
func TestGetMatchingLabelsOnFreshClientDoesNotPanic(t *testing.T) {
	restore := interactiveAuthPossible
	interactiveAuthPossible = func() bool { return false }
	defer func() { interactiveAuthPossible = restore }()

	acc := cfg_acc.AccountConfig{Name: "work", Type: "outlook", Username: "w@example.com"}
	config := &cfg_g.Config{
		SelectedAccount: "work",
		Paths:           cfg_g.Paths{ConfigDir: t.TempDir()},
	}
	client := NewClient(acc, config, report.Discard())

	labels, err := client.GetMatchingLabels("")
	if err == nil {
		t.Fatalf("got labels %v with no credentials; want an error", labels)
	}
}
