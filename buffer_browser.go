//go:build js && wasm

package wgpu

import "context"

// Buffer represents a GPU buffer.
type Buffer struct {
	released bool
}

// Size returns the buffer size in bytes.
func (b *Buffer) Size() uint64 {
	panic("wgpu: browser backend not yet implemented")
}

// Usage returns the buffer's usage flags.
func (b *Buffer) Usage() BufferUsage {
	panic("wgpu: browser backend not yet implemented")
}

// Label returns the buffer's debug label.
func (b *Buffer) Label() string {
	panic("wgpu: browser backend not yet implemented")
}

// Release destroys the buffer.
func (b *Buffer) Release() {
	if b.released {
		return
	}
	b.released = true
}

// MapState returns the current mapping state of the buffer.
func (b *Buffer) MapState() MapState {
	panic("wgpu: browser backend not yet implemented")
}

// Map blocks until a CPU-visible mapping is established.
func (b *Buffer) Map(ctx context.Context, mode MapMode, offset, size uint64) error {
	panic("wgpu: browser backend not yet implemented")
}

// MapAsync initiates a buffer map without blocking the caller.
func (b *Buffer) MapAsync(mode MapMode, offset, size uint64) (*MapPending, error) {
	panic("wgpu: browser backend not yet implemented")
}

// MappedRange returns a safe view over the mapped region.
func (b *Buffer) MappedRange(offset, size uint64) (*MappedRange, error) {
	panic("wgpu: browser backend not yet implemented")
}

// Unmap releases the current mapping.
func (b *Buffer) Unmap() error {
	panic("wgpu: browser backend not yet implemented")
}
