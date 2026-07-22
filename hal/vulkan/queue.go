//go:build !(js && wasm)

// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vulkan

import (
	"fmt"
	"image"
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

	// BUG-WGPU-VK-005: Offscreen submit semaphore suppression.
	// When swapchainSuppressed is true, Submit skips swapchain semaphore binding.
	// savedSwapchain/savedAcquireUsed hold the original values for restoration.
	// Same pattern as VK-004 (WriteTexture, lines 620-634) but caller-controlled.
	swapchainSuppressed bool
	savedSwapchain      *Swapchain
	savedAcquireUsed    bool
}

func validateSwapchainSubmission(swapchain *Swapchain, device *Device) error {
	if swapchain == nil {
		return nil
	}
	if swapchain.device != device {
		return fmt.Errorf("vulkan: active swapchain belongs to a different device")
	}
	if swapchain.destroyed {
		return fmt.Errorf("vulkan: active swapchain has been destroyed")
	}
	if swapchain.broken {
		if swapchain.failureErr != nil {
			return fmt.Errorf("vulkan: active swapchain is broken: %w", swapchain.failureErr)
		}
		return fmt.Errorf("vulkan: active swapchain is broken")
	}
	if !swapchain.imageAcquired {
		return fmt.Errorf("vulkan: no swapchain image acquired for submission")
	}
	if swapchain.currentAcquireIdx < 0 || swapchain.currentAcquireIdx >= len(swapchain.acquireSemaphores) || swapchain.currentAcquireIdx >= len(swapchain.acquireFenceValues) {
		return fmt.Errorf("vulkan: swapchain acquire semaphore state is inconsistent")
	}
	if swapchain.currentImage >= uint32(len(swapchain.presentSemaphores)) {
		return fmt.Errorf("vulkan: swapchain present image index is out of range")
	}
	return nil
}

func (q *Queue) markActiveSwapchainBroken(err error) {
	if q.activeSwapchain != nil {
		q.activeSwapchain.markBroken(err)
	}
}

func (q *Queue) advanceRelay() (wait, signal vk.Semaphore, err error) {
	if q.relay == nil {
		return 0, 0, nil
	}
	wait, signal, err = q.relay.advance(q.device.cmds, q.device.handle)
	if err == nil {
		return wait, signal, nil
	}
	err = fmt.Errorf("vulkan: relay semaphore advance: %w", err)
	q.markActiveSwapchainBroken(err)
	return 0, 0, err
}

func (q *Queue) submitTimeline(
	submitInfo *vk.SubmitInfo,
	signalSems *[3]vk.Semaphore,
	waitCount, signalCount uint32,
	consumedAcquire bool,
	signalValue uint64,
) (uint64, error) {
	if consumedAcquire {
		q.activeSwapchain.acquireFenceValues[q.activeSwapchain.currentAcquireIdx] = signalValue
	}

	signalSems[signalCount] = q.device.timelineFence.timelineSemaphore
	signalCount++
	submitInfo.SignalSemaphoreCount = signalCount
	submitInfo.PSignalSemaphores = &signalSems[0]

	var waitValues [2]uint64
	var signalValues [3]uint64
	signalValues[signalCount-1] = signalValue
	timelineSubmitInfo := vk.TimelineSemaphoreSubmitInfo{
		SType: vk.StructureTypeTimelineSemaphoreSubmitInfo,
	}
	if waitCount > 0 {
		timelineSubmitInfo.WaitSemaphoreValueCount = waitCount
		timelineSubmitInfo.PWaitSemaphoreValues = &waitValues[0]
	}
	timelineSubmitInfo.SignalSemaphoreValueCount = signalCount
	timelineSubmitInfo.PSignalSemaphoreValues = &signalValues[0]
	submitInfo.PNext = (*uintptr)(unsafe.Pointer(&timelineSubmitInfo))

	result := vkQueueSubmit(q, 1, submitInfo, vk.Fence(0))
	if result != vk.Success {
		err := fmt.Errorf("vulkan: vkQueueSubmit failed: %d", result)
		if consumedAcquire {
			q.activeSwapchain.markBroken(err)
		}
		return 0, err
	}
	return signalValue, nil
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
	if err := validateSwapchainSubmission(q.activeSwapchain, q.device); err != nil {
		return 0, err
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
		if q.activeSwapchain.currentAcquireSem == 0 || q.activeSwapchain.currentImage >= uint32(len(q.activeSwapchain.presentSemaphores)) {
			err := fmt.Errorf("vulkan: active swapchain semaphore state is invalid")
			q.activeSwapchain.markBroken(err)
			return 0, err
		}
		presentSemaphore := q.activeSwapchain.presentSemaphores[q.activeSwapchain.currentImage]
		if presentSemaphore == 0 {
			err := fmt.Errorf("vulkan: active swapchain present semaphore is destroyed")
			q.activeSwapchain.markBroken(err)
			return 0, err
		}
		waitSems[waitCount] = q.activeSwapchain.currentAcquireSem
		waitStages[waitCount] = vk.PipelineStageFlags(vk.PipelineStageColorAttachmentOutputBit)
		waitCount++

		signalSems[signalCount] = presentSemaphore
		signalCount++

		q.acquireUsed = true
		consumedAcquire = true
	}

	// VK-SYNC-001: Add relay semaphores for GPU-side submission ordering.
	// This ensures that barriers from one submission are visible to the next
	// (required by the wgpu_hal Queue trait, not guaranteed by Vulkan spec).
	relayWait, relaySignal, err := q.advanceRelay()
	if err != nil {
		return 0, err
	}
	if relayWait != 0 {
		waitSems[waitCount] = relayWait
		waitStages[waitCount] = vk.PipelineStageFlags(vk.PipelineStageTopOfPipeBit)
		waitCount++
	}
	if relaySignal != 0 {
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

	if q.device.timelineFence.isTimeline {
		return q.submitTimeline(&submitInfo, &signalSems, waitCount, signalCount, consumedAcquire, signalValue)
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
		if consumedAcquire {
			q.activeSwapchain.markBroken(fmt.Errorf("vulkan: Submit fencePool signal: %w", err))
		}
		return 0, fmt.Errorf("vulkan: Submit fencePool signal: %w", err)
	}

	// Single vkQueueSubmit with pool fence — no more double submit.
	result := vkQueueSubmit(q, 1, &submitInfo, poolFence)
	if result != vk.Success {
		if consumedAcquire {
			q.activeSwapchain.markBroken(fmt.Errorf("vulkan: vkQueueSubmit failed: %d", result))
		}
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
	if swapchain == nil {
		return fmt.Errorf("vulkan: swapchain is nil")
	}
	if err := validateSwapchainSubmission(swapchain, q.device); err != nil {
		return err
	}
	if q.activeSwapchain != nil && q.activeSwapchain != swapchain {
		return fmt.Errorf("vulkan: queue has a different active swapchain")
	}
	if q.activeSwapchain == swapchain && q.acquireUsed {
		swapchain.markBroken(fmt.Errorf("vulkan: swapchain acquire semaphore was already consumed"))
		return fmt.Errorf("vulkan: swapchain acquire semaphore was already consumed")
	}
	if swapchain.currentAcquireSem == 0 || swapchain.currentImage >= uint32(len(swapchain.presentSemaphores)) {
		err := fmt.Errorf("vulkan: swapchain semaphore state is invalid")
		swapchain.markBroken(err)
		return err
	}
	presentSemaphore := swapchain.presentSemaphores[swapchain.currentImage]
	if presentSemaphore == 0 {
		err := fmt.Errorf("vulkan: swapchain present semaphore is destroyed")
		swapchain.markBroken(err)
		return err
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
	signalSems[signalCount] = presentSemaphore
	signalCount++

	// VK-SYNC-001: Add relay semaphores for GPU-side submission ordering.
	if q.relay != nil {
		relayWait, relaySignal, err := q.relay.advance(q.device.cmds, q.device.handle)
		if err != nil {
			swapchain.markBroken(fmt.Errorf("vulkan: relay semaphore advance: %w", err))
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
			swapchain.markBroken(fmt.Errorf("vulkan: vkQueueSubmit failed: %d", result))
			return fmt.Errorf("vulkan: vkQueueSubmit failed: %d", result)
		}
		q.activeSwapchain = swapchain
		q.acquireUsed = true
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
		swapchain.markBroken(fmt.Errorf("vulkan: SubmitForPresent fencePool signal: %w", err))
		return fmt.Errorf("vulkan: SubmitForPresent fencePool signal: %w", err)
	}

	result := vkQueueSubmit(q, 1, &submitInfo, poolFence)
	if result != vk.Success {
		swapchain.markBroken(fmt.Errorf("vulkan: vkQueueSubmit failed: %d", result))
		return fmt.Errorf("vulkan: vkQueueSubmit failed: %d", result)
	}

	q.activeSwapchain = swapchain
	q.acquireUsed = true
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

	// Buffer must be mapped (host-visible) for direct CPU writes.
	// Device-local buffers require staging at the public API level.
	if vkBuffer.memory.MappedPtr == 0 {
		return fmt.Errorf("vulkan: WriteBuffer: buffer is not mapped")
	}

	// Bounds check: verify the write fits within the mapped region (BUG-VK-001).
	// Without this, a partial/failed vkAllocateMemory that returned a too-small
	// mapping would cause SIGSEGV in copyToMappedMemory.
	if vkBuffer.memory.MappedSize > 0 && offset+uint64(len(data)) > vkBuffer.memory.MappedSize {
		return fmt.Errorf("vulkan: WriteBuffer: write of %d bytes at offset %d exceeds mapped size %d (BUG-VK-001)",
			len(data), offset, vkBuffer.memory.MappedSize)
	}

	// Direct copy using Vulkan mapped memory from vkMapMemory.
	// Use copyToMappedMemory to avoid go vet false positive about unsafe.Pointer.
	copyToMappedMemory(vkBuffer.memory.MappedPtr, offset, data)

	// Flush mapped memory to ensure GPU sees CPU writes.
	// Only needed for non-coherent memory. (BUG-VK-009 Fix 2/5)
	if !vkBuffer.memory.IsCoherent {
		alignedOffset, alignedSize := q.device.alignedMappedRange(vkBuffer.memory.Offset, vkBuffer.memory.Size)
		memRange := vk.MappedMemoryRange{
			SType:  vk.StructureTypeMappedMemoryRange,
			Memory: vkBuffer.memory.Memory,
			Offset: alignedOffset,
			Size:   alignedSize,
		}
		result := q.device.cmds.FlushMappedMemoryRanges(q.device.handle, 1, &memRange)
		if result != vk.Success {
			return fmt.Errorf("vulkan: WriteBuffer: FlushMappedMemoryRanges failed: %d", result)
		}
	}
	return nil
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
//
// damageRects is an optional list of rectangles (physical pixels, top-left
// origin) indicating which surface regions changed this frame. When non-empty
// and VK_KHR_incremental_present is supported, the rects are chained into
// VkPresentInfoKHR.PNext as a compositor hint. When empty or the extension
// is unavailable, the present path is identical to a full-surface present.
func (q *Queue) Present(surface hal.Surface, _ hal.SurfaceTexture, damageRects []image.Rectangle) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	vkSurface, ok := surface.(*Surface)
	if !ok {
		return fmt.Errorf("vulkan: surface is not a Vulkan surface")
	}

	if vkSurface.swapchain == nil {
		return fmt.Errorf("vulkan: surface not configured")
	}

	swapchain := vkSurface.swapchain
	err := swapchain.present(q, damageRects)
	if q.activeSwapchain == swapchain {
		q.activeSwapchain = nil
		q.acquireUsed = false
	}
	return err
}

// GetTimestampPeriod returns the timestamp period in nanoseconds per tick.
// Queried from VkPhysicalDeviceLimits.TimestampPeriod at device init.
// Intel typically 1.0, AMD/NVIDIA vary.
func (q *Queue) GetTimestampPeriod() float32 {
	return q.device.timestampPeriod
}

// SupportsCommandBufferCopies returns true for Vulkan.
// Vulkan uses command buffers for copy operations — PendingWrites batches them.
func (q *Queue) SupportsCommandBufferCopies() bool {
	return true
}

// SetSwapchainSuppressed temporarily disables swapchain semaphore binding
// for subsequent Submit calls. Used for offscreen renders that must not
// consume acquire/present semaphores intended for the compositor submit.
//
// BUG-WGPU-VK-005: When ui uses RepaintBoundary with GPU texture caching,
// there are two Submit calls per frame (offscreen + compositor). Without
// suppression, the first (offscreen) submit hijacks swapchain semaphores,
// leaving the compositor submit without synchronization -> race -> flickering.
//
// When suppressed is true: saves activeSwapchain/acquireUsed, clears activeSwapchain.
// When suppressed is false: restores saved values.
//
// Same save/restore pattern as VK-004 in WriteTexture (lines 620-634).
func (q *Queue) SetSwapchainSuppressed(suppressed bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if suppressed {
		// Save current swapchain state and suppress semaphore binding.
		q.savedSwapchain = q.activeSwapchain
		q.savedAcquireUsed = q.acquireUsed
		q.activeSwapchain = nil
		q.swapchainSuppressed = true
	} else if q.swapchainSuppressed {
		// Restore saved swapchain state so the next Submit (compositor)
		// correctly binds acquire/present semaphores.
		q.activeSwapchain = q.savedSwapchain
		q.acquireUsed = q.savedAcquireUsed
		q.savedSwapchain = nil
		q.savedAcquireUsed = false
		q.swapchainSuppressed = false
	}
}

// Vulkan function wrapper

//nolint:unparam // Vulkan API wrapper — signature mirrors vkQueueSubmit spec
func vkQueueSubmit(q *Queue, submitCount uint32, submits *vk.SubmitInfo, fence vk.Fence) vk.Result {
	return q.device.cmds.QueueSubmit(q.handle, submitCount, submits, fence)
}
