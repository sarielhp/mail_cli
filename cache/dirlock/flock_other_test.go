//go:build !unix && !windows

package dirlock

import (
	"os"
	"testing"
)

func TestLockFileUnsupportedFallback(t *testing.T) {
	dir := t.TempDir()
	f, err := os.CreateTemp(dir, "lock")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer f.Close()

	if err := lockFile(f); err != nil {
		t.Errorf("lockFile failed: %v", err)
	}
	if err := unlockFile(f); err != nil {
		t.Errorf("unlockFile failed: %v", err)
	}
}
