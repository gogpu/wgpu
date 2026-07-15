//go:build !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vulkan

import (
	"fmt"
	"unsafe"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/vulkan/vk"
)

func validateAndroidSurfaceSupport(hasWSIQueries, hasCreateCommand bool) error {
	if !hasWSIQueries || !hasCreateCommand {
		return fmt.Errorf("vulkan: Android surface WSI is unavailable")
	}
	return nil
}

func validateAndroidSurfaceRequest(window uintptr) error {
	if window == 0 {
		return fmt.Errorf("vulkan: Android ANativeWindow must be non-null")
	}
	return nil
}

func validateAndroidSDKVersion(sdk uint32) error {
	if sdk < 29 {
		return fmt.Errorf("vulkan: Android API 29 or newer is required, device reports %d", sdk)
	}
	return nil
}

func validateAndroidInstanceFlags(flags gputypes.InstanceFlags) error {
	if flags&gputypes.InstanceFlagsDebug != 0 {
		return fmt.Errorf("vulkan: debug callbacks are unsupported on Android")
	}
	return nil
}

func setAndroidSurfaceNativeWindow(createInfo *vk.AndroidSurfaceCreateInfoKHR, window uintptr) {
	// Window is generated as *vk.ANativeWindow but contains a raw C pointer.
	// Store its integer bits in-place without manufacturing a Go pointer.
	*(*uintptr)(unsafe.Pointer(&createInfo.Window)) = window
}

func mapAndroidSurfaceCreateError(result vk.Result) error {
	switch result {
	case vk.ErrorSurfaceLostKhr, vk.ErrorInitializationFailed:
		return fmt.Errorf("vulkan: vkCreateAndroidSurfaceKHR failed: %w", hal.ErrSurfaceLost)
	case vk.ErrorNativeWindowInUseKhr:
		return fmt.Errorf("vulkan: vkCreateAndroidSurfaceKHR failed: native window is already in use")
	default:
		return mapVulkanResult("vkCreateAndroidSurfaceKHR", result)
	}
}
