//go:build js && wasm

package resource

// Fence is a GPU synchronization primitive.
// On browser, fences are no-ops — the browser auto-polls GPU completion
// via its internal event loop. This type exists for API compatibility.
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
