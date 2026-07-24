//go:build !(js && wasm)

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
//   - Vulkan backend (Windows, Linux, macOS; Android/arm64 preview)
//   - Metal backend (macOS, iOS)
//   - DX12 backend (Windows)
//   - OpenGL ES backend (Windows, Linux)
//   - Software backend (all supported native platforms)
//
// After importing, use hal.GetBackend or hal.SelectBestBackend to access backends.
//
// Build tags control which backends are available:
//   - Desktop: platform GPU backends plus software fallback
//   - Android/arm64: Vulkan only
//
// The no-op provider is not registered by this package. Import
// github.com/gogpu/wgpu/hal/noop explicitly when it is required for tests.
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
