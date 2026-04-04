package wgpu

import (
	"github.com/gogpu/wgpu/core"
	"github.com/gogpu/wgpu/hal"
)

// RenderPipeline represents a configured render pipeline.
type RenderPipeline struct {
	hal      hal.RenderPipeline
	device   *Device
	released bool
	// bindGroupCount is the number of bind group layouts in this pipeline's
	// layout. Used by RenderPassEncoder.SetBindGroup to validate that
	// the group index is within bounds before issuing the HAL call.
	bindGroupCount uint32
	// bindGroupLayouts stores the layouts from the pipeline layout.
	// Used by the binder for draw-time compatibility validation.
	bindGroupLayouts []*BindGroupLayout
	// requiredVertexBuffers is the number of vertex buffer layouts declared
	// in the pipeline's vertex state. Draw calls validate that at least this
	// many vertex buffers have been set via SetVertexBuffer.
	requiredVertexBuffers uint32
	// blendConstantRequired is true if any color target uses BlendFactorConstant
	// or BlendFactorOneMinusConstant. Draw calls validate that SetBlendConstant
	// has been called when this is true.
	// Matches Rust wgpu-core PipelineFlags::BLEND_CONSTANT.
	blendConstantRequired bool
	// ref is the GPU-aware reference counter for this pipeline (Phase 2).
	// Clone'd when used in a render pass, Drop'd when GPU completes submission.
	ref *core.ResourceRef
}

// Release destroys the render pipeline. Destruction is deferred until the GPU
// completes any submission that may reference this pipeline.
func (p *RenderPipeline) Release() {
	if p.released {
		return
	}
	p.released = true

	halDevice := p.device.halDevice()
	if halDevice == nil {
		return
	}

	dq := p.device.destroyQueue()
	if dq == nil {
		halDevice.DestroyRenderPipeline(p.hal)
		return
	}

	subIdx := p.device.lastSubmissionIndex()
	halPipeline := p.hal
	dq.Defer(subIdx, "RenderPipeline", func() {
		halDevice.DestroyRenderPipeline(halPipeline)
	})
}

// ComputePipeline represents a configured compute pipeline.
type ComputePipeline struct {
	hal      hal.ComputePipeline
	device   *Device
	released bool
	// bindGroupCount is the number of bind group layouts in this pipeline's
	// layout. Used by ComputePassEncoder.SetBindGroup to validate that
	// the group index is within bounds before issuing the HAL call.
	bindGroupCount uint32
	// bindGroupLayouts stores the layouts from the pipeline layout.
	// Used by the binder for draw-time compatibility validation.
	bindGroupLayouts []*BindGroupLayout
	// ref is the GPU-aware reference counter for this pipeline (Phase 2).
	// Clone'd when used in a compute pass, Drop'd when GPU completes submission.
	ref *core.ResourceRef
}

// Release destroys the compute pipeline. Destruction is deferred until the GPU
// completes any submission that may reference this pipeline.
func (p *ComputePipeline) Release() {
	if p.released {
		return
	}
	p.released = true

	halDevice := p.device.halDevice()
	if halDevice == nil {
		return
	}

	dq := p.device.destroyQueue()
	if dq == nil {
		halDevice.DestroyComputePipeline(p.hal)
		return
	}

	subIdx := p.device.lastSubmissionIndex()
	halPipeline := p.hal
	dq.Defer(subIdx, "ComputePipeline", func() {
		halDevice.DestroyComputePipeline(halPipeline)
	})
}
