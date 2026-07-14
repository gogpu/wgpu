//go:build rust

package wgpu

import (
	"math"

	rwgpu "github.com/go-webgpu/webgpu/wgpu"
)

// RenderPassEncoder records draw commands within a render pass.
// On Rust backend, this wraps go-webgpu/webgpu RenderPassEncoder.
type RenderPassEncoder struct {
	r        *rwgpu.RenderPassEncoder
	released bool
}

// SetPipeline sets the active render pipeline.
func (p *RenderPassEncoder) SetPipeline(pipeline *RenderPipeline) {
	if pipeline == nil || pipeline.r == nil {
		return
	}
	p.r.SetPipeline(pipeline.r)
}

// SetBindGroup sets a bind group for the given index.
func (p *RenderPassEncoder) SetBindGroup(index uint32, group *BindGroup, offsets []uint32) {
	if group == nil || group.r == nil {
		return
	}
	p.r.SetBindGroup(index, group.r, offsets)
}

// SetVertexBuffer sets a vertex buffer for the given slot.
// Offset is in bytes.
func (p *RenderPassEncoder) SetVertexBuffer(slot uint32, buffer *Buffer, offset uint64) {
	if buffer == nil || buffer.r == nil {
		return
	}
	// go-webgpu takes (slot, buffer, offset, size). Pass MaxUint64 for "rest of buffer".
	p.r.SetVertexBuffer(slot, buffer.r, offset, math.MaxUint64)
}

// SetIndexBuffer sets the index buffer.
func (p *RenderPassEncoder) SetIndexBuffer(buffer *Buffer, format IndexFormat, offset uint64) {
	if buffer == nil || buffer.r == nil {
		return
	}
	// go-webgpu takes (buffer, format, offset, size). Pass MaxUint64 for "rest of buffer".
	p.r.SetIndexBuffer(buffer.r, format, offset, math.MaxUint64)
}

// SetViewport sets the viewport transformation.
func (p *RenderPassEncoder) SetViewport(x, y, width, height, minDepth, maxDepth float32) {
	p.r.SetViewport(x, y, width, height, minDepth, maxDepth)
}

// SetScissorRect sets the scissor rectangle for clipping.
func (p *RenderPassEncoder) SetScissorRect(x, y, width, height uint32) {
	p.r.SetScissorRect(x, y, width, height)
}

// SetBlendConstant sets the blend constant color.
func (p *RenderPassEncoder) SetBlendConstant(color *Color) {
	if color == nil {
		return
	}
	p.r.SetBlendConstant(&rwgpu.Color{
		R: color.R,
		G: color.G,
		B: color.B,
		A: color.A,
	})
}

// SetStencilReference sets the stencil reference value.
func (p *RenderPassEncoder) SetStencilReference(reference uint32) {
	p.r.SetStencilReference(reference)
}

// Draw draws primitives.
func (p *RenderPassEncoder) Draw(vertexCount, instanceCount, firstVertex, firstInstance uint32) {
	p.r.Draw(vertexCount, instanceCount, firstVertex, firstInstance)
}

// DrawIndexed draws indexed primitives.
func (p *RenderPassEncoder) DrawIndexed(indexCount, instanceCount, firstIndex uint32, baseVertex int32, firstInstance uint32) {
	p.r.DrawIndexed(indexCount, instanceCount, firstIndex, baseVertex, firstInstance)
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
	if buffer == nil || buffer.r == nil {
		return
	}
	if !drawIndirectRangeFits(buffer.Size(), offset, drawCount) {
		p.r.DrawIndirect(buffer.r, indirectDelegatedValidationOffset(buffer.Size(), offset, drawIndirectRecordSize, drawCount))
		return
	}
	for i := uint32(0); i < drawCount; i++ {
		recordOffset, _ := drawIndirectRecordOffset(offset, i)
		p.r.DrawIndirect(buffer.r, recordOffset)
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
	if buffer == nil || buffer.r == nil {
		return
	}
	lowerRustIndexedIndirect(buffer.Size(), offset, drawCount, func(recordOffset uint64) {
		p.r.DrawIndexedIndirect(buffer.r, recordOffset)
	})
}

// lowerRustIndexedIndirect lowers one counted span through the Rust adapter's
// single-record interface. Invalid positive spans delegate exactly one failing
// record before any valid record can be emitted.
func lowerRustIndexedIndirect(bufferSize, offset uint64, drawCount uint32, draw func(uint64)) {
	if drawCount == 0 {
		return
	}
	if !indexedIndirectRangeFits(bufferSize, offset, drawCount) {
		draw(indirectDelegatedValidationOffset(bufferSize, offset, drawIndexedIndirectRecordSize, drawCount))
		return
	}
	for i := uint32(0); i < drawCount; i++ {
		recordOffset, _ := indexedIndirectRecordOffset(offset, i)
		draw(recordOffset)
	}
}

// End ends the render pass.
func (p *RenderPassEncoder) End() error {
	if p.released {
		return ErrReleased
	}
	p.released = true
	p.r.End()
	return nil
}
