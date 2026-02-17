// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vulkan

import (
	"fmt"

	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/vulkan/vk"
)

// fencePool manages binary VkFences for Vulkan <1.2 where timeline semaphores
// are unavailable. Mirrors Rust wgpu-hal's FencePool pattern.
//
// Instead of a fixed 2-slot ring buffer, fencePool tracks per-submission fences
// with monotonic values. This enables fine-grained synchronization: the caller
// can wait for any specific submission rather than just the latest two frames.
//
// Fences are recycled into a free list after GPU completion to avoid repeated
// vkCreateFence/vkDestroyFence calls.
type fencePool struct {
	// active contains submitted fences awaiting GPU completion,
	// ordered by ascending value.
	active []fenceEntry

	// free contains recycled fences ready for reuse.
	free []vk.Fence

	// lastCompleted is the high watermark: largest submission value
	// known to be completed by the GPU.
	lastCompleted uint64
}

// fenceEntry pairs a monotonic submission value with the binary fence
// signaled on that submission.
type fenceEntry struct {
	value uint64   // Monotonic submission value
	fence vk.Fence // Binary fence signaled on this submission
}

// maintain performs a non-blocking poll of active fences, moving signaled
// fences to the free list and updating lastCompleted.
//
// This should be called periodically (e.g., at the start of wait or signal)
// to reclaim fences without blocking.
func (p *fencePool) maintain(cmds *vk.Commands, device vk.Device) {
	n := 0
	for _, entry := range p.active {
		status := cmds.GetFenceStatus(device, entry.fence)
		if status == vk.Success {
			// Fence is signaled: reset and recycle.
			_ = cmds.ResetFences(device, 1, &entry.fence)
			p.free = append(p.free, entry.fence)
			if entry.value > p.lastCompleted {
				p.lastCompleted = entry.value
			}
		} else {
			// Not signaled (NotReady or error): keep in active list.
			p.active[n] = entry
			n++
		}
	}
	p.active = p.active[:n]
}

// signal returns a fence to be passed to vkQueueSubmit for the given
// submission value. The fence is taken from the free list if available,
// otherwise a new one is created.
//
// The caller must pass the returned fence to vkQueueSubmit. After the GPU
// signals it, maintain() or wait() will recycle it.
func (p *fencePool) signal(cmds *vk.Commands, device vk.Device, value uint64) (vk.Fence, error) {
	var fence vk.Fence

	// Pop from free list if available.
	if n := len(p.free); n > 0 {
		fence = p.free[n-1]
		p.free = p.free[:n-1]
	} else {
		// Create a new unsignaled fence.
		createInfo := vk.FenceCreateInfo{
			SType: vk.StructureTypeFenceCreateInfo,
			Flags: 0,
		}
		result := cmds.CreateFence(device, &createInfo, nil, &fence)
		if result != vk.Success {
			return 0, fmt.Errorf("vulkan: fencePool: vkCreateFence failed: %d", result)
		}
	}

	p.active = append(p.active, fenceEntry{value: value, fence: fence})
	return fence, nil
}

// wait blocks until the GPU completes the submission with the given value.
// Returns immediately if the value is already known to be completed.
//
// timeoutNs is the timeout in nanoseconds for vkWaitForFences.
func (p *fencePool) wait(cmds *vk.Commands, device vk.Device, value uint64, timeoutNs uint64) error {
	// Fast path: already completed.
	if value <= p.lastCompleted {
		return nil
	}

	// Fast path: nothing submitted yet.
	if value == 0 {
		return nil
	}

	// Collect any newly completed fences without blocking.
	p.maintain(cmds, device)
	if value <= p.lastCompleted {
		return nil
	}

	// Find the fence for the requested value in the active list.
	var targetFence vk.Fence
	targetIdx := -1
	for i, entry := range p.active {
		if entry.value == value {
			targetFence = entry.fence
			targetIdx = i
			break
		}
		// If the exact value is not found, wait for the smallest value >= requested.
		// This handles the case where multiple submissions share the same fence epoch.
		if entry.value > value && (targetFence == 0 || entry.value < p.active[targetIdx].value) {
			targetFence = entry.fence
			targetIdx = i
		}
	}

	if targetFence == 0 {
		// No active fence covers this value. It must have already completed
		// but lastCompleted was not updated (race with maintain). Treat as done.
		return nil
	}

	result := cmds.WaitForFences(device, 1, &targetFence, vk.Bool32(vk.True), timeoutNs)
	switch result {
	case vk.Success:
		// Reset and recycle the fence.
		_ = cmds.ResetFences(device, 1, &targetFence)
		completedValue := p.active[targetIdx].value
		if completedValue > p.lastCompleted {
			p.lastCompleted = completedValue
		}

		// Remove from active list (swap-remove for O(1)).
		last := len(p.active) - 1
		p.active[targetIdx] = p.active[last]
		p.active = p.active[:last]

		// Also collect any other fences that completed while we waited.
		p.maintain(cmds, device)
		return nil
	case vk.Timeout:
		return fmt.Errorf("vulkan: fencePool: wait timed out (value=%d)", value)
	case vk.ErrorDeviceLost:
		return hal.ErrDeviceLost
	default:
		return fmt.Errorf("vulkan: fencePool: vkWaitForFences failed: %d", result)
	}
}

// waitForLatest blocks until the GPU completes the highest active submission.
// Returns immediately if no submissions are active.
func (p *fencePool) waitForLatest(cmds *vk.Commands, device vk.Device, timeoutNs uint64) error {
	if len(p.active) == 0 {
		return nil
	}

	// Find the highest active value.
	var maxValue uint64
	for _, entry := range p.active {
		if entry.value > maxValue {
			maxValue = entry.value
		}
	}

	return p.wait(cmds, device, maxValue, timeoutNs)
}

// destroy releases all fences (both active and free) via vkDestroyFence.
// Must be called only after the GPU is idle (e.g., after vkDeviceWaitIdle).
func (p *fencePool) destroy(cmds *vk.Commands, device vk.Device) {
	for _, entry := range p.active {
		cmds.DestroyFence(device, entry.fence, nil)
	}
	p.active = nil

	for _, fence := range p.free {
		cmds.DestroyFence(device, fence, nil)
	}
	p.free = nil

	p.lastCompleted = 0
}
