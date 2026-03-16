package wgpu

import (
	"fmt"

	"github.com/gogpu/wgpu/core"
)

// RenderPassEncoder records draw commands within a render pass.
//
// Created by CommandEncoder.BeginRenderPass().
// Must be ended with End() before the CommandEncoder can be finished.
//
// NOT thread-safe.
type RenderPassEncoder struct {
	core    *core.CoreRenderPassEncoder
	encoder *CommandEncoder
	// currentPipelineBindGroupCount tracks the bind group count of the
	// currently set pipeline. Used by SetBindGroup to validate that the
	// group index is within the pipeline layout bounds. Zero means no
	// pipeline has been set yet.
	currentPipelineBindGroupCount uint32
}

// SetPipeline sets the active render pipeline.
func (p *RenderPassEncoder) SetPipeline(pipeline *RenderPipeline) {
	if pipeline == nil {
		p.encoder.setError(fmt.Errorf("wgpu: RenderPass.SetPipeline: pipeline is nil"))
		return
	}
	p.currentPipelineBindGroupCount = pipeline.bindGroupCount
	raw := p.core.RawPass()
	if raw != nil && pipeline.hal != nil {
		raw.SetPipeline(pipeline.hal)
	}
}

// SetBindGroup sets a bind group for the given index.
func (p *RenderPassEncoder) SetBindGroup(index uint32, group *BindGroup, offsets []uint32) {
	if group == nil {
		p.encoder.setError(fmt.Errorf("wgpu: RenderPass.SetBindGroup: bind group is nil"))
		return
	}
	// Validate that the group index is within the current pipeline's layout.
	// Without this check, binding a group beyond the pipeline layout causes
	// a Vulkan validation error or crash on AMD/NVIDIA GPUs (Intel tolerates it).
	if p.currentPipelineBindGroupCount > 0 && index >= p.currentPipelineBindGroupCount {
		p.encoder.setError(fmt.Errorf(
			"wgpu: RenderPass.SetBindGroup: group index %d exceeds pipeline layout bind group count %d",
			index, p.currentPipelineBindGroupCount,
		))
		return
	}
	raw := p.core.RawPass()
	if raw != nil && group.hal != nil {
		raw.SetBindGroup(index, group.hal, offsets)
	}
}

// SetVertexBuffer sets a vertex buffer for the given slot.
func (p *RenderPassEncoder) SetVertexBuffer(slot uint32, buffer *Buffer, offset uint64) {
	if buffer == nil {
		p.encoder.setError(fmt.Errorf("wgpu: RenderPass.SetVertexBuffer: buffer is nil"))
		return
	}
	p.core.SetVertexBuffer(slot, buffer.coreBuffer(), offset)
}

// SetIndexBuffer sets the index buffer.
func (p *RenderPassEncoder) SetIndexBuffer(buffer *Buffer, format IndexFormat, offset uint64) {
	if buffer == nil {
		p.encoder.setError(fmt.Errorf("wgpu: RenderPass.SetIndexBuffer: buffer is nil"))
		return
	}
	p.core.SetIndexBuffer(buffer.coreBuffer(), format, offset)
}

// SetViewport sets the viewport transformation.
func (p *RenderPassEncoder) SetViewport(x, y, width, height, minDepth, maxDepth float32) {
	p.core.SetViewport(x, y, width, height, minDepth, maxDepth)
}

// SetScissorRect sets the scissor rectangle for clipping.
func (p *RenderPassEncoder) SetScissorRect(x, y, width, height uint32) {
	p.core.SetScissorRect(x, y, width, height)
}

// SetBlendConstant sets the blend constant color.
func (p *RenderPassEncoder) SetBlendConstant(color *Color) {
	p.core.SetBlendConstant(color)
}

// SetStencilReference sets the stencil reference value.
func (p *RenderPassEncoder) SetStencilReference(reference uint32) {
	p.core.SetStencilReference(reference)
}

// Draw draws primitives.
func (p *RenderPassEncoder) Draw(vertexCount, instanceCount, firstVertex, firstInstance uint32) {
	p.core.Draw(vertexCount, instanceCount, firstVertex, firstInstance)
}

// DrawIndexed draws indexed primitives.
func (p *RenderPassEncoder) DrawIndexed(indexCount, instanceCount, firstIndex uint32, baseVertex int32, firstInstance uint32) {
	p.core.DrawIndexed(indexCount, instanceCount, firstIndex, baseVertex, firstInstance)
}

// DrawIndirect draws primitives with GPU-generated parameters.
func (p *RenderPassEncoder) DrawIndirect(buffer *Buffer, offset uint64) {
	if buffer == nil {
		p.encoder.setError(fmt.Errorf("wgpu: RenderPass.DrawIndirect: buffer is nil"))
		return
	}
	p.core.DrawIndirect(buffer.coreBuffer(), offset)
}

// DrawIndexedIndirect draws indexed primitives with GPU-generated parameters.
func (p *RenderPassEncoder) DrawIndexedIndirect(buffer *Buffer, offset uint64) {
	if buffer == nil {
		p.encoder.setError(fmt.Errorf("wgpu: RenderPass.DrawIndexedIndirect: buffer is nil"))
		return
	}
	p.core.DrawIndexedIndirect(buffer.coreBuffer(), offset)
}

// End ends the render pass.
// After this call, the encoder cannot be used again.
func (p *RenderPassEncoder) End() error {
	return p.core.End()
}
