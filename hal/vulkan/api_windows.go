//go:build windows && !(js && wasm)

// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vulkan

import (
	"fmt"
	"syscall"

	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/vulkan/vk"
)

var (
	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	getModuleHandleW = kernel32.NewProc("GetModuleHandleW")
)

// platformSurfaceExtensions returns the Windows surface extension.
func platformSurfaceExtensions() []string {
	return []string{"VK_KHR_win32_surface\x00"}
}

// CreateSurface creates a Windows surface from HINSTANCE and HWND.
func (i *Instance) CreateSurface(target hal.SurfaceTarget) (hal.Surface, error) {
	if err := target.RequireKind(hal.SurfaceTargetWindowsHWND); err != nil {
		return nil, fmt.Errorf("vulkan: %w", err)
	}
	hinstance, hwnd := target.DisplayHandle, target.WindowHandle
	// If hinstance is 0, get the current module handle
	if hinstance == 0 {
		hinstance, _, _ = getModuleHandleW.Call(0)
	}

	createInfo := vk.Win32SurfaceCreateInfoKHR{
		SType:     vk.StructureTypeWin32SurfaceCreateInfoKhr,
		Hinstance: hinstance,
		Hwnd:      hwnd,
	}

	if !i.cmds.HasCreateWin32SurfaceKHR() {
		return nil, fmt.Errorf("vulkan: %w: vkCreateWin32SurfaceKHR not available (VK_KHR_win32_surface extension not loaded)", hal.ErrUnsupportedSurfaceTarget)
	}

	var surface vk.SurfaceKHR
	result := i.cmds.CreateWin32SurfaceKHR(i.handle, &createInfo, nil, &surface)
	if result != vk.Success {
		return nil, fmt.Errorf("vulkan: vkCreateWin32SurfaceKHR failed: %d", result)
	}
	if surface == 0 {
		return nil, fmt.Errorf("vulkan: vkCreateWin32SurfaceKHR returned success but surface is null")
	}

	return &Surface{
		handle:   surface,
		instance: i,
	}, nil
}
