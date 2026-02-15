// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vulkan

import (
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/vulkan/vk"
)

// bufferUsageToVk converts WebGPU buffer usage flags to Vulkan buffer usage flags.
func bufferUsageToVk(usage gputypes.BufferUsage) vk.BufferUsageFlags {
	var flags vk.BufferUsageFlags

	if usage&gputypes.BufferUsageCopySrc != 0 {
		flags |= vk.BufferUsageFlags(vk.BufferUsageTransferSrcBit)
	}
	if usage&gputypes.BufferUsageCopyDst != 0 {
		flags |= vk.BufferUsageFlags(vk.BufferUsageTransferDstBit)
	}
	if usage&gputypes.BufferUsageIndex != 0 {
		flags |= vk.BufferUsageFlags(vk.BufferUsageIndexBufferBit)
	}
	if usage&gputypes.BufferUsageVertex != 0 {
		flags |= vk.BufferUsageFlags(vk.BufferUsageVertexBufferBit)
	}
	if usage&gputypes.BufferUsageUniform != 0 {
		flags |= vk.BufferUsageFlags(vk.BufferUsageUniformBufferBit)
	}
	if usage&gputypes.BufferUsageStorage != 0 {
		flags |= vk.BufferUsageFlags(vk.BufferUsageStorageBufferBit)
	}
	if usage&gputypes.BufferUsageIndirect != 0 {
		flags |= vk.BufferUsageFlags(vk.BufferUsageIndirectBufferBit)
	}

	return flags
}

// textureUsageToVk converts WebGPU texture usage flags to Vulkan image usage flags.
func textureUsageToVk(usage gputypes.TextureUsage) vk.ImageUsageFlags {
	var flags vk.ImageUsageFlags

	if usage&gputypes.TextureUsageCopySrc != 0 {
		flags |= vk.ImageUsageFlags(vk.ImageUsageTransferSrcBit)
	}
	if usage&gputypes.TextureUsageCopyDst != 0 {
		flags |= vk.ImageUsageFlags(vk.ImageUsageTransferDstBit)
	}
	if usage&gputypes.TextureUsageTextureBinding != 0 {
		flags |= vk.ImageUsageFlags(vk.ImageUsageSampledBit)
	}
	if usage&gputypes.TextureUsageStorageBinding != 0 {
		flags |= vk.ImageUsageFlags(vk.ImageUsageStorageBit)
	}
	if usage&gputypes.TextureUsageRenderAttachment != 0 {
		flags |= vk.ImageUsageFlags(vk.ImageUsageColorAttachmentBit)
	}

	return flags
}

// textureDimensionToVkImageType converts WebGPU texture dimension to Vulkan image type.
func textureDimensionToVkImageType(dim gputypes.TextureDimension) vk.ImageType {
	switch dim {
	case gputypes.TextureDimension1D:
		return vk.ImageType1d
	case gputypes.TextureDimension2D:
		return vk.ImageType2d
	case gputypes.TextureDimension3D:
		return vk.ImageType3d
	default:
		return vk.ImageType2d
	}
}

// textureFormatToVk converts WebGPU texture format to Vulkan format.
// Uses a lookup table for efficient O(1) conversion.
func textureFormatToVk(format gputypes.TextureFormat) vk.Format {
	if f, ok := textureFormatMap[format]; ok {
		return f
	}
	return vk.FormatUndefined
}

// textureFormatMap maps WebGPU texture formats to Vulkan formats.
var textureFormatMap = map[gputypes.TextureFormat]vk.Format{
	// 8-bit formats
	gputypes.TextureFormatR8Unorm: vk.FormatR8Unorm,
	gputypes.TextureFormatR8Snorm: vk.FormatR8Snorm,
	gputypes.TextureFormatR8Uint:  vk.FormatR8Uint,
	gputypes.TextureFormatR8Sint:  vk.FormatR8Sint,

	// 16-bit formats
	gputypes.TextureFormatR16Uint:  vk.FormatR16Uint,
	gputypes.TextureFormatR16Sint:  vk.FormatR16Sint,
	gputypes.TextureFormatR16Float: vk.FormatR16Sfloat,
	gputypes.TextureFormatRG8Unorm: vk.FormatR8g8Unorm,
	gputypes.TextureFormatRG8Snorm: vk.FormatR8g8Snorm,
	gputypes.TextureFormatRG8Uint:  vk.FormatR8g8Uint,
	gputypes.TextureFormatRG8Sint:  vk.FormatR8g8Sint,

	// 32-bit formats
	gputypes.TextureFormatR32Uint:        vk.FormatR32Uint,
	gputypes.TextureFormatR32Sint:        vk.FormatR32Sint,
	gputypes.TextureFormatR32Float:       vk.FormatR32Sfloat,
	gputypes.TextureFormatRG16Uint:       vk.FormatR16g16Uint,
	gputypes.TextureFormatRG16Sint:       vk.FormatR16g16Sint,
	gputypes.TextureFormatRG16Float:      vk.FormatR16g16Sfloat,
	gputypes.TextureFormatRGBA8Unorm:     vk.FormatR8g8b8a8Unorm,
	gputypes.TextureFormatRGBA8UnormSrgb: vk.FormatR8g8b8a8Srgb,
	gputypes.TextureFormatRGBA8Snorm:     vk.FormatR8g8b8a8Snorm,
	gputypes.TextureFormatRGBA8Uint:      vk.FormatR8g8b8a8Uint,
	gputypes.TextureFormatRGBA8Sint:      vk.FormatR8g8b8a8Sint,
	gputypes.TextureFormatBGRA8Unorm:     vk.FormatB8g8r8a8Unorm,
	gputypes.TextureFormatBGRA8UnormSrgb: vk.FormatB8g8r8a8Srgb,

	// Packed formats
	gputypes.TextureFormatRGB9E5Ufloat:  vk.FormatE5b9g9r9UfloatPack32,
	gputypes.TextureFormatRGB10A2Uint:   vk.FormatA2b10g10r10UintPack32,
	gputypes.TextureFormatRGB10A2Unorm:  vk.FormatA2b10g10r10UnormPack32,
	gputypes.TextureFormatRG11B10Ufloat: vk.FormatB10g11r11UfloatPack32,

	// 64-bit formats
	gputypes.TextureFormatRG32Uint:    vk.FormatR32g32Uint,
	gputypes.TextureFormatRG32Sint:    vk.FormatR32g32Sint,
	gputypes.TextureFormatRG32Float:   vk.FormatR32g32Sfloat,
	gputypes.TextureFormatRGBA16Uint:  vk.FormatR16g16b16a16Uint,
	gputypes.TextureFormatRGBA16Sint:  vk.FormatR16g16b16a16Sint,
	gputypes.TextureFormatRGBA16Float: vk.FormatR16g16b16a16Sfloat,

	// 128-bit formats
	gputypes.TextureFormatRGBA32Uint:  vk.FormatR32g32b32a32Uint,
	gputypes.TextureFormatRGBA32Sint:  vk.FormatR32g32b32a32Sint,
	gputypes.TextureFormatRGBA32Float: vk.FormatR32g32b32a32Sfloat,

	// Depth/stencil formats
	gputypes.TextureFormatStencil8:             vk.FormatS8Uint,
	gputypes.TextureFormatDepth16Unorm:         vk.FormatD16Unorm,
	gputypes.TextureFormatDepth24Plus:          vk.FormatX8D24UnormPack32,
	gputypes.TextureFormatDepth24PlusStencil8:  vk.FormatD24UnormS8Uint,
	gputypes.TextureFormatDepth32Float:         vk.FormatD32Sfloat,
	gputypes.TextureFormatDepth32FloatStencil8: vk.FormatD32SfloatS8Uint,

	// BC compressed formats
	gputypes.TextureFormatBC1RGBAUnorm:     vk.FormatBc1RgbaUnormBlock,
	gputypes.TextureFormatBC1RGBAUnormSrgb: vk.FormatBc1RgbaSrgbBlock,
	gputypes.TextureFormatBC2RGBAUnorm:     vk.FormatBc2UnormBlock,
	gputypes.TextureFormatBC2RGBAUnormSrgb: vk.FormatBc2SrgbBlock,
	gputypes.TextureFormatBC3RGBAUnorm:     vk.FormatBc3UnormBlock,
	gputypes.TextureFormatBC3RGBAUnormSrgb: vk.FormatBc3SrgbBlock,
	gputypes.TextureFormatBC4RUnorm:        vk.FormatBc4UnormBlock,
	gputypes.TextureFormatBC4RSnorm:        vk.FormatBc4SnormBlock,
	gputypes.TextureFormatBC5RGUnorm:       vk.FormatBc5UnormBlock,
	gputypes.TextureFormatBC5RGSnorm:       vk.FormatBc5SnormBlock,
	gputypes.TextureFormatBC6HRGBUfloat:    vk.FormatBc6hUfloatBlock,
	gputypes.TextureFormatBC6HRGBFloat:     vk.FormatBc6hSfloatBlock,
	gputypes.TextureFormatBC7RGBAUnorm:     vk.FormatBc7UnormBlock,
	gputypes.TextureFormatBC7RGBAUnormSrgb: vk.FormatBc7SrgbBlock,

	// ETC2 compressed formats
	gputypes.TextureFormatETC2RGB8Unorm:       vk.FormatEtc2R8g8b8UnormBlock,
	gputypes.TextureFormatETC2RGB8UnormSrgb:   vk.FormatEtc2R8g8b8SrgbBlock,
	gputypes.TextureFormatETC2RGB8A1Unorm:     vk.FormatEtc2R8g8b8a1UnormBlock,
	gputypes.TextureFormatETC2RGB8A1UnormSrgb: vk.FormatEtc2R8g8b8a1SrgbBlock,
	gputypes.TextureFormatETC2RGBA8Unorm:      vk.FormatEtc2R8g8b8a8UnormBlock,
	gputypes.TextureFormatETC2RGBA8UnormSrgb:  vk.FormatEtc2R8g8b8a8SrgbBlock,
	gputypes.TextureFormatEACR11Unorm:         vk.FormatEacR11UnormBlock,
	gputypes.TextureFormatEACR11Snorm:         vk.FormatEacR11SnormBlock,
	gputypes.TextureFormatEACRG11Unorm:        vk.FormatEacR11g11UnormBlock,
	gputypes.TextureFormatEACRG11Snorm:        vk.FormatEacR11g11SnormBlock,

	// ASTC compressed formats
	gputypes.TextureFormatASTC4x4Unorm:       vk.FormatAstc4x4UnormBlock,
	gputypes.TextureFormatASTC4x4UnormSrgb:   vk.FormatAstc4x4SrgbBlock,
	gputypes.TextureFormatASTC5x4Unorm:       vk.FormatAstc5x4UnormBlock,
	gputypes.TextureFormatASTC5x4UnormSrgb:   vk.FormatAstc5x4SrgbBlock,
	gputypes.TextureFormatASTC5x5Unorm:       vk.FormatAstc5x5UnormBlock,
	gputypes.TextureFormatASTC5x5UnormSrgb:   vk.FormatAstc5x5SrgbBlock,
	gputypes.TextureFormatASTC6x5Unorm:       vk.FormatAstc6x5UnormBlock,
	gputypes.TextureFormatASTC6x5UnormSrgb:   vk.FormatAstc6x5SrgbBlock,
	gputypes.TextureFormatASTC6x6Unorm:       vk.FormatAstc6x6UnormBlock,
	gputypes.TextureFormatASTC6x6UnormSrgb:   vk.FormatAstc6x6SrgbBlock,
	gputypes.TextureFormatASTC8x5Unorm:       vk.FormatAstc8x5UnormBlock,
	gputypes.TextureFormatASTC8x5UnormSrgb:   vk.FormatAstc8x5SrgbBlock,
	gputypes.TextureFormatASTC8x6Unorm:       vk.FormatAstc8x6UnormBlock,
	gputypes.TextureFormatASTC8x6UnormSrgb:   vk.FormatAstc8x6SrgbBlock,
	gputypes.TextureFormatASTC8x8Unorm:       vk.FormatAstc8x8UnormBlock,
	gputypes.TextureFormatASTC8x8UnormSrgb:   vk.FormatAstc8x8SrgbBlock,
	gputypes.TextureFormatASTC10x5Unorm:      vk.FormatAstc10x5UnormBlock,
	gputypes.TextureFormatASTC10x5UnormSrgb:  vk.FormatAstc10x5SrgbBlock,
	gputypes.TextureFormatASTC10x6Unorm:      vk.FormatAstc10x6UnormBlock,
	gputypes.TextureFormatASTC10x6UnormSrgb:  vk.FormatAstc10x6SrgbBlock,
	gputypes.TextureFormatASTC10x8Unorm:      vk.FormatAstc10x8UnormBlock,
	gputypes.TextureFormatASTC10x8UnormSrgb:  vk.FormatAstc10x8SrgbBlock,
	gputypes.TextureFormatASTC10x10Unorm:     vk.FormatAstc10x10UnormBlock,
	gputypes.TextureFormatASTC10x10UnormSrgb: vk.FormatAstc10x10SrgbBlock,
	gputypes.TextureFormatASTC12x10Unorm:     vk.FormatAstc12x10UnormBlock,
	gputypes.TextureFormatASTC12x10UnormSrgb: vk.FormatAstc12x10SrgbBlock,
	gputypes.TextureFormatASTC12x12Unorm:     vk.FormatAstc12x12UnormBlock,
	gputypes.TextureFormatASTC12x12UnormSrgb: vk.FormatAstc12x12SrgbBlock,
}

// addressModeToVk converts WebGPU address mode to Vulkan sampler address mode.
func addressModeToVk(mode gputypes.AddressMode) vk.SamplerAddressMode {
	switch mode {
	case gputypes.AddressModeClampToEdge:
		return vk.SamplerAddressModeClampToEdge
	case gputypes.AddressModeRepeat:
		return vk.SamplerAddressModeRepeat
	case gputypes.AddressModeMirrorRepeat:
		return vk.SamplerAddressModeMirroredRepeat
	default:
		return vk.SamplerAddressModeClampToEdge
	}
}

// filterModeToVk converts WebGPU filter mode to Vulkan filter.
func filterModeToVk(mode gputypes.FilterMode) vk.Filter {
	switch mode {
	case gputypes.FilterModeNearest:
		return vk.FilterNearest
	case gputypes.FilterModeLinear:
		return vk.FilterLinear
	default:
		return vk.FilterNearest
	}
}

// mipmapFilterModeToVk converts WebGPU mipmap filter mode to Vulkan sampler mipmap mode.
func mipmapFilterModeToVk(mode gputypes.FilterMode) vk.SamplerMipmapMode {
	switch mode {
	case gputypes.FilterModeNearest:
		return vk.SamplerMipmapModeNearest
	case gputypes.FilterModeLinear:
		return vk.SamplerMipmapModeLinear
	default:
		return vk.SamplerMipmapModeNearest
	}
}

// compareFunctionToVk converts WebGPU compare function to Vulkan compare op.
func compareFunctionToVk(fn gputypes.CompareFunction) vk.CompareOp {
	switch fn {
	case gputypes.CompareFunctionNever:
		return vk.CompareOpNever
	case gputypes.CompareFunctionLess:
		return vk.CompareOpLess
	case gputypes.CompareFunctionEqual:
		return vk.CompareOpEqual
	case gputypes.CompareFunctionLessEqual:
		return vk.CompareOpLessOrEqual
	case gputypes.CompareFunctionGreater:
		return vk.CompareOpGreater
	case gputypes.CompareFunctionNotEqual:
		return vk.CompareOpNotEqual
	case gputypes.CompareFunctionGreaterEqual:
		return vk.CompareOpGreaterOrEqual
	case gputypes.CompareFunctionAlways:
		return vk.CompareOpAlways
	default:
		return vk.CompareOpNever
	}
}

// shaderStagesToVk converts WebGPU shader stages to Vulkan shader stage flags.
func shaderStagesToVk(stages gputypes.ShaderStages) vk.ShaderStageFlags {
	var flags vk.ShaderStageFlags

	if stages&gputypes.ShaderStageVertex != 0 {
		flags |= vk.ShaderStageFlags(vk.ShaderStageVertexBit)
	}
	if stages&gputypes.ShaderStageFragment != 0 {
		flags |= vk.ShaderStageFlags(vk.ShaderStageFragmentBit)
	}
	if stages&gputypes.ShaderStageCompute != 0 {
		flags |= vk.ShaderStageFlags(vk.ShaderStageComputeBit)
	}

	return flags
}

// bufferBindingTypeToVk converts WebGPU buffer binding type to Vulkan descriptor type.
func bufferBindingTypeToVk(bindingType gputypes.BufferBindingType) vk.DescriptorType {
	switch bindingType {
	case gputypes.BufferBindingTypeUniform:
		return vk.DescriptorTypeUniformBuffer
	case gputypes.BufferBindingTypeStorage:
		return vk.DescriptorTypeStorageBuffer
	case gputypes.BufferBindingTypeReadOnlyStorage:
		return vk.DescriptorTypeStorageBuffer
	default:
		return vk.DescriptorTypeUniformBuffer
	}
}

// vertexStepModeToVk converts WebGPU vertex step mode to Vulkan input rate.
func vertexStepModeToVk(mode gputypes.VertexStepMode) vk.VertexInputRate {
	switch mode {
	case gputypes.VertexStepModeVertex:
		return vk.VertexInputRateVertex
	case gputypes.VertexStepModeInstance:
		return vk.VertexInputRateInstance
	default:
		return vk.VertexInputRateVertex
	}
}

// vertexFormatToVk converts WebGPU vertex format to Vulkan format.
func vertexFormatToVk(format gputypes.VertexFormat) vk.Format {
	switch format {
	// 8-bit formats
	case gputypes.VertexFormatUint8x2:
		return vk.FormatR8g8Uint
	case gputypes.VertexFormatUint8x4:
		return vk.FormatR8g8b8a8Uint
	case gputypes.VertexFormatSint8x2:
		return vk.FormatR8g8Sint
	case gputypes.VertexFormatSint8x4:
		return vk.FormatR8g8b8a8Sint
	case gputypes.VertexFormatUnorm8x2:
		return vk.FormatR8g8Unorm
	case gputypes.VertexFormatUnorm8x4:
		return vk.FormatR8g8b8a8Unorm
	case gputypes.VertexFormatSnorm8x2:
		return vk.FormatR8g8Snorm
	case gputypes.VertexFormatSnorm8x4:
		return vk.FormatR8g8b8a8Snorm

	// 16-bit formats
	case gputypes.VertexFormatUint16x2:
		return vk.FormatR16g16Uint
	case gputypes.VertexFormatUint16x4:
		return vk.FormatR16g16b16a16Uint
	case gputypes.VertexFormatSint16x2:
		return vk.FormatR16g16Sint
	case gputypes.VertexFormatSint16x4:
		return vk.FormatR16g16b16a16Sint
	case gputypes.VertexFormatUnorm16x2:
		return vk.FormatR16g16Unorm
	case gputypes.VertexFormatUnorm16x4:
		return vk.FormatR16g16b16a16Unorm
	case gputypes.VertexFormatSnorm16x2:
		return vk.FormatR16g16Snorm
	case gputypes.VertexFormatSnorm16x4:
		return vk.FormatR16g16b16a16Snorm
	case gputypes.VertexFormatFloat16x2:
		return vk.FormatR16g16Sfloat
	case gputypes.VertexFormatFloat16x4:
		return vk.FormatR16g16b16a16Sfloat

	// 32-bit formats
	case gputypes.VertexFormatFloat32:
		return vk.FormatR32Sfloat
	case gputypes.VertexFormatFloat32x2:
		return vk.FormatR32g32Sfloat
	case gputypes.VertexFormatFloat32x3:
		return vk.FormatR32g32b32Sfloat
	case gputypes.VertexFormatFloat32x4:
		return vk.FormatR32g32b32a32Sfloat
	case gputypes.VertexFormatUint32:
		return vk.FormatR32Uint
	case gputypes.VertexFormatUint32x2:
		return vk.FormatR32g32Uint
	case gputypes.VertexFormatUint32x3:
		return vk.FormatR32g32b32Uint
	case gputypes.VertexFormatUint32x4:
		return vk.FormatR32g32b32a32Uint
	case gputypes.VertexFormatSint32:
		return vk.FormatR32Sint
	case gputypes.VertexFormatSint32x2:
		return vk.FormatR32g32Sint
	case gputypes.VertexFormatSint32x3:
		return vk.FormatR32g32b32Sint
	case gputypes.VertexFormatSint32x4:
		return vk.FormatR32g32b32a32Sint

	// Packed formats
	case gputypes.VertexFormatUnorm1010102:
		return vk.FormatA2b10g10r10UnormPack32

	default:
		return vk.FormatR32g32b32a32Sfloat
	}
}

// primitiveTopologyToVk converts WebGPU primitive topology to Vulkan topology.
func primitiveTopologyToVk(topology gputypes.PrimitiveTopology) vk.PrimitiveTopology {
	switch topology {
	case gputypes.PrimitiveTopologyPointList:
		return vk.PrimitiveTopologyPointList
	case gputypes.PrimitiveTopologyLineList:
		return vk.PrimitiveTopologyLineList
	case gputypes.PrimitiveTopologyLineStrip:
		return vk.PrimitiveTopologyLineStrip
	case gputypes.PrimitiveTopologyTriangleList:
		return vk.PrimitiveTopologyTriangleList
	case gputypes.PrimitiveTopologyTriangleStrip:
		return vk.PrimitiveTopologyTriangleStrip
	default:
		return vk.PrimitiveTopologyTriangleList
	}
}

// cullModeToVk converts WebGPU cull mode to Vulkan cull mode flags.
func cullModeToVk(mode gputypes.CullMode) vk.CullModeFlags {
	switch mode {
	case gputypes.CullModeNone:
		return vk.CullModeFlags(vk.CullModeNone)
	case gputypes.CullModeFront:
		return vk.CullModeFlags(vk.CullModeFrontBit)
	case gputypes.CullModeBack:
		return vk.CullModeFlags(vk.CullModeBackBit)
	default:
		return vk.CullModeFlags(vk.CullModeNone)
	}
}

// frontFaceToVk converts WebGPU front face to Vulkan front face.
func frontFaceToVk(face gputypes.FrontFace) vk.FrontFace {
	switch face {
	case gputypes.FrontFaceCCW:
		return vk.FrontFaceCounterClockwise
	case gputypes.FrontFaceCW:
		return vk.FrontFaceClockwise
	default:
		return vk.FrontFaceCounterClockwise
	}
}

// colorWriteMaskToVk converts WebGPU color write mask to Vulkan color component flags.
func colorWriteMaskToVk(mask gputypes.ColorWriteMask) vk.ColorComponentFlags {
	var flags vk.ColorComponentFlags
	if mask&gputypes.ColorWriteMaskRed != 0 {
		flags |= vk.ColorComponentFlags(vk.ColorComponentRBit)
	}
	if mask&gputypes.ColorWriteMaskGreen != 0 {
		flags |= vk.ColorComponentFlags(vk.ColorComponentGBit)
	}
	if mask&gputypes.ColorWriteMaskBlue != 0 {
		flags |= vk.ColorComponentFlags(vk.ColorComponentBBit)
	}
	if mask&gputypes.ColorWriteMaskAlpha != 0 {
		flags |= vk.ColorComponentFlags(vk.ColorComponentABit)
	}
	return flags
}

// blendFactorToVk converts WebGPU blend factor to Vulkan blend factor.
func blendFactorToVk(factor gputypes.BlendFactor) vk.BlendFactor {
	switch factor {
	case gputypes.BlendFactorZero:
		return vk.BlendFactorZero
	case gputypes.BlendFactorOne:
		return vk.BlendFactorOne
	case gputypes.BlendFactorSrc:
		return vk.BlendFactorSrcColor
	case gputypes.BlendFactorOneMinusSrc:
		return vk.BlendFactorOneMinusSrcColor
	case gputypes.BlendFactorSrcAlpha:
		return vk.BlendFactorSrcAlpha
	case gputypes.BlendFactorOneMinusSrcAlpha:
		return vk.BlendFactorOneMinusSrcAlpha
	case gputypes.BlendFactorDst:
		return vk.BlendFactorDstColor
	case gputypes.BlendFactorOneMinusDst:
		return vk.BlendFactorOneMinusDstColor
	case gputypes.BlendFactorDstAlpha:
		return vk.BlendFactorDstAlpha
	case gputypes.BlendFactorOneMinusDstAlpha:
		return vk.BlendFactorOneMinusDstAlpha
	case gputypes.BlendFactorSrcAlphaSaturated:
		return vk.BlendFactorSrcAlphaSaturate
	case gputypes.BlendFactorConstant:
		return vk.BlendFactorConstantColor
	case gputypes.BlendFactorOneMinusConstant:
		return vk.BlendFactorOneMinusConstantColor
	default:
		return vk.BlendFactorOne
	}
}

// blendOperationToVk converts WebGPU blend operation to Vulkan blend op.
func blendOperationToVk(op gputypes.BlendOperation) vk.BlendOp {
	switch op {
	case gputypes.BlendOperationAdd:
		return vk.BlendOpAdd
	case gputypes.BlendOperationSubtract:
		return vk.BlendOpSubtract
	case gputypes.BlendOperationReverseSubtract:
		return vk.BlendOpReverseSubtract
	case gputypes.BlendOperationMin:
		return vk.BlendOpMin
	case gputypes.BlendOperationMax:
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
func textureViewDimensionToVk(dim gputypes.TextureViewDimension) vk.ImageViewType {
	switch dim {
	case gputypes.TextureViewDimension1D:
		return vk.ImageViewType1d
	case gputypes.TextureViewDimension2D:
		return vk.ImageViewType2d
	case gputypes.TextureViewDimension2DArray:
		return vk.ImageViewType2dArray
	case gputypes.TextureViewDimensionCube:
		return vk.ImageViewTypeCube
	case gputypes.TextureViewDimensionCubeArray:
		return vk.ImageViewTypeCubeArray
	case gputypes.TextureViewDimension3D:
		return vk.ImageViewType3d
	default:
		return vk.ImageViewType2d
	}
}

// textureAspectToVk converts WebGPU texture aspect to Vulkan image aspect flags.
func textureAspectToVk(aspect gputypes.TextureAspect, format gputypes.TextureFormat) vk.ImageAspectFlags {
	switch aspect {
	case gputypes.TextureAspectDepthOnly:
		return vk.ImageAspectFlags(vk.ImageAspectDepthBit)
	case gputypes.TextureAspectStencilOnly:
		return vk.ImageAspectFlags(vk.ImageAspectStencilBit)
	default:
		// TextureAspectAll and TextureAspectUndefined both derive
		// the correct aspect mask from the texture format.
		if isDepthStencilFormat(format) {
			flags := vk.ImageAspectFlags(vk.ImageAspectDepthBit)
			if hasStencilAspect(format) {
				flags |= vk.ImageAspectFlags(vk.ImageAspectStencilBit)
			}
			return flags
		}
		return vk.ImageAspectFlags(vk.ImageAspectColorBit)
	}
}

// textureAspectToVkSimple converts texture aspect without format context.
// Used when texture format is not available (e.g., in buffer-texture copy regions).
func textureAspectToVkSimple(aspect gputypes.TextureAspect) vk.ImageAspectFlags {
	switch aspect {
	case gputypes.TextureAspectDepthOnly:
		return vk.ImageAspectFlags(vk.ImageAspectDepthBit)
	case gputypes.TextureAspectStencilOnly:
		return vk.ImageAspectFlags(vk.ImageAspectStencilBit)
	default:
		return vk.ImageAspectFlags(vk.ImageAspectColorBit)
	}
}

// isDepthStencilFormat returns true if the format is a depth or depth-stencil format.
func isDepthStencilFormat(format gputypes.TextureFormat) bool {
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

// hasStencilAspect returns true if the format has a stencil aspect.
func hasStencilAspect(format gputypes.TextureFormat) bool {
	switch format {
	case gputypes.TextureFormatDepth24PlusStencil8,
		gputypes.TextureFormatDepth32FloatStencil8,
		gputypes.TextureFormatStencil8:
		return true
	default:
		return false
	}
}

// textureDimensionToViewType converts WebGPU texture dimension to default Vulkan image view type.
func textureDimensionToViewType(dim gputypes.TextureDimension) vk.ImageViewType {
	switch dim {
	case gputypes.TextureDimension1D:
		return vk.ImageViewType1d
	case gputypes.TextureDimension2D:
		return vk.ImageViewType2d
	case gputypes.TextureDimension3D:
		return vk.ImageViewType3d
	default:
		return vk.ImageViewType2d
	}
}

// vkFormatFeaturesToHAL converts Vulkan format feature flags to HAL texture format capability flags.
func vkFormatFeaturesToHAL(features vk.FormatFeatureFlags) hal.TextureFormatCapabilityFlags {
	var flags hal.TextureFormatCapabilityFlags

	if features&vk.FormatFeatureFlags(vk.FormatFeatureSampledImageBit) != 0 {
		flags |= hal.TextureFormatCapabilitySampled
	}
	if features&vk.FormatFeatureFlags(vk.FormatFeatureStorageImageBit) != 0 {
		flags |= hal.TextureFormatCapabilityStorage
	}
	if features&vk.FormatFeatureFlags(vk.FormatFeatureColorAttachmentBit) != 0 {
		flags |= hal.TextureFormatCapabilityRenderAttachment
	}
	if features&vk.FormatFeatureFlags(vk.FormatFeatureColorAttachmentBlendBit) != 0 {
		flags |= hal.TextureFormatCapabilityBlendable
	}
	if features&vk.FormatFeatureFlags(vk.FormatFeatureDepthStencilAttachmentBit) != 0 {
		flags |= hal.TextureFormatCapabilityRenderAttachment
	}

	return flags
}

// vkFormatToTextureFormat converts Vulkan format to WebGPU texture format.
// Returns TextureFormatUndefined for unsupported formats.
func vkFormatToTextureFormat(format vk.Format) gputypes.TextureFormat {
	if f, ok := vkFormatToTextureMap[format]; ok {
		return f
	}
	return gputypes.TextureFormatUndefined
}

// vkFormatToTextureMap is the reverse mapping from Vulkan formats to WebGPU formats.
var vkFormatToTextureMap = map[vk.Format]gputypes.TextureFormat{
	// BGRA formats (common surface formats)
	vk.FormatB8g8r8a8Unorm: gputypes.TextureFormatBGRA8Unorm,
	vk.FormatB8g8r8a8Srgb:  gputypes.TextureFormatBGRA8UnormSrgb,

	// RGBA formats
	vk.FormatR8g8b8a8Unorm: gputypes.TextureFormatRGBA8Unorm,
	vk.FormatR8g8b8a8Srgb:  gputypes.TextureFormatRGBA8UnormSrgb,
	vk.FormatR8g8b8a8Snorm: gputypes.TextureFormatRGBA8Snorm,
	vk.FormatR8g8b8a8Uint:  gputypes.TextureFormatRGBA8Uint,
	vk.FormatR8g8b8a8Sint:  gputypes.TextureFormatRGBA8Sint,

	// 16-bit float formats
	vk.FormatR16g16b16a16Sfloat: gputypes.TextureFormatRGBA16Float,

	// 32-bit float formats
	vk.FormatR32g32b32a32Sfloat: gputypes.TextureFormatRGBA32Float,

	// Single channel formats
	vk.FormatR8Unorm:   gputypes.TextureFormatR8Unorm,
	vk.FormatR16Sfloat: gputypes.TextureFormatR16Float,
}

// vkPresentModeToHAL converts Vulkan present mode to HAL present mode.
func vkPresentModeToHAL(mode vk.PresentModeKHR) hal.PresentMode {
	switch mode {
	case vk.PresentModeImmediateKhr:
		return hal.PresentModeImmediate
	case vk.PresentModeMailboxKhr:
		return hal.PresentModeMailbox
	case vk.PresentModeFifoKhr:
		return hal.PresentModeFifo
	case vk.PresentModeFifoRelaxedKhr:
		return hal.PresentModeFifoRelaxed
	default:
		return hal.PresentModeFifo
	}
}

// vkCompositeAlphaToHAL converts Vulkan composite alpha flags to HAL composite alpha modes.
func vkCompositeAlphaToHAL(flags vk.CompositeAlphaFlagsKHR) []hal.CompositeAlphaMode {
	var modes []hal.CompositeAlphaMode

	if vk.Flags(flags)&vk.Flags(vk.CompositeAlphaOpaqueBitKhr) != 0 {
		modes = append(modes, hal.CompositeAlphaModeOpaque)
	}
	if vk.Flags(flags)&vk.Flags(vk.CompositeAlphaPreMultipliedBitKhr) != 0 {
		modes = append(modes, hal.CompositeAlphaModePremultiplied)
	}
	if vk.Flags(flags)&vk.Flags(vk.CompositeAlphaPostMultipliedBitKhr) != 0 {
		modes = append(modes, hal.CompositeAlphaModeUnpremultiplied)
	}
	if vk.Flags(flags)&vk.Flags(vk.CompositeAlphaInheritBitKhr) != 0 {
		modes = append(modes, hal.CompositeAlphaModeInherit)
	}

	// Always provide at least opaque mode
	if len(modes) == 0 {
		modes = append(modes, hal.CompositeAlphaModeOpaque)
	}

	return modes
}
