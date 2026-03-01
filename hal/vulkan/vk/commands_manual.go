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
	_ = ffi.CallFunction(&SigVoidHandleU32HandleU32, c.cmdWriteTimestamp, nil, args[:])
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
	_ = ffi.CallFunction(&SigVoidCmdCopyQueryPoolResults, c.cmdCopyQueryPoolResults, nil, args[:])
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
	if err := ffi.CallFunction(&SigResultHandlePtrU64, c.waitSemaphores, unsafe.Pointer(&result), args[:]); err != nil {
		return ErrorInitializationFailed
	}
	return Result(result)
}
