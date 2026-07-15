//go:build android && arm64

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vulkan

import (
	"fmt"

	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/vulkan/vk"
)

func platformSurfaceExtension() string {
	return "VK_KHR_android_surface\x00"
}

// CreateSurface creates a Vulkan surface from an ANativeWindow.
//
// The display handle is ignored, matching Rust wgpu v29's Android path. The
// window handle is the raw ANativeWindow pointer and must be non-zero. The host
// retains its application reference; Vulkan owns its surface reference until
// DestroySurfaceKHR. WGPU does not acquire or release ANativeWindow itself.
func (i *Instance) CreateSurface(_, windowHandle uintptr) (hal.Surface, error) {
	if err := validateAndroidSurfaceRequest(windowHandle); err != nil {
		return nil, err
	}
	if i == nil || i.handle == 0 {
		return nil, fmt.Errorf("vulkan: cannot create Android surface from a destroyed instance")
	}
	if err := validateAndroidSurfaceSupport(i.cmds.HasWSIQueries(), i.cmds.HasCreateAndroidSurfaceKHR()); err != nil {
		return nil, err
	}

	createInfo := vk.AndroidSurfaceCreateInfoKHR{SType: vk.StructureTypeAndroidSurfaceCreateInfoKhr}
	setAndroidSurfaceNativeWindow(&createInfo, windowHandle)

	var handle vk.SurfaceKHR
	result := i.cmds.CreateAndroidSurfaceKHR(i.handle, &createInfo, nil, &handle)
	if result != vk.Success {
		return nil, mapAndroidSurfaceCreateError(result)
	}
	if handle == 0 {
		return nil, fmt.Errorf("vulkan: vkCreateAndroidSurfaceKHR returned success with a null surface")
	}

	return &Surface{handle: handle, instance: i}, nil
}
