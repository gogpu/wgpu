//go:build rust

package encoder

import (
	"math"

	rwgpu "github.com/go-webgpu/webgpu/wgpu"
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/indirect"
	"github.com/gogpu/wgpu/internal/pipeline"
	"github.com/gogpu/wgpu/internal/resource"
)

// RenderPassEncoder records draw commands within a render pass.
// On Rust backend, this wraps go-webgpu/webgpu RenderPassEncoder.
//
// The type lived in the root package until ADR-070 S5; its signatures name
// the domain's handle types, and the root declaration layer converts at the
// call boundary. The nil-argument guards here serve this package's own
// callers; the public wrapper repeats them for its part, because a zero-value
// public pass (nil implementation) must still answer a nil argument by
// returning, as the pre-move zero value did.
type RenderPassEncoder struct {
	r        *rwgpu.RenderPassEncoder
	released bool
}

// SetPipeline sets the active render pipeline.
func (p *RenderPassEncoder) SetPipeline(pl *pipeline.RenderPipeline) {
	if pl == nil || pl.R() == nil {
		return
	}
	p.r.SetPipeline(pl.R())
}

// SetBindGroup sets a bind group for the given index.
func (p *RenderPassEncoder) SetBindGroup(index uint32, group *resource.BindGroup, offsets []uint32) {
	if group == nil || group.Rust() == nil {
		return
	}
	p.r.SetBindGroup(index, group.Rust(), offsets)
}

// SetVertexBuffer sets a vertex buffer for the given slot.
// Offset is in bytes.
func (p *RenderPassEncoder) SetVertexBuffer(slot uint32, buffer *resource.Buffer, offset uint64) {
	if buffer == nil || buffer.R() == nil {
		return
	}
	// go-webgpu takes (slot, buffer, offset, size). Pass MaxUint64 for "rest of buffer".
	p.r.SetVertexBuffer(slot, buffer.R(), offset, math.MaxUint64)
}

// SetIndexBuffer sets the index buffer.
func (p *RenderPassEncoder) SetIndexBuffer(buffer *resource.Buffer, format gputypes.IndexFormat, offset uint64) {
	if buffer == nil || buffer.R() == nil {
		return
	}
	// go-webgpu takes (buffer, format, offset, size). Pass MaxUint64 for "rest of buffer".
	p.r.SetIndexBuffer(buffer.R(), format, offset, math.MaxUint64)
}

// SetViewport sets the viewport transformation.
func (p *RenderPassEncoder) SetViewport(vp gputypes.Viewport) {
	p.r.SetViewport(vp.X, vp.Y, vp.Width, vp.Height, vp.MinDepth, vp.MaxDepth)
}

// SetScissorRect sets the scissor rectangle for clipping.
func (p *RenderPassEncoder) SetScissorRect(rect gputypes.ScissorRect) {
	p.r.SetScissorRect(rect.X, rect.Y, rect.Width, rect.Height)
}

// SetBlendConstant sets the blend constant color.
func (p *RenderPassEncoder) SetBlendConstant(color *gputypes.Color) {
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
func (p *RenderPassEncoder) Draw(args gputypes.DrawArgs) {
	p.r.Draw(args.VertexCount, args.InstanceCount, args.FirstVertex, args.FirstInstance)
}

// DrawIndexed draws indexed primitives.
func (p *RenderPassEncoder) DrawIndexed(args gputypes.DrawIndexedArgs) {
	p.r.DrawIndexed(args.IndexCount, args.InstanceCount, args.FirstIndex, args.BaseVertex, args.FirstInstance)
}

// DrawIndirect draws primitives with GPU-generated parameters.
func (p *RenderPassEncoder) DrawIndirect(buffer *resource.Buffer, offset uint64) {
	p.MultiDrawIndirect(buffer, offset, 1)
}

// MultiDrawIndirect draws consecutive primitives with GPU-generated parameters.
func (p *RenderPassEncoder) MultiDrawIndirect(buffer *resource.Buffer, offset uint64, drawCount uint32) {
	if drawCount == 0 {
		return
	}
	if buffer == nil || buffer.R() == nil {
		return
	}
	if !indirect.RangeFits(buffer.Size(), offset, indirect.DrawIndirectRecordSize, drawCount) {
		p.r.DrawIndirect(buffer.R(), indirect.DelegatedValidationOffset(buffer.Size(), offset, indirect.DrawIndirectRecordSize, drawCount))
		return
	}
	for i := uint32(0); i < drawCount; i++ {
		recordOffset, _ := indirect.RecordOffset(offset, indirect.DrawIndirectRecordSize, i)
		p.r.DrawIndirect(buffer.R(), recordOffset)
	}
}

// DrawIndexedIndirect draws indexed primitives with GPU-generated parameters.
func (p *RenderPassEncoder) DrawIndexedIndirect(buffer *resource.Buffer, offset uint64) {
	p.MultiDrawIndexedIndirect(buffer, offset, 1)
}

// MultiDrawIndexedIndirect draws consecutive indexed primitives with
// GPU-generated parameters.
func (p *RenderPassEncoder) MultiDrawIndexedIndirect(buffer *resource.Buffer, offset uint64, drawCount uint32) {
	if drawCount == 0 {
		return
	}
	if buffer == nil || buffer.R() == nil {
		return
	}
	lowerRustIndexedIndirect(buffer.Size(), offset, drawCount, func(recordOffset uint64) {
		p.r.DrawIndexedIndirect(buffer.R(), recordOffset)
	})
}

// End ends the render pass.
func (p *RenderPassEncoder) End() error {
	if p.released {
		return core.ErrReleased
	}
	p.released = true
	p.r.End()
	return nil
}
