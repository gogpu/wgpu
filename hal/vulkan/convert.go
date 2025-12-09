// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows

package vulkan

import (
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
