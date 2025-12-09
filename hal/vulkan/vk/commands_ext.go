// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows

package vk

// Getter methods for Commands function pointers.
// These provide access to the loaded Vulkan function addresses.

// CreateInstance returns the vkCreateInstance function pointer.
func (c *Commands) CreateInstance() uintptr { return c.createInstance }

// DestroyInstance returns the vkDestroyInstance function pointer.
func (c *Commands) DestroyInstance() uintptr { return c.destroyInstance }

// EnumeratePhysicalDevices returns the vkEnumeratePhysicalDevices function pointer.
func (c *Commands) EnumeratePhysicalDevices() uintptr { return c.enumeratePhysicalDevices }

// GetPhysicalDeviceProperties returns the vkGetPhysicalDeviceProperties function pointer.
func (c *Commands) GetPhysicalDeviceProperties() uintptr { return c.getPhysicalDeviceProperties }

// GetPhysicalDeviceFeatures returns the vkGetPhysicalDeviceFeatures function pointer.
func (c *Commands) GetPhysicalDeviceFeatures() uintptr { return c.getPhysicalDeviceFeatures }

// GetPhysicalDeviceQueueFamilyProperties returns the function pointer.
func (c *Commands) GetPhysicalDeviceQueueFamilyProperties() uintptr {
	return c.getPhysicalDeviceQueueFamilyProperties
}

// CreateDevice returns the vkCreateDevice function pointer.
func (c *Commands) CreateDevice() uintptr { return c.createDevice }

// EnumerateInstanceExtensionProperties returns the function pointer.
func (c *Commands) EnumerateInstanceExtensionProperties() uintptr {
	return c.enumerateInstanceExtensionProperties
}

// EnumerateInstanceLayerProperties returns the function pointer.
func (c *Commands) EnumerateInstanceLayerProperties() uintptr {
	return c.enumerateInstanceLayerProperties
}

// EnumerateInstanceVersion returns the vkEnumerateInstanceVersion function pointer.
func (c *Commands) EnumerateInstanceVersion() uintptr { return c.enumerateInstanceVersion }

// DestroyDevice returns the vkDestroyDevice function pointer.
func (c *Commands) DestroyDevice() uintptr { return c.destroyDevice }

// GetDeviceQueue returns the vkGetDeviceQueue function pointer.
func (c *Commands) GetDeviceQueue() uintptr { return c.getDeviceQueue }

// GetPhysicalDeviceMemoryProperties returns the function pointer.
func (c *Commands) GetPhysicalDeviceMemoryProperties() uintptr {
	return c.getPhysicalDeviceMemoryProperties
}

// AllocateMemory returns the vkAllocateMemory function pointer.
func (c *Commands) AllocateMemory() uintptr { return c.allocateMemory }

// FreeMemory returns the vkFreeMemory function pointer.
func (c *Commands) FreeMemory() uintptr { return c.freeMemory }

// MapMemory returns the vkMapMemory function pointer.
func (c *Commands) MapMemory() uintptr { return c.mapMemory }

// UnmapMemory returns the vkUnmapMemory function pointer.
func (c *Commands) UnmapMemory() uintptr { return c.unmapMemory }

// GetBufferMemoryRequirements returns the function pointer.
func (c *Commands) GetBufferMemoryRequirements() uintptr { return c.getBufferMemoryRequirements }

// BindBufferMemory returns the vkBindBufferMemory function pointer.
func (c *Commands) BindBufferMemory() uintptr { return c.bindBufferMemory }

// GetImageMemoryRequirements returns the function pointer.
func (c *Commands) GetImageMemoryRequirements() uintptr { return c.getImageMemoryRequirements }

// BindImageMemory returns the vkBindImageMemory function pointer.
func (c *Commands) BindImageMemory() uintptr { return c.bindImageMemory }

// CreateBuffer returns the vkCreateBuffer function pointer.
func (c *Commands) CreateBuffer() uintptr { return c.createBuffer }

// DestroyBuffer returns the vkDestroyBuffer function pointer.
func (c *Commands) DestroyBuffer() uintptr { return c.destroyBuffer }

// CreateImage returns the vkCreateImage function pointer.
func (c *Commands) CreateImage() uintptr { return c.createImage }

// DestroyImage returns the vkDestroyImage function pointer.
func (c *Commands) DestroyImage() uintptr { return c.destroyImage }

// FlushMappedMemoryRanges returns the function pointer.
func (c *Commands) FlushMappedMemoryRanges() uintptr { return c.flushMappedMemoryRanges }

// InvalidateMappedMemoryRanges returns the function pointer.
func (c *Commands) InvalidateMappedMemoryRanges() uintptr { return c.invalidateMappedMemoryRanges }

// --- Command Pool & Buffer ---

// CreateCommandPool returns the vkCreateCommandPool function pointer.
func (c *Commands) CreateCommandPool() uintptr { return c.createCommandPool }

// DestroyCommandPool returns the vkDestroyCommandPool function pointer.
func (c *Commands) DestroyCommandPool() uintptr { return c.destroyCommandPool }

// ResetCommandPool returns the vkResetCommandPool function pointer.
func (c *Commands) ResetCommandPool() uintptr { return c.resetCommandPool }

// AllocateCommandBuffers returns the vkAllocateCommandBuffers function pointer.
func (c *Commands) AllocateCommandBuffers() uintptr { return c.allocateCommandBuffers }

// FreeCommandBuffers returns the vkFreeCommandBuffers function pointer.
func (c *Commands) FreeCommandBuffers() uintptr { return c.freeCommandBuffers }

// BeginCommandBuffer returns the vkBeginCommandBuffer function pointer.
func (c *Commands) BeginCommandBuffer() uintptr { return c.beginCommandBuffer }

// EndCommandBuffer returns the vkEndCommandBuffer function pointer.
func (c *Commands) EndCommandBuffer() uintptr { return c.endCommandBuffer }

// ResetCommandBuffer returns the vkResetCommandBuffer function pointer.
func (c *Commands) ResetCommandBuffer() uintptr { return c.resetCommandBuffer }

// --- Pipeline Binding ---

// CmdBindPipeline returns the vkCmdBindPipeline function pointer.
func (c *Commands) CmdBindPipeline() uintptr { return c.cmdBindPipeline }

// CmdBindDescriptorSets returns the vkCmdBindDescriptorSets function pointer.
func (c *Commands) CmdBindDescriptorSets() uintptr { return c.cmdBindDescriptorSets }

// CmdBindVertexBuffers returns the vkCmdBindVertexBuffers function pointer.
func (c *Commands) CmdBindVertexBuffers() uintptr { return c.cmdBindVertexBuffers }

// CmdBindIndexBuffer returns the vkCmdBindIndexBuffer function pointer.
func (c *Commands) CmdBindIndexBuffer() uintptr { return c.cmdBindIndexBuffer }

// CmdPushConstants returns the vkCmdPushConstants function pointer.
func (c *Commands) CmdPushConstants() uintptr { return c.cmdPushConstants }

// --- Drawing ---

// CmdDraw returns the vkCmdDraw function pointer.
func (c *Commands) CmdDraw() uintptr { return c.cmdDraw }

// CmdDrawIndexed returns the vkCmdDrawIndexed function pointer.
func (c *Commands) CmdDrawIndexed() uintptr { return c.cmdDrawIndexed }

// CmdDrawIndirect returns the vkCmdDrawIndirect function pointer.
func (c *Commands) CmdDrawIndirect() uintptr { return c.cmdDrawIndirect }

// CmdDrawIndexedIndirect returns the vkCmdDrawIndexedIndirect function pointer.
func (c *Commands) CmdDrawIndexedIndirect() uintptr { return c.cmdDrawIndexedIndirect }

// --- Compute ---

// CmdDispatch returns the vkCmdDispatch function pointer.
func (c *Commands) CmdDispatch() uintptr { return c.cmdDispatch }

// CmdDispatchIndirect returns the vkCmdDispatchIndirect function pointer.
func (c *Commands) CmdDispatchIndirect() uintptr { return c.cmdDispatchIndirect }

// --- Viewport & Scissor ---

// CmdSetViewport returns the vkCmdSetViewport function pointer.
func (c *Commands) CmdSetViewport() uintptr { return c.cmdSetViewport }

// CmdSetScissor returns the vkCmdSetScissor function pointer.
func (c *Commands) CmdSetScissor() uintptr { return c.cmdSetScissor }

// CmdSetDepthBias returns the vkCmdSetDepthBias function pointer.
func (c *Commands) CmdSetDepthBias() uintptr { return c.cmdSetDepthBias }

// CmdSetBlendConstants returns the vkCmdSetBlendConstants function pointer.
func (c *Commands) CmdSetBlendConstants() uintptr { return c.cmdSetBlendConstants }

// CmdSetStencilReference returns the vkCmdSetStencilReference function pointer.
func (c *Commands) CmdSetStencilReference() uintptr { return c.cmdSetStencilReference }

// --- Render Pass ---

// CmdBeginRenderPass returns the vkCmdBeginRenderPass function pointer.
func (c *Commands) CmdBeginRenderPass() uintptr { return c.cmdBeginRenderPass }

// CmdEndRenderPass returns the vkCmdEndRenderPass function pointer.
func (c *Commands) CmdEndRenderPass() uintptr { return c.cmdEndRenderPass }

// CmdNextSubpass returns the vkCmdNextSubpass function pointer.
func (c *Commands) CmdNextSubpass() uintptr { return c.cmdNextSubpass }

// CmdBeginRendering returns the vkCmdBeginRendering function pointer (Vulkan 1.3+).
func (c *Commands) CmdBeginRendering() uintptr { return c.cmdBeginRendering }

// CmdEndRendering returns the vkCmdEndRendering function pointer (Vulkan 1.3+).
func (c *Commands) CmdEndRendering() uintptr { return c.cmdEndRendering }

// --- Copy Commands ---

// CmdCopyBuffer returns the vkCmdCopyBuffer function pointer.
func (c *Commands) CmdCopyBuffer() uintptr { return c.cmdCopyBuffer }

// CmdCopyImage returns the vkCmdCopyImage function pointer.
func (c *Commands) CmdCopyImage() uintptr { return c.cmdCopyImage }

// CmdCopyBufferToImage returns the vkCmdCopyBufferToImage function pointer.
func (c *Commands) CmdCopyBufferToImage() uintptr { return c.cmdCopyBufferToImage }

// CmdCopyImageToBuffer returns the vkCmdCopyImageToBuffer function pointer.
func (c *Commands) CmdCopyImageToBuffer() uintptr { return c.cmdCopyImageToBuffer }

// CmdBlitImage returns the vkCmdBlitImage function pointer.
func (c *Commands) CmdBlitImage() uintptr { return c.cmdBlitImage }

// --- Clear Commands ---

// CmdFillBuffer returns the vkCmdFillBuffer function pointer.
func (c *Commands) CmdFillBuffer() uintptr { return c.cmdFillBuffer }

// CmdClearColorImage returns the vkCmdClearColorImage function pointer.
func (c *Commands) CmdClearColorImage() uintptr { return c.cmdClearColorImage }

// CmdClearDepthStencilImage returns the vkCmdClearDepthStencilImage function pointer.
func (c *Commands) CmdClearDepthStencilImage() uintptr { return c.cmdClearDepthStencilImage }

// CmdClearAttachments returns the vkCmdClearAttachments function pointer.
func (c *Commands) CmdClearAttachments() uintptr { return c.cmdClearAttachments }

// --- Synchronization ---

// CmdPipelineBarrier returns the vkCmdPipelineBarrier function pointer.
func (c *Commands) CmdPipelineBarrier() uintptr { return c.cmdPipelineBarrier }

// CmdPipelineBarrier2 returns the vkCmdPipelineBarrier2 function pointer (Vulkan 1.3+).
func (c *Commands) CmdPipelineBarrier2() uintptr { return c.cmdPipelineBarrier2 }

// CmdSetEvent returns the vkCmdSetEvent function pointer.
func (c *Commands) CmdSetEvent() uintptr { return c.cmdSetEvent }

// CmdResetEvent returns the vkCmdResetEvent function pointer.
func (c *Commands) CmdResetEvent() uintptr { return c.cmdResetEvent }

// CmdWaitEvents returns the vkCmdWaitEvents function pointer.
func (c *Commands) CmdWaitEvents() uintptr { return c.cmdWaitEvents }

// --- Secondary Command Buffers ---

// CmdExecuteCommands returns the vkCmdExecuteCommands function pointer.
func (c *Commands) CmdExecuteCommands() uintptr { return c.cmdExecuteCommands }
