//go:build linux

// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vulkan

import (
	"fmt"
	"os"
	"unsafe"

	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/vulkan/vk"
)

// platformSurfaceExtensions returns all Linux surface extensions to request.
// Both X11 and Wayland extensions are requested; the driver enables what it supports.
func platformSurfaceExtension() string {
	// Request both — Vulkan instance creation accepts unsupported extensions gracefully.
	// The actual surface creation checks HasCreate*SurfaceKHR at runtime.
	if isWayland() {
		return "VK_KHR_wayland_surface\x00"
	}
	return "VK_KHR_xlib_surface\x00"
}

// isWayland returns true if the session is running under Wayland.
func isWayland() bool {
	return os.Getenv("WAYLAND_DISPLAY") != ""
}

// CreateSurface creates a Vulkan surface from platform-specific handles.
// On Linux, it auto-detects X11 vs Wayland based on available extensions:
//   - Wayland: display = wl_display*, window = wl_surface* (from libwayland-client)
//   - X11: display = Display* (from libX11), window = X11 Window ID
func (i *Instance) CreateSurface(display, window uintptr) (hal.Surface, error) {
	// Try Wayland first if the extension is available
	if i.cmds.HasCreateWaylandSurfaceKHR() && isWayland() {
		return i.createWaylandSurface(display, window)
	}

	// Fall back to X11
	if i.cmds.HasCreateXlibSurfaceKHR() {
		return i.createXlibSurface(display, window)
	}

	return nil, fmt.Errorf("vulkan: no surface creation extension available (need VK_KHR_xlib_surface or VK_KHR_wayland_surface)")
}

// createXlibSurface creates an X11 surface.
func (i *Instance) createXlibSurface(display, window uintptr) (hal.Surface, error) {
	createInfo := vk.XlibSurfaceCreateInfoKHR{
		SType:  vk.StructureTypeXlibSurfaceCreateInfoKhr,
		Window: vk.XlibWindow(window),
	}
	// Write Display* value directly into the Dpy field memory.
	// Dpy is *XlibDisplay (a Go pointer type) but must hold the raw C Display*
	// address. We cannot use unsafe.Pointer(uintptr) — go vet rejects it.
	*(*uintptr)(unsafe.Pointer(&createInfo.Dpy)) = display

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

// createWaylandSurface creates a Wayland surface.
func (i *Instance) createWaylandSurface(display, window uintptr) (hal.Surface, error) {
	createInfo := vk.WaylandSurfaceCreateInfoKHR{
		SType: vk.StructureTypeWaylandSurfaceCreateInfoKhr,
	}
	// Write wl_display* and wl_surface* values directly into fields.
	// Display is *WlDisplay and Surface is *WlSurface — both Go pointer types
	// that must hold raw C pointer values.
	*(*uintptr)(unsafe.Pointer(&createInfo.Display)) = display
	*(*uintptr)(unsafe.Pointer(&createInfo.Surface)) = window

	var surface vk.SurfaceKHR
	result := i.cmds.CreateWaylandSurfaceKHR(i.handle, &createInfo, nil, &surface)
	if result != vk.Success {
		return nil, fmt.Errorf("vulkan: vkCreateWaylandSurfaceKHR failed: %d", result)
	}
	if surface == 0 {
		return nil, fmt.Errorf("vulkan: vkCreateWaylandSurfaceKHR returned success but surface is null")
	}

	return &Surface{
		handle:   surface,
		instance: i,
	}, nil
}
