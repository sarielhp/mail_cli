// Package dirlock serializes read-modify-write access to a cache directory.
//
// The on-disk cache is a set of whole-file JSON indexes updated by
// read-modify-write: cache/label's Add, Remove and Move each read a folder's
// full ID list, change it in memory, and write the whole list back. Nothing
// coordinated those updates, so two writers each read the pre-update list and
// the second write silently clobbered the first. The atomic temp-file-plus-
// rename inside the writers prevents a garbled file; it does nothing about a
// lost update.
//
// Two writers are easy to produce. Within one process, each TUI action runs on
// its own tea.Cmd goroutine, so archiving two messages in quick succession
// races. Across processes, the documented daily pattern -- a long-running TUI
// alongside a scheduled scan -- shares one download directory.
//
// Guard therefore takes both an in-process mutex and an advisory file lock, so
// it is correct against goroutines and against separate mail_cli processes.
package dirlock

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

const lockFileName = ".cache.lock"

type dirLock struct {
	mu    sync.Mutex
	owner int64
	depth int
	f     *os.File
}

var locks sync.Map // downloadDir -> *dirLock

func lockFor(dir string) *dirLock {
	l, _ := locks.LoadOrStore(dir, &dirLock{})
	return l.(*dirLock)
}

func curGoroutineID() int64 {
	var buf [64]byte
	n := runtime.Stack(buf[:], false)
	s := strings.TrimPrefix(string(buf[:n]), "goroutine ")
	idx := strings.IndexByte(s, ' ')
	if idx < 0 {
		return 0
	}
	id, _ := strconv.ParseInt(s[:idx], 10, 64)
	return id
}

func (dl *dirLock) release() {
	dl.depth--
	if dl.depth > 0 {
		return
	}
	if dl.f != nil {
		_ = unlockFile(dl.f)
		_ = dl.f.Close()
		dl.f = nil
	}
	atomic.StoreInt64(&dl.owner, 0)
	dl.mu.Unlock()
}

func (dl *dirLock) wrapRelease() func() {
	var once sync.Once
	return func() {
		once.Do(dl.release)
	}
}

// Guard acquires exclusive access to downloadDir and returns a release
// function. The release function is always non-nil and safe to call once, so
// callers can `defer release()` without checking the error first.
//
// Reentrant calls from the same goroutine increment the lock depth and do
// not block or deadlock.
//
// A failure to take the cross-process lock is not fatal: the in-process mutex is
// still held, which preserves correctness within this process and degrades to
// the previous behavior between processes rather than failing a user's command.
func Guard(downloadDir string) (release func(), err error) {
	dl := lockFor(downloadDir)
	gid := curGoroutineID()

	if gid != 0 && atomic.LoadInt64(&dl.owner) == gid {
		dl.depth++
		return dl.wrapRelease(), nil
	}

	dl.mu.Lock()
	atomic.StoreInt64(&dl.owner, gid)
	dl.depth = 1

	if downloadDir == "" {
		return dl.wrapRelease(), nil
	}
	if mkErr := os.MkdirAll(downloadDir, 0700); mkErr != nil {
		return dl.wrapRelease(), mkErr
	}

	f, openErr := os.OpenFile(filepath.Join(downloadDir, lockFileName), os.O_CREATE|os.O_RDWR, 0600)
	if openErr != nil {
		return dl.wrapRelease(), openErr
	}
	if lockErr := lockFile(f); lockErr != nil {
		_ = f.Close()
		return dl.wrapRelease(), lockErr
	}
	dl.f = f
	return dl.wrapRelease(), nil
}

// IsLockFile reports whether name is the lock file, so cache-clearing code can
// leave it alone.
func IsLockFile(name string) bool {
	return filepath.Base(name) == lockFileName
}
