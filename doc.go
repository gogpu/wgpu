// Package wgpu provides a safe, ergonomic WebGPU API for Go applications.
//
// This package wraps the lower-level hal/ and core/ packages into a user-friendly
// API aligned with the W3C WebGPU specification.
//
// # Quick Start
//
// Import this package; the platform backends are registered automatically:
//
//	import "github.com/gogpu/wgpu"
//
//	instance, err := wgpu.CreateInstance(nil)
//	// ...
//
// # Resource Lifecycle
//
// All GPU resources must be explicitly released with Release().
// Resources are reference-counted internally. Release is idempotent; methods
// on a released resource that return an error report ErrReleased.
//
// # Backend Registration
//
// The platform backends register by default (ADR-083): Vulkan, DX12 and
// OpenGL ES on Windows (386, amd64, arm64), Vulkan and OpenGL ES on Linux
// (amd64, arm64), Metal on macOS (amd64, arm64; MoltenVK only with
// -tags wgpu_moltenvk), Vulkan and OpenGL ES on Android (arm64), the browser's
// WebGPU on js/wasm, and the Pure Go software backend on those platforms.
// Other GOOS/GOARCH combinations are outside the default registration set;
// some cannot build the Pure Go backends at all (goffi has no syscall files
// for them). Such a build registers no backend unless one is imported.
//
// Build tags remove a backend for a smaller binary: wgpu_novulkan,
// wgpu_nodx12, wgpu_nometal, wgpu_nogles, wgpu_nosoftware. The software
// backend registers but is not part of the default adapter mask; request it
// with RequestAdapterOptions.ForceFallbackAdapter.
//
// # Thread Safety
//
// Instance, Adapter, and Device are safe for concurrent use.
// Encoders (CommandEncoder, RenderPassEncoder, ComputePassEncoder) are NOT thread-safe.
package wgpu
