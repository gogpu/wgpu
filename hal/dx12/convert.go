// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows

package dx12

import (
	"github.com/gogpu/wgpu/hal/dx12/d3d12"
	"github.com/gogpu/wgpu/types"
)

// textureFormatToD3D12 converts a WebGPU texture format to D3D12 DXGI format.
func textureFormatToD3D12(format types.TextureFormat) d3d12.DXGI_FORMAT {
	switch format {
	// 8-bit formats
	case types.TextureFormatR8Unorm:
		return d3d12.DXGI_FORMAT_R8_UNORM
	case types.TextureFormatR8Snorm:
		return d3d12.DXGI_FORMAT_R8_SNORM
	case types.TextureFormatR8Uint:
		return d3d12.DXGI_FORMAT_R8_UINT
	case types.TextureFormatR8Sint:
		return d3d12.DXGI_FORMAT_R8_SINT

	// 16-bit formats
	case types.TextureFormatR16Uint:
		return d3d12.DXGI_FORMAT_R16_UINT
	case types.TextureFormatR16Sint:
		return d3d12.DXGI_FORMAT_R16_SINT
	case types.TextureFormatR16Float:
		return d3d12.DXGI_FORMAT_R16_FLOAT
	case types.TextureFormatRG8Unorm:
		return d3d12.DXGI_FORMAT_R8G8_UNORM
	case types.TextureFormatRG8Snorm:
		return d3d12.DXGI_FORMAT_R8G8_SNORM
	case types.TextureFormatRG8Uint:
		return d3d12.DXGI_FORMAT_R8G8_UINT
	case types.TextureFormatRG8Sint:
		return d3d12.DXGI_FORMAT_R8G8_SINT

	// 32-bit formats
	case types.TextureFormatR32Uint:
		return d3d12.DXGI_FORMAT_R32_UINT
	case types.TextureFormatR32Sint:
		return d3d12.DXGI_FORMAT_R32_SINT
	case types.TextureFormatR32Float:
		return d3d12.DXGI_FORMAT_R32_FLOAT
	case types.TextureFormatRG16Uint:
		return d3d12.DXGI_FORMAT_R16G16_UINT
	case types.TextureFormatRG16Sint:
		return d3d12.DXGI_FORMAT_R16G16_SINT
	case types.TextureFormatRG16Float:
		return d3d12.DXGI_FORMAT_R16G16_FLOAT
	case types.TextureFormatRGBA8Unorm:
		return d3d12.DXGI_FORMAT_R8G8B8A8_UNORM
	case types.TextureFormatRGBA8UnormSrgb:
		return d3d12.DXGI_FORMAT_R8G8B8A8_UNORM_SRGB
	case types.TextureFormatRGBA8Snorm:
		return d3d12.DXGI_FORMAT_R8G8B8A8_SNORM
	case types.TextureFormatRGBA8Uint:
		return d3d12.DXGI_FORMAT_R8G8B8A8_UINT
	case types.TextureFormatRGBA8Sint:
		return d3d12.DXGI_FORMAT_R8G8B8A8_SINT
	case types.TextureFormatBGRA8Unorm:
		return d3d12.DXGI_FORMAT_B8G8R8A8_UNORM
	case types.TextureFormatBGRA8UnormSrgb:
		return d3d12.DXGI_FORMAT_B8G8R8A8_UNORM_SRGB

	// Packed formats
	case types.TextureFormatRGB10A2Uint:
		return d3d12.DXGI_FORMAT_R10G10B10A2_UINT
	case types.TextureFormatRGB10A2Unorm:
		return d3d12.DXGI_FORMAT_R10G10B10A2_UNORM
	case types.TextureFormatRG11B10Ufloat:
		return d3d12.DXGI_FORMAT_R11G11B10_FLOAT

	// 64-bit formats
	case types.TextureFormatRG32Uint:
		return d3d12.DXGI_FORMAT_R32G32_UINT
	case types.TextureFormatRG32Sint:
		return d3d12.DXGI_FORMAT_R32G32_SINT
	case types.TextureFormatRG32Float:
		return d3d12.DXGI_FORMAT_R32G32_FLOAT
	case types.TextureFormatRGBA16Uint:
		return d3d12.DXGI_FORMAT_R16G16B16A16_UINT
	case types.TextureFormatRGBA16Sint:
		return d3d12.DXGI_FORMAT_R16G16B16A16_SINT
	case types.TextureFormatRGBA16Float:
		return d3d12.DXGI_FORMAT_R16G16B16A16_FLOAT

	// 128-bit formats
	case types.TextureFormatRGBA32Uint:
		return d3d12.DXGI_FORMAT_R32G32B32A32_UINT
	case types.TextureFormatRGBA32Sint:
		return d3d12.DXGI_FORMAT_R32G32B32A32_SINT
	case types.TextureFormatRGBA32Float:
		return d3d12.DXGI_FORMAT_R32G32B32A32_FLOAT

	// Depth/stencil formats
	case types.TextureFormatDepth16Unorm:
		return d3d12.DXGI_FORMAT_D16_UNORM
	case types.TextureFormatDepth24Plus:
		return d3d12.DXGI_FORMAT_D24_UNORM_S8_UINT // D3D12 doesn't have D24 without stencil
	case types.TextureFormatDepth24PlusStencil8:
		return d3d12.DXGI_FORMAT_D24_UNORM_S8_UINT
	case types.TextureFormatDepth32Float:
		return d3d12.DXGI_FORMAT_D32_FLOAT
	case types.TextureFormatDepth32FloatStencil8:
		return d3d12.DXGI_FORMAT_D32_FLOAT_S8X24_UINT
	case types.TextureFormatStencil8:
		return d3d12.DXGI_FORMAT_D24_UNORM_S8_UINT // Use D24S8 and view only stencil

	// BC compressed formats
	case types.TextureFormatBC1RGBAUnorm:
		return d3d12.DXGI_FORMAT_BC1_UNORM
	case types.TextureFormatBC1RGBAUnormSrgb:
		return d3d12.DXGI_FORMAT_BC1_UNORM_SRGB
	case types.TextureFormatBC2RGBAUnorm:
		return d3d12.DXGI_FORMAT_BC2_UNORM
	case types.TextureFormatBC2RGBAUnormSrgb:
		return d3d12.DXGI_FORMAT_BC2_UNORM_SRGB
	case types.TextureFormatBC3RGBAUnorm:
		return d3d12.DXGI_FORMAT_BC3_UNORM
	case types.TextureFormatBC3RGBAUnormSrgb:
		return d3d12.DXGI_FORMAT_BC3_UNORM_SRGB
	case types.TextureFormatBC4RUnorm:
		return d3d12.DXGI_FORMAT_BC4_UNORM
	case types.TextureFormatBC4RSnorm:
		return d3d12.DXGI_FORMAT_BC4_SNORM
	case types.TextureFormatBC5RGUnorm:
		return d3d12.DXGI_FORMAT_BC5_UNORM
	case types.TextureFormatBC5RGSnorm:
		return d3d12.DXGI_FORMAT_BC5_SNORM
	case types.TextureFormatBC6HRGBUfloat:
		return d3d12.DXGI_FORMAT_BC6H_UF16
	case types.TextureFormatBC6HRGBFloat:
		return d3d12.DXGI_FORMAT_BC6H_SF16
	case types.TextureFormatBC7RGBAUnorm:
		return d3d12.DXGI_FORMAT_BC7_UNORM
	case types.TextureFormatBC7RGBAUnormSrgb:
		return d3d12.DXGI_FORMAT_BC7_UNORM_SRGB

	default:
		return d3d12.DXGI_FORMAT_UNKNOWN
	}
}

// textureDimensionToD3D12 converts a WebGPU texture dimension to D3D12 resource dimension.
func textureDimensionToD3D12(dim types.TextureDimension) d3d12.D3D12_RESOURCE_DIMENSION {
	switch dim {
	case types.TextureDimension1D:
		return d3d12.D3D12_RESOURCE_DIMENSION_TEXTURE1D
	case types.TextureDimension2D:
		return d3d12.D3D12_RESOURCE_DIMENSION_TEXTURE2D
	case types.TextureDimension3D:
		return d3d12.D3D12_RESOURCE_DIMENSION_TEXTURE3D
	default:
		return d3d12.D3D12_RESOURCE_DIMENSION_TEXTURE2D
	}
}

// textureViewDimensionToSRV converts a WebGPU texture view dimension to D3D12 SRV dimension.
func textureViewDimensionToSRV(dim types.TextureViewDimension) d3d12.D3D12_SRV_DIMENSION {
	switch dim {
	case types.TextureViewDimension1D:
		return d3d12.D3D12_SRV_DIMENSION_TEXTURE1D
	case types.TextureViewDimension2D:
		return d3d12.D3D12_SRV_DIMENSION_TEXTURE2D
	case types.TextureViewDimension2DArray:
		return d3d12.D3D12_SRV_DIMENSION_TEXTURE2DARRAY
	case types.TextureViewDimensionCube:
		return d3d12.D3D12_SRV_DIMENSION_TEXTURECUBE
	case types.TextureViewDimensionCubeArray:
		return d3d12.D3D12_SRV_DIMENSION_TEXTURECUBEARRAY
	case types.TextureViewDimension3D:
		return d3d12.D3D12_SRV_DIMENSION_TEXTURE3D
	default:
		return d3d12.D3D12_SRV_DIMENSION_TEXTURE2D
	}
}

// textureViewDimensionToRTV converts a WebGPU texture view dimension to D3D12 RTV dimension.
func textureViewDimensionToRTV(dim types.TextureViewDimension) d3d12.D3D12_RTV_DIMENSION {
	switch dim {
	case types.TextureViewDimension1D:
		return d3d12.D3D12_RTV_DIMENSION_TEXTURE1D
	case types.TextureViewDimension2D:
		return d3d12.D3D12_RTV_DIMENSION_TEXTURE2D
	case types.TextureViewDimension2DArray:
		return d3d12.D3D12_RTV_DIMENSION_TEXTURE2DARRAY
	case types.TextureViewDimension3D:
		return d3d12.D3D12_RTV_DIMENSION_TEXTURE3D
	default:
		return d3d12.D3D12_RTV_DIMENSION_TEXTURE2D
	}
}

// textureViewDimensionToDSV converts a WebGPU texture view dimension to D3D12 DSV dimension.
func textureViewDimensionToDSV(dim types.TextureViewDimension) d3d12.D3D12_DSV_DIMENSION {
	switch dim {
	case types.TextureViewDimension1D:
		return d3d12.D3D12_DSV_DIMENSION_TEXTURE1D
	case types.TextureViewDimension2D:
		return d3d12.D3D12_DSV_DIMENSION_TEXTURE2D
	case types.TextureViewDimension2DArray:
		return d3d12.D3D12_DSV_DIMENSION_TEXTURE2DARRAY
	default:
		return d3d12.D3D12_DSV_DIMENSION_TEXTURE2D
	}
}

// isDepthFormat returns true if the format is a depth/stencil format.
func isDepthFormat(format types.TextureFormat) bool {
	switch format {
	case types.TextureFormatDepth16Unorm,
		types.TextureFormatDepth24Plus,
		types.TextureFormatDepth24PlusStencil8,
		types.TextureFormatDepth32Float,
		types.TextureFormatDepth32FloatStencil8,
		types.TextureFormatStencil8:
		return true
	default:
		return false
	}
}


// depthFormatToTypeless converts a depth format to its typeless equivalent for SRV.
func depthFormatToTypeless(format types.TextureFormat) d3d12.DXGI_FORMAT {
	switch format {
	case types.TextureFormatDepth16Unorm:
		return d3d12.DXGI_FORMAT_R16_TYPELESS
	case types.TextureFormatDepth24Plus, types.TextureFormatDepth24PlusStencil8:
		return d3d12.DXGI_FORMAT_R24G8_TYPELESS
	case types.TextureFormatDepth32Float:
		return d3d12.DXGI_FORMAT_R32_TYPELESS
	case types.TextureFormatDepth32FloatStencil8:
		return d3d12.DXGI_FORMAT_R32G8X24_TYPELESS
	default:
		return d3d12.DXGI_FORMAT_UNKNOWN
	}
}

// depthFormatToSRV converts a depth format to its SRV-compatible format.
func depthFormatToSRV(format types.TextureFormat) d3d12.DXGI_FORMAT {
	switch format {
	case types.TextureFormatDepth16Unorm:
		return d3d12.DXGI_FORMAT_R16_UNORM
	case types.TextureFormatDepth24Plus, types.TextureFormatDepth24PlusStencil8:
		return d3d12.DXGI_FORMAT_R24_UNORM_X8_TYPELESS
	case types.TextureFormatDepth32Float:
		return d3d12.DXGI_FORMAT_R32_FLOAT
	case types.TextureFormatDepth32FloatStencil8:
		return d3d12.DXGI_FORMAT_R32_FLOAT_X8X24_TYPELESS
	default:
		return d3d12.DXGI_FORMAT_UNKNOWN
	}
}

// addressModeToD3D12 converts a WebGPU address mode to D3D12.
func addressModeToD3D12(mode types.AddressMode) d3d12.D3D12_TEXTURE_ADDRESS_MODE {
	switch mode {
	case types.AddressModeRepeat:
		return d3d12.D3D12_TEXTURE_ADDRESS_MODE_WRAP
	case types.AddressModeMirrorRepeat:
		return d3d12.D3D12_TEXTURE_ADDRESS_MODE_MIRROR
	case types.AddressModeClampToEdge:
		return d3d12.D3D12_TEXTURE_ADDRESS_MODE_CLAMP
	default:
		return d3d12.D3D12_TEXTURE_ADDRESS_MODE_CLAMP
	}
}

// filterModeToD3D12 builds a D3D12 filter from WebGPU filter modes.
func filterModeToD3D12(minFilter, magFilter, mipmapFilter types.FilterMode, compare types.CompareFunction) d3d12.D3D12_FILTER {
	// Build filter from components
	var filter uint32

	// Minification filter
	if minFilter == types.FilterModeLinear {
		filter |= 0x10 // D3D12_FILTER_MIN_LINEAR_*
	}

	// Magnification filter
	if magFilter == types.FilterModeLinear {
		filter |= 0x04 // D3D12_FILTER_*_MAG_LINEAR_*
	}

	// Mipmap filter
	if mipmapFilter == types.FilterModeLinear {
		filter |= 0x01 // D3D12_FILTER_*_MIP_LINEAR
	}

	// Comparison filter
	if compare != types.CompareFunctionUndefined {
		filter |= 0x80 // D3D12_FILTER_COMPARISON_*
	}

	return d3d12.D3D12_FILTER(filter)
}

// compareFunctionToD3D12 converts a WebGPU compare function to D3D12.
func compareFunctionToD3D12(fn types.CompareFunction) d3d12.D3D12_COMPARISON_FUNC {
	switch fn {
	case types.CompareFunctionNever:
		return d3d12.D3D12_COMPARISON_FUNC_NEVER
	case types.CompareFunctionLess:
		return d3d12.D3D12_COMPARISON_FUNC_LESS
	case types.CompareFunctionEqual:
		return d3d12.D3D12_COMPARISON_FUNC_EQUAL
	case types.CompareFunctionLessEqual:
		return d3d12.D3D12_COMPARISON_FUNC_LESS_EQUAL
	case types.CompareFunctionGreater:
		return d3d12.D3D12_COMPARISON_FUNC_GREATER
	case types.CompareFunctionNotEqual:
		return d3d12.D3D12_COMPARISON_FUNC_NOT_EQUAL
	case types.CompareFunctionGreaterEqual:
		return d3d12.D3D12_COMPARISON_FUNC_GREATER_EQUAL
	case types.CompareFunctionAlways:
		return d3d12.D3D12_COMPARISON_FUNC_ALWAYS
	default:
		return d3d12.D3D12_COMPARISON_FUNC_NEVER
	}
}

// alignTo256 aligns a size to 256 bytes (required for constant buffers).
func alignTo256(size uint64) uint64 {
	return (size + 255) &^ 255
}
