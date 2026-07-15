// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows && !(js && wasm)

package dx12

import (
	"fmt"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/dx12/d3d12"
)

// CommandAllocator wraps a D3D12 command allocator.
type CommandAllocator struct {
	raw *d3d12.ID3D12CommandAllocator
}

// CommandBuffer holds a recorded D3D12 command list.
// Allocators are managed by Device frame tracking, not by CommandBuffer.
type CommandBuffer struct {
	cmdList          *d3d12.ID3D12GraphicsCommandList
	stateSummaries   []commandStateSummary
	preambleBarriers []stateBarrierPlan // populated by Queue.Submit for diagnostics/replay
}

// Destroy releases the command buffer's command list.
func (c *CommandBuffer) Destroy() {
	if c.cmdList != nil {
		c.cmdList.Release()
		c.cmdList = nil
	}
}

// CommandEncoder implements hal.CommandEncoder for DirectX 12.
//
// Matches Rust wgpu-hal architecture: each encoder OWNS its ID3D12CommandAllocator
// permanently. Command lists are pooled in freeLists and reused via Reset.
// The allocator is only Reset when the encoder is returned to the pool after
// GPU completion (via ResetAll). This avoids per-frame allocator churn that
// causes TDR on Intel Iris Xe.
type CommandEncoder struct {
	device      *Device
	allocator   *d3d12.ID3D12CommandAllocator // Owned permanently by this encoder
	cmdList     *d3d12.ID3D12GraphicsCommandList
	freeLists   []*d3d12.ID3D12GraphicsCommandList // Pool of reusable command lists
	label       string
	isRecording bool

	// descriptorHeaps is a pre-allocated array for SetDescriptorHeaps calls,
	// avoiding a slice allocation per render/compute pass. At most 2 heaps
	// are needed: one for CBV/SRV/UAV views and one for samplers.
	descriptorHeaps     [2]*d3d12.ID3D12DescriptorHeap
	descriptorHeapCount int

	stateTracker commandStateTracker
}

// BeginEncoding begins command recording.
// Reuses a command list from freeLists (Reset with owned allocator) or creates new.
// The allocator is permanently owned by this encoder — not acquired per-call.
func (e *CommandEncoder) BeginEncoding(label string) error {
	e.label = label

	// Try reusing a command list from the free pool.
	if len(e.freeLists) > 0 {
		list := e.freeLists[len(e.freeLists)-1]
		e.freeLists = e.freeLists[:len(e.freeLists)-1]
		if err := list.Reset(e.allocator, nil); err == nil {
			e.cmdList = list
			e.isRecording = true
			e.stateTracker.reset()
			return nil
		}
		// Reset failed — discard this list, try next or create new.
		list.Release()
	}

	// No reusable lists — create a new one with our owned allocator.
	cmdList, err := e.device.raw.CreateCommandList(0, d3d12.D3D12_COMMAND_LIST_TYPE_DIRECT, e.allocator, nil)
	if err != nil {
		return fmt.Errorf("dx12: CreateCommandList failed: %w", err)
	}
	e.cmdList = cmdList
	e.isRecording = true
	e.stateTracker.reset()
	return nil
}

// EndEncoding finishes command recording and returns a command buffer.
// The command list is detached from the encoder — it will be returned
// to freeLists when ResetAll is called after GPU completion.
func (e *CommandEncoder) EndEncoding() (hal.CommandBuffer, error) {
	if !e.isRecording {
		return nil, fmt.Errorf("dx12: command encoder is not recording")
	}

	hal.Logger().Debug("dx12: command list close", "label", e.label)
	if err := e.cmdList.Close(); err != nil {
		return nil, fmt.Errorf("dx12: command list close failed: %w", err)
	}

	e.isRecording = false
	cb := &CommandBuffer{cmdList: e.cmdList, stateSummaries: e.stateTracker.summary()}
	e.cmdList = nil // Detach — owned by CommandBuffer until ResetAll returns it.
	return cb, nil
}

// DiscardEncoding discards the encoder without creating a command buffer.
func (e *CommandEncoder) DiscardEncoding() {
	if e.isRecording {
		// Close the command list even though we're discarding it
		_ = e.cmdList.Close()
		e.isRecording = false
	}
}

// ResetAll resets the encoder's allocator and returns command lists to the free pool.
// Called after GPU confirms completion of all commands recorded by this encoder.
// Matches Rust wgpu-hal CommandEncoder::reset_all (command.rs:442-450).
func (e *CommandEncoder) ResetAll(commandBuffers []hal.CommandBuffer) {
	// Return command lists to the free pool for reuse.
	for _, cb := range commandBuffers {
		if dx12CB, ok := cb.(*CommandBuffer); ok && dx12CB.cmdList != nil {
			e.freeLists = append(e.freeLists, dx12CB.cmdList)
			dx12CB.cmdList = nil // Prevent double-free via CommandBuffer.Destroy.
		}
	}
	// Reset the owned allocator. This frees the internal memory used by
	// previously recorded commands. Safe because GPU is done with them.
	if e.allocator != nil {
		if err := e.allocator.Reset(); err != nil {
			hal.Logger().Error("dx12: ID3D12CommandAllocator::Reset failed", "err", err)
		}
	}
}

// Destroy releases the allocator and all pooled command lists.
// Must be called when the encoder is permanently retired (e.g., device shutdown).
func (e *CommandEncoder) Destroy() {
	for _, list := range e.freeLists {
		list.Release()
	}
	e.freeLists = nil
	if e.cmdList != nil {
		e.cmdList.Release()
		e.cmdList = nil
	}
	if e.allocator != nil {
		e.allocator.Release()
		e.allocator = nil
	}
}

// TransitionBuffers transitions buffer states for synchronization.
func (e *CommandEncoder) TransitionBuffers(barriers []hal.BufferBarrier) {
	if !e.isRecording || len(barriers) == 0 {
		return
	}

	plans := make([]stateBarrierPlan, 0, len(barriers))
	for _, b := range barriers {
		buf, ok := b.Buffer.(*Buffer)
		if !ok || buf.raw == nil {
			hal.Logger().Warn("TransitionBuffers: skipping invalid buffer (nil or destroyed)")
			continue
		}

		afterState := bufferUsageToD3D12State(b.Usage.NewUsage)
		beforeState, needsBarrier := e.stateTracker.transitionBuffer(buf, afterState)
		if needsBarrier {
			plans = append(plans, stateBarrierPlan{resource: buf, subresource: d3d12.D3D12_RESOURCE_BARRIER_ALL_SUBRESOURCES, before: beforeState, after: afterState})
		}
	}
	e.emitStateBarrierPlans(plans)
}

// TransitionTextures transitions texture states for synchronization.
func (e *CommandEncoder) TransitionTextures(barriers []hal.TextureBarrier) {
	if !e.isRecording || len(barriers) == 0 {
		return
	}

	plans := make([]stateBarrierPlan, 0, len(barriers))
	for _, b := range barriers {
		tex, ok := b.Texture.(*Texture)
		if !ok || tex.raw == nil {
			hal.Logger().Warn("TransitionTextures: skipping invalid texture (nil or destroyed)")
			continue
		}

		afterState := textureUsageToD3D12State(b.Usage.NewUsage)
		for _, plan := range e.stateTracker.transitionTextureRange(tex, textureRangeSubresources(tex, b.Range), afterState) {
			plans = append(plans, plan)
		}
	}
	e.emitStateBarrierPlans(plans)
}

func (e *CommandEncoder) emitStateBarrierPlans(plans []stateBarrierPlan) {
	if !e.isRecording || len(plans) == 0 {
		return
	}
	d3dBarriers := make([]d3d12.D3D12_RESOURCE_BARRIER, 0, len(plans))
	for _, plan := range plans {
		var raw *d3d12.ID3D12Resource
		switch resource := plan.resource.(type) {
		case *Buffer:
			raw = resource.raw
		case *Texture:
			raw = resource.raw
		}
		if raw == nil || plan.before == plan.after {
			continue
		}
		d3dBarriers = append(d3dBarriers, d3d12.NewTransitionBarrier(raw, plan.before, plan.after, plan.subresource))
	}
	if len(d3dBarriers) > 0 {
		hal.Logger().Debug("dx12: resource barrier", "label", e.label, "count", len(d3dBarriers))
		e.cmdList.ResourceBarrier(uint32(len(d3dBarriers)), &d3dBarriers[0])
	}
}

// ClearBuffer clears a buffer region to zero.
func (e *CommandEncoder) ClearBuffer(buffer hal.Buffer, offset, size uint64) {
	if !e.isRecording {
		return
	}

	buf, ok := buffer.(*Buffer)
	if !ok {
		return
	}

	// D3D12 doesn't have a direct ClearBuffer command
	// We need to either:
	// 1. Use ClearUnorderedAccessViewUint (requires UAV)
	// 2. Use a compute shader
	// 3. Use CopyBufferRegion from a zero-filled buffer
	// For now, we'll use UAV clear if the buffer supports it

	if buf.usage&gputypes.BufferUsageStorage != 0 {
		// Note: UAV clear requires ClearUnorderedAccessViewUint/Float.
		// This requires setting up a UAV descriptor and calling ClearUnorderedAccessViewUint
		_ = offset
		_ = size
	}
	// For buffers without storage usage, we'd need a different approach
}

// CopyBufferToBuffer copies data between buffers.
// Inserts D3D12_RESOURCE_BARRIER transitions when buffers are not already in
// the required state (COPY_SOURCE for src, COPY_DEST for dst). This is the
// DX12-specific fix for BUG-DX12-012: missing UNORDERED_ACCESS -> COPY_SOURCE
// barrier after compute dispatch causes DEVICE_REMOVED.
func (e *CommandEncoder) CopyBufferToBuffer(src, dst hal.Buffer, regions []hal.BufferCopy) {
	if !e.isRecording {
		return
	}

	srcBuf, srcOk := src.(*Buffer)
	dstBuf, dstOk := dst.(*Buffer)
	if !srcOk || !dstOk {
		return
	}

	plans := make([]stateBarrierPlan, 0, 2)
	if before, needsBarrier := e.stateTracker.transitionBuffer(srcBuf, d3d12.D3D12_RESOURCE_STATE_COPY_SOURCE); needsBarrier {
		plans = append(plans, stateBarrierPlan{resource: srcBuf, subresource: d3d12.D3D12_RESOURCE_BARRIER_ALL_SUBRESOURCES, before: before, after: d3d12.D3D12_RESOURCE_STATE_COPY_SOURCE})
	}
	if before, needsBarrier := e.stateTracker.transitionBuffer(dstBuf, d3d12.D3D12_RESOURCE_STATE_COPY_DEST); needsBarrier {
		plans = append(plans, stateBarrierPlan{resource: dstBuf, subresource: d3d12.D3D12_RESOURCE_BARRIER_ALL_SUBRESOURCES, before: before, after: d3d12.D3D12_RESOURCE_STATE_COPY_DEST})
	}
	e.emitStateBarrierPlans(plans)

	for _, r := range regions {
		hal.Logger().Debug("dx12: copy buffer region", "label", e.label, "offset", r.DstOffset, "size", r.Size)
		e.cmdList.CopyBufferRegion(dstBuf.raw, r.DstOffset, srcBuf.raw, r.SrcOffset, r.Size)
	}
}

// CopyBufferToTexture copies data from a buffer to a texture.
// Inserts a transition barrier if the source buffer is not in COPY_SOURCE state.
func (e *CommandEncoder) CopyBufferToTexture(src hal.Buffer, dst hal.Texture, regions []hal.BufferTextureCopy) {
	if !e.isRecording {
		return
	}

	srcBuf, srcOk := src.(*Buffer)
	dstTex, dstOk := dst.(*Texture)
	if !srcOk || !dstOk {
		return
	}

	plans := make([]stateBarrierPlan, 0, 1+len(regions))
	if before, needsBarrier := e.stateTracker.transitionBuffer(srcBuf, d3d12.D3D12_RESOURCE_STATE_COPY_SOURCE); needsBarrier {
		plans = append(plans, stateBarrierPlan{resource: srcBuf, subresource: d3d12.D3D12_RESOURCE_BARRIER_ALL_SUBRESOURCES, before: before, after: d3d12.D3D12_RESOURCE_STATE_COPY_SOURCE})
	}
	for _, r := range regions {
		for _, copyPlan := range planBufferTextureCopies(dstTex, r.TextureBase, r.BufferLayout, r.Size) {
			if before, needsBarrier := e.stateTracker.transitionTexture(dstTex, copyPlan.subresource, d3d12.D3D12_RESOURCE_STATE_COPY_DEST); needsBarrier {
				plans = append(plans, stateBarrierPlan{resource: dstTex, subresource: copyPlan.subresource, before: before, after: d3d12.D3D12_RESOURCE_STATE_COPY_DEST})
			}
		}
	}
	e.emitStateBarrierPlans(plans)

	for _, r := range regions {
		for _, copyPlan := range planBufferTextureCopies(dstTex, r.TextureBase, r.BufferLayout, r.Size) {
			// A 2D array layer is a distinct subresource and therefore needs a
			// distinct placed footprint with depth=1.
			srcLoc := d3d12.D3D12_TEXTURE_COPY_LOCATION{
				Resource: srcBuf.raw,
				Type:     d3d12.D3D12_TEXTURE_COPY_TYPE_PLACED_FOOTPRINT,
			}
			srcLoc.SetPlacedFootprint(d3d12.D3D12_PLACED_SUBRESOURCE_FOOTPRINT{
				Offset: copyPlan.bufferOffset,
				Footprint: d3d12.D3D12_SUBRESOURCE_FOOTPRINT{
					Format:   textureFormatToD3D12(dstTex.format),
					Width:    r.Size.Width,
					Height:   copyPlan.footprintHeight,
					Depth:    copyPlan.footprintDepth,
					RowPitch: r.BufferLayout.BytesPerRow,
				},
			})

			dstLoc := d3d12.D3D12_TEXTURE_COPY_LOCATION{
				Resource: dstTex.raw,
				Type:     d3d12.D3D12_TEXTURE_COPY_TYPE_SUBRESOURCE_INDEX,
			}
			dstLoc.SetSubresourceIndex(copyPlan.subresource)
			srcBox := d3d12.D3D12_BOX{
				Left:   0,
				Top:    copyPlan.bufferOriginY,
				Front:  0,
				Right:  r.Size.Width,
				Bottom: copyPlan.bufferOriginY + r.Size.Height,
				Back:   copyPlan.footprintDepth,
			}

			e.cmdList.CopyTextureRegion(
				&dstLoc,
				r.TextureBase.Origin.X, r.TextureBase.Origin.Y, copyPlan.textureOriginZ,
				&srcLoc,
				&srcBox,
			)
		}
	}
}

// CopyTextureToBuffer copies data from a texture to a buffer.
// Inserts a transition barrier if the destination buffer is not in COPY_DEST state.
func (e *CommandEncoder) CopyTextureToBuffer(src hal.Texture, dst hal.Buffer, regions []hal.BufferTextureCopy) {
	if !e.isRecording {
		return
	}

	srcTex, srcOk := src.(*Texture)
	dstBuf, dstOk := dst.(*Buffer)
	if !srcOk || !dstOk {
		return
	}

	plans := make([]stateBarrierPlan, 0, 1+len(regions))
	if before, needsBarrier := e.stateTracker.transitionBuffer(dstBuf, d3d12.D3D12_RESOURCE_STATE_COPY_DEST); needsBarrier {
		plans = append(plans, stateBarrierPlan{resource: dstBuf, subresource: d3d12.D3D12_RESOURCE_BARRIER_ALL_SUBRESOURCES, before: before, after: d3d12.D3D12_RESOURCE_STATE_COPY_DEST})
	}
	for _, r := range regions {
		for _, copyPlan := range planBufferTextureCopies(srcTex, r.TextureBase, r.BufferLayout, r.Size) {
			if before, needsBarrier := e.stateTracker.transitionTexture(srcTex, copyPlan.subresource, d3d12.D3D12_RESOURCE_STATE_COPY_SOURCE); needsBarrier {
				plans = append(plans, stateBarrierPlan{resource: srcTex, subresource: copyPlan.subresource, before: before, after: d3d12.D3D12_RESOURCE_STATE_COPY_SOURCE})
			}
		}
	}
	e.emitStateBarrierPlans(plans)

	for _, r := range regions {
		// D3D12 requires RowPitch aligned to 256 bytes.
		// The caller should pass aligned BytesPerRow, but align defensively.
		rowPitch := (r.BufferLayout.BytesPerRow + d3d12TexturePitchAlignment - 1) &^ (d3d12TexturePitchAlignment - 1)
		for _, copyPlan := range planBufferTextureCopies(srcTex, r.TextureBase, r.BufferLayout, r.Size) {
			srcLoc := d3d12.D3D12_TEXTURE_COPY_LOCATION{
				Resource: srcTex.raw,
				Type:     d3d12.D3D12_TEXTURE_COPY_TYPE_SUBRESOURCE_INDEX,
			}
			srcLoc.SetSubresourceIndex(copyPlan.subresource)

			dstLoc := d3d12.D3D12_TEXTURE_COPY_LOCATION{
				Resource: dstBuf.raw,
				Type:     d3d12.D3D12_TEXTURE_COPY_TYPE_PLACED_FOOTPRINT,
			}
			dstLoc.SetPlacedFootprint(d3d12.D3D12_PLACED_SUBRESOURCE_FOOTPRINT{
				Offset: copyPlan.bufferOffset,
				Footprint: d3d12.D3D12_SUBRESOURCE_FOOTPRINT{
					Format:   textureFormatToD3D12(srcTex.format),
					Width:    r.Size.Width,
					Height:   copyPlan.footprintHeight,
					Depth:    copyPlan.footprintDepth,
					RowPitch: rowPitch,
				},
			})

			srcBox := d3d12.D3D12_BOX{
				Left:   r.TextureBase.Origin.X,
				Top:    r.TextureBase.Origin.Y,
				Front:  copyPlan.textureOriginZ,
				Right:  r.TextureBase.Origin.X + r.Size.Width,
				Bottom: r.TextureBase.Origin.Y + r.Size.Height,
				Back:   copyPlan.textureOriginZ + copyPlan.footprintDepth,
			}

			e.cmdList.CopyTextureRegion(&dstLoc, 0, copyPlan.bufferOriginY, 0, &srcLoc, &srcBox)
		}
	}
}

// CopyTextureToTexture copies data between textures.
func (e *CommandEncoder) CopyTextureToTexture(src, dst hal.Texture, regions []hal.TextureCopy) {
	if !e.isRecording {
		return
	}

	srcTex, srcOk := src.(*Texture)
	dstTex, dstOk := dst.(*Texture)
	if !srcOk || !dstOk {
		return
	}

	plans := make([]stateBarrierPlan, 0, len(regions)*2)
	for _, r := range regions {
		for _, copyPlan := range planTextureTextureCopies(srcTex, dstTex, r) {
			if before, needsBarrier := e.stateTracker.transitionTexture(srcTex, copyPlan.srcSubresource, d3d12.D3D12_RESOURCE_STATE_COPY_SOURCE); needsBarrier {
				plans = append(plans, stateBarrierPlan{resource: srcTex, subresource: copyPlan.srcSubresource, before: before, after: d3d12.D3D12_RESOURCE_STATE_COPY_SOURCE})
			}
			if before, needsBarrier := e.stateTracker.transitionTexture(dstTex, copyPlan.dstSubresource, d3d12.D3D12_RESOURCE_STATE_COPY_DEST); needsBarrier {
				plans = append(plans, stateBarrierPlan{resource: dstTex, subresource: copyPlan.dstSubresource, before: before, after: d3d12.D3D12_RESOURCE_STATE_COPY_DEST})
			}
		}
	}
	e.emitStateBarrierPlans(plans)

	for _, r := range regions {
		for _, copyPlan := range planTextureTextureCopies(srcTex, dstTex, r) {
			srcLoc := d3d12.D3D12_TEXTURE_COPY_LOCATION{
				Resource: srcTex.raw,
				Type:     d3d12.D3D12_TEXTURE_COPY_TYPE_SUBRESOURCE_INDEX,
			}
			srcLoc.SetSubresourceIndex(copyPlan.srcSubresource)

			dstLoc := d3d12.D3D12_TEXTURE_COPY_LOCATION{
				Resource: dstTex.raw,
				Type:     d3d12.D3D12_TEXTURE_COPY_TYPE_SUBRESOURCE_INDEX,
			}
			dstLoc.SetSubresourceIndex(copyPlan.dstSubresource)

			srcBox := d3d12.D3D12_BOX{
				Left:   r.SrcBase.Origin.X,
				Top:    r.SrcBase.Origin.Y,
				Front:  copyPlan.srcFront,
				Right:  r.SrcBase.Origin.X + r.Size.Width,
				Bottom: r.SrcBase.Origin.Y + r.Size.Height,
				Back:   copyPlan.srcBack,
			}

			e.cmdList.CopyTextureRegion(
				&dstLoc,
				r.DstBase.Origin.X, r.DstBase.Origin.Y, copyPlan.dstZ,
				&srcLoc,
				&srcBox,
			)
		}
	}
}

// ResolveQuerySet copies query results from a query set into a destination buffer.
// Each timestamp result is a uint64 (8 bytes).
// Rust wgpu-hal reference: dx12/command.rs copy_query_results.
func (e *CommandEncoder) ResolveQuerySet(querySet hal.QuerySet, firstQuery, queryCount uint32, destination hal.Buffer, destinationOffset uint64) {
	if !e.isRecording {
		return
	}
	qs, ok := querySet.(*QuerySet)
	if !ok || qs == nil || qs.raw == nil {
		return
	}
	buf, ok := destination.(*Buffer)
	if !ok || buf == nil || buf.raw == nil {
		return
	}
	if before, needsBarrier := e.stateTracker.transitionBuffer(buf, d3d12.D3D12_RESOURCE_STATE_COPY_DEST); needsBarrier {
		e.emitStateBarrierPlans([]stateBarrierPlan{{resource: buf, subresource: d3d12.D3D12_RESOURCE_BARRIER_ALL_SUBRESOURCES, before: before, after: d3d12.D3D12_RESOURCE_STATE_COPY_DEST}})
	}
	e.cmdList.ResolveQueryData(qs.raw, qs.rawTy, firstQuery, queryCount, buf.raw, destinationOffset)
}

// timestampWrites is a common interface for render/compute pass timestamp writes.
// Both hal.RenderPassTimestampWrites and hal.ComputePassTimestampWrites share
// the same fields; this interface avoids duplicating extraction logic.
type timestampWrites interface {
	querySet() hal.QuerySet
	beginIndex() *uint32
	endIndex() *uint32
}

type renderTSW struct {
	w *hal.RenderPassTimestampWrites
}

func (r renderTSW) querySet() hal.QuerySet { return r.w.QuerySet }
func (r renderTSW) beginIndex() *uint32    { return r.w.BeginningOfPassWriteIndex }
func (r renderTSW) endIndex() *uint32      { return r.w.EndOfPassWriteIndex }

type computeTSW struct {
	w *hal.ComputePassTimestampWrites
}

func (c computeTSW) querySet() hal.QuerySet { return c.w.QuerySet }
func (c computeTSW) beginIndex() *uint32    { return c.w.BeginningOfPassWriteIndex }
func (c computeTSW) endIndex() *uint32      { return c.w.EndOfPassWriteIndex }

// writeBeginTimestamp writes the beginning-of-pass timestamp (if requested) and
// returns the query heap + index for the end-of-pass timestamp write.
// DX12 uses EndQuery for timestamps (NOT begin/end pair).
// Rust wgpu-hal reference: dx12/command.rs begin_render_pass / begin_compute_pass.
func (e *CommandEncoder) writeBeginTimestamp(halQS hal.QuerySet, tw timestampWrites) (*d3d12.ID3D12QueryHeap, uint32) {
	qs, ok := halQS.(*QuerySet)
	if !ok || qs == nil || qs.raw == nil {
		return nil, 0
	}

	if idx := tw.beginIndex(); idx != nil {
		e.cmdList.EndQuery(qs.raw, d3d12.D3D12_QUERY_TYPE_TIMESTAMP, *idx)
	}

	if idx := tw.endIndex(); idx != nil {
		return qs.raw, *idx
	}
	return nil, 0
}

// BeginRenderPass begins a render pass.
func (e *CommandEncoder) BeginRenderPass(desc *hal.RenderPassDescriptor) hal.RenderPassEncoder {
	rpe := &RenderPassEncoder{
		encoder: e,
		desc:    desc,
	}

	hal.Logger().Debug("dx12: begin render pass", "label", e.label, "attachments", len(desc.ColorAttachments))

	if !e.isRecording {
		return rpe
	}

	// Render attachments are first-use requirements in the command-local
	// tracker. Surface views share the same owner as their swapchain back
	// buffer, so PRESENT is captured from that owner rather than guessed here.
	attachmentPlans := make([]stateBarrierPlan, 0)
	for _, ca := range desc.ColorAttachments {
		view, ok := ca.View.(*TextureView)
		if !ok || view.texture == nil || view.texture.raw == nil {
			continue
		}
		for _, subresource := range textureViewSubresources(view) {
			if before, needsBarrier := e.stateTracker.transitionTexture(view.texture, subresource, d3d12.D3D12_RESOURCE_STATE_RENDER_TARGET); needsBarrier {
				attachmentPlans = append(attachmentPlans, stateBarrierPlan{resource: view.texture, subresource: subresource, before: before, after: d3d12.D3D12_RESOURCE_STATE_RENDER_TARGET})
			}
		}
	}
	e.emitStateBarrierPlans(attachmentPlans)

	// Set render targets
	rtvHandles := make([]d3d12.D3D12_CPU_DESCRIPTOR_HANDLE, 0, len(desc.ColorAttachments))
	for _, ca := range desc.ColorAttachments {
		view, ok := ca.View.(*TextureView)
		if !ok || !view.hasRTV {
			continue
		}
		rtvHandles = append(rtvHandles, view.rtvHandle)

		// Clear if needed
		if ca.LoadOp == gputypes.LoadOpClear {
			clearColor := [4]float32{
				float32(ca.ClearValue.R),
				float32(ca.ClearValue.G),
				float32(ca.ClearValue.B),
				float32(ca.ClearValue.A),
			}
			e.cmdList.ClearRenderTargetView(view.rtvHandle, &clearColor, 0, nil)
		}
	}

	// Handle depth/stencil attachment using helper method to reduce nesting
	dsvHandle := e.setupDepthStencilAttachment(desc.DepthStencilAttachment)

	// Set render targets
	if len(rtvHandles) > 0 {
		e.cmdList.OMSetRenderTargets(uint32(len(rtvHandles)), &rtvHandles[0], 0, dsvHandle)
	} else if dsvHandle != nil {
		e.cmdList.OMSetRenderTargets(0, nil, 0, dsvHandle)
	}

	// Set default viewport and scissor based on first color attachment or depth attachment
	var width, height uint32
	if len(desc.ColorAttachments) > 0 {
		if view, ok := desc.ColorAttachments[0].View.(*TextureView); ok {
			width = view.texture.size.Width >> view.baseMip
			height = view.texture.size.Height >> view.baseMip
		}
	} else if desc.DepthStencilAttachment != nil {
		if view, ok := desc.DepthStencilAttachment.View.(*TextureView); ok {
			width = view.texture.size.Width >> view.baseMip
			height = view.texture.size.Height >> view.baseMip
		}
	}

	if width > 0 && height > 0 {
		viewport := d3d12.D3D12_VIEWPORT{
			TopLeftX: 0,
			TopLeftY: 0,
			Width:    float32(width),
			Height:   float32(height),
			MinDepth: 0,
			MaxDepth: 1,
		}
		e.cmdList.RSSetViewports(1, &viewport)

		scissor := d3d12.D3D12_RECT{
			Left:   0,
			Top:    0,
			Right:  int32(width),
			Bottom: int32(height),
		}
		e.cmdList.RSSetScissorRects(1, &scissor)
	}

	// Write beginning-of-pass timestamp and store end-of-pass for later.
	if desc.TimestampWrites != nil {
		tw := renderTSW{desc.TimestampWrites}
		rpe.endOfPassTimerHeap, rpe.endOfPassTimerIndex = e.writeBeginTimestamp(tw.querySet(), tw)
	}

	return rpe
}

// BeginComputePass begins a compute pass.
func (e *CommandEncoder) BeginComputePass(desc *hal.ComputePassDescriptor) hal.ComputePassEncoder {
	cpe := &ComputePassEncoder{
		encoder: e,
	}

	if e.isRecording && desc != nil && desc.TimestampWrites != nil {
		tw := computeTSW{desc.TimestampWrites}
		cpe.endOfPassTimerHeap, cpe.endOfPassTimerIndex = e.writeBeginTimestamp(tw.querySet(), tw)
	}

	return cpe
}

// RenderPassEncoder implements hal.RenderPassEncoder for DirectX 12.
type RenderPassEncoder struct {
	encoder            *CommandEncoder
	desc               *hal.RenderPassDescriptor
	pipeline           *RenderPipeline
	indexFormat        gputypes.IndexFormat
	descriptorHeapsSet bool // Tracks whether descriptor heaps have been bound

	// endOfPassTimerQuery stores the query heap and index for the end-of-pass
	// timestamp write. Set during BeginRenderPass, consumed in End().
	// Rust wgpu-hal reference: dx12/mod.rs end_of_pass_timer_query field.
	endOfPassTimerHeap  *d3d12.ID3D12QueryHeap
	endOfPassTimerIndex uint32
}

// End finishes the render pass.
// Handles MSAA resolve (if ResolveTarget is set), writes end-of-pass
// timestamp, and transitions surface textures back to PRESENT state.
func (e *RenderPassEncoder) End() {
	if e.desc == nil || e.encoder == nil || !e.encoder.isRecording {
		return
	}

	// Write end-of-pass timestamp before state transitions.
	// Rust wgpu-hal reference: dx12/command.rs end_render_pass calls
	// write_pass_end_timestamp_if_requested after resolves but before end_pass.
	if e.endOfPassTimerHeap != nil {
		e.encoder.cmdList.EndQuery(e.endOfPassTimerHeap, d3d12.D3D12_QUERY_TYPE_TIMESTAMP, e.endOfPassTimerIndex)
		e.endOfPassTimerHeap = nil
	}

	for _, ca := range e.desc.ColorAttachments {
		msaaView, ok := ca.View.(*TextureView)
		if !ok || msaaView.texture == nil || msaaView.texture.raw == nil {
			continue
		}

		// Check for MSAA resolve target.
		resolveView, _ := ca.ResolveTarget.(*TextureView)
		if resolveView != nil && resolveView.texture != nil && resolveView.texture.raw != nil && msaaView.texture.samples > 1 {
			// Determine resolve target's resting state based on ownership.
			// Surface (swapchain) textures live in PRESENT between frames.
			// Internal (offscreen) textures live in RENDER_TARGET, matching
			// the initial state set by CreateTexture for RenderAttachment usage.
			resolveRestState := d3d12.D3D12_RESOURCE_STATE_RENDER_TARGET
			if resolveView.texture.isExternal {
				resolveRestState = d3d12.D3D12_RESOURCE_STATE_PRESENT
			}

			// MSAA resolve: render target → resolve source, resolve target → resolve dest.
			msaaSubresources := textureViewSubresources(msaaView)
			resolveSubresources := textureViewSubresources(resolveView)
			plans := make([]stateBarrierPlan, 0, len(msaaSubresources)+len(resolveSubresources))
			for _, subresource := range msaaSubresources {
				if before, needsBarrier := e.encoder.stateTracker.transitionTexture(msaaView.texture, subresource, d3d12.D3D12_RESOURCE_STATE_RESOLVE_SOURCE); needsBarrier {
					plans = append(plans, stateBarrierPlan{resource: msaaView.texture, subresource: subresource, before: before, after: d3d12.D3D12_RESOURCE_STATE_RESOLVE_SOURCE})
				}
			}
			for _, subresource := range resolveSubresources {
				if before, needsBarrier := e.encoder.stateTracker.transitionTexture(resolveView.texture, subresource, d3d12.D3D12_RESOURCE_STATE_RESOLVE_DEST); needsBarrier {
					plans = append(plans, stateBarrierPlan{resource: resolveView.texture, subresource: subresource, before: before, after: d3d12.D3D12_RESOURCE_STATE_RESOLVE_DEST})
				}
			}
			e.encoder.emitStateBarrierPlans(plans)

			// Resolve MSAA → single-sample.
			format := textureFormatToD3D12(msaaView.texture.format)
			e.encoder.cmdList.ResolveSubresource(
				resolveView.texture.raw, resolveSubresourceIndex(resolveView),
				msaaView.texture.raw, resolveSubresourceIndex(msaaView),
				format,
			)

			// Return the resources to their command-local resting states.
			plans = plans[:0]
			for _, subresource := range msaaSubresources {
				if before, needsBarrier := e.encoder.stateTracker.transitionTexture(msaaView.texture, subresource, d3d12.D3D12_RESOURCE_STATE_RENDER_TARGET); needsBarrier {
					plans = append(plans, stateBarrierPlan{resource: msaaView.texture, subresource: subresource, before: before, after: d3d12.D3D12_RESOURCE_STATE_RENDER_TARGET})
				}
			}
			for _, subresource := range resolveSubresources {
				if before, needsBarrier := e.encoder.stateTracker.transitionTexture(resolveView.texture, subresource, resolveRestState); needsBarrier {
					plans = append(plans, stateBarrierPlan{resource: resolveView.texture, subresource: subresource, before: before, after: resolveRestState})
				}
			}
			e.encoder.emitStateBarrierPlans(plans)
			continue
		}

		// No resolve — just transition external surface back to PRESENT.
		if msaaView.texture.isExternal {
			for _, subresource := range textureViewSubresources(msaaView) {
				if before, needsBarrier := e.encoder.stateTracker.transitionTexture(msaaView.texture, subresource, d3d12.D3D12_RESOURCE_STATE_PRESENT); needsBarrier {
					e.encoder.emitStateBarrierPlans([]stateBarrierPlan{{resource: msaaView.texture, subresource: subresource, before: before, after: d3d12.D3D12_RESOURCE_STATE_PRESENT}})
				}
			}
		}
	}
}

// SetPipeline sets the render pipeline.
func (e *RenderPassEncoder) SetPipeline(pipeline hal.RenderPipeline) {
	p, ok := pipeline.(*RenderPipeline)
	if !ok || !e.encoder.isRecording {
		return
	}
	e.pipeline = p

	e.encoder.cmdList.SetPipelineState(p.pso)
	if p.rootSignature != nil {
		e.encoder.cmdList.SetGraphicsRootSignature(p.rootSignature)
	}
	e.encoder.cmdList.IASetPrimitiveTopology(p.topology)

	// Bind the global sampler descriptor heap to the sampler root parameter.
	// This is the ONE root parameter that covers all 2048+2048 sampler slots.
	if p.samplerRootIndex >= 0 && e.encoder.device.samplerHeap != nil {
		e.encoder.ensureDescriptorHeapsBound()
		e.descriptorHeapsSet = true
		e.encoder.cmdList.SetGraphicsRootDescriptorTable(
			uint32(p.samplerRootIndex),
			e.encoder.device.samplerHeap.gpuStart,
		)
	}
}

// SetBindGroup sets a bind group for graphics operations.
// index is the bind group slot (0-3 typically).
// group contains the GPU descriptor handles for resources.
// offsets are dynamic buffer offsets (used for dynamic uniform/storage buffers).
func (e *RenderPassEncoder) SetBindGroup(index uint32, group hal.BindGroup, offsets []uint32) {
	bg, ok := group.(*BindGroup)
	if !ok || !e.encoder.isRecording {
		return
	}

	// Ensure descriptor heaps are bound before setting descriptor tables.
	if !e.descriptorHeapsSet {
		e.encoder.ensureDescriptorHeapsBound()
		e.descriptorHeapsSet = true
	}

	// Get group mappings from the current pipeline.
	var mappings []rootParamMapping
	if e.pipeline != nil {
		mappings = e.pipeline.groupMappings
	}

	// Bind the group using graphics root descriptor tables.
	e.encoder.bindGroupToRootTables(index, bg, false, mappings)
	e.encoder.trackBindGroupState(bg)
	_ = offsets // Dynamic offsets handled via root constants (simplified for now)
}

// SetVertexBuffer sets a vertex buffer.
func (e *RenderPassEncoder) SetVertexBuffer(slot uint32, buffer hal.Buffer, offset uint64) {
	buf, ok := buffer.(*Buffer)
	if !ok || !e.encoder.isRecording {
		return
	}
	if before, target, needsBarrier := e.encoder.stateTracker.transitionBufferRead(buf, d3d12.D3D12_RESOURCE_STATE_VERTEX_AND_CONSTANT_BUFFER); needsBarrier {
		e.encoder.emitStateBarrierPlans([]stateBarrierPlan{{resource: buf, subresource: d3d12.D3D12_RESOURCE_BARRIER_ALL_SUBRESOURCES, before: before, after: target}})
	}

	// Get stride from pipeline if available
	var stride uint32
	if e.pipeline != nil && slot < uint32(len(e.pipeline.vertexStrides)) {
		stride = e.pipeline.vertexStrides[slot]
	}

	vbv := d3d12.D3D12_VERTEX_BUFFER_VIEW{
		BufferLocation: buf.gpuVA + offset,
		SizeInBytes:    uint32(buf.size - offset),
		StrideInBytes:  stride,
	}

	e.encoder.cmdList.IASetVertexBuffers(slot, 1, &vbv)
}

// SetIndexBuffer sets the index buffer.
func (e *RenderPassEncoder) SetIndexBuffer(buffer hal.Buffer, format gputypes.IndexFormat, offset uint64) {
	buf, ok := buffer.(*Buffer)
	if !ok || !e.encoder.isRecording {
		return
	}
	if before, target, needsBarrier := e.encoder.stateTracker.transitionBufferRead(buf, d3d12.D3D12_RESOURCE_STATE_INDEX_BUFFER); needsBarrier {
		e.encoder.emitStateBarrierPlans([]stateBarrierPlan{{resource: buf, subresource: d3d12.D3D12_RESOURCE_BARRIER_ALL_SUBRESOURCES, before: before, after: target}})
	}

	e.indexFormat = format
	dxgiFormat := d3d12.DXGI_FORMAT_R16_UINT
	if format == gputypes.IndexFormatUint32 {
		dxgiFormat = d3d12.DXGI_FORMAT_R32_UINT
	}

	ibv := d3d12.D3D12_INDEX_BUFFER_VIEW{
		BufferLocation: buf.gpuVA + offset,
		SizeInBytes:    uint32(buf.size - offset),
		Format:         dxgiFormat,
	}

	e.encoder.cmdList.IASetIndexBuffer(&ibv)
}

// SetViewport sets the viewport.
func (e *RenderPassEncoder) SetViewport(x, y, width, height, minDepth, maxDepth float32) {
	if !e.encoder.isRecording {
		return
	}

	viewport := d3d12.D3D12_VIEWPORT{
		TopLeftX: x,
		TopLeftY: y,
		Width:    width,
		Height:   height,
		MinDepth: minDepth,
		MaxDepth: maxDepth,
	}

	e.encoder.cmdList.RSSetViewports(1, &viewport)
}

// SetScissorRect sets the scissor rectangle.
func (e *RenderPassEncoder) SetScissorRect(x, y, width, height uint32) {
	if !e.encoder.isRecording {
		return
	}

	scissor := d3d12.D3D12_RECT{
		Left:   int32(x),
		Top:    int32(y),
		Right:  int32(x + width),
		Bottom: int32(y + height),
	}

	e.encoder.cmdList.RSSetScissorRects(1, &scissor)
}

// SetBlendConstant sets the blend constant.
func (e *RenderPassEncoder) SetBlendConstant(color *gputypes.Color) {
	if !e.encoder.isRecording || color == nil {
		return
	}

	blendFactor := [4]float32{
		float32(color.R),
		float32(color.G),
		float32(color.B),
		float32(color.A),
	}

	e.encoder.cmdList.OMSetBlendFactor(&blendFactor)
}

// SetStencilReference sets the stencil reference value.
func (e *RenderPassEncoder) SetStencilReference(ref uint32) {
	if !e.encoder.isRecording {
		return
	}

	e.encoder.cmdList.OMSetStencilRef(ref)
}

// Draw draws primitives.
func (e *RenderPassEncoder) Draw(vertexCount, instanceCount, firstVertex, firstInstance uint32) {
	if !e.encoder.isRecording {
		return
	}

	e.encoder.cmdList.DrawInstanced(vertexCount, instanceCount, firstVertex, firstInstance)
}

// DrawIndexed draws indexed primitives.
func (e *RenderPassEncoder) DrawIndexed(indexCount, instanceCount, firstIndex uint32, baseVertex int32, firstInstance uint32) {
	if !e.encoder.isRecording {
		return
	}

	e.encoder.cmdList.DrawIndexedInstanced(indexCount, instanceCount, firstIndex, baseVertex, firstInstance)
}

// DrawIndirect draws primitives with GPU-generated parameters.
// The buffer must contain a D3D12_DRAW_ARGUMENTS struct at the given offset
// (vertexCountPerInstance, instanceCount, startVertexLocation, startInstanceLocation — 16 bytes).
func (e *RenderPassEncoder) DrawIndirect(buffer hal.Buffer, offset uint64) {
	buf, ok := buffer.(*Buffer)
	if !ok || !e.encoder.isRecording {
		return
	}
	if before, target, needsBarrier := e.encoder.stateTracker.transitionBufferRead(buf, d3d12.D3D12_RESOURCE_STATE_INDIRECT_ARGUMENT); needsBarrier {
		e.encoder.emitStateBarrierPlans([]stateBarrierPlan{{resource: buf, subresource: d3d12.D3D12_RESOURCE_BARRIER_ALL_SUBRESOURCES, before: before, after: target}})
	}

	e.encoder.cmdList.ExecuteIndirect(
		e.encoder.device.cmdSignatures.draw,
		1, buf.raw, offset, nil, 0,
	)
}

// DrawIndexedIndirect draws indexed primitives with GPU-generated parameters.
// The buffer must contain a D3D12_DRAW_INDEXED_ARGUMENTS struct at the given offset
// (indexCountPerInstance, instanceCount, startIndexLocation, baseVertexLocation, startInstanceLocation — 20 bytes).
func (e *RenderPassEncoder) DrawIndexedIndirect(buffer hal.Buffer, offset uint64) {
	buf, ok := buffer.(*Buffer)
	if !ok || !e.encoder.isRecording {
		return
	}
	if before, target, needsBarrier := e.encoder.stateTracker.transitionBufferRead(buf, d3d12.D3D12_RESOURCE_STATE_INDIRECT_ARGUMENT); needsBarrier {
		e.encoder.emitStateBarrierPlans([]stateBarrierPlan{{resource: buf, subresource: d3d12.D3D12_RESOURCE_BARRIER_ALL_SUBRESOURCES, before: before, after: target}})
	}

	e.encoder.cmdList.ExecuteIndirect(
		e.encoder.device.cmdSignatures.drawIndexed,
		1, buf.raw, offset, nil, 0,
	)
}

// ExecuteBundle executes a pre-recorded render bundle.
func (e *RenderPassEncoder) ExecuteBundle(bundle hal.RenderBundle) {
	// Note: DX12 bundles use ID3D12GraphicsCommandList created with D3D12_COMMAND_LIST_TYPE_BUNDLE.
	_ = bundle
}

// ComputePassEncoder implements hal.ComputePassEncoder for DirectX 12.
type ComputePassEncoder struct {
	encoder            *CommandEncoder
	pipeline           *ComputePipeline
	descriptorHeapsSet bool // Tracks whether descriptor heaps have been bound

	// boundStorageBuffers tracks storage buffers from SetBindGroup calls.
	// Dispatch records them as UNORDERED_ACCESS in the command-local tracker so
	// a later copy in the same command list can transition from the UAV state.
	//
	// Matches Rust wgpu-core pattern: compute pass tracks buffer usage per
	// dispatch via BufferUsageScope, then drains barriers on state transitions
	// (wgpu-core/src/command/compute.rs:326-377).
	boundStorageBuffers []*Buffer

	// endOfPassTimerQuery stores the query heap and index for the end-of-pass
	// timestamp write. Set during BeginComputePass, consumed in End().
	endOfPassTimerHeap  *d3d12.ID3D12QueryHeap
	endOfPassTimerIndex uint32
}

// End finishes the compute pass.
// Writes end-of-pass timestamp if requested.
// Rust wgpu-hal reference: dx12/command.rs end_compute_pass.
func (e *ComputePassEncoder) End() {
	if e.encoder == nil || !e.encoder.isRecording {
		return
	}
	if e.endOfPassTimerHeap != nil {
		e.encoder.cmdList.EndQuery(e.endOfPassTimerHeap, d3d12.D3D12_QUERY_TYPE_TIMESTAMP, e.endOfPassTimerIndex)
		e.endOfPassTimerHeap = nil
	}
}

// SetPipeline sets the compute pipeline.
func (e *ComputePassEncoder) SetPipeline(pipeline hal.ComputePipeline) {
	p, ok := pipeline.(*ComputePipeline)
	if !ok || !e.encoder.isRecording {
		return
	}
	e.pipeline = p

	e.encoder.cmdList.SetPipelineState(p.pso)
	if p.rootSignature != nil {
		e.encoder.cmdList.SetComputeRootSignature(p.rootSignature)
	}

	// Bind the global sampler descriptor heap to the sampler root parameter.
	if p.samplerRootIndex >= 0 && e.encoder.device.samplerHeap != nil {
		e.encoder.ensureDescriptorHeapsBound()
		e.descriptorHeapsSet = true
		e.encoder.cmdList.SetComputeRootDescriptorTable(
			uint32(p.samplerRootIndex),
			e.encoder.device.samplerHeap.gpuStart,
		)
	}
}

// SetBindGroup sets a bind group for compute operations.
// index is the bind group slot (0-3 typically).
// group contains the GPU descriptor handles for resources.
// offsets are dynamic buffer offsets (used for dynamic uniform/storage buffers).
func (e *ComputePassEncoder) SetBindGroup(index uint32, group hal.BindGroup, offsets []uint32) {
	bg, ok := group.(*BindGroup)
	if !ok || !e.encoder.isRecording {
		return
	}

	// Ensure descriptor heaps are bound before setting descriptor tables.
	if !e.descriptorHeapsSet {
		e.encoder.ensureDescriptorHeapsBound()
		e.descriptorHeapsSet = true
	}

	// Get group mappings from the current pipeline.
	var mappings []rootParamMapping
	if e.pipeline != nil {
		mappings = e.pipeline.groupMappings
	}

	// Bind the group using compute root descriptor tables.
	e.encoder.bindGroupToRootTables(index, bg, true, mappings)
	e.encoder.trackBindGroupState(bg)

	// Track storage buffers from this bind group for state tracking.
	// After Dispatch(), these buffers will be marked as UNORDERED_ACCESS
	// so that subsequent copy commands can insert correct transition barriers.
	// storageBuffers is populated at CreateBindGroup time by matching layout
	// entry types (BindingTypeStorageBuffer) with buffer binding handles.
	e.boundStorageBuffers = append(e.boundStorageBuffers, bg.storageBuffers...)

	_ = offsets // Dynamic offsets handled via root constants (simplified for now)
}

// Dispatch dispatches compute work.
func (e *ComputePassEncoder) Dispatch(x, y, z uint32) {
	if !e.encoder.isRecording {
		return
	}

	plans := make([]stateBarrierPlan, 0, len(e.boundStorageBuffers))
	for _, buf := range e.boundStorageBuffers {
		if before, needsBarrier := e.encoder.stateTracker.transitionBuffer(buf, d3d12.D3D12_RESOURCE_STATE_UNORDERED_ACCESS); needsBarrier {
			plans = append(plans, stateBarrierPlan{resource: buf, subresource: d3d12.D3D12_RESOURCE_BARRIER_ALL_SUBRESOURCES, before: before, after: d3d12.D3D12_RESOURCE_STATE_UNORDERED_ACCESS})
		}
	}
	e.encoder.emitStateBarrierPlans(plans)
	e.encoder.cmdList.Dispatch(x, y, z)
	e.insertUAVBarrier()

	// Mark all bound storage buffers as UNORDERED_ACCESS. After a compute
	// dispatch, storage buffers are left in UAV state. This enables correct
	// transition barriers when subsequent commands (e.g., CopyBufferToBuffer)
	// need these buffers in a different state (e.g., COPY_SOURCE).
	// Matches Rust wgpu-core pattern: flush_bindings sets BufferUses::STORAGE_READ_WRITE
	// on bound storage buffers, then drain_barriers emits transitions.
	// Clear for next dispatch (same pattern as Rust per-dispatch usage scope).
	e.boundStorageBuffers = e.boundStorageBuffers[:0]
}

// DispatchIndirect dispatches compute work with GPU-generated parameters.
// The buffer must contain a D3D12_DISPATCH_ARGUMENTS struct at the given offset
// (threadGroupCountX, threadGroupCountY, threadGroupCountZ — 12 bytes).
func (e *ComputePassEncoder) DispatchIndirect(buffer hal.Buffer, offset uint64) {
	buf, ok := buffer.(*Buffer)
	if !ok || !e.encoder.isRecording {
		return
	}
	plans := make([]stateBarrierPlan, 0, 1+len(e.boundStorageBuffers))
	if before, target, needsBarrier := e.encoder.stateTracker.transitionBufferRead(buf, d3d12.D3D12_RESOURCE_STATE_INDIRECT_ARGUMENT); needsBarrier {
		plans = append(plans, stateBarrierPlan{resource: buf, subresource: d3d12.D3D12_RESOURCE_BARRIER_ALL_SUBRESOURCES, before: before, after: target})
	}
	for _, storage := range e.boundStorageBuffers {
		if before, needsBarrier := e.encoder.stateTracker.transitionBuffer(storage, d3d12.D3D12_RESOURCE_STATE_UNORDERED_ACCESS); needsBarrier {
			plans = append(plans, stateBarrierPlan{resource: storage, subresource: d3d12.D3D12_RESOURCE_BARRIER_ALL_SUBRESOURCES, before: before, after: d3d12.D3D12_RESOURCE_STATE_UNORDERED_ACCESS})
		}
	}
	e.encoder.emitStateBarrierPlans(plans)

	e.encoder.cmdList.ExecuteIndirect(
		e.encoder.device.cmdSignatures.dispatch,
		1, buf.raw, offset, nil, 0,
	)
	e.insertUAVBarrier()

	// Mark bound storage buffers as UNORDERED_ACCESS, matching Dispatch() pattern.
	// After an indirect dispatch, storage buffers are left in UAV state for correct
	// transition barriers on subsequent commands.
	e.boundStorageBuffers = e.boundStorageBuffers[:0]
}

// insertUAVBarrier inserts a global UAV barrier after a dispatch.
// VAL-008: ensures UAV writes from one dispatch are visible to subsequent
// dispatches. NULL resource = global barrier across all UAV resources.
func (e *ComputePassEncoder) insertUAVBarrier() {
	barrier := d3d12.NewUAVBarrier(nil)
	e.encoder.cmdList.ResourceBarrier(1, &barrier)
}

// --- Buffer state transition helpers ---

// needsExplicitBarrier returns true if transitioning from the given current state
// to the target state requires an explicit D3D12_RESOURCE_BARRIER.
//
// DX12 implicit promotion rules (D3D12 spec, "Using Resource Barriers"):
//   - COMMON state allows implicit promotion to: COPY_DEST, VERTEX_AND_CONSTANT_BUFFER,
//     INDEX_BUFFER, NON_PIXEL_SHADER_RESOURCE, PIXEL_SHADER_RESOURCE, INDIRECT_ARGUMENT.
//   - COMMON does NOT allow implicit promotion to: COPY_SOURCE, UNORDERED_ACCESS.
//   - Same state -> same state: no barrier needed.
func needsExplicitBarrier(current, target d3d12.D3D12_RESOURCE_STATES) bool {
	if current == target {
		return false
	}
	// COMMON state allows implicit promotion to several read states and COPY_DEST,
	// but NOT to COPY_SOURCE or UNORDERED_ACCESS.
	if current == d3d12.D3D12_RESOURCE_STATE_COMMON {
		switch target {
		case d3d12.D3D12_RESOURCE_STATE_COPY_DEST,
			d3d12.D3D12_RESOURCE_STATE_VERTEX_AND_CONSTANT_BUFFER,
			d3d12.D3D12_RESOURCE_STATE_INDEX_BUFFER,
			d3d12.D3D12_RESOURCE_STATE_NON_PIXEL_SHADER_RESOURCE,
			d3d12.D3D12_RESOURCE_STATE_PIXEL_SHADER_RESOURCE,
			d3d12.D3D12_RESOURCE_STATE_INDIRECT_ARGUMENT:
			return false // Implicit promotion allowed
		}
	}
	return true
}

// transitionBufferIfNeeded inserts a transition barrier for a single buffer if
// its command-local state requires an explicit barrier to reach the target.
func (e *CommandEncoder) transitionBufferIfNeeded(buf *Buffer, targetState d3d12.D3D12_RESOURCE_STATES) {
	if before, needsBarrier := e.stateTracker.transitionBuffer(buf, targetState); needsBarrier {
		e.emitStateBarrierPlans([]stateBarrierPlan{{resource: buf, subresource: d3d12.D3D12_RESOURCE_BARRIER_ALL_SUBRESOURCES, before: before, after: targetState}})
	}
}

// transitionBuffersForCopy inserts batched transition barriers for a source and
// destination buffer pair used in a copy command. Barriers are batched into a
// single ResourceBarrier call when both buffers need transitions (Rust pattern:
// drain_barriers emits all pending transitions at once).
func (e *CommandEncoder) transitionBuffersForCopy(
	srcBuf *Buffer, srcTarget d3d12.D3D12_RESOURCE_STATES,
	dstBuf *Buffer, dstTarget d3d12.D3D12_RESOURCE_STATES,
) {
	plans := make([]stateBarrierPlan, 0, 2)
	if before, needsBarrier := e.stateTracker.transitionBuffer(srcBuf, srcTarget); needsBarrier {
		plans = append(plans, stateBarrierPlan{resource: srcBuf, subresource: d3d12.D3D12_RESOURCE_BARRIER_ALL_SUBRESOURCES, before: before, after: srcTarget})
	}
	if before, needsBarrier := e.stateTracker.transitionBuffer(dstBuf, dstTarget); needsBarrier {
		plans = append(plans, stateBarrierPlan{resource: dstBuf, subresource: d3d12.D3D12_RESOURCE_BARRIER_ALL_SUBRESOURCES, before: before, after: dstTarget})
	}
	e.emitStateBarrierPlans(plans)
}

func resolveSubresourceIndex(view *TextureView) uint32 {
	if view == nil || view.texture == nil {
		return 0
	}
	if subresources := textureViewSubresources(view); len(subresources) > 0 {
		return subresources[0]
	}
	return view.texture.subresourceIndex(view.baseMip, view.baseLayer)
}

// --- Helper functions ---

// ensureDescriptorHeapsBound binds the shader-visible descriptor heaps to the command list.
// D3D12 requires descriptor heaps to be bound before setting root descriptor tables.
// This must be called before any SetBindGroup operations.
func (e *CommandEncoder) ensureDescriptorHeapsBound() {
	e.descriptorHeapCount = 0

	// Add shader-visible heaps (viewHeap for CBV/SRV/UAV, samplerHeap for samplers)
	if e.device.viewHeap != nil && e.device.viewHeap.raw != nil {
		e.descriptorHeaps[e.descriptorHeapCount] = e.device.viewHeap.raw
		e.descriptorHeapCount++
	}
	if e.device.samplerHeap != nil && e.device.samplerHeap.raw != nil {
		e.descriptorHeaps[e.descriptorHeapCount] = e.device.samplerHeap.raw
		e.descriptorHeapCount++
	}

	if e.descriptorHeapCount > 0 {
		e.cmdList.SetDescriptorHeaps(uint32(e.descriptorHeapCount), &e.descriptorHeaps[0])
	}
}

// bindGroupToRootTables binds a BindGroup's CBV/SRV/UAV descriptor table to root parameters.
// Samplers are handled separately via the global sampler heap (bound in SetPipeline).
// isCompute determines whether to use compute or graphics root descriptor tables.
// groupMappings provides the actual root parameter indices.
func (e *CommandEncoder) bindGroupToRootTables(bindGroupIndex uint32, bg *BindGroup, isCompute bool, groupMappings []rootParamMapping) {
	// Use mapping if available; fall back to bindGroupIndex for backwards compatibility.
	cbvIdx := int(bindGroupIndex)
	if int(bindGroupIndex) < len(groupMappings) {
		cbvIdx = groupMappings[bindGroupIndex].cbvSrvUavIndex
	}

	// Set CBV/SRV/UAV descriptor table (includes sampler index buffer SRV).
	if bg.gpuDescHandle.Ptr != 0 && cbvIdx >= 0 {
		if isCompute {
			e.cmdList.SetComputeRootDescriptorTable(uint32(cbvIdx), bg.gpuDescHandle)
		} else {
			e.cmdList.SetGraphicsRootDescriptorTable(uint32(cbvIdx), bg.gpuDescHandle)
		}
	}
}

func (e *CommandEncoder) trackBindGroupState(bg *BindGroup) {
	if bg == nil {
		return
	}
	plans := make([]stateBarrierPlan, 0)
	for _, buffer := range bg.uniformBuffers {
		if before, target, needsBarrier := e.stateTracker.transitionBufferRead(buffer, d3d12.D3D12_RESOURCE_STATE_VERTEX_AND_CONSTANT_BUFFER); needsBarrier {
			plans = append(plans, stateBarrierPlan{resource: buffer, subresource: d3d12.D3D12_RESOURCE_BARRIER_ALL_SUBRESOURCES, before: before, after: target})
		}
	}
	for _, buffer := range bg.readOnlyStorageBuffers {
		if before, target, needsBarrier := e.stateTracker.transitionBufferRead(buffer, d3d12.D3D12_RESOURCE_STATE_NON_PIXEL_SHADER_RESOURCE|d3d12.D3D12_RESOURCE_STATE_PIXEL_SHADER_RESOURCE); needsBarrier {
			plans = append(plans, stateBarrierPlan{resource: buffer, subresource: d3d12.D3D12_RESOURCE_BARRIER_ALL_SUBRESOURCES, before: before, after: target})
		}
	}
	for _, buffer := range bg.storageBuffers {
		if before, needsBarrier := e.stateTracker.transitionBuffer(buffer, d3d12.D3D12_RESOURCE_STATE_UNORDERED_ACCESS); needsBarrier {
			plans = append(plans, stateBarrierPlan{resource: buffer, subresource: d3d12.D3D12_RESOURCE_BARRIER_ALL_SUBRESOURCES, before: before, after: d3d12.D3D12_RESOURCE_STATE_UNORDERED_ACCESS})
		}
	}
	for _, view := range bg.sampledTextures {
		for _, subresource := range textureViewSubresources(view) {
			if before, target, needsBarrier := e.stateTracker.transitionTextureRead(view.texture, subresource, d3d12.D3D12_RESOURCE_STATE_NON_PIXEL_SHADER_RESOURCE|d3d12.D3D12_RESOURCE_STATE_PIXEL_SHADER_RESOURCE); needsBarrier {
				plans = append(plans, stateBarrierPlan{resource: view.texture, subresource: subresource, before: before, after: target})
			}
		}
	}
	for _, view := range bg.storageTextures {
		for _, subresource := range textureViewSubresources(view) {
			if before, needsBarrier := e.stateTracker.transitionTexture(view.texture, subresource, d3d12.D3D12_RESOURCE_STATE_UNORDERED_ACCESS); needsBarrier {
				plans = append(plans, stateBarrierPlan{resource: view.texture, subresource: subresource, before: before, after: d3d12.D3D12_RESOURCE_STATE_UNORDERED_ACCESS})
			}
		}
	}
	e.emitStateBarrierPlans(plans)
}

func textureViewSubresources(view *TextureView) []uint32 {
	if view == nil || view.texture == nil {
		return nil
	}
	return textureRangeSubresources(view.texture, hal.TextureRange{
		Aspect:          view.aspect,
		BaseMipLevel:    view.baseMip,
		MipLevelCount:   view.mipCount,
		BaseArrayLayer:  view.baseLayer,
		ArrayLayerCount: view.layerCount,
	})
}

func textureViewSubresourcePlanes(view *TextureView) []textureSubresource {
	if view == nil || view.texture == nil {
		return nil
	}
	return textureRangeSubresourcePlanes(view.texture, hal.TextureRange{
		Aspect:          view.aspect,
		BaseMipLevel:    view.baseMip,
		MipLevelCount:   view.mipCount,
		BaseArrayLayer:  view.baseLayer,
		ArrayLayerCount: view.layerCount,
	})
}

func textureViewPhysicalSubresourcePlanes(view *TextureView) []textureSubresource {
	if view == nil || view.texture == nil {
		return nil
	}
	planes := []uint32{0}
	if view.texture.planeCount() > 1 {
		planes = []uint32{0, 1}
	}
	return textureRangeSubresourcePlanesForPlanes(view.texture, hal.TextureRange{
		BaseMipLevel:    view.baseMip,
		MipLevelCount:   view.mipCount,
		BaseArrayLayer:  view.baseLayer,
		ArrayLayerCount: view.layerCount,
	}, planes)
}

// setupDepthStencilAttachment configures depth/stencil attachment for a render pass.
// Returns the DSV handle if valid, nil otherwise.
func (e *CommandEncoder) setupDepthStencilAttachment(dsa *hal.RenderPassDepthStencilAttachment) *d3d12.D3D12_CPU_DESCRIPTOR_HANDLE {
	if dsa == nil {
		return nil
	}

	view, ok := dsa.View.(*TextureView)
	if !ok || !view.hasDSV {
		return nil
	}

	depthReadOnly, stencilReadOnly := depthStencilReadOnlyFlags(view.format, view.aspect, dsa.DepthReadOnly, dsa.StencilReadOnly)
	dsvFlags := d3d12.D3D12_DSV_FLAG_NONE
	if depthReadOnly {
		dsvFlags |= d3d12.D3D12_DSV_FLAG_READ_ONLY_DEPTH
	}
	if stencilReadOnly {
		dsvFlags |= d3d12.D3D12_DSV_FLAG_READ_ONLY_STENCIL
	}
	_, srvPlane := textureFormatToSRV(view.format, view.aspect)
	plans := make([]stateBarrierPlan, 0)
	for _, subresource := range textureViewPhysicalSubresourcePlanes(view) {
		shaderReadable := view.hasSRV && subresource.plane == srvPlane
		state := depthStencilPlaneState(subresource.plane, depthReadOnly, stencilReadOnly, shaderReadable)
		if before, needsBarrier := e.stateTracker.transitionTexture(view.texture, subresource.subresource, state); needsBarrier {
			plans = append(plans, stateBarrierPlan{resource: view.texture, subresource: subresource.subresource, before: before, after: state})
		}
	}
	e.emitStateBarrierPlans(plans)

	// Determine clear flags
	var clearFlags d3d12.D3D12_CLEAR_FLAGS
	if dsa.DepthLoadOp == gputypes.LoadOpClear {
		clearFlags |= d3d12.D3D12_CLEAR_FLAG_DEPTH
	}
	if dsa.StencilLoadOp == gputypes.LoadOpClear {
		clearFlags |= d3d12.D3D12_CLEAR_FLAG_STENCIL
	}

	dsvHandle := view.dsvHandleForFlags(dsvFlags)

	// Clear if needed. Use the same DSV variant as the pass so an unexposed or
	// explicitly read-only companion plane stays protected during the clear.
	if clearFlags != 0 {
		e.cmdList.ClearDepthStencilView(
			*dsvHandle,
			clearFlags,
			dsa.DepthClearValue,
			uint8(dsa.StencilClearValue),
			0, nil,
		)
	}

	return dsvHandle
}

// bufferUsageToD3D12State converts buffer usage to D3D12 resource state.
func bufferUsageToD3D12State(usage gputypes.BufferUsage) d3d12.D3D12_RESOURCE_STATES {
	var state d3d12.D3D12_RESOURCE_STATES

	if usage&gputypes.BufferUsageCopySrc != 0 {
		state |= d3d12.D3D12_RESOURCE_STATE_COPY_SOURCE
	}
	if usage&gputypes.BufferUsageCopyDst != 0 {
		state |= d3d12.D3D12_RESOURCE_STATE_COPY_DEST
	}
	if usage&gputypes.BufferUsageVertex != 0 {
		state |= d3d12.D3D12_RESOURCE_STATE_VERTEX_AND_CONSTANT_BUFFER
	}
	if usage&gputypes.BufferUsageIndex != 0 {
		state |= d3d12.D3D12_RESOURCE_STATE_INDEX_BUFFER
	}
	if usage&gputypes.BufferUsageUniform != 0 {
		state |= d3d12.D3D12_RESOURCE_STATE_VERTEX_AND_CONSTANT_BUFFER
	}
	if usage&gputypes.BufferUsageStorage != 0 {
		state |= d3d12.D3D12_RESOURCE_STATE_UNORDERED_ACCESS
	}
	if usage&gputypes.BufferUsageIndirect != 0 {
		state |= d3d12.D3D12_RESOURCE_STATE_INDIRECT_ARGUMENT
	}

	if state == 0 {
		state = d3d12.D3D12_RESOURCE_STATE_COMMON
	}

	return state
}

// textureUsageToD3D12State converts texture usage to D3D12 resource state.
// d3d12StateToTextureUsage maps a D3D12 resource state back to gputypes.TextureUsage.
// Used by Texture.CurrentUsage() for PendingWrites barrier computation.
func d3d12StateToTextureUsage(state d3d12.D3D12_RESOURCE_STATES) gputypes.TextureUsage {
	var usage gputypes.TextureUsage

	if state&d3d12.D3D12_RESOURCE_STATE_COPY_SOURCE != 0 {
		usage |= gputypes.TextureUsageCopySrc
	}
	if state&d3d12.D3D12_RESOURCE_STATE_COPY_DEST != 0 {
		usage |= gputypes.TextureUsageCopyDst
	}
	if state&(d3d12.D3D12_RESOURCE_STATE_PIXEL_SHADER_RESOURCE|d3d12.D3D12_RESOURCE_STATE_NON_PIXEL_SHADER_RESOURCE) != 0 {
		usage |= gputypes.TextureUsageTextureBinding
	}
	if state&d3d12.D3D12_RESOURCE_STATE_UNORDERED_ACCESS != 0 {
		usage |= gputypes.TextureUsageStorageBinding
	}
	if state&d3d12.D3D12_RESOURCE_STATE_RENDER_TARGET != 0 {
		usage |= gputypes.TextureUsageRenderAttachment
	}
	if state&(d3d12.D3D12_RESOURCE_STATE_DEPTH_READ|d3d12.D3D12_RESOURCE_STATE_DEPTH_WRITE) != 0 {
		usage |= gputypes.TextureUsageRenderAttachment
	}

	// COMMON (0) maps to 0 (no specific usage)
	return usage
}

func textureUsageToD3D12State(usage gputypes.TextureUsage) d3d12.D3D12_RESOURCE_STATES {
	var state d3d12.D3D12_RESOURCE_STATES

	if usage&gputypes.TextureUsageCopySrc != 0 {
		state |= d3d12.D3D12_RESOURCE_STATE_COPY_SOURCE
	}
	if usage&gputypes.TextureUsageCopyDst != 0 {
		state |= d3d12.D3D12_RESOURCE_STATE_COPY_DEST
	}
	if usage&gputypes.TextureUsageTextureBinding != 0 {
		state |= d3d12.D3D12_RESOURCE_STATE_PIXEL_SHADER_RESOURCE | d3d12.D3D12_RESOURCE_STATE_NON_PIXEL_SHADER_RESOURCE
	}
	if usage&gputypes.TextureUsageStorageBinding != 0 {
		state |= d3d12.D3D12_RESOURCE_STATE_UNORDERED_ACCESS
	}
	if usage&gputypes.TextureUsageRenderAttachment != 0 {
		state |= d3d12.D3D12_RESOURCE_STATE_RENDER_TARGET
	}

	if state == 0 {
		state = d3d12.D3D12_RESOURCE_STATE_COMMON
	}

	return state
}

// --- Compile-time interface assertions ---

var (
	_ hal.CommandEncoder     = (*CommandEncoder)(nil)
	_ hal.CommandBuffer      = (*CommandBuffer)(nil)
	_ hal.RenderPassEncoder  = (*RenderPassEncoder)(nil)
	_ hal.ComputePassEncoder = (*ComputePassEncoder)(nil)
)
