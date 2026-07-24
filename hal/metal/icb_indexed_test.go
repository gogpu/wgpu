// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build darwin && !(js && wasm)

package metal

import (
	"sync"
	"testing"

	"github.com/gogpu/wgpu/hal"
)

func TestIndexedICBArenaCapacityIsGeometricAndBounded(t *testing.T) {
	const max = uint32(52428)
	for _, test := range []struct {
		count uint32
		want  uint32
	}{
		{1024, 1024},
		{1025, 2048},
		{10000, 16384},
		{max, max},
		{max + 1, 0},
	} {
		if got := indexedICBArenaCapacity(test.count, max); got != test.want {
			t.Fatalf("capacity(%d) = %d, want %d", test.count, got, test.want)
		}
	}
}

func TestIndexedICBCountGate(t *testing.T) {
	for count, want := range map[uint32]bool{
		0: false, 1: false, 20: false, 1023: false, 1024: true,
		indexedICBMaxCommands: true, indexedICBMaxCommands + 1: false,
	} {
		if got := indexedICBCountEligible(count); got != want {
			t.Fatalf("eligible(%d) = %v, want %v", count, got, want)
		}
	}
}

func TestIndexedICBOwnershipReleasesExactlyOnceAcrossRaces(t *testing.T) {
	var mu sync.Mutex
	released := make(map[ID]int)
	owner := &indexedICBOwnership{releaseObject: func(id ID) {
		mu.Lock()
		released[id]++
		mu.Unlock()
	}}
	for _, id := range []ID{1, 2, 3} {
		if !owner.retainOwned(id) {
			t.Fatalf("failed to retain object %d", id)
		}
	}

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			owner.release()
		}()
	}
	wg.Wait()
	for _, id := range []ID{1, 2, 3} {
		if released[id] != 1 {
			t.Fatalf("object %d released %d times, want once", id, released[id])
		}
	}
}

func TestCommandEncoderResetAllReleasesFinishedICBOwnership(t *testing.T) {
	releases := 0
	owner := &indexedICBOwnership{releaseObject: func(ID) { releases++ }}
	owner.retainOwned(1)
	cb := &CommandBuffer{icbOwners: []*indexedICBOwnership{owner}}
	encoder := &CommandEncoder{finished: cb}

	encoder.ResetAll(nil)
	encoder.ResetAll([]hal.CommandBuffer{cb})
	if releases != 1 {
		t.Fatalf("release count = %d, want one", releases)
	}
	if encoder.finished != nil || cb.icbOwners != nil {
		t.Fatal("finished ICB ownership was retained after reset")
	}
}

func TestCommandBufferDestroyReleasesExactlyOnceAcrossRaces(t *testing.T) {
	var mu sync.Mutex
	releases := 0
	owner := &indexedICBOwnership{releaseObject: func(ID) {
		mu.Lock()
		releases++
		mu.Unlock()
	}}
	owner.retainOwned(1)
	cb := &CommandBuffer{icbOwners: []*indexedICBOwnership{owner}}

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cb.Destroy()
		}()
	}
	wg.Wait()
	if releases != 1 {
		t.Fatalf("release count = %d, want one", releases)
	}
}
