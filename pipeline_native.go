//go:build !(js && wasm)

package wgpu

import (
	"github.com/gogpu/naga/ir"
	"github.com/gogpu/wgpu/core"
	"github.com/gogpu/wgpu/hal"
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

// makeLateSizedBufferGroups generates late-sized buffer group info for a pipeline
// from the shader binding sizes and the pipeline layout. For each bind group layout,
// filters entries to those with MinBindingSize == 0 (buffer type), and records the
// shader-required size for each.
//
// Equivalent to Rust wgpu-core's Device::make_late_sized_buffer_groups (resource.rs:2413-2448).
func makeLateSizedBufferGroups(
	shaderBindingSizes map[ir.ResourceBinding]uint64,
	layouts []*BindGroupLayout,
) [MaxBindGroups]LateSizedBufferGroup {
	var groups [MaxBindGroups]LateSizedBufferGroup

	for groupIdx, bgl := range layouts {
		if bgl == nil {
			continue
		}
		var sizes []uint64
		for _, entry := range bgl.entries {
			if entry.Buffer == nil || entry.Buffer.MinBindingSize != 0 {
				continue
			}
			// This is a buffer entry with MinBindingSize == 0 —
			// validation is deferred to draw/dispatch time.
			rb := ir.ResourceBinding{
				Group:   uint32(groupIdx),
				Binding: entry.Binding,
			}
			shaderSize := shaderBindingSizes[rb] // 0 if not found
			sizes = append(sizes, shaderSize)
		}
		groups[groupIdx] = LateSizedBufferGroup{ShaderSizes: sizes}
	}

	return groups
}

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
	// lateSizedBufferGroups holds shader-required minimum sizes for buffer bindings
	// whose layout has MinBindingSize == 0. Validated at draw time.
	// Matches Rust wgpu-core RenderPipeline.late_sized_buffer_groups.
	lateSizedBufferGroups [MaxBindGroups]LateSizedBufferGroup
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
	// lateSizedBufferGroups holds shader-required minimum sizes for buffer bindings
	// whose layout has MinBindingSize == 0. Validated at dispatch time.
	// Matches Rust wgpu-core ComputePipeline.late_sized_buffer_groups.
	lateSizedBufferGroups [MaxBindGroups]LateSizedBufferGroup
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
