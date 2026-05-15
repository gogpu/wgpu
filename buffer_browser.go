//go:build js && wasm

package wgpu

import (
	"context"

	"github.com/gogpu/wgpu/internal/browser"
)

// Buffer represents a GPU buffer.
type Buffer struct {
	browser  *browser.Buffer
	size     uint64
	usage    BufferUsage
	released bool
}

// Size returns the buffer size in bytes.
func (b *Buffer) Size() uint64 {
	return b.size
}

// Usage returns the buffer's usage flags.
func (b *Buffer) Usage() BufferUsage {
	return b.usage
}

// Label returns the buffer's debug label.
// Browser WebGPU does not expose label on the object; returns empty string.
func (b *Buffer) Label() string {
	return ""
}

// Release destroys the buffer.
func (b *Buffer) Release() {
	if b.released {
		return
	}
	b.released = true
	if b.browser != nil {
		b.browser.Destroy()
	}
}

// MapState returns the current mapping state of the buffer.
// Phase 5 — full mapping support not yet implemented for browser.
func (b *Buffer) MapState() MapState {
	return MapStateUnmapped
}

// Map blocks until a CPU-visible mapping is established.
// Phase 5 — not yet implemented for browser.
func (b *Buffer) Map(ctx context.Context, mode MapMode, offset, size uint64) error {
	panic("wgpu: browser Buffer.Map not yet implemented (Phase 5)")
}

// MapAsync initiates a buffer map without blocking the caller.
// Phase 5 — not yet implemented for browser.
func (b *Buffer) MapAsync(mode MapMode, offset, size uint64) (*MapPending, error) {
	panic("wgpu: browser Buffer.MapAsync not yet implemented (Phase 5)")
}

// MappedRange returns a safe view over the mapped region.
// Phase 5 — not yet implemented for browser.
func (b *Buffer) MappedRange(offset, size uint64) (*MappedRange, error) {
	panic("wgpu: browser Buffer.MappedRange not yet implemented (Phase 5)")
}

// Unmap releases the current mapping.
// Phase 5 — not yet implemented for browser.
func (b *Buffer) Unmap() error {
	panic("wgpu: browser Buffer.Unmap not yet implemented (Phase 5)")
}
