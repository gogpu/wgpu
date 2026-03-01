// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

package noop_test

import (
	"runtime"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/noop"
)

// benchResult prevents the compiler from optimizing away benchmark results.
var benchResult any

// setupNoopDevice creates a noop device+queue for benchmarks.
// The cleanup function must be deferred.
func setupNoopDevice(b *testing.B) (hal.Device, hal.Queue, func()) {
	b.Helper()

	api := noop.API{}
	instance, err := api.CreateInstance(nil)
	if err != nil {
		b.Fatalf("CreateInstance failed: %v", err)
	}

	adapters := instance.EnumerateAdapters(nil)
	openDevice, err := adapters[0].Adapter.Open(0, gputypes.DefaultLimits())
	if err != nil {
		instance.Destroy()
		b.Fatalf("Open failed: %v", err)
	}

	cleanup := func() {
		openDevice.Device.Destroy()
		instance.Destroy()
	}

	return openDevice.Device, openDevice.Queue, cleanup
}

// BenchmarkNoopSubmitEmpty measures the CPU overhead of submitting zero command buffers.
// Expected: ~0 allocs, sub-microsecond.
func BenchmarkNoopSubmitEmpty(b *testing.B) {
	b.ReportAllocs()
	_, queue, cleanup := setupNoopDevice(b)
	defer cleanup()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := queue.Submit(nil, nil, 0)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkNoopSubmitSingle measures the overhead of submitting one command buffer.
// Expected: ~0 allocs (noop doesn't allocate in Submit).
func BenchmarkNoopSubmitSingle(b *testing.B) {
	b.ReportAllocs()
	device, queue, cleanup := setupNoopDevice(b)
	defer cleanup()

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
	}
	runtime.KeepAlive(cmdBuffers)
}

// BenchmarkNoopSubmitWithFence measures submit + fence signaling overhead.
func BenchmarkNoopSubmitWithFence(b *testing.B) {
	b.ReportAllocs()
	device, queue, cleanup := setupNoopDevice(b)
	defer cleanup()

	encoder, _ := device.CreateCommandEncoder(&hal.CommandEncoderDescriptor{Label: "bench"})
	_ = encoder.BeginEncoding("bench")
	cmdBuffer, _ := encoder.EndEncoding()
	cmdBuffers := []hal.CommandBuffer{cmdBuffer}
	fence, _ := device.CreateFence()
	defer device.DestroyFence(fence)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := queue.Submit(cmdBuffers, fence, uint64(i+1))
		if err != nil {
			b.Fatal(err)
		}
	}
	runtime.KeepAlive(cmdBuffers)
}

// BenchmarkNoopBeginEndEncoding measures the full command encoder cycle.
// This is called every frame in a real application.
func BenchmarkNoopBeginEndEncoding(b *testing.B) {
	b.ReportAllocs()
	device, _, cleanup := setupNoopDevice(b)
	defer cleanup()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		encoder, _ := device.CreateCommandEncoder(&hal.CommandEncoderDescriptor{Label: "bench"})
		_ = encoder.BeginEncoding("bench")
		cb, _ := encoder.EndEncoding()
		benchResult = cb
	}
}

// BenchmarkNoopCreateDestroyBuffer measures buffer create/destroy cycle.
func BenchmarkNoopCreateDestroyBuffer(b *testing.B) {
	b.ReportAllocs()
	device, _, cleanup := setupNoopDevice(b)
	defer cleanup()

	desc := &hal.BufferDescriptor{
		Label: "bench-buffer",
		Size:  4096,
		Usage: gputypes.BufferUsageVertex | gputypes.BufferUsageCopyDst,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf, _ := device.CreateBuffer(desc)
		device.DestroyBuffer(buf)
	}
}

// BenchmarkNoopCreateDestroyBufferMapped measures mapped buffer creation overhead.
// Mapped buffers allocate backing memory, so this measures allocation cost.
func BenchmarkNoopCreateDestroyBufferMapped(b *testing.B) {
	b.ReportAllocs()
	device, _, cleanup := setupNoopDevice(b)
	defer cleanup()

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
			desc := &hal.BufferDescriptor{
				Label:            "bench-mapped",
				Size:             s.size,
				Usage:            gputypes.BufferUsageStorage,
				MappedAtCreation: true,
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				buf, _ := device.CreateBuffer(desc)
				device.DestroyBuffer(buf)
			}
		})
	}
}

// BenchmarkNoopCreateDestroyTexture measures texture create/destroy cycle.
func BenchmarkNoopCreateDestroyTexture(b *testing.B) {
	b.ReportAllocs()
	device, _, cleanup := setupNoopDevice(b)
	defer cleanup()

	desc := &hal.TextureDescriptor{
		Label:         "bench-tex",
		Size:          hal.Extent3D{Width: 512, Height: 512, DepthOrArrayLayers: 1},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     gputypes.TextureDimension2D,
		Format:        gputypes.TextureFormatRGBA8Unorm,
		Usage:         gputypes.TextureUsageTextureBinding | gputypes.TextureUsageRenderAttachment,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tex, _ := device.CreateTexture(desc)
		device.DestroyTexture(tex)
	}
}

// BenchmarkNoopCreateDestroyBindGroup measures bind group creation overhead.
func BenchmarkNoopCreateDestroyBindGroup(b *testing.B) {
	b.ReportAllocs()
	device, _, cleanup := setupNoopDevice(b)
	defer cleanup()

	bgLayout, _ := device.CreateBindGroupLayout(&hal.BindGroupLayoutDescriptor{
		Label: "bench-bgl",
		Entries: []gputypes.BindGroupLayoutEntry{
			{
				Binding:    0,
				Visibility: gputypes.ShaderStageVertex | gputypes.ShaderStageFragment,
				Buffer:     &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeUniform},
			},
		},
	})
	defer device.DestroyBindGroupLayout(bgLayout)

	desc := &hal.BindGroupDescriptor{
		Label:  "bench-bg",
		Layout: bgLayout,
		Entries: []gputypes.BindGroupEntry{
			{
				Binding:  0,
				Resource: gputypes.BufferBinding{Buffer: 0, Offset: 0, Size: 256},
			},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bg, _ := device.CreateBindGroup(desc)
		device.DestroyBindGroup(bg)
	}
}

// BenchmarkNoopRenderPassBeginEnd measures render pass open/close overhead.
func BenchmarkNoopRenderPassBeginEnd(b *testing.B) {
	b.ReportAllocs()
	device, _, cleanup := setupNoopDevice(b)
	defer cleanup()

	texture, _ := device.CreateTexture(&hal.TextureDescriptor{
		Size:          hal.Extent3D{Width: 800, Height: 600, DepthOrArrayLayers: 1},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     gputypes.TextureDimension2D,
		Format:        gputypes.TextureFormatRGBA8Unorm,
		Usage:         gputypes.TextureUsageRenderAttachment,
	})
	defer device.DestroyTexture(texture)

	view, _ := device.CreateTextureView(texture, &hal.TextureViewDescriptor{})
	defer device.DestroyTextureView(view)

	desc := &hal.RenderPassDescriptor{
		ColorAttachments: []hal.RenderPassColorAttachment{
			{
				View:       view,
				LoadOp:     gputypes.LoadOpClear,
				StoreOp:    gputypes.StoreOpStore,
				ClearValue: gputypes.Color{R: 0, G: 0, B: 0, A: 1},
			},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		encoder, _ := device.CreateCommandEncoder(&hal.CommandEncoderDescriptor{})
		_ = encoder.BeginEncoding("bench")
		rp := encoder.BeginRenderPass(desc)
		rp.End()
		cb, _ := encoder.EndEncoding()
		benchResult = cb
	}
}

// BenchmarkNoopComputePassBeginEnd measures compute pass open/close overhead.
func BenchmarkNoopComputePassBeginEnd(b *testing.B) {
	b.ReportAllocs()
	device, _, cleanup := setupNoopDevice(b)
	defer cleanup()

	desc := &hal.ComputePassDescriptor{Label: "bench-compute"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		encoder, _ := device.CreateCommandEncoder(&hal.CommandEncoderDescriptor{})
		_ = encoder.BeginEncoding("bench")
		cp := encoder.BeginComputePass(desc)
		cp.Dispatch(1, 1, 1)
		cp.End()
		cb, _ := encoder.EndEncoding()
		benchResult = cb
	}
}

// BenchmarkNoopFullFrame simulates a realistic frame:
// create encoder -> begin encoding -> begin render pass -> draw calls -> end pass -> end encoding -> submit.
func BenchmarkNoopFullFrame(b *testing.B) {
	b.ReportAllocs()
	device, queue, cleanup := setupNoopDevice(b)
	defer cleanup()

	texture, _ := device.CreateTexture(&hal.TextureDescriptor{
		Size:          hal.Extent3D{Width: 1920, Height: 1080, DepthOrArrayLayers: 1},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     gputypes.TextureDimension2D,
		Format:        gputypes.TextureFormatBGRA8Unorm,
		Usage:         gputypes.TextureUsageRenderAttachment,
	})
	defer device.DestroyTexture(texture)

	view, _ := device.CreateTextureView(texture, &hal.TextureViewDescriptor{})
	defer device.DestroyTextureView(view)

	buffer, _ := device.CreateBuffer(&hal.BufferDescriptor{
		Size:  4096,
		Usage: gputypes.BufferUsageVertex,
	})
	defer device.DestroyBuffer(buffer)

	layout, _ := device.CreatePipelineLayout(&hal.PipelineLayoutDescriptor{})
	defer device.DestroyPipelineLayout(layout)

	module, _ := device.CreateShaderModule(&hal.ShaderModuleDescriptor{
		Source: hal.ShaderSource{WGSL: "@vertex fn vs() {}"},
	})
	defer device.DestroyShaderModule(module)

	pipeline, _ := device.CreateRenderPipeline(&hal.RenderPipelineDescriptor{
		Layout: layout,
		Vertex: hal.VertexState{Module: module, EntryPoint: "vs"},
		Primitive: gputypes.PrimitiveState{
			Topology: gputypes.PrimitiveTopologyTriangleList,
		},
		Multisample: gputypes.MultisampleState{Count: 1, Mask: 0xFFFFFFFF},
	})
	defer device.DestroyRenderPipeline(pipeline)

	fence, _ := device.CreateFence()
	defer device.DestroyFence(fence)

	rpDesc := &hal.RenderPassDescriptor{
		ColorAttachments: []hal.RenderPassColorAttachment{
			{
				View:       view,
				LoadOp:     gputypes.LoadOpClear,
				StoreOp:    gputypes.StoreOpStore,
				ClearValue: gputypes.Color{R: 0.1, G: 0.2, B: 0.3, A: 1.0},
			},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Encode
		encoder, _ := device.CreateCommandEncoder(&hal.CommandEncoderDescriptor{})
		_ = encoder.BeginEncoding("frame")

		rp := encoder.BeginRenderPass(rpDesc)
		rp.SetPipeline(pipeline)
		rp.SetVertexBuffer(0, buffer, 0)
		rp.Draw(3, 1, 0, 0)
		rp.End()

		cmdBuffer, _ := encoder.EndEncoding()

		// Submit
		_ = queue.Submit([]hal.CommandBuffer{cmdBuffer}, fence, uint64(i+1))
	}
}

// BenchmarkNoopCommandRecording measures the overhead of many draw calls in one pass.
func BenchmarkNoopCommandRecording(b *testing.B) {
	drawCounts := []struct {
		name  string
		draws int
	}{
		{"1_draw", 1},
		{"10_draws", 10},
		{"100_draws", 100},
		{"1000_draws", 1000},
	}

	for _, dc := range drawCounts {
		b.Run(dc.name, func(b *testing.B) {
			b.ReportAllocs()
			device, _, cleanup := setupNoopDevice(b)
			defer cleanup()

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				encoder, _ := device.CreateCommandEncoder(&hal.CommandEncoderDescriptor{})
				_ = encoder.BeginEncoding("bench")
				rp := encoder.BeginRenderPass(&hal.RenderPassDescriptor{
					ColorAttachments: []hal.RenderPassColorAttachment{{}},
				})
				for d := 0; d < dc.draws; d++ {
					rp.Draw(3, 1, 0, 0)
				}
				rp.End()
				cb, _ := encoder.EndEncoding()
				benchResult = cb
			}
		})
	}
}

// BenchmarkNoopWriteBuffer measures WriteBuffer overhead for various sizes.
func BenchmarkNoopWriteBuffer(b *testing.B) {
	sizes := []struct {
		name string
		size int
	}{
		{"64B", 64},
		{"1KB", 1024},
		{"64KB", 65536},
		{"1MB", 1 << 20},
	}

	for _, s := range sizes {
		b.Run(s.name, func(b *testing.B) {
			b.ReportAllocs()
			device, queue, cleanup := setupNoopDevice(b)
			defer cleanup()

			buffer, _ := device.CreateBuffer(&hal.BufferDescriptor{
				Size:             uint64(s.size),
				Usage:            gputypes.BufferUsageCopyDst,
				MappedAtCreation: true,
			})
			defer device.DestroyBuffer(buffer)

			data := make([]byte, s.size)

			b.ResetTimer()
			b.SetBytes(int64(s.size))
			for i := 0; i < b.N; i++ {
				if err := queue.WriteBuffer(buffer, 0, data); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkNoopReadBuffer measures ReadBuffer overhead for various sizes.
func BenchmarkNoopReadBuffer(b *testing.B) {
	sizes := []struct {
		name string
		size int
	}{
		{"64B", 64},
		{"1KB", 1024},
		{"64KB", 65536},
		{"1MB", 1 << 20},
	}

	for _, s := range sizes {
		b.Run(s.name, func(b *testing.B) {
			b.ReportAllocs()
			device, queue, cleanup := setupNoopDevice(b)
			defer cleanup()

			buffer, _ := device.CreateBuffer(&hal.BufferDescriptor{
				Size:             uint64(s.size),
				Usage:            gputypes.BufferUsageStorage,
				MappedAtCreation: true,
			})
			defer device.DestroyBuffer(buffer)

			data := make([]byte, s.size)

			b.ResetTimer()
			b.SetBytes(int64(s.size))
			for i := 0; i < b.N; i++ {
				_ = queue.ReadBuffer(buffer, 0, data)
			}
		})
	}
}

// BenchmarkNoopPresent measures present overhead (no-op, baseline).
func BenchmarkNoopPresent(b *testing.B) {
	b.ReportAllocs()
	_, queue, cleanup := setupNoopDevice(b)
	defer cleanup()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := queue.Present(nil, nil)
		if err != nil {
			b.Fatal(err)
		}
	}
}
