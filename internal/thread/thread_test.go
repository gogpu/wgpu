// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

package thread

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestThread_CallVoid(t *testing.T) {
	th := New()
	defer th.Stop()

	var called atomic.Bool
	th.CallVoid(func() {
		called.Store(true)
	})

	if !called.Load() {
		t.Error("CallVoid did not execute function")
	}
}

func TestThread_Call(t *testing.T) {
	th := New()
	defer th.Stop()

	result := th.Call(func() any {
		return 42
	})

	if result != 42 {
		t.Errorf("Call returned %v, want 42", result)
	}
}

func TestThread_CallAsync(t *testing.T) {
	th := New()
	defer th.Stop()

	var called atomic.Bool
	th.CallAsync(func() {
		called.Store(true)
	})

	// Wait for async call to complete
	time.Sleep(10 * time.Millisecond)

	if !called.Load() {
		t.Error("CallAsync did not execute function")
	}
}

func TestThread_Stop(t *testing.T) {
	th := New()

	if !th.IsRunning() {
		t.Error("Thread should be running after New()")
	}

	th.Stop()

	if th.IsRunning() {
		t.Error("Thread should not be running after Stop()")
	}

	// Calling methods on stopped thread should not panic
	th.CallVoid(func() {})
	th.Call(func() any { return nil })
	th.CallAsync(func() {})
}

func TestRenderLoop_RequestResize(t *testing.T) {
	rl := NewRenderLoop()
	defer rl.Stop()

	// No pending resize initially
	if rl.HasPendingResize() {
		t.Error("Should not have pending resize initially")
	}

	// Request resize
	rl.RequestResize(800, 600)

	if !rl.HasPendingResize() {
		t.Error("Should have pending resize after RequestResize")
	}

	// Consume resize
	w, h, ok := rl.ConsumePendingResize()
	if !ok {
		t.Error("ConsumePendingResize should return true")
	}
	if w != 800 || h != 600 {
		t.Errorf("ConsumePendingResize returned %dx%d, want 800x600", w, h)
	}

	// Resize should be consumed
	if rl.HasPendingResize() {
		t.Error("Should not have pending resize after consuming")
	}
}

func TestRenderLoop_PauseRendering(t *testing.T) {
	rl := NewRenderLoop()
	defer rl.Stop()

	if rl.IsRenderingPaused() {
		t.Error("Rendering should not be paused initially")
	}

	rl.PauseRendering()
	if !rl.IsRenderingPaused() {
		t.Error("Rendering should be paused after PauseRendering")
	}

	rl.ResumeRendering()
	if rl.IsRenderingPaused() {
		t.Error("Rendering should not be paused after ResumeRendering")
	}
}

func TestRenderLoop_RunOnRenderThread(t *testing.T) {
	rl := NewRenderLoop()
	defer rl.Stop()

	result := rl.RunOnRenderThread(func() any {
		return "hello"
	})

	if result != "hello" {
		t.Errorf("RunOnRenderThread returned %v, want 'hello'", result)
	}
}
