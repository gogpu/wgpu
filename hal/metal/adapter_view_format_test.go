// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build darwin && !(js && wasm)

package metal

import (
	"testing"

	"github.com/gogpu/gputypes"
)

// TestAdapterMapViewFormat pins the view formats of Rust wgpu-hal
// src/metal/adapter.rs map_view_format: a view of the stencil plane of a
// combined depth/stencil texture is X24_Stencil8 or X32_Stencil8, every other
// view of it takes the texture's pixel format, and views of other textures map
// their own format. The raw values are metal-rs v0.33.0 src/constants.rs.
func TestAdapterMapViewFormat(t *testing.T) {
	const (
		depth32Float         MTLPixelFormat = 252
		depth24UnormStencil8 MTLPixelFormat = 255
		depth32FloatStencil8 MTLPixelFormat = 260
		x32Stencil8          MTLPixelFormat = 261
		x24Stencil8          MTLPixelFormat = 262
	)
	const (
		d24s8   = gputypes.TextureFormatDepth24PlusStencil8
		d32fs8  = gputypes.TextureFormatDepth32FloatStencil8
		d24     = gputypes.TextureFormatDepth24Plus
		d32f    = gputypes.TextureFormatDepth32Float
		s8      = gputypes.TextureFormatStencil8
		undef   = gputypes.TextureFormatUndefined
		all     = gputypes.TextureAspectAll
		noAsp   = gputypes.TextureAspectUndefined
		stencil = gputypes.TextureAspectStencilOnly
		depth   = gputypes.TextureAspectDepthOnly
	)

	tests := []struct {
		name    string
		d24s8   bool // Adapter.formatDepth24Stencil8
		texture gputypes.TextureFormat
		view    gputypes.TextureFormat
		aspect  gputypes.TextureAspect
		want    MTLPixelFormat
	}{
		// Depth32FloatStencil8 storage.
		{"d32fs8 stencil aspect", false, d32fs8, d32fs8, stencil, x32Stencil8},
		{"d32fs8 stencil8 view", false, d32fs8, s8, all, x32Stencil8},
		{"d32fs8 stencil8 view no aspect", false, d32fs8, s8, noAsp, x32Stencil8},
		{"d32fs8 undefined view stencil aspect", false, d32fs8, undef, stencil, x32Stencil8},
		{"d32fs8 depth aspect", false, d32fs8, d32fs8, depth, depth32FloatStencil8},
		{"d32fs8 depth32float view", false, d32fs8, d32f, depth, depth32FloatStencil8},
		{"d32fs8 all", false, d32fs8, d32fs8, all, depth32FloatStencil8},
		{"d32fs8 undefined view all", false, d32fs8, undef, all, depth32FloatStencil8},

		// Depth24PlusStencil8 without Depth24UnormStencil8: Depth32FloatStencil8 storage.
		{"d24s8 fallback stencil aspect", false, d24s8, d24s8, stencil, x32Stencil8},
		{"d24s8 fallback stencil8 view", false, d24s8, s8, all, x32Stencil8},
		{"d24s8 fallback depth aspect", false, d24s8, d24s8, depth, depth32FloatStencil8},
		{"d24s8 fallback depth24plus view", false, d24s8, d24, depth, depth32FloatStencil8},
		{"d24s8 fallback all", false, d24s8, undef, all, depth32FloatStencil8},

		// Depth24PlusStencil8 with Depth24UnormStencil8 storage.
		{"d24s8 native stencil aspect", true, d24s8, d24s8, stencil, x24Stencil8},
		{"d24s8 native stencil8 view", true, d24s8, s8, noAsp, x24Stencil8},
		{"d24s8 native depth aspect", true, d24s8, d24s8, depth, depth24UnormStencil8},
		{"d24s8 native depth24plus view", true, d24s8, d24, depth, depth24UnormStencil8},
		{"d32fs8 ignores native d24s8", true, d32fs8, s8, stencil, x32Stencil8},

		// Textures that are not combined depth/stencil map the view format.
		{"depth32float texture", false, d32f, undef, depth, depth32Float},
		{"depth24plus texture fallback", false, d24, d24, all, depth32Float},
		{"depth24plus texture native", true, d24, undef, all, depth24UnormStencil8},
		{"rgba8 view of rgba8 srgb", false, gputypes.TextureFormatRGBA8UnormSrgb,
			gputypes.TextureFormatRGBA8Unorm, all, MTLPixelFormatRGBA8Unorm},
		{"rgba8 undefined view", false, gputypes.TextureFormatRGBA8Unorm, undef, all, MTLPixelFormatRGBA8Unorm},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &Adapter{formatDepth24Stencil8: tt.d24s8}
			if got := a.mapViewFormat(tt.texture, tt.view, tt.aspect); got != tt.want {
				t.Errorf("mapViewFormat(%v, %v, %v) = %d, want %d", tt.texture, tt.view, tt.aspect, got, tt.want)
			}
		})
	}
}

// TestMetalPixelFormatStencilViewValues pins the stencil view constants to
// metal-rs v0.33.0 src/constants.rs, which the table above relies on.
func TestMetalPixelFormatStencilViewValues(t *testing.T) {
	for _, c := range []struct {
		name string
		got  MTLPixelFormat
		want MTLPixelFormat
	}{
		{"Depth32Float", MTLPixelFormatDepth32Float, 252},
		{"Depth24UnormStencil8", MTLPixelFormatDepth24UnormStencil8, 255},
		{"Depth32FloatStencil8", MTLPixelFormatDepth32FloatStencil8, 260},
		{"X32Stencil8", MTLPixelFormatX32Stencil8, 261},
		{"X24Stencil8", MTLPixelFormatX24Stencil8, 262},
	} {
		if c.got != c.want {
			t.Errorf("MTLPixelFormat%s = %d, want %d", c.name, c.got, c.want)
		}
	}
}
