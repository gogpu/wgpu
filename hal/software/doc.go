//go:build !(js && wasm)

// Package software provides a CPU-based software rendering backend.
//
// The software backend implements all HAL interfaces using pure Go CPU rendering.
// Unlike the noop backend, it actually performs rendering operations in memory,
// including full SPIR-V shader execution for vertex, fragment, and compute stages.
//
// Use cases:
//   - Shader debugging (step through every SPIR-V instruction with breakpoints)
//   - CI/CD testing (no GPU required)
//   - Headless rendering (servers, screenshot generation)
//   - GPU-less fallback (embedded systems)
//   - NOT for real-time production rendering — use GPU backends for that
//
// Implemented features:
//   - Real data storage for buffers and textures
//   - SPIR-V interpreter (~10K LOC): vertex, fragment, compute shaders on CPU
//   - Compute shaders: CreateComputePipeline + Dispatch via SPIR-V interpreter
//   - Texture sampling (nearest, bilinear, 3 wrap modes)
//   - GLSL.std.450 math intrinsics (30+ functions)
//   - Control flow (loops, phi, function calls, switch)
//   - Atomics and workgroup shared memory
//   - Shader debugger (DebugContext, breakpoints, JSON trace, zero overhead when disabled)
//   - Buffer/texture copy operations
//   - Clear operations
//   - Windowed presentation (Windows GDI, Linux X11, macOS CG+Metal)
//   - Thread-safe resource access
//
// Limitations:
//   - Much slower than GPU backends (CPU-bound, interpreter, not JIT)
//   - No hardware acceleration
//   - DispatchIndirect not implemented
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
