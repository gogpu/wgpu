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
// The platform hardware backends register by default (ADR-083): Vulkan, DX12
// and OpenGL ES on Windows (386, amd64, arm64), Vulkan and OpenGL ES on Linux
// (amd64, arm64), Metal on macOS (amd64, arm64; MoltenVK only with
// -tags wgpu_moltenvk), Vulkan and OpenGL ES on Android (arm64), and the
// browser's WebGPU on js/wasm. The Pure Go software backend is not registered
// by default in this release: import hal/software (or hal/allbackends) until
// v0.35.0, where it becomes default-registered outside the default mask.
// Other GOOS/GOARCH combinations are outside the default registration set;
// some cannot build the Pure Go backends at all (goffi has no syscall files
// for them). Such a build registers no backend unless one is imported.
//
// Build tags remove a backend for a smaller binary: wgpu_novulkan,
// wgpu_nodx12, wgpu_nometal, wgpu_nogles. wgpu_nosoftware removes the Pure Go
// software backend from wgputest today and from the default registration in
// v0.35.0. Until then the software backend registers only when the program
// imports it; select it with RequestAdapterOptions.ForceFallbackAdapter.
//
// GOOS=ios satisfies the darwin build tag, so iOS builds in the default set
// register Metal.
//
// # Thread Safety
//
// Instance, Adapter, and Device are safe for concurrent use.
// Encoders (CommandEncoder, RenderPassEncoder, ComputePassEncoder) are NOT thread-safe.
package wgpu
