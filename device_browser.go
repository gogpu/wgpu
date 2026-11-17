//go:build js && wasm

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package wgpu

// This file is the public declaration layer over internal/device on the browser
// build (ADR-070 S11). The implementation moved to internal/device; what stays
// here is the frozen public Device type, its methods, and the descriptor
// conversion to the JS values the implementation takes.

import (
	"fmt"
	"syscall/js"
	"time"

	"github.com/gogpu/gputypes"

	"github.com/gogpu/wgpu/internal/browser"
	"github.com/gogpu/wgpu/internal/device"
	"github.com/gogpu/wgpu/internal/resource"
)

// Device represents a logical GPU device.
// On browser, this wraps a GPUDevice via internal/browser.Device.
type Device struct {
	impl  *device.Device
	queue *Queue
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

// CreateBuffer creates a GPU buffer from the given descriptor.
func (d *Device) CreateBuffer(desc *BufferDescriptor) (*Buffer, error) {
	if desc == nil {
		return nil, fmt.Errorf("wgpu: buffer descriptor is nil")
	}
	impl, err := d.impl.CreateBuffer(desc.Label, desc.Size, desc.Usage, desc.MappedAtCreation)
	if err != nil {
		return nil, err
	}
	return &Buffer{impl: impl}, nil
}

// CreateTexture creates a GPU texture from the given descriptor.
func (d *Device) CreateTexture(desc *TextureDescriptor) (*Texture, error) {
	if desc == nil {
		return nil, fmt.Errorf("wgpu: texture descriptor is nil")
	}
	impl, err := d.impl.CreateTexture(convertTextureDescriptor(desc), desc.Format)
	if err != nil {
		return nil, err
	}
	tex := &Texture{impl: impl}
	impl.Pub = tex
	return tex, nil
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

// convertTextureDescriptor converts a texture descriptor to a
// GPUTextureDescriptor, with the zero members that have a WebGPU default set
// to it (texture_sampler_defaults.go).
func convertTextureDescriptor(desc *TextureDescriptor) js.Value {
	desc = normalizeTextureDescriptor(desc)
	return browser.BuildTextureDescriptor(
		desc.Label,
		desc.Size.Width, desc.Size.Height, desc.Size.DepthOrArrayLayers,
		desc.MipLevelCount, desc.SampleCount,
		desc.Dimension, desc.Format, desc.Usage,
		desc.ViewFormats,
	)
}

// CreateTextureView creates a view into a texture.
func (d *Device) CreateTextureView(texture *Texture, desc *TextureViewDescriptor) (*TextureView, error) {
	// A non-nil wrapper without an implementation is a zero-value handle, not
	// an absent one: it answers ErrReleased on every build (S11 review F3).
	if texture != nil && texture.impl == nil {
		return nil, ErrReleased
	}
	var parent *resource.Texture
	if texture != nil {
		parent = texture.impl
	}
	if desc == nil {
		desc = &TextureViewDescriptor{}
	}
	jsDesc := browser.BuildTextureViewDescriptor(
		desc.Label,
		desc.Format, desc.Dimension, desc.Aspect,
		desc.BaseMipLevel, desc.MipLevelCount,
		desc.BaseArrayLayer, desc.ArrayLayerCount,
	)
	impl, err := d.impl.CreateTextureView(parent, jsDesc)
	if err != nil {
		return nil, err
	}
	return &TextureView{impl: impl}, nil
}

// CreateSampler creates a texture sampler from the given descriptor.
func (d *Device) CreateSampler(desc *SamplerDescriptor) (*Sampler, error) {
	if desc == nil {
		desc = &SamplerDescriptor{}
	}
	jsDesc := browser.BuildSamplerDescriptor(
		desc.Label,
		normalizeAddressMode(desc.AddressModeU), normalizeAddressMode(desc.AddressModeV),
		normalizeAddressMode(desc.AddressModeW),
		normalizeFilterMode(desc.MagFilter), normalizeFilterMode(desc.MinFilter),
		normalizeFilterMode(desc.MipmapFilter),
		desc.LodMinClamp, normalizeLodMaxClamp(desc.LodMaxClamp),
		desc.Compare, normalizeAnisotropy(desc.Anisotropy),
	)
	impl, err := d.impl.CreateSampler(jsDesc)
	if err != nil {
		return nil, err
	}
	return &Sampler{impl: impl}, nil
}

// CreateShaderModule creates a shader module from the given descriptor.
func (d *Device) CreateShaderModule(desc *ShaderModuleDescriptor) (*ShaderModule, error) {
	if desc == nil {
		return nil, fmt.Errorf("wgpu: shader module descriptor is nil")
	}
	jsDesc := browser.BuildShaderModuleDescriptor(desc.Label, desc.WGSL)
	impl, err := d.impl.CreateShaderModule(jsDesc)
	if err != nil {
		return nil, err
	}
	return &ShaderModule{impl: impl}, nil
}

// CreateBindGroupLayout creates a bind group layout from the given descriptor.
func (d *Device) CreateBindGroupLayout(desc *BindGroupLayoutDescriptor) (*BindGroupLayout, error) {
	if desc == nil {
		return nil, fmt.Errorf("wgpu: bind group layout descriptor is nil")
	}
	entries := convertBindGroupLayoutEntries(desc.Entries)
	jsDesc := browser.BuildBindGroupLayoutDescriptor(desc.Label, entries)
	impl, err := d.impl.CreateBindGroupLayout(jsDesc)
	if err != nil {
		return nil, err
	}
	return &BindGroupLayout{impl: impl}, nil
}

// CreatePipelineLayout creates a pipeline layout from the given descriptor.
func (d *Device) CreatePipelineLayout(desc *PipelineLayoutDescriptor) (*PipelineLayout, error) {
	if desc == nil {
		return nil, fmt.Errorf("wgpu: pipeline layout descriptor is nil")
	}
	refs := make([]js.Value, len(desc.BindGroupLayouts))
	for i, bgl := range desc.BindGroupLayouts {
		if bgl != nil && bgl.impl.Browser() != nil {
			refs[i] = bgl.impl.Browser().Ref()
		} else {
			refs[i] = js.Undefined()
		}
	}
	jsDesc := browser.BuildPipelineLayoutDescriptor(desc.Label, refs)
	impl, err := d.impl.CreatePipelineLayout(jsDesc)
	if err != nil {
		return nil, err
	}
	return &PipelineLayout{impl: impl}, nil
}

// CreateBindGroup creates a bind group from the given descriptor.
func (d *Device) CreateBindGroup(desc *BindGroupDescriptor) (*BindGroup, error) {
	if desc == nil {
		return nil, fmt.Errorf("wgpu: bind group descriptor is nil")
	}
	var layoutRef js.Value
	if desc.Layout != nil && desc.Layout.impl.Browser() != nil {
		layoutRef = desc.Layout.impl.Browser().Ref()
	} else {
		layoutRef = js.Undefined()
	}
	entries := convertBindGroupEntries(desc.Entries)
	jsDesc := browser.BuildBindGroupDescriptor(desc.Label, layoutRef, entries)
	impl, err := d.impl.CreateBindGroup(jsDesc)
	if err != nil {
		return nil, err
	}
	return &BindGroup{impl: impl}, nil
}

// CreateRenderPipeline creates a render pipeline from the given descriptor.
func (d *Device) CreateRenderPipeline(desc *RenderPipelineDescriptor) (*RenderPipeline, error) {
	if desc == nil {
		return nil, fmt.Errorf("wgpu: render pipeline descriptor is nil")
	}
	jsDesc := convertRenderPipelineDescriptor(desc)
	hasFragment := desc.Fragment != nil
	var fragmentConstants map[string]float64
	if hasFragment {
		fragmentConstants = desc.Fragment.Constants
	}
	impl, err := d.impl.CreateRenderPipeline(jsDesc, desc.Vertex.Constants, fragmentConstants, hasFragment)
	if err != nil {
		return nil, err
	}
	return &RenderPipeline{impl: impl}, nil
}

// CreateComputePipeline creates a compute pipeline from the given descriptor.
func (d *Device) CreateComputePipeline(desc *ComputePipelineDescriptor) (*ComputePipeline, error) {
	if desc == nil {
		return nil, fmt.Errorf("wgpu: compute pipeline descriptor is nil")
	}
	var layoutRef js.Value
	if desc.Layout != nil && desc.Layout.impl.Browser() != nil {
		layoutRef = desc.Layout.impl.Browser().Ref()
	} else {
		layoutRef = js.Undefined()
	}
	var moduleRef js.Value
	if desc.Module != nil && desc.Module.impl.Browser() != nil {
		moduleRef = desc.Module.impl.Browser().Ref()
	} else {
		moduleRef = js.Undefined()
	}
	jsDesc := browser.BuildComputePipelineDescriptor(
		desc.Label, layoutRef, moduleRef, desc.EntryPoint, desc.Constants,
	)
	impl, err := d.impl.CreateComputePipeline(jsDesc, desc.Constants)
	if err != nil {
		return nil, err
	}
	return &ComputePipeline{impl: impl}, nil
}

// CreateCommandEncoder creates a command encoder for recording GPU commands.
func (d *Device) CreateCommandEncoder(desc *CommandEncoderDescriptor) (*CommandEncoder, error) {
	label := ""
	if desc != nil {
		label = desc.Label
	}
	impl, err := d.impl.CreateCommandEncoder(label)
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

// DestroyFence destroys a fence. No-op on browser — fences are JS-managed.
func (d *Device) DestroyFence(f *Fence) {
	if f != nil {
		f.Release()
	}
}

// ResetFence resets a fence. No-op on browser — fences auto-reset.
func (d *Device) ResetFence(_ *Fence) error { return nil }

// GetFenceStatus returns true (signaled). Browser fences resolve via JS Promises.
func (d *Device) GetFenceStatus(_ *Fence) (bool, error) { return true, nil }

// WaitForFence returns immediately on browser — GPU sync is handled by JS event loop.
func (d *Device) WaitForFence(_ *Fence, _ uint64, _ time.Duration) (bool, error) {
	return true, nil
}

// PushErrorScope pushes a new error scope onto the device's error scope stack.
// A nil or zero-value device answers nil (nothing to push), matching the other
// builds' guards.
func (d *Device) PushErrorScope(filter ErrorFilter) error {
	if d == nil || d.impl == nil {
		return nil
	}
	return d.impl.PushErrorScope(browser.ErrorFilter(filter))
}

// PopErrorScope pops the most recently pushed error scope.
//
// Contract (at most one side is non-nil): a captured error, or (nil, nil) when
// the scope captured nothing — the common path — or (nil, ErrErrorScopeEmpty)
// when the stack was empty (the JS OperationError case). It never panics.
func (d *Device) PopErrorScope() (*GPUError, error) {
	if d == nil || d.impl == nil {
		return nil, nil
	}
	info, err := d.impl.PopErrorScope()
	if err != nil {
		return nil, err
	}
	if info == nil {
		return nil, nil
	}
	return &GPUError{Type: ErrorFilter(info.Type), Message: info.Message}, nil
}

// WaitIdle waits for all GPU work to complete.
// On browser, the GPU is polled automatically. This is a no-op.
func (d *Device) WaitIdle() error {
	return d.impl.WaitIdle()
}

// Poll drives the per-device pending-map triage loop.
// On browser, the GPU is polled automatically by the browser event loop.
// Returns true (devices are always considered polled in browser).
func (d *Device) Poll(pollType PollType) bool {
	return true
}

// FreeCommandBuffer is a no-op on browser — JS GC handles GPU resource cleanup.
func (d *Device) FreeCommandBuffer(_ *CommandBuffer) {}

// Release releases the device and all associated resources.
func (d *Device) Release() {
	if d == nil {
		return
	}
	d.impl.Release()
}

// --- Descriptor conversion helpers ---

// convertBindGroupLayoutEntries converts Go BindGroupLayoutEntry slice to
// browser.BindGroupLayoutEntryJS slice for JS object construction.
func convertBindGroupLayoutEntries(entries []gputypes.BindGroupLayoutEntry) []browser.BindGroupLayoutEntryJS {
	result := make([]browser.BindGroupLayoutEntryJS, len(entries))
	for i, e := range entries {
		entry := browser.BindGroupLayoutEntryJS{
			Binding:    e.Binding,
			Visibility: uint32(e.Visibility),
		}
		if e.Buffer != nil {
			entry.Buffer = &browser.BufferBindingLayoutJS{
				Type:             browser.BufferBindingTypeToJS(e.Buffer.Type),
				HasDynamicOffset: e.Buffer.HasDynamicOffset,
				MinBindingSize:   e.Buffer.MinBindingSize,
			}
		}
		if e.Sampler != nil {
			entry.Sampler = &browser.SamplerBindingLayoutJS{
				Type: browser.SamplerBindingTypeToJS(e.Sampler.Type),
			}
		}
		if e.Texture != nil {
			entry.Texture = &browser.TextureBindingLayoutJS{
				SampleType:    browser.TextureSampleTypeToJS(e.Texture.SampleType),
				ViewDimension: browser.TextureViewDimensionToJS(e.Texture.ViewDimension),
				Multisampled:  e.Texture.Multisampled,
			}
		}
		if e.StorageTexture != nil {
			entry.StorageTexture = &browser.StorageTextureBindingLayoutJS{
				Access:        browser.StorageTextureAccessToJS(e.StorageTexture.Access),
				Format:        browser.TextureFormatToJS(e.StorageTexture.Format),
				ViewDimension: browser.TextureViewDimensionToJS(e.StorageTexture.ViewDimension),
			}
		}
		result[i] = entry
	}
	return result
}

// convertBindGroupEntries converts Go BindGroupEntry slice to
// browser.BindGroupEntryJS slice for JS object construction.
func convertBindGroupEntries(entries []BindGroupEntry) []browser.BindGroupEntryJS {
	result := make([]browser.BindGroupEntryJS, len(entries))
	for i, e := range entries {
		entry := browser.BindGroupEntryJS{
			Binding:        e.Binding,
			BufferRef:      js.Undefined(),
			SamplerRef:     js.Undefined(),
			TextureViewRef: js.Undefined(),
		}
		if e.Buffer != nil && e.Buffer.impl.Browser() != nil {
			entry.BufferRef = e.Buffer.impl.Browser().Ref()
			entry.BufferOffset = e.Offset
			entry.BufferSize = e.Size
		}
		if e.Sampler != nil && e.Sampler.impl.Browser() != nil {
			entry.SamplerRef = e.Sampler.impl.Browser().Ref()
		}
		if e.TextureView != nil && e.TextureView.impl.Browser() != nil {
			entry.TextureViewRef = e.TextureView.impl.Browser().Ref()
		}
		result[i] = entry
	}
	return result
}

// convertRenderPipelineDescriptor converts a Go RenderPipelineDescriptor to
// a JS descriptor object via browser.BuildRenderPipelineDescriptor.
func convertRenderPipelineDescriptor(desc *RenderPipelineDescriptor) js.Value {
	rpd := &browser.RenderPipelineDescriptorJS{
		Label: desc.Label,
	}

	// Layout
	if desc.Layout != nil && desc.Layout.impl.Browser() != nil {
		rpd.LayoutRef = desc.Layout.impl.Browser().Ref()
	} else {
		rpd.LayoutRef = js.Undefined()
	}

	// Vertex state
	rpd.Vertex = browser.VertexStateJS{
		EntryPoint: desc.Vertex.EntryPoint,
		Constants:  desc.Vertex.Constants,
	}
	if desc.Vertex.Module != nil && desc.Vertex.Module.impl.Browser() != nil {
		rpd.Vertex.ModuleRef = desc.Vertex.Module.impl.Browser().Ref()
	} else {
		rpd.Vertex.ModuleRef = js.Undefined()
	}
	rpd.Vertex.Buffers = convertVertexBufferLayouts(desc.Vertex.Buffers)

	// Primitive state
	rpd.Primitive = &browser.PrimitiveStateJS{
		Topology:  browser.PrimitiveTopologyToJS(desc.Primitive.Topology),
		FrontFace: browser.FrontFaceToJS(desc.Primitive.FrontFace),
		CullMode:  browser.CullModeToJS(desc.Primitive.CullMode),
	}
	if desc.Primitive.StripIndexFormat != nil {
		rpd.Primitive.StripIndexFormat = browser.IndexFormatToJS(*desc.Primitive.StripIndexFormat)
	}
	if desc.Primitive.UnclippedDepth {
		rpd.Primitive.UnclippedDepth = true
	}

	// Depth-stencil state, with the WebGPU defaults for zero stencil faces
	// (pipeline_defaults.go). An undefined DepthCompare is left out of the JS
	// object: it is the spec's "not provided", for the browser to validate.
	if desc.DepthStencil != nil {
		rpd.DepthStencil = convertDepthStencilState(normalizeDepthStencilState(desc.DepthStencil))
	}

	// Multisample state; a zero mask means every sample (pipeline_defaults.go).
	ms := normalizeMultisample(desc.Multisample)
	rpd.Multisample = &browser.MultisampleStateJS{
		Count:                  ms.Count,
		Mask:                   ms.Mask & sampleMaskDefault,
		AlphaToCoverageEnabled: ms.AlphaToCoverageEnabled,
	}

	// Fragment state, with the WebGPU defaults for zero blend components.
	if desc.Fragment != nil {
		fs := *desc.Fragment
		fs.Targets = normalizeColorTargets(fs.Targets)
		rpd.Fragment = convertFragmentState(&fs)
	}

	return browser.BuildRenderPipelineDescriptor(rpd)
}

// convertVertexBufferLayouts converts Go VertexBufferLayout slice to JS types.
func convertVertexBufferLayouts(layouts []gputypes.VertexBufferLayout) []browser.VertexBufferLayoutJS {
	result := make([]browser.VertexBufferLayoutJS, len(layouts))
	for i, l := range layouts {
		jsLayout := browser.VertexBufferLayoutJS{
			ArrayStride: l.ArrayStride,
			StepMode:    browser.VertexStepModeToJS(l.StepMode),
		}
		jsLayout.Attributes = make([]browser.VertexAttributeJS, len(l.Attributes))
		for j, a := range l.Attributes {
			jsLayout.Attributes[j] = browser.VertexAttributeJS{
				Format:         browser.VertexFormatToJS(a.Format),
				Offset:         a.Offset,
				ShaderLocation: a.ShaderLocation,
			}
		}
		result[i] = jsLayout
	}
	return result
}

// convertDepthStencilState converts a Go DepthStencilState to JS types.
func convertDepthStencilState(ds *DepthStencilState) *browser.DepthStencilStateJS {
	result := &browser.DepthStencilStateJS{
		Format:              browser.TextureFormatToJS(ds.Format),
		DepthWriteEnabled:   ds.DepthWriteEnabled,
		DepthCompare:        browser.CompareFunctionToJS(ds.DepthCompare),
		StencilReadMask:     ds.StencilReadMask,
		StencilWriteMask:    ds.StencilWriteMask,
		DepthBias:           ds.DepthBias,
		DepthBiasSlopeScale: ds.DepthBiasSlopeScale,
		DepthBiasClamp:      ds.DepthBiasClamp,
	}
	result.StencilFront = convertStencilFaceState(&ds.StencilFront)
	result.StencilBack = convertStencilFaceState(&ds.StencilBack)
	return result
}

// convertStencilFaceState converts a Go StencilFaceState to JS types.
func convertStencilFaceState(s *StencilFaceState) *browser.StencilFaceStateJS {
	return &browser.StencilFaceStateJS{
		Compare:     browser.CompareFunctionToJS(s.Compare),
		FailOp:      stencilOpToJS(s.FailOp),
		DepthFailOp: stencilOpToJS(s.DepthFailOp),
		PassOp:      stencilOpToJS(s.PassOp),
	}
}

// stencilOpToJS converts StencilOperation (gputypes, webgpu.h spec values) to WebGPU JS string.
func stencilOpToJS(op gputypes.StencilOperation) string {
	switch op {
	case StencilOperationKeep:
		return "keep"
	case StencilOperationZero:
		return "zero"
	case StencilOperationReplace:
		return "replace"
	case StencilOperationInvert:
		return "invert"
	case StencilOperationIncrementClamp:
		return "increment-clamp"
	case StencilOperationDecrementClamp:
		return "decrement-clamp"
	case StencilOperationIncrementWrap:
		return "increment-wrap"
	case StencilOperationDecrementWrap:
		return "decrement-wrap"
	default:
		return "keep"
	}
}

// convertFragmentState converts a Go FragmentState to JS types.
func convertFragmentState(fs *FragmentState) *browser.FragmentStateJS {
	result := &browser.FragmentStateJS{
		EntryPoint: fs.EntryPoint,
		Constants:  fs.Constants,
	}
	if fs.Module != nil && fs.Module.impl.Browser() != nil {
		result.ModuleRef = fs.Module.impl.Browser().Ref()
	} else {
		result.ModuleRef = js.Undefined()
	}

	result.Targets = make([]browser.ColorTargetStateJS, len(fs.Targets))
	for i, t := range fs.Targets {
		ct := browser.ColorTargetStateJS{
			Format:    browser.TextureFormatToJS(t.Format),
			WriteMask: uint32(t.WriteMask),
		}
		if t.Blend != nil {
			ct.Blend = &browser.BlendStateJS{
				Color: browser.BlendComponentJS{
					SrcFactor: browser.BlendFactorToJS(t.Blend.Color.SrcFactor),
					DstFactor: browser.BlendFactorToJS(t.Blend.Color.DstFactor),
					Operation: browser.BlendOperationToJS(t.Blend.Color.Operation),
				},
				Alpha: browser.BlendComponentJS{
					SrcFactor: browser.BlendFactorToJS(t.Blend.Alpha.SrcFactor),
					DstFactor: browser.BlendFactorToJS(t.Blend.Alpha.DstFactor),
					Operation: browser.BlendOperationToJS(t.Blend.Alpha.Operation),
				},
			}
		}
		result.Targets[i] = ct
	}
	return result
}
