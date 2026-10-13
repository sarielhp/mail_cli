package ui

import (
	"github.com/sarielhp/mail_cli/email"
	"io"
	"strings"
	"testing"
	"time"
)

func testEmail(id, from, subject string) email.Email {
	return email.Email{
		ID:        email.ID(id),
		FromRaw:   from,
		FromEmail: from,
		Subject:   subject,
		EmailDate: time.Date(2025, 3, 4, 10, 0, 0, 0, time.UTC),
	}
}

// The folder column is dropped when no folder is known, rather than filling
// every row with "(Unknown)". Search results have no folder.
func TestSummaryColumnsDropFolderWhenUnknown(t *testing.T) {
	const width = 100

	with := summaryColumnsFor(width, true)
	without := summaryColumnsFor(width, false)

	if !with.showFolders || with.folder == 0 {
		t.Errorf("with folders: %+v", with)
	}
	if without.showFolders || without.folder != 0 {
		t.Errorf("without folders: %+v", without)
	}
	// The space the folder column gave up goes to the subject.
	if without.subject <= with.subject {
		t.Errorf("subject width %d without folders, %d with; the reclaimed space was not given to the subject",
			without.subject, with.subject)
	}
}

func TestSummaryColumnsFitTheTerminal(t *testing.T) {
	tests := []struct {
		name        string
		width       int
		showFolders bool
		wantExact   bool
	}{
		{name: "wide with folders", width: 120, showFolders: true, wantExact: true},
		{name: "wide without folders", width: 120, showFolders: false, wantExact: true},
		{name: "narrow with folders", width: 60, showFolders: true, wantExact: false},
		{name: "very narrow", width: 40, showFolders: true, wantExact: false},
		{name: "absurdly narrow", width: 20, showFolders: true, wantExact: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cols := summaryColumnsFor(tc.width, tc.showFolders)
			gaps := 3
			if !tc.showFolders {
				gaps = 2
			}
			total := cols.folder + cols.sender + cols.subject + cols.id + gaps

			// The subject has a floor, so a very narrow terminal overflows
			// rather than producing a zero-width column. What must never happen
			// is a negative or zero subject width.
			if cols.subject < 10 {
				t.Errorf("subject width %d is below the floor", cols.subject)
			}
			if tc.wantExact && total != tc.width {
				t.Errorf("columns total %d for a terminal of %d", total, tc.width)
			}
		})
	}
}

func TestSummaryRowState(t *testing.T) {
	tests := []struct {
		name       string
		eml        email.Email
		wantState  string
		wantPrefix string
	}{
		{name: "ordinary", eml: testEmail("a", "x@y.z", "hi"), wantState: "normal", wantPrefix: ""},
		{
			name:       "spam",
			eml:        email.Email{IsSpam: true},
			wantState:  "spam",
			wantPrefix: "[SPAM] ",
		},
		{
			name:       "political",
			eml:        email.Email{IsPolitical: true},
			wantState:  "political",
			wantPrefix: "[POLI] ",
		},
		{
			name:       "blacklisted",
			eml:        email.Email{IsBlacklisted: true},
			wantState:  "blacklist",
			wantPrefix: "[BLACKLIST] ",
		},
		{
			name:       "blacklist outranks spam",
			eml:        email.Email{IsSpam: true, IsPolitical: true, IsBlacklisted: true},
			wantState:  "blacklist",
			wantPrefix: "[BLACKLIST] ",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			state, prefix := summaryRowState(tc.eml)
			if state != tc.wantState || prefix != tc.wantPrefix {
				t.Errorf("got (%q, %q), want (%q, %q)", state, prefix, tc.wantState, tc.wantPrefix)
			}
		})
	}
}

func TestRenderSummaryRowIncludesTheFolderOnlyWhenShown(t *testing.T) {
	eml := testEmail("abc123def456", "alice@example.com", "quarterly report")
	folderMap := map[email.ID]string{"abc123def456": "Receipts"}

	withFolders := renderSummaryRow(io.Discard, eml, folderMap, summaryColumnsFor(100, true), true, false)
	if !strings.Contains(withFolders, "Receipts") {
		t.Errorf("row %q omits the folder", withFolders)
	}

	without := renderSummaryRow(io.Discard, eml, nil, summaryColumnsFor(100, false), true, false)
	if strings.Contains(without, "Receipts") || strings.Contains(without, "(Unknown)") {
		t.Errorf("row %q shows a folder when the column is off", without)
	}
	if !strings.Contains(without, "quarterly report") {
		t.Errorf("row %q lost the subject", without)
	}
}

// A message with no known folder still gets a placeholder, so the columns stay
// aligned when only some messages have one.
func TestRenderSummaryRowUnknownFolder(t *testing.T) {
	eml := testEmail("id1", "a@b.c", "subject")
	row := renderSummaryRow(io.Discard, eml, map[email.ID]string{"other": "Inbox"}, summaryColumnsFor(100, true), true, false)
	if !strings.Contains(row, "(Unknown)") {
		t.Errorf("row %q should mark an unknown folder", row)
	}
}

func TestRenderSummaryRowMarksSpam(t *testing.T) {
	eml := testEmail("id1", "spammer@example.com", "buy now")
	eml.IsSpam = true

	row := renderSummaryRow(io.Discard, eml, nil, summaryColumnsFor(120, false), false, false)
	if !strings.Contains(row, "[SPAM]") {
		t.Errorf("row %q does not mark the message as spam", row)
	}
}

func TestSummaryFooterCountsMessages(t *testing.T) {
	tests := []struct {
		name  string
		total int
		want  string
	}{
		{name: "plural", total: 3, want: "3 messages"},
		{name: "singular", total: 1, want: "1 message "},
		{name: "none", total: 0, want: "0 messages"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := summaryFooter(tc.total, 70)
			if !strings.Contains(got, tc.want) {
				t.Errorf("footer %q does not contain %q", got, tc.want)
			}
			if len(got) < 70 {
				t.Errorf("footer is %d wide, want it padded to 70", len(got))
			}
		})
	}
}

func TestPadOrTruncate(t *testing.T) {
	tests := []struct {
		name  string
		in    string
		width int
		want  int
	}{
		{name: "pads short", in: "ab", width: 6, want: 6},
		{name: "exact", in: "abcdef", width: 6, want: 6},
		{name: "truncates long", in: "abcdefghij", width: 6, want: 6},
		{name: "empty", in: "", width: 4, want: 4},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := padOrTruncate(tc.in, tc.width)
			if len([]rune(got)) != tc.want {
				t.Errorf("padOrTruncate(%q, %d) = %q (%d runes), want %d runes",
					tc.in, tc.width, got, len([]rune(got)), tc.want)
			}
		})
	}
}
