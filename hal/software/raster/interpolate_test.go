package raster

import (
	"math"
	"testing"
)

// =============================================================================
// Float32 Interpolation Tests
// =============================================================================

func TestInterpolateFloat32Basic(t *testing.T) {
	tests := []struct {
		name       string
		v0, v1, v2 float32
		b0, b1, b2 float32
		w0, w1, w2 float32
		expected   float32
		tolerance  float32
	}{
		{
			name: "at_vertex_0",
			v0:   1.0, v1: 0.0, v2: 0.0,
			b0: 1.0, b1: 0.0, b2: 0.0,
			w0: 1.0, w1: 1.0, w2: 1.0,
			expected:  1.0,
			tolerance: 0.001,
		},
		{
			name: "at_vertex_1",
			v0:   0.0, v1: 1.0, v2: 0.0,
			b0: 0.0, b1: 1.0, b2: 0.0,
			w0: 1.0, w1: 1.0, w2: 1.0,
			expected:  1.0,
			tolerance: 0.001,
		},
		{
			name: "at_vertex_2",
			v0:   0.0, v1: 0.0, v2: 1.0,
			b0: 0.0, b1: 0.0, b2: 1.0,
			w0: 1.0, w1: 1.0, w2: 1.0,
			expected:  1.0,
			tolerance: 0.001,
		},
		{
			name: "center_equal_weights",
			v0:   1.0, v1: 1.0, v2: 1.0,
			b0: 1.0 / 3, b1: 1.0 / 3, b2: 1.0 / 3,
			w0: 1.0, w1: 1.0, w2: 1.0,
			expected:  1.0,
			tolerance: 0.001,
		},
		{
			name: "midpoint_01",
			v0:   0.0, v1: 1.0, v2: 0.0,
			b0: 0.5, b1: 0.5, b2: 0.0,
			w0: 1.0, w1: 1.0, w2: 1.0,
			expected:  0.5,
			tolerance: 0.001,
		},
		{
			name: "linear_gradient",
			v0:   0.0, v1: 1.0, v2: 0.5,
			b0: 0.25, b1: 0.5, b2: 0.25,
			w0: 1.0, w1: 1.0, w2: 1.0,
			expected:  0.625, // 0*0.25 + 1*0.5 + 0.5*0.25
			tolerance: 0.001,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := InterpolateFloat32(tt.v0, tt.v1, tt.v2, tt.b0, tt.b1, tt.b2, tt.w0, tt.w1, tt.w2)
			if math.Abs(float64(got-tt.expected)) > float64(tt.tolerance) {
				t.Errorf("InterpolateFloat32() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestInterpolateFloat32PerspectiveCorrection(t *testing.T) {
	// When W values differ, perspective correction should produce different results
	// than linear interpolation

	v0, v1, v2 := float32(0.0), float32(1.0), float32(0.0)
	b0, b1, b2 := float32(0.5), float32(0.5), float32(0.0)

	// Linear case (all W = 1)
	linear := InterpolateFloat32(v0, v1, v2, b0, b1, b2, 1.0, 1.0, 1.0)

	// Perspective case (W varies - closer objects have higher W)
	// V1 is closer (W=2 means 1/w = 2, so w = 0.5, meaning closer to camera)
	perspective := InterpolateFloat32(v0, v1, v2, b0, b1, b2, 1.0, 2.0, 1.0)

	// When V1 is closer, the perspective-corrected value should be weighted more toward V1
	if perspective <= linear {
		t.Errorf("Expected perspective (%v) > linear (%v) when V1 is closer", perspective, linear)
	}

	t.Logf("Linear: %v, Perspective: %v", linear, perspective)
}

func TestInterpolateFloat32ZeroW(t *testing.T) {
	// When all W = 0, should fallback to linear interpolation
	result := InterpolateFloat32(1.0, 2.0, 3.0, 0.5, 0.3, 0.2, 0.0, 0.0, 0.0)
	expected := float32(1.0*0.5 + 2.0*0.3 + 3.0*0.2)

	if math.Abs(float64(result-expected)) > 0.001 {
		t.Errorf("InterpolateFloat32 with zero W = %v, want %v", result, expected)
	}
}

func TestInterpolateFloat32Linear(t *testing.T) {
	// Linear interpolation should match perspective when W=1
	v0, v1, v2 := float32(1.0), float32(2.0), float32(3.0)
	b0, b1, b2 := float32(0.25), float32(0.5), float32(0.25)

	linear := InterpolateFloat32Linear(v0, v1, v2, b0, b1, b2)
	perspective := InterpolateFloat32(v0, v1, v2, b0, b1, b2, 1.0, 1.0, 1.0)

	if math.Abs(float64(linear-perspective)) > 0.001 {
		t.Errorf("Linear (%v) should equal perspective with W=1 (%v)", linear, perspective)
	}
}

// =============================================================================
// Vec2 Interpolation Tests
// =============================================================================

func TestInterpolateVec2Basic(t *testing.T) {
	v0 := [2]float32{0, 0}
	v1 := [2]float32{1, 0}
	v2 := [2]float32{0, 1}

	// At center
	b0, b1, b2 := float32(1.0/3), float32(1.0/3), float32(1.0/3)
	result := InterpolateVec2(v0, v1, v2, b0, b1, b2, 1.0, 1.0, 1.0)

	expectedX := float32(1.0 / 3)
	expectedY := float32(1.0 / 3)

	if math.Abs(float64(result[0]-expectedX)) > 0.01 || math.Abs(float64(result[1]-expectedY)) > 0.01 {
		t.Errorf("InterpolateVec2 at center = %v, want [%v, %v]", result, expectedX, expectedY)
	}
}

func TestInterpolateVec2AtVertices(t *testing.T) {
	v0 := [2]float32{0.0, 0.0}
	v1 := [2]float32{1.0, 0.0}
	v2 := [2]float32{0.5, 1.0}

	tests := []struct {
		name       string
		b0, b1, b2 float32
		expected   [2]float32
	}{
		{"vertex_0", 1.0, 0.0, 0.0, v0},
		{"vertex_1", 0.0, 1.0, 0.0, v1},
		{"vertex_2", 0.0, 0.0, 1.0, v2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := InterpolateVec2(v0, v1, v2, tt.b0, tt.b1, tt.b2, 1.0, 1.0, 1.0)
			if !vec2Equal(result, tt.expected, 0.001) {
				t.Errorf("InterpolateVec2() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// =============================================================================
// Vec3 Interpolation Tests
// =============================================================================

func TestInterpolateVec3Basic(t *testing.T) {
	// RGB colors at each vertex
	red := [3]float32{1, 0, 0}
	green := [3]float32{0, 1, 0}
	blue := [3]float32{0, 0, 1}

	// At center, should be a mix of all three
	b0, b1, b2 := float32(1.0/3), float32(1.0/3), float32(1.0/3)
	result := InterpolateVec3(red, green, blue, b0, b1, b2, 1.0, 1.0, 1.0)

	expected := float32(1.0 / 3)
	for i, v := range result {
		if math.Abs(float64(v-expected)) > 0.01 {
			t.Errorf("InterpolateVec3[%d] = %v, want ~%v", i, v, expected)
		}
	}
}

func TestInterpolateVec3Linear(t *testing.T) {
	v0 := [3]float32{1, 2, 3}
	v1 := [3]float32{4, 5, 6}
	v2 := [3]float32{7, 8, 9}

	b0, b1, b2 := float32(0.25), float32(0.5), float32(0.25)

	linear := InterpolateVec3Linear(v0, v1, v2, b0, b1, b2)
	perspective := InterpolateVec3(v0, v1, v2, b0, b1, b2, 1.0, 1.0, 1.0)

	if !vec3Equal(linear, perspective, 0.001) {
		t.Errorf("Linear %v should equal perspective with W=1 %v", linear, perspective)
	}
}

// =============================================================================
// Vec4 Interpolation Tests
// =============================================================================

func TestInterpolateVec4Basic(t *testing.T) {
	// RGBA colors
	red := [4]float32{1, 0, 0, 1}
	green := [4]float32{0, 1, 0, 1}
	blue := [4]float32{0, 0, 1, 1}

	// Midpoint between red and green
	result := InterpolateVec4(red, green, blue, 0.5, 0.5, 0.0, 1.0, 1.0, 1.0)

	// Should be yellow (0.5, 0.5, 0, 1)
	expected := [4]float32{0.5, 0.5, 0, 1}
	if !vec4Equal(result, expected, 0.001) {
		t.Errorf("InterpolateVec4 midpoint = %v, want %v", result, expected)
	}
}

func TestInterpolateVec4AlphaBlending(t *testing.T) {
	// Test interpolation with varying alpha
	transparent := [4]float32{1, 0, 0, 0}
	opaque := [4]float32{1, 0, 0, 1}
	semi := [4]float32{1, 0, 0, 0.5}

	// Midpoint should have alpha 0.5
	result := InterpolateVec4(transparent, opaque, semi, 0.5, 0.5, 0.0, 1.0, 1.0, 1.0)

	if math.Abs(float64(result[3]-0.5)) > 0.001 {
		t.Errorf("Alpha interpolation = %v, want 0.5", result[3])
	}
}

// =============================================================================
// Attribute Array Interpolation Tests
// =============================================================================

func TestInterpolateAttributesBasic(t *testing.T) {
	attrs0 := []float32{1, 0, 0, 1, 0.0, 0.0}
	attrs1 := []float32{0, 1, 0, 1, 1.0, 0.0}
	attrs2 := []float32{0, 0, 1, 1, 0.5, 1.0}

	b0, b1, b2 := float32(1.0/3), float32(1.0/3), float32(1.0/3)
	result := InterpolateAttributes(attrs0, attrs1, attrs2, b0, b1, b2, 1.0, 1.0, 1.0)

	if len(result) != 6 {
		t.Fatalf("Expected 6 attributes, got %d", len(result))
	}

	// Check that all RGB components are ~1/3
	for i := 0; i < 3; i++ {
		if math.Abs(float64(result[i]-1.0/3)) > 0.01 {
			t.Errorf("result[%d] = %v, want ~0.333", i, result[i])
		}
	}

	// Alpha should be 1
	if math.Abs(float64(result[3]-1.0)) > 0.001 {
		t.Errorf("Alpha = %v, want 1.0", result[3])
	}
}

func TestInterpolateAttributesMismatchedLength(t *testing.T) {
	attrs0 := []float32{1, 2, 3}
	attrs1 := []float32{1, 2}
	attrs2 := []float32{1, 2, 3}

	result := InterpolateAttributes(attrs0, attrs1, attrs2, 0.33, 0.33, 0.34, 1.0, 1.0, 1.0)

	if result != nil {
		t.Error("Expected nil for mismatched attribute lengths")
	}
}

func TestInterpolateAttributesEmpty(t *testing.T) {
	attrs0 := []float32{}
	attrs1 := []float32{}
	attrs2 := []float32{}

	result := InterpolateAttributes(attrs0, attrs1, attrs2, 0.33, 0.33, 0.34, 1.0, 1.0, 1.0)

	if result != nil {
		t.Error("Expected nil for empty attributes")
	}
}

func TestInterpolateAttributesLinear(t *testing.T) {
	attrs0 := []float32{1, 2}
	attrs1 := []float32{3, 4}
	attrs2 := []float32{5, 6}

	b0, b1, b2 := float32(0.5), float32(0.3), float32(0.2)

	linear := InterpolateAttributesLinear(attrs0, attrs1, attrs2, b0, b1, b2)
	perspective := InterpolateAttributes(attrs0, attrs1, attrs2, b0, b1, b2, 1.0, 1.0, 1.0)

	if len(linear) != len(perspective) {
		t.Fatal("Length mismatch")
	}

	for i := range linear {
		if math.Abs(float64(linear[i]-perspective[i])) > 0.001 {
			t.Errorf("linear[%d]=%v != perspective[%d]=%v", i, linear[i], i, perspective[i])
		}
	}
}

// =============================================================================
// Depth Interpolation Tests
// =============================================================================

func TestInterpolateDepth(t *testing.T) {
	tests := []struct {
		name       string
		z0, z1, z2 float32
		b0, b1, b2 float32
		w0, w1, w2 float32
		expected   float32
		tolerance  float32
	}{
		{
			name: "at_vertex_0",
			z0:   0.0, z1: 0.5, z2: 1.0,
			b0: 1.0, b1: 0.0, b2: 0.0,
			w0: 1.0, w1: 1.0, w2: 1.0,
			expected:  0.0,
			tolerance: 0.001,
		},
		{
			name: "midpoint_linear",
			z0:   0.0, z1: 1.0, z2: 0.0,
			b0: 0.5, b1: 0.5, b2: 0.0,
			w0: 1.0, w1: 1.0, w2: 1.0,
			expected:  0.5,
			tolerance: 0.001,
		},
		{
			name: "uniform_depth",
			z0:   0.5, z1: 0.5, z2: 0.5,
			b0: 0.33, b1: 0.33, b2: 0.34,
			w0: 1.0, w1: 1.0, w2: 1.0,
			expected:  0.5,
			tolerance: 0.001,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := InterpolateDepth(tt.z0, tt.z1, tt.z2, tt.b0, tt.b1, tt.b2, tt.w0, tt.w1, tt.w2)
			if math.Abs(float64(got-tt.expected)) > float64(tt.tolerance) {
				t.Errorf("InterpolateDepth() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// =============================================================================
// Benchmarks
// =============================================================================

func BenchmarkInterpolateFloat32(b *testing.B) {
	for i := 0; i < b.N; i++ {
		InterpolateFloat32(0.0, 1.0, 0.5, 0.33, 0.33, 0.34, 1.0, 1.0, 1.0)
	}
}

func BenchmarkInterpolateFloat32Linear(b *testing.B) {
	for i := 0; i < b.N; i++ {
		InterpolateFloat32Linear(0.0, 1.0, 0.5, 0.33, 0.33, 0.34)
	}
}

func BenchmarkInterpolateVec4(b *testing.B) {
	v0 := [4]float32{1, 0, 0, 1}
	v1 := [4]float32{0, 1, 0, 1}
	v2 := [4]float32{0, 0, 1, 1}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		InterpolateVec4(v0, v1, v2, 0.33, 0.33, 0.34, 1.0, 1.0, 1.0)
	}
}

func BenchmarkInterpolateAttributes(b *testing.B) {
	attrs0 := []float32{1, 0, 0, 1, 0.0, 0.0}
	attrs1 := []float32{0, 1, 0, 1, 1.0, 0.0}
	attrs2 := []float32{0, 0, 1, 1, 0.5, 1.0}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		InterpolateAttributes(attrs0, attrs1, attrs2, 0.33, 0.33, 0.34, 1.0, 1.0, 1.0)
	}
}

// =============================================================================
// Helper Functions
// =============================================================================

func vec2Equal(a, b [2]float32, tolerance float32) bool {
	return math.Abs(float64(a[0]-b[0])) <= float64(tolerance) &&
		math.Abs(float64(a[1]-b[1])) <= float64(tolerance)
}

func vec3Equal(a, b [3]float32, tolerance float32) bool {
	return math.Abs(float64(a[0]-b[0])) <= float64(tolerance) &&
		math.Abs(float64(a[1]-b[1])) <= float64(tolerance) &&
		math.Abs(float64(a[2]-b[2])) <= float64(tolerance)
}

func vec4Equal(a, b [4]float32, tolerance float32) bool {
	return math.Abs(float64(a[0]-b[0])) <= float64(tolerance) &&
		math.Abs(float64(a[1]-b[1])) <= float64(tolerance) &&
		math.Abs(float64(a[2]-b[2])) <= float64(tolerance) &&
		math.Abs(float64(a[3]-b[3])) <= float64(tolerance)
}
