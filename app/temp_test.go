package app

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestGetTempDir(t *testing.T) {
	dir1, err := GetTempDir()
	if err != nil {
		t.Fatalf("GetTempDir() failed: %v", err)
	}
	defer os.RemoveAll(dir1)

	info1, err := os.Stat(dir1)
	if err != nil {
		t.Fatalf("Stat(dir1) failed: %v", err)
	}
	if !info1.IsDir() {
		t.Fatalf("expected dir1 to be a directory, got %v", info1.Mode())
	}

	if runtime.GOOS != "windows" {
		perm := info1.Mode().Perm()
		if perm != 0700 {
			t.Errorf("expected permissions 0700, got %04o", perm)
		}
	}

	dir2, err := GetTempDir()
	if err != nil {
		t.Fatalf("second GetTempDir() failed: %v", err)
	}
	defer os.RemoveAll(dir2)

	if dir1 == dir2 {
		t.Errorf("expected distinct unpredictable directories, got duplicate: %s", dir1)
	}
}

func TestSanitizeUsername(t *testing.T) {
	// Every result must be usable as a single path segment: the sanitized name
	// is joined into the temp directory, so a separator surviving here would put
	// the app's directory outside the base it chose.
	tests := []struct {
		username string
		want     string
		why      string
	}{
		{"alice", "alice", "a plain name is already safe"},
		{"bob_123", "bob_123", "digits and underscores survive"},
		{"john-doe", "john-doe", "hyphens survive"},
		{"user/with/slashes", "user_with_slashes", "a separator becomes an underscore"},
		{"user..name", "user__name", "a dot pair becomes underscores too"},
		{"", "user", "an empty name falls back to a literal"},
		{"   ", "___", "whitespace is replaced, not trimmed away"},
	}

	for i, tc := range tests {
		t.Run(fmt.Sprintf("case %d", i), func(t *testing.T) {
			got := sanitizeUsername(tc.username)
			if got != tc.want {
				t.Errorf("sanitizeUsername(%q) = %q, want %q (%s)", tc.username, got, tc.want, tc.why)
			}
			if strings.ContainsRune(got, os.PathSeparator) {
				t.Errorf("sanitizeUsername(%q) = %q still carries a path separator", tc.username, got)
			}
		})
	}
}

func TestCleanStaleTempDirs(t *testing.T) {
	username := resolveUsername()
	prefix := fmt.Sprintf("%s-%s-", AppName, sanitizeUsername(username))
	tempBase := os.TempDir()

	// 1. Stale directory (older than 1 hour)
	staleDir, err := os.MkdirTemp(tempBase, prefix+"stale-*")
	if err != nil {
		t.Fatalf("failed to create stale test temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(staleDir) }()

	past := time.Now().Add(-2 * time.Hour)
	if chtimesErr := os.Chtimes(staleDir, past, past); chtimesErr != nil {
		t.Fatalf("failed to set past mod time: %v", chtimesErr)
	}

	// 2. Fresh directory (recent)
	freshDir, err := os.MkdirTemp(tempBase, prefix+"fresh-*")
	if err != nil {
		t.Fatalf("failed to create fresh test temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(freshDir) }()

	// 3. Other prefix directory (should not be touched even if old)
	otherDir, err := os.MkdirTemp(tempBase, "otherapp-otheruser-*")
	if err != nil {
		t.Fatalf("failed to create other test temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(otherDir) }()
	if err := os.Chtimes(otherDir, past, past); err != nil {
		t.Fatalf("failed to set other past mod time: %v", err)
	}

	// Run cleanup with 1 hour threshold
	if err := CleanStaleTempDirs(1 * time.Hour); err != nil {
		t.Fatalf("CleanStaleTempDirs failed: %v", err)
	}

	// Stale directory should be gone
	if _, err := os.Stat(staleDir); !os.IsNotExist(err) {
		t.Errorf("expected stale dir %q to be deleted, but it still exists", staleDir)
	}

	// Fresh directory should remain
	if _, err := os.Stat(freshDir); err != nil {
		t.Errorf("expected fresh dir %q to be kept, but stat returned: %v", freshDir, err)
	}

	// Other directory should remain untouched
	if _, err := os.Stat(otherDir); err != nil {
		t.Errorf("expected other dir %q to be untouched, but stat returned: %v", otherDir, err)
	}
}
