// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vulkan

import (
	"testing"

	"github.com/gogpu/wgpu/hal/vulkan/vk"
)

// TestSubmitBreaksOnlyTheSwapchainItCannotValidate checks that a swapchain
// rejected before any submit state is committed fails alone. The frame would
// otherwise leave the swapchains before it marked as consumed with no
// submission to wait on their acquire semaphores.
func TestSubmitBreaksOnlyTheSwapchainItCannotValidate(t *testing.T) {
	device := &Device{}
	queue := &Queue{device: device}

	ready := &Swapchain{
		device:             device,
		imageAcquired:      true,
		acquireSemaphores:  []vk.Semaphore{1},
		acquireFenceValues: []uint64{0},
		currentAcquireIdx:  0,
		currentAcquireSem:  1,
		currentImage:       0,
		presentPools:       []presentSemaphorePool{{}},
	}
	invalid := &Swapchain{
		device:             device,
		imageAcquired:      true,
		acquireSemaphores:  []vk.Semaphore{0},
		acquireFenceValues: []uint64{0},
		currentAcquireIdx:  0,
		currentAcquireSem:  0,
		currentImage:       0,
		presentPools:       []presentSemaphorePool{{}},
	}

	if _, err := queue.submitLocked([]vk.CommandBuffer{1}, []*Swapchain{ready, invalid}); err == nil {
		t.Fatal("submitLocked accepted a submission whose swapchain has no acquire semaphore")
	}
	if !invalid.broken {
		t.Fatal("the swapchain that cannot be submitted was not marked broken")
	}
	if ready.broken {
		t.Fatal("a swapchain that only precedes the invalid one was marked broken")
	}
	if ready.acquireUsed {
		t.Fatal("the swapchain before the invalid one was left with its acquire marked used and no submit")
	}
}
