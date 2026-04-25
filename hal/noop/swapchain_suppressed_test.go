//go:build !(js && wasm)

// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

package noop_test

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/noop"
)

// TestSetSwapchainSuppressed_NoopNoPanic verifies that SetSwapchainSuppressed
// does not panic on the noop backend. This is the minimum contract for non-Vulkan
// backends: accept the call silently.
func TestSetSwapchainSuppressed_NoopNoPanic(t *testing.T) {
	api := noop.API{}
	instance, err := api.CreateInstance(nil)
	if err != nil {
		t.Fatalf("CreateInstance failed: %v", err)
	}
	defer instance.Destroy()

	adapters := instance.EnumerateAdapters(nil)
	if len(adapters) == 0 {
		t.Fatal("no adapters")
	}
	openDevice, err := adapters[0].Adapter.Open(0, gputypes.DefaultLimits())
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer openDevice.Device.Destroy()

	queue := openDevice.Queue

	// Should not panic.
	queue.SetSwapchainSuppressed(true)
	queue.SetSwapchainSuppressed(false)
}

// TestSetSwapchainSuppressed_SubmitDuringSuppression verifies that Submit works
// normally while swapchain is suppressed. On noop this is trivially true but
// confirms the interface contract holds end-to-end.
func TestSetSwapchainSuppressed_SubmitDuringSuppression(t *testing.T) {
	api := noop.API{}
	instance, err := api.CreateInstance(nil)
	if err != nil {
		t.Fatalf("CreateInstance failed: %v", err)
	}
	defer instance.Destroy()

	adapters := instance.EnumerateAdapters(nil)
	openDevice, err := adapters[0].Adapter.Open(0, gputypes.DefaultLimits())
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer openDevice.Device.Destroy()

	queue := openDevice.Queue
	device := openDevice.Device

	// Create a command buffer.
	encoder, err := device.CreateCommandEncoder(&hal.CommandEncoderDescriptor{Label: "test"})
	if err != nil {
		t.Fatalf("CreateCommandEncoder failed: %v", err)
	}
	if err := encoder.BeginEncoding("test"); err != nil {
		t.Fatalf("BeginEncoding failed: %v", err)
	}
	cmdBuffer, err := encoder.EndEncoding()
	if err != nil {
		t.Fatalf("EndEncoding failed: %v", err)
	}

	// Suppress -> Submit -> Unsuppress -> Submit.
	queue.SetSwapchainSuppressed(true)
	idx1, err := queue.Submit([]hal.CommandBuffer{cmdBuffer})
	if err != nil {
		t.Fatalf("Submit during suppression failed: %v", err)
	}
	queue.SetSwapchainSuppressed(false)

	encoder2, _ := device.CreateCommandEncoder(&hal.CommandEncoderDescriptor{Label: "test2"})
	_ = encoder2.BeginEncoding("test2")
	cmdBuffer2, _ := encoder2.EndEncoding()

	idx2, err := queue.Submit([]hal.CommandBuffer{cmdBuffer2})
	if err != nil {
		t.Fatalf("Submit after unsuppression failed: %v", err)
	}

	if idx2 <= idx1 {
		t.Errorf("submission indices should be monotonically increasing: %d <= %d", idx2, idx1)
	}
}

// TestSetSwapchainSuppressed_Idempotent verifies that calling
// SetSwapchainSuppressed(true) twice or SetSwapchainSuppressed(false) twice
// does not panic or corrupt state.
func TestSetSwapchainSuppressed_Idempotent(t *testing.T) {
	api := noop.API{}
	instance, err := api.CreateInstance(nil)
	if err != nil {
		t.Fatalf("CreateInstance failed: %v", err)
	}
	defer instance.Destroy()

	adapters := instance.EnumerateAdapters(nil)
	openDevice, err := adapters[0].Adapter.Open(0, gputypes.DefaultLimits())
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer openDevice.Device.Destroy()

	queue := openDevice.Queue

	// Double-true, double-false: must not panic.
	queue.SetSwapchainSuppressed(true)
	queue.SetSwapchainSuppressed(true)
	queue.SetSwapchainSuppressed(false)
	queue.SetSwapchainSuppressed(false)

	// Alternating: must not panic.
	queue.SetSwapchainSuppressed(true)
	queue.SetSwapchainSuppressed(false)
	queue.SetSwapchainSuppressed(true)
	queue.SetSwapchainSuppressed(false)
}

// BenchmarkSetSwapchainSuppressed verifies zero allocations for the
// suppress/unsuppress cycle. This is called twice per frame in the
// RepaintBoundary path (suppress before offscreen, unsuppress after).
func BenchmarkSetSwapchainSuppressed(b *testing.B) {
	b.ReportAllocs()

	api := noop.API{}
	instance, _ := api.CreateInstance(nil)
	defer instance.Destroy()

	adapters := instance.EnumerateAdapters(nil)
	openDevice, _ := adapters[0].Adapter.Open(0, gputypes.DefaultLimits())
	defer openDevice.Device.Destroy()

	queue := openDevice.Queue

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		queue.SetSwapchainSuppressed(true)
		queue.SetSwapchainSuppressed(false)
	}
}
