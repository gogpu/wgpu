// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows

package vulkan

import (
	"fmt"
	"syscall"
	"unsafe"

	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/vulkan/vk"
	"github.com/gogpu/wgpu/types"
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
	// Command buffers are freed when the pool is destroyed or reset
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
				AspectMask:     textureAspectToVk(b.Range.Aspect),
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
		vkRegions[i] = vk.BufferImageCopy{
			BufferOffset:      vk.DeviceSize(r.BufferLayout.Offset),
			BufferRowLength:   r.BufferLayout.BytesPerRow,
			BufferImageHeight: r.BufferLayout.RowsPerImage,
			ImageSubresource: vk.ImageSubresourceLayers{
				AspectMask:     textureAspectToVk(r.TextureBase.Aspect),
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
				AspectMask:     textureAspectToVk(r.SrcBase.Aspect),
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
				AspectMask:     textureAspectToVk(r.DstBase.Aspect),
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

// BeginRenderPass begins a render pass using dynamic rendering (Vulkan 1.3+).
func (e *CommandEncoder) BeginRenderPass(desc *hal.RenderPassDescriptor) hal.RenderPassEncoder {
	rpe := &RenderPassEncoder{
		encoder: e,
		desc:    desc,
	}

	if !e.isRecording {
		return rpe
	}

	// Use dynamic rendering (VK_KHR_dynamic_rendering / Vulkan 1.3)
	// This avoids the need for VkRenderPass and VkFramebuffer objects
	colorAttachments := make([]vk.RenderingAttachmentInfo, len(desc.ColorAttachments))
	for i, ca := range desc.ColorAttachments {
		view, ok := ca.View.(*TextureView)
		if !ok {
			continue
		}

		colorAttachments[i] = vk.RenderingAttachmentInfo{
			SType:       vk.StructureTypeRenderingAttachmentInfo,
			ImageView:   view.handle,
			ImageLayout: vk.ImageLayoutColorAttachmentOptimal,
			LoadOp:      loadOpToVk(ca.LoadOp),
			StoreOp:     storeOpToVk(ca.StoreOp),
			ClearValue: vk.ClearValueColor(
				float32(ca.ClearValue.R),
				float32(ca.ClearValue.G),
				float32(ca.ClearValue.B),
				float32(ca.ClearValue.A),
			),
		}

		// Handle resolve target for MSAA
		if ca.ResolveTarget != nil {
			resolveView, ok := ca.ResolveTarget.(*TextureView)
			if ok {
				colorAttachments[i].ResolveMode = vk.ResolveModeAverageBit
				colorAttachments[i].ResolveImageView = resolveView.handle
				colorAttachments[i].ResolveImageLayout = vk.ImageLayoutColorAttachmentOptimal
			}
		}
	}

	renderingInfo := vk.RenderingInfo{
		SType: vk.StructureTypeRenderingInfo,
		RenderArea: vk.Rect2D{
			Offset: vk.Offset2D{X: 0, Y: 0},
			Extent: vk.Extent2D{Width: 0, Height: 0}, // Will be set from first attachment
		},
		LayerCount:           1,
		ColorAttachmentCount: uint32(len(colorAttachments)),
	}

	if len(colorAttachments) > 0 {
		renderingInfo.PColorAttachments = &colorAttachments[0]
	}

	// Handle depth/stencil attachment
	var depthAttachment vk.RenderingAttachmentInfo
	if desc.DepthStencilAttachment != nil {
		dsa := desc.DepthStencilAttachment
		view, ok := dsa.View.(*TextureView)
		if ok {
			depthAttachment = vk.RenderingAttachmentInfo{
				SType:       vk.StructureTypeRenderingAttachmentInfo,
				ImageView:   view.handle,
				ImageLayout: vk.ImageLayoutDepthStencilAttachmentOptimal,
				LoadOp:      loadOpToVk(dsa.DepthLoadOp),
				StoreOp:     storeOpToVk(dsa.DepthStoreOp),
				ClearValue:  vk.ClearValueDepthStencil(dsa.DepthClearValue, dsa.StencilClearValue),
			}
			renderingInfo.PDepthAttachment = &depthAttachment
			renderingInfo.PStencilAttachment = &depthAttachment // Same attachment for depth/stencil
		}
	}

	vkCmdBeginRendering(e.device.cmds, e.cmdBuffer, &renderingInfo)

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
	indexFormat types.IndexFormat
}

// End finishes the render pass.
func (e *RenderPassEncoder) End() {
	if e.encoder.isRecording {
		vkCmdEndRendering(e.encoder.device.cmds, e.encoder.cmdBuffer)
	}
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
func (e *RenderPassEncoder) SetIndexBuffer(buffer hal.Buffer, format types.IndexFormat, offset uint64) {
	buf, ok := buffer.(*Buffer)
	if !ok || !e.encoder.isRecording {
		return
	}

	e.indexFormat = format
	indexType := vk.IndexTypeUint16
	if format == types.IndexFormatUint32 {
		indexType = vk.IndexTypeUint32
	}

	vkCmdBindIndexBuffer(e.encoder.device.cmds, e.encoder.cmdBuffer, buf.handle, vk.DeviceSize(offset), indexType)
}

// SetViewport sets the viewport.
func (e *RenderPassEncoder) SetViewport(x, y, width, height, minDepth, maxDepth float32) {
	if !e.encoder.isRecording {
		return
	}

	viewport := vk.Viewport{
		X:        x,
		Y:        y,
		Width:    width,
		Height:   height,
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
func (e *RenderPassEncoder) SetBlendConstant(color *types.Color) {
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
	// TODO: Implement using secondary command buffers
	_ = bundle
}

// ComputePassEncoder implements hal.ComputePassEncoder for Vulkan.
type ComputePassEncoder struct {
	encoder  *CommandEncoder
	pipeline *ComputePipeline
}

// End finishes the compute pass.
func (e *ComputePassEncoder) End() {
	// No Vulkan-level end needed for compute passes
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
func bufferUsageToAccessAndStage(usage types.BufferUsage) (vk.AccessFlags, vk.PipelineStageFlags) {
	var access vk.AccessFlags
	var stage vk.PipelineStageFlags

	if usage&types.BufferUsageCopySrc != 0 {
		access |= vk.AccessFlags(vk.AccessTransferReadBit)
		stage |= vk.PipelineStageFlags(vk.PipelineStageTransferBit)
	}
	if usage&types.BufferUsageCopyDst != 0 {
		access |= vk.AccessFlags(vk.AccessTransferWriteBit)
		stage |= vk.PipelineStageFlags(vk.PipelineStageTransferBit)
	}
	if usage&types.BufferUsageVertex != 0 {
		access |= vk.AccessFlags(vk.AccessVertexAttributeReadBit)
		stage |= vk.PipelineStageFlags(vk.PipelineStageVertexInputBit)
	}
	if usage&types.BufferUsageIndex != 0 {
		access |= vk.AccessFlags(vk.AccessIndexReadBit)
		stage |= vk.PipelineStageFlags(vk.PipelineStageVertexInputBit)
	}
	if usage&types.BufferUsageUniform != 0 {
		access |= vk.AccessFlags(vk.AccessUniformReadBit)
		stage |= vk.PipelineStageFlags(vk.PipelineStageVertexShaderBit | vk.PipelineStageFragmentShaderBit)
	}
	if usage&types.BufferUsageStorage != 0 {
		access |= vk.AccessFlags(vk.AccessShaderReadBit | vk.AccessShaderWriteBit)
		stage |= vk.PipelineStageFlags(vk.PipelineStageVertexShaderBit | vk.PipelineStageFragmentShaderBit | vk.PipelineStageComputeShaderBit)
	}
	if usage&types.BufferUsageIndirect != 0 {
		access |= vk.AccessFlags(vk.AccessIndirectCommandReadBit)
		stage |= vk.PipelineStageFlags(vk.PipelineStageDrawIndirectBit)
	}

	if stage == 0 {
		stage = vk.PipelineStageFlags(vk.PipelineStageTopOfPipeBit)
	}

	return access, stage
}

//nolint:unparam // stage will be used when barrier optimization is implemented
func textureUsageToAccessStageLayout(usage types.TextureUsage) (vk.AccessFlags, vk.PipelineStageFlags, vk.ImageLayout) {
	var access vk.AccessFlags
	var stage vk.PipelineStageFlags
	layout := vk.ImageLayoutGeneral

	if usage&types.TextureUsageCopySrc != 0 {
		access |= vk.AccessFlags(vk.AccessTransferReadBit)
		stage |= vk.PipelineStageFlags(vk.PipelineStageTransferBit)
		layout = vk.ImageLayoutTransferSrcOptimal
	}
	if usage&types.TextureUsageCopyDst != 0 {
		access |= vk.AccessFlags(vk.AccessTransferWriteBit)
		stage |= vk.PipelineStageFlags(vk.PipelineStageTransferBit)
		layout = vk.ImageLayoutTransferDstOptimal
	}
	if usage&types.TextureUsageTextureBinding != 0 {
		access |= vk.AccessFlags(vk.AccessShaderReadBit)
		stage |= vk.PipelineStageFlags(vk.PipelineStageFragmentShaderBit)
		layout = vk.ImageLayoutShaderReadOnlyOptimal
	}
	if usage&types.TextureUsageStorageBinding != 0 {
		access |= vk.AccessFlags(vk.AccessShaderReadBit | vk.AccessShaderWriteBit)
		stage |= vk.PipelineStageFlags(vk.PipelineStageComputeShaderBit)
		layout = vk.ImageLayoutGeneral
	}
	if usage&types.TextureUsageRenderAttachment != 0 {
		access |= vk.AccessFlags(vk.AccessColorAttachmentWriteBit | vk.AccessColorAttachmentReadBit)
		stage |= vk.PipelineStageFlags(vk.PipelineStageColorAttachmentOutputBit)
		layout = vk.ImageLayoutColorAttachmentOptimal
	}

	if stage == 0 {
		stage = vk.PipelineStageFlags(vk.PipelineStageTopOfPipeBit)
	}

	return access, stage, layout
}

func textureAspectToVk(aspect types.TextureAspect) vk.ImageAspectFlags {
	switch aspect {
	case types.TextureAspectDepthOnly:
		return vk.ImageAspectFlags(vk.ImageAspectDepthBit)
	case types.TextureAspectStencilOnly:
		return vk.ImageAspectFlags(vk.ImageAspectStencilBit)
	default:
		return vk.ImageAspectFlags(vk.ImageAspectColorBit)
	}
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

func loadOpToVk(op types.LoadOp) vk.AttachmentLoadOp {
	switch op {
	case types.LoadOpClear:
		return vk.AttachmentLoadOpClear
	case types.LoadOpLoad:
		return vk.AttachmentLoadOpLoad
	default:
		return vk.AttachmentLoadOpDontCare
	}
}

func storeOpToVk(op types.StoreOp) vk.AttachmentStoreOp {
	switch op {
	case types.StoreOpStore:
		return vk.AttachmentStoreOpStore
	default:
		return vk.AttachmentStoreOpDontCare
	}
}

// --- Vulkan function wrappers ---

func vkBeginCommandBuffer(cmds *vk.Commands, cmdBuffer vk.CommandBuffer, beginInfo *vk.CommandBufferBeginInfo) vk.Result {
	ret, _, _ := syscall.SyscallN(cmds.BeginCommandBuffer(),
		uintptr(cmdBuffer),
		uintptr(unsafe.Pointer(beginInfo)))
	return vk.Result(ret)
}

func vkEndCommandBuffer(cmds *vk.Commands, cmdBuffer vk.CommandBuffer) vk.Result {
	ret, _, _ := syscall.SyscallN(cmds.EndCommandBuffer(), uintptr(cmdBuffer))
	return vk.Result(ret)
}

func vkResetCommandPool(cmds *vk.Commands, device vk.Device, pool vk.CommandPool, flags vk.CommandPoolResetFlags) vk.Result {
	ret, _, _ := syscall.SyscallN(cmds.ResetCommandPool(),
		uintptr(device),
		uintptr(pool),
		uintptr(flags))
	return vk.Result(ret)
}

func vkCmdPipelineBarrier(cmds *vk.Commands, cmdBuffer vk.CommandBuffer,
	srcStageMask, dstStageMask vk.PipelineStageFlags,
	dependencyFlags vk.DependencyFlags,
	memoryBarrierCount uint32, pMemoryBarriers *vk.MemoryBarrier,
	bufferMemoryBarrierCount uint32, pBufferMemoryBarriers *vk.BufferMemoryBarrier,
	imageMemoryBarrierCount uint32, pImageMemoryBarriers *vk.ImageMemoryBarrier) {
	//nolint:errcheck // Vulkan void function
	syscall.SyscallN(cmds.CmdPipelineBarrier(),
		uintptr(cmdBuffer),
		uintptr(srcStageMask),
		uintptr(dstStageMask),
		uintptr(dependencyFlags),
		uintptr(memoryBarrierCount),
		uintptr(unsafe.Pointer(pMemoryBarriers)),
		uintptr(bufferMemoryBarrierCount),
		uintptr(unsafe.Pointer(pBufferMemoryBarriers)),
		uintptr(imageMemoryBarrierCount),
		uintptr(unsafe.Pointer(pImageMemoryBarriers)))
}

func vkCmdFillBuffer(cmds *vk.Commands, cmdBuffer vk.CommandBuffer, buffer vk.Buffer, offset, size vk.DeviceSize, data uint32) {
	//nolint:errcheck // Vulkan void function
	syscall.SyscallN(cmds.CmdFillBuffer(),
		uintptr(cmdBuffer),
		uintptr(buffer),
		uintptr(offset),
		uintptr(size),
		uintptr(data))
}

func vkCmdCopyBuffer(cmds *vk.Commands, cmdBuffer vk.CommandBuffer, src, dst vk.Buffer, regionCount uint32, pRegions *vk.BufferCopy) {
	//nolint:errcheck // Vulkan void function
	syscall.SyscallN(cmds.CmdCopyBuffer(),
		uintptr(cmdBuffer),
		uintptr(src),
		uintptr(dst),
		uintptr(regionCount),
		uintptr(unsafe.Pointer(pRegions)))
}

func vkCmdCopyBufferToImage(cmds *vk.Commands, cmdBuffer vk.CommandBuffer, src vk.Buffer, dst vk.Image, layout vk.ImageLayout, regionCount uint32, pRegions *vk.BufferImageCopy) {
	//nolint:errcheck // Vulkan void function
	syscall.SyscallN(cmds.CmdCopyBufferToImage(),
		uintptr(cmdBuffer),
		uintptr(src),
		uintptr(dst),
		uintptr(layout),
		uintptr(regionCount),
		uintptr(unsafe.Pointer(pRegions)))
}

func vkCmdCopyImageToBuffer(cmds *vk.Commands, cmdBuffer vk.CommandBuffer, src vk.Image, layout vk.ImageLayout, dst vk.Buffer, regionCount uint32, pRegions *vk.BufferImageCopy) {
	//nolint:errcheck // Vulkan void function
	syscall.SyscallN(cmds.CmdCopyImageToBuffer(),
		uintptr(cmdBuffer),
		uintptr(src),
		uintptr(layout),
		uintptr(dst),
		uintptr(regionCount),
		uintptr(unsafe.Pointer(pRegions)))
}

func vkCmdCopyImage(cmds *vk.Commands, cmdBuffer vk.CommandBuffer, src vk.Image, srcLayout vk.ImageLayout, dst vk.Image, dstLayout vk.ImageLayout, regionCount uint32, pRegions *vk.ImageCopy) {
	//nolint:errcheck // Vulkan void function
	syscall.SyscallN(cmds.CmdCopyImage(),
		uintptr(cmdBuffer),
		uintptr(src),
		uintptr(srcLayout),
		uintptr(dst),
		uintptr(dstLayout),
		uintptr(regionCount),
		uintptr(unsafe.Pointer(pRegions)))
}

func vkCmdBeginRendering(cmds *vk.Commands, cmdBuffer vk.CommandBuffer, renderingInfo *vk.RenderingInfo) {
	//nolint:errcheck // Vulkan void function
	syscall.SyscallN(cmds.CmdBeginRendering(),
		uintptr(cmdBuffer),
		uintptr(unsafe.Pointer(renderingInfo)))
}

func vkCmdEndRendering(cmds *vk.Commands, cmdBuffer vk.CommandBuffer) {
	//nolint:errcheck // Vulkan void function
	syscall.SyscallN(cmds.CmdEndRendering(), uintptr(cmdBuffer))
}

func vkCmdBindPipeline(cmds *vk.Commands, cmdBuffer vk.CommandBuffer, bindPoint vk.PipelineBindPoint, pipeline vk.Pipeline) {
	//nolint:errcheck // Vulkan void function
	syscall.SyscallN(cmds.CmdBindPipeline(),
		uintptr(cmdBuffer),
		uintptr(bindPoint),
		uintptr(pipeline))
}

func vkCmdBindDescriptorSets(cmds *vk.Commands, cmdBuffer vk.CommandBuffer, bindPoint vk.PipelineBindPoint, layout vk.PipelineLayout, firstSet uint32, setCount uint32, pSets *vk.DescriptorSet, dynamicOffsetCount uint32, pDynamicOffsets *uint32) {
	//nolint:errcheck // Vulkan void function
	syscall.SyscallN(cmds.CmdBindDescriptorSets(),
		uintptr(cmdBuffer),
		uintptr(bindPoint),
		uintptr(layout),
		uintptr(firstSet),
		uintptr(setCount),
		uintptr(unsafe.Pointer(pSets)),
		uintptr(dynamicOffsetCount),
		uintptr(unsafe.Pointer(pDynamicOffsets)))
}

func vkCmdBindVertexBuffers(cmds *vk.Commands, cmdBuffer vk.CommandBuffer, firstBinding, bindingCount uint32, pBuffers *vk.Buffer, pOffsets *vk.DeviceSize) {
	//nolint:errcheck // Vulkan void function
	syscall.SyscallN(cmds.CmdBindVertexBuffers(),
		uintptr(cmdBuffer),
		uintptr(firstBinding),
		uintptr(bindingCount),
		uintptr(unsafe.Pointer(pBuffers)),
		uintptr(unsafe.Pointer(pOffsets)))
}

func vkCmdBindIndexBuffer(cmds *vk.Commands, cmdBuffer vk.CommandBuffer, buffer vk.Buffer, offset vk.DeviceSize, indexType vk.IndexType) {
	//nolint:errcheck // Vulkan void function
	syscall.SyscallN(cmds.CmdBindIndexBuffer(),
		uintptr(cmdBuffer),
		uintptr(buffer),
		uintptr(offset),
		uintptr(indexType))
}

func vkCmdSetViewport(cmds *vk.Commands, cmdBuffer vk.CommandBuffer, firstViewport, viewportCount uint32, pViewports *vk.Viewport) {
	//nolint:errcheck // Vulkan void function
	syscall.SyscallN(cmds.CmdSetViewport(),
		uintptr(cmdBuffer),
		uintptr(firstViewport),
		uintptr(viewportCount),
		uintptr(unsafe.Pointer(pViewports)))
}

func vkCmdSetScissor(cmds *vk.Commands, cmdBuffer vk.CommandBuffer, firstScissor, scissorCount uint32, pScissors *vk.Rect2D) {
	//nolint:errcheck // Vulkan void function
	syscall.SyscallN(cmds.CmdSetScissor(),
		uintptr(cmdBuffer),
		uintptr(firstScissor),
		uintptr(scissorCount),
		uintptr(unsafe.Pointer(pScissors)))
}

func vkCmdSetBlendConstants(cmds *vk.Commands, cmdBuffer vk.CommandBuffer, blendConstants *[4]float32) {
	//nolint:errcheck // Vulkan void function
	syscall.SyscallN(cmds.CmdSetBlendConstants(),
		uintptr(cmdBuffer),
		uintptr(unsafe.Pointer(blendConstants)))
}

func vkCmdSetStencilReference(cmds *vk.Commands, cmdBuffer vk.CommandBuffer, faceMask vk.StencilFaceFlags, reference uint32) {
	//nolint:errcheck // Vulkan void function
	syscall.SyscallN(cmds.CmdSetStencilReference(),
		uintptr(cmdBuffer),
		uintptr(faceMask),
		uintptr(reference))
}

func vkCmdDraw(cmds *vk.Commands, cmdBuffer vk.CommandBuffer, vertexCount, instanceCount, firstVertex, firstInstance uint32) {
	//nolint:errcheck // Vulkan void function
	syscall.SyscallN(cmds.CmdDraw(),
		uintptr(cmdBuffer),
		uintptr(vertexCount),
		uintptr(instanceCount),
		uintptr(firstVertex),
		uintptr(firstInstance))
}

func vkCmdDrawIndexed(cmds *vk.Commands, cmdBuffer vk.CommandBuffer, indexCount, instanceCount, firstIndex uint32, vertexOffset int32, firstInstance uint32) {
	//nolint:errcheck // Vulkan void function
	syscall.SyscallN(cmds.CmdDrawIndexed(),
		uintptr(cmdBuffer),
		uintptr(indexCount),
		uintptr(instanceCount),
		uintptr(firstIndex),
		uintptr(vertexOffset),
		uintptr(firstInstance))
}

func vkCmdDrawIndirect(cmds *vk.Commands, cmdBuffer vk.CommandBuffer, buffer vk.Buffer, offset vk.DeviceSize, drawCount, stride uint32) {
	//nolint:errcheck // Vulkan void function
	syscall.SyscallN(cmds.CmdDrawIndirect(),
		uintptr(cmdBuffer),
		uintptr(buffer),
		uintptr(offset),
		uintptr(drawCount),
		uintptr(stride))
}

func vkCmdDrawIndexedIndirect(cmds *vk.Commands, cmdBuffer vk.CommandBuffer, buffer vk.Buffer, offset vk.DeviceSize, drawCount, stride uint32) {
	//nolint:errcheck // Vulkan void function
	syscall.SyscallN(cmds.CmdDrawIndexedIndirect(),
		uintptr(cmdBuffer),
		uintptr(buffer),
		uintptr(offset),
		uintptr(drawCount),
		uintptr(stride))
}

func vkCmdDispatch(cmds *vk.Commands, cmdBuffer vk.CommandBuffer, x, y, z uint32) {
	//nolint:errcheck // Vulkan void function
	syscall.SyscallN(cmds.CmdDispatch(),
		uintptr(cmdBuffer),
		uintptr(x),
		uintptr(y),
		uintptr(z))
}

func vkCmdDispatchIndirect(cmds *vk.Commands, cmdBuffer vk.CommandBuffer, buffer vk.Buffer, offset vk.DeviceSize) {
	//nolint:errcheck // Vulkan void function
	syscall.SyscallN(cmds.CmdDispatchIndirect(),
		uintptr(cmdBuffer),
		uintptr(buffer),
		uintptr(offset))
}
