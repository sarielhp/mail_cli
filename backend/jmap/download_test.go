package jmap

import (
	"bytes"
	"encoding/json"
	"github.com/sarielhp/mail_cli/backend/backendtest"
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/label"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sarielhp/mail_cli/cache/folderindex"
	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"

	"git.sr.ht/~rockorager/go-jmap"
	jmapMail "git.sr.ht/~rockorager/go-jmap/mail"
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

// jsonTransportClient returns an HTTP client whose transport answers every
// request with 200 and the given JSON body.
func jsonTransportClient(body string) *http.Client {
	return &http.Client{Transport: backendtest.OKTransport([]byte(body))}
}

// failingDownloadTransport answers every request with the given status and body,
// counting each attempt in calls.
func failingDownloadTransport(calls *int, status int, body string) mockTransport {
	return func(_ *http.Request) *http.Response {
		*calls++
		return &http.Response{
			StatusCode: status,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
		}
	}
}

// downloadBlobTestClient builds a client bound to a test account and a blob
// download session whose requests are answered by transport.
func downloadBlobTestClient(transport http.RoundTripper) *client {
	return &client{
		config:  &cfg_g.Config{},
		account: cfg_acc.AccountConfig{Name: "test"},
		client: &jmap.Client{
			Session: &jmap.Session{
				DownloadURL: "https://test.local/download/{blobId}",
			},
			HttpClient: &http.Client{Transport: transport},
		},
	}
}

func TestJMAPClient_DownloadBlob_Limits(t *testing.T) {
	t.Run("exceeds max limit", func(t *testing.T) {
		// Stream 50MB + 10 bytes using repeatingReader
		largeReader := io.LimitReader(repeatingReader{'x'}, maxMessageDownloadBytes+10)
		client := downloadBlobTestClient(mockTransport(func(_ *http.Request) *http.Response {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(largeReader),
				Header:     make(http.Header),
			}
		}))

		_, err := client.downloadBlob("acc", "blob1", "name", "text/plain")
		if err == nil {
			t.Fatal("expected error on oversized download, got nil")
		}
		if !strings.Contains(err.Error(), "exceeded maximum allowed size") {
			t.Errorf("expected size limit error, got: %v", err)
		}
	})

	t.Run("within limit", func(t *testing.T) {
		content := []byte("hello world")
		client := downloadBlobTestClient(mockTransport(func(_ *http.Request) *http.Response {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader(content)),
				Header:     make(http.Header),
			}
		}))

		data, err := client.downloadBlob("acc", "blob1", "name", "text/plain")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !bytes.Equal(data, content) {
			t.Errorf("got %q, want %q", data, content)
		}
	})

	t.Run("retry on 429 then succeed", func(t *testing.T) {
		origDelay := blobRetryBaseDelay
		blobRetryBaseDelay = time.Millisecond
		defer func() { blobRetryBaseDelay = origDelay }()

		calls := 0
		content := []byte("recovered payload")
		client := downloadBlobTestClient(mockTransport(func(_ *http.Request) *http.Response {
			calls++
			if calls < 3 {
				return &http.Response{
					StatusCode: http.StatusTooManyRequests,
					Body:       io.NopCloser(strings.NewReader("rate limit")),
					Header:     make(http.Header),
				}
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader(content)),
				Header:     make(http.Header),
			}
		}))

		data, err := client.downloadBlob("acc", "blob1", "name", "text/plain")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if calls != 3 {
			t.Errorf("expected 3 calls, got %d", calls)
		}
		if !bytes.Equal(data, content) {
			t.Errorf("got %q, want %q", data, content)
		}
	})

	t.Run("retry on 503 exhausted", func(t *testing.T) {
		origDelay := blobRetryBaseDelay
		blobRetryBaseDelay = time.Millisecond
		defer func() { blobRetryBaseDelay = origDelay }()

		calls := 0
		client := downloadBlobTestClient(failingDownloadTransport(&calls, http.StatusServiceUnavailable, "unavailable"))

		_, err := client.downloadBlob("acc", "blob1", "name", "text/plain")
		if err == nil {
			t.Fatal("expected error after retries exhausted, got nil")
		}
		if calls != maxBlobDownloadAttempts {
			t.Errorf("expected %d calls, got %d", maxBlobDownloadAttempts, calls)
		}
		if !strings.Contains(err.Error(), "status 503") {
			t.Errorf("expected 503 status in error, got: %v", err)
		}
	})

	t.Run("non-retryable 404 does not retry", func(t *testing.T) {
		calls := 0
		client := downloadBlobTestClient(failingDownloadTransport(&calls, http.StatusNotFound, "not found"))

		_, err := client.downloadBlob("acc", "blob1", "name", "text/plain")
		if err == nil {
			t.Fatal("expected error on 404, got nil")
		}
		if calls != 1 {
			t.Errorf("expected exactly 1 call for 404, got %d", calls)
		}
	})
}

func TestEnsureMessagesDir(t *testing.T) {
	t.Run("creates messages directory in writable dir", func(t *testing.T) {
		tempDir := t.TempDir()
		if err := ensureMessagesDir(tempDir); err != nil {
			t.Fatalf("ensureMessagesDir failed: %v", err)
		}
		info, err := os.Stat(filepath.Join(tempDir, "messages"))
		if err != nil {
			t.Fatalf("messages directory not found: %v", err)
		}
		if !info.IsDir() {
			t.Errorf("expected messages to be directory, got file")
		}
	})

	t.Run("fails when path is an existing file", func(t *testing.T) {
		tempDir := t.TempDir()
		filePath := filepath.Join(tempDir, "file_blocking_dir")
		if err := os.WriteFile(filePath, []byte("blocking"), 0600); err != nil {
			t.Fatalf("failed to create file: %v", err)
		}
		err := ensureMessagesDir(filePath)
		if err == nil {
			t.Fatal("expected ensureMessagesDir to fail when cacheDir is a file, got nil")
		}
		if !strings.Contains(err.Error(), "failed to create messages cache directory") {
			t.Errorf("unexpected error message: %v", err)
		}
	})
}

func TestJMAPClient_FetchAndDownloadEmails_EmptyFolderClearsCache(t *testing.T) {
	tempDir := t.TempDir()
	folderName := "INBOX"

	if err := folderindex.ReplaceAll(cachedir.At(tempDir), folderName, []email.ID{"old-jmap-1", "old-jmap-2"}); err != nil {
		t.Fatalf("seed label failed: %v", err)
	}

	client := &client{
		config:  &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: tempDir}, AccountCache: cachedir.At(tempDir), Policy: cfg_g.Policy{Limit: 10}},
		account: cfg_acc.AccountConfig{Name: "test"},
		accID:   "acc-1",
		mailboxID: map[string]jmap.ID{
			"inbox": "mb-inbox",
		},
		client: &jmap.Client{
			Session: &jmap.Session{
				APIURL: "https://test.local/api",
				RawCapabilities: map[jmap.URI]json.RawMessage{
					jmap.CoreURI: []byte("{}"),
					jmapMail.URI: []byte("{}"),
				},
				PrimaryAccounts: map[jmap.URI]jmap.ID{
					jmapMail.URI: "acc-1",
				},
				Accounts: map[jmap.ID]jmap.Account{
					"acc-1": {
						Capabilities: map[jmap.URI]jmap.Capability{
							jmapMail.URI: &jmapMail.Mail{},
						},
					},
				},
			},
			HttpClient: jsonTransportClient(`{"sessionState":"state","methodResponses":[["Email/query",{"ids":[]},"0"]]}`),
		},
	}

	result, err := client.FetchAndDownloadEmails(label.FromServer(folderName), "")
	if err != nil {
		t.Fatalf("FetchAndDownloadEmails failed: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected 0 results, got %d", len(result))
	}

	clearedIDs, err := folderindex.IDs(cachedir.At(tempDir), folderName)
	if err != nil {
		t.Fatalf("label.IDs failed: %v", err)
	}
	if len(clearedIDs) != 0 {
		t.Errorf("expected empty folder index, got %v", clearedIDs)
	}
}
