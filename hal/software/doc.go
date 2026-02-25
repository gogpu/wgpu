// Package software provides a CPU-based software rendering backend.
//
// Status: IMPLEMENTED (Phase 1 - Headless Rendering)
//
// The software backend implements all HAL interfaces using pure Go CPU rendering.
// Unlike the noop backend, it actually performs rendering operations in memory.
//
// Use cases:
//   - Headless rendering (servers, CI/CD)
//   - Screenshot/image generation without GPU
//   - Testing rendering logic without GPU hardware
//   - Embedded systems without GPU
//   - Fallback when no GPU backend is available
//
// Implemented features (Phase 1):
//   - Real data storage for buffers and textures
//   - Clear operations (fill framebuffer/texture with color)
//   - Buffer/texture copy operations
//   - Framebuffer readback via Surface.GetFramebuffer()
//   - Thread-safe resource access
//
// Limitations:
//   - Much slower than GPU backends (CPU-bound)
//   - No hardware acceleration
//   - No compute shaders (returns error)
//   - No rasterization yet (draw calls are no-op - Phase 2)
//   - No shader execution (basic resources only)
//
// Always compiled (no build tags required).
//
// Example:
//
//	import _ "github.com/gogpu/wgpu/hal/software"
//
//	// Software backend is registered automatically
//	// Adapter name: "Software Renderer"
//	// Device type: types.DeviceTypeCPU
//
// Backend identifier: types.BackendEmpty
package software
