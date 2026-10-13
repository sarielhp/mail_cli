package msg

import (
	"bytes"
	"fmt"
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"github.com/sarielhp/mail_cli/email"
	"sync"
	"testing"
	"time"
)

// Concurrent writers share one index; this is the case the in-process mutex and
// the directory lock exist for. Run with -race.
func TestConcurrentStoreAndClassify(t *testing.T) {
	dir := t.TempDir()
	const writers = 8
	const perWriter = 12

	var wg sync.WaitGroup
	for w := range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range perWriter {
				id := email.ID(fmt.Sprintf("w%02d-%03d", w, i))
				if err := Store(cachedir.At(dir), id, []byte(sampleRaw), time.Now()); err != nil {
					t.Errorf("Store(%s): %v", id, err)
					return
				}
				if err := SetEvidence(cachedir.At(dir), id, Evidence{BogoScore: float32(i)}); err != nil {
					t.Errorf("SetEvidence(%s): %v", id, err)
					return
				}
			}
		}()
	}
	wg.Wait()

	count := 0
	ForEachID(cachedir.At(dir), func(email.ID) bool {
		count++
		return false
	})
	if count != writers*perWriter {
		t.Errorf("index holds %d IDs after concurrent writes, want %d", count, writers*perWriter)
	}
}

// TestConcurrentStoreOfSameMessageNeverProducesAMixedBody is M6's
// regression: concurrent Store calls for the same message ID used to write
// their body through a shared, unlocked "<id>.eml.tmp" name, so one writer's
// truncate-and-write could interleave with another's and publish a body that
// was neither writer's -- bytes from both, or a short read. Run with -race.
func TestConcurrentStoreOfSameMessageNeverProducesAMixedBody(t *testing.T) {
	dir := t.TempDir()
	const writers = 8

	// Same length so a corrupted interleave can't be caught merely by a
	// length mismatch -- it has to actually be one writer's full,
	// unmodified body.
	bodies := make([][]byte, writers)
	for w := range writers {
		bodies[w] = []byte(fmt.Sprintf("Subject: writer %02d\r\n\r\nBody content from writer %02d.", w, w))
	}

	var wg sync.WaitGroup
	for w := range writers {
		wg.Add(1)
		go func(body []byte) {
			defer wg.Done()
			if err := Store(cachedir.At(dir), "contested", body, time.Now()); err != nil {
				t.Errorf("Store: %v", err)
			}
		}(bodies[w])
	}
	wg.Wait()

	got, err := Read(cachedir.At(dir), "contested")
	if err != nil {
		t.Fatalf("Read after concurrent Store: %v", err)
	}
	matched := false
	for _, body := range bodies {
		if bytes.Equal(got, body) {
			matched = true
			break
		}
	}
	if !matched {
		t.Errorf("final body matches none of the %d writers' bodies (corrupted interleave): %q", writers, got)
	}
}

// ForEachID reads while another goroutine writes; both take the same lock.
func TestConcurrentReadDuringWrite(t *testing.T) {
	dir := t.TempDir()
	mustStore(t, dir, "seed", time.Now())

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := range 50 {
			if err := Store(cachedir.At(dir), email.ID(fmt.Sprintf("p%03d", i)), []byte(sampleRaw), time.Now()); err != nil {
				t.Errorf("Store: %v", err)
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		for range 50 {
			ForEachID(cachedir.At(dir), func(email.ID) bool { return false })
		}
	}()
	wg.Wait()
}
