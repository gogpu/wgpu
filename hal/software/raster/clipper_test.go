//go:build !(js && wasm)

package raster

import (
	"math"
	"testing"
)

// clipperTestViewport is an 8x8 target's full viewport.
var clipperTestViewport = FullViewport(8, 8)

func newTestClipper(flat uint64) Clipper {
	return NewClipper(&clipperTestViewport, 8, 8, flat)
}

func clipVertex(x, y, z, w float32, attrs ...float32) ClipSpaceVertex {
	return ClipSpaceVertex{Position: [4]float32{x, y, z, w}, Attributes: attrs}
}

// ndcArea returns twice the signed area of the polygon after the divide.
func ndcArea(poly []ClipSpaceVertex) float64 {
	var a float64
	for i := range poly {
		p, q := poly[i].Position, poly[(i+1)%len(poly)].Position
		a += float64(p[0]/p[3])*float64(q[1]/q[3]) - float64(q[0]/q[3])*float64(p[1]/p[3])
	}
	return a
}

func TestClipperTrivialAccept(t *testing.T) {
	c := newTestClipper(0)
	v0 := clipVertex(-0.5, -0.5, 0.5, 1, 1, 2)
	v1 := clipVertex(0.5, -0.5, 0.5, 1, 3, 4)
	v2 := clipVertex(0, 0.5, 0.5, 1, 5, 6)
	poly := c.Clip(&v0, &v1, &v2)
	if len(poly) != 3 {
		t.Fatalf("Clip returned %d vertices, want the triangle's 3", len(poly))
	}
	for i, v := range []*ClipSpaceVertex{&v0, &v1, &v2} {
		if poly[i].Position != v.Position || &poly[i].Attributes[0] != &v.Attributes[0] {
			t.Errorf("vertex %d = %+v, want the input vertex with its attribute slice", i, poly[i])
		}
	}
	if n := testing.AllocsPerRun(100, func() { c.Clip(&v0, &v1, &v2) }); n != 0 {
		t.Errorf("Clip of a triangle that needs no clipping allocated %v times", n)
	}
}

func TestClipperReject(t *testing.T) {
	c := newTestClipper(0)
	behind := [3]ClipSpaceVertex{clipVertex(0, 0, -1, -1), clipVertex(1, 0, -1, -1), clipVertex(0, 1, -1, -1)}
	if poly := c.Clip(&behind[0], &behind[1], &behind[2]); len(poly) != 0 {
		t.Errorf("a triangle behind the eye clipped to %d vertices, want none", len(poly))
	}
	beyondFar := [3]ClipSpaceVertex{clipVertex(0, 0, 2, 1), clipVertex(1, 0, 2, 1), clipVertex(0, 1, 2, 1)}
	if poly := c.Clip(&beyondFar[0], &beyondFar[1], &beyondFar[2]); len(poly) != 0 {
		t.Errorf("a triangle beyond the far plane clipped to %d vertices, want none", len(poly))
	}
	empty := ViewportTransform{X: 1, Y: 1, Width: 0, Height: 4, MaxDepth: 1}
	c = NewClipper(&empty, 8, 8, 0)
	inside := [3]ClipSpaceVertex{clipVertex(0, 0, 0.5, 1), clipVertex(1, 0, 0.5, 1), clipVertex(0, 1, 0.5, 1)}
	if poly := c.Clip(&inside[0], &inside[1], &inside[2]); len(poly) != 0 {
		t.Errorf("a viewport without area kept %d vertices, want none", len(poly))
	}
}

// TestClipperNearPlane cuts a triangle with one vertex in front of the near
// plane into a quad whose new vertices interpolate in clip space.
func TestClipperNearPlane(t *testing.T) {
	c := newTestClipper(0)
	v0 := clipVertex(-0.5, -0.5, 0.5, 1, 0)
	v1 := clipVertex(0.5, -0.5, 0.5, 1, 1)
	v2 := clipVertex(0, 0.5, -0.5, 1, 2) // z < 0
	poly := c.Clip(&v0, &v1, &v2)
	if len(poly) != 4 {
		t.Fatalf("Clip returned %d vertices, want 4", len(poly))
	}
	if poly[0].Position != v0.Position {
		t.Errorf("first vertex = %v, want v0 %v", poly[0].Position, v0.Position)
	}
	for i, v := range poly {
		if v.Position[2] < 0 {
			t.Errorf("vertex %d z = %g, want >= 0", i, v.Position[2])
		}
	}
	// The edge v1 -> v2 is cut halfway (z goes 0.5 -> -0.5): attribute 1.5.
	if got := poly[2]; got.Position != [4]float32{0.25, 0, 0, 1} || got.Attributes[0] != 1.5 {
		t.Errorf("vertex on v1-v2 = %v %v, want [0.25 0 0 1] [1.5]", got.Position, got.Attributes)
	}
	if ndcArea(poly) <= 0 {
		t.Errorf("the clipped polygon's winding flipped: area %g", ndcArea(poly))
	}
}

// TestClipperBehindEye clips a triangle whose provoking vertex is behind the
// eye: every vertex left has w > 0, the winding of the visible part is kept,
// and the polygon starts with a created vertex that carries v0's flat lane
// bit for bit.
func TestClipperBehindEye(t *testing.T) {
	flatBits := math.Float32frombits(0x7fc01234) // a NaN payload: must not go through arithmetic
	c := newTestClipper(1 << 1)
	v0 := clipVertex(-0.2, 3, -0.5, -1, 7, flatBits)
	v1 := clipVertex(-0.5, -1, 0.5, 1, 0, 1)
	v2 := clipVertex(0.8, -1, 0.5, 1, 0, 2)
	poly := c.Clip(&v0, &v1, &v2)
	if len(poly) < 3 {
		t.Fatalf("Clip returned %d vertices, want the visible part", len(poly))
	}
	for i, v := range poly {
		if !(v.Position[3] > 0) {
			t.Errorf("vertex %d w = %g, want > 0", i, v.Position[3])
		}
		if z := v.Position[2]; z < -1e-6 || z > v.Position[3]+1e-6 {
			t.Errorf("vertex %d z = %g outside [0, w]", i, z)
		}
	}
	if got := math.Float32bits(poly[0].Attributes[1]); got != 0x7fc01234 {
		t.Errorf("first vertex flat lane = %#x, want v0's %#x", got, 0x7fc01234)
	}
	// The visible part winds as the triangle's homogeneous orientation, the
	// sign of det(x, y, w) of its vertices: a created vertex a*vi + b*v0 with
	// a, b > 0 keeps that determinant's sign, whatever the divide does to v0.
	det := func(p, q, r [4]float32) float64 {
		return float64(p[0])*(float64(q[1])*float64(r[3])-float64(q[3])*float64(r[1])) -
			float64(p[1])*(float64(q[0])*float64(r[3])-float64(q[3])*float64(r[0])) +
			float64(p[3])*(float64(q[0])*float64(r[1])-float64(q[1])*float64(r[0]))
	}
	if ref := det(v0.Position, v1.Position, v2.Position); ndcArea(poly)*ref <= 0 {
		t.Errorf("winding of the visible part = %g, want the sign of %g", ndcArea(poly), ref)
	}
}

// TestClipperGuardBand clips a triangle with a vertex millions of pixels out
// to the guard band, inside the range the rasterizer snaps to.
func TestClipperGuardBand(t *testing.T) {
	c := newTestClipper(0)
	v0 := clipVertex(-0.7, -0.75, 0.5, 1, -0.7)
	v1 := clipVertex(1e6, 1e6, 0.5, 1, 1e6)
	v2 := clipVertex(-0.7, 0.85, 0.5, 1, -0.7)
	poly := c.Clip(&v0, &v1, &v2)
	if len(poly) < 4 {
		t.Fatalf("Clip returned %d vertices, want the far vertex cut away", len(poly))
	}
	limit := float32(8 + clipGuardBand + 1)
	for i := range poly {
		sv := clipperTestViewport.Project(&poly[i])
		if sv.X < -limit || sv.X > limit || sv.Y < -limit || sv.Y > limit {
			t.Errorf("vertex %d at (%g, %g) px, want within the guard band ±%g", i, sv.X, sv.Y, limit)
		}
		// The attribute is the clip-space x: interpolated with the position.
		if d := math.Abs(float64(poly[i].Attributes[0] - poly[i].Position[0])); d > 1e-3 {
			t.Errorf("vertex %d attribute %g, want its x %g", i, poly[i].Attributes[0], poly[i].Position[0])
		}
	}
}

// TestClipperSharedEdge checks that two triangles sharing an edge that
// crosses a plane, wound in opposite directions along it, cut it at the same
// point: otherwise a crack opens between them.
func TestClipperSharedEdge(t *testing.T) {
	a := clipVertex(-0.3, -0.6, 0.7, 1)
	b := clipVertex(0.4, 0.9, -0.35, 1) // in front of the near plane
	left := clipVertex(-0.9, 0.5, 0.5, 1)
	right := clipVertex(0.9, -0.2, 0.5, 1)
	c := newTestClipper(0)
	cut := func(poly []ClipSpaceVertex) map[[4]float32]bool {
		m := map[[4]float32]bool{}
		for _, v := range poly {
			if v.Position[2] == 0 {
				m[v.Position] = true
			}
		}
		return m
	}
	p1 := cut(c.Clip(&a, &b, &left))
	p2 := cut(c.Clip(&b, &a, &right))
	shared := 0
	for k := range p1 {
		if p2[k] {
			shared++
		}
	}
	if shared != 1 {
		t.Errorf("the triangles share %d cut points on their common edge, want 1 (%v, %v)", shared, p1, p2)
	}
}

func BenchmarkClipperTrivialAccept(b *testing.B) {
	c := newTestClipper(0)
	v0 := clipVertex(-0.5, -0.5, 0.5, 1, 1, 2, 3, 4)
	v1 := clipVertex(0.5, -0.5, 0.5, 1, 1, 2, 3, 4)
	v2 := clipVertex(0, 0.5, 0.5, 1, 1, 2, 3, 4)
	b.ReportAllocs()
	for range b.N {
		c.Clip(&v0, &v1, &v2)
	}
}

func BenchmarkClipperNearPlane(b *testing.B) {
	c := newTestClipper(0)
	v0 := clipVertex(-0.5, -0.5, 0.5, 1, 1, 2, 3, 4)
	v1 := clipVertex(0.5, -0.5, 0.5, 1, 1, 2, 3, 4)
	v2 := clipVertex(0, 0.5, -0.5, 1, 1, 2, 3, 4)
	b.ReportAllocs()
	for range b.N {
		c.Clip(&v0, &v1, &v2)
	}
}
