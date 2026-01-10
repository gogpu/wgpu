// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows

package main

import (
	"fmt"
	"syscall"
	"unsafe"

	"github.com/go-webgpu/goffi/ffi"
	"github.com/go-webgpu/goffi/types"
	"github.com/gogpu/wgpu/hal/vulkan/vk"
)

// DirectCreatePipeline tests direct FFI call to vkCreateGraphicsPipelines
// bypassing our wrapper to isolate if the issue is in the wrapper or goffi.
func DirectCreatePipeline(cmds *vk.Commands, device vk.Device, createInfo *vk.GraphicsPipelineCreateInfo) (vk.Pipeline, vk.Result, error) {
	// Get the function pointer
	fnPtr := cmds.DebugFunctionPointer("vkCreateGraphicsPipelines")
	if fnPtr == nil {
		return 0, 0, fmt.Errorf("vkCreateGraphicsPipelines not loaded")
	}

	// Prepare call interface manually
	var cif types.CallInterface
	resultRet := types.SInt32TypeDescriptor
	u64 := types.UInt64TypeDescriptor
	u32 := types.UInt32TypeDescriptor
	ptr := types.PointerTypeDescriptor

	err := ffi.PrepareCallInterface(&cif, types.DefaultCall, resultRet,
		[]*types.TypeDescriptor{u64, u64, u32, ptr, ptr, ptr})
	if err != nil {
		return 0, 0, fmt.Errorf("PrepareCallInterface failed: %w", err)
	}

	// Prepare arguments
	pipelineCache := vk.PipelineCache(0)
	createInfoCount := uint32(1)
	var allocator unsafe.Pointer // nil
	var pipeline vk.Pipeline

	// For goffi: args[i] must be pointer to WHERE the value is stored
	createInfoPtr := unsafe.Pointer(createInfo)
	allocatorPtr := allocator
	pipelinePtr := unsafe.Pointer(&pipeline)

	args := [6]unsafe.Pointer{
		unsafe.Pointer(&device),
		unsafe.Pointer(&pipelineCache),
		unsafe.Pointer(&createInfoCount),
		unsafe.Pointer(&createInfoPtr),
		unsafe.Pointer(&allocatorPtr),
		unsafe.Pointer(&pipelinePtr),
	}

	var result int32
	fmt.Printf("    Direct FFI call:\n")
	fmt.Printf("      fnPtr: %p\n", fnPtr)
	fmt.Printf("      device: 0x%X\n", device)
	fmt.Printf("      createInfoPtr: %p\n", createInfoPtr)
	fmt.Printf("      pipelinePtr (output): %p, value: 0x%X\n", pipelinePtr, pipeline)

	_ = ffi.CallFunction(&cif, fnPtr, unsafe.Pointer(&result), args[:])

	fmt.Printf("      result: %d\n", result)
	fmt.Printf("      pipeline after call: 0x%X\n", pipeline)

	return pipeline, vk.Result(result), nil
}

// SyscallCreatePipeline tests using Windows syscall directly
// to completely bypass goffi and test if it's a goffi issue.
func SyscallCreatePipeline(cmds *vk.Commands, device vk.Device, createInfo *vk.GraphicsPipelineCreateInfo) (vk.Pipeline, vk.Result, error) {
	// Get the function pointer
	fnPtr := cmds.DebugFunctionPointer("vkCreateGraphicsPipelines")
	if fnPtr == nil {
		return 0, 0, fmt.Errorf("vkCreateGraphicsPipelines not loaded")
	}

	// Prepare arguments for syscall6
	// VkResult vkCreateGraphicsPipelines(
	//   VkDevice device,
	//   VkPipelineCache pipelineCache,
	//   uint32_t createInfoCount,
	//   const VkGraphicsPipelineCreateInfo* pCreateInfos,
	//   const VkAllocationCallbacks* pAllocator,
	//   VkPipeline* pPipelines
	// )

	var pipeline vk.Pipeline

	fmt.Println("    Syscall6 call:")
	fmt.Printf("      fnPtr: %p\n", fnPtr)
	fmt.Printf("      device: 0x%X\n", device)
	fmt.Printf("      createInfo: %p\n", createInfo)
	fmt.Printf("      pipeline addr: %p\n", &pipeline)

	// Call via syscall6
	r1, _, _ := syscall.SyscallN(
		uintptr(fnPtr),
		uintptr(device),
		0, // pipelineCache = VK_NULL_HANDLE
		1, // createInfoCount = 1
		uintptr(unsafe.Pointer(createInfo)),
		0, // pAllocator = NULL
		uintptr(unsafe.Pointer(&pipeline)),
	)

	result := vk.Result(int32(r1))
	fmt.Printf("      result: %d\n", result)
	fmt.Printf("      pipeline: 0x%X\n", pipeline)

	return pipeline, result, nil
}
