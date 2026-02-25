package shader

import "github.com/gogpu/wgpu/hal/software/raster"

// SolidColorUniforms contains uniform data for the solid color shader.
type SolidColorUniforms struct {
	// MVP is the Model-View-Projection matrix in column-major order.
	MVP [16]float32

	// Color is the RGBA color to render with values in [0, 1].
	Color [4]float32
}

// SolidColorVertexShader transforms vertices using the MVP matrix.
// The color is passed through as attributes for the fragment shader.
func SolidColorVertexShader(
	_ int,
	position [3]float32,
	_ []float32,
	uniforms any,
) raster.ClipSpaceVertex {
	u := uniforms.(*SolidColorUniforms)

	// Transform position by MVP matrix
	clipPos := Mat4MulVec4(u.MVP, [4]float32{position[0], position[1], position[2], 1.0})

	return raster.ClipSpaceVertex{
		Position:   clipPos,
		Attributes: u.Color[:],
	}
}

// SolidColorFragmentShader returns the uniform color for all fragments.
func SolidColorFragmentShader(frag raster.Fragment, _ any) [4]float32 {
	// Color is stored in interpolated attributes
	if len(frag.Attributes) >= 4 {
		return [4]float32{
			frag.Attributes[0],
			frag.Attributes[1],
			frag.Attributes[2],
			frag.Attributes[3],
		}
	}
	return [4]float32{1, 1, 1, 1} // Default to white
}

// VertexColorUniforms contains uniform data for the per-vertex color shader.
type VertexColorUniforms struct {
	// MVP is the Model-View-Projection matrix in column-major order.
	MVP [16]float32
}

// VertexColorVertexShader transforms vertices and passes vertex colors as attributes.
// Expects attrs[0:4] to be RGBA color values in [0, 1].
func VertexColorVertexShader(
	_ int,
	position [3]float32,
	attributes []float32,
	uniforms any,
) raster.ClipSpaceVertex {
	u := uniforms.(*VertexColorUniforms)

	// Transform position by MVP matrix
	clipPos := Mat4MulVec4(u.MVP, [4]float32{position[0], position[1], position[2], 1.0})

	// Copy color attributes
	var attrs []float32
	if len(attributes) >= 4 {
		attrs = make([]float32, 4)
		copy(attrs, attributes[:4])
	}

	return raster.ClipSpaceVertex{
		Position:   clipPos,
		Attributes: attrs,
	}
}

// VertexColorFragmentShader returns the interpolated vertex color.
func VertexColorFragmentShader(frag raster.Fragment, _ any) [4]float32 {
	if len(frag.Attributes) >= 4 {
		return [4]float32{
			frag.Attributes[0],
			frag.Attributes[1],
			frag.Attributes[2],
			frag.Attributes[3],
		}
	}
	return [4]float32{1, 1, 1, 1} // Default to white
}

// TexturedUniforms contains uniform data for the textured shader.
type TexturedUniforms struct {
	// MVP is the Model-View-Projection matrix in column-major order.
	MVP [16]float32

	// TextureData is the RGBA8 texture data.
	TextureData []byte

	// TextureWidth is the width of the texture in pixels.
	TextureWidth int

	// TextureHeight is the height of the texture in pixels.
	TextureHeight int
}

// TexturedVertexShader transforms vertices and passes UV coordinates as attributes.
// Expects attrs[0:2] to be UV coordinates.
func TexturedVertexShader(
	_ int,
	position [3]float32,
	attributes []float32,
	uniforms any,
) raster.ClipSpaceVertex {
	u := uniforms.(*TexturedUniforms)

	// Transform position by MVP matrix
	clipPos := Mat4MulVec4(u.MVP, [4]float32{position[0], position[1], position[2], 1.0})

	// Copy UV attributes
	var attrs []float32
	if len(attributes) >= 2 {
		attrs = make([]float32, 2)
		copy(attrs, attributes[:2])
	}

	return raster.ClipSpaceVertex{
		Position:   clipPos,
		Attributes: attrs,
	}
}

// TexturedFragmentShader samples a texture using interpolated UV coordinates.
func TexturedFragmentShader(frag raster.Fragment, uniforms any) [4]float32 {
	u := uniforms.(*TexturedUniforms)

	if len(frag.Attributes) < 2 || u.TextureData == nil || u.TextureWidth == 0 || u.TextureHeight == 0 {
		return [4]float32{1, 0, 1, 1} // Magenta for missing texture
	}

	// Get UV coordinates
	uvX := frag.Attributes[0]
	uvY := frag.Attributes[1]

	// Wrap UV coordinates
	uvX -= float32(int(uvX))
	if uvX < 0 {
		uvX++
	}
	uvY -= float32(int(uvY))
	if uvY < 0 {
		uvY++
	}

	// Convert to pixel coordinates
	px := int(uvX * float32(u.TextureWidth))
	py := int(uvY * float32(u.TextureHeight))

	// Clamp to valid range
	if px >= u.TextureWidth {
		px = u.TextureWidth - 1
	}
	if py >= u.TextureHeight {
		py = u.TextureHeight - 1
	}

	// Sample texture (nearest neighbor)
	idx := (py*u.TextureWidth + px) * 4
	if idx+3 >= len(u.TextureData) {
		return [4]float32{1, 0, 1, 1} // Magenta for out of bounds
	}

	return [4]float32{
		float32(u.TextureData[idx+0]) / 255.0,
		float32(u.TextureData[idx+1]) / 255.0,
		float32(u.TextureData[idx+2]) / 255.0,
		float32(u.TextureData[idx+3]) / 255.0,
	}
}

// Mat4MulVec4 multiplies a 4x4 matrix by a vec4 (column-major order).
// This is the standard OpenGL/WebGPU matrix-vector multiplication.
func Mat4MulVec4(m [16]float32, v [4]float32) [4]float32 {
	return [4]float32{
		m[0]*v[0] + m[4]*v[1] + m[8]*v[2] + m[12]*v[3],
		m[1]*v[0] + m[5]*v[1] + m[9]*v[2] + m[13]*v[3],
		m[2]*v[0] + m[6]*v[1] + m[10]*v[2] + m[14]*v[3],
		m[3]*v[0] + m[7]*v[1] + m[11]*v[2] + m[15]*v[3],
	}
}

// Mat4Identity returns a 4x4 identity matrix.
func Mat4Identity() [16]float32 {
	return [16]float32{
		1, 0, 0, 0,
		0, 1, 0, 0,
		0, 0, 1, 0,
		0, 0, 0, 1,
	}
}

// Mat4Translate creates a translation matrix.
func Mat4Translate(x, y, z float32) [16]float32 {
	return [16]float32{
		1, 0, 0, 0,
		0, 1, 0, 0,
		0, 0, 1, 0,
		x, y, z, 1,
	}
}

// Mat4Scale creates a scale matrix.
func Mat4Scale(x, y, z float32) [16]float32 {
	return [16]float32{
		x, 0, 0, 0,
		0, y, 0, 0,
		0, 0, z, 0,
		0, 0, 0, 1,
	}
}

// Mat4Ortho creates an orthographic projection matrix.
// Parameters define the view volume: left, right, bottom, top, near, far.
func Mat4Ortho(left, right, bottom, top, near, far float32) [16]float32 {
	rml := right - left
	tmb := top - bottom
	fmn := far - near

	return [16]float32{
		2 / rml, 0, 0, 0,
		0, 2 / tmb, 0, 0,
		0, 0, -2 / fmn, 0,
		-(right + left) / rml, -(top + bottom) / tmb, -(far + near) / fmn, 1,
	}
}

// Mat4Mul multiplies two 4x4 matrices (column-major order).
func Mat4Mul(a, b [16]float32) [16]float32 {
	var result [16]float32

	for col := 0; col < 4; col++ {
		for row := 0; row < 4; row++ {
			sum := float32(0)
			for k := 0; k < 4; k++ {
				sum += a[k*4+row] * b[col*4+k]
			}
			result[col*4+row] = sum
		}
	}

	return result
}
