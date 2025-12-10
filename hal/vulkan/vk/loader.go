// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vk

import (
	"fmt"
	"runtime"
	"sync"
	"unsafe"

	"github.com/go-webgpu/goffi/ffi"
	"github.com/go-webgpu/goffi/types"
)

var (
	vulkanLib              unsafe.Pointer
	vkGetInstanceProcAddr  unsafe.Pointer
	vkGetDeviceProcAddr    unsafe.Pointer
	cifGetInstanceProcAddr types.CallInterface
	cifGetDeviceProcAddr   types.CallInterface

	initOnce sync.Once
	errInit  error
)

// vulkanLibraryName returns platform-specific Vulkan library name.
func vulkanLibraryName() string {
	switch runtime.GOOS {
	case "windows":
		return "vulkan-1.dll"
	case "darwin":
		return "libvulkan.dylib" // MoltenVK
	default: // linux, freebsd, etc.
		return "libvulkan.so.1"
	}
}

// Init loads the Vulkan library and initializes signatures.
// Safe to call multiple times - only first call does actual work.
func Init() error {
	initOnce.Do(func() {
		errInit = doInit()
	})
	return errInit
}

func doInit() error {
	var err error

	// Load Vulkan library
	vulkanLib, err = ffi.LoadLibrary(vulkanLibraryName())
	if err != nil {
		return fmt.Errorf("failed to load Vulkan library %s: %w", vulkanLibraryName(), err)
	}

	// Get vkGetInstanceProcAddr
	vkGetInstanceProcAddr, err = ffi.GetSymbol(vulkanLib, "vkGetInstanceProcAddr")
	if err != nil {
		return fmt.Errorf("vkGetInstanceProcAddr not found: %w", err)
	}

	// Prepare CallInterface for vkGetInstanceProcAddr
	// PFN_vkVoidFunction vkGetInstanceProcAddr(VkInstance instance, const char* pName)
	err = ffi.PrepareCallInterface(&cifGetInstanceProcAddr, types.DefaultCall,
		types.PointerTypeDescriptor, // returns function pointer
		[]*types.TypeDescriptor{
			types.UInt64TypeDescriptor,  // VkInstance (handle, can be 0)
			types.PointerTypeDescriptor, // const char* pName
		})
	if err != nil {
		return fmt.Errorf("failed to prepare GetInstanceProcAddr interface: %w", err)
	}

	// Prepare CallInterface for vkGetDeviceProcAddr
	// PFN_vkVoidFunction vkGetDeviceProcAddr(VkDevice device, const char* pName)
	err = ffi.PrepareCallInterface(&cifGetDeviceProcAddr, types.DefaultCall,
		types.PointerTypeDescriptor,
		[]*types.TypeDescriptor{
			types.UInt64TypeDescriptor,  // VkDevice
			types.PointerTypeDescriptor, // const char* pName
		})
	if err != nil {
		return fmt.Errorf("failed to prepare GetDeviceProcAddr interface: %w", err)
	}

	// Initialize signature templates
	if err := InitSignatures(); err != nil {
		return fmt.Errorf("failed to initialize signatures: %w", err)
	}

	return nil
}

// GetInstanceProcAddr returns function pointer for Vulkan instance function.
// Pass instance=0 for global functions (vkCreateInstance, vkEnumerateInstance*).
func GetInstanceProcAddr(instance Instance, name string) unsafe.Pointer {
	if vkGetInstanceProcAddr == nil {
		return nil
	}

	// Convert name to null-terminated C string
	cname := make([]byte, len(name)+1)
	copy(cname, name)

	var result unsafe.Pointer
	args := [2]unsafe.Pointer{
		unsafe.Pointer(&instance),
		unsafe.Pointer(&cname[0]),
	}

	_ = ffi.CallFunction(&cifGetInstanceProcAddr, vkGetInstanceProcAddr, unsafe.Pointer(&result), args[:])
	return result
}

// GetDeviceProcAddr returns function pointer for Vulkan device function.
func GetDeviceProcAddr(device Device, name string) unsafe.Pointer {
	if vkGetDeviceProcAddr == nil {
		// Lazy load from instance
		vkGetDeviceProcAddr = GetInstanceProcAddr(0, "vkGetDeviceProcAddr")
		if vkGetDeviceProcAddr == nil {
			return nil
		}
	}

	// Convert name to null-terminated C string
	cname := make([]byte, len(name)+1)
	copy(cname, name)

	var result unsafe.Pointer
	args := [2]unsafe.Pointer{
		unsafe.Pointer(&device),
		unsafe.Pointer(&cname[0]),
	}

	_ = ffi.CallFunction(&cifGetDeviceProcAddr, vkGetDeviceProcAddr, unsafe.Pointer(&result), args[:])
	return result
}

// Close releases the Vulkan library.
func Close() error {
	if vulkanLib != nil {
		err := ffi.FreeLibrary(vulkanLib)
		vulkanLib = nil
		vkGetInstanceProcAddr = nil
		vkGetDeviceProcAddr = nil
		return err
	}
	return nil
}
