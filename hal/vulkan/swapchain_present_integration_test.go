// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows && !(js && wasm)

package vulkan

import (
	"syscall"
	"testing"
	"unsafe"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/vulkan/vk"
)

// TestAnImageAcquiredAndPresentedWithNothingSubmittedPresents presents
// images nothing was drawn into, as WebGPU allows: an application may skip
// drawing a frame it acquired. Vulkan presents an image only after a submit
// waited on its acquire semaphore, which no submit did here, so the present
// supplies that submit. It used to fail with "no successful swapchain
// submission is ready for presentation" and break the swapchain, so the next
// acquire failed too.
//
// The next acquire resets the pool the barrier was recorded in, so it first
// waits for the barrier's submit: resetting the pool while that submit is
// pending invalidates its command buffer.
//
// The test is skipped if no Vulkan GPU is available (headless CI).
func TestAnImageAcquiredAndPresentedWithNothingSubmittedPresents(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping GPU integration test in short mode")
	}
	if err := vk.Init(); err != nil {
		t.Skipf("Vulkan not available: %v", err)
	}
	instance, err := Backend{}.CreateInstance(&hal.InstanceDescriptor{Backends: gputypes.BackendsVulkan})
	if err != nil {
		t.Skipf("Vulkan instance creation failed: %v", err)
	}
	defer instance.Destroy()

	window := createTestWindow(t, 256, 256)
	surface, err := instance.CreateSurface(hal.SurfaceTarget{Kind: hal.SurfaceTargetWindowsHWND, WindowHandle: window})
	if err != nil {
		t.Skipf("Vulkan surface creation failed: %v", err)
	}
	defer surface.Destroy()

	adapters := instance.EnumerateAdapters(surface)
	if len(adapters) == 0 {
		t.Skip("no Vulkan adapter presents to the window")
	}
	opened, err := adapters[0].Adapter.Open(0, adapters[0].Capabilities.Limits)
	if err != nil {
		t.Skipf("failed to open Vulkan device: %v", err)
	}
	defer opened.Device.Destroy()
	defer func() { _ = opened.Device.WaitIdle() }()

	err = surface.Configure(opened.Device, &hal.SurfaceConfiguration{
		Width:       256,
		Height:      256,
		Format:      gputypes.TextureFormatBGRA8Unorm,
		Usage:       gputypes.TextureUsageRenderAttachment,
		PresentMode: gputypes.PresentModeFifo,
		AlphaMode:   gputypes.CompositeAlphaModeOpaque,
	})
	if err != nil {
		t.Skipf("surface configure failed: %v", err)
	}
	defer surface.Unconfigure(opened.Device)

	swapchain := surface.(*Surface).swapchain
	fence := opened.Device.(*Device).timelineFence
	var barrierFenceValue uint64
	for frame := range 3 {
		acquired, err := surface.AcquireTexture(nil)
		if err != nil {
			t.Fatalf("frame %d: AcquireTexture answered %v", frame, err)
		}
		if fence.lastCompleted < barrierFenceValue {
			t.Fatalf("frame %d: AcquireTexture reset the barrier pool before the barrier submit %d completed (completed: %d)", frame, barrierFenceValue, fence.lastCompleted)
		}
		if err := opened.Queue.Present(surface, acquired.Texture, nil); err != nil {
			t.Fatalf("frame %d: Present of an image nothing was submitted for answered %v", frame, err)
		}
		barrierFenceValue = swapchain.barrierFenceValue
		if barrierFenceValue == 0 {
			t.Fatalf("frame %d: Present kept no fence value for its barrier submit", frame)
		}
	}
}

// createTestWindow opens a shown window of the predefined STATIC class,
// which needs no class registration, and destroys it when the test ends.
func createTestWindow(t *testing.T, width, height int32) uintptr {
	t.Helper()
	user32 := syscall.NewLazyDLL("user32.dll")
	className, err := syscall.UTF16PtrFromString("STATIC")
	if err != nil {
		t.Fatal(err)
	}
	const overlappedVisibleWindow = 0x00CF0000 | 0x10000000 // WS_OVERLAPPEDWINDOW | WS_VISIBLE
	window, _, callErr := user32.NewProc("CreateWindowExW").Call(
		0, uintptr(unsafe.Pointer(className)), 0, overlappedVisibleWindow,
		0, 0, uintptr(width), uintptr(height), 0, 0, 0, 0,
	)
	if window == 0 {
		t.Skipf("CreateWindowExW failed: %v", callErr)
	}
	t.Cleanup(func() { _, _, _ = user32.NewProc("DestroyWindow").Call(window) })
	return window
}
