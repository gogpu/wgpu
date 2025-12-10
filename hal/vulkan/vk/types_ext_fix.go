// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vk

// Type aliases for extension types that reference core types
// These are typically promoted extensions where KHR/EXT became core

type (
	// MemoryRequirements2KHR is an alias for MemoryRequirements2 (promoted in Vulkan 1.1)
	MemoryRequirements2KHR = MemoryRequirements2

	// PipelineInfoEXT is an alias for PipelineInfoKHR
	PipelineInfoEXT = PipelineInfoKHR

	// RemoteAddressNV is a device address type for RDMA operations
	RemoteAddressNV = DeviceAddress

	// LineRasterizationModeEXT is an alias for LineRasterizationMode (promoted in Vulkan 1.3)
	LineRasterizationModeEXT = LineRasterizationMode
)
