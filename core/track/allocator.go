// Package track provides resource state tracking infrastructure.
//
// TrackerIndex provides dense indexing for efficient O(1) access to resource
// tracking state. Unlike resource IDs (which use epochs and may be sparse),
// tracker indices are always dense (0, 1, 2, ...) for efficient array access.
//
// # Architecture
//
// Each Device owns a TrackerIndexAllocators which manages separate allocators
// for each resource type. When a resource is created, it gets a TrackerIndex
// from the appropriate allocator. When destroyed, the index is returned for
// reuse.
//
// # Thread Safety
//
// SharedTrackerIndexAllocator provides thread-safe allocation/deallocation.
// The underlying TrackerIndexAllocator uses mutex-based synchronization.
package track

import "sync"

// TrackerIndex is a dense index for efficient resource state tracking.
// Unlike resource IDs (which use epochs and may be sparse), tracker indices
// are always dense (0, 1, 2, ...) for efficient array access.
type TrackerIndex uint32

// InvalidTrackerIndex represents an unassigned tracker index.
// Using max uint32 ensures it won't conflict with valid indices.
const InvalidTrackerIndex TrackerIndex = ^TrackerIndex(0)

// IsValid returns true if this is a valid tracker index.
func (i TrackerIndex) IsValid() bool {
	return i != InvalidTrackerIndex
}

// TrackerIndexAllocator allocates dense tracker indices.
// Indices are reused after being freed to maintain density.
type TrackerIndexAllocator struct {
	mu        sync.Mutex
	unused    []TrackerIndex // Free list of released indices
	nextIndex TrackerIndex   // Next fresh index
}

// NewTrackerIndexAllocator creates a new allocator.
func NewTrackerIndexAllocator() *TrackerIndexAllocator {
	return &TrackerIndexAllocator{
		unused:    make([]TrackerIndex, 0, 64),
		nextIndex: 0,
	}
}

// Alloc allocates a new tracker index.
// Reuses released indices when available for optimal density.
func (a *TrackerIndexAllocator) Alloc() TrackerIndex {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Try to reuse a released index (LIFO for cache locality)
	if len(a.unused) > 0 {
		idx := a.unused[len(a.unused)-1]
		a.unused = a.unused[:len(a.unused)-1]
		return idx
	}

	// Allocate fresh index
	idx := a.nextIndex
	a.nextIndex++
	return idx
}

// Free releases a tracker index for reuse.
// Safe to call with InvalidTrackerIndex (no-op).
func (a *TrackerIndexAllocator) Free(idx TrackerIndex) {
	if idx == InvalidTrackerIndex {
		return
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	a.unused = append(a.unused, idx)
}

// Size returns the number of currently allocated indices.
// This equals the total allocated minus the freed count.
func (a *TrackerIndexAllocator) Size() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return int(a.nextIndex) - len(a.unused)
}

// HighWaterMark returns the highest index ever allocated.
// Useful for sizing tracking arrays.
func (a *TrackerIndexAllocator) HighWaterMark() TrackerIndex {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.nextIndex == 0 {
		return InvalidTrackerIndex
	}
	return a.nextIndex - 1
}

// Reset clears the allocator, invalidating all previously allocated indices.
// Use with caution - all resources using old indices become invalid.
func (a *TrackerIndexAllocator) Reset() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.unused = a.unused[:0]
	a.nextIndex = 0
}

// SharedTrackerIndexAllocator is a thread-safe wrapper for sharing
// between device and resources. It's essentially a reference-counted
// pointer to a TrackerIndexAllocator.
type SharedTrackerIndexAllocator struct {
	inner *TrackerIndexAllocator
}

// NewSharedTrackerIndexAllocator creates a new shared allocator.
func NewSharedTrackerIndexAllocator() *SharedTrackerIndexAllocator {
	return &SharedTrackerIndexAllocator{
		inner: NewTrackerIndexAllocator(),
	}
}

// Alloc allocates a new tracker index.
func (s *SharedTrackerIndexAllocator) Alloc() TrackerIndex {
	return s.inner.Alloc()
}

// Free releases a tracker index for reuse.
func (s *SharedTrackerIndexAllocator) Free(idx TrackerIndex) {
	s.inner.Free(idx)
}

// Size returns the number of currently allocated indices.
func (s *SharedTrackerIndexAllocator) Size() int {
	return s.inner.Size()
}

// HighWaterMark returns the highest index ever allocated.
func (s *SharedTrackerIndexAllocator) HighWaterMark() TrackerIndex {
	return s.inner.HighWaterMark()
}

// TrackerIndexAllocators manages all tracker index allocators for a device.
// Each resource type has its own allocator to maintain separate namespaces.
type TrackerIndexAllocators struct {
	Buffers          *SharedTrackerIndexAllocator
	Textures         *SharedTrackerIndexAllocator
	TextureViews     *SharedTrackerIndexAllocator
	Samplers         *SharedTrackerIndexAllocator
	BindGroups       *SharedTrackerIndexAllocator
	BindGroupLayouts *SharedTrackerIndexAllocator
	RenderPipelines  *SharedTrackerIndexAllocator
	ComputePipelines *SharedTrackerIndexAllocator
}

// NewTrackerIndexAllocators creates allocators for all resource types.
func NewTrackerIndexAllocators() *TrackerIndexAllocators {
	return &TrackerIndexAllocators{
		Buffers:          NewSharedTrackerIndexAllocator(),
		Textures:         NewSharedTrackerIndexAllocator(),
		TextureViews:     NewSharedTrackerIndexAllocator(),
		Samplers:         NewSharedTrackerIndexAllocator(),
		BindGroups:       NewSharedTrackerIndexAllocator(),
		BindGroupLayouts: NewSharedTrackerIndexAllocator(),
		RenderPipelines:  NewSharedTrackerIndexAllocator(),
		ComputePipelines: NewSharedTrackerIndexAllocator(),
	}
}
