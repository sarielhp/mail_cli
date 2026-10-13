//go:build unix

package dirlock

import (
	"os"
	"syscall"
)

// lockFile takes an exclusive advisory lock, blocking until it is available.
// The kernel releases it when the descriptor closes, including on SIGKILL, so a
// crashed process cannot leave the cache permanently locked.
func lockFile(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
}

func unlockFile(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}
