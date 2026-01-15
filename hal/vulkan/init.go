// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !android && !js

package vulkan

import "github.com/gogpu/wgpu/hal"

// init registers the Vulkan backend with the HAL registry.
// This is called automatically on package import.
//
// The Vulkan backend is available on:
//   - Windows (x64)
//   - Linux (x64, ARM64)
//   - macOS (via MoltenVK)
//   - Android (excluded via build tag - uses different loader)
//
// Note: Android requires platform-specific Vulkan loader initialization.
func init() {
	hal.RegisterBackend(Backend{})
}
