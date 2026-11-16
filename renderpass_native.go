//go:build !rust && !(js && wasm)

package wgpu

import (
	"fmt"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/encoder"
)

// RenderPassEncoder records draw commands within a render pass.
//
// Created by CommandEncoder.BeginRenderPass().
// Must be ended with End() before the CommandEncoder can be finished.
//
// NOT thread-safe.
//
// The implementation lives in internal/encoder (ADR-070 S5); this declaration
// is the public surface: every method is one delegation, and the wrappers are
// converted to the implementation's handle types at the call boundary. The
// bundle methods stay here: they name the public RenderBundle type, which
// stays in this package through S6, so the destination gets ReplayBundle and
// ResetBundleState instead.
type RenderPassEncoder struct {
	impl *encoder.RenderPassEncoder

	// core is the same core pass the implementation holds: the bundle checks
	// read the pass context through it, exactly as the pre-move pass did.
	core *core.CoreRenderPassEncoder
	// encoder is the encoder that began the pass: bundle errors are recorded
	// there, and the bundle device check compares its device.
	encoder *CommandEncoder
	// depthReadOnly and stencilReadOnly mirror the attachment flags: a bundle
	// executed in the pass must be read-only where the pass is.
	depthReadOnly   bool
	stencilReadOnly bool
}

// newRenderPassEncoder builds the public pass over the destination's. The
// start value carries the core pass, the wgputest hooks and the read-only
// flags — the data the pre-move constructor read.
func newRenderPassEncoder(start *encoder.RenderPassStart, e *CommandEncoder) *RenderPassEncoder {
	return &RenderPassEncoder{
		impl:            encoder.NewRenderPassEncoder(start, e.impl),
		core:            start.Core,
		encoder:         e,
		depthReadOnly:   start.DepthReadOnly,
		stencilReadOnly: start.StencilReadOnly,
	}
}

// SetPipeline sets the active render pipeline.
func (p *RenderPassEncoder) SetPipeline(pipeline *RenderPipeline) {
	if pipeline == nil {
		p.encoder.setError(fmt.Errorf("wgpu: RenderPass.SetPipeline: pipeline is nil"))
		return
	}
	p.impl.SetPipeline(encoderPipeline(pipeline))
}

// SetBindGroup sets a bind group for the given index. It may come before
// SetPipeline, or name an index the current pipeline does not use: the group
// is bound to the backend at the next draw whose pipeline layout uses it.
func (p *RenderPassEncoder) SetBindGroup(index uint32, group *BindGroup, offsets []uint32) {
	p.impl.SetBindGroup(index, encoderBindGroup(group), offsets)
}

// SetVertexBuffer sets a vertex buffer for the given slot.
func (p *RenderPassEncoder) SetVertexBuffer(slot uint32, buffer *Buffer, offset uint64) {
	p.impl.SetVertexBuffer(slot, encoderBuffer(buffer), offset)
}

// SetIndexBuffer sets the index buffer.
func (p *RenderPassEncoder) SetIndexBuffer(buffer *Buffer, format gputypes.IndexFormat, offset uint64) {
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
// Each argument record is 16 bytes.
func (p *RenderPassEncoder) MultiDrawIndirect(buffer *Buffer, offset uint64, drawCount uint32) {
	p.impl.MultiDrawIndirect(encoderBuffer(buffer), offset, drawCount)
}

// DrawIndexedIndirect draws indexed primitives with GPU-generated parameters.
func (p *RenderPassEncoder) DrawIndexedIndirect(buffer *Buffer, offset uint64) {
	p.impl.DrawIndexedIndirect(encoderBuffer(buffer), offset)
}

// MultiDrawIndexedIndirect draws consecutive indexed primitives with
// GPU-generated parameters. Each argument record is 20 bytes.
func (p *RenderPassEncoder) MultiDrawIndexedIndirect(buffer *Buffer, offset uint64, drawCount uint32) {
	p.impl.MultiDrawIndexedIndirect(encoderBuffer(buffer), offset, drawCount)
}

// MultiDrawIndirectCount draws primitives with a GPU-provided draw count.
// countBuffer must hold a uint32 count at countOffset.
func (p *RenderPassEncoder) MultiDrawIndirectCount(
	indirectBuffer *Buffer, indirectOffset uint64,
	countBuffer *Buffer, countOffset uint64, maxDrawCount uint32,
) {
	p.impl.MultiDrawIndirectCount(encoderBuffer(indirectBuffer), indirectOffset, encoderBuffer(countBuffer), countOffset, maxDrawCount)
}

// MultiDrawIndexedIndirectCount draws indexed primitives with a GPU-provided draw count.
func (p *RenderPassEncoder) MultiDrawIndexedIndirectCount(
	indirectBuffer *Buffer, indirectOffset uint64,
	countBuffer *Buffer, countOffset uint64, maxDrawCount uint32,
) {
	p.impl.MultiDrawIndexedIndirectCount(encoderBuffer(indirectBuffer), indirectOffset, encoderBuffer(countBuffer), countOffset, maxDrawCount)
}

// End ends the render pass.
// After this call, the encoder cannot be used again.
func (p *RenderPassEncoder) End() error {
	return p.impl.End()
}

// ExecuteBundles executes reusable render command bundles in this render pass.
// Each bundle's commands are replayed into the pass, its resources join the
// pass's usage scope, and the pass is left without a pipeline, bind groups,
// vertex buffers or index buffer: draws after it must set them again.
//
// The method lives on this declaration layer, not in the destination: it names
// *RenderBundle, a type that stays in this package through S6. The destination
// supplies the two bundle-free halves — ReplayBundle (scope merge, the replay
// callback, the pre-move error wrapping) and ResetBundleState.
func (p *RenderPassEncoder) ExecuteBundles(bundles ...*RenderBundle) {
	for _, bundle := range bundles {
		if err := p.executeBundle(bundle); err != nil {
			p.encoder.setError(fmt.Errorf("wgpu: RenderPass.ExecuteBundles: %w", err))
			break
		}
	}
	if len(bundles) > 0 {
		p.impl.ResetBundleState()
	}
}

// executeBundle checks that a bundle fits this pass, merges its usages into
// the pass's usage scope, keeps its resources alive until the GPU completes
// the submission, and replays its commands into the backend pass.
// Matches Rust wgpu-core command/render.rs execute_bundle and
// command/bundle.rs RenderBundle::execute.
func (p *RenderPassEncoder) executeBundle(bundle *RenderBundle) error {
	if err := p.checkBundle(bundle); err != nil {
		return err
	}
	// The scope merge and the replay are the destination's (it holds the core
	// pass); resource tracking stays here, where the wrappers convert without
	// an allocation, in the pre-move order: merge, track, replay.
	return p.impl.ReplayBundle(bundle.scope, bundle.label, func() {
		for _, ref := range bundle.refs {
			p.encoder.trackRef(ref)
		}
		for _, group := range bundle.groups {
			p.encoder.trackBindGroup(group)
		}
		for _, buffer := range bundle.buffers {
			p.encoder.trackBuffer(buffer.impl)
		}
	}, bundle.replay)
}

// checkBundle checks that a bundle can run in this pass: same device, the
// pass's attachment formats and sample count, and read-only where the pass's
// depth/stencil is. Matches the checks of Rust render.rs execute_bundle.
func (p *RenderPassEncoder) checkBundle(bundle *RenderBundle) error {
	if bundle == nil {
		return fmt.Errorf("%w: render bundle is nil", errBundleCommand)
	}
	if bundle.released.Load() {
		return fmt.Errorf("bundle %q: %w", bundle.label, ErrReleased)
	}
	if bundle.bundleContents == nil {
		return fmt.Errorf("%w: render bundle %q was not created by RenderBundleEncoder.Finish", errBundleCommand, bundle.label)
	}
	if bundle.device != p.encoder.device {
		return fmt.Errorf("bundle %q: %w", bundle.label, errBundleDevice)
	}
	if ctx := p.core.PassContext(); ctx != nil {
		if err := ctx.CheckCompatible(&bundle.target.context, bundle.label); err != nil {
			return fmt.Errorf("bundle %q: %w", bundle.label, err)
		}
	}
	if (p.depthReadOnly && !bundle.target.depthReadOnly) || (p.stencilReadOnly && !bundle.target.stencilReadOnly) {
		return fmt.Errorf("bundle %q: %w", bundle.label, errBundleReadOnly)
	}
	return nil
}
