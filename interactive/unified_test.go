package interactive

import (
	"slices"
	"strings"
	"testing"
	"time"
)

// TestUnifiedInboxActsOnEachAccount opens the unified inbox over two accounts,
// each on its own fake JMAP server, and archives and deletes from it. Every
// action must reach the server that holds the message: these actions once
// resolved the account after the row was gone, so each fell back to the
// active account with "All Inboxes" as the source folder, and nothing moved.
func TestUnifiedInboxActsOnEachAccount(t *testing.T) {
	day := time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC)
	ann, bob := "Ann <ann@example.com>", "Bob <bob@example.com>"
	// Interleaved dates fix the unified order, which is oldest first.
	alpha := newFakeJMAP(t, "alpha@example.com",
		fakeMessage{"alpha-1", "Alpha one", ann, day.Add(10 * time.Hour)},
		fakeMessage{"alpha-2", "Alpha two", ann, day.Add(12 * time.Hour)})
	beta := newFakeJMAP(t, "beta@example.com",
		fakeMessage{"beta-1", "Beta one", bob, day.Add(11 * time.Hour)},
		fakeMessage{"beta-2", "Beta two", bob, day.Add(13 * time.Hour)})
	s := startWith(t, []map[string]any{alpha.account("alpha"), beta.account("beta")}, "tui")

	// The active account is alpha; open the unified inbox from the folder
	// pane, by search rather than by row: the pane's order is the tree's.
	s.WaitFor("alpha - Inbox")
	s.WaitFor("Alpha two")
	s.Key("f")
	s.WaitFor("All Inboxes")
	for _, k := range []string{"slash", "a", "l", "l", "enter"} {
		s.Key(k)
	}
	s.WaitFor("alpha - All Inboxes")
	s.WaitFor("Beta two")
	wantRows(t, s.WaitFor("Beta one"), "Alpha one", "Beta one", "Alpha two", "Beta two")

	// Archive beta's message on the second row while alpha is active.
	s.Key("home")
	s.Key("j")
	s.Type("E")
	wantMove(t, beta.WaitMoves(1)[0], "beta-1", "mb-archive")
	if moves := alpha.Moves(); len(moves) != 0 {
		t.Errorf("alpha's server was asked for %+v when beta's message was archived", moves)
	}

	// Tag one message of each account and delete both: each server must be
	// asked to move its own message, and only that, to its Trash.
	wantRows(t, s.Text(), "Alpha one", "Alpha two", "Beta two")
	s.Key("home")
	s.Type("t") // tags Alpha one and moves to Alpha two
	s.Key("j")
	s.Type("t") // tags Beta two
	s.Type(";")
	s.Type("d")
	wantMove(t, alpha.WaitMoves(1)[0], "alpha-1", "mb-trash")
	wantMove(t, beta.WaitMoves(2)[1], "beta-2", "mb-trash")
	time.Sleep(500 * time.Millisecond)
	if moves := alpha.Moves(); len(moves) != 1 {
		t.Errorf("alpha's server got %d changes, want 1: %+v", len(moves), moves)
	}
	if moves := beta.Moves(); len(moves) != 2 {
		t.Errorf("beta's server got %d changes, want 2: %+v", len(moves), moves)
	}
	wantRows(t, s.WaitFor("Alpha two"), "Alpha two")
}

// wantMove checks that mv took id out of the inbox and into mailbox dest.
func wantMove(t *testing.T, mv mailboxMove, id, dest string) {
	t.Helper()
	if mv.id != id || mv.destroyed || !slices.Equal(mv.added, []string{dest}) || !slices.Equal(mv.removed, []string{"mb-inbox"}) {
		t.Errorf("server recorded %+v, want %s moved from mb-inbox to %s", mv, id, dest)
	}
}

// wantRows fails unless the message rows on screen are exactly subjects, in
// that order.
func wantRows(t *testing.T, screen string, subjects ...string) {
	t.Helper()
	all := []string{"Alpha one", "Alpha two", "Beta one", "Beta two"}
	var rows []string
	for line := range strings.SplitSeq(screen, "\n") {
		for _, s := range all {
			if strings.Contains(line, s) {
				rows = append(rows, s)
			}
		}
	}
	if !slices.Equal(rows, subjects) {
		t.Fatalf("rows are %q, want %q:\n%s", rows, subjects, screen)
	}
}
