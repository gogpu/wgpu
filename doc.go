// Package wgpu provides a safe, ergonomic WebGPU API for Go applications.
//
// This package wraps the lower-level hal/ and core/ packages into a user-friendly
// API aligned with the W3C WebGPU specification.
//
// # Quick Start
//
// Import this package and a backend registration package:
//
//	import (
//	    "github.com/gogpu/wgpu"
//	    _ "github.com/gogpu/wgpu/hal/allbackends"
//	)
//
//	instance, err := wgpu.CreateInstance(nil)
//	// ...
//
// # Resource Lifecycle
//
// All GPU resources must be explicitly released with Release().
// Resources are reference-counted internally. Using a released resource panics.
//
// # Backend Registration
//
// Backends are registered via blank imports:
//
//	_ "github.com/gogpu/wgpu/hal/allbackends"  // all available backends
//	_ "github.com/gogpu/wgpu/hal/vulkan"        // Vulkan only
//	_ "github.com/gogpu/wgpu/hal/noop"           // testing
//
// # Thread Safety
//
// Instance, Adapter, and Device are safe for concurrent use.
// Encoders (CommandEncoder, RenderPassEncoder, ComputePassEncoder) are NOT thread-safe.
package wgpu
