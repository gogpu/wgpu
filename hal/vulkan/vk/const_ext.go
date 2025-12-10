// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vk

import (
	"math"
	"unsafe"
)

// Additional constants not generated from vk.xml but needed for Vulkan 1.3 dynamic rendering.
// Values from VK_KHR_dynamic_rendering extension (promoted to core in Vulkan 1.3).

const (
	// StructureTypeRenderingInfo = VK_STRUCTURE_TYPE_RENDERING_INFO
	StructureTypeRenderingInfo StructureType = 1000044000

	// StructureTypeRenderingAttachmentInfo = VK_STRUCTURE_TYPE_RENDERING_ATTACHMENT_INFO
	StructureTypeRenderingAttachmentInfo StructureType = 1000044001

	// StructureTypePipelineRenderingCreateInfo = VK_STRUCTURE_TYPE_PIPELINE_RENDERING_CREATE_INFO
	StructureTypePipelineRenderingCreateInfo StructureType = 1000044002

	// StructureTypePhysicalDeviceDynamicRenderingFeatures = VK_STRUCTURE_TYPE_PHYSICAL_DEVICE_DYNAMIC_RENDERING_FEATURES
	StructureTypePhysicalDeviceDynamicRenderingFeatures StructureType = 1000044003

	// StructureTypeCommandBufferInheritanceRenderingInfo = VK_STRUCTURE_TYPE_COMMAND_BUFFER_INHERITANCE_RENDERING_INFO
	StructureTypeCommandBufferInheritanceRenderingInfo StructureType = 1000044004
)

// ClearValueColor creates a ClearValue from RGBA float values.
func ClearValueColor(r, g, b, a float32) ClearValue {
	var cv ClearValue
	*(*[4]float32)(unsafe.Pointer(&cv)) = [4]float32{r, g, b, a}
	return cv
}

// ClearValueDepthStencil creates a ClearValue from depth and stencil values.
func ClearValueDepthStencil(depth float32, stencil uint32) ClearValue {
	var cv ClearValue
	*(*float32)(unsafe.Pointer(&cv)) = depth
	*(*uint32)(unsafe.Pointer(uintptr(unsafe.Pointer(&cv)) + 4)) = stencil
	return cv
}

// GetColorFloat32 extracts float32[4] color values from a ClearValue.
func (cv *ClearValue) GetColorFloat32() [4]float32 {
	return *(*[4]float32)(unsafe.Pointer(cv))
}

// GetDepthStencil extracts depth and stencil values from a ClearValue.
func (cv *ClearValue) GetDepthStencil() (depth float32, stencil uint32) {
	depth = *(*float32)(unsafe.Pointer(cv))
	stencil = *(*uint32)(unsafe.Pointer(uintptr(unsafe.Pointer(cv)) + 4))
	return
}

// Ensure math is used (for potential future use).
var _ = math.Float32bits
