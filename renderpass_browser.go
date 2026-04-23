//go:build js && wasm

package wgpu

// RenderPassEncoder records draw commands within a render pass.
type RenderPassEncoder struct {
	released bool
}

// SetPipeline sets the active render pipeline.
func (p *RenderPassEncoder) SetPipeline(pipeline *RenderPipeline) {
	panic("wgpu: browser backend not yet implemented")
}

// SetBindGroup sets a bind group for the given index.
func (p *RenderPassEncoder) SetBindGroup(index uint32, group *BindGroup, offsets []uint32) {
	panic("wgpu: browser backend not yet implemented")
}

// SetVertexBuffer sets a vertex buffer for the given slot.
func (p *RenderPassEncoder) SetVertexBuffer(slot uint32, buffer *Buffer, offset uint64) {
	panic("wgpu: browser backend not yet implemented")
}

// SetIndexBuffer sets the index buffer.
func (p *RenderPassEncoder) SetIndexBuffer(buffer *Buffer, format IndexFormat, offset uint64) {
	panic("wgpu: browser backend not yet implemented")
}

// SetViewport sets the viewport transformation.
func (p *RenderPassEncoder) SetViewport(x, y, width, height, minDepth, maxDepth float32) {
	panic("wgpu: browser backend not yet implemented")
}

// SetScissorRect sets the scissor rectangle for clipping.
func (p *RenderPassEncoder) SetScissorRect(x, y, width, height uint32) {
	panic("wgpu: browser backend not yet implemented")
}

// SetBlendConstant sets the blend constant color.
func (p *RenderPassEncoder) SetBlendConstant(color *Color) {
	panic("wgpu: browser backend not yet implemented")
}

// SetStencilReference sets the stencil reference value.
func (p *RenderPassEncoder) SetStencilReference(reference uint32) {
	panic("wgpu: browser backend not yet implemented")
}

// Draw draws primitives.
func (p *RenderPassEncoder) Draw(vertexCount, instanceCount, firstVertex, firstInstance uint32) {
	panic("wgpu: browser backend not yet implemented")
}

// DrawIndexed draws indexed primitives.
func (p *RenderPassEncoder) DrawIndexed(indexCount, instanceCount, firstIndex uint32, baseVertex int32, firstInstance uint32) {
	panic("wgpu: browser backend not yet implemented")
}

// DrawIndirect draws primitives with GPU-generated parameters.
func (p *RenderPassEncoder) DrawIndirect(buffer *Buffer, offset uint64) {
	panic("wgpu: browser backend not yet implemented")
}

// DrawIndexedIndirect draws indexed primitives with GPU-generated parameters.
func (p *RenderPassEncoder) DrawIndexedIndirect(buffer *Buffer, offset uint64) {
	panic("wgpu: browser backend not yet implemented")
}

// End ends the render pass.
func (p *RenderPassEncoder) End() error {
	panic("wgpu: browser backend not yet implemented")
}
