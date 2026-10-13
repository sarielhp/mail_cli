package runner

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"syscall"
	"testing"

	"github.com/sarielhp/mail_cli/mailclient"
	"github.com/sarielhp/mail_cli/outbox"
	"github.com/sarielhp/mail_cli/report"
)

func TestIsOfflineSendError(t *testing.T) {
	dial := &net.OpError{Op: "dial", Net: "tcp", Err: os.NewSyscallError("connect", syscall.ECONNREFUSED)}
	read := &net.OpError{Op: "read", Net: "tcp", Err: os.NewSyscallError("read", syscall.ECONNRESET)}
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"read-only refusal", mailclient.ErrReadOnlyOperationBlocked, false},
		{"wrapped read-only refusal", fmt.Errorf("send: %w", mailclient.ErrReadOnlyOperationBlocked), false},
		{"re-authentication", mailclient.ErrInteractiveAuthRequired, false},
		{"permanent rejection", errors.New("550 mailbox unavailable"), false},
		{"reset after connecting", &url.Error{Op: "Post", URL: "https://x", Err: read}, false},
		{"dns failure", fmt.Errorf("send: %w", &url.Error{Op: "Post", URL: "https://x", Err: &net.DNSError{Err: "no such host", Name: "x"}}), true},
		{"dial refused", fmt.Errorf("send: %w", &url.Error{Op: "Post", URL: "https://x", Err: dial}), true},
		{"network unreachable", fmt.Errorf("send: %w", syscall.ENETUNREACH), true},
		{"nil", nil, false},
	}
	for _, tc := range tests {
		if got := isOfflineSendError(tc.err); got != tc.want {
			t.Errorf("%s: isOfflineSendError = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestSpoolOfflineQueuesOnlyUnreachable(t *testing.T) {
	dir := t.TempDir()
	s := SendService{Rep: report.Discard()}
	entry := outbox.Entry{Account: "work", To: []string{"a@example.com"}, Subject: "hi"}

	if s.spoolOffline(dir, entry, []byte("raw"), mailclient.ErrReadOnlyOperationBlocked) {
		t.Fatal("a --read-only refusal must not be queued: a later flush would send it")
	}
	if s.spoolOffline(dir, entry, []byte("raw"), errors.New("invalid recipient")) {
		t.Fatal("a permanent failure must not be queued")
	}
	if got, _ := outbox.List(dir); len(got) != 0 {
		t.Fatalf("expected an empty outbox, got %d entries", len(got))
	}

	if !s.spoolOffline(dir, entry, []byte("raw"), &net.DNSError{Err: "no such host", Name: "x"}) {
		t.Fatal("an unreachable server must be queued")
	}
	got, err := outbox.List(dir)
	if err != nil || len(got) != 1 || got[0].Attempts != 1 || got[0].Account != "work" {
		t.Fatalf("expected one spooled entry with one attempt, got %+v (%v)", got, err)
	}
}
