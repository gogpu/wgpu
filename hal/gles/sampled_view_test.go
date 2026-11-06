// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build (windows || linux) && !(js && wasm)

package gles

import (
	"errors"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/gles/gl"
)

// TestNewBindGroupRefusesUnsampleableViews: GL binds a texture only through
// the target it was created with and samples all of its layers, so a sampled
// view whose dimension needs another target, or which covers a subset of the
// layers, must fail with hal.ErrTextureViewNotSampleable instead of reading
// the wrong texels. Storage bindings bind one layer through
// glBindImageTexture and are not checked.
func TestNewBindGroupRefusesUnsampleableViews(t *testing.T) {
	tex2D := &Texture{id: 1, target: gl.TEXTURE_2D, size: hal.Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 1}}
	array4 := &Texture{id: 2, target: gl.TEXTURE_2D_ARRAY, size: hal.Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 4}}
	array12 := &Texture{id: 3, target: gl.TEXTURE_2D_ARRAY, size: hal.Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 12}}
	cube := &Texture{id: 4, target: gl.TEXTURE_CUBE_MAP, size: hal.Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 6}}
	tex3D := &Texture{id: 5, target: gl.TEXTURE_3D, size: hal.Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 4}}
	msaa := &Texture{id: 6, target: gl.TEXTURE_2D_MULTISAMPLE, size: hal.Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 1}}
	cubeArray := &Texture{id: 7, target: gl.TEXTURE_CUBE_MAP_ARRAY, size: hal.Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 12}}

	d2 := gputypes.TextureViewDimension2D
	for _, tc := range []struct {
		name         string
		tex          *Texture
		dim          gputypes.TextureViewDimension
		multisampled bool
		base, count  uint32
		ok           bool
	}{
		{"2d of a 2d texture", tex2D, d2, false, 0, 1, true},
		{"1d view binds TEXTURE_2D", tex2D, gputypes.TextureViewDimension1D, false, 0, 1, true},
		{"2d-array of every layer", array4, gputypes.TextureViewDimension2DArray, false, 0, 4, true},
		{"2d-array, layer count left to the default", array4, gputypes.TextureViewDimension2DArray, false, 0, 0, true},
		{"cube of a cube texture", cube, gputypes.TextureViewDimensionCube, false, 0, 6, true},
		{"3d", tex3D, gputypes.TextureViewDimension3D, false, 0, 1, true},
		{"multisampled 2d", msaa, d2, true, 0, 1, true},
		{"2d of layer 0 of an array", array4, d2, false, 0, 1, false},
		{"2d of layer 2 of an array", array4, d2, false, 2, 1, false},
		{"2d-array of layers 2..4", array4, gputypes.TextureViewDimension2DArray, false, 2, 2, false},
		{"2d-array of layers 0..2", array4, gputypes.TextureViewDimension2DArray, false, 0, 2, false},
		{"cube of a 12-layer array", array12, gputypes.TextureViewDimensionCube, false, 6, 6, false},
		{"cube-array of a 2d array (no cube-array support)", array12, gputypes.TextureViewDimensionCubeArray, false, 0, 12, false},
		{"cube-array of every layer of a cube array", cubeArray, gputypes.TextureViewDimensionCubeArray, false, 0, 12, true},
		{"cube-array of layers 6..12 of a cube array", cubeArray, gputypes.TextureViewDimensionCubeArray, false, 6, 6, false},
		{"cube of a cube array", cubeArray, gputypes.TextureViewDimensionCube, false, 0, 6, false},
		{"2d-array of a cube array", cubeArray, gputypes.TextureViewDimension2DArray, false, 0, 12, false},
		{"non-multisampled view of a multisampled texture", msaa, d2, false, 0, 1, false},
		{"2d-array of a 2d texture", tex2D, gputypes.TextureViewDimension2DArray, false, 0, 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dev := &Device{}
			view, err := dev.CreateTextureView(tc.tex, &TextureViewDescriptor{
				Dimension: tc.dim, MipLevelCount: 1, BaseArrayLayer: tc.base, ArrayLayerCount: tc.count,
			})
			if err != nil {
				t.Fatalf("CreateTextureView: %v", err)
			}
			entries := []gputypes.BindGroupEntry{{Binding: 3, Resource: gputypes.TextureViewBinding{TextureView: view.NativeHandle()}}}

			sampled := &BindGroupLayout{entries: []gputypes.BindGroupLayoutEntry{{
				Binding: 3,
				Texture: &gputypes.TextureBindingLayout{ViewDimension: tc.dim, Multisampled: tc.multisampled},
			}}}
			_, err = newBindGroup(&dev.views, sampled, entries)
			if tc.ok && err != nil {
				t.Fatalf("sampled: unexpected error %v", err)
			}
			if !tc.ok && !errors.Is(err, hal.ErrTextureViewNotSampleable) {
				t.Fatalf("sampled: error %v, want hal.ErrTextureViewNotSampleable", err)
			}

			storage := &BindGroupLayout{entries: []gputypes.BindGroupLayoutEntry{{
				Binding: 3,
				StorageTexture: &gputypes.StorageTextureBindingLayout{
					Access: gputypes.StorageTextureAccessWriteOnly, Format: gputypes.TextureFormatRGBA8Unorm, ViewDimension: tc.dim,
				},
			}}}
			if _, err := newBindGroup(&dev.views, storage, entries); err != nil {
				t.Fatalf("storage: unexpected error %v", err)
			}
		})
	}
}
