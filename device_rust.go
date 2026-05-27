//go:build rust

package wgpu

import (
	"fmt"
	"runtime"
	"time"
	"unsafe"

	rwgpu "github.com/go-webgpu/webgpu/wgpu"
)

// Device represents a logical GPU device.
// On Rust backend, this wraps go-webgpu/webgpu Device.
type Device struct {
	r        *rwgpu.Device
	instance *Instance // stored for PopErrorScope which needs Instance handle
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
func (d *Device) CreateBuffer(desc *BufferDescriptor) (*Buffer, error) {
	if d.released {
		return nil, ErrReleased
	}
	if desc == nil {
		return nil, fmt.Errorf("wgpu: buffer descriptor is nil")
	}

	rb, err := d.r.CreateBuffer(&rwgpu.BufferDescriptor{
		Label:            desc.Label,
		Size:             desc.Size,
		Usage:            desc.Usage,
		MappedAtCreation: desc.MappedAtCreation,
	})
	if err != nil {
		return nil, fmt.Errorf("wgpu: failed to create buffer: %w", err)
	}

	return &Buffer{r: rb, device: d}, nil
}

// CreateTexture creates a GPU texture.
func (d *Device) CreateTexture(desc *TextureDescriptor) (*Texture, error) {
	if d.released {
		return nil, ErrReleased
	}
	if desc == nil {
		return nil, fmt.Errorf("wgpu: texture descriptor is nil")
	}

	rt, err := d.r.CreateTexture(&rwgpu.TextureDescriptor{
		Label:         desc.Label,
		Usage:         desc.Usage,
		Dimension:     desc.Dimension,
		Size:          rwgpu.Extent3D{Width: desc.Size.Width, Height: desc.Size.Height, DepthOrArrayLayers: desc.Size.DepthOrArrayLayers},
		Format:        desc.Format,
		MipLevelCount: desc.MipLevelCount,
		SampleCount:   desc.SampleCount,
		ViewFormats:   desc.ViewFormats,
	})
	if err != nil {
		return nil, fmt.Errorf("wgpu: failed to create texture: %w", err)
	}

	return &Texture{r: rt, device: d, format: desc.Format}, nil
}

// CreateTextureView creates a view into a texture.
// In go-webgpu, CreateView is a method on Texture, not Device.
func (d *Device) CreateTextureView(texture *Texture, desc *TextureViewDescriptor) (*TextureView, error) {
	if d.released {
		return nil, ErrReleased
	}
	if texture == nil || texture.r == nil {
		return nil, fmt.Errorf("wgpu: texture is nil")
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

	rv, err := texture.r.CreateView(rDesc)
	if err != nil {
		return nil, fmt.Errorf("wgpu: failed to create texture view: %w", err)
	}

	return &TextureView{r: rv, device: d, texture: texture}, nil
}

// CreateSampler creates a texture sampler.
func (d *Device) CreateSampler(desc *SamplerDescriptor) (*Sampler, error) {
	if d.released {
		return nil, ErrReleased
	}

	var rDesc *rwgpu.SamplerDescriptor
	if desc != nil {
		rDesc = &rwgpu.SamplerDescriptor{
			Label:        desc.Label,
			AddressModeU: desc.AddressModeU,
			AddressModeV: desc.AddressModeV,
			AddressModeW: desc.AddressModeW,
			MagFilter:    desc.MagFilter,
			MinFilter:    desc.MinFilter,
			MipmapFilter: rwgpu.MipmapFilterMode(desc.MipmapFilter),
			LodMinClamp:  desc.LodMinClamp,
			LodMaxClamp:  desc.LodMaxClamp,
			Compare:      desc.Compare,
			Anisotropy:   desc.Anisotropy,
		}
	}

	rs, err := d.r.CreateSampler(rDesc)
	if err != nil {
		return nil, fmt.Errorf("wgpu: failed to create sampler: %w", err)
	}

	return &Sampler{r: rs, device: d}, nil
}

// CreateShaderModule creates a shader module.
func (d *Device) CreateShaderModule(desc *ShaderModuleDescriptor) (*ShaderModule, error) {
	if d.released {
		return nil, ErrReleased
	}
	if desc == nil {
		return nil, fmt.Errorf("wgpu: shader module descriptor is nil")
	}

	var rm *rwgpu.ShaderModule
	var err error

	switch {
	case desc.WGSL != "":
		rm, err = d.r.CreateShaderModuleWGSL(desc.WGSL)
	case len(desc.SPIRV) > 0:
		rm, err = d.r.CreateShaderModuleSPIRV(desc.Label, desc.SPIRV)
	default:
		return nil, fmt.Errorf("wgpu: shader module descriptor has no source")
	}

	if err != nil {
		return nil, fmt.Errorf("wgpu: failed to create shader module: %w", err)
	}

	return &ShaderModule{r: rm, device: d}, nil
}

// CreateBindGroupLayout creates a bind group layout.
func (d *Device) CreateBindGroupLayout(desc *BindGroupLayoutDescriptor) (*BindGroupLayout, error) {
	if d.released {
		return nil, ErrReleased
	}
	if desc == nil {
		return nil, fmt.Errorf("wgpu: bind group layout descriptor is nil")
	}

	rEntries := make([]rwgpu.BindGroupLayoutEntry, len(desc.Entries))
	for i, e := range desc.Entries {
		rEntries[i] = convertBindGroupLayoutEntry(e)
	}

	rl, err := d.r.CreateBindGroupLayout(&rwgpu.BindGroupLayoutDescriptor{
		Label:   desc.Label,
		Entries: rEntries,
	})
	if err != nil {
		return nil, fmt.Errorf("wgpu: failed to create bind group layout: %w", err)
	}

	return &BindGroupLayout{r: rl, device: d}, nil
}

// CreatePipelineLayout creates a pipeline layout.
func (d *Device) CreatePipelineLayout(desc *PipelineLayoutDescriptor) (*PipelineLayout, error) {
	if d.released {
		return nil, ErrReleased
	}
	if desc == nil {
		return nil, fmt.Errorf("wgpu: pipeline layout descriptor is nil")
	}

	rLayouts := make([]*rwgpu.BindGroupLayout, len(desc.BindGroupLayouts))
	for i, l := range desc.BindGroupLayouts {
		if l != nil {
			rLayouts[i] = l.r
		}
	}

	rl, err := d.r.CreatePipelineLayout(&rwgpu.PipelineLayoutDescriptor{
		Label:            desc.Label,
		BindGroupLayouts: rLayouts,
	})
	if err != nil {
		return nil, fmt.Errorf("wgpu: failed to create pipeline layout: %w", err)
	}

	return &PipelineLayout{r: rl, device: d}, nil
}

// CreateBindGroup creates a bind group.
func (d *Device) CreateBindGroup(desc *BindGroupDescriptor) (*BindGroup, error) {
	if d.released {
		return nil, ErrReleased
	}
	if desc == nil {
		return nil, fmt.Errorf("wgpu: bind group descriptor is nil")
	}

	var rLayout *rwgpu.BindGroupLayout
	if desc.Layout != nil {
		rLayout = desc.Layout.r
	}

	rEntries := make([]rwgpu.BindGroupEntry, len(desc.Entries))
	for i, e := range desc.Entries {
		rEntries[i] = convertBindGroupEntry(e)
	}

	rg, err := d.r.CreateBindGroup(&rwgpu.BindGroupDescriptor{
		Label:   desc.Label,
		Layout:  rLayout,
		Entries: rEntries,
	})
	if err != nil {
		return nil, fmt.Errorf("wgpu: failed to create bind group: %w", err)
	}

	return &BindGroup{r: rg, device: d}, nil
}

// CreateRenderPipeline creates a render pipeline.
func (d *Device) CreateRenderPipeline(desc *RenderPipelineDescriptor) (*RenderPipeline, error) {
	if d.released {
		return nil, ErrReleased
	}
	if desc == nil {
		return nil, fmt.Errorf("wgpu: render pipeline descriptor is nil")
	}

	rDesc := convertRenderPipelineDesc(desc)

	rp, err := d.r.CreateRenderPipeline(rDesc)
	if err != nil {
		return nil, fmt.Errorf("wgpu: failed to create render pipeline: %w", err)
	}

	return &RenderPipeline{r: rp, device: d}, nil
}

// CreateComputePipeline creates a compute pipeline.
func (d *Device) CreateComputePipeline(desc *ComputePipelineDescriptor) (*ComputePipeline, error) {
	if d.released {
		return nil, ErrReleased
	}
	if desc == nil {
		return nil, fmt.Errorf("wgpu: compute pipeline descriptor is nil")
	}

	var rLayout *rwgpu.PipelineLayout
	if desc.Layout != nil {
		rLayout = desc.Layout.r
	}
	var rModule *rwgpu.ShaderModule
	if desc.Module != nil {
		rModule = desc.Module.r
	}

	rp, err := d.r.CreateComputePipeline(&rwgpu.ComputePipelineDescriptor{
		Label:      desc.Label,
		Layout:     rLayout,
		Module:     rModule,
		EntryPoint: desc.EntryPoint,
	})
	if err != nil {
		return nil, fmt.Errorf("wgpu: failed to create compute pipeline: %w", err)
	}

	return &ComputePipeline{r: rp, device: d}, nil
}

// CreateCommandEncoder creates a command encoder for recording GPU commands.
func (d *Device) CreateCommandEncoder(desc *CommandEncoderDescriptor) (*CommandEncoder, error) {
	if d.released {
		return nil, ErrReleased
	}

	var rDesc *rwgpu.CommandEncoderDescriptor
	if desc != nil {
		rDesc = &rwgpu.CommandEncoderDescriptor{
			Label: desc.Label,
		}
	}

	re, err := d.r.CreateCommandEncoder(rDesc)
	if err != nil {
		return nil, fmt.Errorf("wgpu: failed to create command encoder: %w", err)
	}

	return &CommandEncoder{r: re, device: d}, nil
}

// CreateFence creates a GPU synchronization fence.
// On Rust backend, fences are not exposed by wgpu-native.
// Returns a no-op fence for API compatibility.
func (d *Device) CreateFence() (*Fence, error) {
	if d.released {
		return nil, ErrReleased
	}
	return &Fence{}, nil
}

// DestroyFence destroys a fence.
// On Rust backend, fences are no-ops — this is a no-op.
//
// Deprecated: Use Fence.Release() instead.
func (d *Device) DestroyFence(f *Fence) {
	if f != nil {
		f.Release()
	}
}

// ResetFence resets a fence to the unsignaled state.
// On Rust backend, fences are no-ops — this always succeeds.
func (d *Device) ResetFence(f *Fence) error {
	if d.released {
		return ErrReleased
	}
	if f == nil || f.released {
		return ErrReleased
	}
	return nil
}

// GetFenceStatus returns true if the fence is signaled (non-blocking).
// On Rust backend, fences are no-ops — always reports signaled.
func (d *Device) GetFenceStatus(f *Fence) (bool, error) {
	if d.released {
		return false, ErrReleased
	}
	if f == nil || f.released {
		return false, ErrReleased
	}
	return true, nil
}

// WaitForFence waits for a fence to reach the specified value.
// On Rust backend, fences are no-ops — polls the device and returns immediately.
func (d *Device) WaitForFence(f *Fence, _ uint64, _ time.Duration) (bool, error) {
	if d.released {
		return false, ErrReleased
	}
	if f == nil || f.released {
		return false, ErrReleased
	}
	// Poll device to ensure GPU work has progressed.
	d.r.Poll(true)
	return true, nil
}

// PushErrorScope pushes a new error scope onto the device's error scope stack.
func (d *Device) PushErrorScope(filter ErrorFilter) {
	if d.r != nil {
		d.r.PushErrorScope(rwgpu.ErrorFilter(filter)) //nolint:gosec // G115: ErrorFilter values are small enum constants that fit uint32
	}
}

// PopErrorScope pops the most recently pushed error scope.
// Returns the captured error, or nil if no error occurred.
func (d *Device) PopErrorScope() *GPUError {
	if d.r == nil || d.instance == nil || d.instance.r == nil {
		return nil
	}
	errType, message, err := d.r.PopErrorScopeAsync(d.instance.r)
	if err != nil {
		return nil //nolint:nilerr // PopErrorScope returns *GPUError not error; infrastructure failure = no captured error
	}
	if errType == rwgpu.ErrorTypeNoError {
		return nil
	}
	return &GPUError{
		Type:    convertRustErrorType(errType),
		Message: message,
	}
}

// WaitIdle waits for all GPU work to complete.
func (d *Device) WaitIdle() error {
	if d.released {
		return ErrReleased
	}
	// Poll with wait=true blocks until all work completes.
	d.r.Poll(true)
	return nil
}

// Poll drives the per-device pending-map triage loop.
func (d *Device) Poll(pollType PollType) bool {
	if d == nil || d.r == nil {
		return false
	}
	return d.r.Poll(pollType == PollWait)
}

// FreeCommandBuffer is a no-op on Rust backend.
func (d *Device) FreeCommandBuffer(_ *CommandBuffer) {}

// HalDevice returns nil on Rust backend. There is no HAL layer.
func (d *Device) HalDevice() any { return nil }

// Release releases the device and all associated resources.
func (d *Device) Release() {
	if d.released {
		return
	}
	d.released = true
	if d.r != nil {
		d.r.Release()
	}
}

// --- Descriptor conversion helpers ---

// convertBindGroupLayoutEntry converts a gputypes.BindGroupLayoutEntry to rwgpu.
func convertBindGroupLayoutEntry(e BindGroupLayoutEntry) rwgpu.BindGroupLayoutEntry {
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
		re.Buffer = e.Buffer.r
	}
	if e.Sampler != nil {
		re.Sampler = e.Sampler.r
	}
	if e.TextureView != nil {
		re.TextureView = e.TextureView.r
	}
	return re
}

// convertRenderPipelineDesc converts our RenderPipelineDescriptor to rwgpu.
func convertRenderPipelineDesc(desc *RenderPipelineDescriptor) *rwgpu.RenderPipelineDescriptor {
	rDesc := &rwgpu.RenderPipelineDescriptor{
		Label: desc.Label,
	}

	if desc.Layout != nil {
		rDesc.Layout = desc.Layout.r
	}

	// Vertex state.
	rDesc.Vertex = rwgpu.VertexState{
		EntryPoint: desc.Vertex.EntryPoint,
	}
	if desc.Vertex.Module != nil {
		rDesc.Vertex.Module = desc.Vertex.Module.r
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

	// Depth-stencil state.
	if desc.DepthStencil != nil {
		rDesc.DepthStencil = convertDepthStencilState(desc.DepthStencil)
	}

	// Multisample state.
	rDesc.Multisample = rwgpu.MultisampleState{
		Count:                  desc.Multisample.Count,
		Mask:                   uint32(desc.Multisample.Mask), //nolint:gosec // mask truncation is intentional (WebGPU spec: 32-bit)
		AlphaToCoverageEnabled: desc.Multisample.AlphaToCoverageEnabled,
	}

	// Fragment state.
	if desc.Fragment != nil {
		rDesc.Fragment = convertFragmentState(desc.Fragment)
	}

	return rDesc
}

// convertVertexBufferLayouts converts vertex buffer layouts.
// go-webgpu uses C-style pointer+count for attributes (FFI layer).
func convertVertexBufferLayouts(layouts []VertexBufferLayout) []rwgpu.VertexBufferLayout {
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
			FailOp:      rwgpu.StencilOperation(ds.StencilFront.FailOp),
			DepthFailOp: rwgpu.StencilOperation(ds.StencilFront.DepthFailOp),
			PassOp:      rwgpu.StencilOperation(ds.StencilFront.PassOp),
		},
		StencilBack: rwgpu.StencilFaceState{
			Compare:     ds.StencilBack.Compare,
			FailOp:      rwgpu.StencilOperation(ds.StencilBack.FailOp),
			DepthFailOp: rwgpu.StencilOperation(ds.StencilBack.DepthFailOp),
			PassOp:      rwgpu.StencilOperation(ds.StencilBack.PassOp),
		},
	}
}

// convertFragmentState converts fragment state.
func convertFragmentState(fs *FragmentState) *rwgpu.FragmentState {
	result := &rwgpu.FragmentState{
		EntryPoint: fs.EntryPoint,
	}
	if fs.Module != nil {
		result.Module = fs.Module.r
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

// convertRustErrorType maps go-webgpu ErrorType to our ErrorFilter.
func convertRustErrorType(et rwgpu.ErrorType) ErrorFilter {
	switch et {
	case rwgpu.ErrorTypeValidation:
		return ErrorFilterValidation
	case rwgpu.ErrorTypeOutOfMemory:
		return ErrorFilterOutOfMemory
	case rwgpu.ErrorTypeInternal:
		return ErrorFilterInternal
	default:
		return ErrorFilterInternal
	}
}
