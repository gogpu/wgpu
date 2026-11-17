//go:build !rust && !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package wgpu_test

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
	"github.com/gogpu/wgpu/internal/hal"
	"github.com/gogpu/wgpu/internal/hal/software"
)

// BenchmarkSurfacePresent measures the public presentation path end to end:
// GetCurrentTexture followed by Present, on a 1x1 headless software surface.
//
// A swapchain serializes acquisitions, so one Present needs its own acquire;
// the pair is the unit this benchmark measures. Both halves cross the surface
// domain's device seam — acquire reads the released and core state, present
// reads the released state and the HAL queue — which is why ADR-070 §15.7
// records this benchmark as the S10 before/after reading. The 1x1 extent keeps
// the software blit negligible next to the frontend path.
//
// Only the public API is used, so the same file runs unchanged before and
// after the surface Move.
func BenchmarkSurfacePresent(b *testing.B) {
	hal.RegisterBackend(software.NewBackend())
	instance, err := wgpu.CreateInstance(&wgpu.InstanceDescriptor{Backends: gputypes.BackendsSoftware})
	if err != nil {
		b.Fatalf("CreateInstance: %v", err)
	}
	defer instance.Release()

	target, err := wgpu.HeadlessSurfaceTarget{}.SurfaceTarget()
	if err != nil {
		b.Fatalf("HeadlessSurfaceTarget.SurfaceTarget: %v", err)
	}
	surface, err := instance.CreateSurfaceUnsafe(target)
	if err != nil {
		b.Fatalf("CreateSurfaceUnsafe: %v", err)
	}
	defer surface.Release()

	adapter, err := instance.RequestAdapter(&wgpu.RequestAdapterOptions{CompatibleSurface: surface})
	if err != nil {
		b.Fatalf("RequestAdapter: %v", err)
	}
	defer adapter.Release()

	device, err := adapter.RequestDevice(nil)
	if err != nil {
		b.Fatalf("RequestDevice: %v", err)
	}
	defer device.Release()

	if err := surface.Configure(device, &wgpu.SurfaceConfiguration{
		Width:       1,
		Height:      1,
		Format:      gputypes.TextureFormatRGBA8Unorm,
		Usage:       gputypes.TextureUsageRenderAttachment,
		PresentMode: gputypes.PresentModeFifo,
		AlphaMode:   gputypes.CompositeAlphaModeAuto,
	}); err != nil {
		b.Fatalf("Configure: %v", err)
	}
	defer surface.Unconfigure()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		texture, _, err := surface.GetCurrentTexture()
		if err != nil {
			b.Fatalf("GetCurrentTexture: %v", err)
		}
		if err := surface.Present(texture); err != nil {
			b.Fatalf("Present: %v", err)
		}
	}
}
