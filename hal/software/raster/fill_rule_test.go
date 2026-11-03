//go:build !(js && wasm)

package raster

import (
	"fmt"
	"math"
	"math/rand"
	"testing"
)

// Fill rule tests: the D3D11/D3D12 rasterization rules, which Vulkan and
// WebGPU implementations follow. A pixel center is covered when it lies
// strictly inside all three edges, or exactly on an edge that is a top edge
// (exactly horizontal, the triangle below it) or a left edge (not horizontal,
// the triangle to its right). Consequence checked here: in any tiling, every
// pixel center is covered by exactly one triangle, whatever the winding.

const fillW, fillH = 32, 32

// fillPath runs one rasterization entry point over a list of triangles and
// returns how many fragments each pixel received.
type fillPath struct {
	name string
	run  func(tris []Triangle) []int
}

func fillViewport() Viewport {
	return Viewport{X: 0, Y: 0, Width: fillW, Height: fillH, MinDepth: 0, MaxDepth: 1}
}

func countInto(counts []int) RasterCallback {
	return func(f Fragment) {
		if f.X >= 0 && f.X < fillW && f.Y >= 0 && f.Y < fillH {
			counts[f.Y*fillW+f.X]++
		}
	}
}

func fillPaths() []fillPath {
	return []fillPath{
		{"Rasterize", func(tris []Triangle) []int {
			counts := make([]int, fillW*fillH)
			for _, tri := range tris {
				Rasterize(tri, fillViewport(), countInto(counts))
			}
			return counts
		}},
		{"RasterizeIncremental", func(tris []Triangle) []int {
			counts := make([]int, fillW*fillH)
			for _, tri := range tris {
				RasterizeIncremental(tri, fillViewport(), countInto(counts))
			}
			return counts
		}},
		{"RasterizeTile", func(tris []Triangle) []int {
			counts := make([]int, fillW*fillH)
			tiles := NewTileGrid(fillW, fillH).GetAllTiles()
			for _, tri := range tris {
				for _, tile := range tiles {
					RasterizeTile(tri, tile, countInto(counts))
				}
			}
			return counts
		}},
		{"BinnedTiles", func(tris []Triangle) []int {
			// The binning that DrawTrianglesParallel's tiles rely on must never
			// reject a tile that holds a covered pixel.
			counts := make([]int, fillW*fillH)
			grid := NewTileGrid(fillW, fillH)
			tiles := grid.GetAllTiles()
			for idx, binned := range BinTrianglesToTilesWithTest(tris, grid) {
				for _, tri := range binned {
					RasterizeTile(tri, tiles[idx], countInto(counts))
				}
			}
			return counts
		}},
	}
}

// fillTri builds a flat triangle from three screen points.
func fillTri(x0, y0, x1, y1, x2, y2 float32) Triangle {
	return CreateScreenTriangle(x0, y0, 0.5, x1, y1, 0.5, x2, y2, 0.5)
}

// quadTris splits the axis-aligned rectangle [x0,x1]x[y0,y1] into two
// triangles along the top-left to bottom-right diagonal (tlbr) or the
// top-right to bottom-left one.
func quadTris(x0, y0, x1, y1 float32, tlbr bool) []Triangle {
	if tlbr {
		return []Triangle{
			fillTri(x0, y0, x1, y0, x1, y1),
			fillTri(x0, y0, x1, y1, x0, y1),
		}
	}
	return []Triangle{
		fillTri(x0, y0, x1, y0, x0, y1),
		fillTri(x1, y0, x1, y1, x0, y1),
	}
}

// fanTris builds a closed triangle fan around center c through the ring.
func fanTris(cx, cy float32, ring [][2]float32) []Triangle {
	tris := make([]Triangle, 0, len(ring))
	for i := range ring {
		a, b := ring[i], ring[(i+1)%len(ring)]
		tris = append(tris, fillTri(cx, cy, a[0], a[1], b[0], b[1]))
	}
	return tris
}

// meshTris triangulates a grid of points that extends past the viewport on
// every side, so the mesh covers every pixel of the viewport. jitter moves
// each grid point by a fixed random sub-pixel offset; shared vertices are the
// same float32 values in every triangle that uses them, as in an index buffer.
// jitter must stay below step/4 or cells can fold over; the mesh is checked
// to be a tiling (every triangle keeps the grid's orientation).
func meshTris(t testing.TB, origin, step float32, n int, jitter float32, seed int64) []Triangle {
	t.Helper()
	if jitter >= step/4 {
		t.Fatalf("meshTris: jitter %v >= step/4 (%v) can fold the mesh", jitter, step/4)
	}
	rng := rand.New(rand.NewSource(seed))
	pts := make([][2]float32, (n+1)*(n+1))
	for j := 0; j <= n; j++ {
		for i := 0; i <= n; i++ {
			x := origin + float32(i)*step
			y := origin + float32(j)*step
			if jitter != 0 {
				x += (rng.Float32()*2 - 1) * jitter
				y += (rng.Float32()*2 - 1) * jitter
			}
			pts[j*(n+1)+i] = [2]float32{x, y}
		}
	}
	tris := make([]Triangle, 0, 2*n*n)
	for j := 0; j < n; j++ {
		for i := 0; i < n; i++ {
			p00 := pts[j*(n+1)+i]
			p10 := pts[j*(n+1)+i+1]
			p01 := pts[(j+1)*(n+1)+i]
			p11 := pts[(j+1)*(n+1)+i+1]
			if rng.Intn(2) == 0 {
				tris = append(tris,
					fillTri(p00[0], p00[1], p10[0], p10[1], p11[0], p11[1]),
					fillTri(p00[0], p00[1], p11[0], p11[1], p01[0], p01[1]))
			} else {
				tris = append(tris,
					fillTri(p00[0], p00[1], p10[0], p10[1], p01[0], p01[1]),
					fillTri(p10[0], p10[1], p11[0], p11[1], p01[0], p01[1]))
			}
		}
	}
	for i, tri := range tris {
		if ComputeTriangleArea(tri.V0, tri.V1, tri.V2) <= 0 {
			t.Fatalf("meshTris: triangle %d is inverted, the mesh is not a tiling", i)
		}
	}
	return tris
}

// windings returns the tiling as given, with every triangle reversed, and
// with every other triangle reversed: coverage must not depend on winding.
func windings(tris []Triangle) map[string][]Triangle {
	reversed := make([]Triangle, len(tris))
	mixed := make([]Triangle, len(tris))
	for i, tri := range tris {
		r := Triangle{V0: tri.V0, V1: tri.V2, V2: tri.V1}
		reversed[i] = r
		mixed[i] = tri
		if i%2 == 1 {
			mixed[i] = r
		}
	}
	return map[string][]Triangle{"given": tris, "reversed": reversed, "mixed": mixed}
}

// rectMask expects exactly one fragment on pixels [x0,x1)x[y0,y1), none
// elsewhere.
func rectMask(x0, y0, x1, y1 int) func(x, y int) int {
	return func(x, y int) int {
		if x >= x0 && x < x1 && y >= y0 && y < y1 {
			return 1
		}
		return 0
	}
}

// checkCoverage reports every pixel whose fragment count differs from want.
func checkCoverage(t *testing.T, counts []int, want func(x, y int) int) {
	t.Helper()
	bad := 0
	for y := 0; y < fillH; y++ {
		for x := 0; x < fillW; x++ {
			if got, w := counts[y*fillW+x], want(x, y); got != w {
				if bad < 8 {
					t.Errorf("pixel (%d,%d): %d fragments, want %d", x, y, got, w)
				}
				bad++
			}
		}
	}
	if bad > 8 {
		t.Errorf("... %d pixels wrong in total", bad)
	}
}

// TestFillRuleTilings rasterizes tilings whose shared edges and vertices lie
// exactly on pixel centers and checks that every pixel is drawn exactly once,
// on every rasterization path and for both windings.
func TestFillRuleTilings(t *testing.T) {
	cases := []struct {
		name string
		tris []Triangle
		want func(x, y int) int
	}{
		// Integer vertex coordinates: the outer edges miss every pixel center,
		// the diagonal goes through the centers (i+0.5, i+0.5).
		{"quad-diagonal-tlbr", quadTris(8, 8, 24, 24, true), rectMask(8, 8, 24, 24)},
		{"quad-diagonal-trbl", quadTris(8, 8, 24, 24, false), rectMask(8, 8, 24, 24)},
		// Half-integer coordinates: every edge, outer and shared, goes through
		// pixel centers. Top and left outer edges are drawn, bottom and right
		// ones are not, so the covered pixels are [2,18) and [2,20).
		{"shared-vertical-edge", append(
			quadTris(2.5, 2.5, 10.5, 20.5, true),
			quadTris(10.5, 2.5, 18.5, 20.5, false)...), rectMask(2, 2, 18, 20)},
		{"shared-horizontal-edge", append(
			quadTris(2.5, 2.5, 20.5, 10.5, false),
			quadTris(2.5, 10.5, 20.5, 18.5, true)...), rectMask(2, 2, 20, 18)},
		// A pixel center exactly on the vertex shared by eight triangles, with
		// horizontal, vertical and diagonal spokes through pixel centers.
		{"fan-axis-spokes", fanTris(16.5, 16.5, [][2]float32{
			{8.5, 8.5}, {16.5, 8.5}, {24.5, 8.5}, {24.5, 16.5},
			{24.5, 24.5}, {16.5, 24.5}, {8.5, 24.5}, {8.5, 16.5},
		}), rectMask(8, 8, 24, 24)},
		// Meshes covering the whole viewport: every pixel exactly once.
		{"mesh-pixel-centers", meshTris(t, -7.5, 4, 12, 0, 1), rectMask(0, 0, fillW, fillH)},
		{"mesh-jittered", meshTris(t, -8, 5.3, 9, 1.3, 2), rectMask(0, 0, fillW, fillH)},
		{"mesh-jittered-fine", meshTris(t, -4, 1.7, 24, 0.42, 3), rectMask(0, 0, fillW, fillH)},
	}
	for _, c := range cases {
		for wname, tris := range windings(c.tris) {
			for _, path := range fillPaths() {
				t.Run(fmt.Sprintf("%s/%s/%s", c.name, wname, path.name), func(t *testing.T) {
					checkCoverage(t, path.run(tris), c.want)
				})
			}
		}
	}
}

// TestFillRuleIrregularFan checks a fan whose spokes have arbitrary slopes
// around a center vertex that sits exactly on a pixel center: that pixel is
// drawn by exactly one triangle and no pixel is drawn twice.
func TestFillRuleIrregularFan(t *testing.T) {
	const cx, cy = 16.5, 16.5
	tris := fanTris(cx, cy, [][2]float32{
		{5.25, 3.75}, {16.5, 2.5}, {29.1, 6.3}, {30.5, 16.5},
		{27.3, 28.9}, {16.5, 30.5}, {4.7, 27.2}, {2.5, 16.5},
	})
	for wname, wtris := range windings(tris) {
		for _, path := range fillPaths() {
			t.Run(wname+"/"+path.name, func(t *testing.T) {
				counts := path.run(wtris)
				if got := counts[16*fillW+16]; got != 1 {
					t.Errorf("center pixel (16,16): %d fragments, want 1", got)
				}
				for i, n := range counts {
					if n > 1 {
						t.Errorf("pixel (%d,%d): %d fragments, want at most 1", i%fillW, i/fillW, n)
					}
				}
			})
		}
	}
}

// TestFillRuleDegenerate checks that zero-area triangles cover nothing, even
// when they lie exactly on pixel centers.
func TestFillRuleDegenerate(t *testing.T) {
	cases := map[string]Triangle{
		"collinear-on-centers": fillTri(2.5, 2.5, 6.5, 6.5, 10.5, 10.5),
		"collinear-horizontal": fillTri(2.5, 4.5, 12.5, 4.5, 20.5, 4.5),
		"two-equal-vertices":   fillTri(3.5, 3.5, 3.5, 3.5, 12.5, 9.5),
		"point-on-center":      fillTri(7.5, 7.5, 7.5, 7.5, 7.5, 7.5),
	}
	for name, tri := range cases {
		for wname, tris := range windings([]Triangle{tri}) {
			for _, path := range fillPaths() {
				t.Run(name+"/"+wname+"/"+path.name, func(t *testing.T) {
					checkCoverage(t, path.run(tris), func(int, int) int { return 0 })
				})
			}
		}
	}
}

// TestFillRuleSnapRange checks the fixed-point range: a triangle with a vertex
// that cannot be snapped covers nothing, and one with vertices near the ±2^21
// pixel limit still covers the viewport exactly, without int64 overflow.
func TestFillRuleSnapRange(t *testing.T) {
	inf := float32(math.Inf(1))
	nan := float32(math.NaN())
	none := func(int, int) int { return 0 }
	all := rectMask(0, 0, fillW, fillH)
	cases := []struct {
		name string
		tri  Triangle
		want func(x, y int) int
	}{
		{"nan-vertex", fillTri(nan, 0, 30, 0, 0, 30), none},
		{"inf-vertex", fillTri(0, 0, inf, 0, 0, 30), none},
		{"beyond-range", fillTri(-1, -1, 3e6, -1, -1, 40), none},
		// Hypotenuse x + y = 90000, far beyond the viewport.
		{"near-range-limit", fillTri(-2e6, -2e6, 2.09e6, -2e6, -2e6, 2.09e6), all},
	}
	for _, c := range cases {
		for wname, tris := range windings([]Triangle{c.tri}) {
			for _, path := range fillPaths() {
				t.Run(c.name+"/"+wname+"/"+path.name, func(t *testing.T) {
					checkCoverage(t, path.run(tris), c.want)
				})
			}
		}
	}
}

// TestFillRuleBlendedQuad is the reported case at the pipeline level: a quad
// drawn as two triangles with premultiplied alpha 0.5 over an opaque clear.
// Every pixel of the quad must be blended once (100 + 0.5*40 = 120); a pixel
// drawn by both triangles would read 100 + 0.5*120 = 160.
func TestFillRuleBlendedQuad(t *testing.T) {
	src := [4]float32{100.0 / 255, 100.0 / 255, 100.0 / 255, 0.5}
	for _, parallel := range []bool{false, true} {
		for _, tlbr := range []bool{true, false} {
			t.Run(fmt.Sprintf("parallel=%v/tlbr=%v", parallel, tlbr), func(t *testing.T) {
				p := NewPipeline(fillW, fillH)
				defer p.Close()
				p.Clear(40.0/255, 40.0/255, 40.0/255, 1)
				p.SetBlendState(BlendPremultiplied)
				tris := quadTris(8, 8, 24, 24, tlbr)
				if parallel {
					p.EnableParallel(true)
					p.DrawTrianglesParallel(tris, src)
				} else {
					p.DrawTriangles(tris, src)
				}
				for y := 0; y < fillH; y++ {
					for x := 0; x < fillW; x++ {
						r, _, _, a := p.GetPixel(x, y)
						want := 40
						if x >= 8 && x < 24 && y >= 8 && y < 24 {
							want = 120
						}
						if d := int(r) - want; d < -1 || d > 1 || a != 255 {
							t.Errorf("pixel (%d,%d) = (r %d, a %d), want (r %d±1, a 255)", x, y, r, a, want)
						}
					}
				}
			})
		}
	}
}
