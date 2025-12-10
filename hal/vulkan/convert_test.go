// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vulkan

import (
	"testing"

	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/vulkan/vk"
	"github.com/gogpu/wgpu/types"
)

// TestBufferUsageToVk tests buffer usage flag conversions.
func TestBufferUsageToVk(t *testing.T) {
	tests := []struct {
		name   string
		usage  types.BufferUsage
		expect vk.BufferUsageFlags
	}{
		{
			name:   "CopySrc",
			usage:  types.BufferUsageCopySrc,
			expect: vk.BufferUsageFlags(vk.BufferUsageTransferSrcBit),
		},
		{
			name:   "CopyDst",
			usage:  types.BufferUsageCopyDst,
			expect: vk.BufferUsageFlags(vk.BufferUsageTransferDstBit),
		},
		{
			name:   "Index",
			usage:  types.BufferUsageIndex,
			expect: vk.BufferUsageFlags(vk.BufferUsageIndexBufferBit),
		},
		{
			name:   "Vertex",
			usage:  types.BufferUsageVertex,
			expect: vk.BufferUsageFlags(vk.BufferUsageVertexBufferBit),
		},
		{
			name:   "Uniform",
			usage:  types.BufferUsageUniform,
			expect: vk.BufferUsageFlags(vk.BufferUsageUniformBufferBit),
		},
		{
			name:   "Storage",
			usage:  types.BufferUsageStorage,
			expect: vk.BufferUsageFlags(vk.BufferUsageStorageBufferBit),
		},
		{
			name:   "Indirect",
			usage:  types.BufferUsageIndirect,
			expect: vk.BufferUsageFlags(vk.BufferUsageIndirectBufferBit),
		},
		{
			name:  "Multiple flags",
			usage: types.BufferUsageVertex | types.BufferUsageIndex,
			expect: vk.BufferUsageFlags(vk.BufferUsageVertexBufferBit) |
				vk.BufferUsageFlags(vk.BufferUsageIndexBufferBit),
		},
		{
			name:   "All flags",
			usage:  types.BufferUsageCopySrc | types.BufferUsageCopyDst | types.BufferUsageIndex | types.BufferUsageVertex | types.BufferUsageUniform | types.BufferUsageStorage | types.BufferUsageIndirect,
			expect: vk.BufferUsageFlags(vk.BufferUsageTransferSrcBit) | vk.BufferUsageFlags(vk.BufferUsageTransferDstBit) | vk.BufferUsageFlags(vk.BufferUsageIndexBufferBit) | vk.BufferUsageFlags(vk.BufferUsageVertexBufferBit) | vk.BufferUsageFlags(vk.BufferUsageUniformBufferBit) | vk.BufferUsageFlags(vk.BufferUsageStorageBufferBit) | vk.BufferUsageFlags(vk.BufferUsageIndirectBufferBit),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := bufferUsageToVk(tt.usage)
			if got != tt.expect {
				t.Errorf("bufferUsageToVk() = %v, want %v", got, tt.expect)
			}
		})
	}
}

// TestTextureUsageToVk tests texture usage flag conversions.
func TestTextureUsageToVk(t *testing.T) {
	tests := []struct {
		name   string
		usage  types.TextureUsage
		expect vk.ImageUsageFlags
	}{
		{
			name:   "CopySrc",
			usage:  types.TextureUsageCopySrc,
			expect: vk.ImageUsageFlags(vk.ImageUsageTransferSrcBit),
		},
		{
			name:   "CopyDst",
			usage:  types.TextureUsageCopyDst,
			expect: vk.ImageUsageFlags(vk.ImageUsageTransferDstBit),
		},
		{
			name:   "TextureBinding",
			usage:  types.TextureUsageTextureBinding,
			expect: vk.ImageUsageFlags(vk.ImageUsageSampledBit),
		},
		{
			name:   "StorageBinding",
			usage:  types.TextureUsageStorageBinding,
			expect: vk.ImageUsageFlags(vk.ImageUsageStorageBit),
		},
		{
			name:   "RenderAttachment",
			usage:  types.TextureUsageRenderAttachment,
			expect: vk.ImageUsageFlags(vk.ImageUsageColorAttachmentBit),
		},
		{
			name:  "Multiple flags",
			usage: types.TextureUsageCopySrc | types.TextureUsageTextureBinding,
			expect: vk.ImageUsageFlags(vk.ImageUsageTransferSrcBit) |
				vk.ImageUsageFlags(vk.ImageUsageSampledBit),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := textureUsageToVk(tt.usage)
			if got != tt.expect {
				t.Errorf("textureUsageToVk() = %v, want %v", got, tt.expect)
			}
		})
	}
}

// TestTextureDimensionToVkImageType tests texture dimension conversions.
func TestTextureDimensionToVkImageType(t *testing.T) {
	tests := []struct {
		name   string
		dim    types.TextureDimension
		expect vk.ImageType
	}{
		{"1D", types.TextureDimension1D, vk.ImageType1d},
		{"2D", types.TextureDimension2D, vk.ImageType2d},
		{"3D", types.TextureDimension3D, vk.ImageType3d},
		{"Unknown defaults to 2D", types.TextureDimension(99), vk.ImageType2d},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := textureDimensionToVkImageType(tt.dim)
			if got != tt.expect {
				t.Errorf("textureDimensionToVkImageType() = %v, want %v", got, tt.expect)
			}
		})
	}
}

// TestTextureFormatToVk tests texture format conversions.
func TestTextureFormatToVk(t *testing.T) {
	tests := []struct {
		name   string
		format types.TextureFormat
		expect vk.Format
	}{
		// 8-bit formats
		{"R8Unorm", types.TextureFormatR8Unorm, vk.FormatR8Unorm},
		{"R8Snorm", types.TextureFormatR8Snorm, vk.FormatR8Snorm},
		{"R8Uint", types.TextureFormatR8Uint, vk.FormatR8Uint},
		{"R8Sint", types.TextureFormatR8Sint, vk.FormatR8Sint},

		// 16-bit formats
		{"R16Uint", types.TextureFormatR16Uint, vk.FormatR16Uint},
		{"R16Sint", types.TextureFormatR16Sint, vk.FormatR16Sint},
		{"R16Float", types.TextureFormatR16Float, vk.FormatR16Sfloat},
		{"RG8Unorm", types.TextureFormatRG8Unorm, vk.FormatR8g8Unorm},

		// 32-bit formats
		{"R32Uint", types.TextureFormatR32Uint, vk.FormatR32Uint},
		{"R32Sint", types.TextureFormatR32Sint, vk.FormatR32Sint},
		{"R32Float", types.TextureFormatR32Float, vk.FormatR32Sfloat},
		{"RGBA8Unorm", types.TextureFormatRGBA8Unorm, vk.FormatR8g8b8a8Unorm},
		{"RGBA8UnormSrgb", types.TextureFormatRGBA8UnormSrgb, vk.FormatR8g8b8a8Srgb},
		{"BGRA8Unorm", types.TextureFormatBGRA8Unorm, vk.FormatB8g8r8a8Unorm},
		{"BGRA8UnormSrgb", types.TextureFormatBGRA8UnormSrgb, vk.FormatB8g8r8a8Srgb},

		// Packed formats
		{"RGB9E5Ufloat", types.TextureFormatRGB9E5Ufloat, vk.FormatE5b9g9r9UfloatPack32},
		{"RGB10A2Uint", types.TextureFormatRGB10A2Uint, vk.FormatA2b10g10r10UintPack32},
		{"RGB10A2Unorm", types.TextureFormatRGB10A2Unorm, vk.FormatA2b10g10r10UnormPack32},
		{"RG11B10Ufloat", types.TextureFormatRG11B10Ufloat, vk.FormatB10g11r11UfloatPack32},

		// 64-bit formats
		{"RG32Uint", types.TextureFormatRG32Uint, vk.FormatR32g32Uint},
		{"RG32Float", types.TextureFormatRG32Float, vk.FormatR32g32Sfloat},
		{"RGBA16Float", types.TextureFormatRGBA16Float, vk.FormatR16g16b16a16Sfloat},

		// 128-bit formats
		{"RGBA32Float", types.TextureFormatRGBA32Float, vk.FormatR32g32b32a32Sfloat},

		// Depth/stencil formats
		{"Stencil8", types.TextureFormatStencil8, vk.FormatS8Uint},
		{"Depth16Unorm", types.TextureFormatDepth16Unorm, vk.FormatD16Unorm},
		{"Depth24Plus", types.TextureFormatDepth24Plus, vk.FormatX8D24UnormPack32},
		{"Depth24PlusStencil8", types.TextureFormatDepth24PlusStencil8, vk.FormatD24UnormS8Uint},
		{"Depth32Float", types.TextureFormatDepth32Float, vk.FormatD32Sfloat},
		{"Depth32FloatStencil8", types.TextureFormatDepth32FloatStencil8, vk.FormatD32SfloatS8Uint},

		// BC compressed formats
		{"BC1RGBAUnorm", types.TextureFormatBC1RGBAUnorm, vk.FormatBc1RgbaUnormBlock},
		{"BC1RGBAUnormSrgb", types.TextureFormatBC1RGBAUnormSrgb, vk.FormatBc1RgbaSrgbBlock},
		{"BC7RGBAUnorm", types.TextureFormatBC7RGBAUnorm, vk.FormatBc7UnormBlock},

		// ETC2 compressed formats
		{"ETC2RGB8Unorm", types.TextureFormatETC2RGB8Unorm, vk.FormatEtc2R8g8b8UnormBlock},
		{"ETC2RGBA8Unorm", types.TextureFormatETC2RGBA8Unorm, vk.FormatEtc2R8g8b8a8UnormBlock},

		// ASTC compressed formats
		{"ASTC4x4Unorm", types.TextureFormatASTC4x4Unorm, vk.FormatAstc4x4UnormBlock},
		{"ASTC12x12UnormSrgb", types.TextureFormatASTC12x12UnormSrgb, vk.FormatAstc12x12SrgbBlock},

		// Unknown format
		{"Unknown", types.TextureFormat(65535), vk.FormatUndefined},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := textureFormatToVk(tt.format)
			if got != tt.expect {
				t.Errorf("textureFormatToVk() = %v, want %v", got, tt.expect)
			}
		})
	}
}

// TestAddressModeToVk tests address mode conversions.
func TestAddressModeToVk(t *testing.T) {
	tests := []struct {
		name   string
		mode   types.AddressMode
		expect vk.SamplerAddressMode
	}{
		{"ClampToEdge", types.AddressModeClampToEdge, vk.SamplerAddressModeClampToEdge},
		{"Repeat", types.AddressModeRepeat, vk.SamplerAddressModeRepeat},
		{"MirrorRepeat", types.AddressModeMirrorRepeat, vk.SamplerAddressModeMirroredRepeat},
		{"Unknown defaults to ClampToEdge", types.AddressMode(99), vk.SamplerAddressModeClampToEdge},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := addressModeToVk(tt.mode)
			if got != tt.expect {
				t.Errorf("addressModeToVk() = %v, want %v", got, tt.expect)
			}
		})
	}
}

// TestFilterModeToVk tests filter mode conversions.
func TestFilterModeToVk(t *testing.T) {
	tests := []struct {
		name   string
		mode   types.FilterMode
		expect vk.Filter
	}{
		{"Nearest", types.FilterModeNearest, vk.FilterNearest},
		{"Linear", types.FilterModeLinear, vk.FilterLinear},
		{"Unknown defaults to Nearest", types.FilterMode(99), vk.FilterNearest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filterModeToVk(tt.mode)
			if got != tt.expect {
				t.Errorf("filterModeToVk() = %v, want %v", got, tt.expect)
			}
		})
	}
}

// TestMipmapFilterModeToVk tests mipmap filter mode conversions.
func TestMipmapFilterModeToVk(t *testing.T) {
	tests := []struct {
		name   string
		mode   types.FilterMode
		expect vk.SamplerMipmapMode
	}{
		{"Nearest", types.FilterModeNearest, vk.SamplerMipmapModeNearest},
		{"Linear", types.FilterModeLinear, vk.SamplerMipmapModeLinear},
		{"Unknown defaults to Nearest", types.FilterMode(99), vk.SamplerMipmapModeNearest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mipmapFilterModeToVk(tt.mode)
			if got != tt.expect {
				t.Errorf("mipmapFilterModeToVk() = %v, want %v", got, tt.expect)
			}
		})
	}
}

// TestCompareFunctionToVk tests compare function conversions.
func TestCompareFunctionToVk(t *testing.T) {
	tests := []struct {
		name   string
		fn     types.CompareFunction
		expect vk.CompareOp
	}{
		{"Never", types.CompareFunctionNever, vk.CompareOpNever},
		{"Less", types.CompareFunctionLess, vk.CompareOpLess},
		{"Equal", types.CompareFunctionEqual, vk.CompareOpEqual},
		{"LessEqual", types.CompareFunctionLessEqual, vk.CompareOpLessOrEqual},
		{"Greater", types.CompareFunctionGreater, vk.CompareOpGreater},
		{"NotEqual", types.CompareFunctionNotEqual, vk.CompareOpNotEqual},
		{"GreaterEqual", types.CompareFunctionGreaterEqual, vk.CompareOpGreaterOrEqual},
		{"Always", types.CompareFunctionAlways, vk.CompareOpAlways},
		{"Unknown defaults to Never", types.CompareFunction(99), vk.CompareOpNever},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := compareFunctionToVk(tt.fn)
			if got != tt.expect {
				t.Errorf("compareFunctionToVk() = %v, want %v", got, tt.expect)
			}
		})
	}
}

// TestShaderStagesToVk tests shader stage flag conversions.
func TestShaderStagesToVk(t *testing.T) {
	tests := []struct {
		name   string
		stages types.ShaderStages
		expect vk.ShaderStageFlags
	}{
		{"Vertex", types.ShaderStageVertex, vk.ShaderStageFlags(vk.ShaderStageVertexBit)},
		{"Fragment", types.ShaderStageFragment, vk.ShaderStageFlags(vk.ShaderStageFragmentBit)},
		{"Compute", types.ShaderStageCompute, vk.ShaderStageFlags(vk.ShaderStageComputeBit)},
		{
			"Vertex and Fragment",
			types.ShaderStageVertex | types.ShaderStageFragment,
			vk.ShaderStageFlags(vk.ShaderStageVertexBit) | vk.ShaderStageFlags(vk.ShaderStageFragmentBit),
		},
		{
			"All stages",
			types.ShaderStageVertex | types.ShaderStageFragment | types.ShaderStageCompute,
			vk.ShaderStageFlags(vk.ShaderStageVertexBit) | vk.ShaderStageFlags(vk.ShaderStageFragmentBit) | vk.ShaderStageFlags(vk.ShaderStageComputeBit),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shaderStagesToVk(tt.stages)
			if got != tt.expect {
				t.Errorf("shaderStagesToVk() = %v, want %v", got, tt.expect)
			}
		})
	}
}

// TestBufferBindingTypeToVk tests buffer binding type conversions.
func TestBufferBindingTypeToVk(t *testing.T) {
	tests := []struct {
		name        string
		bindingType types.BufferBindingType
		expect      vk.DescriptorType
	}{
		{"Uniform", types.BufferBindingTypeUniform, vk.DescriptorTypeUniformBuffer},
		{"Storage", types.BufferBindingTypeStorage, vk.DescriptorTypeStorageBuffer},
		{"ReadOnlyStorage", types.BufferBindingTypeReadOnlyStorage, vk.DescriptorTypeStorageBuffer},
		{"Unknown defaults to Uniform", types.BufferBindingType(99), vk.DescriptorTypeUniformBuffer},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := bufferBindingTypeToVk(tt.bindingType)
			if got != tt.expect {
				t.Errorf("bufferBindingTypeToVk() = %v, want %v", got, tt.expect)
			}
		})
	}
}

// TestVertexStepModeToVk tests vertex step mode conversions.
func TestVertexStepModeToVk(t *testing.T) {
	tests := []struct {
		name   string
		mode   types.VertexStepMode
		expect vk.VertexInputRate
	}{
		{"Vertex", types.VertexStepModeVertex, vk.VertexInputRateVertex},
		{"Instance", types.VertexStepModeInstance, vk.VertexInputRateInstance},
		{"Unknown defaults to Vertex", types.VertexStepMode(99), vk.VertexInputRateVertex},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := vertexStepModeToVk(tt.mode)
			if got != tt.expect {
				t.Errorf("vertexStepModeToVk() = %v, want %v", got, tt.expect)
			}
		})
	}
}

// TestVertexFormatToVk tests vertex format conversions.
func TestVertexFormatToVk(t *testing.T) {
	tests := []struct {
		name   string
		format types.VertexFormat
		expect vk.Format
	}{
		// 8-bit formats
		{"Uint8x2", types.VertexFormatUint8x2, vk.FormatR8g8Uint},
		{"Uint8x4", types.VertexFormatUint8x4, vk.FormatR8g8b8a8Uint},
		{"Sint8x2", types.VertexFormatSint8x2, vk.FormatR8g8Sint},
		{"Unorm8x4", types.VertexFormatUnorm8x4, vk.FormatR8g8b8a8Unorm},

		// 16-bit formats
		{"Uint16x2", types.VertexFormatUint16x2, vk.FormatR16g16Uint},
		{"Float16x4", types.VertexFormatFloat16x4, vk.FormatR16g16b16a16Sfloat},

		// 32-bit formats
		{"Float32", types.VertexFormatFloat32, vk.FormatR32Sfloat},
		{"Float32x2", types.VertexFormatFloat32x2, vk.FormatR32g32Sfloat},
		{"Float32x3", types.VertexFormatFloat32x3, vk.FormatR32g32b32Sfloat},
		{"Float32x4", types.VertexFormatFloat32x4, vk.FormatR32g32b32a32Sfloat},
		{"Uint32", types.VertexFormatUint32, vk.FormatR32Uint},
		{"Sint32x4", types.VertexFormatSint32x4, vk.FormatR32g32b32a32Sint},

		// Packed formats
		{"Unorm1010102", types.VertexFormatUnorm1010102, vk.FormatA2b10g10r10UnormPack32},

		// Unknown format defaults to Float32x4
		{"Unknown", types.VertexFormat(255), vk.FormatR32g32b32a32Sfloat},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := vertexFormatToVk(tt.format)
			if got != tt.expect {
				t.Errorf("vertexFormatToVk() = %v, want %v", got, tt.expect)
			}
		})
	}
}

// TestPrimitiveTopologyToVk tests primitive topology conversions.
func TestPrimitiveTopologyToVk(t *testing.T) {
	tests := []struct {
		name     string
		topology types.PrimitiveTopology
		expect   vk.PrimitiveTopology
	}{
		{"PointList", types.PrimitiveTopologyPointList, vk.PrimitiveTopologyPointList},
		{"LineList", types.PrimitiveTopologyLineList, vk.PrimitiveTopologyLineList},
		{"LineStrip", types.PrimitiveTopologyLineStrip, vk.PrimitiveTopologyLineStrip},
		{"TriangleList", types.PrimitiveTopologyTriangleList, vk.PrimitiveTopologyTriangleList},
		{"TriangleStrip", types.PrimitiveTopologyTriangleStrip, vk.PrimitiveTopologyTriangleStrip},
		{"Unknown defaults to TriangleList", types.PrimitiveTopology(99), vk.PrimitiveTopologyTriangleList},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := primitiveTopologyToVk(tt.topology)
			if got != tt.expect {
				t.Errorf("primitiveTopologyToVk() = %v, want %v", got, tt.expect)
			}
		})
	}
}

// TestCullModeToVk tests cull mode conversions.
func TestCullModeToVk(t *testing.T) {
	tests := []struct {
		name   string
		mode   types.CullMode
		expect vk.CullModeFlags
	}{
		{"None", types.CullModeNone, vk.CullModeFlags(vk.CullModeNone)},
		{"Front", types.CullModeFront, vk.CullModeFlags(vk.CullModeFrontBit)},
		{"Back", types.CullModeBack, vk.CullModeFlags(vk.CullModeBackBit)},
		{"Unknown defaults to None", types.CullMode(99), vk.CullModeFlags(vk.CullModeNone)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cullModeToVk(tt.mode)
			if got != tt.expect {
				t.Errorf("cullModeToVk() = %v, want %v", got, tt.expect)
			}
		})
	}
}

// TestFrontFaceToVk tests front face conversions.
func TestFrontFaceToVk(t *testing.T) {
	tests := []struct {
		name   string
		face   types.FrontFace
		expect vk.FrontFace
	}{
		{"CCW", types.FrontFaceCCW, vk.FrontFaceCounterClockwise},
		{"CW", types.FrontFaceCW, vk.FrontFaceClockwise},
		{"Unknown defaults to CCW", types.FrontFace(99), vk.FrontFaceCounterClockwise},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := frontFaceToVk(tt.face)
			if got != tt.expect {
				t.Errorf("frontFaceToVk() = %v, want %v", got, tt.expect)
			}
		})
	}
}

// TestColorWriteMaskToVk tests color write mask conversions.
func TestColorWriteMaskToVk(t *testing.T) {
	tests := []struct {
		name   string
		mask   types.ColorWriteMask
		expect vk.ColorComponentFlags
	}{
		{"Red", types.ColorWriteMaskRed, vk.ColorComponentFlags(vk.ColorComponentRBit)},
		{"Green", types.ColorWriteMaskGreen, vk.ColorComponentFlags(vk.ColorComponentGBit)},
		{"Blue", types.ColorWriteMaskBlue, vk.ColorComponentFlags(vk.ColorComponentBBit)},
		{"Alpha", types.ColorWriteMaskAlpha, vk.ColorComponentFlags(vk.ColorComponentABit)},
		{
			"All",
			types.ColorWriteMaskRed | types.ColorWriteMaskGreen | types.ColorWriteMaskBlue | types.ColorWriteMaskAlpha,
			vk.ColorComponentFlags(vk.ColorComponentRBit) | vk.ColorComponentFlags(vk.ColorComponentGBit) | vk.ColorComponentFlags(vk.ColorComponentBBit) | vk.ColorComponentFlags(vk.ColorComponentABit),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := colorWriteMaskToVk(tt.mask)
			if got != tt.expect {
				t.Errorf("colorWriteMaskToVk() = %v, want %v", got, tt.expect)
			}
		})
	}
}

// TestBlendFactorToVk tests blend factor conversions.
func TestBlendFactorToVk(t *testing.T) {
	tests := []struct {
		name   string
		factor types.BlendFactor
		expect vk.BlendFactor
	}{
		{"Zero", types.BlendFactorZero, vk.BlendFactorZero},
		{"One", types.BlendFactorOne, vk.BlendFactorOne},
		{"Src", types.BlendFactorSrc, vk.BlendFactorSrcColor},
		{"OneMinusSrc", types.BlendFactorOneMinusSrc, vk.BlendFactorOneMinusSrcColor},
		{"SrcAlpha", types.BlendFactorSrcAlpha, vk.BlendFactorSrcAlpha},
		{"OneMinusSrcAlpha", types.BlendFactorOneMinusSrcAlpha, vk.BlendFactorOneMinusSrcAlpha},
		{"Dst", types.BlendFactorDst, vk.BlendFactorDstColor},
		{"OneMinusDst", types.BlendFactorOneMinusDst, vk.BlendFactorOneMinusDstColor},
		{"DstAlpha", types.BlendFactorDstAlpha, vk.BlendFactorDstAlpha},
		{"OneMinusDstAlpha", types.BlendFactorOneMinusDstAlpha, vk.BlendFactorOneMinusDstAlpha},
		{"SrcAlphaSaturated", types.BlendFactorSrcAlphaSaturated, vk.BlendFactorSrcAlphaSaturate},
		{"Constant", types.BlendFactorConstant, vk.BlendFactorConstantColor},
		{"OneMinusConstant", types.BlendFactorOneMinusConstant, vk.BlendFactorOneMinusConstantColor},
		{"Unknown defaults to One", types.BlendFactor(99), vk.BlendFactorOne},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := blendFactorToVk(tt.factor)
			if got != tt.expect {
				t.Errorf("blendFactorToVk() = %v, want %v", got, tt.expect)
			}
		})
	}
}

// TestBlendOperationToVk tests blend operation conversions.
func TestBlendOperationToVk(t *testing.T) {
	tests := []struct {
		name   string
		op     types.BlendOperation
		expect vk.BlendOp
	}{
		{"Add", types.BlendOperationAdd, vk.BlendOpAdd},
		{"Subtract", types.BlendOperationSubtract, vk.BlendOpSubtract},
		{"ReverseSubtract", types.BlendOperationReverseSubtract, vk.BlendOpReverseSubtract},
		{"Min", types.BlendOperationMin, vk.BlendOpMin},
		{"Max", types.BlendOperationMax, vk.BlendOpMax},
		{"Unknown defaults to Add", types.BlendOperation(99), vk.BlendOpAdd},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := blendOperationToVk(tt.op)
			if got != tt.expect {
				t.Errorf("blendOperationToVk() = %v, want %v", got, tt.expect)
			}
		})
	}
}

// TestStencilOperationToVk tests stencil operation conversions.
func TestStencilOperationToVk(t *testing.T) {
	tests := []struct {
		name   string
		op     hal.StencilOperation
		expect vk.StencilOp
	}{
		{"Keep", hal.StencilOperationKeep, vk.StencilOpKeep},
		{"Zero", hal.StencilOperationZero, vk.StencilOpZero},
		{"Replace", hal.StencilOperationReplace, vk.StencilOpReplace},
		{"Invert", hal.StencilOperationInvert, vk.StencilOpInvert},
		{"IncrementClamp", hal.StencilOperationIncrementClamp, vk.StencilOpIncrementAndClamp},
		{"DecrementClamp", hal.StencilOperationDecrementClamp, vk.StencilOpDecrementAndClamp},
		{"IncrementWrap", hal.StencilOperationIncrementWrap, vk.StencilOpIncrementAndWrap},
		{"DecrementWrap", hal.StencilOperationDecrementWrap, vk.StencilOpDecrementAndWrap},
		{"Unknown defaults to Keep", hal.StencilOperation(99), vk.StencilOpKeep},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stencilOperationToVk(tt.op)
			if got != tt.expect {
				t.Errorf("stencilOperationToVk() = %v, want %v", got, tt.expect)
			}
		})
	}
}

// TestStencilFaceStateToVk tests stencil face state conversions.
func TestStencilFaceStateToVk(t *testing.T) {
	state := hal.StencilFaceState{
		FailOp:      hal.StencilOperationKeep,
		PassOp:      hal.StencilOperationReplace,
		DepthFailOp: hal.StencilOperationIncrementClamp,
		Compare:     types.CompareFunctionLess,
	}

	got := stencilFaceStateToVk(state)

	if got.FailOp != vk.StencilOpKeep {
		t.Errorf("FailOp = %v, want %v", got.FailOp, vk.StencilOpKeep)
	}
	if got.PassOp != vk.StencilOpReplace {
		t.Errorf("PassOp = %v, want %v", got.PassOp, vk.StencilOpReplace)
	}
	if got.DepthFailOp != vk.StencilOpIncrementAndClamp {
		t.Errorf("DepthFailOp = %v, want %v", got.DepthFailOp, vk.StencilOpIncrementAndClamp)
	}
	if got.CompareOp != vk.CompareOpLess {
		t.Errorf("CompareOp = %v, want %v", got.CompareOp, vk.CompareOpLess)
	}
}

// TestTextureViewDimensionToVk tests texture view dimension conversions.
func TestTextureViewDimensionToVk(t *testing.T) {
	tests := []struct {
		name   string
		dim    types.TextureViewDimension
		expect vk.ImageViewType
	}{
		{"1D", types.TextureViewDimension1D, vk.ImageViewType1d},
		{"2D", types.TextureViewDimension2D, vk.ImageViewType2d},
		{"2DArray", types.TextureViewDimension2DArray, vk.ImageViewType2dArray},
		{"Cube", types.TextureViewDimensionCube, vk.ImageViewTypeCube},
		{"CubeArray", types.TextureViewDimensionCubeArray, vk.ImageViewTypeCubeArray},
		{"3D", types.TextureViewDimension3D, vk.ImageViewType3d},
		{"Unknown defaults to 2D", types.TextureViewDimension(99), vk.ImageViewType2d},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := textureViewDimensionToVk(tt.dim)
			if got != tt.expect {
				t.Errorf("textureViewDimensionToVk() = %v, want %v", got, tt.expect)
			}
		})
	}
}

// TestTextureAspectToVk tests texture aspect conversions with format context.
func TestTextureAspectToVk(t *testing.T) {
	tests := []struct {
		name   string
		aspect types.TextureAspect
		format types.TextureFormat
		expect vk.ImageAspectFlags
	}{
		{"DepthOnly", types.TextureAspectDepthOnly, types.TextureFormatDepth32Float, vk.ImageAspectFlags(vk.ImageAspectDepthBit)},
		{"StencilOnly", types.TextureAspectStencilOnly, types.TextureFormatStencil8, vk.ImageAspectFlags(vk.ImageAspectStencilBit)},
		{"All color", types.TextureAspectAll, types.TextureFormatRGBA8Unorm, vk.ImageAspectFlags(vk.ImageAspectColorBit)},
		{
			"All depth-stencil",
			types.TextureAspectAll,
			types.TextureFormatDepth24PlusStencil8,
			vk.ImageAspectFlags(vk.ImageAspectDepthBit) | vk.ImageAspectFlags(vk.ImageAspectStencilBit),
		},
		{"All depth only", types.TextureAspectAll, types.TextureFormatDepth32Float, vk.ImageAspectFlags(vk.ImageAspectDepthBit)},
		{"Unknown defaults to Color", types.TextureAspect(99), types.TextureFormatRGBA8Unorm, vk.ImageAspectFlags(vk.ImageAspectColorBit)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := textureAspectToVk(tt.aspect, tt.format)
			if got != tt.expect {
				t.Errorf("textureAspectToVk() = %v, want %v", got, tt.expect)
			}
		})
	}
}

// TestTextureAspectToVkSimple tests texture aspect conversions without format context.
func TestTextureAspectToVkSimple(t *testing.T) {
	tests := []struct {
		name   string
		aspect types.TextureAspect
		expect vk.ImageAspectFlags
	}{
		{"DepthOnly", types.TextureAspectDepthOnly, vk.ImageAspectFlags(vk.ImageAspectDepthBit)},
		{"StencilOnly", types.TextureAspectStencilOnly, vk.ImageAspectFlags(vk.ImageAspectStencilBit)},
		{"All defaults to Color", types.TextureAspectAll, vk.ImageAspectFlags(vk.ImageAspectColorBit)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := textureAspectToVkSimple(tt.aspect)
			if got != tt.expect {
				t.Errorf("textureAspectToVkSimple() = %v, want %v", got, tt.expect)
			}
		})
	}
}

// TestIsDepthStencilFormat tests depth-stencil format detection.
func TestIsDepthStencilFormat(t *testing.T) {
	tests := []struct {
		name   string
		format types.TextureFormat
		expect bool
	}{
		{"Depth16Unorm", types.TextureFormatDepth16Unorm, true},
		{"Depth24Plus", types.TextureFormatDepth24Plus, true},
		{"Depth24PlusStencil8", types.TextureFormatDepth24PlusStencil8, true},
		{"Depth32Float", types.TextureFormatDepth32Float, true},
		{"Depth32FloatStencil8", types.TextureFormatDepth32FloatStencil8, true},
		{"Stencil8", types.TextureFormatStencil8, true},
		{"RGBA8Unorm", types.TextureFormatRGBA8Unorm, false},
		{"R32Float", types.TextureFormatR32Float, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isDepthStencilFormat(tt.format)
			if got != tt.expect {
				t.Errorf("isDepthStencilFormat() = %v, want %v", got, tt.expect)
			}
		})
	}
}

// TestHasStencilAspect tests stencil aspect detection.
func TestHasStencilAspect(t *testing.T) {
	tests := []struct {
		name   string
		format types.TextureFormat
		expect bool
	}{
		{"Depth24PlusStencil8", types.TextureFormatDepth24PlusStencil8, true},
		{"Depth32FloatStencil8", types.TextureFormatDepth32FloatStencil8, true},
		{"Stencil8", types.TextureFormatStencil8, true},
		{"Depth16Unorm", types.TextureFormatDepth16Unorm, false},
		{"Depth32Float", types.TextureFormatDepth32Float, false},
		{"RGBA8Unorm", types.TextureFormatRGBA8Unorm, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasStencilAspect(tt.format)
			if got != tt.expect {
				t.Errorf("hasStencilAspect() = %v, want %v", got, tt.expect)
			}
		})
	}
}

// TestTextureDimensionToViewType tests texture dimension to view type conversions.
func TestTextureDimensionToViewType(t *testing.T) {
	tests := []struct {
		name   string
		dim    types.TextureDimension
		expect vk.ImageViewType
	}{
		{"1D", types.TextureDimension1D, vk.ImageViewType1d},
		{"2D", types.TextureDimension2D, vk.ImageViewType2d},
		{"3D", types.TextureDimension3D, vk.ImageViewType3d},
		{"Unknown defaults to 2D", types.TextureDimension(99), vk.ImageViewType2d},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := textureDimensionToViewType(tt.dim)
			if got != tt.expect {
				t.Errorf("textureDimensionToViewType() = %v, want %v", got, tt.expect)
			}
		})
	}
}
