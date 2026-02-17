// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vk

// This file contains manual function pointer loading for Vulkan commands.
// The Commands struct fields are defined in commands_gen.go (auto-generated).
//
// # Function Loading Hierarchy
//
// Vulkan functions are loaded in three stages:
//
//  1. LoadGlobal() — Functions callable without instance (pre-instance)
//     - vkCreateInstance
//     - vkEnumerateInstanceVersion
//     - vkEnumerateInstanceLayerProperties
//     - vkEnumerateInstanceExtensionProperties
//
//  2. LoadInstance(instance) — Instance-level functions
//     - Core: vkDestroyInstance, vkEnumeratePhysicalDevices, vkCreateDevice
//     - WSI:  vkCreateWin32SurfaceKHR, vkGetPhysicalDeviceSurfaceSupportKHR, etc.
//     - Note: Also call SetDeviceProcAddr(instance) for Intel compatibility
//
//  3. LoadDevice(device) — Device-level functions
//     - Memory: vkAllocateMemory, vkFreeMemory, vkMapMemory
//     - Buffers: vkCreateBuffer, vkDestroyBuffer
//     - Images: vkCreateImage, vkCreateImageView
//     - Pipelines: vkCreateGraphicsPipelines, vkCreateComputePipelines
//     - Commands: vkBeginCommandBuffer, vkCmdDraw, etc.
//     - Swapchain: vkCreateSwapchainKHR, vkAcquireNextImageKHR, vkQueuePresentKHR
//
// # Intel Driver Notes
//
// Intel Iris Xe drivers require special handling:
//   - vkGetInstanceProcAddr(NULL, "vkGetDeviceProcAddr") returns NULL
//   - Must call SetDeviceProcAddr(instance) after creating instance
//   - See loader.go for details

import (
	"fmt"
	"unsafe"
)

// NewCommands creates a new Commands instance.
// Function pointers must be loaded via LoadGlobal() and LoadInstance() before use.
func NewCommands() *Commands {
	return &Commands{}
}

// LoadGlobal loads global Vulkan function pointers.
// These are functions that can be called without an instance (like vkCreateInstance).
func (c *Commands) LoadGlobal() error {
	// Load vkCreateInstance
	c.createInstance = GetInstanceProcAddr(0, "vkCreateInstance")
	if c.createInstance == nil {
		return fmt.Errorf("failed to load vkCreateInstance")
	}

	// Load vkEnumerateInstanceVersion
	c.enumerateInstanceVersion = GetInstanceProcAddr(0, "vkEnumerateInstanceVersion")

	// Load vkEnumerateInstanceLayerProperties
	c.enumerateInstanceLayerProperties = GetInstanceProcAddr(0, "vkEnumerateInstanceLayerProperties")

	// Load vkEnumerateInstanceExtensionProperties
	c.enumerateInstanceExtensionProperties = GetInstanceProcAddr(0, "vkEnumerateInstanceExtensionProperties")

	return nil
}

// LoadInstance loads instance-level Vulkan function pointers.
// Must be called after vkCreateInstance succeeds.
func (c *Commands) LoadInstance(instance Instance) error {
	if instance == 0 {
		return fmt.Errorf("invalid instance handle")
	}

	// Load all instance-level functions
	c.destroyInstance = GetInstanceProcAddr(instance, "vkDestroyInstance")
	c.enumeratePhysicalDevices = GetInstanceProcAddr(instance, "vkEnumeratePhysicalDevices")
	c.getPhysicalDeviceProperties = GetInstanceProcAddr(instance, "vkGetPhysicalDeviceProperties")
	c.getPhysicalDeviceQueueFamilyProperties = GetInstanceProcAddr(instance, "vkGetPhysicalDeviceQueueFamilyProperties")
	c.getPhysicalDeviceMemoryProperties = GetInstanceProcAddr(instance, "vkGetPhysicalDeviceMemoryProperties")
	c.getPhysicalDeviceFeatures = GetInstanceProcAddr(instance, "vkGetPhysicalDeviceFeatures")
	c.getPhysicalDeviceFormatProperties = GetInstanceProcAddr(instance, "vkGetPhysicalDeviceFormatProperties")
	c.getPhysicalDeviceImageFormatProperties = GetInstanceProcAddr(instance, "vkGetPhysicalDeviceImageFormatProperties")
	c.createDevice = GetInstanceProcAddr(instance, "vkCreateDevice")
	c.getDeviceProcAddr = GetInstanceProcAddr(instance, "vkGetDeviceProcAddr")
	c.enumerateDeviceLayerProperties = GetInstanceProcAddr(instance, "vkEnumerateDeviceLayerProperties")
	c.enumerateDeviceExtensionProperties = GetInstanceProcAddr(instance, "vkEnumerateDeviceExtensionProperties")
	c.getPhysicalDeviceSparseImageFormatProperties = GetInstanceProcAddr(instance, "vkGetPhysicalDeviceSparseImageFormatProperties")

	// Load WSI (Window System Integration) functions
	c.destroySurfaceKHR = GetInstanceProcAddr(instance, "vkDestroySurfaceKHR")
	c.getPhysicalDeviceSurfaceSupportKHR = GetInstanceProcAddr(instance, "vkGetPhysicalDeviceSurfaceSupportKHR")
	c.getPhysicalDeviceSurfaceCapabilitiesKHR = GetInstanceProcAddr(instance, "vkGetPhysicalDeviceSurfaceCapabilitiesKHR")
	c.getPhysicalDeviceSurfaceFormatsKHR = GetInstanceProcAddr(instance, "vkGetPhysicalDeviceSurfaceFormatsKHR")
	c.getPhysicalDeviceSurfacePresentModesKHR = GetInstanceProcAddr(instance, "vkGetPhysicalDeviceSurfacePresentModesKHR")

	// Platform-specific surface creation
	c.createWin32SurfaceKHR = GetInstanceProcAddr(instance, "vkCreateWin32SurfaceKHR")

	// Vulkan 1.1+ instance functions
	c.getPhysicalDeviceFeatures2 = GetInstanceProcAddr(instance, "vkGetPhysicalDeviceFeatures2")
	c.getPhysicalDeviceProperties2 = GetInstanceProcAddr(instance, "vkGetPhysicalDeviceProperties2")

	// Verify critical functions loaded
	if c.destroyInstance == nil || c.enumeratePhysicalDevices == nil || c.createDevice == nil {
		return fmt.Errorf("failed to load critical instance functions")
	}

	return nil
}

// LoadDevice loads device-level Vulkan function pointers.
// Must be called after vkCreateDevice succeeds.
func (c *Commands) LoadDevice(device Device) error {
	if device == 0 {
		return fmt.Errorf("invalid device handle")
	}

	// Load device-level functions via vkGetDeviceProcAddr
	// For now, use GetDeviceProcAddr from loader.go
	c.destroyDevice = GetDeviceProcAddr(device, "vkDestroyDevice")
	c.getDeviceQueue = GetDeviceProcAddr(device, "vkGetDeviceQueue")
	c.queueSubmit = GetDeviceProcAddr(device, "vkQueueSubmit")
	c.queueWaitIdle = GetDeviceProcAddr(device, "vkQueueWaitIdle")
	c.deviceWaitIdle = GetDeviceProcAddr(device, "vkDeviceWaitIdle")
	c.allocateMemory = GetDeviceProcAddr(device, "vkAllocateMemory")
	c.freeMemory = GetDeviceProcAddr(device, "vkFreeMemory")
	c.mapMemory = GetDeviceProcAddr(device, "vkMapMemory")
	c.unmapMemory = GetDeviceProcAddr(device, "vkUnmapMemory")
	c.flushMappedMemoryRanges = GetDeviceProcAddr(device, "vkFlushMappedMemoryRanges")
	c.invalidateMappedMemoryRanges = GetDeviceProcAddr(device, "vkInvalidateMappedMemoryRanges")
	c.getDeviceMemoryCommitment = GetDeviceProcAddr(device, "vkGetDeviceMemoryCommitment")
	c.getBufferMemoryRequirements = GetDeviceProcAddr(device, "vkGetBufferMemoryRequirements")
	c.bindBufferMemory = GetDeviceProcAddr(device, "vkBindBufferMemory")
	c.getImageMemoryRequirements = GetDeviceProcAddr(device, "vkGetImageMemoryRequirements")
	c.bindImageMemory = GetDeviceProcAddr(device, "vkBindImageMemory")
	c.getImageSparseMemoryRequirements = GetDeviceProcAddr(device, "vkGetImageSparseMemoryRequirements")
	c.queueBindSparse = GetDeviceProcAddr(device, "vkQueueBindSparse")
	c.createFence = GetDeviceProcAddr(device, "vkCreateFence")
	c.destroyFence = GetDeviceProcAddr(device, "vkDestroyFence")
	c.resetFences = GetDeviceProcAddr(device, "vkResetFences")
	c.getFenceStatus = GetDeviceProcAddr(device, "vkGetFenceStatus")
	c.waitForFences = GetDeviceProcAddr(device, "vkWaitForFences")
	c.createSemaphore = GetDeviceProcAddr(device, "vkCreateSemaphore")
	c.destroySemaphore = GetDeviceProcAddr(device, "vkDestroySemaphore")
	c.createEvent = GetDeviceProcAddr(device, "vkCreateEvent")
	c.destroyEvent = GetDeviceProcAddr(device, "vkDestroyEvent")
	c.getEventStatus = GetDeviceProcAddr(device, "vkGetEventStatus")
	c.setEvent = GetDeviceProcAddr(device, "vkSetEvent")
	c.resetEvent = GetDeviceProcAddr(device, "vkResetEvent")
	c.createQueryPool = GetDeviceProcAddr(device, "vkCreateQueryPool")
	c.destroyQueryPool = GetDeviceProcAddr(device, "vkDestroyQueryPool")
	c.getQueryPoolResults = GetDeviceProcAddr(device, "vkGetQueryPoolResults")
	c.resetQueryPool = GetDeviceProcAddr(device, "vkResetQueryPool")
	c.createBuffer = GetDeviceProcAddr(device, "vkCreateBuffer")
	c.destroyBuffer = GetDeviceProcAddr(device, "vkDestroyBuffer")
	c.createBufferView = GetDeviceProcAddr(device, "vkCreateBufferView")
	c.destroyBufferView = GetDeviceProcAddr(device, "vkDestroyBufferView")
	c.createImage = GetDeviceProcAddr(device, "vkCreateImage")
	c.destroyImage = GetDeviceProcAddr(device, "vkDestroyImage")
	c.getImageSubresourceLayout = GetDeviceProcAddr(device, "vkGetImageSubresourceLayout")
	c.createImageView = GetDeviceProcAddr(device, "vkCreateImageView")
	c.destroyImageView = GetDeviceProcAddr(device, "vkDestroyImageView")
	c.createShaderModule = GetDeviceProcAddr(device, "vkCreateShaderModule")
	c.destroyShaderModule = GetDeviceProcAddr(device, "vkDestroyShaderModule")
	c.createPipelineCache = GetDeviceProcAddr(device, "vkCreatePipelineCache")
	c.destroyPipelineCache = GetDeviceProcAddr(device, "vkDestroyPipelineCache")
	c.getPipelineCacheData = GetDeviceProcAddr(device, "vkGetPipelineCacheData")
	c.mergePipelineCaches = GetDeviceProcAddr(device, "vkMergePipelineCaches")
	c.createGraphicsPipelines = GetDeviceProcAddr(device, "vkCreateGraphicsPipelines")
	c.createComputePipelines = GetDeviceProcAddr(device, "vkCreateComputePipelines")
	c.destroyPipeline = GetDeviceProcAddr(device, "vkDestroyPipeline")
	c.createPipelineLayout = GetDeviceProcAddr(device, "vkCreatePipelineLayout")
	c.destroyPipelineLayout = GetDeviceProcAddr(device, "vkDestroyPipelineLayout")
	c.createSampler = GetDeviceProcAddr(device, "vkCreateSampler")
	c.destroySampler = GetDeviceProcAddr(device, "vkDestroySampler")
	c.createDescriptorSetLayout = GetDeviceProcAddr(device, "vkCreateDescriptorSetLayout")
	c.destroyDescriptorSetLayout = GetDeviceProcAddr(device, "vkDestroyDescriptorSetLayout")
	c.createDescriptorPool = GetDeviceProcAddr(device, "vkCreateDescriptorPool")
	c.destroyDescriptorPool = GetDeviceProcAddr(device, "vkDestroyDescriptorPool")
	c.resetDescriptorPool = GetDeviceProcAddr(device, "vkResetDescriptorPool")
	c.allocateDescriptorSets = GetDeviceProcAddr(device, "vkAllocateDescriptorSets")
	c.freeDescriptorSets = GetDeviceProcAddr(device, "vkFreeDescriptorSets")
	c.updateDescriptorSets = GetDeviceProcAddr(device, "vkUpdateDescriptorSets")
	c.createFramebuffer = GetDeviceProcAddr(device, "vkCreateFramebuffer")
	c.destroyFramebuffer = GetDeviceProcAddr(device, "vkDestroyFramebuffer")
	c.createRenderPass = GetDeviceProcAddr(device, "vkCreateRenderPass")
	c.destroyRenderPass = GetDeviceProcAddr(device, "vkDestroyRenderPass")
	c.getRenderAreaGranularity = GetDeviceProcAddr(device, "vkGetRenderAreaGranularity")
	c.createCommandPool = GetDeviceProcAddr(device, "vkCreateCommandPool")
	c.destroyCommandPool = GetDeviceProcAddr(device, "vkDestroyCommandPool")
	c.resetCommandPool = GetDeviceProcAddr(device, "vkResetCommandPool")
	c.allocateCommandBuffers = GetDeviceProcAddr(device, "vkAllocateCommandBuffers")
	c.freeCommandBuffers = GetDeviceProcAddr(device, "vkFreeCommandBuffers")
	c.beginCommandBuffer = GetDeviceProcAddr(device, "vkBeginCommandBuffer")
	c.endCommandBuffer = GetDeviceProcAddr(device, "vkEndCommandBuffer")
	c.resetCommandBuffer = GetDeviceProcAddr(device, "vkResetCommandBuffer")
	c.cmdBindPipeline = GetDeviceProcAddr(device, "vkCmdBindPipeline")
	c.cmdSetViewport = GetDeviceProcAddr(device, "vkCmdSetViewport")
	c.cmdSetScissor = GetDeviceProcAddr(device, "vkCmdSetScissor")
	c.cmdSetLineWidth = GetDeviceProcAddr(device, "vkCmdSetLineWidth")
	c.cmdSetDepthBias = GetDeviceProcAddr(device, "vkCmdSetDepthBias")
	c.cmdSetBlendConstants = GetDeviceProcAddr(device, "vkCmdSetBlendConstants")
	c.cmdSetDepthBounds = GetDeviceProcAddr(device, "vkCmdSetDepthBounds")
	c.cmdSetStencilCompareMask = GetDeviceProcAddr(device, "vkCmdSetStencilCompareMask")
	c.cmdSetStencilWriteMask = GetDeviceProcAddr(device, "vkCmdSetStencilWriteMask")
	c.cmdSetStencilReference = GetDeviceProcAddr(device, "vkCmdSetStencilReference")
	c.cmdBindDescriptorSets = GetDeviceProcAddr(device, "vkCmdBindDescriptorSets")
	c.cmdBindIndexBuffer = GetDeviceProcAddr(device, "vkCmdBindIndexBuffer")
	c.cmdBindVertexBuffers = GetDeviceProcAddr(device, "vkCmdBindVertexBuffers")
	c.cmdDraw = GetDeviceProcAddr(device, "vkCmdDraw")
	c.cmdDrawIndexed = GetDeviceProcAddr(device, "vkCmdDrawIndexed")
	c.cmdDrawIndirect = GetDeviceProcAddr(device, "vkCmdDrawIndirect")
	c.cmdDrawIndexedIndirect = GetDeviceProcAddr(device, "vkCmdDrawIndexedIndirect")
	c.cmdDispatch = GetDeviceProcAddr(device, "vkCmdDispatch")
	c.cmdDispatchIndirect = GetDeviceProcAddr(device, "vkCmdDispatchIndirect")
	c.cmdCopyBuffer = GetDeviceProcAddr(device, "vkCmdCopyBuffer")
	c.cmdCopyImage = GetDeviceProcAddr(device, "vkCmdCopyImage")
	c.cmdBlitImage = GetDeviceProcAddr(device, "vkCmdBlitImage")
	c.cmdCopyBufferToImage = GetDeviceProcAddr(device, "vkCmdCopyBufferToImage")
	c.cmdCopyImageToBuffer = GetDeviceProcAddr(device, "vkCmdCopyImageToBuffer")
	c.cmdUpdateBuffer = GetDeviceProcAddr(device, "vkCmdUpdateBuffer")
	c.cmdFillBuffer = GetDeviceProcAddr(device, "vkCmdFillBuffer")
	c.cmdClearColorImage = GetDeviceProcAddr(device, "vkCmdClearColorImage")
	c.cmdClearDepthStencilImage = GetDeviceProcAddr(device, "vkCmdClearDepthStencilImage")
	c.cmdClearAttachments = GetDeviceProcAddr(device, "vkCmdClearAttachments")
	c.cmdResolveImage = GetDeviceProcAddr(device, "vkCmdResolveImage")
	c.cmdSetEvent = GetDeviceProcAddr(device, "vkCmdSetEvent")
	c.cmdResetEvent = GetDeviceProcAddr(device, "vkCmdResetEvent")
	c.cmdWaitEvents = GetDeviceProcAddr(device, "vkCmdWaitEvents")
	c.cmdPipelineBarrier = GetDeviceProcAddr(device, "vkCmdPipelineBarrier")
	c.cmdBeginQuery = GetDeviceProcAddr(device, "vkCmdBeginQuery")
	c.cmdEndQuery = GetDeviceProcAddr(device, "vkCmdEndQuery")
	c.cmdResetQueryPool = GetDeviceProcAddr(device, "vkCmdResetQueryPool")
	c.cmdWriteTimestamp = GetDeviceProcAddr(device, "vkCmdWriteTimestamp")
	c.cmdCopyQueryPoolResults = GetDeviceProcAddr(device, "vkCmdCopyQueryPoolResults")
	c.cmdPushConstants = GetDeviceProcAddr(device, "vkCmdPushConstants")
	c.cmdBeginRenderPass = GetDeviceProcAddr(device, "vkCmdBeginRenderPass")
	c.cmdNextSubpass = GetDeviceProcAddr(device, "vkCmdNextSubpass")
	c.cmdEndRenderPass = GetDeviceProcAddr(device, "vkCmdEndRenderPass")
	c.cmdExecuteCommands = GetDeviceProcAddr(device, "vkCmdExecuteCommands")

	// Vulkan 1.2+ timeline semaphore functions
	c.getSemaphoreCounterValue = GetDeviceProcAddr(device, "vkGetSemaphoreCounterValue")
	c.waitSemaphores = GetDeviceProcAddr(device, "vkWaitSemaphores")
	c.signalSemaphore = GetDeviceProcAddr(device, "vkSignalSemaphore")

	// Swapchain functions (WSI)
	c.createSwapchainKHR = GetDeviceProcAddr(device, "vkCreateSwapchainKHR")
	c.destroySwapchainKHR = GetDeviceProcAddr(device, "vkDestroySwapchainKHR")
	c.getSwapchainImagesKHR = GetDeviceProcAddr(device, "vkGetSwapchainImagesKHR")
	c.acquireNextImageKHR = GetDeviceProcAddr(device, "vkAcquireNextImageKHR")
	c.queuePresentKHR = GetDeviceProcAddr(device, "vkQueuePresentKHR")

	// Verify critical functions loaded
	if c.destroyDevice == nil || c.getDeviceQueue == nil || c.queueSubmit == nil {
		return fmt.Errorf("failed to load critical device functions")
	}

	return nil
}

// HasTimelineSemaphore returns true if timeline semaphore functions were loaded.
// These are Vulkan 1.2 core functions and should be available on all conformant drivers.
func (c *Commands) HasTimelineSemaphore() bool {
	return c.getSemaphoreCounterValue != nil &&
		c.waitSemaphores != nil &&
		c.signalSemaphore != nil
}

// HasPhysicalDeviceFeatures2 returns true if vkGetPhysicalDeviceFeatures2 is available.
// This is a Vulkan 1.1 core function used to query extended feature support via PNext chains.
func (c *Commands) HasPhysicalDeviceFeatures2() bool {
	return c.getPhysicalDeviceFeatures2 != nil
}

// DebugFunctionPointer returns the address of the specified Vulkan function.
// This is only for debugging purposes.
func (c *Commands) DebugFunctionPointer(name string) unsafe.Pointer {
	switch name {
	case "vkCreateGraphicsPipelines":
		return c.createGraphicsPipelines
	case "vkCreateComputePipelines":
		return c.createComputePipelines
	case "vkCreateRenderPass":
		return c.createRenderPass
	default:
		return nil
	}
}
