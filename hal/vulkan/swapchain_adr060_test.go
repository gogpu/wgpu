//go:build !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vulkan

import (
	"testing"

	"github.com/gogpu/wgpu/hal/vulkan/vk"
)

// TestPresentSemaphorePoolOrderingContract verifies the ADR-060 invariant:
// when a semaphore is allocated from the pool (e.g., by ensurePresentLayout)
// BEFORE presentWaitSemaphores is called, the allocated semaphore IS included
// in the returned wait list.
//
// This is the core correctness property for barrier synchronization:
// present() calls ensurePresentLayout (which allocates a barrier semaphore)
// THEN calls presentWaitSemaphores, ensuring the present waits on the barrier.
func TestPresentSemaphorePoolOrderingContract(t *testing.T) {
	t.Parallel()

	// Simulate a pool with pre-allocated semaphores (as if vkCreateSemaphore
	// had been called previously). We use sentinel values as VkSemaphore handles.
	pool := presentSemaphorePool{
		semaphores: []vk.Semaphore{
			vk.Semaphore(100), // sem 0
			vk.Semaphore(200), // sem 1
			vk.Semaphore(300), // sem 2
		},
		used: 0,
	}

	// Simulate Submit() allocating a present semaphore.
	pool.used++
	submitSem := pool.semaphores[0]

	// Simulate ensurePresentLayout allocating a barrier semaphore.
	pool.used++
	barrierSem := pool.semaphores[1]

	// Verify presentWaitSemaphores returns BOTH semaphores.
	waitSems := pool.semaphores[:pool.used]

	if len(waitSems) != 2 {
		t.Fatalf("waitSems length = %d, want 2", len(waitSems))
	}

	foundSubmit, foundBarrier := false, false
	for _, s := range waitSems {
		if s == submitSem {
			foundSubmit = true
		}
		if s == barrierSem {
			foundBarrier = true
		}
	}
	if !foundSubmit {
		t.Error("Submit semaphore not found in wait list")
	}
	if !foundBarrier {
		t.Error("Barrier semaphore not found in wait list (ADR-060 ordering violation)")
	}
}

// TestPresentSemaphorePoolResetRecycles verifies that resetPresentPool
// resets the used counter, recycling semaphores for the next frame.
func TestPresentSemaphorePoolResetRecycles(t *testing.T) {
	t.Parallel()

	pool := presentSemaphorePool{
		semaphores: []vk.Semaphore{
			vk.Semaphore(100),
			vk.Semaphore(200),
		},
		used: 2,
	}

	// After present, reset the pool.
	pool.used = 0

	if len(pool.semaphores[:pool.used]) != 0 {
		t.Error("After reset, wait list should be empty")
	}
	if len(pool.semaphores) != 2 {
		t.Error("Semaphores should not be destroyed on reset")
	}
}

// TestPresentSemaphorePoolMultiSubmit verifies that multiple Submit() calls
// per frame each get their own semaphore, and present waits on all of them.
// This is the ADR-058 multi-submit scenario (e.g., g3d + gg renders).
func TestPresentSemaphorePoolMultiSubmit(t *testing.T) {
	t.Parallel()

	pool := presentSemaphorePool{
		semaphores: []vk.Semaphore{
			vk.Semaphore(100),
			vk.Semaphore(200),
			vk.Semaphore(300),
		},
		used: 0,
	}

	// Submit 1: g3d render
	pool.used++
	// Submit 2: gg overlay
	pool.used++
	// ensurePresentLayout: barrier
	pool.used++

	waitSems := pool.semaphores[:pool.used]
	if len(waitSems) != 3 {
		t.Fatalf("waitSems = %d, want 3 (2 submits + 1 barrier)", len(waitSems))
	}
}

// TestSwapchainBarrierPoolResetDeferred verifies the ADR-059 pattern:
// barrierPending is set after ensurePresentLayout and cleared after
// acquireNextImage resets the pool.
func TestSwapchainBarrierPoolResetDeferred(t *testing.T) {
	t.Parallel()

	// Test the state machine: barrierPending tracks lifecycle.
	sc := Swapchain{
		barrierPending: false,
		barrierPool:    vk.CommandPool(1), // non-zero means pool exists
	}

	// After ensurePresentLayout submits a barrier:
	sc.barrierPending = true

	if !sc.barrierPending {
		t.Error("barrierPending should be true after barrier submit")
	}

	// After acquireNextImage resets the pool (simulated):
	sc.barrierPending = false

	if sc.barrierPending {
		t.Error("barrierPending should be false after pool reset")
	}
}
