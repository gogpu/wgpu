//go:build !rust && !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package encoder

// This file holds the device's HAL command encoder pool. It arrived here from
// the root package with the device in the S11 Move (ADR-070 §15.1: encoder.Pool),
// which supersedes the two stale comments on the old root files: encoder_pool.go
// said the pool "moves to internal/device at S11" and recording_registry.go said
// the same. The contract is the ADR's, and the types implement internal/core's
// seam interfaces, which the encoder domain names.

import (
	"fmt"
	"sync"

	"github.com/gogpu/wgpu/internal/hal"
)

// Pool pools hal.CommandEncoder instances to avoid creating expensive GPU
// resources (DX12 command allocators, Vulkan command pools) every frame.
// Matches Rust wgpu-core's CommandAllocator pattern (allocator.rs).
//
// Each encoder in the pool is in "closed" state — ready for BeginEncoding.
// After EndEncoding returns a command buffer, the encoder retains its internal
// resources. After GPU completion, ResetAll prepares the encoder for reuse.
//
// The pool is backend-agnostic: DX12 encoders keep their ID3D12CommandAllocator,
// Vulkan encoders keep their VkCommandPool, and lightweight backends (GLES,
// Software, Metal, Noop) pass through with negligible overhead.
type Pool struct {
	mu   sync.Mutex
	free []hal.CommandEncoder

	// outstanding holds the encoders acquire handed out that have not come
	// back through release or retire: recording, finished but not yet
	// recycled, or abandoned by the caller (an encoder never finished or
	// discarded, a command buffer never submitted or released). Rust returns
	// an abandoned encoder from InnerCommandEncoder::drop, and the device
	// outlives it; Go has no destructor, so destroy releases these too. On
	// Vulkan a command buffer alive at vkDestroyDevice is
	// VUID-vkDestroyDevice-device-05137. Keyed by identity: encoders of a
	// zero-size type (noop) may share a key, which is harmless because their
	// Destroy does nothing.
	outstanding map[hal.CommandEncoder]struct{}

	halDevice hal.Device
}

// NewPool creates a new encoder pool for the given HAL device.
func NewPool(halDevice hal.Device) *Pool {
	return &Pool{
		outstanding: make(map[hal.CommandEncoder]struct{}),
		halDevice:   halDevice,
	}
}

// poolManagedSetter is implemented by HAL encoders that need special setup
// when managed by the encoder pool (e.g., Vulkan CommandEncoder).
type poolManagedSetter interface {
	SetPoolManaged(managed bool)
}

// Acquire returns a command encoder from the pool, or creates a new one.
// The returned encoder is in "closed" state, ready for BeginEncoding.
func (p *Pool) Acquire() (hal.CommandEncoder, error) {
	p.mu.Lock()
	if n := len(p.free); n > 0 {
		enc := p.free[n-1]
		p.free[n-1] = nil // avoid retaining reference
		p.free = p.free[:n-1]
		p.outstanding[enc] = struct{}{}
		p.mu.Unlock()
		return enc, nil
	}
	p.mu.Unlock()

	// Create a new encoder from the HAL device.
	enc, err := p.halDevice.CreateCommandEncoder(&hal.CommandEncoderDescriptor{
		Label: "(wgpu internal) pending writes",
	})
	if err != nil {
		return nil, fmt.Errorf("encoder pool: create encoder: %w", err)
	}

	// Mark as pool-managed if the backend supports it (e.g., Vulkan).
	if setter, ok := enc.(poolManagedSetter); ok {
		setter.SetPoolManaged(true)
	}

	p.mu.Lock()
	p.outstanding[enc] = struct{}{}
	p.mu.Unlock()
	return enc, nil
}

// Release returns an encoder to the pool after ResetAll has been called.
// The encoder must be in "closed" state (not recording).
func (p *Pool) Release(enc hal.CommandEncoder) {
	p.mu.Lock()
	delete(p.outstanding, enc)
	p.free = append(p.free, enc)
	p.mu.Unlock()
}

// Retire destroys an acquired encoder that its holder will not return (the
// pending writes of a device being released).
func (p *Pool) Retire(enc hal.CommandEncoder) {
	p.mu.Lock()
	delete(p.outstanding, enc)
	p.mu.Unlock()
	enc.Destroy()
}

// Destroy releases every encoder of the pool, the pooled ones and the ones
// still handed out. Called during device shutdown, after the GPU is idle:
// destroying an encoder frees its command buffers in any state but pending.
func (p *Pool) Destroy() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, enc := range p.free {
		enc.Destroy()
	}
	p.free = nil
	for enc := range p.outstanding {
		enc.Destroy()
	}
	clear(p.outstanding)
}

// FreeCount returns the number of encoders the pool holds ready for reuse.
// The root package's test hooks read it; the pool's lifecycle never uses it.
func (p *Pool) FreeCount() int {
	if p == nil {
		return 0
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.free)
}

// OutstandingCount returns the number of encoders the pool handed out that
// have not come back. The root package's test hooks read it; the pool's
// lifecycle never uses it.
func (p *Pool) OutstandingCount() int {
	if p == nil {
		return 0
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.outstanding)
}
