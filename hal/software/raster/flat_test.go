// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !(js && wasm)

package raster

import (
	"math"
	"testing"
)

// TestRasterizeFlatAttributes checks that a flat attribute reaches every
// fragment as the provoking vertex V0's lane, bit for bit (here a signaling
// NaN pattern, which any arithmetic would change), while the other attributes
// are still interpolated.
func TestRasterizeFlatAttributes(t *testing.T) {
	flatBits := uint32(0x7F800001)
	flat := math.Float32frombits(flatBits)
	other := math.Float32frombits(0xFFFFFFFF)
	vertex := func(x, y, smooth, flatLane float32) ScreenVertex {
		return ScreenVertex{X: x, Y: y, W: 1, Attributes: []float32{smooth, flatLane}}
	}
	tri := Triangle{
		V0:   vertex(0, 0, 0, flat),
		V1:   vertex(8, 0, 1, other),
		V2:   vertex(0, 8, 2, other),
		Flat: 1 << 1,
	}
	var n int
	Rasterize(tri, Viewport{Width: 8, Height: 8, MaxDepth: 1}, func(f Fragment) {
		n++
		if got := math.Float32bits(f.Attributes[1]); got != flatBits {
			t.Fatalf("fragment (%d,%d): flat lane = %#08x, want %#08x", f.X, f.Y, got, flatBits)
		}
		want := f.Bary[1]*1 + f.Bary[2]*2
		if d := f.Attributes[0] - want; d > 1e-5 || d < -1e-5 {
			t.Fatalf("fragment (%d,%d): smooth lane = %v, want %v", f.X, f.Y, f.Attributes[0], want)
		}
	})
	if n == 0 {
		t.Fatal("no fragments")
	}
}
