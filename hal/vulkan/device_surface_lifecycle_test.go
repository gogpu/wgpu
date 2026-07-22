// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !(js && wasm)

package vulkan

import (
	"errors"
	"testing"

	"github.com/gogpu/wgpu/hal"
)

func TestDeviceOwnsConfiguredSurfacesUntilDestroyBegins(t *testing.T) {
	device := &Device{handle: 1}
	first := &Surface{}
	second := &Surface{}
	if err := device.registerConfiguredSurface(first); err != nil {
		t.Fatalf("register first surface: %v", err)
	}
	if err := device.registerConfiguredSurface(second); err != nil {
		t.Fatalf("register second surface: %v", err)
	}

	surfaces, ok := device.beginDestroy()
	if !ok {
		t.Fatal("beginDestroy rejected a live device")
	}
	got := make(map[*Surface]bool, len(surfaces))
	for _, surface := range surfaces {
		got[surface] = true
	}
	if !got[first] || !got[second] || len(got) != 2 {
		t.Fatalf("owned surfaces = %v, want both configured surfaces", surfaces)
	}
	if _, ok := device.beginDestroy(); ok {
		t.Fatal("second beginDestroy succeeded")
	}
	if err := device.registerConfiguredSurface(&Surface{}); !errors.Is(err, hal.ErrDeviceLost) {
		t.Fatalf("register while destroying = %v, want ErrDeviceLost", err)
	}
}

func TestSurfaceAbandonsSwapchainHandlesAfterDeviceLoss(t *testing.T) {
	device := &Device{handle: 1}
	surface := &Surface{device: device}
	texture := &SwapchainTexture{handle: 2, view: 3}
	swapchain := &Swapchain{
		handle:          4,
		device:          device,
		surface:         surface,
		surfaceTextures: []*SwapchainTexture{texture},
		imageAcquired:   true,
	}
	texture.swapchain = swapchain
	surface.swapchain = swapchain

	surface.releaseConfiguredDevice(device, false)
	if surface.device != nil || surface.swapchain != nil {
		t.Fatal("surface retained device-owned swapchain state")
	}
	if swapchain.device != nil || swapchain.surface != nil || swapchain.handle != 0 {
		t.Fatal("swapchain retained native ownership after device loss")
	}
	if texture.handle != 0 || texture.view != 0 || texture.swapchain != nil {
		t.Fatal("retained texture exposed abandoned swapchain handles")
	}
}
