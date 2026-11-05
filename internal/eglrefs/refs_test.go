// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package eglrefs

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

func TestRefsZeroValueHoldsTheCreatorsReference(t *testing.T) {
	var r Refs
	if !r.Alive() {
		t.Fatal("zero Refs is not alive")
	}
	last, err := r.Release()
	if err != nil || !last {
		t.Fatalf("Release of the only reference = (%v, %v), want (true, nil)", last, err)
	}
	if r.Alive() {
		t.Fatal("Refs alive after its last reference was released")
	}
}

// TestRefsStateMachine walks the surface and device references of an EGL
// context: the surface holds the first one, the device acquires its own, and
// the context dies exactly once, with the last of them.
func TestRefsStateMachine(t *testing.T) {
	var r Refs // the surface's reference
	if !r.Acquire() {
		t.Fatal("device Acquire on a live context failed")
	}

	// Surface released first: the device keeps the context alive.
	if last, err := r.Release(); err != nil || last {
		t.Fatalf("surface Release = (%v, %v), want (false, nil)", last, err)
	}
	if !r.Alive() {
		t.Fatal("context died while the device still holds it")
	}

	// A weak holder may still take a temporary reference.
	if !r.Acquire() {
		t.Fatal("weak Acquire on a live context failed")
	}
	if last, err := r.Release(); err != nil || last {
		t.Fatalf("weak Release = (%v, %v), want (false, nil)", last, err)
	}

	// Device released: last reference.
	if last, err := r.Release(); err != nil || !last {
		t.Fatalf("device Release = (%v, %v), want (true, nil)", last, err)
	}

	// Acquire after zero fails: the context never comes back.
	if r.Acquire() {
		t.Fatal("Acquire after the last Release succeeded")
	}
	if r.Alive() {
		t.Fatal("dead Refs reports alive")
	}

	// A release after death is a bug, reported, never a second "last".
	last, err := r.Release()
	if !errors.Is(err, ErrNotAcquired) || last {
		t.Fatalf("Release after death = (%v, %v), want (false, ErrNotAcquired)", last, err)
	}
}

// TestRefsConcurrentAcquireRelease checks that exactly one Release reports
// the last reference however acquires and releases interleave, and that no
// Acquire succeeds after it.
func TestRefsConcurrentAcquireRelease(t *testing.T) {
	var r Refs
	var lasts atomic.Int32
	var wg sync.WaitGroup
	for range 32 {
		wg.Go(func() {
			for range 200 {
				if !r.Acquire() {
					return
				}
				last, err := r.Release()
				if err != nil {
					t.Errorf("Release: %v", err)
					return
				}
				if last {
					lasts.Add(1)
				}
			}
		})
	}
	wg.Wait()
	if lasts.Load() != 0 {
		t.Fatalf("%d temporary releases reported the last reference", lasts.Load())
	}
	if last, err := r.Release(); err != nil || !last {
		t.Fatalf("creator Release = (%v, %v), want (true, nil)", last, err)
	}
	if r.Acquire() {
		t.Fatal("Acquire after death succeeded")
	}
}
