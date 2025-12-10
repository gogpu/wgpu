// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vulkan

import (
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/vulkan/vk"
	"github.com/gogpu/wgpu/types"
)

// bufferUsageToVk converts WebGPU buffer usage flags to Vulkan buffer usage flags.
func bufferUsageToVk(usage types.BufferUsage) vk.BufferUsageFlags {
	var flags vk.BufferUsageFlags

	if usage&types.BufferUsageCopySrc != 0 {
		flags |= vk.BufferUsageFlags(vk.BufferUsageTransferSrcBit)
	}
	if usage&types.BufferUsageCopyDst != 0 {
		flags |= vk.BufferUsageFlags(vk.BufferUsageTransferDstBit)
	}
	if usage&types.BufferUsageIndex != 0 {
		flags |= vk.BufferUsageFlags(vk.BufferUsageIndexBufferBit)
	}
	if usage&types.BufferUsageVertex != 0 {
		flags |= vk.BufferUsageFlags(vk.BufferUsageVertexBufferBit)
	}
	if usage&types.BufferUsageUniform != 0 {
		flags |= vk.BufferUsageFlags(vk.BufferUsageUniformBufferBit)
	}
	if usage&types.BufferUsageStorage != 0 {
		flags |= vk.BufferUsageFlags(vk.BufferUsageStorageBufferBit)
	}
	if usage&types.BufferUsageIndirect != 0 {
		flags |= vk.BufferUsageFlags(vk.BufferUsageIndirectBufferBit)
	}

	return flags
}

// textureUsageToVk converts WebGPU texture usage flags to Vulkan image usage flags.
func textureUsageToVk(usage types.TextureUsage) vk.ImageUsageFlags {
	var flags vk.ImageUsageFlags

	if usage&types.TextureUsageCopySrc != 0 {
		flags |= vk.ImageUsageFlags(vk.ImageUsageTransferSrcBit)
	}
	if usage&types.TextureUsageCopyDst != 0 {
		flags |= vk.ImageUsageFlags(vk.ImageUsageTransferDstBit)
	}
	if usage&types.TextureUsageTextureBinding != 0 {
		flags |= vk.ImageUsageFlags(vk.ImageUsageSampledBit)
	}
	if usage&types.TextureUsageStorageBinding != 0 {
		flags |= vk.ImageUsageFlags(vk.ImageUsageStorageBit)
	}
	if usage&types.TextureUsageRenderAttachment != 0 {
		flags |= vk.ImageUsageFlags(vk.ImageUsageColorAttachmentBit)
	}

	return flags
}

// textureDimensionToVkImageType converts WebGPU texture dimension to Vulkan image type.
func textureDimensionToVkImageType(dim types.TextureDimension) vk.ImageType {
	switch dim {
	case types.TextureDimension1D:
		return vk.ImageType1d
	case types.TextureDimension2D:
		return vk.ImageType2d
	case types.TextureDimension3D:
		return vk.ImageType3d
	default:
		return vk.ImageType2d
	}
}

// textureFormatToVk converts WebGPU texture format to Vulkan format.
// Uses a lookup table for efficient O(1) conversion.
func textureFormatToVk(format types.TextureFormat) vk.Format {
	if f, ok := textureFormatMap[format]; ok {
		return f
	}
	return vk.FormatUndefined
}

// textureFormatMap maps WebGPU texture formats to Vulkan formats.
var textureFormatMap = map[types.TextureFormat]vk.Format{
	// 8-bit formats
	types.TextureFormatR8Unorm: vk.FormatR8Unorm,
	types.TextureFormatR8Snorm: vk.FormatR8Snorm,
	types.TextureFormatR8Uint:  vk.FormatR8Uint,
	types.TextureFormatR8Sint:  vk.FormatR8Sint,

	// 16-bit formats
	types.TextureFormatR16Uint:  vk.FormatR16Uint,
	types.TextureFormatR16Sint:  vk.FormatR16Sint,
	types.TextureFormatR16Float: vk.FormatR16Sfloat,
	types.TextureFormatRG8Unorm: vk.FormatR8g8Unorm,
	types.TextureFormatRG8Snorm: vk.FormatR8g8Snorm,
	types.TextureFormatRG8Uint:  vk.FormatR8g8Uint,
	types.TextureFormatRG8Sint:  vk.FormatR8g8Sint,

	// 32-bit formats
	types.TextureFormatR32Uint:        vk.FormatR32Uint,
	types.TextureFormatR32Sint:        vk.FormatR32Sint,
	types.TextureFormatR32Float:       vk.FormatR32Sfloat,
	types.TextureFormatRG16Uint:       vk.FormatR16g16Uint,
	types.TextureFormatRG16Sint:       vk.FormatR16g16Sint,
	types.TextureFormatRG16Float:      vk.FormatR16g16Sfloat,
	types.TextureFormatRGBA8Unorm:     vk.FormatR8g8b8a8Unorm,
	types.TextureFormatRGBA8UnormSrgb: vk.FormatR8g8b8a8Srgb,
	types.TextureFormatRGBA8Snorm:     vk.FormatR8g8b8a8Snorm,
	types.TextureFormatRGBA8Uint:      vk.FormatR8g8b8a8Uint,
	types.TextureFormatRGBA8Sint:      vk.FormatR8g8b8a8Sint,
	types.TextureFormatBGRA8Unorm:     vk.FormatB8g8r8a8Unorm,
	types.TextureFormatBGRA8UnormSrgb: vk.FormatB8g8r8a8Srgb,

	// Packed formats
	types.TextureFormatRGB9E5Ufloat:  vk.FormatE5b9g9r9UfloatPack32,
	types.TextureFormatRGB10A2Uint:   vk.FormatA2b10g10r10UintPack32,
	types.TextureFormatRGB10A2Unorm:  vk.FormatA2b10g10r10UnormPack32,
	types.TextureFormatRG11B10Ufloat: vk.FormatB10g11r11UfloatPack32,

	// 64-bit formats
	types.TextureFormatRG32Uint:    vk.FormatR32g32Uint,
	types.TextureFormatRG32Sint:    vk.FormatR32g32Sint,
	types.TextureFormatRG32Float:   vk.FormatR32g32Sfloat,
	types.TextureFormatRGBA16Uint:  vk.FormatR16g16b16a16Uint,
	types.TextureFormatRGBA16Sint:  vk.FormatR16g16b16a16Sint,
	types.TextureFormatRGBA16Float: vk.FormatR16g16b16a16Sfloat,

	// 128-bit formats
	types.TextureFormatRGBA32Uint:  vk.FormatR32g32b32a32Uint,
	types.TextureFormatRGBA32Sint:  vk.FormatR32g32b32a32Sint,
	types.TextureFormatRGBA32Float: vk.FormatR32g32b32a32Sfloat,

	// Depth/stencil formats
	types.TextureFormatStencil8:             vk.FormatS8Uint,
	types.TextureFormatDepth16Unorm:         vk.FormatD16Unorm,
	types.TextureFormatDepth24Plus:          vk.FormatX8D24UnormPack32,
	types.TextureFormatDepth24PlusStencil8:  vk.FormatD24UnormS8Uint,
	types.TextureFormatDepth32Float:         vk.FormatD32Sfloat,
	types.TextureFormatDepth32FloatStencil8: vk.FormatD32SfloatS8Uint,

	// BC compressed formats
	types.TextureFormatBC1RGBAUnorm:     vk.FormatBc1RgbaUnormBlock,
	types.TextureFormatBC1RGBAUnormSrgb: vk.FormatBc1RgbaSrgbBlock,
	types.TextureFormatBC2RGBAUnorm:     vk.FormatBc2UnormBlock,
	types.TextureFormatBC2RGBAUnormSrgb: vk.FormatBc2SrgbBlock,
	types.TextureFormatBC3RGBAUnorm:     vk.FormatBc3UnormBlock,
	types.TextureFormatBC3RGBAUnormSrgb: vk.FormatBc3SrgbBlock,
	types.TextureFormatBC4RUnorm:        vk.FormatBc4UnormBlock,
	types.TextureFormatBC4RSnorm:        vk.FormatBc4SnormBlock,
	types.TextureFormatBC5RGUnorm:       vk.FormatBc5UnormBlock,
	types.TextureFormatBC5RGSnorm:       vk.FormatBc5SnormBlock,
	types.TextureFormatBC6HRGBUfloat:    vk.FormatBc6hUfloatBlock,
	types.TextureFormatBC6HRGBFloat:     vk.FormatBc6hSfloatBlock,
	types.TextureFormatBC7RGBAUnorm:     vk.FormatBc7UnormBlock,
	types.TextureFormatBC7RGBAUnormSrgb: vk.FormatBc7SrgbBlock,

	// ETC2 compressed formats
	types.TextureFormatETC2RGB8Unorm:       vk.FormatEtc2R8g8b8UnormBlock,
	types.TextureFormatETC2RGB8UnormSrgb:   vk.FormatEtc2R8g8b8SrgbBlock,
	types.TextureFormatETC2RGB8A1Unorm:     vk.FormatEtc2R8g8b8a1UnormBlock,
	types.TextureFormatETC2RGB8A1UnormSrgb: vk.FormatEtc2R8g8b8a1SrgbBlock,
	types.TextureFormatETC2RGBA8Unorm:      vk.FormatEtc2R8g8b8a8UnormBlock,
	types.TextureFormatETC2RGBA8UnormSrgb:  vk.FormatEtc2R8g8b8a8SrgbBlock,
	types.TextureFormatEACR11Unorm:         vk.FormatEacR11UnormBlock,
	types.TextureFormatEACR11Snorm:         vk.FormatEacR11SnormBlock,
	types.TextureFormatEACRG11Unorm:        vk.FormatEacR11g11UnormBlock,
	types.TextureFormatEACRG11Snorm:        vk.FormatEacR11g11SnormBlock,

	// ASTC compressed formats
	types.TextureFormatASTC4x4Unorm:       vk.FormatAstc4x4UnormBlock,
	types.TextureFormatASTC4x4UnormSrgb:   vk.FormatAstc4x4SrgbBlock,
	types.TextureFormatASTC5x4Unorm:       vk.FormatAstc5x4UnormBlock,
	types.TextureFormatASTC5x4UnormSrgb:   vk.FormatAstc5x4SrgbBlock,
	types.TextureFormatASTC5x5Unorm:       vk.FormatAstc5x5UnormBlock,
	types.TextureFormatASTC5x5UnormSrgb:   vk.FormatAstc5x5SrgbBlock,
	types.TextureFormatASTC6x5Unorm:       vk.FormatAstc6x5UnormBlock,
	types.TextureFormatASTC6x5UnormSrgb:   vk.FormatAstc6x5SrgbBlock,
	types.TextureFormatASTC6x6Unorm:       vk.FormatAstc6x6UnormBlock,
	types.TextureFormatASTC6x6UnormSrgb:   vk.FormatAstc6x6SrgbBlock,
	types.TextureFormatASTC8x5Unorm:       vk.FormatAstc8x5UnormBlock,
	types.TextureFormatASTC8x5UnormSrgb:   vk.FormatAstc8x5SrgbBlock,
	types.TextureFormatASTC8x6Unorm:       vk.FormatAstc8x6UnormBlock,
	types.TextureFormatASTC8x6UnormSrgb:   vk.FormatAstc8x6SrgbBlock,
	types.TextureFormatASTC8x8Unorm:       vk.FormatAstc8x8UnormBlock,
	types.TextureFormatASTC8x8UnormSrgb:   vk.FormatAstc8x8SrgbBlock,
	types.TextureFormatASTC10x5Unorm:      vk.FormatAstc10x5UnormBlock,
	types.TextureFormatASTC10x5UnormSrgb:  vk.FormatAstc10x5SrgbBlock,
	types.TextureFormatASTC10x6Unorm:      vk.FormatAstc10x6UnormBlock,
	types.TextureFormatASTC10x6UnormSrgb:  vk.FormatAstc10x6SrgbBlock,
	types.TextureFormatASTC10x8Unorm:      vk.FormatAstc10x8UnormBlock,
	types.TextureFormatASTC10x8UnormSrgb:  vk.FormatAstc10x8SrgbBlock,
	types.TextureFormatASTC10x10Unorm:     vk.FormatAstc10x10UnormBlock,
	types.TextureFormatASTC10x10UnormSrgb: vk.FormatAstc10x10SrgbBlock,
	types.TextureFormatASTC12x10Unorm:     vk.FormatAstc12x10UnormBlock,
	types.TextureFormatASTC12x10UnormSrgb: vk.FormatAstc12x10SrgbBlock,
	types.TextureFormatASTC12x12Unorm:     vk.FormatAstc12x12UnormBlock,
	types.TextureFormatASTC12x12UnormSrgb: vk.FormatAstc12x12SrgbBlock,
}

// addressModeToVk converts WebGPU address mode to Vulkan sampler address mode.
func addressModeToVk(mode types.AddressMode) vk.SamplerAddressMode {
	switch mode {
	case types.AddressModeClampToEdge:
		return vk.SamplerAddressModeClampToEdge
	case types.AddressModeRepeat:
		return vk.SamplerAddressModeRepeat
	case types.AddressModeMirrorRepeat:
		return vk.SamplerAddressModeMirroredRepeat
	default:
		return vk.SamplerAddressModeClampToEdge
	}
}

// filterModeToVk converts WebGPU filter mode to Vulkan filter.
func filterModeToVk(mode types.FilterMode) vk.Filter {
	switch mode {
	case types.FilterModeNearest:
		return vk.FilterNearest
	case types.FilterModeLinear:
		return vk.FilterLinear
	default:
		return vk.FilterNearest
	}
}

// mipmapFilterModeToVk converts WebGPU mipmap filter mode to Vulkan sampler mipmap mode.
func mipmapFilterModeToVk(mode types.FilterMode) vk.SamplerMipmapMode {
	switch mode {
	case types.FilterModeNearest:
		return vk.SamplerMipmapModeNearest
	case types.FilterModeLinear:
		return vk.SamplerMipmapModeLinear
	default:
		return vk.SamplerMipmapModeNearest
	}
}

// compareFunctionToVk converts WebGPU compare function to Vulkan compare op.
func compareFunctionToVk(fn types.CompareFunction) vk.CompareOp {
	switch fn {
	case types.CompareFunctionNever:
		return vk.CompareOpNever
	case types.CompareFunctionLess:
		return vk.CompareOpLess
	case types.CompareFunctionEqual:
		return vk.CompareOpEqual
	case types.CompareFunctionLessEqual:
		return vk.CompareOpLessOrEqual
	case types.CompareFunctionGreater:
		return vk.CompareOpGreater
	case types.CompareFunctionNotEqual:
		return vk.CompareOpNotEqual
	case types.CompareFunctionGreaterEqual:
		return vk.CompareOpGreaterOrEqual
	case types.CompareFunctionAlways:
		return vk.CompareOpAlways
	default:
		return vk.CompareOpNever
	}
}

// shaderStagesToVk converts WebGPU shader stages to Vulkan shader stage flags.
func shaderStagesToVk(stages types.ShaderStages) vk.ShaderStageFlags {
	var flags vk.ShaderStageFlags

	if stages&types.ShaderStageVertex != 0 {
		flags |= vk.ShaderStageFlags(vk.ShaderStageVertexBit)
	}
	if stages&types.ShaderStageFragment != 0 {
		flags |= vk.ShaderStageFlags(vk.ShaderStageFragmentBit)
	}
	if stages&types.ShaderStageCompute != 0 {
		flags |= vk.ShaderStageFlags(vk.ShaderStageComputeBit)
	}

	return flags
}

// bufferBindingTypeToVk converts WebGPU buffer binding type to Vulkan descriptor type.
func bufferBindingTypeToVk(bindingType types.BufferBindingType) vk.DescriptorType {
	switch bindingType {
	case types.BufferBindingTypeUniform:
		return vk.DescriptorTypeUniformBuffer
	case types.BufferBindingTypeStorage:
		return vk.DescriptorTypeStorageBuffer
	case types.BufferBindingTypeReadOnlyStorage:
		return vk.DescriptorTypeStorageBuffer
	default:
		return vk.DescriptorTypeUniformBuffer
	}
}

// vertexStepModeToVk converts WebGPU vertex step mode to Vulkan input rate.
func vertexStepModeToVk(mode types.VertexStepMode) vk.VertexInputRate {
	switch mode {
	case types.VertexStepModeVertex:
		return vk.VertexInputRateVertex
	case types.VertexStepModeInstance:
		return vk.VertexInputRateInstance
	default:
		return vk.VertexInputRateVertex
	}
}

// vertexFormatToVk converts WebGPU vertex format to Vulkan format.
func vertexFormatToVk(format types.VertexFormat) vk.Format {
	switch format {
	// 8-bit formats
	case types.VertexFormatUint8x2:
		return vk.FormatR8g8Uint
	case types.VertexFormatUint8x4:
		return vk.FormatR8g8b8a8Uint
	case types.VertexFormatSint8x2:
		return vk.FormatR8g8Sint
	case types.VertexFormatSint8x4:
		return vk.FormatR8g8b8a8Sint
	case types.VertexFormatUnorm8x2:
		return vk.FormatR8g8Unorm
	case types.VertexFormatUnorm8x4:
		return vk.FormatR8g8b8a8Unorm
	case types.VertexFormatSnorm8x2:
		return vk.FormatR8g8Snorm
	case types.VertexFormatSnorm8x4:
		return vk.FormatR8g8b8a8Snorm

	// 16-bit formats
	case types.VertexFormatUint16x2:
		return vk.FormatR16g16Uint
	case types.VertexFormatUint16x4:
		return vk.FormatR16g16b16a16Uint
	case types.VertexFormatSint16x2:
		return vk.FormatR16g16Sint
	case types.VertexFormatSint16x4:
		return vk.FormatR16g16b16a16Sint
	case types.VertexFormatUnorm16x2:
		return vk.FormatR16g16Unorm
	case types.VertexFormatUnorm16x4:
		return vk.FormatR16g16b16a16Unorm
	case types.VertexFormatSnorm16x2:
		return vk.FormatR16g16Snorm
	case types.VertexFormatSnorm16x4:
		return vk.FormatR16g16b16a16Snorm
	case types.VertexFormatFloat16x2:
		return vk.FormatR16g16Sfloat
	case types.VertexFormatFloat16x4:
		return vk.FormatR16g16b16a16Sfloat

	// 32-bit formats
	case types.VertexFormatFloat32:
		return vk.FormatR32Sfloat
	case types.VertexFormatFloat32x2:
		return vk.FormatR32g32Sfloat
	case types.VertexFormatFloat32x3:
		return vk.FormatR32g32b32Sfloat
	case types.VertexFormatFloat32x4:
		return vk.FormatR32g32b32a32Sfloat
	case types.VertexFormatUint32:
		return vk.FormatR32Uint
	case types.VertexFormatUint32x2:
		return vk.FormatR32g32Uint
	case types.VertexFormatUint32x3:
		return vk.FormatR32g32b32Uint
	case types.VertexFormatUint32x4:
		return vk.FormatR32g32b32a32Uint
	case types.VertexFormatSint32:
		return vk.FormatR32Sint
	case types.VertexFormatSint32x2:
		return vk.FormatR32g32Sint
	case types.VertexFormatSint32x3:
		return vk.FormatR32g32b32Sint
	case types.VertexFormatSint32x4:
		return vk.FormatR32g32b32a32Sint

	// Packed formats
	case types.VertexFormatUnorm1010102:
		return vk.FormatA2b10g10r10UnormPack32

	default:
		return vk.FormatR32g32b32a32Sfloat
	}
}

// primitiveTopologyToVk converts WebGPU primitive topology to Vulkan topology.
func primitiveTopologyToVk(topology types.PrimitiveTopology) vk.PrimitiveTopology {
	switch topology {
	case types.PrimitiveTopologyPointList:
		return vk.PrimitiveTopologyPointList
	case types.PrimitiveTopologyLineList:
		return vk.PrimitiveTopologyLineList
	case types.PrimitiveTopologyLineStrip:
		return vk.PrimitiveTopologyLineStrip
	case types.PrimitiveTopologyTriangleList:
		return vk.PrimitiveTopologyTriangleList
	case types.PrimitiveTopologyTriangleStrip:
		return vk.PrimitiveTopologyTriangleStrip
	default:
		return vk.PrimitiveTopologyTriangleList
	}
}

// cullModeToVk converts WebGPU cull mode to Vulkan cull mode flags.
func cullModeToVk(mode types.CullMode) vk.CullModeFlags {
	switch mode {
	case types.CullModeNone:
		return vk.CullModeFlags(vk.CullModeNone)
	case types.CullModeFront:
		return vk.CullModeFlags(vk.CullModeFrontBit)
	case types.CullModeBack:
		return vk.CullModeFlags(vk.CullModeBackBit)
	default:
		return vk.CullModeFlags(vk.CullModeNone)
	}
}

// frontFaceToVk converts WebGPU front face to Vulkan front face.
func frontFaceToVk(face types.FrontFace) vk.FrontFace {
	switch face {
	case types.FrontFaceCCW:
		return vk.FrontFaceCounterClockwise
	case types.FrontFaceCW:
		return vk.FrontFaceClockwise
	default:
		return vk.FrontFaceCounterClockwise
	}
}

// colorWriteMaskToVk converts WebGPU color write mask to Vulkan color component flags.
func colorWriteMaskToVk(mask types.ColorWriteMask) vk.ColorComponentFlags {
	var flags vk.ColorComponentFlags
	if mask&types.ColorWriteMaskRed != 0 {
		flags |= vk.ColorComponentFlags(vk.ColorComponentRBit)
	}
	if mask&types.ColorWriteMaskGreen != 0 {
		flags |= vk.ColorComponentFlags(vk.ColorComponentGBit)
	}
	if mask&types.ColorWriteMaskBlue != 0 {
		flags |= vk.ColorComponentFlags(vk.ColorComponentBBit)
	}
	if mask&types.ColorWriteMaskAlpha != 0 {
		flags |= vk.ColorComponentFlags(vk.ColorComponentABit)
	}
	return flags
}

// blendFactorToVk converts WebGPU blend factor to Vulkan blend factor.
func blendFactorToVk(factor types.BlendFactor) vk.BlendFactor {
	switch factor {
	case types.BlendFactorZero:
		return vk.BlendFactorZero
	case types.BlendFactorOne:
		return vk.BlendFactorOne
	case types.BlendFactorSrc:
		return vk.BlendFactorSrcColor
	case types.BlendFactorOneMinusSrc:
		return vk.BlendFactorOneMinusSrcColor
	case types.BlendFactorSrcAlpha:
		return vk.BlendFactorSrcAlpha
	case types.BlendFactorOneMinusSrcAlpha:
		return vk.BlendFactorOneMinusSrcAlpha
	case types.BlendFactorDst:
		return vk.BlendFactorDstColor
	case types.BlendFactorOneMinusDst:
		return vk.BlendFactorOneMinusDstColor
	case types.BlendFactorDstAlpha:
		return vk.BlendFactorDstAlpha
	case types.BlendFactorOneMinusDstAlpha:
		return vk.BlendFactorOneMinusDstAlpha
	case types.BlendFactorSrcAlphaSaturated:
		return vk.BlendFactorSrcAlphaSaturate
	case types.BlendFactorConstant:
		return vk.BlendFactorConstantColor
	case types.BlendFactorOneMinusConstant:
		return vk.BlendFactorOneMinusConstantColor
	default:
		return vk.BlendFactorOne
	}
}

// blendOperationToVk converts WebGPU blend operation to Vulkan blend op.
func blendOperationToVk(op types.BlendOperation) vk.BlendOp {
	switch op {
	case types.BlendOperationAdd:
		return vk.BlendOpAdd
	case types.BlendOperationSubtract:
		return vk.BlendOpSubtract
	case types.BlendOperationReverseSubtract:
		return vk.BlendOpReverseSubtract
	case types.BlendOperationMin:
		return vk.BlendOpMin
	case types.BlendOperationMax:
		return vk.BlendOpMax
	default:
		return vk.BlendOpAdd
	}
}

// stencilOperationToVk converts HAL stencil operation to Vulkan stencil op.
func stencilOperationToVk(op hal.StencilOperation) vk.StencilOp {
	switch op {
	case hal.StencilOperationKeep:
		return vk.StencilOpKeep
	case hal.StencilOperationZero:
		return vk.StencilOpZero
	case hal.StencilOperationReplace:
		return vk.StencilOpReplace
	case hal.StencilOperationInvert:
		return vk.StencilOpInvert
	case hal.StencilOperationIncrementClamp:
		return vk.StencilOpIncrementAndClamp
	case hal.StencilOperationDecrementClamp:
		return vk.StencilOpDecrementAndClamp
	case hal.StencilOperationIncrementWrap:
		return vk.StencilOpIncrementAndWrap
	case hal.StencilOperationDecrementWrap:
		return vk.StencilOpDecrementAndWrap
	default:
		return vk.StencilOpKeep
	}
}

// stencilFaceStateToVk converts HAL stencil face state to Vulkan stencil op state.
func stencilFaceStateToVk(state hal.StencilFaceState) vk.StencilOpState {
	return vk.StencilOpState{
		FailOp:      stencilOperationToVk(state.FailOp),
		PassOp:      stencilOperationToVk(state.PassOp),
		DepthFailOp: stencilOperationToVk(state.DepthFailOp),
		CompareOp:   compareFunctionToVk(state.Compare),
	}
}

// textureViewDimensionToVk converts WebGPU texture view dimension to Vulkan image view type.
func textureViewDimensionToVk(dim types.TextureViewDimension) vk.ImageViewType {
	switch dim {
	case types.TextureViewDimension1D:
		return vk.ImageViewType1d
	case types.TextureViewDimension2D:
		return vk.ImageViewType2d
	case types.TextureViewDimension2DArray:
		return vk.ImageViewType2dArray
	case types.TextureViewDimensionCube:
		return vk.ImageViewTypeCube
	case types.TextureViewDimensionCubeArray:
		return vk.ImageViewTypeCubeArray
	case types.TextureViewDimension3D:
		return vk.ImageViewType3d
	default:
		return vk.ImageViewType2d
	}
}

// textureAspectToVk converts WebGPU texture aspect to Vulkan image aspect flags.
func textureAspectToVk(aspect types.TextureAspect, format types.TextureFormat) vk.ImageAspectFlags {
	switch aspect {
	case types.TextureAspectDepthOnly:
		return vk.ImageAspectFlags(vk.ImageAspectDepthBit)
	case types.TextureAspectStencilOnly:
		return vk.ImageAspectFlags(vk.ImageAspectStencilBit)
	case types.TextureAspectAll:
		// For depth-stencil formats, include both aspects
		if isDepthStencilFormat(format) {
			flags := vk.ImageAspectFlags(vk.ImageAspectDepthBit)
			if hasStencilAspect(format) {
				flags |= vk.ImageAspectFlags(vk.ImageAspectStencilBit)
			}
			return flags
		}
		return vk.ImageAspectFlags(vk.ImageAspectColorBit)
	default:
		return vk.ImageAspectFlags(vk.ImageAspectColorBit)
	}
}

// textureAspectToVkSimple converts texture aspect without format context.
// Used when texture format is not available (e.g., in buffer-texture copy regions).
func textureAspectToVkSimple(aspect types.TextureAspect) vk.ImageAspectFlags {
	switch aspect {
	case types.TextureAspectDepthOnly:
		return vk.ImageAspectFlags(vk.ImageAspectDepthBit)
	case types.TextureAspectStencilOnly:
		return vk.ImageAspectFlags(vk.ImageAspectStencilBit)
	default:
		return vk.ImageAspectFlags(vk.ImageAspectColorBit)
	}
}

// isDepthStencilFormat returns true if the format is a depth or depth-stencil format.
func isDepthStencilFormat(format types.TextureFormat) bool {
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

// hasStencilAspect returns true if the format has a stencil aspect.
func hasStencilAspect(format types.TextureFormat) bool {
	switch format {
	case types.TextureFormatDepth24PlusStencil8,
		types.TextureFormatDepth32FloatStencil8,
		types.TextureFormatStencil8:
		return true
	default:
		return false
	}
}

// textureDimensionToViewType converts WebGPU texture dimension to default Vulkan image view type.
func textureDimensionToViewType(dim types.TextureDimension) vk.ImageViewType {
	switch dim {
	case types.TextureDimension1D:
		return vk.ImageViewType1d
	case types.TextureDimension2D:
		return vk.ImageViewType2d
	case types.TextureDimension3D:
		return vk.ImageViewType3d
	default:
		return vk.ImageViewType2d
	}
}
