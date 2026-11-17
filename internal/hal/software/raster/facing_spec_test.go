//go:build !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package raster

import (
	"math/rand/v2"
	"testing"
)

// specArea is the WebGPU spec polygon area (§ "Polygon rasterization",
// gpuweb spec index.bs), for vertices v1..vn in framebuffer coordinates
// (origin top-left, y down):
//
//	area = 0.5 * ((v1.x*vn.y - vn.x*v1.y) + sum_{i=1..n-1}(v[i+1].x*v[i].y - v[i].x*v[i+1].y))
func specArea(v [3][2]float64) float64 {
	n := len(v)
	area := v[0][0]*v[n-1][1] - v[n-1][0]*v[0][1]
	for i := 0; i < n-1; i++ {
		area += v[i+1][0]*v[i][1] - v[i][0]*v[i+1][1]
	}
	return 0.5 * area
}

// specBackFacing is the spec's facing rule: "ccw" — area > 0 is front-facing,
// otherwise back-facing; "cw" — area < 0 is front-facing, otherwise back.
func specBackFacing(area float64, frontFace FrontFace) bool {
	if frontFace == FrontFaceCCW {
		return !(area > 0)
	}
	return !(area < 0)
}

// TestIsBackFacingMatchesSpec checks IsBackFacing against the spec formula on
// random and degenerate triangles with integer coordinates (exact in float32),
// for both front faces (P1-SW-014: the sign used to be inverted).
func TestIsBackFacingMatchesSpec(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	var cases [][3][2]float64
	for i := 0; i < 500; i++ {
		var v [3][2]float64
		for j := range v {
			v[j] = [2]float64{float64(rng.IntN(64)), float64(rng.IntN(64))}
		}
		cases = append(cases, v)
	}
	cases = append(cases,
		[3][2]float64{{0, 0}, {5, 10}, {10, 0}}, // counter-clockwise as displayed
		[3][2]float64{{0, 0}, {10, 0}, {5, 10}}, // clockwise as displayed
		[3][2]float64{{0, 0}, {5, 5}, {10, 10}}, // collinear: zero area
		[3][2]float64{{3, 3}, {3, 3}, {3, 3}},   // a point
		[3][2]float64{{0, 0}, {10, 0}, {10, 0}}, // two equal vertices
	)

	for _, v := range cases {
		tri := CreateScreenTriangle(
			float32(v[0][0]), float32(v[0][1]), 0.5,
			float32(v[1][0]), float32(v[1][1]), 0.5,
			float32(v[2][0]), float32(v[2][1]), 0.5,
		)
		area := specArea(v)
		for _, ff := range []FrontFace{FrontFaceCCW, FrontFaceCW} {
			if got, want := IsBackFacing(tri, ff), specBackFacing(area, ff); got != want {
				t.Fatalf("IsBackFacing(%v, frontFace=%d) = %v, spec area %v says %v", v, ff, got, area, want)
			}
		}
	}
}
