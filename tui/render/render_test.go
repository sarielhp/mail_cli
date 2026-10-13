package render

import (
	"strings"
	"testing"

	"github.com/sarielhp/mail_cli/email"
)

func TestRenderEmailRow_ColumnOrder(t *testing.T) {
	theme := NewDefaultTheme()
	e := email.Email{
		ID:            "msg100",
		Subject:       "Invoice Report",
		FromRaw:       "Alice <alice@example.com>",
		FormattedDate: "12:00",
		IsRead:        false,
		HasAttachment: true,
	}

	row := EmailRow(e, 80, 20, 1, theme, false, false)
	rowPlain := row.Plain()

	// Order must be counter, sep, nBadge, sep, date, sep, sender, sep, count, sep, subj
	// Counter is "   1", nBadge has "N" and "📎"
	counterIdx := strings.Index(rowPlain, "   1")
	badgeIdx := strings.Index(rowPlain, "N")
	attIdx := strings.Index(rowPlain, "📎")
	dateIdx := strings.Index(rowPlain, "12:00")
	senderIdx := strings.Index(rowPlain, "Alice")
	subjIdx := strings.Index(rowPlain, "Invoice Report")

	if counterIdx == -1 || badgeIdx == -1 || attIdx == -1 || dateIdx == -1 || senderIdx == -1 || subjIdx == -1 {
		t.Fatalf("missing expected column content in rendered row: %q", rowPlain)
	}

	if !(counterIdx < badgeIdx && badgeIdx < dateIdx && dateIdx < senderIdx && senderIdx < subjIdx) {
		t.Errorf("column order mismatch: counter=%d, badge=%d, date=%d, sender=%d, subj=%d. Row: %q",
			counterIdx, badgeIdx, dateIdx, senderIdx, subjIdx, rowPlain)
	}
}

func TestRenderEmailRowTagged(t *testing.T) {
	theme := NewDefaultTheme()
	e := email.Email{
		ID:            "msg100",
		Subject:       "Invoice Report",
		FromRaw:       "Alice <alice@example.com>",
		FormattedDate: "12:00",
		IsRead:        true,
	}

	rowUntagged := EmailRowTagged(e, 80, 20, 1, theme, false, false, false)
	rowTagged := EmailRowTagged(e, 80, 20, 1, theme, false, false, true)

	if strings.Contains(rowUntagged.Plain(), "*") {
		t.Errorf("untagged row should not contain '*', got %q", rowUntagged.Plain())
	}
	if !strings.Contains(rowTagged.Plain(), "*") {
		t.Errorf("tagged row must contain '*' in badge, got %q", rowTagged.Plain())
	}
}

func TestRenderEmailRow_AccountBadge(t *testing.T) {
	theme := NewDefaultTheme()
	e := email.Email{
		ID:            "msg200",
		Account:       "Work",
		Subject:       "Quarterly Review",
		FromRaw:       "Boss <boss@example.com>",
		FormattedDate: "13:00",
		IsRead:        true,
	}

	// No badge configured: the account's initial.
	row := EmailRow(e, 80, 40, 1, theme, false, false)
	if !strings.Contains(row.Plain(), "W  Quarterly Review") {
		t.Errorf("expected 'W  Quarterly Review' in row, got %q", row.Plain())
	}

	// A configured badge replaces it, and an emoji's double width does not
	// shift the row: it is exactly as wide as the one with a letter.
	e.AccountBadge = "💼"
	emojiRow := EmailRow(e, 80, 40, 1, theme, false, false)
	if !strings.Contains(emojiRow.Plain(), "💼 Quarterly Review") {
		t.Errorf("expected the configured badge in row, got %q", emojiRow.Plain())
	}
	if emojiRow.Width() != row.Width() {
		t.Errorf("emoji badge changed the row width: %d, want %d", emojiRow.Width(), row.Width())
	}
}

// Accounts are told apart by colour: each position in the configuration gets
// its own, and the palette repeats rather than running out.
func TestAccountColorDistinguishesAccounts(t *testing.T) {
	badgeFg := func(idx int) ColorID {
		e := email.Email{Account: "Work", AccountIndex: idx, Subject: "s"}
		for _, seg := range renderSubject(e, 40, false).Segments {
			if strings.TrimSpace(seg.Text) == "W" {
				return seg.Fg
			}
		}
		t.Fatalf("no badge segment for account %d", idx)
		return ColorDefault
	}
	seen := map[ColorID]bool{}
	for idx := range accountColors {
		fg := badgeFg(idx)
		if seen[fg] {
			t.Errorf("account %d reuses colour %d", idx, fg)
		}
		seen[fg] = true
		if NewDefaultTheme().Get(fg) == NewDefaultTheme().Get(ColorFg) && fg != ColorFg {
			t.Errorf("account colour %d has no theme entry", fg)
		}
	}
	if badgeFg(accountColors) != badgeFg(0) {
		t.Errorf("the palette should wrap after %d accounts", accountColors)
	}
}
