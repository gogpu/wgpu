// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vulkan

import (
	"fmt"
	"unsafe"

	"github.com/go-webgpu/goffi/ffi"
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/vulkan/vk"
)

// Adapter implements hal.Adapter for Vulkan.
type Adapter struct {
	instance       *Instance
	physicalDevice vk.PhysicalDevice
	properties     vk.PhysicalDeviceProperties
	features       vk.PhysicalDeviceFeatures
}

// Open creates a logical device with the requested features and limits.
func (a *Adapter) Open(features gputypes.Features, limits gputypes.Limits) (hal.OpenDevice, error) {
	// Find queue families
	var queueFamilyCount uint32
	vkGetPhysicalDeviceQueueFamilyProperties(a.instance, a.physicalDevice, &queueFamilyCount, nil)

	if queueFamilyCount == 0 {
		return hal.OpenDevice{}, fmt.Errorf("vulkan: no queue families found")
	}

	queueFamilies := make([]vk.QueueFamilyProperties, queueFamilyCount)
	vkGetPhysicalDeviceQueueFamilyProperties(a.instance, a.physicalDevice, &queueFamilyCount, &queueFamilies[0])

	// Find graphics queue family
	graphicsFamily := int32(-1)
	for i, family := range queueFamilies {
		if family.QueueFlags&vk.QueueFlags(vk.QueueGraphicsBit) != 0 {
			graphicsFamily = int32(i)
			break
		}
	}

	if graphicsFamily < 0 {
		return hal.OpenDevice{}, fmt.Errorf("vulkan: no graphics queue family found")
	}

	// Create device with graphics queue
	queuePriority := float32(1.0)
	queueCreateInfo := vk.DeviceQueueCreateInfo{
		SType:            vk.StructureTypeDeviceQueueCreateInfo,
		QueueFamilyIndex: uint32(graphicsFamily),
		QueueCount:       1,
		PQueuePriorities: &queuePriority,
	}

	// Required extensions
	extensions := []string{
		"VK_KHR_swapchain\x00",
	}
	extensionPtrs := make([]uintptr, len(extensions))
	for i, ext := range extensions {
		extensionPtrs[i] = uintptr(unsafe.Pointer(unsafe.StringData(ext)))
	}

	// Device create info
	deviceCreateInfo := vk.DeviceCreateInfo{
		SType:                   vk.StructureTypeDeviceCreateInfo,
		QueueCreateInfoCount:    1,
		PQueueCreateInfos:       &queueCreateInfo,
		EnabledExtensionCount:   uint32(len(extensions)),
		PpEnabledExtensionNames: uintptr(unsafe.Pointer(&extensionPtrs[0])),
		PEnabledFeatures:        &a.features,
	}

	var device vk.Device
	result := vkCreateDevice(a.instance, a.physicalDevice, &deviceCreateInfo, nil, &device)
	if result != vk.Success {
		return hal.OpenDevice{}, fmt.Errorf("vulkan: vkCreateDevice failed: %d", result)
	}

	// Load device-level commands
	var deviceCmds vk.Commands
	if err := deviceCmds.LoadDevice(device); err != nil {
		vkDestroyDevice(device, nil)
		return hal.OpenDevice{}, fmt.Errorf("vulkan: failed to load device commands: %w", err)
	}

	// Get queue handle
	var queue vk.Queue
	vkGetDeviceQueue(&deviceCmds, device, uint32(graphicsFamily), 0, &queue)

	dev := &Device{
		handle:         device,
		physicalDevice: a.physicalDevice,
		instance:       a.instance,
		graphicsFamily: uint32(graphicsFamily),
		cmds:           &deviceCmds,
	}

	// Initialize memory allocator
	if err := dev.initAllocator(); err != nil {
		vkDestroyDevice(device, nil)
		return hal.OpenDevice{}, fmt.Errorf("vulkan: failed to initialize allocator: %w", err)
	}

	q := &Queue{
		handle:      queue,
		device:      dev,
		familyIndex: uint32(graphicsFamily),
	}

	// Store queue reference in device for swapchain synchronization
	dev.queue = q

	return hal.OpenDevice{
		Device: dev,
		Queue:  q,
	}, nil
}

// TextureFormatCapabilities returns capabilities for a texture format.
func (a *Adapter) TextureFormatCapabilities(format gputypes.TextureFormat) hal.TextureFormatCapabilities {
	// Note: Full format support requires vkGetPhysicalDeviceFormatProperties. See DX12 adapter.go for pattern.
	flags := hal.TextureFormatCapabilitySampled

	switch format {
	case gputypes.TextureFormatRGBA8Unorm,
		gputypes.TextureFormatRGBA8UnormSrgb,
		gputypes.TextureFormatBGRA8Unorm,
		gputypes.TextureFormatBGRA8UnormSrgb,
		gputypes.TextureFormatRGBA16Float,
		gputypes.TextureFormatRGBA32Float:
		flags |= hal.TextureFormatCapabilityRenderAttachment |
			hal.TextureFormatCapabilityBlendable |
			hal.TextureFormatCapabilityMultisample |
			hal.TextureFormatCapabilityMultisampleResolve

	case gputypes.TextureFormatDepth16Unorm,
		gputypes.TextureFormatDepth24Plus,
		gputypes.TextureFormatDepth24PlusStencil8,
		gputypes.TextureFormatDepth32Float,
		gputypes.TextureFormatDepth32FloatStencil8:
		flags |= hal.TextureFormatCapabilityRenderAttachment |
			hal.TextureFormatCapabilityMultisample
	}

	return hal.TextureFormatCapabilities{
		Flags: flags,
	}
}

// SurfaceCapabilities returns surface capabilities.
func (a *Adapter) SurfaceCapabilities(surface hal.Surface) *hal.SurfaceCapabilities {
	// Note: Full surface capabilities require vkGetPhysicalDeviceSurfaceCapabilitiesKHR.
	return &hal.SurfaceCapabilities{
		Formats: []gputypes.TextureFormat{
			gputypes.TextureFormatBGRA8Unorm,
			gputypes.TextureFormatRGBA8Unorm,
			gputypes.TextureFormatBGRA8UnormSrgb,
			gputypes.TextureFormatRGBA8UnormSrgb,
		},
		PresentModes: []hal.PresentMode{
			hal.PresentModeFifo,
			hal.PresentModeMailbox,
			hal.PresentModeImmediate,
		},
		AlphaModes: []hal.CompositeAlphaMode{
			hal.CompositeAlphaModeOpaque,
			hal.CompositeAlphaModePremultiplied,
		},
	}
}

// Destroy releases the adapter.
func (a *Adapter) Destroy() {
	// Adapter doesn't own resources
}

// Vulkan function wrappers using Commands methods

func vkGetPhysicalDeviceQueueFamilyProperties(i *Instance, device vk.PhysicalDevice, count *uint32, props *vk.QueueFamilyProperties) {
	i.cmds.GetPhysicalDeviceQueueFamilyProperties(device, count, props)
}

func vkCreateDevice(i *Instance, physicalDevice vk.PhysicalDevice, createInfo *vk.DeviceCreateInfo, allocator *vk.AllocationCallbacks, device *vk.Device) vk.Result {
	return i.cmds.CreateDevice(physicalDevice, createInfo, allocator, device)
}

func vkGetDeviceQueue(cmds *vk.Commands, device vk.Device, queueFamilyIndex, queueIndex uint32, queue *vk.Queue) {
	cmds.GetDeviceQueue(device, queueFamilyIndex, queueIndex, queue)
}

func vkDestroyDevice(device vk.Device, allocator *vk.AllocationCallbacks) {
	// Get vkDestroyDevice function pointer directly since device commands
	// may not be available when destroying the device
	proc := vk.GetDeviceProcAddr(device, "vkDestroyDevice")
	if proc == nil {
		return
	}
	// Call vkDestroyDevice(VkDevice, VkAllocationCallbacks*) via goffi
	args := [2]unsafe.Pointer{
		unsafe.Pointer(&device),
		unsafe.Pointer(&allocator),
	}
	_ = ffi.CallFunction(&vk.SigVoidHandlePtr, proc, nil, args[:])
}
