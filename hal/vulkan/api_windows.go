//go:build windows

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

// platformSurfaceExtension returns the Windows surface extension.
func platformSurfaceExtension() string {
	return "VK_KHR_win32_surface\x00"
}

// CreateSurface creates a Windows surface from HINSTANCE and HWND.
func (i *Instance) CreateSurface(hinstance, hwnd uintptr) (hal.Surface, error) {
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
		return nil, fmt.Errorf("vulkan: vkCreateWin32SurfaceKHR not available (VK_KHR_win32_surface extension not loaded)")
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
