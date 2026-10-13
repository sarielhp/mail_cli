package jmap

import (
	"context"
	"errors"
	"testing"

	"github.com/sarielhp/mail_cli/label"
)

// The check runs before any network call, so a zero client is enough: a
// folder named with the label sigil must never reach Mailbox/set.
func TestJMAP_RefusesSigilFolderNames(t *testing.T) {
	c := &client{}
	if _, err := c.createMailboxLevel(context.Background(), "%archive", "%archive", ""); !errors.Is(err, label.ErrReservedName) {
		t.Errorf("createMailboxLevel(%%archive) = %v, want ErrReservedName", err)
	}
	if err := c.RenameLabel(label.FromServer("Archive"), label.FromServer("%archive")); !errors.Is(err, label.ErrReservedName) {
		t.Errorf("RenameLabel(→%%archive) = %v, want ErrReservedName", err)
	}
}
