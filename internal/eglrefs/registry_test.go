// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package eglrefs

import (
	"errors"
	"sync"
	"testing"
)

// fakeDisplays records the initialize and terminate calls a Registry makes,
// standing in for eglInitialize and eglTerminate.
type fakeDisplays struct {
	mu          sync.Mutex
	initialized map[uintptr]int // initialize calls per display
	terminated  map[uintptr]int // terminate calls per display
	failInit    map[uintptr]bool
}

func newFakeDisplays() *fakeDisplays {
	return &fakeDisplays{
		initialized: map[uintptr]int{},
		terminated:  map[uintptr]int{},
		failInit:    map[uintptr]bool{},
	}
}

var errInitFailed = errors.New("fake eglInitialize failed")

func (f *fakeDisplays) initialize(d uintptr) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failInit[d] {
		return 0, errInitFailed
	}
	f.initialized[d]++
	return 15, nil // "EGL 1.5"
}

func (f *fakeDisplays) terminate(d uintptr) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.terminated[d]++
	return nil
}

func (f *fakeDisplays) terminations(d uintptr) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.terminated[d]
}

func newTestRegistry(f *fakeDisplays) *Registry[uintptr, int] {
	return NewRegistry(f.initialize, f.terminate)
}

// count returns the users of display.
func (r *Registry[K, V]) count(display K) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.counts[display]
}

func TestRegistryTerminatesOnLastRelease(t *testing.T) {
	f := newFakeDisplays()
	r := newTestRegistry(f)
	const d = 0x10

	for range 2 {
		v, err := r.Acquire(d)
		if err != nil {
			t.Fatalf("Acquire: %v", err)
		}
		if v != 15 {
			t.Fatalf("Acquire returned %d, want the initialize result 15", v)
		}
	}
	// Every acquire initializes, as eglInitialize on an initialized display
	// only returns the version (Rust initialize_display).
	if got := f.initialized[d]; got != 2 {
		t.Fatalf("initialize calls = %d, want 2", got)
	}

	if err := r.Release(d); err != nil {
		t.Fatalf("first Release: %v", err)
	}
	if got := f.terminations(d); got != 0 {
		t.Fatalf("terminate after the first of two releases = %d, want 0", got)
	}
	if err := r.Release(d); err != nil {
		t.Fatalf("second Release: %v", err)
	}
	if got := f.terminations(d); got != 1 {
		t.Fatalf("terminate after the last release = %d, want 1", got)
	}
	if got := r.count(d); got != 0 {
		t.Fatalf("count after the last release = %d, want 0", got)
	}
}

func TestRegistryDisplaysAreIndependent(t *testing.T) {
	f := newFakeDisplays()
	r := newTestRegistry(f)
	const a, b = 0x10, 0x20

	for _, d := range []uintptr{a, b} {
		if _, err := r.Acquire(d); err != nil {
			t.Fatalf("Acquire(%#x): %v", d, err)
		}
	}
	if err := r.Release(a); err != nil {
		t.Fatalf("Release(a): %v", err)
	}
	if f.terminations(a) != 1 || f.terminations(b) != 0 {
		t.Fatalf("terminations a=%d b=%d, want 1 and 0", f.terminations(a), f.terminations(b))
	}
	if err := r.Release(b); err != nil {
		t.Fatalf("Release(b): %v", err)
	}
	if f.terminations(b) != 1 {
		t.Fatalf("terminations b=%d, want 1", f.terminations(b))
	}
}

func TestRegistryInitializeFailureRollsBack(t *testing.T) {
	f := newFakeDisplays()
	r := newTestRegistry(f)
	const d = 0x30

	// A first user holds the display.
	if _, err := r.Acquire(d); err != nil {
		t.Fatalf("Acquire: %v", err)
	}

	f.failInit[d] = true
	if _, err := r.Acquire(d); !errors.Is(err, errInitFailed) {
		t.Fatalf("Acquire with a failing initialize = %v, want errInitFailed", err)
	}
	if got := r.count(d); got != 1 {
		t.Fatalf("count after the failed acquire = %d, want 1 (rolled back)", got)
	}
	if got := f.terminations(d); got != 0 {
		t.Fatalf("a failed initialize terminated the display %d times", got)
	}

	// The failed acquire left no reference behind: one release terminates.
	if err := r.Release(d); err != nil {
		t.Fatalf("Release: %v", err)
	}
	if got := f.terminations(d); got != 1 {
		t.Fatalf("terminations = %d, want 1", got)
	}

	// A failure on a display nobody holds leaves no entry at all.
	const e = 0x40
	f.failInit[e] = true
	if _, err := r.Acquire(e); err == nil {
		t.Fatal("Acquire with a failing initialize returned no error")
	}
	if _, ok := r.counts[e]; ok {
		t.Fatal("a failed first acquire left a registry entry")
	}
}

func TestRegistryReleaseWithoutAcquireIsAnError(t *testing.T) {
	f := newFakeDisplays()
	r := newTestRegistry(f)
	const d = 0x50

	if err := r.Release(d); !errors.Is(err, ErrNotAcquired) {
		t.Fatalf("Release without Acquire = %v, want ErrNotAcquired", err)
	}
	if got := f.terminations(d); got != 0 {
		t.Fatalf("a release without an acquire terminated the display %d times", got)
	}

	// Releasing once more than acquired is the same bug.
	if _, err := r.Acquire(d); err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if err := r.Release(d); err != nil {
		t.Fatalf("Release: %v", err)
	}
	if err := r.Release(d); !errors.Is(err, ErrNotAcquired) {
		t.Fatalf("extra Release = %v, want ErrNotAcquired", err)
	}
	if got := f.terminations(d); got != 1 {
		t.Fatalf("terminations = %d, want 1", got)
	}
}

func TestRegistryTerminateErrorIsReturned(t *testing.T) {
	errTerm := errors.New("fake eglTerminate failed")
	r := NewRegistry(
		func(uintptr) (int, error) { return 0, nil },
		func(uintptr) error { return errTerm },
	)
	if _, err := r.Acquire(1); err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if err := r.Release(1); !errors.Is(err, errTerm) {
		t.Fatalf("Release = %v, want the terminate error", err)
	}
	// The display is gone from the registry whatever terminate reported.
	if got := r.count(1); got != 0 {
		t.Fatalf("count = %d, want 0", got)
	}
}

// TestRegistryHoldsLockAcrossInitAndTerminate checks that a terminate never
// runs while another acquire of the same display is between its increment
// and its initialize: the lock covers both calls, as in Rust.
func TestRegistryHoldsLockAcrossInitAndTerminate(t *testing.T) {
	var (
		mu     sync.Mutex
		inCall int
		maxIn  int
	)
	enter := func() {
		mu.Lock()
		inCall++
		maxIn = max(maxIn, inCall)
		mu.Unlock()
	}
	leave := func() {
		mu.Lock()
		inCall--
		mu.Unlock()
	}
	r := NewRegistry(
		func(uintptr) (int, error) { enter(); defer leave(); return 0, nil },
		func(uintptr) error { enter(); defer leave(); return nil },
	)

	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			for range 100 {
				if _, err := r.Acquire(7); err != nil {
					t.Errorf("Acquire: %v", err)
					return
				}
				if err := r.Release(7); err != nil {
					t.Errorf("Release: %v", err)
					return
				}
			}
		})
	}
	wg.Wait()
	if maxIn != 1 {
		t.Fatalf("%d initialize/terminate calls overlapped, want them serialized", maxIn)
	}
	if got := r.count(7); got != 0 {
		t.Fatalf("count = %d, want 0", got)
	}
}
