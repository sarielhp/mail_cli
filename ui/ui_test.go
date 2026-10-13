package ui

import (
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"testing"
	"time"

	"github.com/sarielhp/mail_cli/cache/msg"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/spam"
)

const uiRaw = "From: Alice Smith <alice@example.com>\r\n" +
	"To: bob@example.com\r\n" +
	"Subject: quarterly report\r\n" +
	"Date: Tue, 02 Sep 2025 10:04:05 +0000\r\n" +
	"Message-ID: <report@example.com>\r\n" +
	"References: <first@example.com>\r\n\r\n" +
	"body\r\n"

func seedUIMessage(t *testing.T, id string) string {
	t.Helper()
	dir := t.TempDir()
	if err := msg.Store(cachedir.At(dir), email.ID(id), []byte(uiRaw), time.Now()); err != nil {
		t.Fatalf("seeding cache: %v", err)
	}
	return dir
}

// loadFolderEmail delegates to uicommon.CachedEmail now; this covers what the
// listing layer adds on top of it.
func TestLoadFolderEmail(t *testing.T) {
	dir := seedUIMessage(t, "m1")

	eml := loadFolderEmail(cachedir.At(dir), "m1", nil, false, nil)
	if eml.Subject != "quarterly report" {
		t.Errorf("Subject = %q", eml.Subject)
	}
	if eml.FromEmail != "alice@example.com" {
		t.Errorf("FromEmail = %q", eml.FromEmail)
	}
	// The listing path lost the References header for a long time, which broke
	// thread building.
	if eml.References != "<first@example.com>" {
		t.Errorf("References = %q, want the header to survive", eml.References)
	}
}

// A message that cannot be read becomes a visible error row rather than being
// dropped, so a listing never silently shortens.
func TestLoadFolderEmailUnreadableBecomesPlaceholder(t *testing.T) {
	eml := loadFolderEmail(cachedir.At(t.TempDir()), "absent", nil, false, nil)

	if eml.ID != "absent" {
		t.Errorf("ID = %q, want the requested ID preserved", eml.ID)
	}
	if eml.FromEmail != "[Error]" {
		t.Errorf("FromEmail = %q, want an error marker", eml.FromEmail)
	}
	if eml.EmailDate.IsZero() {
		t.Error("placeholder has a zero date, which would sort unpredictably")
	}
}

func TestLoadFolderEmailAppliesSpamFlag(t *testing.T) {
	dir := seedUIMessage(t, "m1")

	eml := loadFolderEmail(cachedir.At(dir), "m1", nil, true, nil)
	if !eml.IsSpam {
		t.Error("IsSpam was not applied from the caller's verdict")
	}
}

// The scan response, not the cache, decides political and blacklist state in a
// listing produced by a scan.
func TestLoadFolderEmailAppliesScanResponse(t *testing.T) {
	dir := seedUIMessage(t, "m1")

	res := &spam.Response{
		Symbols: map[string]spam.Symbol{"CUSTOM_POLITICAL_BLOCK": {}},
		Action:  "Blacklisted Sender",
	}
	eml := loadFolderEmail(cachedir.At(dir), "m1", nil, false, res)

	if !eml.IsPolitical {
		t.Error("IsPolitical was not taken from the scan response")
	}
	if !eml.IsBlacklisted {
		t.Error("IsBlacklisted was not taken from the scan response")
	}
}

// Bad messages sort to the end, and the rest run oldest first.
func TestSortFolderEmails(t *testing.T) {
	day := func(n int) time.Time { return time.Date(2025, 1, n, 0, 0, 0, 0, time.UTC) }

	emails := []email.Email{
		{ID: "spam-old", EmailDate: day(1), IsSpam: true},
		{ID: "good-new", EmailDate: day(20)},
		{ID: "good-old", EmailDate: day(2)},
		{ID: "poli", EmailDate: day(3), IsPolitical: true},
	}
	sortFolderEmails(emails)

	if emails[0].ID != "good-old" || emails[1].ID != "good-new" {
		t.Errorf("clean messages came out as %s, %s; want good-old then good-new",
			emails[0].ID, emails[1].ID)
	}
	for _, e := range emails[2:] {
		if !e.IsSpam && !e.IsPolitical && !e.IsBlacklisted {
			t.Errorf("%s is clean but sorted after the flagged messages", e.ID)
		}
	}
}

func TestEmailState(t *testing.T) {
	tests := []struct {
		name string
		eml  email.Email
		want string
	}{
		{name: "normal", eml: email.Email{}, want: "normal"},
		{name: "spam", eml: email.Email{IsSpam: true}, want: "spam"},
		{name: "political", eml: email.Email{IsPolitical: true}, want: "political"},
		{name: "blacklist", eml: email.Email{IsBlacklisted: true}, want: "blacklist"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := emailState(tc.eml); got != tc.want {
				t.Errorf("emailState = %q, want %q", got, tc.want)
			}
		})
	}
}
