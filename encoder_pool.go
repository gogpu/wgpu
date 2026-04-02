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
	mu        sync.Mutex
	free      []hal.CommandEncoder
	halDevice hal.Device
}

// newEncoderPool creates a new encoder pool for the given HAL device.
func newEncoderPool(halDevice hal.Device) *encoderPool {
	return &encoderPool{
		halDevice: halDevice,
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

	return enc, nil
}

// release returns an encoder to the pool after ResetAll has been called.
// The encoder must be in "closed" state (not recording).
func (p *encoderPool) release(enc hal.CommandEncoder) {
	p.mu.Lock()
	p.free = append(p.free, enc)
	p.mu.Unlock()
}

// destroy releases all pooled encoders. Called during device shutdown.
func (p *encoderPool) destroy() {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, enc := range p.free {
		enc.Destroy()
	}
	p.free = nil
}
