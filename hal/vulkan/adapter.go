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
	"github.com/gogpu/wgpu/types"
)

// Adapter implements hal.Adapter for Vulkan.
type Adapter struct {
	instance       *Instance
	physicalDevice vk.PhysicalDevice
	properties     vk.PhysicalDeviceProperties
	features       vk.PhysicalDeviceFeatures
}

// Open creates a logical device with the requested features and limits.
func (a *Adapter) Open(features types.Features, limits types.Limits) (hal.OpenDevice, error) {
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

	// Get queue handle
	var queue vk.Queue
	vkGetDeviceQueue(device, uint32(graphicsFamily), 0, &queue)

	dev := &Device{
		handle:         device,
		physicalDevice: a.physicalDevice,
		instance:       a.instance,
		graphicsFamily: uint32(graphicsFamily),
		cmds:           &a.instance.cmds,
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

	return hal.OpenDevice{
		Device: dev,
		Queue:  q,
	}, nil
}

// TextureFormatCapabilities returns capabilities for a texture format.
func (a *Adapter) TextureFormatCapabilities(format types.TextureFormat) hal.TextureFormatCapabilities {
	// TODO: Query VkFormatProperties for actual support
	flags := hal.TextureFormatCapabilitySampled

	switch format {
	case types.TextureFormatRGBA8Unorm,
		types.TextureFormatRGBA8UnormSrgb,
		types.TextureFormatBGRA8Unorm,
		types.TextureFormatBGRA8UnormSrgb,
		types.TextureFormatRGBA16Float,
		types.TextureFormatRGBA32Float:
		flags |= hal.TextureFormatCapabilityRenderAttachment |
			hal.TextureFormatCapabilityBlendable |
			hal.TextureFormatCapabilityMultisample |
			hal.TextureFormatCapabilityMultisampleResolve

	case types.TextureFormatDepth16Unorm,
		types.TextureFormatDepth24Plus,
		types.TextureFormatDepth24PlusStencil8,
		types.TextureFormatDepth32Float,
		types.TextureFormatDepth32FloatStencil8:
		flags |= hal.TextureFormatCapabilityRenderAttachment |
			hal.TextureFormatCapabilityMultisample
	}

	return hal.TextureFormatCapabilities{
		Flags: flags,
	}
}

// SurfaceCapabilities returns surface capabilities.
func (a *Adapter) SurfaceCapabilities(surface hal.Surface) *hal.SurfaceCapabilities {
	// TODO: Query VkSurfaceCapabilitiesKHR
	return &hal.SurfaceCapabilities{
		Formats: []types.TextureFormat{
			types.TextureFormatBGRA8Unorm,
			types.TextureFormatRGBA8Unorm,
			types.TextureFormatBGRA8UnormSrgb,
			types.TextureFormatRGBA8UnormSrgb,
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

// Vulkan function wrappers

func vkGetPhysicalDeviceQueueFamilyProperties(i *Instance, device vk.PhysicalDevice, count *uint32, props *vk.QueueFamilyProperties) {
	//nolint:errcheck // Vulkan void function, no return value to check
	syscall.SyscallN(i.cmds.GetPhysicalDeviceQueueFamilyProperties(),
		uintptr(device),
		uintptr(unsafe.Pointer(count)),
		uintptr(unsafe.Pointer(props)))
}

func vkCreateDevice(i *Instance, physicalDevice vk.PhysicalDevice, createInfo *vk.DeviceCreateInfo, allocator unsafe.Pointer, device *vk.Device) vk.Result {
	r, _, _ := syscall.SyscallN(i.cmds.CreateDevice(),
		uintptr(physicalDevice),
		uintptr(unsafe.Pointer(createInfo)),
		uintptr(allocator),
		uintptr(unsafe.Pointer(device)))
	return vk.Result(r)
}

func vkGetDeviceQueue(device vk.Device, queueFamilyIndex, queueIndex uint32, queue *vk.Queue) {
	proc := vk.GetInstanceProcAddr(0, "vkGetDeviceQueue")
	if proc == 0 {
		return
	}
	//nolint:errcheck // Vulkan void function, no return value to check
	syscall.SyscallN(proc,
		uintptr(device),
		uintptr(queueFamilyIndex),
		uintptr(queueIndex),
		uintptr(unsafe.Pointer(queue)))
}
