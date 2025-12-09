// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows

package vulkan

import (
	"fmt"
	"runtime"
	"syscall"
	"unsafe"

	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/vulkan/vk"
	"github.com/gogpu/wgpu/types"
)

// Backend implements hal.Backend for Vulkan.
type Backend struct{}

// Variant returns the backend type identifier.
func (Backend) Variant() types.Backend {
	return types.BackendVulkan
}

// CreateInstance creates a new Vulkan instance.
func (Backend) CreateInstance(desc *hal.InstanceDescriptor) (hal.Instance, error) {
	// Initialize Vulkan library
	if err := vk.Init(); err != nil {
		return nil, fmt.Errorf("vulkan: failed to initialize: %w", err)
	}

	// Load global commands
	var cmds vk.Commands
	cmds.LoadGlobal()

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
	switch runtime.GOOS {
	case "windows":
		extensions = append(extensions, "VK_KHR_win32_surface\x00")
	case "linux":
		extensions = append(extensions, "VK_KHR_xlib_surface\x00")
	case "darwin":
		extensions = append(extensions, "VK_EXT_metal_surface\x00")
	}

	// Optional: validation layers for debug
	var layers []string
	if desc != nil && desc.Flags&types.InstanceFlagsDebug != 0 {
		layers = append(layers, "VK_LAYER_KHRONOS_validation\x00")
		extensions = append(extensions, "VK_EXT_debug_utils\x00")
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
	result := vkCreateInstance(&cmds, &createInfo, nil, &instance)
	if result != vk.Success {
		return nil, fmt.Errorf("vulkan: vkCreateInstance failed: %d", result)
	}

	// Load instance-level commands
	cmds.LoadInstance(instance)

	// Keep references alive
	runtime.KeepAlive(appName)
	runtime.KeepAlive(engineName)
	runtime.KeepAlive(extensions)
	runtime.KeepAlive(layers)
	runtime.KeepAlive(extensionPtrs)
	runtime.KeepAlive(layerPtrs)

	return &Instance{
		handle: instance,
		cmds:   cmds,
	}, nil
}

// Instance implements hal.Instance for Vulkan.
type Instance struct {
	handle vk.Instance
	cmds   vk.Commands
}

// CreateSurface creates a Vulkan surface from window handles.
func (i *Instance) CreateSurface(displayHandle, windowHandle uintptr) (hal.Surface, error) {
	var surface vk.SurfaceKHR

	// Windows: VK_KHR_win32_surface
	createInfo := vk.Win32SurfaceCreateInfoKHR{
		SType:     vk.StructureTypeWin32SurfaceCreateInfoKhr,
		Hinstance: getModuleHandle(),
		Hwnd:      windowHandle,
	}

	result := vkCreateWin32SurfaceKHR(i, &createInfo, nil, &surface)
	if result != vk.Success {
		return nil, fmt.Errorf("vulkan: vkCreateWin32SurfaceKHR failed: %d", result)
	}

	return &Surface{
		handle:   surface,
		instance: i,
	}, nil
}

// EnumerateAdapters returns available Vulkan adapters (physical devices).
func (i *Instance) EnumerateAdapters(surfaceHint hal.Surface) []hal.ExposedAdapter {
	// Get physical device count
	var count uint32
	vkEnumeratePhysicalDevices(i, &count, nil)
	if count == 0 {
		return nil
	}

	// Get physical devices
	devices := make([]vk.PhysicalDevice, count)
	vkEnumeratePhysicalDevices(i, &count, &devices[0])

	adapters := make([]hal.ExposedAdapter, 0, count)
	for _, device := range devices {
		// Get device properties
		var props vk.PhysicalDeviceProperties
		vkGetPhysicalDeviceProperties(i, device, &props)

		// Get device features
		var features vk.PhysicalDeviceFeatures
		vkGetPhysicalDeviceFeatures(i, device, &features)

		// Check surface support if surface hint provided
		if surfaceHint != nil {
			if s, ok := surfaceHint.(*Surface); ok {
				var supported vk.Bool32
				vkGetPhysicalDeviceSurfaceSupportKHR(i, device, 0, s.handle, &supported)
				if supported == 0 {
					continue // Skip devices that don't support this surface
				}
			}
		}

		// Convert device type
		deviceType := types.DeviceTypeOther
		switch props.DeviceType {
		case vk.PhysicalDeviceTypeDiscreteGpu:
			deviceType = types.DeviceTypeDiscreteGPU
		case vk.PhysicalDeviceTypeIntegratedGpu:
			deviceType = types.DeviceTypeIntegratedGPU
		case vk.PhysicalDeviceTypeVirtualGpu:
			deviceType = types.DeviceTypeVirtualGPU
		case vk.PhysicalDeviceTypeCpu:
			deviceType = types.DeviceTypeCPU
		}

		// Extract device name
		deviceName := cStringToGo(props.DeviceName[:])

		adapter := &Adapter{
			instance:       i,
			physicalDevice: device,
			properties:     props,
			features:       features,
		}

		adapters = append(adapters, hal.ExposedAdapter{
			Adapter: adapter,
			Info: types.AdapterInfo{
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
				Backend: types.BackendVulkan,
			},
			Features: 0, // TODO: Map Vulkan features to WebGPU features
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
		vkDestroyInstance(i, i.handle, nil)
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
func (s *Surface) Configure(device hal.Device, config *hal.SurfaceConfiguration) error {
	vkDevice, ok := device.(*Device)
	if !ok {
		return fmt.Errorf("vulkan: device is not a Vulkan device")
	}
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
func (s *Surface) AcquireTexture(_ hal.Fence) (*hal.AcquiredSurfaceTexture, error) {
	if s.swapchain == nil {
		return nil, fmt.Errorf("vulkan: surface not configured")
	}

	texture, suboptimal, err := s.swapchain.acquireNextImage()
	if err != nil {
		return nil, err
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
		vkDestroySurfaceKHR(s.instance, s.handle, nil)
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

func limitsFromProps(props *vk.PhysicalDeviceProperties) types.Limits {
	_ = props // TODO: Map Vulkan limits to WebGPU limits from props.Limits
	return types.DefaultLimits()
}

// Vulkan function wrappers using syscall.SyscallN

func vkCreateInstance(cmds *vk.Commands, createInfo *vk.InstanceCreateInfo, allocator unsafe.Pointer, instance *vk.Instance) vk.Result {
	r, _, _ := syscall.SyscallN(cmds.CreateInstance(),
		uintptr(unsafe.Pointer(createInfo)),
		uintptr(allocator),
		uintptr(unsafe.Pointer(instance)))
	return vk.Result(r)
}

func vkDestroyInstance(i *Instance, instance vk.Instance, allocator unsafe.Pointer) {
	//nolint:errcheck // Vulkan void function, no return value to check
	syscall.SyscallN(i.cmds.DestroyInstance(),
		uintptr(instance),
		uintptr(allocator))
}

//nolint:unparam // result is used in caller but linter doesn't see it
func vkEnumeratePhysicalDevices(i *Instance, count *uint32, devices *vk.PhysicalDevice) vk.Result {
	r, _, _ := syscall.SyscallN(i.cmds.EnumeratePhysicalDevices(),
		uintptr(i.handle),
		uintptr(unsafe.Pointer(count)),
		uintptr(unsafe.Pointer(devices)))
	return vk.Result(r)
}

func vkGetPhysicalDeviceProperties(i *Instance, device vk.PhysicalDevice, props *vk.PhysicalDeviceProperties) {
	//nolint:errcheck // Vulkan void function, no return value to check
	syscall.SyscallN(i.cmds.GetPhysicalDeviceProperties(),
		uintptr(device),
		uintptr(unsafe.Pointer(props)))
}

func vkGetPhysicalDeviceFeatures(i *Instance, device vk.PhysicalDevice, features *vk.PhysicalDeviceFeatures) {
	//nolint:errcheck // Vulkan void function, no return value to check
	syscall.SyscallN(i.cmds.GetPhysicalDeviceFeatures(),
		uintptr(device),
		uintptr(unsafe.Pointer(features)))
}

func vkCreateWin32SurfaceKHR(i *Instance, createInfo *vk.Win32SurfaceCreateInfoKHR, allocator unsafe.Pointer, surface *vk.SurfaceKHR) vk.Result {
	proc := vk.GetInstanceProcAddr(i.handle, "vkCreateWin32SurfaceKHR")
	if proc == 0 {
		return vk.ErrorExtensionNotPresent
	}
	r, _, _ := syscall.SyscallN(proc,
		uintptr(i.handle),
		uintptr(unsafe.Pointer(createInfo)),
		uintptr(allocator),
		uintptr(unsafe.Pointer(surface)))
	return vk.Result(r)
}

func vkDestroySurfaceKHR(i *Instance, surface vk.SurfaceKHR, allocator unsafe.Pointer) {
	proc := vk.GetInstanceProcAddr(i.handle, "vkDestroySurfaceKHR")
	if proc == 0 {
		return
	}
	//nolint:errcheck // Vulkan void function, no return value to check
	syscall.SyscallN(proc,
		uintptr(i.handle),
		uintptr(surface),
		uintptr(allocator))
}

func vkGetPhysicalDeviceSurfaceSupportKHR(i *Instance, device vk.PhysicalDevice, queueFamily uint32, surface vk.SurfaceKHR, supported *vk.Bool32) vk.Result {
	proc := vk.GetInstanceProcAddr(i.handle, "vkGetPhysicalDeviceSurfaceSupportKHR")
	if proc == 0 {
		return vk.ErrorExtensionNotPresent
	}
	r, _, _ := syscall.SyscallN(proc,
		uintptr(device),
		uintptr(queueFamily),
		uintptr(surface),
		uintptr(unsafe.Pointer(supported)))
	return vk.Result(r)
}

// Windows-specific
var (
	kernel32             = syscall.NewLazyDLL("kernel32.dll")
	procGetModuleHandleW = kernel32.NewProc("GetModuleHandleW")
)

func getModuleHandle() uintptr {
	h, _, _ := procGetModuleHandleW.Call(0)
	return h
}
