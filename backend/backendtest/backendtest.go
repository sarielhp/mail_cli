// Package backendtest holds the test helpers the concrete mail backends share.
//
// The backends implement the same consumer contracts, so their tests assert the
// same things: that a MessageAccessor refuses an already-cancelled context
// before it touches the network, and that an HTTP client answers with a canned
// 200 body. Keeping one copy of each here means a change to the contract is
// made once instead of per backend, and the backend tests stay about what is
// specific to them.
//
// It is an ordinary package, not a _test.go file, because the helpers are
// imported by the tests of several other packages.
package backendtest

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/mailclient"
)

// AssertAccessorHonorsCancellation checks that every MessageAccessor method
// returns context.Canceled when handed an already-cancelled context, rather
// than starting network work.
//
// It takes the role interface, so a backend test passes its own client and no
// backend type is named here. Each failed method is reported separately, so one
// method that ignores the context does not hide the others.
func AssertAccessorHonorsCancellation(t *testing.T, acc mailclient.MessageAccessor) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := acc.GetRawMessageContext(ctx, "m1"); !errors.Is(err, context.Canceled) {
		t.Errorf("GetRawMessageContext error = %v, want context.Canceled", err)
	}
	if _, err := acc.GetRawMessagesBatchContext(ctx, []email.ID{"m1"}); !errors.Is(err, context.Canceled) {
		t.Errorf("GetRawMessagesBatchContext error = %v, want context.Canceled", err)
	}
	if _, err := acc.GetMessageSummaryContext(ctx, "m1"); !errors.Is(err, context.Canceled) {
		t.Errorf("GetMessageSummaryContext error = %v, want context.Canceled", err)
	}
	if _, err := acc.GetMessageSummariesBatchContext(ctx, []email.ID{"m1"}); !errors.Is(err, context.Canceled) {
		t.Errorf("GetMessageSummariesBatchContext error = %v, want context.Canceled", err)
	}
	if err := acc.PrefetchMessagesContext(ctx, []email.ID{"m1"}); !errors.Is(err, context.Canceled) {
		t.Errorf("PrefetchMessagesContext error = %v, want context.Canceled", err)
	}
}

// OKTransport returns a RoundTripper that answers every request with 200 and
// body, the shape a backend sees when a download or API call succeeds.
//
// Tests that only need a successful canned body use this rather than
// hand-rolling a transport for status, body and headers.
func OKTransport(body []byte) http.RoundTripper {
	return roundTripFunc(func(*http.Request) *http.Response {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader(body)),
			Header:     make(http.Header),
		}
	})
}

// roundTripFunc adapts a response-building function to http.RoundTripper.
type roundTripFunc func(*http.Request) *http.Response

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req), nil
}
