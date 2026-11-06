// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build darwin && !(js && wasm)

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
	case gputypes.TextureFormatRGB10A2Uint:
		return MTLPixelFormatRGB10A2Uint
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
	case gputypes.TextureFormatDepth24Plus:
		return MTLPixelFormatDepth32Float
	case gputypes.TextureFormatDepth24PlusStencil8:
		return MTLPixelFormatDepth32FloatStencil8
	case gputypes.TextureFormatDepth32FloatStencil8:
		return MTLPixelFormatDepth32FloatStencil8
	case gputypes.TextureFormatStencil8:
		return MTLPixelFormatStencil8
	default:
		return compressedTextureFormatToMTL(format)
	}
}

// compressedTextureFormatToMTL maps the block-compressed formats, as Rust
// wgpu-hal src/metal/adapter.rs map_format does. Whether the device can sample
// a format is the adapter's feature list, not this table: a format whose
// feature the adapter does not offer is refused by core before it reaches the
// HAL.
func compressedTextureFormatToMTL(format gputypes.TextureFormat) MTLPixelFormat {
	switch format {
	case gputypes.TextureFormatBC1RGBAUnorm:
		return MTLPixelFormatBC1RGBA
	case gputypes.TextureFormatBC1RGBAUnormSrgb:
		return MTLPixelFormatBC1RGBASRGB
	case gputypes.TextureFormatBC2RGBAUnorm:
		return MTLPixelFormatBC2RGBA
	case gputypes.TextureFormatBC2RGBAUnormSrgb:
		return MTLPixelFormatBC2RGBASRGB
	case gputypes.TextureFormatBC3RGBAUnorm:
		return MTLPixelFormatBC3RGBA
	case gputypes.TextureFormatBC3RGBAUnormSrgb:
		return MTLPixelFormatBC3RGBASRGB
	case gputypes.TextureFormatBC4RUnorm:
		return MTLPixelFormatBC4RUnorm
	case gputypes.TextureFormatBC4RSnorm:
		return MTLPixelFormatBC4RSnorm
	case gputypes.TextureFormatBC5RGUnorm:
		return MTLPixelFormatBC5RGUnorm
	case gputypes.TextureFormatBC5RGSnorm:
		return MTLPixelFormatBC5RGSnorm
	case gputypes.TextureFormatBC6HRGBUfloat:
		return MTLPixelFormatBC6HRGBUfloat
	case gputypes.TextureFormatBC6HRGBFloat:
		return MTLPixelFormatBC6HRGBFloat
	case gputypes.TextureFormatBC7RGBAUnorm:
		return MTLPixelFormatBC7RGBAUnorm
	case gputypes.TextureFormatBC7RGBAUnormSrgb:
		return MTLPixelFormatBC7RGBAUnormSRGB
	case gputypes.TextureFormatETC2RGB8Unorm:
		return MTLPixelFormatETC2RGB8
	case gputypes.TextureFormatETC2RGB8UnormSrgb:
		return MTLPixelFormatETC2RGB8SRGB
	case gputypes.TextureFormatETC2RGB8A1Unorm:
		return MTLPixelFormatETC2RGB8A1
	case gputypes.TextureFormatETC2RGB8A1UnormSrgb:
		return MTLPixelFormatETC2RGB8A1SRGB
	case gputypes.TextureFormatETC2RGBA8Unorm:
		return MTLPixelFormatEACRGBA8
	case gputypes.TextureFormatETC2RGBA8UnormSrgb:
		return MTLPixelFormatEACRGBA8SRGB
	case gputypes.TextureFormatEACR11Unorm:
		return MTLPixelFormatEACR11Unorm
	case gputypes.TextureFormatEACR11Snorm:
		return MTLPixelFormatEACR11Snorm
	case gputypes.TextureFormatEACRG11Unorm:
		return MTLPixelFormatEACRG11Unorm
	case gputypes.TextureFormatEACRG11Snorm:
		return MTLPixelFormatEACRG11Snorm
	default:
		return astcTextureFormatToMTL(format)
	}
}

// astcTextureFormatToMTL maps the ASTC LDR formats: gputypes Unorm is Metal's
// LDR variant and UnormSrgb its sRGB one (Rust wgpu-hal map_format,
// AstcChannel::Unorm and AstcChannel::UnormSrgb).
func astcTextureFormatToMTL(format gputypes.TextureFormat) MTLPixelFormat {
	switch format {
	case gputypes.TextureFormatASTC4x4Unorm:
		return MTLPixelFormatASTC4x4LDR
	case gputypes.TextureFormatASTC4x4UnormSrgb:
		return MTLPixelFormatASTC4x4SRGB
	case gputypes.TextureFormatASTC5x4Unorm:
		return MTLPixelFormatASTC5x4LDR
	case gputypes.TextureFormatASTC5x4UnormSrgb:
		return MTLPixelFormatASTC5x4SRGB
	case gputypes.TextureFormatASTC5x5Unorm:
		return MTLPixelFormatASTC5x5LDR
	case gputypes.TextureFormatASTC5x5UnormSrgb:
		return MTLPixelFormatASTC5x5SRGB
	case gputypes.TextureFormatASTC6x5Unorm:
		return MTLPixelFormatASTC6x5LDR
	case gputypes.TextureFormatASTC6x5UnormSrgb:
		return MTLPixelFormatASTC6x5SRGB
	case gputypes.TextureFormatASTC6x6Unorm:
		return MTLPixelFormatASTC6x6LDR
	case gputypes.TextureFormatASTC6x6UnormSrgb:
		return MTLPixelFormatASTC6x6SRGB
	case gputypes.TextureFormatASTC8x5Unorm:
		return MTLPixelFormatASTC8x5LDR
	case gputypes.TextureFormatASTC8x5UnormSrgb:
		return MTLPixelFormatASTC8x5SRGB
	case gputypes.TextureFormatASTC8x6Unorm:
		return MTLPixelFormatASTC8x6LDR
	case gputypes.TextureFormatASTC8x6UnormSrgb:
		return MTLPixelFormatASTC8x6SRGB
	case gputypes.TextureFormatASTC8x8Unorm:
		return MTLPixelFormatASTC8x8LDR
	case gputypes.TextureFormatASTC8x8UnormSrgb:
		return MTLPixelFormatASTC8x8SRGB
	case gputypes.TextureFormatASTC10x5Unorm:
		return MTLPixelFormatASTC10x5LDR
	case gputypes.TextureFormatASTC10x5UnormSrgb:
		return MTLPixelFormatASTC10x5SRGB
	case gputypes.TextureFormatASTC10x6Unorm:
		return MTLPixelFormatASTC10x6LDR
	case gputypes.TextureFormatASTC10x6UnormSrgb:
		return MTLPixelFormatASTC10x6SRGB
	case gputypes.TextureFormatASTC10x8Unorm:
		return MTLPixelFormatASTC10x8LDR
	case gputypes.TextureFormatASTC10x8UnormSrgb:
		return MTLPixelFormatASTC10x8SRGB
	case gputypes.TextureFormatASTC10x10Unorm:
		return MTLPixelFormatASTC10x10LDR
	case gputypes.TextureFormatASTC10x10UnormSrgb:
		return MTLPixelFormatASTC10x10SRGB
	case gputypes.TextureFormatASTC12x10Unorm:
		return MTLPixelFormatASTC12x10LDR
	case gputypes.TextureFormatASTC12x10UnormSrgb:
		return MTLPixelFormatASTC12x10SRGB
	case gputypes.TextureFormatASTC12x12Unorm:
		return MTLPixelFormatASTC12x12LDR
	case gputypes.TextureFormatASTC12x12UnormSrgb:
		return MTLPixelFormatASTC12x12SRGB
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

// textureStorageMode selects the MTLStorageMode for a texture based on GPU family
// and sample count. Returns the storage mode and whether it is Shared (CPU-writable).
//
// Apple GPU (A-series, M-series) + single-sample: Shared — zero-copy CPU writes via
// replaceRegion: and setPurgeableState(empty) for immediate OS memory reclaim.
//
// Apple GPU + multisample: Private — MSAA textures are never CPU-read, have a
// one-frame lifetime, and gain no benefit from Shared storage.
//
// Non-Apple GPU (Intel, AMD): Private always — Metal spec requires Private storage
// for multisample textures on non-Apple GPU families, and Shared offers no advantage
// on discrete/non-UMA architectures.
func textureStorageMode(isAppleGPU bool, sampleCount uint32) (MTLStorageMode, bool) {
	if isAppleGPU && sampleCount <= 1 {
		return MTLStorageModeShared, true
	}
	return MTLStorageModePrivate, false
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

// stencilOperationToMTL maps a WebGPU stencil operation to Metal. The two enums
// use different numeric values, so an explicit switch is required.
func stencilOperationToMTL(op gputypes.StencilOperation) MTLStencilOperation {
	switch op {
	case gputypes.StencilOperationKeep:
		return MTLStencilOperationKeep
	case gputypes.StencilOperationZero:
		return MTLStencilOperationZero
	case gputypes.StencilOperationReplace:
		return MTLStencilOperationReplace
	case gputypes.StencilOperationInvert:
		return MTLStencilOperationInvert
	case gputypes.StencilOperationIncrementClamp:
		return MTLStencilOperationIncrementClamp
	case gputypes.StencilOperationDecrementClamp:
		return MTLStencilOperationDecrementClamp
	case gputypes.StencilOperationIncrementWrap:
		return MTLStencilOperationIncrementWrap
	case gputypes.StencilOperationDecrementWrap:
		return MTLStencilOperationDecrementWrap
	default:
		return MTLStencilOperationKeep
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

// colorWriteMaskToMTL maps a WebGPU color write mask to Metal channel by
// channel. The bit orders differ: gputypes has Red=1, Green=2, Blue=4,
// Alpha=8, MTLColorWriteMask has Red=8, Green=4, Blue=2, Alpha=1, so a raw
// cast swaps red with alpha and green with blue. As Rust wgpu-hal
// src/metal/conv.rs map_color_write.
func colorWriteMaskToMTL(mask gputypes.ColorWriteMask) MTLColorWriteMask {
	out := MTLColorWriteMaskNone
	if mask&gputypes.ColorWriteMaskRed != 0 {
		out |= MTLColorWriteMaskRed
	}
	if mask&gputypes.ColorWriteMaskGreen != 0 {
		out |= MTLColorWriteMaskGreen
	}
	if mask&gputypes.ColorWriteMaskBlue != 0 {
		out |= MTLColorWriteMaskBlue
	}
	if mask&gputypes.ColorWriteMaskAlpha != 0 {
		out |= MTLColorWriteMaskAlpha
	}
	return out
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

func vertexFormatToMTL(format gputypes.VertexFormat) MTLVertexFormat {
	switch format {
	case gputypes.VertexFormatUint8x2:
		return MTLVertexFormatUChar2
	case gputypes.VertexFormatUint8x4:
		return MTLVertexFormatUChar4
	case gputypes.VertexFormatSint8x2:
		return MTLVertexFormatChar2
	case gputypes.VertexFormatSint8x4:
		return MTLVertexFormatChar4
	case gputypes.VertexFormatUnorm8x2:
		return MTLVertexFormatUChar2Normalized
	case gputypes.VertexFormatUnorm8x4:
		return MTLVertexFormatUChar4Normalized
	case gputypes.VertexFormatSnorm8x2:
		return MTLVertexFormatChar2Normalized
	case gputypes.VertexFormatSnorm8x4:
		return MTLVertexFormatChar4Normalized
	case gputypes.VertexFormatUint16x2:
		return MTLVertexFormatUShort2
	case gputypes.VertexFormatUint16x4:
		return MTLVertexFormatUShort4
	case gputypes.VertexFormatSint16x2:
		return MTLVertexFormatShort2
	case gputypes.VertexFormatSint16x4:
		return MTLVertexFormatShort4
	case gputypes.VertexFormatUnorm16x2:
		return MTLVertexFormatUShort2Normalized
	case gputypes.VertexFormatUnorm16x4:
		return MTLVertexFormatUShort4Normalized
	case gputypes.VertexFormatSnorm16x2:
		return MTLVertexFormatShort2Normalized
	case gputypes.VertexFormatSnorm16x4:
		return MTLVertexFormatShort4Normalized
	case gputypes.VertexFormatFloat16x2:
		return MTLVertexFormatHalf2
	case gputypes.VertexFormatFloat16x4:
		return MTLVertexFormatHalf4
	case gputypes.VertexFormatFloat32:
		return MTLVertexFormatFloat
	case gputypes.VertexFormatFloat32x2:
		return MTLVertexFormatFloat2
	case gputypes.VertexFormatFloat32x3:
		return MTLVertexFormatFloat3
	case gputypes.VertexFormatFloat32x4:
		return MTLVertexFormatFloat4
	case gputypes.VertexFormatUint32:
		return MTLVertexFormatUInt
	case gputypes.VertexFormatUint32x2:
		return MTLVertexFormatUInt2
	case gputypes.VertexFormatUint32x3:
		return MTLVertexFormatUInt3
	case gputypes.VertexFormatUint32x4:
		return MTLVertexFormatUInt4
	case gputypes.VertexFormatSint32:
		return MTLVertexFormatInt
	case gputypes.VertexFormatSint32x2:
		return MTLVertexFormatInt2
	case gputypes.VertexFormatSint32x3:
		return MTLVertexFormatInt3
	case gputypes.VertexFormatSint32x4:
		return MTLVertexFormatInt4
	case gputypes.VertexFormatUnorm1010102:
		return MTLVertexFormatUInt1010102Normalized
	default:
		return MTLVertexFormatInvalid
	}
}

// mtlVertexBufferLayout is the MTLVertexBufferLayoutDescriptor setup for one
// WebGPU vertex buffer layout.
type mtlVertexBufferLayout struct {
	stride   uint64
	stepFunc MTLVertexStepFunction
	stepRate uint64
}

// vertexBufferLayoutToMTL converts a vertex buffer layout. Metal rejects a
// zero stride, so arrayStride 0 (WebGPU's broadcast: every vertex and instance
// reads element 0) becomes the constant step function with step rate 0 and a
// stride covering the attributes, rounded up to Metal's 4-byte multiple, as in
// Rust wgpu-hal src/metal/device.rs.
func vertexBufferLayoutToMTL(buf *gputypes.VertexBufferLayout) mtlVertexBufferLayout {
	if buf.ArrayStride != 0 {
		return mtlVertexBufferLayout{
			stride:   buf.ArrayStride,
			stepFunc: vertexStepModeToMTL(buf.StepMode),
			stepRate: 1,
		}
	}
	var stride uint64
	for _, attr := range buf.Attributes {
		stride = max(stride, attr.Offset+attr.Format.Size())
	}
	return mtlVertexBufferLayout{
		stride:   (stride + 3) &^ 3,
		stepFunc: MTLVertexStepFunctionConstant,
		stepRate: 0,
	}
}

func vertexStepModeToMTL(mode gputypes.VertexStepMode) MTLVertexStepFunction {
	switch mode {
	case gputypes.VertexStepModeVertex:
		return MTLVertexStepFunctionPerVertex
	case gputypes.VertexStepModeInstance:
		return MTLVertexStepFunctionPerInstance
	default:
		return MTLVertexStepFunctionPerVertex
	}
}
