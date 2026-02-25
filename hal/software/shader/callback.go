package shader

import "github.com/gogpu/wgpu/hal/software/raster"

// VertexShaderFunc transforms a vertex from object space to clip space.
// It receives vertex data and returns a clip-space vertex with position and attributes.
//
// Parameters:
//   - vertexIndex: The index of the vertex being processed.
//   - position: The 3D position of the vertex in object/world space.
//   - attributes: Additional vertex attributes (colors, UVs, normals, etc.).
//   - uniforms: User-defined uniform data (matrices, colors, etc.).
//
// Returns:
//   - ClipSpaceVertex: The transformed vertex with clip-space position and attributes
//     to be interpolated across the triangle.
type VertexShaderFunc func(
	vertexIndex int,
	position [3]float32,
	attributes []float32,
	uniforms any,
) raster.ClipSpaceVertex

// FragmentShaderFunc computes the final color for a fragment.
// It receives interpolated fragment data and returns an RGBA color.
//
// Parameters:
//   - fragment: The fragment with interpolated position, depth, and attributes.
//   - uniforms: User-defined uniform data (textures, colors, etc.).
//
// Returns:
//   - [4]float32: RGBA color values in the range [0, 1].
type FragmentShaderFunc func(
	fragment raster.Fragment,
	uniforms any,
) [4]float32

// ShaderProgram combines vertex and fragment shaders into a complete program.
type ShaderProgram struct {
	// Vertex is the vertex shader function.
	Vertex VertexShaderFunc

	// Fragment is the fragment shader function.
	Fragment FragmentShaderFunc
}

// IsValid returns true if the shader program has both vertex and fragment shaders.
func (p ShaderProgram) IsValid() bool {
	return p.Vertex != nil && p.Fragment != nil
}

// PassthroughVertexShader is a simple vertex shader that passes position through
// without transformation. Useful for screen-space rendering.
func PassthroughVertexShader(
	vertexIndex int,
	position [3]float32,
	attributes []float32,
	_ any,
) raster.ClipSpaceVertex {
	return raster.ClipSpaceVertex{
		Position:   [4]float32{position[0], position[1], position[2], 1.0},
		Attributes: attributes,
	}
}

// WhiteFragmentShader returns white for all fragments.
// Useful for testing and debugging.
func WhiteFragmentShader(_ raster.Fragment, _ any) [4]float32 {
	return [4]float32{1, 1, 1, 1}
}

// DepthFragmentShader returns a grayscale color based on fragment depth.
// Useful for visualizing the depth buffer.
func DepthFragmentShader(fragment raster.Fragment, _ any) [4]float32 {
	d := fragment.Depth
	return [4]float32{d, d, d, 1}
}

// BarycentricFragmentShader returns a color based on barycentric coordinates.
// Useful for debugging triangle rasterization.
func BarycentricFragmentShader(fragment raster.Fragment, _ any) [4]float32 {
	return [4]float32{
		fragment.Bary[0],
		fragment.Bary[1],
		fragment.Bary[2],
		1,
	}
}

// Vertex represents input vertex data for processing.
type Vertex struct {
	// Position in object/model space.
	Position [3]float32

	// Attributes are additional per-vertex data (colors, UVs, normals, etc.).
	Attributes []float32
}

// NewVertex creates a vertex with position only.
func NewVertex(x, y, z float32) Vertex {
	return Vertex{
		Position: [3]float32{x, y, z},
	}
}

// NewVertexWithColor creates a vertex with position and RGBA color.
func NewVertexWithColor(x, y, z, r, g, b, a float32) Vertex {
	return Vertex{
		Position:   [3]float32{x, y, z},
		Attributes: []float32{r, g, b, a},
	}
}

// NewVertexWithUV creates a vertex with position and UV texture coordinates.
func NewVertexWithUV(x, y, z, u, v float32) Vertex {
	return Vertex{
		Position:   [3]float32{x, y, z},
		Attributes: []float32{u, v},
	}
}

// NewVertexWithColorAndUV creates a vertex with position, RGBA color, and UV coordinates.
func NewVertexWithColorAndUV(x, y, z, r, g, b, a, u, v float32) Vertex {
	return Vertex{
		Position:   [3]float32{x, y, z},
		Attributes: []float32{r, g, b, a, u, v},
	}
}
