package core

import (
	"sync/atomic"
)

// ResourceRef provides GPU-aware reference counting for HAL resources.
//
// Go's garbage collector doesn't know about GPU lifetimes — a buffer might be
// released by the application while the GPU is still reading from it in a
// previously submitted command buffer. ResourceRef prevents use-after-free by
// tracking how many owners (user + in-flight submissions) hold the resource.
//
// This is the Go equivalent of Rust's Arc<Buffer> pattern used in wgpu-core's
// EncoderInFlight. Phase 1 uses DestroyQueue.Defer() with submission indices
// for conservative deferred destruction. Phase 2 will add Clone()/Drop() calls
// in command encoders for precise per-command-buffer tracking.
//
// Thread-safe for concurrent use.
type ResourceRef struct {
	refCount atomic.Int32
	onZero   func() // HAL destroy callback, called when last ref is dropped
	label    string // debug label for logging
}

// NewResourceRef creates a ResourceRef with an initial reference count of 1.
// The onZero callback is invoked when the last reference is dropped (refCount
// reaches 0), typically calling the appropriate HAL destroy method.
//
// Parameters:
//   - label: debug label for logging (e.g., "Buffer 'vertex_data'")
//   - onZero: callback to invoke on final drop (may be nil for testing)
func NewResourceRef(label string, onZero func()) *ResourceRef {
	ref := &ResourceRef{
		onZero: onZero,
		label:  label,
	}
	ref.refCount.Store(1)
	return ref
}

// Clone increments the reference count by 1.
// Called when a new owner (e.g., an in-flight command buffer) takes a reference.
func (r *ResourceRef) Clone() {
	r.refCount.Add(1)
}

// Drop decrements the reference count by 1.
// If the count reaches zero, the onZero callback is invoked to destroy the
// underlying HAL resource. Safe to call multiple times — negative counts are
// harmless (no callback after first zero crossing).
func (r *ResourceRef) Drop() {
	if r.refCount.Add(-1) == 0 {
		if r.onZero != nil {
			r.onZero()
		}
	}
}

// RefCount returns the current reference count. For debugging and testing only.
func (r *ResourceRef) RefCount() int32 {
	return r.refCount.Load()
}

// Label returns the debug label.
func (r *ResourceRef) Label() string {
	return r.label
}
