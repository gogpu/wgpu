//go:build android && arm64

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package allbackends

import (
	// Android intentionally registers Vulkan only. GLES and software fallback
	// are outside the Android/arm64 preview contract.
	_ "github.com/gogpu/wgpu/hal/vulkan"
)
