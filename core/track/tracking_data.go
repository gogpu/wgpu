package track

import "sync/atomic"

// TrackingData holds per-resource tracking information.
// This struct is embedded in each tracked resource (Buffer, Texture, etc.)
// to provide efficient O(1) access to tracking state.
//
// # Lifecycle
//
//  1. Created with NewTrackingData during resource creation
//  2. Index() used during command encoding for state tracking
//  3. Release() called during resource destruction to recycle the index
//
// # Thread Safety
//
// TrackingData is safe for concurrent use. The index is immutable after
// creation, and Release() uses atomic operations to prevent double-free.
type TrackingData struct {
	// Dense index for this resource (immutable after creation)
	index TrackerIndex

	// Reference to allocator for cleanup
	allocator *SharedTrackerIndexAllocator

	// Tracks whether this data has been released (0 = active, 1 = released)
	released atomic.Uint32
}

// NewTrackingData creates tracking data and allocates an index.
// The allocator must not be nil.
func NewTrackingData(allocator *SharedTrackerIndexAllocator) *TrackingData {
	if allocator == nil {
		return &TrackingData{
			index:     InvalidTrackerIndex,
			allocator: nil,
		}
	}
	return &TrackingData{
		index:     allocator.Alloc(),
		allocator: allocator,
	}
}

// Index returns the tracker index.
// Returns InvalidTrackerIndex if the tracking data was created with
// a nil allocator or has been released.
func (t *TrackingData) Index() TrackerIndex {
	return t.index
}

// IsReleased returns true if Release() has been called.
func (t *TrackingData) IsReleased() bool {
	return t.released.Load() != 0
}

// Release frees the tracker index for reuse.
// Called when the resource is destroyed.
// Safe to call multiple times (subsequent calls are no-ops).
func (t *TrackingData) Release() {
	// Atomically set released flag, return if already released
	if !t.released.CompareAndSwap(0, 1) {
		return
	}

	if t.allocator != nil {
		t.allocator.Free(t.index)
	}
}

// TrackingDataInit is a convenience interface for resources that
// need tracking data initialization.
type TrackingDataInit interface {
	// InitTracking initializes the tracking data for this resource.
	InitTracking(allocator *SharedTrackerIndexAllocator)
}
