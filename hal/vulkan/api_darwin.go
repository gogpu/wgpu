//go:build darwin

// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vulkan

import (
	"fmt"
	"unsafe"

	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/vulkan/vk"
)

// platformSurfaceExtension returns the macOS surface extension.
func platformSurfaceExtension() string {
	return "VK_EXT_metal_surface\x00"
}

// CreateSurface creates a Metal surface from a CAMetalLayer.
// Parameters:
//   - _: unused first parameter for API consistency with other platforms
//   - metalLayer: Pointer to CAMetalLayer
func (i *Instance) CreateSurface(_, metalLayer uintptr) (hal.Surface, error) {
	// Convert layer pointer: CAMetalLayer* -> *CAMetalLayer (which is *uintptr internally)
	layer := (*vk.CAMetalLayer)(unsafe.Pointer(&metalLayer))

	createInfo := vk.MetalSurfaceCreateInfoEXT{
		SType:  vk.StructureTypeMetalSurfaceCreateInfoExt,
		PLayer: layer,
	}

	var surface vk.SurfaceKHR
	result := i.cmds.CreateMetalSurfaceEXT(i.handle, &createInfo, nil, &surface)
	if result != vk.Success {
		return nil, fmt.Errorf("vulkan: vkCreateMetalSurfaceEXT failed: %d", result)
	}

	return &Surface{
		handle:   surface,
		instance: i,
	}, nil
}
