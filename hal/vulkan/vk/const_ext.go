//go:build !(js && wasm)

// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vk

import (
	"math"
	"unsafe"
)

// Additional constants not generated from vk.xml.

const (
	// === VK_KHR_acceleration_structure object type ===

	// ObjectTypeAccelerationStructureKHR = VK_OBJECT_TYPE_ACCELERATION_STRUCTURE_KHR (1000150000)
	ObjectTypeAccelerationStructureKHR ObjectType = 1000150000
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
	*(*uint32)(unsafe.Add(unsafe.Pointer(&cv), 4)) = stencil
	return cv
}

// GetColorFloat32 extracts float32[4] color values from a ClearValue.
func (cv *ClearValue) GetColorFloat32() [4]float32 {
	return *(*[4]float32)(unsafe.Pointer(cv))
}

// GetDepthStencil extracts depth and stencil values from a ClearValue.
func (cv *ClearValue) GetDepthStencil() (depth float32, stencil uint32) {
	depth = *(*float32)(unsafe.Pointer(cv))
	stencil = *(*uint32)(unsafe.Add(unsafe.Pointer(cv), 4))
	return
}

// Ensure math is used (for potential future use).
var _ = math.Float32bits
