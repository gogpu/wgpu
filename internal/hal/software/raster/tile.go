//go:build !(js && wasm)

package raster

// TileSize is the size of each tile in pixels (8x8).
// This value balances work distribution with cache efficiency.
const TileSize = 8

// Tile represents a rectangular region of the framebuffer.
// Tiles are used for work distribution in parallel rasterization.
type Tile struct {
	// X is the tile coordinate (not pixel coordinate).
	X int

	// Y is the tile coordinate (not pixel coordinate).
	Y int

	// MinX is the left pixel boundary (inclusive).
	MinX int

	// MinY is the top pixel boundary (inclusive).
	MinY int

	// MaxX is the right pixel boundary (exclusive).
	MaxX int

	// MaxY is the bottom pixel boundary (exclusive).
	MaxY int
}

// TileCorners holds the four corner pixel coordinates of a tile.
// Used for hierarchical tile-triangle overlap testing.
type TileCorners struct {
	// TL is the top-left corner (MinX, MinY).
	TL [2]float32

	// TR is the top-right corner (MaxX-1, MinY).
	TR [2]float32

	// BL is the bottom-left corner (MinX, MaxY-1).
	BL [2]float32

	// BR is the bottom-right corner (MaxX-1, MaxY-1).
	BR [2]float32
}

// Corners returns the four corner pixels of the tile.
// Corner coordinates are at pixel centers (x+0.5, y+0.5).
func (t Tile) Corners() TileCorners {
	return TileCorners{
		TL: [2]float32{float32(t.MinX) + 0.5, float32(t.MinY) + 0.5},
		TR: [2]float32{float32(t.MaxX-1) + 0.5, float32(t.MinY) + 0.5},
		BL: [2]float32{float32(t.MinX) + 0.5, float32(t.MaxY-1) + 0.5},
		BR: [2]float32{float32(t.MaxX-1) + 0.5, float32(t.MaxY-1) + 0.5},
	}
}

// Width returns the width of the tile in pixels.
func (t Tile) Width() int {
	return t.MaxX - t.MinX
}

// Height returns the height of the tile in pixels.
func (t Tile) Height() int {
	return t.MaxY - t.MinY
}

// TileGrid manages tiles for the framebuffer.
// It divides the framebuffer into a grid of tiles for parallel processing.
type TileGrid struct {
	tiles  []Tile
	tilesX int
	tilesY int
	width  int
	height int
}

// NewTileGrid creates a new tile grid for the given framebuffer dimensions.
// The grid is divided into TileSize x TileSize tiles.
func NewTileGrid(width, height int) *TileGrid {
	tilesX := (width + TileSize - 1) / TileSize
	tilesY := (height + TileSize - 1) / TileSize

	tiles := make([]Tile, tilesX*tilesY)

	for ty := 0; ty < tilesY; ty++ {
		for tx := 0; tx < tilesX; tx++ {
			idx := ty*tilesX + tx
			minX := tx * TileSize
			minY := ty * TileSize
			maxX := minInt(minX+TileSize, width)
			maxY := minInt(minY+TileSize, height)

			tiles[idx] = Tile{
				X:    tx,
				Y:    ty,
				MinX: minX,
				MinY: minY,
				MaxX: maxX,
				MaxY: maxY,
			}
		}
	}

	return &TileGrid{
		tiles:  tiles,
		tilesX: tilesX,
		tilesY: tilesY,
		width:  width,
		height: height,
	}
}

// TileCount returns the total number of tiles in the grid.
func (g *TileGrid) TileCount() int {
	return len(g.tiles)
}

// TilesX returns the number of tiles in the horizontal direction.
func (g *TileGrid) TilesX() int {
	return g.tilesX
}

// TilesY returns the number of tiles in the vertical direction.
func (g *TileGrid) TilesY() int {
	return g.tilesY
}

// GetTile returns the tile at grid coordinates (tx, ty).
// Returns an empty tile if coordinates are out of bounds.
func (g *TileGrid) GetTile(tx, ty int) Tile {
	if tx < 0 || tx >= g.tilesX || ty < 0 || ty >= g.tilesY {
		return Tile{}
	}
	return g.tiles[ty*g.tilesX+tx]
}

// GetTileAt returns the tile grid coordinates containing pixel (px, py).
func (g *TileGrid) GetTileAt(px, py int) (tx, ty int) {
	tx = px / TileSize
	ty = py / TileSize
	return tx, ty
}

// GetTilesForRect returns all tiles that overlap with the given pixel rectangle.
// The rectangle is defined by [minX, maxX) x [minY, maxY).
func (g *TileGrid) GetTilesForRect(minX, minY, maxX, maxY int) []Tile {
	// Clamp to framebuffer bounds
	if minX < 0 {
		minX = 0
	}
	if minY < 0 {
		minY = 0
	}
	if maxX > g.width {
		maxX = g.width
	}
	if maxY > g.height {
		maxY = g.height
	}

	// Early exit if empty rect
	if minX >= maxX || minY >= maxY {
		return nil
	}

	// Compute tile range
	startTX := minX / TileSize
	startTY := minY / TileSize
	endTX := (maxX - 1) / TileSize
	endTY := (maxY - 1) / TileSize

	// Clamp to tile grid bounds
	if endTX >= g.tilesX {
		endTX = g.tilesX - 1
	}
	if endTY >= g.tilesY {
		endTY = g.tilesY - 1
	}

	// Collect overlapping tiles
	result := make([]Tile, 0, (endTX-startTX+1)*(endTY-startTY+1))
	for ty := startTY; ty <= endTY; ty++ {
		for tx := startTX; tx <= endTX; tx++ {
			result = append(result, g.tiles[ty*g.tilesX+tx])
		}
	}

	return result
}

// GetTilesForTriangle returns tiles overlapping a screen-space triangle.
// It computes the bounding box of the triangle and returns all tiles in that region.
func (g *TileGrid) GetTilesForTriangle(tri Triangle) []Tile {
	// Compute triangle bounding box
	minX := int(min3(tri.V0.X, tri.V1.X, tri.V2.X))
	maxX := int(max3(tri.V0.X, tri.V1.X, tri.V2.X)) + 1
	minY := int(min3(tri.V0.Y, tri.V1.Y, tri.V2.Y))
	maxY := int(max3(tri.V0.Y, tri.V1.Y, tri.V2.Y)) + 1

	return g.GetTilesForRect(minX, minY, maxX, maxY)
}

// TileTriangleTest tests if a tile potentially overlaps with a triangle.
// It evaluates the triangle's fixed-point edge functions at the centers of the
// tile's corner pixels, with the same top-left fill rule as the rasterizer.
//
// Returns:
//   - -1: no pixel of the tile is covered (reject)
//   - 0: tile partially overlaps the triangle (needs per-pixel testing)
//   - 1: every pixel of the tile is covered (can skip edge tests for all pixels)
func TileTriangleTest(tile Tile, tri Triangle) int {
	it := NewIncrementalTriangle(tri)
	return tileTriangleTest(tile, &it)
}

// tileTriangleTest is TileTriangleTest for an already set up triangle.
//
// An edge value is linear and the corner centers span every pixel center of
// the tile, so if all four corners fail one edge's test, every pixel fails it
// (all values < 0, or all <= 0 on an edge that does not own its points), and
// if all four pass every edge's test, every pixel passes.
func tileTriangleTest(tile Tile, it *IncrementalTriangle) int {
	if it.IsDegenerate() || tile.MinX >= tile.MaxX || tile.MinY >= tile.MaxY {
		return -1
	}
	xs := [2]int64{pixelCenter(tile.MinX), pixelCenter(tile.MaxX - 1)}
	ys := [2]int64{pixelCenter(tile.MinY), pixelCenter(tile.MaxY - 1)}

	result := 1
	for _, e := range [3]*IncrementalEdge{&it.E12, &it.E20, &it.E01} {
		passed := 0
		for _, y := range ys {
			for _, x := range xs {
				if e.covers(e.evaluate(x, y)) {
					passed++
				}
			}
		}
		if passed == 0 {
			return -1 // Completely outside this edge
		}
		if passed < 4 {
			result = 0 // Partial overlap
		}
	}
	return result
}

// TileIndex returns the linear index of a tile in the grid.
func (g *TileGrid) TileIndex(tx, ty int) int {
	return ty*g.tilesX + tx
}

// GetAllTiles returns all tiles in the grid.
// The tiles are ordered row by row (Y-major order).
func (g *TileGrid) GetAllTiles() []Tile {
	result := make([]Tile, len(g.tiles))
	copy(result, g.tiles)
	return result
}
