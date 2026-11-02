//go:build !rust && !(js && wasm)

package wgpu

import (
	"fmt"
	"sync"

	"github.com/gogpu/wgpu/hal"
)

// encoderPool pools hal.CommandEncoder instances to avoid creating expensive
// GPU resources (DX12 command allocators, Vulkan command pools) every frame.
// Matches Rust wgpu-core's CommandAllocator pattern (allocator.rs).
//
// Each encoder in the pool is in "closed" state — ready for BeginEncoding.
// After EndEncoding returns a command buffer, the encoder retains its internal
// resources. After GPU completion, ResetAll prepares the encoder for reuse.
//
// The pool is backend-agnostic: DX12 encoders keep their ID3D12CommandAllocator,
// Vulkan encoders keep their VkCommandPool, and lightweight backends (GLES,
// Software, Metal, Noop) pass through with negligible overhead.
type encoderPool struct {
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

// newEncoderPool creates a new encoder pool for the given HAL device.
func newEncoderPool(halDevice hal.Device) *encoderPool {
	return &encoderPool{
		outstanding: make(map[hal.CommandEncoder]struct{}),
		halDevice:   halDevice,
	}
}

// poolManagedSetter is implemented by HAL encoders that need special setup
// when managed by the encoder pool (e.g., Vulkan CommandEncoder).
type poolManagedSetter interface {
	SetPoolManaged(managed bool)
}

// acquire returns a command encoder from the pool, or creates a new one.
// The returned encoder is in "closed" state, ready for BeginEncoding.
func (p *encoderPool) acquire() (hal.CommandEncoder, error) {
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

// release returns an encoder to the pool after ResetAll has been called.
// The encoder must be in "closed" state (not recording).
func (p *encoderPool) release(enc hal.CommandEncoder) {
	p.mu.Lock()
	delete(p.outstanding, enc)
	p.free = append(p.free, enc)
	p.mu.Unlock()
}

// retire destroys an acquired encoder that its holder will not return (the
// pending writes of a device being released).
func (p *encoderPool) retire(enc hal.CommandEncoder) {
	p.mu.Lock()
	delete(p.outstanding, enc)
	p.mu.Unlock()
	enc.Destroy()
}

// destroy releases every encoder of the pool, the pooled ones and the ones
// still handed out. Called during device shutdown, after the GPU is idle:
// destroying an encoder frees its command buffers in any state but pending.
func (p *encoderPool) destroy() {
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
