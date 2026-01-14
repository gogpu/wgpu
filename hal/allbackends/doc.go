// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

// Package allbackends imports all HAL backend implementations.
//
// Import this package for side effects to register all available backends:
//
//	import (
//		_ "github.com/gogpu/wgpu/hal/allbackends"
//	)
//
// This will register:
//   - Vulkan backend (Windows, Linux, macOS)
//   - Metal backend (macOS, iOS)
//   - DX12 backend (Windows)
//   - OpenGL ES backend (Windows, Linux)
//   - No-op backend (all platforms, for testing)
//
// After importing, use hal.GetBackend or hal.SelectBestBackend to access backends.
//
// Build tags control which backends are available:
//   - Default: All backends for the current platform
//   - "!android": Excludes Android-specific Vulkan loader
//   - "software": Includes software rasterizer backend
//
// Example usage:
//
//	import (
//		_ "github.com/gogpu/wgpu/hal/allbackends"
//		"github.com/gogpu/wgpu/core"
//	)
//
//	func main() {
//		// Instance will now enumerate real GPUs
//		instance := core.NewInstance(nil)
//		adapters := instance.EnumerateAdapters()
//		for _, a := range adapters {
//			fmt.Println(a) // Real GPU adapters
//		}
//	}
package allbackends
