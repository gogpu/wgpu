// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vulkan

import (
	"fmt"
	"time"

	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/vulkan/vk"
	"github.com/gogpu/wgpu/types"
)

// Queue implements hal.Queue for Vulkan.
type Queue struct {
	handle      vk.Queue
	device      *Device
	familyIndex uint32
}

// Submit submits command buffers to the GPU.
func (q *Queue) Submit(commandBuffers []hal.CommandBuffer, fence hal.Fence, fenceValue uint64) error {
	if len(commandBuffers) == 0 {
		return nil
	}

	// Convert command buffers to Vulkan handles
	vkCmdBuffers := make([]vk.CommandBuffer, len(commandBuffers))
	for i, cb := range commandBuffers {
		vkCB, ok := cb.(*CommandBuffer)
		if !ok {
			return fmt.Errorf("vulkan: command buffer is not a Vulkan command buffer")
		}
		vkCmdBuffers[i] = vkCB.handle
	}

	// Get wait/signal semaphores from surface if this is a present submit
	var waitSemaphore, signalSemaphore vk.Semaphore
	waitStage := vk.PipelineStageFlags(vk.PipelineStageColorAttachmentOutputBit)

	// Check if any command buffer was used with a swapchain texture
	// For now, we assume no synchronization needed without explicit fence
	submitInfo := vk.SubmitInfo{
		SType:              vk.StructureTypeSubmitInfo,
		CommandBufferCount: uint32(len(vkCmdBuffers)),
		PCommandBuffers:    &vkCmdBuffers[0],
	}

	// If we have semaphores from a swapchain, add them
	if waitSemaphore != 0 {
		submitInfo.WaitSemaphoreCount = 1
		submitInfo.PWaitSemaphores = &waitSemaphore
		submitInfo.PWaitDstStageMask = &waitStage
	}
	if signalSemaphore != 0 {
		submitInfo.SignalSemaphoreCount = 1
		submitInfo.PSignalSemaphores = &signalSemaphore
	}

	// Get fence handle if provided
	var vkFence vk.Fence
	if fence != nil {
		if vkF, ok := fence.(*Fence); ok {
			vkFence = vkF.handle
		}
	}

	result := vkQueueSubmit(q, 1, &submitInfo, vkFence)
	if result != vk.Success {
		return fmt.Errorf("vulkan: vkQueueSubmit failed: %d", result)
	}

	return nil
}

// SubmitForPresent submits command buffers with swapchain synchronization.
func (q *Queue) SubmitForPresent(commandBuffers []hal.CommandBuffer, swapchain *Swapchain) error {
	if len(commandBuffers) == 0 {
		return nil
	}

	// Convert command buffers to Vulkan handles
	vkCmdBuffers := make([]vk.CommandBuffer, len(commandBuffers))
	for i, cb := range commandBuffers {
		vkCB, ok := cb.(*CommandBuffer)
		if !ok {
			return fmt.Errorf("vulkan: command buffer is not a Vulkan command buffer")
		}
		vkCmdBuffers[i] = vkCB.handle
	}

	waitStage := vk.PipelineStageFlags(vk.PipelineStageColorAttachmentOutputBit)

	submitInfo := vk.SubmitInfo{
		SType:                vk.StructureTypeSubmitInfo,
		WaitSemaphoreCount:   1,
		PWaitSemaphores:      &swapchain.imageAvailable,
		PWaitDstStageMask:    &waitStage,
		CommandBufferCount:   uint32(len(vkCmdBuffers)),
		PCommandBuffers:      &vkCmdBuffers[0],
		SignalSemaphoreCount: 1,
		PSignalSemaphores:    &swapchain.renderFinished,
	}

	result := vkQueueSubmit(q, 1, &submitInfo, 0)
	if result != vk.Success {
		return fmt.Errorf("vulkan: vkQueueSubmit failed: %d", result)
	}

	return nil
}

// WriteBuffer writes data to a buffer immediately.
func (q *Queue) WriteBuffer(buffer hal.Buffer, offset uint64, data []byte) {
	vkBuffer, ok := buffer.(*Buffer)
	if !ok || vkBuffer.memory == nil {
		return
	}

	// Map, copy, unmap
	if vkBuffer.memory.MappedPtr != 0 {
		// Already mapped - direct copy using Vulkan mapped memory from vkMapMemory
		// Use copyToMappedMemory to avoid go vet false positive about unsafe.Pointer
		copyToMappedMemory(vkBuffer.memory.MappedPtr, offset, data)
	}
	// TODO: Implement staging buffer for non-host-visible memory
}

// WriteTexture writes data to a texture immediately.
func (q *Queue) WriteTexture(dst *hal.ImageCopyTexture, data []byte, layout *hal.ImageDataLayout, size *hal.Extent3D) {
	if dst == nil || dst.Texture == nil || len(data) == 0 || size == nil {
		return
	}

	vkTexture, ok := dst.Texture.(*Texture)
	if !ok || vkTexture == nil {
		return
	}

	// Create staging buffer
	stagingDesc := &hal.BufferDescriptor{
		Label: "staging-buffer-for-texture",
		Size:  uint64(len(data)),
		Usage: types.BufferUsageCopySrc | types.BufferUsageMapWrite,
	}

	stagingBuffer, err := q.device.CreateBuffer(stagingDesc)
	if err != nil {
		return
	}
	defer q.device.DestroyBuffer(stagingBuffer)

	// Copy data to staging buffer
	vkStaging, ok := stagingBuffer.(*Buffer)
	if !ok || vkStaging.memory == nil || vkStaging.memory.MappedPtr == 0 {
		return
	}
	copyToMappedMemory(vkStaging.memory.MappedPtr, 0, data)

	// Create one-shot command buffer
	cmdEncoder, err := q.device.CreateCommandEncoder(&hal.CommandEncoderDescriptor{
		Label: "texture-upload-encoder",
	})
	if err != nil {
		return
	}

	encoder, ok := cmdEncoder.(*CommandEncoder)
	if !ok {
		return
	}

	// Begin recording
	if err := encoder.BeginEncoding("texture-upload"); err != nil {
		return
	}

	// Transition texture to transfer destination layout
	encoder.TransitionTextures([]hal.TextureBarrier{
		{
			Texture: dst.Texture,
			Usage: hal.TextureUsageTransition{
				OldUsage: 0,
				NewUsage: types.TextureUsageCopyDst,
			},
		},
	})

	// Copy from staging buffer to texture
	bytesPerRow := layout.BytesPerRow
	if bytesPerRow == 0 {
		// Calculate based on format and width
		bytesPerRow = size.Width * 4 // Assume 4 bytes per pixel for RGBA
	}

	rowsPerImage := layout.RowsPerImage
	if rowsPerImage == 0 {
		rowsPerImage = size.Height
	}

	regions := []hal.BufferTextureCopy{
		{
			BufferLayout: hal.ImageDataLayout{
				Offset:       layout.Offset,
				BytesPerRow:  bytesPerRow,
				RowsPerImage: rowsPerImage,
			},
			TextureBase: hal.ImageCopyTexture{
				Texture:  dst.Texture,
				MipLevel: dst.MipLevel,
				Origin: hal.Origin3D{
					X: dst.Origin.X,
					Y: dst.Origin.Y,
					Z: dst.Origin.Z,
				},
				Aspect: dst.Aspect,
			},
			Size: hal.Extent3D{
				Width:              size.Width,
				Height:             size.Height,
				DepthOrArrayLayers: size.DepthOrArrayLayers,
			},
		},
	}

	encoder.CopyBufferToTexture(stagingBuffer, dst.Texture, regions)

	// Transition texture to shader read layout
	encoder.TransitionTextures([]hal.TextureBarrier{
		{
			Texture: dst.Texture,
			Usage: hal.TextureUsageTransition{
				OldUsage: types.TextureUsageCopyDst,
				NewUsage: types.TextureUsageTextureBinding,
			},
		},
	})

	// End recording and submit
	cmdBuffer, err := encoder.EndEncoding()
	if err != nil {
		return
	}

	// Submit and wait
	fence, err := q.device.CreateFence()
	if err != nil {
		return
	}
	defer q.device.DestroyFence(fence)

	if err := q.Submit([]hal.CommandBuffer{cmdBuffer}, fence, 0); err != nil {
		return
	}

	// Wait for completion (60 second timeout)
	_, _ = q.device.Wait(fence, 0, 60*time.Second)
}

// Present presents a surface texture to the screen.
func (q *Queue) Present(surface hal.Surface, texture hal.SurfaceTexture) error {
	vkSurface, ok := surface.(*Surface)
	if !ok {
		return fmt.Errorf("vulkan: surface is not a Vulkan surface")
	}

	if vkSurface.swapchain == nil {
		return fmt.Errorf("vulkan: surface not configured")
	}

	return vkSurface.swapchain.present(q)
}

// GetTimestampPeriod returns the timestamp period in nanoseconds.
func (q *Queue) GetTimestampPeriod() float32 {
	// TODO: Get from physical device properties
	return 1.0
}

// Vulkan function wrapper

func vkQueueSubmit(q *Queue, submitCount uint32, submits *vk.SubmitInfo, fence vk.Fence) vk.Result {
	return q.device.cmds.QueueSubmit(q.handle, submitCount, submits, fence)
}
