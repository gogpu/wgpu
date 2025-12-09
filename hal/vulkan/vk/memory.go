// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows

package vk

import (
	"syscall"
	"unsafe"
)

// Global commands instance for memory operations.
// Must be initialized via LoadDevice before using memory functions.
var deviceCmds *Commands

// SetDeviceCommands sets the device-level commands for memory operations.
func SetDeviceCommands(cmds *Commands) {
	deviceCmds = cmds
}

// AllocateMemory allocates device memory.
//
// Wraps vkAllocateMemory.
func AllocateMemory(device Device, allocInfo *MemoryAllocateInfo, allocator *AllocationCallbacks, memory *DeviceMemory) Result {
	if deviceCmds == nil || deviceCmds.allocateMemory == 0 {
		return ErrorInitializationFailed
	}

	var pAllocator uintptr
	if allocator != nil {
		pAllocator = uintptr(unsafe.Pointer(allocator))
	}

	ret, _, _ := syscall.SyscallN(
		deviceCmds.allocateMemory,
		uintptr(device),
		uintptr(unsafe.Pointer(allocInfo)),
		pAllocator,
		uintptr(unsafe.Pointer(memory)),
	)

	return Result(ret)
}

// FreeMemory frees device memory.
//
// Wraps vkFreeMemory.
func FreeMemory(device Device, memory DeviceMemory, allocator *AllocationCallbacks) {
	if deviceCmds == nil || deviceCmds.freeMemory == 0 {
		return
	}

	var pAllocator uintptr
	if allocator != nil {
		pAllocator = uintptr(unsafe.Pointer(allocator))
	}

	//nolint:errcheck // Vulkan void function, no return value to check
	syscall.SyscallN(
		deviceCmds.freeMemory,
		uintptr(device),
		uintptr(memory),
		pAllocator,
	)
}

// MapMemory maps device memory to host address space.
//
// Wraps vkMapMemory.
func MapMemory(device Device, memory DeviceMemory, offset, size uint64, flags MemoryMapFlags, data *uintptr) Result {
	if deviceCmds == nil || deviceCmds.mapMemory == 0 {
		return ErrorInitializationFailed
	}

	ret, _, _ := syscall.SyscallN(
		deviceCmds.mapMemory,
		uintptr(device),
		uintptr(memory),
		uintptr(offset),
		uintptr(size),
		uintptr(flags),
		uintptr(unsafe.Pointer(data)),
	)

	return Result(ret)
}

// UnmapMemory unmaps device memory from host address space.
//
// Wraps vkUnmapMemory.
func UnmapMemory(device Device, memory DeviceMemory) {
	if deviceCmds == nil || deviceCmds.unmapMemory == 0 {
		return
	}

	//nolint:errcheck // Vulkan void function, no return value to check
	syscall.SyscallN(
		deviceCmds.unmapMemory,
		uintptr(device),
		uintptr(memory),
	)
}

// GetBufferMemoryRequirements queries memory requirements for a buffer.
//
// Wraps vkGetBufferMemoryRequirements.
func GetBufferMemoryRequirements(device Device, buffer Buffer, requirements *MemoryRequirements) {
	if deviceCmds == nil || deviceCmds.getBufferMemoryRequirements == 0 {
		return
	}

	//nolint:errcheck // Vulkan void function, no return value to check
	syscall.SyscallN(
		deviceCmds.getBufferMemoryRequirements,
		uintptr(device),
		uintptr(buffer),
		uintptr(unsafe.Pointer(requirements)),
	)
}

// BindBufferMemory binds memory to a buffer.
//
// Wraps vkBindBufferMemory.
func BindBufferMemory(device Device, buffer Buffer, memory DeviceMemory, offset uint64) Result {
	if deviceCmds == nil || deviceCmds.bindBufferMemory == 0 {
		return ErrorInitializationFailed
	}

	ret, _, _ := syscall.SyscallN(
		deviceCmds.bindBufferMemory,
		uintptr(device),
		uintptr(buffer),
		uintptr(memory),
		uintptr(offset),
	)

	return Result(ret)
}

// GetImageMemoryRequirements queries memory requirements for an image.
//
// Wraps vkGetImageMemoryRequirements.
func GetImageMemoryRequirements(device Device, image Image, requirements *MemoryRequirements) {
	if deviceCmds == nil || deviceCmds.getImageMemoryRequirements == 0 {
		return
	}

	//nolint:errcheck // Vulkan void function, no return value to check
	syscall.SyscallN(
		deviceCmds.getImageMemoryRequirements,
		uintptr(device),
		uintptr(image),
		uintptr(unsafe.Pointer(requirements)),
	)
}

// BindImageMemory binds memory to an image.
//
// Wraps vkBindImageMemory.
func BindImageMemory(device Device, image Image, memory DeviceMemory, offset uint64) Result {
	if deviceCmds == nil || deviceCmds.bindImageMemory == 0 {
		return ErrorInitializationFailed
	}

	ret, _, _ := syscall.SyscallN(
		deviceCmds.bindImageMemory,
		uintptr(device),
		uintptr(image),
		uintptr(memory),
		uintptr(offset),
	)

	return Result(ret)
}

// CreateBuffer creates a new buffer.
//
// Wraps vkCreateBuffer.
func CreateBuffer(device Device, createInfo *BufferCreateInfo, allocator *AllocationCallbacks, buffer *Buffer) Result {
	if deviceCmds == nil || deviceCmds.createBuffer == 0 {
		return ErrorInitializationFailed
	}

	var pAllocator uintptr
	if allocator != nil {
		pAllocator = uintptr(unsafe.Pointer(allocator))
	}

	ret, _, _ := syscall.SyscallN(
		deviceCmds.createBuffer,
		uintptr(device),
		uintptr(unsafe.Pointer(createInfo)),
		pAllocator,
		uintptr(unsafe.Pointer(buffer)),
	)

	return Result(ret)
}

// DestroyBuffer destroys a buffer.
//
// Wraps vkDestroyBuffer.
func DestroyBuffer(device Device, buffer Buffer, allocator *AllocationCallbacks) {
	if deviceCmds == nil || deviceCmds.destroyBuffer == 0 {
		return
	}

	var pAllocator uintptr
	if allocator != nil {
		pAllocator = uintptr(unsafe.Pointer(allocator))
	}

	//nolint:errcheck // Vulkan void function, no return value to check
	syscall.SyscallN(
		deviceCmds.destroyBuffer,
		uintptr(device),
		uintptr(buffer),
		pAllocator,
	)
}

// CreateImage creates a new image.
//
// Wraps vkCreateImage.
func CreateImage(device Device, createInfo *ImageCreateInfo, allocator *AllocationCallbacks, image *Image) Result {
	if deviceCmds == nil || deviceCmds.createImage == 0 {
		return ErrorInitializationFailed
	}

	var pAllocator uintptr
	if allocator != nil {
		pAllocator = uintptr(unsafe.Pointer(allocator))
	}

	ret, _, _ := syscall.SyscallN(
		deviceCmds.createImage,
		uintptr(device),
		uintptr(unsafe.Pointer(createInfo)),
		pAllocator,
		uintptr(unsafe.Pointer(image)),
	)

	return Result(ret)
}

// DestroyImage destroys an image.
//
// Wraps vkDestroyImage.
func DestroyImage(device Device, image Image, allocator *AllocationCallbacks) {
	if deviceCmds == nil || deviceCmds.destroyImage == 0 {
		return
	}

	var pAllocator uintptr
	if allocator != nil {
		pAllocator = uintptr(unsafe.Pointer(allocator))
	}

	//nolint:errcheck // Vulkan void function, no return value to check
	syscall.SyscallN(
		deviceCmds.destroyImage,
		uintptr(device),
		uintptr(image),
		pAllocator,
	)
}

// FlushMappedMemoryRanges flushes mapped memory ranges.
//
// Wraps vkFlushMappedMemoryRanges.
func FlushMappedMemoryRanges(device Device, memoryRangeCount uint32, memoryRanges *MappedMemoryRange) Result {
	if deviceCmds == nil || deviceCmds.flushMappedMemoryRanges == 0 {
		return ErrorInitializationFailed
	}

	ret, _, _ := syscall.SyscallN(
		deviceCmds.flushMappedMemoryRanges,
		uintptr(device),
		uintptr(memoryRangeCount),
		uintptr(unsafe.Pointer(memoryRanges)),
	)

	return Result(ret)
}

// InvalidateMappedMemoryRanges invalidates mapped memory ranges.
//
// Wraps vkInvalidateMappedMemoryRanges.
func InvalidateMappedMemoryRanges(device Device, memoryRangeCount uint32, memoryRanges *MappedMemoryRange) Result {
	if deviceCmds == nil || deviceCmds.invalidateMappedMemoryRanges == 0 {
		return ErrorInitializationFailed
	}

	ret, _, _ := syscall.SyscallN(
		deviceCmds.invalidateMappedMemoryRanges,
		uintptr(device),
		uintptr(memoryRangeCount),
		uintptr(unsafe.Pointer(memoryRanges)),
	)

	return Result(ret)
}

// GetPhysicalDeviceMemoryProperties queries memory properties of a physical device.
//
// Wraps vkGetPhysicalDeviceMemoryProperties.
func GetPhysicalDeviceMemoryProperties(cmds *Commands, physicalDevice PhysicalDevice, properties *PhysicalDeviceMemoryProperties) {
	if cmds == nil || cmds.getPhysicalDeviceMemoryProperties == 0 {
		return
	}

	//nolint:errcheck // Vulkan void function, no return value to check
	syscall.SyscallN(
		cmds.getPhysicalDeviceMemoryProperties,
		uintptr(physicalDevice),
		uintptr(unsafe.Pointer(properties)),
	)
}
