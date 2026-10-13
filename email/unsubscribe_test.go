package email

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExecuteHTTPUnsubscribe_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Unsubscribed successfully"))
	}))
	defer ts.Close()

	testClient := newSafeHTTPClient(true)
	err := executeHTTPUnsubscribeWithClient(testClient, ts.URL, true)
	if err != nil {
		t.Fatalf("expected nil error on HTTP 200, got: %v", err)
	}
}

func TestExecuteHTTPUnsubscribe_Errors(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		statusText string
	}{
		{
			name:       "404 Not Found",
			statusCode: http.StatusNotFound,
			statusText: "404",
		},
		{
			name:       "410 Gone",
			statusCode: http.StatusGone,
			statusText: "410",
		},
		{
			name:       "500 Internal Server Error",
			statusCode: http.StatusInternalServerError,
			statusText: "500",
		},
		{
			name:       "503 Service Unavailable",
			statusCode: http.StatusServiceUnavailable,
			statusText: "503",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.statusCode)
			}))
			defer ts.Close()

			testClient := newSafeHTTPClient(true)
			err := executeHTTPUnsubscribeWithClient(testClient, ts.URL, true)
			if err == nil {
				t.Fatalf("expected error for status %d, got nil", tt.statusCode)
			}
			if !strings.Contains(err.Error(), tt.statusText) {
				t.Errorf("expected error message to contain %q, got %q", tt.statusText, err.Error())
			}
		})
	}
}

func TestExecuteHTTPUnsubscribe_BlocksSSRF(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	err := ExecuteHTTPUnsubscribe(ts.URL)
	if err == nil {
		t.Fatal("expected ExecuteHTTPUnsubscribe to block loopback test server, got nil")
	}
	if !strings.Contains(err.Error(), "blocked") {
		t.Errorf("expected error to mention 'blocked', got: %v", err)
	}

	targets := []struct {
		url     string
		wantErr string
	}{
		{"http://127.0.0.1:8080/unsub", "blocked"},
		{"http://localhost:8080/unsub", "blocked"},
		{"http://169.254.169.254/latest/meta-data/", "blocked"},
		{"http://10.0.0.1/admin", "blocked"},
		{"http://192.168.1.1/router", "blocked"},
		{"http://172.16.0.1/test", "blocked"},
		{"file:///etc/passwd", "unsupported scheme"},
		{"ftp://example.com/unsub", "unsupported scheme"},
	}

	for _, tt := range targets {
		t.Run(tt.url, func(t *testing.T) {
			err := ExecuteHTTPUnsubscribe(tt.url)
			if err == nil {
				t.Fatalf("expected error for URL %q, got nil", tt.url)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error for %q was %q, want containing %q", tt.url, err.Error(), tt.wantErr)
			}
		})
	}
}

func TestExecuteHTTPUnsubscribe_BlocksRedirectToSSRF(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://169.254.169.254/latest/meta-data/", http.StatusFound)
	}))
	defer ts.Close()

	testClient := newSafeHTTPClient(true)
	err := executeHTTPUnsubscribeWithClient(testClient, ts.URL, true)
	if err == nil {
		t.Fatal("expected redirect to metadata IP to be blocked, got nil")
	}
	if !strings.Contains(err.Error(), "blocked") {
		t.Errorf("expected error to contain 'blocked', got: %v", err)
	}
}

func TestParseListUnsubscribe(t *testing.T) {
	header := "<mailto:unsubscribe@example.com?subject=unsub>, <https://example.com/unsub?token=123>"
	mailto, httpURL := ParseListUnsubscribe(header)

	expectedMailto := "mailto:unsubscribe@example.com?subject=unsub"
	expectedHTTP := "https://example.com/unsub?token=123"

	if mailto != expectedMailto {
		t.Errorf("mailto = %q, want %q", mailto, expectedMailto)
	}
	if httpURL != expectedHTTP {
		t.Errorf("httpURL = %q, want %q", httpURL, expectedHTTP)
	}
}

func TestLogUnsubscription(t *testing.T) {
	tmpDir := t.TempDir()

	LogUnsubscription(tmpDir, "spammer@example.com", "http", "https://example.com/unsub", nil)
	LogUnsubscription(tmpDir, "failed@example.com", "http", "https://example.com/fail", errors.New("HTTP 404"))

	data, err := os.ReadFile(filepath.Join(tmpDir, "unsubscriptions.json"))
	if err != nil {
		t.Fatalf("failed to read unsubscriptions.json: %v", err)
	}

	var entries []UnsubscriptionEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		t.Fatalf("failed to parse entries: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("expected 2 log entries, got %d", len(entries))
	}
	if !entries[0].Success || entries[0].Sender != "spammer@example.com" {
		t.Errorf("entry 0 mismatch: %+v", entries[0])
	}
	if entries[1].Success || entries[1].Error != "HTTP 404" {
		t.Errorf("entry 1 mismatch: %+v", entries[1])
	}
}
