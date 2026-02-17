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
// (Vulkan 1.2 core) or binary VkFences as fallback.
//
// Timeline semaphore advantages over binary fences:
//   - Single VkSemaphore with monotonic uint64 counter (no ring buffer needed)
//   - Signal is attached to the real vkQueueSubmit (no empty submit)
//   - Wait uses vkWaitSemaphores (no reset needed unlike binary fences)
//   - Replaces BOTH frame fences AND transfer fence with one primitive
//
// Fallback path uses the existing 2-slot binary fence ring buffer when
// timeline semaphores are not available (pre-Vulkan 1.2 drivers).
type deviceFence struct {
	// Timeline semaphore path (preferred, Vulkan 1.2+).
	timelineSemaphore vk.Semaphore
	// lastSignaled is the most recent value signaled on the timeline semaphore.
	// Incremented atomically for each submit.
	lastSignaled atomic.Uint64
	// lastCompleted tracks the value known to be completed by the GPU.
	// Updated after successful vkWaitSemaphores.
	lastCompleted uint64

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

// nextSignalValue returns the next value to signal on the timeline semaphore.
func (f *deviceFence) nextSignalValue() uint64 {
	return f.lastSignaled.Add(1)
}

// currentSignalValue returns the current (most recent) signal value.
func (f *deviceFence) currentSignalValue() uint64 {
	return f.lastSignaled.Load()
}

// waitForValue waits until the timeline semaphore reaches the specified value.
// Returns immediately if the value is already completed.
// timeoutNs is the timeout in nanoseconds.
func (f *deviceFence) waitForValue(cmds *vk.Commands, device vk.Device, value uint64, timeoutNs uint64) error {
	if !f.isTimeline {
		return fmt.Errorf("waitForValue called on binary fence path")
	}

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
func (f *deviceFence) waitForLatest(cmds *vk.Commands, device vk.Device, timeoutNs uint64) error {
	return f.waitForValue(cmds, device, f.currentSignalValue(), timeoutNs)
}

// destroy releases the timeline semaphore.
func (f *deviceFence) destroy(cmds *vk.Commands, device vk.Device) {
	if f.timelineSemaphore != 0 {
		cmds.DestroySemaphore(device, f.timelineSemaphore, nil)
		f.timelineSemaphore = 0
	}
}
