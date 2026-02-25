//go:build linux

// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vulkan

import (
	"fmt"
	"unsafe"

	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/vulkan/vk"
)

// platformSurfaceExtension returns the Linux surface extension.
func platformSurfaceExtension() string {
	return "VK_KHR_xlib_surface\x00"
}

// CreateSurface creates an X11 surface from display and window handles.
// Parameters:
//   - display: X11 Display pointer (Display*)
//   - window: X11 Window handle (Window)
func (i *Instance) CreateSurface(display, window uintptr) (hal.Surface, error) {
	createInfo := vk.XlibSurfaceCreateInfoKHR{
		SType:  vk.StructureTypeXlibSurfaceCreateInfoKhr,
		Window: vk.XlibWindow(window),
	}
	// Write Display* value directly into the Dpy field memory.
	// Dpy is *XlibDisplay (a Go pointer type) but must hold the raw C Display*
	// address. We cannot use unsafe.Pointer(uintptr) — go vet rejects it.
	// Instead, write the uintptr value into the field's memory location.
	// Previous bug: &display stored Go stack address instead of Display* value.
	*(*uintptr)(unsafe.Pointer(&createInfo.Dpy)) = display

	if !i.cmds.HasCreateXlibSurfaceKHR() {
		return nil, fmt.Errorf("vulkan: vkCreateXlibSurfaceKHR not available (VK_KHR_xlib_surface extension not loaded)")
	}

	var surface vk.SurfaceKHR
	result := i.cmds.CreateXlibSurfaceKHR(i.handle, &createInfo, nil, &surface)
	if result != vk.Success {
		return nil, fmt.Errorf("vulkan: vkCreateXlibSurfaceKHR failed: %d", result)
	}
	if surface == 0 {
		return nil, fmt.Errorf("vulkan: vkCreateXlibSurfaceKHR returned success but surface is null")
	}

	return &Surface{
		handle:   surface,
		instance: i,
	}, nil
}
