//go:build !rust && !(js && wasm)

package encoder

import (
	"fmt"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/core/track"
	"github.com/gogpu/wgpu/internal/hal"
	"github.com/gogpu/wgpu/internal/resource"
	"github.com/gogpu/wgpu/internal/stencilblit"
)

// CommandEncoder records GPU commands for later submission.
//
// A command encoder is single-use. After calling Finish(), the encoder
// cannot be used again. Call Device.CreateCommandEncoder() to create a new one.
//
// NOT thread-safe - do not use from multiple goroutines.
type CommandEncoder struct {
	core     *core.CoreCommandEncoder
	device   DeviceHost
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
	usedBuffers map[*resource.Buffer]struct{}

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
	internal []stencilblit.Releasable
}

// CommandEncoderInit carries what the device has already built when it creates
// a user command encoder: the core encoder over the HAL encoder, the device
// seam view the encoder reaches the pool, the recordings registry and the
// stencil upload through, and the reused HAL encoder when the device has a
// pool.
//
// The fields are read once, at construction; the encoder never resolves the
// device again except through the seam methods.
type CommandEncoderInit struct {
	// Core is the core encoder, already recording.
	Core *core.CoreCommandEncoder
	// Device is the seam view of the device that created the encoder.
	Device DeviceHost
	// HALEncoder is the HAL encoder Core records into, when the device
	// acquired it from its pool. Nil for the fallback path, where the core
	// encoder created its own.
	HALEncoder hal.CommandEncoder
	// Pooled preallocates the tracked-refs slice the pooled path uses: an
	// encoder recycling every frame keeps the slice for the frame's refs.
	Pooled bool
}

// NewCommandEncoder builds a user command encoder. The device calls it from
// CreateCommandEncoder; the root declaration layer wraps the result in the
// public CommandEncoder.
func NewCommandEncoder(init CommandEncoderInit) *CommandEncoder {
	e := &CommandEncoder{
		core:       init.Core,
		device:     init.Device,
		halEncoder: init.HALEncoder,
		released:   false,
	}
	if init.Pooled {
		e.trackedRefs = make([]*core.ResourceRef, 0, 64)
	}
	return e
}

// SetReleasedForTest sets the encoder's released flag. It backs the root
// package's white-box tests of the released paths, which must hand the encoder
// a state DiscardEncoding alone cannot produce without a core encoder.
func (e *CommandEncoder) SetReleasedForTest(released bool) { e.released = released }

// SetTestHooks binds the wgputest recorder to the encoder and records its
// creation. A device without a recorder does not call it.
func (e *CommandEncoder) SetTestHooks(test core.TestHooks, encoderIndex int) {
	e.test = test
	e.testEncoder = encoderIndex
}

// Released reports whether the encoder has been finished or discarded. A
// released encoder records nothing and its Finish fails with core.ErrReleased.
func (e *CommandEncoder) Released() bool { return e.released }

// Core returns the core command encoder the encoder records into, or nil for
// a zero value. The public render and compute pass wrappers and the queue read
// it for the state core tracks (raw HAL encoder, usage scopes).
func (e *CommandEncoder) Core() *core.CoreCommandEncoder { return e.core }

// TrackedRefs returns the ResourceRefs the encoder holds, or nil. It backs the
// root test hooks and the CommandBuffer transfer check; the refs themselves
// are transferred, never copied, on Finish.
func (e *CommandEncoder) TrackedRefs() []*core.ResourceRef { return e.trackedRefs }

// Own hands r to the encoder, which releases it with its command buffer.
func (e *CommandEncoder) Own(r stencilblit.Releasable) {
	e.internal = append(e.internal, r)
}

// releaseInternal releases the frontend's own resources of a spent encoder or
// command buffer.
func releaseInternal(resources []stencilblit.Releasable) {
	for _, r := range resources {
		r.Release()
	}
}

// setError records a deferred error on the underlying command encoder.
// This implements the WebGPU deferred error pattern: encoding-phase errors
// are collected and surfaced when Finish() is called.
func (e *CommandEncoder) SetError(err error) {
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

// TrackRef Clone()'s a ResourceRef and accumulates it for transfer to the
// CommandBuffer on Finish(). This keeps the resource alive until the GPU
// completes the submission. Used for encoder-level operations (copy commands).
func (e *CommandEncoder) TrackRef(ref *core.ResourceRef) {
	if ref != nil {
		ref.Clone()
		e.trackedRefs = append(e.trackedRefs, ref)
	}
}

// UsedBuffers returns the buffers the encoder recorded for submit-time
// validation, or nil. It backs the root package's white-box tests of the copy
// path, which assert that a skipped copy tracks nothing; the sets themselves
// transfer to the command buffer on Finish.
func (e *CommandEncoder) UsedBuffers() map[*resource.Buffer]struct{} { return e.usedBuffers }

// UsedTextures returns the textures the encoder recorded for submit-time
// validation, or nil. It backs the root package's white-box tests of the copy
// path.
func (e *CommandEncoder) UsedTextures() map[*resource.Texture]struct{} { return e.usedTextures }

// trackBuffer records a buffer reference for submit-time validation (VAL-A6).
// The map is lazily initialized to avoid allocation when no buffers are used.
func (e *CommandEncoder) TrackBuffer(buf *resource.Buffer) {
	if buf == nil {
		return
	}
	if e.usedBuffers == nil {
		e.usedBuffers = make(map[*resource.Buffer]struct{})
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
		e.TrackRef(tex.Ref())
	}
}

// trackBindGroup records a bind group reference for submit-time validation (VAL-B5).
// The map is lazily initialized to avoid allocation when no bind groups are used.
// Matches Rust wgpu-core's cmd_buf_data.trackers.bind_groups (device/queue.rs:1815-1817).
func (e *CommandEncoder) TrackBindGroup(bg *resource.BindGroup) {
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
	e.TrackRef(qs.Core().Ref)
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
		e.SetError(fmt.Errorf("wgpu: texture usage conflict: %w", err))
		return false
	}
	preparedBuffers, err := prepareCopyBufferUsages(buffers)
	if err != nil {
		e.SetError(fmt.Errorf("wgpu: buffer usage conflict: %w", err))
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
	var transitionStorage [2]track.TexturePendingTransition
	for _, request := range requests {
		transitions := tracker.AppendSingle(transitionStorage[:0], request.index, track.TextureSelector{}, request.usage)
		if len(transitions) == 0 {
			continue
		}
		if halTexture := request.texture.ResolveHAL(); halTexture != nil {
			for _, transition := range transitions {
				barriers = append(barriers, transition.IntoHAL(halTexture))
			}
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
		if halBuffer := request.buffer.HALBuffer(); halBuffer != nil {
			barriers = append(barriers, transition.IntoHAL(halBuffer))
		}
	}
	if len(barriers) > 0 && raw != nil {
		raw.TransitionBuffers(barriers)
	}
}

// RenderPassStart is what BeginRenderPass hands to the public pass wrapper:
// the core pass over which the wrapper records, the wgputest recorder and the
// pass's dense index when one is installed, and the depth/stencil read-only
// flags a bundle executed in the pass is validated against.
type RenderPassStart struct {
	Core            *core.CoreRenderPassEncoder
	Test            core.TestHooks
	TestPass        int
	DepthReadOnly   bool
	StencilReadOnly bool
}

// BeginRenderPass begins a render pass.
// The returned RenderPassStart carries the core pass the public
// RenderPassEncoder records through.
func (e *CommandEncoder) BeginRenderPass(desc *RenderPassDescriptor) (*RenderPassStart, error) {
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

	start := &RenderPassStart{Core: corePass}
	if e.test != nil {
		start.Test = e.test
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
		start.TestPass = e.test.PassBegan(e.testEncoder, false, label, loadOps, clears)
	}
	if desc != nil && desc.DepthStencilAttachment != nil {
		ds := desc.DepthStencilAttachment
		start.DepthReadOnly, start.StencilReadOnly = ds.DepthReadOnly, ds.StencilReadOnly
	}
	if desc != nil && desc.TimestampWrites != nil && desc.TimestampWrites.QuerySet != nil {
		e.trackQuerySet(desc.TimestampWrites.QuerySet)
	}
	return start, nil
}

// ComputePassStart is RenderPassStart for a compute pass.
type ComputePassStart struct {
	Core     *core.CoreComputePassEncoder
	Test     core.TestHooks
	TestPass int
}

// BeginComputePass begins a compute pass.
// The returned ComputePassStart carries the core pass the public
// ComputePassEncoder records through.
func (e *CommandEncoder) BeginComputePass(desc *ComputePassDescriptor) (*ComputePassStart, error) {
	if e.released {
		return nil, core.ErrReleased
	}

	var coreDesc *core.CoreComputePassDescriptor
	if desc != nil {
		coreDesc = &core.CoreComputePassDescriptor{Label: desc.Label}
		if writes := desc.TimestampWrites; writes != nil {
			var liveCore *core.QuerySet
			if writes.QuerySet != nil {
				liveCore = writes.QuerySet.LiveCore()
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
		e.trackQuerySet(desc.TimestampWrites.QuerySet)
	}
	start := &ComputePassStart{Core: corePass}
	if e.test != nil {
		start.Test = e.test
		label := ""
		if desc != nil {
			label = desc.Label
		}
		start.TestPass = e.test.PassBegan(e.testEncoder, true, label, nil, nil)
	}
	return start, nil
}

// CopyBufferToBuffer copies data between buffers.
func (e *CommandEncoder) CopyBufferToBuffer(src *resource.Buffer, srcOffset uint64, dst *resource.Buffer, dstOffset uint64, size uint64) {
	if e.released || e.locked("CopyBufferToBuffer") {
		return
	}
	if src == nil {
		e.SetError(fmt.Errorf("wgpu: CommandEncoder.CopyBufferToBuffer: source buffer is nil"))
		return
	}
	if dst == nil {
		e.SetError(fmt.Errorf("wgpu: CommandEncoder.CopyBufferToBuffer: destination buffer is nil"))
		return
	}
	raw := e.core.RawEncoder()
	if raw == nil {
		return
	}
	halSrc := src.HALBuffer()
	halDst := dst.HALBuffer()
	if halSrc == nil || halDst == nil {
		e.SetError(fmt.Errorf("wgpu: CommandEncoder.CopyBufferToBuffer: source or destination buffer is released: %w", core.ErrReleased))
		return
	}
	// WebGPU copyBufferToBuffer valid usage; Rust wgpu-core transfer.rs
	// copy_buffer_to_buffer. An invalid copy must not reach the backend.
	if err := core.ValidateBufferToBufferCopy(src.CoreBuffer() == dst.CoreBuffer(),
		core.BufferCopyEndpoint{Usage: src.Usage(), Size: src.Size(), Offset: srcOffset},
		core.BufferCopyEndpoint{Usage: dst.Usage(), Size: dst.Size(), Offset: dstOffset},
		size); err != nil {
		e.SetError(fmt.Errorf("wgpu: CommandEncoder.CopyBufferToBuffer: %w", err))
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
	e.TrackRef(src.CoreBuffer().Ref)
	e.TrackRef(dst.CoreBuffer().Ref)
	e.TrackBuffer(src)
	e.TrackBuffer(dst)
	raw.CopyBufferToBuffer(halSrc, halDst, []hal.BufferCopy{
		{SrcOffset: srcOffset, DstOffset: dstOffset, Size: size},
	})
}

// CopyTextureToBuffer copies data from a texture to a buffer.
// This is used for GPU-to-CPU readback of rendered content.
func (e *CommandEncoder) CopyTextureToBuffer(src *resource.Texture, dst *resource.Buffer, regions []BufferTextureCopy) {
	if e.released || e.locked("CopyTextureToBuffer") {
		return
	}
	if src == nil {
		e.SetError(fmt.Errorf("wgpu: CommandEncoder.CopyTextureToBuffer: source texture is nil"))
		return
	}
	if dst == nil {
		e.SetError(fmt.Errorf("wgpu: CommandEncoder.CopyTextureToBuffer: destination buffer is nil"))
		return
	}
	halSrc := src.ResolveHAL()
	if halSrc == nil {
		e.SetError(fmt.Errorf("wgpu: CommandEncoder.CopyTextureToBuffer: source texture is released: %w", core.ErrReleased))
		return
	}
	for _, region := range regions {
		if region.TextureBase.Texture != nil && region.TextureBase.Texture.ResolveHAL() == nil {
			e.SetError(fmt.Errorf("wgpu: CommandEncoder.CopyTextureToBuffer: region texture is released: %w", core.ErrReleased))
			return
		}
	}
	halDst := dst.HALBuffer()
	if halDst == nil {
		e.SetError(fmt.Errorf("wgpu: CommandEncoder.CopyTextureToBuffer: destination buffer is released: %w", core.ErrReleased))
		return
	}
	if !e.validateBufferTextureCopies("CopyTextureToBuffer", core.CopySideSource, src, dst, regions) {
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
		[]copyTextureUsage{{texture: src, usage: track.TextureUsesCopySrc}},
		[]copyBufferUsage{{buffer: dst, usage: track.BufferUsesCopyDst}},
	) {
		return
	}
	for _, region := range regions {
		if region.TextureBase.Texture != nil {
			e.trackTexture(region.TextureBase.Texture)
		}
	}
	e.trackTexture(src)
	e.TrackBuffer(dst)
	e.TrackRef(dst.CoreBuffer().Ref)
	halRegions := make([]hal.BufferTextureCopy, len(regions))
	for i, r := range regions {
		halRegions[i] = r.toHAL()
	}
	raw.CopyTextureToBuffer(halSrc, halDst, halRegions)
}

// CopyTextureToTexture copies data between textures using DMA hardware copy.
// WebGPU spec: GPUCommandEncoder.copyTextureToTexture()
func (e *CommandEncoder) CopyTextureToTexture(src, dst *resource.Texture, regions []TextureCopy) {
	if e.released || e.locked("CopyTextureToTexture") {
		return
	}
	if src == nil {
		e.SetError(fmt.Errorf("wgpu: CommandEncoder.CopyTextureToTexture: source texture is nil"))
		return
	}
	if dst == nil {
		e.SetError(fmt.Errorf("wgpu: CommandEncoder.CopyTextureToTexture: destination texture is nil"))
		return
	}
	halSrc := src.ResolveHAL()
	halDst := dst.ResolveHAL()
	if halSrc == nil || halDst == nil {
		e.SetError(fmt.Errorf("wgpu: CommandEncoder.CopyTextureToTexture: texture is released: %w", core.ErrReleased))
		return
	}
	for _, region := range regions {
		if (region.Source.Texture != nil && region.Source.Texture.ResolveHAL() == nil) ||
			(region.Destination.Texture != nil && region.Destination.Texture.ResolveHAL() == nil) {
			e.SetError(fmt.Errorf("wgpu: CommandEncoder.CopyTextureToTexture: region texture is released: %w", core.ErrReleased))
			return
		}
	}
	if !e.validateTextureToTextureCopies(src, dst, regions) {
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
		{texture: src, usage: track.TextureUsesCopySrc},
		{texture: dst, usage: track.TextureUsesCopyDst},
	}, nil) {
		return
	}
	for _, region := range regions {
		if region.Source.Texture != nil {
			e.trackTexture(region.Source.Texture)
		}
		if region.Destination.Texture != nil {
			e.trackTexture(region.Destination.Texture)
		}
	}
	e.trackTexture(src)
	e.trackTexture(dst)
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
		if b.Texture != nil && b.Texture.ResolveHAL() == nil {
			e.SetError(fmt.Errorf("wgpu: CommandEncoder.TransitionTextures: texture is released: %w", core.ErrReleased))
			return
		}
		if b.Texture == nil || b.Texture.ResolveHAL() == nil {
			continue
		}
		e.trackTexture(b.Texture)
		halBarriers = append(halBarriers, b.toHAL())
		validBarriers = append(validBarriers, b)
	}
	if len(halBarriers) > 0 {
		raw.TransitionTextures(halBarriers)
		for _, b := range validBarriers {
			e.core.RecordTextureTransition(b.Texture.CoreTexture(), b.Usage.OldUsage, b.Usage.NewUsage)
		}
	}
}

// CopyBufferToTexture copies data from a buffer to a texture.
// WebGPU spec: GPUCommandEncoder.copyBufferToTexture.
func (e *CommandEncoder) CopyBufferToTexture(src *resource.Buffer, dst *resource.Texture, regions []BufferTextureCopy) {
	if e.released || e.locked("CopyBufferToTexture") {
		return
	}
	if src == nil {
		e.SetError(fmt.Errorf("wgpu: CommandEncoder.CopyBufferToTexture: source buffer is nil"))
		return
	}
	if dst == nil {
		e.SetError(fmt.Errorf("wgpu: CommandEncoder.CopyBufferToTexture: destination texture is nil"))
		return
	}
	halDst := dst.ResolveHAL()
	if halDst == nil {
		e.SetError(fmt.Errorf("wgpu: CommandEncoder.CopyBufferToTexture: destination texture is released: %w", core.ErrReleased))
		return
	}
	halSrc := src.HALBuffer()
	if halSrc == nil {
		e.SetError(fmt.Errorf("wgpu: CommandEncoder.CopyBufferToTexture: source buffer is released: %w", core.ErrReleased))
		return
	}
	if !e.validateBufferTextureCopies("CopyBufferToTexture", core.CopySideDestination, dst, src, regions) {
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
		if !dst.NeedsStencilBlit(regions[i].TextureBase.Aspect) {
			continue
		}
		if i > start && !e.recordBufferToTexture(src, halSrc, dst, halDst, regions[start:i]) {
			return
		}
		if err := e.lowerStencilCopy(src, dst, regions[i]); err != nil {
			e.SetError(fmt.Errorf("wgpu: CommandEncoder.CopyBufferToTexture: %w", core.StencilCopyNeedsBlitError(dst.Format(), err)))
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
func (e *CommandEncoder) lowerStencilCopy(src *resource.Buffer, dst *resource.Texture, region BufferTextureCopy) error {
	u, err := e.device.NewStencilUpload(dst, region.TextureBase.MipLevel, region.TextureBase.Origin, region.Size)
	if err != nil {
		return err
	}
	e.Own(u.Staging())
	e.CopyBufferToTexture(src, u.Staging(), []BufferTextureCopy{{
		BufferLayout: region.BufferLayout,
		TextureBase:  ImageCopyTexture{Texture: u.Staging(), Aspect: gputypes.TextureAspectAll},
		Size:         region.Size,
	}})
	e.trackTexture(dst)
	return u.Record(stencilRecorder{e})
}

// recordBufferToTexture records a validated buffer-to-texture copy of regions
// the backend writes itself. It returns false when the copy was rejected (the
// error recorded on the encoder).
func (e *CommandEncoder) recordBufferToTexture(src *resource.Buffer, halSrc hal.Buffer, dst *resource.Texture, halDst hal.Texture, regions []BufferTextureCopy) bool {
	raw := e.core.RawEncoder()
	if raw == nil {
		return false
	}
	if !e.recordCopyUsages(raw,
		[]copyTextureUsage{{texture: dst, usage: track.TextureUsesCopyDst}},
		[]copyBufferUsage{{buffer: src, usage: track.BufferUsesCopySrc}},
	) {
		return false
	}
	e.trackTexture(dst)
	e.TrackBuffer(src)
	e.TrackRef(src.CoreBuffer().Ref)
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
	e.TrackRef(view.Ref())
}

// ClearBuffer clears a buffer region to zero.
// WebGPU spec: GPUCommandEncoder.clearBuffer.
//
// The size is explicit: a zero size clears nothing (WebGPU's default of "the
// rest of the buffer" has no Go spelling; pass buffer.Size() - offset).
// Validation follows Rust wgpu-core command/clear.rs clear_buffer and the
// W3C clearBuffer valid usage list; failures are *core.ClearError from Finish.
func (e *CommandEncoder) ClearBuffer(buffer *resource.Buffer, offset, size uint64) {
	if e.released || e.locked("ClearBuffer") {
		return
	}
	if buffer == nil {
		e.SetError(fmt.Errorf("wgpu: CommandEncoder.ClearBuffer: buffer is nil"))
		return
	}
	raw := e.core.RawEncoder()
	if raw == nil {
		return
	}
	halBuffer := buffer.HALBuffer()
	if halBuffer == nil {
		e.SetError(fmt.Errorf("wgpu: CommandEncoder.ClearBuffer: buffer is released: %w", core.ErrReleased))
		return
	}
	if err := core.ValidateBufferClear(buffer.Usage(), buffer.Size(), offset, size); err != nil {
		e.SetError(fmt.Errorf("wgpu: CommandEncoder.ClearBuffer: %w", err))
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
	e.TrackRef(buffer.CoreBuffer().Ref)
	e.TrackBuffer(buffer)
	raw.ClearBuffer(halBuffer, offset, size)
}

// ResolveQuerySet copies query results into a buffer.
func (e *CommandEncoder) ResolveQuerySet(querySet *resource.QuerySet, firstQuery, queryCount uint32, dst *resource.Buffer, dstOffset uint64) {
	if e.released || e.locked("ResolveQuerySet") {
		return
	}

	if dst == nil {
		e.SetError(fmt.Errorf("wgpu: CommandEncoder.ResolveQuerySet: destination buffer is nil"))
		return
	}
	if dst.IsReleased() {
		e.SetError(fmt.Errorf("wgpu: CommandEncoder.ResolveQuerySet: destination buffer is released: %w", core.ErrReleased))
		return
	}

	raw := e.core.RawEncoder()
	if raw == nil {
		return
	}

	halDestination := dst.HALBuffer()
	if halDestination == nil {
		e.SetError(fmt.Errorf("wgpu: CommandEncoder.ResolveQuerySet: destination buffer is released: %w", core.ErrReleased))
		return
	}

	var halQuerySet hal.QuerySet
	if querySet != nil {
		halQuerySet = querySet.HALQuerySet()
	}
	if halQuerySet == nil {
		e.SetError(fmt.Errorf("wgpu: CommandEncoder.ResolveQuerySet: query set is released: %w", core.ErrReleased))
		return
	}

	// The resolve writes the buffer as a copy destination; Rust wgpu-core
	// tracks it as COPY_DST (command/query.rs resolve_query_set), which
	// orders it after earlier uses and before a later copy or map.
	if !e.recordCopyUsages(raw, nil, []copyBufferUsage{{buffer: dst, usage: track.BufferUsesCopyDst}}) {
		return
	}
	e.TrackRef(dst.CoreBuffer().Ref)
	e.TrackBuffer(dst)
	e.trackQuerySet(querySet)

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
		e.device.Recordings().RemoveEncoder(e)
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
	if e.halEncoder == nil || e.device == nil {
		return
	}
	pool := e.device.EncoderPool()
	if pool == nil {
		return
	}
	e.halEncoder.ResetAll(closed)
	pool.Release(e.halEncoder)
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
		e.device.Recordings().RemoveEncoder(e)
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
		e.device.Recordings().AddBuffer(cb)
	}
	return cb, nil
}

// attachmentHALTextures maps the core views created for one render pass to
// the HAL texture of their parent texture handle. It backs
// core.RenderPassDescriptor.ResolveAttachmentTexture: the texture is the
// authority for the current HAL handle (a borrowed surface texture shares one
// core Texture across swapchain images).
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

// convertRenderPassDesc converts the encoder's render pass descriptor to the
// core descriptor. The conversion wires core.TextureView.Parent from the
// attachment view's parent texture's core texture, enabling TrackerIndex-based
// usage tracking of the pass attachments, and supplies the attachments' HAL
// textures for the transitions core records before the pass begins.
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
			coreCA.View = halTextures.convert(ca.View)
		}
		if ca.ResolveTarget != nil {
			coreCA.ResolveTarget = halTextures.convert(ca.ResolveTarget)
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
			coreDSA.View = halTextures.convert(ds.View)
		}
		coreDesc.DepthStencilAttachment = coreDSA
	}

	if writes := desc.TimestampWrites; writes != nil {
		var liveCore *core.QuerySet
		if writes.QuerySet != nil {
			liveCore = writes.QuerySet.LiveCore()
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
	device DeviceHost
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
	usedBuffers map[*resource.Buffer]struct{}

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
	internal []stencilblit.Releasable

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
		cb.device.Recordings().RemoveBuffer(cb)
	}
	// Return encoder to pool (reset native allocator).
	// For multi-CB encoders, pass all HAL command buffers to ResetAll
	// so the underlying pool/allocator can reclaim them.
	if cb.halEncoder != nil && cb.device != nil {
		if pool := cb.device.EncoderPool(); pool != nil {
			cb.halEncoder.ResetAll(cb.HALBufferList())
			pool.Release(cb.halEncoder)
			cb.halEncoder = nil
		}
	}
	// Drop tracked resource refs.
	for _, ref := range cb.trackedRefs {
		ref.Drop()
	}
	cb.trackedRefs = nil
	cb.dropUsedSets()
	cb.ReleaseInternal()
}

// ReleaseInternal releases the frontend's own resources the command buffer's
// commands use. After a submit the submission's references keep them alive
// until the GPU completes it; the queue calls it once its submission holds
// those references.
func (cb *CommandBuffer) ReleaseInternal() {
	releaseInternal(cb.internal)
	cb.internal = nil
}

// MarkSubmitted marks the command buffer submitted and spent: a command
// buffer cannot be submitted twice, so the encode-time validation sets are
// dropped and the buffer leaves the device's recordings registry. The queue
// calls it once the HAL submit succeeded — before its DestroyQueue lookup —
// so the buffer is spent whether or not there is a queue to triage.
func (cb *CommandBuffer) MarkSubmitted() {
	cb.submitted = true
	cb.dropUsedSets()
	if cb.device != nil {
		cb.device.Recordings().RemoveBuffer(cb)
	}
}

// IsSubmitted reports whether the command buffer has been submitted.
func (cb *CommandBuffer) IsSubmitted() bool { return cb.submitted }

// NewTestCommandBuffer builds a command buffer over core state and a device
// seam view, without an encoder behind it. It backs the root package's
// white-box tests of the barrier-injection path, which need a command buffer
// carrying only its core state, the device and the validation sets.
func NewTestCommandBuffer(coreBuf *core.CoreCommandBuffer, device DeviceHost) *CommandBuffer {
	return &CommandBuffer{core: coreBuf, device: device}
}

// SetUsedSetsForTest seeds the encode-time validation sets the encoder
// transfers on Finish. It backs the root package's white-box tests of the
// submit path, which build a command buffer around the sets and check that
// Release and MarkSubmitted drop them.
func (cb *CommandBuffer) SetUsedSetsForTest(buffers map[*resource.Buffer]struct{}, textures map[*resource.Texture]struct{}, groups map[*resource.BindGroup]struct{}) {
	cb.usedBuffers, cb.usedTextures, cb.usedBindGroups = buffers, textures, groups
}

// TrackedRefs returns the ResourceRefs the command buffer holds, or nil. It
// backs the root package's white-box tests.
func (cb *CommandBuffer) TrackedRefs() []*core.ResourceRef { return cb.trackedRefs }

// TakeTrackedRefs returns and clears the ResourceRefs the encoding took. The
// queue hands them to the DestroyQueue so they are Drop'd when the GPU
// completes the submission.
func (cb *CommandBuffer) TakeTrackedRefs() []*core.ResourceRef {
	refs := cb.trackedRefs
	cb.trackedRefs = nil
	return refs
}

// HALEncoder returns the HAL encoder that produced the command buffer, or nil
// once ownership has moved on.
func (cb *CommandBuffer) HALEncoder() hal.CommandEncoder { return cb.halEncoder }

// TakeHALEncoder returns the HAL encoder and clears the field: ownership moves
// to the caller (the queue's deferred recycling after GPU completion).
func (cb *CommandBuffer) TakeHALEncoder() hal.CommandEncoder {
	enc := cb.halEncoder
	cb.halEncoder = nil
	return enc
}

// UsedBuffers returns the buffers referenced during encoding, keyed for
// deduplication (VAL-A6 submit-time validation). A submitted or released
// buffer has nil sets.
func (cb *CommandBuffer) UsedBuffers() map[*resource.Buffer]struct{} { return cb.usedBuffers }

// UsedTextures returns the textures referenced during encoding (VAL-A6).
func (cb *CommandBuffer) UsedTextures() map[*resource.Texture]struct{} { return cb.usedTextures }

// UsedBindGroups returns the bind groups referenced during encoding (VAL-B5).
func (cb *CommandBuffer) UsedBindGroups() map[*resource.BindGroup]struct{} {
	return cb.usedBindGroups
}

// UsedQuerySets returns the query sets referenced during encoding.
func (cb *CommandBuffer) UsedQuerySets() map[*resource.QuerySet]struct{} { return cb.usedQuerySets }

// Core returns the core command buffer, or nil for a zero value. The queue
// reads it for the command buffer's usage scopes.
func (cb *CommandBuffer) Core() *core.CoreCommandBuffer { return cb.core }

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

// HALBufferList returns all HAL command buffers in submission order.
// For single-CB recording (the common case), returns a single-element slice.
// For multi-CB recording (via OpenPass/CloseCB/CloseAndSwap/CloseAndPushFront),
// returns all accumulated CBs.
//
// Reference: Rust wgpu-core BakedCommands.encoder.list (command/mod.rs:742-749)
func (cb *CommandBuffer) HALBufferList() []hal.CommandBuffer {
	if cb == nil || cb.core == nil {
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
			e.SetError(fmt.Errorf("wgpu: CommandEncoder.CopyTextureToTexture: region %d: source or destination texture is nil", i))
			return false
		}
		srcView, srcOK := src.CoreTexture().CopyView(r.Source.MipLevel, r.Source.Origin, r.Source.Aspect)
		dstView, dstOK := dst.CoreTexture().CopyView(r.Destination.MipLevel, r.Destination.Origin, r.Destination.Aspect)
		if !srcOK || !dstOK {
			continue
		}
		sameTexture := src.CoreTexture() == dst.CoreTexture()
		if err := core.ValidateTextureToTextureCopy(sameTexture, srcView, dstView, r.Size); err != nil {
			e.SetError(fmt.Errorf("wgpu: CommandEncoder.CopyTextureToTexture: region %d: %w", i, err))
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
		if !core.IsEmptyExtent(regions[i].Size) {
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
		if !core.IsEmptyExtent(regions[i].Size) {
			kept = append(kept, regions[i])
		}
	}
	return kept, len(regions) > 0 && len(kept) == 0
}

// validateBufferTextureCopies checks every region of a buffer-texture copy
// (validateBufferTextureRegion). side is the texture's side of the copy.
// Returns false, with the error recorded on the encoder, for the first
// invalid region.
func (e *CommandEncoder) validateBufferTextureCopies(op string, side core.CopySide, texture *resource.Texture, buffer *resource.Buffer, regions []BufferTextureCopy) bool {
	for i := range regions {
		if err := validateBufferTextureRegion(side, texture, buffer, &regions[i]); err != nil {
			e.SetError(fmt.Errorf("wgpu: CommandEncoder.%s: region %d: %w", op, i, err))
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
func validateBufferTextureRegion(side core.CopySide, texture *resource.Texture, buffer *resource.Buffer, region *BufferTextureCopy) error {
	size := region.Size
	view, haveView := texture.CoreTexture().CopyView(region.TextureBase.MipLevel, region.TextureBase.Origin, region.TextureBase.Aspect)
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
