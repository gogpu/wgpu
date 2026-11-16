//go:build !rust && !(js && wasm)

package wgpu

import (
	"fmt"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/core/track"
	"github.com/gogpu/wgpu/internal/hal"
	"github.com/gogpu/wgpu/internal/resource"
)

// CommandEncoder records GPU commands for later submission.
//
// A command encoder is single-use. After calling Finish(), the encoder
// cannot be used again. Call Device.CreateCommandEncoder() to create a new one.
//
// NOT thread-safe - do not use from multiple goroutines.
type CommandEncoder struct {
	core     *core.CoreCommandEncoder
	device   *Device
	released bool

	// test, when non-nil, records the public calls for package wgputest;
	// testEncoder is this user encoder's dense index.
	test        core.TestHooks
	testEncoder int
	// trackedRefs accumulates Clone'd ResourceRefs from render/compute passes.
	// Transferred to the CommandBuffer on Finish(), then to the DestroyQueue
	// on Submit(). Phase 2: per-command-buffer resource tracking.
	trackedRefs []*core.ResourceRef

	// halEncoder is the HAL command encoder acquired from the Device's pool.
	// On Finish(), ownership transfers to the CommandBuffer for post-GPU recycling.
	// On DiscardEncoding(), the encoder is reset and returned to the pool immediately.
	halEncoder hal.CommandEncoder

	// usedBuffers tracks root-level buffers referenced during encoding for
	// submit-time validation (VAL-A6). At Submit, each buffer is checked for
	// destroyed/mapped state. Using a map for O(1) deduplication — the same
	// buffer may be set as vertex, index, and bind group buffer in a single pass.
	usedBuffers map[*Buffer]struct{}

	// usedTextures tracks root-level textures referenced during encoding for
	// submit-time validation (VAL-A6). At Submit, each texture is checked for
	// destroyed state.
	usedTextures map[*resource.Texture]struct{}

	// usedBindGroups tracks bind groups referenced during encoding for
	// submit-time validation (VAL-B5). At Submit, each bind group is checked
	// for destroyed state. Matches Rust wgpu-core's cmd_buf_data.trackers.bind_groups
	// (device/queue.rs:1815-1817).
	usedBindGroups map[*resource.BindGroup]struct{}

	// usedQuerySets tracks query sets referenced during encoding for
	// submit-time validation. At Submit, a set whose HAL query set is gone
	// causes an error, as Rust wgpu-core's query_set.try_raw does
	// (device/queue.rs:2167-2168).
	usedQuerySets map[*resource.QuerySet]struct{}

	// internal holds the resources the frontend created for the encoder's own
	// commands (the stencil upload blit's staging texture, views, uniform
	// buffer and bind groups). They have no application owner, so the command
	// buffer releases them once it is spent: submitted or released. They stay
	// unreleased until then because Submit refuses a command buffer that
	// references a released resource (VAL-A6); the commands' own references
	// keep them alive until the GPU is done.
	internal []internalResource
}

// internalResource is a resource the frontend created for its own commands.
type internalResource interface{ Release() }

// own hands r to the encoder, which releases it with its command buffer.
func (e *CommandEncoder) own(r internalResource) {
	e.internal = append(e.internal, r)
}

// releaseInternal releases the frontend's own resources of a spent encoder or
// command buffer.
func releaseInternal(resources []internalResource) {
	for _, r := range resources {
		r.Release()
	}
}

// setError records a deferred error on the underlying command encoder.
// This implements the WebGPU deferred error pattern: encoding-phase errors
// are collected and surfaced when Finish() is called.
func (e *CommandEncoder) setError(err error) {
	if e.core != nil {
		e.core.SetError(err)
	}
}

// locked reports whether a pass of the encoder is open. A command recorded on
// the encoder itself then invalidates it and records nothing
// (core.CoreCommandEncoder.RequireUnlocked).
func (e *CommandEncoder) locked(op string) bool {
	return e.core != nil && !e.core.RequireUnlocked("CommandEncoder."+op)
}

// trackRef Clone()'s a ResourceRef and accumulates it for transfer to the
// CommandBuffer on Finish(). This keeps the resource alive until the GPU
// completes the submission. Used for encoder-level operations (copy commands).
func (e *CommandEncoder) trackRef(ref *core.ResourceRef) {
	if ref != nil {
		ref.Clone()
		e.trackedRefs = append(e.trackedRefs, ref)
	}
}

// trackBuffer records a buffer reference for submit-time validation (VAL-A6).
// The map is lazily initialized to avoid allocation when no buffers are used.
func (e *CommandEncoder) trackBuffer(buf *Buffer) {
	if buf == nil {
		return
	}
	if e.usedBuffers == nil {
		e.usedBuffers = make(map[*Buffer]struct{})
	}
	e.usedBuffers[buf] = struct{}{}
}

// trackTexture records a texture reference for submit-time validation (VAL-A6)
// and, the first time, clones the texture's reference so it stays alive until
// the command buffer is discarded or its submission completes, as Rust
// wgpu-core's command buffer trackers hold an Arc<Texture>. The map is lazily
// initialized to avoid allocation when no textures are used.
func (e *CommandEncoder) trackTexture(tex *resource.Texture) {
	if tex == nil {
		return
	}
	if e.usedTextures == nil {
		e.usedTextures = make(map[*resource.Texture]struct{})
	}
	if _, seen := e.usedTextures[tex]; !seen {
		e.usedTextures[tex] = struct{}{}
		e.trackRef(tex.Ref())
	}
}

// trackBindGroup records a bind group reference for submit-time validation (VAL-B5).
// The map is lazily initialized to avoid allocation when no bind groups are used.
// Matches Rust wgpu-core's cmd_buf_data.trackers.bind_groups (device/queue.rs:1815-1817).
func (e *CommandEncoder) trackBindGroup(bg *resource.BindGroup) {
	if bg == nil {
		return
	}
	if e.usedBindGroups == nil {
		e.usedBindGroups = make(map[*resource.BindGroup]struct{})
	}
	e.usedBindGroups[bg] = struct{}{}
}

// trackQuerySet records a query set reference for submit-time validation and,
// the first time, clones the set's reference so it stays alive until the
// command buffer's submission completes, as Rust wgpu-core's command buffer
// trackers hold an Arc<QuerySet>. The map is lazily initialized to avoid
// allocation when no query sets are used (P1-CORE-021).
func (e *CommandEncoder) trackQuerySet(qs *resource.QuerySet) {
	if qs == nil || qs.Core() == nil {
		return
	}
	if e.usedQuerySets == nil {
		e.usedQuerySets = make(map[*resource.QuerySet]struct{})
	}
	if _, seen := e.usedQuerySets[qs]; seen {
		return
	}
	e.usedQuerySets[qs] = struct{}{}
	e.trackRef(qs.Core().Ref)
}

// recordCopyUsages records the resource usages of ONE copy command.
//
// A copy command is its own WebGPU usage scope. Its endpoints are validated
// against each other only (for example a texture or buffer copied onto
// itself); usages by earlier commands of this encoder — a render pass that
// drew into the source, an upload into a texture sampled later — belong to
// other usage scopes and are never conflicts. After validation each usage is
// applied to the command buffer tracker, and the transitions from the
// previous usages are recorded on raw before the copy.
//
// Validation finishes before any state changes, so a rejected copy leaves
// the tracker, the HAL encoder and the submit validation sets untouched.
//
// Reference: wgpu-core command/transfer.rs (tracker.textures.set_single /
// tracker.buffers.set_single, then transition_textures/buffers before the copy)
func (e *CommandEncoder) recordCopyUsages(raw hal.CommandEncoder, textures []copyTextureUsage, buffers []copyBufferUsage) bool {
	if e.core == nil {
		return true
	}
	preparedTextures, err := prepareCopyTextureUsages(textures)
	if err != nil {
		e.setError(fmt.Errorf("wgpu: texture usage conflict: %w", err))
		return false
	}
	preparedBuffers, err := prepareCopyBufferUsages(buffers)
	if err != nil {
		e.setError(fmt.Errorf("wgpu: buffer usage conflict: %w", err))
		return false
	}
	e.transitionCopyTextures(raw, preparedTextures)
	e.transitionCopyBuffers(raw, preparedBuffers)
	return true
}

// transitionCopyTextures applies a copy command's texture usages to the
// command buffer tracker and records the needed barriers on raw.
func (e *CommandEncoder) transitionCopyTextures(raw hal.CommandEncoder, requests []preparedCopyTextureUsage) {
	tracker := e.core.Mutable().TextureScope()
	if tracker == nil {
		return
	}
	var barriers []hal.TextureBarrier
	for _, request := range requests {
		transition, needed := tracker.SetSingle(request.index, request.usage)
		if !needed {
			continue
		}
		if halTexture := request.texture.ResolveHAL(); halTexture != nil {
			barriers = append(barriers, transition.IntoHAL(halTexture))
		}
	}
	if len(barriers) > 0 && raw != nil {
		raw.TransitionTextures(barriers)
	}
}

// transitionCopyBuffers applies a copy command's buffer usages to the command
// buffer tracker and records the needed barriers on raw.
func (e *CommandEncoder) transitionCopyBuffers(raw hal.CommandEncoder, requests []preparedCopyBufferUsage) {
	tracker := e.core.Mutable().BufferScope()
	if tracker == nil {
		return
	}
	var barriers []hal.BufferBarrier
	for _, request := range requests {
		transition, needed := tracker.SetSingle(request.index, request.usage)
		if !needed {
			continue
		}
		if halBuffer := request.buffer.impl.HALBuffer(); halBuffer != nil {
			barriers = append(barriers, transition.IntoHAL(halBuffer))
		}
	}
	if len(barriers) > 0 && raw != nil {
		raw.TransitionBuffers(barriers)
	}
}

// BeginRenderPass begins a render pass.
// The returned RenderPassEncoder records draw commands.
// Call RenderPassEncoder.End() when done.
func (e *CommandEncoder) BeginRenderPass(desc *RenderPassDescriptor) (*RenderPassEncoder, error) {
	if e.released {
		return nil, core.ErrReleased
	}
	if err := validateRenderPassTextureViews(desc); err != nil {
		return nil, err
	}
	trackRenderPassTextureViews(e, desc)

	coreDesc := convertRenderPassDesc(desc)

	corePass, err := e.core.BeginRenderPass(coreDesc)
	if err != nil {
		return nil, err
	}

	pass := &RenderPassEncoder{core: corePass, encoder: e}
	if e.test != nil {
		pass.test = e.test
		label := ""
		var loadOps []gputypes.LoadOp
		var clears []gputypes.Color
		if desc != nil {
			label = desc.Label
			if n := len(desc.ColorAttachments); n > 0 {
				loadOps = make([]gputypes.LoadOp, n)
				clears = make([]gputypes.Color, n)
				for i := range desc.ColorAttachments {
					loadOps[i] = desc.ColorAttachments[i].LoadOp
					clears[i] = desc.ColorAttachments[i].ClearValue
				}
			}
		}
		pass.testPass = e.test.PassBegan(e.testEncoder, false, label, loadOps, clears)
	}
	if desc != nil && desc.DepthStencilAttachment != nil {
		ds := desc.DepthStencilAttachment
		pass.depthReadOnly, pass.stencilReadOnly = ds.DepthReadOnly, ds.StencilReadOnly
	}
	if desc != nil && desc.TimestampWrites != nil && desc.TimestampWrites.QuerySet != nil {
		e.trackQuerySet(desc.TimestampWrites.QuerySet.impl)
	}
	return pass, nil
}

// BeginComputePass begins a compute pass.
// The returned ComputePassEncoder records dispatch commands.
// Call ComputePassEncoder.End() when done.
func (e *CommandEncoder) BeginComputePass(desc *ComputePassDescriptor) (*ComputePassEncoder, error) {
	if e.released {
		return nil, core.ErrReleased
	}

	var coreDesc *core.CoreComputePassDescriptor
	if desc != nil {
		coreDesc = &core.CoreComputePassDescriptor{Label: desc.Label}
		if writes := desc.TimestampWrites; writes != nil {
			var liveCore *core.QuerySet
			if writes.QuerySet != nil && writes.QuerySet.impl != nil {
				liveCore = writes.QuerySet.impl.LiveCore()
			}
			coreDesc.TimestampWrites = &core.CoreComputePassTimestampWrites{
				QuerySet:                  liveCore,
				BeginningOfPassWriteIndex: writes.BeginningOfPassWriteIndex,
				EndOfPassWriteIndex:       writes.EndOfPassWriteIndex,
			}
		}
	}

	corePass, err := e.core.BeginComputePass(coreDesc)
	if err != nil {
		return nil, err
	}

	if desc != nil && desc.TimestampWrites != nil && desc.TimestampWrites.QuerySet != nil {
		e.trackQuerySet(desc.TimestampWrites.QuerySet.impl)
	}
	pass := &ComputePassEncoder{core: corePass, encoder: e}
	if e.test != nil {
		pass.test = e.test
		label := ""
		if desc != nil {
			label = desc.Label
		}
		pass.testPass = e.test.PassBegan(e.testEncoder, true, label, nil, nil)
	}
	return pass, nil
}

// CopyBufferToBuffer copies data between buffers.
func (e *CommandEncoder) CopyBufferToBuffer(src *Buffer, srcOffset uint64, dst *Buffer, dstOffset uint64, size uint64) {
	if e.released || e.locked("CopyBufferToBuffer") {
		return
	}
	if src == nil {
		e.setError(fmt.Errorf("wgpu: CommandEncoder.CopyBufferToBuffer: source buffer is nil"))
		return
	}
	if dst == nil {
		e.setError(fmt.Errorf("wgpu: CommandEncoder.CopyBufferToBuffer: destination buffer is nil"))
		return
	}
	raw := e.core.RawEncoder()
	if raw == nil {
		return
	}
	halSrc := src.impl.HALBuffer()
	halDst := dst.impl.HALBuffer()
	if halSrc == nil || halDst == nil {
		e.setError(fmt.Errorf("wgpu: CommandEncoder.CopyBufferToBuffer: source or destination buffer is released: %w", core.ErrReleased))
		return
	}
	// WebGPU copyBufferToBuffer valid usage; Rust wgpu-core transfer.rs
	// copy_buffer_to_buffer. An invalid copy must not reach the backend.
	if err := core.ValidateBufferToBufferCopy(src.impl.CoreBuffer() == dst.impl.CoreBuffer(),
		core.BufferCopyEndpoint{Usage: src.Usage(), Size: src.Size(), Offset: srcOffset},
		core.BufferCopyEndpoint{Usage: dst.Usage(), Size: dst.Size(), Offset: dstOffset},
		size); err != nil {
		e.setError(fmt.Errorf("wgpu: CommandEncoder.CopyBufferToBuffer: %w", err))
		return
	}
	// Rust drops an empty copy after validation and before any side effect
	// (transfer.rs copy_buffer_to_buffer); Vulkan rejects a zero size.
	if size == 0 {
		return
	}
	if !e.recordCopyUsages(raw, nil, []copyBufferUsage{
		{buffer: src, usage: track.BufferUsesCopySrc},
		{buffer: dst, usage: track.BufferUsesCopyDst},
	}) {
		return
	}
	e.trackRef(src.impl.CoreBuffer().Ref)
	e.trackRef(dst.impl.CoreBuffer().Ref)
	e.trackBuffer(src)
	e.trackBuffer(dst)
	raw.CopyBufferToBuffer(halSrc, halDst, []hal.BufferCopy{
		{SrcOffset: srcOffset, DstOffset: dstOffset, Size: size},
	})
}

// CopyTextureToBuffer copies data from a texture to a buffer.
// This is used for GPU-to-CPU readback of rendered content.
func (e *CommandEncoder) CopyTextureToBuffer(src *Texture, dst *Buffer, regions []BufferTextureCopy) {
	if e.released || e.locked("CopyTextureToBuffer") {
		return
	}
	if src == nil {
		e.setError(fmt.Errorf("wgpu: CommandEncoder.CopyTextureToBuffer: source texture is nil"))
		return
	}
	if dst == nil {
		e.setError(fmt.Errorf("wgpu: CommandEncoder.CopyTextureToBuffer: destination buffer is nil"))
		return
	}
	halSrc := src.impl.ResolveHAL()
	if halSrc == nil {
		e.setError(fmt.Errorf("wgpu: CommandEncoder.CopyTextureToBuffer: source texture is released: %w", core.ErrReleased))
		return
	}
	for _, region := range regions {
		if region.TextureBase.Texture != nil && region.TextureBase.Texture.impl.ResolveHAL() == nil {
			e.setError(fmt.Errorf("wgpu: CommandEncoder.CopyTextureToBuffer: region texture is released: %w", core.ErrReleased))
			return
		}
	}
	halDst := dst.impl.HALBuffer()
	if halDst == nil {
		e.setError(fmt.Errorf("wgpu: CommandEncoder.CopyTextureToBuffer: destination buffer is released: %w", core.ErrReleased))
		return
	}
	if !e.validateBufferTextureCopies("CopyTextureToBuffer", core.CopySideSource, src.impl, dst, regions) {
		return
	}
	regions, skip := nonEmptyBufferTextureRegions(regions)
	if skip {
		return
	}
	raw := e.core.RawEncoder()
	if raw == nil {
		return
	}
	if !e.recordCopyUsages(raw,
		[]copyTextureUsage{{texture: src.impl, usage: track.TextureUsesCopySrc}},
		[]copyBufferUsage{{buffer: dst, usage: track.BufferUsesCopyDst}},
	) {
		return
	}
	for _, region := range regions {
		if region.TextureBase.Texture != nil {
			e.trackTexture(region.TextureBase.Texture.impl)
		}
	}
	e.trackTexture(src.impl)
	e.trackBuffer(dst)
	e.trackRef(dst.impl.CoreBuffer().Ref)
	halRegions := make([]hal.BufferTextureCopy, len(regions))
	for i, r := range regions {
		halRegions[i] = r.toHAL()
	}
	raw.CopyTextureToBuffer(halSrc, halDst, halRegions)
}

// CopyTextureToTexture copies data between textures using DMA hardware copy.
// WebGPU spec: GPUCommandEncoder.copyTextureToTexture()
func (e *CommandEncoder) CopyTextureToTexture(src, dst *Texture, regions []TextureCopy) {
	if e.released || e.locked("CopyTextureToTexture") {
		return
	}
	if src == nil {
		e.setError(fmt.Errorf("wgpu: CommandEncoder.CopyTextureToTexture: source texture is nil"))
		return
	}
	if dst == nil {
		e.setError(fmt.Errorf("wgpu: CommandEncoder.CopyTextureToTexture: destination texture is nil"))
		return
	}
	halSrc := src.impl.ResolveHAL()
	halDst := dst.impl.ResolveHAL()
	if halSrc == nil || halDst == nil {
		e.setError(fmt.Errorf("wgpu: CommandEncoder.CopyTextureToTexture: texture is released: %w", core.ErrReleased))
		return
	}
	for _, region := range regions {
		if (region.Source.Texture != nil && region.Source.Texture.impl.ResolveHAL() == nil) ||
			(region.Destination.Texture != nil && region.Destination.Texture.impl.ResolveHAL() == nil) {
			e.setError(fmt.Errorf("wgpu: CommandEncoder.CopyTextureToTexture: region texture is released: %w", core.ErrReleased))
			return
		}
	}
	if !e.validateTextureToTextureCopies(src.impl, dst.impl, regions) {
		return
	}
	regions, skip := nonEmptyTextureCopyRegions(regions)
	if skip {
		return
	}
	raw := e.core.RawEncoder()
	if raw == nil {
		return
	}
	if !e.recordCopyUsages(raw, []copyTextureUsage{
		{texture: src.impl, usage: track.TextureUsesCopySrc},
		{texture: dst.impl, usage: track.TextureUsesCopyDst},
	}, nil) {
		return
	}
	for _, region := range regions {
		if region.Source.Texture != nil {
			e.trackTexture(region.Source.Texture.impl)
		}
		if region.Destination.Texture != nil {
			e.trackTexture(region.Destination.Texture.impl)
		}
	}
	e.trackTexture(src.impl)
	e.trackTexture(dst.impl)
	halRegions := make([]hal.TextureCopy, len(regions))
	for i, r := range regions {
		halRegions[i] = r.toHAL()
	}
	raw.CopyTextureToTexture(halSrc, halDst, halRegions)
}

// TransitionTextures records explicit texture barriers.
//
// Copy commands and render passes already transition their textures
// automatically from the state left by earlier commands of the same encoder,
// so explicit barriers are only needed for states the encoder cannot see.
// Each explicit barrier also updates the encoder's tracker: later commands
// transition from the barrier's NewUsage instead of repeating it.
// On Metal, GLES, and software backends the barrier itself is a no-op.
func (e *CommandEncoder) TransitionTextures(barriers []TextureBarrier) {
	if e.released || e.locked("TransitionTextures") {
		return
	}
	raw := e.core.RawEncoder()
	if raw == nil {
		return
	}
	halBarriers := make([]hal.TextureBarrier, 0, len(barriers))
	validBarriers := make([]TextureBarrier, 0, len(barriers))
	for _, b := range barriers {
		if b.Texture != nil && b.Texture.impl.ResolveHAL() == nil {
			e.setError(fmt.Errorf("wgpu: CommandEncoder.TransitionTextures: texture is released: %w", core.ErrReleased))
			return
		}
		if b.Texture == nil || b.Texture.impl.ResolveHAL() == nil {
			continue
		}
		e.trackTexture(b.Texture.impl)
		halBarriers = append(halBarriers, b.toHAL())
		validBarriers = append(validBarriers, b)
	}
	if len(halBarriers) > 0 {
		raw.TransitionTextures(halBarriers)
		for _, b := range validBarriers {
			e.core.RecordTextureTransition(b.Texture.impl.CoreTexture(), b.Usage.OldUsage, b.Usage.NewUsage)
		}
	}
}

// CopyBufferToTexture copies data from a buffer to a texture.
// WebGPU spec: GPUCommandEncoder.copyBufferToTexture.
func (e *CommandEncoder) CopyBufferToTexture(src *Buffer, dst *Texture, regions []BufferTextureCopy) {
	if e.released || e.locked("CopyBufferToTexture") {
		return
	}
	if src == nil {
		e.setError(fmt.Errorf("wgpu: CommandEncoder.CopyBufferToTexture: source buffer is nil"))
		return
	}
	if dst == nil {
		e.setError(fmt.Errorf("wgpu: CommandEncoder.CopyBufferToTexture: destination texture is nil"))
		return
	}
	halDst := dst.impl.ResolveHAL()
	if halDst == nil {
		e.setError(fmt.Errorf("wgpu: CommandEncoder.CopyBufferToTexture: destination texture is released: %w", core.ErrReleased))
		return
	}
	halSrc := src.impl.HALBuffer()
	if halSrc == nil {
		e.setError(fmt.Errorf("wgpu: CommandEncoder.CopyBufferToTexture: source buffer is released: %w", core.ErrReleased))
		return
	}
	if !e.validateBufferTextureCopies("CopyBufferToTexture", core.CopySideDestination, dst.impl, src, regions) {
		return
	}
	regions, skip := nonEmptyBufferTextureRegions(regions)
	if skip {
		return
	}
	// Uploads the backend cannot write (the stencil of a combined format on
	// GLES) are lowered to the stencil blit, in region order; the regions
	// between them are copied as they are.
	start := 0
	for i := range regions {
		if !dst.impl.NeedsStencilBlit(regions[i].TextureBase.Aspect) {
			continue
		}
		if i > start && !e.recordBufferToTexture(src, halSrc, dst, halDst, regions[start:i]) {
			return
		}
		if err := e.lowerStencilCopy(src, dst, regions[i]); err != nil {
			e.setError(fmt.Errorf("wgpu: CommandEncoder.CopyBufferToTexture: %w", core.StencilCopyNeedsBlitError(dst.impl.Format(), err)))
			return
		}
		start = i + 1
	}
	if start == 0 || start < len(regions) {
		e.recordBufferToTexture(src, halSrc, dst, halDst, regions[start:])
	}
}

// lowerStencilCopy records the copy of region from src into the stencil of
// dst as the stencil blit: a copy into a staging R8Uint texture, then the
// blit's render passes (stencilUpload).
func (e *CommandEncoder) lowerStencilCopy(src *Buffer, dst *Texture, region BufferTextureCopy) error {
	u, err := e.device.newStencilUpload(dst, region.TextureBase.MipLevel, region.TextureBase.Origin, region.Size)
	if err != nil {
		return err
	}
	e.own(u.temp)
	e.CopyBufferToTexture(src, u.temp, []BufferTextureCopy{{
		BufferLayout: region.BufferLayout,
		TextureBase:  u.tempCopy(),
		Size:         region.Size,
	}})
	e.trackTexture(dst.impl)
	return u.Record(stencilRecorder{e})
}

// recordBufferToTexture records a validated buffer-to-texture copy of regions
// the backend writes itself. It returns false when the copy was rejected (the
// error recorded on the encoder).
func (e *CommandEncoder) recordBufferToTexture(src *Buffer, halSrc hal.Buffer, dst *Texture, halDst hal.Texture, regions []BufferTextureCopy) bool {
	raw := e.core.RawEncoder()
	if raw == nil {
		return false
	}
	if !e.recordCopyUsages(raw,
		[]copyTextureUsage{{texture: dst.impl, usage: track.TextureUsesCopyDst}},
		[]copyBufferUsage{{buffer: src, usage: track.BufferUsesCopySrc}},
	) {
		return false
	}
	e.trackTexture(dst.impl)
	e.trackBuffer(src)
	e.trackRef(src.impl.CoreBuffer().Ref)
	halRegions := make([]hal.BufferTextureCopy, len(regions))
	for i, r := range regions {
		halRegions[i] = hal.BufferTextureCopy{
			BufferLayout: hal.ImageDataLayout{
				Offset:       r.BufferLayout.Offset,
				BytesPerRow:  r.BufferLayout.BytesPerRow,
				RowsPerImage: r.BufferLayout.RowsPerImage,
			},
			TextureBase: hal.ImageCopyTexture{
				Texture:  halDst,
				MipLevel: r.TextureBase.MipLevel,
				Origin:   hal.Origin3D(r.TextureBase.Origin),
				Aspect:   r.TextureBase.Aspect,
			},
			Size: hal.Extent3D(r.Size),
		}
	}
	raw.CopyBufferToTexture(halSrc, halDst, halRegions)
	return true
}

// trackTextureView keeps a render pass attachment view alive like
// trackTexture does its texture: the pass records the view itself (a Vulkan
// framebuffer, a D3D12 RTV/DSV), not only the texture.
func (e *CommandEncoder) trackTextureView(view *resource.TextureView) {
	if view == nil {
		return
	}
	e.trackTexture(view.Parent())
	e.trackRef(view.Ref())
}

// ClearBuffer clears a buffer region to zero.
// WebGPU spec: GPUCommandEncoder.clearBuffer.
//
// The size is explicit: a zero size clears nothing (WebGPU's default of "the
// rest of the buffer" has no Go spelling; pass buffer.Size() - offset).
// Validation follows Rust wgpu-core command/clear.rs clear_buffer and the
// W3C clearBuffer valid usage list; failures are *core.ClearError from Finish.
func (e *CommandEncoder) ClearBuffer(buffer *Buffer, offset, size uint64) {
	if e.released || e.locked("ClearBuffer") {
		return
	}
	if buffer == nil {
		e.setError(fmt.Errorf("wgpu: CommandEncoder.ClearBuffer: buffer is nil"))
		return
	}
	raw := e.core.RawEncoder()
	if raw == nil {
		return
	}
	halBuffer := buffer.impl.HALBuffer()
	if halBuffer == nil {
		e.setError(fmt.Errorf("wgpu: CommandEncoder.ClearBuffer: buffer is released: %w", core.ErrReleased))
		return
	}
	if err := core.ValidateBufferClear(buffer.Usage(), buffer.Size(), offset, size); err != nil {
		e.setError(fmt.Errorf("wgpu: CommandEncoder.ClearBuffer: %w", err))
		return
	}
	if size == 0 {
		return
	}
	// The clear writes the buffer as a copy destination; Rust wgpu-core tracks
	// it as COPY_DST (command/clear.rs clear_buffer), which orders it after
	// earlier uses, and keeps the buffer alive until the submission completes.
	if !e.recordCopyUsages(raw, nil, []copyBufferUsage{{buffer: buffer, usage: track.BufferUsesCopyDst}}) {
		return
	}
	e.trackRef(buffer.impl.CoreBuffer().Ref)
	e.trackBuffer(buffer)
	raw.ClearBuffer(halBuffer, offset, size)
}

// ResolveQuerySet copies query results into a buffer.
func (e *CommandEncoder) ResolveQuerySet(querySet *QuerySet, firstQuery, queryCount uint32, dst *Buffer, dstOffset uint64) {
	if e.released || e.locked("ResolveQuerySet") {
		return
	}

	if dst == nil {
		e.setError(fmt.Errorf("wgpu: CommandEncoder.ResolveQuerySet: destination buffer is nil"))
		return
	}
	if dst.impl.IsReleased() {
		e.setError(fmt.Errorf("wgpu: CommandEncoder.ResolveQuerySet: destination buffer is released: %w", core.ErrReleased))
		return
	}

	raw := e.core.RawEncoder()
	if raw == nil {
		return
	}

	halDestination := dst.impl.HALBuffer()
	if halDestination == nil {
		e.setError(fmt.Errorf("wgpu: CommandEncoder.ResolveQuerySet: destination buffer is released: %w", core.ErrReleased))
		return
	}

	var halQuerySet hal.QuerySet
	if querySet != nil && querySet.impl != nil {
		halQuerySet = querySet.impl.HALQuerySet()
	}
	if halQuerySet == nil {
		e.setError(fmt.Errorf("wgpu: CommandEncoder.ResolveQuerySet: query set is released: %w", core.ErrReleased))
		return
	}

	// The resolve writes the buffer as a copy destination; Rust wgpu-core
	// tracks it as COPY_DST (command/query.rs resolve_query_set), which
	// orders it after earlier uses and before a later copy or map.
	if !e.recordCopyUsages(raw, nil, []copyBufferUsage{{buffer: dst, usage: track.BufferUsesCopyDst}}) {
		return
	}
	e.trackRef(dst.impl.CoreBuffer().Ref)
	e.trackBuffer(dst)
	e.trackQuerySet(querySet.impl)

	raw.ResolveQuerySet(halQuerySet, firstQuery, queryCount, halDestination, dstOffset)
}

// DiscardEncoding discards the encoder without producing a command buffer.
// Use this to abandon an in-progress encoding when an error occurs.
// If the encoder was acquired from the pool, it is returned for reuse.
func (e *CommandEncoder) DiscardEncoding() {
	if e.released {
		return
	}
	e.released = true
	if e.device != nil {
		e.device.recordings.RemoveEncoder(e)
	}
	e.discard()
}

// discard ends the backend encoding of an encoder that will not be
// submitted, returns the HAL encoder to the pool, and only then drops the
// references its commands took. The last reference may destroy a resource,
// and an open D3D12 command list must not outlive a resource it records
// ("ID3D12CommandList::Close: An ID3D12Resource ... was deleted prior to
// closing the command list"). Rust wgpu-core gets the same order from
// InnerCommandEncoder::drop (discard_encoding, reset_all, release_encoder),
// which runs before the trackers holding the resources drop.
func (e *CommandEncoder) discard() {
	e.returnEncoderToPool(e.core.DiscardHAL())
	for _, ref := range e.trackedRefs {
		ref.Drop()
	}
	e.trackedRefs = nil
	releaseInternal(e.internal)
	e.internal = nil
}

// returnEncoderToPool resets and returns the HAL encoder to the device's pool.
// Called when the encoder will not be submitted (error or discard).
// ResetAll must be called before release to ensure the command pool is reset
// and all command buffers return to initial state — otherwise the next
// BeginEncoding will fail with VUID-vkBeginCommandBuffer-commandBuffer-00049
// because CBs from an unreset pool may still be in executable/recording state.
// Matches Rust wgpu-core InnerCommandEncoder::drop (command/mod.rs:726-738).
//
// closed are the command buffers the encoder already ended (render pass
// splits); ResetAll reclaims them.
func (e *CommandEncoder) returnEncoderToPool(closed []hal.CommandBuffer) {
	if e.halEncoder == nil || e.device == nil || e.device.cmdEncoderPool == nil {
		return
	}
	e.halEncoder.ResetAll(closed)
	e.device.cmdEncoderPool.Release(e.halEncoder)
	e.halEncoder = nil
}

// Finish completes command recording and returns a CommandBuffer.
// After calling Finish(), the encoder cannot be used again.
//
// The HAL encoder ownership transfers from the CommandEncoder to the
// CommandBuffer. After GPU completion, Submit() schedules the encoder
// to be reset via ResetAll and returned to the Device's encoder pool.
func (e *CommandEncoder) Finish() (*CommandBuffer, error) {
	if e.released {
		return nil, core.ErrReleased
	}
	e.released = true
	if e.device != nil {
		e.device.recordings.RemoveEncoder(e)
	}

	coreCmdBuffer, err := e.core.Finish()
	if err != nil {
		e.discard()
		return nil, err
	}

	// Transfer HAL encoder ownership to the CommandBuffer for post-GPU recycling.
	// Extract the HAL encoder from the core encoder's Snatchable so the pool
	// will own the only reference after recycling. Without this, the Snatchable
	// would hold a dangling reference after ResetAll.
	if e.halEncoder != nil {
		e.core.TakeHALEncoder()
	}

	cb := &CommandBuffer{
		core:           coreCmdBuffer,
		device:         e.device,
		trackedRefs:    e.trackedRefs,
		halEncoder:     e.halEncoder,
		usedBuffers:    e.usedBuffers,
		usedTextures:   e.usedTextures,
		usedBindGroups: e.usedBindGroups,
		usedQuerySets:  e.usedQuerySets,
		internal:       e.internal,
	}
	e.trackedRefs = nil
	e.internal = nil
	e.halEncoder = nil     // ownership transferred
	e.usedBuffers = nil    // ownership transferred
	e.usedTextures = nil   // ownership transferred
	e.usedBindGroups = nil // ownership transferred
	e.usedQuerySets = nil  // ownership transferred
	if e.device != nil {
		e.device.recordings.AddBuffer(cb)
	}
	return cb, nil
}

// attachmentHALTextures maps the core views created for one render pass to
// the HAL texture of their public parent texture. It backs
// core.RenderPassDescriptor.ResolveAttachmentTexture: the public Texture is
// the authority for the current HAL handle (a borrowed surface texture shares
// one core Texture across swapchain images).
type attachmentHALTextures map[*core.TextureView]hal.Texture

// convert creates the core view for v and remembers its parent HAL texture.
func (m attachmentHALTextures) convert(v *resource.TextureView) *core.TextureView {
	cv := resource.CoreTextureViewFrom(v)
	if v.Parent() != nil {
		m[cv] = v.Parent().ResolveHAL()
	}
	return cv
}

// resolve returns the HAL texture recorded for a converted view.
func (m attachmentHALTextures) resolve(view *core.TextureView) hal.Texture {
	return m[view]
}

// convertRenderPassDesc converts a public descriptor to core descriptor.
// The conversion wires core.TextureView.Parent from the public TextureView's
// parent Texture coreTexture, enabling TrackerIndex-based usage tracking of
// the pass attachments, and supplies the attachments' HAL textures for the
// transitions core records before the pass begins.
func convertRenderPassDesc(desc *RenderPassDescriptor) *core.RenderPassDescriptor {
	if desc == nil {
		return &core.RenderPassDescriptor{}
	}

	halTextures := make(attachmentHALTextures, len(desc.ColorAttachments)+1)
	coreDesc := &core.RenderPassDescriptor{
		Label:                    desc.Label,
		ResolveAttachmentTexture: halTextures.resolve,
	}

	for _, ca := range desc.ColorAttachments {
		coreCA := core.RenderPassColorAttachment{
			DepthSlice: ca.DepthSlice,
			LoadOp:     ca.LoadOp,
			StoreOp:    ca.StoreOp,
			ClearValue: ca.ClearValue,
		}
		if ca.View != nil {
			coreCA.View = halTextures.convert(ca.View.impl)
		}
		if ca.ResolveTarget != nil {
			coreCA.ResolveTarget = halTextures.convert(ca.ResolveTarget.impl)
		}
		coreDesc.ColorAttachments = append(coreDesc.ColorAttachments, coreCA)
	}

	if desc.DepthStencilAttachment != nil {
		ds := desc.DepthStencilAttachment
		coreDSA := &core.RenderPassDepthStencilAttachment{
			DepthLoadOp:       ds.DepthLoadOp,
			DepthStoreOp:      ds.DepthStoreOp,
			DepthClearValue:   ds.DepthClearValue,
			DepthReadOnly:     ds.DepthReadOnly,
			StencilLoadOp:     ds.StencilLoadOp,
			StencilStoreOp:    ds.StencilStoreOp,
			StencilClearValue: ds.StencilClearValue,
			StencilReadOnly:   ds.StencilReadOnly,
		}
		if ds.View != nil {
			coreDSA.View = halTextures.convert(ds.View.impl)
		}
		coreDesc.DepthStencilAttachment = coreDSA
	}

	if writes := desc.TimestampWrites; writes != nil {
		var liveCore *core.QuerySet
		if writes.QuerySet != nil && writes.QuerySet.impl != nil {
			liveCore = writes.QuerySet.impl.LiveCore()
		}
		coreDesc.TimestampWrites = &core.RenderPassTimestampWrites{
			QuerySet:                  liveCore,
			BeginningOfPassWriteIndex: writes.BeginningOfPassWriteIndex,
			EndOfPassWriteIndex:       writes.EndOfPassWriteIndex,
		}
	}

	return coreDesc
}

// coreTextureViewFrom lives in internal/resource as CoreTextureViewFrom: the
// stencil upload payload and the queue record against the same conversion, and
// the destination of the encoder move cannot call a root helper.

// CommandBuffer holds recorded GPU commands ready for submission.
// Created by CommandEncoder.Finish().
type CommandBuffer struct {
	core   *core.CoreCommandBuffer
	device *Device
	// trackedRefs holds Clone'd ResourceRefs from encoding. Transferred to
	// the DestroyQueue on Submit() so refs are Drop'd when GPU completes.
	// Phase 2: per-command-buffer resource tracking.
	trackedRefs []*core.ResourceRef

	// halEncoder is the HAL command encoder that produced this command buffer.
	// Ownership transfers from CommandEncoder to CommandBuffer on Finish(),
	// then to the DestroyQueue on Submit() for recycling after GPU completion.
	// After GPU completion, the encoder is reset via ResetAll and returned to
	// the Device's encoder pool. This avoids creating new DX12 command allocators
	// (~64KB each) or Vulkan command pools every frame.
	//
	// Matches Rust wgpu-core where the encoder travels:
	// CommandEncoder -> CommandBuffer -> EncoderInFlight -> GPU done -> pool
	halEncoder hal.CommandEncoder

	// usedBuffers tracks all buffers referenced during encoding (VAL-A6).
	// Validated at Submit time: destroyed or mapped buffers cause an error.
	// Matches Rust wgpu-core's cmd_buf_data.trackers.buffers.used_resources()
	// (device/queue.rs:1780-1787).
	usedBuffers map[*Buffer]struct{}

	// usedTextures tracks all textures referenced during encoding (VAL-A6).
	// Validated at Submit time: destroyed textures cause an error.
	// Matches Rust wgpu-core's cmd_buf_data.trackers.textures.used_resources()
	// (device/queue.rs:1791-1808).
	usedTextures map[*resource.Texture]struct{}

	// usedBindGroups tracks all bind groups referenced during encoding (VAL-B5).
	// Validated at Submit time: destroyed bind groups cause an error.
	// Matches Rust wgpu-core's cmd_buf_data.trackers.bind_groups
	// (device/queue.rs:1815-1817).
	usedBindGroups map[*resource.BindGroup]struct{}

	// usedQuerySets tracks all query sets referenced during encoding.
	// Validated at Submit time: a set whose HAL query set is gone causes an
	// error, as Rust wgpu-core's query_set.try_raw does (device/queue.rs:2167-2168).
	usedQuerySets map[*resource.QuerySet]struct{}

	// internal is CommandEncoder.internal, released once the command buffer
	// is spent.
	internal []internalResource

	// submitted is set to true after this command buffer has been submitted
	// to a queue. A command buffer cannot be submitted twice.
	// Matches Rust wgpu-core's CommandBuffer::take_finished() which consumes
	// the buffer, preventing reuse.
	submitted bool
}

// Release releases a CommandBuffer that will NOT be submitted to the GPU.
// This returns the HAL encoder to the device pool and drops tracked resource refs.
//
// In normal flow, Submit() takes ownership of the encoder and handles recycling
// after GPU completion. Release() is for error paths and canceled operations
// where the CommandBuffer is discarded without submitting.
//
// Matches Rust wgpu-core InnerCommandEncoder::Drop (command/mod.rs:726-738)
// which always calls reset_all + release_encoder regardless of whether the
// command buffer was submitted.
//
// A CommandBuffer MUST be either Submit()'d or Release()'d. Failing to do
// either leaks the HAL encoder (DX12 ~64KB allocator, Vulkan VkCommandPool).
func (cb *CommandBuffer) Release() {
	if cb == nil {
		return
	}
	if cb.device != nil {
		cb.device.recordings.RemoveBuffer(cb)
	}
	// Return encoder to pool (reset native allocator).
	// For multi-CB encoders, pass all HAL command buffers to ResetAll
	// so the underlying pool/allocator can reclaim them.
	if cb.halEncoder != nil && cb.device != nil && cb.device.cmdEncoderPool != nil {
		cb.halEncoder.ResetAll(cb.halBufferList())
		cb.device.cmdEncoderPool.Release(cb.halEncoder)
		cb.halEncoder = nil
	}
	// Drop tracked resource refs.
	for _, ref := range cb.trackedRefs {
		ref.Drop()
	}
	cb.trackedRefs = nil
	cb.dropUsedSets()
	cb.releaseInternal()
}

// releaseInternal releases the frontend's own resources the command buffer's
// commands use. After a submit the submission's references keep them alive
// until the GPU completes it.
func (cb *CommandBuffer) releaseInternal() {
	releaseInternal(cb.internal)
	cb.internal = nil
}

// releaseSubmittedInternal now lives in queue_native.go, its only caller: a
// free function over the command buffers cannot stay in this file once the
// buffers move (the queue keeps calling it from the root package until S9).

// dropUsedSets releases the encode-time validation sets. usedBuffers,
// usedTextures, usedBindGroups and usedQuerySets exist only for
// validateCommandBufferForSubmit; once a command buffer is spent — submitted
// or released — they are hard references pinning every resource the frame
// touched for as long as the command buffer stays reachable.
func (cb *CommandBuffer) dropUsedSets() {
	cb.usedBuffers = nil
	cb.usedTextures = nil
	cb.usedBindGroups = nil
	cb.usedQuerySets = nil
}

// halBufferList returns all HAL command buffers in submission order.
// For single-CB recording (the common case), returns a single-element slice.
// For multi-CB recording (via OpenPass/CloseCB/CloseAndSwap/CloseAndPushFront),
// returns all accumulated CBs.
//
// Reference: Rust wgpu-core BakedCommands.encoder.list (command/mod.rs:742-749)
func (cb *CommandBuffer) halBufferList() []hal.CommandBuffer {
	if cb.core == nil {
		return nil
	}
	return cb.core.HalBufferList()
}

// validateTextureToTextureCopies checks every region of a texture-to-texture
// copy with core.ValidateTextureToTextureCopy: mip levels, aspects, overlap
// inside one texture, sample counts and the COPY_SRC / COPY_DST usages.
// Textures without a core texture (test stubs) are not validated. Returns
// false, with the error recorded on the encoder, for the first invalid region.
func (e *CommandEncoder) validateTextureToTextureCopies(src, dst *resource.Texture, regions []TextureCopy) bool {
	for i := range regions {
		r := &regions[i]
		if r.Source.Texture == nil || r.Destination.Texture == nil {
			e.setError(fmt.Errorf("wgpu: CommandEncoder.CopyTextureToTexture: region %d: source or destination texture is nil", i))
			return false
		}
		srcView, srcOK := src.CoreTexture().CopyView(r.Source.MipLevel, gputypes.Origin3D(r.Source.Origin), r.Source.Aspect)
		dstView, dstOK := dst.CoreTexture().CopyView(r.Destination.MipLevel, gputypes.Origin3D(r.Destination.Origin), r.Destination.Aspect)
		if !srcOK || !dstOK {
			continue
		}
		sameTexture := src.CoreTexture() == dst.CoreTexture()
		if err := core.ValidateTextureToTextureCopy(sameTexture, srcView, dstView, gputypes.Extent3D(r.Size)); err != nil {
			e.setError(fmt.Errorf("wgpu: CommandEncoder.CopyTextureToTexture: region %d: %w", i, err))
			return false
		}
	}
	return true
}

// isEmptyExtent lives in internal/core as IsEmptyExtent: the queue drops
// zero-size regions with the same rule, and the destination of the encoder
// move cannot call a root helper.

// nonEmptyBufferTextureRegions drops the regions of zero size. Rust ignores
// such a copy after its validation and before touching the tracker
// (transfer.rs "Ignoring copy_* of size 0"); Vulkan rejects a zero extent.
// skip is true when regions held copies and every one was empty: the whole
// command is dropped. An empty region list is not a zero-size copy and is left
// as it is (it still records its usages, so its resource conflicts surface).
func nonEmptyBufferTextureRegions(regions []BufferTextureCopy) (kept []BufferTextureCopy, skip bool) {
	kept = make([]BufferTextureCopy, 0, len(regions))
	for i := range regions {
		if !core.IsEmptyExtent(gputypes.Extent3D(regions[i].Size)) {
			kept = append(kept, regions[i])
		}
	}
	return kept, len(regions) > 0 && len(kept) == 0
}

// nonEmptyTextureCopyRegions is nonEmptyBufferTextureRegions for
// texture-to-texture regions.
func nonEmptyTextureCopyRegions(regions []TextureCopy) (kept []TextureCopy, skip bool) {
	kept = make([]TextureCopy, 0, len(regions))
	for i := range regions {
		if !core.IsEmptyExtent(gputypes.Extent3D(regions[i].Size)) {
			kept = append(kept, regions[i])
		}
	}
	return kept, len(regions) > 0 && len(kept) == 0
}

// validateBufferTextureCopies checks every region of a buffer-texture copy
// (validateBufferTextureRegion). side is the texture's side of the copy.
// Returns false, with the error recorded on the encoder, for the first
// invalid region.
func (e *CommandEncoder) validateBufferTextureCopies(op string, side core.CopySide, texture *resource.Texture, buffer *Buffer, regions []BufferTextureCopy) bool {
	for i := range regions {
		if err := validateBufferTextureRegion(side, texture, buffer, &regions[i]); err != nil {
			e.setError(fmt.Errorf("wgpu: CommandEncoder.%s: region %d: %w", op, i, err))
			return false
		}
	}
	return true
}

// validateBufferTextureRegion checks one region of a buffer-texture copy in
// the order of Rust wgpu-core copy_buffer_to_texture and copy_texture_to_buffer:
// the copy range, usages, aspect and format of the texture and the buffer
// (core.ValidateBufferTextureCopyTexture), the sample count, aligned offset and
// bytesPerRow (core.ValidateTextureBufferCopy), the buffer layout
// (core.ValidateLinearTextureData) and the downlevel flag for depth/stencil
// copies (core.Device.RequireDepthTextureAndBufferCopies). An upload into a
// combined format's stencil aspect that the backend cannot write is valid
// here and lowered after validation (core.Device.StencilUploadNeedsBlit). A
// texture without a core texture (test stubs) only gets the layout alignment
// checks.
func validateBufferTextureRegion(side core.CopySide, texture *resource.Texture, buffer *Buffer, region *BufferTextureCopy) error {
	size := gputypes.Extent3D(region.Size)
	view, haveView := texture.CoreTexture().CopyView(region.TextureBase.MipLevel, gputypes.Origin3D(region.TextureBase.Origin), region.TextureBase.Aspect)
	if !haveView {
		return core.ValidateTextureBufferCopy(texture.Format(), 1, region.BufferLayout.Offset, region.BufferLayout.BytesPerRow)
	}
	if err := core.ValidateBufferTextureCopyTexture(side, view, buffer.Usage(), size); err != nil {
		return err
	}
	if err := core.ValidateTextureBufferCopy(view.Format, view.SampleCount, region.BufferLayout.Offset, region.BufferLayout.BytesPerRow); err != nil {
		return err
	}
	bufferSide := core.CopySideSource
	if side == core.CopySideSource {
		bufferSide = core.CopySideDestination
	}
	layout := core.TexelCopyBufferLayout{
		Offset:       region.BufferLayout.Offset,
		BytesPerRow:  region.BufferLayout.BytesPerRow,
		RowsPerImage: region.BufferLayout.RowsPerImage,
	}
	if err := core.ValidateLinearTextureData(layout, view.Format, region.TextureBase.Aspect, buffer.Size(), bufferSide, size); err != nil {
		return err
	}
	return texture.CoreTexture().Device().RequireDepthTextureAndBufferCopies(view.Format)
}
