package gmail

import (
	"encoding/json"
	"fmt"
	"github.com/sarielhp/mail_cli/report"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

func TestGenerateOAuthState(t *testing.T) {
	s1, err := generateOAuthState()
	if err != nil {
		t.Fatalf("generateOAuthState() failed: %v", err)
	}
	if len(s1) != 32 { // 16 bytes hex-encoded = 32 chars
		t.Errorf("expected 32 hex chars, got %d (%q)", len(s1), s1)
	}

	s2, err := generateOAuthState()
	if err != nil {
		t.Fatalf("second generateOAuthState() failed: %v", err)
	}
	if s1 == s2 {
		t.Errorf("expected distinct random states, got duplicate: %q", s1)
	}
}

func TestStartOAuthCallbackServer(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	expectedState := "valid-test-state-token-12345"

	codeChan := make(chan string, 10)
	errChan := make(chan error, 10)

	srv := startOAuthCallbackServer(listener, expectedState, codeChan, errChan)
	defer srv.Close()

	httpClient := &http.Client{Timeout: 2 * time.Second}

	t.Run("valid state and code", func(t *testing.T) {
		reqURL := fmt.Sprintf("%s/?state=%s&code=test-auth-code", baseURL, expectedState)
		resp, err := httpClient.Get(reqURL)
		if err != nil {
			t.Fatalf("GET failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected status 200, got %d", resp.StatusCode)
		}

		select {
		case code := <-codeChan:
			if code != "test-auth-code" {
				t.Errorf("expected code 'test-auth-code', got %q", code)
			}
		case <-time.After(1 * time.Second):
			t.Fatal("timed out waiting for code on codeChan")
		}
	})

	t.Run("state mismatch rejected", func(t *testing.T) {
		reqURL := fmt.Sprintf("%s/?state=wrong-state&code=attacker-code", baseURL)
		assertCallbackRejected(t, httpClient, reqURL, "state mismatch", errChan)
	})

	t.Run("missing state rejected", func(t *testing.T) {
		reqURL := fmt.Sprintf("%s/?code=some-code", baseURL)
		assertCallbackRejected(t, httpClient, reqURL, "state mismatch", errChan)
	})

	t.Run("missing code rejected", func(t *testing.T) {
		reqURL := fmt.Sprintf("%s/?state=%s", baseURL, expectedState)
		assertCallbackRejected(t, httpClient, reqURL, "without code", errChan)
	})
}

// assertCallbackRejected requests reqURL from the OAuth callback server and
// requires a 400 response plus an error on errChan containing wantErr.
func assertCallbackRejected(t *testing.T, httpClient *http.Client, reqURL, wantErr string, errChan <-chan error) {
	t.Helper()
	resp, err := httpClient.Get(reqURL)
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", resp.StatusCode)
	}

	select {
	case err := <-errChan:
		if !strings.Contains(err.Error(), wantErr) {
			t.Errorf("expected %s error, got: %v", wantErr, err)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for error on errChan")
	}
}

func TestGetOAuthClient_TimeoutConfigured(t *testing.T) {
	tempDir := t.TempDir()
	tokenPath := filepath.Join(tempDir, "token.json")
	tok := &oauth2.Token{
		AccessToken: "dummy-token",
		TokenType:   "Bearer",
		Expiry:      time.Now().Add(1 * time.Hour),
	}
	data, err := json.Marshal(tok)
	if err != nil {
		t.Fatalf("failed to marshal token: %v", err)
	}
	if writeErr := os.WriteFile(tokenPath, data, 0600); writeErr != nil {
		t.Fatalf("failed to write token: %v", writeErr)
	}

	conf := &oauth2.Config{}
	client, err := getOAuthClient(report.Discard(), conf, tokenPath, nil)
	if err != nil {
		t.Fatalf("getOAuthClient failed: %v", err)
	}
	if client.Timeout != 60*time.Second {
		t.Errorf("expected client timeout 60s, got %v", client.Timeout)
	}
}

func TestStartOAuthCallbackServer_MultipleRequestsNoDeadlock(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	expectedState := "multi-req-state"

	// Single-capacity channels
	codeChan := make(chan string, 1)
	errChan := make(chan error, 1)

	// Pre-fill channels to simulate buffer saturation
	codeChan <- "pre-existing-code"
	errChan <- fmt.Errorf("pre-existing-error")

	srv := startOAuthCallbackServer(listener, expectedState, codeChan, errChan)
	defer srv.Close()

	httpClient := &http.Client{Timeout: 1 * time.Second}

	// Send redirect request: must not block despite full codeChan
	reqURL := fmt.Sprintf("%s/?state=%s&code=secondary-code", baseURL, expectedState)
	resp, err := httpClient.Get(reqURL)
	if err != nil {
		t.Fatalf("GET with full codeChan hung or failed: %v", err)
	}
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	// Send error request: must not block despite full errChan
	badURL := fmt.Sprintf("%s/?state=wrong-state", baseURL)
	resp2, err2 := httpClient.Get(badURL)
	if err2 != nil {
		t.Fatalf("GET with full errChan hung or failed: %v", err2)
	}
	_ = resp2.Body.Close()

	if resp2.StatusCode != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", resp2.StatusCode)
	}
}

func TestSaveToken_AtomicWrite(t *testing.T) {
	tempDir := t.TempDir()
	tokenPath := filepath.Join(tempDir, "oauth_token.json")
	tok := &oauth2.Token{
		AccessToken:  "test-secret-token",
		TokenType:    "Bearer",
		RefreshToken: "test-refresh-token",
		Expiry:       time.Now().Add(2 * time.Hour),
	}

	if err := saveToken(report.Discard(), tokenPath, tok); err != nil {
		t.Fatalf("saveToken failed: %v", err)
	}

	fi, err := os.Stat(tokenPath)
	if err != nil {
		t.Fatalf("os.Stat failed on saved token: %v", err)
	}
	if fi.Mode().Perm() != 0600 {
		t.Errorf("expected file permissions 0600, got %o", fi.Mode().Perm())
	}

	data, err := os.ReadFile(tokenPath)
	if err != nil {
		t.Fatalf("os.ReadFile failed: %v", err)
	}
	var loaded oauth2.Token
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("failed to unmarshal saved token: %v", err)
	}
	if loaded.AccessToken != tok.AccessToken || loaded.RefreshToken != tok.RefreshToken {
		t.Errorf("loaded token mismatch: got %+v, want %+v", loaded, tok)
	}
}

// A refreshed access token used to be discarded: it was written back only on
// the re-authentication path, so the file kept one that had already expired.
// Every invocation then paid a refresh round-trip at startup, and a TUI and a
// cron scan running together each spent one -- against a provider that
// rate-limits refreshes.
func TestPersistRefreshedWritesBackARenewedToken(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "token.json")

	old := &oauth2.Token{AccessToken: "stale", RefreshToken: "keep-me"}
	renewed := &oauth2.Token{AccessToken: "fresh", RefreshToken: "keep-me"}

	persistRefreshed(report.Discard(), path, old, renewed)

	got := readTokenFile(t, path)
	if got.AccessToken != "fresh" {
		t.Errorf("AccessToken = %q, want the renewed one", got.AccessToken)
	}
}

// A refresh response may omit the refresh token, meaning "keep the one you
// have". Writing the zero value would lose the only credential that can renew
// anything, forcing a browser round-trip on the next run.
func TestPersistRefreshedKeepsTheRefreshTokenWhenOmitted(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "token.json")

	old := &oauth2.Token{AccessToken: "stale", RefreshToken: "the-only-one"}
	renewed := &oauth2.Token{AccessToken: "fresh"} // no refresh token in the response

	persistRefreshed(report.Discard(), path, old, renewed)

	got := readTokenFile(t, path)
	if got.RefreshToken != "the-only-one" {
		t.Errorf("RefreshToken = %q; the existing one must be carried over", got.RefreshToken)
	}
	if got.AccessToken != "fresh" {
		t.Errorf("AccessToken = %q, want the renewed one", got.AccessToken)
	}
}

// An unchanged token is not rewritten: there is nothing to save, and the write
// would be pure contention between concurrent processes.
func TestPersistRefreshedSkipsAnUnchangedToken(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "token.json")

	same := &oauth2.Token{AccessToken: "unchanged", RefreshToken: "r"}
	persistRefreshed(report.Discard(), path, same, same)

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("an unchanged token was written to disk (stat err: %v)", err)
	}
}

func readTokenFile(t *testing.T, path string) *oauth2.Token {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	var tok oauth2.Token
	if err := json.Unmarshal(data, &tok); err != nil {
		t.Fatalf("decoding the token: %v", err)
	}
	return &tok
}
