//go:build !(js && wasm)

// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

// Package allbackends registers every HAL backend implemented in this module.
//
// It is this repository's own registration convenience, for its examples and
// tests. Since ADR-083 the platform backends and the software backend are
// registered by default, so a user of the public API needs no blank import.
//
// The package lives at github.com/gogpu/wgpu/internal/hal/allbackends, so it is
// importable only inside this module. Importing it for side effects registers:
//   - Vulkan backend (Windows, Linux, macOS; Android/arm64 preview)
//   - Metal backend (macOS; iOS untested)
//   - DX12 backend (Windows)
//   - OpenGL ES backend (Windows, Linux)
//   - Software backend (all supported native platforms)
//
// After importing, reach the backends through internal/hal (GetBackend,
// SelectBestBackend). The no-op provider is not registered here; import
// github.com/gogpu/wgpu/internal/hal/noop explicitly when a test needs it.
//
// Build tags control which backends are available:
//   - Desktop: platform GPU backends plus software fallback
//   - Android/arm64: Vulkan only
//
// Example usage:
//
//	import (
//		_ "github.com/gogpu/wgpu/internal/hal/allbackends"
//		"github.com/gogpu/wgpu"
//	)
//
//	func main() {
//		instance, err := wgpu.CreateInstance(nil)
//		if err != nil {
//			panic(err)
//		}
//		defer instance.Release()
//
//		adapter, err := instance.RequestAdapter(nil)
//		if err != nil {
//			panic(err)
//		}
//		_ = adapter // a real GPU adapter
//	}
package allbackends
