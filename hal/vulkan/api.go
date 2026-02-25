// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vulkan

import (
	"fmt"
	"runtime"
	"unsafe"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/vulkan/vk"
)

// Backend implements hal.Backend for Vulkan.
type Backend struct{}

// Variant returns the backend type identifier.
func (Backend) Variant() gputypes.Backend {
	return gputypes.BackendVulkan
}

// CreateInstance creates a new Vulkan instance.
func (Backend) CreateInstance(desc *hal.InstanceDescriptor) (hal.Instance, error) {
	// Initialize Vulkan library
	if err := vk.Init(); err != nil {
		return nil, fmt.Errorf("vulkan: failed to initialize: %w", err)
	}

	// Create Commands and load global Vulkan functions
	cmds := vk.NewCommands()
	if err := cmds.LoadGlobal(); err != nil {
		return nil, fmt.Errorf("vulkan: failed to load global commands: %w", err)
	}

	// Prepare application info
	appName := []byte("gogpu\x00")
	engineName := []byte("gogpu/wgpu\x00")

	appInfo := vk.ApplicationInfo{
		SType:              vk.StructureTypeApplicationInfo,
		PApplicationName:   uintptr(unsafe.Pointer(&appName[0])),
		ApplicationVersion: vkMakeVersion(1, 0, 0),
		PEngineName:        uintptr(unsafe.Pointer(&engineName[0])),
		EngineVersion:      vkMakeVersion(0, 1, 0),
		ApiVersion:         vkMakeVersion(1, 2, 0), // Vulkan 1.2
	}

	// Required extensions
	extensions := []string{
		"VK_KHR_surface\x00",
	}

	// Platform-specific surface extension
	extensions = append(extensions, platformSurfaceExtension())

	// Optional: validation layers for debug (only if available)
	var layers []string
	var validationEnabled bool
	if desc != nil && desc.Flags&gputypes.InstanceFlagsDebug != 0 {
		if isLayerAvailable(cmds, "VK_LAYER_KHRONOS_validation") {
			layers = append(layers, "VK_LAYER_KHRONOS_validation\x00")
			extensions = append(extensions, "VK_EXT_debug_utils\x00")
			validationEnabled = true
		}
		// Silently skip if validation layers not installed (Vulkan SDK not present)
	}

	// Convert to C strings
	extensionPtrs := make([]uintptr, len(extensions))
	for i, ext := range extensions {
		extensionPtrs[i] = uintptr(unsafe.Pointer(unsafe.StringData(ext)))
	}

	layerPtrs := make([]uintptr, len(layers))
	for i, layer := range layers {
		layerPtrs[i] = uintptr(unsafe.Pointer(unsafe.StringData(layer)))
	}

	// Create instance
	createInfo := vk.InstanceCreateInfo{
		SType:                 vk.StructureTypeInstanceCreateInfo,
		PApplicationInfo:      &appInfo,
		EnabledExtensionCount: uint32(len(extensions)),
		EnabledLayerCount:     uint32(len(layers)),
	}

	if len(extensionPtrs) > 0 {
		createInfo.PpEnabledExtensionNames = uintptr(unsafe.Pointer(&extensionPtrs[0]))
	}
	if len(layerPtrs) > 0 {
		createInfo.PpEnabledLayerNames = uintptr(unsafe.Pointer(&layerPtrs[0]))
	}

	var instance vk.Instance
	result := cmds.CreateInstance(&createInfo, nil, &instance)
	if result != vk.Success {
		return nil, fmt.Errorf("vulkan: vkCreateInstance failed: %d", result)
	}

	// Load instance-level commands
	if err := cmds.LoadInstance(instance); err != nil {
		cmds.DestroyInstance(instance, nil)
		return nil, fmt.Errorf("vulkan: failed to load instance commands: %w", err)
	}

	// Set vkGetDeviceProcAddr for device function loading.
	// Some drivers (e.g., Intel) don't support loading it with instance=0.
	vk.SetDeviceProcAddr(instance)

	// Keep references alive
	runtime.KeepAlive(appName)
	runtime.KeepAlive(engineName)
	runtime.KeepAlive(extensions)
	runtime.KeepAlive(layers)
	runtime.KeepAlive(extensionPtrs)
	runtime.KeepAlive(layerPtrs)

	inst := &Instance{
		handle:       instance,
		cmds:         *cmds,
		debugEnabled: validationEnabled,
	}

	// Create debug messenger when validation layers are active.
	// This captures validation errors and logs them via Go's log package.
	if validationEnabled {
		inst.debugMessenger = createDebugMessenger(inst)
	}

	hal.Logger().Info("vulkan: instance created",
		"apiVersion", fmt.Sprintf("%d.%d.%d", vkVersionMajor(appInfo.ApiVersion), vkVersionMinor(appInfo.ApiVersion), vkVersionPatch(appInfo.ApiVersion)),
		"validation", validationEnabled,
	)

	return inst, nil
}

// Instance implements hal.Instance for Vulkan.
type Instance struct {
	handle         vk.Instance
	cmds           vk.Commands
	debugMessenger vk.DebugUtilsMessengerEXT
	debugEnabled   bool
}

// EnumerateAdapters returns available Vulkan adapters (physical devices).
func (i *Instance) EnumerateAdapters(surfaceHint hal.Surface) []hal.ExposedAdapter {
	// Get physical device count
	var count uint32
	i.cmds.EnumeratePhysicalDevices(i.handle, &count, nil)
	if count == 0 {
		return nil
	}

	// Get physical devices
	devices := make([]vk.PhysicalDevice, count)
	i.cmds.EnumeratePhysicalDevices(i.handle, &count, &devices[0])

	adapters := make([]hal.ExposedAdapter, 0, count)
	hal.Logger().Debug("vulkan: enumerating adapters", "count", count)
	for _, device := range devices {
		// Get device properties
		var props vk.PhysicalDeviceProperties
		i.cmds.GetPhysicalDeviceProperties(device, &props)

		// Get device features
		var features vk.PhysicalDeviceFeatures
		i.cmds.GetPhysicalDeviceFeatures(device, &features)

		// Check surface support if surface hint provided
		if surfaceHint != nil {
			if s, ok := surfaceHint.(*Surface); ok && s.handle != 0 {
				var supported vk.Bool32
				i.cmds.GetPhysicalDeviceSurfaceSupportKHR(device, 0, s.handle, &supported)
				if supported == 0 {
					continue // Skip devices that don't support this surface
				}
			}
		}

		// Convert device type
		deviceType := gputypes.DeviceTypeOther
		switch props.DeviceType {
		case vk.PhysicalDeviceTypeDiscreteGpu:
			deviceType = gputypes.DeviceTypeDiscreteGPU
		case vk.PhysicalDeviceTypeIntegratedGpu:
			deviceType = gputypes.DeviceTypeIntegratedGPU
		case vk.PhysicalDeviceTypeVirtualGpu:
			deviceType = gputypes.DeviceTypeVirtualGPU
		case vk.PhysicalDeviceTypeCpu:
			deviceType = gputypes.DeviceTypeCPU
		}

		// Extract device name
		deviceName := cStringToGo(props.DeviceName[:])

		adapter := &Adapter{
			instance:       i,
			physicalDevice: device,
			properties:     props,
			features:       features,
		}

		hal.Logger().Info("vulkan: adapter found",
			"name", deviceName,
			"type", deviceType,
			"vendor", vendorIDToName(props.VendorID),
			"apiVersion", fmt.Sprintf("%d.%d.%d", vkVersionMajor(props.ApiVersion), vkVersionMinor(props.ApiVersion), vkVersionPatch(props.ApiVersion)),
		)

		adapters = append(adapters, hal.ExposedAdapter{
			Adapter: adapter,
			Info: gputypes.AdapterInfo{
				Name:       deviceName,
				Vendor:     vendorIDToName(props.VendorID),
				VendorID:   props.VendorID,
				DeviceID:   props.DeviceID,
				DeviceType: deviceType,
				Driver:     "Vulkan",
				DriverInfo: fmt.Sprintf("Vulkan %d.%d.%d",
					vkVersionMajor(props.ApiVersion),
					vkVersionMinor(props.ApiVersion),
					vkVersionPatch(props.ApiVersion)),
				Backend: gputypes.BackendVulkan,
			},
			Features: featuresFromPhysicalDevice(&features),
			Capabilities: hal.Capabilities{
				Limits: limitsFromProps(&props),
				AlignmentsMask: hal.Alignments{
					BufferCopyOffset: 4,
					BufferCopyPitch:  256,
				},
				DownlevelCapabilities: hal.DownlevelCapabilities{
					ShaderModel: 60, // SM6.0 equivalent
					Flags:       0,
				},
			},
		})
	}

	return adapters
}

// Destroy releases the Vulkan instance.
func (i *Instance) Destroy() {
	if i.handle != 0 {
		// Destroy debug messenger before the instance (required by Vulkan spec).
		if i.debugMessenger != 0 {
			destroyDebugMessenger(i, i.debugMessenger)
			i.debugMessenger = 0
		}
		i.cmds.DestroyInstance(i.handle, nil)
		i.handle = 0
	}
}

// Surface implements hal.Surface for Vulkan.
type Surface struct {
	handle    vk.SurfaceKHR
	instance  *Instance
	swapchain *Swapchain
	device    *Device
}

// Configure configures the surface for presentation.
//
// Returns hal.ErrZeroArea if width or height is zero.
// This commonly happens when the window is minimized or not yet fully visible.
// Wait until the window has valid dimensions before calling Configure again.
func (s *Surface) Configure(device hal.Device, config *hal.SurfaceConfiguration) error {
	// Validate dimensions first (before any side effects).
	// This matches wgpu-core behavior which returns ConfigureSurfaceError::ZeroArea.
	if config.Width == 0 || config.Height == 0 {
		return hal.ErrZeroArea
	}

	vkDevice, ok := device.(*Device)
	if !ok {
		return fmt.Errorf("vulkan: device is not a Vulkan device")
	}
	hal.Logger().Info("vulkan: surface configuring",
		"width", config.Width,
		"height", config.Height,
		"format", config.Format,
		"presentMode", config.PresentMode,
	)
	return s.createSwapchain(vkDevice, config)
}

// Unconfigure removes surface configuration.
func (s *Surface) Unconfigure(_ hal.Device) {
	if s.swapchain != nil {
		s.swapchain.Destroy()
		s.swapchain = nil
	}
	s.device = nil
}

// AcquireTexture acquires the next surface texture for rendering.
// Returns hal.ErrNotReady if no image is available (non-blocking mode).
func (s *Surface) AcquireTexture(_ hal.Fence) (*hal.AcquiredSurfaceTexture, error) {
	if s.swapchain == nil {
		return nil, fmt.Errorf("vulkan: surface not configured")
	}

	texture, suboptimal, err := s.swapchain.acquireNextImage()
	if err != nil {
		return nil, err
	}

	// No image available right now - skip this frame
	if texture == nil {
		return nil, hal.ErrNotReady
	}

	// Register swapchain with queue for proper synchronization in Submit.
	// This ensures the queue waits for image acquisition before rendering
	// and signals completion before present.
	if s.device != nil && s.device.queue != nil {
		s.device.queue.activeSwapchain = s.swapchain
		s.device.queue.acquireUsed = false // Reset for new frame
	}

	return &hal.AcquiredSurfaceTexture{
		Texture:    texture,
		Suboptimal: suboptimal,
	}, nil
}

// DiscardTexture discards a surface texture without presenting it.
func (s *Surface) DiscardTexture(_ hal.SurfaceTexture) {
	if s.swapchain != nil {
		s.swapchain.imageAcquired = false
	}
}

// Destroy releases the surface.
func (s *Surface) Destroy() {
	if s.swapchain != nil {
		s.swapchain.Destroy()
		s.swapchain = nil
	}
	if s.handle != 0 && s.instance != nil {
		s.instance.cmds.DestroySurfaceKHR(s.instance.handle, s.handle, nil)
		s.handle = 0
	}
}

// Helper functions

func vkMakeVersion(major, minor, patch uint32) uint32 {
	return (major << 22) | (minor << 12) | patch
}

func vkVersionMajor(version uint32) uint32 {
	return version >> 22
}

func vkVersionMinor(version uint32) uint32 {
	return (version >> 12) & 0x3FF
}

func vkVersionPatch(version uint32) uint32 {
	return version & 0xFFF
}

func cStringToGo(b []byte) string {
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}

func vendorIDToName(id uint32) string {
	switch id {
	case 0x1002:
		return "AMD"
	case 0x10DE:
		return "NVIDIA"
	case 0x8086:
		return "Intel"
	case 0x13B5:
		return "ARM"
	case 0x5143:
		return "Qualcomm"
	case 0x1010:
		return "ImgTec"
	default:
		return fmt.Sprintf("0x%04X", id)
	}
}

// featuresFromPhysicalDevice maps Vulkan physical device features to WebGPU features.
// Reference: wgpu-hal/src/vulkan/adapter.rs:584-829
func featuresFromPhysicalDevice(features *vk.PhysicalDeviceFeatures) gputypes.Features {
	var result gputypes.Features

	// Texture compression features
	if features.TextureCompressionBC != 0 {
		result |= gputypes.Features(gputypes.FeatureTextureCompressionBC)
	}
	if features.TextureCompressionETC2 != 0 {
		result |= gputypes.Features(gputypes.FeatureTextureCompressionETC2)
	}
	if features.TextureCompressionASTC_LDR != 0 {
		result |= gputypes.Features(gputypes.FeatureTextureCompressionASTC)
	}

	// Draw features
	if features.DrawIndirectFirstInstance != 0 {
		result |= gputypes.Features(gputypes.FeatureIndirectFirstInstance)
	}
	if features.MultiDrawIndirect != 0 {
		result |= gputypes.Features(gputypes.FeatureMultiDrawIndirect)
	}

	// Depth/clipping features
	if features.DepthClamp != 0 {
		result |= gputypes.Features(gputypes.FeatureDepthClipControl)
	}

	// Shader features
	if features.ShaderFloat64 != 0 {
		result |= gputypes.Features(gputypes.FeatureShaderFloat64)
	}

	// Query features
	if features.PipelineStatisticsQuery != 0 {
		result |= gputypes.Features(gputypes.FeaturePipelineStatisticsQuery)
	}

	// Depth32FloatStencil8 is always available in Vulkan 1.0+
	result |= gputypes.Features(gputypes.FeatureDepth32FloatStencil8)

	return result
}

// limitsFromProps maps Vulkan physical device limits to WebGPU limits.
// Reference: wgpu-hal/src/vulkan/adapter.rs:1254-1392
func limitsFromProps(props *vk.PhysicalDeviceProperties) gputypes.Limits {
	vkLimits := props.Limits

	// Start with default limits and override with actual hardware values
	limits := gputypes.DefaultLimits()

	// Texture dimensions
	limits.MaxTextureDimension1D = vkLimits.MaxImageDimension1D
	limits.MaxTextureDimension2D = vkLimits.MaxImageDimension2D
	limits.MaxTextureDimension3D = vkLimits.MaxImageDimension3D
	limits.MaxTextureArrayLayers = vkLimits.MaxImageArrayLayers

	// Descriptor/binding limits
	limits.MaxBindGroups = min(vkLimits.MaxBoundDescriptorSets, 8) // WebGPU max is 8
	limits.MaxSampledTexturesPerShaderStage = vkLimits.MaxPerStageDescriptorSampledImages
	limits.MaxSamplersPerShaderStage = vkLimits.MaxPerStageDescriptorSamplers
	limits.MaxStorageBuffersPerShaderStage = vkLimits.MaxPerStageDescriptorStorageBuffers
	limits.MaxStorageTexturesPerShaderStage = vkLimits.MaxPerStageDescriptorStorageImages
	limits.MaxUniformBuffersPerShaderStage = vkLimits.MaxPerStageDescriptorUniformBuffers

	// Buffer limits
	limits.MaxUniformBufferBindingSize = uint64(vkLimits.MaxUniformBufferRange)
	limits.MaxStorageBufferBindingSize = uint64(vkLimits.MaxStorageBufferRange)
	limits.MinUniformBufferOffsetAlignment = uint32(vkLimits.MinUniformBufferOffsetAlignment)
	limits.MinStorageBufferOffsetAlignment = uint32(vkLimits.MinStorageBufferOffsetAlignment)

	// Vertex limits
	limits.MaxVertexAttributes = min(vkLimits.MaxVertexInputAttributes, 32) // WebGPU max is 32
	limits.MaxVertexBufferArrayStride = min(vkLimits.MaxVertexInputBindingStride, 2048)

	// Color attachment limits
	limits.MaxColorAttachments = min(vkLimits.MaxColorAttachments, 8) // WebGPU max is 8

	// Compute limits
	limits.MaxComputeWorkgroupStorageSize = vkLimits.MaxComputeSharedMemorySize
	limits.MaxComputeInvocationsPerWorkgroup = vkLimits.MaxComputeWorkGroupInvocations
	limits.MaxComputeWorkgroupSizeX = vkLimits.MaxComputeWorkGroupSize[0]
	limits.MaxComputeWorkgroupSizeY = vkLimits.MaxComputeWorkGroupSize[1]
	limits.MaxComputeWorkgroupSizeZ = vkLimits.MaxComputeWorkGroupSize[2]
	limits.MaxComputeWorkgroupsPerDimension = vkLimits.MaxComputeWorkGroupCount[0]

	// Push constants
	limits.MaxPushConstantSize = vkLimits.MaxPushConstantsSize

	return limits
}

// isLayerAvailable checks if a Vulkan instance layer is available.
// Used to gracefully skip validation layers when Vulkan SDK is not installed.
func isLayerAvailable(cmds *vk.Commands, layerName string) bool {
	// Get layer count
	var count uint32
	cmds.EnumerateInstanceLayerProperties(&count, nil)
	if count == 0 {
		return false
	}

	// Get layer properties
	layers := make([]vk.LayerProperties, count)
	cmds.EnumerateInstanceLayerProperties(&count, &layers[0])

	// Check if requested layer is available
	for i := range layers {
		name := cStringToGo(layers[i].LayerName[:])
		if name == layerName {
			return true
		}
	}
	return false
}
