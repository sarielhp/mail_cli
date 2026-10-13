package outlook

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sarielhp/mail_cli/cfg_g"
)

func TestTokenFileName_Injectivity(t *testing.T) {
	// Finding 5: SanitizeLabelForCache collision between plus-address and underscore
	f1 := tokenFileName("user+work@outlook.com")
	f2 := tokenFileName("user_work@outlook.com")
	if f1 == f2 {
		t.Fatalf("tokenFileName collision detected for distinct accounts: %q vs %q", f1, f2)
	}

	// An account name made only of '-'/'_' sanitizes to the empty string (both
	// runes pass through SanitizeLabelForCache unescaped) and must fall back
	// to "account".
	for _, raw := range []string{"--", "__", "-_-"} {
		name := tokenFileName(raw)
		if !strings.HasPrefix(name, "outlook_token_account_") {
			t.Errorf("expected %q to prefix with outlook_token_account_, got %q", raw, name)
		}
		if !strings.HasSuffix(name, ".json") {
			t.Errorf("expected %q to end in .json, got %q", raw, name)
		}
	}

	// Other punctuation-only names now sanitize to a non-empty, injective
	// "~<hex>~" escape (H4) rather than collapsing to nothing, so they no
	// longer need the "account" fallback at all -- they just need to be
	// distinct, non-empty, and .json-suffixed.
	otherPunctCases := []string{"..", "@", "!!!"}
	for _, raw := range otherPunctCases {
		name := tokenFileName(raw)
		if !strings.HasSuffix(name, ".json") {
			t.Errorf("expected %q to end in .json, got %q", raw, name)
		}
		if name == "outlook_token_.json" {
			t.Errorf("expected %q to sanitize to a non-empty name, got %q", raw, name)
		}
	}
}

func TestGetTokenPath_Validation(t *testing.T) {
	// Finding 4: Nil config
	if _, err := GetTokenPath(nil, "work"); err == nil {
		t.Error("expected error for nil config, got nil")
	}

	// Non-absolute ConfigDir must fail
	relCfg := &cfg_g.Config{Paths: cfg_g.Paths{ConfigDir: "relative/path"}}
	if _, err := GetTokenPath(relCfg, "work"); err == nil {
		t.Error("expected error for relative ConfigDir, got nil")
	}

	// Absolute ConfigDir must succeed and return absolute path
	tmpDir := t.TempDir()
	absCfg := &cfg_g.Config{Paths: cfg_g.Paths{ConfigDir: tmpDir}}
	path, err := GetTokenPath(absCfg, "work")
	if err != nil {
		t.Fatalf("unexpected error for absolute ConfigDir: %v", err)
	}
	if !filepath.IsAbs(path) {
		t.Errorf("expected absolute path, got %q", path)
	}
	if !strings.HasPrefix(path, tmpDir) {
		t.Errorf("path %q does not start with tmpDir %q", path, tmpDir)
	}
}

func TestSaveToken_PermissionsAndAtomicity(t *testing.T) {
	// Finding 1 & 2: 0600 mode on file, 0700 on directory, and atomic write
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, "config_sub")
	// Pre-create directory with looser 0755 permissions
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}

	tokenPath := filepath.Join(configDir, "token.json")
	tok := &Token{
		AccessToken:  "access-123",
		RefreshToken: "refresh-456",
		Expiry:       time.Now().Add(1 * time.Hour),
	}

	if err := saveToken(tokenPath, tok); err != nil {
		t.Fatalf("saveToken failed: %v", err)
	}

	// Check file mode (0600)
	fi, err := os.Stat(tokenPath)
	if err != nil {
		t.Fatalf("stat token file failed: %v", err)
	}
	if fi.Mode().Perm() != 0600 {
		t.Errorf("expected file mode 0600, got %o", fi.Mode().Perm())
	}

	// Check directory mode (0700)
	di, err := os.Stat(configDir)
	if err != nil {
		t.Fatalf("stat directory failed: %v", err)
	}
	if di.Mode().Perm() != 0700 {
		t.Errorf("expected directory mode 0700, got %o", di.Mode().Perm())
	}

	// Check loaded contents
	loaded, err := loadToken(tokenPath)
	if err != nil {
		t.Fatalf("loadToken failed: %v", err)
	}
	if loaded.AccessToken != tok.AccessToken || loaded.RefreshToken != tok.RefreshToken {
		t.Errorf("loaded token mismatch: got %+v, want %+v", loaded, tok)
	}
}

func TestSaveToken_SymlinkReplacement(t *testing.T) {
	// Finding 2: Symlink defense - replacing symlink rather than writing through it
	tmpDir := t.TempDir()
	externalTarget := filepath.Join(tmpDir, "external_secret.txt")
	if err := os.WriteFile(externalTarget, []byte("original_target_content"), 0600); err != nil {
		t.Fatal(err)
	}

	tokenSymlink := filepath.Join(tmpDir, "token_link.json")
	if err := os.Symlink(externalTarget, tokenSymlink); err != nil {
		t.Fatal(err)
	}

	tok := &Token{
		AccessToken:  "secret-tok",
		RefreshToken: "secret-refresh",
		Expiry:       time.Now().Add(time.Hour),
	}

	if err := saveToken(tokenSymlink, tok); err != nil {
		t.Fatalf("saveToken failed on symlink: %v", err)
	}

	// Verify external target was NOT overwritten
	targetContent, err := os.ReadFile(externalTarget)
	if err != nil {
		t.Fatal(err)
	}
	if string(targetContent) != "original_target_content" {
		t.Errorf("external target was overwritten through symlink! got %q", string(targetContent))
	}

	// Verify tokenSymlink is now a regular file, not a symlink
	lfi, err := os.Lstat(tokenSymlink)
	if err != nil {
		t.Fatal(err)
	}
	if lfi.Mode()&os.ModeSymlink != 0 {
		t.Error("expected tokenSymlink to be replaced with regular file, but still a symlink")
	}
}

func TestRefreshToken_ValidationAndNonMutation(t *testing.T) {
	// Finding 7: Validate response and do not mutate in-place on failure
	origTok := &Token{
		AccessToken:  "orig-access",
		RefreshToken: "orig-refresh",
		Expiry:       time.Now().Add(10 * time.Minute),
	}

	// Case A: 200 OK with empty json {}
	srvEmpty := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srvEmpty.Close()

	origTokenURL := tokenURL
	defer func() { tokenURL = origTokenURL }()

	tokenURL = srvEmpty.URL
	tokCopy := *origTok
	_, err := refreshToken(context.Background(), &tokCopy)
	if err == nil {
		t.Error("expected error when refresh response is empty json, got nil")
	}
	if tokCopy.AccessToken != origTok.AccessToken {
		t.Errorf("original token was mutated in place: got %q, want %q", tokCopy.AccessToken, origTok.AccessToken)
	}

	// Case B: 200 OK with error payload
	srvErr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"token revoked"}`))
	}))
	defer srvErr.Close()

	tokenURL = srvErr.URL
	tokCopy = *origTok
	_, err = refreshToken(context.Background(), &tokCopy)
	if err == nil {
		t.Error("expected error when refresh response contains error field, got nil")
	}
	if tokCopy.AccessToken != origTok.AccessToken {
		t.Errorf("original token was mutated in place on error payload: got %q, want %q", tokCopy.AccessToken, origTok.AccessToken)
	}

	// Case C: 200 OK with valid tokens
	srvValid := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := tokenResponse{
			AccessToken:  "new-access",
			RefreshToken: "new-refresh",
			ExpiresIn:    3600,
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srvValid.Close()

	tokenURL = srvValid.URL
	tokCopy = *origTok
	next, err := refreshToken(context.Background(), &tokCopy)
	if err != nil {
		t.Fatalf("unexpected error for valid refresh: %v", err)
	}
	if next.AccessToken != "new-access" || next.RefreshToken != "new-refresh" {
		t.Errorf("unexpected refreshed token: %+v", next)
	}
	if tokCopy.AccessToken != origTok.AccessToken {
		t.Errorf("input token should not be mutated in-place; got %q, want %q", tokCopy.AccessToken, origTok.AccessToken)
	}
}

func TestPollDeviceCodeOnce_Handling(t *testing.T) {
	// Finding 3: RFC 8628 slow_down and pending handling
	origTokenURL := tokenURL
	defer func() { tokenURL = origTokenURL }()

	// Case A: authorization_pending
	srvPending := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(tokenResponse{Error: "authorization_pending"})
	}))
	defer srvPending.Close()

	tokenURL = srvPending.URL
	interval := 5 * time.Second
	tok, retry, err := pollDeviceCodeOnce(context.Background(), "dev-123", &interval)
	if err != nil || !retry || tok != nil {
		t.Errorf("expected pending retry=true err=nil, got tok=%v retry=%v err=%v", tok, retry, err)
	}

	// Case B: slow_down increases interval by 5s per RFC 8628
	srvSlow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(tokenResponse{Error: "slow_down"})
	}))
	defer srvSlow.Close()

	tokenURL = srvSlow.URL
	interval = 5 * time.Second
	tok, retry, err = pollDeviceCodeOnce(context.Background(), "dev-123", &interval)
	if err != nil || !retry || tok != nil {
		t.Errorf("expected slow_down retry=true err=nil, got tok=%v retry=%v err=%v", tok, retry, err)
	}
	if interval != 10*time.Second {
		t.Errorf("expected interval to increase to 10s, got %v", interval)
	}

	// Case C: fatal error
	srvFatal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(tokenResponse{Error: "expired_token"})
	}))
	defer srvFatal.Close()

	tokenURL = srvFatal.URL
	tok, retry, err = pollDeviceCodeOnce(context.Background(), "dev-123", &interval)
	if err == nil || retry || tok != nil {
		t.Errorf("expected fatal error retry=false tok=nil, got tok=%v retry=%v err=%v", tok, retry, err)
	}
}

func TestTokenSource_ConcurrentAccessAndInitErr(t *testing.T) {
	// Finding 4 & concurrency: Init error propagation
	badCfg := &cfg_g.Config{Paths: cfg_g.Paths{ConfigDir: "relative/dir"}}
	tsBad := newTokenSource(badCfg, "test")
	if _, err := tsBad.Token(); err == nil {
		t.Error("expected Token() to fail when initialized with invalid configDir")
	}

	// Concurrency test on valid token
	tmpDir := t.TempDir()
	goodCfg := &cfg_g.Config{Paths: cfg_g.Paths{ConfigDir: tmpDir}}
	path, err := GetTokenPath(goodCfg, "test")
	if err != nil {
		t.Fatal(err)
	}
	initialTok := &Token{
		AccessToken:  "concur-access",
		RefreshToken: "concur-refresh",
		Expiry:       time.Now().Add(10 * time.Minute),
	}
	if err := saveToken(path, initialTok); err != nil {
		t.Fatal(err)
	}

	ts := newTokenSource(goodCfg, "test")
	var wg sync.WaitGroup
	errCount := 0
	var mu sync.Mutex

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tok, err := ts.Token()
			if err != nil || tok.AccessToken != "concur-access" {
				mu.Lock()
				errCount++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if errCount > 0 {
		t.Errorf("%d goroutines encountered error reading token", errCount)
	}
}

func TestOAuthHTTPClient_TimeoutConfigured(t *testing.T) {
	// Finding 6: Bounded HTTP client timeout
	if oauthHTTPClient == nil {
		t.Fatal("oauthHTTPClient is nil")
	}
	if oauthHTTPClient.Timeout != 30*time.Second {
		t.Errorf("expected 30s timeout on oauthHTTPClient, got %v", oauthHTTPClient.Timeout)
	}
}

func TestGetLegacyTokenPath(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &cfg_g.Config{Paths: cfg_g.Paths{ConfigDir: tmpDir}}
	legacy, err := GetLegacyTokenPath(cfg, "work")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := filepath.Join(tmpDir, "outlook_token_work.json")
	if legacy != expected {
		t.Errorf("expected %q, got %q", expected, legacy)
	}
}

func TestTokenSource_LegacyFallback(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &cfg_g.Config{Paths: cfg_g.Paths{ConfigDir: tmpDir}}

	legacyPath, err := GetLegacyTokenPath(cfg, "my_acc")
	if err != nil {
		t.Fatal(err)
	}
	canonicalPath, err := GetTokenPath(cfg, "my_acc")
	if err != nil {
		t.Fatal(err)
	}

	initialTok := &Token{
		AccessToken:  "legacy-access-token",
		RefreshToken: "legacy-refresh-token",
		Expiry:       time.Now().Add(1 * time.Hour),
	}
	if saveErr := saveToken(legacyPath, initialTok); saveErr != nil {
		t.Fatalf("saving legacy token: %v", saveErr)
	}

	ts := newTokenSource(cfg, "my_acc")
	tok, err := ts.Token()
	if err != nil {
		t.Fatalf("Token() failed with fallback: %v", err)
	}
	if tok.AccessToken != "legacy-access-token" {
		t.Errorf("expected AccessToken %q, got %q", "legacy-access-token", tok.AccessToken)
	}

	// Verify it automatically persisted to the canonical path
	if _, err := os.Stat(canonicalPath); err != nil {
		t.Errorf("expected canonical token file %q to be created, got error: %v", canonicalPath, err)
	}

	// Verify the unhashed legacy token was retired
	if _, err := os.Stat(legacyPath); !os.IsNotExist(err) {
		t.Errorf("expected legacy token file %q to be removed, got error: %v", legacyPath, err)
	}
}

func TestMigrateLegacyToken_PreservedOnSaveFailure(t *testing.T) {
	legacyDir := t.TempDir()
	legacyPath := filepath.Join(legacyDir, "legacy_token.json")

	initialTok := &Token{
		AccessToken:  "legacy-access-token",
		RefreshToken: "legacy-refresh-token",
		Expiry:       time.Now().Add(1 * time.Hour),
	}
	if err := saveToken(legacyPath, initialTok); err != nil {
		t.Fatalf("saving legacy token: %v", err)
	}

	canonicalPath := filepath.Join("/dev/null", "cannot_create_subpath", "token.json")

	tok, err := migrateLegacyToken(canonicalPath, legacyPath)
	if err == nil {
		t.Fatal("expected migrateLegacyToken to fail when canonical save fails, got nil error")
	}
	if tok != nil {
		t.Errorf("expected nil token on save failure, got %+v", tok)
	}
	if !strings.Contains(err.Error(), "failed to migrate legacy outlook token to secure storage") {
		t.Errorf("expected error message to mention migration failure, got: %v", err)
	}

	// Because canonical save failed, legacy file MUST still exist to preserve credentials
	if _, err := os.Stat(legacyPath); err != nil {
		t.Errorf("expected legacy file %q to be preserved on save failure, got error: %v", legacyPath, err)
	}
}

func TestMigrateLegacyToken_RemoveFailure(t *testing.T) {
	tmpDir := t.TempDir()
	legacySubdir := filepath.Join(tmpDir, "readonly_legacy_dir")
	if err := os.MkdirAll(legacySubdir, 0700); err != nil {
		t.Fatal(err)
	}
	legacyPath := filepath.Join(legacySubdir, "legacy_token.json")

	initialTok := &Token{
		AccessToken:  "legacy-access-token",
		RefreshToken: "legacy-refresh-token",
		Expiry:       time.Now().Add(1 * time.Hour),
	}
	if err := saveToken(legacyPath, initialTok); err != nil {
		t.Fatalf("saving legacy token: %v", err)
	}

	// Make directory read-only so os.Remove(legacyPath) fails with permission error
	if err := os.Chmod(legacySubdir, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(legacySubdir, 0700)
	})

	canonicalPath := filepath.Join(tmpDir, "canonical_token.json")

	tok, err := migrateLegacyToken(canonicalPath, legacyPath)
	if err == nil {
		t.Fatal("expected migrateLegacyToken to fail when unlink fails, got nil error")
	}
	if tok != nil {
		t.Errorf("expected nil token on unlink failure, got %+v", tok)
	}
	if !strings.Contains(err.Error(), "failed to remove insecure legacy copy") {
		t.Errorf("expected error to mention failed removal of insecure copy, got: %v", err)
	}

	// Canonical token must have been successfully persisted before the remove step
	if _, err := os.Stat(canonicalPath); err != nil {
		t.Errorf("expected canonical token file %q to exist, got error: %v", canonicalPath, err)
	}

	// Legacy file still exists because unlink failed
	if _, err := os.Stat(legacyPath); err != nil {
		t.Errorf("expected legacy token file %q to still exist, got error: %v", legacyPath, err)
	}
}

func TestTokenSource_MigrationFailurePropagated(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &cfg_g.Config{Paths: cfg_g.Paths{ConfigDir: tmpDir}}

	legacyPath, err := GetLegacyTokenPath(cfg, "failing_acc")
	if err != nil {
		t.Fatal(err)
	}

	initialTok := &Token{
		AccessToken:  "legacy-token",
		RefreshToken: "legacy-refresh",
		Expiry:       time.Now().Add(1 * time.Hour),
	}
	if saveErr := saveToken(legacyPath, initialTok); saveErr != nil {
		t.Fatalf("saving legacy token: %v", saveErr)
	}

	// Create a read-only parent directory so creating canonicalDir in saveToken fails
	parentDir := filepath.Join(tmpDir, "readonly_parent")
	if mkdirErr := os.MkdirAll(parentDir, 0700); mkdirErr != nil {
		t.Fatal(mkdirErr)
	}
	if chmodErr := os.Chmod(parentDir, 0500); chmodErr != nil {
		t.Fatal(chmodErr)
	}
	t.Cleanup(func() {
		_ = os.Chmod(parentDir, 0700)
	})

	canonicalDir := filepath.Join(parentDir, "sub_canonical")
	ts := newTokenSource(cfg, "failing_acc")
	ts.tokenPath = filepath.Join(canonicalDir, "token.json")

	_, err = ts.TokenContext(context.Background())
	if err == nil {
		t.Fatal("expected TokenContext to fail when migration fails, got nil error")
	}
	if !strings.Contains(err.Error(), "failed to migrate legacy outlook token to secure storage") {
		t.Errorf("expected migration error to be surfaced directly, got: %v", err)
	}
}

type mockRoundTripper struct {
	roundTripFunc func(req *http.Request) (*http.Response, error)
}

func (m *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.roundTripFunc(req)
}

func TestOAuthTransport_RoundTrip_DecoupledContext(t *testing.T) {
	tmpDir := t.TempDir()
	tokenPath := filepath.Join(tmpDir, "token.json")
	initialTok := &Token{
		AccessToken:  "valid-token",
		RefreshToken: "refresh-tok",
		Expiry:       time.Now().Add(10 * time.Minute),
	}
	if err := saveToken(tokenPath, initialTok); err != nil {
		t.Fatal(err)
	}

	ts := &tokenSource{
		tokenPath: tokenPath,
		token:     initialTok,
	}

	baseCalled := false
	base := &mockRoundTripper{
		roundTripFunc: func(req *http.Request) (*http.Response, error) {
			baseCalled = true
			if auth := req.Header.Get("Authorization"); !strings.HasPrefix(auth, "Bearer ") {
				t.Errorf("expected Bearer token, got %q", auth)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       http.NoBody,
			}, nil
		},
	}

	tr := &oauthTransport{
		base:        base,
		tokenSource: ts,
	}

	reqCtx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, "https://graph.microsoft.com/v1.0/me", nil)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := tr.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip failed: %v", err)
	}
	_ = resp.Body.Close()

	if !baseCalled {
		t.Error("expected base round tripper to be called")
	}

	cancel()

	refreshCalled := false
	oldTokenURL := tokenURL
	defer func() { tokenURL = oldTokenURL }()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		refreshCalled = true
		if r.Context().Err() != nil {
			t.Errorf("expected token refresh request context to be active, got %v", r.Context().Err())
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(tokenResponse{
			AccessToken:  "refreshed-token",
			RefreshToken: "new-refresh",
			ExpiresIn:    3600,
		})
	}))
	defer srv.Close()
	tokenURL = srv.URL

	expiredTok := &Token{
		AccessToken:  "expired-token",
		RefreshToken: "refresh-tok",
		Expiry:       time.Now().Add(-5 * time.Minute),
	}
	tsExpired := &tokenSource{
		tokenPath: tokenPath,
		token:     expiredTok,
	}
	trExpired := &oauthTransport{
		base:        base,
		tokenSource: tsExpired,
	}

	reqExpired, _ := http.NewRequestWithContext(reqCtx, http.MethodGet, "https://graph.microsoft.com/v1.0/me", nil)
	_, _ = trExpired.RoundTrip(reqExpired)

	if !refreshCalled {
		t.Error("expected token refresh endpoint to be called despite canceled reqCtx")
	}
}

func TestOAuthTransport_RoundTrip_HostValidation(t *testing.T) {
	ts := &tokenSource{
		token: &Token{
			AccessToken: "valid-bearer-token",
			Expiry:      time.Now().Add(1 * time.Hour),
		},
	}

	var capturedAuth string
	base := &mockRoundTripper{
		roundTripFunc: func(req *http.Request) (*http.Response, error) {
			capturedAuth = req.Header.Get("Authorization")
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       http.NoBody,
			}, nil
		},
	}
	tr := &oauthTransport{
		base:        base,
		tokenSource: ts,
	}

	tests := []struct {
		name        string
		urlStr      string
		shouldPass  bool
		errContains string
	}{
		{
			name:       "valid graph host",
			urlStr:     "https://graph.microsoft.com/v1.0/me",
			shouldPass: true,
		},
		{
			name:       "valid graph host with port",
			urlStr:     "https://graph.microsoft.com:443/v1.0/me",
			shouldPass: true,
		},
		{
			name:       "valid graph host mixed case",
			urlStr:     "https://Graph.Microsoft.Com/v1.0/me",
			shouldPass: true,
		},
		{
			name:        "non-graph host rejected",
			urlStr:      "https://example.com/api",
			shouldPass:  false,
			errContains: `refusing to attach outlook credential to non-Graph host "example.com"`,
		},
		{
			name:        "subdomain of graph rejected",
			urlStr:      "https://attacker.graph.microsoft.com/v1.0/me",
			shouldPass:  false,
			errContains: `refusing to attach outlook credential to non-Graph host "attacker.graph.microsoft.com"`,
		},
		{
			name:        "microsoft online oauth rejected from graph transport",
			urlStr:      "https://login.microsoftonline.com/common/oauth2/v2.0/token",
			shouldPass:  false,
			errContains: `refusing to attach outlook credential to non-Graph host "login.microsoftonline.com"`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			capturedAuth = ""
			req, err := http.NewRequest(http.MethodGet, tc.urlStr, nil)
			if err != nil {
				t.Fatalf("failed to construct request: %v", err)
			}
			resp, err := tr.RoundTrip(req)
			if tc.shouldPass {
				if err != nil {
					t.Fatalf("expected request to succeed, got %v", err)
				}
				_ = resp.Body.Close()
				if capturedAuth != "Bearer valid-bearer-token" {
					t.Errorf("expected Bearer token attached, got %q", capturedAuth)
				}
				return
			}
			if err == nil {
				t.Fatal("expected request to fail, but it succeeded")
			}
			if !strings.Contains(err.Error(), tc.errContains) {
				t.Errorf("expected error containing %q, got %q", tc.errContains, err.Error())
			}
			if capturedAuth != "" {
				t.Errorf("base transport should not have been called, got capturedAuth %q", capturedAuth)
			}
		})
	}

	t.Run("nil URL rejected", func(t *testing.T) {
		req := &http.Request{}
		_, err := tr.RoundTrip(req)
		if err == nil || !strings.Contains(err.Error(), "refusing to attach outlook credential to non-Graph host") {
			t.Fatalf("expected error on nil URL, got %v", err)
		}
	})
}

// TestLegacyTokenFileName_UsesLegacySanitizer is a regression for a bug H4
// introduced: legacyTokenFileName reconstructs the pre-v0.6 unhashed token
// filename, which was necessarily written with whatever sanitizer existed
// at the time -- the old, collision-prone one. Computing it with the
// current SanitizeLabelForCache instead means GetLegacyTokenPath looks
// under a name nothing was ever written to for any account name holding a
// character outside [A-Za-z0-9_-], and the legacy-token fallback silently
// never finds a real legacy token.
// See oauth_migrate_test.go for the H4 token-sanitization-migration
// regression tests (TestLegacyTokenFileName_UsesLegacySanitizer,
// TestGetTokenPath_MigratesHashedTokenForward,
// TestTokenSource_LegacyFallback_SpecialCharacterAccountName).
