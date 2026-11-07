//go:build !(js && wasm)

package raster

import (
	"slices"
	"testing"
)

// BenchmarkQuadRasterizer rasterizes a 256x256 triangle with four attributes
// in quads; with one QuadRasterizer reused it allocates nothing per quad.
func BenchmarkQuadRasterizer(b *testing.B) {
	attrs := func(v float32) []float32 { return []float32{v, v, v, v} }
	tri := Triangle{
		V0: ScreenVertex{X: 0, Y: 0, W: 1, Attributes: attrs(0)},
		V1: ScreenVertex{X: 256, Y: 0, W: 1, Attributes: attrs(1)},
		V2: ScreenVertex{X: 0, Y: 256, W: 1, Attributes: attrs(2)},
	}
	viewport := Viewport{Width: 256, Height: 256}
	var r QuadRasterizer
	var sum float32
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		r.Rasterize(&tri, viewport, func(q *Quad) { sum += q.Frags[0].Attributes[0] })
	}
	_ = sum
}

// TestQuadRasterizerMatchesRasterize checks that QuadRasterizer covers
// exactly the pixels Rasterize produces, each with the same depth, 1/w,
// barycentrics and attributes, and that every quad is aligned to even
// coordinates. The triangles have odd and fractional bounds, a viewport that
// starts on an odd pixel, and shared edges, so the top-left rule and the
// alignment of partial quads are exercised.
func TestQuadRasterizerMatchesRasterize(t *testing.T) {
	vtx := func(x, y, z, w float32, attrs ...float32) ScreenVertex {
		return ScreenVertex{X: x, Y: y, Z: z, W: w, Attributes: attrs}
	}
	tests := []struct {
		name     string
		tri      Triangle
		viewport Viewport
	}{
		{"diagonal half", Triangle{V0: vtx(0, 0, 0, 1, 1, 2), V1: vtx(8, 0, 0.5, 1, 3, 4), V2: vtx(0, 8, 1, 1, 5, 6)}, Viewport{Width: 8, Height: 8}},
		{"other half (shared edge)", Triangle{V0: vtx(8, 0, 0, 1, 1, 2), V1: vtx(8, 8, 0.5, 1, 3, 4), V2: vtx(0, 8, 1, 1, 5, 6)}, Viewport{Width: 8, Height: 8}},
		{"fractional, perspective", Triangle{V0: vtx(1.3, 0.7, 0.1, 0.5, 1, 0, 0), V1: vtx(14.6, 3.2, 0.4, 1, 0, 1, 0), V2: vtx(5.1, 12.9, 0.9, 0.25, 0, 0, 1)}, Viewport{Width: 16, Height: 16}},
		{"odd viewport origin", Triangle{V0: vtx(0, 0, 0, 1, 7), V1: vtx(16, 1, 0, 1, 8), V2: vtx(3, 15, 0, 1, 9)}, Viewport{X: 3, Y: 5, Width: 9, Height: 7}},
		{"flat attribute", Triangle{V0: vtx(0, 0, 0, 1, 1, 2), V1: vtx(9, 2, 0, 1, 3, 4), V2: vtx(2, 9, 0, 1, 5, 6), Flat: 1}, Viewport{Width: 10, Height: 10}},
		{"no attributes", Triangle{V0: vtx(0.5, 0.5, 0, 1), V1: vtx(6.5, 1.5, 0, 1), V2: vtx(2.5, 6.5, 0, 1)}, Viewport{Width: 8, Height: 8}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			want := map[[2]int]Fragment{}
			Rasterize(tc.tri, tc.viewport, func(frag Fragment) {
				want[[2]int{frag.X, frag.Y}] = frag
			})
			if len(want) == 0 {
				t.Fatal("Rasterize produced no fragment")
			}

			got := map[[2]int]bool{}
			var r QuadRasterizer
			r.Rasterize(&tc.tri, tc.viewport, func(q *Quad) {
				if q.X&1 != 0 || q.Y&1 != 0 {
					t.Errorf("quad at (%d, %d) is not aligned to even coordinates", q.X, q.Y)
				}
				for lane := range QuadLanes {
					frag := q.Frags[lane]
					if frag.X != q.X+lane&1 || frag.Y != q.Y+lane>>1 {
						t.Errorf("lane %d of quad (%d, %d) is pixel (%d, %d)", lane, q.X, q.Y, frag.X, frag.Y)
					}
					if q.Covered&(1<<lane) == 0 {
						continue
					}
					key := [2]int{frag.X, frag.Y}
					if got[key] {
						t.Errorf("pixel %v covered twice", key)
					}
					got[key] = true
					w, ok := want[key]
					switch {
					case !ok:
						t.Errorf("pixel %v covered by the quad path only", key)
					case frag.Depth != w.Depth || frag.InvW != w.InvW || frag.Bary != w.Bary ||
						!slices.Equal(frag.Attributes, w.Attributes):
						t.Errorf("pixel %v: quad fragment %+v, want %+v", key, frag, w)
					}
				}
			})
			for key := range want {
				if !got[key] {
					t.Errorf("pixel %v covered by Rasterize only", key)
				}
			}
		})
	}
}
