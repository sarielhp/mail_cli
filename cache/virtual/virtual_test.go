package virtual

import (
	"fmt"
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"github.com/sarielhp/mail_cli/email"
	"sync"
	"testing"
)

func TestVirtualMailbox_SaveLoad(t *testing.T) {
	tempDir := t.TempDir()

	vm := &Mailbox{
		Name:        "test-virtual",
		Description: "A test virtual mailbox",
		MessageIDs:  []email.ID{"msg1", "msg2", "msg3"},
		FolderMap: map[email.ID]string{
			"msg1": "INBOX",
			"msg2": "Sent",
			"msg3": "Archive",
		},
	}

	if err := Save(cachedir.At(tempDir), vm); err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	loaded, err := Load(cachedir.At(tempDir), "test-virtual")
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if loaded.Name != vm.Name {
		t.Errorf("expected name %q, got %q", vm.Name, loaded.Name)
	}
	if len(loaded.MessageIDs) != 3 {
		t.Errorf("expected 3 message IDs, got %d", len(loaded.MessageIDs))
	}
	if loaded.FolderMap["msg2"] != "Sent" {
		t.Errorf("expected FolderMap[msg2] = 'Sent', got %q", loaded.FolderMap["msg2"])
	}

	folder := FindMessageFolder(cachedir.At(tempDir), "msg3")
	if folder != "Archive" {
		t.Errorf("expected FindMessageFolder('msg3') = 'Archive', got %q", folder)
	}
}

// TestConcurrentSaveOfSameMailboxNeverProducesAMangledFile is M7's
// regression: concurrent Save calls for the same virtual mailbox name used
// to write through a shared, unlocked "<name>.json.tmp", so their writes
// could interleave and publish neither writer's JSON. Every concurrent
// writer's data is self-consistent (a single message ID mapped to a distinct
// folder), so whatever wins the race must still parse and its message ID
// must still resolve to its own folder -- never a mix. Run with -race.
func TestConcurrentSaveOfSameMailboxNeverProducesAMangledFile(t *testing.T) {
	tempDir := t.TempDir()
	const writers = 8

	var wg sync.WaitGroup
	for w := range writers {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			id := email.ID(fmt.Sprintf("msg-from-writer-%02d", w))
			vm := &Mailbox{
				Name:       "contested",
				MessageIDs: []email.ID{id},
				FolderMap:  map[email.ID]string{id: fmt.Sprintf("Folder%02d", w)},
			}
			if err := Save(cachedir.At(tempDir), vm); err != nil {
				t.Errorf("Save: %v", err)
			}
		}(w)
	}
	wg.Wait()

	loaded, err := Load(cachedir.At(tempDir), "contested")
	if err != nil {
		t.Fatalf("Load after concurrent Save: %v", err)
	}
	if len(loaded.MessageIDs) != 1 {
		t.Fatalf("expected exactly one writer's single-ID mailbox to win, got %v", loaded.MessageIDs)
	}
	id := loaded.MessageIDs[0]
	folder, ok := loaded.FolderMap[id]
	if !ok || folder == "" {
		t.Fatalf("winning message ID %q has no matching folder entry (mangled write): %+v", id, loaded)
	}
}

// TestFindMessageFolder_NameRequiringEscape is a regression for a bug H4
// introduced: FindMessageFolder used to re-derive a mailbox's name from its
// filename and hand that to Load, which sanitizes whatever name it is
// given. That was harmless while sanitization only ever produced already
// -safe characters, but a name containing one now sanitizes to something
// with '~' in it -- itself not a safe character -- so re-sanitizing the
// filename escapes it a second time and looks under the wrong path.
func TestFindMessageFolder_NameRequiringEscape(t *testing.T) {
	dir := t.TempDir()
	vm := &Mailbox{
		Name:       "search: quarterly.report",
		MessageIDs: []email.ID{"m1"},
		FolderMap:  map[email.ID]string{"m1": "Reports"},
	}
	if err := Save(cachedir.At(dir), vm); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if folder := FindMessageFolder(cachedir.At(dir), "m1"); folder != "Reports" {
		t.Errorf("FindMessageFolder(m1) = %q, want %q", folder, "Reports")
	}
}
