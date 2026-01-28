// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build darwin

package metal

import "github.com/gogpu/gputypes"

// textureFormatToMTL converts WebGPU texture format to Metal pixel format.
func textureFormatToMTL(format gputypes.TextureFormat) MTLPixelFormat {
	switch format {
	case gputypes.TextureFormatR8Unorm:
		return MTLPixelFormatR8Unorm
	case gputypes.TextureFormatR8Snorm:
		return MTLPixelFormatR8Snorm
	case gputypes.TextureFormatR8Uint:
		return MTLPixelFormatR8Uint
	case gputypes.TextureFormatR8Sint:
		return MTLPixelFormatR8Sint
	case gputypes.TextureFormatR16Uint:
		return MTLPixelFormatR16Uint
	case gputypes.TextureFormatR16Sint:
		return MTLPixelFormatR16Sint
	case gputypes.TextureFormatR16Float:
		return MTLPixelFormatR16Float
	case gputypes.TextureFormatRG8Unorm:
		return MTLPixelFormatRG8Unorm
	case gputypes.TextureFormatRG8Snorm:
		return MTLPixelFormatRG8Snorm
	case gputypes.TextureFormatRG8Uint:
		return MTLPixelFormatRG8Uint
	case gputypes.TextureFormatRG8Sint:
		return MTLPixelFormatRG8Sint
	case gputypes.TextureFormatR32Uint:
		return MTLPixelFormatR32Uint
	case gputypes.TextureFormatR32Sint:
		return MTLPixelFormatR32Sint
	case gputypes.TextureFormatR32Float:
		return MTLPixelFormatR32Float
	case gputypes.TextureFormatRG16Uint:
		return MTLPixelFormatRG16Uint
	case gputypes.TextureFormatRG16Sint:
		return MTLPixelFormatRG16Sint
	case gputypes.TextureFormatRG16Float:
		return MTLPixelFormatRG16Float
	case gputypes.TextureFormatRGBA8Unorm:
		return MTLPixelFormatRGBA8Unorm
	case gputypes.TextureFormatRGBA8UnormSrgb:
		return MTLPixelFormatRGBA8UnormSRGB
	case gputypes.TextureFormatRGBA8Snorm:
		return MTLPixelFormatRGBA8Snorm
	case gputypes.TextureFormatRGBA8Uint:
		return MTLPixelFormatRGBA8Uint
	case gputypes.TextureFormatRGBA8Sint:
		return MTLPixelFormatRGBA8Sint
	case gputypes.TextureFormatBGRA8Unorm:
		return MTLPixelFormatBGRA8Unorm
	case gputypes.TextureFormatBGRA8UnormSrgb:
		return MTLPixelFormatBGRA8UnormSRGB
	case gputypes.TextureFormatRGB10A2Unorm:
		return MTLPixelFormatRGB10A2Unorm
	case gputypes.TextureFormatRG11B10Ufloat:
		return MTLPixelFormatRG11B10Float
	case gputypes.TextureFormatRGB9E5Ufloat:
		return MTLPixelFormatRGB9E5Float
	case gputypes.TextureFormatRG32Uint:
		return MTLPixelFormatRG32Uint
	case gputypes.TextureFormatRG32Sint:
		return MTLPixelFormatRG32Sint
	case gputypes.TextureFormatRG32Float:
		return MTLPixelFormatRG32Float
	case gputypes.TextureFormatRGBA16Uint:
		return MTLPixelFormatRGBA16Uint
	case gputypes.TextureFormatRGBA16Sint:
		return MTLPixelFormatRGBA16Sint
	case gputypes.TextureFormatRGBA16Float:
		return MTLPixelFormatRGBA16Float
	case gputypes.TextureFormatRGBA32Uint:
		return MTLPixelFormatRGBA32Uint
	case gputypes.TextureFormatRGBA32Sint:
		return MTLPixelFormatRGBA32Sint
	case gputypes.TextureFormatRGBA32Float:
		return MTLPixelFormatRGBA32Float
	case gputypes.TextureFormatDepth16Unorm:
		return MTLPixelFormatDepth16Unorm
	case gputypes.TextureFormatDepth32Float:
		return MTLPixelFormatDepth32Float
	case gputypes.TextureFormatDepth24PlusStencil8:
		return MTLPixelFormatDepth24UnormStencil8
	case gputypes.TextureFormatDepth32FloatStencil8:
		return MTLPixelFormatDepth32FloatStencil8
	case gputypes.TextureFormatStencil8:
		return MTLPixelFormatStencil8
	default:
		return MTLPixelFormatInvalid
	}
}

// textureUsageToMTL converts WebGPU texture usage to Metal texture usage.
func textureUsageToMTL(usage gputypes.TextureUsage) MTLTextureUsage {
	var mtlUsage MTLTextureUsage
	if usage&gputypes.TextureUsageCopySrc != 0 || usage&gputypes.TextureUsageCopyDst != 0 {
		mtlUsage |= MTLTextureUsageShaderRead
	}
	if usage&gputypes.TextureUsageTextureBinding != 0 {
		mtlUsage |= MTLTextureUsageShaderRead
	}
	if usage&gputypes.TextureUsageStorageBinding != 0 {
		mtlUsage |= MTLTextureUsageShaderRead | MTLTextureUsageShaderWrite
	}
	if usage&gputypes.TextureUsageRenderAttachment != 0 {
		mtlUsage |= MTLTextureUsageRenderTarget
	}
	if mtlUsage == 0 {
		mtlUsage = MTLTextureUsageUnknown
	}
	return mtlUsage
}

// textureTypeFromDimension converts WebGPU texture dimension to Metal texture type.
func textureTypeFromDimension(dimension gputypes.TextureDimension, sampleCount, depth uint32) MTLTextureType {
	switch dimension {
	case gputypes.TextureDimension1D:
		if depth > 1 {
			return MTLTextureType1DArray
		}
		return MTLTextureType1D
	case gputypes.TextureDimension2D:
		if sampleCount > 1 {
			return MTLTextureType2DMultisample
		}
		if depth > 1 {
			return MTLTextureType2DArray
		}
		return MTLTextureType2D
	case gputypes.TextureDimension3D:
		return MTLTextureType3D
	default:
		return MTLTextureType2D
	}
}

// textureViewDimensionToMTL converts WebGPU texture view dimension to Metal texture type.
func textureViewDimensionToMTL(dimension gputypes.TextureViewDimension) MTLTextureType {
	switch dimension {
	case gputypes.TextureViewDimension1D:
		return MTLTextureType1D
	case gputypes.TextureViewDimension2D:
		return MTLTextureType2D
	case gputypes.TextureViewDimension2DArray:
		return MTLTextureType2DArray
	case gputypes.TextureViewDimensionCube:
		return MTLTextureTypeCube
	case gputypes.TextureViewDimensionCubeArray:
		return MTLTextureTypeCubeArray
	case gputypes.TextureViewDimension3D:
		return MTLTextureType3D
	default:
		return MTLTextureType2D
	}
}

// filterModeToMTL converts WebGPU filter mode to Metal sampler filter.
func filterModeToMTL(mode gputypes.FilterMode) MTLSamplerMinMagFilter {
	switch mode {
	case gputypes.FilterModeNearest:
		return MTLSamplerMinMagFilterNearest
	case gputypes.FilterModeLinear:
		return MTLSamplerMinMagFilterLinear
	default:
		return MTLSamplerMinMagFilterNearest
	}
}

// mipmapFilterModeToMTL converts WebGPU mipmap filter mode to Metal sampler mip filter.
func mipmapFilterModeToMTL(mode gputypes.FilterMode) MTLSamplerMipFilter {
	switch mode {
	case gputypes.FilterModeNearest:
		return MTLSamplerMipFilterNearest
	case gputypes.FilterModeLinear:
		return MTLSamplerMipFilterLinear
	default:
		return MTLSamplerMipFilterNotMipmapped
	}
}

// addressModeToMTL converts WebGPU address mode to Metal sampler address mode.
func addressModeToMTL(mode gputypes.AddressMode) MTLSamplerAddressMode {
	switch mode {
	case gputypes.AddressModeClampToEdge:
		return MTLSamplerAddressModeClampToEdge
	case gputypes.AddressModeRepeat:
		return MTLSamplerAddressModeRepeat
	case gputypes.AddressModeMirrorRepeat:
		return MTLSamplerAddressModeMirrorRepeat
	default:
		return MTLSamplerAddressModeClampToEdge
	}
}

// compareFunctionToMTL converts WebGPU compare function to Metal compare function.
func compareFunctionToMTL(fn gputypes.CompareFunction) MTLCompareFunction {
	switch fn {
	case gputypes.CompareFunctionNever:
		return MTLCompareFunctionNever
	case gputypes.CompareFunctionLess:
		return MTLCompareFunctionLess
	case gputypes.CompareFunctionEqual:
		return MTLCompareFunctionEqual
	case gputypes.CompareFunctionLessEqual:
		return MTLCompareFunctionLessEqual
	case gputypes.CompareFunctionGreater:
		return MTLCompareFunctionGreater
	case gputypes.CompareFunctionNotEqual:
		return MTLCompareFunctionNotEqual
	case gputypes.CompareFunctionGreaterEqual:
		return MTLCompareFunctionGreaterEqual
	case gputypes.CompareFunctionAlways:
		return MTLCompareFunctionAlways
	default:
		return MTLCompareFunctionAlways
	}
}

// primitiveTopologyToMTL converts WebGPU primitive topology to Metal primitive type.
func primitiveTopologyToMTL(topology gputypes.PrimitiveTopology) MTLPrimitiveType {
	switch topology {
	case gputypes.PrimitiveTopologyPointList:
		return MTLPrimitiveTypePoint
	case gputypes.PrimitiveTopologyLineList:
		return MTLPrimitiveTypeLine
	case gputypes.PrimitiveTopologyLineStrip:
		return MTLPrimitiveTypeLineStrip
	case gputypes.PrimitiveTopologyTriangleList:
		return MTLPrimitiveTypeTriangle
	case gputypes.PrimitiveTopologyTriangleStrip:
		return MTLPrimitiveTypeTriangleStrip
	default:
		return MTLPrimitiveTypeTriangle
	}
}

// blendFactorToMTL converts WebGPU blend factor to Metal blend factor.
func blendFactorToMTL(factor gputypes.BlendFactor) MTLBlendFactor {
	switch factor {
	case gputypes.BlendFactorZero:
		return MTLBlendFactorZero
	case gputypes.BlendFactorOne:
		return MTLBlendFactorOne
	case gputypes.BlendFactorSrc:
		return MTLBlendFactorSourceColor
	case gputypes.BlendFactorOneMinusSrc:
		return MTLBlendFactorOneMinusSourceColor
	case gputypes.BlendFactorSrcAlpha:
		return MTLBlendFactorSourceAlpha
	case gputypes.BlendFactorOneMinusSrcAlpha:
		return MTLBlendFactorOneMinusSourceAlpha
	case gputypes.BlendFactorDst:
		return MTLBlendFactorDestinationColor
	case gputypes.BlendFactorOneMinusDst:
		return MTLBlendFactorOneMinusDestinationColor
	case gputypes.BlendFactorDstAlpha:
		return MTLBlendFactorDestinationAlpha
	case gputypes.BlendFactorOneMinusDstAlpha:
		return MTLBlendFactorOneMinusDestinationAlpha
	case gputypes.BlendFactorSrcAlphaSaturated:
		return MTLBlendFactorSourceAlphaSaturated
	case gputypes.BlendFactorConstant:
		return MTLBlendFactorBlendColor
	case gputypes.BlendFactorOneMinusConstant:
		return MTLBlendFactorOneMinusBlendColor
	default:
		return MTLBlendFactorOne
	}
}

// blendOperationToMTL converts WebGPU blend operation to Metal blend operation.
func blendOperationToMTL(op gputypes.BlendOperation) MTLBlendOperation {
	switch op {
	case gputypes.BlendOperationAdd:
		return MTLBlendOperationAdd
	case gputypes.BlendOperationSubtract:
		return MTLBlendOperationSubtract
	case gputypes.BlendOperationReverseSubtract:
		return MTLBlendOperationReverseSubtract
	case gputypes.BlendOperationMin:
		return MTLBlendOperationMin
	case gputypes.BlendOperationMax:
		return MTLBlendOperationMax
	default:
		return MTLBlendOperationAdd
	}
}

// loadOpToMTL converts WebGPU load operation to Metal load action.
func loadOpToMTL(op gputypes.LoadOp) MTLLoadAction {
	switch op {
	case gputypes.LoadOpClear:
		return MTLLoadActionClear
	case gputypes.LoadOpLoad:
		return MTLLoadActionLoad
	default:
		return MTLLoadActionDontCare
	}
}

// storeOpToMTL converts WebGPU store operation to Metal store action.
func storeOpToMTL(op gputypes.StoreOp) MTLStoreAction {
	switch op {
	case gputypes.StoreOpStore:
		return MTLStoreActionStore
	case gputypes.StoreOpDiscard:
		return MTLStoreActionDontCare
	default:
		return MTLStoreActionStore
	}
}

// cullModeToMTL converts WebGPU cull mode to Metal cull mode.
func cullModeToMTL(mode gputypes.CullMode) MTLCullMode {
	switch mode {
	case gputypes.CullModeNone:
		return MTLCullModeNone
	case gputypes.CullModeFront:
		return MTLCullModeFront
	case gputypes.CullModeBack:
		return MTLCullModeBack
	default:
		return MTLCullModeNone
	}
}

// frontFaceToMTL converts WebGPU front face to Metal winding order.
func frontFaceToMTL(face gputypes.FrontFace) MTLWinding {
	switch face {
	case gputypes.FrontFaceCCW:
		return MTLWindingCounterClockwise
	case gputypes.FrontFaceCW:
		return MTLWindingClockwise
	default:
		return MTLWindingCounterClockwise
	}
}

// indexFormatToMTL converts WebGPU index format to Metal index type.
func indexFormatToMTL(format gputypes.IndexFormat) MTLIndexType {
	switch format {
	case gputypes.IndexFormatUint16:
		return MTLIndexTypeUInt16
	case gputypes.IndexFormatUint32:
		return MTLIndexTypeUInt32
	default:
		return MTLIndexTypeUInt32
	}
}
