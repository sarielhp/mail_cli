package app

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"
)

// DefaultStaleTempDirAge is the default threshold beyond which temp directories are swept.
const DefaultStaleTempDirAge = 30 * time.Minute

// GetTempDir returns a secure, unpredictably-named temporary directory with mode 0700.
// It automatically sweeps stale temporary directories older than DefaultStaleTempDirAge.
func GetTempDir() (string, error) {
	_ = CleanStaleTempDirs(DefaultStaleTempDirAge)
	username := resolveUsername()
	pattern := fmt.Sprintf("%s-%s-*", AppName, sanitizeUsername(username))
	return os.MkdirTemp("", pattern)
}

// CleanStaleTempDirs removes temporary directories created by this app for the current user
// that are older than maxAge.
func CleanStaleTempDirs(maxAge time.Duration) error {
	username := resolveUsername()
	prefix := fmt.Sprintf("%s-%s-", AppName, sanitizeUsername(username))
	tempBase := os.TempDir()
	entries, err := os.ReadDir(tempBase)
	if err != nil {
		return err
	}
	now := time.Now()
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), prefix) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if now.Sub(info.ModTime()) > maxAge {
			_ = os.RemoveAll(filepath.Join(tempBase, entry.Name()))
		}
	}
	return nil
}

func resolveUsername() string {
	currUser, err := user.Current()
	if err == nil && currUser.Username != "" {
		return currUser.Username
	}
	if u := os.Getenv("USER"); u != "" {
		return u
	}
	return "default"
}

func sanitizeUsername(username string) string {
	res := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, username)
	if res == "" {
		return "user"
	}
	return res
}
