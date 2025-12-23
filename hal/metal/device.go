// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build darwin

package metal

import (
	"fmt"
	"time"

	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/types"
)

// Device implements hal.Device for Metal.
type Device struct {
	raw          ID // id<MTLDevice>
	commandQueue ID // id<MTLCommandQueue>
	adapter      *Adapter
}

// newDevice creates a new Device from a Metal device.
func newDevice(adapter *Adapter) (*Device, error) {
	if adapter.raw == 0 {
		return nil, fmt.Errorf("metal: adapter has no device")
	}

	queue := MsgSend(adapter.raw, Sel("newCommandQueue"))
	if queue == 0 {
		return nil, fmt.Errorf("metal: failed to create command queue")
	}

	return &Device{
		raw:          adapter.raw,
		commandQueue: queue,
		adapter:      adapter,
	}, nil
}

// CreateBuffer creates a GPU buffer.
func (d *Device) CreateBuffer(desc *hal.BufferDescriptor) (hal.Buffer, error) {
	if desc == nil {
		return nil, fmt.Errorf("metal: buffer descriptor is nil")
	}
	if desc.Size == 0 {
		return nil, fmt.Errorf("metal: buffer size must be > 0")
	}

	var options MTLResourceOptions
	mapRead := desc.Usage&types.BufferUsageMapRead != 0
	mapWrite := desc.Usage&types.BufferUsageMapWrite != 0

	if mapRead || mapWrite {
		options = MTLResourceStorageModeShared
	} else {
		options = MTLResourceStorageModePrivate
	}

	if mapWrite && !mapRead {
		options |= MTLResourceCPUCacheModeWriteCombined
	}

	pool := NewAutoreleasePool()
	defer pool.Drain()

	raw := MsgSend(d.raw, Sel("newBufferWithLength:options:"),
		uintptr(desc.Size), uintptr(options))
	if raw == 0 {
		return nil, fmt.Errorf("metal: failed to create buffer")
	}

	if desc.Label != "" {
		label := NSString(desc.Label)
		_ = MsgSend(raw, Sel("setLabel:"), uintptr(label))
	}

	return &Buffer{
		raw:     raw,
		size:    desc.Size,
		usage:   desc.Usage,
		options: options,
		device:  d,
	}, nil
}

// DestroyBuffer destroys a GPU buffer.
func (d *Device) DestroyBuffer(buffer hal.Buffer) {
	mtlBuffer, ok := buffer.(*Buffer)
	if !ok || mtlBuffer == nil {
		return
	}
	if mtlBuffer.raw != 0 {
		Release(mtlBuffer.raw)
		mtlBuffer.raw = 0
	}
	mtlBuffer.device = nil
}

// CreateTexture creates a GPU texture.
func (d *Device) CreateTexture(desc *hal.TextureDescriptor) (hal.Texture, error) {
	if desc == nil {
		return nil, fmt.Errorf("metal: texture descriptor is nil")
	}
	if desc.Size.Width == 0 || desc.Size.Height == 0 {
		return nil, fmt.Errorf("metal: texture size must be > 0")
	}

	pool := NewAutoreleasePool()
	defer pool.Drain()

	texDesc := MsgSend(ID(GetClass("MTLTextureDescriptor")), Sel("new"))
	if texDesc == 0 {
		return nil, fmt.Errorf("metal: failed to create texture descriptor")
	}
	defer Release(texDesc)

	texType := textureTypeFromDimension(desc.Dimension, desc.SampleCount, desc.Size.DepthOrArrayLayers)
	_ = MsgSend(texDesc, Sel("setTextureType:"), uintptr(texType))

	pixelFormat := textureFormatToMTL(desc.Format)
	_ = MsgSend(texDesc, Sel("setPixelFormat:"), uintptr(pixelFormat))

	_ = MsgSend(texDesc, Sel("setWidth:"), uintptr(desc.Size.Width))
	_ = MsgSend(texDesc, Sel("setHeight:"), uintptr(desc.Size.Height))

	depth := desc.Size.DepthOrArrayLayers
	if depth == 0 {
		depth = 1
	}
	_ = MsgSend(texDesc, Sel("setDepth:"), uintptr(depth))

	mipLevels := desc.MipLevelCount
	if mipLevels == 0 {
		mipLevels = 1
	}
	_ = MsgSend(texDesc, Sel("setMipmapLevelCount:"), uintptr(mipLevels))

	sampleCount := desc.SampleCount
	if sampleCount == 0 {
		sampleCount = 1
	}
	_ = MsgSend(texDesc, Sel("setSampleCount:"), uintptr(sampleCount))

	usage := textureUsageToMTL(desc.Usage)
	_ = MsgSend(texDesc, Sel("setUsage:"), uintptr(usage))
	_ = MsgSend(texDesc, Sel("setStorageMode:"), uintptr(MTLStorageModePrivate))

	raw := MsgSend(d.raw, Sel("newTextureWithDescriptor:"), uintptr(texDesc))
	if raw == 0 {
		return nil, fmt.Errorf("metal: failed to create texture")
	}

	if desc.Label != "" {
		label := NSString(desc.Label)
		_ = MsgSend(raw, Sel("setLabel:"), uintptr(label))
	}

	return &Texture{
		raw:        raw,
		format:     desc.Format,
		width:      desc.Size.Width,
		height:     desc.Size.Height,
		depth:      depth,
		mipLevels:  mipLevels,
		samples:    sampleCount,
		dimension:  desc.Dimension,
		usage:      desc.Usage,
		device:     d,
		isExternal: false,
	}, nil
}

// DestroyTexture destroys a GPU texture.
func (d *Device) DestroyTexture(texture hal.Texture) {
	mtlTexture, ok := texture.(*Texture)
	if !ok || mtlTexture == nil {
		return
	}
	if mtlTexture.raw != 0 && !mtlTexture.isExternal {
		Release(mtlTexture.raw)
		mtlTexture.raw = 0
	}
	mtlTexture.device = nil
}

// CreateTextureView creates a view into a texture.
func (d *Device) CreateTextureView(texture hal.Texture, desc *hal.TextureViewDescriptor) (hal.TextureView, error) {
	mtlTexture, ok := texture.(*Texture)
	if !ok || mtlTexture == nil {
		return nil, fmt.Errorf("metal: invalid texture")
	}

	pool := NewAutoreleasePool()
	defer pool.Drain()

	format := desc.Format
	if format == types.TextureFormatUndefined {
		format = mtlTexture.format
	}
	pixelFormat := textureFormatToMTL(format)

	baseMip := desc.BaseMipLevel
	mipCount := desc.MipLevelCount
	if mipCount == 0 {
		mipCount = mtlTexture.mipLevels - baseMip
	}

	baseLayer := desc.BaseArrayLayer
	layerCount := desc.ArrayLayerCount
	if layerCount == 0 {
		layerCount = 1
	}
	_ = baseLayer
	_ = layerCount

	var viewType MTLTextureType
	if desc.Dimension == types.TextureViewDimensionUndefined {
		viewType = textureTypeFromDimension(mtlTexture.dimension, mtlTexture.samples, mtlTexture.depth)
	} else {
		viewType = textureViewDimensionToMTL(desc.Dimension)
	}

	raw := MsgSend(mtlTexture.raw, Sel("newTextureViewWithPixelFormat:textureType:levels:slices:"),
		uintptr(pixelFormat), uintptr(viewType),
		uintptr(baseMip), uintptr(mipCount),
	)
	if raw == 0 {
		return nil, fmt.Errorf("metal: failed to create texture view")
	}

	return &TextureView{raw: raw, texture: mtlTexture, device: d}, nil
}

// DestroyTextureView destroys a texture view.
func (d *Device) DestroyTextureView(view hal.TextureView) {
	mtlView, ok := view.(*TextureView)
	if !ok || mtlView == nil {
		return
	}
	if mtlView.raw != 0 {
		Release(mtlView.raw)
		mtlView.raw = 0
	}
	mtlView.device = nil
}

// CreateSampler creates a texture sampler.
func (d *Device) CreateSampler(desc *hal.SamplerDescriptor) (hal.Sampler, error) {
	if desc == nil {
		return nil, fmt.Errorf("metal: sampler descriptor is nil")
	}

	pool := NewAutoreleasePool()
	defer pool.Drain()

	sampDesc := MsgSend(ID(GetClass("MTLSamplerDescriptor")), Sel("new"))
	if sampDesc == 0 {
		return nil, fmt.Errorf("metal: failed to create sampler descriptor")
	}
	defer Release(sampDesc)

	_ = MsgSend(sampDesc, Sel("setMinFilter:"), uintptr(filterModeToMTL(desc.MinFilter)))
	_ = MsgSend(sampDesc, Sel("setMagFilter:"), uintptr(filterModeToMTL(desc.MagFilter)))
	_ = MsgSend(sampDesc, Sel("setMipFilter:"), uintptr(mipmapFilterModeToMTL(desc.MipmapFilter)))
	_ = MsgSend(sampDesc, Sel("setSAddressMode:"), uintptr(addressModeToMTL(desc.AddressModeU)))
	_ = MsgSend(sampDesc, Sel("setTAddressMode:"), uintptr(addressModeToMTL(desc.AddressModeV)))
	_ = MsgSend(sampDesc, Sel("setRAddressMode:"), uintptr(addressModeToMTL(desc.AddressModeW)))

	if desc.Anisotropy > 1 {
		_ = MsgSend(sampDesc, Sel("setMaxAnisotropy:"), uintptr(desc.Anisotropy))
	}

	if desc.Compare != types.CompareFunctionUndefined {
		_ = MsgSend(sampDesc, Sel("setCompareFunction:"), uintptr(compareFunctionToMTL(desc.Compare)))
	}

	raw := MsgSend(d.raw, Sel("newSamplerStateWithDescriptor:"), uintptr(sampDesc))
	if raw == 0 {
		return nil, fmt.Errorf("metal: failed to create sampler state")
	}

	return &Sampler{raw: raw, device: d}, nil
}

// DestroySampler destroys a sampler.
func (d *Device) DestroySampler(sampler hal.Sampler) {
	mtlSampler, ok := sampler.(*Sampler)
	if !ok || mtlSampler == nil {
		return
	}
	if mtlSampler.raw != 0 {
		Release(mtlSampler.raw)
		mtlSampler.raw = 0
	}
	mtlSampler.device = nil
}

// CreateBindGroupLayout creates a bind group layout.
func (d *Device) CreateBindGroupLayout(desc *hal.BindGroupLayoutDescriptor) (hal.BindGroupLayout, error) {
	return &BindGroupLayout{entries: desc.Entries, device: d}, nil
}

// DestroyBindGroupLayout destroys a bind group layout.
func (d *Device) DestroyBindGroupLayout(layout hal.BindGroupLayout) {
	mtlLayout, ok := layout.(*BindGroupLayout)
	if !ok || mtlLayout == nil {
		return
	}
	mtlLayout.device = nil
}

// CreateBindGroup creates a bind group.
func (d *Device) CreateBindGroup(desc *hal.BindGroupDescriptor) (hal.BindGroup, error) {
	return &BindGroup{layout: desc.Layout.(*BindGroupLayout), entries: desc.Entries, device: d}, nil
}

// DestroyBindGroup destroys a bind group.
func (d *Device) DestroyBindGroup(group hal.BindGroup) {
	mtlGroup, ok := group.(*BindGroup)
	if !ok || mtlGroup == nil {
		return
	}
	mtlGroup.device = nil
}

// CreatePipelineLayout creates a pipeline layout.
func (d *Device) CreatePipelineLayout(desc *hal.PipelineLayoutDescriptor) (hal.PipelineLayout, error) {
	return &PipelineLayout{layouts: desc.BindGroupLayouts, device: d}, nil
}

// DestroyPipelineLayout destroys a pipeline layout.
func (d *Device) DestroyPipelineLayout(layout hal.PipelineLayout) {
	mtlLayout, ok := layout.(*PipelineLayout)
	if !ok || mtlLayout == nil {
		return
	}
	mtlLayout.device = nil
}

// CreateShaderModule creates a shader module.
func (d *Device) CreateShaderModule(desc *hal.ShaderModuleDescriptor) (hal.ShaderModule, error) {
	return &ShaderModule{source: desc.Source, device: d}, nil
}

// DestroyShaderModule destroys a shader module.
func (d *Device) DestroyShaderModule(module hal.ShaderModule) {
	mtlModule, ok := module.(*ShaderModule)
	if !ok || mtlModule == nil {
		return
	}
	if mtlModule.library != 0 {
		Release(mtlModule.library)
		mtlModule.library = 0
	}
	mtlModule.device = nil
}

// CreateRenderPipeline creates a render pipeline.
func (d *Device) CreateRenderPipeline(desc *hal.RenderPipelineDescriptor) (hal.RenderPipeline, error) {
	return nil, fmt.Errorf("metal: CreateRenderPipeline not yet implemented")
}

// DestroyRenderPipeline destroys a render pipeline.
func (d *Device) DestroyRenderPipeline(pipeline hal.RenderPipeline) {
	mtlPipeline, ok := pipeline.(*RenderPipeline)
	if !ok || mtlPipeline == nil {
		return
	}
	if mtlPipeline.raw != 0 {
		Release(mtlPipeline.raw)
		mtlPipeline.raw = 0
	}
	mtlPipeline.device = nil
}

// CreateComputePipeline creates a compute pipeline.
func (d *Device) CreateComputePipeline(desc *hal.ComputePipelineDescriptor) (hal.ComputePipeline, error) {
	return nil, fmt.Errorf("metal: CreateComputePipeline not yet implemented")
}

// DestroyComputePipeline destroys a compute pipeline.
func (d *Device) DestroyComputePipeline(pipeline hal.ComputePipeline) {
	mtlPipeline, ok := pipeline.(*ComputePipeline)
	if !ok || mtlPipeline == nil {
		return
	}
	if mtlPipeline.raw != 0 {
		Release(mtlPipeline.raw)
		mtlPipeline.raw = 0
	}
	mtlPipeline.device = nil
}

// CreateCommandEncoder creates a command encoder.
func (d *Device) CreateCommandEncoder(desc *hal.CommandEncoderDescriptor) (hal.CommandEncoder, error) {
	pool := NewAutoreleasePool()
	cmdBuffer := MsgSend(d.commandQueue, Sel("commandBuffer"))
	if cmdBuffer == 0 {
		pool.Drain()
		return nil, fmt.Errorf("metal: failed to create command buffer")
	}
	Retain(cmdBuffer)
	label := ""
	if desc != nil {
		label = desc.Label
	}
	return &CommandEncoder{device: d, cmdBuffer: cmdBuffer, pool: pool, label: label}, nil
}

// CreateFence creates a synchronization fence.
func (d *Device) CreateFence() (hal.Fence, error) {
	event := MsgSend(d.raw, Sel("newEvent"))
	if event == 0 {
		return nil, fmt.Errorf("metal: failed to create event")
	}
	return &Fence{event: event, value: 0, device: d}, nil
}

// DestroyFence destroys a fence.
func (d *Device) DestroyFence(fence hal.Fence) {
	mtlFence, ok := fence.(*Fence)
	if !ok || mtlFence == nil {
		return
	}
	if mtlFence.event != 0 {
		Release(mtlFence.event)
		mtlFence.event = 0
	}
	mtlFence.device = nil
}

// Wait waits for a fence to reach the specified value.
func (d *Device) Wait(fence hal.Fence, value uint64, timeout time.Duration) (bool, error) {
	mtlFence, ok := fence.(*Fence)
	if !ok || mtlFence == nil {
		return false, fmt.Errorf("metal: invalid fence")
	}
	if mtlFence.value >= value {
		return true, nil
	}
	return false, nil
}

// Destroy releases the device.
func (d *Device) Destroy() {
	if d.commandQueue != 0 {
		Release(d.commandQueue)
		d.commandQueue = 0
	}
}
