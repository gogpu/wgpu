// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vulkan

import (
	"fmt"
	"sync"
	"time"
	"unsafe"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/vulkan/vk"
)

// cmdBufferPool reuses slices of vk.CommandBuffer handles across Submit calls.
// Without pooling, every Submit allocates a new []vk.CommandBuffer on the heap
// (1-8 elements per frame at 60+ FPS = 60-480 allocations/second).
var cmdBufferPool = sync.Pool{
	New: func() any {
		s := make([]vk.CommandBuffer, 0, 8)
		return &s
	},
}

// Queue implements hal.Queue for Vulkan.
type Queue struct {
	handle          vk.Queue
	device          *Device
	familyIndex     uint32
	activeSwapchain *Swapchain // Set by AcquireTexture, used by Submit for synchronization
	acquireUsed     bool       // True if acquire semaphore was consumed by a submit
	mu              sync.Mutex // Protects Submit() and Present() from concurrent access

	// transferFence tracks the last queue submission for WriteBuffer/ReadBuffer
	// synchronization. Instead of vkQueueWaitIdle (which stalls the entire GPU
	// pipeline), we signal this fence after every Submit and wait on it only
	// when CPU needs to access buffer memory.
	transferFence       vk.Fence
	transferFenceActive bool // True if transferFence was signaled and not yet waited on
}

// Submit submits command buffers to the GPU.
func (q *Queue) Submit(commandBuffers []hal.CommandBuffer, fence hal.Fence, fenceValue uint64) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(commandBuffers) == 0 {
		return nil
	}

	// Convert command buffers to Vulkan handles.
	// Use sync.Pool to avoid per-frame heap allocation (VK-PERF-001).
	pooledSlice := cmdBufferPool.Get().(*[]vk.CommandBuffer)
	vkCmdBuffers := (*pooledSlice)[:0]
	for _, cb := range commandBuffers {
		vkCB, ok := cb.(*CommandBuffer)
		if !ok {
			*pooledSlice = vkCmdBuffers
			cmdBufferPool.Put(pooledSlice)
			return fmt.Errorf("vulkan: command buffer is not a Vulkan command buffer")
		}
		vkCmdBuffers = append(vkCmdBuffers, vkCB.handle)
	}
	defer func() {
		*pooledSlice = vkCmdBuffers[:0]
		cmdBufferPool.Put(pooledSlice)
	}()

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

	// Timeline path (VK-IMPL-001): Attach timeline semaphore signal to the real submit.
	// This replaces the empty signalFrameFence submit AND the transfer fence with
	// a single inline signal on the actual command buffer submit.
	var timelineSubmitInfo vk.TimelineSemaphoreSubmitInfo
	if q.device.timelineFence != nil && q.device.timelineFence.isTimeline {
		signalValue := q.device.timelineFence.nextSignalValue()
		timelineSubmitInfo = vk.TimelineSemaphoreSubmitInfo{
			SType:                     vk.StructureTypeTimelineSemaphoreSubmitInfo,
			SignalSemaphoreValueCount: 1,
			PSignalSemaphoreValues:    &signalValue,
		}

		// Chain timeline submit info into the submit info.
		// If there are already signal semaphores (e.g., present semaphore),
		// we need to add our timeline semaphore to the signal list.
		if submitInfo.SignalSemaphoreCount > 0 {
			// Already have a signal semaphore (present path).
			// We need to signal BOTH the present semaphore AND the timeline semaphore.
			signalSems := [2]vk.Semaphore{
				*submitInfo.PSignalSemaphores,            // original (e.g., present semaphore)
				q.device.timelineFence.timelineSemaphore, // timeline
			}
			signalValues := [2]uint64{
				0,           // binary semaphore: value ignored
				signalValue, // timeline semaphore: value to signal
			}
			submitInfo.SignalSemaphoreCount = 2
			submitInfo.PSignalSemaphores = &signalSems[0]
			timelineSubmitInfo.SignalSemaphoreValueCount = 2
			timelineSubmitInfo.PSignalSemaphoreValues = &signalValues[0]
		} else {
			// No existing signal semaphores — just signal the timeline.
			submitInfo.SignalSemaphoreCount = 1
			submitInfo.PSignalSemaphores = &q.device.timelineFence.timelineSemaphore
		}
		submitInfo.PNext = (*uintptr)(unsafe.Pointer(&timelineSubmitInfo))

		// Also chain timeline wait values if we have wait semaphores.
		if submitInfo.WaitSemaphoreCount > 0 {
			waitValue := uint64(0) // Binary semaphore wait: value ignored.
			timelineSubmitInfo.WaitSemaphoreValueCount = 1
			timelineSubmitInfo.PWaitSemaphoreValues = &waitValue
		}

		result := vkQueueSubmit(q, 1, &submitInfo, submitFence)
		if result != vk.Success {
			return fmt.Errorf("vulkan: vkQueueSubmit failed: %d", result)
		}
		return nil
	}

	// Binary path: If no user fence is provided, use our transfer fence directly on this submit.
	// This avoids an extra vkQueueSubmit call for the common rendering path.
	if submitFence == 0 && q.transferFence != 0 {
		// Wait for previous GPU work before resetting the fence.
		// Without this, vkResetFences fails with "fence is in use" validation error
		// because the GPU may still be processing the previous frame's commands.
		if q.transferFenceActive {
			q.waitForTransferFence()
		}
		submitFence = q.transferFence
	}

	result := vkQueueSubmit(q, 1, &submitInfo, submitFence)
	if result != vk.Success {
		return fmt.Errorf("vulkan: vkQueueSubmit failed: %d", result)
	}

	// Track whether transferFence was signaled on this submit.
	if submitFence == q.transferFence {
		q.transferFenceActive = true
	} else if q.transferFence != 0 {
		// User provided their own fence. Signal transferFence with an empty submit
		// so WriteBuffer/ReadBuffer can wait on it for proper synchronization.
		q.signalTransferFence()
	}

	return nil
}

// SubmitForPresent submits command buffers with swapchain synchronization.
func (q *Queue) SubmitForPresent(commandBuffers []hal.CommandBuffer, swapchain *Swapchain) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(commandBuffers) == 0 {
		return nil
	}

	// Convert command buffers to Vulkan handles.
	// Use sync.Pool to avoid per-frame heap allocation (VK-PERF-001).
	pooledSlice := cmdBufferPool.Get().(*[]vk.CommandBuffer)
	vkCmdBuffers := (*pooledSlice)[:0]
	for _, cb := range commandBuffers {
		vkCB, ok := cb.(*CommandBuffer)
		if !ok {
			*pooledSlice = vkCmdBuffers
			cmdBufferPool.Put(pooledSlice)
			return fmt.Errorf("vulkan: command buffer is not a Vulkan command buffer")
		}
		vkCmdBuffers = append(vkCmdBuffers, vkCB.handle)
	}
	defer func() {
		*pooledSlice = vkCmdBuffers[:0]
		cmdBufferPool.Put(pooledSlice)
	}()

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

	// Timeline path (VK-IMPL-001): Also signal the timeline semaphore on this submit.
	var timelineSubmitInfo vk.TimelineSemaphoreSubmitInfo
	if q.device.timelineFence != nil && q.device.timelineFence.isTimeline {
		signalValue := q.device.timelineFence.nextSignalValue()
		signalSems := [2]vk.Semaphore{presentSem, q.device.timelineFence.timelineSemaphore}
		signalValues := [2]uint64{0, signalValue} // 0 for binary, value for timeline
		waitValue := uint64(0)                    // Binary acquire semaphore: value ignored

		submitInfo.SignalSemaphoreCount = 2
		submitInfo.PSignalSemaphores = &signalSems[0]

		timelineSubmitInfo = vk.TimelineSemaphoreSubmitInfo{
			SType:                     vk.StructureTypeTimelineSemaphoreSubmitInfo,
			WaitSemaphoreValueCount:   1,
			PWaitSemaphoreValues:      &waitValue,
			SignalSemaphoreValueCount: 2,
			PSignalSemaphoreValues:    &signalValues[0],
		}
		submitInfo.PNext = (*uintptr)(unsafe.Pointer(&timelineSubmitInfo))
	}

	result := vkQueueSubmit(q, 1, &submitInfo, vk.Fence(0))
	if result != vk.Success {
		return fmt.Errorf("vulkan: vkQueueSubmit failed: %d", result)
	}

	return nil
}

// WriteBuffer writes data to a buffer immediately.
// Uses fence-based synchronization instead of vkQueueWaitIdle to avoid
// stalling the entire GPU pipeline. Only waits for the last queue submission
// to complete, which per Khronos benchmarks improves frame times by ~22%.
//
// Timeline path (VK-IMPL-001): Uses vkWaitSemaphores on the latest signal value.
// Binary path: Uses the transfer fence.
func (q *Queue) WriteBuffer(buffer hal.Buffer, offset uint64, data []byte) {
	vkBuffer, ok := buffer.(*Buffer)
	if !ok || vkBuffer.memory == nil {
		return
	}

	// Wait for the last queue submission to complete before CPU writes.
	// This prevents race conditions where GPU reads stale/partial data.
	q.waitForGPU()

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
// Uses fence-based synchronization instead of vkQueueWaitIdle.
//
// Timeline path (VK-IMPL-001): Uses vkWaitSemaphores on the latest signal value.
// Binary path: Uses the transfer fence.
func (q *Queue) ReadBuffer(buffer hal.Buffer, offset uint64, data []byte) error {
	vkBuffer, ok := buffer.(*Buffer)
	if !ok || vkBuffer.memory == nil {
		return fmt.Errorf("vulkan: invalid buffer for ReadBuffer")
	}

	// Wait for the last queue submission to complete before CPU reads.
	q.waitForGPU()

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

// waitForGPU waits for the latest GPU submission to complete.
// Timeline path (VK-IMPL-001): Uses vkWaitSemaphores on the latest signal value.
// Binary path: Uses the transfer fence.
func (q *Queue) waitForGPU() {
	if q.device.timelineFence != nil && q.device.timelineFence.isTimeline {
		timeoutNs := uint64(60 * time.Second)
		_ = q.device.timelineFence.waitForLatest(q.device.cmds, q.device.handle, timeoutNs)
		return
	}
	q.waitForTransferFence()
}

// signalTransferFence signals the transfer fence with an empty queue submit.
// Called when the user provides their own fence to Submit() -- the user's fence
// is used for the main submit, so we need a separate empty submit to signal
// the transfer fence. vkQueueSubmit with 0 command buffers is valid per Vulkan
// spec and just inserts a fence signal after all previously submitted work.
func (q *Queue) signalTransferFence() {
	if q.transferFenceActive {
		q.waitForTransferFence()
	}
	emptySubmit := vk.SubmitInfo{
		SType: vk.StructureTypeSubmitInfo,
	}
	result := vkQueueSubmit(q, 1, &emptySubmit, q.transferFence)
	if result == vk.Success {
		q.transferFenceActive = true
	} else {
		hal.Logger().Warn("vulkan: failed to signal transfer fence",
			"result", result,
		)
		q.transferFenceActive = false
	}
}

// waitForTransferFence waits for the transfer fence to be signaled, indicating
// the last queue submission has completed. This is used by WriteBuffer and
// ReadBuffer instead of vkQueueWaitIdle to avoid stalling the entire GPU pipeline.
// If no submission has been made yet, this is a no-op.
func (q *Queue) waitForTransferFence() {
	if !q.transferFenceActive || q.transferFence == 0 {
		return
	}

	// Wait with a generous timeout (60 seconds) to match WriteTexture behavior.
	timeoutNs := uint64(60 * time.Second)
	result := vkWaitForFences(q.device.cmds, q.device.handle, 1, &q.transferFence, vk.Bool32(vk.True), timeoutNs)
	if result != vk.Success && result != vk.Timeout {
		hal.Logger().Warn("vulkan: waitForTransferFence failed",
			"result", result,
		)
	}

	// Reset for next use
	_ = vkResetFences(q.device.cmds, q.device.handle, 1, &q.transferFence)
	q.transferFenceActive = false
}

// destroyTransferFence releases the transfer fence. Called during queue cleanup.
func (q *Queue) destroyTransferFence() {
	if q.transferFence != 0 {
		vkDestroyFence(q.device.cmds, q.device.handle, q.transferFence, nil)
		q.transferFence = 0
		q.transferFenceActive = false
	}
}

// Present presents a surface texture to the screen.
// After presenting, advances the frame index and recycles the old command pool
// from the reused slot (VK-OPT-002/003).
func (q *Queue) Present(surface hal.Surface, texture hal.SurfaceTexture) error {
	q.mu.Lock()
	defer q.mu.Unlock()

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

	if err != nil {
		return err
	}

	// Signal the frame fence once per frame (after all submits, before advance).
	// This marks the current slot as in-flight so advanceFrame knows when to wait.
	if err := q.device.signalFrameFence(); err != nil {
		return err
	}

	// Advance the frame index and recycle the old command pool.
	// This waits only for the specific old frame in the reused slot --
	// not all GPU work -- enabling CPU/GPU overlap (VK-OPT-003).
	return q.device.advanceFrame()
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
