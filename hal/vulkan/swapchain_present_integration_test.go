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
	instance := createTestInstance(t)
	surface := createTestSurface(t, instance)
	device, queue := openTestDevice(t, instance, surface)
	configureTestSurface(t, surface, device)

	swapchain := surface.(*Surface).swapchain
	fence := device.(*Device).timelineFence
	var barrierFenceValue uint64
	for frame := range 3 {
		acquired, err := surface.AcquireTexture(nil)
		if err != nil {
			t.Fatalf("frame %d: AcquireTexture answered %v", frame, err)
		}
		if fence.lastCompleted < barrierFenceValue {
			t.Fatalf("frame %d: AcquireTexture reset the barrier pool before the barrier submit %d completed (completed: %d)", frame, barrierFenceValue, fence.lastCompleted)
		}
		if err := queue.Present(surface, acquired.Texture, nil); err != nil {
			t.Fatalf("frame %d: Present of an image nothing was submitted for answered %v", frame, err)
		}
		barrierFenceValue = swapchain.barrierFenceValue
		if barrierFenceValue == 0 {
			t.Fatalf("frame %d: Present kept no fence value for its barrier submit", frame)
		}
	}
}

// TestAnEmptySubmissionThenPresentTransitionsWithATrackedBarrier presents an
// image whose frame submitted work that never touched the swapchain image: the
// submission consumed the acquire semaphore, so present() does not supply one,
// and no render pass injected the inline PRESENT_SRC_KHR barrier, so present
// falls back to ensurePresentLayout's own barrier submission.
//
// That submission must be tracked like every other one. The next acquire
// resets the command pool the barrier was recorded in, and resetting the pool
// while its command buffer is still pending invalidates the transition
// (VUID-vkResetCommandPool-commandPool-00040) and leaves the image in
// UNDEFINED at present (VUID-VkPresentInfoKHR-pImageIndices-01430).
//
// The test is skipped if no Vulkan GPU is available (headless CI).
func TestAnEmptySubmissionThenPresentTransitionsWithATrackedBarrier(t *testing.T) {
	instance := createTestInstance(t)
	surface := createTestSurface(t, instance)
	device, queue := openTestDevice(t, instance, surface)
	configureTestSurface(t, surface, device)

	swapchain := surface.(*Surface).swapchain
	fence := device.(*Device).timelineFence
	commandBuffers := make([]hal.CommandBuffer, 0, 3)
	var barrierFenceValue uint64
	for frame := range 3 {
		acquired, err := surface.AcquireTexture(nil)
		if err != nil {
			t.Fatalf("frame %d: AcquireTexture answered %v", frame, err)
		}
		if fence.lastCompleted < barrierFenceValue {
			t.Fatalf("frame %d: AcquireTexture reset the barrier pool before the barrier submit %d completed (completed: %d)", frame, barrierFenceValue, fence.lastCompleted)
		}

		encoder, err := device.CreateCommandEncoder(&hal.CommandEncoderDescriptor{Label: "empty submission"})
		if err != nil {
			t.Fatalf("frame %d: CreateCommandEncoder answered %v", frame, err)
		}
		if err := encoder.BeginEncoding("empty submission"); err != nil {
			t.Fatalf("frame %d: BeginEncoding answered %v", frame, err)
		}
		commandBuffer, err := encoder.EndEncoding()
		if err != nil {
			t.Fatalf("frame %d: EndEncoding answered %v", frame, err)
		}
		if _, err := queue.Submit([]hal.CommandBuffer{commandBuffer}); err != nil {
			t.Fatalf("frame %d: Submit answered %v", frame, err)
		}
		commandBuffers = append(commandBuffers, commandBuffer)

		if err := queue.Present(surface, acquired.Texture, nil); err != nil {
			t.Fatalf("frame %d: Present answered %v", frame, err)
		}
		barrierFenceValue = swapchain.barrierFenceValue
		if barrierFenceValue == 0 {
			t.Fatalf("frame %d: Present kept no fence value for its layout barrier submit", frame)
		}
	}

	if err := device.WaitIdle(); err != nil {
		t.Fatalf("WaitIdle answered %v", err)
	}
	for _, commandBuffer := range commandBuffers {
		device.FreeCommandBuffer(commandBuffer)
	}
}

// TestTwoSurfacesAcquiredTogetherBothPresent acquires an image from each of
// two surfaces, submits once, and presents both, as a frame drawn into two
// windows does. The queue used to track only the surface acquired last, so
// presenting the first failed with "no successful swapchain submission is
// ready for presentation" and broke its swapchain.
//
// The test is skipped if no Vulkan GPU is available (headless CI).
func TestTwoSurfacesAcquiredTogetherBothPresent(t *testing.T) {
	instance := createTestInstance(t)
	surfaces := []hal.Surface{createTestSurface(t, instance), createTestSurface(t, instance)}
	device, queue := openTestDevice(t, instance, surfaces[0])
	for _, surface := range surfaces {
		configureTestSurface(t, surface, device)
	}

	for frame := range 3 {
		textures := make([]hal.SurfaceTexture, len(surfaces))
		for index, surface := range surfaces {
			acquired, err := surface.AcquireTexture(nil)
			if err != nil {
				t.Fatalf("frame %d: AcquireTexture of surface %d answered %v", frame, index, err)
			}
			textures[index] = acquired.Texture
		}
		encoder, err := device.CreateCommandEncoder(&hal.CommandEncoderDescriptor{Label: "two surfaces"})
		if err != nil {
			t.Fatalf("frame %d: CreateCommandEncoder answered %v", frame, err)
		}
		if err := encoder.BeginEncoding("two surfaces"); err != nil {
			t.Fatalf("frame %d: BeginEncoding answered %v", frame, err)
		}
		commandBuffer, err := encoder.EndEncoding()
		if err != nil {
			t.Fatalf("frame %d: EndEncoding answered %v", frame, err)
		}
		if _, err := queue.Submit([]hal.CommandBuffer{commandBuffer}); err != nil {
			t.Fatalf("frame %d: Submit answered %v", frame, err)
		}
		for index, surface := range surfaces {
			if err := queue.Present(surface, textures[index], nil); err != nil {
				t.Fatalf("frame %d: Present of surface %d answered %v", frame, index, err)
			}
		}
		if err := device.WaitIdle(); err != nil {
			t.Fatalf("frame %d: WaitIdle answered %v", frame, err)
		}
		device.FreeCommandBuffer(commandBuffer)
	}
}

// createTestInstance creates a Vulkan instance and destroys it when the test
// ends. It skips the test in short mode and when Vulkan is not available.
func createTestInstance(t *testing.T) hal.Instance {
	t.Helper()
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
	t.Cleanup(instance.Destroy)
	return instance
}

// createTestSurface makes a surface for a new shown window, and destroys it
// when the test ends.
func createTestSurface(t *testing.T, instance hal.Instance) hal.Surface {
	t.Helper()
	window := createTestWindow(t, 256, 256)
	surface, err := instance.CreateSurface(hal.SurfaceTarget{Kind: hal.SurfaceTargetWindowsHWND, WindowHandle: window})
	if err != nil {
		t.Skipf("Vulkan surface creation failed: %v", err)
	}
	t.Cleanup(surface.Destroy)
	return surface
}

// openTestDevice opens a device on the first adapter that presents to
// surface, and destroys it when the test ends.
func openTestDevice(t *testing.T, instance hal.Instance, surface hal.Surface) (hal.Device, hal.Queue) {
	t.Helper()
	adapters := instance.EnumerateAdapters(surface)
	if len(adapters) == 0 {
		t.Skip("no Vulkan adapter presents to the window")
	}
	opened, err := adapters[0].Adapter.Open(0, adapters[0].Capabilities.Limits)
	if err != nil {
		t.Skipf("failed to open Vulkan device: %v", err)
	}
	t.Cleanup(opened.Device.Destroy)
	t.Cleanup(func() { _ = opened.Device.WaitIdle() })
	return opened.Device, opened.Queue
}

// configureTestSurface configures surface on device, and unconfigures it
// when the test ends.
func configureTestSurface(t *testing.T, surface hal.Surface, device hal.Device) {
	t.Helper()
	err := surface.Configure(device, &hal.SurfaceConfiguration{
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
	t.Cleanup(func() { surface.Unconfigure(device) })
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
