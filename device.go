package wgpu

import (
	"fmt"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/core"
	"github.com/gogpu/wgpu/hal"
)

// Device represents a logical GPU device.
// It is the main interface for creating GPU resources.
//
// Thread-safe for concurrent use.
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

	halDesc := &hal.TextureDescriptor{
		Label:         desc.Label,
		Size:          hal.Extent3D{Width: desc.Size.Width, Height: desc.Size.Height, DepthOrArrayLayers: desc.Size.DepthOrArrayLayers},
		MipLevelCount: desc.MipLevelCount,
		SampleCount:   desc.SampleCount,
		Dimension:     desc.Dimension,
		Format:        desc.Format,
		Usage:         desc.Usage,
		ViewFormats:   desc.ViewFormats,
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

	halLayout, err := halDevice.CreateBindGroupLayout(halDesc)
	if err != nil {
		return nil, fmt.Errorf("wgpu: failed to create bind group layout: %w", err)
	}

	return &BindGroupLayout{hal: halLayout, device: d}, nil
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

	return &PipelineLayout{hal: halLayout, device: d}, nil
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

	return &BindGroup{hal: halGroup, device: d}, nil
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

	halPipeline, err := halDevice.CreateRenderPipeline(halDesc)
	if err != nil {
		return nil, fmt.Errorf("wgpu: failed to create render pipeline: %w", err)
	}

	return &RenderPipeline{hal: halPipeline, device: d}, nil
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

	halPipeline, err := halDevice.CreateComputePipeline(halDesc)
	if err != nil {
		return nil, fmt.Errorf("wgpu: failed to create compute pipeline: %w", err)
	}

	return &ComputePipeline{hal: halPipeline, device: d}, nil
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
