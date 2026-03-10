// Package hal provides the Hardware Abstraction Layer for WebGPU implementations.
//
// The HAL defines backend-agnostic interfaces for GPU operations, allowing
// different graphics backends (Vulkan, Metal, DX12, GL) to be used interchangeably.
//
// # Architecture
//
// The HAL is organized into several layers:
//
//  1. Backend - Factory for creating instances (entry point)
//  2. Instance - Entry point for adapter enumeration and surface creation
//  3. Adapter - Physical GPU representation with capability queries
//  4. Device - Logical device for resource creation and command submission
//  5. Queue - Command buffer submission and presentation
//  6. CommandEncoder - Command recording
//
// # Validation Contract
//
// The HAL follows a defense-in-depth validation pattern:
//
//   - The core layer (wgpu/core) performs exhaustive spec-level validation BEFORE
//     calling HAL methods. This includes dimension checks, format validation, usage
//     flags, mip levels, sample counts, and all WebGPU spec rules.
//
//   - HAL methods assume their input has been validated by core. They do NOT
//     re-validate spec rules (size > 0, valid format, usage checks, etc.).
//
//   - HAL methods DO retain nil/null pointer checks as defense-in-depth guards.
//     These checks use a "BUG:" prefix in their error messages to indicate that
//     they should never fire in normal operation. If a "BUG:" error is returned,
//     it means the core validation layer has a gap that must be fixed.
//
//   - Example BUG error: "BUG: buffer descriptor is nil in Vulkan.CreateBuffer — core validation gap"
//
// This contract ensures:
//   - No redundant validation between core and HAL (single source of truth)
//   - No panics from nil pointer dereferences (safety-critical)
//   - Clear diagnostics when core validation is incomplete
//
// # Resource Types
//
// All GPU resources (buffers, textures, pipelines, etc.) implement the Resource
// interface which provides a Destroy method. Resources must be explicitly destroyed
// to free GPU memory.
//
// # Backend Registration
//
// Backends register themselves using RegisterBackend. The core layer can then
// query available backends and create instances dynamically:
//
//	backend, ok := hal.GetBackend(types.BackendVulkan)
//	if !ok {
//		return fmt.Errorf("vulkan backend not available")
//	}
//	instance, err := backend.CreateInstance(desc)
//
// # Thread Safety
//
// Unless explicitly stated, HAL interfaces are not thread-safe. Synchronization
// is the caller's responsibility. Notable exceptions:
//
//   - Backend registration (RegisterBackend, GetBackend) is thread-safe
//   - Queue.Submit is typically thread-safe (backend-specific)
//
// # Error Handling
//
// The HAL uses error values for unrecoverable errors:
//
//   - ErrDeviceOutOfMemory - GPU memory exhausted
//   - ErrDeviceLost - GPU disconnected or driver reset
//   - ErrSurfaceLost - Window destroyed or surface invalidated
//   - ErrSurfaceOutdated - Window resized, need reconfiguration
//
// Validation errors (invalid descriptors, incorrect usage) are the core layer's
// responsibility and are not checked by the HAL. HAL nil checks are defense-in-depth
// only, prefixed with "BUG:" to signal a core validation gap.
//
// # Reference
//
// This design is based on wgpu-hal from the Rust WebGPU implementation.
// See: https://github.com/gfx-rs/wgpu/tree/trunk/wgpu-hal
package hal
