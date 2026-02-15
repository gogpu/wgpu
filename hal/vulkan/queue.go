// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vulkan

import (
	"fmt"
	"time"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/vulkan/vk"
)

// Queue implements hal.Queue for Vulkan.
type Queue struct {
	handle          vk.Queue
	device          *Device
	familyIndex     uint32
	activeSwapchain *Swapchain // Set by AcquireTexture, used by Submit for synchronization
	acquireUsed     bool       // True if acquire semaphore was consumed by a submit
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

	submitInfo := vk.SubmitInfo{
		SType:              vk.StructureTypeSubmitInfo,
		CommandBufferCount: uint32(len(vkCmdBuffers)),
		PCommandBuffers:    &vkCmdBuffers[0],
	}

	// If we have an active swapchain, use its semaphores for GPU-side synchronization.
	// CRITICAL: Semaphores can only be used ONCE per frame.
	// - Wait on currentAcquireSem: ONLY on first submit (signaled by acquire)
	// - Signal presentSemaphores: ONLY on first submit (waited on by present)
	// Subsequent submits in the same frame run without semaphore synchronization.
	waitStage := vk.PipelineStageFlags(vk.PipelineStageColorAttachmentOutputBit)
	var submitFence vk.Fence
	if q.activeSwapchain != nil && !q.acquireUsed {
		acquireSem := q.activeSwapchain.currentAcquireSem
		presentSem := q.activeSwapchain.presentSemaphores[q.activeSwapchain.currentImage]
		submitInfo.WaitSemaphoreCount = 1
		submitInfo.PWaitSemaphores = &acquireSem
		submitInfo.PWaitDstStageMask = &waitStage
		submitInfo.SignalSemaphoreCount = 1
		submitInfo.PSignalSemaphores = &presentSem
		q.acquireUsed = true // Mark as used for this frame
	}

	// Use user-provided fence if available
	if fence != nil {
		if vkF, ok := fence.(*Fence); ok {
			submitFence = vkF.handle
		}
	}

	result := vkQueueSubmit(q, 1, &submitInfo, submitFence)
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

	// Use the rotating acquire semaphore and per-image present semaphore (wgpu-style).
	acquireSem := swapchain.currentAcquireSem
	presentSem := swapchain.presentSemaphores[swapchain.currentImage]

	submitInfo := vk.SubmitInfo{
		SType:                vk.StructureTypeSubmitInfo,
		WaitSemaphoreCount:   1,
		PWaitSemaphores:      &acquireSem,
		PWaitDstStageMask:    &waitStage,
		CommandBufferCount:   uint32(len(vkCmdBuffers)),
		PCommandBuffers:      &vkCmdBuffers[0],
		SignalSemaphoreCount: 1,
		PSignalSemaphores:    &presentSem,
	}

	// No fence needed - GPU synchronization is done via semaphores
	result := vkQueueSubmit(q, 1, &submitInfo, vk.Fence(0))
	if result != vk.Success {
		return fmt.Errorf("vulkan: vkQueueSubmit failed: %d", result)
	}

	return nil
}

// WriteBuffer writes data to a buffer immediately.
// Note: This waits for GPU to finish to avoid race conditions.
// A more optimal solution would use a staging belt (like wgpu-rs).
func (q *Queue) WriteBuffer(buffer hal.Buffer, offset uint64, data []byte) {
	vkBuffer, ok := buffer.(*Buffer)
	if !ok || vkBuffer.memory == nil {
		return
	}

	// Wait for GPU to finish using the buffer before writing
	// This prevents race conditions where GPU reads stale/partial data
	_ = q.device.cmds.QueueWaitIdle(q.handle)

	// Map, copy, unmap
	if vkBuffer.memory.MappedPtr != 0 {
		// Already mapped - direct copy using Vulkan mapped memory from vkMapMemory
		// Use copyToMappedMemory to avoid go vet false positive about unsafe.Pointer
		copyToMappedMemory(vkBuffer.memory.MappedPtr, offset, data)

		// Flush mapped memory to ensure GPU sees CPU writes.
		// Required for non-HOST_COHERENT memory; harmless on coherent memory.
		memRange := vk.MappedMemoryRange{
			SType:  vk.StructureTypeMappedMemoryRange,
			Memory: vkBuffer.memory.Memory,
			Offset: vk.DeviceSize(vkBuffer.memory.Offset),
			Size:   vk.DeviceSize(vk.WholeSize),
		}
		_ = q.device.cmds.FlushMappedMemoryRanges(q.device.handle, 1, &memRange)
	}
	// Note(v0.6.0): Staging buffer needed for device-local memory writes.
}

// ReadBuffer reads data from a GPU buffer.
// The buffer must have host-visible memory (created with MapRead usage).
func (q *Queue) ReadBuffer(buffer hal.Buffer, offset uint64, data []byte) error {
	vkBuffer, ok := buffer.(*Buffer)
	if !ok || vkBuffer.memory == nil {
		return fmt.Errorf("vulkan: invalid buffer for ReadBuffer")
	}

	// Wait for GPU to finish using the buffer
	_ = q.device.cmds.QueueWaitIdle(q.handle)

	if vkBuffer.memory.MappedPtr != 0 {
		// Invalidate CPU cache so we see the latest GPU writes.
		// Required for non-HOST_COHERENT memory; harmless on coherent memory.
		memRange := vk.MappedMemoryRange{
			SType:  vk.StructureTypeMappedMemoryRange,
			Memory: vkBuffer.memory.Memory,
			Offset: vk.DeviceSize(vkBuffer.memory.Offset),
			Size:   vk.DeviceSize(vk.WholeSize),
		}
		_ = q.device.cmds.InvalidateMappedMemoryRanges(q.device.handle, 1, &memRange)

		copyFromMappedMemory(data, vkBuffer.memory.MappedPtr, offset)
		return nil
	}
	return fmt.Errorf("vulkan: buffer is not mapped, cannot read")
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
		Usage: gputypes.BufferUsageCopySrc | gputypes.BufferUsageMapWrite,
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
				NewUsage: gputypes.TextureUsageCopyDst,
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
				OldUsage: gputypes.TextureUsageCopyDst,
				NewUsage: gputypes.TextureUsageTextureBinding,
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

	// Free command buffer back to pool after GPU finishes
	q.device.FreeCommandBuffer(cmdBuffer)

	hal.Logger().Debug("vulkan: WriteTexture completed",
		"width", size.Width,
		"height", size.Height,
		"dataSize", len(data),
	)
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

	err := vkSurface.swapchain.present(q)

	// Clear active swapchain after present
	q.activeSwapchain = nil

	return err
}

// GetTimestampPeriod returns the timestamp period in nanoseconds.
func (q *Queue) GetTimestampPeriod() float32 {
	// Note: Should query VkPhysicalDeviceLimits.timestampPeriod.
	return 1.0
}

// Vulkan function wrapper

func vkQueueSubmit(q *Queue, submitCount uint32, submits *vk.SubmitInfo, fence vk.Fence) vk.Result {
	return q.device.cmds.QueueSubmit(q.handle, submitCount, submits, fence)
}
