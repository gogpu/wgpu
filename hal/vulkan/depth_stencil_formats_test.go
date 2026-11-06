//go:build !(js && wasm)

package vulkan

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/vulkan/vk"
)

// TestDepthStencilFormatSelection: the image format of Depth24Plus,
// Depth24PlusStencil8 and Stencil8 follows the formats the device supports,
// as Rust wgpu-hal PrivateCapabilities::map_texture_format chooses them.
func TestDepthStencilFormatSelection(t *testing.T) {
	tests := []struct {
		name      string
		supported depthStencilFormats
		format    gputypes.TextureFormat
		want      vk.Format
	}{
		{"depth24plus native", depthStencilFormats{d24: true}, gputypes.TextureFormatDepth24Plus, vk.FormatX8D24UnormPack32},
		{"depth24plus without X8_D24", depthStencilFormats{}, gputypes.TextureFormatDepth24Plus, vk.FormatD32Sfloat},
		{"depth24plus-stencil8 native", depthStencilFormats{d24s8: true}, gputypes.TextureFormatDepth24PlusStencil8, vk.FormatD24UnormS8Uint},
		{"depth24plus-stencil8 without D24S8", depthStencilFormats{d24: true, s8: true}, gputypes.TextureFormatDepth24PlusStencil8, vk.FormatD32SfloatS8Uint},
		{"stencil8 native", depthStencilFormats{s8: true, d24s8: true}, gputypes.TextureFormatStencil8, vk.FormatS8Uint},
		{"stencil8 without S8 on D24S8", depthStencilFormats{d24s8: true}, gputypes.TextureFormatStencil8, vk.FormatD24UnormS8Uint},
		{"stencil8 without S8 and D24S8", depthStencilFormats{d24: true}, gputypes.TextureFormatStencil8, vk.FormatD32SfloatS8Uint},
		{"guaranteed depth16", depthStencilFormats{}, gputypes.TextureFormatDepth16Unorm, vk.FormatD16Unorm},
		{"guaranteed depth32float", depthStencilFormats{}, gputypes.TextureFormatDepth32Float, vk.FormatD32Sfloat},
		{"depth32float-stencil8", depthStencilFormats{}, gputypes.TextureFormatDepth32FloatStencil8, vk.FormatD32SfloatS8Uint},
		{"color formats are unaffected", depthStencilFormats{}, gputypes.TextureFormatRGBA8Unorm, vk.FormatR8g8b8a8Unorm},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.supported.textureFormatToVk(tt.format); got != tt.want {
				t.Fatalf("textureFormatToVk(%v) = %v, want %v", tt.format, got, tt.want)
			}
		})
	}
}

// TestEmulatedStencil8Aspects: a Stencil8 texture on a combined image is still
// a stencil-only texture to WebGPU. Copies and views name STENCIL, one byte
// per texel; barriers add the hidden DEPTH aspect, since a layout transition
// of a combined image covers both (Rust map_subresource_range_combined_aspect).
func TestEmulatedStencil8Aspects(t *testing.T) {
	depth := vk.ImageAspectFlags(vk.ImageAspectDepthBit)
	stencil := vk.ImageAspectFlags(vk.ImageAspectStencilBit)
	native := depthStencilFormats{s8: true, d24s8: true}
	emulated := depthStencilFormats{d24s8: true}

	if got := native.barrierAspects(gputypes.TextureAspectAll, gputypes.TextureFormatStencil8); got != stencil {
		t.Errorf("native stencil8 barrier aspects = %#x, want STENCIL", got)
	}
	if got := emulated.barrierAspects(gputypes.TextureAspectAll, gputypes.TextureFormatStencil8); got != depth|stencil {
		t.Errorf("emulated stencil8 barrier aspects = %#x, want DEPTH|STENCIL", got)
	}
	if got := emulated.barrierAspects(gputypes.TextureAspectAll, gputypes.TextureFormatDepth24PlusStencil8); got != depth|stencil {
		t.Errorf("depth24plus-stencil8 barrier aspects = %#x, want DEPTH|STENCIL", got)
	}
	if got := emulated.barrierAspects(gputypes.TextureAspectAll, gputypes.TextureFormatDepth24Plus); got != depth {
		t.Errorf("depth24plus barrier aspects = %#x, want DEPTH", got)
	}

	// Views and copies take the aspects of the WebGPU format, whatever image
	// backs it.
	if got := textureAspectToVk(gputypes.TextureAspectAll, gputypes.TextureFormatStencil8); got != stencil {
		t.Errorf("stencil8 view aspects = %#x, want STENCIL", got)
	}
	regions := []hal.BufferTextureCopy{{
		BufferLayout: hal.ImageDataLayout{BytesPerRow: 256, RowsPerImage: 4},
		TextureBase:  hal.ImageCopyTexture{Aspect: gputypes.TextureAspectAll},
		Size:         hal.Extent3D{Width: 16, Height: 4, DepthOrArrayLayers: 1},
	}}
	got := convertBufferImageCopyRegions(regions, gputypes.TextureFormatStencil8, gputypes.TextureDimension2D)[0]
	if got.ImageSubresource.AspectMask != stencil || got.BufferRowLength != 256 {
		t.Errorf("stencil8 copy: aspect %#x, row length %d, want STENCIL, 256", got.ImageSubresource.AspectMask, got.BufferRowLength)
	}
	got = convertBufferImageCopyRegions(regions, gputypes.TextureFormatDepth24Plus, gputypes.TextureDimension2D)[0]
	if got.ImageSubresource.AspectMask != depth {
		t.Errorf("depth24plus copy aspect %#x, want DEPTH", got.ImageSubresource.AspectMask)
	}
}
