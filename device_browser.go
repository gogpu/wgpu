//go:build js && wasm

package wgpu

// Device represents a logical GPU device.
// On browser, this wraps a JavaScript GPUDevice via syscall/js.
type Device struct {
	released bool
}

// Queue returns the device's command queue.
func (d *Device) Queue() *Queue {
	panic("wgpu: browser backend not yet implemented")
}

// Features returns the device's enabled features.
func (d *Device) Features() Features {
	panic("wgpu: browser backend not yet implemented")
}

// Limits returns the device's resource limits.
func (d *Device) Limits() Limits {
	panic("wgpu: browser backend not yet implemented")
}

// CreateBuffer creates a GPU buffer.
func (d *Device) CreateBuffer(desc *BufferDescriptor) (*Buffer, error) {
	panic("wgpu: browser backend not yet implemented")
}

// CreateTexture creates a GPU texture.
func (d *Device) CreateTexture(desc *TextureDescriptor) (*Texture, error) {
	panic("wgpu: browser backend not yet implemented")
}

// CreateTextureView creates a view into a texture.
func (d *Device) CreateTextureView(texture *Texture, desc *TextureViewDescriptor) (*TextureView, error) {
	panic("wgpu: browser backend not yet implemented")
}

// CreateSampler creates a texture sampler.
func (d *Device) CreateSampler(desc *SamplerDescriptor) (*Sampler, error) {
	panic("wgpu: browser backend not yet implemented")
}

// CreateShaderModule creates a shader module.
func (d *Device) CreateShaderModule(desc *ShaderModuleDescriptor) (*ShaderModule, error) {
	panic("wgpu: browser backend not yet implemented")
}

// CreateBindGroupLayout creates a bind group layout.
func (d *Device) CreateBindGroupLayout(desc *BindGroupLayoutDescriptor) (*BindGroupLayout, error) {
	panic("wgpu: browser backend not yet implemented")
}

// CreatePipelineLayout creates a pipeline layout.
func (d *Device) CreatePipelineLayout(desc *PipelineLayoutDescriptor) (*PipelineLayout, error) {
	panic("wgpu: browser backend not yet implemented")
}

// CreateBindGroup creates a bind group.
func (d *Device) CreateBindGroup(desc *BindGroupDescriptor) (*BindGroup, error) {
	panic("wgpu: browser backend not yet implemented")
}

// CreateRenderPipeline creates a render pipeline.
func (d *Device) CreateRenderPipeline(desc *RenderPipelineDescriptor) (*RenderPipeline, error) {
	panic("wgpu: browser backend not yet implemented")
}

// CreateComputePipeline creates a compute pipeline.
func (d *Device) CreateComputePipeline(desc *ComputePipelineDescriptor) (*ComputePipeline, error) {
	panic("wgpu: browser backend not yet implemented")
}

// CreateCommandEncoder creates a command encoder for recording GPU commands.
func (d *Device) CreateCommandEncoder(desc *CommandEncoderDescriptor) (*CommandEncoder, error) {
	panic("wgpu: browser backend not yet implemented")
}

// CreateFence creates a GPU synchronization fence.
func (d *Device) CreateFence() (*Fence, error) {
	panic("wgpu: browser backend not yet implemented")
}

// PushErrorScope pushes a new error scope onto the device's error scope stack.
func (d *Device) PushErrorScope(filter ErrorFilter) {
	panic("wgpu: browser backend not yet implemented")
}

// PopErrorScope pops the most recently pushed error scope.
func (d *Device) PopErrorScope() *GPUError {
	panic("wgpu: browser backend not yet implemented")
}

// WaitIdle waits for all GPU work to complete.
func (d *Device) WaitIdle() error {
	panic("wgpu: browser backend not yet implemented")
}

// Poll drives the per-device pending-map triage loop.
func (d *Device) Poll(pollType PollType) bool {
	panic("wgpu: browser backend not yet implemented")
}

// Release releases the device and all associated resources.
func (d *Device) Release() {
	if d.released {
		return
	}
	d.released = true
}
