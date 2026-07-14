//go:build js && wasm

package wgpu

import "github.com/gogpu/wgpu/internal/browser"

// RenderPassEncoder records draw commands within a render pass.
// On browser, this wraps a GPURenderPassEncoder via internal/browser.RenderPassEncoder.
type RenderPassEncoder struct {
	browser  *browser.RenderPassEncoder
	released bool
}

// SetPipeline sets the active render pipeline.
func (p *RenderPassEncoder) SetPipeline(pipeline *RenderPipeline) {
	if pipeline == nil || pipeline.browser == nil {
		return
	}
	p.browser.SetPipeline(pipeline.browser.Ref())
}

// SetBindGroup sets a bind group for the given index.
func (p *RenderPassEncoder) SetBindGroup(index uint32, group *BindGroup, offsets []uint32) {
	if group == nil || group.browser == nil {
		return
	}
	p.browser.SetBindGroup(index, group.browser.Ref(), offsets)
}

// SetVertexBuffer sets a vertex buffer for the given slot.
// Offset is in bytes. Pass 0 for size to use the rest of the buffer.
func (p *RenderPassEncoder) SetVertexBuffer(slot uint32, buffer *Buffer, offset uint64) {
	if buffer == nil || buffer.browser == nil {
		return
	}
	// size=0 tells the browser layer to omit the size parameter,
	// which means "rest of buffer" per the WebGPU spec.
	p.browser.SetVertexBuffer(slot, buffer.browser.Ref(), offset, 0)
}

// SetIndexBuffer sets the index buffer.
func (p *RenderPassEncoder) SetIndexBuffer(buffer *Buffer, format IndexFormat, offset uint64) {
	if buffer == nil || buffer.browser == nil {
		return
	}
	formatStr := browser.IndexFormatToJS(format)
	// size=0 tells the browser layer to omit the size parameter.
	p.browser.SetIndexBuffer(buffer.browser.Ref(), formatStr, offset, 0)
}

// SetViewport sets the viewport transformation.
func (p *RenderPassEncoder) SetViewport(x, y, width, height, minDepth, maxDepth float32) {
	p.browser.SetViewport(x, y, width, height, minDepth, maxDepth)
}

// SetScissorRect sets the scissor rectangle for clipping.
func (p *RenderPassEncoder) SetScissorRect(x, y, width, height uint32) {
	p.browser.SetScissorRect(x, y, width, height)
}

// SetBlendConstant sets the blend constant color.
func (p *RenderPassEncoder) SetBlendConstant(color *Color) {
	if color == nil {
		return
	}
	jsColor := browser.BuildColorDict(color.R, color.G, color.B, color.A)
	p.browser.SetBlendConstant(jsColor)
}

// SetStencilReference sets the stencil reference value.
func (p *RenderPassEncoder) SetStencilReference(reference uint32) {
	p.browser.SetStencilReference(reference)
}

// Draw draws primitives.
func (p *RenderPassEncoder) Draw(vertexCount, instanceCount, firstVertex, firstInstance uint32) {
	p.browser.Draw(vertexCount, instanceCount, firstVertex, firstInstance)
}

// DrawIndexed draws indexed primitives.
func (p *RenderPassEncoder) DrawIndexed(indexCount, instanceCount, firstIndex uint32, baseVertex int32, firstInstance uint32) {
	p.browser.DrawIndexed(indexCount, instanceCount, firstIndex, baseVertex, firstInstance)
}

// DrawIndirect draws primitives with GPU-generated parameters.
func (p *RenderPassEncoder) DrawIndirect(buffer *Buffer, offset uint64) {
	p.MultiDrawIndirect(buffer, offset, 1)
}

// MultiDrawIndirect draws consecutive primitives with GPU-generated parameters.
func (p *RenderPassEncoder) MultiDrawIndirect(buffer *Buffer, offset uint64, drawCount uint32) {
	if drawCount == 0 {
		return
	}
	if buffer == nil || buffer.browser == nil {
		return
	}
	if !drawIndirectRangeFits(buffer.Size(), offset, drawCount) {
		p.browser.DrawIndirect(buffer.browser.Ref(), indirectDelegatedValidationOffset(buffer.Size(), offset, drawIndirectRecordSize, drawCount))
		return
	}
	for i := uint32(0); i < drawCount; i++ {
		recordOffset, _ := drawIndirectRecordOffset(offset, i)
		p.browser.DrawIndirect(buffer.browser.Ref(), recordOffset)
	}
}

// DrawIndexedIndirect draws indexed primitives with GPU-generated parameters.
func (p *RenderPassEncoder) DrawIndexedIndirect(buffer *Buffer, offset uint64) {
	p.MultiDrawIndexedIndirect(buffer, offset, 1)
}

// MultiDrawIndexedIndirect draws consecutive indexed primitives with
// GPU-generated parameters.
func (p *RenderPassEncoder) MultiDrawIndexedIndirect(buffer *Buffer, offset uint64, drawCount uint32) {
	if drawCount == 0 {
		return
	}
	if buffer == nil || buffer.browser == nil {
		return
	}
	if !indexedIndirectRangeFits(buffer.Size(), offset, drawCount) {
		p.browser.DrawIndexedIndirect(buffer.browser.Ref(), indirectDelegatedValidationOffset(buffer.Size(), offset, drawIndexedIndirectRecordSize, drawCount))
		return
	}
	for i := uint32(0); i < drawCount; i++ {
		recordOffset, _ := indexedIndirectRecordOffset(offset, i)
		p.browser.DrawIndexedIndirect(buffer.browser.Ref(), recordOffset)
	}
}

// End ends the render pass.
func (p *RenderPassEncoder) End() error {
	if p.released {
		return ErrReleased
	}
	p.released = true
	p.browser.End()
	return nil
}
