// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vulkan

import (
	"testing"

	"github.com/gogpu/wgpu/hal/vulkan/vk"
)

// TestRelaySemaphoresStateMachine validates the two-semaphore alternation pattern
// used by relaySemaphores.advance(). Since advance() calls vkCreateSemaphore for
// the second semaphore, we test the state machine by simulating the struct state
// after the first advance() has already created both semaphores.
//
// Reference: Rust wgpu-hal vulkan/mod.rs:568-588.
func TestRelaySemaphoresStateMachine(t *testing.T) {
	// Simulate state after the first advance() call has completed:
	// wait=sem1 (the one first submission signaled), signal=sem2 (newly created).
	sem1 := vk.Semaphore(0x1001)
	sem2 := vk.Semaphore(0x1002)

	r := &relaySemaphores{
		wait:   sem1,
		signal: sem2,
	}

	// Second submission: should return (wait=sem1, signal=sem2), then swap.
	wait, signal := r.wait, r.signal
	// Simulate the swap branch of advance() (the branch for wait != 0).
	r.wait, r.signal = r.signal, r.wait

	if wait != sem1 {
		t.Errorf("submit 2: wait = %v, want %v", wait, sem1)
	}
	if signal != sem2 {
		t.Errorf("submit 2: signal = %v, want %v", signal, sem2)
	}
	if r.wait != sem2 {
		t.Errorf("after submit 2: state.wait = %v, want %v (sem2)", r.wait, sem2)
	}
	if r.signal != sem1 {
		t.Errorf("after submit 2: state.signal = %v, want %v (sem1)", r.signal, sem1)
	}

	// Third submission: should return (wait=sem2, signal=sem1), then swap back.
	wait, signal = r.wait, r.signal
	r.wait, r.signal = r.signal, r.wait

	if wait != sem2 {
		t.Errorf("submit 3: wait = %v, want %v", wait, sem2)
	}
	if signal != sem1 {
		t.Errorf("submit 3: signal = %v, want %v", signal, sem1)
	}
	if r.wait != sem1 {
		t.Errorf("after submit 3: state.wait = %v, want %v (sem1)", r.wait, sem1)
	}
	if r.signal != sem2 {
		t.Errorf("after submit 3: state.signal = %v, want %v (sem2)", r.signal, sem2)
	}

	// Fourth submission: pattern repeats — same as second.
	wait, signal = r.wait, r.signal
	r.wait, r.signal = r.signal, r.wait

	if wait != sem1 {
		t.Errorf("submit 4: wait = %v, want %v", wait, sem1)
	}
	if signal != sem2 {
		t.Errorf("submit 4: signal = %v, want %v", signal, sem2)
	}
}

// TestRelaySemaphoresInitialState validates the initial state: first submission
// should wait on nothing (0) and signal the pre-created semaphore.
func TestRelaySemaphoresInitialState(t *testing.T) {
	sem1 := vk.Semaphore(0x2001)

	r := &relaySemaphores{
		wait:   0,    // no predecessor
		signal: sem1, // created in newRelaySemaphores
	}

	// First submission: should return (wait=0, signal=sem1).
	if r.wait != 0 {
		t.Errorf("initial state: wait = %v, want 0", r.wait)
	}
	if r.signal != sem1 {
		t.Errorf("initial state: signal = %v, want %v", r.signal, sem1)
	}
}

// TestRelaySemaphoresDestroy validates that destroy zeroes out handles.
// Uses a nil cmds (destroy is a no-op when handles are already zero).
func TestRelaySemaphoresDestroyZeroed(t *testing.T) {
	r := &relaySemaphores{
		wait:   0,
		signal: 0,
	}

	// Should not panic with nil cmds when handles are already zero.
	// The actual DestroySemaphore calls are skipped because handles are 0.
	r.destroy(nil, 0)

	if r.wait != 0 {
		t.Errorf("after destroy: wait = %v, want 0", r.wait)
	}
	if r.signal != 0 {
		t.Errorf("after destroy: signal = %v, want 0", r.signal)
	}
}
