//go:build !(js && wasm)

// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

// Manual Vulkan command wrappers for functions with signatures
// unsupported by the vk-gen generator.
// These are NOT overwritten by code generation.

package vk

import (
	"unsafe"

	"github.com/go-webgpu/goffi/ffi"
)

// CmdWriteTimestamp wraps vkCmdWriteTimestamp.
// Manual: generator cannot handle mixed handle+u32+handle+u32 signature.
func (c *Commands) CmdWriteTimestamp(commandBuffer CommandBuffer, pipelineStage PipelineStageFlagBits, queryPool QueryPool, query uint32) {
	if c.cmdWriteTimestamp == nil {
		return
	}
	args := [4]unsafe.Pointer{
		unsafe.Pointer(&commandBuffer),
		unsafe.Pointer(&pipelineStage),
		unsafe.Pointer(&queryPool),
		unsafe.Pointer(&query),
	}
	_, _ = ffi.CallFunction(&SigVoidHandleU32HandleU32, c.cmdWriteTimestamp, nil, args[:])
}

// CmdCopyQueryPoolResults wraps vkCmdCopyQueryPoolResults.
// Manual: generator cannot handle mixed handle+handle+u32+u32+handle+u64+u64+u32 signature.
func (c *Commands) CmdCopyQueryPoolResults(commandBuffer CommandBuffer, queryPool QueryPool, firstQuery, queryCount uint32, dstBuffer Buffer, dstOffset, stride uint64, flags QueryResultFlags) {
	if c.cmdCopyQueryPoolResults == nil {
		return
	}
	args := [8]unsafe.Pointer{
		unsafe.Pointer(&commandBuffer),
		unsafe.Pointer(&queryPool),
		unsafe.Pointer(&firstQuery),
		unsafe.Pointer(&queryCount),
		unsafe.Pointer(&dstBuffer),
		unsafe.Pointer(&dstOffset),
		unsafe.Pointer(&stride),
		unsafe.Pointer(&flags),
	}
	_, _ = ffi.CallFunction(&SigVoidCmdCopyQueryPoolResults, c.cmdCopyQueryPoolResults, nil, args[:])
}

// WaitSemaphores wraps vkWaitSemaphores (VK_KHR_timeline_semaphore / Vulkan 1.2).
// Manual: generator cannot handle handle+ptr+u64 signature.
func (c *Commands) WaitSemaphores(device Device, pWaitInfo *SemaphoreWaitInfo, timeout uint64) Result {
	var result int32
	args := [3]unsafe.Pointer{
		unsafe.Pointer(&device),
		unsafe.Pointer(&pWaitInfo),
		unsafe.Pointer(&timeout),
	}
	if _, err := ffi.CallFunction(&SigResultHandlePtrU64, c.waitSemaphores, unsafe.Pointer(&result), args[:]); err != nil {
		return ErrorInitializationFailed
	}
	return Result(result)
}

// === Ray tracing manual wrappers (VK_KHR_acceleration_structure) ===

// GetBufferDeviceAddress wraps vkGetBufferDeviceAddress (Vulkan 1.2 core).
// Manual: returns VkDeviceAddress (uint64), not VkResult — unsupported by generator.
func (c *Commands) GetBufferDeviceAddress(device Device, pInfo *BufferDeviceAddressInfo) DeviceAddress {
	if c.getBufferDeviceAddress == nil {
		return 0
	}
	var result uint64
	args := [2]unsafe.Pointer{
		unsafe.Pointer(&device),
		unsafe.Pointer(&pInfo),
	}
	_, _ = ffi.CallFunction(&SigU64HandlePtr, c.getBufferDeviceAddress, unsafe.Pointer(&result), args[:])
	return DeviceAddress(result)
}

// GetAccelerationStructureDeviceAddressKHR wraps vkGetAccelerationStructureDeviceAddressKHR.
// Manual: returns VkDeviceAddress (uint64), not VkResult — unsupported by generator.
func (c *Commands) GetAccelerationStructureDeviceAddressKHR(device Device, pInfo *AccelerationStructureDeviceAddressInfoKHR) DeviceAddress {
	if c.getAccelerationStructureDeviceAddressKHR == nil {
		return 0
	}
	var result uint64
	args := [2]unsafe.Pointer{
		unsafe.Pointer(&device),
		unsafe.Pointer(&pInfo),
	}
	_, _ = ffi.CallFunction(&SigU64HandlePtr, c.getAccelerationStructureDeviceAddressKHR, unsafe.Pointer(&result), args[:])
	return DeviceAddress(result)
}

// CmdBuildAccelerationStructuresKHR wraps vkCmdBuildAccelerationStructuresKHR.
// Manual: generator cannot handle void(handle, u32, ptr, ptr) with ptr-to-ptr second pointer.
func (c *Commands) CmdBuildAccelerationStructuresKHR(
	commandBuffer CommandBuffer,
	infoCount uint32,
	pInfos *AccelerationStructureBuildGeometryInfoKHR,
	ppBuildRangeInfos **AccelerationStructureBuildRangeInfoKHR,
) {
	if c.cmdBuildAccelerationStructuresKHR == nil {
		return
	}
	args := [4]unsafe.Pointer{
		unsafe.Pointer(&commandBuffer),
		unsafe.Pointer(&infoCount),
		unsafe.Pointer(&pInfos),
		unsafe.Pointer(&ppBuildRangeInfos),
	}
	_, _ = ffi.CallFunction(&SigVoidCmdBuildAccelerationStructures, c.cmdBuildAccelerationStructuresKHR, nil, args[:])
}

// CmdWriteAccelerationStructuresPropertiesKHR wraps vkCmdWriteAccelerationStructuresPropertiesKHR.
// Manual: generator cannot handle void(handle, u32, ptr, u32, handle, u32).
func (c *Commands) CmdWriteAccelerationStructuresPropertiesKHR(
	commandBuffer CommandBuffer,
	accelerationStructureCount uint32,
	pAccelerationStructures *AccelerationStructureKHR,
	queryType QueryType,
	queryPool QueryPool,
	firstQuery uint32,
) {
	if c.cmdWriteAccelerationStructuresPropertiesKHR == nil {
		return
	}
	args := [6]unsafe.Pointer{
		unsafe.Pointer(&commandBuffer),
		unsafe.Pointer(&accelerationStructureCount),
		unsafe.Pointer(&pAccelerationStructures),
		unsafe.Pointer(&queryType),
		unsafe.Pointer(&queryPool),
		unsafe.Pointer(&firstQuery),
	}
	_, _ = ffi.CallFunction(&SigVoidCmdWriteASProperties, c.cmdWriteAccelerationStructuresPropertiesKHR, nil, args[:])
}

// GetAccelerationStructureBuildSizesKHR wraps vkGetAccelerationStructureBuildSizesKHR.
// Manual: generator cannot handle void(handle, handle, ptr, ptr, ptr) with enum second arg.
func (c *Commands) GetAccelerationStructureBuildSizesKHR(
	device Device,
	buildType AccelerationStructureBuildTypeKHR,
	pBuildInfo *AccelerationStructureBuildGeometryInfoKHR,
	pMaxPrimitiveCounts *uint32,
	pSizeInfo *AccelerationStructureBuildSizesInfoKHR,
) {
	if c.getAccelerationStructureBuildSizesKHR == nil {
		return
	}
	args := [5]unsafe.Pointer{
		unsafe.Pointer(&device),
		unsafe.Pointer(&buildType),
		unsafe.Pointer(&pBuildInfo),
		unsafe.Pointer(&pMaxPrimitiveCounts),
		unsafe.Pointer(&pSizeInfo),
	}
	_, _ = ffi.CallFunction(&SigVoidHandleHandlePtrPtrPtr, c.getAccelerationStructureBuildSizesKHR, nil, args[:])
}
