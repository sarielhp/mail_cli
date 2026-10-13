package tui

import (
	"testing"

	"github.com/sarielhp/mail_cli/email"
)

// A header Message-ID and a backend ID that happen to be the same string are
// different messages. When both kinds shared one map of bare strings,
// expanding one also expanded the other.
func TestThreadKeyKeepsHeaderAndBackendIDsApart(t *testing.T) {
	withHeader := email.Email{ID: "m1", MessageID: "x"}
	withoutHeader := email.Email{ID: "x"}
	if threadKeyOf(&withHeader) == threadKeyOf(&withoutHeader) {
		t.Error("a header ID and an equal backend ID produced the same key")
	}
	if threadKeyOf(&withHeader) != (threadKey{header: "x"}) {
		t.Error("a message with a Message-ID must be keyed by it")
	}
}
