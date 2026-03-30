package wgpu

import (
	"fmt"
	"time"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/core"
	"github.com/gogpu/wgpu/hal"
)

// Device represents a logical GPU device.
// It is the main interface for creating GPU resources.
//
// Device methods are safe for concurrent use, except Release() which
// must not be called concurrently with other methods.
type Device struct {
	core     *core.Device
	queue    *Queue
	released bool
}

// Queue returns the device's command queue.
func (d *Device) Queue() *Queue {
	return d.queue
}

// Features returns the device's enabled features.
func (d *Device) Features() Features {
	return d.core.Features
}

// Limits returns the device's resource limits.
func (d *Device) Limits() Limits {
	return d.core.Limits
}

// CreateBuffer creates a GPU buffer.
func (d *Device) CreateBuffer(desc *BufferDescriptor) (*Buffer, error) {
	if d.released {
		return nil, ErrReleased
	}
	if desc == nil {
		return nil, fmt.Errorf("wgpu: buffer descriptor is nil")
	}

	gpuDesc := &gputypes.BufferDescriptor{
		Label:            desc.Label,
		Size:             desc.Size,
		Usage:            desc.Usage,
		MappedAtCreation: desc.MappedAtCreation,
	}

	coreBuffer, err := d.core.CreateBuffer(gpuDesc)
	if err != nil {
		return nil, err
	}

	return &Buffer{core: coreBuffer, device: d}, nil
}

// CreateTexture creates a GPU texture.
func (d *Device) CreateTexture(desc *TextureDescriptor) (*Texture, error) {
	if d.released {
		return nil, ErrReleased
	}
	if desc == nil {
		return nil, fmt.Errorf("wgpu: texture descriptor is nil")
	}

	halDevice := d.halDevice()
	if halDevice == nil {
		return nil, ErrReleased
	}

	halDesc := desc.toHAL()

	if err := core.ValidateTextureDescriptor(halDesc, d.core.Limits); err != nil {
		return nil, err
	}

	halTexture, err := halDevice.CreateTexture(halDesc)
	if err != nil {
		return nil, fmt.Errorf("wgpu: failed to create texture: %w", err)
	}

	return &Texture{hal: halTexture, device: d, format: desc.Format}, nil
}

// CreateTextureView creates a view into a texture.
func (d *Device) CreateTextureView(texture *Texture, desc *TextureViewDescriptor) (*TextureView, error) {
	if d.released {
		return nil, ErrReleased
	}
	if texture == nil {
		return nil, fmt.Errorf("wgpu: texture is nil")
	}

	halDevice := d.halDevice()
	if halDevice == nil {
		return nil, ErrReleased
	}

	halDesc := &hal.TextureViewDescriptor{}
	if desc != nil {
		halDesc.Label = desc.Label
		halDesc.Format = desc.Format
		halDesc.Dimension = desc.Dimension
		halDesc.Aspect = desc.Aspect
		halDesc.BaseMipLevel = desc.BaseMipLevel
		halDesc.MipLevelCount = desc.MipLevelCount
		halDesc.BaseArrayLayer = desc.BaseArrayLayer
		halDesc.ArrayLayerCount = desc.ArrayLayerCount
	}

	halView, err := halDevice.CreateTextureView(texture.hal, halDesc)
	if err != nil {
		return nil, fmt.Errorf("wgpu: failed to create texture view: %w", err)
	}

	return &TextureView{hal: halView, device: d, texture: texture}, nil
}

// CreateSampler creates a texture sampler.
func (d *Device) CreateSampler(desc *SamplerDescriptor) (*Sampler, error) {
	if d.released {
		return nil, ErrReleased
	}

	halDevice := d.halDevice()
	if halDevice == nil {
		return nil, ErrReleased
	}

	halDesc := &hal.SamplerDescriptor{}
	if desc != nil {
		halDesc.Label = desc.Label
		halDesc.AddressModeU = desc.AddressModeU
		halDesc.AddressModeV = desc.AddressModeV
		halDesc.AddressModeW = desc.AddressModeW
		halDesc.MagFilter = desc.MagFilter
		halDesc.MinFilter = desc.MinFilter
		halDesc.MipmapFilter = desc.MipmapFilter
		halDesc.LodMinClamp = desc.LodMinClamp
		halDesc.LodMaxClamp = desc.LodMaxClamp
		halDesc.Compare = desc.Compare
		halDesc.Anisotropy = desc.Anisotropy
	}

	if err := core.ValidateSamplerDescriptor(halDesc); err != nil {
		return nil, err
	}

	halSampler, err := halDevice.CreateSampler(halDesc)
	if err != nil {
		return nil, fmt.Errorf("wgpu: failed to create sampler: %w", err)
	}

	return &Sampler{hal: halSampler, device: d}, nil
}

// CreateShaderModule creates a shader module.
func (d *Device) CreateShaderModule(desc *ShaderModuleDescriptor) (*ShaderModule, error) {
	if d.released {
		return nil, ErrReleased
	}
	if desc == nil {
		return nil, fmt.Errorf("wgpu: shader module descriptor is nil")
	}

	halDevice := d.halDevice()
	if halDevice == nil {
		return nil, ErrReleased
	}

	halDesc := &hal.ShaderModuleDescriptor{
		Label: desc.Label,
		Source: hal.ShaderSource{
			WGSL:  desc.WGSL,
			SPIRV: desc.SPIRV,
		},
	}

	if err := core.ValidateShaderModuleDescriptor(halDesc); err != nil {
		return nil, err
	}

	halModule, err := halDevice.CreateShaderModule(halDesc)
	if err != nil {
		return nil, fmt.Errorf("wgpu: failed to create shader module: %w", err)
	}

	return &ShaderModule{hal: halModule, device: d}, nil
}

// CreateBindGroupLayout creates a bind group layout.
func (d *Device) CreateBindGroupLayout(desc *BindGroupLayoutDescriptor) (*BindGroupLayout, error) {
	if d.released {
		return nil, ErrReleased
	}
	if desc == nil {
		return nil, fmt.Errorf("wgpu: bind group layout descriptor is nil")
	}

	halDevice := d.halDevice()
	if halDevice == nil {
		return nil, ErrReleased
	}

	halDesc := &hal.BindGroupLayoutDescriptor{
		Label:   desc.Label,
		Entries: desc.Entries,
	}

	if err := core.ValidateBindGroupLayoutDescriptor(halDesc, d.core.Limits); err != nil {
		return nil, err
	}

	halLayout, err := halDevice.CreateBindGroupLayout(halDesc)
	if err != nil {
		return nil, fmt.Errorf("wgpu: failed to create bind group layout: %w", err)
	}

	// Store a defensive copy of entries for entry-by-entry compatibility checks.
	// This matches Rust wgpu-core's pattern where binder compares layouts by entries.
	entriesCopy := make([]gputypes.BindGroupLayoutEntry, len(desc.Entries))
	copy(entriesCopy, desc.Entries)

	return &BindGroupLayout{hal: halLayout, device: d, entries: entriesCopy}, nil
}

// CreatePipelineLayout creates a pipeline layout.
func (d *Device) CreatePipelineLayout(desc *PipelineLayoutDescriptor) (*PipelineLayout, error) {
	if d.released {
		return nil, ErrReleased
	}
	if desc == nil {
		return nil, fmt.Errorf("wgpu: pipeline layout descriptor is nil")
	}

	halDevice := d.halDevice()
	if halDevice == nil {
		return nil, ErrReleased
	}

	halLayouts := make([]hal.BindGroupLayout, len(desc.BindGroupLayouts))
	for i, layout := range desc.BindGroupLayouts {
		if layout == nil {
			return nil, fmt.Errorf("wgpu: bind group layout at index %d is nil", i)
		}
		halLayouts[i] = layout.hal
	}

	halDesc := &hal.PipelineLayoutDescriptor{
		Label:            desc.Label,
		BindGroupLayouts: halLayouts,
	}

	halLayout, err := halDevice.CreatePipelineLayout(halDesc)
	if err != nil {
		return nil, fmt.Errorf("wgpu: failed to create pipeline layout: %w", err)
	}

	// Store a copy of the bind group layouts slice for binder validation.
	bgLayouts := make([]*BindGroupLayout, len(desc.BindGroupLayouts))
	copy(bgLayouts, desc.BindGroupLayouts)

	return &PipelineLayout{
		hal:              halLayout,
		device:           d,
		bindGroupCount:   uint32(len(desc.BindGroupLayouts)), //nolint:gosec // layout count fits uint32
		bindGroupLayouts: bgLayouts,
	}, nil
}

// CreateBindGroup creates a bind group.
func (d *Device) CreateBindGroup(desc *BindGroupDescriptor) (*BindGroup, error) {
	if d.released {
		return nil, ErrReleased
	}
	if desc == nil {
		return nil, fmt.Errorf("wgpu: bind group descriptor is nil")
	}

	halDevice := d.halDevice()
	if halDevice == nil {
		return nil, ErrReleased
	}

	if desc.Layout == nil {
		return nil, &core.CreateBindGroupError{
			Kind:  core.CreateBindGroupErrorMissingLayout,
			Label: desc.Label,
		}
	}

	halEntries := make([]gputypes.BindGroupEntry, len(desc.Entries))
	for i, entry := range desc.Entries {
		halEntries[i] = entry.toHAL()
	}

	halDesc := &hal.BindGroupDescriptor{
		Label:   desc.Label,
		Layout:  desc.Layout.hal,
		Entries: halEntries,
	}

	halGroup, err := halDevice.CreateBindGroup(halDesc)
	if err != nil {
		return nil, fmt.Errorf("wgpu: failed to create bind group: %w", err)
	}

	return &BindGroup{hal: halGroup, device: d, layout: desc.Layout}, nil
}

// CreateRenderPipeline creates a render pipeline.
func (d *Device) CreateRenderPipeline(desc *RenderPipelineDescriptor) (*RenderPipeline, error) {
	if d.released {
		return nil, ErrReleased
	}
	if desc == nil {
		return nil, fmt.Errorf("wgpu: render pipeline descriptor is nil")
	}

	halDevice := d.halDevice()
	if halDevice == nil {
		return nil, ErrReleased
	}

	halDesc := desc.toHAL()

	if err := core.ValidateRenderPipelineDescriptor(halDesc, d.core.Limits); err != nil {
		return nil, err
	}

	halPipeline, err := halDevice.CreateRenderPipeline(halDesc)
	if err != nil {
		return nil, fmt.Errorf("wgpu: failed to create render pipeline: %w", err)
	}

	var bgCount uint32
	var bgLayouts []*BindGroupLayout
	if desc.Layout != nil {
		bgCount = desc.Layout.bindGroupCount
		bgLayouts = desc.Layout.bindGroupLayouts
	}
	return &RenderPipeline{
		hal:                   halPipeline,
		device:                d,
		bindGroupCount:        bgCount,
		bindGroupLayouts:      bgLayouts,
		requiredVertexBuffers: uint32(len(desc.Vertex.Buffers)), //nolint:gosec // buffer count fits uint32
	}, nil
}

// CreateComputePipeline creates a compute pipeline.
func (d *Device) CreateComputePipeline(desc *ComputePipelineDescriptor) (*ComputePipeline, error) {
	if d.released {
		return nil, ErrReleased
	}
	if desc == nil {
		return nil, fmt.Errorf("wgpu: compute pipeline descriptor is nil")
	}

	halDevice := d.halDevice()
	if halDevice == nil {
		return nil, ErrReleased
	}

	halDesc := desc.toHAL()

	if err := core.ValidateComputePipelineDescriptor(halDesc); err != nil {
		return nil, err
	}

	halPipeline, err := halDevice.CreateComputePipeline(halDesc)
	if err != nil {
		return nil, fmt.Errorf("wgpu: failed to create compute pipeline: %w", err)
	}

	var bgCount uint32
	var bgLayouts []*BindGroupLayout
	if desc.Layout != nil {
		bgCount = desc.Layout.bindGroupCount
		bgLayouts = desc.Layout.bindGroupLayouts
	}
	return &ComputePipeline{
		hal:              halPipeline,
		device:           d,
		bindGroupCount:   bgCount,
		bindGroupLayouts: bgLayouts,
	}, nil
}

// CreateCommandEncoder creates a command encoder for recording GPU commands.
func (d *Device) CreateCommandEncoder(desc *CommandEncoderDescriptor) (*CommandEncoder, error) {
	if d.released {
		return nil, ErrReleased
	}

	label := ""
	if desc != nil {
		label = desc.Label
	}

	coreEncoder, err := d.core.CreateCommandEncoder(label)
	if err != nil {
		return nil, err
	}

	return &CommandEncoder{core: coreEncoder, device: d}, nil
}

// CreateFence creates a GPU synchronization fence.
// Fences are primarily used by the HAL internally for synchronization.
// Most callers should use Queue.Submit + Queue.Poll instead.
func (d *Device) CreateFence() (*Fence, error) {
	if d.released {
		return nil, ErrReleased
	}
	halDevice := d.halDevice()
	if halDevice == nil {
		return nil, ErrReleased
	}

	halFence, err := halDevice.CreateFence()
	if err != nil {
		return nil, fmt.Errorf("wgpu: failed to create fence: %w", err)
	}

	return &Fence{hal: halFence, device: d}, nil
}

// DestroyFence destroys a fence.
// The fence must not be in use by the GPU when destroyed.
//
// Deprecated: Use Fence.Release() instead.
func (d *Device) DestroyFence(f *Fence) {
	if f != nil {
		f.Release()
	}
}

// ResetFence resets a fence to the unsignaled state.
// The fence must not be in use by the GPU.
func (d *Device) ResetFence(f *Fence) error {
	if d.released {
		return ErrReleased
	}
	if f == nil || f.released {
		return ErrReleased
	}
	halDevice := d.halDevice()
	if halDevice == nil {
		return ErrReleased
	}
	return halDevice.ResetFence(f.hal)
}

// GetFenceStatus returns true if the fence is signaled (non-blocking).
// This is used for polling completion without blocking.
func (d *Device) GetFenceStatus(f *Fence) (bool, error) {
	if d.released {
		return false, ErrReleased
	}
	if f == nil || f.released {
		return false, ErrReleased
	}
	halDevice := d.halDevice()
	if halDevice == nil {
		return false, ErrReleased
	}
	return halDevice.GetFenceStatus(f.hal)
}

// WaitForFence waits for a fence to reach the specified value.
// Returns true if the fence reached the value, false if timeout expired.
func (d *Device) WaitForFence(f *Fence, value uint64, timeout time.Duration) (bool, error) {
	if d.released {
		return false, ErrReleased
	}
	if f == nil || f.released {
		return false, ErrReleased
	}
	halDevice := d.halDevice()
	if halDevice == nil {
		return false, ErrReleased
	}
	return halDevice.Wait(f.hal, value, timeout)
}

// FreeCommandBuffer returns a command buffer to the command pool.
// This must be called after the GPU has finished using the command buffer.
// The command buffer handle becomes invalid after this call.
func (d *Device) FreeCommandBuffer(cb *CommandBuffer) {
	if d.released || cb == nil {
		return
	}
	halDevice := d.halDevice()
	if halDevice == nil {
		return
	}
	raw := cb.halBuffer()
	if raw != nil {
		halDevice.FreeCommandBuffer(raw)
	}
}

// PushErrorScope pushes a new error scope onto the device's error scope stack.
func (d *Device) PushErrorScope(filter ErrorFilter) {
	d.core.PushErrorScope(filter)
}

// PopErrorScope pops the most recently pushed error scope.
// Returns the captured error, or nil if no error occurred.
func (d *Device) PopErrorScope() *GPUError {
	return d.core.PopErrorScope()
}

// WaitIdle waits for all GPU work to complete.
func (d *Device) WaitIdle() error {
	if d.released {
		return ErrReleased
	}
	halDevice := d.halDevice()
	if halDevice == nil {
		return ErrReleased
	}
	return halDevice.WaitIdle()
}

// Release releases the device and all associated resources.
func (d *Device) Release() {
	if d.released {
		return
	}
	d.released = true

	if d.queue != nil {
		d.queue.release()
	}

	d.core.Destroy()
}

// halDevice returns the underlying HAL device for direct resource creation.
func (d *Device) halDevice() hal.Device {
	if d.core == nil || !d.core.HasHAL() {
		return nil
	}
	guard := d.core.SnatchLock().Read()
	defer guard.Release()
	return d.core.Raw(guard)
}
