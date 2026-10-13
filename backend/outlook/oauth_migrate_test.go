package outlook

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sarielhp/mail_cli/cfg_g"
)

// TestLegacyTokenFileName_UsesLegacySanitizer is a regression for a bug H4
// introduced: legacyTokenFileName reconstructs the pre-v0.6 unhashed token
// filename, which was necessarily written with whatever sanitizer existed
// at the time -- the old, collision-prone one. Computing it with the
// current SanitizeLabelForCache instead means GetLegacyTokenPath looks
// under a name nothing was ever written to for any account name holding a
// character outside [A-Za-z0-9_-], and the legacy-token fallback silently
// never finds a real legacy token.
func TestLegacyTokenFileName_UsesLegacySanitizer(t *testing.T) {
	accName := "corp.com"
	got := legacyTokenFileName(accName)
	want := fmt.Sprintf("outlook_token_%s.json", cfg_g.LegacySanitizeLabelForCache(accName))
	if got != want {
		t.Errorf("legacyTokenFileName(%q) = %q, want %q (the legacy-sanitized form)", accName, got, want)
	}
	if got == fmt.Sprintf("outlook_token_%s.json", cfg_g.SanitizeLabelForCache(accName)) {
		t.Fatalf("test fixture invalid: %q sanitizes the same both ways", accName)
	}
}

// TestGetTokenPath_MigratesHashedTokenForward is H4's regression for the
// *current* (hash-suffixed) token scheme: tokenFileName's "safe" prefix
// also shifted for an account name holding a character outside
// [A-Za-z0-9_-], so a token saved under the old prefix before this fix
// would otherwise look missing after it, forcing an unnecessary
// re-authentication. GetTokenPath must find and move it forward itself.
func TestGetTokenPath_MigratesHashedTokenForward(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &cfg_g.Config{Paths: cfg_g.Paths{ConfigDir: tmpDir}}
	accName := "corp.com"

	oldPath := filepath.Join(tmpDir, legacyHashedTokenFileName(accName))
	tok := &Token{AccessToken: "old-scheme-token", Expiry: time.Now().Add(time.Hour)}
	if err := saveToken(oldPath, tok); err != nil {
		t.Fatalf("seeding old-scheme token: %v", err)
	}

	newPath, err := GetTokenPath(cfg, accName)
	if err != nil {
		t.Fatalf("GetTokenPath: %v", err)
	}
	if newPath == oldPath {
		t.Fatalf("test fixture invalid: %q sanitizes the same both ways", accName)
	}

	loaded, err := loadToken(newPath)
	if err != nil {
		t.Fatalf("expected the old-scheme token migrated to %q: %v", newPath, err)
	}
	if loaded.AccessToken != "old-scheme-token" {
		t.Errorf("migrated token content mismatch: got %q", loaded.AccessToken)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Errorf("expected old-scheme token file removed, stat err = %v", err)
	}
}

// TestTokenSource_LegacyFallback_SpecialCharacterAccountName exercises the
// full pre-v0.6 unhashed-scheme fallback (TestTokenSource_LegacyFallback in
// oauth_test.go) for an account name that actually needs the legacy
// sanitizer to be found -- "my_acc" in that test sanitizes identically both
// ways and so never exercised the bug legacyTokenFileName had.
func TestTokenSource_LegacyFallback_SpecialCharacterAccountName(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &cfg_g.Config{Paths: cfg_g.Paths{ConfigDir: tmpDir}}
	accName := "corp.com"

	legacyPath, err := GetLegacyTokenPath(cfg, accName)
	if err != nil {
		t.Fatal(err)
	}
	canonicalPath, err := GetTokenPath(cfg, accName)
	if err != nil {
		t.Fatal(err)
	}
	if legacyPath == canonicalPath {
		t.Fatalf("test fixture invalid: legacy and canonical paths coincide")
	}

	initialTok := &Token{
		AccessToken:  "legacy-access-token",
		RefreshToken: "legacy-refresh-token",
		Expiry:       time.Now().Add(1 * time.Hour),
	}
	if saveErr := saveToken(legacyPath, initialTok); saveErr != nil {
		t.Fatalf("saving legacy token: %v", saveErr)
	}

	ts := newTokenSource(cfg, accName)
	tok, err := ts.Token()
	if err != nil {
		t.Fatalf("Token() failed with fallback: %v", err)
	}
	if tok.AccessToken != "legacy-access-token" {
		t.Errorf("expected AccessToken %q, got %q", "legacy-access-token", tok.AccessToken)
	}
	if _, err := os.Stat(canonicalPath); err != nil {
		t.Errorf("expected canonical token file to be created: %v", err)
	}
}
