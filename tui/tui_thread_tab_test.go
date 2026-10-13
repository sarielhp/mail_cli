package tui

import (
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"strings"
	"testing"
	"time"

	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/mailclient"
	"github.com/sarielhp/mail_cli/tui/render"

	tea "github.com/charmbracelet/bubbletea"
)

func TestThread_TabCycleExpansion(t *testing.T) {
	baseTime := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	e0 := email.Email{ID: "e0", Subject: "Thread Root", MessageID: "<e0>", FromRaw: "Alice <a@b.com>", EmailDate: baseTime}
	e1 := email.Email{ID: "e1", Subject: "Re: Thread Root", MessageID: "<e1>", InReplyTo: "<e0>", FromRaw: "Bob <b@b.com>", EmailDate: baseTime.Add(time.Minute)}
	e2 := email.Email{ID: "e2", Subject: "Re: Thread Root", MessageID: "<e2>", InReplyTo: "<e1>", FromRaw: "Charlie <c@b.com>", EmailDate: baseTime.Add(2 * time.Minute)}
	e3 := email.Email{ID: "e3", Subject: "Other email", MessageID: "<e3>", FromRaw: "Dave <d@b.com>", EmailDate: baseTime.Add(3 * time.Minute)}

	m := newTuiModel(nil, "INBOX", nil)
	m.rawEmails = []email.Email{e0, e1, e2, e3}
	m.rebuildVisibleEmails()

	if len(m.emails) != 2 {
		t.Fatalf("expected 2 visible emails initially, got %d", len(m.emails))
	}
	if m.emails[0].ID != "e0" || m.emails[1].ID != "e3" {
		t.Fatalf("expected e0 and e3 visible, got %s and %s", m.emails[0].ID, m.emails[1].ID)
	}
	if !m.emails[0].ThreadCollapsed {
		t.Error("expected e0 to start collapsed")
	}
	if m.emails[0].ThreadPrefix != "━━━ " {
		t.Errorf("expected e0 collapsed ThreadPrefix to be \x27━━━ \x27, got %q", m.emails[0].ThreadPrefix)
	}

	m.eIdx = 0

	// 1st Tab: expands Level 0 (e0). e1 becomes visible, but e2 remains hidden
	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = res.(*tuiModel)

	if len(m.emails) != 3 {
		t.Fatalf("expected 3 visible emails after 1st Tab, got %d", len(m.emails))
	}
	if m.emails[0].ID != "e0" || m.emails[1].ID != "e1" || m.emails[2].ID != "e3" {
		t.Fatalf("expected e0, e1, e3 visible, got %v", []string{string(m.emails[0].ID), string(m.emails[1].ID), string(m.emails[2].ID)})
	}
	if m.emails[0].ThreadPrefix != "┌── " {
		t.Errorf("expected e0 expanded ThreadPrefix to be \x27┌── \x27, got %q", m.emails[0].ThreadPrefix)
	}

	// 2nd Tab: expands Level 1 (e1). e2 becomes visible
	res, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = res.(*tuiModel)

	if len(m.emails) != 4 {
		t.Fatalf("expected 4 visible emails after 2nd Tab, got %d", len(m.emails))
	}
	if m.emails[0].ID != "e0" || m.emails[1].ID != "e1" || m.emails[2].ID != "e2" || m.emails[3].ID != "e3" {
		t.Fatalf("expected all emails visible, got %v", []string{string(m.emails[0].ID), string(m.emails[1].ID), string(m.emails[2].ID), string(m.emails[3].ID)})
	}

	// 3rd Tab: all descendants expanded -> folds the whole thread subtree
	res, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = res.(*tuiModel)

	if len(m.emails) != 2 {
		t.Fatalf("expected 2 visible emails after 3rd Tab (collapse), got %d", len(m.emails))
	}
	if m.emails[0].ID != "e0" || m.emails[1].ID != "e3" {
		t.Fatalf("expected e0 and e3 visible after collapse, got %s and %s", m.emails[0].ID, m.emails[1].ID)
	}
	if !m.emails[0].ThreadCollapsed {
		t.Error("expected e0 to be collapsed again after 3rd Tab")
	}
	if m.emails[0].ThreadPrefix != "━━━ " {
		t.Errorf("expected e0 collapsed ThreadPrefix to be \x27━━━ \x27, got %q", m.emails[0].ThreadPrefix)
	}
}

func TestMenu_AltKeyToggle(t *testing.T) {
	m := newTuiModel(nil, "INBOX", nil)
	if m.menuOpen {
		t.Fatal("expected menuOpen to be false initially")
	}

	// Press Alt key in index mode
	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{}, Alt: true})
	m = res.(*tuiModel)
	if !m.menuOpen {
		t.Error("expected menuOpen to be true after Alt key in index mode")
	}

	// Press Alt key again -> closes menu
	res, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{}, Alt: true})
	m = res.(*tuiModel)
	if m.menuOpen {
		t.Error("expected menuOpen to be false after second Alt key")
	}

	// Switch to detail mode and test Alt key
	m.mode = modeDetail
	res, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{}, Alt: true})
	m = res.(*tuiModel)
	if !m.menuOpen {
		t.Error("expected menuOpen to be true after Alt key in detail mode")
	}
}

func TestEmail_RepliedStatusIndicator(t *testing.T) {
	theme := render.NewDefaultTheme()
	e := email.Email{
		ID:            "msg1",
		Subject:       "Hello World",
		FromRaw:       "Sender <s@example.com>",
		FormattedDate: "12:00",
		IsRead:        true,
		IsReplied:     true,
	}

	row := render.EmailRow(e, 80, 40, 1, theme, false, false)
	rowStr := row.Plain()

	if !strings.Contains(rowStr, "r ") {
		t.Errorf("expected rendered row to contain 'r ' status flag, got %q", rowStr)
	}
}

func TestEmail_AttachmentAndUnreadIndicator(t *testing.T) {
	theme := render.NewDefaultTheme()
	e := email.Email{
		ID:            "msg2",
		Subject:       "Invoice Attached",
		FromRaw:       "Billing <billing@example.com>",
		FormattedDate: "12:00",
		IsRead:        false,
		HasAttachment: true,
	}

	row := render.EmailRow(e, 80, 40, 1, theme, false, false)
	rowStr := row.Plain()

	if !strings.Contains(rowStr, "📎") {
		t.Errorf("expected rendered row to contain '📎', got %q", rowStr)
	}

	eUnreadNoAtt := email.Email{
		ID:            "msg3",
		Subject:       "Invoice Unread No Attachment",
		FromRaw:       "Billing <billing@example.com>",
		FormattedDate: "12:05",
		IsRead:        false,
		HasAttachment: false,
	}
	row2 := render.EmailRow(eUnreadNoAtt, 80, 40, 1, theme, false, false)
	row2Str := row2.Plain()
	if !strings.Contains(row2Str, "N ") {
		t.Errorf("expected rendered row to contain 'N ', got %q", row2Str)
	}
}

func TestEmail_ThreadPrefixDropsRe(t *testing.T) {
	theme := render.NewDefaultTheme()
	e := email.Email{
		ID:            "msg4",
		Subject:       "Re: Project Update",
		FromRaw:       "Alice <alice@example.com>",
		FormattedDate: "12:10",
		IsRead:        true,
		ThreadPrefix:  "━━━ ",
	}

	row := render.EmailRow(e, 80, 40, 1, theme, false, false)
	rowStr := row.Plain()

	if !strings.Contains(rowStr, "━━━ Project Update") {
		t.Errorf("expected rendered row to drop 'Re: ' and have '━━━ Project Update', got %q", rowStr)
	}
}

func TestEmail_ICSAndAttachmentIndicators(t *testing.T) {
	theme := render.NewDefaultTheme()

	// ICS only -> clock icon, no background highlight
	eICS := email.Email{
		ID:            "msg_ics",
		Subject:       "Sam Research Meeting",
		FromRaw:       "Sam <sam@example.com>",
		FormattedDate: "14:00",
		IsRead:        true,
		HasICS:        true,
		HasAttachment: false,
	}
	rowICS := render.EmailRow(eICS, 80, 40, 1, theme, false, false)
	rowICSStr := rowICS.Plain()
	if !strings.Contains(rowICSStr, "⏰") {
		t.Errorf("expected row to contain '⏰', got %q", rowICSStr)
	}
	if strings.Contains(rowICSStr, "📎") {
		t.Errorf("expected row NOT to contain '📎' when only ICS is present, got %q", rowICSStr)
	}

	// ICS + other attachment -> both clock and paperclip
	eBoth := email.Email{
		ID:            "msg_both",
		Subject:       "Meeting with Slides",
		FromRaw:       "Sam <sam@example.com>",
		FormattedDate: "14:00",
		IsRead:        true,
		HasICS:        true,
		HasAttachment: true,
	}
	rowBoth := render.EmailRow(eBoth, 80, 40, 1, theme, false, false)
	rowBothStr := rowBoth.Plain()
	if !strings.Contains(rowBothStr, "⏰📎") {
		t.Errorf("expected row to contain both '⏰📎' adjacent, got %q", rowBothStr)
	}

	// Replied + ICS + attachment -> 'r' and '⏰📎'
	eRepliedBoth := email.Email{
		ID:            "msg_replied_both",
		Subject:       "Re: Meeting with Slides",
		FromRaw:       "Sam <sam@example.com>",
		FormattedDate: "14:00",
		IsRead:        true,
		IsReplied:     true,
		HasICS:        true,
		HasAttachment: true,
	}
	rowRepliedBoth := render.EmailRow(eRepliedBoth, 80, 40, 1, theme, false, false)
	rowRepliedBothStr := rowRepliedBoth.Plain()
	if !strings.Contains(rowRepliedBothStr, "r") || !strings.Contains(rowRepliedBothStr, "⏰📎") {
		t.Errorf("expected row to contain 'r' and '⏰📎', got %q", rowRepliedBothStr)
	}
}

func TestThread_AutoExpandOnArchive(t *testing.T) {
	baseTime := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	e0 := email.Email{ID: "e0", Subject: "Thread Root", MessageID: "<e0>", EmailDate: baseTime}
	e1 := email.Email{ID: "e1", Subject: "Re: Thread Root", MessageID: "<e1>", InReplyTo: "<e0>", EmailDate: baseTime.Add(time.Minute)}
	e2 := email.Email{ID: "e2", Subject: "Re: Thread Root", MessageID: "<e2>", InReplyTo: "<e1>", EmailDate: baseTime.Add(2 * time.Minute)}

	mockClient := &mailclient.MockMailClient{Cfg: &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: t.TempDir()}, AccountCache: cachedir.At(t.TempDir())}}
	m := newTuiModel(mockClient, "INBOX", nil)
	m.rawEmails = []email.Email{e0, e1, e2}
	m.rebuildVisibleEmails()

	// Initially collapsed: only root is visible
	if len(m.emails) != 1 {
		t.Fatalf("expected 1 visible email initially, got %d", len(m.emails))
	}
	if m.emails[0].ID != "e0" {
		t.Fatalf("expected e0 visible, got %s", m.emails[0].ID)
	}

	// Press 'E' to archive root email e0
	m.eIdx = 0
	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'E'}})
	tm := res.(*tuiModel)

	// Thread should have been expanded before e0 was removed, so e1 and e2 are now visible!
	if len(tm.emails) != 2 {
		t.Fatalf("expected remaining 2 thread emails (e1, e2) to be visible after E, got %d", len(tm.emails))
	}
	if tm.emails[0].ID != "e1" || tm.emails[1].ID != "e2" {
		t.Fatalf("expected e1 and e2 visible, got %s and %s", tm.emails[0].ID, tm.emails[1].ID)
	}
	if tm.eIdx != 0 || tm.selectedID != "e1" {
		t.Errorf("expected cursor on e1, got eIdx=%d selectedID=%s", tm.eIdx, tm.selectedID)
	}
}
