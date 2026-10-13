// Package zonetest waits for bubblezone to record a zone a view has drawn.
//
// It is a leaf test helper: it knows only about bubblezone, so both the
// filebrowser and the tui tests can share one way of resolving a zone's
// coordinates without either importing the other.
package zonetest

import (
	"testing"
	"time"

	zone "github.com/lrstanley/bubblezone"
)

// WaitForZone draws with render and returns where it put zone id. bubblezone
// records zones on a worker goroutine, so a read straight after drawing can
// see where an earlier frame -- or an earlier test -- put it, which made
// clicks miss under -race. Updates are handled in order, so clearing,
// drawing and waiting twice leaves only the current frame's.
func WaitForZone(t *testing.T, render func(), id string) *zone.ZoneInfo {
	t.Helper()
	for range 2 {
		zone.Clear(id)
		render()
		deadline := time.Now().Add(2 * time.Second)
		for zone.Get(id).IsZero() {
			if time.Now().After(deadline) {
				t.Fatalf("zone %q was never drawn", id)
			}
			time.Sleep(time.Millisecond)
		}
	}
	return zone.Get(id)
}
