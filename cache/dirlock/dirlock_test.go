package dirlock

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// dirlock serializes read-modify-write access to a cache directory. The cache
// indexes are whole-file rewrites, so two writers without it lose an update --
// which is what this exists to prevent and what nothing was checking.

func TestGuardSerializesWriters(t *testing.T) {
	dir := t.TempDir()

	var mu sync.Mutex
	var concurrent, peak int

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			release, err := Guard(dir)
			if err != nil {
				t.Errorf("Guard: %v", err)
				return
			}
			defer release()

			mu.Lock()
			concurrent++
			if concurrent > peak {
				peak = concurrent
			}
			mu.Unlock()

			time.Sleep(time.Millisecond)

			mu.Lock()
			concurrent--
			mu.Unlock()
		}()
	}
	wg.Wait()

	if peak != 1 {
		t.Errorf("%d goroutines held the guard at once; it must serialize them", peak)
	}
}

// The release function is documented as always non-nil and safe to defer
// without checking the error first, which callers rely on.
func TestGuardAlwaysReturnsAReleaseFunction(t *testing.T) {
	tests := []struct {
		name string
		dir  func(t *testing.T) string
	}{
		{name: "ordinary directory", dir: func(t *testing.T) string { return t.TempDir() }},
		{name: "empty path", dir: func(_ *testing.T) string { return "" }},
		{
			name: "directory that cannot be created",
			dir: func(t *testing.T) string {
				// A path under a regular file cannot be made into a directory.
				f := filepath.Join(t.TempDir(), "file")
				if err := os.WriteFile(f, []byte("x"), 0600); err != nil {
					t.Fatalf("seeding: %v", err)
				}
				return filepath.Join(f, "under-a-file")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			release, _ := Guard(tc.dir(t))
			if release == nil {
				t.Fatal("Guard returned a nil release function; callers defer it unconditionally")
			}
			release()
		})
	}
}

// A failure to take the cross-process lock still holds the in-process mutex,
// so correctness within this process is preserved rather than abandoned.
func TestGuardStillSerializesWhenTheLockFileCannotBeMade(t *testing.T) {
	f := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(f, []byte("x"), 0600); err != nil {
		t.Fatalf("seeding: %v", err)
	}
	dir := filepath.Join(f, "under-a-file")

	var mu sync.Mutex
	var concurrent, peak int

	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			release, _ := Guard(dir)
			defer release()

			mu.Lock()
			concurrent++
			if concurrent > peak {
				peak = concurrent
			}
			mu.Unlock()
			time.Sleep(time.Millisecond)
			mu.Lock()
			concurrent--
			mu.Unlock()
		}()
	}
	wg.Wait()

	if peak != 1 {
		t.Errorf("peak concurrency %d without a usable lock file; the in-process mutex must still apply", peak)
	}
}

func TestGuardCreatesTheLockFile(t *testing.T) {
	dir := t.TempDir()
	release, err := Guard(dir)
	if err != nil {
		t.Fatalf("Guard: %v", err)
	}
	release()

	if _, err := os.Stat(filepath.Join(dir, ".cache.lock")); err != nil {
		t.Errorf("no lock file after Guard: %v", err)
	}
}

// Different directories are independent locks, or one account's cache would
// block another's.
func TestGuardIsPerDirectory(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()

	releaseA, err := Guard(a)
	if err != nil {
		t.Fatalf("Guard(a): %v", err)
	}
	defer releaseA()

	done := make(chan struct{})
	go func() {
		releaseB, err := Guard(b)
		if err == nil {
			releaseB()
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Error("holding one directory's guard blocked another directory")
	}
}

// Cache-clearing code has to leave the lock file alone.
func TestIsLockFile(t *testing.T) {
	tests := []struct {
		name string
		path string
		want bool
		note string
	}{
		{name: "bare name", path: ".cache.lock", want: true, note: "the lock sitting at the cache root"},
		{name: "full path", path: "/home/x/.cache/mail_cli/acc/.cache.lock", want: true, note: "the lock for one account's cache"},
		{name: "a message", path: "/home/x/messages/2025/01/02/abc.eml", want: false, note: "a downloaded message body"},
		{name: "the index", path: "message_info.bin", want: false, note: "the index, which a wipe is allowed to rewrite"},
		{name: "empty", path: "", want: false, note: "no path at all"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := IsLockFile(tc.path)
			if got == tc.want {
				return
			}
			t.Errorf("%s: IsLockFile(%q) = %v, want %v", tc.note, tc.path, got, tc.want)
		})
	}
}

func TestAtomicWrite(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "sub", "test.json")
	data := []byte(`{"hello":"world"}`)

	if err := AtomicWrite(target, data, 0600); err != nil {
		t.Fatalf("AtomicWrite: %v", err)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != string(data) {
		t.Errorf("got %q, want %q", got, data)
	}
}

func TestAtomicWriteConcurrent(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "concurrent.json")

	var wg sync.WaitGroup
	for i := range 10 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			payload := []byte(string(rune('0' + n)))
			if err := AtomicWrite(target, payload, 0600); err != nil {
				t.Errorf("AtomicWrite %d: %v", n, err)
			}
		}(i)
	}
	wg.Wait()

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("target file is empty after concurrent writes")
	}
}

func TestAtomicWriteNestedDirectory(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "level1", "level2", "level3", "state.json")
	payload := []byte(`{"persisted":true}`)

	if err := AtomicWrite(target, payload, 0600); err != nil {
		t.Fatalf("AtomicWrite deeply nested: %v", err)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != string(payload) {
		t.Errorf("got %q, want %q", got, payload)
	}
}

func TestMkdirSyncedExisting(t *testing.T) {
	dir := t.TempDir()
	if err := mkdirSynced(dir); err != nil {
		t.Fatalf("mkdirSynced on existing dir: %v", err)
	}
}

func TestGuardReentrant(t *testing.T) {
	dir := t.TempDir()

	rel1, err := Guard(dir)
	if err != nil {
		t.Fatalf("outer Guard failed: %v", err)
	}
	defer rel1()

	rel2, err := Guard(dir)
	if err != nil {
		t.Fatalf("nested Guard failed: %v", err)
	}
	rel2()

	rel3, err := Guard(dir)
	if err != nil {
		t.Fatalf("second nested Guard failed: %v", err)
	}
	rel3()
}
