package msg

import (
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"github.com/sarielhp/mail_cli/email"
	"os"
	"testing"
	"time"
)

const withThreadHeaders = "From: Alice Smith <alice@example.com>\r\n" +
	"To: bob@example.com\r\n" +
	"Subject: Re: quarterly report\r\n" +
	"Date: Tue, 02 Sep 2025 10:04:05 +0000\r\n" +
	"Message-ID: <reply@example.com>\r\n" +
	"In-Reply-To: <original@example.com>\r\n" +
	"References: <first@example.com> <original@example.com>\r\n\r\n" +
	"body\r\n"

func seed(t *testing.T, raw string, date time.Time) (dir string, id email.ID) {
	t.Helper()
	dir = t.TempDir()
	id = "seeded1"
	if err := Store(cachedir.At(dir), id, []byte(raw), date); err != nil {
		t.Fatalf("seeding cache: %v", err)
	}
	return dir, id
}

func TestCachedEmailParsesHeaders(t *testing.T) {
	dir, id := seed(t, withThreadHeaders, time.Now())

	em, err := CachedEmail(cachedir.At(dir), id)
	if err != nil {
		t.Fatalf("CachedEmail: %v", err)
	}
	if em.Subject != "Re: quarterly report" {
		t.Errorf("Subject = %q", em.Subject)
	}
	if em.FromEmail != "alice@example.com" {
		t.Errorf("FromEmail = %q", em.FromEmail)
	}
	if em.FromRaw != "Alice Smith <alice@example.com>" {
		t.Errorf("FromRaw = %q", em.FromRaw)
	}
	if em.ID != id {
		t.Errorf("ID = %q, want %q", em.ID, id)
	}
	if em.EmailDate.Year() != 2025 || em.EmailDate.Month() != time.September {
		t.Errorf("EmailDate = %v", em.EmailDate)
	}
}

// The listing path previously declared References as a named return and never
// assigned it, so every message came back with an empty References header and
// thread building had nothing to work with.
func TestCachedEmailPopulatesThreadHeaders(t *testing.T) {
	dir, id := seed(t, withThreadHeaders, time.Now())

	em, err := CachedEmail(cachedir.At(dir), id)
	if err != nil {
		t.Fatalf("CachedEmail: %v", err)
	}
	if em.MessageID != "<reply@example.com>" {
		t.Errorf("MessageID = %q", em.MessageID)
	}
	if em.InReplyTo != "<original@example.com>" {
		t.Errorf("InReplyTo = %q", em.InReplyTo)
	}
	if em.References != "<first@example.com> <original@example.com>" {
		t.Errorf("References = %q; thread building depends on this", em.References)
	}
}

func TestCachedEmailFallsBackToIndexDate(t *testing.T) {
	stored := time.Date(2021, 7, 14, 0, 0, 0, 0, time.UTC)
	dir, id := seed(t, "From: a@example.com\r\nSubject: undated\r\n\r\nbody\r\n", stored)

	em, err := CachedEmail(cachedir.At(dir), id)
	if err != nil {
		t.Fatalf("CachedEmail: %v", err)
	}
	if em.EmailDate.Year() != 2021 || em.EmailDate.Day() != 14 {
		t.Errorf("EmailDate = %v, want the index date %v", em.EmailDate, stored)
	}
	if em.FormattedDate == "" {
		t.Error("FormattedDate was not recomputed after the date fallback")
	}
}

func TestCachedEmailAppliesClassification(t *testing.T) {
	dir, id := seed(t, withThreadHeaders, time.Now())
	if err := SetEvidence(cachedir.At(dir), id, Evidence{Checks: CheckBogofilter | CheckPolitical, BogoScore: 5}); err != nil {
		t.Fatalf("SetEvidence: %v", err)
	}

	em, err := CachedEmail(cachedir.At(dir), id)
	if err != nil {
		t.Fatalf("CachedEmail: %v", err)
	}
	if !em.IsSpam || !em.IsPolitical {
		t.Errorf("classification not applied: %+v", em)
	}
	if em.IsBlacklisted {
		t.Error("IsBlacklisted set when it should not be")
	}
}

func TestCachedEmailErrors(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T) (dir string, id email.ID)
	}{
		{
			name:  "not cached",
			setup: func(t *testing.T) (string, email.ID) { return t.TempDir(), "absent" },
		},
		{
			name: "malformed message",
			setup: func(t *testing.T) (string, email.ID) {
				return seed(t, "this is not a MIME message at all", time.Now())
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir, id := tc.setup(t)
			em, err := CachedEmail(cachedir.At(dir), id)
			if err == nil {
				t.Fatalf("CachedEmail = %+v, want an error", em)
			}
			if em != nil {
				t.Error("an error result must not also return an email")
			}
		})
	}
}

// Subject reversal is not applied here: the listing paths do it and the TUI
// does not, so it stays a decision of the caller.
func TestCachedEmailLeavesSubjectUnreversed(t *testing.T) {
	raw := "From: a@example.com\r\nSubject: \xd7\xa9\xd7\x9c\xd7\x95\xd7\x9d world\r\n\r\nbody\r\n"
	dir, id := seed(t, raw, time.Now())

	em, err := CachedEmail(cachedir.At(dir), id)
	if err != nil {
		t.Fatalf("CachedEmail: %v", err)
	}
	// Reversing Hebrew runs for display is the caller's decision, so the cache
	// must hand back the header exactly as it arrived.
	if em.Subject != "\xd7\xa9\xd7\x9c\xd7\x95\xd7\x9d world" {
		t.Errorf("Subject = %q, want the raw header unchanged", em.Subject)
	}
}

func TestCachedEmail_SummaryCachePersistence(t *testing.T) {
	dir, id := seed(t, withThreadHeaders, time.Now())

	em1, err := CachedEmail(cachedir.At(dir), id)
	if err != nil {
		t.Fatalf("first CachedEmail call failed: %v", err)
	}
	if em1.Subject != "Re: quarterly report" {
		t.Errorf("expected Subject %q, got %q", "Re: quarterly report", em1.Subject)
	}

	// Verify summary file was persisted in the message cache
	sumData, err := ReadParsedSummary(cachedir.At(dir), id)
	if err != nil {
		t.Fatalf("expected parsed summary to be stored, got err: %v", err)
	}
	if len(sumData) == 0 {
		t.Fatal("stored parsed summary is empty")
	}

	// Dynamic classifications update reflected on second read
	if evidenceErr := SetEvidence(cachedir.At(dir), id, Evidence{Checks: CheckBogofilter, BogoScore: 5}); evidenceErr != nil {
		t.Fatalf("SetEvidence failed: %v", evidenceErr)
	}
	em2, err := CachedEmail(cachedir.At(dir), id)
	if err != nil {
		t.Fatalf("second CachedEmail call failed: %v", err)
	}
	if !em2.IsSpam {
		t.Error("expected IsSpam to be true after classification update")
	}

	// Message deletion removes the summary
	if err := Delete(cachedir.At(dir), id); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	if _, err := ReadParsedSummary(cachedir.At(dir), id); err == nil {
		t.Error("expected ReadParsedSummary to fail after Delete")
	}
}

func TestCachedEmail_SummaryHitParity(t *testing.T) {
	dir, id := seed(t, withThreadHeaders, time.Now())
	if err := SetEvidence(cachedir.At(dir), id, Evidence{Checks: CheckBogofilter | CheckBlacklist, BogoScore: 8.0}); err != nil {
		t.Fatalf("SetEvidence failed: %v", err)
	}

	// Warm the summary cache
	first, err := CachedEmail(cachedir.At(dir), id)
	if err != nil {
		t.Fatalf("first CachedEmail: %v", err)
	}

	// Read again hitting the ReadParsedSummaryAndInfo fast path
	second, err := CachedEmail(cachedir.At(dir), id)
	if err != nil {
		t.Fatalf("second CachedEmail: %v", err)
	}

	if first.Subject != second.Subject || first.FromEmail != second.FromEmail {
		t.Errorf("subject or from email mismatch: %q vs %q", first.Subject, second.Subject)
	}
	if !second.IsSpam || !second.IsBlacklisted {
		t.Errorf("flags mismatch: spam=%v blacklist=%v", second.IsSpam, second.IsBlacklisted)
	}
}

func TestCachedEmail_SummaryStoreFailureDoesNotBlockParse(t *testing.T) {
	dir, id := seed(t, withThreadHeaders, time.Now())

	sumPath, err := parsedSummaryPath(dir, id)
	if err != nil {
		t.Fatalf("parsedSummaryPath: %v", err)
	}
	if mkdirErr := os.Mkdir(sumPath, 0700); mkdirErr != nil {
		t.Fatalf("failed to create directory blocking summary: %v", mkdirErr)
	}

	em, err := CachedEmail(cachedir.At(dir), id)
	if err != nil {
		t.Fatalf("CachedEmail failed when summary storage failed: %v", err)
	}
	if em.Subject != "Re: quarterly report" {
		t.Errorf("Subject = %q, want %q", em.Subject, "Re: quarterly report")
	}
}
