// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vulkan

import (
	"fmt"
	"sync/atomic"
	"unsafe"

	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/vulkan/vk"
)

// deviceFence abstracts GPU synchronization using either a VK_KHR_timeline_semaphore
// (Vulkan 1.2 core) or a fencePool of binary VkFences as fallback.
//
// Timeline semaphore advantages over binary fences:
//   - Single VkSemaphore with monotonic uint64 counter (no ring buffer needed)
//   - Signal is attached to the real vkQueueSubmit (no empty submit)
//   - Wait uses vkWaitSemaphores (no reset needed unlike binary fences)
//   - Replaces BOTH frame fences AND transfer fence with one primitive
//
// Binary path (VK-IMPL-003): Uses a fencePool with per-submission tracking.
// Each submission gets a dedicated VkFence from the pool; signaled fences are
// recycled into a free list. This replaces the previous 2-slot ring buffer
// and the separate transferFence with a single unified mechanism.
type deviceFence struct {
	// Timeline semaphore path (preferred, Vulkan 1.2+).
	timelineSemaphore vk.Semaphore

	// lastSignaled is the most recent value signaled.
	// Incremented atomically for each submit on both paths.
	lastSignaled atomic.Uint64

	// lastCompleted tracks the value known to be completed by the GPU.
	// Updated after successful wait. On binary path, this mirrors pool.lastCompleted.
	lastCompleted uint64

	// pool manages binary VkFences for the fallback path (VK-IMPL-003).
	// nil on the timeline path.
	pool *fencePool

	// isTimeline indicates which path is active.
	isTimeline bool
}

// initTimelineFence creates a timeline semaphore for GPU synchronization.
// Returns an error if the driver does not support timeline semaphores.
func initTimelineFence(cmds *vk.Commands, device vk.Device) (*deviceFence, error) {
	if !cmds.HasTimelineSemaphore() {
		return nil, fmt.Errorf("timeline semaphore functions not available")
	}

	// Create timeline semaphore with initial value 0.
	semTypeInfo := vk.SemaphoreTypeCreateInfo{
		SType:         vk.StructureTypeSemaphoreTypeCreateInfo,
		SemaphoreType: vk.SemaphoreTypeTimeline,
		InitialValue:  0,
	}

	createInfo := vk.SemaphoreCreateInfo{
		SType: vk.StructureTypeSemaphoreCreateInfo,
		PNext: (*uintptr)(unsafe.Pointer(&semTypeInfo)),
	}

	var sem vk.Semaphore
	result := cmds.CreateSemaphore(device, &createInfo, nil, &sem)
	if result != vk.Success {
		return nil, fmt.Errorf("vkCreateSemaphore (timeline) failed: %d", result)
	}

	hal.Logger().Debug("vulkan: timeline semaphore fence created")

	return &deviceFence{
		timelineSemaphore: sem,
		isTimeline:        true,
	}, nil
}

// initBinaryFence creates a deviceFence backed by a fencePool for Vulkan <1.2
// where timeline semaphores are not available (VK-IMPL-003).
func initBinaryFence() *deviceFence {
	hal.Logger().Debug("vulkan: binary fence pool created (VK-IMPL-003)")
	return &deviceFence{
		pool:       &fencePool{},
		isTimeline: false,
	}
}

// nextSignalValue returns the next value to signal on the timeline semaphore.
func (f *deviceFence) nextSignalValue() uint64 {
	return f.lastSignaled.Add(1)
}

// currentSignalValue returns the current (most recent) signal value.
func (f *deviceFence) currentSignalValue() uint64 {
	return f.lastSignaled.Load()
}

// waitForValue waits until the GPU completes the submission with the specified value.
// Returns immediately if the value is already completed.
// timeoutNs is the timeout in nanoseconds.
//
// Timeline path: uses vkWaitSemaphores on the timeline semaphore.
// Binary path (VK-IMPL-003): delegates to fencePool.wait().
func (f *deviceFence) waitForValue(cmds *vk.Commands, device vk.Device, value uint64, timeoutNs uint64) error {
	if !f.isTimeline {
		// Binary path: delegate to fence pool (VK-IMPL-003).
		if err := f.pool.wait(cmds, device, value, timeoutNs); err != nil {
			return err
		}
		f.lastCompleted = f.pool.lastCompleted
		return nil
	}

	// Timeline path.
	// Fast path: already completed.
	if value <= f.lastCompleted {
		return nil
	}

	// Fast path: nothing has been signaled yet (value 0 means no submit).
	if value == 0 {
		return nil
	}

	waitInfo := vk.SemaphoreWaitInfo{
		SType:          vk.StructureTypeSemaphoreWaitInfo,
		SemaphoreCount: 1,
		PSemaphores:    &f.timelineSemaphore,
		PValues:        &value,
	}

	result := cmds.WaitSemaphores(device, &waitInfo, timeoutNs)
	switch result {
	case vk.Success:
		f.lastCompleted = value
		return nil
	case vk.Timeout:
		return fmt.Errorf("vulkan: timeline semaphore wait timed out (value=%d)", value)
	case vk.ErrorDeviceLost:
		return hal.ErrDeviceLost
	default:
		return fmt.Errorf("vulkan: vkWaitSemaphores failed: %d", result)
	}
}

// waitForLatest waits for the most recently signaled value to complete.
// This is used as a replacement for the transfer fence wait pattern.
//
// Timeline path: waits for the current signal value.
// Binary path (VK-IMPL-003): delegates to fencePool.waitForLatest().
func (f *deviceFence) waitForLatest(cmds *vk.Commands, device vk.Device, timeoutNs uint64) error {
	if !f.isTimeline {
		if err := f.pool.waitForLatest(cmds, device, timeoutNs); err != nil {
			return err
		}
		f.lastCompleted = f.pool.lastCompleted
		return nil
	}
	return f.waitForValue(cmds, device, f.currentSignalValue(), timeoutNs)
}

// destroy releases synchronization resources.
// Timeline path: destroys the timeline semaphore.
// Binary path (VK-IMPL-003): destroys all fences in the pool.
func (f *deviceFence) destroy(cmds *vk.Commands, device vk.Device) {
	if f.timelineSemaphore != 0 {
		cmds.DestroySemaphore(device, f.timelineSemaphore, nil)
		f.timelineSemaphore = 0
	}
	if f.pool != nil {
		f.pool.destroy(cmds, device)
		f.pool = nil
	}
}
