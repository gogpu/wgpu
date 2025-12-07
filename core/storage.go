package core

import (
	"sync"
)

// StorageItem is a constraint for items that can be stored in Storage.
// Items must have an associated marker type for type-safe ID access.
type StorageItem interface {
	// Marker returns the marker type for this item.
	// This is used for compile-time type checking.
}

// slot holds an item along with its current epoch.
type slot[T any] struct {
	item  T
	epoch Epoch
	valid bool
}

// Storage is an indexed array that stores items by their ID index.
//
// It provides O(1) access to items using the index component of an ID,
// while validating the epoch to prevent use-after-free.
//
// Thread-safe for concurrent use.
type Storage[T any, M Marker] struct {
	mu    sync.RWMutex
	slots []slot[T]
}

// NewStorage creates a new storage with optional initial capacity.
func NewStorage[T any, M Marker](capacity int) *Storage[T, M] {
	if capacity <= 0 {
		capacity = 64 // Default capacity
	}
	return &Storage[T, M]{
		slots: make([]slot[T], 0, capacity),
	}
}

// Insert stores an item at the given ID's index with its epoch.
// If the index is beyond current capacity, the storage grows automatically.
func (s *Storage[T, M]) Insert(id ID[M], item T) {
	s.mu.Lock()
	defer s.mu.Unlock()

	index, epoch := id.Unzip()
	s.ensureCapacity(index + 1)
	s.slots[index] = slot[T]{
		item:  item,
		epoch: epoch,
		valid: true,
	}
}

// Get retrieves an item by ID, validating the epoch.
// Returns the item and true if found with matching epoch, zero value and false otherwise.
func (s *Storage[T, M]) Get(id ID[M]) (T, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	index, epoch := id.Unzip()
	if int(index) >= len(s.slots) {
		var zero T
		return zero, false
	}

	slot := &s.slots[index]
	if !slot.valid || slot.epoch != epoch {
		var zero T
		return zero, false
	}

	return slot.item, true
}

// GetMut retrieves an item by ID for mutation.
// The callback is called with the item if found, while holding the write lock.
// Returns true if the item was found and the callback was called.
func (s *Storage[T, M]) GetMut(id ID[M], fn func(*T)) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	index, epoch := id.Unzip()
	if int(index) >= len(s.slots) {
		return false
	}

	slot := &s.slots[index]
	if !slot.valid || slot.epoch != epoch {
		return false
	}

	fn(&slot.item)
	return true
}

// Remove removes an item by ID, returning it if found.
// Returns the item and true if found, zero value and false otherwise.
func (s *Storage[T, M]) Remove(id ID[M]) (T, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	index, epoch := id.Unzip()
	if int(index) >= len(s.slots) {
		var zero T
		return zero, false
	}

	slot := &s.slots[index]
	if !slot.valid || slot.epoch != epoch {
		var zero T
		return zero, false
	}

	item := slot.item
	var zero T
	slot.item = zero
	slot.valid = false
	// Note: we don't clear the epoch, so future inserts with higher epochs work

	return item, true
}

// Contains checks if an item exists at the given ID with matching epoch.
func (s *Storage[T, M]) Contains(id ID[M]) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	index, epoch := id.Unzip()
	if int(index) >= len(s.slots) {
		return false
	}

	slot := &s.slots[index]
	return slot.valid && slot.epoch == epoch
}

// Len returns the number of valid items in storage.
func (s *Storage[T, M]) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	count := 0
	for i := range s.slots {
		if s.slots[i].valid {
			count++
		}
	}
	return count
}

// Capacity returns the current capacity of the storage.
func (s *Storage[T, M]) Capacity() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.slots)
}

// ForEach iterates over all valid items in storage.
// The callback receives the ID and item for each valid entry.
// Iteration order is by index.
func (s *Storage[T, M]) ForEach(fn func(ID[M], T) bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for i := range s.slots {
		slot := &s.slots[i]
		if slot.valid {
			//nolint:gosec // G115: Safe conversion - i is storage index, always < 2^32
			id := NewID[M](Index(i), slot.epoch)
			if !fn(id, slot.item) {
				break
			}
		}
	}
}

// Clear removes all items from storage.
func (s *Storage[T, M]) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.slots {
		var zero T
		s.slots[i].item = zero
		s.slots[i].valid = false
	}
}

// ensureCapacity grows the slots slice if needed.
// Must be called with write lock held.
func (s *Storage[T, M]) ensureCapacity(needed Index) {
	//nolint:gosec // G115: Safe conversion - len(s.slots) is always < 2^32 in practice
	current := Index(len(s.slots))
	if needed <= current {
		return
	}

	// Grow by at least doubling, but at least to needed
	newCap := current * 2
	if newCap < needed {
		newCap = needed
	}
	if newCap < 64 {
		newCap = 64
	}

	newSlots := make([]slot[T], needed, newCap)
	copy(newSlots, s.slots)
	s.slots = newSlots
}
