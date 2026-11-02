//go:build !(js && wasm)

// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

// Package metal provides a Metal backend for the HAL.
//
// Metal is Apple's low-overhead, low-level graphics and compute API
// for macOS, iOS, iPadOS, and tvOS platforms.
//
// # Architecture
//
// The Metal backend uses Pure Go FFI via github.com/go-webgpu/goffi
// to call into Apple's Metal.framework and Objective-C runtime without
// requiring CGO. This enables zero-CGO cross-compilation for macOS/iOS.
//
// # Key Components
//
//   - objc.go: Objective-C runtime bindings for message passing
//   - metal.go: Metal framework loader and initialization
//   - types.go: Metal types, enums, and constants
//   - adapter.go, device.go: MTLDevice wrapper, resource and pipeline creation
//   - queue.go: MTLCommandQueue wrapper
//   - resource.go: buffers, textures, samplers, bind groups and pipeline state objects
//   - texture_copy.go: texture copy planning (array layers, 3D slices)
//   - surface.go, frame_pool.go: CAMetalLayer integration for windowed rendering
//   - encoder.go: MTLCommandBuffer, render, compute and blit encoders
//   - icb_indexed.go, icb_pipeline.go: indirect command buffers for large multi-draws
//   - raytracing.go: MTLAccelerationStructure (macOS 15+)
//   - conv.go: WebGPU to Metal type conversions
//   - api.go: HAL Backend interface implementation
//
// # Storage Modes
//
// Metal uses storage modes for memory management:
//   - Shared: CPU and GPU can both access (for mappable buffers)
//   - Private: GPU-only access (fastest for GPU operations)
//   - Memoryless: Tile-based deferred rendering (iOS optimization)
//
// # Autorelease Pools
//
// Metal objects returned from APIs like nextDrawable are autoreleased.
// This backend uses autorelease pools to manage object lifetimes correctly.
//
// # References
//
//   - Apple Metal Documentation: https://developer.apple.com/metal/
//   - Metal Feature Set Tables: https://developer.apple.com/metal/Metal-Feature-Set-Tables.pdf
//   - Ebitengine Metal backend: Uses purego patterns (excellent reference)
//   - metal-rs: Rust Metal bindings (architecture reference)
//   - wgpu-hal Metal: Comprehensive implementation reference
package metal
