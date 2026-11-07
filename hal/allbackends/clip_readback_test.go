// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !android && !(js && wasm) && !rust

package allbackends_test

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// The clipping tests draw triangles that only primitive clipping in clip
// space renders right (index.bs "Primitive Clipping"): one with a vertex
// behind the eye (w < 0), whose divided position is mirrored through the
// center of projection, and one with a vertex far outside the viewport,
// beyond any fixed-point range a rasterizer can snap to. The software
// backend divided every vertex by its w and rasterized the result: the
// first triangle came out mirrored and the second vanished (P1-SW-027).

const clipShader = `
struct VOut {
    @builtin(position) pos: vec4f,
    @location(0) ndc: vec2f,
}

// z = 0.5 * w at every vertex, so the depth z/w is 0.5 wherever the triangle
// is visible, and the near and far planes clip exactly where w crosses 0.
@vertex
fn vs_behind(@builtin(vertex_index) i: u32) -> VOut {
    var p = array<vec4f, 3>(
        vec4f(-0.5, -1.0, 0.5, 1.0),
        vec4f(0.8, -1.0, 0.5, 1.0),
        vec4f(-0.2, 3.0, -0.5, -1.0));
    return VOut(p[i], p[i].xy);
}

// w = 1, so the varying is the vertex's NDC position, linear on the screen.
@vertex
fn vs_far(@builtin(vertex_index) i: u32) -> VOut {
    var p = array<vec4f, 3>(
        vec4f(-0.7, -0.75, 0.5, 1.0),
        vec4f(1.0e6, 1.0e6, 0.5, 1.0),
        vec4f(-0.7, 0.85, 0.5, 1.0));
    return VOut(p[i], p[i].xy);
}

@fragment
fn fs_main(in: VOut) -> @location(0) vec4f {
    return vec4f(in.pos.xy, in.ndc);
}
`

// clipBehindVertices and clipFarVertices are the clip-space positions of
// vs_behind and vs_far.
var (
	clipBehindVertices = [3][4]float64{{-0.5, -1, 0.5, 1}, {0.8, -1, 0.5, 1}, {-0.2, 3, -0.5, -1}}
	clipFarVertices    = [3][4]float64{{-0.7, -0.75, 0.5, 1}, {1e6, 1e6, 0.5, 1}, {-0.7, 0.85, 0.5, 1}}
)

// clipVaryingTolerance bounds the error of the ndc varying of vs_far: the
// clipped vertices lie thousands of pixels out, and their varyings, about
// 2000 in magnitude, carry float32 rounding of 1e-4 into the visible pixels
// on every backend.
const clipVaryingTolerance = 1e-3

// TestClipTriangleBehindEye draws a triangle with one vertex behind the eye.
// Only the part in front of it (w > 0) is visible: a wedge that widens from
// the bottom edge of the target upward. Divided by its negative w, the
// vertex lands below the target, which drew nothing at all.
func TestClipTriangleBehindEye(t *testing.T) {
	forEachOverrideBackend(t, 0, func(t *testing.T, device *wgpu.Device) {
		runClipCase(t, device, "vs_behind", clipBehindVertices, false)
	})
}

// TestClipTriangleFarVertex draws a triangle with one vertex a million NDC
// units out, four million pixels on an 8-pixel target: a diagonal band.
// That vertex lies far beyond the ±2^21 pixels the rasterizer snaps to, and
// the triangle vanished.
func TestClipTriangleFarVertex(t *testing.T) {
	forEachOverrideBackend(t, 0, func(t *testing.T, device *wgpu.Device) {
		runClipCase(t, device, "vs_far", clipFarVertices, true)
	})
}

func runClipCase(t *testing.T, device *wgpu.Device, vsEntry string, verts [3][4]float64, checkVarying bool) {
	t.Helper()
	scene := rigScene{size: 8, wgsl: clipShader, vsEntry: vsEntry, fsEntry: "fs_main", targets: 1,
		draw: func(_ *testing.T, _ *wgpu.Device, pass *wgpu.RenderPassEncoder) {
			pass.Draw(gputypes.DrawArgs{VertexCount: 3, InstanceCount: 1})
		}}
	img := scene.run(t, device)
	background := [4]float64{rigClearColor, rigClearColor, rigClearColor, rigClearColor}
	covered := 0
	for y := range scene.size {
		for x := range scene.size {
			px, py := float64(x)+0.5, float64(y)+0.5
			nx, ny := px/float64(scene.size)*2-1, 1-py/float64(scene.size)*2
			in, sure := homogeneousCoverage(verts, nx, ny, 0.05*2/float64(scene.size))
			if !sure {
				continue // a pixel center on an edge: the fill rule decides
			}
			if !in {
				checkTexel(t, "color", x, y, img.at(0, x, y), background, 0)
				checkDepth(t, x, y, img.depthAt(x, y), 1, 0)
				continue
			}
			covered++
			checkDepth(t, x, y, img.depthAt(x, y), 0.5, viewportTolerance)
			got := img.at(0, x, y)
			checkTexel(t, "position", x, y, [4]float64{got[0], got[1]}, [4]float64{px, py}, viewportTolerance)
			if checkVarying {
				checkTexel(t, "varying", x, y, [4]float64{got[2], got[3]}, [4]float64{nx, ny}, clipVaryingTolerance)
			}
		}
	}
	if covered == 0 {
		t.Fatal("the expected coverage is empty: the test geometry is wrong")
	}
}

// homogeneousCoverage reports whether the NDC point (nx, ny) is covered by
// the visible part (w > 0) of the triangle with clip-space vertices verts,
// by the 2D homogeneous edge test: the point is s*(nx, ny, 1) = Σ λi*(xi,
// yi, wi) with s > 0 exactly when every μ = M⁻¹(nx, ny, 1) is positive, M's
// columns being the vertices' (x, y, w). sure is false when the answer
// changes within eps of the point, so a pixel center on an edge is skipped.
func homogeneousCoverage(verts [3][4]float64, nx, ny, eps float64) (in, sure bool) {
	in = homogeneousInside(verts, nx, ny)
	for _, d := range [4][2]float64{{eps, 0}, {-eps, 0}, {0, eps}, {0, -eps}} {
		if homogeneousInside(verts, nx+d[0], ny+d[1]) != in {
			return in, false
		}
	}
	return in, true
}

func homogeneousInside(verts [3][4]float64, nx, ny float64) bool {
	col := func(i int) [3]float64 { return [3]float64{verts[i][0], verts[i][1], verts[i][3]} }
	det := func(a, b, c [3]float64) float64 {
		return a[0]*(b[1]*c[2]-b[2]*c[1]) - a[1]*(b[0]*c[2]-b[2]*c[0]) + a[2]*(b[0]*c[1]-b[1]*c[0])
	}
	p := [3]float64{nx, ny, 1}
	c0, c1, c2 := col(0), col(1), col(2)
	m := det(c0, c1, c2)
	return det(p, c1, c2)/m > 0 && det(c0, p, c2)/m > 0 && det(c0, c1, p)/m > 0
}
