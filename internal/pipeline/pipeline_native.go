//go:build !rust && !(js && wasm)

package pipeline

import (
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/hal"
	"github.com/gogpu/wgpu/internal/resource"
)

// LateSizedBufferGroup holds the shader-required minimum buffer sizes for
// bind group entries whose layout specifies MinBindingSize == 0. These sizes
// are validated at draw/dispatch time against the actual bound buffer sizes.
//
// The order of ShaderSizes matches the order of buffer entries with
// MinBindingSize == 0 in the corresponding BindGroupLayout, matching
// Rust wgpu-core's pipeline::LateSizedBufferGroup.
type LateSizedBufferGroup struct {
	ShaderSizes []uint64
}

// RenderPipeline represents a configured render pipeline.
type RenderPipeline struct {
	hal hal.RenderPipeline
	// device is the device view the pipeline was created on. The render bundle
	// checks it against its own device before recording the pipeline, so a
	// pipeline of another device is rejected (ADR-070 §15.3: view identity is
	// device identity, one cached view per Device).
	device   core.DeviceView
	released bool
	// bindGroupCount is the number of bind group layouts in this pipeline's
	// layout. Used by RenderPassEncoder.SetBindGroup to validate that
	// the group index is within bounds before issuing the HAL call.
	bindGroupCount uint32
	// bindGroupLayouts stores the layouts from the pipeline layout.
	// Used by the binder for draw-time compatibility validation.
	bindGroupLayouts []*resource.BindGroupLayout
	// requiredVertexBuffers is the number of vertex buffer layouts declared
	// in the pipeline's vertex state. Draw calls validate that at least this
	// many vertex buffers have been set via SetVertexBuffer.
	requiredVertexBuffers uint32
	// stripIndexFormat is the index format required by strip topologies.
	// Nil for non-strip topologies. When non-nil, DrawIndexed/DrawIndexedIndirect
	// validate that the bound index buffer format matches this value.
	// Matches Rust wgpu-core RenderPipeline.strip_index_format (render.rs:568-582).
	stripIndexFormat *gputypes.IndexFormat
	// blendConstantRequired is true if any color target uses BlendFactorConstant
	// or BlendFactorOneMinusConstant. Draw calls validate that SetBlendConstant
	// has been called when this is true.
	// Matches Rust wgpu-core PipelineFlags::BLEND_CONSTANT.
	blendConstantRequired bool
	// lateSizedBufferGroups holds shader-required minimum sizes for buffer bindings
	// whose layout has MinBindingSize == 0. Validated at draw time.
	// Matches Rust wgpu-core RenderPipeline.late_sized_buffer_groups.
	lateSizedBufferGroups [core.MaxBindGroups]LateSizedBufferGroup
	// passContext holds the color target formats, the depth/stencil format
	// and the sample count the pipeline renders to. A render bundle checks
	// it against its own formats at SetPipeline.
	// Matches Rust wgpu-core RenderPipeline.pass_context.
	passContext core.RenderPassContext
	// writesDepth and writesStencil report whether the pipeline writes the
	// depth or the stencil aspect, which a read-only bundle forbids.
	// Matches Rust wgpu-core PipelineFlags::WRITES_DEPTH and WRITES_STENCIL.
	writesDepth   bool
	writesStencil bool
	// ref is the GPU-aware reference counter for this pipeline (Phase 2).
	// Clone'd when used in a render pass, Drop'd when GPU completes submission.
	ref *core.ResourceRef
}

// RenderPipelineInit carries the creation state of a RenderPipeline. The
// device fills it from a validated descriptor; the constructor owns the
// resulting fields.
type RenderPipelineInit struct {
	// HAL is the backend render pipeline.
	HAL hal.RenderPipeline
	// Device is the device view the pipeline was created on.
	Device core.DeviceView
	// BindGroupCount is the number of bind group layouts in the pipeline layout.
	BindGroupCount uint32
	// BindGroupLayouts are the layouts of the pipeline layout.
	BindGroupLayouts []*resource.BindGroupLayout
	// RequiredVertexBuffers is the number of vertex buffer layouts declared in
	// the pipeline's vertex state.
	RequiredVertexBuffers uint32
	// StripIndexFormat is the index format required by strip topologies; nil
	// for non-strip topologies.
	StripIndexFormat *gputypes.IndexFormat
	// BlendConstantRequired is true if any color target uses a constant blend
	// factor.
	BlendConstantRequired bool
	// LateSizedBufferGroups holds the shader-required minimum sizes for buffer
	// bindings whose layout has MinBindingSize == 0.
	LateSizedBufferGroups [core.MaxBindGroups]LateSizedBufferGroup
	// PassContext is the target formats and sample count the pipeline renders to.
	PassContext core.RenderPassContext
	// WritesDepth and WritesStencil report whether the pipeline writes the
	// depth or the stencil aspect.
	WritesDepth   bool
	WritesStencil bool
	// Ref is the GPU-aware reference counter for this pipeline.
	Ref *core.ResourceRef
}

// NewRenderPipeline builds a render pipeline from its creation state.
func NewRenderPipeline(init *RenderPipelineInit) *RenderPipeline {
	return &RenderPipeline{
		hal:                   init.HAL,
		device:                init.Device,
		bindGroupCount:        init.BindGroupCount,
		bindGroupLayouts:      init.BindGroupLayouts,
		requiredVertexBuffers: init.RequiredVertexBuffers,
		stripIndexFormat:      init.StripIndexFormat,
		blendConstantRequired: init.BlendConstantRequired,
		lateSizedBufferGroups: init.LateSizedBufferGroups,
		passContext:           init.PassContext,
		writesDepth:           init.WritesDepth,
		writesStencil:         init.WritesStencil,
		ref:                   init.Ref,
	}
}

// Release drops the application's ownership reference to the render pipeline.
//
// If the pipeline is still referenced by in-flight GPU submissions (Clone'd
// via SetPipeline), the HAL pipeline stays alive until the GPU completes and
// Triage drops all tracked refs. The onZero callback (set at CreateRenderPipeline)
// fires only when the last reference drops. ADR-056: unified resource lifecycle.
func (p *RenderPipeline) Release() {
	if p.released {
		return
	}
	p.released = true

	if p.ref != nil {
		p.ref.Drop()
	}
}

// HAL returns the backend render pipeline, or nil.
func (p *RenderPipeline) HAL() hal.RenderPipeline {
	if p == nil {
		return nil
	}
	return p.hal
}

// Device returns the device view the pipeline was created on, or nil.
func (p *RenderPipeline) Device() core.DeviceView {
	if p == nil {
		return nil
	}
	return p.device
}

// IsReleased reports whether Release has been called.
func (p *RenderPipeline) IsReleased() bool {
	return p != nil && p.released
}

// Ref returns the GPU-aware reference counter, or nil.
func (p *RenderPipeline) Ref() *core.ResourceRef {
	if p == nil {
		return nil
	}
	return p.ref
}

// BindGroupCount returns the number of bind group layouts in the pipeline layout.
func (p *RenderPipeline) BindGroupCount() uint32 {
	if p == nil {
		return 0
	}
	return p.bindGroupCount
}

// BindGroupLayouts returns the layouts of the pipeline layout, or nil.
func (p *RenderPipeline) BindGroupLayouts() []*resource.BindGroupLayout {
	if p == nil {
		return nil
	}
	return p.bindGroupLayouts
}

// RequiredVertexBuffers returns the number of vertex buffer layouts the
// pipeline's vertex state declares.
func (p *RenderPipeline) RequiredVertexBuffers() uint32 {
	if p == nil {
		return 0
	}
	return p.requiredVertexBuffers
}

// StripIndexFormat returns the index format required by strip topologies, or
// nil for non-strip topologies.
func (p *RenderPipeline) StripIndexFormat() *gputypes.IndexFormat {
	if p == nil {
		return nil
	}
	return p.stripIndexFormat
}

// BlendConstantRequired reports whether any color target uses a constant blend
// factor.
func (p *RenderPipeline) BlendConstantRequired() bool {
	return p != nil && p.blendConstantRequired
}

// LateSizedBufferGroups returns the shader-required minimum sizes for buffer
// bindings whose layout has MinBindingSize == 0.
func (p *RenderPipeline) LateSizedBufferGroups() [core.MaxBindGroups]LateSizedBufferGroup {
	if p == nil {
		return [core.MaxBindGroups]LateSizedBufferGroup{}
	}
	return p.lateSizedBufferGroups
}

// PassContext returns the target formats and sample count the pipeline renders to.
func (p *RenderPipeline) PassContext() core.RenderPassContext {
	if p == nil {
		return core.RenderPassContext{}
	}
	return p.passContext
}

// WritesDepth reports whether the pipeline writes the depth aspect.
func (p *RenderPipeline) WritesDepth() bool {
	return p != nil && p.writesDepth
}

// WritesStencil reports whether the pipeline writes the stencil aspect.
func (p *RenderPipeline) WritesStencil() bool {
	return p != nil && p.writesStencil
}

// SetRequiredVertexBuffers sets the vertex buffer requirement. The public
// creation path fills it from the descriptor; the setter exists for the root
// test hooks that build a pipeline fixture.
func (p *RenderPipeline) SetRequiredVertexBuffers(count uint32) {
	p.requiredVertexBuffers = count
}

// SetBindGroupLayouts replaces the pipeline layout's layouts and their count.
// Test-support, as SetRequiredVertexBuffers.
func (p *RenderPipeline) SetBindGroupLayouts(layouts []*resource.BindGroupLayout) {
	p.bindGroupLayouts = layouts
	p.bindGroupCount = uint32(len(layouts)) //nolint:gosec // G115: a validated pipeline layout holds at most core.MaxBindGroups layouts
}

// SetStripIndexFormat sets the strip index format. Test-support, as
// SetRequiredVertexBuffers.
func (p *RenderPipeline) SetStripIndexFormat(format *gputypes.IndexFormat) {
	p.stripIndexFormat = format
}

// ComputePipeline represents a configured compute pipeline.
type ComputePipeline struct {
	hal      hal.ComputePipeline
	released bool
	// bindGroupCount is the number of bind group layouts in this pipeline's
	// layout. Used by ComputePassEncoder.SetBindGroup to validate that
	// the group index is within bounds before issuing the HAL call.
	bindGroupCount uint32
	// bindGroupLayouts stores the layouts from the pipeline layout.
	// Used by the binder for draw-time compatibility validation.
	bindGroupLayouts []*resource.BindGroupLayout
	// lateSizedBufferGroups holds shader-required minimum sizes for buffer bindings
	// whose layout has MinBindingSize == 0. Validated at dispatch time.
	// Matches Rust wgpu-core ComputePipeline.late_sized_buffer_groups.
	lateSizedBufferGroups [core.MaxBindGroups]LateSizedBufferGroup
	// ref is the GPU-aware reference counter for this pipeline (Phase 2).
	// Clone'd when used in a compute pass, Drop'd when GPU completes submission.
	ref *core.ResourceRef
}

// ComputePipelineInit carries the creation state of a ComputePipeline. The
// device fills it from a validated descriptor; the constructor owns the
// resulting fields.
type ComputePipelineInit struct {
	// HAL is the backend compute pipeline.
	HAL hal.ComputePipeline
	// BindGroupCount is the number of bind group layouts in the pipeline layout.
	BindGroupCount uint32
	// BindGroupLayouts are the layouts of the pipeline layout.
	BindGroupLayouts []*resource.BindGroupLayout
	// LateSizedBufferGroups holds the shader-required minimum sizes for buffer
	// bindings whose layout has MinBindingSize == 0.
	LateSizedBufferGroups [core.MaxBindGroups]LateSizedBufferGroup
	// Ref is the GPU-aware reference counter for this pipeline.
	Ref *core.ResourceRef
}

// NewComputePipeline builds a compute pipeline from its creation state.
func NewComputePipeline(init *ComputePipelineInit) *ComputePipeline {
	return &ComputePipeline{
		hal:                   init.HAL,
		bindGroupCount:        init.BindGroupCount,
		bindGroupLayouts:      init.BindGroupLayouts,
		lateSizedBufferGroups: init.LateSizedBufferGroups,
		ref:                   init.Ref,
	}
}

// Release drops the application's ownership reference to the compute pipeline.
//
// If the pipeline is still referenced by in-flight GPU submissions (Clone'd
// via SetPipeline), the HAL pipeline stays alive until the GPU completes and
// Triage drops all tracked refs. The onZero callback (set at CreateComputePipeline)
// fires only when the last reference drops. ADR-056: unified resource lifecycle.
func (p *ComputePipeline) Release() {
	if p.released {
		return
	}
	p.released = true

	if p.ref != nil {
		p.ref.Drop()
	}
}

// HAL returns the backend compute pipeline, or nil.
func (p *ComputePipeline) HAL() hal.ComputePipeline {
	if p == nil {
		return nil
	}
	return p.hal
}

// IsReleased reports whether Release has been called.
func (p *ComputePipeline) IsReleased() bool {
	return p != nil && p.released
}

// BindGroupCount returns the number of bind group layouts in the pipeline layout.
func (p *ComputePipeline) BindGroupCount() uint32 {
	if p == nil {
		return 0
	}
	return p.bindGroupCount
}

// BindGroupLayouts returns the layouts of the pipeline layout, or nil.
func (p *ComputePipeline) BindGroupLayouts() []*resource.BindGroupLayout {
	if p == nil {
		return nil
	}
	return p.bindGroupLayouts
}

// LateSizedBufferGroups returns the shader-required minimum sizes for buffer
// bindings whose layout has MinBindingSize == 0.
func (p *ComputePipeline) LateSizedBufferGroups() [core.MaxBindGroups]LateSizedBufferGroup {
	if p == nil {
		return [core.MaxBindGroups]LateSizedBufferGroup{}
	}
	return p.lateSizedBufferGroups
}

// Ref returns the GPU-aware reference counter, or nil.
func (p *ComputePipeline) Ref() *core.ResourceRef {
	if p == nil {
		return nil
	}
	return p.ref
}

// SetBindGroupLayouts replaces the pipeline layout's layouts and their count.
// The public creation path fills them from the descriptor; the setter exists
// for the root test hooks that build a pipeline fixture.
func (p *ComputePipeline) SetBindGroupLayouts(layouts []*resource.BindGroupLayout) {
	p.bindGroupLayouts = layouts
	p.bindGroupCount = uint32(len(layouts)) //nolint:gosec // G115: a validated pipeline layout holds at most core.MaxBindGroups layouts
}
