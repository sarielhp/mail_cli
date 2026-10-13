package outlook

import (
	"bytes"
	"fmt"
	"github.com/sarielhp/mail_cli/label"
	"io"
	"net/http"
	"testing"

	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"
)

// closeTrackingBody wraps a response body and records when it was closed.
type closeTrackingBody struct {
	io.Reader
	closed *bool
}

func (b *closeTrackingBody) Close() error {
	*b.closed = true
	return nil
}

// TestEnsureLabelExists_ClosesEachResponseBodyBeforeTheNextRequest is L4's
// regression: `defer resp.Body.Close()` inside EnsureLabelExists's per-level
// loop used to schedule every level's close for the whole function's return,
// so all of them stayed open until the last one was created. Creating a
// three-level path here checks the direct symptom: by the time each POST
// request for level N+1 goes out, level N's response body must already be
// closed, not merely closed eventually once EnsureLabelExists returns.
func TestEnsureLabelExists_ClosesEachResponseBodyBeforeTheNextRequest(t *testing.T) {
	var postBodiesClosed []*bool
	requestNumber := 0

	client := &client{
		config:  &cfg_g.Config{},
		account: cfg_acc.AccountConfig{Name: "test"},
		client: &http.Client{
			Transport: mockTransport(func(req *http.Request) *http.Response {
				if req.Method == http.MethodGet {
					requestNumber++
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(bytes.NewReader([]byte(`{"value":[]}`))),
						Header:     make(http.Header),
					}
				}

				// Every earlier POST level's response body must be closed
				// before this next level's request is even issued.
				for i, wasClosed := range postBodiesClosed {
					if !*wasClosed {
						t.Errorf("level %d's response body was still open when level %d's request was issued", i, len(postBodiesClosed))
					}
				}

				requestNumber++
				closed := new(bool)
				postBodiesClosed = append(postBodiesClosed, closed)
				body := fmt.Sprintf(`{"id":"folder-%d","displayName":"level"}`, requestNumber)
				return &http.Response{
					StatusCode: http.StatusCreated,
					Body:       &closeTrackingBody{Reader: bytes.NewReader([]byte(body)), closed: closed},
					Header:     make(http.Header),
				}
			}),
		},
	}

	if err := client.EnsureLabelExists(label.FromServer("A/B/C")); err != nil {
		t.Fatalf("EnsureLabelExists failed: %v", err)
	}

	// One GET (fetchFoldersRecursive) plus three POSTs (levels A, A/B, A/B/C).
	if requestNumber != 4 {
		t.Fatalf("expected 4 requests (1 GET + 3 POST), got %d", requestNumber)
	}
	if len(postBodiesClosed) != 3 {
		t.Fatalf("expected 3 POST response bodies tracked, got %d", len(postBodiesClosed))
	}
	for i, wasClosed := range postBodiesClosed {
		if !*wasClosed {
			t.Errorf("level %d's response body was never closed", i)
		}
	}
}
