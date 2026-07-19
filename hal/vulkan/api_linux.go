//go:build linux && !android && !(js && wasm)

// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vulkan

import (
	"fmt"
	"unsafe"

	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/vulkan/vk"
)

// platformSurfaceExtensions returns every Linux WSI extension the backend can
// use. CreateInstance filters this list against the loader's advertised
// extensions, independent of DISPLAY or WAYLAND_DISPLAY.
func platformSurfaceExtensions() []string {
	return []string{
		extensionWaylandSurface,
		extensionXlibSurface,
	}
}

// CreateSurface creates a Vulkan surface from an explicit Xlib or Wayland
// target. Both target kinds can coexist in one process; the corresponding
// command is present when the loader advertised that WSI extension.
func (i *Instance) CreateSurface(target hal.SurfaceTarget) (hal.Surface, error) {
	switch target.Kind {
	case hal.SurfaceTargetXlibWindow:
		if !i.cmds.HasCreateXlibSurfaceKHR() {
			return nil, fmt.Errorf("vulkan: %w: vkCreateXlibSurfaceKHR not available", hal.ErrUnsupportedSurfaceTarget)
		}
		return i.createXlibSurface(target.DisplayHandle, target.WindowHandle)
	case hal.SurfaceTargetWaylandSurface:
		if !i.cmds.HasCreateWaylandSurfaceKHR() {
			return nil, fmt.Errorf("vulkan: %w: vkCreateWaylandSurfaceKHR not available", hal.ErrUnsupportedSurfaceTarget)
		}
		return i.createWaylandSurface(target.DisplayHandle, target.WindowHandle)
	default:
		return nil, fmt.Errorf("vulkan: %w: got %s, backend requires Xlib window or Wayland surface", hal.ErrUnsupportedSurfaceTarget, target.Kind)
	}
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
