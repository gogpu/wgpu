//go:build !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package wgpu_test

import (
	"context"
	"testing"

	"github.com/gogpu/wgpu"
)

// FEAT-WGPU-MAPPING-001 — zero-alloc benchmark gate for Buffer.Map.
//
// The primary blocking path must allocate 0 bytes per iteration after
// warm-up. The async escape-hatch path must also stay zero-alloc when
// driven via sync.Pool (MapPending). If either gate regresses, CI
// should fail.

func setupMapBench(b *testing.B) (*wgpu.Instance, *wgpu.Adapter, *wgpu.Device, *wgpu.Buffer) {
	b.Helper()
	instance, err := wgpu.CreateInstance(nil)
	if err != nil {
		b.Skipf("cannot create instance: %v", err)
	}
	adapter, err := instance.RequestAdapter(nil)
	if err != nil {
		instance.Release()
		b.Skipf("cannot request adapter: %v", err)
	}
	device, err := adapter.RequestDevice(nil)
	if err != nil {
		adapter.Release()
		instance.Release()
		b.Skipf("cannot request device: %v", err)
	}
	if device.Queue() == nil {
		device.Release()
		adapter.Release()
		instance.Release()
		b.Skip("no GPU backend available")
	}
	buf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "bench-map",
		Size:  1024,
		Usage: wgpu.BufferUsageMapRead | wgpu.BufferUsageCopyDst,
	})
	if err != nil {
		device.Release()
		adapter.Release()
		instance.Release()
		b.Fatalf("CreateBuffer: %v", err)
	}
	return instance, adapter, device, buf
}

// BenchmarkBufferMapReadPrimary measures the primary blocking Map path.
// Target: 0 allocs/op after warm-up.
func BenchmarkBufferMapReadPrimary(b *testing.B) {
	instance, adapter, device, buf := setupMapBench(b)
	defer instance.Release()
	defer adapter.Release()
	defer device.Release()
	defer buf.Release()

	const size = 1024
	ctx := context.Background()

	// Warm up the lazy state machine and the sync.Pool entries so the
	// measurement reflects steady-state cost, not first-call init.
	_ = buf.Map(ctx, wgpu.MapModeRead, 0, size)
	rng, _ := buf.MappedRange(0, size)
	_ = rng.Bytes()
	_ = buf.Unmap()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if err := buf.Map(ctx, wgpu.MapModeRead, 0, size); err != nil {
			b.Fatal(err)
		}
		rng, _ := buf.MappedRange(0, size)
		_ = rng.Bytes()
		rng.Release()
		if err := buf.Unmap(); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkBufferMapAsyncEscapeHatch measures the MapAsync + Status path
// that game loops would use. Target: 0 allocs/op after warm-up thanks to
// MapPending's sync.Pool.
func BenchmarkBufferMapAsyncEscapeHatch(b *testing.B) {
	instance, adapter, device, buf := setupMapBench(b)
	defer instance.Release()
	defer adapter.Release()
	defer device.Release()
	defer buf.Release()

	const size = 1024

	// Warm up.
	pending, _ := buf.MapAsync(wgpu.MapModeRead, 0, size)
	device.Poll(wgpu.PollWait)
	_, _ = pending.Status()
	rng, _ := buf.MappedRange(0, size)
	_ = rng.Bytes()
	_ = buf.Unmap()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		pending, err := buf.MapAsync(wgpu.MapModeRead, 0, size)
		if err != nil {
			b.Fatal(err)
		}
		device.Poll(wgpu.PollPoll)
		if ready, werr := pending.Status(); !ready || werr != nil {
			b.Fatalf("Status: ready=%v err=%v", ready, werr)
		}
		pending.Release()
		rng, _ := buf.MappedRange(0, size)
		_ = rng.Bytes()
		rng.Release()
		if err := buf.Unmap(); err != nil {
			b.Fatal(err)
		}
	}
}
