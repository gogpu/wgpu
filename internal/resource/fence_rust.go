//go:build rust

package resource

// Fence is a GPU synchronization primitive.
// On Rust backend, fences are no-ops — wgpu-native handles synchronization
// internally via device polling. This type exists for API compatibility.
type Fence struct {
	released bool
}

// IsReleased reports whether Release has run on the fence.
func (f *Fence) IsReleased() bool {
	return f != nil && f.released
}

// Release destroys the fence.
func (f *Fence) Release() {
	if f.released {
		return
	}
	f.released = true
}
