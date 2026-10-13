package gmail

import (
	"strings"
	"testing"

	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/report"
)

// doctor is the one diagnostic that writes: it creates a label, inserts a
// message into INBOX, moves it, and deletes both. It builds its own service
// from a *Config, so the ReadOnlyMailClient decorator that guards every other
// mutation never sees it -- `mail_cli --read-only doctor` created a label and
// inserted a message into a mailbox whose owner had just asked for nothing to
// change.
//
// The refusal must come before GetGmailService, so a machine with no
// credentials at all still reports the right reason.
func TestDoctorRefusesToMutateUnderReadOnlyAndDryRun(t *testing.T) {
	tests := []struct {
		name     string
		config   *cfg_g.Config
		wantFlag string
	}{
		{name: "read-only", config: &cfg_g.Config{ReadOnly: true}, wantFlag: "--read-only"},
		{name: "dry-run", config: &cfg_g.Config{DryRun: true}, wantFlag: "--dry-run"},
		{
			name:     "dry-run implies read-only, and is the name reported",
			config:   &cfg_g.Config{ReadOnly: true, DryRun: true},
			wantFlag: "--dry-run",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := RunRESTTests(report.Discard(), tc.config)
			if err == nil {
				t.Fatal("the diagnostic ran; it mutates the account and must be refused")
			}
			if !strings.Contains(err.Error(), tc.wantFlag) {
				t.Errorf("the refusal should name %s, got %v", tc.wantFlag, err)
			}
			if !strings.Contains(err.Error(), "mutates") {
				t.Errorf("the refusal should say why, got %v", err)
			}
		})
	}
}
