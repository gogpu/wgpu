// Package types defines WebGPU types that are backend-agnostic.
//
// This package is a pure Go port of wgpu-types from the Rust wgpu project.
// It provides the fundamental types used by WebGPU:
//
//   - Backend and adapter types (BackendType, AdapterInfo)
//   - Resource types (Buffer, Texture, Sampler)
//   - Pipeline types (BindGroup, Pipeline, RenderPass)
//   - Descriptor types (BufferDescriptor, TextureDescriptor, etc.)
//   - Enums and constants (TextureFormat, CompareFunction, etc.)
//
// These types are designed to be serializable and can be used for:
//
//   - Shader reflection
//   - Resource management
//   - State tracking
//   - Configuration
//
// Reference: https://github.com/gfx-rs/wgpu/tree/trunk/wgpu-types
package types
