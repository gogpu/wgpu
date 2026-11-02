//go:build !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vulkan

import (
	"unsafe"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal/vulkan/vk"
)

// Device extensions behind the f16 feature. Vulkan 1.1 promoted the first two
// and Vulkan 1.2 the third; below those versions the device must enable them.
const (
	extStorageBufferStorageClass = "VK_KHR_storage_buffer_storage_class"
	ext16BitStorage              = "VK_KHR_16bit_storage"
	extShaderFloat16Int8         = "VK_KHR_shader_float16_int8"
	extIncrementalPresent        = "VK_KHR_incremental_present"
)

// extendedFeatureSupport is what adapter enumeration reads beyond
// VkPhysicalDeviceFeatures: the device extensions and the pNext feature
// structs of vkGetPhysicalDeviceFeatures2. A nil struct pointer means the
// device cannot report that struct (neither its core version nor the
// extension), so none of its features are available.
type extendedFeatureSupport struct {
	// apiVersion is the version the device is used at: the lower of its own
	// version and the instance version.
	apiVersion  uint32
	extensions  map[string]bool
	float16Int8 *vk.PhysicalDeviceShaderFloat16Int8Features
	storage16   *vk.PhysicalDevice16BitStorageFeatures
}

// shaderF16 reports whether the device can run WGSL f16, as Rust wgpu-hal
// decides it (vulkan/adapter.rs, PhysicalDeviceFeatures::to_wgpu): f16
// arithmetic plus 16-bit access to storage and uniform buffers.
// storageInputOutput16 is not required: naga passes f16 stage inputs and
// outputs as f32 unless the device enables it.
func (s *extendedFeatureSupport) shaderF16() bool {
	return s.float16Int8 != nil && s.storage16 != nil &&
		s.float16Int8.ShaderFloat16 != 0 &&
		s.storage16.StorageBuffer16BitAccess != 0 &&
		s.storage16.UniformAndStorageBuffer16BitAccess != 0
}

// shaderF16Extensions returns the NUL-terminated device extensions the f16
// feature needs at the device's API version: none on Vulkan 1.2, where both
// feature structs are core.
func (s *extendedFeatureSupport) shaderF16Extensions() []string {
	var names []string
	if s.apiVersion < vkMakeVersion(1, 1, 0) {
		names = append(names, ext16BitStorage+"\x00", extStorageBufferStorageClass+"\x00")
	}
	if s.apiVersion < vkMakeVersion(1, 2, 0) {
		names = append(names, extShaderFloat16Int8+"\x00")
	}
	return names
}

// storageInputOutput16 reports whether f16 stage inputs and outputs can stay
// f16 (StorageInputOutput16), which naga uses instead of the f32 polyfill.
func (s *extendedFeatureSupport) storageInputOutput16() bool {
	return s.storage16 != nil && s.storage16.StorageInputOutput16 != 0
}

// webGPUFeatures returns the WebGPU features that depend on the pNext feature
// structs.
func (s *extendedFeatureSupport) webGPUFeatures() gputypes.Features {
	var result gputypes.Features
	if s.shaderF16() {
		result |= gputypes.Features(gputypes.FeatureShaderF16)
	}
	return result
}

// queryExtendedFeatures reads the device extensions and the pNext feature
// structs the device can report at its API version.
func (i *Instance) queryExtendedFeatures(device vk.PhysicalDevice, deviceAPIVersion uint32) extendedFeatureSupport {
	s := extendedFeatureSupport{
		apiVersion: min(deviceAPIVersion, instanceAPIVersion),
		extensions: i.deviceExtensions(device),
	}
	if !i.cmds.HasPhysicalDeviceFeatures2() {
		return s
	}

	// Chain only the structs the device knows, as Rust does: a struct from an
	// extension the device lacks is not a valid pNext member.
	features2 := vk.PhysicalDeviceFeatures2{SType: vk.StructureTypePhysicalDeviceFeatures2}
	next := &features2.PNext
	var float16Int8 vk.PhysicalDeviceShaderFloat16Int8Features
	if s.apiVersion >= vkMakeVersion(1, 2, 0) || s.extensions[extShaderFloat16Int8] {
		float16Int8.SType = vk.StructureTypePhysicalDeviceShaderFloat16Int8Features
		*next = (*uintptr)(unsafe.Pointer(&float16Int8))
		next = &float16Int8.PNext
		s.float16Int8 = &float16Int8
	}
	var storage16 vk.PhysicalDevice16BitStorageFeatures
	if s.apiVersion >= vkMakeVersion(1, 1, 0) ||
		(s.extensions[ext16BitStorage] && s.extensions[extStorageBufferStorageClass]) {
		storage16.SType = vk.StructureTypePhysicalDevice16bitStorageFeatures
		*next = (*uintptr)(unsafe.Pointer(&storage16))
		s.storage16 = &storage16
	}
	if features2.PNext != nil {
		i.cmds.GetPhysicalDeviceFeatures2(device, &features2)
	}
	return s
}

// deviceExtensions returns the names of the extensions the physical device
// supports.
func (i *Instance) deviceExtensions(device vk.PhysicalDevice) map[string]bool {
	var count uint32
	i.cmds.EnumerateDeviceExtensionProperties(device, 0, &count, nil)
	if count == 0 {
		return nil
	}
	props := make([]vk.ExtensionProperties, count)
	i.cmds.EnumerateDeviceExtensionProperties(device, 0, &count, &props[0])
	names := make(map[string]bool, count)
	for j := range props[:min(count, uint32(len(props)))] {
		names[cStringToGo(props[j].ExtensionName[:])] = true
	}
	return names
}
