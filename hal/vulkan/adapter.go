//go:build !(js && wasm)

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
func (a *Adapter) Open(_ gputypes.Features, _ gputypes.Limits) (hal.OpenDevice, error) {
	return a.open(nil)
}

// open creates a logical device, optionally constraining it to one queue
// family. Surface-qualified adapters use the constrained path so the queue
// selected during the surface query is the same queue passed into Open.
func (a *Adapter) open(requestedQueueFamily *uint32) (hal.OpenDevice, error) {
	// Find queue families
	var queueFamilyCount uint32
	vkGetPhysicalDeviceQueueFamilyProperties(a.instance, a.physicalDevice, &queueFamilyCount, nil)

	if queueFamilyCount == 0 {
		return hal.OpenDevice{}, fmt.Errorf("vulkan: no queue families found")
	}

	queueFamilies := make([]vk.QueueFamilyProperties, queueFamilyCount)
	vkGetPhysicalDeviceQueueFamilyProperties(a.instance, a.physicalDevice, &queueFamilyCount, &queueFamilies[0])

	if queueFamilyCount < uint32(len(queueFamilies)) {
		queueFamilies = queueFamilies[:queueFamilyCount]
	}
	graphicsFamily, err := selectGraphicsQueueFamily(queueFamilies, requestedQueueFamily)
	if err != nil {
		return hal.OpenDevice{}, err
	}

	// Create device with graphics queue
	queuePriority := float32(1.0)
	queueCreateInfo := vk.DeviceQueueCreateInfo{
		SType:            vk.StructureTypeDeviceQueueCreateInfo,
		QueueFamilyIndex: graphicsFamily,
		QueueCount:       1,
		PQueuePriorities: &queuePriority,
	}

	// Query supported device extensions to enable optional features.
	hasIncrementalPresent := false
	{
		var extCount uint32
		a.instance.cmds.EnumerateDeviceExtensionProperties(a.physicalDevice, 0, &extCount, nil)
		if extCount > 0 {
			extProps := make([]vk.ExtensionProperties, extCount)
			a.instance.cmds.EnumerateDeviceExtensionProperties(a.physicalDevice, 0, &extCount, &extProps[0])
			for i := range extProps {
				name := cStringToGo(extProps[i].ExtensionName[:])
				if name == "VK_KHR_incremental_present" {
					hasIncrementalPresent = true
					break
				}
			}
		}
	}

	// Required extensions
	extensions := []string{
		"VK_KHR_swapchain\x00",
	}
	// Optional: VK_KHR_incremental_present for damage-aware presentation.
	// Allows chaining VkPresentRegionsKHR into VkPresentInfoKHR.PNext
	// so the compositor can skip recompositing unchanged pixels.
	if hasIncrementalPresent {
		extensions = append(extensions, "VK_KHR_incremental_present\x00")
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
	vkGetDeviceQueue(&deviceCmds, device, graphicsFamily, 0, &queue)

	dev := &Device{
		handle:                     device,
		physicalDevice:             a.physicalDevice,
		instance:                   a.instance,
		graphicsFamily:             graphicsFamily,
		cmds:                       &deviceCmds,
		supportsMultiDrawIndirect:  a.features.MultiDrawIndirect != 0,
		maxDrawIndirectCount:       a.properties.Limits.MaxDrawIndirectCount,
		supportsIncrementalPresent: hasIncrementalPresent,
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

	// VK-SYNC-001: Create relay semaphores for GPU-side submission ordering.
	// This ensures consecutive vkQueueSubmit calls execute in order on the GPU,
	// which is required by the wgpu_hal Queue trait but not guaranteed by Vulkan.
	relay, err := newRelaySemaphores(dev.cmds, dev.handle)
	if err != nil {
		dev.allocator.Destroy()
		dev.timelineFence.destroy(dev.cmds, dev.handle)
		vkDestroyDevice(device, nil)
		return hal.OpenDevice{}, fmt.Errorf("vulkan: failed to create relay semaphores: %w", err)
	}

	q := &Queue{
		handle:      queue,
		device:      dev,
		familyIndex: graphicsFamily,
		relay:       relay,
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

// selectGraphicsQueueFamily preserves the exact family chosen during surface
// qualification, while keeping the ordinary headless path first-graphics.
func selectGraphicsQueueFamily(families []vk.QueueFamilyProperties, requested *uint32) (uint32, error) {
	if requested != nil {
		index := *requested
		if index >= uint32(len(families)) {
			return 0, fmt.Errorf("vulkan: requested queue family %d is unavailable", index)
		}
		family := families[index]
		if family.QueueCount == 0 {
			return 0, fmt.Errorf("vulkan: requested queue family %d has no queues", index)
		}
		if family.QueueFlags&vk.QueueFlags(vk.QueueGraphicsBit) == 0 {
			return 0, fmt.Errorf("vulkan: requested queue family %d has no graphics queue", index)
		}
		return index, nil
	}
	for index, family := range families {
		if family.QueueCount > 0 && family.QueueFlags&vk.QueueFlags(vk.QueueGraphicsBit) != 0 {
			return uint32(index), nil
		}
	}
	return 0, fmt.Errorf("vulkan: no graphics queue family found")
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
	if !ok || vkSurface == nil || vkSurface.handle == 0 || vkSurface.instance == nil || vkSurface.instance != a.instance {
		return nil
	}

	snapshot, err := a.querySurfaceSnapshot(vkSurface)
	if err != nil {
		return nil
	}
	return cloneSurfaceCapabilities(snapshot.public)
}

// surfaceSnapshot keeps the exact Vulkan format/color-space pairs alongside
// the public projection. The public HAL shape predates color spaces, so the
// raw pairs stay private to the Vulkan adapter and are never silently reduced
// to a fabricated format list.
type surfaceSnapshot struct {
	capabilities vk.SurfaceCapabilitiesKHR
	formats      []vk.SurfaceFormatKHR
	presentModes []vk.PresentModeKHR
	public       hal.SurfaceCapabilities
}

// qualifiedAdapter is a request-local Vulkan adapter. It carries the queue
// family proven to support both graphics and presentation for one surface and
// opens the underlying physical device with that family unchanged.
type qualifiedAdapter struct {
	base        *Adapter
	surface     *Surface
	queueFamily uint32
	snapshot    surfaceSnapshot
}

func (a *qualifiedAdapter) Open(_ gputypes.Features, _ gputypes.Limits) (hal.OpenDevice, error) {
	return a.base.open(&a.queueFamily)
}

func (a *qualifiedAdapter) TextureFormatCapabilities(format gputypes.TextureFormat) hal.TextureFormatCapabilities {
	return a.base.TextureFormatCapabilities(format)
}

func (a *qualifiedAdapter) SurfaceCapabilities(surface hal.Surface) *hal.SurfaceCapabilities {
	vkSurface, ok := surface.(*Surface)
	if !ok || vkSurface != a.surface {
		return nil
	}
	return cloneSurfaceCapabilities(a.snapshot.public)
}

// Destroy is intentionally a no-op. The cached physical adapter owns no
// Vulkan object, and the core instance retains ownership of the base adapter.
func (a *qualifiedAdapter) Destroy() {}

// QualifySurface selects a same-family graphics+present queue and records a
// checked surface capability snapshot without mutating the cached adapter.
func (a *Adapter) QualifySurface(surface hal.Surface) (hal.Adapter, error) {
	vkSurface, ok := surface.(*Surface)
	if !ok || vkSurface == nil || vkSurface.handle == 0 {
		return nil, fmt.Errorf("vulkan: invalid surface for adapter qualification")
	}
	if vkSurface.instance == nil || vkSurface.instance != a.instance {
		return nil, fmt.Errorf("vulkan: surface belongs to a different instance")
	}

	queueFamilies, err := a.queueFamilies()
	if err != nil {
		return nil, err
	}
	queueFamily, err := a.presentGraphicsQueueFamily(vkSurface, queueFamilies)
	if err != nil {
		return nil, err
	}

	snapshot, err := a.querySurfaceSnapshot(vkSurface)
	if err != nil {
		return nil, err
	}

	return &qualifiedAdapter{
		base:        a,
		surface:     vkSurface,
		queueFamily: queueFamily,
		snapshot:    snapshot,
	}, nil
}

func (a *Adapter) queueFamilies() ([]vk.QueueFamilyProperties, error) {
	var count uint32
	vkGetPhysicalDeviceQueueFamilyProperties(a.instance, a.physicalDevice, &count, nil)
	if count == 0 {
		return nil, fmt.Errorf("vulkan: no queue families found")
	}

	families := make([]vk.QueueFamilyProperties, count)
	vkGetPhysicalDeviceQueueFamilyProperties(a.instance, a.physicalDevice, &count, &families[0])
	if count == 0 {
		return nil, fmt.Errorf("vulkan: no queue families found")
	}
	if count < uint32(len(families)) {
		families = families[:count]
	}
	return families, nil
}

func (a *Adapter) presentGraphicsQueueFamily(surface *Surface, families []vk.QueueFamilyProperties) (uint32, error) {
	supports := make([]bool, len(families))
	for index, family := range families {
		if family.QueueCount == 0 || family.QueueFlags&vk.QueueFlags(vk.QueueGraphicsBit) == 0 {
			continue
		}

		var supported vk.Bool32
		result := a.instance.cmds.GetPhysicalDeviceSurfaceSupportKHR(
			a.physicalDevice, uint32(index), surface.handle, &supported)
		if result != vk.Success {
			return 0, fmt.Errorf("vulkan: vkGetPhysicalDeviceSurfaceSupportKHR(queueFamily=%d) failed: %d", index, result)
		}
		supports[index] = supported != 0
	}
	return selectPresentGraphicsQueueFamily(families, supports)
}

func selectPresentGraphicsQueueFamily(families []vk.QueueFamilyProperties, supports []bool) (uint32, error) {
	if len(supports) != len(families) {
		return 0, fmt.Errorf("vulkan: queue family support query is incomplete")
	}
	for index, family := range families {
		if family.QueueCount > 0 && family.QueueFlags&vk.QueueFlags(vk.QueueGraphicsBit) != 0 && supports[index] {
			return uint32(index), nil
		}
	}
	return 0, fmt.Errorf("vulkan: no graphics queue family supports presentation for surface")
}

func (a *Adapter) querySurfaceSnapshot(surface *Surface) (surfaceSnapshot, error) {
	if surface == nil || surface.handle == 0 {
		return surfaceSnapshot{}, fmt.Errorf("vulkan: invalid surface")
	}

	var capabilities vk.SurfaceCapabilitiesKHR
	result := a.instance.cmds.GetPhysicalDeviceSurfaceCapabilitiesKHR(
		a.physicalDevice, surface.handle, &capabilities)
	if result != vk.Success {
		return surfaceSnapshot{}, fmt.Errorf("vulkan: vkGetPhysicalDeviceSurfaceCapabilitiesKHR failed: %d", result)
	}

	formats, err := querySurfaceFormats(a.instance, a.physicalDevice, surface.handle)
	if err != nil {
		return surfaceSnapshot{}, err
	}

	presentModes, err := queryPresentModes(a.instance, a.physicalDevice, surface.handle)
	if err != nil {
		return surfaceSnapshot{}, err
	}
	return makeSurfaceSnapshot(capabilities, formats, presentModes)
}

func querySurfaceFormats(instance *Instance, device vk.PhysicalDevice, surface vk.SurfaceKHR) ([]vk.SurfaceFormatKHR, error) {
	return querySurfaceFormatsWith(func(count *uint32, formats *vk.SurfaceFormatKHR) vk.Result {
		return instance.cmds.GetPhysicalDeviceSurfaceFormatsKHR(device, surface, count, formats)
	})
}

func querySurfaceFormatsWith(query func(count *uint32, formats *vk.SurfaceFormatKHR) vk.Result) ([]vk.SurfaceFormatKHR, error) {
	return queryOptionalSurfaceValues("vkGetPhysicalDeviceSurfaceFormatsKHR", query)
}

func queryOptionalSurfaceValues[T any](operation string, query func(count *uint32, values *T) vk.Result) ([]T, error) {
	for attempt := 0; attempt < 2; attempt++ {
		var count uint32
		result := query(&count, nil)
		if result != vk.Success && result != vk.Incomplete {
			return nil, fmt.Errorf("vulkan: %s count failed: %d", operation, result)
		}
		if count == 0 {
			return []T{}, nil
		}

		values := make([]T, count)
		returned := count
		result = query(&returned, &values[0])
		if result != vk.Success && result != vk.Incomplete {
			return nil, fmt.Errorf("vulkan: %s failed: %d", operation, result)
		}
		if result == vk.Incomplete || returned > uint32(len(values)) {
			continue
		}
		return values[:returned], nil
	}
	return nil, fmt.Errorf("vulkan: %s returned an unstable count", operation)
}

func queryPresentModes(instance *Instance, device vk.PhysicalDevice, surface vk.SurfaceKHR) ([]vk.PresentModeKHR, error) {
	return queryPresentModesWith(func(count *uint32, modes *vk.PresentModeKHR) vk.Result {
		return instance.cmds.GetPhysicalDeviceSurfacePresentModesKHR(device, surface, count, modes)
	})
}

func queryPresentModesWith(query func(count *uint32, modes *vk.PresentModeKHR) vk.Result) ([]vk.PresentModeKHR, error) {
	return queryOptionalSurfaceValues("vkGetPhysicalDeviceSurfacePresentModesKHR", query)
}

func makeSurfaceSnapshot(capabilities vk.SurfaceCapabilitiesKHR, formats []vk.SurfaceFormatKHR, presentModes []vk.PresentModeKHR) (surfaceSnapshot, error) {
	if len(formats) == 0 {
		return surfaceSnapshot{}, fmt.Errorf("vulkan: surface returned no formats")
	}
	if len(presentModes) == 0 {
		return surfaceSnapshot{}, fmt.Errorf("vulkan: surface returned no present modes")
	}

	alphaModes := vkCompositeAlphaToHALChecked(capabilities.SupportedCompositeAlpha)
	if len(alphaModes) == 0 {
		return surfaceSnapshot{}, fmt.Errorf("vulkan: surface returned no supported composite alpha modes")
	}

	public := hal.SurfaceCapabilities{
		Formats:      make([]gputypes.TextureFormat, 0, len(formats)),
		PresentModes: make([]hal.PresentMode, 0, len(presentModes)),
		AlphaModes:   alphaModes,
	}
	for _, format := range formats {
		if textureFormat := textureFormatForSurfacePair(format); textureFormat != gputypes.TextureFormatUndefined {
			public.Formats = append(public.Formats, textureFormat)
		}
	}
	for _, mode := range presentModes {
		if halMode, ok := vkPresentModeToHALChecked(mode); ok {
			public.PresentModes = append(public.PresentModes, halMode)
		}
	}
	if len(public.PresentModes) == 0 {
		return surfaceSnapshot{}, fmt.Errorf("vulkan: surface returned no supported present modes")
	}
	if len(public.Formats) == 0 {
		return surfaceSnapshot{}, fmt.Errorf("vulkan: surface returned no supported formats")
	}

	return surfaceSnapshot{
		capabilities: capabilities,
		formats:      append([]vk.SurfaceFormatKHR(nil), formats...),
		presentModes: append([]vk.PresentModeKHR(nil), presentModes...),
		public:       public,
	}, nil
}

func vkPresentModeToHALChecked(mode vk.PresentModeKHR) (hal.PresentMode, bool) {
	switch mode {
	case vk.PresentModeImmediateKhr:
		return hal.PresentModeImmediate, true
	case vk.PresentModeMailboxKhr:
		return hal.PresentModeMailbox, true
	case vk.PresentModeFifoKhr:
		return hal.PresentModeFifo, true
	case vk.PresentModeFifoRelaxedKhr:
		return hal.PresentModeFifoRelaxed, true
	default:
		return 0, false
	}
}

func vkCompositeAlphaToHALChecked(flags vk.CompositeAlphaFlagsKHR) []hal.CompositeAlphaMode {
	var modes []hal.CompositeAlphaMode
	if vk.Flags(flags)&vk.Flags(vk.CompositeAlphaOpaqueBitKhr) != 0 {
		modes = append(modes, hal.CompositeAlphaModeOpaque)
	}
	if vk.Flags(flags)&vk.Flags(vk.CompositeAlphaPreMultipliedBitKhr) != 0 {
		modes = append(modes, hal.CompositeAlphaModePremultiplied)
	}
	if vk.Flags(flags)&vk.Flags(vk.CompositeAlphaPostMultipliedBitKhr) != 0 {
		modes = append(modes, hal.CompositeAlphaModeUnpremultiplied)
	}
	if vk.Flags(flags)&vk.Flags(vk.CompositeAlphaInheritBitKhr) != 0 {
		modes = append(modes, hal.CompositeAlphaModeInherit)
	}
	return modes
}

func cloneSurfaceCapabilities(capabilities hal.SurfaceCapabilities) *hal.SurfaceCapabilities {
	return &hal.SurfaceCapabilities{
		Formats:      append([]gputypes.TextureFormat(nil), capabilities.Formats...),
		PresentModes: append([]hal.PresentMode(nil), capabilities.PresentModes...),
		AlphaModes:   append([]hal.CompositeAlphaMode(nil), capabilities.AlphaModes...),
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

func vkDestroyDevice(device vk.Device, allocator *vk.AllocationCallbacks) { //nolint:unparam // allocator kept for Vulkan API signature parity
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
	_, _ = ffi.CallFunction(&vk.SigVoidHandlePtr, proc, nil, args[:])
}
