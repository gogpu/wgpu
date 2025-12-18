//go:build software

// Package shader provides callback-based shader execution for the software backend.
//
// Since there is no SPIR-V interpreter in Go, we use callback functions to define
// vertex and fragment shaders. This allows testing the rendering pipeline without
// full shader compilation.
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
