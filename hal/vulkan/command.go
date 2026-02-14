// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vulkan

import (
	"fmt"
	"runtime"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/vulkan/vk"
)

// CommandPool manages command buffer allocation.
type CommandPool struct {
	handle vk.CommandPool
	device *Device
}

// CommandBuffer holds a recorded Vulkan command buffer.
type CommandBuffer struct {
	handle vk.CommandBuffer
	pool   *CommandPool
}

// Destroy releases the command buffer resources.
func (c *CommandBuffer) Destroy() {
	// Command buffers are freed when the pool is destroyed or reset.
	// We cannot call vkFreeCommandBuffers here because:
	// 1. GPU may still be using this command buffer (async submission)
	// 2. Proper solution requires fence-based tracking or pool reset after WaitIdle
	c.handle = 0
}

// CommandEncoder implements hal.CommandEncoder for Vulkan.
type CommandEncoder struct {
	device      *Device
	pool        *CommandPool
	cmdBuffer   vk.CommandBuffer
	label       string
	isRecording bool
}

// BeginEncoding begins command recording.
func (e *CommandEncoder) BeginEncoding(label string) error {
	e.label = label

	// Begin command buffer
	beginInfo := vk.CommandBufferBeginInfo{
		SType: vk.StructureTypeCommandBufferBeginInfo,
		Flags: vk.CommandBufferUsageFlags(vk.CommandBufferUsageOneTimeSubmitBit),
	}

	result := vkBeginCommandBuffer(e.device.cmds, e.cmdBuffer, &beginInfo)
	if result != vk.Success {
		return fmt.Errorf("vulkan: vkBeginCommandBuffer failed: %d", result)
	}

	e.isRecording = true
	return nil
}

// EndEncoding finishes command recording and returns a command buffer.
func (e *CommandEncoder) EndEncoding() (hal.CommandBuffer, error) {
	if !e.isRecording {
		return nil, fmt.Errorf("vulkan: command encoder is not recording")
	}

	result := vkEndCommandBuffer(e.device.cmds, e.cmdBuffer)
	if result != vk.Success {
		return nil, fmt.Errorf("vulkan: vkEndCommandBuffer failed: %d", result)
	}

	e.isRecording = false

	return &CommandBuffer{
		handle: e.cmdBuffer,
		pool:   e.pool,
	}, nil
}

// DiscardEncoding discards the encoder.
func (e *CommandEncoder) DiscardEncoding() {
	if e.isRecording {
		// End the command buffer even though we're discarding it
		_ = vkEndCommandBuffer(e.device.cmds, e.cmdBuffer)
		e.isRecording = false
	}
}

// ResetAll resets command buffers for reuse.
func (e *CommandEncoder) ResetAll(commandBuffers []hal.CommandBuffer) {
	// Reset the pool instead of individual buffers for better performance
	if e.pool != nil {
		vkResetCommandPool(e.device.cmds, e.device.handle, e.pool.handle, 0)
	}
	_ = commandBuffers // Individual buffers are reset with the pool
}

// TransitionBuffers transitions buffer states for synchronization.
func (e *CommandEncoder) TransitionBuffers(barriers []hal.BufferBarrier) {
	if !e.isRecording || len(barriers) == 0 {
		return
	}

	// Convert to Vulkan buffer memory barriers
	bufferBarriers := make([]vk.BufferMemoryBarrier, len(barriers))
	for i, b := range barriers {
		buf, ok := b.Buffer.(*Buffer)
		if !ok {
			continue
		}

		srcAccess, srcStage := bufferUsageToAccessAndStage(b.Usage.OldUsage)
		dstAccess, dstStage := bufferUsageToAccessAndStage(b.Usage.NewUsage)

		bufferBarriers[i] = vk.BufferMemoryBarrier{
			SType:               vk.StructureTypeBufferMemoryBarrier,
			SrcAccessMask:       srcAccess,
			DstAccessMask:       dstAccess,
			SrcQueueFamilyIndex: vk.QueueFamilyIgnored,
			DstQueueFamilyIndex: vk.QueueFamilyIgnored,
			Buffer:              buf.handle,
			Offset:              0,
			Size:                vk.DeviceSize(vk.WholeSize),
		}

		// Track pipeline stages for the barrier command
		_ = srcStage
		_ = dstStage
	}

	// Use vkCmdPipelineBarrier with buffer memory barriers
	vkCmdPipelineBarrier(
		e.device.cmds,
		e.cmdBuffer,
		vk.PipelineStageFlags(vk.PipelineStageAllCommandsBit),
		vk.PipelineStageFlags(vk.PipelineStageAllCommandsBit),
		0,      // dependencyFlags
		0, nil, // memory barriers
		uint32(len(bufferBarriers)), &bufferBarriers[0],
		0, nil, // image barriers
	)
}

// TransitionTextures transitions texture states for synchronization.
func (e *CommandEncoder) TransitionTextures(barriers []hal.TextureBarrier) {
	if !e.isRecording || len(barriers) == 0 {
		return
	}

	// Convert to Vulkan image memory barriers
	imageBarriers := make([]vk.ImageMemoryBarrier, len(barriers))
	for i, b := range barriers {
		tex, ok := b.Texture.(*Texture)
		if !ok {
			continue
		}

		srcAccess, srcStage, oldLayout := textureUsageToAccessStageLayout(b.Usage.OldUsage)
		dstAccess, dstStage, newLayout := textureUsageToAccessStageLayout(b.Usage.NewUsage)

		imageBarriers[i] = vk.ImageMemoryBarrier{
			SType:               vk.StructureTypeImageMemoryBarrier,
			SrcAccessMask:       srcAccess,
			DstAccessMask:       dstAccess,
			OldLayout:           oldLayout,
			NewLayout:           newLayout,
			SrcQueueFamilyIndex: vk.QueueFamilyIgnored,
			DstQueueFamilyIndex: vk.QueueFamilyIgnored,
			Image:               tex.handle,
			SubresourceRange: vk.ImageSubresourceRange{
				AspectMask:     textureAspectToVk(b.Range.Aspect, tex.format),
				BaseMipLevel:   b.Range.BaseMipLevel,
				LevelCount:     mipLevelCountOrRemaining(b.Range.MipLevelCount),
				BaseArrayLayer: b.Range.BaseArrayLayer,
				LayerCount:     arrayLayerCountOrRemaining(b.Range.ArrayLayerCount),
			},
		}

		_ = srcStage
		_ = dstStage
	}

	vkCmdPipelineBarrier(
		e.device.cmds,
		e.cmdBuffer,
		vk.PipelineStageFlags(vk.PipelineStageAllCommandsBit),
		vk.PipelineStageFlags(vk.PipelineStageAllCommandsBit),
		0,
		0, nil,
		0, nil,
		uint32(len(imageBarriers)), &imageBarriers[0],
	)
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

	// vkCmdFillBuffer fills with a 32-bit value (0 for zero fill)
	vkCmdFillBuffer(e.device.cmds, e.cmdBuffer, buf.handle, vk.DeviceSize(offset), vk.DeviceSize(size), 0)
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

	vkRegions := make([]vk.BufferCopy, len(regions))
	for i, r := range regions {
		vkRegions[i] = vk.BufferCopy{
			SrcOffset: vk.DeviceSize(r.SrcOffset),
			DstOffset: vk.DeviceSize(r.DstOffset),
			Size:      vk.DeviceSize(r.Size),
		}
	}

	vkCmdCopyBuffer(e.device.cmds, e.cmdBuffer, srcBuf.handle, dstBuf.handle, uint32(len(vkRegions)), &vkRegions[0])
}

// convertBufferImageCopyRegions converts HAL BufferTextureCopy regions to Vulkan BufferImageCopy.
func convertBufferImageCopyRegions(regions []hal.BufferTextureCopy) []vk.BufferImageCopy {
	vkRegions := make([]vk.BufferImageCopy, len(regions))
	for i, r := range regions {
		// Vulkan bufferRowLength is in TEXELS, not bytes!
		// For RGBA8 format (4 bytes per texel), convert bytes to texels.
		// A value of 0 means tightly packed (row length = image width).
		bufferRowLength := uint32(0)
		if r.BufferLayout.BytesPerRow > 0 && r.Size.Width > 0 {
			bytesPerTexel := r.BufferLayout.BytesPerRow / r.Size.Width
			if bytesPerTexel > 0 {
				bufferRowLength = r.BufferLayout.BytesPerRow / bytesPerTexel
			}
		}

		vkRegions[i] = vk.BufferImageCopy{
			BufferOffset:      vk.DeviceSize(r.BufferLayout.Offset),
			BufferRowLength:   bufferRowLength,
			BufferImageHeight: r.BufferLayout.RowsPerImage,
			ImageSubresource: vk.ImageSubresourceLayers{
				AspectMask:     textureAspectToVkSimple(r.TextureBase.Aspect),
				MipLevel:       r.TextureBase.MipLevel,
				BaseArrayLayer: 0,
				LayerCount:     1,
			},
			ImageOffset: vk.Offset3D{
				X: int32(r.TextureBase.Origin.X),
				Y: int32(r.TextureBase.Origin.Y),
				Z: int32(r.TextureBase.Origin.Z),
			},
			ImageExtent: vk.Extent3D{
				Width:  r.Size.Width,
				Height: r.Size.Height,
				Depth:  r.Size.DepthOrArrayLayers,
			},
		}
	}
	return vkRegions
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

	vkRegions := convertBufferImageCopyRegions(regions)
	vkCmdCopyBufferToImage(
		e.device.cmds,
		e.cmdBuffer,
		srcBuf.handle,
		dstTex.handle,
		vk.ImageLayoutTransferDstOptimal,
		uint32(len(vkRegions)),
		&vkRegions[0],
	)
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

	vkRegions := convertBufferImageCopyRegions(regions)
	vkCmdCopyImageToBuffer(
		e.device.cmds,
		e.cmdBuffer,
		srcTex.handle,
		vk.ImageLayoutTransferSrcOptimal,
		dstBuf.handle,
		uint32(len(vkRegions)),
		&vkRegions[0],
	)
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

	vkRegions := make([]vk.ImageCopy, len(regions))
	for i, r := range regions {
		vkRegions[i] = vk.ImageCopy{
			SrcSubresource: vk.ImageSubresourceLayers{
				AspectMask:     textureAspectToVk(r.SrcBase.Aspect, srcTex.format),
				MipLevel:       r.SrcBase.MipLevel,
				BaseArrayLayer: 0,
				LayerCount:     1,
			},
			SrcOffset: vk.Offset3D{
				X: int32(r.SrcBase.Origin.X),
				Y: int32(r.SrcBase.Origin.Y),
				Z: int32(r.SrcBase.Origin.Z),
			},
			DstSubresource: vk.ImageSubresourceLayers{
				AspectMask:     textureAspectToVk(r.DstBase.Aspect, dstTex.format),
				MipLevel:       r.DstBase.MipLevel,
				BaseArrayLayer: 0,
				LayerCount:     1,
			},
			DstOffset: vk.Offset3D{
				X: int32(r.DstBase.Origin.X),
				Y: int32(r.DstBase.Origin.Y),
				Z: int32(r.DstBase.Origin.Z),
			},
			Extent: vk.Extent3D{
				Width:  r.Size.Width,
				Height: r.Size.Height,
				Depth:  r.Size.DepthOrArrayLayers,
			},
		}
	}

	vkCmdCopyImage(
		e.device.cmds,
		e.cmdBuffer,
		srcTex.handle,
		vk.ImageLayoutTransferSrcOptimal,
		dstTex.handle,
		vk.ImageLayoutTransferDstOptimal,
		uint32(len(vkRegions)),
		&vkRegions[0],
	)
}

// BeginRenderPass begins a render pass using VkRenderPass (classic Vulkan approach).
// This is compatible with Intel drivers that don't properly support dynamic rendering.
// Supports MSAA render passes with resolve targets and depth/stencil attachments.
func (e *CommandEncoder) BeginRenderPass(desc *hal.RenderPassDescriptor) hal.RenderPassEncoder {
	rpe := &RenderPassEncoder{
		encoder: e,
		desc:    desc,
	}

	if !e.isRecording || len(desc.ColorAttachments) == 0 {
		return rpe
	}

	// Get first color attachment info
	ca := desc.ColorAttachments[0]
	view, ok := ca.View.(*TextureView)
	if !ok {
		return rpe
	}

	renderWidth := view.size.Width
	renderHeight := view.size.Height

	// Determine color format from the view
	var colorFormat vk.Format
	if view.texture != nil {
		colorFormat = textureFormatToVk(view.texture.format)
	} else if view.isSwapchain {
		// Use the format stored in the view (set when creating swapchain view)
		colorFormat = view.vkFormat
	}

	// Get sample count from the view's texture (defaults to 1)
	sampleCount := vk.SampleCountFlagBits(1)
	if view.texture != nil && view.texture.samples > 1 {
		sampleCount = vk.SampleCountFlagBits(view.texture.samples)
	}

	// Check for MSAA resolve target.
	// Resolve is only meaningful when the color attachment has multiple samples.
	// The resolve attachment count must match between render pass and framebuffer,
	// so we use hasMSAAResolve consistently for both.
	var resolveView *TextureView
	if ca.ResolveTarget != nil {
		resolveView, _ = ca.ResolveTarget.(*TextureView)
	}
	hasMSAAResolve := resolveView != nil && sampleCount > vk.SampleCountFlagBits(1)

	// Determine the final layout for the "output" attachment:
	// - Without MSAA: the color attachment itself
	// - With MSAA: the resolve target (the MSAA color stays ColorAttachmentOptimal)
	colorFinalLayout := vk.ImageLayoutPresentSrcKhr // Default for swapchain
	if !view.isSwapchain {
		// Offscreen rendering
		colorFinalLayout = vk.ImageLayoutColorAttachmentOptimal
	}
	if hasMSAAResolve {
		// With resolve, the final layout applies to the resolve target.
		// Check if the resolve target is a swapchain image.
		if resolveView.isSwapchain {
			colorFinalLayout = vk.ImageLayoutPresentSrcKhr
		} else {
			colorFinalLayout = vk.ImageLayoutColorAttachmentOptimal
		}
	}

	// Build render pass key
	rpKey := RenderPassKey{
		ColorFormat:      colorFormat,
		ColorLoadOp:      loadOpToVk(ca.LoadOp),
		ColorStoreOp:     storeOpToVk(ca.StoreOp),
		SampleCount:      sampleCount,
		ColorFinalLayout: colorFinalLayout,
		HasResolve:       hasMSAAResolve,
	}

	// Handle depth/stencil attachment
	if desc.DepthStencilAttachment != nil {
		dsa := desc.DepthStencilAttachment
		if dsView, ok := dsa.View.(*TextureView); ok && dsView.texture != nil {
			rpKey.DepthFormat = textureFormatToVk(dsView.texture.format)
			rpKey.DepthLoadOp = loadOpToVk(dsa.DepthLoadOp)
			rpKey.DepthStoreOp = storeOpToVk(dsa.DepthStoreOp)
			rpKey.StencilLoadOp = loadOpToVk(dsa.StencilLoadOp)
			rpKey.StencilStoreOp = storeOpToVk(dsa.StencilStoreOp)
		}
	}

	// Get or create render pass from cache
	cache := e.device.GetRenderPassCache()
	renderPass, err := cache.GetOrCreateRenderPass(rpKey)
	if err != nil {
		return rpe
	}
	rpe.renderPass = renderPass

	// Build framebuffer key with all attachment views
	fbKey := FramebufferKey{
		RenderPass: renderPass,
		ColorView:  view.handle,
		Width:      renderWidth,
		Height:     renderHeight,
	}
	if hasMSAAResolve {
		fbKey.ResolveView = resolveView.handle
	}
	if desc.DepthStencilAttachment != nil {
		if dsView, ok := desc.DepthStencilAttachment.View.(*TextureView); ok {
			fbKey.DepthView = dsView.handle
		}
	}

	// Get or create framebuffer from cache
	framebuffer, err := cache.GetOrCreateFramebuffer(fbKey)
	if err != nil {
		return rpe
	}
	rpe.framebuffer = framebuffer

	// Prepare clear values.
	// Order MUST match attachment order in the render pass:
	// [0] color, [1] resolve (if present), [2] depth/stencil (if present).
	clearValues := make([]vk.ClearValue, 0, 3)
	clearValues = append(clearValues, vk.ClearValueColor(
		float32(ca.ClearValue.R),
		float32(ca.ClearValue.G),
		float32(ca.ClearValue.B),
		float32(ca.ClearValue.A),
	))

	if hasMSAAResolve {
		// Resolve attachment clear value (not used since LoadOp is DontCare,
		// but Vulkan requires one clear value per attachment)
		clearValues = append(clearValues, vk.ClearValueColor(0, 0, 0, 0))
	}

	if desc.DepthStencilAttachment != nil {
		dsa := desc.DepthStencilAttachment
		clearValues = append(clearValues, vk.ClearValueDepthStencil(dsa.DepthClearValue, dsa.StencilClearValue))
	}

	// Begin render pass
	renderPassBegin := vk.RenderPassBeginInfo{
		SType:       vk.StructureTypeRenderPassBeginInfo,
		RenderPass:  renderPass,
		Framebuffer: framebuffer,
		RenderArea: vk.Rect2D{
			Offset: vk.Offset2D{X: 0, Y: 0},
			Extent: vk.Extent2D{Width: renderWidth, Height: renderHeight},
		},
		ClearValueCount: uint32(len(clearValues)),
		PClearValues:    &clearValues[0],
	}

	vkCmdBeginRenderPass(e.device.cmds, e.cmdBuffer, &renderPassBegin, vk.SubpassContentsInline)
	runtime.KeepAlive(clearValues)

	// Set default viewport and scissor for the render area.
	// These are required since the pipeline uses dynamic viewport/scissor state.
	// NOTE: Viewport Y-flip is required for WebGPU/OpenGL coordinate system compatibility.
	// Vulkan has Y pointing down, WebGPU has Y pointing up.
	// Solution: Start Y at height and use negative height (matches Rust wgpu).
	if renderWidth > 0 && renderHeight > 0 {
		// Y-flip for WebGPU compatibility: Vulkan Y points down, WebGPU Y points up.
		// Use negative height and start Y at bottom (matches Rust wgpu approach).
		viewport := vk.Viewport{
			X:        0,
			Y:        float32(renderHeight), // Start at bottom
			Width:    float32(renderWidth),
			Height:   -float32(renderHeight), // Negative height for Y-flip
			MinDepth: 0.0,
			MaxDepth: 1.0,
		}
		vkCmdSetViewport(e.device.cmds, e.cmdBuffer, 0, 1, &viewport)

		scissor := vk.Rect2D{
			Offset: vk.Offset2D{X: 0, Y: 0},
			Extent: vk.Extent2D{Width: renderWidth, Height: renderHeight},
		}
		vkCmdSetScissor(e.device.cmds, e.cmdBuffer, 0, 1, &scissor)
	}

	return rpe
}

// BeginComputePass begins a compute pass.
func (e *CommandEncoder) BeginComputePass(desc *hal.ComputePassDescriptor) hal.ComputePassEncoder {
	_ = desc // Compute passes don't need Vulkan-level begin/end
	return &ComputePassEncoder{
		encoder: e,
	}
}

// RenderPassEncoder implements hal.RenderPassEncoder for Vulkan.
type RenderPassEncoder struct {
	encoder     *CommandEncoder
	desc        *hal.RenderPassDescriptor
	pipeline    *RenderPipeline
	indexFormat gputypes.IndexFormat
	// For VkRenderPass-based rendering (not dynamic rendering)
	renderPass  vk.RenderPass
	framebuffer vk.Framebuffer
}

// End finishes the render pass.
func (e *RenderPassEncoder) End() {
	if !e.encoder.isRecording {
		return
	}

	// Use vkCmdEndRenderPass (VkRenderPass handles layout transitions automatically
	// via FinalLayout in AttachmentDescription)
	vkCmdEndRenderPass(e.encoder.device.cmds, e.encoder.cmdBuffer)
}

// SetPipeline sets the render pipeline.
func (e *RenderPassEncoder) SetPipeline(pipeline hal.RenderPipeline) {
	p, ok := pipeline.(*RenderPipeline)
	if !ok || !e.encoder.isRecording {
		return
	}
	e.pipeline = p
	vkCmdBindPipeline(e.encoder.device.cmds, e.encoder.cmdBuffer, vk.PipelineBindPointGraphics, p.handle)
}

// SetBindGroup sets a bind group.
func (e *RenderPassEncoder) SetBindGroup(index uint32, group hal.BindGroup, offsets []uint32) {
	bg, ok := group.(*BindGroup)
	if !ok || !e.encoder.isRecording {
		return
	}

	var pOffsets *uint32
	if len(offsets) > 0 {
		pOffsets = &offsets[0]
	}

	vkCmdBindDescriptorSets(
		e.encoder.device.cmds,
		e.encoder.cmdBuffer,
		vk.PipelineBindPointGraphics,
		e.pipeline.layout,
		index,
		1,
		&bg.handle,
		uint32(len(offsets)),
		pOffsets,
	)
}

// SetVertexBuffer sets a vertex buffer.
func (e *RenderPassEncoder) SetVertexBuffer(slot uint32, buffer hal.Buffer, offset uint64) {
	buf, ok := buffer.(*Buffer)
	if !ok || !e.encoder.isRecording {
		return
	}

	offsets := []vk.DeviceSize{vk.DeviceSize(offset)}
	buffers := []vk.Buffer{buf.handle}

	vkCmdBindVertexBuffers(e.encoder.device.cmds, e.encoder.cmdBuffer, slot, 1, &buffers[0], &offsets[0])
}

// SetIndexBuffer sets the index buffer.
func (e *RenderPassEncoder) SetIndexBuffer(buffer hal.Buffer, format gputypes.IndexFormat, offset uint64) {
	buf, ok := buffer.(*Buffer)
	if !ok || !e.encoder.isRecording {
		return
	}

	e.indexFormat = format
	indexType := vk.IndexTypeUint16
	if format == gputypes.IndexFormatUint32 {
		indexType = vk.IndexTypeUint32
	}

	vkCmdBindIndexBuffer(e.encoder.device.cmds, e.encoder.cmdBuffer, buf.handle, vk.DeviceSize(offset), indexType)
}

// SetViewport sets the viewport.
// NOTE: Applies Y-flip for WebGPU/OpenGL coordinate system compatibility (matches Rust wgpu).
func (e *RenderPassEncoder) SetViewport(x, y, width, height, minDepth, maxDepth float32) {
	if !e.encoder.isRecording {
		return
	}

	// Y-flip: Start Y at y+height, use negative height
	viewport := vk.Viewport{
		X:        x,
		Y:        y + height, // Y-flip: start at bottom
		Width:    width,
		Height:   -height, // Y-flip: negative height
		MinDepth: minDepth,
		MaxDepth: maxDepth,
	}

	vkCmdSetViewport(e.encoder.device.cmds, e.encoder.cmdBuffer, 0, 1, &viewport)
}

// SetScissorRect sets the scissor rectangle.
func (e *RenderPassEncoder) SetScissorRect(x, y, width, height uint32) {
	if !e.encoder.isRecording {
		return
	}

	scissor := vk.Rect2D{
		Offset: vk.Offset2D{X: int32(x), Y: int32(y)},
		Extent: vk.Extent2D{Width: width, Height: height},
	}

	vkCmdSetScissor(e.encoder.device.cmds, e.encoder.cmdBuffer, 0, 1, &scissor)
}

// SetBlendConstant sets the blend constant.
func (e *RenderPassEncoder) SetBlendConstant(color *gputypes.Color) {
	if !e.encoder.isRecording || color == nil {
		return
	}

	blendConstants := [4]float32{
		float32(color.R),
		float32(color.G),
		float32(color.B),
		float32(color.A),
	}

	vkCmdSetBlendConstants(e.encoder.device.cmds, e.encoder.cmdBuffer, &blendConstants)
}

// SetStencilReference sets the stencil reference value.
func (e *RenderPassEncoder) SetStencilReference(ref uint32) {
	if !e.encoder.isRecording {
		return
	}

	// Set for both front and back faces
	vkCmdSetStencilReference(e.encoder.device.cmds, e.encoder.cmdBuffer, vk.StencilFaceFlags(vk.StencilFaceFrontAndBack), ref)
}

// Draw draws primitives.
func (e *RenderPassEncoder) Draw(vertexCount, instanceCount, firstVertex, firstInstance uint32) {
	if !e.encoder.isRecording {
		return
	}
	vkCmdDraw(e.encoder.device.cmds, e.encoder.cmdBuffer, vertexCount, instanceCount, firstVertex, firstInstance)
}

// DrawIndexed draws indexed primitives.
func (e *RenderPassEncoder) DrawIndexed(indexCount, instanceCount, firstIndex uint32, baseVertex int32, firstInstance uint32) {
	if !e.encoder.isRecording {
		return
	}

	vkCmdDrawIndexed(e.encoder.device.cmds, e.encoder.cmdBuffer, indexCount, instanceCount, firstIndex, baseVertex, firstInstance)
}

// DrawIndirect draws primitives with GPU-generated parameters.
func (e *RenderPassEncoder) DrawIndirect(buffer hal.Buffer, offset uint64) {
	buf, ok := buffer.(*Buffer)
	if !ok || !e.encoder.isRecording {
		return
	}

	vkCmdDrawIndirect(e.encoder.device.cmds, e.encoder.cmdBuffer, buf.handle, vk.DeviceSize(offset), 1, 0)
}

// DrawIndexedIndirect draws indexed primitives with GPU-generated parameters.
func (e *RenderPassEncoder) DrawIndexedIndirect(buffer hal.Buffer, offset uint64) {
	buf, ok := buffer.(*Buffer)
	if !ok || !e.encoder.isRecording {
		return
	}

	vkCmdDrawIndexedIndirect(e.encoder.device.cmds, e.encoder.cmdBuffer, buf.handle, vk.DeviceSize(offset), 1, 0)
}

// ExecuteBundle executes a pre-recorded render bundle.
func (e *RenderPassEncoder) ExecuteBundle(bundle hal.RenderBundle) {
	vkBundle, ok := bundle.(*RenderBundle)
	if !ok || vkBundle == nil || !e.encoder.isRecording {
		return
	}

	// Execute the secondary command buffer
	e.encoder.device.cmds.CmdExecuteCommands(
		e.encoder.cmdBuffer,
		1,
		&vkBundle.commandBuffer,
	)
}

// ComputePassEncoder implements hal.ComputePassEncoder for Vulkan.
type ComputePassEncoder struct {
	encoder  *CommandEncoder
	pipeline *ComputePipeline
}

// End finishes the compute pass.
// Inserts a global memory barrier so compute shader writes are visible to
// subsequent commands (transfers, other dispatches, etc.). Without this
// barrier the GPU may reorder a CopyBufferToBuffer before the compute
// shader has finished writing, causing stale/zero reads.
func (e *ComputePassEncoder) End() {
	if e.encoder == nil || !e.encoder.isRecording {
		return
	}

	// Global memory barrier: compute writes → everything after.
	memBarrier := vk.MemoryBarrier{
		SType:         vk.StructureTypeMemoryBarrier,
		SrcAccessMask: vk.AccessFlags(vk.AccessShaderWriteBit),
		DstAccessMask: vk.AccessFlags(vk.AccessShaderReadBit | vk.AccessTransferReadBit | vk.AccessTransferWriteBit | vk.AccessHostReadBit),
	}
	vkCmdPipelineBarrier(
		e.encoder.device.cmds,
		e.encoder.cmdBuffer,
		vk.PipelineStageFlags(vk.PipelineStageComputeShaderBit),
		vk.PipelineStageFlags(vk.PipelineStageComputeShaderBit|vk.PipelineStageTransferBit|vk.PipelineStageHostBit),
		0,
		1, &memBarrier,
		0, nil,
		0, nil,
	)
}

// SetPipeline sets the compute pipeline.
func (e *ComputePassEncoder) SetPipeline(pipeline hal.ComputePipeline) {
	p, ok := pipeline.(*ComputePipeline)
	if !ok || !e.encoder.isRecording {
		return
	}
	e.pipeline = p

	vkCmdBindPipeline(e.encoder.device.cmds, e.encoder.cmdBuffer, vk.PipelineBindPointCompute, p.handle)
}

// SetBindGroup sets a bind group.
func (e *ComputePassEncoder) SetBindGroup(index uint32, group hal.BindGroup, offsets []uint32) {
	bg, ok := group.(*BindGroup)
	if !ok || !e.encoder.isRecording || e.pipeline == nil {
		return
	}

	var pOffsets *uint32
	if len(offsets) > 0 {
		pOffsets = &offsets[0]
	}

	vkCmdBindDescriptorSets(
		e.encoder.device.cmds,
		e.encoder.cmdBuffer,
		vk.PipelineBindPointCompute,
		e.pipeline.layout,
		index,
		1,
		&bg.handle,
		uint32(len(offsets)),
		pOffsets,
	)
}

// Dispatch dispatches compute work.
func (e *ComputePassEncoder) Dispatch(x, y, z uint32) {
	if !e.encoder.isRecording {
		return
	}

	vkCmdDispatch(e.encoder.device.cmds, e.encoder.cmdBuffer, x, y, z)
}

// DispatchIndirect dispatches compute work with GPU-generated parameters.
func (e *ComputePassEncoder) DispatchIndirect(buffer hal.Buffer, offset uint64) {
	buf, ok := buffer.(*Buffer)
	if !ok || !e.encoder.isRecording {
		return
	}

	vkCmdDispatchIndirect(e.encoder.device.cmds, e.encoder.cmdBuffer, buf.handle, vk.DeviceSize(offset))
}

// --- Helper functions ---

//nolint:unparam // stage will be used when barrier optimization is implemented
func bufferUsageToAccessAndStage(usage gputypes.BufferUsage) (vk.AccessFlags, vk.PipelineStageFlags) {
	var access vk.AccessFlags
	var stage vk.PipelineStageFlags

	if usage&gputypes.BufferUsageCopySrc != 0 {
		access |= vk.AccessFlags(vk.AccessTransferReadBit)
		stage |= vk.PipelineStageFlags(vk.PipelineStageTransferBit)
	}
	if usage&gputypes.BufferUsageCopyDst != 0 {
		access |= vk.AccessFlags(vk.AccessTransferWriteBit)
		stage |= vk.PipelineStageFlags(vk.PipelineStageTransferBit)
	}
	if usage&gputypes.BufferUsageVertex != 0 {
		access |= vk.AccessFlags(vk.AccessVertexAttributeReadBit)
		stage |= vk.PipelineStageFlags(vk.PipelineStageVertexInputBit)
	}
	if usage&gputypes.BufferUsageIndex != 0 {
		access |= vk.AccessFlags(vk.AccessIndexReadBit)
		stage |= vk.PipelineStageFlags(vk.PipelineStageVertexInputBit)
	}
	if usage&gputypes.BufferUsageUniform != 0 {
		access |= vk.AccessFlags(vk.AccessUniformReadBit)
		stage |= vk.PipelineStageFlags(vk.PipelineStageVertexShaderBit | vk.PipelineStageFragmentShaderBit)
	}
	if usage&gputypes.BufferUsageStorage != 0 {
		access |= vk.AccessFlags(vk.AccessShaderReadBit | vk.AccessShaderWriteBit)
		stage |= vk.PipelineStageFlags(vk.PipelineStageVertexShaderBit | vk.PipelineStageFragmentShaderBit | vk.PipelineStageComputeShaderBit)
	}
	if usage&gputypes.BufferUsageIndirect != 0 {
		access |= vk.AccessFlags(vk.AccessIndirectCommandReadBit)
		stage |= vk.PipelineStageFlags(vk.PipelineStageDrawIndirectBit)
	}

	if stage == 0 {
		stage = vk.PipelineStageFlags(vk.PipelineStageTopOfPipeBit)
	}

	return access, stage
}

//nolint:unparam // stage will be used when barrier optimization is implemented
func textureUsageToAccessStageLayout(usage gputypes.TextureUsage) (vk.AccessFlags, vk.PipelineStageFlags, vk.ImageLayout) {
	// Usage 0 means "initial/undefined" — the image has no prior usage.
	// Newly created Vulkan images start in VK_IMAGE_LAYOUT_UNDEFINED.
	// Using ImageLayoutGeneral here would lie about the old layout,
	// causing validation errors and undefined behavior on the barrier.
	if usage == 0 {
		return 0, vk.PipelineStageFlags(vk.PipelineStageTopOfPipeBit), vk.ImageLayoutUndefined
	}

	var access vk.AccessFlags
	var stage vk.PipelineStageFlags
	layout := vk.ImageLayoutGeneral

	if usage&gputypes.TextureUsageCopySrc != 0 {
		access |= vk.AccessFlags(vk.AccessTransferReadBit)
		stage |= vk.PipelineStageFlags(vk.PipelineStageTransferBit)
		layout = vk.ImageLayoutTransferSrcOptimal
	}
	if usage&gputypes.TextureUsageCopyDst != 0 {
		access |= vk.AccessFlags(vk.AccessTransferWriteBit)
		stage |= vk.PipelineStageFlags(vk.PipelineStageTransferBit)
		layout = vk.ImageLayoutTransferDstOptimal
	}
	if usage&gputypes.TextureUsageTextureBinding != 0 {
		access |= vk.AccessFlags(vk.AccessShaderReadBit)
		stage |= vk.PipelineStageFlags(vk.PipelineStageFragmentShaderBit)
		layout = vk.ImageLayoutShaderReadOnlyOptimal
	}
	if usage&gputypes.TextureUsageStorageBinding != 0 {
		access |= vk.AccessFlags(vk.AccessShaderReadBit | vk.AccessShaderWriteBit)
		stage |= vk.PipelineStageFlags(vk.PipelineStageComputeShaderBit)
		layout = vk.ImageLayoutGeneral
	}
	if usage&gputypes.TextureUsageRenderAttachment != 0 {
		access |= vk.AccessFlags(vk.AccessColorAttachmentWriteBit | vk.AccessColorAttachmentReadBit)
		stage |= vk.PipelineStageFlags(vk.PipelineStageColorAttachmentOutputBit)
		layout = vk.ImageLayoutColorAttachmentOptimal
	}

	if stage == 0 {
		stage = vk.PipelineStageFlags(vk.PipelineStageTopOfPipeBit)
	}

	return access, stage, layout
}

func mipLevelCountOrRemaining(count uint32) uint32 {
	if count == 0 {
		return vk.RemainingMipLevels
	}
	return count
}

func arrayLayerCountOrRemaining(count uint32) uint32 {
	if count == 0 {
		return vk.RemainingArrayLayers
	}
	return count
}

func loadOpToVk(op gputypes.LoadOp) vk.AttachmentLoadOp {
	switch op {
	case gputypes.LoadOpClear:
		return vk.AttachmentLoadOpClear
	case gputypes.LoadOpLoad:
		return vk.AttachmentLoadOpLoad
	default:
		return vk.AttachmentLoadOpDontCare
	}
}

func storeOpToVk(op gputypes.StoreOp) vk.AttachmentStoreOp {
	switch op {
	case gputypes.StoreOpStore:
		return vk.AttachmentStoreOpStore
	default:
		return vk.AttachmentStoreOpDontCare
	}
}

// --- Vulkan function wrappers ---

func vkBeginCommandBuffer(cmds *vk.Commands, cmdBuffer vk.CommandBuffer, beginInfo *vk.CommandBufferBeginInfo) vk.Result {
	return cmds.BeginCommandBuffer(cmdBuffer, beginInfo)
}

func vkEndCommandBuffer(cmds *vk.Commands, cmdBuffer vk.CommandBuffer) vk.Result {
	return cmds.EndCommandBuffer(cmdBuffer)
}

func vkResetCommandPool(cmds *vk.Commands, device vk.Device, pool vk.CommandPool, flags vk.CommandPoolResetFlags) vk.Result {
	return cmds.ResetCommandPool(device, pool, flags)
}

func vkCmdPipelineBarrier(cmds *vk.Commands, cmdBuffer vk.CommandBuffer,
	srcStageMask, dstStageMask vk.PipelineStageFlags,
	dependencyFlags vk.DependencyFlags,
	memoryBarrierCount uint32, pMemoryBarriers *vk.MemoryBarrier,
	bufferMemoryBarrierCount uint32, pBufferMemoryBarriers *vk.BufferMemoryBarrier,
	imageMemoryBarrierCount uint32, pImageMemoryBarriers *vk.ImageMemoryBarrier) {
	cmds.CmdPipelineBarrier(cmdBuffer, srcStageMask, dstStageMask, dependencyFlags,
		memoryBarrierCount, pMemoryBarriers,
		bufferMemoryBarrierCount, pBufferMemoryBarriers,
		imageMemoryBarrierCount, pImageMemoryBarriers)
}

func vkCmdFillBuffer(cmds *vk.Commands, cmdBuffer vk.CommandBuffer, buffer vk.Buffer, offset, size vk.DeviceSize, data uint32) {
	cmds.CmdFillBuffer(cmdBuffer, buffer, offset, size, data)
}

func vkCmdCopyBuffer(cmds *vk.Commands, cmdBuffer vk.CommandBuffer, src, dst vk.Buffer, regionCount uint32, pRegions *vk.BufferCopy) {
	cmds.CmdCopyBuffer(cmdBuffer, src, dst, regionCount, pRegions)
}

func vkCmdCopyBufferToImage(cmds *vk.Commands, cmdBuffer vk.CommandBuffer, src vk.Buffer, dst vk.Image, layout vk.ImageLayout, regionCount uint32, pRegions *vk.BufferImageCopy) {
	cmds.CmdCopyBufferToImage(cmdBuffer, src, dst, layout, regionCount, pRegions)
}

func vkCmdCopyImageToBuffer(cmds *vk.Commands, cmdBuffer vk.CommandBuffer, src vk.Image, layout vk.ImageLayout, dst vk.Buffer, regionCount uint32, pRegions *vk.BufferImageCopy) {
	cmds.CmdCopyImageToBuffer(cmdBuffer, src, layout, dst, regionCount, pRegions)
}

func vkCmdCopyImage(cmds *vk.Commands, cmdBuffer vk.CommandBuffer, src vk.Image, srcLayout vk.ImageLayout, dst vk.Image, dstLayout vk.ImageLayout, regionCount uint32, pRegions *vk.ImageCopy) {
	cmds.CmdCopyImage(cmdBuffer, src, srcLayout, dst, dstLayout, regionCount, pRegions)
}

//nolint:unused // Reserved for VK_KHR_dynamic_rendering support (disabled on Intel)
func vkCmdBeginRendering(cmds *vk.Commands, cmdBuffer vk.CommandBuffer, renderingInfo *vk.RenderingInfo) {
	cmds.CmdBeginRendering(cmdBuffer, renderingInfo)
}

//nolint:unused // Reserved for VK_KHR_dynamic_rendering support (disabled on Intel)
func vkCmdEndRendering(cmds *vk.Commands, cmdBuffer vk.CommandBuffer) {
	cmds.CmdEndRendering(cmdBuffer)
}

func vkCmdBeginRenderPass(cmds *vk.Commands, cmdBuffer vk.CommandBuffer, renderPassBegin *vk.RenderPassBeginInfo, contents vk.SubpassContents) {
	cmds.CmdBeginRenderPass(cmdBuffer, renderPassBegin, contents)
}

func vkCmdEndRenderPass(cmds *vk.Commands, cmdBuffer vk.CommandBuffer) {
	cmds.CmdEndRenderPass(cmdBuffer)
}

func vkCmdBindPipeline(cmds *vk.Commands, cmdBuffer vk.CommandBuffer, bindPoint vk.PipelineBindPoint, pipeline vk.Pipeline) {
	cmds.CmdBindPipeline(cmdBuffer, bindPoint, pipeline)
}

func vkCmdBindDescriptorSets(cmds *vk.Commands, cmdBuffer vk.CommandBuffer, bindPoint vk.PipelineBindPoint, layout vk.PipelineLayout, firstSet uint32, setCount uint32, pSets *vk.DescriptorSet, dynamicOffsetCount uint32, pDynamicOffsets *uint32) {
	cmds.CmdBindDescriptorSets(cmdBuffer, bindPoint, layout, firstSet, setCount, pSets, dynamicOffsetCount, pDynamicOffsets)
}

func vkCmdBindVertexBuffers(cmds *vk.Commands, cmdBuffer vk.CommandBuffer, firstBinding, bindingCount uint32, pBuffers *vk.Buffer, pOffsets *vk.DeviceSize) {
	cmds.CmdBindVertexBuffers(cmdBuffer, firstBinding, bindingCount, pBuffers, pOffsets)
}

func vkCmdBindIndexBuffer(cmds *vk.Commands, cmdBuffer vk.CommandBuffer, buffer vk.Buffer, offset vk.DeviceSize, indexType vk.IndexType) {
	cmds.CmdBindIndexBuffer(cmdBuffer, buffer, offset, indexType)
}

func vkCmdSetViewport(cmds *vk.Commands, cmdBuffer vk.CommandBuffer, firstViewport, viewportCount uint32, pViewports *vk.Viewport) {
	cmds.CmdSetViewport(cmdBuffer, firstViewport, viewportCount, pViewports)
}

func vkCmdSetScissor(cmds *vk.Commands, cmdBuffer vk.CommandBuffer, firstScissor, scissorCount uint32, pScissors *vk.Rect2D) {
	cmds.CmdSetScissor(cmdBuffer, firstScissor, scissorCount, pScissors)
}

func vkCmdSetBlendConstants(cmds *vk.Commands, cmdBuffer vk.CommandBuffer, blendConstants *[4]float32) {
	cmds.CmdSetBlendConstants(cmdBuffer, *blendConstants)
}

func vkCmdSetStencilReference(cmds *vk.Commands, cmdBuffer vk.CommandBuffer, faceMask vk.StencilFaceFlags, reference uint32) {
	cmds.CmdSetStencilReference(cmdBuffer, faceMask, reference)
}

func vkCmdDraw(cmds *vk.Commands, cmdBuffer vk.CommandBuffer, vertexCount, instanceCount, firstVertex, firstInstance uint32) {
	cmds.CmdDraw(cmdBuffer, vertexCount, instanceCount, firstVertex, firstInstance)
}

func vkCmdDrawIndexed(cmds *vk.Commands, cmdBuffer vk.CommandBuffer, indexCount, instanceCount, firstIndex uint32, vertexOffset int32, firstInstance uint32) {
	cmds.CmdDrawIndexed(cmdBuffer, indexCount, instanceCount, firstIndex, vertexOffset, firstInstance)
}

func vkCmdDrawIndirect(cmds *vk.Commands, cmdBuffer vk.CommandBuffer, buffer vk.Buffer, offset vk.DeviceSize, drawCount, stride uint32) {
	cmds.CmdDrawIndirect(cmdBuffer, buffer, offset, drawCount, stride)
}

func vkCmdDrawIndexedIndirect(cmds *vk.Commands, cmdBuffer vk.CommandBuffer, buffer vk.Buffer, offset vk.DeviceSize, drawCount, stride uint32) {
	cmds.CmdDrawIndexedIndirect(cmdBuffer, buffer, offset, drawCount, stride)
}

func vkCmdDispatch(cmds *vk.Commands, cmdBuffer vk.CommandBuffer, x, y, z uint32) {
	cmds.CmdDispatch(cmdBuffer, x, y, z)
}

func vkCmdDispatchIndirect(cmds *vk.Commands, cmdBuffer vk.CommandBuffer, buffer vk.Buffer, offset vk.DeviceSize) {
	cmds.CmdDispatchIndirect(cmdBuffer, buffer, offset)
}
