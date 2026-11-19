//go:build rust

package resource

import "sync/atomic"

// Fence is a GPU synchronization primitive.
// On Rust backend, fences are no-ops — wgpu-native handles synchronization
// internally via device polling. This type exists for API compatibility.
type Fence struct {
	// released is the CAS guard that admits exactly one destroy: concurrent
	// Release calls see CompareAndSwap succeed for one caller only. It is nil
	// only on an unguarded zero value, whose Release is a no-op.
	released *atomic.Bool
}

// NewFence builds a fence. It carries no HAL object on this build.
func NewFence() *Fence {
	return &Fence{released: new(atomic.Bool)}
}

// IsReleased reports whether Release has run on the fence.
func (f *Fence) IsReleased() bool {
	return f != nil && f.released != nil && f.released.Load()
}

// Release destroys the fence.
func (f *Fence) Release() {
	if f.released == nil || !f.released.CompareAndSwap(false, true) {
		return
	}
}
