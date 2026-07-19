//go:build darwin && !(js && wasm)

// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vulkan

import (
	"fmt"
	"unsafe"

	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/vulkan/vk"
)

// platformSurfaceExtensions returns the macOS surface extension.
func platformSurfaceExtensions() []string {
	return []string{"VK_EXT_metal_surface\x00"}
}

// CreateSurface creates a Metal surface from a CAMetalLayer target.
func (i *Instance) CreateSurface(target hal.SurfaceTarget) (hal.Surface, error) {
	if err := target.RequireKind(hal.SurfaceTargetMetalLayer); err != nil {
		return nil, fmt.Errorf("vulkan: %w", err)
	}
	metalLayer := target.WindowHandle
	createInfo := vk.MetalSurfaceCreateInfoEXT{
		SType: vk.StructureTypeMetalSurfaceCreateInfoExt,
	}
	// Write CAMetalLayer* value directly into the PLayer field memory.
	// PLayer is *CAMetalLayer (a Go pointer type) but must hold the raw C pointer
	// address. We cannot use unsafe.Pointer(uintptr) — go vet rejects it.
	// Instead, write the uintptr value into the field's memory location.
	// Previous bug: &metalLayer stored Go stack address instead of CAMetalLayer* value.
	*(*uintptr)(unsafe.Pointer(&createInfo.PLayer)) = metalLayer

	if !i.cmds.HasCreateMetalSurfaceEXT() {
		return nil, fmt.Errorf("vulkan: %w: vkCreateMetalSurfaceEXT not available (VK_EXT_metal_surface extension not loaded)", hal.ErrUnsupportedSurfaceTarget)
	}

	var surface vk.SurfaceKHR
	result := i.cmds.CreateMetalSurfaceEXT(i.handle, &createInfo, nil, &surface)
	if result != vk.Success {
		return nil, fmt.Errorf("vulkan: vkCreateMetalSurfaceEXT failed: %d", result)
	}
	if surface == 0 {
		return nil, fmt.Errorf("vulkan: vkCreateMetalSurfaceEXT returned success but surface is null")
	}

	return &Surface{
		handle:   surface,
		instance: i,
	}, nil
}
