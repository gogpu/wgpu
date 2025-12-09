// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows

package vulkan

import (
	"fmt"
	"syscall"
	"unsafe"

	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/vulkan/vk"
)

// Queue implements hal.Queue for Vulkan.
type Queue struct {
	handle      vk.Queue
	device      *Device
	familyIndex uint32
}

// Submit submits command buffers to the GPU.
func (q *Queue) Submit(commandBuffers []hal.CommandBuffer, fence hal.Fence, fenceValue uint64) error {
	if len(commandBuffers) == 0 {
		return nil
	}

	// Convert command buffers to Vulkan handles
	vkCmdBuffers := make([]vk.CommandBuffer, len(commandBuffers))
	for i, cb := range commandBuffers {
		vkCB, ok := cb.(*CommandBuffer)
		if !ok {
			return fmt.Errorf("vulkan: command buffer is not a Vulkan command buffer")
		}
		vkCmdBuffers[i] = vkCB.handle
	}

	// Get wait/signal semaphores from surface if this is a present submit
	var waitSemaphore, signalSemaphore vk.Semaphore
	waitStage := vk.PipelineStageFlags(vk.PipelineStageColorAttachmentOutputBit)

	// Check if any command buffer was used with a swapchain texture
	// For now, we assume no synchronization needed without explicit fence
	submitInfo := vk.SubmitInfo{
		SType:              vk.StructureTypeSubmitInfo,
		CommandBufferCount: uint32(len(vkCmdBuffers)),
		PCommandBuffers:    &vkCmdBuffers[0],
	}

	// If we have semaphores from a swapchain, add them
	if waitSemaphore != 0 {
		submitInfo.WaitSemaphoreCount = 1
		submitInfo.PWaitSemaphores = &waitSemaphore
		submitInfo.PWaitDstStageMask = &waitStage
	}
	if signalSemaphore != 0 {
		submitInfo.SignalSemaphoreCount = 1
		submitInfo.PSignalSemaphores = &signalSemaphore
	}

	// Get fence handle if provided
	var vkFence vk.Fence
	if fence != nil {
		if vkF, ok := fence.(*Fence); ok {
			vkFence = vkF.handle
		}
	}

	result := vkQueueSubmit(q, 1, &submitInfo, vkFence)
	if result != vk.Success {
		return fmt.Errorf("vulkan: vkQueueSubmit failed: %d", result)
	}

	return nil
}

// SubmitForPresent submits command buffers with swapchain synchronization.
func (q *Queue) SubmitForPresent(commandBuffers []hal.CommandBuffer, swapchain *Swapchain) error {
	if len(commandBuffers) == 0 {
		return nil
	}

	// Convert command buffers to Vulkan handles
	vkCmdBuffers := make([]vk.CommandBuffer, len(commandBuffers))
	for i, cb := range commandBuffers {
		vkCB, ok := cb.(*CommandBuffer)
		if !ok {
			return fmt.Errorf("vulkan: command buffer is not a Vulkan command buffer")
		}
		vkCmdBuffers[i] = vkCB.handle
	}

	waitStage := vk.PipelineStageFlags(vk.PipelineStageColorAttachmentOutputBit)

	submitInfo := vk.SubmitInfo{
		SType:                vk.StructureTypeSubmitInfo,
		WaitSemaphoreCount:   1,
		PWaitSemaphores:      &swapchain.imageAvailable,
		PWaitDstStageMask:    &waitStage,
		CommandBufferCount:   uint32(len(vkCmdBuffers)),
		PCommandBuffers:      &vkCmdBuffers[0],
		SignalSemaphoreCount: 1,
		PSignalSemaphores:    &swapchain.renderFinished,
	}

	result := vkQueueSubmit(q, 1, &submitInfo, 0)
	if result != vk.Success {
		return fmt.Errorf("vulkan: vkQueueSubmit failed: %d", result)
	}

	return nil
}

// WriteBuffer writes data to a buffer immediately.
func (q *Queue) WriteBuffer(buffer hal.Buffer, offset uint64, data []byte) {
	vkBuffer, ok := buffer.(*Buffer)
	if !ok || vkBuffer.memory == nil {
		return
	}

	// Map, copy, unmap
	if vkBuffer.memory.MappedPtr != 0 {
		// Already mapped - direct copy using Vulkan mapped memory from vkMapMemory
		// Use copyToMappedMemory to avoid go vet false positive about unsafe.Pointer
		copyToMappedMemory(vkBuffer.memory.MappedPtr, offset, data)
	}
	// TODO: Implement staging buffer for non-host-visible memory
}

// WriteTexture writes data to a texture immediately.
func (q *Queue) WriteTexture(dst *hal.ImageCopyTexture, data []byte, layout *hal.ImageDataLayout, size *hal.Extent3D) {
	// TODO: Implement staging buffer to image copy
}

// Present presents a surface texture to the screen.
func (q *Queue) Present(surface hal.Surface, texture hal.SurfaceTexture) error {
	vkSurface, ok := surface.(*Surface)
	if !ok {
		return fmt.Errorf("vulkan: surface is not a Vulkan surface")
	}

	if vkSurface.swapchain == nil {
		return fmt.Errorf("vulkan: surface not configured")
	}

	return vkSurface.swapchain.present(q)
}

// GetTimestampPeriod returns the timestamp period in nanoseconds.
func (q *Queue) GetTimestampPeriod() float32 {
	// TODO: Get from physical device properties
	return 1.0
}

// Vulkan function wrapper

func vkQueueSubmit(q *Queue, submitCount uint32, submits *vk.SubmitInfo, fence vk.Fence) vk.Result {
	proc := vk.GetDeviceProcAddr(q.device.handle, "vkQueueSubmit")
	if proc == 0 {
		return vk.ErrorExtensionNotPresent
	}
	r, _, _ := syscall.SyscallN(proc,
		uintptr(q.handle),
		uintptr(submitCount),
		uintptr(unsafe.Pointer(submits)),
		uintptr(fence))
	return vk.Result(r)
}
