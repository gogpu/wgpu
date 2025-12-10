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
	// Convert display pointer: Display* -> *XlibDisplay (which is *uintptr internally)
	dpy := (*vk.XlibDisplay)(unsafe.Pointer(&display))

	createInfo := vk.XlibSurfaceCreateInfoKHR{
		SType:  vk.StructureTypeXlibSurfaceCreateInfoKhr,
		Dpy:    dpy,
		Window: vk.XlibWindow(window),
	}

	var surface vk.SurfaceKHR
	result := i.cmds.CreateXlibSurfaceKHR(i.handle, &createInfo, nil, &surface)
	if result != vk.Success {
		return nil, fmt.Errorf("vulkan: vkCreateXlibSurfaceKHR failed: %d", result)
	}

	return &Surface{
		handle:   surface,
		instance: i,
	}, nil
}
