//go:build rust

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package wgpu

import (
	"context"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/resource"
)

// Buffer represents a GPU buffer.
//
// The implementation lives in internal/resource (ADR-070). This declaration is
// the public surface: every method is one delegation, and nil/released
// behavior matches the implementation's guards. The file is per context, like
// the implementation it fronts; its two siblings (browser, rust) declare the
// same surface, so a later step can let them diverge without moving the type.
type Buffer struct {
	impl *resource.Buffer
}

// Size returns the buffer size in bytes.
func (b *Buffer) Size() uint64 { return b.impl.Size() }

// Usage returns the buffer's usage flags.
func (b *Buffer) Usage() gputypes.BufferUsage { return b.impl.Usage() }

// Label returns the buffer's debug label.
func (b *Buffer) Label() string { return b.impl.Label() }

// Release drops the application's ownership reference to the buffer; see the
// implementation's documentation for the refcount-driven destruction contract.
func (b *Buffer) Release() {
	if b.impl == nil {
		return
	}
	b.impl.Release()
}

// MapState returns the current mapping state of the buffer.
func (b *Buffer) MapState() MapState {
	if b == nil || b.impl == nil {
		return MapStateUnmapped
	}
	return MapState(b.impl.MapState())
}

// Map blocks until a CPU-visible mapping is established for the given byte
// range, or until ctx is canceled.
func (b *Buffer) Map(ctx context.Context, mode MapMode, offset, size uint64) error {
	if b == nil || b.impl == nil {
		return ErrReleased
	}
	return b.impl.Map(ctx, resource.MapMode(mode), offset, size)
}

// MapAsync initiates a buffer map without blocking the caller.
func (b *Buffer) MapAsync(mode MapMode, offset, size uint64) (*MapPending, error) {
	if b == nil || b.impl == nil {
		return nil, ErrReleased
	}
	pending, err := b.impl.MapAsync(resource.MapMode(mode), offset, size)
	if err != nil {
		return nil, err
	}
	return &MapPending{impl: pending}, nil
}

// MappedRange returns a safe view over the mapped region [offset, offset+size).
func (b *Buffer) MappedRange(offset, size uint64) (*MappedRange, error) {
	if b == nil || b.impl == nil {
		return nil, ErrReleased
	}
	mapped, err := b.impl.MappedRange(offset, size)
	if err != nil {
		return nil, err
	}
	return &MappedRange{impl: mapped}, nil
}

// Unmap releases the current mapping and invalidates all outstanding
// MappedRange handles for this buffer.
func (b *Buffer) Unmap() error {
	if b == nil || b.impl == nil {
		return ErrReleased
	}
	return b.impl.Unmap()
}
