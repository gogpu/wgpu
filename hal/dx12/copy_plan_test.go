// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows && !(js && wasm)

package dx12

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

func testTexture(format gputypes.TextureFormat, dimension gputypes.TextureDimension, width, height, depth, mips uint32) *Texture {
	return &Texture{format: format, dimension: dimension, size: hal.Extent3D{Width: width, Height: height, DepthOrArrayLayers: depth}, mipLevels: mips}
}

func TestPlanBufferTextureCopiesPlacementAndArraySlices(t *testing.T) {
	texture := testTexture(gputypes.TextureFormatRGBA8Unorm, gputypes.TextureDimension2D, 128, 16, 3, 2)
	copy := hal.ImageCopyTexture{MipLevel: 1, Origin: hal.Origin3D{X: 4, Y: 2, Z: 1}, Aspect: gputypes.TextureAspectAll}
	size := hal.Extent3D{Width: 8, Height: 2, DepthOrArrayLayers: 2}
	plans := planBufferTextureCopies(texture, copy, hal.ImageDataLayout{Offset: 0, BytesPerRow: 256, RowsPerImage: 4}, size)
	if len(plans) != 2 {
		t.Fatalf("got %d plans, want 2", len(plans))
	}
	for i, plan := range plans {
		if plan.bufferOffset%512 != 0 {
			t.Errorf("plan %d offset %d is not 512-aligned", i, plan.bufferOffset)
		}
		if plan.subresource != texture.subresourceIndex(1, 1+uint32(i)) {
			t.Errorf("plan %d subresource %d", i, plan.subresource)
		}
		if plan.textureOriginX != copy.Origin.X || plan.textureOriginY != copy.Origin.Y || plan.textureOriginZ != 0 {
			t.Errorf("plan %d texture origin = (%d,%d,%d)", i, plan.textureOriginX, plan.textureOriginY, plan.textureOriginZ)
		}
	}
}

func TestPlanBufferTextureCopiesReconstructsPlacedDelta(t *testing.T) {
	texture := testTexture(gputypes.TextureFormatRGBA8Unorm, gputypes.TextureDimension2D, 128, 64, 1, 1)
	copy := hal.ImageCopyTexture{Origin: hal.Origin3D{}, Aspect: gputypes.TextureAspectAll}
	for _, tc := range []struct {
		name   string
		offset uint64
		pitch  uint32
	}{
		{name: "zero", offset: 0, pitch: 256},
		{name: "one-block", offset: 4, pitch: 256},
		{name: "256", offset: 256, pitch: 768},
		{name: "512", offset: 512, pitch: 256},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plans := planBufferTextureCopies(texture, copy, hal.ImageDataLayout{Offset: tc.offset, BytesPerRow: tc.pitch, RowsPerImage: 2}, hal.Extent3D{Width: 8, Height: 2, DepthOrArrayLayers: 1})
			if len(plans) != 1 {
				t.Fatalf("got %d plans", len(plans))
			}
			plan := plans[0]
			logical := plan.bufferOffset + uint64(plan.bufferOriginY)*uint64(plan.rowPitch)/uint64(1) + uint64(plan.bufferOriginX)*4
			if logical != tc.offset {
				t.Fatalf("reconstructed offset %d, want %d", logical, tc.offset)
			}
		})
	}
}

func TestPlanBufferTextureCopiesBCAndRejectsUnrepresentableDelta(t *testing.T) {
	texture := testTexture(gputypes.TextureFormatBC1RGBAUnorm, gputypes.TextureDimension2D, 64, 64, 1, 1)
	copy := hal.ImageCopyTexture{Aspect: gputypes.TextureAspectAll}
	plans := planBufferTextureCopies(texture, copy, hal.ImageDataLayout{Offset: 256, BytesPerRow: 512, RowsPerImage: 4}, hal.Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 1})
	if len(plans) != 1 {
		t.Fatalf("got %d BC plans", len(plans))
	}
	if plans[0].bufferOriginX != 128 || plans[0].footprintWidth < 132 {
		t.Fatalf("BC plan origin/footprint = (%d,%d)", plans[0].bufferOriginX, plans[0].footprintWidth)
	}
	plan := plans[0]
	reconstructed := plan.bufferOffset + uint64(plan.bufferOriginY/4)*uint64(plan.rowPitch) + uint64(plan.bufferOriginX/4)*8
	if reconstructed != 256 {
		t.Fatalf("BC reconstructed offset %d, want 256", reconstructed)
	}
	if got := planBufferTextureCopies(texture, copy, hal.ImageDataLayout{Offset: 4, BytesPerRow: 256, RowsPerImage: 4}, hal.Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 1}); got != nil {
		t.Fatal("expected byte delta not divisible by BC block size to be rejected")
	}
}

func TestPlanBufferTextureCopiesZeroPitchUsesNativeAlignment(t *testing.T) {
	rgba := testTexture(gputypes.TextureFormatRGBA8Unorm, gputypes.TextureDimension2D, 8, 8, 1, 1)
	rgbaPlans := planBufferTextureCopies(rgba, hal.ImageCopyTexture{Aspect: gputypes.TextureAspectAll}, hal.ImageDataLayout{}, hal.Extent3D{Width: 8, Height: 1, DepthOrArrayLayers: 1})
	if len(rgbaPlans) != 1 || rgbaPlans[0].rowPitch != 256 {
		t.Fatalf("RGBA zero-pitch plan = %#v, want one 256-byte row", rgbaPlans)
	}
	bc := testTexture(gputypes.TextureFormatBC1RGBAUnorm, gputypes.TextureDimension2D, 8, 8, 1, 1)
	bcPlans := planBufferTextureCopies(bc, hal.ImageCopyTexture{Aspect: gputypes.TextureAspectAll}, hal.ImageDataLayout{}, hal.Extent3D{Width: 8, Height: 4, DepthOrArrayLayers: 1})
	if len(bcPlans) != 1 || bcPlans[0].rowPitch != 256 {
		t.Fatalf("BC zero-pitch plan = %#v, want one 256-byte block row", bcPlans)
	}
	if planBufferTextureCopies(rgba, hal.ImageCopyTexture{Aspect: gputypes.TextureAspectAll}, hal.ImageDataLayout{}, hal.Extent3D{Width: 8, Height: 2, DepthOrArrayLayers: 1}) != nil {
		t.Fatal("zero BytesPerRow must remain invalid for multi-row RGBA copies")
	}
}

func TestTextureWriteSourceOffsetUsesBlockRows(t *testing.T) {
	// BC1's four texel rows occupy one source block row. A two-layer upload
	// with RowsPerImage=4 must therefore advance by one row pitch per layer.
	if got := textureWriteSourceOffset(16, 256, 1, 1, 0); got != 272 {
		t.Fatalf("BC layer offset = %d, want 272", got)
	}
	if got := textureWriteSourceOffset(16, 256, 2, 1, 0); got != 528 {
		t.Fatalf("two block-row layer offset = %d, want 528", got)
	}
}

func TestWriteTextureNativeLayoutRepacksUnalignedSourcePitch(t *testing.T) {
	texture := testTexture(gputypes.TextureFormatRGBA8Unorm, gputypes.TextureDimension2D, 64, 8, 1, 1)
	_, native, sourceBPR, _, ok := writeTextureNativeLayout(texture, hal.ImageDataLayout{BytesPerRow: 300, RowsPerImage: 2}, hal.Extent3D{Width: 64, Height: 2, DepthOrArrayLayers: 1})
	if !ok || native.BytesPerRow != 256 || sourceBPR != 300 {
		t.Fatalf("native layout = %#v, source BPR = %d, ok=%v", native, sourceBPR, ok)
	}
}

func TestWriteTextureNativeLayoutKeepsSourcePaddingOutOfStaging(t *testing.T) {
	texture := testTexture(gputypes.TextureFormatBC1RGBAUnorm, gputypes.TextureDimension2D, 8, 8, 2, 1)
	_, native, _, sourceBlockRows, ok := writeTextureNativeLayout(texture, hal.ImageDataLayout{BytesPerRow: 300, RowsPerImage: 8}, hal.Extent3D{Width: 8, Height: 4, DepthOrArrayLayers: 2})
	if !ok || sourceBlockRows != 2 || native.RowsPerImage != 4 {
		t.Fatalf("native layout = %#v, source block rows = %d, ok=%v", native, sourceBlockRows, ok)
	}
}

func TestPlanBufferTextureCopiesPadded3DUsesAbsoluteDepth(t *testing.T) {
	texture := testTexture(gputypes.TextureFormatRGBA8Unorm, gputypes.TextureDimension3D, 32, 32, 8, 1)
	copy := hal.ImageCopyTexture{Origin: hal.Origin3D{X: 2, Y: 3, Z: 2}, Aspect: gputypes.TextureAspectAll}
	plans := planBufferTextureCopies(texture, copy, hal.ImageDataLayout{BytesPerRow: 256, RowsPerImage: 8}, hal.Extent3D{Width: 4, Height: 2, DepthOrArrayLayers: 3})
	if len(plans) != 3 {
		t.Fatalf("got %d plans", len(plans))
	}
	for i, plan := range plans {
		if plan.subresource != 0 || plan.textureOriginZ != 2+uint32(i) {
			t.Errorf("plan %d subresource/depth = %d/%d", i, plan.subresource, plan.textureOriginZ)
		}
	}
}

func TestPlanTextureTextureCopiesArraysAnd3D(t *testing.T) {
	array := testTexture(gputypes.TextureFormatRGBA8Unorm, gputypes.TextureDimension2D, 32, 32, 4, 2)
	arrayCopy := hal.TextureCopy{
		SrcBase: hal.ImageCopyTexture{MipLevel: 1, Origin: hal.Origin3D{Z: 1}, Aspect: gputypes.TextureAspectAll},
		DstBase: hal.ImageCopyTexture{MipLevel: 0, Origin: hal.Origin3D{Z: 2}, Aspect: gputypes.TextureAspectAll},
		Size:    hal.Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 2},
	}
	if plans := planTextureTextureCopies(array, array, arrayCopy); len(plans) != 2 || plans[0].srcFront != 0 || plans[0].dstZ != 0 {
		t.Fatalf("array plans = %#v", plans)
	}
	volume := testTexture(gputypes.TextureFormatRGBA8Unorm, gputypes.TextureDimension3D, 32, 32, 8, 1)
	volumeCopy := hal.TextureCopy{SrcBase: hal.ImageCopyTexture{Origin: hal.Origin3D{Z: 2}, Aspect: gputypes.TextureAspectAll}, DstBase: hal.ImageCopyTexture{Origin: hal.Origin3D{Z: 3}, Aspect: gputypes.TextureAspectAll}, Size: hal.Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 3}}
	plans := planTextureTextureCopies(volume, volume, volumeCopy)
	if len(plans) != 1 || plans[0].srcFront != 2 || plans[0].srcBack != 5 || plans[0].dstZ != 3 {
		t.Fatalf("3D plans = %#v", plans)
	}
	volumeCopy.Size.DepthOrArrayLayers = 6
	if planTextureTextureCopies(volume, volume, volumeCopy) != nil {
		t.Fatal("expected absolute 3D bounds to reject an oversized copy")
	}
}
