//go:build !rust && !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package wgpu_test

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
	"github.com/gogpu/wgpu/wgputest"
)

// The write-path allocation guards of ADR-070 §15.7, recorded before the queue
// Move (30aec3c, noop backend, -benchtime=2000x -count=3):
//
//	WriteBuffer:  0 allocs/op, 0 B/op
//	WriteTexture: 3 allocs/op, 80 B/op
//
// The move to internal/queue must not add an allocation to the public boundary
// conversion (the internal descriptor mirrors are small structs; they must not
// escape to the heap). Re-run with the same benchtime and compare: any delta is
// a regression to fix, not to explain away. The Submit protocol lives in
// queue_bench_test.go; the destination's own zero-value and allocation checks
// are in internal/queue/zero_value_test.go.

// BenchmarkWriteBufferAllocs measures one public Queue.WriteBuffer call into a
// CopyDst buffer on the noop backend.
func BenchmarkWriteBufferAllocs(b *testing.B) {
	b.ReportAllocs()

	device, _, cleanup, err := wgputest.NewNoopDevice()
	if err != nil {
		b.Fatalf("wgputest.NewNoopDevice: %v", err)
	}
	defer cleanup()

	buf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "bench-write-buffer",
		Size:  64,
		Usage: wgpu.BufferUsageVertex | wgpu.BufferUsageCopyDst,
	})
	if err != nil {
		b.Fatalf("CreateBuffer: %v", err)
	}
	defer buf.Release()

	data := make([]byte, 16)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := device.Queue().WriteBuffer(buf, 0, data); err != nil {
			b.Fatalf("WriteBuffer: %v", err)
		}
	}
}

// BenchmarkWriteTextureAllocs measures one public Queue.WriteTexture call: the
// boundary conversion (public descriptor to the internal mirror), the
// validation and the pending-writes staging path on the noop backend.
func BenchmarkWriteTextureAllocs(b *testing.B) {
	b.ReportAllocs()

	device, _, cleanup, err := wgputest.NewNoopDevice()
	if err != nil {
		b.Fatalf("wgputest.NewNoopDevice: %v", err)
	}
	defer cleanup()

	tex, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Label:         "bench-write-texture",
		Size:          wgpu.Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 1},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     wgpu.TextureDimension2D,
		Format:        gputypes.TextureFormatRGBA8Unorm,
		Usage:         wgpu.TextureUsageCopyDst,
	})
	if err != nil {
		b.Fatalf("CreateTexture: %v", err)
	}
	defer tex.Release()

	dst := &wgpu.ImageCopyTexture{Texture: tex}
	layout := &wgpu.ImageDataLayout{BytesPerRow: 16, RowsPerImage: 4}
	size := &wgpu.Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 1}
	data := make([]byte, 64)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := device.Queue().WriteTexture(dst, data, layout, size); err != nil {
			b.Fatalf("WriteTexture: %v", err)
		}
	}
}
