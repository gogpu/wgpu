//go:build !(js && wasm)

// Package shader provides shader execution for the software backend.
//
// Two execution paths are supported:
//
//   - SPIR-V interpreter: parses and executes SPIR-V bytecode (compiled from WGSL
//     via naga). This enables rendering shaders that use @builtin(vertex_index) with
//     no vertex buffers (e.g. the gogpu triangle example). See Module, ParseModule,
//     and Module.Execute.
//
//   - Callback shaders: Go functions implementing VertexShaderFunc / FragmentShaderFunc
//     for programmatic shader definition without SPIR-V. Useful for testing and
//     built-in effects (solid color, vertex color, textured).
//
// # Shader Types
//
// There are two main shader stages:
//
//   - Vertex Shader: Transforms vertices from object space to clip space.
//     Receives vertex position and attributes, outputs clip-space position and
//     interpolated attributes.
//
//   - Fragment Shader: Computes the final color for each fragment (pixel candidate).
//     Receives interpolated fragment data and outputs RGBA color.
//
// # Built-in Shaders
//
// The package provides several built-in shaders for common use cases:
//
//   - SolidColor: Renders geometry with a uniform color.
//   - VertexColor: Interpolates per-vertex colors across the triangle.
//
// # Usage
//
//	// Create a shader program
//	program := shader.ShaderProgram{
//	    Vertex:   shader.SolidColorVertexShader,
//	    Fragment: shader.SolidColorFragmentShader,
//	}
//
//	// Prepare uniforms
//	uniforms := &shader.SolidColorUniforms{
//	    MVP:   myMVPMatrix,
//	    Color: [4]float32{1, 0, 0, 1}, // Red
//	}
//
//	// Use with the rasterization pipeline
//	// (integration code varies based on pipeline implementation)
//
// # Custom Shaders
//
// To create custom shaders, implement the VertexShaderFunc and FragmentShaderFunc
// signatures:
//
//	func MyVertexShader(
//	    vertexIndex int,
//	    position [3]float32,
//	    attributes []float32,
//	    uniforms any,
//	) raster.ClipSpaceVertex {
//	    // Transform position and prepare attributes
//	}
//
//	func MyFragmentShader(
//	    fragment raster.Fragment,
//	    uniforms any,
//	) [4]float32 {
//	    // Compute and return RGBA color
//	}
package shader
