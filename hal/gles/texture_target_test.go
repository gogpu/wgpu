// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build (windows || linux) && !(js && wasm)

package gles

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/gles/gl"
)

// TestTextureTarget checks the target CreateTexture picks, which follows Rust
// wgpu-hal gles/mod.rs get_info_from_desc: a square single-sampled 2D texture
// with a multiple of 6 layers is a cube map at 6 layers and a cube map array
// otherwise, the latter only where the context has cube map arrays.
func TestTextureTarget(t *testing.T) {
	d1, d2, d3 := gputypes.TextureDimension1D, gputypes.TextureDimension2D, gputypes.TextureDimension3D
	size := func(w, h, layers uint32) hal.Extent3D {
		return hal.Extent3D{Width: w, Height: h, DepthOrArrayLayers: layers}
	}
	for _, tc := range []struct {
		name       string
		dim        gputypes.TextureDimension
		size       hal.Extent3D
		samples    uint32
		cubeArrays bool
		want       uint32
	}{
		{"1d", d1, size(16, 1, 1), 1, true, gl.TEXTURE_2D},
		{"2d", d2, size(4, 4, 1), 1, true, gl.TEXTURE_2D},
		{"2d multisampled", d2, size(4, 4, 1), 4, true, gl.TEXTURE_2D_MULTISAMPLE},
		{"2d array of 4", d2, size(4, 4, 4), 1, true, gl.TEXTURE_2D_ARRAY},
		{"square, 6 layers: cube", d2, size(4, 4, 6), 1, false, gl.TEXTURE_CUBE_MAP},
		{"non-square, 6 layers: array", d2, size(8, 4, 6), 1, true, gl.TEXTURE_2D_ARRAY},
		{"square, 12 layers: cube array", d2, size(4, 4, 12), 1, true, gl.TEXTURE_CUBE_MAP_ARRAY},
		{"square, 12 layers, no cube arrays: array", d2, size(4, 4, 12), 1, false, gl.TEXTURE_2D_ARRAY},
		{"square, 8 layers: array", d2, size(4, 4, 8), 1, true, gl.TEXTURE_2D_ARRAY},
		{"3d of 6 slices", d3, size(4, 4, 6), 1, true, gl.TEXTURE_3D},
	} {
		if got := textureTarget(tc.dim, tc.size, tc.samples, tc.cubeArrays); got != tc.want {
			t.Errorf("%s: target %#x, want %#x", tc.name, got, tc.want)
		}
	}
}
