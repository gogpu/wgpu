// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package indirect

import (
	"sync"

	"github.com/gogpu/wgpu/hal"
)

// DispatchArgsSize is the size of DispatchIndirect arguments: x, y, z as u32.
const DispatchArgsSize = 12

// DispatchSource says where the indirect dispatch validation shader reads the
// arguments of one DispatchIndirect: a storage binding of the arguments
// buffer at an offset aligned to minStorageBufferOffsetAlignment, and the u32
// index of x inside that binding.
//
// Rust wgpu-core (indirect_validation/dispatch.rs Dispatch::params) splits the
// offset the same way: the aligned part becomes the dynamic offset of the
// source binding and the remainder an immediate. Without immediates and
// dynamic offsets in every HAL, core binds the aligned offset statically and
// selects the remainder from a constant table, so Index is always below
// alignment/4.
type DispatchSource struct {
	BindingOffset uint64 // byte offset of the binding, a multiple of the alignment
	BindingSize   uint64 // byte size of the binding, a multiple of 4
	Index         uint32 // u32 index of x inside the binding
}

// DispatchSourceFor returns the source binding for arguments at offset in a
// buffer of bufferSize bytes. The caller has checked that offset is a
// multiple of 4 and that offset+DispatchArgsSize <= bufferSize; alignment is
// minStorageBufferOffsetAlignment (a power of two, at least 4).
func DispatchSourceFor(offset, bufferSize uint64, alignment uint32) DispatchSource {
	align := uint64(alignment)
	bindingOffset := offset - offset%align
	// The arguments start at most align-4 bytes into the binding, so
	// align+8 bytes always hold them; the end of the buffer may cut it
	// shorter, never below the arguments' end.
	size := min(bufferSize-bindingOffset, align+DispatchArgsSize-4) &^ 3
	return DispatchSource{
		BindingOffset: bindingOffset,
		BindingSize:   size,
		Index:         uint32((offset - bindingOffset) / 4), //nolint:gosec // below alignment/4
	}
}

// BindGroupCache holds the validation bind groups of one arguments buffer,
// one per binding offset, created on first use. The bind groups live as long
// as the buffer: commands recorded with them may run until the buffer's last
// use completes, so they are destroyed with the buffer (Rust wgpu-core
// Buffer::indirect_validation_bind_groups, disposed when the buffer drops).
type BindGroupCache struct {
	mu     sync.Mutex
	groups map[uint64]hal.BindGroup
}

// Get returns the bind group for key, creating it with create on first use.
func (c *BindGroupCache) Get(key uint64, create func() (hal.BindGroup, error)) (hal.BindGroup, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if g, ok := c.groups[key]; ok {
		return g, nil
	}
	g, err := create()
	if err != nil {
		return nil, err
	}
	if c.groups == nil {
		c.groups = make(map[uint64]hal.BindGroup)
	}
	c.groups[key] = g
	return g, nil
}

// Destroy destroys every bind group in the cache with device and empties it.
func (c *BindGroupCache) Destroy(device hal.Device) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, g := range c.groups {
		device.DestroyBindGroup(g)
	}
	c.groups = nil
}
