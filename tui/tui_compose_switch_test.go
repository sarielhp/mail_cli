package tui

import (
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/mailclient"
)

// Switching the compose account must build the next client from the root
// config, not from the current client's per-account copy. When the root and
// an account's directory shared one field, deriving the next client from that
// copy filed the new account's cache under the old account's directory.
func TestComposeSwitchAccount_UsesRootConfigForNextClient(t *testing.T) {
	m, mockClient, tempDir := setupTestComposeModel(t, false)

	perAccount := *m.cfg
	perAccount.CacheRoot = filepath.Join(tempDir, "Personal")
	perAccount.AccountCache = cachedir.At(perAccount.CacheRoot)
	mockClient.Cfg = &perAccount

	var gotCfg *cfg_g.Config
	var gotAcc cfg_acc.AccountConfig
	m.bk.Clients = stubClients{
		ForAccountFn: func(c *cfg_g.Config, acc cfg_acc.AccountConfig) (mailclient.MailClient, error) {
			gotCfg, gotAcc = c, acc
			return &mailclient.MockMailClient{Cfg: c, BackendTypeVal: acc.Type}, nil
		},
	}
	m.compose = &composeState{AccountIndex: 0, AccountName: "Personal", To: "x@example.com", Subject: "s", Body: "b"}

	m.switchComposeAccountTo(1)

	if gotAcc.Name != "Work" {
		t.Fatalf("switched to %q, want the next account Work", gotAcc.Name)
	}
	if gotCfg == nil || gotCfg.CacheRoot != tempDir {
		t.Fatalf("client built from cache root %q, want the root %q", gotCfg.CacheRoot, tempDir)
	}
	if m.compose.AccountName != "Work" || m.client.BackendType() != "jmap" {
		t.Fatalf("compose state %q / client %q not switched", m.compose.AccountName, m.client.BackendType())
	}
}

// The compose screen is a modal of its own, so the picker has to be drawn
// ahead of it or F3 would open an overlay nobody can see.
func TestComposeSwitchAccount_PickerRendersOverComposeScreen(t *testing.T) {
	m, _, _ := setupTestComposeModel(t, false)
	m.mode = modeCompose
	m.compose = &composeState{AccountIndex: 1, AccountName: "Work", To: "x@example.com", Subject: "s", Body: "b"}

	m.openAccountOverlay()

	view := m.View()
	if !strings.Contains(view, "SEND FROM ACCOUNT") {
		t.Fatalf("picker not drawn over the compose screen:\n%s", view)
	}
	if m.accountCursor != 1 {
		t.Fatalf("picker cursor %d, want the sending account 1", m.accountCursor)
	}
}
