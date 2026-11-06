// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build darwin && !(js && wasm)

package metal

import (
	"math"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// TestMetalBlitOption pins metalBlitOption to Rust wgpu-hal
// src/metal/conv.rs get_blit_option, and the option values to metal-rs
// v0.33.0 src/encoder.rs (DepthFromDepthStencil = 1 << 0,
// StencilFromDepthStencil = 1 << 1).
func TestMetalBlitOption(t *testing.T) {
	tests := []struct {
		format gputypes.TextureFormat
		aspect gputypes.TextureAspect
		want   MTLBlitOption
		ok     bool
	}{
		{gputypes.TextureFormatRGBA8Unorm, gputypes.TextureAspectAll, 0, true},
		{gputypes.TextureFormatBC1RGBAUnorm, gputypes.TextureAspectAll, 0, true},
		{gputypes.TextureFormatDepth32Float, gputypes.TextureAspectDepthOnly, 0, true},
		{gputypes.TextureFormatDepth32Float, gputypes.TextureAspectAll, 0, true},
		{gputypes.TextureFormatStencil8, gputypes.TextureAspectStencilOnly, 0, true},
		{gputypes.TextureFormatDepth24PlusStencil8, gputypes.TextureAspectDepthOnly, 1, true},
		{gputypes.TextureFormatDepth24PlusStencil8, gputypes.TextureAspectStencilOnly, 2, true},
		{gputypes.TextureFormatDepth32FloatStencil8, gputypes.TextureAspectDepthOnly, 1, true},
		{gputypes.TextureFormatDepth32FloatStencil8, gputypes.TextureAspectStencilOnly, 2, true},
		{gputypes.TextureFormatDepth24PlusStencil8, gputypes.TextureAspectAll, 0, false},
		{gputypes.TextureFormatDepth32FloatStencil8, gputypes.TextureAspectUndefined, 0, false},
	}
	for _, tt := range tests {
		got, ok := metalBlitOption(tt.format, tt.aspect)
		if got != tt.want || ok != tt.ok {
			t.Errorf("metalBlitOption(%s, %s) = %d, %t; want %d, %t", tt.format, tt.aspect, got, ok, tt.want, tt.ok)
		}
	}
}

// TestMetalTextureDataCopyLayoutSizesAspectPlane checks that a copy of one
// plane of a combined depth/stencil format is sized by that plane: one byte
// per texel for stencil, four for the Depth32FloatStencil8 depth plane, and
// no copy at all for the Depth24Plus depth plane or for both planes at once.
func TestMetalTextureDataCopyLayoutSizesAspectPlane(t *testing.T) {
	size := hal.Extent3D{Width: 5, Height: 3, DepthOrArrayLayers: 2}
	tests := []struct {
		name          string
		format        gputypes.TextureFormat
		aspect        gputypes.TextureAspect
		bytesPerRow   uint32
		bytesPerImage uint64
		required      uint64
	}{
		// Two images of 3 rows of 5 bytes: 15 + 2*5 + 5.
		{"Depth24PlusStencil8 stencil", gputypes.TextureFormatDepth24PlusStencil8, gputypes.TextureAspectStencilOnly, 5, 15, 30},
		{"Depth32FloatStencil8 stencil", gputypes.TextureFormatDepth32FloatStencil8, gputypes.TextureAspectStencilOnly, 5, 15, 30},
		// Two images of 3 rows of 20 bytes: 60 + 2*20 + 20.
		{"Depth32FloatStencil8 depth", gputypes.TextureFormatDepth32FloatStencil8, gputypes.TextureAspectDepthOnly, 20, 60, 120},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validateMetalTextureDataCopyLayout(tt.required, tt.format, tt.aspect, hal.ImageDataLayout{}, size)
			if err != nil {
				t.Fatalf("exact plane size rejected: %v", err)
			}
			if got.bytesPerRow != tt.bytesPerRow || got.bytesPerImage != tt.bytesPerImage {
				t.Fatalf("layout = %+v; want bytesPerRow %d, bytesPerImage %d", got, tt.bytesPerRow, tt.bytesPerImage)
			}
			if _, err := validateMetalTextureDataCopyLayout(tt.required-1, tt.format, tt.aspect, hal.ImageDataLayout{}, size); err == nil {
				t.Fatal("data one byte short of the plane accepted")
			}
		})
	}

	rejected := []struct {
		format gputypes.TextureFormat
		aspect gputypes.TextureAspect
	}{
		{gputypes.TextureFormatDepth24PlusStencil8, gputypes.TextureAspectAll},
		{gputypes.TextureFormatDepth24PlusStencil8, gputypes.TextureAspectDepthOnly},
		{gputypes.TextureFormatDepth32FloatStencil8, gputypes.TextureAspectAll},
		{gputypes.TextureFormatDepth24Plus, gputypes.TextureAspectAll},
	}
	for _, tt := range rejected {
		if _, err := validateMetalTextureDataCopyLayout(math.MaxUint64, tt.format, tt.aspect, hal.ImageDataLayout{}, size); err == nil {
			t.Errorf("%s with aspect %s has no copyable plane but was accepted", tt.format, tt.aspect)
		}
	}
}

// TestValidateMetalBufferTextureCopy checks the preflight of an encoder blit:
// every rejection happens before any Metal call, and an accepted stencil copy
// of a 2D array gets the stencil option, the caller's row stride and a buffer
// offset per layer.
func TestValidateMetalBufferTextureCopy(t *testing.T) {
	stencil := hal.ImageCopyTexture{Aspect: gputypes.TextureAspectStencilOnly, Origin: hal.Origin3D{Z: 1}}
	region := hal.BufferTextureCopy{
		BufferLayout: hal.ImageDataLayout{Offset: 512, BytesPerRow: 256, RowsPerImage: 4},
		TextureBase:  stencil,
		Size:         hal.Extent3D{Width: 8, Height: 4, DepthOrArrayLayers: 2},
	}
	// The last row of the second layer ends at 512 + 1024 + 3*256 + 8.
	const required = 512 + 1024 + 3*256 + 8

	blit, skip, err := validateMetalBufferTextureCopy(required, gputypes.TextureFormatDepth24PlusStencil8, gputypes.TextureDimension2D, &region)
	if err != nil || skip {
		t.Fatalf("valid stencil copy: skip=%t err=%v", skip, err)
	}
	if blit.option != MTLBlitOptionStencilFromDepthStencil {
		t.Errorf("option = %d; want StencilFromDepthStencil", blit.option)
	}
	if blit.strides.bytesPerRow != 256 || blit.strides.bytesPerImage != 0 {
		t.Errorf("strides = %+v; want 256 bytes per row and no image stride for a 2D array", blit.strides)
	}
	if blit.plan.operationCount != 2 {
		t.Fatalf("operation count = %d; want one per layer", blit.plan.operationCount)
	}
	if offset, ok := blit.bufferOffset(&region, 1); !ok || offset != 512+1024 {
		t.Errorf("second layer offset = %d, %t; want %d", offset, ok, 512+1024)
	}

	if _, _, err := validateMetalBufferTextureCopy(required-1, gputypes.TextureFormatDepth24PlusStencil8, gputypes.TextureDimension2D, &region); err == nil {
		t.Error("buffer one byte short of the stencil plane accepted")
	}

	all := region
	all.TextureBase.Aspect = gputypes.TextureAspectAll
	if _, _, err := validateMetalBufferTextureCopy(math.MaxUint64, gputypes.TextureFormatDepth24PlusStencil8, gputypes.TextureDimension2D, &all); err == nil {
		t.Error("copy of both planes of a combined format accepted")
	}

	wrapped := region
	wrapped.BufferLayout.Offset = math.MaxUint64 - 1
	if _, _, err := validateMetalBufferTextureCopy(math.MaxUint64, gputypes.TextureFormatDepth24PlusStencil8, gputypes.TextureDimension2D, &wrapped); err == nil {
		t.Error("buffer range that wraps uint64 accepted")
	}

	layerOverflow := region
	layerOverflow.TextureBase.Origin.Z = math.MaxUint32
	if _, _, err := validateMetalBufferTextureCopy(math.MaxUint64, gputypes.TextureFormatDepth24PlusStencil8, gputypes.TextureDimension2D, &layerOverflow); err == nil {
		t.Error("array layer that wraps uint32 accepted")
	}

	empty := region
	empty.Size.Height = 0
	if _, skip, err := validateMetalBufferTextureCopy(0, gputypes.TextureFormatDepth24PlusStencil8, gputypes.TextureDimension2D, &empty); err != nil || !skip {
		t.Errorf("zero extent: skip=%t err=%v; want a skipped no-op", skip, err)
	}

	dense := hal.BufferTextureCopy{
		TextureBase: hal.ImageCopyTexture{Aspect: gputypes.TextureAspectAll},
		Size:        hal.Extent3D{Width: 3, Height: 1, DepthOrArrayLayers: 1},
	}
	blit, _, err = validateMetalBufferTextureCopy(12, gputypes.TextureFormatRGBA8Unorm, gputypes.TextureDimension2D, &dense)
	if err != nil || blit.option != MTLBlitOptionNone || blit.strides.bytesPerRow != 12 {
		t.Errorf("single-row color copy: %+v, %v; want option None and a 12-byte row", blit, err)
	}
}
