//go:build rust

package wgpu

import (
	"context"
	"fmt"

	rwgpu "github.com/go-webgpu/webgpu/wgpu"
)

// Buffer represents a GPU buffer.
// On Rust backend, this wraps go-webgpu/webgpu Buffer.
type Buffer struct {
	r        *rwgpu.Buffer
	device   *Device
	released bool
}

// Size returns the buffer size in bytes.
func (b *Buffer) Size() uint64 {
	if b.r == nil {
		return 0
	}
	return b.r.Size()
}

// Usage returns the buffer's usage flags.
func (b *Buffer) Usage() BufferUsage {
	if b.r == nil {
		return 0
	}
	return b.r.Usage()
}

// Label returns the buffer's debug label.
func (b *Buffer) Label() string {
	return ""
}

// Release destroys the buffer.
func (b *Buffer) Release() {
	if b.released {
		return
	}
	b.released = true
	if b.r != nil {
		b.r.Release()
	}
}

// MapState returns the current mapping state of the buffer.
func (b *Buffer) MapState() MapState {
	if b == nil || b.released || b.r == nil {
		return MapStateUnmapped
	}
	rState := b.r.MapState()
	switch rState {
	case rwgpu.BufferMapStateMapped:
		return MapStateMapped
	case rwgpu.BufferMapStatePending:
		return MapStatePending
	default:
		return MapStateUnmapped
	}
}

// Map blocks until a CPU-visible mapping is established for the given
// byte range, or until ctx is canceled.
func (b *Buffer) Map(ctx context.Context, mode MapMode, offset, size uint64) error {
	if b == nil || b.r == nil {
		return ErrReleased
	}
	if b.released {
		return ErrBufferDestroyed
	}

	rMode := rwgpu.MapMode(mode)
	return b.r.Map(ctx, rMode, offset, size)
}

// MapAsync initiates a buffer map without blocking the caller.
func (b *Buffer) MapAsync(mode MapMode, offset, size uint64) (*MapPending, error) {
	if b == nil || b.r == nil {
		return nil, ErrReleased
	}
	if b.released {
		return nil, ErrBufferDestroyed
	}

	rMode := rwgpu.MapMode(mode)
	rp, err := b.r.MapAsync(rMode, offset, size)
	if err != nil {
		return nil, fmt.Errorf("wgpu: mapAsync: %w", err)
	}

	return &MapPending{r: rp, buf: b}, nil
}

// MappedRange returns a safe view over the mapped region [offset, offset+size).
func (b *Buffer) MappedRange(offset, size uint64) (*MappedRange, error) {
	if b == nil || b.r == nil {
		return nil, ErrReleased
	}
	if b.released {
		return nil, ErrBufferDestroyed
	}
	if b.MapState() != MapStateMapped {
		return nil, ErrMapNotMapped
	}

	rm, err := b.r.MappedRange(offset, size)
	if err != nil {
		return nil, fmt.Errorf("wgpu: mapped range: %w", err)
	}

	return &MappedRange{r: rm}, nil
}

// Unmap releases the current mapping.
func (b *Buffer) Unmap() error {
	if b == nil || b.r == nil {
		return ErrReleased
	}
	if b.released {
		return ErrBufferDestroyed
	}
	return b.r.Unmap()
}
