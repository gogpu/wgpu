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
// # Design Principles
//
// The HAL prioritizes portability over safety, delegating validation to the
// higher core layer. This means:
//
//   - Most methods are unsafe in terms of GPU state validation
//   - Validation is the caller's responsibility
//   - Only unrecoverable errors are returned (out of memory, device lost)
//   - Invalid usage results in undefined behavior at the GPU level
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
// Validation errors (invalid descriptors, incorrect usage) are the caller's
// responsibility and are not checked by the HAL.
//
// # Reference
//
// This design is based on wgpu-hal from the Rust WebGPU implementation.
// See: https://github.com/gfx-rs/wgpu/tree/trunk/wgpu-hal
package hal
