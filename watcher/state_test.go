package watcher

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sarielhp/mail_cli/email"
)

func TestStateMissingFileIsEmptyNotDamaged(t *testing.T) {
	dir := t.TempDir()
	state, err := LoadState(dir)
	if err != nil {
		t.Fatalf("LoadState on empty dir: %v", err)
	}
	if state.Known("work") {
		t.Fatal("an account is known before anything was recorded")
	}
	if got := state.Unseen("work", []email.ID{"a"}); len(got) != 1 {
		t.Fatalf("Unseen on an unknown account = %v, want the id back", got)
	}
}

func TestStateDamagedFileIsAnError(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "watcher"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stateFile(dir), []byte("{not json"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadState(dir); err == nil {
		t.Fatal("a damaged state file must be an error, not an empty state")
	}
}

func TestStateRoundTrip(t *testing.T) {
	dir := t.TempDir()
	at := time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)

	state, err := LoadState(dir)
	if err != nil {
		t.Fatal(err)
	}
	state.MarkSeen("work", []email.ID{"a", "b"}, at)
	if saveErr := state.Save(dir); saveErr != nil {
		t.Fatalf("Save: %v", saveErr)
	}

	reloaded, err := LoadState(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !reloaded.Known("work") {
		t.Fatal("account is not known after a round trip")
	}
	if got := reloaded.Accounts["work"].LastChecked; !got.Equal(at) {
		t.Fatalf("last checked = %v, want %v", got, at)
	}
	if got := reloaded.Unseen("work", []email.ID{"a", "b", "c"}); len(got) != 1 || got[0] != "c" {
		t.Fatalf("Unseen = %v, want only the new id c", got)
	}
}

func TestStateMarkSeenKeepsNewestFirstAndDeduplicates(t *testing.T) {
	state := &State{Accounts: map[string]Account{}}
	state.MarkSeen("work", []email.ID{"a", "b"}, time.Now())
	state.MarkSeen("work", []email.ID{"c", "a"}, time.Now())

	got := state.Accounts["work"].LastSeenIDs
	want := []string{"c", "a", "b"}
	if len(got) != len(want) {
		t.Fatalf("LastSeenIDs = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("LastSeenIDs = %v, want %v", got, want)
		}
	}
}

func TestStateMarkSeenCapsTheList(t *testing.T) {
	state := &State{Accounts: map[string]Account{}}
	ids := make([]email.ID, 0, maxSeen+50)
	for i := range maxSeen + 50 {
		ids = append(ids, email.ID(string(rune('a'+i%26))+string(rune('0'+i%10))+string(rune(i))))
	}
	state.MarkSeen("work", ids, time.Now())
	if got := len(state.Accounts["work"].LastSeenIDs); got != maxSeen {
		t.Fatalf("LastSeenIDs length = %d, want the cap %d", got, maxSeen)
	}
}
