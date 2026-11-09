//go:build !(js && wasm)

// Package software provides a CPU-based software rendering backend.
//
// The package is not built for js/wasm: in a browser, wgpu uses the browser's
// WebGPU implementation instead.
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
//   - Textures with every mip level, array layer, cube face and 3D slice;
//     copies, WriteTexture, render attachments and storage textures address
//     the subresource they name
//   - Color attachments written in their view's format (sRGB encode, blend
//     in linear, write masks); depth-only passes; a stencil plane in the
//     depth-stencil formats
//   - Texture sampling (nearest, bilinear, 3 wrap modes) through the view's
//     format, with sRGB decoded before filtering, and mip selection
//     from the sampler's mipmap filter, LOD clamps and bias; array, cube and
//     3D views sample their layer, face or slice
//   - GLSL.std.450 math intrinsics (30+ functions)
//   - Control flow (loops, phi, function calls, switch)
//   - Atomics, and workgroup memory shared by a workgroup's invocations, with
//     workgroup barriers that suspend each invocation until all reach them
//   - Indirect draws and dispatches
//   - Shader debugger (DebugContext, breakpoints, JSON trace, zero overhead when disabled)
//   - Buffer/texture copy operations
//   - Clear operations
//   - Windowed presentation (Windows GDI, Linux X11, macOS CG+Metal)
//   - Thread-safe resource access
//
// Limitations:
//   - Much slower than GPU backends (CPU-bound, interpreter, not JIT)
//   - No hardware acceleration
//   - Implicit-LOD textureSample uses the view's base level: fragments are
//     shaded one at a time, so there are no derivatives
//   - Cube sampling is not seamless across faces
//   - No multisampling (CreateTexture rejects SampleCount > 1)
//
// Always compiled (no build tags required).
//
// Example:
//
//	import _ "github.com/gogpu/wgpu/hal/software"
//
//	// Software backend is registered automatically
//	// Adapter name: "Software Renderer"
//	// Device type: gputypes.DeviceTypeCPU
//
// Backend identifier: gputypes.BackendEmpty
package software
