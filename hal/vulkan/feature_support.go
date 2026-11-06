//go:build !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vulkan

import (
	"fmt"
	"unsafe"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/vulkan/vk"
)

// Device extensions behind the f16 feature. Vulkan 1.1 promoted the first two
// and Vulkan 1.2 the third; below those versions the device must enable them.
const (
	extStorageBufferStorageClass = "VK_KHR_storage_buffer_storage_class"
	ext16BitStorage              = "VK_KHR_16bit_storage"
	extShaderFloat16Int8         = "VK_KHR_shader_float16_int8"
	extIncrementalPresent        = "VK_KHR_incremental_present"
	// extMaintenance1 allows 2D views of a 3D image's depth slices, for
	// render passes on a depth slice. Vulkan 1.1 promoted it.
	extMaintenance1 = "VK_KHR_maintenance1"

	// Extensions behind shader capability features. Vulkan 1.2 promoted
	// shader_atomic_int64 and Vulkan 1.1 shader_draw_parameters.
	extShaderAtomicInt64         = "VK_KHR_shader_atomic_int64"
	extShaderAtomicFloat         = "VK_EXT_shader_atomic_float"
	extFragmentShaderBarycentric = "VK_KHR_fragment_shader_barycentric"
	extShaderDrawParameters      = "VK_KHR_shader_draw_parameters"

	// Robustness extensions, enabled wherever supported. Vulkan 1.3 promoted
	// image_robustness; driver_properties (Vulkan 1.2) identifies the driver
	// for the robustness2 exclusion below.
	extImageRobustness  = "VK_EXT_image_robustness"
	extRobustness2      = "VK_EXT_robustness2"
	extDriverProperties = "VK_KHR_driver_properties"

	// extZeroInitWorkgroupMemory lets a shader zero workgroup memory with a
	// null initializer (naga's Native mode). Vulkan 1.3 promoted it.
	extZeroInitWorkgroupMemory = "VK_KHR_zero_initialize_workgroup_memory"

	// extSubgroupSizeControl lets a pipeline stage run with any subgroup size
	// the device picks (ALLOW_VARYING_SUBGROUP_SIZE), which a device with the
	// subgroup feature sets on every stage. Vulkan 1.3 promoted it.
	extSubgroupSizeControl = "VK_EXT_subgroup_size_control"
)

// requiredSubgroupOperations are the subgroup operation classes WGSL
// subgroups need, as Rust wgpu-hal requires them (vulkan/adapter.rs,
// PhysicalDeviceFeatures::to_wgpu): naga's SPIR-V backend emits each class
// for its group of builtins.
const requiredSubgroupOperations = vk.SubgroupFeatureFlags(vk.SubgroupFeatureBasicBit |
	vk.SubgroupFeatureVoteBit |
	vk.SubgroupFeatureArithmeticBit |
	vk.SubgroupFeatureBallotBit |
	vk.SubgroupFeatureShuffleBit |
	vk.SubgroupFeatureShuffleRelativeBit |
	vk.SubgroupFeatureQuadBit)

// subgroupStagesComputeFragment are the stages Rust's SUBGROUP needs.
const subgroupStagesComputeFragment = vk.ShaderStageFlags(vk.ShaderStageComputeBit | vk.ShaderStageFragmentBit)

// Intel's proprietary Windows driver for integrated GPUs breaks rendering
// with VK_EXT_robustness2 before 31.0.101.2115 (gfx-rs/wgpu#4599). Rust
// wgpu-hal hides the extension there (vulkan/adapter.rs,
// is_intel_igpu_outdated_for_robustness2); so do we. The driver packs the
// last two parts of its version as (101 << 14) | 2115.
const (
	intelVendorID                 = 0x8086
	intelRobustness2DriverVersion = 101<<14 | 2115
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
	imageRobust *vk.PhysicalDeviceImageRobustnessFeatures
	robustness2 *vk.PhysicalDeviceRobustness2FeaturesKHR
	zeroInitWG  *vk.PhysicalDeviceZeroInitializeWorkgroupMemoryFeatures
	sizeControl *vk.PhysicalDeviceSubgroupSizeControlFeatures
	// subgroup is VkPhysicalDeviceSubgroupProperties, read through
	// vkGetPhysicalDeviceProperties2 on Vulkan 1.1 and later.
	subgroup *vk.PhysicalDeviceSubgroupProperties
}

// subgroupFeatures returns the subgroup features as Rust wgpu-hal decides
// them (vulkan/adapter.rs, PhysicalDeviceFeatures::to_wgpu): with subgroup
// size control (Vulkan 1.3 or VK_EXT_subgroup_size_control) and every
// operation class WGSL needs, SUBGROUP where compute and fragment shaders
// support subgroups and SUBGROUP_VERTEX where vertex shaders do, and
// SUBGROUP_BARRIER whenever the operations are supported.
//
// SUBGROUP_BARRIER needs naga v0.21.1 or later: earlier naga (and Rust naga at
// the time of writing) compiled subgroupBarrier to OpControlBarrier with
// AcquireRelease|SubgroupMemory semantics, which the Vulkan validation layer
// rejects (VUID-StandaloneSpirv-MemorySemantics-10870).
func (s *extendedFeatureSupport) subgroupFeatures() gputypes.Features {
	if s.subgroup == nil || !s.hasSubgroupSizeControl() ||
		s.subgroup.SupportedOperations&requiredSubgroupOperations != requiredSubgroupOperations {
		return 0
	}
	var result gputypes.Features
	result.Insert(gputypes.FeatureSubgroupBarrier)
	if s.subgroup.SupportedStages&subgroupStagesComputeFragment == subgroupStagesComputeFragment {
		result.Insert(gputypes.FeatureSubgroupOperations)
	}
	if s.subgroup.SupportedStages&vk.ShaderStageFlags(vk.ShaderStageVertexBit) != 0 {
		result.Insert(gputypes.FeatureSubgroupVertex)
	}
	return result
}

// has2DViewsOf3D reports whether a 2D view of one depth slice of a 3D image
// can be created (VK_IMAGE_CREATE_2D_ARRAY_COMPATIBLE_BIT), which rendering
// into a depth slice needs: Vulkan 1.1 core or VK_KHR_maintenance1. Rust
// wgpu-hal requires maintenance1 of every adapter (vulkan/adapter.rs).
func (s *extendedFeatureSupport) has2DViewsOf3D() bool {
	return s.apiVersion >= vkMakeVersion(1, 1, 0) || s.extensions[extMaintenance1]
}

// maintenance1Extensions returns the NUL-terminated extension that gives a
// Vulkan 1.0 device 2D views of 3D images: none from Vulkan 1.1, where
// maintenance1 is core, or when the device does not have it.
func (s *extendedFeatureSupport) maintenance1Extensions() []string {
	if s.apiVersion >= vkMakeVersion(1, 1, 0) || !s.extensions[extMaintenance1] {
		return nil
	}
	return []string{extMaintenance1 + "\x00"}
}

// hasSubgroupSizeControl reports whether the device can enable subgroup
// size control: Vulkan 1.3 core or VK_EXT_subgroup_size_control.
func (s *extendedFeatureSupport) hasSubgroupSizeControl() bool {
	return s.apiVersion >= vkMakeVersion(1, 3, 0) || s.extensions[extSubgroupSizeControl]
}

// allowVaryingSubgroupSize reports whether a device created for requested
// sets ALLOW_VARYING_SUBGROUP_SIZE on its pipeline stages, as Rust wgpu-hal
// does on a device with SUBGROUP (vulkan/device.rs, compile_stage): the
// subgroup_size builtin then reports the size the stage really runs with.
// The flag needs the subgroupSizeControl feature, which enable turns on with
// it (VUID-VkPipelineShaderStageCreateInfo-flags-02784).
func (s *extendedFeatureSupport) allowVaryingSubgroupSize(requested gputypes.Features) bool {
	return requested.Contains(gputypes.FeatureSubgroupOperations) &&
		s.sizeControl != nil && s.sizeControl.SubgroupSizeControl != 0
}

// zeroInitializeWorkgroupMemory reports shaderZeroInitializeWorkgroupMemory:
// the driver zeroes workgroup variables declared with a null initializer, so
// naga can use its Native mode instead of zero stores in the shader (Rust
// PrivateCapabilities::zero_initialize_workgroup_memory).
func (s *extendedFeatureSupport) zeroInitializeWorkgroupMemory() bool {
	return s.zeroInitWG != nil && s.zeroInitWG.ShaderZeroInitializeWorkgroupMemory != 0
}

// robustBufferAccess2 reports whether the device bounds-checks shader buffer
// accesses itself (VK_EXT_robustness2): out-of-bounds reads return zero and
// writes are discarded, so naga's buffer checks can be left out. It needs
// the core robustBufferAccess, which Open enables with the other supported
// core features (VUID-VkPhysicalDeviceRobustness2FeaturesEXT-robustBufferAccess2-04000).
func (s *extendedFeatureSupport) robustBufferAccess2() bool {
	return s.robustness2 != nil && s.robustness2.RobustBufferAccess2 != 0 && s.core.RobustBufferAccess != 0
}

// robustImageAccess reports whether the device bounds-checks image loads
// itself: robustImageAccess2 or robustImageAccess, as Rust's
// PrivateCapabilities::robust_image_access.
func (s *extendedFeatureSupport) robustImageAccess() bool {
	return (s.robustness2 != nil && s.robustness2.RobustImageAccess2 != 0) ||
		(s.imageRobust != nil && s.imageRobust.RobustImageAccess != 0)
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
	return result | s.subgroupFeatures()
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
func (i *Instance) queryExtendedFeatures(device vk.PhysicalDevice, props *vk.PhysicalDeviceProperties, core *vk.PhysicalDeviceFeatures) *extendedFeatureSupport {
	s := &extendedFeatureSupport{
		apiVersion: min(props.ApiVersion, instanceAPIVersion),
		extensions: i.deviceExtensions(device),
		core:       *core,
	}
	if !i.cmds.HasPhysicalDeviceFeatures2() {
		return s
	}
	if intelIGPUOutdatedForRobustness2(props, i.driverID(device, s)) {
		hal.Logger().Debug("vulkan: VK_EXT_robustness2 disabled: outdated Intel integrated GPU driver",
			"driverVersion", fmt.Sprintf("%#x", props.DriverVersion))
		delete(s.extensions, extRobustness2)
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
		next = &drawParams.PNext
		s.drawParams = &drawParams
	}
	var imageRobust vk.PhysicalDeviceImageRobustnessFeatures
	if s.apiVersion >= vkMakeVersion(1, 3, 0) || s.extensions[extImageRobustness] {
		imageRobust.SType = vk.StructureTypePhysicalDeviceImageRobustnessFeatures
		*next = (*uintptr)(unsafe.Pointer(&imageRobust))
		next = &imageRobust.PNext
		s.imageRobust = &imageRobust
	}
	var robustness2 vk.PhysicalDeviceRobustness2FeaturesKHR
	if s.extensions[extRobustness2] {
		robustness2.SType = vk.StructureTypePhysicalDeviceRobustness2FeaturesKhr
		*next = (*uintptr)(unsafe.Pointer(&robustness2))
		next = &robustness2.PNext
		s.robustness2 = &robustness2
	}
	var zeroInitWG vk.PhysicalDeviceZeroInitializeWorkgroupMemoryFeatures
	if s.apiVersion >= vkMakeVersion(1, 3, 0) || s.extensions[extZeroInitWorkgroupMemory] {
		zeroInitWG.SType = vk.StructureTypePhysicalDeviceZeroInitializeWorkgroupMemoryFeatures
		*next = (*uintptr)(unsafe.Pointer(&zeroInitWG))
		next = &zeroInitWG.PNext
		s.zeroInitWG = &zeroInitWG
	}
	var sizeControl vk.PhysicalDeviceSubgroupSizeControlFeatures
	if s.hasSubgroupSizeControl() {
		sizeControl.SType = vk.StructureTypePhysicalDeviceSubgroupSizeControlFeatures
		*next = (*uintptr)(unsafe.Pointer(&sizeControl))
		s.sizeControl = &sizeControl
	}
	if features2.PNext != nil {
		i.cmds.GetPhysicalDeviceFeatures2(device, &features2)
	}
	s.subgroup = i.subgroupProperties(device, s)
	return s
}

// subgroupProperties returns VkPhysicalDeviceSubgroupProperties, or nil
// below Vulkan 1.1, where the struct does not exist (Rust wgpu-hal
// vulkan/adapter.rs, query_capabilities).
func (i *Instance) subgroupProperties(device vk.PhysicalDevice, s *extendedFeatureSupport) *vk.PhysicalDeviceSubgroupProperties {
	if s.apiVersion < vkMakeVersion(1, 1, 0) {
		return nil
	}
	subgroup := &vk.PhysicalDeviceSubgroupProperties{SType: vk.StructureTypePhysicalDeviceSubgroupProperties}
	props2 := vk.PhysicalDeviceProperties2{
		SType: vk.StructureTypePhysicalDeviceProperties2,
		PNext: (*uintptr)(unsafe.Pointer(subgroup)),
	}
	i.cmds.GetPhysicalDeviceProperties2(device, &props2)
	return subgroup
}

// driverID returns the driver of the physical device from
// VkPhysicalDeviceDriverProperties (Vulkan 1.2 or VK_KHR_driver_properties),
// or 0 when the device cannot report it.
func (i *Instance) driverID(device vk.PhysicalDevice, s *extendedFeatureSupport) vk.DriverId {
	if s.apiVersion < vkMakeVersion(1, 2, 0) && !s.extensions[extDriverProperties] {
		return 0
	}
	driver := vk.PhysicalDeviceDriverProperties{SType: vk.StructureTypePhysicalDeviceDriverProperties}
	props2 := vk.PhysicalDeviceProperties2{
		SType: vk.StructureTypePhysicalDeviceProperties2,
		PNext: (*uintptr)(unsafe.Pointer(&driver)),
	}
	i.cmds.GetPhysicalDeviceProperties2(device, &props2)
	return driver.DriverID
}

// intelIGPUOutdatedForRobustness2 reports an Intel integrated GPU on the
// proprietary Windows driver older than 31.0.101.2115, which breaks
// rendering with VK_EXT_robustness2 (Rust wgpu-hal vulkan/adapter.rs,
// is_intel_igpu_outdated_for_robustness2).
func intelIGPUOutdatedForRobustness2(props *vk.PhysicalDeviceProperties, driver vk.DriverId) bool {
	return props.VendorID == intelVendorID &&
		props.DeviceType == vk.PhysicalDeviceTypeIntegratedGpu &&
		props.DriverVersion < intelRobustness2DriverVersion &&
		driver == vk.DriverIdIntelProprietaryWindows
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
	gputypes.Features(gputypes.FeatureShaderDrawIndex) |
	gputypes.Features(gputypes.FeatureSubgroupOperations) |
	gputypes.Features(gputypes.FeatureSubgroupBarrier) |
	gputypes.Features(gputypes.FeatureSubgroupVertex)

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
	imageRobust vk.PhysicalDeviceImageRobustnessFeatures
	robustness2 vk.PhysicalDeviceRobustness2FeaturesKHR
	zeroInitWG  vk.PhysicalDeviceZeroInitializeWorkgroupMemoryFeatures
	sizeControl vk.PhysicalDeviceSubgroupSizeControlFeatures
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
	// Robustness is enabled wherever supported, whatever was requested, as
	// Rust does: the SPIR-V bounds-check policies rely on it (spirvOptions).
	if s.imageRobust != nil && s.imageRobust.RobustImageAccess != 0 {
		if s.apiVersion < vkMakeVersion(1, 3, 0) {
			extensions = append(extensions, extImageRobustness+"\x00")
		}
		c.imageRobust.RobustImageAccess = vk.Bool32(vk.True)
		link(vk.StructureTypePhysicalDeviceImageRobustnessFeatures, &c.imageRobust.SType, &c.imageRobust.PNext, unsafe.Pointer(&c.imageRobust))
	}
	if s.robustness2 != nil && (s.robustBufferAccess2() || s.robustness2.RobustImageAccess2 != 0) {
		extensions = append(extensions, extRobustness2+"\x00")
		c.robustness2.RobustBufferAccess2 = boolToVk(s.robustBufferAccess2())
		c.robustness2.RobustImageAccess2 = s.robustness2.RobustImageAccess2
		link(vk.StructureTypePhysicalDeviceRobustness2FeaturesKhr, &c.robustness2.SType, &c.robustness2.PNext, unsafe.Pointer(&c.robustness2))
	}
	// Workgroup zeroing by the driver, behind naga's Native mode
	// (spirvOptions). Rust never enables the extension: it relies on the
	// Vulkan 1.3 core feature. Our instance is Vulkan 1.2, so the device
	// enables the extension, without which the feature struct is invalid.
	if s.zeroInitializeWorkgroupMemory() {
		if s.apiVersion < vkMakeVersion(1, 3, 0) {
			extensions = append(extensions, extZeroInitWorkgroupMemory+"\x00")
		}
		c.zeroInitWG.ShaderZeroInitializeWorkgroupMemory = vk.Bool32(vk.True)
		link(vk.StructureTypePhysicalDeviceZeroInitializeWorkgroupMemoryFeatures, &c.zeroInitWG.SType, &c.zeroInitWG.PNext, unsafe.Pointer(&c.zeroInitWG))
	}
	// Subgroup size control, for the ALLOW_VARYING_SUBGROUP_SIZE stages of a
	// device with SUBGROUP (allowVaryingSubgroupSize). Rust enables the
	// extension below Vulkan 1.3 when SUBGROUP is requested
	// (get_required_extensions) and turns the feature on wherever the
	// extension or 1.3 is enabled (from_extensions_and_requested_features);
	// nothing else here uses size control, so the feature follows the
	// SUBGROUP request on 1.3 too. The subgroup operations themselves are
	// Vulkan 1.1 core and need no device feature.
	if s.allowVaryingSubgroupSize(requested) {
		if s.apiVersion < vkMakeVersion(1, 3, 0) {
			extensions = append(extensions, extSubgroupSizeControl+"\x00")
		}
		c.sizeControl.SubgroupSizeControl = vk.Bool32(vk.True)
		link(vk.StructureTypePhysicalDeviceSubgroupSizeControlFeatures, &c.sizeControl.SType, &c.sizeControl.PNext, unsafe.Pointer(&c.sizeControl))
	}
	return pNext, extensions
}
