//go:build !unix && !windows

package dirlock

import (
	"errors"
	"os"
)

// ErrLockUnsupported is kept for backwards compatibility when referencing
// unsupported locking capabilities.
var ErrLockUnsupported = errors.New("cross-process cache locking unsupported")

func lockFile(_ *os.File) error {
	return nil // Guard's process-local mutex remains authoritative.
}

func unlockFile(_ *os.File) error {
	return nil
}
