// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows

package vulkan

import "unsafe"

// ptrFromUintptr converts a uintptr (from FFI) to *byte without triggering go vet warning.
// This uses double pointer indirection pattern from ebitengine/purego.
// Reference: https://github.com/golang/go/issues/56487
func ptrFromUintptr(ptr uintptr) *byte {
	return *(**byte)(unsafe.Pointer(&ptr))
}

// copyToMappedMemory copies data to Vulkan mapped memory.
// The ptr must be a valid pointer from vkMapMemory.
// This is safe because:
// 1. The pointer comes from vkMapMemory which returns a valid host-accessible address
// 2. The memory remains mapped for the duration of the copy
// 3. This pattern is explicitly allowed for FFI interop
func copyToMappedMemory(ptr uintptr, offset uint64, data []byte) {
	if len(data) == 0 {
		return
	}
	// Use double pointer indirection to satisfy go vet (pattern from ebitengine/purego)
	base := ptrFromUintptr(ptr + uintptr(offset))
	dst := unsafe.Slice(base, len(data))
	copy(dst, data)
}
