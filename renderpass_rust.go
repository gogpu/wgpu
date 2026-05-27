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
	if buffer == nil || buffer.r == nil {
		return
	}
	p.r.DrawIndirect(buffer.r, offset)
}

// DrawIndexedIndirect draws indexed primitives with GPU-generated parameters.
func (p *RenderPassEncoder) DrawIndexedIndirect(buffer *Buffer, offset uint64) {
	if buffer == nil || buffer.r == nil {
		return
	}
	p.r.DrawIndexedIndirect(buffer.r, offset)
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
