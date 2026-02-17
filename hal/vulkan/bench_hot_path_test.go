// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows

package vulkan

import (
	"runtime"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/vulkan/vk"
)

// benchSink prevents the compiler from optimizing away benchmark results.
var benchSink any

// tryCreateVulkanDeviceForBench is a benchmark variant of tryCreateVulkanDevice.
// Skips the benchmark if Vulkan is not available.
func tryCreateVulkanDeviceForBench(b *testing.B) (hal.Device, hal.Queue, func()) {
	b.Helper()
	return tryCreateVulkanDeviceB(b)
}

// tryCreateVulkanDeviceB creates a Vulkan device for benchmarks.
// Skips if Vulkan is not available (e.g., headless CI).
func tryCreateVulkanDeviceB(b *testing.B) (hal.Device, hal.Queue, func()) {
	b.Helper()

	if err := vk.Init(); err != nil {
		b.Skipf("Vulkan not available: %v", err)
		return nil, nil, nil
	}

	backend := Backend{}
	instance, err := backend.CreateInstance(&hal.InstanceDescriptor{
		Backends: gputypes.BackendsVulkan,
	})
	if err != nil {
		b.Skipf("Vulkan instance creation failed: %v", err)
		return nil, nil, nil
	}

	adapters := instance.EnumerateAdapters(nil)
	if len(adapters) == 0 {
		instance.Destroy()
		b.Skipf("no Vulkan adapters found")
		return nil, nil, nil
	}

	openDev, err := adapters[0].Adapter.Open(0, adapters[0].Capabilities.Limits)
	if err != nil {
		instance.Destroy()
		b.Skipf("failed to open Vulkan device: %v", err)
		return nil, nil, nil
	}

	cleanup := func() {
		_ = openDev.Device.WaitIdle()
		openDev.Device.Destroy()
		instance.Destroy()
	}

	return openDev.Device, openDev.Queue, cleanup
}

// BenchmarkVulkanSubmitEmpty measures the overhead of Submit with an empty command buffer slice.
// Expected: This tests the mutex lock/unlock path only.
func BenchmarkVulkanSubmitEmpty(b *testing.B) {
	b.ReportAllocs()
	_, queue, cleanup := tryCreateVulkanDeviceForBench(b)
	if queue == nil {
		return
	}
	defer cleanup()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := queue.Submit(nil, nil, 0)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkVulkanBeginEndEncoding measures a full encode cycle:
// CreateCommandEncoder -> BeginEncoding -> EndEncoding.
// This is the per-frame minimum cost for recording any GPU work.
func BenchmarkVulkanBeginEndEncoding(b *testing.B) {
	b.ReportAllocs()
	device, _, cleanup := tryCreateVulkanDeviceForBench(b)
	if device == nil {
		return
	}
	defer cleanup()

	desc := &hal.CommandEncoderDescriptor{Label: "bench-encoder"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		encoder, err := device.CreateCommandEncoder(desc)
		if err != nil {
			b.Fatal(err)
		}
		if err := encoder.BeginEncoding("bench"); err != nil {
			b.Fatal(err)
		}
		cb, err := encoder.EndEncoding()
		if err != nil {
			b.Fatal(err)
		}
		benchSink = cb
	}
}

// BenchmarkVulkanSubmitSingle measures Submit with a single recorded command buffer.
// This is the most common path: one encoder per frame.
func BenchmarkVulkanSubmitSingle(b *testing.B) {
	b.ReportAllocs()
	device, queue, cleanup := tryCreateVulkanDeviceForBench(b)
	if device == nil {
		return
	}
	defer cleanup()

	// Pre-record a command buffer
	encoder, _ := device.CreateCommandEncoder(&hal.CommandEncoderDescriptor{Label: "bench"})
	_ = encoder.BeginEncoding("bench")
	cmdBuffer, _ := encoder.EndEncoding()
	cmdBuffers := []hal.CommandBuffer{cmdBuffer}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := queue.Submit(cmdBuffers, nil, 0)
		if err != nil {
			b.Fatal(err)
		}
		// Wait for GPU to finish before resubmitting
		_ = device.WaitIdle()
	}
	runtime.KeepAlive(cmdBuffers)
}

// BenchmarkVulkanComputePassBeginEnd measures compute pass open/close overhead.
// Note: Does not call Dispatch because that requires a bound pipeline.
// This measures the pure pass begin/end cost including memory barriers.
func BenchmarkVulkanComputePassBeginEnd(b *testing.B) {
	b.ReportAllocs()
	device, _, cleanup := tryCreateVulkanDeviceForBench(b)
	if device == nil {
		return
	}
	defer cleanup()

	desc := &hal.ComputePassDescriptor{Label: "bench-compute"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		encoder, _ := device.CreateCommandEncoder(&hal.CommandEncoderDescriptor{Label: "bench"})
		_ = encoder.BeginEncoding("bench")
		cp := encoder.BeginComputePass(desc)
		cp.End()
		cb, _ := encoder.EndEncoding()
		benchSink = cb
	}
}

// BenchmarkVulkanEncodeSubmitCycle measures the full encode -> submit cycle
// that happens every frame, without render pass overhead.
func BenchmarkVulkanEncodeSubmitCycle(b *testing.B) {
	b.ReportAllocs()
	device, queue, cleanup := tryCreateVulkanDeviceForBench(b)
	if device == nil {
		return
	}
	defer cleanup()

	fence, _ := device.CreateFence()
	defer device.DestroyFence(fence)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		encoder, _ := device.CreateCommandEncoder(&hal.CommandEncoderDescriptor{Label: "bench"})
		_ = encoder.BeginEncoding("frame")
		cb, _ := encoder.EndEncoding()

		_ = queue.Submit([]hal.CommandBuffer{cb}, nil, 0)
		_ = device.WaitIdle()
	}
}

// BenchmarkVulkanSubmitMultiple measures Submit with multiple command buffers.
// Tests the `make([]vk.CommandBuffer, N)` allocation path.
func BenchmarkVulkanSubmitMultiple(b *testing.B) {
	counts := []struct {
		name  string
		count int
	}{
		{"1_cb", 1},
		{"2_cb", 2},
		{"4_cb", 4},
		{"8_cb", 8},
	}

	for _, tc := range counts {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			device, queue, cleanup := tryCreateVulkanDeviceForBench(b)
			if device == nil {
				return
			}
			defer cleanup()

			// Pre-record command buffers
			cmdBuffers := make([]hal.CommandBuffer, tc.count)
			for j := 0; j < tc.count; j++ {
				encoder, _ := device.CreateCommandEncoder(&hal.CommandEncoderDescriptor{Label: "bench"})
				_ = encoder.BeginEncoding("bench")
				cb, _ := encoder.EndEncoding()
				cmdBuffers[j] = cb
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = queue.Submit(cmdBuffers, nil, 0)
				_ = device.WaitIdle()
			}
		})
	}
}

// BenchmarkVulkanCommandRecording measures the overhead of recording multiple
// compute passes in a single command buffer, testing command buffer recording throughput.
// Note: Dispatch is not called because it requires a bound pipeline. This measures
// the pure pass begin/end recording cost including memory barriers.
func BenchmarkVulkanCommandRecording(b *testing.B) {
	passCounts := []struct {
		name   string
		passes int
	}{
		{"1_pass", 1},
		{"4_passes", 4},
		{"16_passes", 16},
	}

	for _, pc := range passCounts {
		b.Run(pc.name, func(b *testing.B) {
			b.ReportAllocs()
			device, _, cleanup := tryCreateVulkanDeviceForBench(b)
			if device == nil {
				return
			}
			defer cleanup()

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				encoder, _ := device.CreateCommandEncoder(&hal.CommandEncoderDescriptor{Label: "bench"})
				_ = encoder.BeginEncoding("bench")
				for p := 0; p < pc.passes; p++ {
					cp := encoder.BeginComputePass(&hal.ComputePassDescriptor{Label: "bench"})
					cp.End()
				}
				cb, _ := encoder.EndEncoding()
				benchSink = cb
			}
		})
	}
}

// BenchmarkVulkanCreateDestroyBuffer measures Vulkan buffer create/destroy overhead.
// This includes real Vulkan memory allocation.
func BenchmarkVulkanCreateDestroyBuffer(b *testing.B) {
	sizes := []struct {
		name string
		size uint64
	}{
		{"256B", 256},
		{"4KB", 4096},
		{"64KB", 65536},
		{"1MB", 1 << 20},
	}

	for _, s := range sizes {
		b.Run(s.name, func(b *testing.B) {
			b.ReportAllocs()
			device, _, cleanup := tryCreateVulkanDeviceForBench(b)
			if device == nil {
				return
			}
			defer cleanup()

			desc := &hal.BufferDescriptor{
				Label: "bench-buffer",
				Size:  s.size,
				Usage: gputypes.BufferUsageVertex | gputypes.BufferUsageCopyDst,
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				buf, err := device.CreateBuffer(desc)
				if err != nil {
					b.Fatal(err)
				}
				device.DestroyBuffer(buf)
			}
		})
	}
}

// BenchmarkVulkanCreateDestroyFence measures fence lifecycle overhead.
func BenchmarkVulkanCreateDestroyFence(b *testing.B) {
	b.ReportAllocs()
	device, _, cleanup := tryCreateVulkanDeviceForBench(b)
	if device == nil {
		return
	}
	defer cleanup()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fence, err := device.CreateFence()
		if err != nil {
			b.Fatal(err)
		}
		device.DestroyFence(fence)
	}
}
