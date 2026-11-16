//go:build !rust && !(js && wasm)

package wgpu

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
