// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

// Package eglrefs holds the reference counting behind the GLES backend's EGL
// lifetimes (ADR-077): a process-wide registry of initialized EGL displays,
// and the reference count of one EGL context.
//
// The package has no EGL dependency. The EGL calls are injected, so the
// counting is tested on every OS while the backend that wires it in builds on
// Linux only.
package eglrefs

import (
	"errors"
	"fmt"
	"sync"
)

// ErrNotAcquired reports a release with no matching acquire: a reference
// counting bug in the caller.
var ErrNotAcquired = errors.New("eglrefs: release without a matching acquire")

// Registry counts the users of each EGL display in the process, so that
// eglTerminate runs only when the last of them goes.
//
// Every eglGetPlatformDisplay call for the same native display returns the
// same EGLDisplay, and eglTerminate on it invalidates every context and
// surface of that display for all its users. Rust wgpu-hal keeps the same
// count (DISPLAYS_REFERENCE_COUNT in wgpu-hal/src/gles/egl.rs, keyed by
// EGLDisplay). The count covers wgpu's own users only.
type Registry[K comparable, V any] struct {
	mu         sync.Mutex
	counts     map[K]int
	initialize func(K) (V, error)
	terminate  func(K) error
}

// NewRegistry returns a Registry that calls initialize on every acquire and
// terminate on the last release of a display.
func NewRegistry[K comparable, V any](initialize func(K) (V, error), terminate func(K) error) *Registry[K, V] {
	return &Registry[K, V]{
		counts:     make(map[K]int),
		initialize: initialize,
		terminate:  terminate,
	}
}

// Acquire counts one more user of display and initializes it, returning what
// initialize returned (eglInitialize on an initialized display only reports
// its version). The lock is held across the call, so it cannot interleave
// with the terminate of the last release, as in Rust.
//
// When initialize fails, the count is rolled back and nothing is terminated.
// Rust leaves the count incremented; rolling back keeps a display nobody
// initialized out of the registry.
func (r *Registry[K, V]) Acquire(display K) (V, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.counts[display]++
	v, err := r.initialize(display)
	if err != nil {
		r.decrementLocked(display)
		var zero V
		return zero, err
	}
	return v, nil
}

// Release counts one user of display less and terminates it when that was
// the last one, under the lock. A release without an acquire returns
// ErrNotAcquired and terminates nothing. A terminate error is returned; the
// display leaves the registry either way.
func (r *Registry[K, V]) Release(display K) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.counts[display] == 0 {
		return fmt.Errorf("%w: display %v", ErrNotAcquired, display)
	}
	if r.decrementLocked(display) > 0 {
		return nil
	}
	return r.terminate(display)
}

// decrementLocked drops one count of display and returns what is left,
// removing the entry at zero.
func (r *Registry[K, V]) decrementLocked(display K) int {
	n := r.counts[display] - 1
	if n <= 0 {
		delete(r.counts, display)
		return 0
	}
	r.counts[display] = n
	return n
}
