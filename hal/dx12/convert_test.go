// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows

package dx12

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/dx12/d3d12"
)

func TestTextureFormatToD3D12(t *testing.T) {
	tests := []struct {
		name   string
		format gputypes.TextureFormat
		expect d3d12.DXGI_FORMAT
	}{
		// 8-bit formats
		{"R8Unorm", gputypes.TextureFormatR8Unorm, d3d12.DXGI_FORMAT_R8_UNORM},
		{"R8Snorm", gputypes.TextureFormatR8Snorm, d3d12.DXGI_FORMAT_R8_SNORM},
		{"R8Uint", gputypes.TextureFormatR8Uint, d3d12.DXGI_FORMAT_R8_UINT},
		{"R8Sint", gputypes.TextureFormatR8Sint, d3d12.DXGI_FORMAT_R8_SINT},

		// 16-bit formats
		{"R16Uint", gputypes.TextureFormatR16Uint, d3d12.DXGI_FORMAT_R16_UINT},
		{"R16Sint", gputypes.TextureFormatR16Sint, d3d12.DXGI_FORMAT_R16_SINT},
		{"R16Float", gputypes.TextureFormatR16Float, d3d12.DXGI_FORMAT_R16_FLOAT},
		{"RG8Unorm", gputypes.TextureFormatRG8Unorm, d3d12.DXGI_FORMAT_R8G8_UNORM},
		{"RG8Snorm", gputypes.TextureFormatRG8Snorm, d3d12.DXGI_FORMAT_R8G8_SNORM},
		{"RG8Uint", gputypes.TextureFormatRG8Uint, d3d12.DXGI_FORMAT_R8G8_UINT},
		{"RG8Sint", gputypes.TextureFormatRG8Sint, d3d12.DXGI_FORMAT_R8G8_SINT},

		// 32-bit formats
		{"R32Uint", gputypes.TextureFormatR32Uint, d3d12.DXGI_FORMAT_R32_UINT},
		{"R32Sint", gputypes.TextureFormatR32Sint, d3d12.DXGI_FORMAT_R32_SINT},
		{"R32Float", gputypes.TextureFormatR32Float, d3d12.DXGI_FORMAT_R32_FLOAT},
		{"RG16Uint", gputypes.TextureFormatRG16Uint, d3d12.DXGI_FORMAT_R16G16_UINT},
		{"RG16Sint", gputypes.TextureFormatRG16Sint, d3d12.DXGI_FORMAT_R16G16_SINT},
		{"RG16Float", gputypes.TextureFormatRG16Float, d3d12.DXGI_FORMAT_R16G16_FLOAT},
		{"RGBA8Unorm", gputypes.TextureFormatRGBA8Unorm, d3d12.DXGI_FORMAT_R8G8B8A8_UNORM},
		{"RGBA8UnormSrgb", gputypes.TextureFormatRGBA8UnormSrgb, d3d12.DXGI_FORMAT_R8G8B8A8_UNORM_SRGB},
		{"RGBA8Snorm", gputypes.TextureFormatRGBA8Snorm, d3d12.DXGI_FORMAT_R8G8B8A8_SNORM},
		{"RGBA8Uint", gputypes.TextureFormatRGBA8Uint, d3d12.DXGI_FORMAT_R8G8B8A8_UINT},
		{"RGBA8Sint", gputypes.TextureFormatRGBA8Sint, d3d12.DXGI_FORMAT_R8G8B8A8_SINT},
		{"BGRA8Unorm", gputypes.TextureFormatBGRA8Unorm, d3d12.DXGI_FORMAT_B8G8R8A8_UNORM},
		{"BGRA8UnormSrgb", gputypes.TextureFormatBGRA8UnormSrgb, d3d12.DXGI_FORMAT_B8G8R8A8_UNORM_SRGB},

		// Packed formats
		{"RGB10A2Uint", gputypes.TextureFormatRGB10A2Uint, d3d12.DXGI_FORMAT_R10G10B10A2_UINT},
		{"RGB10A2Unorm", gputypes.TextureFormatRGB10A2Unorm, d3d12.DXGI_FORMAT_R10G10B10A2_UNORM},
		{"RG11B10Ufloat", gputypes.TextureFormatRG11B10Ufloat, d3d12.DXGI_FORMAT_R11G11B10_FLOAT},

		// 64-bit formats
		{"RG32Uint", gputypes.TextureFormatRG32Uint, d3d12.DXGI_FORMAT_R32G32_UINT},
		{"RG32Sint", gputypes.TextureFormatRG32Sint, d3d12.DXGI_FORMAT_R32G32_SINT},
		{"RG32Float", gputypes.TextureFormatRG32Float, d3d12.DXGI_FORMAT_R32G32_FLOAT},
		{"RGBA16Uint", gputypes.TextureFormatRGBA16Uint, d3d12.DXGI_FORMAT_R16G16B16A16_UINT},
		{"RGBA16Sint", gputypes.TextureFormatRGBA16Sint, d3d12.DXGI_FORMAT_R16G16B16A16_SINT},
		{"RGBA16Float", gputypes.TextureFormatRGBA16Float, d3d12.DXGI_FORMAT_R16G16B16A16_FLOAT},

		// 128-bit formats
		{"RGBA32Uint", gputypes.TextureFormatRGBA32Uint, d3d12.DXGI_FORMAT_R32G32B32A32_UINT},
		{"RGBA32Sint", gputypes.TextureFormatRGBA32Sint, d3d12.DXGI_FORMAT_R32G32B32A32_SINT},
		{"RGBA32Float", gputypes.TextureFormatRGBA32Float, d3d12.DXGI_FORMAT_R32G32B32A32_FLOAT},

		// Depth/stencil formats
		{"Depth16Unorm", gputypes.TextureFormatDepth16Unorm, d3d12.DXGI_FORMAT_D16_UNORM},
		{"Depth24Plus", gputypes.TextureFormatDepth24Plus, d3d12.DXGI_FORMAT_D24_UNORM_S8_UINT},
		{"Depth24PlusStencil8", gputypes.TextureFormatDepth24PlusStencil8, d3d12.DXGI_FORMAT_D24_UNORM_S8_UINT},
		{"Depth32Float", gputypes.TextureFormatDepth32Float, d3d12.DXGI_FORMAT_D32_FLOAT},
		{"Depth32FloatStencil8", gputypes.TextureFormatDepth32FloatStencil8, d3d12.DXGI_FORMAT_D32_FLOAT_S8X24_UINT},
		{"Stencil8", gputypes.TextureFormatStencil8, d3d12.DXGI_FORMAT_D24_UNORM_S8_UINT},

		// BC compressed formats
		{"BC1RGBAUnorm", gputypes.TextureFormatBC1RGBAUnorm, d3d12.DXGI_FORMAT_BC1_UNORM},
		{"BC1RGBAUnormSrgb", gputypes.TextureFormatBC1RGBAUnormSrgb, d3d12.DXGI_FORMAT_BC1_UNORM_SRGB},
		{"BC2RGBAUnorm", gputypes.TextureFormatBC2RGBAUnorm, d3d12.DXGI_FORMAT_BC2_UNORM},
		{"BC2RGBAUnormSrgb", gputypes.TextureFormatBC2RGBAUnormSrgb, d3d12.DXGI_FORMAT_BC2_UNORM_SRGB},
		{"BC3RGBAUnorm", gputypes.TextureFormatBC3RGBAUnorm, d3d12.DXGI_FORMAT_BC3_UNORM},
		{"BC3RGBAUnormSrgb", gputypes.TextureFormatBC3RGBAUnormSrgb, d3d12.DXGI_FORMAT_BC3_UNORM_SRGB},
		{"BC4RUnorm", gputypes.TextureFormatBC4RUnorm, d3d12.DXGI_FORMAT_BC4_UNORM},
		{"BC4RSnorm", gputypes.TextureFormatBC4RSnorm, d3d12.DXGI_FORMAT_BC4_SNORM},
		{"BC5RGUnorm", gputypes.TextureFormatBC5RGUnorm, d3d12.DXGI_FORMAT_BC5_UNORM},
		{"BC5RGSnorm", gputypes.TextureFormatBC5RGSnorm, d3d12.DXGI_FORMAT_BC5_SNORM},
		{"BC6HRGBUfloat", gputypes.TextureFormatBC6HRGBUfloat, d3d12.DXGI_FORMAT_BC6H_UF16},
		{"BC6HRGBFloat", gputypes.TextureFormatBC6HRGBFloat, d3d12.DXGI_FORMAT_BC6H_SF16},
		{"BC7RGBAUnorm", gputypes.TextureFormatBC7RGBAUnorm, d3d12.DXGI_FORMAT_BC7_UNORM},
		{"BC7RGBAUnormSrgb", gputypes.TextureFormatBC7RGBAUnormSrgb, d3d12.DXGI_FORMAT_BC7_UNORM_SRGB},

		// Unknown format
		{"Unknown", gputypes.TextureFormat(65535), d3d12.DXGI_FORMAT_UNKNOWN},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := textureFormatToD3D12(tt.format)
			if got != tt.expect {
				t.Errorf("textureFormatToD3D12(%v) = %v, want %v", tt.format, got, tt.expect)
			}
		})
	}
}

func TestTextureDimensionToD3D12(t *testing.T) {
	tests := []struct {
		name   string
		dim    gputypes.TextureDimension
		expect d3d12.D3D12_RESOURCE_DIMENSION
	}{
		{"1D", gputypes.TextureDimension1D, d3d12.D3D12_RESOURCE_DIMENSION_TEXTURE1D},
		{"2D", gputypes.TextureDimension2D, d3d12.D3D12_RESOURCE_DIMENSION_TEXTURE2D},
		{"3D", gputypes.TextureDimension3D, d3d12.D3D12_RESOURCE_DIMENSION_TEXTURE3D},
		{"Unknown defaults to 2D", gputypes.TextureDimension(99), d3d12.D3D12_RESOURCE_DIMENSION_TEXTURE2D},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := textureDimensionToD3D12(tt.dim)
			if got != tt.expect {
				t.Errorf("textureDimensionToD3D12(%v) = %v, want %v", tt.dim, got, tt.expect)
			}
		})
	}
}

func TestTextureViewDimensionToSRV(t *testing.T) {
	tests := []struct {
		name   string
		dim    gputypes.TextureViewDimension
		expect d3d12.D3D12_SRV_DIMENSION
	}{
		{"1D", gputypes.TextureViewDimension1D, d3d12.D3D12_SRV_DIMENSION_TEXTURE1D},
		{"2D", gputypes.TextureViewDimension2D, d3d12.D3D12_SRV_DIMENSION_TEXTURE2D},
		{"2DArray", gputypes.TextureViewDimension2DArray, d3d12.D3D12_SRV_DIMENSION_TEXTURE2DARRAY},
		{"Cube", gputypes.TextureViewDimensionCube, d3d12.D3D12_SRV_DIMENSION_TEXTURECUBE},
		{"CubeArray", gputypes.TextureViewDimensionCubeArray, d3d12.D3D12_SRV_DIMENSION_TEXTURECUBEARRAY},
		{"3D", gputypes.TextureViewDimension3D, d3d12.D3D12_SRV_DIMENSION_TEXTURE3D},
		{"Unknown defaults to 2D", gputypes.TextureViewDimension(99), d3d12.D3D12_SRV_DIMENSION_TEXTURE2D},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := textureViewDimensionToSRV(tt.dim)
			if got != tt.expect {
				t.Errorf("textureViewDimensionToSRV(%v) = %v, want %v", tt.dim, got, tt.expect)
			}
		})
	}
}

func TestTextureViewDimensionToRTV(t *testing.T) {
	tests := []struct {
		name   string
		dim    gputypes.TextureViewDimension
		expect d3d12.D3D12_RTV_DIMENSION
	}{
		{"1D", gputypes.TextureViewDimension1D, d3d12.D3D12_RTV_DIMENSION_TEXTURE1D},
		{"2D", gputypes.TextureViewDimension2D, d3d12.D3D12_RTV_DIMENSION_TEXTURE2D},
		{"2DArray", gputypes.TextureViewDimension2DArray, d3d12.D3D12_RTV_DIMENSION_TEXTURE2DARRAY},
		{"3D", gputypes.TextureViewDimension3D, d3d12.D3D12_RTV_DIMENSION_TEXTURE3D},
		{"Unknown defaults to 2D", gputypes.TextureViewDimension(99), d3d12.D3D12_RTV_DIMENSION_TEXTURE2D},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := textureViewDimensionToRTV(tt.dim)
			if got != tt.expect {
				t.Errorf("textureViewDimensionToRTV(%v) = %v, want %v", tt.dim, got, tt.expect)
			}
		})
	}
}

func TestTextureViewDimensionToDSV(t *testing.T) {
	tests := []struct {
		name   string
		dim    gputypes.TextureViewDimension
		expect d3d12.D3D12_DSV_DIMENSION
	}{
		{"1D", gputypes.TextureViewDimension1D, d3d12.D3D12_DSV_DIMENSION_TEXTURE1D},
		{"2D", gputypes.TextureViewDimension2D, d3d12.D3D12_DSV_DIMENSION_TEXTURE2D},
		{"2DArray", gputypes.TextureViewDimension2DArray, d3d12.D3D12_DSV_DIMENSION_TEXTURE2DARRAY},
		{"Unknown defaults to 2D", gputypes.TextureViewDimension(99), d3d12.D3D12_DSV_DIMENSION_TEXTURE2D},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := textureViewDimensionToDSV(tt.dim)
			if got != tt.expect {
				t.Errorf("textureViewDimensionToDSV(%v) = %v, want %v", tt.dim, got, tt.expect)
			}
		})
	}
}

func TestIsDepthFormat(t *testing.T) {
	tests := []struct {
		name   string
		format gputypes.TextureFormat
		expect bool
	}{
		{"Depth16Unorm", gputypes.TextureFormatDepth16Unorm, true},
		{"Depth24Plus", gputypes.TextureFormatDepth24Plus, true},
		{"Depth24PlusStencil8", gputypes.TextureFormatDepth24PlusStencil8, true},
		{"Depth32Float", gputypes.TextureFormatDepth32Float, true},
		{"Depth32FloatStencil8", gputypes.TextureFormatDepth32FloatStencil8, true},
		{"Stencil8", gputypes.TextureFormatStencil8, true},
		{"RGBA8Unorm", gputypes.TextureFormatRGBA8Unorm, false},
		{"R32Float", gputypes.TextureFormatR32Float, false},
		{"BGRA8Unorm", gputypes.TextureFormatBGRA8Unorm, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isDepthFormat(tt.format)
			if got != tt.expect {
				t.Errorf("isDepthFormat(%v) = %v, want %v", tt.format, got, tt.expect)
			}
		})
	}
}

func TestDepthFormatToTypeless(t *testing.T) {
	tests := []struct {
		name   string
		format gputypes.TextureFormat
		expect d3d12.DXGI_FORMAT
	}{
		{"Depth16Unorm", gputypes.TextureFormatDepth16Unorm, d3d12.DXGI_FORMAT_R16_TYPELESS},
		{"Depth24Plus", gputypes.TextureFormatDepth24Plus, d3d12.DXGI_FORMAT_R24G8_TYPELESS},
		{"Depth24PlusStencil8", gputypes.TextureFormatDepth24PlusStencil8, d3d12.DXGI_FORMAT_R24G8_TYPELESS},
		{"Depth32Float", gputypes.TextureFormatDepth32Float, d3d12.DXGI_FORMAT_R32_TYPELESS},
		{"Depth32FloatStencil8", gputypes.TextureFormatDepth32FloatStencil8, d3d12.DXGI_FORMAT_R32G8X24_TYPELESS},
		{"Non-depth returns UNKNOWN", gputypes.TextureFormatRGBA8Unorm, d3d12.DXGI_FORMAT_UNKNOWN},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := depthFormatToTypeless(tt.format)
			if got != tt.expect {
				t.Errorf("depthFormatToTypeless(%v) = %v, want %v", tt.format, got, tt.expect)
			}
		})
	}
}

func TestDepthFormatToSRV(t *testing.T) {
	tests := []struct {
		name   string
		format gputypes.TextureFormat
		expect d3d12.DXGI_FORMAT
	}{
		{"Depth16Unorm", gputypes.TextureFormatDepth16Unorm, d3d12.DXGI_FORMAT_R16_UNORM},
		{"Depth24Plus", gputypes.TextureFormatDepth24Plus, d3d12.DXGI_FORMAT_R24_UNORM_X8_TYPELESS},
		{"Depth24PlusStencil8", gputypes.TextureFormatDepth24PlusStencil8, d3d12.DXGI_FORMAT_R24_UNORM_X8_TYPELESS},
		{"Depth32Float", gputypes.TextureFormatDepth32Float, d3d12.DXGI_FORMAT_R32_FLOAT},
		{"Depth32FloatStencil8", gputypes.TextureFormatDepth32FloatStencil8, d3d12.DXGI_FORMAT_R32_FLOAT_X8X24_TYPELESS},
		{"Non-depth returns UNKNOWN", gputypes.TextureFormatRGBA8Unorm, d3d12.DXGI_FORMAT_UNKNOWN},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := depthFormatToSRV(tt.format)
			if got != tt.expect {
				t.Errorf("depthFormatToSRV(%v) = %v, want %v", tt.format, got, tt.expect)
			}
		})
	}
}

func TestAddressModeToD3D12(t *testing.T) {
	tests := []struct {
		name   string
		mode   gputypes.AddressMode
		expect d3d12.D3D12_TEXTURE_ADDRESS_MODE
	}{
		{"Repeat", gputypes.AddressModeRepeat, d3d12.D3D12_TEXTURE_ADDRESS_MODE_WRAP},
		{"MirrorRepeat", gputypes.AddressModeMirrorRepeat, d3d12.D3D12_TEXTURE_ADDRESS_MODE_MIRROR},
		{"ClampToEdge", gputypes.AddressModeClampToEdge, d3d12.D3D12_TEXTURE_ADDRESS_MODE_CLAMP},
		{"Unknown defaults to Clamp", gputypes.AddressMode(99), d3d12.D3D12_TEXTURE_ADDRESS_MODE_CLAMP},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := addressModeToD3D12(tt.mode)
			if got != tt.expect {
				t.Errorf("addressModeToD3D12(%v) = %v, want %v", tt.mode, got, tt.expect)
			}
		})
	}
}

func TestFilterModeToD3D12(t *testing.T) {
	tests := []struct {
		name    string
		min     gputypes.FilterMode
		mag     gputypes.FilterMode
		mipmap  gputypes.FilterMode
		compare gputypes.CompareFunction
		expect  d3d12.D3D12_FILTER
	}{
		{"AllNearest", gputypes.FilterModeNearest, gputypes.FilterModeNearest, gputypes.FilterModeNearest, gputypes.CompareFunctionUndefined, d3d12.D3D12_FILTER(0x00)},
		{"MipLinear", gputypes.FilterModeNearest, gputypes.FilterModeNearest, gputypes.FilterModeLinear, gputypes.CompareFunctionUndefined, d3d12.D3D12_FILTER(0x01)},
		{"MagLinear", gputypes.FilterModeNearest, gputypes.FilterModeLinear, gputypes.FilterModeNearest, gputypes.CompareFunctionUndefined, d3d12.D3D12_FILTER(0x04)},
		{"MinLinear", gputypes.FilterModeLinear, gputypes.FilterModeNearest, gputypes.FilterModeNearest, gputypes.CompareFunctionUndefined, d3d12.D3D12_FILTER(0x10)},
		{"AllLinear", gputypes.FilterModeLinear, gputypes.FilterModeLinear, gputypes.FilterModeLinear, gputypes.CompareFunctionUndefined, d3d12.D3D12_FILTER(0x15)},
		{"ComparisonAllNearest", gputypes.FilterModeNearest, gputypes.FilterModeNearest, gputypes.FilterModeNearest, gputypes.CompareFunctionLess, d3d12.D3D12_FILTER(0x80)},
		{"ComparisonAllLinear", gputypes.FilterModeLinear, gputypes.FilterModeLinear, gputypes.FilterModeLinear, gputypes.CompareFunctionLess, d3d12.D3D12_FILTER(0x95)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filterModeToD3D12(tt.min, tt.mag, tt.mipmap, tt.compare)
			if got != tt.expect {
				t.Errorf("filterModeToD3D12() = %#x, want %#x", got, tt.expect)
			}
		})
	}
}

func TestCompareFunctionToD3D12(t *testing.T) {
	tests := []struct {
		name   string
		fn     gputypes.CompareFunction
		expect d3d12.D3D12_COMPARISON_FUNC
	}{
		{"Never", gputypes.CompareFunctionNever, d3d12.D3D12_COMPARISON_FUNC_NEVER},
		{"Less", gputypes.CompareFunctionLess, d3d12.D3D12_COMPARISON_FUNC_LESS},
		{"Equal", gputypes.CompareFunctionEqual, d3d12.D3D12_COMPARISON_FUNC_EQUAL},
		{"LessEqual", gputypes.CompareFunctionLessEqual, d3d12.D3D12_COMPARISON_FUNC_LESS_EQUAL},
		{"Greater", gputypes.CompareFunctionGreater, d3d12.D3D12_COMPARISON_FUNC_GREATER},
		{"NotEqual", gputypes.CompareFunctionNotEqual, d3d12.D3D12_COMPARISON_FUNC_NOT_EQUAL},
		{"GreaterEqual", gputypes.CompareFunctionGreaterEqual, d3d12.D3D12_COMPARISON_FUNC_GREATER_EQUAL},
		{"Always", gputypes.CompareFunctionAlways, d3d12.D3D12_COMPARISON_FUNC_ALWAYS},
		{"Unknown defaults to Never", gputypes.CompareFunction(99), d3d12.D3D12_COMPARISON_FUNC_NEVER},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := compareFunctionToD3D12(tt.fn)
			if got != tt.expect {
				t.Errorf("compareFunctionToD3D12(%v) = %v, want %v", tt.fn, got, tt.expect)
			}
		})
	}
}

func TestAlignTo256(t *testing.T) {
	tests := []struct {
		name   string
		input  uint64
		expect uint64
	}{
		{"zero", 0, 0},
		{"1", 1, 256},
		{"255", 255, 256},
		{"256", 256, 256},
		{"257", 257, 512},
		{"512", 512, 512},
		{"1000", 1000, 1024},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := alignTo256(tt.input)
			if got != tt.expect {
				t.Errorf("alignTo256(%d) = %d, want %d", tt.input, got, tt.expect)
			}
		})
	}
}

func TestBlendFactorToD3D12(t *testing.T) {
	tests := []struct {
		name   string
		factor gputypes.BlendFactor
		expect d3d12.D3D12_BLEND
	}{
		{"Zero", gputypes.BlendFactorZero, d3d12.D3D12_BLEND_ZERO},
		{"One", gputypes.BlendFactorOne, d3d12.D3D12_BLEND_ONE},
		{"Src", gputypes.BlendFactorSrc, d3d12.D3D12_BLEND_SRC_COLOR},
		{"OneMinusSrc", gputypes.BlendFactorOneMinusSrc, d3d12.D3D12_BLEND_INV_SRC_COLOR},
		{"SrcAlpha", gputypes.BlendFactorSrcAlpha, d3d12.D3D12_BLEND_SRC_ALPHA},
		{"OneMinusSrcAlpha", gputypes.BlendFactorOneMinusSrcAlpha, d3d12.D3D12_BLEND_INV_SRC_ALPHA},
		{"Dst", gputypes.BlendFactorDst, d3d12.D3D12_BLEND_DEST_COLOR},
		{"OneMinusDst", gputypes.BlendFactorOneMinusDst, d3d12.D3D12_BLEND_INV_DEST_COLOR},
		{"DstAlpha", gputypes.BlendFactorDstAlpha, d3d12.D3D12_BLEND_DEST_ALPHA},
		{"OneMinusDstAlpha", gputypes.BlendFactorOneMinusDstAlpha, d3d12.D3D12_BLEND_INV_DEST_ALPHA},
		{"SrcAlphaSaturated", gputypes.BlendFactorSrcAlphaSaturated, d3d12.D3D12_BLEND_SRC_ALPHA_SAT},
		{"Constant", gputypes.BlendFactorConstant, d3d12.D3D12_BLEND_BLEND_FACTOR},
		{"OneMinusConstant", gputypes.BlendFactorOneMinusConstant, d3d12.D3D12_BLEND_INV_BLEND_FACTOR},
		{"Unknown defaults to One", gputypes.BlendFactor(99), d3d12.D3D12_BLEND_ONE},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := blendFactorToD3D12(tt.factor)
			if got != tt.expect {
				t.Errorf("blendFactorToD3D12(%v) = %v, want %v", tt.factor, got, tt.expect)
			}
		})
	}
}

func TestBlendOperationToD3D12(t *testing.T) {
	tests := []struct {
		name   string
		op     gputypes.BlendOperation
		expect d3d12.D3D12_BLEND_OP
	}{
		{"Add", gputypes.BlendOperationAdd, d3d12.D3D12_BLEND_OP_ADD},
		{"Subtract", gputypes.BlendOperationSubtract, d3d12.D3D12_BLEND_OP_SUBTRACT},
		{"ReverseSubtract", gputypes.BlendOperationReverseSubtract, d3d12.D3D12_BLEND_OP_REV_SUBTRACT},
		{"Min", gputypes.BlendOperationMin, d3d12.D3D12_BLEND_OP_MIN},
		{"Max", gputypes.BlendOperationMax, d3d12.D3D12_BLEND_OP_MAX},
		{"Unknown defaults to Add", gputypes.BlendOperation(99), d3d12.D3D12_BLEND_OP_ADD},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := blendOperationToD3D12(tt.op)
			if got != tt.expect {
				t.Errorf("blendOperationToD3D12(%v) = %v, want %v", tt.op, got, tt.expect)
			}
		})
	}
}

func TestCullModeToD3D12(t *testing.T) {
	tests := []struct {
		name   string
		mode   gputypes.CullMode
		expect d3d12.D3D12_CULL_MODE
	}{
		{"None", gputypes.CullModeNone, d3d12.D3D12_CULL_MODE_NONE},
		{"Front", gputypes.CullModeFront, d3d12.D3D12_CULL_MODE_FRONT},
		{"Back", gputypes.CullModeBack, d3d12.D3D12_CULL_MODE_BACK},
		{"Unknown defaults to None", gputypes.CullMode(99), d3d12.D3D12_CULL_MODE_NONE},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cullModeToD3D12(tt.mode)
			if got != tt.expect {
				t.Errorf("cullModeToD3D12(%v) = %v, want %v", tt.mode, got, tt.expect)
			}
		})
	}
}

func TestFrontFaceToD3D12(t *testing.T) {
	tests := []struct {
		name   string
		face   gputypes.FrontFace
		expect int32
	}{
		{"CCW", gputypes.FrontFaceCCW, 1},
		{"CW", gputypes.FrontFaceCW, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := frontFaceToD3D12(tt.face)
			if got != tt.expect {
				t.Errorf("frontFaceToD3D12(%v) = %v, want %v", tt.face, got, tt.expect)
			}
		})
	}
}

func TestPrimitiveTopologyTypeToD3D12(t *testing.T) {
	tests := []struct {
		name     string
		topology gputypes.PrimitiveTopology
		expect   d3d12.D3D12_PRIMITIVE_TOPOLOGY_TYPE
	}{
		{"PointList", gputypes.PrimitiveTopologyPointList, d3d12.D3D12_PRIMITIVE_TOPOLOGY_TYPE_POINT},
		{"LineList", gputypes.PrimitiveTopologyLineList, d3d12.D3D12_PRIMITIVE_TOPOLOGY_TYPE_LINE},
		{"LineStrip", gputypes.PrimitiveTopologyLineStrip, d3d12.D3D12_PRIMITIVE_TOPOLOGY_TYPE_LINE},
		{"TriangleList", gputypes.PrimitiveTopologyTriangleList, d3d12.D3D12_PRIMITIVE_TOPOLOGY_TYPE_TRIANGLE},
		{"TriangleStrip", gputypes.PrimitiveTopologyTriangleStrip, d3d12.D3D12_PRIMITIVE_TOPOLOGY_TYPE_TRIANGLE},
		{"Unknown defaults to Triangle", gputypes.PrimitiveTopology(99), d3d12.D3D12_PRIMITIVE_TOPOLOGY_TYPE_TRIANGLE},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := primitiveTopologyTypeToD3D12(tt.topology)
			if got != tt.expect {
				t.Errorf("primitiveTopologyTypeToD3D12(%v) = %v, want %v", tt.topology, got, tt.expect)
			}
		})
	}
}

func TestPrimitiveTopologyToD3D12(t *testing.T) {
	tests := []struct {
		name     string
		topology gputypes.PrimitiveTopology
		expect   d3d12.D3D_PRIMITIVE_TOPOLOGY
	}{
		{"PointList", gputypes.PrimitiveTopologyPointList, d3d12.D3D_PRIMITIVE_TOPOLOGY_POINTLIST},
		{"LineList", gputypes.PrimitiveTopologyLineList, d3d12.D3D_PRIMITIVE_TOPOLOGY_LINELIST},
		{"LineStrip", gputypes.PrimitiveTopologyLineStrip, d3d12.D3D_PRIMITIVE_TOPOLOGY_LINESTRIP},
		{"TriangleList", gputypes.PrimitiveTopologyTriangleList, d3d12.D3D_PRIMITIVE_TOPOLOGY_TRIANGLELIST},
		{"TriangleStrip", gputypes.PrimitiveTopologyTriangleStrip, d3d12.D3D_PRIMITIVE_TOPOLOGY_TRIANGLESTRIP},
		{"Unknown defaults to TriangleList", gputypes.PrimitiveTopology(99), d3d12.D3D_PRIMITIVE_TOPOLOGY_TRIANGLELIST},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := primitiveTopologyToD3D12(tt.topology)
			if got != tt.expect {
				t.Errorf("primitiveTopologyToD3D12(%v) = %v, want %v", tt.topology, got, tt.expect)
			}
		})
	}
}

func TestStencilOpToD3D12(t *testing.T) {
	tests := []struct {
		name   string
		op     hal.StencilOperation
		expect d3d12.D3D12_STENCIL_OP
	}{
		{"Keep", hal.StencilOperationKeep, d3d12.D3D12_STENCIL_OP_KEEP},
		{"Zero", hal.StencilOperationZero, d3d12.D3D12_STENCIL_OP_ZERO},
		{"Replace", hal.StencilOperationReplace, d3d12.D3D12_STENCIL_OP_REPLACE},
		{"Invert", hal.StencilOperationInvert, d3d12.D3D12_STENCIL_OP_INVERT},
		{"IncrementClamp", hal.StencilOperationIncrementClamp, d3d12.D3D12_STENCIL_OP_INCR_SAT},
		{"DecrementClamp", hal.StencilOperationDecrementClamp, d3d12.D3D12_STENCIL_OP_DECR_SAT},
		{"IncrementWrap", hal.StencilOperationIncrementWrap, d3d12.D3D12_STENCIL_OP_INCR},
		{"DecrementWrap", hal.StencilOperationDecrementWrap, d3d12.D3D12_STENCIL_OP_DECR},
		{"Unknown defaults to Keep", hal.StencilOperation(99), d3d12.D3D12_STENCIL_OP_KEEP},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stencilOpToD3D12(tt.op)
			if got != tt.expect {
				t.Errorf("stencilOpToD3D12(%v) = %v, want %v", tt.op, got, tt.expect)
			}
		})
	}
}

func TestInputStepModeToD3D12(t *testing.T) {
	tests := []struct {
		name   string
		mode   gputypes.VertexStepMode
		expect d3d12.D3D12_INPUT_CLASSIFICATION
	}{
		{"Vertex", gputypes.VertexStepModeVertex, d3d12.D3D12_INPUT_CLASSIFICATION_PER_VERTEX_DATA},
		{"Instance", gputypes.VertexStepModeInstance, d3d12.D3D12_INPUT_CLASSIFICATION_PER_INSTANCE_DATA},
		{"Unknown defaults to Vertex", gputypes.VertexStepMode(99), d3d12.D3D12_INPUT_CLASSIFICATION_PER_VERTEX_DATA},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := inputStepModeToD3D12(tt.mode)
			if got != tt.expect {
				t.Errorf("inputStepModeToD3D12(%v) = %v, want %v", tt.mode, got, tt.expect)
			}
		})
	}
}

func TestVertexFormatToD3D12(t *testing.T) {
	tests := []struct {
		name   string
		format gputypes.VertexFormat
		expect d3d12.DXGI_FORMAT
	}{
		// 8-bit formats
		{"Uint8x2", gputypes.VertexFormatUint8x2, d3d12.DXGI_FORMAT_R8G8_UINT},
		{"Uint8x4", gputypes.VertexFormatUint8x4, d3d12.DXGI_FORMAT_R8G8B8A8_UINT},
		{"Sint8x2", gputypes.VertexFormatSint8x2, d3d12.DXGI_FORMAT_R8G8_SINT},
		{"Sint8x4", gputypes.VertexFormatSint8x4, d3d12.DXGI_FORMAT_R8G8B8A8_SINT},
		{"Unorm8x2", gputypes.VertexFormatUnorm8x2, d3d12.DXGI_FORMAT_R8G8_UNORM},
		{"Unorm8x4", gputypes.VertexFormatUnorm8x4, d3d12.DXGI_FORMAT_R8G8B8A8_UNORM},
		{"Snorm8x2", gputypes.VertexFormatSnorm8x2, d3d12.DXGI_FORMAT_R8G8_SNORM},
		{"Snorm8x4", gputypes.VertexFormatSnorm8x4, d3d12.DXGI_FORMAT_R8G8B8A8_SNORM},

		// 16-bit formats
		{"Uint16x2", gputypes.VertexFormatUint16x2, d3d12.DXGI_FORMAT_R16G16_UINT},
		{"Uint16x4", gputypes.VertexFormatUint16x4, d3d12.DXGI_FORMAT_R16G16B16A16_UINT},
		{"Sint16x2", gputypes.VertexFormatSint16x2, d3d12.DXGI_FORMAT_R16G16_SINT},
		{"Sint16x4", gputypes.VertexFormatSint16x4, d3d12.DXGI_FORMAT_R16G16B16A16_SINT},
		{"Unorm16x2", gputypes.VertexFormatUnorm16x2, d3d12.DXGI_FORMAT_R16G16_UNORM},
		{"Unorm16x4", gputypes.VertexFormatUnorm16x4, d3d12.DXGI_FORMAT_R16G16B16A16_UNORM},
		{"Snorm16x2", gputypes.VertexFormatSnorm16x2, d3d12.DXGI_FORMAT_R16G16_SNORM},
		{"Snorm16x4", gputypes.VertexFormatSnorm16x4, d3d12.DXGI_FORMAT_R16G16B16A16_SNORM},
		{"Float16x2", gputypes.VertexFormatFloat16x2, d3d12.DXGI_FORMAT_R16G16_FLOAT},
		{"Float16x4", gputypes.VertexFormatFloat16x4, d3d12.DXGI_FORMAT_R16G16B16A16_FLOAT},

		// 32-bit formats
		{"Float32", gputypes.VertexFormatFloat32, d3d12.DXGI_FORMAT_R32_FLOAT},
		{"Float32x2", gputypes.VertexFormatFloat32x2, d3d12.DXGI_FORMAT_R32G32_FLOAT},
		{"Float32x3", gputypes.VertexFormatFloat32x3, d3d12.DXGI_FORMAT_R32G32B32_FLOAT},
		{"Float32x4", gputypes.VertexFormatFloat32x4, d3d12.DXGI_FORMAT_R32G32B32A32_FLOAT},
		{"Uint32", gputypes.VertexFormatUint32, d3d12.DXGI_FORMAT_R32_UINT},
		{"Uint32x2", gputypes.VertexFormatUint32x2, d3d12.DXGI_FORMAT_R32G32_UINT},
		{"Uint32x3", gputypes.VertexFormatUint32x3, d3d12.DXGI_FORMAT_R32G32B32_UINT},
		{"Uint32x4", gputypes.VertexFormatUint32x4, d3d12.DXGI_FORMAT_R32G32B32A32_UINT},
		{"Sint32", gputypes.VertexFormatSint32, d3d12.DXGI_FORMAT_R32_SINT},
		{"Sint32x2", gputypes.VertexFormatSint32x2, d3d12.DXGI_FORMAT_R32G32_SINT},
		{"Sint32x3", gputypes.VertexFormatSint32x3, d3d12.DXGI_FORMAT_R32G32B32_SINT},
		{"Sint32x4", gputypes.VertexFormatSint32x4, d3d12.DXGI_FORMAT_R32G32B32A32_SINT},

		// Packed
		{"Unorm1010102", gputypes.VertexFormatUnorm1010102, d3d12.DXGI_FORMAT_R10G10B10A2_UNORM},

		// Unknown
		{"Unknown", gputypes.VertexFormat(255), d3d12.DXGI_FORMAT_UNKNOWN},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := vertexFormatToD3D12(tt.format)
			if got != tt.expect {
				t.Errorf("vertexFormatToD3D12(%v) = %v, want %v", tt.format, got, tt.expect)
			}
		})
	}
}

func TestColorWriteMaskToD3D12(t *testing.T) {
	tests := []struct {
		name   string
		mask   gputypes.ColorWriteMask
		expect uint8
	}{
		{"Red", gputypes.ColorWriteMaskRed, uint8(d3d12.D3D12_COLOR_WRITE_ENABLE_RED)},
		{"Green", gputypes.ColorWriteMaskGreen, uint8(d3d12.D3D12_COLOR_WRITE_ENABLE_GREEN)},
		{"Blue", gputypes.ColorWriteMaskBlue, uint8(d3d12.D3D12_COLOR_WRITE_ENABLE_BLUE)},
		{"Alpha", gputypes.ColorWriteMaskAlpha, uint8(d3d12.D3D12_COLOR_WRITE_ENABLE_ALPHA)},
		{
			"All",
			gputypes.ColorWriteMaskRed | gputypes.ColorWriteMaskGreen | gputypes.ColorWriteMaskBlue | gputypes.ColorWriteMaskAlpha,
			uint8(d3d12.D3D12_COLOR_WRITE_ENABLE_RED) | uint8(d3d12.D3D12_COLOR_WRITE_ENABLE_GREEN) | uint8(d3d12.D3D12_COLOR_WRITE_ENABLE_BLUE) | uint8(d3d12.D3D12_COLOR_WRITE_ENABLE_ALPHA),
		},
		{"None", gputypes.ColorWriteMask(0), 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := colorWriteMaskToD3D12(tt.mask)
			if got != tt.expect {
				t.Errorf("colorWriteMaskToD3D12(%v) = %v, want %v", tt.mask, got, tt.expect)
			}
		})
	}
}

func TestShaderStagesToD3D12Visibility(t *testing.T) {
	tests := []struct {
		name   string
		stages gputypes.ShaderStages
		expect d3d12.D3D12_SHADER_VISIBILITY
	}{
		{"Vertex only", gputypes.ShaderStageVertex, d3d12.D3D12_SHADER_VISIBILITY_VERTEX},
		{"Fragment only", gputypes.ShaderStageFragment, d3d12.D3D12_SHADER_VISIBILITY_PIXEL},
		{"All stages", gputypes.ShaderStageVertex | gputypes.ShaderStageFragment | gputypes.ShaderStageCompute, d3d12.D3D12_SHADER_VISIBILITY_ALL},
		{"Vertex+Fragment", gputypes.ShaderStageVertex | gputypes.ShaderStageFragment, d3d12.D3D12_SHADER_VISIBILITY_ALL},
		{"Compute only", gputypes.ShaderStageCompute, d3d12.D3D12_SHADER_VISIBILITY_ALL},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shaderStagesToD3D12Visibility(tt.stages)
			if got != tt.expect {
				t.Errorf("shaderStagesToD3D12Visibility(%v) = %v, want %v", tt.stages, got, tt.expect)
			}
		})
	}
}
