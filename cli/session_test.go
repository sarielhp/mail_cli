package cli

import (
	"testing"

	"github.com/sarielhp/mail_cli/app"
	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/mailclient"
)

func TestFormatAccountLabel(t *testing.T) {
	session := &Session{
		Config: &cfg_g.Config{
			Accounts: []cfg_acc.AccountConfig{
				{Name: "acc1", Type: "gmail"},
				{Name: "acc2", Type: "jmap"},
			},
		},
	}

	mockClient1 := &mailclient.MockMailClient{
		Cfg: &cfg_g.Config{
			SelectedAccount: "acc1",
		},
	}
	mockClient2 := &mailclient.MockMailClient{
		Cfg: &cfg_g.Config{
			SelectedAccount: "acc2",
		},
	}

	out1 := FormatAccountLabel(session, mockClient1, "sort/mailing_list/wuna")
	if out1 != "acc1:sort/mailing_list/wuna" {
		t.Errorf("got %q, want %q", out1, "acc1:sort/mailing_list/wuna")
	}

	out2 := FormatAccountLabel(session, mockClient2, "keep/2026/01/wuna-2026-01")
	if out2 != "acc2:keep/2026/01/wuna-2026-01" {
		t.Errorf("got %q, want %q", out2, "acc2:keep/2026/01/wuna-2026-01")
	}

	mockClientDefault := &mailclient.MockMailClient{
		Cfg: &cfg_g.Config{
			SelectedAccount: "default",
		},
	}
	outDefault := FormatAccountLabel(session, mockClientDefault, "INBOX")
	if outDefault != "acc1:INBOX" {
		t.Errorf("got %q, want %q", outDefault, "acc1:INBOX")
	}

	sessionWithDisplayName := &Session{
		Config: &cfg_g.Config{
			Accounts: []cfg_acc.AccountConfig{
				{Name: "default", DisplayName: "GMail", Type: "gmail"},
			},
		},
	}
	outGMail := FormatAccountLabel(sessionWithDisplayName, mockClientDefault, "INBOX")
	if outGMail != "GMail:INBOX" {
		t.Errorf("got %q, want %q", outGMail, "GMail:INBOX")
	}
}

func TestFormatAccountFolder(t *testing.T) {
	session := &Session{
		Config: &cfg_g.Config{
			Accounts: []cfg_acc.AccountConfig{
				{Name: "default", DisplayName: "GMail", Type: "gmail"},
				{Name: "fastmail", DisplayName: "FastMail", Type: "jmap"},
			},
		},
	}
	client := &mailclient.MockMailClient{
		Cfg: &cfg_g.Config{
			SelectedAccount: "fastmail",
		},
	}
	out := FormatAccountFolder(session, client, "services")
	want := "FastMail:" + app.ColorBlue.Sprint("services")
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}
