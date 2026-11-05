// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package eglrefs

import "sync"

// Refs is the reference count of an object that dies once, with its last
// reference, and never comes back: an EGL context and the GL objects in it.
//
// The zero value holds one reference, its creator's. Acquire adds one while
// the object is alive; Release drops one and reports the last. The lock
// guards the count only and is never held across work on the object, so
// bookkeeping never waits behind a GL call.
type Refs struct {
	mu    sync.Mutex
	extra int  // references beyond the creator's
	dead  bool // the last reference was released
}

// Acquire takes one more reference and reports whether it did: false once
// the last reference was released.
func (r *Refs) Acquire() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.dead {
		return false
	}
	r.extra++
	return true
}

// Release drops one reference. last is true for the release that drops the
// count to zero; it marks the object dead in the same critical section, so
// no Acquire can succeed after it. A release of a dead object returns
// ErrNotAcquired.
func (r *Refs) Release() (last bool, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	switch {
	case r.dead:
		return false, ErrNotAcquired
	case r.extra > 0:
		r.extra--
		return false, nil
	default:
		r.dead = true
		return true, nil
	}
}

// Alive reports whether the last reference is still held.
func (r *Refs) Alive() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return !r.dead
}
