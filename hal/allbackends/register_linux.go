// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build linux && !android

package allbackends

import (
	// Linux-specific HAL backend imports.

	// Vulkan backend - primary backend on Linux.
	_ "github.com/gogpu/wgpu/hal/vulkan"

	// OpenGL ES backend - fallback for systems without Vulkan.
	_ "github.com/gogpu/wgpu/hal/gles"
)
