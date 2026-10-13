package jmap

import (
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"

	"git.sr.ht/~rockorager/go-jmap"
)

// TestGetOrCreateMailbox_RetryAfterAmbiguousFailureDoesNotDuplicate is a
// deep-review finding: getOrCreateMailbox used to retry a Mailbox/set
// Create call through the shared executeJMAPOpWithRetry helper exactly like
// every idempotent-safe read call in this package, on a timeout or
// 502/503/504 -- the ambiguous case where the server may already have
// created the mailbox and only the response was lost. Retrying blindly in
// that case creates a second, duplicate mailbox with the identical name.
//
// This simulates exactly that: the first Mailbox/set call fails with a
// retryable 503, and (standing in for "the create actually landed
// server-side") the mailbox list the fix re-checks before retrying now
// reports the mailbox as already existing. The fix must use that mailbox
// instead of creating a second one.
func TestGetOrCreateMailbox_RetryAfterAmbiguousFailureDoesNotDuplicate(t *testing.T) {
	var getCalls, setCalls atomic.Int32

	sess := newTestMailSession("https://test.local/api")
	client := &client{
		config: &cfg_g.Config{},
		account: cfg_acc.AccountConfig{
			Name: "test",
		},
		accID: "acc-1",
		client: &jmap.Client{
			Session: sess,
			HttpClient: &http.Client{
				Transport: mockTransport(func(req *http.Request) *http.Response {
					bodyBytes, _ := io.ReadAll(req.Body)
					reqStr := string(bodyBytes)

					switch {
					case strings.Contains(reqStr, "Mailbox/set"):
						n := setCalls.Add(1)
						if n == 1 {
							// First create: ambiguous failure. The mailbox
							// may or may not have actually been created
							// server-side; the client cannot tell from this
							// response alone.
							return &http.Response{
								StatusCode: http.StatusServiceUnavailable,
								Header:     http.Header{"Content-Type": []string{"application/json"}},
								Body:       io.NopCloser(strings.NewReader(`{"type":"serviceUnavailable","status":503,"detail":"retry later"}`)),
							}
						}
						// A second create call means the fix failed to
						// re-check existence before retrying.
						return &http.Response{
							StatusCode: http.StatusOK,
							Header:     make(http.Header),
							Body: io.NopCloser(strings.NewReader(
								`{"sessionState":"state","methodResponses":[["Mailbox/set",{"created":{"new-mb":{"id":"mb-duplicate"}}},"0"]]}`)),
						}
					case strings.Contains(reqStr, "Mailbox/get"):
						n := getCalls.Add(1)
						if n <= 2 {
							// getOrCreateMailbox checks existence twice
							// before ever attempting a create (once at its
							// own top level, once inside its per-path-
							// segment loop) -- both must genuinely report
							// the folder missing so a create is attempted
							// at all.
							return &http.Response{
								StatusCode: http.StatusOK,
								Header:     make(http.Header),
								Body: io.NopCloser(strings.NewReader(
									`{"sessionState":"state","methodResponses":[["Mailbox/get",{"list":[]},"0"]]}`)),
							}
						}
						// The fix's re-check before retrying the create:
						// the mailbox actually landed server-side despite
						// the lost response.
						return &http.Response{
							StatusCode: http.StatusOK,
							Header:     make(http.Header),
							Body: io.NopCloser(strings.NewReader(
								`{"sessionState":"state","methodResponses":[["Mailbox/get",{"list":[{"id":"mb-real","name":"keep"}]},"0"]]}`)),
						}
					default:
						t.Fatalf("unexpected JMAP method call: %s", reqStr)
						return nil
					}
				}),
			},
		},
	}

	id, err := client.getOrCreateMailbox("keep")
	if err != nil {
		t.Fatalf("getOrCreateMailbox: %v", err)
	}
	if id != "mb-real" {
		t.Errorf("expected the mailbox found on re-check (mb-real), got %q -- a duplicate was likely created", id)
	}
	if got := setCalls.Load(); got != 1 {
		t.Errorf("expected exactly 1 Mailbox/set (create) call, got %d -- a retry created a duplicate mailbox", got)
	}
}
