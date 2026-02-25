package raster

import (
	"testing"
)

// =============================================================================
// Tile Tests
// =============================================================================

func TestTileBasic(t *testing.T) {
	tile := Tile{
		X:    0,
		Y:    0,
		MinX: 0,
		MinY: 0,
		MaxX: 8,
		MaxY: 8,
	}

	if tile.Width() != 8 {
		t.Errorf("Width() = %d, want 8", tile.Width())
	}
	if tile.Height() != 8 {
		t.Errorf("Height() = %d, want 8", tile.Height())
	}
}

func TestTileCorners(t *testing.T) {
	tile := Tile{
		X:    0,
		Y:    0,
		MinX: 0,
		MinY: 0,
		MaxX: 8,
		MaxY: 8,
	}

	corners := tile.Corners()

	// Top-left at pixel center (0.5, 0.5)
	if corners.TL[0] != 0.5 || corners.TL[1] != 0.5 {
		t.Errorf("TL = (%v, %v), want (0.5, 0.5)", corners.TL[0], corners.TL[1])
	}

	// Top-right at pixel center (7.5, 0.5)
	if corners.TR[0] != 7.5 || corners.TR[1] != 0.5 {
		t.Errorf("TR = (%v, %v), want (7.5, 0.5)", corners.TR[0], corners.TR[1])
	}

	// Bottom-left at pixel center (0.5, 7.5)
	if corners.BL[0] != 0.5 || corners.BL[1] != 7.5 {
		t.Errorf("BL = (%v, %v), want (0.5, 7.5)", corners.BL[0], corners.BL[1])
	}

	// Bottom-right at pixel center (7.5, 7.5)
	if corners.BR[0] != 7.5 || corners.BR[1] != 7.5 {
		t.Errorf("BR = (%v, %v), want (7.5, 7.5)", corners.BR[0], corners.BR[1])
	}
}

// =============================================================================
// TileGrid Tests
// =============================================================================

func TestTileGridCreation(t *testing.T) {
	tests := []struct {
		name       string
		width      int
		height     int
		wantTilesX int
		wantTilesY int
		wantTotal  int
	}{
		{"exact_fit", 16, 16, 2, 2, 4},
		{"partial_fit", 20, 20, 3, 3, 9},
		{"single_tile", 5, 5, 1, 1, 1},
		{"wide", 100, 8, 13, 1, 13},
		{"tall", 8, 100, 1, 13, 13},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			grid := NewTileGrid(tt.width, tt.height)

			if grid.TilesX() != tt.wantTilesX {
				t.Errorf("TilesX() = %d, want %d", grid.TilesX(), tt.wantTilesX)
			}
			if grid.TilesY() != tt.wantTilesY {
				t.Errorf("TilesY() = %d, want %d", grid.TilesY(), tt.wantTilesY)
			}
			if grid.TileCount() != tt.wantTotal {
				t.Errorf("TileCount() = %d, want %d", grid.TileCount(), tt.wantTotal)
			}
		})
	}
}

func TestTileGridGetTile(t *testing.T) {
	grid := NewTileGrid(24, 24) // 3x3 tiles

	// Get first tile
	tile := grid.GetTile(0, 0)
	if tile.MinX != 0 || tile.MinY != 0 {
		t.Errorf("Tile(0,0) min = (%d, %d), want (0, 0)", tile.MinX, tile.MinY)
	}
	if tile.MaxX != 8 || tile.MaxY != 8 {
		t.Errorf("Tile(0,0) max = (%d, %d), want (8, 8)", tile.MaxX, tile.MaxY)
	}

	// Get middle tile
	tile = grid.GetTile(1, 1)
	if tile.MinX != 8 || tile.MinY != 8 {
		t.Errorf("Tile(1,1) min = (%d, %d), want (8, 8)", tile.MinX, tile.MinY)
	}
	if tile.MaxX != 16 || tile.MaxY != 16 {
		t.Errorf("Tile(1,1) max = (%d, %d), want (16, 16)", tile.MaxX, tile.MaxY)
	}

	// Out of bounds
	tile = grid.GetTile(-1, 0)
	if tile.MaxX != 0 {
		t.Error("Out of bounds tile should be empty")
	}
}

func TestTileGridGetTileAt(t *testing.T) {
	grid := NewTileGrid(24, 24)

	tests := []struct {
		px, py int
		wantTX int
		wantTY int
	}{
		{0, 0, 0, 0},
		{7, 7, 0, 0},
		{8, 8, 1, 1},
		{15, 15, 1, 1},
		{16, 16, 2, 2},
		{23, 23, 2, 2},
	}

	for _, tt := range tests {
		gotTX, gotTY := grid.GetTileAt(tt.px, tt.py)
		if gotTX != tt.wantTX || gotTY != tt.wantTY {
			t.Errorf("GetTileAt(%d, %d) = (%d, %d), want (%d, %d)",
				tt.px, tt.py, gotTX, gotTY, tt.wantTX, tt.wantTY)
		}
	}
}

func TestTileGridGetTilesForRect(t *testing.T) {
	grid := NewTileGrid(32, 32) // 4x4 tiles

	tests := []struct {
		name       string
		minX, minY int
		maxX, maxY int
		wantCount  int
	}{
		{"single_tile", 0, 0, 8, 8, 1},
		{"four_tiles", 0, 0, 16, 16, 4},
		{"partial_overlap", 4, 4, 12, 12, 4},
		{"full_grid", 0, 0, 32, 32, 16},
		{"empty_rect", 0, 0, 0, 0, 0},
		{"outside_bounds", 100, 100, 200, 200, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tiles := grid.GetTilesForRect(tt.minX, tt.minY, tt.maxX, tt.maxY)
			if len(tiles) != tt.wantCount {
				t.Errorf("GetTilesForRect(%d, %d, %d, %d) returned %d tiles, want %d",
					tt.minX, tt.minY, tt.maxX, tt.maxY, len(tiles), tt.wantCount)
			}
		})
	}
}

func TestTileGridGetTilesForTriangle(t *testing.T) {
	grid := NewTileGrid(32, 32)

	// Small triangle within one tile
	smallTri := CreateScreenTriangle(
		2, 2, 0.5,
		6, 2, 0.5,
		4, 6, 0.5,
	)
	tiles := grid.GetTilesForTriangle(smallTri)
	if len(tiles) != 1 {
		t.Errorf("Small triangle: got %d tiles, want 1", len(tiles))
	}

	// Triangle spanning multiple tiles
	largeTri := CreateScreenTriangle(
		4, 4, 0.5,
		24, 4, 0.5,
		14, 24, 0.5,
	)
	tiles = grid.GetTilesForTriangle(largeTri)
	if len(tiles) < 4 {
		t.Errorf("Large triangle: got %d tiles, want >= 4", len(tiles))
	}
}

// =============================================================================
// TileTriangleTest Tests
// =============================================================================

func TestTileTriangleTest(t *testing.T) {
	// Create a triangle at (10, 10), (30, 10), (20, 30)
	tri := CreateScreenTriangle(
		10, 10, 0.5,
		30, 10, 0.5,
		20, 30, 0.5,
	)

	e01 := NewEdgeFunction(tri.V0.X, tri.V0.Y, tri.V1.X, tri.V1.Y)
	e12 := NewEdgeFunction(tri.V1.X, tri.V1.Y, tri.V2.X, tri.V2.Y)
	e20 := NewEdgeFunction(tri.V2.X, tri.V2.Y, tri.V0.X, tri.V0.Y)

	// Tile completely outside (to the left)
	outsideTile := Tile{X: 0, Y: 0, MinX: 0, MinY: 0, MaxX: 8, MaxY: 8}
	result := TileTriangleTest(outsideTile, e01, e12, e20)
	if result != -1 {
		t.Errorf("Outside tile: got %d, want -1 (reject)", result)
	}

	// Tile partially inside
	partialTile := Tile{X: 1, Y: 1, MinX: 8, MinY: 8, MaxX: 16, MaxY: 16}
	result = TileTriangleTest(partialTile, e01, e12, e20)
	if result != 0 {
		t.Errorf("Partial tile: got %d, want 0 (partial)", result)
	}

	// Tile inside triangle (small tile in center of large triangle)
	// Need a large triangle for this test
	bigTri := CreateScreenTriangle(
		0, 0, 0.5,
		100, 0, 0.5,
		50, 100, 0.5,
	)
	e01Big := NewEdgeFunction(bigTri.V0.X, bigTri.V0.Y, bigTri.V1.X, bigTri.V1.Y)
	e12Big := NewEdgeFunction(bigTri.V1.X, bigTri.V1.Y, bigTri.V2.X, bigTri.V2.Y)
	e20Big := NewEdgeFunction(bigTri.V2.X, bigTri.V2.Y, bigTri.V0.X, bigTri.V0.Y)

	insideTile := Tile{X: 5, Y: 5, MinX: 40, MinY: 40, MaxX: 48, MaxY: 48}
	result = TileTriangleTest(insideTile, e01Big, e12Big, e20Big)
	if result != 1 {
		t.Errorf("Inside tile: got %d, want 1 (accept)", result)
	}
}

// =============================================================================
// TileGrid Edge Cases
// =============================================================================

func TestTileGridEdgeTiles(t *testing.T) {
	// Grid that doesn't divide evenly by TileSize
	grid := NewTileGrid(20, 20) // 3x3 tiles, last row/column is 4 pixels

	// Check last tile has correct size
	lastTile := grid.GetTile(2, 2)
	if lastTile.Width() != 4 || lastTile.Height() != 4 {
		t.Errorf("Last tile size = (%d, %d), want (4, 4)",
			lastTile.Width(), lastTile.Height())
	}

	// Check first tile has correct size
	firstTile := grid.GetTile(0, 0)
	if firstTile.Width() != 8 || firstTile.Height() != 8 {
		t.Errorf("First tile size = (%d, %d), want (8, 8)",
			firstTile.Width(), firstTile.Height())
	}
}

func TestTileGridGetAllTiles(t *testing.T) {
	grid := NewTileGrid(16, 16)

	tiles := grid.GetAllTiles()
	if len(tiles) != 4 {
		t.Errorf("GetAllTiles() returned %d tiles, want 4", len(tiles))
	}

	// Verify tiles are ordered correctly (row-major)
	expectedOrder := [][2]int{{0, 0}, {1, 0}, {0, 1}, {1, 1}}
	for i, tile := range tiles {
		if tile.X != expectedOrder[i][0] || tile.Y != expectedOrder[i][1] {
			t.Errorf("Tile %d: (%d, %d), want (%d, %d)",
				i, tile.X, tile.Y, expectedOrder[i][0], expectedOrder[i][1])
		}
	}
}

func TestTileIndex(t *testing.T) {
	grid := NewTileGrid(24, 24) // 3x3 tiles

	tests := []struct {
		tx, ty    int
		wantIndex int
	}{
		{0, 0, 0},
		{1, 0, 1},
		{2, 0, 2},
		{0, 1, 3},
		{1, 1, 4},
		{2, 2, 8},
	}

	for _, tt := range tests {
		got := grid.TileIndex(tt.tx, tt.ty)
		if got != tt.wantIndex {
			t.Errorf("TileIndex(%d, %d) = %d, want %d",
				tt.tx, tt.ty, got, tt.wantIndex)
		}
	}
}

// =============================================================================
// Benchmarks
// =============================================================================

func BenchmarkNewTileGrid(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = NewTileGrid(800, 600)
	}
}

func BenchmarkTileGridGetTilesForRect(b *testing.B) {
	grid := NewTileGrid(800, 600)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = grid.GetTilesForRect(100, 100, 500, 400)
	}
}

func BenchmarkTileGridGetTilesForTriangle(b *testing.B) {
	grid := NewTileGrid(800, 600)
	tri := CreateScreenTriangle(
		100, 100, 0.5,
		500, 100, 0.5,
		300, 400, 0.5,
	)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = grid.GetTilesForTriangle(tri)
	}
}

func BenchmarkTileTriangleTest(b *testing.B) {
	tile := Tile{X: 5, Y: 5, MinX: 40, MinY: 40, MaxX: 48, MaxY: 48}
	tri := CreateScreenTriangle(
		0, 0, 0.5,
		100, 0, 0.5,
		50, 100, 0.5,
	)
	e01 := NewEdgeFunction(tri.V0.X, tri.V0.Y, tri.V1.X, tri.V1.Y)
	e12 := NewEdgeFunction(tri.V1.X, tri.V1.Y, tri.V2.X, tri.V2.Y)
	e20 := NewEdgeFunction(tri.V2.X, tri.V2.Y, tri.V0.X, tri.V0.Y)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = TileTriangleTest(tile, e01, e12, e20)
	}
}
