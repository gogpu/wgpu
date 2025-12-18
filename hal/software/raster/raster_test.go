//go:build software

package raster

import (
	"math"
	"testing"
)

// =============================================================================
// Edge Function Tests
// =============================================================================

func TestEdgeFunctionBasic(t *testing.T) {
	// Edge from (0, 0) to (10, 0) - horizontal edge along X axis, going right
	// In screen space with Y increasing downward:
	// - Edge function: A = y0 - y1 = 0, B = x1 - x0 = 10, C = 0
	// - E(x, y) = 0*x + 10*y + 0 = 10*y
	// - Points with y > 0 are positive (below the edge in screen space)
	// - Points with y < 0 are negative (above the edge in screen space)
	e := NewEdgeFunction(0, 0, 10, 0)

	tests := []struct {
		name     string
		x, y     float32
		wantSign int // -1, 0, or 1
	}{
		{"point_above_screen", 5, -5, -1}, // Above the edge in screen coords (y < 0)
		{"point_on", 5, 0, 0},             // On the edge
		{"point_below_screen", 5, 5, 1},   // Below the edge in screen coords (y > 0)
		{"point_left", -5, 0, 0},          // On the edge (extended)
		{"point_right", 15, 0, 0},         // On the edge (extended)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := e.Evaluate(tt.x, tt.y)
			gotSign := sign(got)
			if gotSign != tt.wantSign {
				t.Errorf("Evaluate(%v, %v) = %v (sign %v), want sign %v",
					tt.x, tt.y, got, gotSign, tt.wantSign)
			}
		})
	}
}

func TestEdgeFunctionSign(t *testing.T) {
	// Edge from (0, 0) to (10, 10) - diagonal
	e := NewEdgeFunction(0, 0, 10, 10)

	// Points to the left of the diagonal should be positive
	if got := e.Evaluate(0, 5); got <= 0 {
		t.Errorf("Point (0, 5) should be left of diagonal, got %v", got)
	}

	// Points to the right of the diagonal should be negative
	if got := e.Evaluate(5, 0); got >= 0 {
		t.Errorf("Point (5, 0) should be right of diagonal, got %v", got)
	}

	// Point on the diagonal should be zero
	if got := e.Evaluate(5, 5); got != 0 {
		t.Errorf("Point (5, 5) should be on diagonal, got %v", got)
	}
}

func TestEdgeFunctionIsTopLeft(t *testing.T) {
	// IsTopLeft determines if an edge should include pixels exactly on it.
	// For CCW triangle (0,0)-(10,0)-(5,10):
	// - Edge 0->1: (0,0)->(10,0) is horizontal going right, A=0, B=10 > 0 (NOT top-left)
	// - Edge 1->2: (10,0)->(5,10) going down-left, A=-10 < 0 (NOT top-left)
	// - Edge 2->0: (5,10)->(0,0) going up-left, A=10 > 0 (IS top-left)
	//
	// Top-left rule: A > 0 OR (A == 0 AND B < 0)
	// - A > 0 means edge goes "up" in screen space (decreasing Y)
	// - A == 0 && B < 0 means horizontal edge going left (decreasing X)
	tests := []struct {
		name           string
		x0, y0, x1, y1 float32
		wantTopLeft    bool
	}{
		// Horizontal edge going right: A=0, B=10 > 0, NOT top-left
		{"horizontal_right", 0, 0, 10, 0, false},
		// Horizontal edge going left: A=0, B=-10 < 0, IS top-left
		{"horizontal_left", 10, 0, 0, 0, true},
		// Edge going up (Y decreasing): A > 0, IS top-left
		{"edge_up", 0, 10, 0, 0, true},
		// Edge going down (Y increasing): A < 0, NOT top-left
		{"edge_down", 0, 0, 0, 10, false},
		// Diagonal up-right: A=10 > 0, IS top-left
		{"diagonal_up_right", 0, 10, 10, 0, true},
		// Diagonal down-right: A=-10 < 0, NOT top-left
		{"diagonal_down_right", 0, 0, 10, 10, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := NewEdgeFunction(tt.x0, tt.y0, tt.x1, tt.y1)
			got := e.IsTopLeft()
			if got != tt.wantTopLeft {
				t.Errorf("IsTopLeft() = %v, want %v (A=%v, B=%v)", got, tt.wantTopLeft, e.A, e.B)
			}
		})
	}
}

// =============================================================================
// Rasterization Tests
// =============================================================================

func TestRasterizeSmallTriangle(t *testing.T) {
	// Small triangle: vertices at (2,2), (5,2), (3.5,5)
	tri := CreateScreenTriangle(
		2, 2, 0.5,
		5, 2, 0.5,
		3.5, 5, 0.5,
	)

	viewport := Viewport{X: 0, Y: 0, Width: 10, Height: 10, MinDepth: 0, MaxDepth: 1}

	fragments := make([]Fragment, 0)
	Rasterize(tri, viewport, func(frag Fragment) {
		fragments = append(fragments, frag)
	})

	// Should generate some fragments
	if len(fragments) == 0 {
		t.Error("Expected fragments for small triangle, got none")
	}

	// All fragments should be within bounding box
	for _, f := range fragments {
		if f.X < 2 || f.X > 5 || f.Y < 2 || f.Y > 5 {
			t.Errorf("Fragment (%d, %d) outside bounding box [2,5]x[2,5]", f.X, f.Y)
		}
	}

	// Barycentric coordinates should sum to approximately 1
	for _, f := range fragments {
		sum := f.Bary[0] + f.Bary[1] + f.Bary[2]
		if math.Abs(float64(sum-1.0)) > 0.01 {
			t.Errorf("Barycentric coordinates sum to %v, expected 1.0", sum)
		}
	}

	t.Logf("Generated %d fragments for small triangle", len(fragments))
}

func TestRasterizeLargeTriangle(t *testing.T) {
	// Large triangle covering most of a 100x100 area
	tri := CreateScreenTriangle(
		10, 10, 0.5,
		90, 10, 0.5,
		50, 90, 0.5,
	)

	viewport := Viewport{X: 0, Y: 0, Width: 100, Height: 100, MinDepth: 0, MaxDepth: 1}

	count := 0
	Rasterize(tri, viewport, func(frag Fragment) {
		count++
	})

	// Should generate many fragments
	minExpected := 1000 // Rough estimate for this triangle size
	if count < minExpected {
		t.Errorf("Expected at least %d fragments for large triangle, got %d", minExpected, count)
	}

	t.Logf("Generated %d fragments for large triangle", count)
}

func TestRasterizeClippedTriangle(t *testing.T) {
	// Triangle partially outside viewport
	tri := CreateScreenTriangle(
		-10, 5, 0.5, // Outside left
		20, 5, 0.5,
		5, 15, 0.5,
	)

	viewport := Viewport{X: 0, Y: 0, Width: 10, Height: 10, MinDepth: 0, MaxDepth: 1}

	fragments := make([]Fragment, 0)
	Rasterize(tri, viewport, func(frag Fragment) {
		fragments = append(fragments, frag)
	})

	// All fragments should be within viewport
	for _, f := range fragments {
		if f.X < 0 || f.X >= 10 || f.Y < 0 || f.Y >= 10 {
			t.Errorf("Fragment (%d, %d) outside viewport [0,10)x[0,10)", f.X, f.Y)
		}
	}

	t.Logf("Generated %d fragments for clipped triangle", len(fragments))
}

func TestRasterizeDegenerateTriangle(t *testing.T) {
	// Zero-area triangle (all points on a line)
	tri := CreateScreenTriangle(
		0, 0, 0.5,
		5, 5, 0.5,
		10, 10, 0.5,
	)

	viewport := Viewport{X: 0, Y: 0, Width: 20, Height: 20, MinDepth: 0, MaxDepth: 1}

	count := 0
	Rasterize(tri, viewport, func(frag Fragment) {
		count++
	})

	// Should generate zero fragments
	if count != 0 {
		t.Errorf("Expected 0 fragments for degenerate triangle, got %d", count)
	}
}

func TestRasterizeWithAttributes(t *testing.T) {
	// Triangle with color attributes
	red := [4]float32{1, 0, 0, 1}
	green := [4]float32{0, 1, 0, 1}
	blue := [4]float32{0, 0, 1, 1}

	tri := CreateScreenTriangleWithColor(
		10, 10, 0.5, red,
		30, 10, 0.5, green,
		20, 30, 0.5, blue,
	)

	viewport := Viewport{X: 0, Y: 0, Width: 40, Height: 40, MinDepth: 0, MaxDepth: 1}

	fragments := make([]Fragment, 0)
	Rasterize(tri, viewport, func(frag Fragment) {
		fragments = append(fragments, frag)
	})

	// All fragments should have 4 interpolated attributes
	for _, f := range fragments {
		if len(f.Attributes) != 4 {
			t.Errorf("Fragment at (%d, %d) has %d attributes, expected 4", f.X, f.Y, len(f.Attributes))
			continue
		}

		// Color values should be in valid range
		for i, v := range f.Attributes {
			if v < 0 || v > 1 {
				t.Errorf("Fragment at (%d, %d) attribute[%d] = %v, expected [0,1]", f.X, f.Y, i, v)
			}
		}
	}

	t.Logf("Generated %d fragments with color interpolation", len(fragments))
}

// =============================================================================
// Depth Buffer Tests
// =============================================================================

func TestDepthBufferCreation(t *testing.T) {
	db := NewDepthBuffer(100, 100)

	if db.Width() != 100 {
		t.Errorf("Width() = %d, want 100", db.Width())
	}
	if db.Height() != 100 {
		t.Errorf("Height() = %d, want 100", db.Height())
	}

	// All values should be initialized to 1.0
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			if got := db.Get(x, y); got != 1.0 {
				t.Errorf("Initial value at (%d, %d) = %v, want 1.0", x, y, got)
				return // Stop after first error
			}
		}
	}
}

func TestDepthBufferClear(t *testing.T) {
	db := NewDepthBuffer(10, 10)

	db.Clear(0.5)

	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			if got := db.Get(x, y); got != 0.5 {
				t.Errorf("After Clear(0.5), value at (%d, %d) = %v, want 0.5", x, y, got)
			}
		}
	}
}

func TestDepthBufferGetSet(t *testing.T) {
	db := NewDepthBuffer(10, 10)

	db.Set(5, 5, 0.3)
	if got := db.Get(5, 5); got != 0.3 {
		t.Errorf("Get(5, 5) = %v, want 0.3", got)
	}

	// Out of bounds should return 1.0
	if got := db.Get(-1, 0); got != 1.0 {
		t.Errorf("Get(-1, 0) = %v, want 1.0 for out of bounds", got)
	}
	if got := db.Get(100, 100); got != 1.0 {
		t.Errorf("Get(100, 100) = %v, want 1.0 for out of bounds", got)
	}
}

func TestDepthBufferTest(t *testing.T) {
	db := NewDepthBuffer(10, 10)
	db.Set(5, 5, 0.5)

	tests := []struct {
		name    string
		depth   float32
		compare CompareFunc
		want    bool
	}{
		{"less_pass", 0.3, CompareLess, true},
		{"less_fail", 0.7, CompareLess, false},
		{"less_equal_pass", 0.5, CompareLessEqual, true},
		{"greater_pass", 0.7, CompareGreater, true},
		{"greater_fail", 0.3, CompareGreater, false},
		{"equal_pass", 0.5, CompareEqual, true},
		{"equal_fail", 0.6, CompareEqual, false},
		{"always_pass", 0.9, CompareAlways, true},
		{"never_fail", 0.3, CompareNever, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := db.Test(5, 5, tt.depth, tt.compare)
			if got != tt.want {
				t.Errorf("Test(5, 5, %v, %v) = %v, want %v", tt.depth, tt.compare, got, tt.want)
			}
		})
	}
}

func TestDepthBufferTestAndSet(t *testing.T) {
	db := NewDepthBuffer(10, 10)
	db.Set(5, 5, 0.5)

	// Test that passes and writes
	if !db.TestAndSet(5, 5, 0.3, CompareLess, true) {
		t.Error("TestAndSet should pass for 0.3 < 0.5")
	}
	if got := db.Get(5, 5); got != 0.3 {
		t.Errorf("After TestAndSet, Get(5, 5) = %v, want 0.3", got)
	}

	// Test that fails (0.4 is not less than 0.3)
	if db.TestAndSet(5, 5, 0.4, CompareLess, true) {
		t.Error("TestAndSet should fail for 0.4 < 0.3")
	}
	if got := db.Get(5, 5); got != 0.3 {
		t.Errorf("After failed TestAndSet, Get(5, 5) = %v, should still be 0.3", got)
	}

	// Test that passes but doesn't write
	db.Set(5, 5, 0.5)
	if !db.TestAndSet(5, 5, 0.3, CompareLess, false) {
		t.Error("TestAndSet should pass for 0.3 < 0.5 (no write)")
	}
	if got := db.Get(5, 5); got != 0.5 {
		t.Errorf("After TestAndSet(write=false), Get(5, 5) = %v, should still be 0.5", got)
	}
}

// =============================================================================
// Pipeline Tests
// =============================================================================

func TestPipelineCreation(t *testing.T) {
	p := NewPipeline(100, 100)

	if p.Width() != 100 {
		t.Errorf("Width() = %d, want 100", p.Width())
	}
	if p.Height() != 100 {
		t.Errorf("Height() = %d, want 100", p.Height())
	}

	buf := p.GetColorBuffer()
	if len(buf) != 100*100*4 {
		t.Errorf("GetColorBuffer() len = %d, want %d", len(buf), 100*100*4)
	}
}

func TestPipelineClear(t *testing.T) {
	p := NewPipeline(10, 10)

	// Clear to red
	p.Clear(1, 0, 0, 1)

	r, g, b, a := p.GetPixel(5, 5)
	if r != 255 || g != 0 || b != 0 || a != 255 {
		t.Errorf("After Clear(red), GetPixel(5, 5) = (%d, %d, %d, %d), want (255, 0, 0, 255)", r, g, b, a)
	}

	// Clear to 50% gray with 50% alpha
	p.Clear(0.5, 0.5, 0.5, 0.5)

	r, g, b, a = p.GetPixel(5, 5)
	if r != 127 || g != 127 || b != 127 || a != 127 {
		t.Errorf("After Clear(gray), GetPixel(5, 5) = (%d, %d, %d, %d), want (127, 127, 127, 127)", r, g, b, a)
	}
}

func TestPipelineDrawTriangle(t *testing.T) {
	p := NewPipeline(100, 100)
	p.Clear(0, 0, 0, 1) // Black background

	// Draw a red triangle
	tri := CreateScreenTriangle(
		10, 10, 0.5,
		50, 10, 0.5,
		30, 50, 0.5,
	)

	p.DrawTriangles([]Triangle{tri}, [4]float32{1, 0, 0, 1})

	// Check center of triangle is red
	r, g, b, a := p.GetPixel(30, 25)
	if r != 255 || g != 0 || b != 0 || a != 255 {
		t.Errorf("Triangle center GetPixel(30, 25) = (%d, %d, %d, %d), want (255, 0, 0, 255)", r, g, b, a)
	}

	// Check outside triangle is black
	r, g, b, a = p.GetPixel(0, 0)
	if r != 0 || g != 0 || b != 0 || a != 255 {
		t.Errorf("Outside triangle GetPixel(0, 0) = (%d, %d, %d, %d), want (0, 0, 0, 255)", r, g, b, a)
	}
}

func TestPipelineMultipleTriangles(t *testing.T) {
	p := NewPipeline(100, 100)
	p.Clear(0, 0, 0, 1)
	p.SetDepthTest(true, CompareLess)
	p.ClearDepth(1.0)

	// Draw two triangles - red in front, blue behind
	triRed := CreateScreenTriangle(
		20, 20, 0.3, // Closer (smaller Z)
		60, 20, 0.3,
		40, 60, 0.3,
	)

	triBlue := CreateScreenTriangle(
		20, 20, 0.7, // Further (larger Z)
		60, 20, 0.7,
		40, 60, 0.7,
	)

	// Draw blue first, then red on top
	p.DrawTriangles([]Triangle{triBlue}, [4]float32{0, 0, 1, 1})
	p.DrawTriangles([]Triangle{triRed}, [4]float32{1, 0, 0, 1})

	// Center should be red (closer)
	r, g, b, _ := p.GetPixel(40, 35)
	if r != 255 || g != 0 || b != 0 {
		t.Errorf("With depth test, center = (%d, %d, %d), want red (255, 0, 0)", r, g, b)
	}

	// Reset and draw in opposite order
	p.Clear(0, 0, 0, 1)
	p.ClearDepth(1.0)
	p.DrawTriangles([]Triangle{triRed}, [4]float32{1, 0, 0, 1})
	p.DrawTriangles([]Triangle{triBlue}, [4]float32{0, 0, 1, 1})

	// Center should still be red (closer)
	r, g, b, _ = p.GetPixel(40, 35)
	if r != 255 || g != 0 || b != 0 {
		t.Errorf("With depth test (reverse order), center = (%d, %d, %d), want red (255, 0, 0)", r, g, b)
	}
}

func TestPipelineDepthWriteDisabled(t *testing.T) {
	p := NewPipeline(100, 100)
	p.Clear(0, 0, 0, 1)
	p.SetDepthTest(true, CompareLess)
	p.SetDepthWrite(false)
	p.ClearDepth(1.0)

	// Draw a triangle - should not update depth buffer
	tri := CreateScreenTriangle(
		20, 20, 0.5,
		60, 20, 0.5,
		40, 60, 0.5,
	)
	p.DrawTriangles([]Triangle{tri}, [4]float32{1, 0, 0, 1})

	// Depth at center should still be 1.0
	depth := p.GetDepthBuffer().Get(40, 35)
	if depth != 1.0 {
		t.Errorf("With depthWrite=false, depth at center = %v, want 1.0", depth)
	}
}

func TestPipelineCulling(t *testing.T) {
	p := NewPipeline(100, 100)

	// CCW triangle (front-facing by default)
	triCCW := CreateScreenTriangle(
		10, 10, 0.5,
		50, 10, 0.5,
		30, 50, 0.5,
	)

	// CW triangle (back-facing by default)
	triCW := CreateScreenTriangle(
		60, 10, 0.5,
		60, 50, 0.5,
		90, 30, 0.5,
	)

	// With back-face culling, only CCW should be drawn
	p.Clear(0, 0, 0, 1)
	p.SetCullMode(CullBack)
	p.DrawTriangles([]Triangle{triCCW}, [4]float32{1, 0, 0, 1})
	p.DrawTriangles([]Triangle{triCW}, [4]float32{0, 0, 1, 1})

	r, g, b, a := p.GetPixel(30, 25)
	_ = g
	_ = b
	_ = a
	if r != 255 {
		t.Error("CCW triangle should be drawn with back-face culling")
	}

	// With front-face culling, only CW should be drawn
	p.Clear(0, 0, 0, 1)
	p.SetCullMode(CullFront)
	p.DrawTriangles([]Triangle{triCCW}, [4]float32{1, 0, 0, 1})
	p.DrawTriangles([]Triangle{triCW}, [4]float32{0, 0, 1, 1})

	r, g, b, a = p.GetPixel(30, 25)
	_ = g
	_ = b
	_ = a
	if r != 0 {
		t.Error("CCW triangle should NOT be drawn with front-face culling")
	}
}

func TestPipelineInterpolatedColors(t *testing.T) {
	p := NewPipeline(100, 100)
	p.Clear(0, 0, 0, 1)

	// Triangle with red, green, blue corners
	red := [4]float32{1, 0, 0, 1}
	green := [4]float32{0, 1, 0, 1}
	blue := [4]float32{0, 0, 1, 1}

	tri := CreateScreenTriangleWithColor(
		10, 10, 0.5, red,
		90, 10, 0.5, green,
		50, 90, 0.5, blue,
	)

	p.DrawTrianglesInterpolated([]Triangle{tri})

	// Center should be a mix of all three colors
	r, g, b, _ := p.GetPixel(50, 35)

	// All channels should be present (mixed color)
	if r == 0 && g == 0 && b == 0 {
		t.Error("Center pixel should have interpolated color, got black")
	}

	// Near red corner should be mostly red
	r, g, b, _ = p.GetPixel(15, 12)
	if r == 0 {
		t.Error("Near red corner should have red component")
	}

	t.Logf("Center color: (%d, %d, %d)", r, g, b)
}

func TestPipelineResize(t *testing.T) {
	p := NewPipeline(100, 100)
	p.Clear(1, 0, 0, 1) // Red

	p.Resize(50, 50)

	if p.Width() != 50 || p.Height() != 50 {
		t.Errorf("After Resize, dimensions = (%d, %d), want (50, 50)", p.Width(), p.Height())
	}

	buf := p.GetColorBuffer()
	if len(buf) != 50*50*4 {
		t.Errorf("After Resize, buffer len = %d, want %d", len(buf), 50*50*4)
	}

	// Buffer should be cleared (black)
	r, g, b, a := p.GetPixel(25, 25)
	if r != 0 || g != 0 || b != 0 || a != 0 {
		t.Errorf("After Resize, pixel should be zeroed, got (%d, %d, %d, %d)", r, g, b, a)
	}
}

// =============================================================================
// Culling Tests
// =============================================================================

func TestShouldCull(t *testing.T) {
	// CCW triangle
	triCCW := CreateScreenTriangle(
		0, 0, 0.5,
		10, 0, 0.5,
		5, 10, 0.5,
	)

	// CW triangle
	triCW := CreateScreenTriangle(
		0, 0, 0.5,
		5, 10, 0.5,
		10, 0, 0.5,
	)

	tests := []struct {
		name      string
		tri       Triangle
		cullMode  CullMode
		frontFace FrontFace
		want      bool
	}{
		{"ccw_cull_none", triCCW, CullNone, FrontFaceCCW, false},
		{"ccw_cull_back", triCCW, CullBack, FrontFaceCCW, false},
		{"ccw_cull_front", triCCW, CullFront, FrontFaceCCW, true},
		{"cw_cull_none", triCW, CullNone, FrontFaceCCW, false},
		{"cw_cull_back", triCW, CullBack, FrontFaceCCW, true},
		{"cw_cull_front", triCW, CullFront, FrontFaceCCW, false},
		// With CW as front face
		{"ccw_cw_front_cull_back", triCCW, CullBack, FrontFaceCW, true},
		{"cw_cw_front_cull_back", triCW, CullBack, FrontFaceCW, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ShouldCull(tt.tri, tt.cullMode, tt.frontFace)
			if got != tt.want {
				t.Errorf("ShouldCull() = %v, want %v", got, tt.want)
			}
		})
	}
}

// =============================================================================
// Benchmarks
// =============================================================================

func BenchmarkRasterize10x10Triangle(b *testing.B) {
	tri := CreateScreenTriangle(
		0, 0, 0.5,
		10, 0, 0.5,
		5, 10, 0.5,
	)
	viewport := Viewport{X: 0, Y: 0, Width: 20, Height: 20, MinDepth: 0, MaxDepth: 1}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Rasterize(tri, viewport, func(frag Fragment) {
			// Just count
		})
	}
}

func BenchmarkRasterize100x100Triangle(b *testing.B) {
	tri := CreateScreenTriangle(
		0, 0, 0.5,
		100, 0, 0.5,
		50, 100, 0.5,
	)
	viewport := Viewport{X: 0, Y: 0, Width: 120, Height: 120, MinDepth: 0, MaxDepth: 1}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Rasterize(tri, viewport, func(frag Fragment) {
			// Just count
		})
	}
}

func BenchmarkRasterize1000Triangles(b *testing.B) {
	// Create 1000 small triangles
	triangles := make([]Triangle, 1000)
	for i := range triangles {
		x := float32(i % 100)
		y := float32(i / 100)
		triangles[i] = CreateScreenTriangle(
			x*5, y*5, 0.5,
			x*5+5, y*5, 0.5,
			x*5+2.5, y*5+5, 0.5,
		)
	}
	viewport := Viewport{X: 0, Y: 0, Width: 500, Height: 100, MinDepth: 0, MaxDepth: 1}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j := range triangles {
			Rasterize(triangles[j], viewport, func(frag Fragment) {
				// Just count
			})
		}
	}
}

func BenchmarkPipelineDrawTriangles(b *testing.B) {
	p := NewPipeline(800, 600)

	triangles := make([]Triangle, 100)
	for i := range triangles {
		x := float32(i%10) * 80
		y := float32(i/10) * 60
		triangles[i] = CreateScreenTriangle(
			x, y, 0.5,
			x+70, y, 0.5,
			x+35, y+50, 0.5,
		)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p.Clear(0, 0, 0, 1)
		p.DrawTriangles(triangles, [4]float32{1, 0, 0, 1})
	}
}

func BenchmarkDepthBufferTestAndSet(b *testing.B) {
	db := NewDepthBuffer(800, 600)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		x := i % 800
		y := (i / 800) % 600
		db.TestAndSet(x, y, 0.5, CompareLess, true)
	}
}

// =============================================================================
// Helper Functions
// =============================================================================

func sign(v float32) int {
	if v > 0 {
		return 1
	}
	if v < 0 {
		return -1
	}
	return 0
}
