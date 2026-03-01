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
	consumedAcquire := false // tracks whether THIS submit consumes the acquire semaphore
	if q.activeSwapchain != nil && !q.acquireUsed {
		acquireSem := q.activeSwapchain.currentAcquireSem
		presentSem := q.activeSwapchain.presentSemaphores[q.activeSwapchain.currentImage]
		submitInfo.WaitSemaphoreCount = 1
		submitInfo.PWaitSemaphores = &acquireSem
		submitInfo.PWaitDstStageMask = &waitStage
		submitInfo.SignalSemaphoreCount = 1
		submitInfo.PSignalSemaphores = &presentSem
		q.acquireUsed = true
		consumedAcquire = true
	}

	// Use user-provided fence if available
	if fence != nil {
		if vkF, ok := fence.(*Fence); ok {
			submitFence = vkF.handle
		}
	}

	// Timeline path (VK-IMPL-001): Attach timeline semaphore signal to the real submit.
	// This enables waitForGPU to track the latest submission.
	var timelineSubmitInfo vk.TimelineSemaphoreSubmitInfo
	if q.device.timelineFence.isTimeline { //nolint:nestif // timeline PNext chaining requires conditional semaphore setup
		signalValue := q.device.timelineFence.nextSignalValue()

		// VK-IMPL-004: Record which submission consumed this acquire semaphore.
		// Pre-acquire wait in acquireNextImage() uses this to ensure the GPU
		// has finished before reusing the semaphore.
		if consumedAcquire {
			q.activeSwapchain.acquireFenceValues[q.activeSwapchain.currentAcquireIdx] = signalValue
		}
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

	// Binary path (VK-IMPL-003): Get a fence from the pool to track this submission.
	// The fencePool replaces the old transferFence with per-submission tracking,
	// enabling waitForGPU to wait for specific or latest submissions.
	pool := q.device.timelineFence.pool
	signalValue := q.device.timelineFence.nextSignalValue()

	// VK-IMPL-004: Record fence value for pre-acquire wait (binary path).
	if consumedAcquire {
		q.activeSwapchain.acquireFenceValues[q.activeSwapchain.currentAcquireIdx] = signalValue
	}
	poolFence, err := pool.signal(q.device.cmds, q.device.handle, signalValue)
	if err != nil {
		return fmt.Errorf("vulkan: Submit fencePool signal: %w", err)
	}

	// If user provided their own fence, we submit with the user fence and
	// signal the pool fence via a separate empty submit for tracking.
	if submitFence != 0 {
		result := vkQueueSubmit(q, 1, &submitInfo, submitFence)
		if result != vk.Success {
			return fmt.Errorf("vulkan: vkQueueSubmit failed: %d", result)
		}
		// Empty submit to signal the pool fence for waitForGPU tracking.
		emptySubmit := vk.SubmitInfo{SType: vk.StructureTypeSubmitInfo}
		result = vkQueueSubmit(q, 1, &emptySubmit, poolFence)
		if result != vk.Success {
			return fmt.Errorf("vulkan: vkQueueSubmit (pool fence) failed: %d", result)
		}
		return nil
	}

	// Common case: no user fence, submit with pool fence directly.
	result := vkQueueSubmit(q, 1, &submitInfo, poolFence)
	if result != vk.Success {
		return fmt.Errorf("vulkan: vkQueueSubmit failed: %d", result)
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
	if q.device.timelineFence.isTimeline {
		signalValue := q.device.timelineFence.nextSignalValue()

		// VK-IMPL-004: Record which submission consumed this acquire semaphore.
		swapchain.acquireFenceValues[swapchain.currentAcquireIdx] = signalValue

		signalSems := [2]vk.Semaphore{presentSem, q.device.timelineFence.timelineSemaphore}
		signalValues := [2]uint64{0, signalValue} // 0 for binary, value for timeline
		waitValue := uint64(0)                    // Binary acquire semaphore: value ignored

		submitInfo.SignalSemaphoreCount = 2
		submitInfo.PSignalSemaphores = &signalSems[0]

		timelineSubmitInfo := vk.TimelineSemaphoreSubmitInfo{
			SType:                     vk.StructureTypeTimelineSemaphoreSubmitInfo,
			WaitSemaphoreValueCount:   1,
			PWaitSemaphoreValues:      &waitValue,
			SignalSemaphoreValueCount: 2,
			PSignalSemaphoreValues:    &signalValues[0],
		}
		submitInfo.PNext = (*uintptr)(unsafe.Pointer(&timelineSubmitInfo))

		result := vkQueueSubmit(q, 1, &submitInfo, vk.Fence(0))
		if result != vk.Success {
			return fmt.Errorf("vulkan: vkQueueSubmit failed: %d", result)
		}
		return nil
	}

	// Binary path (VK-IMPL-003): Track submission with fence pool for waitForGPU
	// and VK-IMPL-004 pre-acquire semaphore wait.
	pool := q.device.timelineFence.pool
	signalValue := q.device.timelineFence.nextSignalValue()

	// VK-IMPL-004: Record which submission consumed this acquire semaphore.
	swapchain.acquireFenceValues[swapchain.currentAcquireIdx] = signalValue

	poolFence, err := pool.signal(q.device.cmds, q.device.handle, signalValue)
	if err != nil {
		return fmt.Errorf("vulkan: SubmitForPresent fencePool signal: %w", err)
	}

	result := vkQueueSubmit(q, 1, &submitInfo, poolFence)
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
// Both paths use the unified deviceFence: timeline semaphore (VK-IMPL-001)
// or binary fence pool (VK-IMPL-003).
func (q *Queue) WriteBuffer(buffer hal.Buffer, offset uint64, data []byte) error {
	vkBuffer, ok := buffer.(*Buffer)
	if !ok || vkBuffer.memory == nil {
		return fmt.Errorf("vulkan: WriteBuffer: invalid buffer")
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
		result := q.device.cmds.FlushMappedMemoryRanges(q.device.handle, 1, &memRange)
		if result != vk.Success {
			return fmt.Errorf("vulkan: WriteBuffer: FlushMappedMemoryRanges failed: %d", result)
		}
		return nil
	}
	// Note(v0.6.0): Staging buffer needed for device-local memory writes.
	return fmt.Errorf("vulkan: WriteBuffer: buffer is not mapped")
}

// ReadBuffer reads data from a GPU buffer.
// The buffer must have host-visible memory (created with MapRead usage).
// Uses fence-based synchronization instead of vkQueueWaitIdle.
//
// Both paths use the unified deviceFence: timeline semaphore (VK-IMPL-001)
// or binary fence pool (VK-IMPL-003).
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
// Returns an error if any step fails (VK-003: no more silent error swallowing).
func (q *Queue) WriteTexture(dst *hal.ImageCopyTexture, data []byte, layout *hal.ImageDataLayout, size *hal.Extent3D) error {
	if dst == nil || dst.Texture == nil || len(data) == 0 || size == nil {
		return fmt.Errorf("vulkan: WriteTexture: invalid arguments")
	}

	vkTexture, ok := dst.Texture.(*Texture)
	if !ok || vkTexture == nil {
		return fmt.Errorf("vulkan: WriteTexture: invalid texture type")
	}

	// Create staging buffer
	stagingDesc := &hal.BufferDescriptor{
		Label: "staging-buffer-for-texture",
		Size:  uint64(len(data)),
		Usage: gputypes.BufferUsageCopySrc | gputypes.BufferUsageMapWrite,
	}

	stagingBuffer, err := q.device.CreateBuffer(stagingDesc)
	if err != nil {
		return fmt.Errorf("vulkan: WriteTexture: CreateBuffer failed: %w", err)
	}
	defer q.device.DestroyBuffer(stagingBuffer)

	// Copy data to staging buffer
	vkStaging, ok := stagingBuffer.(*Buffer)
	if !ok || vkStaging.memory == nil || vkStaging.memory.MappedPtr == 0 {
		return fmt.Errorf("vulkan: WriteTexture: staging buffer not mapped")
	}
	copyToMappedMemory(vkStaging.memory.MappedPtr, 0, data)

	// Create one-shot command buffer
	cmdEncoder, err := q.device.CreateCommandEncoder(&hal.CommandEncoderDescriptor{
		Label: "texture-upload-encoder",
	})
	if err != nil {
		return fmt.Errorf("vulkan: WriteTexture: CreateCommandEncoder failed: %w", err)
	}

	encoder, ok := cmdEncoder.(*CommandEncoder)
	if !ok {
		return fmt.Errorf("vulkan: WriteTexture: unexpected encoder type")
	}

	// Begin recording
	if err := encoder.BeginEncoding("texture-upload"); err != nil {
		return fmt.Errorf("vulkan: WriteTexture: BeginEncoding failed: %w", err)
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
		return fmt.Errorf("vulkan: WriteTexture: EndEncoding failed: %w", err)
	}

	// Submit and wait
	fence, err := q.device.CreateFence()
	if err != nil {
		return fmt.Errorf("vulkan: WriteTexture: CreateFence failed: %w", err)
	}
	defer q.device.DestroyFence(fence)

	// VK-004: Staging uploads must NOT consume swapchain semaphores.
	// When WriteTexture is called between BeginFrame/EndFrame (e.g., in onDraw),
	// the activeSwapchain acquire semaphore must be preserved for the render pass
	// Submit, not consumed by this staging upload. Temporarily clear activeSwapchain
	// so the internal Submit runs without render-pass synchronization.
	q.mu.Lock()
	savedSwapchain := q.activeSwapchain
	savedAcquireUsed := q.acquireUsed
	q.activeSwapchain = nil
	q.mu.Unlock()
	defer func() {
		q.mu.Lock()
		q.activeSwapchain = savedSwapchain
		q.acquireUsed = savedAcquireUsed
		q.mu.Unlock()
	}()

	if err := q.Submit([]hal.CommandBuffer{cmdBuffer}, fence, 0); err != nil {
		return fmt.Errorf("vulkan: WriteTexture: Submit failed: %w", err)
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

	return nil
}

// waitForGPU waits for the latest GPU submission to complete.
// Both paths use the unified deviceFence: timeline semaphore (VK-IMPL-001)
// or binary fence pool (VK-IMPL-003).
func (q *Queue) waitForGPU() {
	timeoutNs := uint64(60 * time.Second)
	_ = q.device.timelineFence.waitForLatest(q.device.cmds, q.device.handle, timeoutNs)
}

// Present presents a surface texture to the screen.
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
	q.activeSwapchain = nil
	return err
}

// GetTimestampPeriod returns the timestamp period in nanoseconds.
func (q *Queue) GetTimestampPeriod() float32 {
	// Note: Should query VkPhysicalDeviceLimits.timestampPeriod.
	return 1.0
}

// Vulkan function wrapper

//nolint:unparam // Vulkan API wrapper — signature mirrors vkQueueSubmit spec
func vkQueueSubmit(q *Queue, submitCount uint32, submits *vk.SubmitInfo, fence vk.Fence) vk.Result {
	return q.device.cmds.QueueSubmit(q.handle, submitCount, submits, fence)
}
