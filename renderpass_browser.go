//go:build js && wasm

package wgpu

import (
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/encoder"
)

// RenderPassEncoder records draw commands within a render pass.
// On browser, this wraps a GPURenderPassEncoder via internal/encoder.
//
// The implementation lives in internal/encoder (ADR-070 S5); this declaration
// is the public surface. The nil-argument guards keep the pre-move contract of
// a zero-value pass: a nil pipeline, bind group, buffer or blend constant
// returns before the nil implementation is touched.
type RenderPassEncoder struct {
	impl *encoder.RenderPassEncoder
}

// SetPipeline sets the active render pipeline.
func (p *RenderPassEncoder) SetPipeline(pipeline *RenderPipeline) {
	if pipeline == nil {
		return
	}
	p.impl.SetPipeline(encoderPipeline(pipeline))
}

// SetBindGroup sets a bind group for the given index.
func (p *RenderPassEncoder) SetBindGroup(index uint32, group *BindGroup, offsets []uint32) {
	if group == nil {
		return
	}
	p.impl.SetBindGroup(index, encoderBindGroup(group), offsets)
}

// SetVertexBuffer sets a vertex buffer for the given slot.
// Offset is in bytes. Pass 0 for size to use the rest of the buffer.
func (p *RenderPassEncoder) SetVertexBuffer(slot uint32, buffer *Buffer, offset uint64) {
	if buffer == nil {
		return
	}
	p.impl.SetVertexBuffer(slot, encoderBuffer(buffer), offset)
}

// SetIndexBuffer sets the index buffer.
func (p *RenderPassEncoder) SetIndexBuffer(buffer *Buffer, format gputypes.IndexFormat, offset uint64) {
	if buffer == nil {
		return
	}
	p.impl.SetIndexBuffer(encoderBuffer(buffer), format, offset)
}

// SetViewport sets the viewport transformation.
func (p *RenderPassEncoder) SetViewport(vp gputypes.Viewport) {
	p.impl.SetViewport(vp)
}

// SetScissorRect sets the scissor rectangle for clipping.
func (p *RenderPassEncoder) SetScissorRect(rect gputypes.ScissorRect) {
	p.impl.SetScissorRect(rect)
}

// SetBlendConstant sets the blend constant color.
func (p *RenderPassEncoder) SetBlendConstant(color *gputypes.Color) {
	if color == nil {
		return
	}
	p.impl.SetBlendConstant(color)
}

// SetStencilReference sets the stencil reference value.
func (p *RenderPassEncoder) SetStencilReference(reference uint32) {
	p.impl.SetStencilReference(reference)
}

// Draw draws primitives.
func (p *RenderPassEncoder) Draw(args gputypes.DrawArgs) {
	p.impl.Draw(args)
}

// DrawIndexed draws indexed primitives.
func (p *RenderPassEncoder) DrawIndexed(args gputypes.DrawIndexedArgs) {
	p.impl.DrawIndexed(args)
}

// DrawIndirect draws primitives with GPU-generated parameters.
func (p *RenderPassEncoder) DrawIndirect(buffer *Buffer, offset uint64) {
	p.impl.DrawIndirect(encoderBuffer(buffer), offset)
}

// MultiDrawIndirect draws consecutive primitives with GPU-generated parameters.
func (p *RenderPassEncoder) MultiDrawIndirect(buffer *Buffer, offset uint64, drawCount uint32) {
	p.impl.MultiDrawIndirect(encoderBuffer(buffer), offset, drawCount)
}

// DrawIndexedIndirect draws indexed primitives with GPU-generated parameters.
func (p *RenderPassEncoder) DrawIndexedIndirect(buffer *Buffer, offset uint64) {
	p.impl.DrawIndexedIndirect(encoderBuffer(buffer), offset)
}

// MultiDrawIndexedIndirect draws consecutive indexed primitives with
// GPU-generated parameters.
func (p *RenderPassEncoder) MultiDrawIndexedIndirect(buffer *Buffer, offset uint64, drawCount uint32) {
	p.impl.MultiDrawIndexedIndirect(encoderBuffer(buffer), offset, drawCount)
}

// End ends the render pass.
func (p *RenderPassEncoder) End() error {
	return p.impl.End()
}
