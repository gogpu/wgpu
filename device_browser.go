//go:build js && wasm

package wgpu

import (
	"github.com/gogpu/wgpu/internal/browser"
)

// Device represents a logical GPU device.
// On browser, this wraps a GPUDevice via internal/browser.Device.
type Device struct {
	browser  *browser.Device
	queue    *Queue
	features Features
	limits   Limits
	released bool
}

// Queue returns the device's command queue.
func (d *Device) Queue() *Queue {
	return d.queue
}

// Features returns the device's enabled features.
func (d *Device) Features() Features {
	return d.features
}

// Limits returns the device's resource limits.
func (d *Device) Limits() Limits {
	return d.limits
}

// CreateBuffer creates a GPU buffer.
// Phase 2 — not yet implemented for browser.
func (d *Device) CreateBuffer(desc *BufferDescriptor) (*Buffer, error) {
	panic("wgpu: browser CreateBuffer not yet implemented (Phase 2)")
}

// CreateTexture creates a GPU texture.
// Phase 2 — not yet implemented for browser.
func (d *Device) CreateTexture(desc *TextureDescriptor) (*Texture, error) {
	panic("wgpu: browser CreateTexture not yet implemented (Phase 2)")
}

// CreateTextureView creates a view into a texture.
// Phase 2 — not yet implemented for browser.
func (d *Device) CreateTextureView(texture *Texture, desc *TextureViewDescriptor) (*TextureView, error) {
	panic("wgpu: browser CreateTextureView not yet implemented (Phase 2)")
}

// CreateSampler creates a texture sampler.
// Phase 2 — not yet implemented for browser.
func (d *Device) CreateSampler(desc *SamplerDescriptor) (*Sampler, error) {
	panic("wgpu: browser CreateSampler not yet implemented (Phase 2)")
}

// CreateShaderModule creates a shader module.
// Phase 2 — not yet implemented for browser.
func (d *Device) CreateShaderModule(desc *ShaderModuleDescriptor) (*ShaderModule, error) {
	panic("wgpu: browser CreateShaderModule not yet implemented (Phase 2)")
}

// CreateBindGroupLayout creates a bind group layout.
// Phase 2 — not yet implemented for browser.
func (d *Device) CreateBindGroupLayout(desc *BindGroupLayoutDescriptor) (*BindGroupLayout, error) {
	panic("wgpu: browser CreateBindGroupLayout not yet implemented (Phase 2)")
}

// CreatePipelineLayout creates a pipeline layout.
// Phase 2 — not yet implemented for browser.
func (d *Device) CreatePipelineLayout(desc *PipelineLayoutDescriptor) (*PipelineLayout, error) {
	panic("wgpu: browser CreatePipelineLayout not yet implemented (Phase 2)")
}

// CreateBindGroup creates a bind group.
// Phase 2 — not yet implemented for browser.
func (d *Device) CreateBindGroup(desc *BindGroupDescriptor) (*BindGroup, error) {
	panic("wgpu: browser CreateBindGroup not yet implemented (Phase 2)")
}

// CreateRenderPipeline creates a render pipeline.
// Phase 2 — not yet implemented for browser.
func (d *Device) CreateRenderPipeline(desc *RenderPipelineDescriptor) (*RenderPipeline, error) {
	panic("wgpu: browser CreateRenderPipeline not yet implemented (Phase 2)")
}

// CreateComputePipeline creates a compute pipeline.
// Phase 2 — not yet implemented for browser.
func (d *Device) CreateComputePipeline(desc *ComputePipelineDescriptor) (*ComputePipeline, error) {
	panic("wgpu: browser CreateComputePipeline not yet implemented (Phase 2)")
}

// CreateCommandEncoder creates a command encoder for recording GPU commands.
// Phase 3 — not yet implemented for browser.
func (d *Device) CreateCommandEncoder(desc *CommandEncoderDescriptor) (*CommandEncoder, error) {
	panic("wgpu: browser CreateCommandEncoder not yet implemented (Phase 3)")
}

// CreateFence creates a GPU synchronization fence.
// On browser, fences are not needed (browser auto-polls).
// Returns a no-op fence for API compatibility.
func (d *Device) CreateFence() (*Fence, error) {
	if d.released {
		return nil, ErrReleased
	}
	return &Fence{}, nil
}

// PushErrorScope pushes a new error scope onto the device's error scope stack.
// Phase 2 — not yet implemented for browser.
func (d *Device) PushErrorScope(filter ErrorFilter) {
	panic("wgpu: browser PushErrorScope not yet implemented (Phase 2)")
}

// PopErrorScope pops the most recently pushed error scope.
// Phase 2 — not yet implemented for browser.
func (d *Device) PopErrorScope() *GPUError {
	panic("wgpu: browser PopErrorScope not yet implemented (Phase 2)")
}

// WaitIdle waits for all GPU work to complete.
// On browser, the GPU is polled automatically. This is a no-op.
func (d *Device) WaitIdle() error {
	return nil
}

// Poll drives the per-device pending-map triage loop.
// On browser, the GPU is polled automatically by the browser event loop.
// Returns true (devices are always considered polled in browser).
func (d *Device) Poll(pollType PollType) bool {
	return true
}

// Release releases the device and all associated resources.
func (d *Device) Release() {
	if d.released {
		return
	}
	d.released = true
	if d.browser != nil {
		d.browser.Destroy()
	}
}
