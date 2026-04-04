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
	// pipelineSet tracks whether SetPipeline has been called.
	// Draw commands require a pipeline to be set first.
	pipelineSet bool
	// binder tracks bind group assignments and validates compatibility
	// at draw time, matching Rust wgpu-core's Binder pattern.
	binder binder
	// vertexBufferCount tracks the highest vertex buffer slot set + 1.
	// Updated by SetVertexBuffer; validated against pipeline requirements at draw time.
	vertexBufferCount uint32
	// requiredVertexBuffers is the number of vertex buffers required by the
	// current pipeline. Set by SetPipeline from RenderPipeline.requiredVertexBuffers.
	requiredVertexBuffers uint32
	// indexBufferSet tracks whether SetIndexBuffer has been called.
	// DrawIndexed and DrawIndexedIndirect require an index buffer.
	indexBufferSet bool
	// blendConstantRequired is true if the current pipeline uses
	// BlendFactorConstant or BlendFactorOneMinusConstant.
	// Set by SetPipeline from RenderPipeline.blendConstantRequired.
	blendConstantRequired bool
	// blendConstantSet tracks whether SetBlendConstant has been called.
	// Matches Rust wgpu-core OptionalState for blend_constant.
	blendConstantSet bool
	// trackedRefs accumulates Clone'd ResourceRefs for resources used in
	// this render pass. Transferred to the parent CommandEncoder on End().
	// Phase 2: per-command-buffer resource tracking.
	trackedRefs []*core.ResourceRef
}

// trackRef Clone()'s a ResourceRef and accumulates it for later transfer
// to the parent CommandEncoder. This keeps the resource alive until the
// GPU completes the submission containing this render pass.
func (p *RenderPassEncoder) trackRef(ref *core.ResourceRef) {
	if ref != nil {
		ref.Clone()
		p.trackedRefs = append(p.trackedRefs, ref)
	}
}

// SetPipeline sets the active render pipeline.
func (p *RenderPassEncoder) SetPipeline(pipeline *RenderPipeline) {
	if pipeline == nil {
		p.encoder.setError(fmt.Errorf("wgpu: RenderPass.SetPipeline: pipeline is nil"))
		return
	}
	p.currentPipelineBindGroupCount = pipeline.bindGroupCount
	p.pipelineSet = true
	p.requiredVertexBuffers = pipeline.requiredVertexBuffers
	if pipeline.blendConstantRequired {
		p.blendConstantRequired = true
	}
	p.binder.updateExpectations(pipeline.bindGroupLayouts)
	p.trackRef(pipeline.ref)
	raw := p.core.RawPass()
	if raw != nil && pipeline.hal != nil {
		raw.SetPipeline(pipeline.hal)
	}
}

// SetBindGroup sets a bind group for the given index.
func (p *RenderPassEncoder) SetBindGroup(index uint32, group *BindGroup, offsets []uint32) {
	if err := validateSetBindGroup("RenderPass", index, group, offsets, p.currentPipelineBindGroupCount); err != nil {
		p.encoder.setError(err)
		return
	}
	p.binder.assign(index, group.layout)
	p.trackRef(group.ref)
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
	if slot+1 > p.vertexBufferCount {
		p.vertexBufferCount = slot + 1
	}
	p.trackRef(buffer.core.Ref)
	p.core.SetVertexBuffer(slot, buffer.coreBuffer(), offset)
}

// SetIndexBuffer sets the index buffer.
func (p *RenderPassEncoder) SetIndexBuffer(buffer *Buffer, format IndexFormat, offset uint64) {
	if buffer == nil {
		p.encoder.setError(fmt.Errorf("wgpu: RenderPass.SetIndexBuffer: buffer is nil"))
		return
	}
	p.indexBufferSet = true
	p.trackRef(buffer.core.Ref)
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
	p.blendConstantSet = true
	p.core.SetBlendConstant(color)
}

// SetStencilReference sets the stencil reference value.
func (p *RenderPassEncoder) SetStencilReference(reference uint32) {
	p.core.SetStencilReference(reference)
}

// validateDrawState checks that a pipeline has been set, all bind groups
// are compatible, and enough vertex buffers have been set before a draw call.
// Returns true if validation passes, false if an error was recorded.
func (p *RenderPassEncoder) validateDrawState(method string) bool {
	if !p.pipelineSet {
		p.encoder.setError(fmt.Errorf("wgpu: RenderPass.%s: no pipeline set (call SetPipeline first)", method))
		return false
	}
	if err := p.binder.checkCompatibility(); err != nil {
		p.encoder.setError(fmt.Errorf("wgpu: RenderPass.%s: %w", method, err))
		return false
	}
	if p.vertexBufferCount < p.requiredVertexBuffers {
		p.encoder.setError(fmt.Errorf(
			"wgpu: RenderPass.%s: pipeline requires %d vertex buffer(s) but only %d set",
			method, p.requiredVertexBuffers, p.vertexBufferCount,
		))
		return false
	}
	if p.blendConstantRequired && !p.blendConstantSet {
		p.encoder.setError(fmt.Errorf(
			"wgpu: RenderPass.%s: blend constant needs to be set (pipeline uses BlendFactorConstant)",
			method,
		))
		return false
	}
	return true
}

// Draw draws primitives.
func (p *RenderPassEncoder) Draw(vertexCount, instanceCount, firstVertex, firstInstance uint32) {
	if !p.validateDrawState("Draw") {
		return
	}
	p.core.Draw(vertexCount, instanceCount, firstVertex, firstInstance)
}

// DrawIndexed draws indexed primitives.
func (p *RenderPassEncoder) DrawIndexed(indexCount, instanceCount, firstIndex uint32, baseVertex int32, firstInstance uint32) {
	if !p.validateDrawState("DrawIndexed") {
		return
	}
	if !p.indexBufferSet {
		p.encoder.setError(fmt.Errorf("wgpu: RenderPass.DrawIndexed: no index buffer set (call SetIndexBuffer first)"))
		return
	}
	p.core.DrawIndexed(indexCount, instanceCount, firstIndex, baseVertex, firstInstance)
}

// DrawIndirect draws primitives with GPU-generated parameters.
func (p *RenderPassEncoder) DrawIndirect(buffer *Buffer, offset uint64) {
	if !p.validateDrawState("DrawIndirect") {
		return
	}
	if buffer == nil {
		p.encoder.setError(fmt.Errorf("wgpu: RenderPass.DrawIndirect: buffer is nil"))
		return
	}
	p.trackRef(buffer.core.Ref)
	p.core.DrawIndirect(buffer.coreBuffer(), offset)
}

// DrawIndexedIndirect draws indexed primitives with GPU-generated parameters.
func (p *RenderPassEncoder) DrawIndexedIndirect(buffer *Buffer, offset uint64) {
	if !p.validateDrawState("DrawIndexedIndirect") {
		return
	}
	if !p.indexBufferSet {
		p.encoder.setError(fmt.Errorf("wgpu: RenderPass.DrawIndexedIndirect: no index buffer set (call SetIndexBuffer first)"))
		return
	}
	if buffer == nil {
		p.encoder.setError(fmt.Errorf("wgpu: RenderPass.DrawIndexedIndirect: buffer is nil"))
		return
	}
	p.trackRef(buffer.core.Ref)
	p.core.DrawIndexedIndirect(buffer.coreBuffer(), offset)
}

// End ends the render pass.
// After this call, the encoder cannot be used again.
func (p *RenderPassEncoder) End() error {
	// Transfer tracked refs to parent CommandEncoder before ending.
	if len(p.trackedRefs) > 0 {
		p.encoder.trackedRefs = append(p.encoder.trackedRefs, p.trackedRefs...)
		p.trackedRefs = nil
	}
	return p.core.End()
}
