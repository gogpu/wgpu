// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows

package dx12

import (
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/dx12/d3d12"
	"github.com/gogpu/gputypes"
)

// textureFormatToD3D12 converts a WebGPU texture format to D3D12 DXGI format.
func textureFormatToD3D12(format gputypes.TextureFormat) d3d12.DXGI_FORMAT {
	switch format {
	// 8-bit formats
	case gputypes.TextureFormatR8Unorm:
		return d3d12.DXGI_FORMAT_R8_UNORM
	case gputypes.TextureFormatR8Snorm:
		return d3d12.DXGI_FORMAT_R8_SNORM
	case gputypes.TextureFormatR8Uint:
		return d3d12.DXGI_FORMAT_R8_UINT
	case gputypes.TextureFormatR8Sint:
		return d3d12.DXGI_FORMAT_R8_SINT

	// 16-bit formats
	case gputypes.TextureFormatR16Uint:
		return d3d12.DXGI_FORMAT_R16_UINT
	case gputypes.TextureFormatR16Sint:
		return d3d12.DXGI_FORMAT_R16_SINT
	case gputypes.TextureFormatR16Float:
		return d3d12.DXGI_FORMAT_R16_FLOAT
	case gputypes.TextureFormatRG8Unorm:
		return d3d12.DXGI_FORMAT_R8G8_UNORM
	case gputypes.TextureFormatRG8Snorm:
		return d3d12.DXGI_FORMAT_R8G8_SNORM
	case gputypes.TextureFormatRG8Uint:
		return d3d12.DXGI_FORMAT_R8G8_UINT
	case gputypes.TextureFormatRG8Sint:
		return d3d12.DXGI_FORMAT_R8G8_SINT

	// 32-bit formats
	case gputypes.TextureFormatR32Uint:
		return d3d12.DXGI_FORMAT_R32_UINT
	case gputypes.TextureFormatR32Sint:
		return d3d12.DXGI_FORMAT_R32_SINT
	case gputypes.TextureFormatR32Float:
		return d3d12.DXGI_FORMAT_R32_FLOAT
	case gputypes.TextureFormatRG16Uint:
		return d3d12.DXGI_FORMAT_R16G16_UINT
	case gputypes.TextureFormatRG16Sint:
		return d3d12.DXGI_FORMAT_R16G16_SINT
	case gputypes.TextureFormatRG16Float:
		return d3d12.DXGI_FORMAT_R16G16_FLOAT
	case gputypes.TextureFormatRGBA8Unorm:
		return d3d12.DXGI_FORMAT_R8G8B8A8_UNORM
	case gputypes.TextureFormatRGBA8UnormSrgb:
		return d3d12.DXGI_FORMAT_R8G8B8A8_UNORM_SRGB
	case gputypes.TextureFormatRGBA8Snorm:
		return d3d12.DXGI_FORMAT_R8G8B8A8_SNORM
	case gputypes.TextureFormatRGBA8Uint:
		return d3d12.DXGI_FORMAT_R8G8B8A8_UINT
	case gputypes.TextureFormatRGBA8Sint:
		return d3d12.DXGI_FORMAT_R8G8B8A8_SINT
	case gputypes.TextureFormatBGRA8Unorm:
		return d3d12.DXGI_FORMAT_B8G8R8A8_UNORM
	case gputypes.TextureFormatBGRA8UnormSrgb:
		return d3d12.DXGI_FORMAT_B8G8R8A8_UNORM_SRGB

	// Packed formats
	case gputypes.TextureFormatRGB10A2Uint:
		return d3d12.DXGI_FORMAT_R10G10B10A2_UINT
	case gputypes.TextureFormatRGB10A2Unorm:
		return d3d12.DXGI_FORMAT_R10G10B10A2_UNORM
	case gputypes.TextureFormatRG11B10Ufloat:
		return d3d12.DXGI_FORMAT_R11G11B10_FLOAT

	// 64-bit formats
	case gputypes.TextureFormatRG32Uint:
		return d3d12.DXGI_FORMAT_R32G32_UINT
	case gputypes.TextureFormatRG32Sint:
		return d3d12.DXGI_FORMAT_R32G32_SINT
	case gputypes.TextureFormatRG32Float:
		return d3d12.DXGI_FORMAT_R32G32_FLOAT
	case gputypes.TextureFormatRGBA16Uint:
		return d3d12.DXGI_FORMAT_R16G16B16A16_UINT
	case gputypes.TextureFormatRGBA16Sint:
		return d3d12.DXGI_FORMAT_R16G16B16A16_SINT
	case gputypes.TextureFormatRGBA16Float:
		return d3d12.DXGI_FORMAT_R16G16B16A16_FLOAT

	// 128-bit formats
	case gputypes.TextureFormatRGBA32Uint:
		return d3d12.DXGI_FORMAT_R32G32B32A32_UINT
	case gputypes.TextureFormatRGBA32Sint:
		return d3d12.DXGI_FORMAT_R32G32B32A32_SINT
	case gputypes.TextureFormatRGBA32Float:
		return d3d12.DXGI_FORMAT_R32G32B32A32_FLOAT

	// Depth/stencil formats
	case gputypes.TextureFormatDepth16Unorm:
		return d3d12.DXGI_FORMAT_D16_UNORM
	case gputypes.TextureFormatDepth24Plus:
		return d3d12.DXGI_FORMAT_D24_UNORM_S8_UINT // D3D12 doesn't have D24 without stencil
	case gputypes.TextureFormatDepth24PlusStencil8:
		return d3d12.DXGI_FORMAT_D24_UNORM_S8_UINT
	case gputypes.TextureFormatDepth32Float:
		return d3d12.DXGI_FORMAT_D32_FLOAT
	case gputypes.TextureFormatDepth32FloatStencil8:
		return d3d12.DXGI_FORMAT_D32_FLOAT_S8X24_UINT
	case gputypes.TextureFormatStencil8:
		return d3d12.DXGI_FORMAT_D24_UNORM_S8_UINT // Use D24S8 and view only stencil

	// BC compressed formats
	case gputypes.TextureFormatBC1RGBAUnorm:
		return d3d12.DXGI_FORMAT_BC1_UNORM
	case gputypes.TextureFormatBC1RGBAUnormSrgb:
		return d3d12.DXGI_FORMAT_BC1_UNORM_SRGB
	case gputypes.TextureFormatBC2RGBAUnorm:
		return d3d12.DXGI_FORMAT_BC2_UNORM
	case gputypes.TextureFormatBC2RGBAUnormSrgb:
		return d3d12.DXGI_FORMAT_BC2_UNORM_SRGB
	case gputypes.TextureFormatBC3RGBAUnorm:
		return d3d12.DXGI_FORMAT_BC3_UNORM
	case gputypes.TextureFormatBC3RGBAUnormSrgb:
		return d3d12.DXGI_FORMAT_BC3_UNORM_SRGB
	case gputypes.TextureFormatBC4RUnorm:
		return d3d12.DXGI_FORMAT_BC4_UNORM
	case gputypes.TextureFormatBC4RSnorm:
		return d3d12.DXGI_FORMAT_BC4_SNORM
	case gputypes.TextureFormatBC5RGUnorm:
		return d3d12.DXGI_FORMAT_BC5_UNORM
	case gputypes.TextureFormatBC5RGSnorm:
		return d3d12.DXGI_FORMAT_BC5_SNORM
	case gputypes.TextureFormatBC6HRGBUfloat:
		return d3d12.DXGI_FORMAT_BC6H_UF16
	case gputypes.TextureFormatBC6HRGBFloat:
		return d3d12.DXGI_FORMAT_BC6H_SF16
	case gputypes.TextureFormatBC7RGBAUnorm:
		return d3d12.DXGI_FORMAT_BC7_UNORM
	case gputypes.TextureFormatBC7RGBAUnormSrgb:
		return d3d12.DXGI_FORMAT_BC7_UNORM_SRGB

	default:
		return d3d12.DXGI_FORMAT_UNKNOWN
	}
}

// textureDimensionToD3D12 converts a WebGPU texture dimension to D3D12 resource dimension.
func textureDimensionToD3D12(dim gputypes.TextureDimension) d3d12.D3D12_RESOURCE_DIMENSION {
	switch dim {
	case gputypes.TextureDimension1D:
		return d3d12.D3D12_RESOURCE_DIMENSION_TEXTURE1D
	case gputypes.TextureDimension2D:
		return d3d12.D3D12_RESOURCE_DIMENSION_TEXTURE2D
	case gputypes.TextureDimension3D:
		return d3d12.D3D12_RESOURCE_DIMENSION_TEXTURE3D
	default:
		return d3d12.D3D12_RESOURCE_DIMENSION_TEXTURE2D
	}
}

// textureViewDimensionToSRV converts a WebGPU texture view dimension to D3D12 SRV dimension.
func textureViewDimensionToSRV(dim gputypes.TextureViewDimension) d3d12.D3D12_SRV_DIMENSION {
	switch dim {
	case gputypes.TextureViewDimension1D:
		return d3d12.D3D12_SRV_DIMENSION_TEXTURE1D
	case gputypes.TextureViewDimension2D:
		return d3d12.D3D12_SRV_DIMENSION_TEXTURE2D
	case gputypes.TextureViewDimension2DArray:
		return d3d12.D3D12_SRV_DIMENSION_TEXTURE2DARRAY
	case gputypes.TextureViewDimensionCube:
		return d3d12.D3D12_SRV_DIMENSION_TEXTURECUBE
	case gputypes.TextureViewDimensionCubeArray:
		return d3d12.D3D12_SRV_DIMENSION_TEXTURECUBEARRAY
	case gputypes.TextureViewDimension3D:
		return d3d12.D3D12_SRV_DIMENSION_TEXTURE3D
	default:
		return d3d12.D3D12_SRV_DIMENSION_TEXTURE2D
	}
}

// textureViewDimensionToRTV converts a WebGPU texture view dimension to D3D12 RTV dimension.
func textureViewDimensionToRTV(dim gputypes.TextureViewDimension) d3d12.D3D12_RTV_DIMENSION {
	switch dim {
	case gputypes.TextureViewDimension1D:
		return d3d12.D3D12_RTV_DIMENSION_TEXTURE1D
	case gputypes.TextureViewDimension2D:
		return d3d12.D3D12_RTV_DIMENSION_TEXTURE2D
	case gputypes.TextureViewDimension2DArray:
		return d3d12.D3D12_RTV_DIMENSION_TEXTURE2DARRAY
	case gputypes.TextureViewDimension3D:
		return d3d12.D3D12_RTV_DIMENSION_TEXTURE3D
	default:
		return d3d12.D3D12_RTV_DIMENSION_TEXTURE2D
	}
}

// textureViewDimensionToDSV converts a WebGPU texture view dimension to D3D12 DSV dimension.
func textureViewDimensionToDSV(dim gputypes.TextureViewDimension) d3d12.D3D12_DSV_DIMENSION {
	switch dim {
	case gputypes.TextureViewDimension1D:
		return d3d12.D3D12_DSV_DIMENSION_TEXTURE1D
	case gputypes.TextureViewDimension2D:
		return d3d12.D3D12_DSV_DIMENSION_TEXTURE2D
	case gputypes.TextureViewDimension2DArray:
		return d3d12.D3D12_DSV_DIMENSION_TEXTURE2DARRAY
	default:
		return d3d12.D3D12_DSV_DIMENSION_TEXTURE2D
	}
}

// isDepthFormat returns true if the format is a depth/stencil format.
func isDepthFormat(format gputypes.TextureFormat) bool {
	switch format {
	case gputypes.TextureFormatDepth16Unorm,
		gputypes.TextureFormatDepth24Plus,
		gputypes.TextureFormatDepth24PlusStencil8,
		gputypes.TextureFormatDepth32Float,
		gputypes.TextureFormatDepth32FloatStencil8,
		gputypes.TextureFormatStencil8:
		return true
	default:
		return false
	}
}

// depthFormatToTypeless converts a depth format to its typeless equivalent for SRV.
func depthFormatToTypeless(format gputypes.TextureFormat) d3d12.DXGI_FORMAT {
	switch format {
	case gputypes.TextureFormatDepth16Unorm:
		return d3d12.DXGI_FORMAT_R16_TYPELESS
	case gputypes.TextureFormatDepth24Plus, gputypes.TextureFormatDepth24PlusStencil8:
		return d3d12.DXGI_FORMAT_R24G8_TYPELESS
	case gputypes.TextureFormatDepth32Float:
		return d3d12.DXGI_FORMAT_R32_TYPELESS
	case gputypes.TextureFormatDepth32FloatStencil8:
		return d3d12.DXGI_FORMAT_R32G8X24_TYPELESS
	default:
		return d3d12.DXGI_FORMAT_UNKNOWN
	}
}

// depthFormatToSRV converts a depth format to its SRV-compatible format.
func depthFormatToSRV(format gputypes.TextureFormat) d3d12.DXGI_FORMAT {
	switch format {
	case gputypes.TextureFormatDepth16Unorm:
		return d3d12.DXGI_FORMAT_R16_UNORM
	case gputypes.TextureFormatDepth24Plus, gputypes.TextureFormatDepth24PlusStencil8:
		return d3d12.DXGI_FORMAT_R24_UNORM_X8_TYPELESS
	case gputypes.TextureFormatDepth32Float:
		return d3d12.DXGI_FORMAT_R32_FLOAT
	case gputypes.TextureFormatDepth32FloatStencil8:
		return d3d12.DXGI_FORMAT_R32_FLOAT_X8X24_TYPELESS
	default:
		return d3d12.DXGI_FORMAT_UNKNOWN
	}
}

// addressModeToD3D12 converts a WebGPU address mode to D3D12.
func addressModeToD3D12(mode gputypes.AddressMode) d3d12.D3D12_TEXTURE_ADDRESS_MODE {
	switch mode {
	case gputypes.AddressModeRepeat:
		return d3d12.D3D12_TEXTURE_ADDRESS_MODE_WRAP
	case gputypes.AddressModeMirrorRepeat:
		return d3d12.D3D12_TEXTURE_ADDRESS_MODE_MIRROR
	case gputypes.AddressModeClampToEdge:
		return d3d12.D3D12_TEXTURE_ADDRESS_MODE_CLAMP
	default:
		return d3d12.D3D12_TEXTURE_ADDRESS_MODE_CLAMP
	}
}

// filterModeToD3D12 builds a D3D12 filter from WebGPU filter modes.
func filterModeToD3D12(minFilter, magFilter, mipmapFilter gputypes.FilterMode, compare gputypes.CompareFunction) d3d12.D3D12_FILTER {
	// Build filter from components
	var filter uint32

	// Minification filter
	if minFilter == gputypes.FilterModeLinear {
		filter |= 0x10 // D3D12_FILTER_MIN_LINEAR_*
	}

	// Magnification filter
	if magFilter == gputypes.FilterModeLinear {
		filter |= 0x04 // D3D12_FILTER_*_MAG_LINEAR_*
	}

	// Mipmap filter
	if mipmapFilter == gputypes.FilterModeLinear {
		filter |= 0x01 // D3D12_FILTER_*_MIP_LINEAR
	}

	// Comparison filter
	if compare != gputypes.CompareFunctionUndefined {
		filter |= 0x80 // D3D12_FILTER_COMPARISON_*
	}

	return d3d12.D3D12_FILTER(filter)
}

// compareFunctionToD3D12 converts a WebGPU compare function to D3D12.
func compareFunctionToD3D12(fn gputypes.CompareFunction) d3d12.D3D12_COMPARISON_FUNC {
	switch fn {
	case gputypes.CompareFunctionNever:
		return d3d12.D3D12_COMPARISON_FUNC_NEVER
	case gputypes.CompareFunctionLess:
		return d3d12.D3D12_COMPARISON_FUNC_LESS
	case gputypes.CompareFunctionEqual:
		return d3d12.D3D12_COMPARISON_FUNC_EQUAL
	case gputypes.CompareFunctionLessEqual:
		return d3d12.D3D12_COMPARISON_FUNC_LESS_EQUAL
	case gputypes.CompareFunctionGreater:
		return d3d12.D3D12_COMPARISON_FUNC_GREATER
	case gputypes.CompareFunctionNotEqual:
		return d3d12.D3D12_COMPARISON_FUNC_NOT_EQUAL
	case gputypes.CompareFunctionGreaterEqual:
		return d3d12.D3D12_COMPARISON_FUNC_GREATER_EQUAL
	case gputypes.CompareFunctionAlways:
		return d3d12.D3D12_COMPARISON_FUNC_ALWAYS
	default:
		return d3d12.D3D12_COMPARISON_FUNC_NEVER
	}
}

// alignTo256 aligns a size to 256 bytes (required for constant buffers).
func alignTo256(size uint64) uint64 {
	return (size + 255) &^ 255
}

// -----------------------------------------------------------------------------
// Pipeline Conversion Helpers
// -----------------------------------------------------------------------------

// blendFactorToD3D12 converts a WebGPU blend factor to D3D12.
func blendFactorToD3D12(factor gputypes.BlendFactor) d3d12.D3D12_BLEND {
	switch factor {
	case gputypes.BlendFactorZero:
		return d3d12.D3D12_BLEND_ZERO
	case gputypes.BlendFactorOne:
		return d3d12.D3D12_BLEND_ONE
	case gputypes.BlendFactorSrc:
		return d3d12.D3D12_BLEND_SRC_COLOR
	case gputypes.BlendFactorOneMinusSrc:
		return d3d12.D3D12_BLEND_INV_SRC_COLOR
	case gputypes.BlendFactorSrcAlpha:
		return d3d12.D3D12_BLEND_SRC_ALPHA
	case gputypes.BlendFactorOneMinusSrcAlpha:
		return d3d12.D3D12_BLEND_INV_SRC_ALPHA
	case gputypes.BlendFactorDst:
		return d3d12.D3D12_BLEND_DEST_COLOR
	case gputypes.BlendFactorOneMinusDst:
		return d3d12.D3D12_BLEND_INV_DEST_COLOR
	case gputypes.BlendFactorDstAlpha:
		return d3d12.D3D12_BLEND_DEST_ALPHA
	case gputypes.BlendFactorOneMinusDstAlpha:
		return d3d12.D3D12_BLEND_INV_DEST_ALPHA
	case gputypes.BlendFactorSrcAlphaSaturated:
		return d3d12.D3D12_BLEND_SRC_ALPHA_SAT
	case gputypes.BlendFactorConstant:
		return d3d12.D3D12_BLEND_BLEND_FACTOR
	case gputypes.BlendFactorOneMinusConstant:
		return d3d12.D3D12_BLEND_INV_BLEND_FACTOR
	default:
		return d3d12.D3D12_BLEND_ONE
	}
}

// blendOperationToD3D12 converts a WebGPU blend operation to D3D12.
func blendOperationToD3D12(op gputypes.BlendOperation) d3d12.D3D12_BLEND_OP {
	switch op {
	case gputypes.BlendOperationAdd:
		return d3d12.D3D12_BLEND_OP_ADD
	case gputypes.BlendOperationSubtract:
		return d3d12.D3D12_BLEND_OP_SUBTRACT
	case gputypes.BlendOperationReverseSubtract:
		return d3d12.D3D12_BLEND_OP_REV_SUBTRACT
	case gputypes.BlendOperationMin:
		return d3d12.D3D12_BLEND_OP_MIN
	case gputypes.BlendOperationMax:
		return d3d12.D3D12_BLEND_OP_MAX
	default:
		return d3d12.D3D12_BLEND_OP_ADD
	}
}

// cullModeToD3D12 converts a WebGPU cull mode to D3D12.
func cullModeToD3D12(mode gputypes.CullMode) d3d12.D3D12_CULL_MODE {
	switch mode {
	case gputypes.CullModeNone:
		return d3d12.D3D12_CULL_MODE_NONE
	case gputypes.CullModeFront:
		return d3d12.D3D12_CULL_MODE_FRONT
	case gputypes.CullModeBack:
		return d3d12.D3D12_CULL_MODE_BACK
	default:
		return d3d12.D3D12_CULL_MODE_NONE
	}
}

// frontFaceToD3D12 converts a WebGPU front face to D3D12 winding order.
// Returns 1 (TRUE) if counter-clockwise, 0 (FALSE) if clockwise.
func frontFaceToD3D12(face gputypes.FrontFace) int32 {
	if face == gputypes.FrontFaceCCW {
		return 1 // TRUE - counter-clockwise is front
	}
	return 0 // FALSE - clockwise is front
}

// primitiveTopologyTypeToD3D12 converts a WebGPU primitive topology to D3D12 topology type.
func primitiveTopologyTypeToD3D12(topology gputypes.PrimitiveTopology) d3d12.D3D12_PRIMITIVE_TOPOLOGY_TYPE {
	switch topology {
	case gputypes.PrimitiveTopologyPointList:
		return d3d12.D3D12_PRIMITIVE_TOPOLOGY_TYPE_POINT
	case gputypes.PrimitiveTopologyLineList, gputypes.PrimitiveTopologyLineStrip:
		return d3d12.D3D12_PRIMITIVE_TOPOLOGY_TYPE_LINE
	case gputypes.PrimitiveTopologyTriangleList, gputypes.PrimitiveTopologyTriangleStrip:
		return d3d12.D3D12_PRIMITIVE_TOPOLOGY_TYPE_TRIANGLE
	default:
		return d3d12.D3D12_PRIMITIVE_TOPOLOGY_TYPE_TRIANGLE
	}
}

// primitiveTopologyToD3D12 converts a WebGPU primitive topology to D3D12 primitive topology.
func primitiveTopologyToD3D12(topology gputypes.PrimitiveTopology) d3d12.D3D_PRIMITIVE_TOPOLOGY {
	switch topology {
	case gputypes.PrimitiveTopologyPointList:
		return d3d12.D3D_PRIMITIVE_TOPOLOGY_POINTLIST
	case gputypes.PrimitiveTopologyLineList:
		return d3d12.D3D_PRIMITIVE_TOPOLOGY_LINELIST
	case gputypes.PrimitiveTopologyLineStrip:
		return d3d12.D3D_PRIMITIVE_TOPOLOGY_LINESTRIP
	case gputypes.PrimitiveTopologyTriangleList:
		return d3d12.D3D_PRIMITIVE_TOPOLOGY_TRIANGLELIST
	case gputypes.PrimitiveTopologyTriangleStrip:
		return d3d12.D3D_PRIMITIVE_TOPOLOGY_TRIANGLESTRIP
	default:
		return d3d12.D3D_PRIMITIVE_TOPOLOGY_TRIANGLELIST
	}
}

// stencilOpToD3D12 converts a HAL stencil operation to D3D12.
func stencilOpToD3D12(op hal.StencilOperation) d3d12.D3D12_STENCIL_OP {
	switch op {
	case hal.StencilOperationKeep:
		return d3d12.D3D12_STENCIL_OP_KEEP
	case hal.StencilOperationZero:
		return d3d12.D3D12_STENCIL_OP_ZERO
	case hal.StencilOperationReplace:
		return d3d12.D3D12_STENCIL_OP_REPLACE
	case hal.StencilOperationInvert:
		return d3d12.D3D12_STENCIL_OP_INVERT
	case hal.StencilOperationIncrementClamp:
		return d3d12.D3D12_STENCIL_OP_INCR_SAT
	case hal.StencilOperationDecrementClamp:
		return d3d12.D3D12_STENCIL_OP_DECR_SAT
	case hal.StencilOperationIncrementWrap:
		return d3d12.D3D12_STENCIL_OP_INCR
	case hal.StencilOperationDecrementWrap:
		return d3d12.D3D12_STENCIL_OP_DECR
	default:
		return d3d12.D3D12_STENCIL_OP_KEEP
	}
}

// inputStepModeToD3D12 converts a WebGPU vertex step mode to D3D12 input classification.
func inputStepModeToD3D12(mode gputypes.VertexStepMode) d3d12.D3D12_INPUT_CLASSIFICATION {
	switch mode {
	case gputypes.VertexStepModeVertex:
		return d3d12.D3D12_INPUT_CLASSIFICATION_PER_VERTEX_DATA
	case gputypes.VertexStepModeInstance:
		return d3d12.D3D12_INPUT_CLASSIFICATION_PER_INSTANCE_DATA
	default:
		return d3d12.D3D12_INPUT_CLASSIFICATION_PER_VERTEX_DATA
	}
}

// vertexFormatToD3D12 converts a WebGPU vertex format to DXGI format.
func vertexFormatToD3D12(format gputypes.VertexFormat) d3d12.DXGI_FORMAT {
	switch format {
	case gputypes.VertexFormatUint8x2:
		return d3d12.DXGI_FORMAT_R8G8_UINT
	case gputypes.VertexFormatUint8x4:
		return d3d12.DXGI_FORMAT_R8G8B8A8_UINT
	case gputypes.VertexFormatSint8x2:
		return d3d12.DXGI_FORMAT_R8G8_SINT
	case gputypes.VertexFormatSint8x4:
		return d3d12.DXGI_FORMAT_R8G8B8A8_SINT
	case gputypes.VertexFormatUnorm8x2:
		return d3d12.DXGI_FORMAT_R8G8_UNORM
	case gputypes.VertexFormatUnorm8x4:
		return d3d12.DXGI_FORMAT_R8G8B8A8_UNORM
	case gputypes.VertexFormatSnorm8x2:
		return d3d12.DXGI_FORMAT_R8G8_SNORM
	case gputypes.VertexFormatSnorm8x4:
		return d3d12.DXGI_FORMAT_R8G8B8A8_SNORM
	case gputypes.VertexFormatUint16x2:
		return d3d12.DXGI_FORMAT_R16G16_UINT
	case gputypes.VertexFormatUint16x4:
		return d3d12.DXGI_FORMAT_R16G16B16A16_UINT
	case gputypes.VertexFormatSint16x2:
		return d3d12.DXGI_FORMAT_R16G16_SINT
	case gputypes.VertexFormatSint16x4:
		return d3d12.DXGI_FORMAT_R16G16B16A16_SINT
	case gputypes.VertexFormatUnorm16x2:
		return d3d12.DXGI_FORMAT_R16G16_UNORM
	case gputypes.VertexFormatUnorm16x4:
		return d3d12.DXGI_FORMAT_R16G16B16A16_UNORM
	case gputypes.VertexFormatSnorm16x2:
		return d3d12.DXGI_FORMAT_R16G16_SNORM
	case gputypes.VertexFormatSnorm16x4:
		return d3d12.DXGI_FORMAT_R16G16B16A16_SNORM
	case gputypes.VertexFormatFloat16x2:
		return d3d12.DXGI_FORMAT_R16G16_FLOAT
	case gputypes.VertexFormatFloat16x4:
		return d3d12.DXGI_FORMAT_R16G16B16A16_FLOAT
	case gputypes.VertexFormatFloat32:
		return d3d12.DXGI_FORMAT_R32_FLOAT
	case gputypes.VertexFormatFloat32x2:
		return d3d12.DXGI_FORMAT_R32G32_FLOAT
	case gputypes.VertexFormatFloat32x3:
		return d3d12.DXGI_FORMAT_R32G32B32_FLOAT
	case gputypes.VertexFormatFloat32x4:
		return d3d12.DXGI_FORMAT_R32G32B32A32_FLOAT
	case gputypes.VertexFormatUint32:
		return d3d12.DXGI_FORMAT_R32_UINT
	case gputypes.VertexFormatUint32x2:
		return d3d12.DXGI_FORMAT_R32G32_UINT
	case gputypes.VertexFormatUint32x3:
		return d3d12.DXGI_FORMAT_R32G32B32_UINT
	case gputypes.VertexFormatUint32x4:
		return d3d12.DXGI_FORMAT_R32G32B32A32_UINT
	case gputypes.VertexFormatSint32:
		return d3d12.DXGI_FORMAT_R32_SINT
	case gputypes.VertexFormatSint32x2:
		return d3d12.DXGI_FORMAT_R32G32_SINT
	case gputypes.VertexFormatSint32x3:
		return d3d12.DXGI_FORMAT_R32G32B32_SINT
	case gputypes.VertexFormatSint32x4:
		return d3d12.DXGI_FORMAT_R32G32B32A32_SINT
	case gputypes.VertexFormatUnorm1010102:
		return d3d12.DXGI_FORMAT_R10G10B10A2_UNORM
	default:
		return d3d12.DXGI_FORMAT_UNKNOWN
	}
}

// colorWriteMaskToD3D12 converts a WebGPU color write mask to D3D12.
func colorWriteMaskToD3D12(mask gputypes.ColorWriteMask) uint8 {
	var d3d12Mask uint8
	if mask&gputypes.ColorWriteMaskRed != 0 {
		d3d12Mask |= uint8(d3d12.D3D12_COLOR_WRITE_ENABLE_RED)
	}
	if mask&gputypes.ColorWriteMaskGreen != 0 {
		d3d12Mask |= uint8(d3d12.D3D12_COLOR_WRITE_ENABLE_GREEN)
	}
	if mask&gputypes.ColorWriteMaskBlue != 0 {
		d3d12Mask |= uint8(d3d12.D3D12_COLOR_WRITE_ENABLE_BLUE)
	}
	if mask&gputypes.ColorWriteMaskAlpha != 0 {
		d3d12Mask |= uint8(d3d12.D3D12_COLOR_WRITE_ENABLE_ALPHA)
	}
	return d3d12Mask
}

// shaderStagesToD3D12Visibility converts WebGPU shader stages to D3D12 shader visibility.
//
//nolint:unused // Will be used when bind groups are fully implemented
func shaderStagesToD3D12Visibility(stages gputypes.ShaderStages) d3d12.D3D12_SHADER_VISIBILITY {
	// If all stages, use ALL
	if stages&(gputypes.ShaderStageVertex|gputypes.ShaderStageFragment|gputypes.ShaderStageCompute) ==
		(gputypes.ShaderStageVertex | gputypes.ShaderStageFragment | gputypes.ShaderStageCompute) {
		return d3d12.D3D12_SHADER_VISIBILITY_ALL
	}

	// If only vertex
	if stages == gputypes.ShaderStageVertex {
		return d3d12.D3D12_SHADER_VISIBILITY_VERTEX
	}

	// If only fragment (pixel)
	if stages == gputypes.ShaderStageFragment {
		return d3d12.D3D12_SHADER_VISIBILITY_PIXEL
	}

	// For compute, we don't use shader visibility (compute uses separate root signature)
	// For combinations, use ALL
	return d3d12.D3D12_SHADER_VISIBILITY_ALL
}
