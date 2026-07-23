//go:build !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vulkan

import (
	"fmt"

	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/vulkan/vk"
)

// swapchainPlatformPolicy isolates the few WSI choices that differ on
// Android while keeping the Vulkan swapchain mechanism shared.
type swapchainPlatformPolicy struct {
	android           bool
	androidSDKVersion uint32
}

func defaultSwapchainPlatformPolicy() swapchainPlatformPolicy {
	return swapchainPlatformPolicy{}
}

func androidSwapchainPlatformPolicy(sdk uint32) swapchainPlatformPolicy {
	return swapchainPlatformPolicy{android: true, androidSDKVersion: sdk}
}

func (p swapchainPlatformPolicy) acquireTimeout(requested uint64) uint64 {
	// Android 10's presentation implementation does not support finite image
	// acquire timeouts. Android 11 (API 30) added support.
	if p.android && p.androidSDKVersion < 30 {
		return ^uint64(0)
	}
	return requested
}

func (p swapchainPlatformPolicy) preTransform(capabilities vk.SurfaceCapabilitiesKHR) (vk.SurfaceTransformFlagBitsKHR, error) {
	transform := capabilities.CurrentTransform
	if p.android {
		// Rust wgpu v29 leaves pre-rotation to the compositor on Android.
		transform = vk.SurfaceTransformIdentityBitKhr
	}
	if transform == 0 {
		return 0, fmt.Errorf("vulkan: surface returned no usable transform")
	}
	if vk.Flags(capabilities.SupportedTransforms)&vk.Flags(transform) == 0 {
		if p.android {
			return 0, fmt.Errorf("vulkan: Android surface does not support the required identity transform")
		}
		return 0, fmt.Errorf("vulkan: surface current transform is not supported")
	}
	return transform, nil
}

func (p swapchainPlatformPolicy) reportSuboptimal(suboptimal bool) bool {
	// Android reports SUBOPTIMAL when identity pre-transform differs from the
	// current orientation. Rust wgpu v29 intentionally treats that as success.
	return suboptimal && !p.android
}

func swapchainPolicyForSurface(surface *Surface) swapchainPlatformPolicy {
	if surface == nil || surface.instance == nil {
		return defaultSwapchainPlatformPolicy()
	}
	return surface.instance.platform.swapchain
}

// mapVulkanResult preserves the typed HAL errors callers use for recovery.
func mapVulkanResult(operation string, result vk.Result) error {
	switch result {
	case vk.Success:
		return nil
	case vk.Timeout:
		return fmt.Errorf("vulkan: %s failed: %w", operation, hal.ErrTimeout)
	case vk.NotReady:
		return fmt.Errorf("vulkan: %s failed: %w", operation, hal.ErrNotReady)
	case vk.ErrorOutOfHostMemory, vk.ErrorOutOfDeviceMemory:
		return fmt.Errorf("vulkan: %s failed: %w", operation, hal.ErrDeviceOutOfMemory)
	case vk.ErrorDeviceLost:
		return fmt.Errorf("vulkan: %s failed: %w", operation, hal.ErrDeviceLost)
	case vk.ErrorSurfaceLostKhr:
		return fmt.Errorf("vulkan: %s failed: %w", operation, hal.ErrSurfaceLost)
	case vk.ErrorOutOfDateKhr:
		return fmt.Errorf("vulkan: %s failed: %w", operation, hal.ErrSurfaceOutdated)
	default:
		return fmt.Errorf("vulkan: %s failed: %d", operation, result)
	}
}
