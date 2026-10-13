package tui

import (
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/label"
	"testing"

	"github.com/sarielhp/mail_cli/cache/folderindex"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/mailclient"
)

// Under --read-only the decorator simulates the server move and returns nil.
// The TUI then wrote the folder index anyway, recording a move that had not
// happened -- the cache claimed the message had left a folder it was still in.
//
// CheckingMailClient owns reconciliation and correctly skips it under
// --read-only; the TUI's second copy of that work did not, which is why the
// duplicate was a defect rather than merely redundant.
func TestReadOnlyArchiveLeavesTheIndexAlone(t *testing.T) {
	dir := t.TempDir()
	cfg := &cfg_g.Config{
		Paths: cfg_g.Paths{CacheRoot: dir}, AccountCache: cachedir.At(dir),
		ReadOnly: true,
	}

	mock := &mailclient.MockMailClient{Cfg: cfg, InboxFolderVal: "INBOX"}
	client := mailclient.NewReadOnlyMailClient(mock, "acct")
	checking := mailclient.NewCheckingMailClient(client, "acct", nil)

	if err := folderindex.ReplaceAll(cachedir.At(dir), "INBOX", []email.ID{"m1"}); err != nil {
		t.Fatalf("seeding: %v", err)
	}

	if err := checking.MoveEmail([]email.ID{"m1"}, label.FromServer("INBOX"), label.FromServer("Archive")); err != nil {
		t.Fatalf("MoveEmail: %v", err)
	}

	srcIDs, _ := folderindex.IDs(cachedir.At(dir), "INBOX")
	if len(srcIDs) != 1 || srcIDs[0] != "m1" {
		t.Errorf("source index is %v; under --read-only nothing moved, so m1 must still be listed", srcIDs)
	}
	if dstIDs, _ := folderindex.IDs(cachedir.At(dir), "Archive"); len(dstIDs) != 0 {
		t.Errorf("destination index is %v; the move was simulated, not made", dstIDs)
	}
}
