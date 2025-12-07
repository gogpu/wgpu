package core

// Registry manages the lifecycle of resources of a specific type.
//
// It combines IdentityManager (for ID allocation) with Storage (for item storage)
// to provide a complete resource management solution.
//
// Thread-safe for concurrent use.
type Registry[T any, M Marker] struct {
	identity *IdentityManager[M]
	storage  *Storage[T, M]
}

// NewRegistry creates a new registry for the given types.
func NewRegistry[T any, M Marker]() *Registry[T, M] {
	return &Registry[T, M]{
		identity: NewIdentityManager[M](),
		storage:  NewStorage[T, M](64),
	}
}

// Register allocates a new ID and stores the item.
// Returns the allocated ID.
func (r *Registry[T, M]) Register(item T) ID[M] {
	id := r.identity.Alloc()
	r.storage.Insert(id, item)
	return id
}

// Get retrieves an item by ID.
// Returns the item and nil error if found, or zero value and error if not found
// or epoch mismatch.
func (r *Registry[T, M]) Get(id ID[M]) (T, error) {
	if id.IsZero() {
		var zero T
		return zero, ErrInvalidID
	}

	item, ok := r.storage.Get(id)
	if !ok {
		var zero T
		// Check if it's epoch mismatch or not found
		if r.storage.Capacity() > int(id.Index()) {
			return zero, ErrEpochMismatch
		}
		return zero, ErrResourceNotFound
	}

	return item, nil
}

// GetMut retrieves an item by ID for mutation.
// The callback is called with a pointer to the item if found.
// Returns nil if successful, or error if not found.
func (r *Registry[T, M]) GetMut(id ID[M], fn func(*T)) error {
	if id.IsZero() {
		return ErrInvalidID
	}

	if !r.storage.GetMut(id, fn) {
		if r.storage.Capacity() > int(id.Index()) {
			return ErrEpochMismatch
		}
		return ErrResourceNotFound
	}

	return nil
}

// Unregister removes an item by ID and releases the ID for reuse.
// Returns the removed item and nil error, or zero value and error if not found.
func (r *Registry[T, M]) Unregister(id ID[M]) (T, error) {
	if id.IsZero() {
		var zero T
		return zero, ErrInvalidID
	}

	item, ok := r.storage.Remove(id)
	if !ok {
		var zero T
		if r.storage.Capacity() > int(id.Index()) {
			return zero, ErrEpochMismatch
		}
		return zero, ErrResourceNotFound
	}

	r.identity.Release(id)
	return item, nil
}

// Contains checks if an item exists at the given ID.
func (r *Registry[T, M]) Contains(id ID[M]) bool {
	if id.IsZero() {
		return false
	}
	return r.storage.Contains(id)
}

// Count returns the number of registered items.
func (r *Registry[T, M]) Count() uint64 {
	return r.identity.Count()
}

// ForEach iterates over all registered items.
// The callback receives the ID and item for each entry.
// Return false from the callback to stop iteration.
func (r *Registry[T, M]) ForEach(fn func(ID[M], T) bool) {
	r.storage.ForEach(fn)
}

// Clear removes all items from the registry.
// Note: This does not release IDs properly - use only for cleanup.
func (r *Registry[T, M]) Clear() {
	r.storage.Clear()
}
