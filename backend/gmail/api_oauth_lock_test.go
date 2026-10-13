package gmail

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"github.com/sarielhp/mail_cli/report"
)

// TestGetOAuthClientContext_ConcurrentRefreshIsSerialized is a concurrency-race
// regression test: getOAuthClientContext used to read the cached token, refresh
// it, and persist the result with no lock around any of that. A worker pool
// (scan/scanner.go runs eight workers per account) calling it concurrently for
// the same token file raced every reader against the token endpoint: each
// worker read the same still-expired token before any of them had written
// back a refreshed one, so N concurrent workers meant N redundant refresh
// round-trips against Google. Worse, on a token Google actually rejects, each
// of those N callers would independently delete the token file and start its
// own interactive re-authentication flow (its own local HTTP listener, its
// own browser prompt) for what should have been a single re-auth.
//
// This test proves the race by counting how many callers are inside the
// refresh's network call at the same instant. Without a per-token-path lock,
// several of the N concurrent calls reach the fake token endpoint
// simultaneously (maxConcurrent > 1) and each performs its own refresh
// (totalRequests > 1). With the lock, calls are serialized: the first caller
// refreshes and persists a token that is no longer expired, so every later
// caller finds a valid cached token and never reaches the network at all.
func TestGetOAuthClientContext_ConcurrentRefreshIsSerialized(t *testing.T) {
	const numCallers = 10

	var (
		mu            sync.Mutex
		concurrent    int
		maxConcurrent int
		totalRequests int
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		concurrent++
		if concurrent > maxConcurrent {
			maxConcurrent = concurrent
		}
		totalRequests++
		id := totalRequests
		mu.Unlock()

		// Widen the race window so concurrent callers overlap inside the
		// network call rather than racing only around it.
		time.Sleep(50 * time.Millisecond)

		mu.Lock()
		concurrent--
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"access_token":"refreshed-token-%d","token_type":"Bearer","expires_in":3600}`, id)
	}))
	defer server.Close()

	tokenPath := filepath.Join(t.TempDir(), "token.json")
	expiredTok := &oauth2.Token{
		AccessToken:  "stale-access-token",
		RefreshToken: "refresh-token-xyz",
		TokenType:    "Bearer",
		Expiry:       time.Now().Add(-1 * time.Hour),
	}
	data, err := json.Marshal(expiredTok)
	if err != nil {
		t.Fatalf("failed to marshal token: %v", err)
	}
	if err := os.WriteFile(tokenPath, data, 0600); err != nil {
		t.Fatalf("failed to write token: %v", err)
	}

	oauthConfig := &oauth2.Config{
		ClientID:     "test-client",
		ClientSecret: "test-secret",
		Endpoint: oauth2.Endpoint{
			TokenURL:  server.URL,
			AuthStyle: oauth2.AuthStyleInParams,
		},
	}

	var wg sync.WaitGroup
	start := make(chan struct{})
	for range numCallers {
		wg.Go(func() {
			<-start
			if _, err := getOAuthClient(report.Discard(), oauthConfig, tokenPath, nil); err != nil {
				t.Errorf("getOAuthClient failed: %v", err)
			}
		})
	}
	close(start)
	wg.Wait()

	if maxConcurrent > 1 {
		t.Errorf("token refresh is not serialized across concurrent callers: %d requests were in flight at once (want at most 1)", maxConcurrent)
	}
	if totalRequests > 1 {
		t.Errorf("expected a single refresh to satisfy every caller once persisted, got %d refresh requests", totalRequests)
	}
}
