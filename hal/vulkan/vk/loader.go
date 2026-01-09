// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

// Package vk provides Pure Go Vulkan bindings using goffi for FFI calls.
//
// # goffi Calling Convention
//
// CRITICAL: goffi expects args[] to contain pointers to WHERE argument values are stored,
// NOT the values themselves. This applies to ALL argument types, including pointers.
//
// For scalar types (uint32, uint64, etc.):
//
//	var value uint64 = 42
//	args[i] = unsafe.Pointer(&value)  // ✓ Correct: pointer to value storage
//
// For pointer types (const char*, void*, etc.):
//
//	ptr := unsafe.Pointer(&data[0])   // This IS the pointer value
//	args[i] = unsafe.Pointer(&ptr)    // ✓ Correct: pointer TO the pointer
//
//	// WRONG: args[i] = unsafe.Pointer(&data[0])
//	// This passes the data address, but goffi reads it AS IF it contains a pointer,
//	// interpreting the data bytes as a memory address → crash!
//
// This pattern is required because goffi uses ffi_call() internally, which reads
// argument values FROM the addresses provided in the args array.
//
// # Intel Driver Compatibility
//
// Intel drivers (Iris Xe, 12th Gen+) have known quirks:
//   - vkGetInstanceProcAddr(NULL, "vkGetDeviceProcAddr") returns NULL
//   - Use SetDeviceProcAddr(instance) after vkCreateInstance
//   - vkCreateGraphicsPipelines may return VK_SUCCESS with VK_NULL_HANDLE pipeline
//
// See: https://github.com/gogpu/wgpu/issues/24
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
	// goffi expects args[] to contain pointers to WHERE values are stored.
	// For pointer arguments, we need pointer-to-pointer: store the pointer
	// in a variable, then pass &variable.
	namePtr := unsafe.Pointer(&cname[0])
	args := [2]unsafe.Pointer{
		unsafe.Pointer(&instance),
		unsafe.Pointer(&namePtr), // pointer TO the pointer
	}

	_ = ffi.CallFunction(&cifGetInstanceProcAddr, vkGetInstanceProcAddr, unsafe.Pointer(&result), args[:])
	return result
}

// SetDeviceProcAddr sets the vkGetDeviceProcAddr function pointer.
// Must be called with a valid instance after vkCreateInstance.
// Some drivers (e.g., Intel) don't support loading vkGetDeviceProcAddr with instance=0.
func SetDeviceProcAddr(instance Instance) {
	if vkGetDeviceProcAddr == nil {
		vkGetDeviceProcAddr = GetInstanceProcAddr(instance, "vkGetDeviceProcAddr")
	}
}

// GetDeviceProcAddr returns function pointer for Vulkan device function.
func GetDeviceProcAddr(device Device, name string) unsafe.Pointer {
	if vkGetDeviceProcAddr == nil {
		// Try lazy load from global (may not work on all drivers)
		vkGetDeviceProcAddr = GetInstanceProcAddr(0, "vkGetDeviceProcAddr")
		if vkGetDeviceProcAddr == nil {
			return nil
		}
	}

	// Convert name to null-terminated C string
	cname := make([]byte, len(name)+1)
	copy(cname, name)

	var result unsafe.Pointer
	// goffi expects args[] to contain pointers to WHERE values are stored.
	// For pointer arguments, we need pointer-to-pointer.
	namePtr := unsafe.Pointer(&cname[0])
	args := [2]unsafe.Pointer{
		unsafe.Pointer(&device),
		unsafe.Pointer(&namePtr), // pointer TO the pointer
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
