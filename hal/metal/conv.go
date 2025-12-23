// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build darwin

package metal

import "github.com/gogpu/wgpu/types"

// textureFormatToMTL converts WebGPU texture format to Metal pixel format.
func textureFormatToMTL(format types.TextureFormat) MTLPixelFormat {
	switch format {
	case types.TextureFormatR8Unorm:
		return MTLPixelFormatR8Unorm
	case types.TextureFormatR8Snorm:
		return MTLPixelFormatR8Snorm
	case types.TextureFormatR8Uint:
		return MTLPixelFormatR8Uint
	case types.TextureFormatR8Sint:
		return MTLPixelFormatR8Sint
	case types.TextureFormatR16Uint:
		return MTLPixelFormatR16Uint
	case types.TextureFormatR16Sint:
		return MTLPixelFormatR16Sint
	case types.TextureFormatR16Float:
		return MTLPixelFormatR16Float
	case types.TextureFormatRG8Unorm:
		return MTLPixelFormatRG8Unorm
	case types.TextureFormatRG8Snorm:
		return MTLPixelFormatRG8Snorm
	case types.TextureFormatRG8Uint:
		return MTLPixelFormatRG8Uint
	case types.TextureFormatRG8Sint:
		return MTLPixelFormatRG8Sint
	case types.TextureFormatR32Uint:
		return MTLPixelFormatR32Uint
	case types.TextureFormatR32Sint:
		return MTLPixelFormatR32Sint
	case types.TextureFormatR32Float:
		return MTLPixelFormatR32Float
	case types.TextureFormatRG16Uint:
		return MTLPixelFormatRG16Uint
	case types.TextureFormatRG16Sint:
		return MTLPixelFormatRG16Sint
	case types.TextureFormatRG16Float:
		return MTLPixelFormatRG16Float
	case types.TextureFormatRGBA8Unorm:
		return MTLPixelFormatRGBA8Unorm
	case types.TextureFormatRGBA8UnormSrgb:
		return MTLPixelFormatRGBA8UnormSRGB
	case types.TextureFormatRGBA8Snorm:
		return MTLPixelFormatRGBA8Snorm
	case types.TextureFormatRGBA8Uint:
		return MTLPixelFormatRGBA8Uint
	case types.TextureFormatRGBA8Sint:
		return MTLPixelFormatRGBA8Sint
	case types.TextureFormatBGRA8Unorm:
		return MTLPixelFormatBGRA8Unorm
	case types.TextureFormatBGRA8UnormSrgb:
		return MTLPixelFormatBGRA8UnormSRGB
	case types.TextureFormatRGB10A2Unorm:
		return MTLPixelFormatRGB10A2Unorm
	case types.TextureFormatRG11B10Ufloat:
		return MTLPixelFormatRG11B10Float
	case types.TextureFormatRGB9E5Ufloat:
		return MTLPixelFormatRGB9E5Float
	case types.TextureFormatRG32Uint:
		return MTLPixelFormatRG32Uint
	case types.TextureFormatRG32Sint:
		return MTLPixelFormatRG32Sint
	case types.TextureFormatRG32Float:
		return MTLPixelFormatRG32Float
	case types.TextureFormatRGBA16Uint:
		return MTLPixelFormatRGBA16Uint
	case types.TextureFormatRGBA16Sint:
		return MTLPixelFormatRGBA16Sint
	case types.TextureFormatRGBA16Float:
		return MTLPixelFormatRGBA16Float
	case types.TextureFormatRGBA32Uint:
		return MTLPixelFormatRGBA32Uint
	case types.TextureFormatRGBA32Sint:
		return MTLPixelFormatRGBA32Sint
	case types.TextureFormatRGBA32Float:
		return MTLPixelFormatRGBA32Float
	case types.TextureFormatDepth16Unorm:
		return MTLPixelFormatDepth16Unorm
	case types.TextureFormatDepth32Float:
		return MTLPixelFormatDepth32Float
	case types.TextureFormatDepth24PlusStencil8:
		return MTLPixelFormatDepth24UnormStencil8
	case types.TextureFormatDepth32FloatStencil8:
		return MTLPixelFormatDepth32FloatStencil8
	case types.TextureFormatStencil8:
		return MTLPixelFormatStencil8
	default:
		return MTLPixelFormatInvalid
	}
}

// textureUsageToMTL converts WebGPU texture usage to Metal texture usage.
func textureUsageToMTL(usage types.TextureUsage) MTLTextureUsage {
	var mtlUsage MTLTextureUsage
	if usage&types.TextureUsageCopySrc != 0 || usage&types.TextureUsageCopyDst != 0 {
		mtlUsage |= MTLTextureUsageShaderRead
	}
	if usage&types.TextureUsageTextureBinding != 0 {
		mtlUsage |= MTLTextureUsageShaderRead
	}
	if usage&types.TextureUsageStorageBinding != 0 {
		mtlUsage |= MTLTextureUsageShaderRead | MTLTextureUsageShaderWrite
	}
	if usage&types.TextureUsageRenderAttachment != 0 {
		mtlUsage |= MTLTextureUsageRenderTarget
	}
	if mtlUsage == 0 {
		mtlUsage = MTLTextureUsageUnknown
	}
	return mtlUsage
}

// textureTypeFromDimension converts WebGPU texture dimension to Metal texture type.
func textureTypeFromDimension(dimension types.TextureDimension, sampleCount, depth uint32) MTLTextureType {
	switch dimension {
	case types.TextureDimension1D:
		if depth > 1 {
			return MTLTextureType1DArray
		}
		return MTLTextureType1D
	case types.TextureDimension2D:
		if sampleCount > 1 {
			return MTLTextureType2DMultisample
		}
		if depth > 1 {
			return MTLTextureType2DArray
		}
		return MTLTextureType2D
	case types.TextureDimension3D:
		return MTLTextureType3D
	default:
		return MTLTextureType2D
	}
}

// textureViewDimensionToMTL converts WebGPU texture view dimension to Metal texture type.
func textureViewDimensionToMTL(dimension types.TextureViewDimension) MTLTextureType {
	switch dimension {
	case types.TextureViewDimension1D:
		return MTLTextureType1D
	case types.TextureViewDimension2D:
		return MTLTextureType2D
	case types.TextureViewDimension2DArray:
		return MTLTextureType2DArray
	case types.TextureViewDimensionCube:
		return MTLTextureTypeCube
	case types.TextureViewDimensionCubeArray:
		return MTLTextureTypeCubeArray
	case types.TextureViewDimension3D:
		return MTLTextureType3D
	default:
		return MTLTextureType2D
	}
}

// filterModeToMTL converts WebGPU filter mode to Metal sampler filter.
func filterModeToMTL(mode types.FilterMode) MTLSamplerMinMagFilter {
	switch mode {
	case types.FilterModeNearest:
		return MTLSamplerMinMagFilterNearest
	case types.FilterModeLinear:
		return MTLSamplerMinMagFilterLinear
	default:
		return MTLSamplerMinMagFilterNearest
	}
}

// mipmapFilterModeToMTL converts WebGPU mipmap filter mode to Metal sampler mip filter.
func mipmapFilterModeToMTL(mode types.FilterMode) MTLSamplerMipFilter {
	switch mode {
	case types.FilterModeNearest:
		return MTLSamplerMipFilterNearest
	case types.FilterModeLinear:
		return MTLSamplerMipFilterLinear
	default:
		return MTLSamplerMipFilterNotMipmapped
	}
}

// addressModeToMTL converts WebGPU address mode to Metal sampler address mode.
func addressModeToMTL(mode types.AddressMode) MTLSamplerAddressMode {
	switch mode {
	case types.AddressModeClampToEdge:
		return MTLSamplerAddressModeClampToEdge
	case types.AddressModeRepeat:
		return MTLSamplerAddressModeRepeat
	case types.AddressModeMirrorRepeat:
		return MTLSamplerAddressModeMirrorRepeat
	default:
		return MTLSamplerAddressModeClampToEdge
	}
}

// compareFunctionToMTL converts WebGPU compare function to Metal compare function.
func compareFunctionToMTL(fn types.CompareFunction) MTLCompareFunction {
	switch fn {
	case types.CompareFunctionNever:
		return MTLCompareFunctionNever
	case types.CompareFunctionLess:
		return MTLCompareFunctionLess
	case types.CompareFunctionEqual:
		return MTLCompareFunctionEqual
	case types.CompareFunctionLessEqual:
		return MTLCompareFunctionLessEqual
	case types.CompareFunctionGreater:
		return MTLCompareFunctionGreater
	case types.CompareFunctionNotEqual:
		return MTLCompareFunctionNotEqual
	case types.CompareFunctionGreaterEqual:
		return MTLCompareFunctionGreaterEqual
	case types.CompareFunctionAlways:
		return MTLCompareFunctionAlways
	default:
		return MTLCompareFunctionAlways
	}
}

// primitiveTopologyToMTL converts WebGPU primitive topology to Metal primitive type.
func primitiveTopologyToMTL(topology types.PrimitiveTopology) MTLPrimitiveType {
	switch topology {
	case types.PrimitiveTopologyPointList:
		return MTLPrimitiveTypePoint
	case types.PrimitiveTopologyLineList:
		return MTLPrimitiveTypeLine
	case types.PrimitiveTopologyLineStrip:
		return MTLPrimitiveTypeLineStrip
	case types.PrimitiveTopologyTriangleList:
		return MTLPrimitiveTypeTriangle
	case types.PrimitiveTopologyTriangleStrip:
		return MTLPrimitiveTypeTriangleStrip
	default:
		return MTLPrimitiveTypeTriangle
	}
}

// blendFactorToMTL converts WebGPU blend factor to Metal blend factor.
func blendFactorToMTL(factor types.BlendFactor) MTLBlendFactor {
	switch factor {
	case types.BlendFactorZero:
		return MTLBlendFactorZero
	case types.BlendFactorOne:
		return MTLBlendFactorOne
	case types.BlendFactorSrc:
		return MTLBlendFactorSourceColor
	case types.BlendFactorOneMinusSrc:
		return MTLBlendFactorOneMinusSourceColor
	case types.BlendFactorSrcAlpha:
		return MTLBlendFactorSourceAlpha
	case types.BlendFactorOneMinusSrcAlpha:
		return MTLBlendFactorOneMinusSourceAlpha
	case types.BlendFactorDst:
		return MTLBlendFactorDestinationColor
	case types.BlendFactorOneMinusDst:
		return MTLBlendFactorOneMinusDestinationColor
	case types.BlendFactorDstAlpha:
		return MTLBlendFactorDestinationAlpha
	case types.BlendFactorOneMinusDstAlpha:
		return MTLBlendFactorOneMinusDestinationAlpha
	case types.BlendFactorSrcAlphaSaturated:
		return MTLBlendFactorSourceAlphaSaturated
	case types.BlendFactorConstant:
		return MTLBlendFactorBlendColor
	case types.BlendFactorOneMinusConstant:
		return MTLBlendFactorOneMinusBlendColor
	default:
		return MTLBlendFactorOne
	}
}

// blendOperationToMTL converts WebGPU blend operation to Metal blend operation.
func blendOperationToMTL(op types.BlendOperation) MTLBlendOperation {
	switch op {
	case types.BlendOperationAdd:
		return MTLBlendOperationAdd
	case types.BlendOperationSubtract:
		return MTLBlendOperationSubtract
	case types.BlendOperationReverseSubtract:
		return MTLBlendOperationReverseSubtract
	case types.BlendOperationMin:
		return MTLBlendOperationMin
	case types.BlendOperationMax:
		return MTLBlendOperationMax
	default:
		return MTLBlendOperationAdd
	}
}

// loadOpToMTL converts WebGPU load operation to Metal load action.
func loadOpToMTL(op types.LoadOp) MTLLoadAction {
	switch op {
	case types.LoadOpClear:
		return MTLLoadActionClear
	case types.LoadOpLoad:
		return MTLLoadActionLoad
	default:
		return MTLLoadActionDontCare
	}
}

// storeOpToMTL converts WebGPU store operation to Metal store action.
func storeOpToMTL(op types.StoreOp) MTLStoreAction {
	switch op {
	case types.StoreOpStore:
		return MTLStoreActionStore
	case types.StoreOpDiscard:
		return MTLStoreActionDontCare
	default:
		return MTLStoreActionStore
	}
}

// cullModeToMTL converts WebGPU cull mode to Metal cull mode.
func cullModeToMTL(mode types.CullMode) MTLCullMode {
	switch mode {
	case types.CullModeNone:
		return MTLCullModeNone
	case types.CullModeFront:
		return MTLCullModeFront
	case types.CullModeBack:
		return MTLCullModeBack
	default:
		return MTLCullModeNone
	}
}

// frontFaceToMTL converts WebGPU front face to Metal winding order.
func frontFaceToMTL(face types.FrontFace) MTLWinding {
	switch face {
	case types.FrontFaceCCW:
		return MTLWindingCounterClockwise
	case types.FrontFaceCW:
		return MTLWindingClockwise
	default:
		return MTLWindingCounterClockwise
	}
}

// indexFormatToMTL converts WebGPU index format to Metal index type.
func indexFormatToMTL(format types.IndexFormat) MTLIndexType {
	switch format {
	case types.IndexFormatUint16:
		return MTLIndexTypeUInt16
	case types.IndexFormatUint32:
		return MTLIndexTypeUInt32
	default:
		return MTLIndexTypeUInt32
	}
}
