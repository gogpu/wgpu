//go:build software

package raster

import (
	"math"
	"testing"
)

// =============================================================================
// Blend Factor Tests
// =============================================================================

func TestApplyBlendFactorZeroOne(t *testing.T) {
	src := [4]float32{0.5, 0.6, 0.7, 0.8}
	dst := [4]float32{0.1, 0.2, 0.3, 0.4}
	constant := [4]float32{0.9, 0.9, 0.9, 0.9}

	tests := []struct {
		name     string
		factor   BlendFactor
		expected [3]float32
	}{
		{"zero", BlendFactorZero, [3]float32{0, 0, 0}},
		{"one", BlendFactorOne, [3]float32{1, 1, 1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := applyBlendFactor(tt.factor, src, dst, constant)
			if !rgb3Equal(got, tt.expected, 0.001) {
				t.Errorf("applyBlendFactor(%v) = %v, want %v", tt.factor, got, tt.expected)
			}
		})
	}
}

func TestApplyBlendFactorSrcDst(t *testing.T) {
	src := [4]float32{0.2, 0.4, 0.6, 0.8}
	dst := [4]float32{0.1, 0.3, 0.5, 0.7}
	constant := [4]float32{0, 0, 0, 0}

	tests := []struct {
		name     string
		factor   BlendFactor
		expected [3]float32
	}{
		{"src", BlendFactorSrc, [3]float32{0.2, 0.4, 0.6}},
		{"one_minus_src", BlendFactorOneMinusSrc, [3]float32{0.8, 0.6, 0.4}},
		{"dst", BlendFactorDst, [3]float32{0.1, 0.3, 0.5}},
		{"one_minus_dst", BlendFactorOneMinusDst, [3]float32{0.9, 0.7, 0.5}},
		{"src_alpha", BlendFactorSrcAlpha, [3]float32{0.8, 0.8, 0.8}},
		{"one_minus_src_alpha", BlendFactorOneMinusSrcAlpha, [3]float32{0.2, 0.2, 0.2}},
		{"dst_alpha", BlendFactorDstAlpha, [3]float32{0.7, 0.7, 0.7}},
		{"one_minus_dst_alpha", BlendFactorOneMinusDstAlpha, [3]float32{0.3, 0.3, 0.3}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := applyBlendFactor(tt.factor, src, dst, constant)
			if !rgb3Equal(got, tt.expected, 0.001) {
				t.Errorf("applyBlendFactor(%v) = %v, want %v", tt.factor, got, tt.expected)
			}
		})
	}
}

func TestApplyBlendFactorConstant(t *testing.T) {
	src := [4]float32{0.5, 0.5, 0.5, 0.5}
	dst := [4]float32{0.5, 0.5, 0.5, 0.5}
	constant := [4]float32{0.3, 0.5, 0.7, 0.9}

	tests := []struct {
		name     string
		factor   BlendFactor
		expected [3]float32
	}{
		{"constant", BlendFactorConstant, [3]float32{0.3, 0.5, 0.7}},
		{"one_minus_constant", BlendFactorOneMinusConstant, [3]float32{0.7, 0.5, 0.3}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := applyBlendFactor(tt.factor, src, dst, constant)
			if !rgb3Equal(got, tt.expected, 0.001) {
				t.Errorf("applyBlendFactor(%v) = %v, want %v", tt.factor, got, tt.expected)
			}
		})
	}
}

func TestApplyBlendFactorSrcAlphaSaturated(t *testing.T) {
	// srcAlphaSaturated = min(srcAlpha, 1-dstAlpha)
	tests := []struct {
		name     string
		srcAlpha float32
		dstAlpha float32
		expected float32
	}{
		{"src_less", 0.3, 0.5, 0.3}, // min(0.3, 0.5) = 0.3
		{"dst_less", 0.7, 0.5, 0.5}, // min(0.7, 0.5) = 0.5
		{"equal", 0.5, 0.5, 0.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := [4]float32{1, 1, 1, tt.srcAlpha}
			dst := [4]float32{0, 0, 0, tt.dstAlpha}
			constant := [4]float32{0, 0, 0, 0}

			got := applyBlendFactor(BlendFactorSrcAlphaSaturated, src, dst, constant)
			expected := [3]float32{tt.expected, tt.expected, tt.expected}

			if !rgb3Equal(got, expected, 0.001) {
				t.Errorf("srcAlphaSaturated = %v, want %v", got, expected)
			}
		})
	}
}

// =============================================================================
// Blend Operation Tests
// =============================================================================

func TestApplyBlendOp(t *testing.T) {
	tests := []struct {
		name     string
		op       BlendOperation
		src, dst float32
		expected float32
	}{
		{"add", BlendOpAdd, 0.3, 0.4, 0.7},
		{"subtract", BlendOpSubtract, 0.5, 0.3, 0.2},
		{"reverse_subtract", BlendOpReverseSubtract, 0.3, 0.5, 0.2},
		{"min", BlendOpMin, 0.3, 0.5, 0.3},
		{"max", BlendOpMax, 0.3, 0.5, 0.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := applyBlendOp(tt.op, tt.src, tt.dst)
			if math.Abs(float64(got-tt.expected)) > 0.001 {
				t.Errorf("applyBlendOp(%v, %v, %v) = %v, want %v", tt.op, tt.src, tt.dst, got, tt.expected)
			}
		})
	}
}

// =============================================================================
// Blend State Tests
// =============================================================================

func TestBlendDisabled(t *testing.T) {
	src := [4]float32{1, 0, 0, 1}
	dst := [4]float32{0, 1, 0, 1}

	result := Blend(src, dst, BlendDisabled)

	// With blending disabled, result should equal source
	if !rgba4Equal(result, src, 0.001) {
		t.Errorf("BlendDisabled: got %v, want %v (source)", result, src)
	}
}

func TestBlendSourceOver(t *testing.T) {
	// Standard alpha blending: out = src * srcAlpha + dst * (1 - srcAlpha)
	tests := []struct {
		name     string
		src, dst [4]float32
		expected [4]float32
	}{
		{
			name:     "opaque_over_opaque",
			src:      [4]float32{1, 0, 0, 1}, // Opaque red
			dst:      [4]float32{0, 1, 0, 1}, // Opaque green
			expected: [4]float32{1, 0, 0, 1}, // Red (src completely covers)
		},
		{
			name:     "transparent_over_opaque",
			src:      [4]float32{1, 0, 0, 0}, // Fully transparent red
			dst:      [4]float32{0, 1, 0, 1}, // Opaque green
			expected: [4]float32{0, 1, 0, 1}, // Green (src is invisible)
		},
		{
			name:     "half_transparent_over_opaque",
			src:      [4]float32{1, 0, 0, 0.5}, // 50% red
			dst:      [4]float32{0, 1, 0, 1},   // Opaque green
			expected: [4]float32{0.5, 0.5, 0, 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Blend(tt.src, tt.dst, BlendSourceOver)
			if !rgba4Equal(result, tt.expected, 0.01) {
				t.Errorf("BlendSourceOver(%v, %v) = %v, want %v", tt.src, tt.dst, result, tt.expected)
			}
		})
	}
}

func TestBlendAdditive(t *testing.T) {
	// Additive: out = src + dst
	src := [4]float32{0.3, 0.0, 0.0, 1}
	dst := [4]float32{0.0, 0.4, 0.0, 1}

	result := Blend(src, dst, BlendAdditive)

	// Should add the colors (clamped to 1.0)
	expected := [4]float32{0.3, 0.4, 0.0, 1}
	if !rgba4Equal(result, expected, 0.01) {
		t.Errorf("BlendAdditive = %v, want %v", result, expected)
	}

	// Test clamping
	src = [4]float32{0.8, 0.0, 0.0, 1}
	dst = [4]float32{0.5, 0.0, 0.0, 1}
	result = Blend(src, dst, BlendAdditive)
	if result[0] > 1.0 {
		t.Errorf("BlendAdditive should clamp, got R=%v", result[0])
	}
}

func TestBlendMultiply(t *testing.T) {
	// Multiply: out = src * dst
	src := [4]float32{0.5, 1.0, 0.5, 1}
	dst := [4]float32{1.0, 0.5, 0.5, 1}

	result := Blend(src, dst, BlendMultiply)

	expected := [4]float32{0.5, 0.5, 0.25, 1}
	if !rgba4Equal(result, expected, 0.01) {
		t.Errorf("BlendMultiply = %v, want %v", result, expected)
	}
}

func TestBlendScreen(t *testing.T) {
	// Screen: out = 1 - (1-src)*(1-dst) = src + dst - src*dst
	src := [4]float32{0.5, 0.0, 0.0, 1}
	dst := [4]float32{0.0, 0.5, 0.0, 1}

	result := Blend(src, dst, BlendScreen)

	// Screen should lighten
	// R: 0.5 + 0 - 0 = 0.5
	// G: 0 + 0.5 - 0 = 0.5
	expected := [4]float32{0.5, 0.5, 0, 1}
	if !rgba4Equal(result, expected, 0.01) {
		t.Errorf("BlendScreen = %v, want %v", result, expected)
	}
}

func TestBlendPremultiplied(t *testing.T) {
	// Premultiplied alpha: src color is already multiplied by alpha
	// out = src + dst * (1 - srcAlpha)
	tests := []struct {
		name     string
		src, dst [4]float32
		expected [4]float32
	}{
		{
			name:     "opaque",
			src:      [4]float32{1, 0, 0, 1}, // Premultiplied opaque red
			dst:      [4]float32{0, 1, 0, 1},
			expected: [4]float32{1, 0, 0, 1},
		},
		{
			name:     "half_transparent",
			src:      [4]float32{0.5, 0, 0, 0.5}, // 50% red, premultiplied (0.5*1, 0, 0)
			dst:      [4]float32{0, 1, 0, 1},
			expected: [4]float32{0.5, 0.5, 0, 1}, // 0.5 + 0*(1-0.5), 0 + 1*0.5, 0
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Blend(tt.src, tt.dst, BlendPremultiplied)
			if !rgba4Equal(result, tt.expected, 0.01) {
				t.Errorf("BlendPremultiplied(%v, %v) = %v, want %v", tt.src, tt.dst, result, tt.expected)
			}
		})
	}
}

// =============================================================================
// Byte Conversion Tests
// =============================================================================

func TestBlendBytes(t *testing.T) {
	// 50% red over green
	r, g, b, a := BlendBytes(255, 0, 0, 128, 0, 255, 0, 255, BlendSourceOver)

	// Should be roughly 50% blend
	if r < 100 || r > 156 {
		t.Errorf("BlendBytes R = %d, expected ~128", r)
	}
	if g < 100 || g > 156 {
		t.Errorf("BlendBytes G = %d, expected ~128", g)
	}
	if b != 0 {
		t.Errorf("BlendBytes B = %d, expected 0", b)
	}
	if a != 255 {
		t.Errorf("BlendBytes A = %d, expected 255", a)
	}
}

func TestBlendFloatToByte(t *testing.T) {
	src := [4]float32{1, 0, 0, 0.5}
	r, g, b, a := BlendFloatToByte(src, 0, 255, 0, 255, BlendSourceOver)

	// Should be roughly 50% blend
	if r < 100 || r > 156 {
		t.Errorf("BlendFloatToByte R = %d, expected ~128", r)
	}
	if g < 100 || g > 156 {
		t.Errorf("BlendFloatToByte G = %d, expected ~128", g)
	}
	if b != 0 {
		t.Errorf("BlendFloatToByte B = %d, expected 0", b)
	}
	if a != 255 {
		t.Errorf("BlendFloatToByte A = %d, expected 255", a)
	}
}

// =============================================================================
// Pipeline Blending Integration Tests
// =============================================================================

func TestPipelineBlendingSourceOver(t *testing.T) {
	p := NewPipeline(100, 100)

	// Clear to opaque green
	p.Clear(0, 1, 0, 1)

	// Enable alpha blending
	p.SetBlendState(BlendSourceOver)

	// Draw a 50% transparent red triangle
	tri := CreateScreenTriangle(
		10, 10, 0.5,
		50, 10, 0.5,
		30, 50, 0.5,
	)

	// 50% transparent red
	p.DrawTriangles([]Triangle{tri}, [4]float32{1, 0, 0, 0.5})

	// Check center - should be blended (yellow-ish)
	r, g, b, a := p.GetPixel(30, 25)

	// Red and green should both be present (non-zero)
	if r < 100 {
		t.Errorf("Expected red component, got R=%d", r)
	}
	if g < 100 {
		t.Errorf("Expected green component, got G=%d", g)
	}
	if b > 10 {
		t.Errorf("Expected no blue, got B=%d", b)
	}
	if a < 250 {
		t.Errorf("Expected opaque, got A=%d", a)
	}

	// Check outside triangle - should still be green
	r, g, b, a = p.GetPixel(5, 5)
	if r != 0 || g != 255 || b != 0 || a != 255 {
		t.Errorf("Outside triangle should be green, got (%d, %d, %d, %d)", r, g, b, a)
	}
}

func TestPipelineBlendingDisabled(t *testing.T) {
	p := NewPipeline(100, 100)

	// Clear to green
	p.Clear(0, 1, 0, 1)

	// Keep blending disabled (default)
	p.SetBlendState(BlendDisabled)

	// Draw a 50% transparent red triangle
	tri := CreateScreenTriangle(
		10, 10, 0.5,
		50, 10, 0.5,
		30, 50, 0.5,
	)

	p.DrawTriangles([]Triangle{tri}, [4]float32{1, 0, 0, 0.5})

	// Check center - should be exactly the source color (no blending)
	r, g, b, _ := p.GetPixel(30, 25)

	// Should be red with no green (blending disabled)
	if r != 255 {
		t.Errorf("Expected R=255 (no blending), got R=%d", r)
	}
	if g != 0 {
		t.Errorf("Expected G=0 (no blending), got G=%d", g)
	}
	if b != 0 {
		t.Errorf("Expected B=0, got B=%d", b)
	}
}

func TestPipelineBlendingAdditive(t *testing.T) {
	p := NewPipeline(100, 100)

	// Clear to dark red
	p.Clear(0.3, 0, 0, 1)

	// Enable additive blending
	p.SetBlendState(BlendAdditive)

	// Draw a blue triangle
	tri := CreateScreenTriangle(
		10, 10, 0.5,
		50, 10, 0.5,
		30, 50, 0.5,
	)

	p.DrawTriangles([]Triangle{tri}, [4]float32{0, 0, 0.5, 1})

	// Check center - should have both red and blue
	r, g, b, _ := p.GetPixel(30, 25)

	if r < 70 { // 0.3 * 255 = ~76
		t.Errorf("Expected red from background, got R=%d", r)
	}
	if g != 0 {
		t.Errorf("Expected no green, got G=%d", g)
	}
	if b < 100 { // 0.5 * 255 = ~128
		t.Errorf("Expected blue from triangle, got B=%d", b)
	}
}

func TestPipelineBlendingInterpolated(t *testing.T) {
	p := NewPipeline(100, 100)

	// Clear to white
	p.Clear(1, 1, 1, 1)

	// Enable alpha blending
	p.SetBlendState(BlendSourceOver)

	// Create triangle with transparent vertex colors
	red := [4]float32{1, 0, 0, 0.5}
	green := [4]float32{0, 1, 0, 0.5}
	blue := [4]float32{0, 0, 1, 0.5}

	tri := CreateScreenTriangleWithColor(
		10, 10, 0.5, red,
		90, 10, 0.5, green,
		50, 90, 0.5, blue,
	)

	p.DrawTrianglesInterpolated([]Triangle{tri})

	// Center should be a blend of interpolated color and white background
	r, g, b, a := p.GetPixel(50, 35)

	// All components should be present
	if r < 100 {
		t.Errorf("Expected red component, got R=%d", r)
	}
	if g < 100 {
		t.Errorf("Expected green component, got G=%d", g)
	}
	if b < 100 {
		t.Errorf("Expected blue component, got B=%d", b)
	}
	if a < 250 {
		t.Errorf("Expected opaque, got A=%d", a)
	}
}

// =============================================================================
// Clamping Tests
// =============================================================================

func TestBlendClamping(t *testing.T) {
	// Test that results are clamped to [0, 1]
	src := [4]float32{0.8, 0, 0, 1}
	dst := [4]float32{0.8, 0, 0, 1}

	result := Blend(src, dst, BlendAdditive)

	// 0.8 + 0.8 = 1.6, should clamp to 1.0
	if result[0] > 1.0 || result[0] < 0.99 {
		t.Errorf("Expected R clamped to 1.0, got %v", result[0])
	}
}

func TestBlendSubtractClamping(t *testing.T) {
	// Test negative result clamping
	src := [4]float32{0.2, 0, 0, 1}
	dst := [4]float32{0.5, 0, 0, 1}

	state := BlendState{
		Enabled:  true,
		SrcColor: BlendFactorOne,
		DstColor: BlendFactorOne,
		ColorOp:  BlendOpSubtract, // src - dst = 0.2 - 0.5 = -0.3
		SrcAlpha: BlendFactorOne,
		DstAlpha: BlendFactorOne,
		AlphaOp:  BlendOpAdd,
	}

	result := Blend(src, dst, state)

	// -0.3 should clamp to 0
	if result[0] < 0 || result[0] > 0.001 {
		t.Errorf("Expected R clamped to 0, got %v", result[0])
	}
}

// =============================================================================
// Helper Functions
// =============================================================================

func TestClampFloat(t *testing.T) {
	tests := []struct {
		v, min, max, expected float32
	}{
		{0.5, 0, 1, 0.5},
		{-1, 0, 1, 0},
		{2, 0, 1, 1},
		{0, 0, 1, 0},
		{1, 0, 1, 1},
	}

	for _, tt := range tests {
		got := clampFloat(tt.v, tt.min, tt.max)
		if got != tt.expected {
			t.Errorf("clampFloat(%v, %v, %v) = %v, want %v", tt.v, tt.min, tt.max, got, tt.expected)
		}
	}
}

// =============================================================================
// Benchmarks
// =============================================================================

func BenchmarkBlendSourceOver(b *testing.B) {
	src := [4]float32{1, 0, 0, 0.5}
	dst := [4]float32{0, 1, 0, 1}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Blend(src, dst, BlendSourceOver)
	}
}

func BenchmarkBlendBytes(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		BlendBytes(255, 0, 0, 128, 0, 255, 0, 255, BlendSourceOver)
	}
}

func BenchmarkBlendFloatToByte(b *testing.B) {
	src := [4]float32{1, 0, 0, 0.5}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		BlendFloatToByte(src, 0, 255, 0, 255, BlendSourceOver)
	}
}

// =============================================================================
// Helper Functions for Tests
// =============================================================================

func rgb3Equal(a, b [3]float32, tolerance float32) bool {
	return math.Abs(float64(a[0]-b[0])) <= float64(tolerance) &&
		math.Abs(float64(a[1]-b[1])) <= float64(tolerance) &&
		math.Abs(float64(a[2]-b[2])) <= float64(tolerance)
}

func rgba4Equal(a, b [4]float32, tolerance float32) bool {
	return math.Abs(float64(a[0]-b[0])) <= float64(tolerance) &&
		math.Abs(float64(a[1]-b[1])) <= float64(tolerance) &&
		math.Abs(float64(a[2]-b[2])) <= float64(tolerance) &&
		math.Abs(float64(a[3]-b[3])) <= float64(tolerance)
}
