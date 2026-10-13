package runner

import (
	"io"
	"testing"

	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/mailclient"
	"github.com/sarielhp/mail_cli/report"
	"github.com/sarielhp/mail_cli/show"
)

func TestNewTuiBackend(t *testing.T) {
	cfg := &cfg_g.Config{
		Accounts: []cfg_acc.AccountConfig{
			{Name: "test", Type: "gmail"},
		},
	}

	backend := NewTuiBackend(cfg, report.Discard())
	if backend == nil || backend.Clients == nil || backend.Folders == nil || backend.Calendar == nil {
		t.Errorf("expected backend delegates to be non-nil")
	}
}

// The label sigil removed the need to guess: an argument without it is a
// message ID by construction, so showByMessageID tries the fetch rather than
// inspecting the argument's shape first.
func TestShowByMessageIDDoesNotInspectTheArgument(t *testing.T) {
	mock := &mailclient.MockMailClient{
		Cfg:         &cfg_g.Config{},
		RawMessages: map[email.ID][]byte{"short": []byte("From: a@b.c\r\nSubject: s\r\n\r\nbody\r\n")},
	}

	// "short" is five characters -- the old heuristic required twelve and would
	// never have tried it.
	if !showByMessageID(show.Options{}, io.Discard, mock, "short") {
		t.Error("expected a retrievable ID to be shown regardless of its shape")
	}
	if showByMessageID(show.Options{}, io.Discard, mock, "absent") {
		t.Error("expected an unretrievable ID to report failure")
	}
}
