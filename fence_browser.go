//go:build js && wasm

package wgpu

// Fence is a GPU synchronization primitive.
// On browser, fences are not needed (browser auto-polls).
type Fence struct {
	released bool
}

// Release destroys the fence.
func (f *Fence) Release() {
	if f.released {
		return
	}
	f.released = true
}
