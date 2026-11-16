//go:build rust

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package wgpu

// This file is the public declaration layer over internal/device on the rust
// build (ADR-070 S11). The implementation moved to internal/device; what stays
// here is the frozen public Device type, its methods, and the descriptor
// conversion to the go-webgpu/webgpu types the implementation takes.

import (
	"fmt"
	"runtime"
	"time"
	"unsafe"

	rwgpu "github.com/go-webgpu/webgpu/wgpu"
	"github.com/gogpu/gputypes"

	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/device"
	"github.com/gogpu/wgpu/internal/resource"
)

// Device represents a logical GPU device.
// On Rust backend, this wraps go-webgpu/webgpu Device.
type Device struct {
	impl     *device.Device
	instance *Instance // the identity cache; the implementation holds the seam view
	queue    *Queue
}

// rustInstanceHost adapts the root Instance to device.InstanceHost on the rust
// build: the one wgpu-native handle the device's error scopes read.
type rustInstanceHost struct{ i *Instance }

var _ device.InstanceHost = rustInstanceHost{}

// InstanceR returns the wgpu-native instance, or nil for an absent one.
func (h rustInstanceHost) InstanceR() *rwgpu.Instance {
	if h.i == nil {
		return nil
	}
	return h.i.r
}

// Queue returns the device's command queue.
func (d *Device) Queue() *Queue {
	return d.queue
}

// Features returns the device's enabled features.
func (d *Device) Features() gputypes.Features {
	if d == nil || d.impl == nil {
		return 0
	}
	return d.impl.Features()
}

// Limits returns the device's resource limits.
func (d *Device) Limits() gputypes.Limits {
	if d == nil || d.impl == nil {
		return gputypes.Limits{}
	}
	return d.impl.Limits()
}

// CreateBuffer creates a GPU buffer.
func (d *Device) CreateBuffer(desc *BufferDescriptor) (*Buffer, error) {
	if desc == nil {
		return nil, fmt.Errorf("wgpu: buffer descriptor is nil")
	}
	impl, err := d.impl.CreateBuffer(&rwgpu.BufferDescriptor{
		Label:            desc.Label,
		Size:             desc.Size,
		Usage:            desc.Usage,
		MappedAtCreation: desc.MappedAtCreation,
	})
	if err != nil {
		return nil, err
	}
	return &Buffer{impl: impl}, nil
}

// CreateTexture creates a GPU texture.
func (d *Device) CreateTexture(desc *TextureDescriptor) (*Texture, error) {
	if desc == nil {
		return nil, fmt.Errorf("wgpu: texture descriptor is nil")
	}
	impl, err := d.impl.CreateTexture(convertTextureDesc(desc), desc.Format)
	if err != nil {
		return nil, err
	}
	tex := &Texture{impl: impl}
	impl.Pub = tex
	return tex, nil
}

// CreateTextureView creates a view into a texture.
func (d *Device) CreateTextureView(texture *Texture, desc *TextureViewDescriptor) (*TextureView, error) {
	var parent *resource.Texture
	if texture != nil {
		parent = texture.impl
	}
	var rDesc *rwgpu.TextureViewDescriptor
	if desc != nil {
		rDesc = &rwgpu.TextureViewDescriptor{
			Label:           desc.Label,
			Format:          desc.Format,
			Dimension:       desc.Dimension,
			Aspect:          rwgpu.TextureAspect(desc.Aspect),
			BaseMipLevel:    desc.BaseMipLevel,
			MipLevelCount:   desc.MipLevelCount,
			BaseArrayLayer:  desc.BaseArrayLayer,
			ArrayLayerCount: desc.ArrayLayerCount,
		}
	}
	impl, err := d.impl.CreateTextureView(parent, rDesc)
	if err != nil {
		return nil, err
	}
	return &TextureView{impl: impl}, nil
}

// CreateSampler creates a texture sampler.
func (d *Device) CreateSampler(desc *SamplerDescriptor) (*Sampler, error) {
	impl, err := d.impl.CreateSampler(samplerDescriptorToRust(desc))
	if err != nil {
		return nil, err
	}
	return &Sampler{impl: impl}, nil
}

// CreateShaderModule creates a shader module.
func (d *Device) CreateShaderModule(desc *ShaderModuleDescriptor) (*ShaderModule, error) {
	if desc == nil {
		return nil, fmt.Errorf("wgpu: shader module descriptor is nil")
	}
	impl, err := d.impl.CreateShaderModule(desc.Label, desc.WGSL, desc.SPIRV)
	if err != nil {
		return nil, err
	}
	return &ShaderModule{impl: impl}, nil
}

// CreateBindGroupLayout creates a bind group layout.
func (d *Device) CreateBindGroupLayout(desc *BindGroupLayoutDescriptor) (*BindGroupLayout, error) {
	if desc == nil {
		return nil, fmt.Errorf("wgpu: bind group layout descriptor is nil")
	}
	rEntries := make([]rwgpu.BindGroupLayoutEntry, len(desc.Entries))
	for i, e := range desc.Entries {
		rEntries[i] = convertBindGroupLayoutEntry(e)
	}
	impl, err := d.impl.CreateBindGroupLayout(&rwgpu.BindGroupLayoutDescriptor{
		Label:   desc.Label,
		Entries: rEntries,
	})
	if err != nil {
		return nil, err
	}
	return &BindGroupLayout{impl: impl}, nil
}

// CreatePipelineLayout creates a pipeline layout.
func (d *Device) CreatePipelineLayout(desc *PipelineLayoutDescriptor) (*PipelineLayout, error) {
	if desc == nil {
		return nil, fmt.Errorf("wgpu: pipeline layout descriptor is nil")
	}
	rLayouts := make([]*rwgpu.BindGroupLayout, len(desc.BindGroupLayouts))
	for i, l := range desc.BindGroupLayouts {
		if l != nil {
			rLayouts[i] = l.impl.Rust()
		}
	}
	impl, err := d.impl.CreatePipelineLayout(&rwgpu.PipelineLayoutDescriptor{
		Label:            desc.Label,
		BindGroupLayouts: rLayouts,
	})
	if err != nil {
		return nil, err
	}
	return &PipelineLayout{impl: impl}, nil
}

// CreateBindGroup creates a bind group.
func (d *Device) CreateBindGroup(desc *BindGroupDescriptor) (*BindGroup, error) {
	if desc == nil {
		return nil, fmt.Errorf("wgpu: bind group descriptor is nil")
	}
	var rLayout *rwgpu.BindGroupLayout
	if desc.Layout != nil {
		rLayout = desc.Layout.impl.Rust()
	}
	rEntries := make([]rwgpu.BindGroupEntry, len(desc.Entries))
	for i, e := range desc.Entries {
		rEntries[i] = convertBindGroupEntry(e)
	}
	impl, err := d.impl.CreateBindGroup(&rwgpu.BindGroupDescriptor{
		Label:   desc.Label,
		Layout:  rLayout,
		Entries: rEntries,
	})
	if err != nil {
		return nil, err
	}
	return &BindGroup{impl: impl}, nil
}

// CreateRenderPipeline creates a render pipeline.
func (d *Device) CreateRenderPipeline(desc *RenderPipelineDescriptor) (*RenderPipeline, error) {
	if desc == nil {
		return nil, fmt.Errorf("wgpu: render pipeline descriptor is nil")
	}
	var vertexConstants, fragmentConstants map[string]float64
	vertexConstants = desc.Vertex.Constants
	hasFragment := desc.Fragment != nil
	if hasFragment {
		fragmentConstants = desc.Fragment.Constants
	}
	impl, err := d.impl.CreateRenderPipeline(convertRenderPipelineDesc(desc), vertexConstants, fragmentConstants, hasFragment)
	if err != nil {
		return nil, err
	}
	return &RenderPipeline{impl: impl}, nil
}

// CreateComputePipeline creates a compute pipeline.
func (d *Device) CreateComputePipeline(desc *ComputePipelineDescriptor) (*ComputePipeline, error) {
	if desc == nil {
		return nil, fmt.Errorf("wgpu: compute pipeline descriptor is nil")
	}
	var rLayout *rwgpu.PipelineLayout
	if desc.Layout != nil {
		rLayout = desc.Layout.impl.Rust()
	}
	var rModule *rwgpu.ShaderModule
	if desc.Module != nil {
		rModule = desc.Module.impl.Rust()
	}
	impl, err := d.impl.CreateComputePipeline(&rwgpu.ComputePipelineDescriptor{
		Label:      desc.Label,
		Layout:     rLayout,
		Module:     rModule,
		EntryPoint: desc.EntryPoint,
	}, desc.Constants)
	if err != nil {
		return nil, err
	}
	return &ComputePipeline{impl: impl}, nil
}

// CreateCommandEncoder creates a command encoder for recording GPU commands.
func (d *Device) CreateCommandEncoder(desc *CommandEncoderDescriptor) (*CommandEncoder, error) {
	var rDesc *rwgpu.CommandEncoderDescriptor
	if desc != nil {
		rDesc = &rwgpu.CommandEncoderDescriptor{Label: desc.Label}
	}
	impl, err := d.impl.CreateCommandEncoder(rDesc)
	if err != nil {
		return nil, err
	}
	return &CommandEncoder{impl: impl}, nil
}

// CreateFence creates a no-op fence for API compatibility.
func (d *Device) CreateFence() (*Fence, error) {
	impl, err := d.impl.CreateFence()
	if err != nil {
		return nil, err
	}
	return &Fence{impl: impl}, nil
}

// DestroyFence destroys a fence.
//
// Deprecated: Use Fence.Release() instead.
func (d *Device) DestroyFence(f *Fence) {
	if f != nil {
		f.Release()
	}
}

// ResetFence resets a fence to the unsignaled state.
func (d *Device) ResetFence(f *Fence) error {
	var impl *resource.Fence
	if f != nil {
		impl = f.impl
	}
	return d.impl.ResetFence(impl)
}

// GetFenceStatus returns true if the fence is signaled (non-blocking).
func (d *Device) GetFenceStatus(f *Fence) (bool, error) {
	var impl *resource.Fence
	if f != nil {
		impl = f.impl
	}
	return d.impl.GetFenceStatus(impl)
}

// WaitForFence waits for a fence to reach the specified value.
func (d *Device) WaitForFence(f *Fence, _ uint64, _ time.Duration) (bool, error) {
	var impl *resource.Fence
	if f != nil {
		impl = f.impl
	}
	return d.impl.WaitForFence(impl, 0, 0)
}

// PushErrorScope pushes a new error scope onto the device's error scope stack.
func (d *Device) PushErrorScope(filter ErrorFilter) error {
	return d.impl.PushErrorScope(core.ErrorFilter(filter))
}

// PopErrorScope pops the most recently pushed error scope.
func (d *Device) PopErrorScope() *GPUError {
	if d == nil || d.impl == nil {
		return nil
	}
	e := d.impl.PopErrorScope()
	if e == nil {
		return nil
	}
	return &GPUError{Type: ErrorFilter(e.Type), Message: e.Message}
}

// WaitIdle waits for all GPU work to complete.
func (d *Device) WaitIdle() error {
	return d.impl.WaitIdle()
}

// Poll drives the per-device pending-map triage loop.
func (d *Device) Poll(pollType PollType) bool {
	if d == nil || d.impl == nil {
		return false
	}
	return d.impl.Poll(core.PollType(pollType))
}

// FreeCommandBuffer is a no-op on the Rust backend.
func (d *Device) FreeCommandBuffer(_ *CommandBuffer) {}

// Release releases the device and all associated resources.
func (d *Device) Release() {
	if d == nil {
		return
	}
	d.impl.Release()
	d.instance = nil
}

// newDetachedTexture returns a texture that carries the descriptor's format
// and no backend object. It backs the public wgputest.NewDetachedTexture.
func newDetachedTexture(desc *TextureDescriptor) *Texture {
	var format gputypes.TextureFormat
	if desc != nil {
		format = desc.Format
	}
	impl := resource.NewDetachedTexture(format)
	tex := &Texture{impl: impl}
	impl.Pub = tex
	return tex
}

// convertTextureDesc converts a texture descriptor for wgpu-native, with the
// zero members that have a WebGPU default set to it
// (texture_sampler_defaults.go).
func convertTextureDesc(desc *TextureDescriptor) *rwgpu.TextureDescriptor {
	desc = normalizeTextureDescriptor(desc)
	return &rwgpu.TextureDescriptor{
		Label:         desc.Label,
		Usage:         desc.Usage,
		Dimension:     desc.Dimension,
		Size:          rwgpu.Extent3D{Width: desc.Size.Width, Height: desc.Size.Height, DepthOrArrayLayers: desc.Size.DepthOrArrayLayers},
		Format:        desc.Format,
		MipLevelCount: desc.MipLevelCount,
		SampleCount:   desc.SampleCount,
		ViewFormats:   desc.ViewFormats,
	}
}

// samplerDescriptorToRust converts a SamplerDescriptor for wgpu-native. A nil
// descriptor means the defaults, as on the native and browser paths (go-webgpu
// rejects a nil one). wgpu-native takes LodMaxClamp and maxAnisotropy
// literally, so their zero values are resolved to 32 and 1 here.
func samplerDescriptorToRust(desc *SamplerDescriptor) *rwgpu.SamplerDescriptor {
	if desc == nil {
		desc = &SamplerDescriptor{}
	}
	return &rwgpu.SamplerDescriptor{
		Label:        desc.Label,
		AddressModeU: normalizeAddressMode(desc.AddressModeU),
		AddressModeV: normalizeAddressMode(desc.AddressModeV),
		AddressModeW: normalizeAddressMode(desc.AddressModeW),
		MagFilter:    normalizeFilterMode(desc.MagFilter),
		MinFilter:    normalizeFilterMode(desc.MinFilter),
		MipmapFilter: rwgpu.MipmapFilterMode(normalizeFilterMode(desc.MipmapFilter)),
		LodMinClamp:  desc.LodMinClamp,
		LodMaxClamp:  normalizeLodMaxClamp(desc.LodMaxClamp),
		Compare:      desc.Compare,
		Anisotropy:   normalizeAnisotropy(desc.Anisotropy),
	}
}

// convertBindGroupLayoutEntry converts a gputypes.BindGroupLayoutEntry to rwgpu.
func convertBindGroupLayoutEntry(e gputypes.BindGroupLayoutEntry) rwgpu.BindGroupLayoutEntry {
	re := rwgpu.BindGroupLayoutEntry{
		Binding:    e.Binding,
		Visibility: e.Visibility,
	}
	if e.Buffer != nil {
		re.Buffer = &rwgpu.BufferBindingLayout{
			Type:             e.Buffer.Type,
			HasDynamicOffset: e.Buffer.HasDynamicOffset,
			MinBindingSize:   e.Buffer.MinBindingSize,
		}
	}
	if e.Sampler != nil {
		re.Sampler = &rwgpu.SamplerBindingLayout{
			Type: e.Sampler.Type,
		}
	}
	if e.Texture != nil {
		re.Texture = &rwgpu.TextureBindingLayout{
			SampleType:    e.Texture.SampleType,
			ViewDimension: e.Texture.ViewDimension,
			Multisampled:  e.Texture.Multisampled,
		}
	}
	if e.StorageTexture != nil {
		re.StorageTexture = &rwgpu.StorageTextureBindingLayout{
			Access:        e.StorageTexture.Access,
			Format:        e.StorageTexture.Format,
			ViewDimension: e.StorageTexture.ViewDimension,
		}
	}
	return re
}

// convertBindGroupEntry converts a BindGroupEntry to rwgpu.BindGroupEntry.
func convertBindGroupEntry(e BindGroupEntry) rwgpu.BindGroupEntry {
	re := rwgpu.BindGroupEntry{
		Binding: e.Binding,
		Offset:  e.Offset,
		Size:    e.Size,
	}
	if e.Buffer != nil {
		re.Buffer = e.Buffer.impl.R()
	}
	if e.Sampler != nil {
		re.Sampler = e.Sampler.impl.Rust()
	}
	if e.TextureView != nil {
		re.TextureView = e.TextureView.impl.Rust()
	}
	return re
}

// convertRenderPipelineDesc converts our RenderPipelineDescriptor to rwgpu.
func convertRenderPipelineDesc(desc *RenderPipelineDescriptor) *rwgpu.RenderPipelineDescriptor {
	rDesc := &rwgpu.RenderPipelineDescriptor{
		Label: desc.Label,
	}

	if desc.Layout != nil {
		rDesc.Layout = desc.Layout.impl.Rust()
	}

	// Vertex state.
	rDesc.Vertex = rwgpu.VertexState{
		EntryPoint: desc.Vertex.EntryPoint,
	}
	if desc.Vertex.Module != nil {
		rDesc.Vertex.Module = desc.Vertex.Module.impl.Rust()
	}
	rDesc.Vertex.Buffers = convertVertexBufferLayouts(desc.Vertex.Buffers)

	// Primitive state.
	rDesc.Primitive = rwgpu.PrimitiveState{
		Topology:  desc.Primitive.Topology,
		FrontFace: desc.Primitive.FrontFace,
		CullMode:  desc.Primitive.CullMode,
	}
	if desc.Primitive.StripIndexFormat != nil {
		rDesc.Primitive.StripIndexFormat = *desc.Primitive.StripIndexFormat
	}

	// Depth-stencil state, with the WebGPU defaults for zero stencil faces
	// (pipeline_defaults.go). An undefined DepthCompare is sent as is: it is
	// the spec's "not provided", for wgpu-native to resolve.
	if desc.DepthStencil != nil {
		rDesc.DepthStencil = convertDepthStencilState(normalizeDepthStencilState(desc.DepthStencil))
	}

	// Multisample state; a zero mask means every sample (pipeline_defaults.go).
	ms := normalizeMultisample(desc.Multisample)
	rDesc.Multisample = rwgpu.MultisampleState{
		Count:                  ms.Count,
		Mask:                   uint32(ms.Mask), //nolint:gosec // mask truncation is intentional (WebGPU spec: 32-bit)
		AlphaToCoverageEnabled: ms.AlphaToCoverageEnabled,
	}

	// Fragment state, with the WebGPU defaults for zero blend components; a
	// copy, so the caller's targets are unchanged.
	if desc.Fragment != nil {
		fs := *desc.Fragment
		fs.Targets = normalizeColorTargets(fs.Targets)
		rDesc.Fragment = convertFragmentState(&fs)
	}

	return rDesc
}

// convertVertexBufferLayouts converts vertex buffer layouts.
// go-webgpu uses C-style pointer+count for attributes (FFI layer).
func convertVertexBufferLayouts(layouts []gputypes.VertexBufferLayout) []rwgpu.VertexBufferLayout {
	result := make([]rwgpu.VertexBufferLayout, len(layouts))
	for i, l := range layouts {
		attrs := make([]rwgpu.VertexAttribute, len(l.Attributes))
		for j, a := range l.Attributes {
			attrs[j] = rwgpu.VertexAttribute{
				Format:         a.Format,
				Offset:         a.Offset,
				ShaderLocation: a.ShaderLocation,
			}
		}
		result[i] = rwgpu.VertexBufferLayout{
			ArrayStride:    l.ArrayStride,
			StepMode:       l.StepMode,
			AttributeCount: uintptr(len(attrs)),
		}
		if len(attrs) > 0 {
			result[i].Attributes = (*rwgpu.VertexAttribute)(unsafe.Pointer(&attrs[0])) //nolint:gosec // G103: FFI interop requires unsafe pointer to C-style attribute array
		}
		// Keep attrs alive past the unsafe.Pointer conversion so GC
		// does not collect the backing array while the pointer is in use.
		runtime.KeepAlive(attrs)
	}
	return result
}

// convertDepthStencilState converts depth-stencil state.
func convertDepthStencilState(ds *DepthStencilState) *rwgpu.DepthStencilState {
	return &rwgpu.DepthStencilState{
		Format:              ds.Format,
		DepthWriteEnabled:   ds.DepthWriteEnabled,
		DepthCompare:        ds.DepthCompare,
		StencilReadMask:     ds.StencilReadMask,
		StencilWriteMask:    ds.StencilWriteMask,
		DepthBias:           ds.DepthBias,
		DepthBiasSlopeScale: ds.DepthBiasSlopeScale,
		DepthBiasClamp:      ds.DepthBiasClamp,
		StencilFront: rwgpu.StencilFaceState{
			Compare:     ds.StencilFront.Compare,
			FailOp:      ds.StencilFront.FailOp,
			DepthFailOp: ds.StencilFront.DepthFailOp,
			PassOp:      ds.StencilFront.PassOp,
		},
		StencilBack: rwgpu.StencilFaceState{
			Compare:     ds.StencilBack.Compare,
			FailOp:      ds.StencilBack.FailOp,
			DepthFailOp: ds.StencilBack.DepthFailOp,
			PassOp:      ds.StencilBack.PassOp,
		},
	}
}

// convertFragmentState converts fragment state.
func convertFragmentState(fs *FragmentState) *rwgpu.FragmentState {
	result := &rwgpu.FragmentState{
		EntryPoint: fs.EntryPoint,
	}
	if fs.Module != nil {
		result.Module = fs.Module.impl.Rust()
	}

	result.Targets = make([]rwgpu.ColorTargetState, len(fs.Targets))
	for i, t := range fs.Targets {
		ct := rwgpu.ColorTargetState{
			Format:    t.Format,
			WriteMask: t.WriteMask,
		}
		if t.Blend != nil {
			ct.Blend = &rwgpu.BlendState{
				Color: rwgpu.BlendComponent{
					SrcFactor: t.Blend.Color.SrcFactor,
					DstFactor: t.Blend.Color.DstFactor,
					Operation: t.Blend.Color.Operation,
				},
				Alpha: rwgpu.BlendComponent{
					SrcFactor: t.Blend.Alpha.SrcFactor,
					DstFactor: t.Blend.Alpha.DstFactor,
					Operation: t.Blend.Alpha.Operation,
				},
			}
		}
		result.Targets[i] = ct
	}
	return result
}
