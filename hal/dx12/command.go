// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows

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
	cmdList *d3d12.ID3D12GraphicsCommandList
}

// Destroy releases the command buffer's command list.
func (c *CommandBuffer) Destroy() {
	if c.cmdList != nil {
		c.cmdList.Release()
		c.cmdList = nil
	}
}

// CommandEncoder implements hal.CommandEncoder for DirectX 12.
type CommandEncoder struct {
	device      *Device
	allocator   *CommandAllocator
	cmdList     *d3d12.ID3D12GraphicsCommandList
	label       string
	isRecording bool
}

// BeginEncoding begins command recording.
// Acquires a command allocator from the pool (already reset by advanceFrame)
// and creates or resets the command list for recording.
func (e *CommandEncoder) BeginEncoding(label string) error {
	e.label = label

	// Acquire a safe allocator from the pool. Allocators are reset by advanceFrame
	// after their frame slot's fence is reached — no full GPU stall needed here.
	alloc, err := e.device.acquireAllocator()
	if err != nil {
		return fmt.Errorf("dx12: acquire allocator: %w", err)
	}
	e.allocator = alloc

	if e.cmdList == nil {
		// First use — create command list (starts in recording state).
		cmdList, err := e.device.raw.CreateCommandList(0, d3d12.D3D12_COMMAND_LIST_TYPE_DIRECT, alloc.raw, nil)
		if err != nil {
			return fmt.Errorf("dx12: CreateCommandList failed: %w", err)
		}
		e.cmdList = cmdList
	} else {
		// Subsequent use — reset command list with the new allocator.
		if err := e.cmdList.Reset(alloc.raw, nil); err != nil {
			return fmt.Errorf("dx12: command list reset failed: %w", err)
		}
	}

	e.isRecording = true
	return nil
}

// EndEncoding finishes command recording and returns a command buffer.
func (e *CommandEncoder) EndEncoding() (hal.CommandBuffer, error) {
	if !e.isRecording {
		return nil, fmt.Errorf("dx12: command encoder is not recording")
	}

	// Close the command list
	if err := e.cmdList.Close(); err != nil {
		return nil, fmt.Errorf("dx12: command list close failed: %w", err)
	}

	e.isRecording = false

	return &CommandBuffer{
		cmdList: e.cmdList,
	}, nil
}

// DiscardEncoding discards the encoder without creating a command buffer.
func (e *CommandEncoder) DiscardEncoding() {
	if e.isRecording {
		// Close the command list even though we're discarding it
		_ = e.cmdList.Close()
		e.isRecording = false
	}
}

// ResetAll resets command buffers for reuse.
func (e *CommandEncoder) ResetAll(commandBuffers []hal.CommandBuffer) {
	// In DX12, command allocators are reset when BeginEncoding is called
	// Command lists can be reset and reused
	_ = commandBuffers
}

// TransitionBuffers transitions buffer states for synchronization.
func (e *CommandEncoder) TransitionBuffers(barriers []hal.BufferBarrier) {
	if !e.isRecording || len(barriers) == 0 {
		return
	}

	// Convert to D3D12 resource barriers
	d3dBarriers := make([]d3d12.D3D12_RESOURCE_BARRIER, 0, len(barriers))
	for _, b := range barriers {
		buf, ok := b.Buffer.(*Buffer)
		if !ok {
			continue
		}

		beforeState := bufferUsageToD3D12State(b.Usage.OldUsage)
		afterState := bufferUsageToD3D12State(b.Usage.NewUsage)

		// Skip if no transition needed
		if beforeState == afterState {
			continue
		}

		d3dBarriers = append(d3dBarriers, d3d12.NewTransitionBarrier(buf.raw, beforeState, afterState, d3d12.D3D12_RESOURCE_BARRIER_ALL_SUBRESOURCES))
	}

	if len(d3dBarriers) > 0 {
		e.cmdList.ResourceBarrier(uint32(len(d3dBarriers)), &d3dBarriers[0])
	}
}

// TransitionTextures transitions texture states for synchronization.
func (e *CommandEncoder) TransitionTextures(barriers []hal.TextureBarrier) {
	if !e.isRecording || len(barriers) == 0 {
		return
	}

	// Convert to D3D12 resource barriers
	d3dBarriers := make([]d3d12.D3D12_RESOURCE_BARRIER, 0, len(barriers))
	for _, b := range barriers {
		tex, ok := b.Texture.(*Texture)
		if !ok {
			continue
		}

		beforeState := textureUsageToD3D12State(b.Usage.OldUsage)
		afterState := textureUsageToD3D12State(b.Usage.NewUsage)

		// Skip if no transition needed
		if beforeState == afterState {
			continue
		}

		// Calculate subresource or use all
		subresource := d3d12.D3D12_RESOURCE_BARRIER_ALL_SUBRESOURCES
		if b.Range.MipLevelCount == 1 && b.Range.ArrayLayerCount == 1 {
			// Single subresource
			subresource = b.Range.BaseMipLevel + b.Range.BaseArrayLayer*tex.mipLevels
		}

		d3dBarriers = append(d3dBarriers, d3d12.NewTransitionBarrier(tex.raw, beforeState, afterState, subresource))
	}

	if len(d3dBarriers) > 0 {
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
func (e *CommandEncoder) CopyBufferToBuffer(src, dst hal.Buffer, regions []hal.BufferCopy) {
	if !e.isRecording {
		return
	}

	srcBuf, srcOk := src.(*Buffer)
	dstBuf, dstOk := dst.(*Buffer)
	if !srcOk || !dstOk {
		return
	}

	for _, r := range regions {
		e.cmdList.CopyBufferRegion(dstBuf.raw, r.DstOffset, srcBuf.raw, r.SrcOffset, r.Size)
	}
}

// CopyBufferToTexture copies data from a buffer to a texture.
func (e *CommandEncoder) CopyBufferToTexture(src hal.Buffer, dst hal.Texture, regions []hal.BufferTextureCopy) {
	if !e.isRecording {
		return
	}

	srcBuf, srcOk := src.(*Buffer)
	dstTex, dstOk := dst.(*Texture)
	if !srcOk || !dstOk {
		return
	}

	for _, r := range regions {
		// Source location (buffer)
		srcLoc := d3d12.D3D12_TEXTURE_COPY_LOCATION{
			Resource: srcBuf.raw,
			Type:     d3d12.D3D12_TEXTURE_COPY_TYPE_PLACED_FOOTPRINT,
		}
		srcLoc.SetPlacedFootprint(d3d12.D3D12_PLACED_SUBRESOURCE_FOOTPRINT{
			Offset: r.BufferLayout.Offset,
			Footprint: d3d12.D3D12_SUBRESOURCE_FOOTPRINT{
				Format:   textureFormatToD3D12(dstTex.format),
				Width:    r.Size.Width,
				Height:   r.Size.Height,
				Depth:    r.Size.DepthOrArrayLayers,
				RowPitch: r.BufferLayout.BytesPerRow,
			},
		})

		// Destination location (texture)
		subresource := r.TextureBase.MipLevel + r.TextureBase.Origin.Z*dstTex.mipLevels
		dstLoc := d3d12.D3D12_TEXTURE_COPY_LOCATION{
			Resource: dstTex.raw,
			Type:     d3d12.D3D12_TEXTURE_COPY_TYPE_SUBRESOURCE_INDEX,
		}
		dstLoc.SetSubresourceIndex(subresource)

		// Copy region (nil means entire subresource)
		e.cmdList.CopyTextureRegion(
			&dstLoc,
			r.TextureBase.Origin.X, r.TextureBase.Origin.Y, r.TextureBase.Origin.Z,
			&srcLoc,
			nil, // Copy entire source
		)
	}
}

// CopyTextureToBuffer copies data from a texture to a buffer.
func (e *CommandEncoder) CopyTextureToBuffer(src hal.Texture, dst hal.Buffer, regions []hal.BufferTextureCopy) {
	if !e.isRecording {
		return
	}

	srcTex, srcOk := src.(*Texture)
	dstBuf, dstOk := dst.(*Buffer)
	if !srcOk || !dstOk {
		return
	}

	for _, r := range regions {
		// Source location (texture)
		subresource := r.TextureBase.MipLevel + r.TextureBase.Origin.Z*srcTex.mipLevels
		srcLoc := d3d12.D3D12_TEXTURE_COPY_LOCATION{
			Resource: srcTex.raw,
			Type:     d3d12.D3D12_TEXTURE_COPY_TYPE_SUBRESOURCE_INDEX,
		}
		srcLoc.SetSubresourceIndex(subresource)

		// D3D12 requires RowPitch aligned to 256 bytes.
		// The caller should pass aligned BytesPerRow, but align defensively.
		rowPitch := (r.BufferLayout.BytesPerRow + d3d12TexturePitchAlignment - 1) &^ (d3d12TexturePitchAlignment - 1)

		// Destination location (buffer)
		dstLoc := d3d12.D3D12_TEXTURE_COPY_LOCATION{
			Resource: dstBuf.raw,
			Type:     d3d12.D3D12_TEXTURE_COPY_TYPE_PLACED_FOOTPRINT,
		}
		dstLoc.SetPlacedFootprint(d3d12.D3D12_PLACED_SUBRESOURCE_FOOTPRINT{
			Offset: r.BufferLayout.Offset,
			Footprint: d3d12.D3D12_SUBRESOURCE_FOOTPRINT{
				Format:   textureFormatToD3D12(srcTex.format),
				Width:    r.Size.Width,
				Height:   r.Size.Height,
				Depth:    r.Size.DepthOrArrayLayers,
				RowPitch: rowPitch,
			},
		})

		// Source box
		srcBox := d3d12.D3D12_BOX{
			Left:   r.TextureBase.Origin.X,
			Top:    r.TextureBase.Origin.Y,
			Front:  0,
			Right:  r.TextureBase.Origin.X + r.Size.Width,
			Bottom: r.TextureBase.Origin.Y + r.Size.Height,
			Back:   r.Size.DepthOrArrayLayers,
		}

		e.cmdList.CopyTextureRegion(&dstLoc, 0, 0, 0, &srcLoc, &srcBox)
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

	for _, r := range regions {
		// Source location
		srcSubresource := r.SrcBase.MipLevel + r.SrcBase.Origin.Z*srcTex.mipLevels
		srcLoc := d3d12.D3D12_TEXTURE_COPY_LOCATION{
			Resource: srcTex.raw,
			Type:     d3d12.D3D12_TEXTURE_COPY_TYPE_SUBRESOURCE_INDEX,
		}
		srcLoc.SetSubresourceIndex(srcSubresource)

		// Destination location
		dstSubresource := r.DstBase.MipLevel + r.DstBase.Origin.Z*dstTex.mipLevels
		dstLoc := d3d12.D3D12_TEXTURE_COPY_LOCATION{
			Resource: dstTex.raw,
			Type:     d3d12.D3D12_TEXTURE_COPY_TYPE_SUBRESOURCE_INDEX,
		}
		dstLoc.SetSubresourceIndex(dstSubresource)

		// Source box
		srcBox := d3d12.D3D12_BOX{
			Left:   r.SrcBase.Origin.X,
			Top:    r.SrcBase.Origin.Y,
			Front:  0,
			Right:  r.SrcBase.Origin.X + r.Size.Width,
			Bottom: r.SrcBase.Origin.Y + r.Size.Height,
			Back:   r.Size.DepthOrArrayLayers,
		}

		e.cmdList.CopyTextureRegion(
			&dstLoc,
			r.DstBase.Origin.X, r.DstBase.Origin.Y, r.DstBase.Origin.Z,
			&srcLoc,
			&srcBox,
		)
	}
}

// ResolveQuerySet copies query results from a query set into a destination buffer.
// TODO: implement using ID3D12GraphicsCommandList::ResolveQueryData when QuerySet is implemented.
func (e *CommandEncoder) ResolveQuerySet(_ hal.QuerySet, _, _ uint32, _ hal.Buffer, _ uint64) {
	// Stub: DX12 timestamp query implementation pending.
}

// BeginRenderPass begins a render pass.
func (e *CommandEncoder) BeginRenderPass(desc *hal.RenderPassDescriptor) hal.RenderPassEncoder {
	rpe := &RenderPassEncoder{
		encoder: e,
		desc:    desc,
	}

	if !e.isRecording {
		return rpe
	}

	// Transition surface textures from PRESENT to RENDER_TARGET state.
	// DX12 requires explicit barriers (unlike Vulkan which uses render pass layout transitions).
	for _, ca := range desc.ColorAttachments {
		view, ok := ca.View.(*TextureView)
		if !ok || view.texture == nil || view.texture.raw == nil {
			continue
		}
		if view.texture.isExternal {
			barrier := d3d12.NewTransitionBarrier(
				view.texture.raw,
				d3d12.D3D12_RESOURCE_STATE_PRESENT,
				d3d12.D3D12_RESOURCE_STATE_RENDER_TARGET,
				d3d12.D3D12_RESOURCE_BARRIER_ALL_SUBRESOURCES,
			)
			e.cmdList.ResourceBarrier(1, &barrier)
		}
	}

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

	return rpe
}

// BeginComputePass begins a compute pass.
func (e *CommandEncoder) BeginComputePass(desc *hal.ComputePassDescriptor) hal.ComputePassEncoder {
	_ = desc // Compute passes don't need special setup in D3D12
	return &ComputePassEncoder{
		encoder: e,
	}
}

// RenderPassEncoder implements hal.RenderPassEncoder for DirectX 12.
type RenderPassEncoder struct {
	encoder            *CommandEncoder
	desc               *hal.RenderPassDescriptor
	pipeline           *RenderPipeline
	indexFormat        gputypes.IndexFormat
	descriptorHeapsSet bool // Tracks whether descriptor heaps have been bound
}

// End finishes the render pass.
// Handles MSAA resolve (if ResolveTarget is set) and transitions surface
// textures back to PRESENT state for presentation.
func (e *RenderPassEncoder) End() {
	if e.desc == nil || e.encoder == nil || !e.encoder.isRecording {
		return
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
			b1 := d3d12.NewTransitionBarrier(
				msaaView.texture.raw,
				d3d12.D3D12_RESOURCE_STATE_RENDER_TARGET,
				d3d12.D3D12_RESOURCE_STATE_RESOLVE_SOURCE,
				d3d12.D3D12_RESOURCE_BARRIER_ALL_SUBRESOURCES,
			)
			b2 := d3d12.NewTransitionBarrier(
				resolveView.texture.raw,
				resolveRestState,
				d3d12.D3D12_RESOURCE_STATE_RESOLVE_DEST,
				d3d12.D3D12_RESOURCE_BARRIER_ALL_SUBRESOURCES,
			)
			barriers := [2]d3d12.D3D12_RESOURCE_BARRIER{b1, b2}
			e.encoder.cmdList.ResourceBarrier(2, &barriers[0])

			// Resolve MSAA → single-sample.
			format := textureFormatToD3D12(msaaView.texture.format)
			e.encoder.cmdList.ResolveSubresource(
				resolveView.texture.raw, 0,
				msaaView.texture.raw, 0,
				format,
			)

			// Transition back: MSAA → render target (for next frame),
			// resolve target → resting state.
			b3 := d3d12.NewTransitionBarrier(
				msaaView.texture.raw,
				d3d12.D3D12_RESOURCE_STATE_RESOLVE_SOURCE,
				d3d12.D3D12_RESOURCE_STATE_RENDER_TARGET,
				d3d12.D3D12_RESOURCE_BARRIER_ALL_SUBRESOURCES,
			)
			b4 := d3d12.NewTransitionBarrier(
				resolveView.texture.raw,
				d3d12.D3D12_RESOURCE_STATE_RESOLVE_DEST,
				resolveRestState,
				d3d12.D3D12_RESOURCE_BARRIER_ALL_SUBRESOURCES,
			)
			barriers2 := [2]d3d12.D3D12_RESOURCE_BARRIER{b3, b4}
			e.encoder.cmdList.ResourceBarrier(2, &barriers2[0])
			continue
		}

		// No resolve — just transition external surface back to PRESENT.
		if msaaView.texture.isExternal {
			barrier := d3d12.NewTransitionBarrier(
				msaaView.texture.raw,
				d3d12.D3D12_RESOURCE_STATE_RENDER_TARGET,
				d3d12.D3D12_RESOURCE_STATE_PRESENT,
				d3d12.D3D12_RESOURCE_BARRIER_ALL_SUBRESOURCES,
			)
			e.encoder.cmdList.ResourceBarrier(1, &barrier)
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
	_ = offsets // Dynamic offsets handled via root constants (simplified for now)
}

// SetVertexBuffer sets a vertex buffer.
func (e *RenderPassEncoder) SetVertexBuffer(slot uint32, buffer hal.Buffer, offset uint64) {
	buf, ok := buffer.(*Buffer)
	if !ok || !e.encoder.isRecording {
		return
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
func (e *RenderPassEncoder) DrawIndirect(buffer hal.Buffer, offset uint64) {
	buf, ok := buffer.(*Buffer)
	if !ok || !e.encoder.isRecording {
		return
	}

	// Note: ExecuteIndirect requires a pre-created ID3D12CommandSignature.
	// For now, this is a stub
	_ = buf
	_ = offset
}

// DrawIndexedIndirect draws indexed primitives with GPU-generated parameters.
func (e *RenderPassEncoder) DrawIndexedIndirect(buffer hal.Buffer, offset uint64) {
	buf, ok := buffer.(*Buffer)
	if !ok || !e.encoder.isRecording {
		return
	}

	// Note: ExecuteIndirect requires a pre-created ID3D12CommandSignature.
	// For now, this is a stub
	_ = buf
	_ = offset
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
}

// End finishes the compute pass.
func (e *ComputePassEncoder) End() {
	// No explicit end needed for D3D12 compute passes
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
	_ = offsets // Dynamic offsets handled via root constants (simplified for now)
}

// Dispatch dispatches compute work.
func (e *ComputePassEncoder) Dispatch(x, y, z uint32) {
	if !e.encoder.isRecording {
		return
	}

	e.encoder.cmdList.Dispatch(x, y, z)
}

// DispatchIndirect dispatches compute work with GPU-generated parameters.
func (e *ComputePassEncoder) DispatchIndirect(buffer hal.Buffer, offset uint64) {
	buf, ok := buffer.(*Buffer)
	if !ok || !e.encoder.isRecording {
		return
	}

	// Note: ExecuteIndirect requires a pre-created ID3D12CommandSignature.
	// For now, this is a stub
	_ = buf
	_ = offset
}

// --- Helper functions ---

// ensureDescriptorHeapsBound binds the shader-visible descriptor heaps to the command list.
// D3D12 requires descriptor heaps to be bound before setting root descriptor tables.
// This must be called before any SetBindGroup operations.
func (e *CommandEncoder) ensureDescriptorHeapsBound() {
	heaps := make([]*d3d12.ID3D12DescriptorHeap, 0, 2)

	// Add shader-visible heaps (viewHeap for CBV/SRV/UAV, samplerHeap for samplers)
	if e.device.viewHeap != nil && e.device.viewHeap.raw != nil {
		heaps = append(heaps, e.device.viewHeap.raw)
	}
	if e.device.samplerHeap != nil && e.device.samplerHeap.raw != nil {
		heaps = append(heaps, e.device.samplerHeap.raw)
	}

	if len(heaps) > 0 {
		e.cmdList.SetDescriptorHeaps(uint32(len(heaps)), &heaps[0])
	}
}

// bindGroupToRootTables binds a BindGroup's descriptor tables to root parameters.
// isCompute determines whether to use compute or graphics root descriptor tables.
// groupMappings provides the actual root parameter indices (not all groups have both table types).
func (e *CommandEncoder) bindGroupToRootTables(bindGroupIndex uint32, bg *BindGroup, isCompute bool, groupMappings []rootParamMapping) {
	// Use mapping if available; fall back to bindGroupIndex*2 for backwards compatibility.
	cbvIdx := int(bindGroupIndex) * 2
	samplerIdx := cbvIdx + 1
	if int(bindGroupIndex) < len(groupMappings) {
		cbvIdx = groupMappings[bindGroupIndex].cbvSrvUavIndex
		samplerIdx = groupMappings[bindGroupIndex].samplerIndex
	}

	// Set CBV/SRV/UAV descriptor table if the bind group has one.
	if bg.gpuDescHandle.Ptr != 0 && cbvIdx >= 0 {
		if isCompute {
			e.cmdList.SetComputeRootDescriptorTable(uint32(cbvIdx), bg.gpuDescHandle)
		} else {
			e.cmdList.SetGraphicsRootDescriptorTable(uint32(cbvIdx), bg.gpuDescHandle)
		}
	}

	// Set sampler descriptor table if the bind group has one.
	if bg.samplerGPUHandle.Ptr != 0 && samplerIdx >= 0 {
		if isCompute {
			e.cmdList.SetComputeRootDescriptorTable(uint32(samplerIdx), bg.samplerGPUHandle)
		} else {
			e.cmdList.SetGraphicsRootDescriptorTable(uint32(samplerIdx), bg.samplerGPUHandle)
		}
	}
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

	// Determine clear flags
	var clearFlags d3d12.D3D12_CLEAR_FLAGS
	if dsa.DepthLoadOp == gputypes.LoadOpClear {
		clearFlags |= d3d12.D3D12_CLEAR_FLAG_DEPTH
	}
	if dsa.StencilLoadOp == gputypes.LoadOpClear {
		clearFlags |= d3d12.D3D12_CLEAR_FLAG_STENCIL
	}

	// Clear if needed
	if clearFlags != 0 {
		e.cmdList.ClearDepthStencilView(
			view.dsvHandle,
			clearFlags,
			dsa.DepthClearValue,
			uint8(dsa.StencilClearValue),
			0, nil,
		)
	}

	return &view.dsvHandle
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
