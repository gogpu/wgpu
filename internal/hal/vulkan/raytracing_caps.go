//go:build !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vulkan

import (
	"math"
	"unsafe"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/hal"
	"github.com/gogpu/wgpu/internal/hal/vulkan/vk"
)

// Device extensions required for gputypes.FeatureRayQuery.
//
// VK_KHR_buffer_device_address and VK_EXT_descriptor_indexing are also
// dependencies of VK_KHR_acceleration_structure, but both are core in
// Vulkan 1.2, which ray query support requires here (see rayQueryQueryable).
const (
	extDeferredHostOperations = "VK_KHR_deferred_host_operations"
	extAccelerationStructure  = "VK_KHR_acceleration_structure"
	extRayQuery               = "VK_KHR_ray_query"
)

// rayQueryDeviceExtensions are the extensions Open enables when
// gputypes.FeatureRayQuery is requested.
var rayQueryDeviceExtensions = []string{
	extDeferredHostOperations,
	extAccelerationStructure,
	extRayQuery,
}

// maxShaderStagesPerPipeline mirrors Rust wgpu-hal's MAX_SHADER_STAGES_PER_PIPELINE,
// used to split per-set descriptor limits into per-stage limits.
const maxShaderStagesPerPipeline = 2

// rayQueryQueryable reports whether the device can report the ray query
// feature structs: the extensions are present and the device is used at
// Vulkan 1.2 or later.
//
// Vulkan 1.2 is required so bufferDeviceAddress can be enabled through
// VkPhysicalDeviceVulkan12Features, which Open already chains for timeline
// semaphores (the spec forbids also chaining the standalone BDA struct,
// VUID-VkDeviceCreateInfo-pNext-02830).
func (s *extendedFeatureSupport) rayQueryQueryable() bool {
	if s.apiVersion < vkMakeVersion(1, 2, 0) {
		return false
	}
	for _, name := range rayQueryDeviceExtensions {
		if !s.extensions[name] {
			return false
		}
	}
	return true
}

// chainRayQueryFeatures appends the feature structs ray query support is
// decided by to a vkGetPhysicalDeviceFeatures2 chain, when the device can
// report them, and returns the new tail.
// Reference: Rust wgpu-hal PhysicalDeviceFeatures::{acceleration_structure,
// buffer_device_address, ray_query}.
func (s *extendedFeatureSupport) chainRayQueryFeatures(next **uintptr) **uintptr {
	if !s.rayQueryQueryable() {
		return next
	}
	s.bufferDeviceAddress = &vk.PhysicalDeviceBufferDeviceAddressFeatures{
		SType: vk.StructureTypePhysicalDeviceBufferDeviceAddressFeatures,
	}
	s.accelerationStructure = &vk.PhysicalDeviceAccelerationStructureFeaturesKHR{
		SType: vk.StructureTypePhysicalDeviceAccelerationStructureFeaturesKhr,
	}
	s.rayQueryFeatures = &vk.PhysicalDeviceRayQueryFeaturesKHR{
		SType: vk.StructureTypePhysicalDeviceRayQueryFeaturesKhr,
	}
	*next = (*uintptr)(unsafe.Pointer(s.bufferDeviceAddress))
	next = &s.bufferDeviceAddress.PNext
	*next = (*uintptr)(unsafe.Pointer(s.accelerationStructure))
	next = &s.accelerationStructure.PNext
	*next = (*uintptr)(unsafe.Pointer(s.rayQueryFeatures))
	return &s.rayQueryFeatures.PNext
}

// rayQuery reports whether the device can expose gputypes.FeatureRayQuery:
// the extensions are present and accelerationStructure, rayQuery and
// bufferDeviceAddress are all supported.
//
// Rust wgpu-hal reports EXPERIMENTAL_RAY_QUERY from extension presence alone.
// This also checks the feature bits that Open will enable, so a device that
// lists the extensions but can't enable them isn't advertised.
func (s *extendedFeatureSupport) rayQuery() bool {
	return s.bufferDeviceAddress != nil && s.bufferDeviceAddress.BufferDeviceAddress != 0 &&
		s.accelerationStructure != nil && s.accelerationStructure.AccelerationStructure != 0 &&
		s.rayQueryFeatures != nil && s.rayQueryFeatures.RayQuery != 0
}

// accelerationStructureProperties returns
// VkPhysicalDeviceAccelerationStructurePropertiesKHR, or nil when the device
// does not support ray query.
func (i *Instance) accelerationStructureProperties(device vk.PhysicalDevice, s *extendedFeatureSupport) *vk.PhysicalDeviceAccelerationStructurePropertiesKHR {
	if !s.rayQuery() {
		return nil
	}
	asProps := &vk.PhysicalDeviceAccelerationStructurePropertiesKHR{
		SType: vk.StructureTypePhysicalDeviceAccelerationStructurePropertiesKhr,
	}
	props2 := vk.PhysicalDeviceProperties2{
		SType: vk.StructureTypePhysicalDeviceProperties2,
		PNext: (*uintptr)(unsafe.Pointer(asProps)),
	}
	i.cmds.GetPhysicalDeviceProperties2(device, &props2)
	asProps.PNext = nil
	return asProps
}

// enableRayQuery fills the ray query part of a device feature chain and
// returns the new tail and the extensions it needs (NUL-terminated). The
// caller enables bufferDeviceAddress through the Vulkan 1.2 struct.
// Reference: Rust adapter.rs PhysicalDeviceFeatures::from_extensions_and_requested_features
// and PhysicalDeviceCapabilities::get_required_extensions.
func (c *deviceFeatureChain) enableRayQuery(link func(vk.StructureType, *vk.StructureType, **uintptr, unsafe.Pointer)) []string {
	c.accelerationStructure.AccelerationStructure = vk.Bool32(vk.True)
	link(vk.StructureTypePhysicalDeviceAccelerationStructureFeaturesKhr, &c.accelerationStructure.SType, &c.accelerationStructure.PNext, unsafe.Pointer(&c.accelerationStructure))
	c.rayQuery.RayQuery = vk.Bool32(vk.True)
	link(vk.StructureTypePhysicalDeviceRayQueryFeaturesKhr, &c.rayQuery.SType, &c.rayQuery.PNext, unsafe.Pointer(&c.rayQuery))
	extensions := make([]string, 0, len(rayQueryDeviceExtensions))
	for _, name := range rayQueryDeviceExtensions {
		extensions = append(extensions, name+"\x00")
	}
	return extensions
}

// applyRayQueryLimits fills the acceleration structure limits.
// Reference: Rust adapter.rs PhysicalDeviceProperties::to_wgpu_limits.
func (s *extendedFeatureSupport) applyRayQueryLimits(limits *gputypes.Limits) {
	if s.asProperties == nil {
		return
	}
	p := s.asProperties
	limits.MaxBlasGeometryCount = clampU64ToU32(p.MaxGeometryCount)
	limits.MaxBlasPrimitiveCount = clampU64ToU32(p.MaxPrimitiveCount)
	limits.MaxTlasInstanceCount = clampU64ToU32(p.MaxInstanceCount)
	limits.MaxAccelerationStructuresPerShaderStage = min(
		p.MaxPerStageDescriptorAccelerationStructures,
		p.MaxDescriptorSetAccelerationStructures/maxShaderStagesPerPipeline,
	)
	limits.MaxBuffersAndAccelerationStructuresPerShaderStage = math.MaxUint32
}

// applyRayQueryAlignments fills the ray tracing alignments.
// Reference: Rust adapter.rs PhysicalDeviceProperties::to_hal_alignments.
func (s *extendedFeatureSupport) applyRayQueryAlignments(alignments *hal.Alignments) {
	alignments.RawTlasInstanceSize = 64 // sizeof(VkAccelerationStructureInstanceKHR)
	if s.asProperties != nil {
		alignments.RayTracingScratchBufferAlignment = s.asProperties.MinAccelerationStructureScratchOffsetAlignment
	}
}

// scratchBufferAlignment returns minAccelerationStructureScratchOffsetAlignment,
// or 0 when the device does not support ray query.
func (s *extendedFeatureSupport) scratchBufferAlignment() uint32 {
	if s.asProperties == nil {
		return 0
	}
	return s.asProperties.MinAccelerationStructureScratchOffsetAlignment
}

func clampU64ToU32(v uint64) uint32 {
	if v > math.MaxUint32 {
		return math.MaxUint32
	}
	return uint32(v)
}
