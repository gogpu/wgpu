//go:build js && wasm

package browser

import "syscall/js"

// Buffer wraps a browser GPUBuffer.
//
// Holds a reference to the JavaScript GPUBuffer object and caches the size
// to avoid repeated JS property lookups.
type Buffer struct {
	// ref_ is the GPUBuffer JavaScript object.
	ref_ js.Value

	// size is the buffer's byte size, cached at creation time.
	size uint64

	// usage is the buffer's usage flags, cached at creation time.
	usage uint32
}

// NewBuffer constructs a Buffer from a GPUBuffer js.Value.
func NewBuffer(ref js.Value) *Buffer {
	return &Buffer{
		ref_:  ref,
		size:  uint64(ref.Get("size").Float()), //nolint:gosec // JS API returns safe integers
		usage: uint32(ref.Get("usage").Int()),  //nolint:gosec // JS API returns safe integers
	}
}

// Ref returns the underlying GPUBuffer js.Value.
func (b *Buffer) Ref() js.Value { return b.ref_ }

// Size returns the buffer size in bytes.
func (b *Buffer) Size() uint64 { return b.size }

// Usage returns the buffer usage flags.
func (b *Buffer) Usage() uint32 { return b.usage }

// Destroy calls GPUBuffer.destroy() to release GPU memory.
func (b *Buffer) Destroy() {
	destroy := b.ref_.Get("destroy")
	if !destroy.IsUndefined() && !destroy.IsNull() {
		b.ref_.Call("destroy")
	}
}

// GetMappedRange returns a JS ArrayBuffer covering [offset, offset+size)
// of the mapped buffer. Must be called while the buffer is mapped.
func (b *Buffer) GetMappedRange(offset, size uint64) js.Value {
	return b.ref_.Call("getMappedRange", float64(offset), float64(size))
}

// Unmap unmaps the buffer, making its mapped ranges invalid.
func (b *Buffer) Unmap() {
	b.ref_.Call("unmap")
}
