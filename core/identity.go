package core

import (
	"sync"
)

// IdentityManager allocates and manages type-safe resource IDs.
//
// It maintains a pool of available indices and tracks epochs to prevent
// use-after-free bugs. When an ID is released, its index is recycled
// but with an incremented epoch, ensuring old IDs become invalid.
//
// Thread-safe for concurrent use.
type IdentityManager[T Marker] struct {
	mu        sync.Mutex
	free      []freeSlot // Pool of released (index, epoch) pairs
	nextIndex Index      // Next fresh index to allocate
	count     uint64     // Number of currently allocated IDs
}

// freeSlot represents a released ID slot available for reuse.
type freeSlot struct {
	index Index
	epoch Epoch
}

// NewIdentityManager creates a new identity manager for the given marker type.
func NewIdentityManager[T Marker]() *IdentityManager[T] {
	return &IdentityManager[T]{
		free:      make([]freeSlot, 0, 64), // Pre-allocate for common case
		nextIndex: 0,
		count:     0,
	}
}

// Alloc allocates a fresh, never-before-seen ID.
//
// If there are released IDs available, their indices are reused with
// an incremented epoch. Otherwise, a new index is allocated.
//
// The epoch starts at 1 (not 0) so that zero IDs are always invalid.
func (m *IdentityManager[T]) Alloc() ID[T] {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.count++

	// Try to reuse a released slot
	if len(m.free) > 0 {
		// Pop from the end (most recently freed)
		slot := m.free[len(m.free)-1]
		m.free = m.free[:len(m.free)-1]
		// Increment epoch to invalidate old IDs with this index
		return NewID[T](slot.index, slot.epoch+1)
	}

	// Allocate a fresh index
	index := m.nextIndex
	m.nextIndex++
	// Start with epoch 1 (epoch 0 would make zero IDs valid)
	return NewID[T](index, 1)
}

// Release marks an ID as freed, making its index available for reuse.
//
// After release, the ID becomes invalid. Any attempt to use it will
// fail with an epoch mismatch error.
func (m *IdentityManager[T]) Release(id ID[T]) {
	m.mu.Lock()
	defer m.mu.Unlock()

	index, epoch := id.Unzip()
	m.free = append(m.free, freeSlot{index: index, epoch: epoch})
	m.count--
}

// Count returns the number of currently allocated IDs.
func (m *IdentityManager[T]) Count() uint64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.count
}

// NextIndex returns the next index that would be allocated (for testing).
func (m *IdentityManager[T]) NextIndex() Index {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.nextIndex
}

// FreeCount returns the number of IDs available for reuse (for testing).
func (m *IdentityManager[T]) FreeCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.free)
}
