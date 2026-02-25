package raster

import (
	"math"
	"testing"
)

// =============================================================================
// IncrementalEdge Tests
// =============================================================================

func TestIncrementalEdgeCreation(t *testing.T) {
	// Create edge from (0, 0) to (10, 0)
	e := NewEdgeFunction(0, 0, 10, 0)
	ie := NewIncrementalEdge(e)

	if ie.A != e.A || ie.B != e.B || ie.C != e.C {
		t.Errorf("IncrementalEdge coefficients don't match EdgeFunction")
	}
}

func TestIncrementalEdgeFromPoints(t *testing.T) {
	ie := NewIncrementalEdgeFromPoints(0, 0, 10, 0)
	e := NewEdgeFunction(0, 0, 10, 0)

	if ie.A != e.A || ie.B != e.B || ie.C != e.C {
		t.Errorf("NewIncrementalEdgeFromPoints: A=%v, B=%v, C=%v, want A=%v, B=%v, C=%v",
			ie.A, ie.B, ie.C, e.A, e.B, e.C)
	}
}

func TestIncrementalEdgeSetRow(t *testing.T) {
	ie := NewIncrementalEdgeFromPoints(0, 0, 10, 0)
	ie.SetRow(5, 5)

	expected := ie.A*5 + ie.B*5 + ie.C
	if ie.Value() != expected {
		t.Errorf("After SetRow(5, 5): Value() = %v, want %v", ie.Value(), expected)
	}
}

func TestIncrementalEdgeStepX(t *testing.T) {
	ie := NewIncrementalEdgeFromPoints(0, 0, 10, 5)
	ie.SetRow(0, 0)

	initial := ie.Value()
	ie.StepX()

	expected := initial + ie.A
	if ie.Value() != expected {
		t.Errorf("After StepX: Value() = %v, want %v", ie.Value(), expected)
	}
}

func TestIncrementalEdgeNextRow(t *testing.T) {
	ie := NewIncrementalEdgeFromPoints(0, 0, 10, 5)
	ie.SetRow(0, 0)

	initial := ie.Value()

	// Step right a few times
	ie.StepX()
	ie.StepX()

	// Move to next row
	ie.NextRow()

	// Value should be initial + B (back to x=0, y=1)
	expected := initial + ie.B
	if ie.Value() != expected {
		t.Errorf("After NextRow: Value() = %v, want %v", ie.Value(), expected)
	}
}

func TestIncrementalEdgeMatchesStandard(t *testing.T) {
	// Create edge and incremental version
	e := NewEdgeFunction(0, 0, 10, 10)
	ie := NewIncrementalEdge(e)

	// Test at various points
	testPoints := [][2]float32{
		{0.5, 0.5},
		{5.5, 5.5},
		{10.5, 10.5},
		{0.5, 10.5},
		{10.5, 0.5},
	}

	for _, pt := range testPoints {
		standard := e.Evaluate(pt[0], pt[1])
		ie.SetRow(pt[0], pt[1])
		incremental := ie.Value()

		if math.Abs(float64(standard-incremental)) > 1e-6 {
			t.Errorf("At (%v, %v): standard=%v, incremental=%v",
				pt[0], pt[1], standard, incremental)
		}
	}
}

func TestIncrementalEdgeIsTopLeft(t *testing.T) {
	tests := []struct {
		name           string
		x0, y0, x1, y1 float32
		want           bool
	}{
		{"horizontal_right", 0, 0, 10, 0, false},
		{"horizontal_left", 10, 0, 0, 0, true},
		{"edge_up", 0, 10, 0, 0, true},
		{"edge_down", 0, 0, 0, 10, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ie := NewIncrementalEdgeFromPoints(tt.x0, tt.y0, tt.x1, tt.y1)
			if got := ie.IsTopLeft(); got != tt.want {
				t.Errorf("IsTopLeft() = %v, want %v", got, tt.want)
			}
		})
	}
}

// =============================================================================
// IncrementalTriangle Tests
// =============================================================================

func TestIncrementalTriangleCreation(t *testing.T) {
	tri := CreateScreenTriangle(
		0, 0, 0.5,
		10, 0, 0.5,
		5, 10, 0.5,
	)

	incTri := NewIncrementalTriangle(tri)

	if incTri.IsDegenerate() {
		t.Error("Triangle should not be degenerate")
	}

	area := incTri.Area()
	if area <= 0 {
		t.Errorf("CCW triangle should have positive area, got %v", area)
	}
}

func TestIncrementalTriangleDegenerate(t *testing.T) {
	// Collinear points
	tri := CreateScreenTriangle(
		0, 0, 0.5,
		5, 5, 0.5,
		10, 10, 0.5,
	)

	incTri := NewIncrementalTriangle(tri)

	if !incTri.IsDegenerate() {
		t.Error("Collinear triangle should be degenerate")
	}
}

func TestIncrementalTriangleIsInside(t *testing.T) {
	tri := CreateScreenTriangle(
		10, 10, 0.5,
		30, 10, 0.5,
		20, 30, 0.5,
	)

	incTri := NewIncrementalTriangle(tri)

	tests := []struct {
		name       string
		x, y       float32
		wantInside bool
	}{
		{"center", 20, 17, true},
		{"outside_left", 5, 15, false},
		{"outside_right", 35, 15, false},
		{"outside_top", 20, 5, false},
		{"outside_bottom", 20, 35, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			incTri.SetRow(tt.x+0.5, tt.y+0.5)
			got := incTri.IsInside()
			if got != tt.wantInside {
				t.Errorf("IsInside at (%v, %v) = %v, want %v",
					tt.x, tt.y, got, tt.wantInside)
			}
		})
	}
}

func TestIncrementalTriangleBarycentric(t *testing.T) {
	tri := CreateScreenTriangle(
		0, 0, 0.5,
		10, 0, 0.5,
		5, 10, 0.5,
	)

	incTri := NewIncrementalTriangle(tri)

	// Test at vertices
	vertices := [][2]float32{
		{0.5, 0.5}, // Near V0
		{9.5, 0.5}, // Near V1
		{5.0, 9.5}, // Near V2
		{5.0, 3.5}, // Center-ish
	}

	for _, v := range vertices {
		incTri.SetRow(v[0], v[1])
		if incTri.IsInside() {
			b0, b1, b2 := incTri.Barycentric()
			sum := b0 + b1 + b2

			if math.Abs(float64(sum-1.0)) > 0.1 {
				t.Errorf("At (%v, %v): barycentric sum = %v, want ~1.0",
					v[0], v[1], sum)
			}
		}
	}
}

func TestIncrementalTriangleStepX(t *testing.T) {
	tri := CreateScreenTriangle(
		0, 0, 0.5,
		20, 0, 0.5,
		10, 20, 0.5,
	)

	incTri := NewIncrementalTriangle(tri)
	incTri.SetRow(5.5, 5.5)

	// Get initial values
	w0Initial, w1Initial, w2Initial := incTri.EdgeValues()

	incTri.StepX()

	w0After, w1After, w2After := incTri.EdgeValues()

	// Each edge value should increase by its A coefficient
	expectedW0 := w0Initial + incTri.E12.A
	expectedW1 := w1Initial + incTri.E20.A
	expectedW2 := w2Initial + incTri.E01.A

	if math.Abs(float64(w0After-expectedW0)) > 1e-6 {
		t.Errorf("E12 after StepX: got %v, want %v", w0After, expectedW0)
	}
	if math.Abs(float64(w1After-expectedW1)) > 1e-6 {
		t.Errorf("E20 after StepX: got %v, want %v", w1After, expectedW1)
	}
	if math.Abs(float64(w2After-expectedW2)) > 1e-6 {
		t.Errorf("E01 after StepX: got %v, want %v", w2After, expectedW2)
	}
}

func TestIncrementalTriangleNextRow(t *testing.T) {
	tri := CreateScreenTriangle(
		0, 0, 0.5,
		20, 0, 0.5,
		10, 20, 0.5,
	)

	incTri := NewIncrementalTriangle(tri)
	incTri.SetRow(5.5, 5.5)

	w0Initial, w1Initial, w2Initial := incTri.EdgeValues()

	// Step right then next row
	incTri.StepX()
	incTri.StepX()
	incTri.NextRow()

	w0After, w1After, w2After := incTri.EdgeValues()

	// After NextRow, we should be at (5.5, 6.5) - x reset, y+1
	expectedW0 := w0Initial + incTri.E12.B
	expectedW1 := w1Initial + incTri.E20.B
	expectedW2 := w2Initial + incTri.E01.B

	if math.Abs(float64(w0After-expectedW0)) > 1e-6 {
		t.Errorf("E12 after NextRow: got %v, want %v", w0After, expectedW0)
	}
	if math.Abs(float64(w1After-expectedW1)) > 1e-6 {
		t.Errorf("E20 after NextRow: got %v, want %v", w1After, expectedW1)
	}
	if math.Abs(float64(w2After-expectedW2)) > 1e-6 {
		t.Errorf("E01 after NextRow: got %v, want %v", w2After, expectedW2)
	}
}

func TestIncrementalTriangleCWWinding(t *testing.T) {
	// CW triangle (vertices in clockwise order)
	tri := CreateScreenTriangle(
		10, 10, 0.5,
		20, 30, 0.5, // Swapped from CCW
		30, 10, 0.5,
	)

	incTri := NewIncrementalTriangle(tri)

	if incTri.IsDegenerate() {
		t.Error("CW triangle should not be degenerate")
	}

	area := incTri.Area()
	if area >= 0 {
		t.Errorf("CW triangle should have negative area, got %v", area)
	}

	// Test that point inside works correctly
	incTri.SetRow(20, 18) // Center point
	if !incTri.IsInside() {
		t.Error("Center of CW triangle should be inside")
	}
}

// =============================================================================
// RasterizeIncremental Tests
// =============================================================================

func TestRasterizeIncrementalMatchesStandard(t *testing.T) {
	tri := CreateScreenTriangle(
		10, 10, 0.5,
		50, 10, 0.5,
		30, 50, 0.5,
	)
	viewport := Viewport{X: 0, Y: 0, Width: 100, Height: 100, MinDepth: 0, MaxDepth: 1}

	// Rasterize with standard method
	standardFragments := make(map[[2]int]Fragment)
	Rasterize(tri, viewport, func(frag Fragment) {
		standardFragments[[2]int{frag.X, frag.Y}] = frag
	})

	// Rasterize with incremental method
	incrementalFragments := make(map[[2]int]Fragment)
	RasterizeIncremental(tri, viewport, func(frag Fragment) {
		incrementalFragments[[2]int{frag.X, frag.Y}] = frag
	})

	// Compare results
	if len(standardFragments) != len(incrementalFragments) {
		t.Errorf("Fragment count: standard=%d, incremental=%d",
			len(standardFragments), len(incrementalFragments))
	}

	// Check that all standard fragments exist in incremental
	for key, stdFrag := range standardFragments {
		incFrag, ok := incrementalFragments[key]
		if !ok {
			t.Errorf("Missing fragment at (%d, %d)", key[0], key[1])
			continue
		}

		// Compare depth
		if math.Abs(float64(stdFrag.Depth-incFrag.Depth)) > 1e-5 {
			t.Errorf("Depth mismatch at (%d, %d): standard=%v, incremental=%v",
				key[0], key[1], stdFrag.Depth, incFrag.Depth)
		}

		// Compare barycentric (with tolerance)
		for i := 0; i < 3; i++ {
			if math.Abs(float64(stdFrag.Bary[i]-incFrag.Bary[i])) > 0.01 {
				t.Errorf("Bary[%d] mismatch at (%d, %d): standard=%v, incremental=%v",
					i, key[0], key[1], stdFrag.Bary[i], incFrag.Bary[i])
			}
		}
	}
}

func TestRasterizeIncrementalWithAttributes(t *testing.T) {
	red := [4]float32{1, 0, 0, 1}
	green := [4]float32{0, 1, 0, 1}
	blue := [4]float32{0, 0, 1, 1}

	tri := CreateScreenTriangleWithColor(
		10, 10, 0.5, red,
		50, 10, 0.5, green,
		30, 50, 0.5, blue,
	)
	viewport := Viewport{X: 0, Y: 0, Width: 100, Height: 100, MinDepth: 0, MaxDepth: 1}

	fragmentCount := 0
	RasterizeIncremental(tri, viewport, func(frag Fragment) {
		fragmentCount++

		if len(frag.Attributes) != 4 {
			t.Errorf("Expected 4 attributes, got %d", len(frag.Attributes))
		}

		// Check color values are in valid range
		for i, v := range frag.Attributes {
			if v < 0 || v > 1 {
				t.Errorf("Attribute[%d] = %v, out of range [0, 1]", i, v)
			}
		}
	})

	if fragmentCount == 0 {
		t.Error("Expected fragments to be generated")
	}
}

func TestRasterizeIncrementalDegenerate(t *testing.T) {
	tri := CreateScreenTriangle(
		0, 0, 0.5,
		5, 5, 0.5,
		10, 10, 0.5, // Collinear
	)
	viewport := Viewport{X: 0, Y: 0, Width: 100, Height: 100, MinDepth: 0, MaxDepth: 1}

	count := 0
	RasterizeIncremental(tri, viewport, func(frag Fragment) {
		count++
	})

	if count != 0 {
		t.Errorf("Degenerate triangle should produce no fragments, got %d", count)
	}
}

func TestRasterizeIncrementalClipped(t *testing.T) {
	// Triangle partially outside viewport
	tri := CreateScreenTriangle(
		-10, 10, 0.5,
		50, 10, 0.5,
		20, 50, 0.5,
	)
	viewport := Viewport{X: 0, Y: 0, Width: 40, Height: 40, MinDepth: 0, MaxDepth: 1}

	RasterizeIncremental(tri, viewport, func(frag Fragment) {
		if frag.X < 0 || frag.X >= 40 || frag.Y < 0 || frag.Y >= 40 {
			t.Errorf("Fragment at (%d, %d) outside viewport", frag.X, frag.Y)
		}
	})
}

// =============================================================================
// RasterizeTile Tests
// =============================================================================

func TestRasterizeTile(t *testing.T) {
	tri := CreateScreenTriangle(
		0, 0, 0.5,
		20, 0, 0.5,
		10, 20, 0.5,
	)

	tile := Tile{
		X:    0,
		Y:    0,
		MinX: 0,
		MinY: 0,
		MaxX: 8,
		MaxY: 8,
	}

	fragments := make([]Fragment, 0)
	RasterizeTile(tri, tile, func(frag Fragment) {
		fragments = append(fragments, frag)
	})

	// Should have some fragments
	if len(fragments) == 0 {
		t.Error("Expected fragments in tile")
	}

	// All fragments should be within tile
	for _, frag := range fragments {
		if frag.X < tile.MinX || frag.X >= tile.MaxX ||
			frag.Y < tile.MinY || frag.Y >= tile.MaxY {
			t.Errorf("Fragment at (%d, %d) outside tile bounds", frag.X, frag.Y)
		}
	}
}

func TestRasterizeTileNoOverlap(t *testing.T) {
	tri := CreateScreenTriangle(
		50, 50, 0.5,
		60, 50, 0.5,
		55, 60, 0.5,
	)

	tile := Tile{
		X:    0,
		Y:    0,
		MinX: 0,
		MinY: 0,
		MaxX: 8,
		MaxY: 8,
	}

	count := 0
	RasterizeTile(tri, tile, func(frag Fragment) {
		count++
	})

	if count != 0 {
		t.Errorf("Expected no fragments for non-overlapping tile, got %d", count)
	}
}

// =============================================================================
// Benchmarks
// =============================================================================

func BenchmarkIncrementalEdgeStep(b *testing.B) {
	ie := NewIncrementalEdgeFromPoints(0, 0, 100, 100)
	ie.SetRow(0, 0)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ie.StepX()
		if i%100 == 99 {
			ie.NextRow()
		}
	}
}

func BenchmarkIncrementalTriangleIsInside(b *testing.B) {
	tri := CreateScreenTriangle(
		0, 0, 0.5,
		100, 0, 0.5,
		50, 100, 0.5,
	)
	incTri := NewIncrementalTriangle(tri)
	incTri.SetRow(50, 50)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = incTri.IsInside()
		incTri.StepX()
	}
}

func BenchmarkIncrementalVsStandard10x10(b *testing.B) {
	tri := CreateScreenTriangle(
		0, 0, 0.5,
		10, 0, 0.5,
		5, 10, 0.5,
	)
	viewport := Viewport{X: 0, Y: 0, Width: 20, Height: 20, MinDepth: 0, MaxDepth: 1}

	b.Run("Standard", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			Rasterize(tri, viewport, func(frag Fragment) {})
		}
	})

	b.Run("Incremental", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			RasterizeIncremental(tri, viewport, func(frag Fragment) {})
		}
	})
}

func BenchmarkIncrementalVsStandard100x100(b *testing.B) {
	tri := CreateScreenTriangle(
		0, 0, 0.5,
		100, 0, 0.5,
		50, 100, 0.5,
	)
	viewport := Viewport{X: 0, Y: 0, Width: 120, Height: 120, MinDepth: 0, MaxDepth: 1}

	b.Run("Standard", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			Rasterize(tri, viewport, func(frag Fragment) {})
		}
	})

	b.Run("Incremental", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			RasterizeIncremental(tri, viewport, func(frag Fragment) {})
		}
	})
}

func BenchmarkRasterizeTile(b *testing.B) {
	tri := CreateScreenTriangle(
		0, 0, 0.5,
		32, 0, 0.5,
		16, 32, 0.5,
	)
	tile := Tile{X: 1, Y: 1, MinX: 8, MinY: 8, MaxX: 16, MaxY: 16}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		RasterizeTile(tri, tile, func(frag Fragment) {})
	}
}
