// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !(js && wasm)

package vulkan

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// TestSplitDepthStencilLayerCopies checks the region list that both copy
// directions (vkCmdCopyBufferToImage and vkCmdCopyImageToBuffer) get for a
// depth/stencil array copy of several layers: one region per layer, each one
// image further into the buffer (splitDepthStencilLayerCopies names the
// driver bug). Other copies keep their region.
func TestSplitDepthStencilLayerCopies(t *testing.T) {
	stencilAt := func(z, layers uint32, layout hal.ImageDataLayout) hal.BufferTextureCopy {
		return hal.BufferTextureCopy{
			BufferLayout: layout,
			TextureBase:  hal.ImageCopyTexture{MipLevel: 1, Origin: hal.Origin3D{Z: z}, Aspect: gputypes.TextureAspectStencilOnly},
			Size:         hal.Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: layers},
		}
	}
	type want struct {
		offset uint64
		layer  uint32
		count  uint32
	}
	tests := []struct {
		name      string
		format    gputypes.TextureFormat
		dimension gputypes.TextureDimension
		regions   []hal.BufferTextureCopy
		want      []want
	}{
		{"stencil 3 layers at z=1", gputypes.TextureFormatDepth24PlusStencil8, gputypes.TextureDimension2D,
			[]hal.BufferTextureCopy{stencilAt(1, 3, hal.ImageDataLayout{Offset: 512, BytesPerRow: 256, RowsPerImage: 4})},
			[]want{{512, 1, 1}, {512 + 1024, 2, 1}, {512 + 2048, 3, 1}}},
		{"rowsPerImage taller than the copy", gputypes.TextureFormatStencil8, gputypes.TextureDimension2D,
			[]hal.BufferTextureCopy{stencilAt(0, 2, hal.ImageDataLayout{BytesPerRow: 256, RowsPerImage: 6})},
			[]want{{0, 0, 1}, {1536, 1, 1}}},
		{"tightly packed layout", gputypes.TextureFormatDepth16Unorm, gputypes.TextureDimension2D,
			[]hal.BufferTextureCopy{{
				TextureBase: hal.ImageCopyTexture{Origin: hal.Origin3D{Z: 2}, Aspect: gputypes.TextureAspectDepthOnly},
				Size:        hal.Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 2},
			}},
			[]want{{0, 2, 1}, {32, 3, 1}}},
		{"only the multi-layer region splits", gputypes.TextureFormatStencil8, gputypes.TextureDimension2D,
			[]hal.BufferTextureCopy{
				stencilAt(0, 1, hal.ImageDataLayout{BytesPerRow: 256, RowsPerImage: 4}),
				stencilAt(1, 2, hal.ImageDataLayout{Offset: 1024, BytesPerRow: 256, RowsPerImage: 4}),
				stencilAt(3, 1, hal.ImageDataLayout{Offset: 4096, BytesPerRow: 256, RowsPerImage: 4}),
			},
			[]want{{0, 0, 1}, {1024, 1, 1}, {2048, 2, 1}, {4096, 3, 1}}},
		{"color keeps one region", gputypes.TextureFormatRGBA8Unorm, gputypes.TextureDimension2D,
			[]hal.BufferTextureCopy{{
				BufferLayout: hal.ImageDataLayout{BytesPerRow: 256, RowsPerImage: 4},
				TextureBase:  hal.ImageCopyTexture{Origin: hal.Origin3D{Z: 1}, Aspect: gputypes.TextureAspectAll},
				Size:         hal.Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 3},
			}},
			[]want{{0, 1, 3}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertBufferImageCopyRegions(tt.regions, tt.format, tt.dimension)
			if len(got) != len(tt.want) {
				t.Fatalf("%d regions, want %d", len(got), len(tt.want))
			}
			for i, w := range tt.want {
				r := got[i]
				if uint64(r.BufferOffset) != w.offset || r.ImageSubresource.BaseArrayLayer != w.layer ||
					r.ImageSubresource.LayerCount != w.count {
					t.Errorf("region %d = offset %d layers %d+%d, want offset %d layers %d+%d", i,
						r.BufferOffset, r.ImageSubresource.BaseArrayLayer, r.ImageSubresource.LayerCount,
						w.offset, w.layer, w.count)
				}
			}
		})
	}
}
