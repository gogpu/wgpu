// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

package hal_test

import (
	"runtime"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/noop"
)

// benchHALSink prevents the compiler from optimizing away benchmark results.
var benchHALSink any

// setupHALDevice creates a noop device+queue through the HAL interface.
// Used to measure interface dispatch overhead.
func setupHALDevice(b *testing.B) (hal.Device, hal.Queue, func()) {
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

// BenchmarkHALSubmitOverhead measures the overhead of calling Submit through
// the hal.Queue interface vs a concrete type. The noop backend does minimal work,
// so this primarily measures interface dispatch + type assertion overhead.
func BenchmarkHALSubmitOverhead(b *testing.B) {
	b.ReportAllocs()
	device, queue, cleanup := setupHALDevice(b)
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

// BenchmarkHALCommandEncoding measures the cost of CreateCommandEncoder +
// BeginEncoding + EndEncoding through the HAL interface.
func BenchmarkHALCommandEncoding(b *testing.B) {
	b.ReportAllocs()
	device, _, cleanup := setupHALDevice(b)
	defer cleanup()

	desc := &hal.CommandEncoderDescriptor{Label: "bench"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		encoder, _ := device.CreateCommandEncoder(desc)
		_ = encoder.BeginEncoding("bench")
		cb, _ := encoder.EndEncoding()
		benchHALSink = cb
	}
}

// BenchmarkHALBufferCreation measures buffer creation through the HAL interface.
func BenchmarkHALBufferCreation(b *testing.B) {
	sizes := []struct {
		name string
		size uint64
	}{
		{"256B", 256},
		{"4KB", 4096},
		{"1MB", 1 << 20},
	}

	for _, s := range sizes {
		b.Run(s.name, func(b *testing.B) {
			b.ReportAllocs()
			device, _, cleanup := setupHALDevice(b)
			defer cleanup()

			desc := &hal.BufferDescriptor{
				Label: "bench-buffer",
				Size:  s.size,
				Usage: gputypes.BufferUsageVertex | gputypes.BufferUsageCopyDst,
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				buf, _ := device.CreateBuffer(desc)
				device.DestroyBuffer(buf)
			}
		})
	}
}

// BenchmarkHALTextureCreation measures texture creation through the HAL interface.
func BenchmarkHALTextureCreation(b *testing.B) {
	b.ReportAllocs()
	device, _, cleanup := setupHALDevice(b)
	defer cleanup()

	desc := &hal.TextureDescriptor{
		Label:         "bench-tex",
		Size:          hal.Extent3D{Width: 512, Height: 512, DepthOrArrayLayers: 1},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     gputypes.TextureDimension2D,
		Format:        gputypes.TextureFormatRGBA8Unorm,
		Usage:         gputypes.TextureUsageTextureBinding,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tex, _ := device.CreateTexture(desc)
		device.DestroyTexture(tex)
	}
}

// BenchmarkHALRenderPassEncoding measures the full render pass recording path
// through the HAL interface: encode -> render pass -> draw -> end -> finish.
func BenchmarkHALRenderPassEncoding(b *testing.B) {
	b.ReportAllocs()
	device, _, cleanup := setupHALDevice(b)
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

	rpDesc := &hal.RenderPassDescriptor{
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
		rp := encoder.BeginRenderPass(rpDesc)
		rp.Draw(3, 1, 0, 0)
		rp.End()
		cb, _ := encoder.EndEncoding()
		benchHALSink = cb
	}
}

// BenchmarkHALComputePassEncoding measures compute pass recording through
// the HAL interface.
func BenchmarkHALComputePassEncoding(b *testing.B) {
	b.ReportAllocs()
	device, _, cleanup := setupHALDevice(b)
	defer cleanup()

	cpDesc := &hal.ComputePassDescriptor{Label: "bench"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		encoder, _ := device.CreateCommandEncoder(&hal.CommandEncoderDescriptor{})
		_ = encoder.BeginEncoding("bench")
		cp := encoder.BeginComputePass(cpDesc)
		cp.Dispatch(1, 1, 1)
		cp.End()
		cb, _ := encoder.EndEncoding()
		benchHALSink = cb
	}
}

// BenchmarkHALFullFrameSimulation simulates a typical frame through the HAL interface:
// create encoder -> begin -> render pass with draws -> end -> submit with fence.
func BenchmarkHALFullFrameSimulation(b *testing.B) {
	b.ReportAllocs()
	device, queue, cleanup := setupHALDevice(b)
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
		encoder, _ := device.CreateCommandEncoder(&hal.CommandEncoderDescriptor{})
		_ = encoder.BeginEncoding("frame")

		rp := encoder.BeginRenderPass(rpDesc)
		rp.Draw(3, 1, 0, 0)
		rp.Draw(6, 1, 0, 0)
		rp.Draw(36, 1, 0, 0)
		rp.End()

		cb, _ := encoder.EndEncoding()
		_ = queue.Submit([]hal.CommandBuffer{cb}, fence, uint64(i+1))
	}
}

// BenchmarkHALWriteBuffer measures WriteBuffer throughput through the HAL interface.
func BenchmarkHALWriteBuffer(b *testing.B) {
	sizes := []struct {
		name string
		size int
	}{
		{"256B", 256},
		{"4KB", 4096},
		{"64KB", 65536},
	}

	for _, s := range sizes {
		b.Run(s.name, func(b *testing.B) {
			b.ReportAllocs()
			device, queue, cleanup := setupHALDevice(b)
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
				queue.WriteBuffer(buffer, 0, data)
			}
		})
	}
}
