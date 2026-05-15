//go:build js && wasm

package wgpu

import (
	"syscall/js"

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

// CreateBuffer creates a GPU buffer from the given descriptor.
func (d *Device) CreateBuffer(desc *BufferDescriptor) (*Buffer, error) {
	if d.released {
		return nil, ErrReleased
	}
	jsDesc := browser.BuildBufferDescriptor(
		desc.Label,
		desc.Size,
		uint64(desc.Usage),
		desc.MappedAtCreation,
	)
	bb := d.browser.CreateBufferFromDesc(jsDesc)

	state := MapStateUnmapped
	if desc.MappedAtCreation {
		// When mappedAtCreation is true the buffer starts in the Mapped state
		// with the entire range [0, size) mapped. Record this so that
		// MappedRange / Unmap work without calling MapAsync first.
		bb.SetMappedAtCreation()
		state = MapStateMapped
	}

	return &Buffer{
		browser:  bb,
		size:     desc.Size,
		usage:    desc.Usage,
		released: false,
		mapState: state,
	}, nil
}

// CreateTexture creates a GPU texture from the given descriptor.
func (d *Device) CreateTexture(desc *TextureDescriptor) (*Texture, error) {
	if d.released {
		return nil, ErrReleased
	}
	jsDesc := browser.BuildTextureDescriptor(
		desc.Label,
		desc.Size.Width, desc.Size.Height, desc.Size.DepthOrArrayLayers,
		desc.MipLevelCount, desc.SampleCount,
		desc.Dimension, desc.Format, desc.Usage,
		desc.ViewFormats,
	)
	bt := d.browser.CreateTextureFromDesc(jsDesc)
	return &Texture{
		browser:  bt,
		format:   desc.Format,
		released: false,
	}, nil
}

// CreateTextureView creates a view into a texture.
func (d *Device) CreateTextureView(texture *Texture, desc *TextureViewDescriptor) (*TextureView, error) {
	if d.released {
		return nil, ErrReleased
	}
	if texture == nil || texture.browser == nil {
		return nil, ErrReleased
	}
	jsDesc := browser.BuildTextureViewDescriptor(
		desc.Label,
		desc.Format, desc.Dimension, desc.Aspect,
		desc.BaseMipLevel, desc.MipLevelCount,
		desc.BaseArrayLayer, desc.ArrayLayerCount,
	)
	bv := texture.browser.CreateView(jsDesc)
	return &TextureView{
		browser:  bv,
		released: false,
	}, nil
}

// CreateSampler creates a texture sampler from the given descriptor.
func (d *Device) CreateSampler(desc *SamplerDescriptor) (*Sampler, error) {
	if d.released {
		return nil, ErrReleased
	}
	jsDesc := browser.BuildSamplerDescriptor(
		desc.Label,
		desc.AddressModeU, desc.AddressModeV, desc.AddressModeW,
		desc.MagFilter, desc.MinFilter, desc.MipmapFilter,
		desc.LodMinClamp, desc.LodMaxClamp,
		desc.Compare, desc.Anisotropy,
	)
	bs := d.browser.CreateSamplerFromDesc(jsDesc)
	return &Sampler{
		browser:  bs,
		released: false,
	}, nil
}

// CreateShaderModule creates a shader module from the given descriptor.
// On browser, WGSL code goes directly to the browser's createShaderModule.
// SPIR-V bytecode is not supported in the browser (browser only accepts WGSL).
func (d *Device) CreateShaderModule(desc *ShaderModuleDescriptor) (*ShaderModule, error) {
	if d.released {
		return nil, ErrReleased
	}
	jsDesc := browser.BuildShaderModuleDescriptor(desc.Label, desc.WGSL)
	bm := d.browser.CreateShaderModuleFromDesc(jsDesc)
	return &ShaderModule{
		browser:  bm,
		released: false,
	}, nil
}

// CreateBindGroupLayout creates a bind group layout from the given descriptor.
func (d *Device) CreateBindGroupLayout(desc *BindGroupLayoutDescriptor) (*BindGroupLayout, error) {
	if d.released {
		return nil, ErrReleased
	}
	entries := convertBindGroupLayoutEntries(desc.Entries)
	jsDesc := browser.BuildBindGroupLayoutDescriptor(desc.Label, entries)
	bl := d.browser.CreateBindGroupLayoutFromDesc(jsDesc)
	return &BindGroupLayout{
		browser:  bl,
		released: false,
	}, nil
}

// CreatePipelineLayout creates a pipeline layout from the given descriptor.
func (d *Device) CreatePipelineLayout(desc *PipelineLayoutDescriptor) (*PipelineLayout, error) {
	if d.released {
		return nil, ErrReleased
	}
	refs := make([]js.Value, len(desc.BindGroupLayouts))
	for i, bgl := range desc.BindGroupLayouts {
		if bgl != nil && bgl.browser != nil {
			refs[i] = bgl.browser.Ref()
		} else {
			refs[i] = js.Undefined()
		}
	}
	jsDesc := browser.BuildPipelineLayoutDescriptor(desc.Label, refs)
	bp := d.browser.CreatePipelineLayoutFromDesc(jsDesc)
	return &PipelineLayout{
		browser:  bp,
		released: false,
	}, nil
}

// CreateBindGroup creates a bind group from the given descriptor.
func (d *Device) CreateBindGroup(desc *BindGroupDescriptor) (*BindGroup, error) {
	if d.released {
		return nil, ErrReleased
	}
	var layoutRef js.Value
	if desc.Layout != nil && desc.Layout.browser != nil {
		layoutRef = desc.Layout.browser.Ref()
	} else {
		layoutRef = js.Undefined()
	}
	entries := convertBindGroupEntries(desc.Entries)
	jsDesc := browser.BuildBindGroupDescriptor(desc.Label, layoutRef, entries)
	bg := d.browser.CreateBindGroupFromDesc(jsDesc)
	return &BindGroup{
		browser:  bg,
		released: false,
	}, nil
}

// CreateRenderPipeline creates a render pipeline from the given descriptor.
func (d *Device) CreateRenderPipeline(desc *RenderPipelineDescriptor) (*RenderPipeline, error) {
	if d.released {
		return nil, ErrReleased
	}
	jsDesc := convertRenderPipelineDescriptor(desc)
	bp := d.browser.CreateRenderPipelineFromDesc(jsDesc)
	return &RenderPipeline{
		browser:  bp,
		released: false,
	}, nil
}

// CreateComputePipeline creates a compute pipeline from the given descriptor.
func (d *Device) CreateComputePipeline(desc *ComputePipelineDescriptor) (*ComputePipeline, error) {
	if d.released {
		return nil, ErrReleased
	}
	var layoutRef js.Value
	if desc.Layout != nil && desc.Layout.browser != nil {
		layoutRef = desc.Layout.browser.Ref()
	} else {
		layoutRef = js.Undefined()
	}
	var moduleRef js.Value
	if desc.Module != nil && desc.Module.browser != nil {
		moduleRef = desc.Module.browser.Ref()
	} else {
		moduleRef = js.Undefined()
	}
	jsDesc := browser.BuildComputePipelineDescriptor(
		desc.Label, layoutRef, moduleRef, desc.EntryPoint,
	)
	bp := d.browser.CreateComputePipelineFromDesc(jsDesc)
	return &ComputePipeline{
		browser:  bp,
		released: false,
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
	jsDesc := browser.BuildCommandEncoderDescriptor(label)
	jsEncoder := d.browser.CreateCommandEncoder().Invoke(jsDesc)
	be := browser.NewCommandEncoder(jsEncoder)
	return &CommandEncoder{
		browser:  be,
		released: false,
	}, nil
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

// --- Descriptor conversion helpers ---

// convertBindGroupLayoutEntries converts Go BindGroupLayoutEntry slice to
// browser.BindGroupLayoutEntryJS slice for JS object construction.
func convertBindGroupLayoutEntries(entries []BindGroupLayoutEntry) []browser.BindGroupLayoutEntryJS {
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
		if e.Buffer != nil && e.Buffer.browser != nil {
			entry.BufferRef = e.Buffer.browser.Ref()
			entry.BufferOffset = e.Offset
			entry.BufferSize = e.Size
		}
		if e.Sampler != nil && e.Sampler.browser != nil {
			entry.SamplerRef = e.Sampler.browser.Ref()
		}
		if e.TextureView != nil && e.TextureView.browser != nil {
			entry.TextureViewRef = e.TextureView.browser.Ref()
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
	if desc.Layout != nil && desc.Layout.browser != nil {
		rpd.LayoutRef = desc.Layout.browser.Ref()
	} else {
		rpd.LayoutRef = js.Undefined()
	}

	// Vertex state
	rpd.Vertex = browser.VertexStateJS{
		EntryPoint: desc.Vertex.EntryPoint,
	}
	if desc.Vertex.Module != nil && desc.Vertex.Module.browser != nil {
		rpd.Vertex.ModuleRef = desc.Vertex.Module.browser.Ref()
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

	// Depth-stencil state
	if desc.DepthStencil != nil {
		rpd.DepthStencil = convertDepthStencilState(desc.DepthStencil)
	}

	// Multisample state
	rpd.Multisample = &browser.MultisampleStateJS{
		Count:                  desc.Multisample.Count,
		Mask:                   desc.Multisample.Mask,
		AlphaToCoverageEnabled: desc.Multisample.AlphaToCoverageEnabled,
	}

	// Fragment state
	if desc.Fragment != nil {
		rpd.Fragment = convertFragmentState(desc.Fragment)
	}

	return browser.BuildRenderPipelineDescriptor(rpd)
}

// convertVertexBufferLayouts converts Go VertexBufferLayout slice to JS types.
func convertVertexBufferLayouts(layouts []VertexBufferLayout) []browser.VertexBufferLayoutJS {
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

// stencilOpToJS converts the local StencilOperation enum to a WebGPU JS string.
// The local enum (descriptor_browser.go) uses 0-based values while gputypes uses
// 1-based (with Undefined=0), so we map explicitly to be correct.
func stencilOpToJS(op StencilOperation) string {
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
	}
	if fs.Module != nil && fs.Module.browser != nil {
		result.ModuleRef = fs.Module.browser.Ref()
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
