// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build darwin

package allbackends

import (
	// macOS/iOS-specific HAL backend imports.

	// Metal backend - primary backend on Apple platforms.
	_ "github.com/gogpu/wgpu/hal/metal"

	// Vulkan backend - available via MoltenVK on macOS.
	_ "github.com/gogpu/wgpu/hal/vulkan"
)
