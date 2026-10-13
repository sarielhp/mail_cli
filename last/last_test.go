package last

import (
	"bytes"
	"fmt"
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"github.com/sarielhp/mail_cli/email"
	"testing"
	"time"

	"github.com/sarielhp/mail_cli/cache/msg"
	"github.com/sarielhp/mail_cli/cache/virtual"
	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/mailclient"
	"github.com/sarielhp/mail_cli/report"
)

func TestPerform_LastN(t *testing.T) {
	tempDir := t.TempDir()

	// Store dummy messages in msg cache
	now := time.Now()
	for i := 1; i <= 5; i++ {
		msgID := fmt.Sprintf("id%d", i)
		raw := []byte(fmt.Sprintf("From: sender%d@example.com\r\nSubject: Test Email %d\r\nDate: %s\r\n\r\nBody %d",
			i, i, now.Add(time.Duration(i)*time.Hour).Format(time.RFC1123Z), i))
		if err := msg.Store(cachedir.At(tempDir), email.ID(msgID), raw, now.Add(time.Duration(i)*time.Hour)); err != nil {
			t.Fatalf("msg.Store failed: %v", err)
		}
	}

	mock := &mailclient.MockMailClient{
		LabelItems: []cfg_acc.Item{
			{Name: "INBOX", FullName: "INBOX", MessagesTotal: 3},
			{Name: "Work", FullName: "Work", MessagesTotal: 1},
			{Name: "Archive", FullName: "Archive", MessagesTotal: 1},
		},
		AccountLatestRefs: []cfg_acc.MessageFolderRef{
			{MessageID: "id5", Folder: "INBOX"},
			{MessageID: "id4", Folder: "Archive"},
			{MessageID: "id3", Folder: "INBOX"},
			{MessageID: "id2", Folder: "Work"},
			{MessageID: "id1", Folder: "INBOX"},
		},
		DownloadedIDs: map[string][]email.ID{
			"INBOX":   {"id1", "id3", "id5"},
			"Work":    {"id2"},
			"Archive": {"id4"},
		},
	}

	config := &cfg_g.Config{Accounts: []cfg_acc.AccountConfig{
		{
			Name: "default",
		},
	}, Paths: cfg_g.Paths{CacheRoot: tempDir}, AccountCache: cachedir.At(tempDir)}

	// The listing is the command's product, so it lands on the writer Perform
	// is given rather than on the process's stdout.
	var out bytes.Buffer
	if err := Perform(Options{}, &out, report.Discard(), mock, config, 3); err != nil {
		t.Fatalf("Perform: %v", err)
	}

	if out.Len() == 0 {
		t.Error("expected Perform to write the listing to its writer")
	}

	// Check virtual mailbox was created
	vm, err := virtual.Load(cachedir.At(tempDir), "last")
	if err != nil {
		t.Fatalf("expected 'last' virtual mailbox to exist: %v", err)
	}
	if len(vm.MessageIDs) != 3 {
		t.Errorf("expected 3 messages in virtual mailbox, got %d", len(vm.MessageIDs))
	}
	// The 3 most recent should be id3, id4, id5
	expectedIDs := map[string]bool{"id3": true, "id4": true, "id5": true}
	for _, id := range vm.MessageIDs {
		if !expectedIDs[string(id)] {
			t.Errorf("unexpected message ID %q in virtual mailbox", id)
		}
	}

	// Test JSON output
	var jsonOut bytes.Buffer
	if err := Perform(Options{JSON: true}, &jsonOut, report.Discard(), mock, config, 3); err != nil {
		t.Fatalf("Perform with JSON: %v", err)
	}
	if jsonOut.Len() == 0 || jsonOut.Bytes()[0] != '[' {
		t.Errorf("expected valid JSON array output, got: %s", jsonOut.String())
	}
}
