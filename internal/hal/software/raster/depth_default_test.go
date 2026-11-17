//go:build !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package raster

import "testing"

// TestDrawWithoutDepthStateWritesNoDepth proves the 2D path pays no throwaway
// depth write. A pipeline with no depth-stencil attachment (the WebGPU 2D
// case; the software backend's configureRasterPipeline leaves the defaults)
// must not write the depth buffer: WebGPU has depth writes only through a
// depthWriteEnabled depth-stencil state, and Rust's DepthStencilState::default
// has depth_write_enabled = false.
func TestDrawWithoutDepthStateWritesNoDepth(t *testing.T) {
	p := NewPipeline(100, 100)
	p.Clear(0, 0, 0, 1)
	p.ClearDepth(1.0)

	tri := CreateScreenTriangle(
		20, 20, 0.5,
		60, 20, 0.5,
		40, 60, 0.5,
	)
	p.DrawTriangles([]Triangle{tri}, [4]float32{1, 0, 0, 1})

	// Sanity: the triangle covered the centre, so the colour was written.
	if r, _, _, _ := p.GetPixel(40, 35); r != 255 {
		t.Fatalf("centre pixel not painted: r = %d, want 255", r)
	}
	if depth := p.GetDepthBuffer().Get(40, 35); depth != 1.0 {
		t.Errorf("depth at centre = %v after a draw with no depth state, want 1.0: nothing may write depth without a depth attachment", depth)
	}
}
