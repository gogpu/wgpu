// Package software provides a CPU-based software rendering backend.
//
// Status: PLANNED - Not yet implemented.
//
// The software backend will implement all HAL interfaces using pure Go CPU rendering.
// Unlike the noop backend, it will actually perform rendering operations in memory.
//
// Planned use cases:
//   - Headless rendering (servers, CI/CD)
//   - Screenshot/image generation without GPU
//   - Testing rendering logic without GPU hardware
//   - Embedded systems without GPU
//   - Fallback when no GPU backend is available
//
// Planned features:
//   - Clear operations (fill framebuffer with color)
//   - Buffer/texture copy operations
//   - Basic triangle rasterization
//   - Texture sampling (nearest/linear filtering)
//   - Framebuffer readback via GetFramebuffer()
//
// Limitations (planned):
//   - Much slower than GPU backends (CPU-bound)
//   - No hardware acceleration
//   - No compute shaders (will return error)
//   - Limited shader support (basic vertex/fragment only)
//
// Build tag: -tags software
//
// Example (when implemented):
//
//	import _ "github.com/gogpu/wgpu/hal/software"
//
//	// Software backend will be registered automatically
//	// Adapter name will contain "Software Renderer"
//
// Implementation tracking: TASK-029 in gogpu/gogpu kanban
package software
