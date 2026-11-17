package wgpu

import "github.com/gogpu/gputypes"

// The WebGPU defaults of the texture and sampler descriptor members whose Go
// zero value can never be an explicit, valid spec value (ADR-080 Rule 1,
// LEAD-017). The pipeline descriptor defaults live in pipeline_defaults.go.
//
// Textures (index.bs, dictionary GPUTextureDescriptor):
//
//   - mipLevelCount 1, sampleCount 1, dimension "2d";
//   - size.height 1 and size.depthOrArrayLayers 1 (index.bs, dictionary
//     GPUExtent3DDict).
//
// A zero for any of them is a validation error in the spec (every size and
// the mip level count must be at least 1, the sample count 1 or 4), and
// GPUTextureDimension has no "undefined" member, so the zero always means
// "not set". size.width has no default: it is required, and a zero width
// stays an error.
//
// Samplers (index.bs, dictionary GPUSamplerDescriptor): addressModeU/V/W
// "clamp-to-edge", magFilter/minFilter/mipmapFilter "nearest", lodMaxClamp 32
// and maxAnisotropy 1. GPUAddressMode, GPUFilterMode and GPUMipmapFilterMode
// have no "undefined" member, so the zero can only mean "not set"; Metal
// mapped an undefined mipmap filter to "not mipmapped", so a zero-value
// sampler read only the base level there, and the software backend repeated
// for an undefined address mode while every GPU backend clamped.
//
// Every CreateTexture and CreateSampler path (native, -tags rust, js/wasm)
// applies these before validation and before the descriptor reaches a
// backend, wgpu-native or the browser. The HAL takes the values literally.
//
// They live in the root package with the descriptor types they normalize: the
// descriptor and its conversion stay public while the resource types move to
// an extraction package, and an unexported package-scope name cannot cross
// that boundary (ADR-070 §11, S2).
const (
	textureMipLevelCountDefault = 1
	textureSampleCountDefault   = 1
	textureDimensionDefault     = gputypes.TextureDimension2D
	textureExtentDefault        = 1

	lodMaxClampDefault float32 = 32
	anisotropyDefault  uint16  = 1
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

// normalizeFilterMode maps FilterModeUndefined, the Go zero value, to the
// WebGPU default "nearest" (GPUSamplerDescriptor magFilter, minFilter and
// mipmapFilter, index.bs). Every explicit value is kept.
func normalizeFilterMode(m gputypes.FilterMode) gputypes.FilterMode {
	if m == gputypes.FilterModeUndefined {
		return gputypes.FilterModeNearest
	}
	return m
}

// normalizeAddressMode maps AddressModeUndefined, the Go zero value, to the
// WebGPU default "clamp-to-edge" (GPUSamplerDescriptor addressModeU/V/W).
func normalizeAddressMode(m gputypes.AddressMode) gputypes.AddressMode {
	if m == gputypes.AddressModeUndefined {
		return gputypes.AddressModeClampToEdge
	}
	return m
}

// normalizeLodMaxClamp maps the zero value of SamplerDescriptor.LodMaxClamp to
// the WebGPU default, so a zero-value descriptor samples the whole mip chain.
//
// Every CreateSampler path (native, -tags rust, js/wasm) calls it once before
// validation and before the value reaches a backend. The HAL, wgpu-native and
// the browser take the value literally, as the spec and Rust wgpu do, so the
// rule has to live here: none of them treats 0 as "no clamp" on its own.
func normalizeLodMaxClamp(v float32) float32 {
	if v == 0 {
		return lodMaxClampDefault
	}
	return v
}

// normalizeAnisotropy maps the zero value of SamplerDescriptor.Anisotropy to
// the WebGPU default 1. The spec has no maxAnisotropy 0 (validation requires
// at least 1), so the Go zero can only mean "not set". Every CreateSampler
// path calls it, so the backends, wgpu-native and the browser get the same
// value; it was already the behavior of each path (core validation treated 0
// as 1, go-webgpu sends 1 to wgpu-native, the browser omits it).
func normalizeAnisotropy(v uint16) uint16 {
	if v == 0 {
		return anisotropyDefault
	}
	return v
}
