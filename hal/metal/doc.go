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
//   - device.go: MTLDevice wrapper
//   - queue.go: MTLCommandQueue wrapper
//   - buffer.go: MTLBuffer management with storage modes
//   - texture.go: MTLTexture management
//   - sampler.go: MTLSamplerState
//   - surface.go: CAMetalLayer integration for windowed rendering
//   - encoder.go: MTLCommandBuffer and MTLRenderCommandEncoder
//   - pipeline.go: MTLRenderPipelineState and MTLComputePipelineState
//   - bindgroup.go: Resource binding via argument buffers
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
