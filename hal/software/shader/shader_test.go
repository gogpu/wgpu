package shader

import (
	"math"
	"testing"

	"github.com/gogpu/wgpu/hal/software/raster"
)

// =============================================================================
// Shader Program Tests
// =============================================================================

func TestShaderProgramIsValid(t *testing.T) {
	tests := []struct {
		name    string
		program ShaderProgram
		want    bool
	}{
		{
			name: "valid_program",
			program: ShaderProgram{
				Vertex:   PassthroughVertexShader,
				Fragment: WhiteFragmentShader,
			},
			want: true,
		},
		{
			name: "missing_vertex",
			program: ShaderProgram{
				Vertex:   nil,
				Fragment: WhiteFragmentShader,
			},
			want: false,
		},
		{
			name: "missing_fragment",
			program: ShaderProgram{
				Vertex:   PassthroughVertexShader,
				Fragment: nil,
			},
			want: false,
		},
		{
			name:    "empty_program",
			program: ShaderProgram{},
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.program.IsValid()
			if got != tt.want {
				t.Errorf("IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

// =============================================================================
// Passthrough Vertex Shader Tests
// =============================================================================

func TestPassthroughVertexShader(t *testing.T) {
	position := [3]float32{1.0, 2.0, 3.0}
	attributes := []float32{0.5, 0.6, 0.7, 1.0}

	result := PassthroughVertexShader(0, position, attributes, nil)

	// Position should be passed through with W=1
	expectedPos := [4]float32{1.0, 2.0, 3.0, 1.0}
	if result.Position != expectedPos {
		t.Errorf("Position = %v, want %v", result.Position, expectedPos)
	}

	// Attributes should be passed through
	if len(result.Attributes) != len(attributes) {
		t.Fatalf("Attributes length = %d, want %d", len(result.Attributes), len(attributes))
	}
	for i, v := range attributes {
		if result.Attributes[i] != v {
			t.Errorf("Attributes[%d] = %v, want %v", i, result.Attributes[i], v)
		}
	}
}

// =============================================================================
// Fragment Shader Tests
// =============================================================================

func TestWhiteFragmentShader(t *testing.T) {
	frag := raster.Fragment{X: 10, Y: 20, Depth: 0.5}

	result := WhiteFragmentShader(frag, nil)

	expected := [4]float32{1, 1, 1, 1}
	if result != expected {
		t.Errorf("WhiteFragmentShader() = %v, want %v", result, expected)
	}
}

func TestDepthFragmentShader(t *testing.T) {
	tests := []struct {
		name     string
		depth    float32
		expected [4]float32
	}{
		{"near", 0.0, [4]float32{0, 0, 0, 1}},
		{"far", 1.0, [4]float32{1, 1, 1, 1}},
		{"mid", 0.5, [4]float32{0.5, 0.5, 0.5, 1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frag := raster.Fragment{X: 0, Y: 0, Depth: tt.depth}
			result := DepthFragmentShader(frag, nil)

			if !colorEqual(result, tt.expected, 0.001) {
				t.Errorf("DepthFragmentShader(depth=%v) = %v, want %v", tt.depth, result, tt.expected)
			}
		})
	}
}

func TestBarycentricFragmentShader(t *testing.T) {
	tests := []struct {
		name     string
		bary     [3]float32
		expected [4]float32
	}{
		{"vertex_0", [3]float32{1, 0, 0}, [4]float32{1, 0, 0, 1}},
		{"vertex_1", [3]float32{0, 1, 0}, [4]float32{0, 1, 0, 1}},
		{"vertex_2", [3]float32{0, 0, 1}, [4]float32{0, 0, 1, 1}},
		{"center", [3]float32{0.33, 0.33, 0.34}, [4]float32{0.33, 0.33, 0.34, 1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frag := raster.Fragment{X: 0, Y: 0, Bary: tt.bary}
			result := BarycentricFragmentShader(frag, nil)

			if !colorEqual(result, tt.expected, 0.001) {
				t.Errorf("BarycentricFragmentShader(bary=%v) = %v, want %v", tt.bary, result, tt.expected)
			}
		})
	}
}

// =============================================================================
// Solid Color Shader Tests
// =============================================================================

func TestSolidColorShader(t *testing.T) {
	// Use identity matrix
	uniforms := &SolidColorUniforms{
		MVP:   Mat4Identity(),
		Color: [4]float32{1, 0, 0, 1}, // Red
	}

	// Transform a vertex
	position := [3]float32{0.5, 0.5, 0.0}
	result := SolidColorVertexShader(0, position, nil, uniforms)

	// Position should be transformed by identity (same as input)
	if result.Position[0] != 0.5 || result.Position[1] != 0.5 || result.Position[3] != 1.0 {
		t.Errorf("Position = %v, expected [0.5, 0.5, 0, 1]", result.Position)
	}

	// Attributes should contain the color
	if len(result.Attributes) != 4 {
		t.Fatalf("Expected 4 attributes, got %d", len(result.Attributes))
	}
	for i, v := range uniforms.Color {
		if result.Attributes[i] != v {
			t.Errorf("Attributes[%d] = %v, want %v", i, result.Attributes[i], v)
		}
	}

	// Fragment shader should return the interpolated color
	frag := raster.Fragment{
		Attributes: result.Attributes,
	}
	fragColor := SolidColorFragmentShader(frag, uniforms)

	if !colorEqual(fragColor, uniforms.Color, 0.001) {
		t.Errorf("Fragment color = %v, want %v", fragColor, uniforms.Color)
	}
}

func TestSolidColorFragmentShaderDefault(t *testing.T) {
	// Test with no attributes - should return white
	frag := raster.Fragment{Attributes: nil}
	result := SolidColorFragmentShader(frag, nil)

	expected := [4]float32{1, 1, 1, 1}
	if result != expected {
		t.Errorf("SolidColorFragmentShader with no attrs = %v, want %v", result, expected)
	}
}

// =============================================================================
// Vertex Color Shader Tests
// =============================================================================

func TestVertexColorShader(t *testing.T) {
	uniforms := &VertexColorUniforms{
		MVP: Mat4Identity(),
	}

	position := [3]float32{0.5, -0.5, 0.0}
	color := []float32{0, 1, 0, 1} // Green

	result := VertexColorVertexShader(0, position, color, uniforms)

	// Position should be transformed
	if result.Position[0] != 0.5 || result.Position[1] != -0.5 {
		t.Errorf("Position = %v, expected [0.5, -0.5, 0, 1]", result.Position)
	}

	// Attributes should contain the color
	if len(result.Attributes) != 4 {
		t.Fatalf("Expected 4 attributes, got %d", len(result.Attributes))
	}

	// Fragment shader should return the interpolated color
	frag := raster.Fragment{
		Attributes: result.Attributes,
	}
	fragColor := VertexColorFragmentShader(frag, uniforms)

	expected := [4]float32{0, 1, 0, 1}
	if !colorEqual(fragColor, expected, 0.001) {
		t.Errorf("Fragment color = %v, want %v", fragColor, expected)
	}
}

// =============================================================================
// Textured Shader Tests
// =============================================================================

func TestTexturedShader(t *testing.T) {
	// Create a simple 2x2 texture: red, green, blue, white
	texture := []byte{
		255, 0, 0, 255, // (0,0) red
		0, 255, 0, 255, // (1,0) green
		0, 0, 255, 255, // (0,1) blue
		255, 255, 255, 255, // (1,1) white
	}

	uniforms := &TexturedUniforms{
		MVP:           Mat4Identity(),
		TextureData:   texture,
		TextureWidth:  2,
		TextureHeight: 2,
	}

	// Test vertex shader
	position := [3]float32{0, 0, 0}
	uv := []float32{0.5, 0.5} // Center of texture
	result := TexturedVertexShader(0, position, uv, uniforms)

	if len(result.Attributes) != 2 {
		t.Fatalf("Expected 2 UV attributes, got %d", len(result.Attributes))
	}

	// Test fragment shader at different UV coordinates
	tests := []struct {
		name     string
		uv       [2]float32
		expected [4]float32
	}{
		{"top_left", [2]float32{0, 0}, [4]float32{1, 0, 0, 1}},           // red
		{"top_right", [2]float32{0.99, 0}, [4]float32{0, 1, 0, 1}},       // green
		{"bottom_left", [2]float32{0, 0.99}, [4]float32{0, 0, 1, 1}},     // blue
		{"bottom_right", [2]float32{0.99, 0.99}, [4]float32{1, 1, 1, 1}}, // white
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frag := raster.Fragment{
				Attributes: tt.uv[:],
			}
			color := TexturedFragmentShader(frag, uniforms)

			if !colorEqual(color, tt.expected, 0.01) {
				t.Errorf("TexturedFragmentShader(uv=%v) = %v, want %v", tt.uv, color, tt.expected)
			}
		})
	}
}

func TestTexturedShaderMissingTexture(t *testing.T) {
	uniforms := &TexturedUniforms{
		MVP:           Mat4Identity(),
		TextureData:   nil,
		TextureWidth:  0,
		TextureHeight: 0,
	}

	frag := raster.Fragment{
		Attributes: []float32{0.5, 0.5},
	}
	color := TexturedFragmentShader(frag, uniforms)

	// Should return magenta for missing texture
	expected := [4]float32{1, 0, 1, 1}
	if color != expected {
		t.Errorf("Missing texture color = %v, want %v (magenta)", color, expected)
	}
}

func TestTexturedShaderUVWrapping(t *testing.T) {
	// 1x1 white texture
	texture := []byte{255, 255, 255, 255}

	uniforms := &TexturedUniforms{
		MVP:           Mat4Identity(),
		TextureData:   texture,
		TextureWidth:  1,
		TextureHeight: 1,
	}

	// Test UV wrapping with values > 1
	tests := []struct {
		name string
		uv   [2]float32
	}{
		{"normal", [2]float32{0.5, 0.5}},
		{"wrap_u", [2]float32{1.5, 0.5}},
		{"wrap_v", [2]float32{0.5, 2.5}},
		{"wrap_both", [2]float32{3.5, 4.5}},
	}

	expected := [4]float32{1, 1, 1, 1}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frag := raster.Fragment{
				Attributes: tt.uv[:],
			}
			color := TexturedFragmentShader(frag, uniforms)

			if !colorEqual(color, expected, 0.01) {
				t.Errorf("UV wrapping at %v = %v, want white", tt.uv, color)
			}
		})
	}
}

// =============================================================================
// Matrix Tests
// =============================================================================

func TestMat4Identity(t *testing.T) {
	identity := Mat4Identity()

	// Check diagonal is 1, rest is 0
	for i := 0; i < 16; i++ {
		row := i % 4
		col := i / 4
		expected := float32(0)
		if row == col {
			expected = 1
		}
		if identity[i] != expected {
			t.Errorf("identity[%d] = %v, want %v", i, identity[i], expected)
		}
	}
}

func TestMat4MulVec4Identity(t *testing.T) {
	identity := Mat4Identity()
	v := [4]float32{1, 2, 3, 1}

	result := Mat4MulVec4(identity, v)

	if result != v {
		t.Errorf("identity * v = %v, want %v", result, v)
	}
}

func TestMat4Translate(t *testing.T) {
	translate := Mat4Translate(1, 2, 3)
	v := [4]float32{0, 0, 0, 1}

	result := Mat4MulVec4(translate, v)

	expected := [4]float32{1, 2, 3, 1}
	if result != expected {
		t.Errorf("translate * origin = %v, want %v", result, expected)
	}
}

func TestMat4Scale(t *testing.T) {
	scale := Mat4Scale(2, 3, 4)
	v := [4]float32{1, 1, 1, 1}

	result := Mat4MulVec4(scale, v)

	expected := [4]float32{2, 3, 4, 1}
	if result != expected {
		t.Errorf("scale * (1,1,1) = %v, want %v", result, expected)
	}
}

func TestMat4Ortho(t *testing.T) {
	// Create orthographic projection for a 100x100 viewport
	ortho := Mat4Ortho(0, 100, 100, 0, -1, 1) // left, right, bottom, top, near, far

	// Test center of viewport maps to origin
	center := [4]float32{50, 50, 0, 1}
	result := Mat4MulVec4(ortho, center)

	// After ortho projection, (50,50) should map to (0,0) in NDC
	if math.Abs(float64(result[0])) > 0.01 || math.Abs(float64(result[1])) > 0.01 {
		t.Errorf("ortho * center = %v, expected near (0,0,0,1)", result)
	}
}

func TestMat4Mul(t *testing.T) {
	// Test: translate(1,0,0) * scale(2,2,2) should scale first, then translate
	translate := Mat4Translate(1, 0, 0)
	scale := Mat4Scale(2, 2, 2)

	combined := Mat4Mul(translate, scale)

	v := [4]float32{1, 0, 0, 1}
	result := Mat4MulVec4(combined, v)

	// scale(1,0,0) = (2,0,0), translate(2,0,0) = (3,0,0)
	expected := [4]float32{3, 0, 0, 1}
	if !vec4Equal(result, expected, 0.001) {
		t.Errorf("(translate * scale) * v = %v, want %v", result, expected)
	}
}

// =============================================================================
// Vertex Helper Tests
// =============================================================================

func TestNewVertex(t *testing.T) {
	v := NewVertex(1, 2, 3)

	if v.Position != [3]float32{1, 2, 3} {
		t.Errorf("Position = %v, want [1, 2, 3]", v.Position)
	}
	if v.Attributes != nil {
		t.Errorf("Attributes should be nil, got %v", v.Attributes)
	}
}

func TestNewVertexWithColor(t *testing.T) {
	v := NewVertexWithColor(1, 2, 3, 0.5, 0.6, 0.7, 1.0)

	if v.Position != [3]float32{1, 2, 3} {
		t.Errorf("Position = %v, want [1, 2, 3]", v.Position)
	}
	if len(v.Attributes) != 4 {
		t.Fatalf("Expected 4 attributes, got %d", len(v.Attributes))
	}
	expected := []float32{0.5, 0.6, 0.7, 1.0}
	for i, e := range expected {
		if v.Attributes[i] != e {
			t.Errorf("Attributes[%d] = %v, want %v", i, v.Attributes[i], e)
		}
	}
}

func TestNewVertexWithUV(t *testing.T) {
	v := NewVertexWithUV(1, 2, 3, 0.5, 0.75)

	if len(v.Attributes) != 2 {
		t.Fatalf("Expected 2 attributes, got %d", len(v.Attributes))
	}
	if v.Attributes[0] != 0.5 || v.Attributes[1] != 0.75 {
		t.Errorf("UV = [%v, %v], want [0.5, 0.75]", v.Attributes[0], v.Attributes[1])
	}
}

func TestNewVertexWithColorAndUV(t *testing.T) {
	v := NewVertexWithColorAndUV(1, 2, 3, 1, 0, 0, 1, 0.5, 0.5)

	if len(v.Attributes) != 6 {
		t.Fatalf("Expected 6 attributes, got %d", len(v.Attributes))
	}
	// RGBA
	if v.Attributes[0] != 1 || v.Attributes[1] != 0 || v.Attributes[2] != 0 || v.Attributes[3] != 1 {
		t.Errorf("Color = %v, want [1, 0, 0, 1]", v.Attributes[:4])
	}
	// UV
	if v.Attributes[4] != 0.5 || v.Attributes[5] != 0.5 {
		t.Errorf("UV = %v, want [0.5, 0.5]", v.Attributes[4:])
	}
}

// =============================================================================
// Benchmarks
// =============================================================================

func BenchmarkMat4MulVec4(b *testing.B) {
	m := Mat4Ortho(0, 100, 100, 0, -1, 1)
	v := [4]float32{50, 50, 0, 1}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Mat4MulVec4(m, v)
	}
}

func BenchmarkMat4Mul(b *testing.B) {
	a := Mat4Translate(1, 2, 3)
	bm := Mat4Scale(2, 2, 2)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Mat4Mul(a, bm)
	}
}

func BenchmarkSolidColorVertexShader(b *testing.B) {
	uniforms := &SolidColorUniforms{
		MVP:   Mat4Identity(),
		Color: [4]float32{1, 0, 0, 1},
	}
	position := [3]float32{0.5, 0.5, 0}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		SolidColorVertexShader(0, position, nil, uniforms)
	}
}

func BenchmarkTexturedFragmentShader(b *testing.B) {
	texture := make([]byte, 256*256*4)
	for i := range texture {
		texture[i] = byte(i % 256)
	}

	uniforms := &TexturedUniforms{
		MVP:           Mat4Identity(),
		TextureData:   texture,
		TextureWidth:  256,
		TextureHeight: 256,
	}

	frag := raster.Fragment{
		Attributes: []float32{0.5, 0.5},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		TexturedFragmentShader(frag, uniforms)
	}
}

// =============================================================================
// Helper Functions
// =============================================================================

func colorEqual(a, b [4]float32, tolerance float32) bool {
	return math.Abs(float64(a[0]-b[0])) <= float64(tolerance) &&
		math.Abs(float64(a[1]-b[1])) <= float64(tolerance) &&
		math.Abs(float64(a[2]-b[2])) <= float64(tolerance) &&
		math.Abs(float64(a[3]-b[3])) <= float64(tolerance)
}

func vec4Equal(a, b [4]float32, tolerance float32) bool {
	return colorEqual(a, b, tolerance)
}
