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

	// Detect timeline semaphore support (VK-IMPL-001).
	// Query via PhysicalDeviceVulkan12Features with PNext chain on GetPhysicalDeviceFeatures2.
	hasTimelineSemaphore := false
	if a.instance.cmds.HasPhysicalDeviceFeatures2() {
		var vulkan12Features vk.PhysicalDeviceVulkan12Features
		vulkan12Features.SType = vk.StructureTypePhysicalDeviceVulkan12Features

		features2 := vk.PhysicalDeviceFeatures2{
			SType: vk.StructureTypePhysicalDeviceFeatures2,
			PNext: (*uintptr)(unsafe.Pointer(&vulkan12Features)),
		}
		a.instance.cmds.GetPhysicalDeviceFeatures2(a.physicalDevice, &features2)
		hasTimelineSemaphore = vulkan12Features.TimelineSemaphore != 0
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

	// Enable timeline semaphore feature if supported.
	// Vulkan 1.2 requires explicitly enabling features via PNext chain.
	var vulkan12Enable vk.PhysicalDeviceVulkan12Features
	if hasTimelineSemaphore {
		vulkan12Enable.SType = vk.StructureTypePhysicalDeviceVulkan12Features
		vulkan12Enable.TimelineSemaphore = vk.Bool32(vk.True)
		deviceCreateInfo.PNext = (*uintptr)(unsafe.Pointer(&vulkan12Enable))
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

	// Initialize synchronization fence (VK-IMPL-001 / VK-IMPL-003).
	// Prefer timeline semaphore (Vulkan 1.2+); fall back to fencePool of binary
	// VkFences on older drivers. Either way, dev.timelineFence is always set so
	// the rest of the codebase can use a single path without nil checks.
	if hasTimelineSemaphore {
		tlFence, err := initTimelineFence(dev.cmds, dev.handle)
		if err != nil {
			hal.Logger().Warn("vulkan: timeline semaphore feature reported but init failed, using binary fence pool",
				"error", err,
			)
			dev.timelineFence = initBinaryFence()
		} else {
			dev.timelineFence = tlFence
			hal.Logger().Info("vulkan: using timeline semaphore fence (VK-IMPL-001)")
		}
	} else {
		dev.timelineFence = initBinaryFence()
	}

	// Initialize memory allocator
	if err := dev.initAllocator(); err != nil {
		dev.timelineFence.destroy(dev.cmds, dev.handle)
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

	syncMode := "binary fence pool (VK-IMPL-003)"
	if dev.timelineFence.isTimeline {
		syncMode = "timeline semaphore (VK-IMPL-001)"
	}
	hal.Logger().Info("vulkan: device created",
		"name", cStringToGo(a.properties.DeviceName[:]),
		"queueFamily", graphicsFamily,
		"syncMode", syncMode,
	)

	return hal.OpenDevice{
		Device: dev,
		Queue:  q,
	}, nil
}

// TextureFormatCapabilities returns capabilities for a texture format.
func (a *Adapter) TextureFormatCapabilities(format gputypes.TextureFormat) hal.TextureFormatCapabilities {
	vkFormat := textureFormatToVk(format)
	if vkFormat == vk.FormatUndefined {
		return hal.TextureFormatCapabilities{}
	}

	var props vk.FormatProperties
	a.instance.cmds.GetPhysicalDeviceFormatProperties(a.physicalDevice, vkFormat, &props)

	// Use OptimalTilingFeatures for texture capabilities (most common use case)
	flags := vkFormatFeaturesToHAL(props.OptimalTilingFeatures)

	// Check multisampling support via image format properties
	// TODO: Query vkGetPhysicalDeviceImageFormatProperties for accurate multisample support
	// For now, assume common formats support multisampling if they support rendering
	if flags&hal.TextureFormatCapabilityRenderAttachment != 0 {
		flags |= hal.TextureFormatCapabilityMultisample | hal.TextureFormatCapabilityMultisampleResolve
	}

	return hal.TextureFormatCapabilities{
		Flags: flags,
	}
}

// SurfaceCapabilities returns surface capabilities.
func (a *Adapter) SurfaceCapabilities(surface hal.Surface) *hal.SurfaceCapabilities {
	vkSurface, ok := surface.(*Surface)
	if !ok || vkSurface == nil || vkSurface.handle == 0 {
		return nil
	}

	// Query surface capabilities for alpha modes
	var surfaceCaps vk.SurfaceCapabilitiesKHR
	a.instance.cmds.GetPhysicalDeviceSurfaceCapabilitiesKHR(
		a.physicalDevice, vkSurface.handle, &surfaceCaps)

	// Query supported surface formats
	var formatCount uint32
	a.instance.cmds.GetPhysicalDeviceSurfaceFormatsKHR(
		a.physicalDevice, vkSurface.handle, &formatCount, nil)

	formats := make([]gputypes.TextureFormat, 0, formatCount)
	if formatCount > 0 {
		vkFormats := make([]vk.SurfaceFormatKHR, formatCount)
		a.instance.cmds.GetPhysicalDeviceSurfaceFormatsKHR(
			a.physicalDevice, vkSurface.handle, &formatCount, &vkFormats[0])

		for _, f := range vkFormats {
			if tf := vkFormatToTextureFormat(f.Format); tf != gputypes.TextureFormatUndefined {
				formats = append(formats, tf)
			}
		}
	}

	// Fallback if no formats found
	if len(formats) == 0 {
		formats = []gputypes.TextureFormat{
			gputypes.TextureFormatBGRA8Unorm,
			gputypes.TextureFormatRGBA8Unorm,
		}
	}

	// Query supported present modes
	var modeCount uint32
	a.instance.cmds.GetPhysicalDeviceSurfacePresentModesKHR(
		a.physicalDevice, vkSurface.handle, &modeCount, nil)

	presentModes := make([]hal.PresentMode, 0, modeCount)
	if modeCount > 0 {
		vkModes := make([]vk.PresentModeKHR, modeCount)
		a.instance.cmds.GetPhysicalDeviceSurfacePresentModesKHR(
			a.physicalDevice, vkSurface.handle, &modeCount, &vkModes[0])

		for _, m := range vkModes {
			presentModes = append(presentModes, vkPresentModeToHAL(m))
		}
	}

	// Fallback if no present modes found
	if len(presentModes) == 0 {
		presentModes = []hal.PresentMode{hal.PresentModeFifo}
	}

	// Convert composite alpha modes
	alphaModes := vkCompositeAlphaToHAL(surfaceCaps.SupportedCompositeAlpha)

	return &hal.SurfaceCapabilities{
		Formats:      formats,
		PresentModes: presentModes,
		AlphaModes:   alphaModes,
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
