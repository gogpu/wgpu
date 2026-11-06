//go:build !(js && wasm)

package vulkan

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/vulkan/vk"
)

func TestConvertBufferImageCopyRegionsUsesArrayLayersFor2DTextures(t *testing.T) {
	regions := []hal.BufferTextureCopy{{
		BufferLayout: hal.ImageDataLayout{BytesPerRow: 256, RowsPerImage: 4},
		TextureBase: hal.ImageCopyTexture{
			Origin: hal.Origin3D{X: 2, Y: 3, Z: 1},
			Aspect: gputypes.TextureAspectAll,
		},
		Size: hal.Extent3D{Width: 8, Height: 4, DepthOrArrayLayers: 2},
	}}

	converted := convertBufferImageCopyRegions(regions, gputypes.TextureFormatRGBA8Unorm, gputypes.TextureDimension2D)
	got := converted[0]
	if got.ImageSubresource.BaseArrayLayer != 1 || got.ImageSubresource.LayerCount != 2 {
		t.Fatalf("array range = %d+%d, want 1+2", got.ImageSubresource.BaseArrayLayer, got.ImageSubresource.LayerCount)
	}
	if got.ImageOffset.Z != 0 || got.ImageExtent.Depth != 1 {
		t.Fatalf("depth offset+extent = %d+%d, want 0+1", got.ImageOffset.Z, got.ImageExtent.Depth)
	}
}

func TestConvertBufferImageCopyRegionsUsesDepthFor3DTextures(t *testing.T) {
	regions := []hal.BufferTextureCopy{{
		TextureBase: hal.ImageCopyTexture{
			Origin: hal.Origin3D{X: 2, Y: 3, Z: 1},
			Aspect: gputypes.TextureAspectAll,
		},
		Size: hal.Extent3D{Width: 8, Height: 4, DepthOrArrayLayers: 2},
	}}

	converted := convertBufferImageCopyRegions(regions, gputypes.TextureFormatRGBA8Unorm, gputypes.TextureDimension3D)
	got := converted[0]
	if got.ImageSubresource.BaseArrayLayer != 0 || got.ImageSubresource.LayerCount != 1 {
		t.Fatalf("array range = %d+%d, want 0+1", got.ImageSubresource.BaseArrayLayer, got.ImageSubresource.LayerCount)
	}
	if got.ImageOffset.Z != 1 || got.ImageExtent.Depth != 2 {
		t.Fatalf("depth offset+extent = %d+%d, want 1+2", got.ImageOffset.Z, got.ImageExtent.Depth)
	}
}

// TestConvertBufferImageCopyRegionsAspect: a buffer-texture copy names the
// aspect of the texture format it copies, DEPTH for a depth format and STENCIL
// for Stencil8, never COLOR (VUID-vkCmdCopyBufferToImage-imageSubresource-09105;
// wgpu-hal vulkan/conv.rs map_subresource_layers with FormatAspects::new).
func TestConvertBufferImageCopyRegionsAspect(t *testing.T) {
	depth := vk.ImageAspectFlags(vk.ImageAspectDepthBit)
	stencil := vk.ImageAspectFlags(vk.ImageAspectStencilBit)
	tests := []struct {
		name   string
		format gputypes.TextureFormat
		aspect gputypes.TextureAspect
		want   vk.ImageAspectFlags
	}{
		{"color", gputypes.TextureFormatRGBA8Unorm, gputypes.TextureAspectAll, vk.ImageAspectFlags(vk.ImageAspectColorBit)},
		{"depth16 all", gputypes.TextureFormatDepth16Unorm, gputypes.TextureAspectAll, depth},
		{"depth32float all", gputypes.TextureFormatDepth32Float, gputypes.TextureAspectAll, depth},
		{"stencil8 all", gputypes.TextureFormatStencil8, gputypes.TextureAspectAll, stencil},
		{"depth24plus-stencil8 stencil", gputypes.TextureFormatDepth24PlusStencil8, gputypes.TextureAspectStencilOnly, stencil},
		{"depth32float-stencil8 depth", gputypes.TextureFormatDepth32FloatStencil8, gputypes.TextureAspectDepthOnly, depth},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			regions := []hal.BufferTextureCopy{{
				TextureBase: hal.ImageCopyTexture{Aspect: tt.aspect},
				Size:        hal.Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 1},
			}}
			got := convertBufferImageCopyRegions(regions, tt.format, gputypes.TextureDimension2D)[0].ImageSubresource.AspectMask
			if got != tt.want {
				t.Fatalf("aspectMask = %#x, want %#x", got, tt.want)
			}
		})
	}
}

// TestConvertBufferImageCopyRegionsRowLengthInTexels: Vulkan's bufferRowLength
// and bufferImageHeight are in texels. wgpu-hal vulkan/command.rs
// map_buffer_copies computes block_width * (bytesPerRow / blockSize) and
// rowsPerImage * block_height, with the block size of the copied aspect.
func TestConvertBufferImageCopyRegionsRowLengthInTexels(t *testing.T) {
	tests := []struct {
		name         string
		format       gputypes.TextureFormat
		aspect       gputypes.TextureAspect
		bytesPerRow  uint32
		rowsPerImage uint32
		wantRow      uint32
		wantHeight   uint32
	}{
		{name: "uncompressed", format: gputypes.TextureFormatRGBA8Unorm, aspect: gputypes.TextureAspectAll,
			bytesPerRow: 256, rowsPerImage: 4, wantRow: 64, wantHeight: 4},
		{name: "BC1 4x4 blocks of 8 bytes", format: gputypes.TextureFormatBC1RGBAUnorm, aspect: gputypes.TextureAspectAll,
			bytesPerRow: 256, rowsPerImage: 8, wantRow: 128, wantHeight: 32},
		{name: "BC7 4x4 blocks of 16 bytes", format: gputypes.TextureFormatBC7RGBAUnorm, aspect: gputypes.TextureAspectUndefined,
			bytesPerRow: 256, rowsPerImage: 2, wantRow: 64, wantHeight: 8},
		{name: "ASTC 10x5 blocks of 16 bytes", format: gputypes.TextureFormatASTC10x5Unorm, aspect: gputypes.TextureAspectAll,
			bytesPerRow: 256, rowsPerImage: 3, wantRow: 160, wantHeight: 15},
		{name: "stencil aspect of a combined format is one byte per texel", format: gputypes.TextureFormatDepth24PlusStencil8,
			aspect: gputypes.TextureAspectStencilOnly, bytesPerRow: 256, rowsPerImage: 4, wantRow: 256, wantHeight: 4},
		{name: "stencil8", format: gputypes.TextureFormatStencil8, aspect: gputypes.TextureAspectAll,
			bytesPerRow: 256, rowsPerImage: 4, wantRow: 256, wantHeight: 4},
		{name: "depth aspect of depth32float-stencil8 is four bytes per texel", format: gputypes.TextureFormatDepth32FloatStencil8,
			aspect: gputypes.TextureAspectDepthOnly, bytesPerRow: 256, rowsPerImage: 4, wantRow: 64, wantHeight: 4},
		{name: "unspecified layout stays tightly packed", format: gputypes.TextureFormatBC1RGBAUnorm, aspect: gputypes.TextureAspectAll},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			regions := []hal.BufferTextureCopy{{
				BufferLayout: hal.ImageDataLayout{BytesPerRow: tt.bytesPerRow, RowsPerImage: tt.rowsPerImage},
				TextureBase:  hal.ImageCopyTexture{Aspect: tt.aspect},
				Size:         hal.Extent3D{Width: 8, Height: 8, DepthOrArrayLayers: 1},
			}}
			got := convertBufferImageCopyRegions(regions, tt.format, gputypes.TextureDimension2D)[0]
			if got.BufferRowLength != tt.wantRow || got.BufferImageHeight != tt.wantHeight {
				t.Fatalf("bufferRowLength, bufferImageHeight = %d, %d, want %d, %d",
					got.BufferRowLength, got.BufferImageHeight, tt.wantRow, tt.wantHeight)
			}
		})
	}
}
