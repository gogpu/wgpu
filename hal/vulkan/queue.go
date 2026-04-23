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

// relaySemaphores enforces GPU-side ordering between consecutive vkQueueSubmit calls.
//
// The wgpu_hal Queue trait (lib.rs:1059-1068) promises that if two calls to Submit
// are ordered, the first submission finishes on the GPU before the second begins.
// Vulkan only guarantees that submissions START in order, not that they finish in order.
// Relay semaphores close this gap by chaining a wait-then-signal dependency between
// every pair of consecutive submissions.
//
// We alternate between two binary semaphores instead of reusing one. This works around
// Mesa ANV driver bug #5508 (https://gitlab.freedesktop.org/mesa/mesa/-/issues/5508)
// where a single binary semaphore waited and signaled in consecutive submissions hangs.
// The bug is fixed in Mesa, but the workaround should be retained until at least Oct 2026.
//
// Reference: Rust wgpu-hal vulkan/mod.rs:526-598.
type relaySemaphores struct {
	// wait is the semaphore the next submission should wait on before beginning
	// execution on the GPU. Zero for the first submission (no dependency yet).
	wait vk.Semaphore

	// signal is the semaphore the next submission should signal when it finishes
	// execution on the GPU. Always valid (non-zero).
	signal vk.Semaphore
}

// newRelaySemaphores creates the initial relay state with one binary semaphore.
// The first submission will signal this semaphore without waiting on anything.
func newRelaySemaphores(cmds *vk.Commands, device vk.Device) (*relaySemaphores, error) {
	createInfo := vk.SemaphoreCreateInfo{
		SType: vk.StructureTypeSemaphoreCreateInfo,
	}
	var sem vk.Semaphore
	result := cmds.CreateSemaphore(device, &createInfo, nil, &sem)
	if result != vk.Success {
		return nil, fmt.Errorf("vulkan: vkCreateSemaphore (relay 1) failed: %d", result)
	}
	return &relaySemaphores{
		wait:   0, // first submission has no predecessor to wait on
		signal: sem,
	}, nil
}

// advance returns the (wait, signal) semaphores for the current submission and
// prepares the state for the next one.
//
// State machine:
//
//	Submit 1: returns (0, sem1) — no wait, signal sem1.    State becomes (sem1, sem2).
//	Submit 2: returns (sem1, sem2) — wait sem1, signal sem2. State becomes (sem2, sem1) [swap].
//	Submit 3: returns (sem2, sem1) — wait sem2, signal sem1. State becomes (sem1, sem2) [swap].
//	...alternating indefinitely.
//
// The second semaphore is created on demand (during the transition from first to second
// submission) to avoid allocating a semaphore that might never be needed.
func (r *relaySemaphores) advance(cmds *vk.Commands, device vk.Device) (wait, signal vk.Semaphore, err error) {
	// Capture current state for the caller.
	wait = r.wait
	signal = r.signal

	if r.wait == 0 {
		// First submission just happened. The second submission should wait on
		// what we just signaled, and signal a new semaphore.
		r.wait = r.signal
		createInfo := vk.SemaphoreCreateInfo{
			SType: vk.StructureTypeSemaphoreCreateInfo,
		}
		var sem2 vk.Semaphore
		result := cmds.CreateSemaphore(device, &createInfo, nil, &sem2)
		if result != vk.Success {
			return 0, 0, fmt.Errorf("vulkan: vkCreateSemaphore (relay 2) failed: %d", result)
		}
		r.signal = sem2
	} else {
		// Subsequent submissions: swap wait and signal so the next submission
		// waits on what this one signals, and signals the one this one waited on.
		r.wait, r.signal = r.signal, r.wait
	}

	return wait, signal, nil
}

// destroy releases both relay semaphores.
func (r *relaySemaphores) destroy(cmds *vk.Commands, device vk.Device) {
	if r.wait != 0 {
		cmds.DestroySemaphore(device, r.wait, nil)
		r.wait = 0
	}
	if r.signal != 0 {
		cmds.DestroySemaphore(device, r.signal, nil)
		r.signal = 0
	}
}

// Queue implements hal.Queue for Vulkan.
type Queue struct {
	handle          vk.Queue
	device          *Device
	familyIndex     uint32
	activeSwapchain *Swapchain // Set by AcquireTexture, used by Submit for synchronization
	acquireUsed     bool       // True if acquire semaphore was consumed by a submit
	relay           *relaySemaphores
	mu              sync.Mutex // Protects Submit() and Present() from concurrent access
}

// Submit submits command buffers to the GPU.
// Returns a monotonically increasing submission index for tracking completion.
// The HAL internally manages fence/timeline semaphore synchronization.
//
// VK-SYNC-001: Every submission chains through relay semaphores to guarantee
// GPU-side execution ordering between consecutive vkQueueSubmit calls.
func (q *Queue) Submit(commandBuffers []hal.CommandBuffer) (uint64, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(commandBuffers) == 0 {
		return 0, nil
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
			return 0, fmt.Errorf("vulkan: command buffer is not a Vulkan command buffer")
		}
		vkCmdBuffers = append(vkCmdBuffers, vkCB.handle)
	}
	defer func() {
		*pooledSlice = vkCmdBuffers[:0]
		cmdBufferPool.Put(pooledSlice)
	}()

	// Build wait/signal semaphore arrays. Maximum sizes:
	//   wait:   acquire(1) + relay(1) = 2
	//   signal: present(1) + relay(1) + timeline(1) = 3
	var (
		waitSems   [2]vk.Semaphore
		waitStages [2]vk.PipelineStageFlags
		waitCount  uint32

		signalSems  [3]vk.Semaphore
		signalCount uint32
	)

	// If we have an active swapchain, use its semaphores for GPU-side synchronization.
	// CRITICAL: Semaphores can only be used ONCE per frame.
	// - Wait on currentAcquireSem: ONLY on first submit (signaled by acquire)
	// - Signal presentSemaphores: ONLY on first submit (waited on by present)
	// Subsequent submits in the same frame run without semaphore synchronization.
	consumedAcquire := false
	if q.activeSwapchain != nil && !q.acquireUsed {
		waitSems[waitCount] = q.activeSwapchain.currentAcquireSem
		waitStages[waitCount] = vk.PipelineStageFlags(vk.PipelineStageColorAttachmentOutputBit)
		waitCount++

		signalSems[signalCount] = q.activeSwapchain.presentSemaphores[q.activeSwapchain.currentImage]
		signalCount++

		q.acquireUsed = true
		consumedAcquire = true
	}

	// VK-SYNC-001: Add relay semaphores for GPU-side submission ordering.
	// This ensures that barriers from one submission are visible to the next
	// (required by the wgpu_hal Queue trait, not guaranteed by Vulkan spec).
	if q.relay != nil {
		relayWait, relaySignal, err := q.relay.advance(q.device.cmds, q.device.handle)
		if err != nil {
			return 0, fmt.Errorf("vulkan: relay semaphore advance: %w", err)
		}
		if relayWait != 0 {
			waitSems[waitCount] = relayWait
			waitStages[waitCount] = vk.PipelineStageFlags(vk.PipelineStageTopOfPipeBit)
			waitCount++
		}
		signalSems[signalCount] = relaySignal
		signalCount++
	}

	submitInfo := vk.SubmitInfo{
		SType:              vk.StructureTypeSubmitInfo,
		CommandBufferCount: uint32(len(vkCmdBuffers)),
		PCommandBuffers:    &vkCmdBuffers[0],
	}
	if waitCount > 0 {
		submitInfo.WaitSemaphoreCount = waitCount
		submitInfo.PWaitSemaphores = &waitSems[0]
		submitInfo.PWaitDstStageMask = &waitStages[0]
	}
	if signalCount > 0 {
		submitInfo.SignalSemaphoreCount = signalCount
		submitInfo.PSignalSemaphores = &signalSems[0]
	}

	signalValue := q.device.timelineFence.nextSignalValue()

	// Timeline path (VK-IMPL-001): Attach timeline semaphore signal to the real submit.
	// This enables waitForGPU to track the latest submission.
	var timelineSubmitInfo vk.TimelineSemaphoreSubmitInfo
	if q.device.timelineFence.isTimeline {
		// VK-IMPL-004: Record which submission consumed this acquire semaphore.
		// Pre-acquire wait in acquireNextImage() uses this to ensure the GPU
		// has finished before reusing the semaphore.
		if consumedAcquire {
			q.activeSwapchain.acquireFenceValues[q.activeSwapchain.currentAcquireIdx] = signalValue
		}

		// Add timeline semaphore to the signal list.
		signalSems[signalCount] = q.device.timelineFence.timelineSemaphore
		signalCount++
		submitInfo.SignalSemaphoreCount = signalCount
		submitInfo.PSignalSemaphores = &signalSems[0]

		// Build timeline values arrays. For binary semaphores the value is 0
		// (ignored by the driver), for the timeline semaphore it is signalValue.
		// The values arrays MUST have the same count as the semaphore arrays.
		var waitValues [2]uint64   // all zeros — binary semaphores
		var signalValues [3]uint64 // zeros for binary, signalValue for timeline (always last)
		signalValues[signalCount-1] = signalValue

		timelineSubmitInfo = vk.TimelineSemaphoreSubmitInfo{
			SType: vk.StructureTypeTimelineSemaphoreSubmitInfo,
		}
		if waitCount > 0 {
			timelineSubmitInfo.WaitSemaphoreValueCount = waitCount
			timelineSubmitInfo.PWaitSemaphoreValues = &waitValues[0]
		}
		timelineSubmitInfo.SignalSemaphoreValueCount = signalCount
		timelineSubmitInfo.PSignalSemaphoreValues = &signalValues[0]

		submitInfo.PNext = (*uintptr)(unsafe.Pointer(&timelineSubmitInfo))

		result := vkQueueSubmit(q, 1, &submitInfo, vk.Fence(0))
		if result != vk.Success {
			return 0, fmt.Errorf("vulkan: vkQueueSubmit failed: %d", result)
		}
		return signalValue, nil
	}

	// Binary path (VK-IMPL-003): Get a fence from the pool to track this submission.
	// BUG-GOGPU-004 FIX: Always use pool fence directly — single vkQueueSubmit per frame.
	// Previously, user-provided fences caused a double submit (real + empty for pool tracking).
	// Now the HAL manages fences internally, eliminating the double submit entirely.
	pool := q.device.timelineFence.pool

	// VK-IMPL-004: Record fence value for pre-acquire wait (binary path).
	if consumedAcquire {
		q.activeSwapchain.acquireFenceValues[q.activeSwapchain.currentAcquireIdx] = signalValue
	}
	poolFence, err := pool.signal(q.device.cmds, q.device.handle, signalValue)
	if err != nil {
		return 0, fmt.Errorf("vulkan: Submit fencePool signal: %w", err)
	}

	// Single vkQueueSubmit with pool fence — no more double submit.
	result := vkQueueSubmit(q, 1, &submitInfo, poolFence)
	if result != vk.Success {
		return 0, fmt.Errorf("vulkan: vkQueueSubmit failed: %d", result)
	}
	return signalValue, nil
}

// PollCompleted returns the highest submission index known to be completed by the GPU.
// Non-blocking.
func (q *Queue) PollCompleted() uint64 {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.device.timelineFence.isTimeline {
		// Timeline path: query the semaphore counter value.
		var value uint64
		result := q.device.cmds.GetSemaphoreCounterValue(q.device.handle, q.device.timelineFence.timelineSemaphore, &value)
		if result == vk.Success {
			if value > q.device.timelineFence.lastCompleted {
				q.device.timelineFence.lastCompleted = value
			}
			return value
		}
		return q.device.timelineFence.lastCompleted
	}

	// Binary path: poll fences in the pool.
	pool := q.device.timelineFence.pool
	pool.maintain(q.device.cmds, q.device.handle)
	q.device.timelineFence.lastCompleted = pool.lastCompleted
	return pool.lastCompleted
}

// SubmitForPresent submits command buffers with swapchain synchronization.
//
// VK-SYNC-001: Every submission chains through relay semaphores to guarantee
// GPU-side execution ordering between consecutive vkQueueSubmit calls.
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

	// Build wait/signal semaphore arrays. Maximum sizes:
	//   wait:   acquire(1) + relay(1) = 2
	//   signal: present(1) + relay(1) + timeline(1) = 3
	var (
		waitSems   [2]vk.Semaphore
		waitStages [2]vk.PipelineStageFlags
		waitCount  uint32

		signalSems  [3]vk.Semaphore
		signalCount uint32
	)

	// Acquire semaphore: always present for SubmitForPresent.
	waitSems[waitCount] = swapchain.currentAcquireSem
	waitStages[waitCount] = vk.PipelineStageFlags(vk.PipelineStageColorAttachmentOutputBit)
	waitCount++

	// Present semaphore: always present for SubmitForPresent.
	signalSems[signalCount] = swapchain.presentSemaphores[swapchain.currentImage]
	signalCount++

	// VK-SYNC-001: Add relay semaphores for GPU-side submission ordering.
	if q.relay != nil {
		relayWait, relaySignal, err := q.relay.advance(q.device.cmds, q.device.handle)
		if err != nil {
			return fmt.Errorf("vulkan: relay semaphore advance: %w", err)
		}
		if relayWait != 0 {
			waitSems[waitCount] = relayWait
			waitStages[waitCount] = vk.PipelineStageFlags(vk.PipelineStageTopOfPipeBit)
			waitCount++
		}
		signalSems[signalCount] = relaySignal
		signalCount++
	}

	submitInfo := vk.SubmitInfo{
		SType:                vk.StructureTypeSubmitInfo,
		WaitSemaphoreCount:   waitCount,
		PWaitSemaphores:      &waitSems[0],
		PWaitDstStageMask:    &waitStages[0],
		CommandBufferCount:   uint32(len(vkCmdBuffers)),
		PCommandBuffers:      &vkCmdBuffers[0],
		SignalSemaphoreCount: signalCount,
		PSignalSemaphores:    &signalSems[0],
	}

	// Timeline path (VK-IMPL-001): Also signal the timeline semaphore on this submit.
	if q.device.timelineFence.isTimeline {
		signalValue := q.device.timelineFence.nextSignalValue()

		// VK-IMPL-004: Record which submission consumed this acquire semaphore.
		swapchain.acquireFenceValues[swapchain.currentAcquireIdx] = signalValue

		// Add timeline semaphore to the signal list.
		signalSems[signalCount] = q.device.timelineFence.timelineSemaphore
		signalCount++
		submitInfo.SignalSemaphoreCount = signalCount
		submitInfo.PSignalSemaphores = &signalSems[0]

		// Build timeline values arrays. Binary semaphores get value 0 (ignored),
		// timeline semaphore (always last) gets signalValue.
		var waitValues [2]uint64
		var signalValues [3]uint64
		signalValues[signalCount-1] = signalValue

		timelineSubmitInfo := vk.TimelineSemaphoreSubmitInfo{
			SType:                     vk.StructureTypeTimelineSemaphoreSubmitInfo,
			WaitSemaphoreValueCount:   waitCount,
			PWaitSemaphoreValues:      &waitValues[0],
			SignalSemaphoreValueCount: signalCount,
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
		// Bounds check: verify the write fits within the mapped region (BUG-VK-001).
		// Without this, a partial/failed vkAllocateMemory that returned a too-small
		// mapping would cause SIGSEGV in copyToMappedMemory.
		if vkBuffer.memory.MappedSize > 0 && offset+uint64(len(data)) > vkBuffer.memory.MappedSize {
			return fmt.Errorf("vulkan: WriteBuffer: write of %d bytes at offset %d exceeds mapped size %d (BUG-VK-001)",
				len(data), offset, vkBuffer.memory.MappedSize)
		}

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

	// Submit and wait for completion — WriteTexture must block because
	// the staging buffer data must be fully uploaded before return.
	subIdx, err := q.Submit([]hal.CommandBuffer{cmdBuffer})
	if err != nil {
		return fmt.Errorf("vulkan: WriteTexture: Submit failed: %w", err)
	}

	// Wait for the submission to complete (60 second timeout).
	timeoutNs := uint64(60 * time.Second)
	if waitErr := q.device.timelineFence.waitForValue(q.device.cmds, q.device.handle, subIdx, timeoutNs); waitErr != nil {
		hal.Logger().Warn("vulkan: WriteTexture: wait failed", "err", waitErr)
	}

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

// SupportsCommandBufferCopies returns true for Vulkan.
// Vulkan uses command buffers for copy operations — PendingWrites batches them.
func (q *Queue) SupportsCommandBufferCopies() bool {
	return true
}

// Vulkan function wrapper

//nolint:unparam // Vulkan API wrapper — signature mirrors vkQueueSubmit spec
func vkQueueSubmit(q *Queue, submitCount uint32, submits *vk.SubmitInfo, fence vk.Fence) vk.Result {
	return q.device.cmds.QueueSubmit(q.handle, submitCount, submits, fence)
}
