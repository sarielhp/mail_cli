package tui

import (
	"errors"
	"strings"
	"testing"

	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/mailclient"
	"github.com/sarielhp/mail_cli/tui/render"

	tea "github.com/charmbracelet/bubbletea"
)

func newTestMultiAccountModel(accounts []cfg_acc.AccountConfig, selected string) *tuiModel {
	cfg := &cfg_g.Config{
		Accounts:        accounts,
		SelectedAccount: cfg_g.AccountName(selected),
	}
	m := newTuiModel(&mailclient.MockMailClient{Cfg: cfg}, "", nil)
	m.cfg = cfg
	m.width = 80
	m.height = 24
	m.theme = render.NewThemeManager()
	return m
}

func TestAccountSwitch_GetAccountsAndCurrentIndex(t *testing.T) {
	accs := []cfg_acc.AccountConfig{
		{Name: "personal", DisplayName: "Personal Gmail", Type: "gmail", Username: "user1@gmail.com"},
		{Name: "work", DisplayName: "Work Fastmail", Type: "jmap", Username: "user2@fastmail.com"},
	}
	m := newTestMultiAccountModel(accs, "work")

	got := m.getAccounts()
	if len(got) != 2 {
		t.Fatalf("expected 2 accounts, got %d", len(got))
	}

	idx := m.currentAccountIndex()
	if idx != 1 {
		t.Fatalf("expected current account index 1 for 'work', got %d", idx)
	}

	m.Config().SelectedAccount = "personal"
	idx = m.currentAccountIndex()
	if idx != 0 {
		t.Fatalf("expected current account index 0 for 'personal', got %d", idx)
	}
}

func TestAccountSwitch_OpenCloseOverlay(t *testing.T) {
	accs := []cfg_acc.AccountConfig{
		{Name: "acc1", DisplayName: "Account 1", Type: "gmail"},
		{Name: "acc2", DisplayName: "Account 2", Type: "jmap"},
	}
	m := newTestMultiAccountModel(accs, "acc1")

	// Pressing 'a' in index mode should open account overlay
	m.key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	if !m.accountOverlayOpen {
		t.Error("expected accountOverlayOpen to be true after pressing 'a'")
	}
	if m.accountCursor != 0 {
		t.Errorf("expected accountCursor 0, got %d", m.accountCursor)
	}

	// Pressing 'Esc' should close account overlay
	m.key(tea.KeyMsg{Type: tea.KeyEsc})
	if m.accountOverlayOpen {
		t.Error("expected accountOverlayOpen to be false after pressing Esc")
	}

	// F3 is the one key that opens the picker
	m.key(tea.KeyMsg{Type: tea.KeyF3})
	if !m.accountOverlayOpen {
		t.Error("expected accountOverlayOpen to be true after pressing F3")
	}

	// Pressing 'q' should close overlay
	m.key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if m.accountOverlayOpen {
		t.Error("expected accountOverlayOpen to be false after pressing 'q'")
	}
}

func TestAccountSwitch_OverlayNavigationAndSelection(t *testing.T) {
	accs := []cfg_acc.AccountConfig{
		{Name: "acc1", DisplayName: "Account 1", Type: "gmail"},
		{Name: "acc2", DisplayName: "Account 2", Type: "jmap"},
		{Name: "acc3", DisplayName: "Account 3", Type: "outlook"},
	}
	m := newTestMultiAccountModel(accs, "acc1")
	m.openAccountOverlay()

	// Navigate down with 'j'
	m.key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if m.accountCursor != 1 {
		t.Errorf("expected cursor 1 after 'j', got %d", m.accountCursor)
	}

	// Navigate down with 'down' arrow
	m.key(tea.KeyMsg{Type: tea.KeyDown})
	if m.accountCursor != 2 {
		t.Errorf("expected cursor 2 after Down arrow, got %d", m.accountCursor)
	}

	// Wrap around down
	m.key(tea.KeyMsg{Type: tea.KeyDown})
	if m.accountCursor != 0 {
		t.Errorf("expected cursor 0 after wrap-around down, got %d", m.accountCursor)
	}

	// Navigate up with 'k'
	m.key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if m.accountCursor != 2 {
		t.Errorf("expected cursor 2 after 'k' wrap-around up, got %d", m.accountCursor)
	}

	// Select with Enter
	m.key(tea.KeyMsg{Type: tea.KeyEnter})
	if m.accountOverlayOpen {
		t.Error("expected account overlay to close after selection")
	}
	if m.Config().SelectedAccount != "acc3" {
		t.Errorf("expected SelectedAccount 'acc3', got %q", m.Config().SelectedAccount)
	}
}

// Digits used to jump straight to an account from the index and detail
// views, with no dialog; only the picker's own digits do that now.
func TestAccountSwitch_DigitsOutsidePickerDoNothing(t *testing.T) {
	accs := []cfg_acc.AccountConfig{
		{Name: "acc1", DisplayName: "Account 1", Type: "gmail"},
		{Name: "acc2", DisplayName: "Account 2", Type: "jmap"},
	}
	for _, mode := range []int{modeIndex, modeDetail} {
		m := newTestMultiAccountModel(accs, "acc1")
		m.mode = mode
		m.key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
		if m.Config().SelectedAccount != "acc1" || m.accountOverlayOpen {
			t.Errorf("mode %d: '2' switched to %q or opened the picker", mode, m.Config().SelectedAccount)
		}
	}
}

// Accounts are chosen only through the F3 picker. The old aliases -- A for
// the picker, [ and ] to cycle -- switched or opened things on a stray key
// press; they must now do nothing.
func TestAccountSwitch_NoAliasesOutsidePicker(t *testing.T) {
	accs := []cfg_acc.AccountConfig{
		{Name: "acc1", DisplayName: "Account 1", Type: "gmail"},
		{Name: "acc2", DisplayName: "Account 2", Type: "jmap"},
		{Name: "acc3", DisplayName: "Account 3", Type: "outlook"},
	}
	for _, mode := range []int{modeIndex, modeDetail} {
		m := newTestMultiAccountModel(accs, "acc1")
		m.mode = mode
		for _, r := range []rune{']', '[', 'A'} {
			m.key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
			if m.Config().SelectedAccount != "acc1" || m.accountOverlayOpen {
				t.Fatalf("mode %v: %q switched to %q or opened the picker (%v)", mode, r, m.Config().SelectedAccount, m.accountOverlayOpen)
			}
		}
	}
}

func TestAccountSwitch_MenuOption(t *testing.T) {
	accs := []cfg_acc.AccountConfig{
		{Name: "acc1", DisplayName: "Account 1", Type: "gmail"},
		{Name: "acc2", DisplayName: "Account 2", Type: "jmap"},
	}
	m := newTestMultiAccountModel(accs, "acc1")

	// Open menu with 'm'
	m.key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	if !m.menuOpen {
		t.Fatal("expected menuOpen to be true")
	}

	// Move cursor down to "Switch Account"
	m.key(tea.KeyMsg{Type: tea.KeyDown})
	if m.menuItems[m.menuCursor] != "Switch Account" {
		t.Fatalf("expected 'Switch Account' at cursor 1, got %q", m.menuItems[m.menuCursor])
	}

	// Press Enter to open account overlay
	m.key(tea.KeyMsg{Type: tea.KeyEnter})
	if !m.accountOverlayOpen {
		t.Error("expected accountOverlayOpen to be true after selecting 'Switch Account' from menu")
	}
	if m.menuOpen {
		t.Error("expected menuOpen to be false after selection")
	}
}

func TestAccountSwitch_RenderOverlay(t *testing.T) {
	accs := []cfg_acc.AccountConfig{
		{Name: "personal", DisplayName: "Personal", Type: "gmail", Username: "me@gmail.com"},
		{Name: "work", DisplayName: "Work", Type: "jmap", Username: "me@work.com"},
	}
	m := newTestMultiAccountModel(accs, "personal")
	m.accountOverlayOpen = true

	view := m.View()
	if !strings.Contains(view, "SWITCH ACCOUNT") {
		t.Error("expected view to contain 'SWITCH ACCOUNT'")
	}
	if !strings.Contains(view, "Personal") {
		t.Error("expected view to contain 'Personal'")
	}
	if !strings.Contains(view, "Work") {
		t.Error("expected view to contain 'Work'")
	}
	if strings.Contains(view, "● Active") {
		t.Error("expected view not to contain '● Active'")
	}
	if !strings.Contains(view, "●=active") {
		t.Error("expected view to contain '●=active'")
	}
	if !strings.Contains(view, "[1]") || !strings.Contains(view, "●") {
		t.Error("expected view to contain '[1]' and '●' for active account")
	}
}

func TestAccountSwitch_F3KeyOpensOverlay(t *testing.T) {
	accs := []cfg_acc.AccountConfig{
		{Name: "personal", DisplayName: "Personal", Type: "gmail"},
		{Name: "work", DisplayName: "Work", Type: "jmap"},
	}
	m := newTestMultiAccountModel(accs, "personal")
	m.accountOverlayOpen = false

	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyF3})
	m = res.(*tuiModel)
	if !m.accountOverlayOpen {
		t.Error("expected accountOverlayOpen=true after F3 key")
	}
}

func TestAccountSwitch_RenderOverlay_ContainsLoginHint(t *testing.T) {
	accs := []cfg_acc.AccountConfig{
		{Name: "work", DisplayName: "Work", Type: "outlook"},
	}
	m := newTestMultiAccountModel(accs, "work")
	m.accountOverlayOpen = true

	view := m.View()
	if !strings.Contains(view, "l: Login") {
		t.Errorf("expected overlay to contain 'l: Login', got: %s", view)
	}
}

func TestAccountSwitch_OverlayLoginKeyTriggersCmd(t *testing.T) {
	accs := []cfg_acc.AccountConfig{
		{Name: "work", DisplayName: "Work", Type: "outlook"},
	}
	m := newTestMultiAccountModel(accs, "work")
	m.accountOverlayOpen = true

	res, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	_ = res.(*tuiModel)
	if cmd == nil {
		t.Fatal("expected non-nil tea.Cmd for 'l' key in account overlay")
	}
}

func TestAccountSwitch_AccountLoginFinishedMsg(t *testing.T) {
	accs := []cfg_acc.AccountConfig{
		{Name: "personal", DisplayName: "Personal", Type: "gmail"},
		{Name: "work", DisplayName: "Work", Type: "outlook"},
	}
	m := newTestMultiAccountModel(accs, "personal")
	m.accountOverlayOpen = true

	// Failure case
	failMsg := accountLoginFinishedMsg{
		acc: accs[1],
		err: errors.New("login failed"),
	}
	res, _ := m.Update(failMsg)
	m = res.(*tuiModel)
	if !m.showError || m.accountOverlayOpen {
		t.Errorf("expected showError=true, accountOverlayOpen=false on login error; got showError=%v overlay=%v", m.showError, m.accountOverlayOpen)
	}

	// Success case
	m.showError = false
	m.err = nil
	m.accountOverlayOpen = true
	successMsg := accountLoginFinishedMsg{
		acc: accs[1],
		err: nil,
	}
	res, _ = m.Update(successMsg)
	m = res.(*tuiModel)
	if m.accountOverlayOpen {
		t.Error("expected accountOverlayOpen=false on login success")
	}
	if m.Config().SelectedAccount != "work" {
		t.Errorf("expected selected account 'work', got %q", m.Config().SelectedAccount)
	}
}

func TestHandleErrorKey_InteractiveAuthKeys(t *testing.T) {
	accs := []cfg_acc.AccountConfig{
		{Name: "work", DisplayName: "Work", Type: "outlook"},
	}
	m := newTestMultiAccountModel(accs, "work")

	// Triggering 'a' when auth error is shown opens account overlay
	m.showError = true
	m.err = mailclient.ErrInteractiveAuthRequired
	res, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	m = res.(*tuiModel)
	if !m.accountOverlayOpen {
		t.Error("expected accountOverlayOpen=true after pressing 'a' on auth error")
	}
	if m.showError {
		t.Error("expected showError=false after pressing 'a'")
	}
	if cmd != nil {
		t.Error("expected nil cmd for 'a'")
	}

	// Triggering 'l' when auth error is shown initiates login
	m.accountOverlayOpen = false
	m.showError = true
	m.err = mailclient.ErrInteractiveAuthRequired
	res, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	m = res.(*tuiModel)
	if m.showError {
		t.Error("expected showError=false after pressing 'l'")
	}
	if cmd == nil {
		t.Fatal("expected non-nil tea.Cmd for 'l' on auth error")
	}
}
