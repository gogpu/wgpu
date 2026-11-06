package wgpu

import "github.com/gogpu/gputypes"

// The WebGPU defaults of the GPUTextureDescriptor members whose Go zero value
// can never be an explicit, valid spec value (ADR-080 Rule 1, LEAD-017):
//
//   - mipLevelCount 1, sampleCount 1, dimension "2d" (index.bs, dictionary
//     GPUTextureDescriptor);
//   - size.height 1 and size.depthOrArrayLayers 1 (index.bs, dictionary
//     GPUExtent3DDict).
//
// A zero for any of them is a validation error in the spec (every size and
// the mip level count must be at least 1, the sample count 1 or 4), and
// GPUTextureDimension has no "undefined" member, so the zero always means
// "not set". size.width has no default: it is required, and a zero width
// stays an error.
//
// Every CreateTexture path (native, -tags rust, js/wasm) applies these before
// validation and before the descriptor reaches a backend, wgpu-native or the
// browser. The HAL takes the values literally.
const (
	textureMipLevelCountDefault = 1
	textureSampleCountDefault   = 1
	textureDimensionDefault     = gputypes.TextureDimension2D
	textureExtentDefault        = 1
)

// normalizeTextureDescriptor returns a copy of d with the zero members listed
// above set to their WebGPU defaults. The caller's descriptor is not modified;
// ViewFormats is shared, not copied.
func normalizeTextureDescriptor(d *TextureDescriptor) *TextureDescriptor {
	out := *d
	if out.MipLevelCount == 0 {
		out.MipLevelCount = textureMipLevelCountDefault
	}
	if out.SampleCount == 0 {
		out.SampleCount = textureSampleCountDefault
	}
	if out.Dimension == gputypes.TextureDimensionUndefined {
		out.Dimension = textureDimensionDefault
	}
	if out.Size.Height == 0 {
		out.Size.Height = textureExtentDefault
	}
	if out.Size.DepthOrArrayLayers == 0 {
		out.Size.DepthOrArrayLayers = textureExtentDefault
	}
	return &out
}
