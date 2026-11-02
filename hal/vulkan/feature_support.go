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

	// Extensions behind shader capability features. Vulkan 1.2 promoted
	// shader_atomic_int64 and Vulkan 1.1 shader_draw_parameters.
	extShaderAtomicInt64         = "VK_KHR_shader_atomic_int64"
	extShaderAtomicFloat         = "VK_EXT_shader_atomic_float"
	extFragmentShaderBarycentric = "VK_KHR_fragment_shader_barycentric"
	extShaderDrawParameters      = "VK_KHR_shader_draw_parameters"
)

// minTimestampValidBits is the smallest non-zero timestampValidBits Vulkan
// allows: a queue family reports 36 to 64 valid bits, or 0 when it cannot
// write timestamps at all.
const minTimestampValidBits = 36

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
	core        vk.PhysicalDeviceFeatures
	float16Int8 *vk.PhysicalDeviceShaderFloat16Int8Features
	storage16   *vk.PhysicalDevice16BitStorageFeatures
	atomicInt64 *vk.PhysicalDeviceShaderAtomicInt64Features
	atomicFloat *vk.PhysicalDeviceShaderAtomicFloatFeaturesEXT
	barycentric *vk.PhysicalDeviceFragmentShaderBarycentricFeaturesKHR
	drawParams  *vk.PhysicalDeviceShaderDrawParametersFeatures
}

// int64Atomics reports 64-bit atomics on storage buffers and workgroup
// memory, behind both SHADER_INT64_ATOMIC_MIN_MAX and _ALL_OPS in Rust.
func (s *extendedFeatureSupport) int64Atomics() bool {
	return s.atomicInt64 != nil &&
		s.atomicInt64.ShaderBufferInt64Atomics != 0 && s.atomicInt64.ShaderSharedInt64Atomics != 0
}

// float32Atomics reports f32 atomic exchange and add on storage buffers.
func (s *extendedFeatureSupport) float32Atomics() bool {
	return s.atomicFloat != nil &&
		s.atomicFloat.ShaderBufferFloat32Atomics != 0 && s.atomicFloat.ShaderBufferFloat32AtomicAdd != 0
}

// fragmentBarycentrics reports VK_KHR_fragment_shader_barycentric, behind
// both SHADER_BARYCENTRICS and SHADER_PER_VERTEX in Rust.
func (s *extendedFeatureSupport) fragmentBarycentrics() bool {
	return s.barycentric != nil && s.barycentric.FragmentShaderBarycentric != 0
}

// drawParameters reports shaderDrawParameters (DrawIndex): the Vulkan 1.1
// feature, or the extension below 1.1, which has no feature struct.
func (s *extendedFeatureSupport) drawParameters() bool {
	if s.drawParams != nil {
		return s.drawParams.ShaderDrawParameters != 0
	}
	return s.extensions[extShaderDrawParameters]
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
// structs and on the queue family the device will create its queue in.
func (s *extendedFeatureSupport) webGPUFeatures(family vk.QueueFamilyProperties) gputypes.Features {
	var result gputypes.Features
	if s.shaderF16() {
		result |= gputypes.Features(gputypes.FeatureShaderF16)
	}
	// Rust wgpu-hal (vulkan/adapter.rs, PhysicalDeviceFeatures::to_wgpu)
	// offers timestamps when the queue family writes them.
	if family.TimestampValidBits >= minTimestampValidBits {
		result |= gputypes.Features(gputypes.FeatureTimestampQuery)
	}
	return result | s.shaderCapabilityFeatures()
}

// shaderCapabilityFeatures returns the shader capability features (the naga
// capabilities a device grants), each as Rust wgpu-hal detects it
// (vulkan/adapter.rs, PhysicalDeviceFeatures::to_wgpu), with Rust's groups:
// both 64-bit atomic features from one struct, barycentrics and per-vertex
// inputs from one struct.
func (s *extendedFeatureSupport) shaderCapabilityFeatures() gputypes.Features {
	var result gputypes.Features
	set := func(f gputypes.Feature, on bool) {
		if on {
			result.Insert(f)
		}
	}
	set(gputypes.FeatureShaderEarlyDepthTest, true)
	set(gputypes.FeaturePrimitiveIndex, s.core.GeometryShader != 0)
	set(gputypes.FeatureClipDistances, s.core.ShaderClipDistance != 0)
	set(gputypes.FeatureShaderInt64, s.core.ShaderInt64 != 0)
	set(gputypes.FeatureShaderInt64AtomicMinMax, s.int64Atomics())
	set(gputypes.FeatureShaderInt64AtomicAllOps, s.int64Atomics())
	set(gputypes.FeatureShaderFloat32Atomic, s.float32Atomics())
	set(gputypes.FeatureShaderBarycentrics, s.fragmentBarycentrics())
	set(gputypes.FeatureShaderPerVertex, s.fragmentBarycentrics())
	set(gputypes.FeatureShaderDrawIndex, s.drawParameters())
	return result
}

// extendedWebGPUFeatures returns the features decided by the pNext feature
// structs and by the queue family the device will create its queue in:
// queueFamily for a surface-qualified adapter, the first graphics family
// otherwise (the family Open picks).
func (a *Adapter) extendedWebGPUFeatures(queueFamily *uint32) gputypes.Features {
	var family vk.QueueFamilyProperties
	if families, err := a.queueFamilies(); err == nil {
		if idx, err := selectGraphicsQueueFamily(families, queueFamily); err == nil {
			family = families[idx]
		}
	}
	return a.ext.webGPUFeatures(family)
}

// queryExtendedFeatures reads the device extensions and the pNext feature
// structs the device can report at its API version.
func (i *Instance) queryExtendedFeatures(device vk.PhysicalDevice, deviceAPIVersion uint32, core *vk.PhysicalDeviceFeatures) *extendedFeatureSupport {
	s := &extendedFeatureSupport{
		apiVersion: min(deviceAPIVersion, instanceAPIVersion),
		extensions: i.deviceExtensions(device),
		core:       *core,
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
		next = &storage16.PNext
		s.storage16 = &storage16
	}
	var atomicInt64 vk.PhysicalDeviceShaderAtomicInt64Features
	if s.apiVersion >= vkMakeVersion(1, 2, 0) || s.extensions[extShaderAtomicInt64] {
		atomicInt64.SType = vk.StructureTypePhysicalDeviceShaderAtomicInt64Features
		*next = (*uintptr)(unsafe.Pointer(&atomicInt64))
		next = &atomicInt64.PNext
		s.atomicInt64 = &atomicInt64
	}
	var atomicFloat vk.PhysicalDeviceShaderAtomicFloatFeaturesEXT
	if s.extensions[extShaderAtomicFloat] {
		atomicFloat.SType = vk.StructureTypePhysicalDeviceShaderAtomicFloatFeaturesExt
		*next = (*uintptr)(unsafe.Pointer(&atomicFloat))
		next = &atomicFloat.PNext
		s.atomicFloat = &atomicFloat
	}
	var barycentric vk.PhysicalDeviceFragmentShaderBarycentricFeaturesKHR
	if s.extensions[extFragmentShaderBarycentric] {
		barycentric.SType = vk.StructureTypePhysicalDeviceFragmentShaderBarycentricFeaturesKhr
		*next = (*uintptr)(unsafe.Pointer(&barycentric))
		next = &barycentric.PNext
		s.barycentric = &barycentric
	}
	var drawParams vk.PhysicalDeviceShaderDrawParametersFeatures
	if s.apiVersion >= vkMakeVersion(1, 1, 0) {
		drawParams.SType = vk.StructureTypePhysicalDeviceShaderDrawParametersFeatures
		*next = (*uintptr)(unsafe.Pointer(&drawParams))
		s.drawParams = &drawParams
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

// enableableFeatures are the features Open enables through the device create
// info; a request for one the adapter does not offer is refused.
const enableableFeatures = gputypes.Features(gputypes.FeatureShaderF16) |
	gputypes.Features(gputypes.FeaturePrimitiveIndex) |
	gputypes.Features(gputypes.FeatureClipDistances) |
	gputypes.Features(gputypes.FeatureShaderEarlyDepthTest) |
	gputypes.Features(gputypes.FeatureShaderInt64) |
	gputypes.Features(gputypes.FeatureShaderInt64AtomicMinMax) |
	gputypes.Features(gputypes.FeatureShaderInt64AtomicAllOps) |
	gputypes.Features(gputypes.FeatureShaderFloat32Atomic) |
	gputypes.Features(gputypes.FeatureShaderBarycentrics) |
	gputypes.Features(gputypes.FeatureShaderPerVertex) |
	gputypes.Features(gputypes.FeatureShaderDrawIndex)

// unsupportedRequest returns the requested features Open enables that the
// adapter does not offer.
func (s *extendedFeatureSupport) unsupportedRequest(requested gputypes.Features) gputypes.Features {
	offered := s.shaderCapabilityFeatures()
	if s.shaderF16() {
		offered.Insert(gputypes.FeatureShaderF16)
	}
	return requested & enableableFeatures &^ offered
}

// deviceFeatureChain holds the feature structs a device is created with. The
// create info points into it, so it must stay alive until vkCreateDevice
// returns.
type deviceFeatureChain struct {
	core        vk.PhysicalDeviceFeatures
	vulkan12    vk.PhysicalDeviceVulkan12Features
	float16Int8 vk.PhysicalDeviceShaderFloat16Int8Features
	storage16   vk.PhysicalDevice16BitStorageFeatures
	atomicInt64 vk.PhysicalDeviceShaderAtomicInt64Features
	atomicFloat vk.PhysicalDeviceShaderAtomicFloatFeaturesEXT
	barycentric vk.PhysicalDeviceFragmentShaderBarycentricFeaturesKHR
	drawParams  vk.PhysicalDeviceShaderDrawParametersFeatures
}

// enable fills c with the features to create the device with and returns
// the extensions they need (NUL-terminated), as Rust wgpu-hal enables them
// (vulkan/adapter.rs, from_extensions_and_requested_features and
// get_required_extensions): every feature struct only when a requested
// feature needs it. The core features keep their supported values except
// the ones a shader capability feature gates, which follow the request.
//
// A chain holding PhysicalDeviceVulkan12Features must not also hold the
// structs Vulkan 1.2 absorbed (VUID-VkDeviceCreateInfo-pNext-02830), so on
// Vulkan 1.2 shaderFloat16, the 64-bit atomics and the timeline semaphore go
// into the Vulkan 1.2 struct.
func (s *extendedFeatureSupport) enable(c *deviceFeatureChain, requested gputypes.Features, timelineSemaphore bool) (pNext *uintptr, extensions []string) {
	f16 := requested.Contains(gputypes.FeatureShaderF16)
	int64Atomics := requested.Contains(gputypes.FeatureShaderInt64AtomicMinMax) ||
		requested.Contains(gputypes.FeatureShaderInt64AtomicAllOps)
	barycentrics := requested.Contains(gputypes.FeatureShaderBarycentrics) ||
		requested.Contains(gputypes.FeatureShaderPerVertex)
	floatAtomics := requested.Contains(gputypes.FeatureShaderFloat32Atomic)
	drawIndex := requested.Contains(gputypes.FeatureShaderDrawIndex)
	vulkan12 := s.apiVersion >= vkMakeVersion(1, 2, 0)

	c.core = s.core
	c.core.GeometryShader = boolToVk(requested.Contains(gputypes.FeaturePrimitiveIndex))
	c.core.ShaderClipDistance = boolToVk(requested.Contains(gputypes.FeatureClipDistances))
	c.core.ShaderInt64 = boolToVk(requested.Contains(gputypes.FeatureShaderInt64))
	c.core.ShaderInt16 = vk.Bool32(vk.False)

	tail := &pNext
	link := func(sType vk.StructureType, header *vk.StructureType, next **uintptr, ptr unsafe.Pointer) {
		*header = sType
		*tail = (*uintptr)(ptr)
		tail = next
	}
	if timelineSemaphore || (vulkan12 && (f16 || int64Atomics)) {
		c.vulkan12.TimelineSemaphore = boolToVk(timelineSemaphore)
		c.vulkan12.ShaderFloat16 = boolToVk(vulkan12 && f16)
		c.vulkan12.ShaderBufferInt64Atomics = boolToVk(vulkan12 && int64Atomics)
		c.vulkan12.ShaderSharedInt64Atomics = boolToVk(vulkan12 && int64Atomics)
		link(vk.StructureTypePhysicalDeviceVulkan12Features, &c.vulkan12.SType, &c.vulkan12.PNext, unsafe.Pointer(&c.vulkan12))
	}
	if f16 {
		extensions = append(extensions, s.shaderF16Extensions()...)
		if !vulkan12 {
			c.float16Int8.ShaderFloat16 = vk.Bool32(vk.True)
			link(vk.StructureTypePhysicalDeviceShaderFloat16Int8Features, &c.float16Int8.SType, &c.float16Int8.PNext, unsafe.Pointer(&c.float16Int8))
		}
		c.storage16.StorageBuffer16BitAccess = vk.Bool32(vk.True)
		c.storage16.UniformAndStorageBuffer16BitAccess = vk.Bool32(vk.True)
		c.storage16.StorageInputOutput16 = boolToVk(s.storageInputOutput16())
		link(vk.StructureTypePhysicalDevice16bitStorageFeatures, &c.storage16.SType, &c.storage16.PNext, unsafe.Pointer(&c.storage16))
	}
	if int64Atomics && !vulkan12 {
		extensions = append(extensions, extShaderAtomicInt64+"\x00")
		c.atomicInt64.ShaderBufferInt64Atomics = vk.Bool32(vk.True)
		c.atomicInt64.ShaderSharedInt64Atomics = vk.Bool32(vk.True)
		link(vk.StructureTypePhysicalDeviceShaderAtomicInt64Features, &c.atomicInt64.SType, &c.atomicInt64.PNext, unsafe.Pointer(&c.atomicInt64))
	}
	if floatAtomics {
		extensions = append(extensions, extShaderAtomicFloat+"\x00")
		c.atomicFloat.ShaderBufferFloat32Atomics = vk.Bool32(vk.True)
		c.atomicFloat.ShaderBufferFloat32AtomicAdd = vk.Bool32(vk.True)
		link(vk.StructureTypePhysicalDeviceShaderAtomicFloatFeaturesExt, &c.atomicFloat.SType, &c.atomicFloat.PNext, unsafe.Pointer(&c.atomicFloat))
	}
	if barycentrics {
		extensions = append(extensions, extFragmentShaderBarycentric+"\x00")
		c.barycentric.FragmentShaderBarycentric = vk.Bool32(vk.True)
		link(vk.StructureTypePhysicalDeviceFragmentShaderBarycentricFeaturesKhr, &c.barycentric.SType, &c.barycentric.PNext, unsafe.Pointer(&c.barycentric))
	}
	if drawIndex {
		if s.apiVersion >= vkMakeVersion(1, 1, 0) {
			c.drawParams.ShaderDrawParameters = vk.Bool32(vk.True)
			link(vk.StructureTypePhysicalDeviceShaderDrawParametersFeatures, &c.drawParams.SType, &c.drawParams.PNext, unsafe.Pointer(&c.drawParams))
		} else {
			extensions = append(extensions, extShaderDrawParameters+"\x00")
		}
	}
	return pNext, extensions
}
