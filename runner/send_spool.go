package runner

import (
	"errors"
	"net"
	"syscall"

	"github.com/sarielhp/mail_cli/outbox"
)

// isOfflineSendError reports whether a SendEmail failure means the server was
// never reached, so the message certainly was not accepted and a later flush
// can deliver it exactly once.
//
// The classification is deliberately narrow:
//   - spooled: DNS failures, a failed dial (including a dial timeout), and
//     connection refused, network unreachable or host unreachable;
//   - not spooled: everything else. That covers the read-only refusal (a
//     queued message would later be really sent by a flush, defeating
//     --read-only), authentication and permanent server rejections (retrying
//     them only fails again), and timeouts or resets after the connection was
//     made, where the server may already have accepted the message and a
//     retry would send it twice.
//
// Backends wrap transport errors with %w (and net/http's *url.Error unwraps),
// so errors.As reaches the net error beneath them.
func isOfflineSendError(err error) bool {
	if err == nil {
		return false
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return true
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) && opErr.Op == "dial" {
		return true
	}
	return errors.Is(err, syscall.ECONNREFUSED) ||
		errors.Is(err, syscall.ENETUNREACH) ||
		errors.Is(err, syscall.EHOSTUNREACH)
}

// spoolOffline queues a message whose send failed because the server could not
// be reached, and reports whether it did. Any other failure is left to the
// caller to report: queueing it would only retry, or worse, later deliver what
// the session refused to send.
func (s SendService) spoolOffline(dir string, entry outbox.Entry, raw []byte, sendErr error) bool {
	if !isOfflineSendError(sendErr) {
		return false
	}
	spooled, err := outbox.Enqueue(dir, entry.Account, entry.To, entry.Subject, raw)
	if err != nil {
		s.rep().Warnf("Server unreachable and spooling to the outbox failed: %v", err)
		return false
	}
	if err := outbox.RecordAttempt(dir, spooled.ID, sendErr); err != nil {
		s.rep().Warnf("Recording the failed attempt on outbox entry %s: %v", spooled.ID, err)
	}
	s.rep().Warnf("Server unreachable; spooled to outbox as %s for later flush", spooled.ID)
	return true
}
