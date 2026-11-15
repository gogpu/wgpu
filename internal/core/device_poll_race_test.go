//go:build !(js && wasm)

package core

import (
	"sync"
	"testing"
)

// TestMapTrackerConcurrentFirstUse pins the lazy tracker initialization: concurrent first
// registrations on one Device must all observe one tracker. Two trackers would let a buffer
// register in a tracker nobody polls, so its map would never resolve. Under -race this also
// pins that the slot's lazy first use is synchronized (the unsynchronized write it replaced
// was flagged by the detector).
func TestMapTrackerConcurrentFirstUse(t *testing.T) {
	var d Device
	const workers = 8
	trackers := make([]*deviceMapTracker, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			trackers[i] = d.mapTracker()
		}(i)
	}
	wg.Wait()
	for i, got := range trackers {
		if got == nil {
			t.Fatalf("worker %d got a nil tracker", i)
		}
		if got != trackers[0] {
			t.Fatalf("mapTracker returned distinct trackers for one Device (worker 0 vs %d): a registration can land in a tracker nobody polls", i)
		}
	}
}
