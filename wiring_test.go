package main

import (
	"testing"

	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"
)

// A dropped binding would leave the command that needs it failing at runtime
// with ErrNotConfigured, so assert the composition root fills every slot.
func TestBuildSessionWiresEveryDependency(t *testing.T) {
	cfg := &cfg_g.Config{
		Accounts: []cfg_acc.AccountConfig{
			{Name: "test", Type: "gmail"},
		},
	}

	session := buildSession(cfg)
	if session == nil || session.Config != cfg {
		t.Fatalf("expected a session carrying the config it was built from")
	}

	deps := session.Deps
	for name, bound := range map[string]bool{
		"Clients":  deps.Clients != nil,
		"Mail":     deps.Mail != nil,
		"Spam":     deps.Spam != nil,
		"Rules":    deps.Rules != nil,
		"Config":   deps.Config != nil,
		"Organize": deps.Organize != nil,
		"Calendar": deps.Calendar != nil,
		"Accounts": deps.Accounts != nil,
		"TUI":      deps.TUI != nil,
	} {
		if !bound {
			t.Errorf("dependency %s was not wired", name)
		}
	}
}
