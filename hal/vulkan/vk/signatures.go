// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

// Package vk provides Pure Go Vulkan bindings using goffi.
//
// This file contains CallInterface signatures that are reused across
// multiple Vulkan functions with identical parameter types.

package vk

import (
	"github.com/go-webgpu/goffi/ffi"
	"github.com/go-webgpu/goffi/types"
)

// Signature templates - reused across functions with identical signatures.
// Vulkan has ~700 functions but only ~30 unique signatures.
var (
	// === Result-returning signatures ===

	// VkResult(ptr, ptr, ptr) - vkCreateInstance, vkCreateDevice, etc.
	SigResultPtrPtrPtr types.CallInterface

	// VkResult(ptr, ptr) - vkBeginCommandBuffer, etc.
	SigResultPtrPtr types.CallInterface

	// VkResult(handle, ptr) - vkEndCommandBuffer (returns result)
	SigResultHandlePtr types.CallInterface

	// VkResult(handle) - vkEndCommandBuffer
	SigResultHandle types.CallInterface

	// VkResult(handle, u32, ptr, ptr) - vkEnumeratePhysicalDevices
	SigResultHandleU32PtrPtr types.CallInterface

	// VkResult(handle, ptr, ptr, ptr, ptr) - vkCreateGraphicsPipelines
	SigResultHandlePtrPtrPtrPtr types.CallInterface

	// VkResult(handle, u64, u32, ptr) - vkWaitForFences
	SigResultHandleU64U32Ptr types.CallInterface

	// VkResult(handle, ptr, ptr) - vkCreateCommandPool, vkCreateFence, etc.
	SigResultHandlePtrPtr types.CallInterface

	// VkResult(handle, ptr, ptr, ptr) - vkCreateSwapchainKHR, etc.
	SigResultHandlePtrPtrPtr types.CallInterface

	// VkResult(handle, handle, ptr) - vkResetCommandPool
	SigResultHandleHandlePtr types.CallInterface

	// VkResult(handle, u32, u32, ptr, ptr) - vkAllocateDescriptorSets
	SigResultHandleU32U32PtrPtr types.CallInterface

	// === Void-returning signatures ===

	// void(handle, ptr) - vkDestroyInstance, vkDestroyDevice, etc.
	SigVoidHandlePtr types.CallInterface

	// void(handle, handle, ptr) - vkDestroyPipeline, vkDestroyCommandPool, etc.
	SigVoidHandleHandlePtr types.CallInterface

	// void(handle, ptr, ptr) - vkGetPhysicalDeviceProperties, etc.
	SigVoidHandlePtrPtr types.CallInterface

	// void(handle, u32, ptr) - vkGetPhysicalDeviceQueueFamilyProperties
	SigVoidHandleU32Ptr types.CallInterface

	// void(handle) - vkCmdEndRendering
	SigVoidHandle types.CallInterface

	// void(handle, u32, u32) - vkCmdBindPipeline (handle, enum, handle)
	SigVoidHandleU32Handle types.CallInterface

	// void(handle, u32, u32, u32, u32) - vkCmdDraw
	SigVoidHandleU32x4 types.CallInterface

	// void(handle, u32, u32, u32, i32, u32) - vkCmdDrawIndexed
	SigVoidHandleU32x3I32U32 types.CallInterface

	// void(handle, u32, u32, ptr, ptr) - vkCmdBindVertexBuffers
	SigVoidHandleU32U32PtrPtr types.CallInterface

	// void(handle, handle, u64, u32) - vkCmdBindIndexBuffer
	SigVoidHandleHandleU64U32 types.CallInterface

	// void(handle, u32, u32, ptr) - vkCmdSetViewport, vkCmdSetScissor
	SigVoidHandleU32U32Ptr types.CallInterface

	// void(handle, ptr) - vkCmdSetBlendConstants
	SigVoidHandleFloatPtr types.CallInterface

	// void(handle, u32, u32, u32) - vkCmdSetStencilReference
	SigVoidHandleU32x3 types.CallInterface

	// void(handle, handle, u64, u32) - vkCmdDrawIndirect
	SigVoidHandleHandleU64U32Count types.CallInterface

	// void(handle, u32, u32, u32) - vkCmdDispatch
	SigVoidHandleU32U32U32 types.CallInterface

	// void(handle, handle, u64) - vkCmdDispatchIndirect
	SigVoidHandleHandleU64 types.CallInterface

	// void(handle, ptr) - vkCmdBeginRendering
	SigVoidHandlePtrRendering types.CallInterface

	// void(handle, u32, u32, u32, u32, u32, ptr, u32, ptr) - vkCmdBindDescriptorSets
	SigVoidCmdBindDescriptorSets types.CallInterface

	// void(handle, ...) - vkCmdPipelineBarrier (11 args)
	SigVoidCmdPipelineBarrier types.CallInterface

	// void(handle, handle, u64, u64, u32) - vkCmdFillBuffer
	SigVoidCmdFillBuffer types.CallInterface

	// void(handle, handle, handle, u32, ptr) - vkCmdCopyBuffer
	SigVoidCmdCopyBuffer types.CallInterface

	// void(handle, handle, handle, u32, u32, ptr) - vkCmdCopyBufferToImage
	SigVoidCmdCopyBufferToImage types.CallInterface

	// void(handle, handle, u32, handle, u32, ptr) - vkCmdCopyImageToBuffer
	SigVoidCmdCopyImageToBuffer types.CallInterface

	// void(handle, handle, u32, handle, u32, u32, ptr) - vkCmdCopyImage
	SigVoidCmdCopyImage types.CallInterface

	// void(device, u32, u32, ptr) - vkUpdateDescriptorSets
	SigVoidDeviceUpdateDescriptorSets types.CallInterface

	// void(device, u32, ptr) - vkGetDeviceQueue
	SigVoidDeviceU32Ptr types.CallInterface

	// VkResult(device, ptr) - vkQueueSubmit2
	SigResultDevicePtr types.CallInterface

	// VkResult(handle, u32, ptr, ptr, ptr) - vkQueueSubmit2
	SigResultHandleU32PtrPtrPtr types.CallInterface

	// Additional Result-returning signatures

	// VkResult(ptr) - vkEnumerateInstanceVersion
	SigResultPtr types.CallInterface

	// VkResult(handle, handle, u32) - vkResetCommandPool, vkResetDescriptorPool
	SigResultHandleHandleU32 types.CallInterface

	// VkResult(handle, u32) - vkResetCommandBuffer
	SigResultHandleU32 types.CallInterface

	// VkResult(handle, handle) - vkGetFenceStatus, vkSetEvent, vkResetEvent
	SigResultHandleHandle types.CallInterface

	// VkResult(handle, u32, ptr) - vkFlushMappedMemoryRanges
	SigResultHandleU32Ptr types.CallInterface

	// VkResult(handle, handle, handle, u64) - vkBindBufferMemory, vkBindImageMemory
	SigResultHandle4 types.CallInterface

	// VkResult(handle, handle, u64, u64, u32, ptr) - vkMapMemory
	SigResultMapMemory types.CallInterface

	// VkResult(handle, u32, ptr, handle) - vkQueueSubmit
	SigResultHandleU32PtrHandle types.CallInterface

	// VkResult(handle, u32, ptr, u32, u64) - vkWaitForFences
	SigResultWaitForFences types.CallInterface

	// VkResult(handle, handle, ptr, ptr) - vkGetSwapchainImagesKHR
	SigResultHandleHandlePtrPtr types.CallInterface

	// VkResult(handle, handle, u64, handle, handle, ptr) - vkAcquireNextImageKHR
	SigResultAcquireNextImage types.CallInterface

	// VkResult(handle, u32, handle, ptr) - vkGetPhysicalDeviceSurfaceSupportKHR
	SigResultHandleU32HandlePtr types.CallInterface

	// Additional Void-returning signatures

	// void(handle, handle) - vkUnmapMemory
	SigVoidHandleHandle types.CallInterface

	// void(handle, handle, handle) - vkCmdBindPipeline
	SigVoidHandleHandleHandle types.CallInterface

	// void(handle, u32, u32) - vkCmdSetStencilCompareMask, etc.
	SigVoidHandleU32U32 types.CallInterface

	// void(handle, u32) - vkCmdSetAttachmentFeedbackLoopEnableEXT
	SigVoidHandleU32 types.CallInterface

	// void(handle, handle, u32, u32) - vkCmdBeginQuery, vkResetQueryPool
	SigVoidHandleHandleU32U32 types.CallInterface

	// void(handle, handle, u32) - vkCmdSetEvent, vkCmdResetEvent
	SigVoidHandleHandleU32 types.CallInterface

	// void(handle, handle, handle, u32, u32) - vkCmdDrawIndirect
	SigVoidHandle3U32U32 types.CallInterface

	// void(handle, ptr, u32) - vkCmdBeginRenderPass
	SigVoidHandlePtrU32 types.CallInterface

	// void(handle, handle, u32, ptr) - vkFreeCommandBuffers
	SigVoidHandleHandleU32Ptr types.CallInterface

	// void(handle, f32) - vkCmdSetBlendConstants (receives ptr to float array)
	SigVoidHandleF32 types.CallInterface

	// void(handle, handle, u64, u32, u32) - vkCmdDrawIndirect
	SigVoidHandleHandleU64U32U32 types.CallInterface

	// VkResult(handle, handle, u32, ptr) - vkFreeDescriptorSets, vkFreeCommandBuffers
	SigResultHandleHandleU32Ptr types.CallInterface

	// VkResult(handle, handle, u32, ptr, ptr, ptr) - vkCreateGraphicsPipelines, vkCreateComputePipelines
	SigResultCreatePipelines types.CallInterface
)

// InitSignatures prepares all CallInterface templates.
// Must be called once after loading Vulkan library.
//
//nolint:maintidx // This function initializes all 60+ signature types - high complexity is inherent
func InitSignatures() error {
	var err error

	// Helper for common pointer type slice
	ptr := types.PointerTypeDescriptor
	u32 := types.UInt32TypeDescriptor
	u64 := types.UInt64TypeDescriptor
	i32 := types.SInt32TypeDescriptor
	voidRet := types.VoidTypeDescriptor
	resultRet := types.SInt32TypeDescriptor // VkResult is int32

	// === Result-returning signatures ===

	// VkResult(ptr, ptr, ptr)
	err = ffi.PrepareCallInterface(&SigResultPtrPtrPtr, types.DefaultCall, resultRet,
		[]*types.TypeDescriptor{ptr, ptr, ptr})
	if err != nil {
		return err
	}

	// VkResult(ptr, ptr)
	err = ffi.PrepareCallInterface(&SigResultPtrPtr, types.DefaultCall, resultRet,
		[]*types.TypeDescriptor{ptr, ptr})
	if err != nil {
		return err
	}

	// VkResult(handle, ptr)
	err = ffi.PrepareCallInterface(&SigResultHandlePtr, types.DefaultCall, resultRet,
		[]*types.TypeDescriptor{u64, ptr})
	if err != nil {
		return err
	}

	// VkResult(handle)
	err = ffi.PrepareCallInterface(&SigResultHandle, types.DefaultCall, resultRet,
		[]*types.TypeDescriptor{u64})
	if err != nil {
		return err
	}

	// VkResult(handle, u32, ptr, ptr)
	err = ffi.PrepareCallInterface(&SigResultHandleU32PtrPtr, types.DefaultCall, resultRet,
		[]*types.TypeDescriptor{u64, u32, ptr, ptr})
	if err != nil {
		return err
	}

	// VkResult(handle, ptr, ptr, ptr, ptr)
	err = ffi.PrepareCallInterface(&SigResultHandlePtrPtrPtrPtr, types.DefaultCall, resultRet,
		[]*types.TypeDescriptor{u64, ptr, ptr, ptr, ptr})
	if err != nil {
		return err
	}

	// VkResult(handle, u64, u32, ptr)
	err = ffi.PrepareCallInterface(&SigResultHandleU64U32Ptr, types.DefaultCall, resultRet,
		[]*types.TypeDescriptor{u64, u64, u32, ptr})
	if err != nil {
		return err
	}

	// VkResult(handle, ptr, ptr)
	err = ffi.PrepareCallInterface(&SigResultHandlePtrPtr, types.DefaultCall, resultRet,
		[]*types.TypeDescriptor{u64, ptr, ptr})
	if err != nil {
		return err
	}

	// VkResult(handle, ptr, ptr, ptr)
	err = ffi.PrepareCallInterface(&SigResultHandlePtrPtrPtr, types.DefaultCall, resultRet,
		[]*types.TypeDescriptor{u64, ptr, ptr, ptr})
	if err != nil {
		return err
	}

	// VkResult(handle, handle, ptr)
	err = ffi.PrepareCallInterface(&SigResultHandleHandlePtr, types.DefaultCall, resultRet,
		[]*types.TypeDescriptor{u64, u64, ptr})
	if err != nil {
		return err
	}

	// VkResult(handle, u32, u32, ptr, ptr)
	err = ffi.PrepareCallInterface(&SigResultHandleU32U32PtrPtr, types.DefaultCall, resultRet,
		[]*types.TypeDescriptor{u64, u32, u32, ptr, ptr})
	if err != nil {
		return err
	}

	// VkResult(handle, u32, ptr, ptr, ptr)
	err = ffi.PrepareCallInterface(&SigResultHandleU32PtrPtrPtr, types.DefaultCall, resultRet,
		[]*types.TypeDescriptor{u64, u32, ptr, ptr, ptr})
	if err != nil {
		return err
	}

	// VkResult(device, ptr)
	err = ffi.PrepareCallInterface(&SigResultDevicePtr, types.DefaultCall, resultRet,
		[]*types.TypeDescriptor{u64, ptr})
	if err != nil {
		return err
	}

	// === Void-returning signatures ===

	// void(handle, ptr)
	err = ffi.PrepareCallInterface(&SigVoidHandlePtr, types.DefaultCall, voidRet,
		[]*types.TypeDescriptor{u64, ptr})
	if err != nil {
		return err
	}

	// void(handle, handle, ptr)
	err = ffi.PrepareCallInterface(&SigVoidHandleHandlePtr, types.DefaultCall, voidRet,
		[]*types.TypeDescriptor{u64, u64, ptr})
	if err != nil {
		return err
	}

	// void(handle, ptr, ptr)
	err = ffi.PrepareCallInterface(&SigVoidHandlePtrPtr, types.DefaultCall, voidRet,
		[]*types.TypeDescriptor{u64, ptr, ptr})
	if err != nil {
		return err
	}

	// void(handle, u32, ptr)
	err = ffi.PrepareCallInterface(&SigVoidHandleU32Ptr, types.DefaultCall, voidRet,
		[]*types.TypeDescriptor{u64, u32, ptr})
	if err != nil {
		return err
	}

	// void(handle)
	err = ffi.PrepareCallInterface(&SigVoidHandle, types.DefaultCall, voidRet,
		[]*types.TypeDescriptor{u64})
	if err != nil {
		return err
	}

	// void(handle, u32, handle)
	err = ffi.PrepareCallInterface(&SigVoidHandleU32Handle, types.DefaultCall, voidRet,
		[]*types.TypeDescriptor{u64, u32, u64})
	if err != nil {
		return err
	}

	// void(handle, u32, u32, u32, u32)
	err = ffi.PrepareCallInterface(&SigVoidHandleU32x4, types.DefaultCall, voidRet,
		[]*types.TypeDescriptor{u64, u32, u32, u32, u32})
	if err != nil {
		return err
	}

	// void(handle, u32, u32, u32, i32, u32)
	err = ffi.PrepareCallInterface(&SigVoidHandleU32x3I32U32, types.DefaultCall, voidRet,
		[]*types.TypeDescriptor{u64, u32, u32, u32, i32, u32})
	if err != nil {
		return err
	}

	// void(handle, u32, u32, ptr, ptr)
	err = ffi.PrepareCallInterface(&SigVoidHandleU32U32PtrPtr, types.DefaultCall, voidRet,
		[]*types.TypeDescriptor{u64, u32, u32, ptr, ptr})
	if err != nil {
		return err
	}

	// void(handle, handle, u64, u32)
	err = ffi.PrepareCallInterface(&SigVoidHandleHandleU64U32, types.DefaultCall, voidRet,
		[]*types.TypeDescriptor{u64, u64, u64, u32})
	if err != nil {
		return err
	}

	// void(handle, u32, u32, ptr)
	err = ffi.PrepareCallInterface(&SigVoidHandleU32U32Ptr, types.DefaultCall, voidRet,
		[]*types.TypeDescriptor{u64, u32, u32, ptr})
	if err != nil {
		return err
	}

	// void(handle, ptr) for floats
	err = ffi.PrepareCallInterface(&SigVoidHandleFloatPtr, types.DefaultCall, voidRet,
		[]*types.TypeDescriptor{u64, ptr})
	if err != nil {
		return err
	}

	// void(handle, u32, u32, u32)
	err = ffi.PrepareCallInterface(&SigVoidHandleU32x3, types.DefaultCall, voidRet,
		[]*types.TypeDescriptor{u64, u32, u32, u32})
	if err != nil {
		return err
	}

	// void(handle, handle, u64, u32) for draw indirect
	err = ffi.PrepareCallInterface(&SigVoidHandleHandleU64U32Count, types.DefaultCall, voidRet,
		[]*types.TypeDescriptor{u64, u64, u64, u32})
	if err != nil {
		return err
	}

	// void(handle, u32, u32, u32) for dispatch
	err = ffi.PrepareCallInterface(&SigVoidHandleU32U32U32, types.DefaultCall, voidRet,
		[]*types.TypeDescriptor{u64, u32, u32, u32})
	if err != nil {
		return err
	}

	// void(handle, handle, u64)
	err = ffi.PrepareCallInterface(&SigVoidHandleHandleU64, types.DefaultCall, voidRet,
		[]*types.TypeDescriptor{u64, u64, u64})
	if err != nil {
		return err
	}

	// void(handle, ptr) for rendering
	err = ffi.PrepareCallInterface(&SigVoidHandlePtrRendering, types.DefaultCall, voidRet,
		[]*types.TypeDescriptor{u64, ptr})
	if err != nil {
		return err
	}

	// void(handle, u32, u32, u32, u32, u32, ptr, u32, ptr) - vkCmdBindDescriptorSets
	err = ffi.PrepareCallInterface(&SigVoidCmdBindDescriptorSets, types.DefaultCall, voidRet,
		[]*types.TypeDescriptor{u64, u32, u64, u32, u32, ptr, u32, ptr})
	if err != nil {
		return err
	}

	// void(handle, u32, u32, u32, u32, u32, u32, ptr, u32, ptr, u32, ptr) - vkCmdPipelineBarrier
	err = ffi.PrepareCallInterface(&SigVoidCmdPipelineBarrier, types.DefaultCall, voidRet,
		[]*types.TypeDescriptor{u64, u32, u32, u32, u32, ptr, u32, ptr, u32, ptr})
	if err != nil {
		return err
	}

	// void(handle, handle, u64, u64, u32) - vkCmdFillBuffer
	err = ffi.PrepareCallInterface(&SigVoidCmdFillBuffer, types.DefaultCall, voidRet,
		[]*types.TypeDescriptor{u64, u64, u64, u64, u32})
	if err != nil {
		return err
	}

	// void(handle, handle, handle, u32, ptr) - vkCmdCopyBuffer
	err = ffi.PrepareCallInterface(&SigVoidCmdCopyBuffer, types.DefaultCall, voidRet,
		[]*types.TypeDescriptor{u64, u64, u64, u32, ptr})
	if err != nil {
		return err
	}

	// void(handle, handle, handle, u32, u32, ptr) - vkCmdCopyBufferToImage
	err = ffi.PrepareCallInterface(&SigVoidCmdCopyBufferToImage, types.DefaultCall, voidRet,
		[]*types.TypeDescriptor{u64, u64, u64, u32, u32, ptr})
	if err != nil {
		return err
	}

	// void(handle, handle, u32, handle, u32, ptr) - vkCmdCopyImageToBuffer
	err = ffi.PrepareCallInterface(&SigVoidCmdCopyImageToBuffer, types.DefaultCall, voidRet,
		[]*types.TypeDescriptor{u64, u64, u32, u64, u32, ptr})
	if err != nil {
		return err
	}

	// void(handle, handle, u32, handle, u32, u32, ptr) - vkCmdCopyImage
	err = ffi.PrepareCallInterface(&SigVoidCmdCopyImage, types.DefaultCall, voidRet,
		[]*types.TypeDescriptor{u64, u64, u32, u64, u32, u32, ptr})
	if err != nil {
		return err
	}

	// void(device, u32, u32, ptr) - vkUpdateDescriptorSets
	err = ffi.PrepareCallInterface(&SigVoidDeviceUpdateDescriptorSets, types.DefaultCall, voidRet,
		[]*types.TypeDescriptor{u64, u32, ptr, u32, ptr})
	if err != nil {
		return err
	}

	// void(device, u32, ptr) - vkGetDeviceQueue
	err = ffi.PrepareCallInterface(&SigVoidDeviceU32Ptr, types.DefaultCall, voidRet,
		[]*types.TypeDescriptor{u64, u32, u32, ptr})
	if err != nil {
		return err
	}

	// === Additional Result-returning signatures ===

	// VkResult(ptr)
	err = ffi.PrepareCallInterface(&SigResultPtr, types.DefaultCall, resultRet,
		[]*types.TypeDescriptor{ptr})
	if err != nil {
		return err
	}

	// VkResult(handle, handle, u32)
	err = ffi.PrepareCallInterface(&SigResultHandleHandleU32, types.DefaultCall, resultRet,
		[]*types.TypeDescriptor{u64, u64, u32})
	if err != nil {
		return err
	}

	// VkResult(handle, u32)
	err = ffi.PrepareCallInterface(&SigResultHandleU32, types.DefaultCall, resultRet,
		[]*types.TypeDescriptor{u64, u32})
	if err != nil {
		return err
	}

	// VkResult(handle, handle)
	err = ffi.PrepareCallInterface(&SigResultHandleHandle, types.DefaultCall, resultRet,
		[]*types.TypeDescriptor{u64, u64})
	if err != nil {
		return err
	}

	// VkResult(handle, u32, ptr)
	err = ffi.PrepareCallInterface(&SigResultHandleU32Ptr, types.DefaultCall, resultRet,
		[]*types.TypeDescriptor{u64, u32, ptr})
	if err != nil {
		return err
	}

	// VkResult(handle, handle, handle, u64) - vkBindBufferMemory
	err = ffi.PrepareCallInterface(&SigResultHandle4, types.DefaultCall, resultRet,
		[]*types.TypeDescriptor{u64, u64, u64, u64})
	if err != nil {
		return err
	}

	// VkResult(handle, handle, u64, u64, u32, ptr) - vkMapMemory
	err = ffi.PrepareCallInterface(&SigResultMapMemory, types.DefaultCall, resultRet,
		[]*types.TypeDescriptor{u64, u64, u64, u64, u32, ptr})
	if err != nil {
		return err
	}

	// VkResult(handle, u32, ptr, handle) - vkQueueSubmit
	err = ffi.PrepareCallInterface(&SigResultHandleU32PtrHandle, types.DefaultCall, resultRet,
		[]*types.TypeDescriptor{u64, u32, ptr, u64})
	if err != nil {
		return err
	}

	// VkResult(handle, u32, ptr, u32, u64) - vkWaitForFences
	err = ffi.PrepareCallInterface(&SigResultWaitForFences, types.DefaultCall, resultRet,
		[]*types.TypeDescriptor{u64, u32, ptr, u32, u64})
	if err != nil {
		return err
	}

	// VkResult(handle, handle, ptr, ptr)
	err = ffi.PrepareCallInterface(&SigResultHandleHandlePtrPtr, types.DefaultCall, resultRet,
		[]*types.TypeDescriptor{u64, u64, ptr, ptr})
	if err != nil {
		return err
	}

	// VkResult(handle, handle, u64, handle, handle, ptr) - vkAcquireNextImageKHR
	err = ffi.PrepareCallInterface(&SigResultAcquireNextImage, types.DefaultCall, resultRet,
		[]*types.TypeDescriptor{u64, u64, u64, u64, u64, ptr})
	if err != nil {
		return err
	}

	// VkResult(handle, u32, handle, ptr) - vkGetPhysicalDeviceSurfaceSupportKHR
	err = ffi.PrepareCallInterface(&SigResultHandleU32HandlePtr, types.DefaultCall, resultRet,
		[]*types.TypeDescriptor{u64, u32, u64, ptr})
	if err != nil {
		return err
	}

	// === Additional Void-returning signatures ===

	// void(handle, handle)
	err = ffi.PrepareCallInterface(&SigVoidHandleHandle, types.DefaultCall, voidRet,
		[]*types.TypeDescriptor{u64, u64})
	if err != nil {
		return err
	}

	// void(handle, handle, handle)
	err = ffi.PrepareCallInterface(&SigVoidHandleHandleHandle, types.DefaultCall, voidRet,
		[]*types.TypeDescriptor{u64, u64, u64})
	if err != nil {
		return err
	}

	// void(handle, u32, u32)
	err = ffi.PrepareCallInterface(&SigVoidHandleU32U32, types.DefaultCall, voidRet,
		[]*types.TypeDescriptor{u64, u32, u32})
	if err != nil {
		return err
	}

	// void(handle, u32)
	err = ffi.PrepareCallInterface(&SigVoidHandleU32, types.DefaultCall, voidRet,
		[]*types.TypeDescriptor{u64, u32})
	if err != nil {
		return err
	}

	// void(handle, handle, u32, u32)
	err = ffi.PrepareCallInterface(&SigVoidHandleHandleU32U32, types.DefaultCall, voidRet,
		[]*types.TypeDescriptor{u64, u64, u32, u32})
	if err != nil {
		return err
	}

	// void(handle, handle, u32)
	err = ffi.PrepareCallInterface(&SigVoidHandleHandleU32, types.DefaultCall, voidRet,
		[]*types.TypeDescriptor{u64, u64, u32})
	if err != nil {
		return err
	}

	// void(handle, handle, handle, u32, u32)
	err = ffi.PrepareCallInterface(&SigVoidHandle3U32U32, types.DefaultCall, voidRet,
		[]*types.TypeDescriptor{u64, u64, u64, u32, u32})
	if err != nil {
		return err
	}

	// void(handle, ptr, u32)
	err = ffi.PrepareCallInterface(&SigVoidHandlePtrU32, types.DefaultCall, voidRet,
		[]*types.TypeDescriptor{u64, ptr, u32})
	if err != nil {
		return err
	}

	// void(handle, handle, u32, ptr)
	err = ffi.PrepareCallInterface(&SigVoidHandleHandleU32Ptr, types.DefaultCall, voidRet,
		[]*types.TypeDescriptor{u64, u64, u32, ptr})
	if err != nil {
		return err
	}

	// void(handle, f32) - for blend constants (ptr)
	f32 := types.FloatTypeDescriptor
	err = ffi.PrepareCallInterface(&SigVoidHandleF32, types.DefaultCall, voidRet,
		[]*types.TypeDescriptor{u64, f32})
	if err != nil {
		return err
	}

	// void(handle, handle, u64, u32, u32)
	err = ffi.PrepareCallInterface(&SigVoidHandleHandleU64U32U32, types.DefaultCall, voidRet,
		[]*types.TypeDescriptor{u64, u64, u64, u32, u32})
	if err != nil {
		return err
	}

	// VkResult(handle, handle, u32, ptr) - vkFreeDescriptorSets
	err = ffi.PrepareCallInterface(&SigResultHandleHandleU32Ptr, types.DefaultCall, resultRet,
		[]*types.TypeDescriptor{u64, u64, u32, ptr})
	if err != nil {
		return err
	}

	// VkResult(handle, handle, u32, ptr, ptr, ptr) - vkCreateGraphicsPipelines
	err = ffi.PrepareCallInterface(&SigResultCreatePipelines, types.DefaultCall, resultRet,
		[]*types.TypeDescriptor{u64, u64, u32, ptr, ptr, ptr})
	if err != nil {
		return err
	}

	return nil
}
