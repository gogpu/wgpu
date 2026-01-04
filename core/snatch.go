// Package core provides the snatch pattern for safe deferred destruction of HAL resources.
//
// The Snatchable[T] pattern allows resources to be safely destroyed while
// potentially still being accessed by other goroutines. A resource can only
// be "snatched" (taken for destruction) once, and subsequent snatch attempts
// return nil.
//
// This is based on the snatch pattern from Rust wgpu-core.

package core

import (
	"sync"
)

// Snatchable wraps a value that can be "snatched" for destruction.
//
// The value can be accessed via Get() while it hasn't been snatched,
// and can be taken via Snatch() exactly once. After being snatched,
// Get() returns nil.
//
// Thread-safe for concurrent use.
type Snatchable[T any] struct {
	mu       sync.RWMutex
	value    *T
	snatched bool
}

// NewSnatchable creates a new Snatchable wrapper for the given value.
// The value is stored by pointer for efficient access.
func NewSnatchable[T any](value T) *Snatchable[T] {
	return &Snatchable[T]{
		value:    &value,
		snatched: false,
	}
}

// Get returns a pointer to the wrapped value if it hasn't been snatched.
// Returns nil if the value has already been snatched.
//
// The caller must hold a SnatchGuard obtained from a SnatchLock.
// This ensures that the value won't be snatched during access.
//
// Note: The guard parameter is used for API clarity and to enforce
// the pattern of acquiring a lock before accessing snatchable resources.
func (s *Snatchable[T]) Get(_ *SnatchGuard) *T {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.snatched {
		return nil
	}
	return s.value
}

// Snatch takes ownership of the wrapped value for destruction.
// Returns the value if it hasn't been snatched yet, nil otherwise.
//
// This can only succeed once - subsequent calls return nil.
//
// The caller must hold an ExclusiveSnatchGuard obtained from a SnatchLock.
// This ensures exclusive access during the snatch operation.
//
// Note: The guard parameter is used for API clarity and to enforce
// the pattern of acquiring exclusive lock before snatching.
func (s *Snatchable[T]) Snatch(_ *ExclusiveSnatchGuard) *T {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.snatched {
		return nil
	}

	s.snatched = true
	result := s.value
	s.value = nil
	return result
}

// IsSnatched returns true if the value has been snatched.
func (s *Snatchable[T]) IsSnatched() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.snatched
}

// SnatchLock provides device-global coordination for snatchable resources.
//
// Multiple resources can be accessed concurrently using read guards,
// but destruction (snatching) requires an exclusive write guard.
// This prevents resources from being destroyed while they're being accessed.
//
// Thread-safe for concurrent use.
type SnatchLock struct {
	mu sync.RWMutex
}

// NewSnatchLock creates a new SnatchLock.
func NewSnatchLock() *SnatchLock {
	return &SnatchLock{}
}

// Read acquires a read lock and returns a SnatchGuard.
// Multiple goroutines can hold read locks simultaneously.
//
// The returned guard must be released by calling Release().
// Using defer is recommended:
//
//	guard := lock.Read()
//	defer guard.Release()
func (l *SnatchLock) Read() *SnatchGuard {
	l.mu.RLock()
	return &SnatchGuard{lock: l, released: false}
}

// Write acquires an exclusive write lock and returns an ExclusiveSnatchGuard.
// Only one goroutine can hold the write lock, and it blocks all readers.
//
// The returned guard must be released by calling Release().
// Using defer is recommended:
//
//	guard := lock.Write()
//	defer guard.Release()
func (l *SnatchLock) Write() *ExclusiveSnatchGuard {
	l.mu.Lock()
	return &ExclusiveSnatchGuard{lock: l, released: false}
}

// SnatchGuard represents a held read lock on a SnatchLock.
//
// It must be released by calling Release() when done.
// Not releasing the guard will cause a deadlock.
type SnatchGuard struct {
	lock     *SnatchLock
	released bool
}

// Release releases the read lock.
// This must be called exactly once. Subsequent calls are no-ops.
func (g *SnatchGuard) Release() {
	if g.released {
		return
	}
	g.released = true
	g.lock.mu.RUnlock()
}

// ExclusiveSnatchGuard represents a held write lock on a SnatchLock.
//
// It must be released by calling Release() when done.
// Not releasing the guard will cause a deadlock.
type ExclusiveSnatchGuard struct {
	lock     *SnatchLock
	released bool
}

// Release releases the write lock.
// This must be called exactly once. Subsequent calls are no-ops.
func (g *ExclusiveSnatchGuard) Release() {
	if g.released {
		return
	}
	g.released = true
	g.lock.mu.Unlock()
}
