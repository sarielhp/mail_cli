package organize

import (
	"fmt"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/label"
	"testing"
	"time"

	"github.com/sarielhp/mail_cli/mailclient"
)

func rawMail(to, subject, msgID string, date time.Time) []byte {
	return []byte(fmt.Sprintf("From: me@example.com\r\nTo: %s\r\nSubject: %s\r\nMessage-ID: %s\r\nDate: %s\r\n\r\nbody\r\n",
		to, subject, msgID, date.Format(time.RFC1123Z)))
}

// An unreadable draft used to vanish from the listing, so rm could never
// name it; it is listed and marked instead.
func TestListFolderParsesNewestFirstAndKeepsUnreadable(t *testing.T) {
	t0 := time.Date(2026, 9, 30, 10, 0, 0, 0, time.UTC)
	mock := &mailclient.MockMailClient{
		DownloadedIDs: map[string][]email.ID{"Drafts": {"old", "new", "broken"}},
		RawMessages: map[email.ID][]byte{
			"old": rawMail("a@example.com", "Hello", "<1@x>", t0),
			"new": rawMail("b@example.com", "Later", "<2@x>", t0.Add(time.Hour)),
		},
	}
	drafts, err := listFolder(mock, label.FromServer("Drafts"))
	if err != nil {
		t.Fatal(err)
	}
	if len(drafts) != 3 || drafts[0].ID != "new" || drafts[1].ID != "old" || drafts[2].ID != "broken" {
		t.Fatalf("drafts = %+v", drafts)
	}
	if d := drafts[0]; d.To != "b@example.com" || d.Subject != "Later" || d.MessageID != "<2@x>" || d.ShortID == "" {
		t.Errorf("first draft = %+v", d)
	}
	if d := drafts[2]; !d.Unreadable || d.ShortID == "" {
		t.Errorf("broken draft = %+v, want listed as unreadable", d)
	}
}

func TestMatchServerDraftsRefusesUnknownAndAmbiguous(t *testing.T) {
	drafts := []ServerDraft{
		{ID: "full-one", ShortID: "AB12CD34"},
		{ID: "full-two", ShortID: "EF56AB78"},
		{ID: "full-three", ShortID: "EF56AB78"},
	}
	matched, unknown := MatchServerDrafts(drafts, []string{"ab12cd34", "full-two", "nope", "ef56ab78"})
	if len(matched) != 2 || matched[0].ID != "full-one" || matched[1].ID != "full-two" {
		t.Errorf("matched = %+v", matched)
	}
	if len(unknown) != 2 || unknown[0] != "nope" || unknown[1] != "ef56ab78" {
		t.Errorf("unknown = %v, want the missing name and the ambiguous short id", unknown)
	}
}

func TestPlanPrune(t *testing.T) {
	t0 := time.Date(2026, 9, 30, 10, 0, 0, 0, time.UTC)
	at := func(h int) time.Time { return t0.Add(time.Duration(h) * time.Hour) }
	cases := []struct {
		name     string
		drafts   []ServerDraft // newest first
		sent     []ServerDraft
		opts     PruneOptions
		wantDup  []string
		wantSent []string
	}{
		{
			name: "copies of one session share a Message-ID; the newest is kept",
			drafts: []ServerDraft{
				{ID: "c3", To: "a@x", Subject: "Re: plan", MessageID: "<s@x>", Date: at(3)},
				{ID: "c2", To: "a@x", Subject: "Re: plan", MessageID: "<s@x>", Date: at(2)},
				{ID: "c1", To: "a@x", Subject: "Re: plan", MessageID: "<s@x>", Date: at(1)},
			},
			wantDup: []string{"c2", "c1"},
		},
		{
			name: "two replies in one thread are distinct messages",
			drafts: []ServerDraft{
				{ID: "r2", To: "a@x", Subject: "Re: plan", MessageID: "<r2@x>", Date: at(2)},
				{ID: "r1", To: "a@x", Subject: "Re: plan", MessageID: "<r1@x>", Date: at(1)},
			},
			opts: PruneOptions{BySubject: true},
		},
		{
			name: "untitled notes to oneself are never matched by subject",
			drafts: []ServerDraft{
				{ID: "n2", To: "me@x", Date: at(2)},
				{ID: "n1", To: "me@x", Date: at(1)},
			},
			opts: PruneOptions{BySubject: true},
		},
		{
			name: "without a Message-ID, by-subject dedups only when asked",
			drafts: []ServerDraft{
				{ID: "o2", To: "a@x", Subject: "Old", Date: at(2)},
				{ID: "o1", To: "a@x", Subject: "Old", Date: at(1)},
			},
		},
		{
			name: "without a Message-ID, by-subject dedups when asked",
			drafts: []ServerDraft{
				{ID: "o2", To: "a@x", Subject: "Old", Date: at(2)},
				{ID: "o1", To: "a@x", Subject: "old", Date: at(1)},
			},
			opts:    PruneOptions{BySubject: true},
			wantDup: []string{"o1"},
		},
		{
			name:     "sent: a Message-ID match is sent",
			drafts:   []ServerDraft{{ID: "d", To: "b@x", Subject: "Invoice", MessageID: "<i@x>", Date: at(1)}},
			sent:     []ServerDraft{{ID: "m", To: "b@x", Subject: "Invoice", MessageID: "<i@x>", Date: at(2)}},
			opts:     PruneOptions{Sent: true},
			wantSent: []string{"d"},
		},
		{
			name:   "sent: disagreeing Message-IDs are not sent, whatever the heuristic says",
			drafts: []ServerDraft{{ID: "d", To: "b@x", Subject: "Notes", MessageID: "<d@x>", Date: at(1)}},
			sent:   []ServerDraft{{ID: "m", To: "b@x", Subject: "Notes", MessageID: "<m@x>", Date: at(2)}},
			opts:   PruneOptions{Sent: true, BySubject: true},
		},
		{
			name:   "sent: zero dates never satisfy the heuristic",
			drafts: []ServerDraft{{ID: "d", To: "b@x", Subject: "Notes"}},
			sent:   []ServerDraft{{ID: "m", To: "b@x", Subject: "Notes"}},
			opts:   PruneOptions{Sent: true, BySubject: true},
		},
		{
			name:   "sent: a draft written after the last send survives",
			drafts: []ServerDraft{{ID: "d", To: "b@x", Subject: "Re: x", Date: at(3)}},
			sent:   []ServerDraft{{ID: "m", To: "b@x", Subject: "Re: x", Date: at(2)}},
			opts:   PruneOptions{Sent: true, BySubject: true},
		},
		{
			name:     "sent: heuristic applies when a Message-ID is missing and the send is later",
			drafts:   []ServerDraft{{ID: "d", To: "b@x", Subject: "Re: x", Date: at(1)}},
			sent:     []ServerDraft{{ID: "m", To: "b@x", Subject: "Re: x", MessageID: "<m@x>", Date: at(2)}},
			opts:     PruneOptions{Sent: true, BySubject: true},
			wantSent: []string{"d"},
		},
		{
			name:   "sent is ignored unless asked",
			drafts: []ServerDraft{{ID: "d", MessageID: "<i@x>", Date: at(1)}},
			sent:   []ServerDraft{{ID: "m", MessageID: "<i@x>", Date: at(2)}},
		},
		{
			name: "unreadable drafts are never pruned",
			drafts: []ServerDraft{
				{ID: "u2", Unreadable: true},
				{ID: "u1", Unreadable: true},
			},
			opts: PruneOptions{Sent: true, BySubject: true},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			plan := PlanPrune(tc.drafts, tc.sent, tc.opts)
			if got := ids(plan.Duplicates); !equal(got, tc.wantDup) {
				t.Errorf("duplicates = %v, want %v", got, tc.wantDup)
			}
			if got := ids(plan.Sent); !equal(got, tc.wantSent) {
				t.Errorf("sent = %v, want %v", got, tc.wantSent)
			}
		})
	}
}

func TestPrunePlanIDsDeduplicates(t *testing.T) {
	d := ServerDraft{ID: "x"}
	plan := PrunePlan{Duplicates: []ServerDraft{d}, Sent: []ServerDraft{d, {ID: "y"}}}
	if got := plan.IDs(); !equal(email.IDStrings(got), []string{"x", "y"}) {
		t.Errorf("IDs = %v", got)
	}
}

func ids(ds []ServerDraft) []string {
	var out []string
	for _, d := range ds {
		out = append(out, string(d.ID))
	}
	return out
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
