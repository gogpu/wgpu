//go:build js && wasm

package encoder

import (
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/browser"
	"github.com/gogpu/wgpu/internal/indirect"
	"github.com/gogpu/wgpu/internal/pipeline"
	"github.com/gogpu/wgpu/internal/resource"
)

// RenderPassEncoder records draw commands within a render pass.
// On browser, this wraps a GPURenderPassEncoder via internal/browser.RenderPassEncoder.
//
// The type lived in the root package until ADR-070 S5; its signatures name
// the domain's handle types, and the root declaration layer converts at the
// call boundary. The nil-argument guards here carry the contract alone: the
// public wrapper delegates unconditionally, and a zero-value public pass
// (nil implementation) is answered by these guards returning — the same shape
// the pre-move zero value had (one check, one place).
type RenderPassEncoder struct {
	browser  *browser.RenderPassEncoder
	released bool
}

// SetPipeline sets the active render pipeline.
func (p *RenderPassEncoder) SetPipeline(pl *pipeline.RenderPipeline) {
	if pl == nil || pl.Browser() == nil {
		return
	}
	p.browser.SetPipeline(pl.Browser().Ref())
}

// SetBindGroup sets a bind group for the given index.
func (p *RenderPassEncoder) SetBindGroup(index uint32, group *resource.BindGroup, offsets []uint32) {
	if group == nil || group.Browser() == nil {
		return
	}
	p.browser.SetBindGroup(index, group.Browser().Ref(), offsets)
}

// SetVertexBuffer sets a vertex buffer for the given slot.
// Offset is in bytes. Pass 0 for size to use the rest of the buffer.
func (p *RenderPassEncoder) SetVertexBuffer(slot uint32, buffer *resource.Buffer, offset uint64) {
	if buffer == nil || buffer.Browser() == nil {
		return
	}
	// size=0 tells the browser layer to omit the size parameter,
	// which means "rest of buffer" per the WebGPU spec.
	p.browser.SetVertexBuffer(slot, buffer.Browser().Ref(), offset, 0)
}

// SetIndexBuffer sets the index buffer.
func (p *RenderPassEncoder) SetIndexBuffer(buffer *resource.Buffer, format gputypes.IndexFormat, offset uint64) {
	if buffer == nil || buffer.Browser() == nil {
		return
	}
	formatStr := browser.IndexFormatToJS(format)
	// size=0 tells the browser layer to omit the size parameter.
	p.browser.SetIndexBuffer(buffer.Browser().Ref(), formatStr, offset, 0)
}

// SetViewport sets the viewport transformation.
func (p *RenderPassEncoder) SetViewport(vp gputypes.Viewport) {
	p.browser.SetViewport(vp.X, vp.Y, vp.Width, vp.Height, vp.MinDepth, vp.MaxDepth)
}

// SetScissorRect sets the scissor rectangle for clipping.
func (p *RenderPassEncoder) SetScissorRect(rect gputypes.ScissorRect) {
	p.browser.SetScissorRect(rect.X, rect.Y, rect.Width, rect.Height)
}

// SetBlendConstant sets the blend constant color.
func (p *RenderPassEncoder) SetBlendConstant(color *gputypes.Color) {
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
func (p *RenderPassEncoder) Draw(args gputypes.DrawArgs) {
	p.browser.Draw(args.VertexCount, args.InstanceCount, args.FirstVertex, args.FirstInstance)
}

// DrawIndexed draws indexed primitives.
func (p *RenderPassEncoder) DrawIndexed(args gputypes.DrawIndexedArgs) {
	p.browser.DrawIndexed(args.IndexCount, args.InstanceCount, args.FirstIndex, args.BaseVertex, args.FirstInstance)
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
	if buffer == nil || buffer.Browser() == nil {
		return
	}
	if !indirect.RangeFits(buffer.Size(), offset, indirect.DrawIndirectRecordSize, drawCount) {
		p.browser.DrawIndirect(buffer.Browser().Ref(), indirect.DelegatedValidationOffset(buffer.Size(), offset, indirect.DrawIndirectRecordSize, drawCount))
		return
	}
	for i := uint32(0); i < drawCount; i++ {
		recordOffset, _ := indirect.RecordOffset(offset, indirect.DrawIndirectRecordSize, i)
		p.browser.DrawIndirect(buffer.Browser().Ref(), recordOffset)
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
	if buffer == nil || buffer.Browser() == nil {
		return
	}
	if !indirect.RangeFits(buffer.Size(), offset, indirect.DrawIndexedIndirectRecordSize, drawCount) {
		p.browser.DrawIndexedIndirect(buffer.Browser().Ref(), indirect.DelegatedValidationOffset(buffer.Size(), offset, indirect.DrawIndexedIndirectRecordSize, drawCount))
		return
	}
	for i := uint32(0); i < drawCount; i++ {
		recordOffset, _ := indirect.RecordOffset(offset, indirect.DrawIndexedIndirectRecordSize, i)
		p.browser.DrawIndexedIndirect(buffer.Browser().Ref(), recordOffset)
	}
}

// End ends the render pass.
func (p *RenderPassEncoder) End() error {
	if p.released {
		return resource.ErrReleased
	}
	p.released = true
	p.browser.End()
	return nil
}
