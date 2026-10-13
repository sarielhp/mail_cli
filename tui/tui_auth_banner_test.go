package tui

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/sarielhp/mail_cli/cache/cachedir"
	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/mailclient"
)

func authBannerModel(t *testing.T, err error) *tuiModel {
	t.Helper()
	cfg := &cfg_g.Config{
		Paths:           cfg_g.Paths{CacheRoot: t.TempDir()},
		AccountCache:    cachedir.At(t.TempDir()),
		Accounts:        []cfg_acc.AccountConfig{{Name: "work", Type: "outlook"}},
		SelectedAccount: "work",
	}
	m := newTuiModel(&mailclient.MockMailClient{Cfg: cfg}, "INBOX", nil)
	m.width, m.height = 160, 30
	m.err, m.showError = err, true
	return m
}

// An expired sign-in used to show the generic banner -- "Press R to retry" --
// so the 'L' that signs in from inside the TUI was never discovered.
func TestErrorBannerOffersSignInForAuthErrors(t *testing.T) {
	authErr := fmt.Errorf("fetch: %w: run `mail_cli account login work` from a terminal",
		mailclient.ErrInteractiveAuthRequired)
	banner := renderErrorBanner(authBannerModel(t, authErr))
	if !strings.Contains(banner, "Press L to sign in") || !strings.Contains(banner, "work") {
		t.Errorf("auth banner does not offer sign-in for the account: %q", banner)
	}

	plain := renderErrorBanner(authBannerModel(t, errors.New("connection refused")))
	if strings.Contains(plain, "Press L") || !strings.Contains(plain, "Press R to retry") {
		t.Errorf("an ordinary error got the sign-in banner: %q", plain)
	}
}

// 'L' on the sign-in banner runs `account login` for the current account; on
// any other error it is not consumed as a login.
func TestLOnAuthBannerStartsLogin(t *testing.T) {
	m := authBannerModel(t, mailclient.ErrInteractiveAuthRequired)
	_, cmd, handled := m.handleAuthErrorLogin("L")
	if !handled || cmd == nil || m.showError {
		t.Fatalf("L on an auth error: handled=%v cmd=%v showError=%v", handled, cmd != nil, m.showError)
	}

	other := authBannerModel(t, errors.New("connection refused"))
	if _, _, handled := other.handleAuthErrorLogin("L"); handled {
		t.Error("L on an ordinary error was taken as a login")
	}
}
