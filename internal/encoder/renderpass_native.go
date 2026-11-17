//go:build !rust && !(js && wasm)

package encoder

import (
	"errors"
	"fmt"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/indirect"
	"github.com/gogpu/wgpu/internal/pipeline"
	"github.com/gogpu/wgpu/internal/resource"
)

// RenderPassEncoder records draw commands within a render pass.
//
// Created by CommandEncoder.BeginRenderPass().
// Must be ended with End() before the CommandEncoder can be finished.
//
// NOT thread-safe.
//
// The type lived in the root package until ADR-070 S5; its signatures name
// the domain's handle types (resource.Buffer, pipeline.RenderPipeline,
// resource.BindGroup), and the root declaration layer converts at the call
// boundary.
//
// The S5 split of the bundle path was executed at S6 (2026-11-16): RenderBundle
// moved into this package, so the whole path is this package's — the
// conformance checks, the usage merge, the resource tracking, the replay and
// the pass-state reset live in renderbundle_execute.go against this package's
// own RenderBundle, and the root wrapper delegates. The reason the S5 fix
// 56e4471 pinned holds in both directions: a method — here a check or a
// replay — cannot stay behind the type whose resolved state it reads.
type RenderPassEncoder struct {
	core    *core.CoreRenderPassEncoder
	encoder *CommandEncoder

	// test, when non-nil, records the public calls for package wgputest;
	// testPass is this pass's dense index.
	test     core.TestHooks
	testPass int
	// pipelineSet tracks whether SetPipeline has been called.
	// Draw commands require a pipeline to be set first.
	pipelineSet bool
	// binder holds the bind groups set on the pass, validates them at draw
	// time and binds them to the backend before each draw, matching Rust
	// wgpu-core's Binder.
	binder Binder
	// vertexBufferCount tracks the highest vertex buffer slot set + 1.
	// Updated by SetVertexBuffer; validated against pipeline requirements at draw time.
	vertexBufferCount uint32
	// requiredVertexBuffers is the number of vertex buffers required by the
	// current pipeline. Set by SetPipeline from RenderPipeline.requiredVertexBuffers.
	requiredVertexBuffers uint32
	// indexBufferSet tracks whether SetIndexBuffer has been called.
	// DrawIndexed and DrawIndexedIndirect require an index buffer.
	indexBufferSet bool
	// indexBufferFormat stores the format passed to the most recent SetIndexBuffer call.
	// Used to validate against the pipeline's StripIndexFormat at DrawIndexed/DrawIndexedIndirect time.
	// Matches Rust wgpu-core State.index.buffer_format (render.rs:568-582).
	indexBufferFormat gputypes.IndexFormat
	// currentStripIndexFormat stores the pipeline's StripIndexFormat for draw-time validation.
	// Set by SetPipeline from RenderPipeline.stripIndexFormat.
	currentStripIndexFormat *gputypes.IndexFormat
	// blendConstantRequired is true if the current pipeline uses
	// BlendFactorConstant or BlendFactorOneMinusConstant.
	// Set by SetPipeline from RenderPipeline.blendConstantRequired.
	blendConstantRequired bool
	// blendConstantSet tracks whether SetBlendConstant has been called.
	// Matches Rust wgpu-core OptionalState for blend_constant.
	blendConstantSet bool
	// depthReadOnly and stencilReadOnly are the read-only flags of the
	// depth/stencil attachment; a bundle executed in the pass must be
	// read-only where the pass is (Rust RenderPassInfo.is_depth_read_only).
	depthReadOnly   bool
	stencilReadOnly bool
}

// NewRenderPassEncoder builds the pass encoder BeginRenderPass describes. The
// root declaration layer calls it with the start value and the encoder; the
// pass holds the read-only flags and the core pass the bundle checks read.
func NewRenderPassEncoder(start *RenderPassStart, e *CommandEncoder) *RenderPassEncoder {
	return &RenderPassEncoder{
		core:            start.Core,
		encoder:         e,
		test:            start.Test,
		testPass:        start.TestPass,
		depthReadOnly:   start.DepthReadOnly,
		stencilReadOnly: start.StencilReadOnly,
	}
}

// trackRef Clone()'s a ResourceRef and appends directly to the parent
// CommandEncoder's trackedRefs. No intermediate pass-level slice; the clone
// happens once, in the encoder's own TrackRef.
func (p *RenderPassEncoder) trackRef(ref *core.ResourceRef) {
	p.encoder.TrackRef(ref)
}

// SetPipeline sets the active render pipeline.
//
// A nil pipeline is the zero-value public pipeline's implementation: the reads
// below are nil-safe and yield the zero values the pre-move type carried. The
// public wrapper reports the nil-pipeline argument error itself, before the
// call, exactly as the pre-move method did; this method is the state update
// that follows a non-nil pipeline.
func (p *RenderPassEncoder) SetPipeline(pl *pipeline.RenderPipeline) {
	p.pipelineSet = true
	p.requiredVertexBuffers = pl.RequiredVertexBuffers()
	p.currentStripIndexFormat = pl.StripIndexFormat()
	if pl.BlendConstantRequired() {
		p.blendConstantRequired = true
	}
	p.binder.UpdateExpectations(pl.BindGroupLayouts())
	p.binder.UpdateLateBufferBindingsFromPipeline(pl.LateSizedBufferGroups())
	p.trackRef(pl.Ref())
	raw := p.core.RawPass()
	if raw != nil && pl.HAL() != nil {
		raw.SetPipeline(pl.HAL())
	}
}

// SetBindGroup sets a bind group for the given index. It may come before
// SetPipeline, or name an index the current pipeline does not use: the group
// is bound to the backend at the next draw whose pipeline layout uses it.
func (p *RenderPassEncoder) SetBindGroup(index uint32, group *resource.BindGroup, offsets []uint32) {
	// The core pass owns the device that created it: the limits are read from
	// there, not through the root device wrapper, so the read needs no seam
	// method after the move into internal/encoder (ADR-070 S5).
	limits := &p.core.Device().Limits
	if group == nil {
		p.encoder.SetError(fmt.Errorf("wgpu: RenderPass.SetBindGroup: bind group is nil"))
		return
	}
	if err := ValidateSetBindGroup("RenderPass", index, group, offsets, limits); err != nil {
		p.encoder.SetError(err)
		return
	}
	p.binder.SetGroup(index, group, offsets)
	p.trackBindGroup(group)
	// A render pass is one usage scope: every bind group set in it counts.
	if err := p.core.MergeBindGroup(group.Usages()); err != nil {
		p.encoder.SetError(fmt.Errorf("wgpu: RenderPass.SetBindGroup: index %d: %w", index, err))
	}
	if p.test != nil {
		p.test.BindGroupBound(p.testPass, index, offsets)
	}
}

// trackBindGroup keeps a bind group used in the pass and its buffers alive
// until the GPU completes the submission, and records the group for
// submit-time validation.
func (p *RenderPassEncoder) trackBindGroup(group *resource.BindGroup) {
	p.trackRef(group.Ref())
	// Clone individual buffer ResourceRefs to keep them alive until GPU completes.
	// Matches Rust wgpu merge_bind_group → ResourceMetadata.insert(Arc<Buffer>).
	// Without this, Buffer.Release() before Submit can HAL-destroy buffers
	// that are still referenced by a pending command buffer.
	for _, buf := range group.BoundBuffers() {
		if cb := buf.CoreBuffer(); cb != nil && cb.Ref != nil {
			p.trackRef(cb.Ref)
		}
	}
	// Track bind group itself for submit-time validation (VAL-B5 and, via
	// group.boundBuffers/boundTextures, VAL-A6). The group's own slices are
	// walked at Submit, so there is nothing to fan out here.
	p.encoder.TrackBindGroup(group)
}

// flushBindGroups binds the bind groups whose backend binding is stale, right
// before a draw that passed validation (Rust wgpu-core State::flush_bindings).
func (p *RenderPassEncoder) flushBindGroups() {
	if raw := p.core.RawPass(); raw != nil {
		p.binder.Flush(raw)
	}
}

// SetVertexBuffer sets a vertex buffer for the given slot.
func (p *RenderPassEncoder) SetVertexBuffer(slot uint32, buffer *resource.Buffer, offset uint64) {
	if buffer == nil {
		p.encoder.SetError(fmt.Errorf("wgpu: RenderPass.SetVertexBuffer: buffer is nil"))
		return
	}
	if slot+1 > p.vertexBufferCount {
		p.vertexBufferCount = slot + 1
	}
	p.trackRef(buffer.CoreBuffer().Ref)
	p.encoder.TrackBuffer(buffer)
	p.core.SetVertexBuffer(slot, buffer.CoreBuffer(), offset)
	if p.test != nil {
		p.test.VertexBufferBound(p.testPass, slot, buffer.Label(), offset, buffer.Size())
	}
}

// SetIndexBuffer sets the index buffer.
func (p *RenderPassEncoder) SetIndexBuffer(buffer *resource.Buffer, format gputypes.IndexFormat, offset uint64) {
	if buffer == nil {
		p.encoder.SetError(fmt.Errorf("wgpu: RenderPass.SetIndexBuffer: buffer is nil"))
		return
	}
	p.indexBufferSet = true
	p.indexBufferFormat = format
	p.trackRef(buffer.CoreBuffer().Ref)
	p.encoder.TrackBuffer(buffer)
	p.core.SetIndexBuffer(buffer.CoreBuffer(), format, offset)
	if p.test != nil {
		p.test.IndexBufferBound(p.testPass, buffer.Label(), format, offset, buffer.Size())
	}
}

// SetViewport sets the viewport transformation.
func (p *RenderPassEncoder) SetViewport(vp gputypes.Viewport) {
	p.core.SetViewport(vp)
}

// SetScissorRect sets the scissor rectangle for clipping.
func (p *RenderPassEncoder) SetScissorRect(rect gputypes.ScissorRect) {
	p.core.SetScissorRect(rect)
}

// SetBlendConstant sets the blend constant color.
func (p *RenderPassEncoder) SetBlendConstant(color *gputypes.Color) {
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
//
// Each validation failure wraps a typed sentinel error so that callers can
// use errors.Is() to identify the failure category programmatically.
// Matches Rust wgpu-core State::is_ready (command/render.rs:542-593).
func (p *RenderPassEncoder) validateDrawState(method string) bool {
	if !p.pipelineSet {
		p.encoder.SetError(fmt.Errorf("wgpu: RenderPass.%s: no pipeline set (call SetPipeline first): %w",
			method, core.ErrDrawMissingPipeline))
		return false
	}
	if err := p.binder.CheckCompatibility(); err != nil {
		// Wrap the binder error with the appropriate draw-time sentinel
		// so errors.Is works for both the specific binder cause and the
		// public draw-time category.
		sentinel := core.ErrDrawMissingBindGroup
		if errors.Is(err, ErrBindGroupIncompatible) {
			sentinel = core.ErrDrawIncompatibleBindGroup
		}
		p.encoder.SetError(fmt.Errorf("wgpu: RenderPass.%s: %w: %w", method, sentinel, err))
		return false
	}
	if p.vertexBufferCount < p.requiredVertexBuffers {
		p.encoder.SetError(fmt.Errorf(
			"wgpu: RenderPass.%s: pipeline requires %d vertex buffer(s) but only %d set: %w",
			method, p.requiredVertexBuffers, p.vertexBufferCount,
			core.ErrDrawMissingVertexBuffer,
		))
		return false
	}
	if p.blendConstantRequired && !p.blendConstantSet {
		p.encoder.SetError(fmt.Errorf(
			"wgpu: RenderPass.%s: %w",
			method, core.ErrDrawMissingBlendConstant,
		))
		return false
	}
	// Late buffer binding size validation: check that bound buffers are large enough
	// for bindings with MinBindingSize == 0. Matches Rust wgpu-core's is_ready()
	// call to check_late_buffer_bindings before draw (render.rs:542-545).
	if err := p.binder.CheckLateBufferBindings(); err != nil {
		p.encoder.SetError(fmt.Errorf("wgpu: RenderPass.%s: %w: %w", method, core.ErrDrawLateBufferTooSmall, err))
		return false
	}
	return true
}

// Draw draws primitives.
func (p *RenderPassEncoder) Draw(args gputypes.DrawArgs) {
	if !p.validateDrawState("Draw") {
		return
	}
	// firstInstance is accepted unconditionally: "indirect-first-instance" only governs
	// the firstInstance field of INDIRECT draw args (W3C spec, drawIndirect), and
	// Rust wgpu-core command/render.rs only runs validate_instance_limit here.
	p.flushBindGroups()
	p.core.Draw(args)
	if p.test != nil {
		p.test.Drew(p.testPass, args)
	}
}

// DrawIndexed draws indexed primitives.
func (p *RenderPassEncoder) DrawIndexed(args gputypes.DrawIndexedArgs) {
	if !p.validateDrawState("DrawIndexed") {
		return
	}
	if !p.indexBufferSet {
		p.encoder.SetError(fmt.Errorf("wgpu: RenderPass.DrawIndexed: no index buffer set (call SetIndexBuffer first): %w",
			core.ErrDrawMissingIndexBuffer))
		return
	}
	// VAL-B2: Validate index format matches pipeline's strip index format.
	// Matches Rust wgpu-core render.rs:568-582 (UnmatchedIndexFormats).
	if p.currentStripIndexFormat != nil && p.indexBufferFormat != *p.currentStripIndexFormat {
		p.encoder.SetError(fmt.Errorf(
			"wgpu: RenderPass.DrawIndexed: index buffer format %v does not match pipeline strip index format %v: %w",
			p.indexBufferFormat, *p.currentStripIndexFormat, core.ErrDrawIndexFormatMismatch))
		return
	}
	// firstInstance needs no feature gate (see Draw).
	p.flushBindGroups()
	p.core.DrawIndexed(args)
	if p.test != nil {
		p.test.DrewIndexed(p.testPass, args)
	}
}

// DrawIndirect draws primitives with GPU-generated parameters.
func (p *RenderPassEncoder) DrawIndirect(buffer *resource.Buffer, offset uint64) {
	p.MultiDrawIndirect(buffer, offset, 1)
}

// MultiDrawIndirect draws consecutive primitives with GPU-generated parameters.
// Each argument record is 16 bytes.
func (p *RenderPassEncoder) MultiDrawIndirect(buffer *resource.Buffer, offset uint64, drawCount uint32) {
	if drawCount == 0 {
		return
	}
	if !p.validateDrawState("DrawIndirect") {
		return
	}
	// VAL-C1: Multi-draw indirect requires FeatureMultiDrawIndirect when drawCount > 1.
	// Reference: wgpu-core command/render.rs multi-draw feature check.
	if drawCount > 1 {
		if err := core.RequireFeature(
			p.encoder.device.Features(),
			gputypes.FeatureMultiDrawIndirect,
			"MultiDrawIndirect",
		); err != nil {
			p.encoder.SetError(err)
			return
		}
	}
	if buffer == nil {
		p.encoder.SetError(fmt.Errorf("wgpu: RenderPass.DrawIndirect: buffer is nil"))
		return
	}
	// VAL-B3: Validate indirect buffer has INDIRECT usage.
	// Matches Rust wgpu-core render.rs:2763 (check_usage(BufferUsages::INDIRECT)).
	if buffer.Usage()&gputypes.BufferUsageIndirect == 0 {
		p.encoder.SetError(fmt.Errorf(
			"wgpu: RenderPass.DrawIndirect: buffer %q missing BufferUsageIndirect usage: %w",
			buffer.Label(), core.ErrDrawIndirectBufferUsage))
		return
	}
	// VAL-B3: Validate indirect buffer offset is 4-byte aligned.
	// Matches Rust wgpu-core render.rs:2766 (offset % 4 != 0).
	if offset%4 != 0 {
		p.encoder.SetError(fmt.Errorf(
			"wgpu: RenderPass.DrawIndirect: offset %d is not 4-byte aligned: %w",
			offset, core.ErrDrawIndirectOffsetAlignment))
		return
	}
	// VAL-B3: Validate indirect args fit within buffer.
	// DrawIndirect args: 4 × uint32 = 16 bytes. Matches Rust render.rs:2772-2779.
	if !indirect.RangeFits(buffer.Size(), offset, indirect.DrawIndirectRecordSize, drawCount) {
		p.encoder.SetError(fmt.Errorf(
			"wgpu: RenderPass.MultiDrawIndirect: offset %d + %d draw(s) exceeds buffer size %d: %w",
			offset, drawCount, buffer.Size(), core.ErrDrawIndirectBufferOverrun))
		return
	}
	p.trackRef(buffer.CoreBuffer().Ref)
	p.encoder.TrackBuffer(buffer)
	p.flushBindGroups()
	p.core.MultiDrawIndirect(buffer.CoreBuffer(), offset, drawCount)
}

// DrawIndexedIndirect draws indexed primitives with GPU-generated parameters.
func (p *RenderPassEncoder) DrawIndexedIndirect(buffer *resource.Buffer, offset uint64) {
	p.MultiDrawIndexedIndirect(buffer, offset, 1)
}

// MultiDrawIndexedIndirect draws consecutive indexed primitives with
// GPU-generated parameters. Each argument record is 20 bytes.
func (p *RenderPassEncoder) MultiDrawIndexedIndirect(buffer *resource.Buffer, offset uint64, drawCount uint32) {
	if drawCount == 0 {
		return
	}
	if !p.validateDrawState("DrawIndexedIndirect") {
		return
	}
	// VAL-C1: Multi-draw indexed indirect requires FeatureMultiDrawIndirect when drawCount > 1.
	// Reference: wgpu-core command/render.rs multi-draw feature check.
	if drawCount > 1 {
		if err := core.RequireFeature(
			p.encoder.device.Features(),
			gputypes.FeatureMultiDrawIndirect,
			"MultiDrawIndexedIndirect",
		); err != nil {
			p.encoder.SetError(err)
			return
		}
	}
	if !p.indexBufferSet {
		p.encoder.SetError(fmt.Errorf("wgpu: RenderPass.DrawIndexedIndirect: no index buffer set (call SetIndexBuffer first): %w",
			core.ErrDrawMissingIndexBuffer))
		return
	}
	// VAL-B2: Validate index format matches pipeline's strip index format.
	// Matches Rust wgpu-core render.rs:568-582 (UnmatchedIndexFormats).
	if p.currentStripIndexFormat != nil && p.indexBufferFormat != *p.currentStripIndexFormat {
		p.encoder.SetError(fmt.Errorf(
			"wgpu: RenderPass.DrawIndexedIndirect: index buffer format %v does not match pipeline strip index format %v: %w",
			p.indexBufferFormat, *p.currentStripIndexFormat, core.ErrDrawIndexFormatMismatch))
		return
	}
	if buffer == nil {
		p.encoder.SetError(fmt.Errorf("wgpu: RenderPass.DrawIndexedIndirect: buffer is nil"))
		return
	}
	// VAL-B3: Validate indirect buffer has INDIRECT usage.
	// Matches Rust wgpu-core render.rs:2763 (check_usage(BufferUsages::INDIRECT)).
	if buffer.Usage()&gputypes.BufferUsageIndirect == 0 {
		p.encoder.SetError(fmt.Errorf(
			"wgpu: RenderPass.DrawIndexedIndirect: buffer %q missing BufferUsageIndirect usage: %w",
			buffer.Label(), core.ErrDrawIndirectBufferUsage))
		return
	}
	// VAL-B3: Validate indirect buffer offset is 4-byte aligned.
	// Matches Rust wgpu-core render.rs:2766 (offset % 4 != 0).
	if offset%4 != 0 {
		p.encoder.SetError(fmt.Errorf(
			"wgpu: RenderPass.DrawIndexedIndirect: offset %d is not 4-byte aligned: %w",
			offset, core.ErrDrawIndirectOffsetAlignment))
		return
	}
	// VAL-B3: Validate all indirect args fit within the buffer without allowing
	// offset+drawCount*20 to wrap around uint64.
	if !indirect.RangeFits(buffer.Size(), offset, indirect.DrawIndexedIndirectRecordSize, drawCount) {
		p.encoder.SetError(fmt.Errorf(
			"wgpu: RenderPass.MultiDrawIndexedIndirect: offset %d + %d draw(s) exceeds buffer size %d: %w",
			offset, drawCount, buffer.Size(), core.ErrDrawIndirectBufferOverrun))
		return
	}
	p.trackRef(buffer.CoreBuffer().Ref)
	p.encoder.TrackBuffer(buffer)
	p.flushBindGroups()
	p.core.MultiDrawIndexedIndirect(buffer.CoreBuffer(), offset, drawCount)
}

// MultiDrawIndirectCount draws primitives with a GPU-provided draw count.
// countBuffer must hold a uint32 count at countOffset. VAL-C2: requires FeatureMultiDrawIndirectCount.
func (p *RenderPassEncoder) MultiDrawIndirectCount(
	indirectBuffer *resource.Buffer, indirectOffset uint64,
	countBuffer *resource.Buffer, countOffset uint64, maxDrawCount uint32,
) {
	p.executeIndirectCountDraw(indirectCountDrawConfig{
		validateDrawOp:  "DrawIndirect",
		featureResource: "MultiDrawIndirectCount",
		indirectBuffer:  indirectBuffer,
		indirectOffset:  indirectOffset,
		countBuffer:     countBuffer,
		countOffset:     countOffset,
		maxDrawCount:    maxDrawCount,
		recordStride:    indirect.DrawIndirectRecordSize,
		record: func() {
			p.flushBindGroups()
			p.core.DrawIndirectCount(
				indirectBuffer.CoreBuffer(), indirectOffset,
				countBuffer.CoreBuffer(), countOffset, maxDrawCount,
			)
		},
	})
}

// MultiDrawIndexedIndirectCount draws indexed primitives with a GPU-provided draw count.
func (p *RenderPassEncoder) MultiDrawIndexedIndirectCount(
	indirectBuffer *resource.Buffer, indirectOffset uint64,
	countBuffer *resource.Buffer, countOffset uint64, maxDrawCount uint32,
) {
	p.executeIndirectCountDraw(indirectCountDrawConfig{
		validateDrawOp:  "DrawIndexedIndirect",
		featureResource: "MultiDrawIndexedIndirectCount",
		indirectBuffer:  indirectBuffer,
		indirectOffset:  indirectOffset,
		countBuffer:     countBuffer,
		countOffset:     countOffset,
		maxDrawCount:    maxDrawCount,
		recordStride:    indirect.DrawIndexedIndirectRecordSize,
		preValidate:     p.validateIndexedIndirectCountPreconditions,
		record: func() {
			p.flushBindGroups()
			p.core.DrawIndexedIndirectCount(
				indirectBuffer.CoreBuffer(), indirectOffset,
				countBuffer.CoreBuffer(), countOffset, maxDrawCount,
			)
		},
	})
}

func (p *RenderPassEncoder) validateIndirectCountBuffers(
	indirectBuffer *resource.Buffer, indirectOffset uint64,
	countBuffer *resource.Buffer, countOffset uint64,
	maxDrawCount uint32, recordStride uint64,
) error {
	if indirectBuffer == nil {
		return fmt.Errorf("wgpu: RenderPass.DrawIndirect: indirect buffer is nil")
	}
	if countBuffer == nil {
		return fmt.Errorf("wgpu: RenderPass.DrawIndirect: count buffer is nil")
	}
	if indirectBuffer.Usage()&gputypes.BufferUsageIndirect == 0 {
		return fmt.Errorf(
			"wgpu: RenderPass.DrawIndirect: buffer %q missing BufferUsageIndirect usage: %w",
			indirectBuffer.Label(), core.ErrDrawIndirectBufferUsage)
	}
	if countBuffer.Usage()&gputypes.BufferUsageIndirect == 0 {
		return fmt.Errorf(
			"wgpu: RenderPass.DrawIndirect: count buffer %q missing BufferUsageIndirect usage: %w",
			countBuffer.Label(), core.ErrDrawIndirectBufferUsage)
	}
	if indirectOffset%4 != 0 {
		return fmt.Errorf(
			"wgpu: RenderPass.DrawIndirect: offset %d is not 4-byte aligned: %w",
			indirectOffset, core.ErrDrawIndirectOffsetAlignment)
	}
	if countOffset%4 != 0 {
		return fmt.Errorf(
			"wgpu: RenderPass.DrawIndirect: count offset %d is not 4-byte aligned: %w",
			countOffset, core.ErrDrawIndirectOffsetAlignment)
	}
	if countOffset+4 > countBuffer.Size() {
		return fmt.Errorf(
			"wgpu: RenderPass.DrawIndirect: count offset %d + 4 exceeds count buffer size %d: %w",
			countOffset, countBuffer.Size(), core.ErrDrawIndirectBufferOverrun)
	}
	if recordStride == indirect.DrawIndirectRecordSize {
		if !indirect.RangeFits(indirectBuffer.Size(), indirectOffset, indirect.DrawIndirectRecordSize, maxDrawCount) {
			return fmt.Errorf(
				"wgpu: RenderPass.MultiDrawIndirectCount: offset %d + max %d draw(s) exceeds buffer size %d: %w",
				indirectOffset, maxDrawCount, indirectBuffer.Size(), core.ErrDrawIndirectBufferOverrun)
		}
	} else if !indirect.RangeFits(indirectBuffer.Size(), indirectOffset, indirect.DrawIndexedIndirectRecordSize, maxDrawCount) {
		return fmt.Errorf(
			"wgpu: RenderPass.MultiDrawIndexedIndirectCount: offset %d + max %d draw(s) exceeds buffer size %d: %w",
			indirectOffset, maxDrawCount, indirectBuffer.Size(), core.ErrDrawIndirectBufferOverrun)
	}
	return nil
}

// End ends the render pass.
// After this call, the encoder cannot be used again.
func (p *RenderPassEncoder) End() error {
	return p.core.End()
}

// resetBundleState clears the pipeline, bind groups, vertex buffers and
// index buffer after ExecuteBundles: a bundle leaves the backend with its own
// state, and WebGPU requires them to be set again (executeBundles; Rust
// wgpu-core State::reset_bundle). Without it the binder would take the
// backend's bindings for current.
//
// ExecuteBundles calls it once after its bundle loop, exactly where the
// pre-move method did — including after an error break.
func (p *RenderPassEncoder) resetBundleState() {
	p.binder.reset()
	p.pipelineSet = false
	p.requiredVertexBuffers = 0
	p.currentStripIndexFormat = nil
	p.vertexBufferCount = 0
	p.indexBufferSet = false
}

// SetPassStateForTest seeds the pass state the root package's white-box test
// of ExecuteBundles asserts on: a pipeline is considered set and the binder
// holds one group at slot 0 with its expected layout. Test support only.
func (p *RenderPassEncoder) SetPassStateForTest(layout *resource.BindGroupLayout, group *resource.BindGroup) {
	p.pipelineSet = true
	p.binder.UpdateExpectations([]*resource.BindGroupLayout{layout})
	p.binder.SetGroup(0, group, nil)
}

// PassStateForTest reports the pipeline flag, the active group at slot 0 and
// the layout assigned at slot 0, the three fields ExecuteBundles resets. Test
// support only.
func (p *RenderPassEncoder) PassStateForTest() (pipelineSet bool, activeGroup *resource.BindGroup, assignedLayout *resource.BindGroupLayout) {
	return p.pipelineSet, p.binder.ActiveGroup(0), p.binder.assigned[0]
}
