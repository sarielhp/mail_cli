package outlook

import (
	"bytes"
	"errors"
	"github.com/sarielhp/mail_cli/backend/backendtest"
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/label"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/sarielhp/mail_cli/cache/folderindex"
	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/mailclient"
)

type mockTransport func(req *http.Request) *http.Response

func (f mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req), nil
}

type repeatingReader struct {
	b byte
}

func (r repeatingReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = r.b
	}
	return len(p), nil
}

func TestOutlookClient_DownloadRawMessage_Limits(t *testing.T) {
	t.Run("exceeds max limit", func(t *testing.T) {
		largeReader := io.LimitReader(repeatingReader{'x'}, maxMessageDownloadBytes+10)
		client := &client{
			config:  &cfg_g.Config{},
			account: cfg_acc.AccountConfig{Name: "test"},
			client: &http.Client{
				Transport: mockTransport(func(_ *http.Request) *http.Response {
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(largeReader),
						Header:     make(http.Header),
					}
				}),
			},
		}

		_, err := client.downloadRawMessage("msg-1")
		if err == nil {
			t.Fatal("expected error on oversized message download, got nil")
		}
		if !strings.Contains(err.Error(), "exceeded maximum allowed size") {
			t.Errorf("expected size limit error, got: %v", err)
		}
	})

	t.Run("within limit", func(t *testing.T) {
		content := []byte("Subject: test\r\n\r\nbody")
		client := &client{
			config:  &cfg_g.Config{},
			account: cfg_acc.AccountConfig{Name: "test"},
			client:  &http.Client{Transport: backendtest.OKTransport(content)},
		}

		data, err := client.downloadRawMessage("msg-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !bytes.Equal(data, content) {
			t.Errorf("got %q, want %q", data, content)
		}
	})
}

func TestOutlookClient_FetchAndDownloadEmails_EmptyFolderClearsCache(t *testing.T) {
	tempDir := t.TempDir()
	folderName := "Inbox"

	// Pre-seed local cache with stale IDs
	if err := folderindex.ReplaceAll(cachedir.At(tempDir), folderName, []email.ID{"old-1", "old-2"}); err != nil {
		t.Fatalf("seed label failed: %v", err)
	}
	ids, err := folderindex.IDs(cachedir.At(tempDir), folderName)
	if err != nil || len(ids) != 2 {
		t.Fatalf("expected 2 seeded IDs, got %d (err: %v)", len(ids), err)
	}

	client := &client{
		config:  &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: tempDir}, AccountCache: cachedir.At(tempDir), Policy: cfg_g.Policy{Limit: 10}},
		account: cfg_acc.AccountConfig{Name: "test"},
		client: &http.Client{
			Transport: mockTransport(func(req *http.Request) *http.Response {
				urlStr := req.URL.String()
				if strings.Contains(urlStr, "/mailFolders") && !strings.Contains(urlStr, "/messages") {
					body := `{"value":[{"id":"folder-inbox","displayName":"Inbox"}]}`
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader(body)),
						Header:     make(http.Header),
					}
				}
				if strings.Contains(urlStr, "/messages") {
					body := `{"value":[]}`
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader(body)),
						Header:     make(http.Header),
					}
				}
				return &http.Response{
					StatusCode: http.StatusNotFound,
					Body:       io.NopCloser(strings.NewReader("not found")),
					Header:     make(http.Header),
				}
			}),
		},
	}

	result, err := client.FetchAndDownloadEmails(label.FromServer(folderName), "")
	if err != nil {
		t.Fatalf("FetchAndDownloadEmails failed: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected 0 results, got %d", len(result))
	}

	// Stale local cache must have been cleared
	clearedIDs, err := folderindex.IDs(cachedir.At(tempDir), folderName)
	if err != nil {
		t.Fatalf("label.IDs failed: %v", err)
	}
	if len(clearedIDs) != 0 {
		t.Errorf("expected empty folder index, got %v", clearedIDs)
	}
}

// outlookTestClient builds a client with a test account whose requests are
// answered by transport.
func outlookTestClient(transport http.RoundTripper) *client {
	return &client{
		config:  &cfg_g.Config{},
		account: cfg_acc.AccountConfig{Name: "test"},
		client:  &http.Client{Transport: transport},
	}
}

// partialFailureTransport resolves the Archive folder, rejects the given
// message's move or copy with 400, and accepts every other operation, so a
// batch call fails on exactly that one message.
func partialFailureTransport(failMessageID, operation string) mockTransport {
	return func(req *http.Request) *http.Response {
		urlStr := req.URL.String()
		if strings.Contains(urlStr, "/mailFolders") {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"value":[{"id":"folder-archive","displayName":"Archive"}]}`)),
				Header:     make(http.Header),
			}
		}
		if strings.Contains(urlStr, "/messages/"+failMessageID+"/"+operation) {
			return &http.Response{
				StatusCode: http.StatusBadRequest,
				Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"bad request"}}`)),
				Header:     make(http.Header),
			}
		}
		if strings.Contains(urlStr, "/"+operation) {
			return &http.Response{
				StatusCode: http.StatusCreated,
				Body:       io.NopCloser(strings.NewReader(`{"id":"new-id"}`)),
				Header:     make(http.Header),
			}
		}
		return &http.Response{
			StatusCode: http.StatusNotFound,
			Body:       io.NopCloser(strings.NewReader("not found")),
			Header:     make(http.Header),
		}
	}
}

func TestOutlookClient_MoveEmail_PartialMoveError(t *testing.T) {
	client := outlookTestClient(partialFailureTransport("msg-2", "move"))

	err := client.MoveEmail([]email.ID{"msg-1", "msg-2", "msg-3"}, label.FromServer("Inbox"), label.FromServer("Archive"))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var partial *mailclient.PartialMoveError
	if !errors.As(err, &partial) {
		t.Fatalf("expected *mailclient.PartialMoveError, got %T: %v", err, err)
	}
	if len(partial.Failed) != 1 || partial.Failed[0] != "msg-2" {
		t.Fatalf("expected Failed=[msg-2], got %v", partial.Failed)
	}
}

func TestOutlookClient_CopyEmail_PartialMoveError(t *testing.T) {
	client := outlookTestClient(partialFailureTransport("msg-1", "copy"))

	err := client.CopyEmail([]email.ID{"msg-1", "msg-2"}, label.FromServer("Inbox"), label.FromServer("Archive"))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var partial *mailclient.PartialMoveError
	if !errors.As(err, &partial) {
		t.Fatalf("expected *mailclient.PartialMoveError, got %T: %v", err, err)
	}
	if len(partial.Failed) != 1 || partial.Failed[0] != "msg-1" {
		t.Fatalf("expected Failed=[msg-1], got %v", partial.Failed)
	}
}

func TestOutlookClient_MoveEmail_ThrottlingRetry(t *testing.T) {
	attempts := 0
	client := &client{
		config:  &cfg_g.Config{},
		account: cfg_acc.AccountConfig{Name: "test"},
		client: &http.Client{
			Transport: mockTransport(func(req *http.Request) *http.Response {
				urlStr := req.URL.String()
				if strings.Contains(urlStr, "/mailFolders") {
					body := `{"value":[{"id":"folder-archive","displayName":"Archive"}]}`
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader(body)),
						Header:     make(http.Header),
					}
				}
				if strings.Contains(urlStr, "/messages/msg-1/move") {
					attempts++
					if attempts == 1 {
						header := make(http.Header)
						header.Set("Retry-After", "0")
						return &http.Response{
							StatusCode: http.StatusTooManyRequests,
							Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"throttled"}}`)),
							Header:     header,
						}
					}
					return &http.Response{
						StatusCode: http.StatusCreated,
						Body:       io.NopCloser(strings.NewReader(`{"id":"new-id"}`)),
						Header:     make(http.Header),
					}
				}
				return &http.Response{
					StatusCode: http.StatusNotFound,
					Body:       io.NopCloser(strings.NewReader("not found")),
					Header:     make(http.Header),
				}
			}),
		},
	}

	err := client.MoveEmail([]email.ID{"msg-1"}, label.FromServer("Inbox"), label.FromServer("Archive"))
	if err != nil {
		t.Fatalf("expected success after retry, got: %v", err)
	}
	if attempts != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts)
	}
}
