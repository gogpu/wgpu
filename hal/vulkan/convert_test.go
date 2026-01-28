// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vulkan

import (
	"testing"

	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/vulkan/vk"
	"github.com/gogpu/gputypes"
)

// TestBufferUsageToVk tests buffer usage flag conversions.
func TestBufferUsageToVk(t *testing.T) {
	tests := []struct {
		name   string
		usage  gputypes.BufferUsage
		expect vk.BufferUsageFlags
	}{
		{
			name:   "CopySrc",
			usage:  gputypes.BufferUsageCopySrc,
			expect: vk.BufferUsageFlags(vk.BufferUsageTransferSrcBit),
		},
		{
			name:   "CopyDst",
			usage:  gputypes.BufferUsageCopyDst,
			expect: vk.BufferUsageFlags(vk.BufferUsageTransferDstBit),
		},
		{
			name:   "Index",
			usage:  gputypes.BufferUsageIndex,
			expect: vk.BufferUsageFlags(vk.BufferUsageIndexBufferBit),
		},
		{
			name:   "Vertex",
			usage:  gputypes.BufferUsageVertex,
			expect: vk.BufferUsageFlags(vk.BufferUsageVertexBufferBit),
		},
		{
			name:   "Uniform",
			usage:  gputypes.BufferUsageUniform,
			expect: vk.BufferUsageFlags(vk.BufferUsageUniformBufferBit),
		},
		{
			name:   "Storage",
			usage:  gputypes.BufferUsageStorage,
			expect: vk.BufferUsageFlags(vk.BufferUsageStorageBufferBit),
		},
		{
			name:   "Indirect",
			usage:  gputypes.BufferUsageIndirect,
			expect: vk.BufferUsageFlags(vk.BufferUsageIndirectBufferBit),
		},
		{
			name:  "Multiple flags",
			usage: gputypes.BufferUsageVertex | gputypes.BufferUsageIndex,
			expect: vk.BufferUsageFlags(vk.BufferUsageVertexBufferBit) |
				vk.BufferUsageFlags(vk.BufferUsageIndexBufferBit),
		},
		{
			name:   "All flags",
			usage:  gputypes.BufferUsageCopySrc | gputypes.BufferUsageCopyDst | gputypes.BufferUsageIndex | gputypes.BufferUsageVertex | gputypes.BufferUsageUniform | gputypes.BufferUsageStorage | gputypes.BufferUsageIndirect,
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
		usage  gputypes.TextureUsage
		expect vk.ImageUsageFlags
	}{
		{
			name:   "CopySrc",
			usage:  gputypes.TextureUsageCopySrc,
			expect: vk.ImageUsageFlags(vk.ImageUsageTransferSrcBit),
		},
		{
			name:   "CopyDst",
			usage:  gputypes.TextureUsageCopyDst,
			expect: vk.ImageUsageFlags(vk.ImageUsageTransferDstBit),
		},
		{
			name:   "TextureBinding",
			usage:  gputypes.TextureUsageTextureBinding,
			expect: vk.ImageUsageFlags(vk.ImageUsageSampledBit),
		},
		{
			name:   "StorageBinding",
			usage:  gputypes.TextureUsageStorageBinding,
			expect: vk.ImageUsageFlags(vk.ImageUsageStorageBit),
		},
		{
			name:   "RenderAttachment",
			usage:  gputypes.TextureUsageRenderAttachment,
			expect: vk.ImageUsageFlags(vk.ImageUsageColorAttachmentBit),
		},
		{
			name:  "Multiple flags",
			usage: gputypes.TextureUsageCopySrc | gputypes.TextureUsageTextureBinding,
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
		dim    gputypes.TextureDimension
		expect vk.ImageType
	}{
		{"1D", gputypes.TextureDimension1D, vk.ImageType1d},
		{"2D", gputypes.TextureDimension2D, vk.ImageType2d},
		{"3D", gputypes.TextureDimension3D, vk.ImageType3d},
		{"Unknown defaults to 2D", gputypes.TextureDimension(99), vk.ImageType2d},
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
		format gputypes.TextureFormat
		expect vk.Format
	}{
		// 8-bit formats
		{"R8Unorm", gputypes.TextureFormatR8Unorm, vk.FormatR8Unorm},
		{"R8Snorm", gputypes.TextureFormatR8Snorm, vk.FormatR8Snorm},
		{"R8Uint", gputypes.TextureFormatR8Uint, vk.FormatR8Uint},
		{"R8Sint", gputypes.TextureFormatR8Sint, vk.FormatR8Sint},

		// 16-bit formats
		{"R16Uint", gputypes.TextureFormatR16Uint, vk.FormatR16Uint},
		{"R16Sint", gputypes.TextureFormatR16Sint, vk.FormatR16Sint},
		{"R16Float", gputypes.TextureFormatR16Float, vk.FormatR16Sfloat},
		{"RG8Unorm", gputypes.TextureFormatRG8Unorm, vk.FormatR8g8Unorm},

		// 32-bit formats
		{"R32Uint", gputypes.TextureFormatR32Uint, vk.FormatR32Uint},
		{"R32Sint", gputypes.TextureFormatR32Sint, vk.FormatR32Sint},
		{"R32Float", gputypes.TextureFormatR32Float, vk.FormatR32Sfloat},
		{"RGBA8Unorm", gputypes.TextureFormatRGBA8Unorm, vk.FormatR8g8b8a8Unorm},
		{"RGBA8UnormSrgb", gputypes.TextureFormatRGBA8UnormSrgb, vk.FormatR8g8b8a8Srgb},
		{"BGRA8Unorm", gputypes.TextureFormatBGRA8Unorm, vk.FormatB8g8r8a8Unorm},
		{"BGRA8UnormSrgb", gputypes.TextureFormatBGRA8UnormSrgb, vk.FormatB8g8r8a8Srgb},

		// Packed formats
		{"RGB9E5Ufloat", gputypes.TextureFormatRGB9E5Ufloat, vk.FormatE5b9g9r9UfloatPack32},
		{"RGB10A2Uint", gputypes.TextureFormatRGB10A2Uint, vk.FormatA2b10g10r10UintPack32},
		{"RGB10A2Unorm", gputypes.TextureFormatRGB10A2Unorm, vk.FormatA2b10g10r10UnormPack32},
		{"RG11B10Ufloat", gputypes.TextureFormatRG11B10Ufloat, vk.FormatB10g11r11UfloatPack32},

		// 64-bit formats
		{"RG32Uint", gputypes.TextureFormatRG32Uint, vk.FormatR32g32Uint},
		{"RG32Float", gputypes.TextureFormatRG32Float, vk.FormatR32g32Sfloat},
		{"RGBA16Float", gputypes.TextureFormatRGBA16Float, vk.FormatR16g16b16a16Sfloat},

		// 128-bit formats
		{"RGBA32Float", gputypes.TextureFormatRGBA32Float, vk.FormatR32g32b32a32Sfloat},

		// Depth/stencil formats
		{"Stencil8", gputypes.TextureFormatStencil8, vk.FormatS8Uint},
		{"Depth16Unorm", gputypes.TextureFormatDepth16Unorm, vk.FormatD16Unorm},
		{"Depth24Plus", gputypes.TextureFormatDepth24Plus, vk.FormatX8D24UnormPack32},
		{"Depth24PlusStencil8", gputypes.TextureFormatDepth24PlusStencil8, vk.FormatD24UnormS8Uint},
		{"Depth32Float", gputypes.TextureFormatDepth32Float, vk.FormatD32Sfloat},
		{"Depth32FloatStencil8", gputypes.TextureFormatDepth32FloatStencil8, vk.FormatD32SfloatS8Uint},

		// BC compressed formats
		{"BC1RGBAUnorm", gputypes.TextureFormatBC1RGBAUnorm, vk.FormatBc1RgbaUnormBlock},
		{"BC1RGBAUnormSrgb", gputypes.TextureFormatBC1RGBAUnormSrgb, vk.FormatBc1RgbaSrgbBlock},
		{"BC7RGBAUnorm", gputypes.TextureFormatBC7RGBAUnorm, vk.FormatBc7UnormBlock},

		// ETC2 compressed formats
		{"ETC2RGB8Unorm", gputypes.TextureFormatETC2RGB8Unorm, vk.FormatEtc2R8g8b8UnormBlock},
		{"ETC2RGBA8Unorm", gputypes.TextureFormatETC2RGBA8Unorm, vk.FormatEtc2R8g8b8a8UnormBlock},

		// ASTC compressed formats
		{"ASTC4x4Unorm", gputypes.TextureFormatASTC4x4Unorm, vk.FormatAstc4x4UnormBlock},
		{"ASTC12x12UnormSrgb", gputypes.TextureFormatASTC12x12UnormSrgb, vk.FormatAstc12x12SrgbBlock},

		// Unknown format
		{"Unknown", gputypes.TextureFormat(65535), vk.FormatUndefined},
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
		mode   gputypes.AddressMode
		expect vk.SamplerAddressMode
	}{
		{"ClampToEdge", gputypes.AddressModeClampToEdge, vk.SamplerAddressModeClampToEdge},
		{"Repeat", gputypes.AddressModeRepeat, vk.SamplerAddressModeRepeat},
		{"MirrorRepeat", gputypes.AddressModeMirrorRepeat, vk.SamplerAddressModeMirroredRepeat},
		{"Unknown defaults to ClampToEdge", gputypes.AddressMode(99), vk.SamplerAddressModeClampToEdge},
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
		mode   gputypes.FilterMode
		expect vk.Filter
	}{
		{"Nearest", gputypes.FilterModeNearest, vk.FilterNearest},
		{"Linear", gputypes.FilterModeLinear, vk.FilterLinear},
		{"Unknown defaults to Nearest", gputypes.FilterMode(99), vk.FilterNearest},
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
		mode   gputypes.FilterMode
		expect vk.SamplerMipmapMode
	}{
		{"Nearest", gputypes.FilterModeNearest, vk.SamplerMipmapModeNearest},
		{"Linear", gputypes.FilterModeLinear, vk.SamplerMipmapModeLinear},
		{"Unknown defaults to Nearest", gputypes.FilterMode(99), vk.SamplerMipmapModeNearest},
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
		fn     gputypes.CompareFunction
		expect vk.CompareOp
	}{
		{"Never", gputypes.CompareFunctionNever, vk.CompareOpNever},
		{"Less", gputypes.CompareFunctionLess, vk.CompareOpLess},
		{"Equal", gputypes.CompareFunctionEqual, vk.CompareOpEqual},
		{"LessEqual", gputypes.CompareFunctionLessEqual, vk.CompareOpLessOrEqual},
		{"Greater", gputypes.CompareFunctionGreater, vk.CompareOpGreater},
		{"NotEqual", gputypes.CompareFunctionNotEqual, vk.CompareOpNotEqual},
		{"GreaterEqual", gputypes.CompareFunctionGreaterEqual, vk.CompareOpGreaterOrEqual},
		{"Always", gputypes.CompareFunctionAlways, vk.CompareOpAlways},
		{"Unknown defaults to Never", gputypes.CompareFunction(99), vk.CompareOpNever},
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
		stages gputypes.ShaderStages
		expect vk.ShaderStageFlags
	}{
		{"Vertex", gputypes.ShaderStageVertex, vk.ShaderStageFlags(vk.ShaderStageVertexBit)},
		{"Fragment", gputypes.ShaderStageFragment, vk.ShaderStageFlags(vk.ShaderStageFragmentBit)},
		{"Compute", gputypes.ShaderStageCompute, vk.ShaderStageFlags(vk.ShaderStageComputeBit)},
		{
			"Vertex and Fragment",
			gputypes.ShaderStageVertex | gputypes.ShaderStageFragment,
			vk.ShaderStageFlags(vk.ShaderStageVertexBit) | vk.ShaderStageFlags(vk.ShaderStageFragmentBit),
		},
		{
			"All stages",
			gputypes.ShaderStageVertex | gputypes.ShaderStageFragment | gputypes.ShaderStageCompute,
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
		bindingType gputypes.BufferBindingType
		expect      vk.DescriptorType
	}{
		{"Uniform", gputypes.BufferBindingTypeUniform, vk.DescriptorTypeUniformBuffer},
		{"Storage", gputypes.BufferBindingTypeStorage, vk.DescriptorTypeStorageBuffer},
		{"ReadOnlyStorage", gputypes.BufferBindingTypeReadOnlyStorage, vk.DescriptorTypeStorageBuffer},
		{"Unknown defaults to Uniform", gputypes.BufferBindingType(99), vk.DescriptorTypeUniformBuffer},
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
		mode   gputypes.VertexStepMode
		expect vk.VertexInputRate
	}{
		{"Vertex", gputypes.VertexStepModeVertex, vk.VertexInputRateVertex},
		{"Instance", gputypes.VertexStepModeInstance, vk.VertexInputRateInstance},
		{"Unknown defaults to Vertex", gputypes.VertexStepMode(99), vk.VertexInputRateVertex},
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
		format gputypes.VertexFormat
		expect vk.Format
	}{
		// 8-bit formats
		{"Uint8x2", gputypes.VertexFormatUint8x2, vk.FormatR8g8Uint},
		{"Uint8x4", gputypes.VertexFormatUint8x4, vk.FormatR8g8b8a8Uint},
		{"Sint8x2", gputypes.VertexFormatSint8x2, vk.FormatR8g8Sint},
		{"Unorm8x4", gputypes.VertexFormatUnorm8x4, vk.FormatR8g8b8a8Unorm},

		// 16-bit formats
		{"Uint16x2", gputypes.VertexFormatUint16x2, vk.FormatR16g16Uint},
		{"Float16x4", gputypes.VertexFormatFloat16x4, vk.FormatR16g16b16a16Sfloat},

		// 32-bit formats
		{"Float32", gputypes.VertexFormatFloat32, vk.FormatR32Sfloat},
		{"Float32x2", gputypes.VertexFormatFloat32x2, vk.FormatR32g32Sfloat},
		{"Float32x3", gputypes.VertexFormatFloat32x3, vk.FormatR32g32b32Sfloat},
		{"Float32x4", gputypes.VertexFormatFloat32x4, vk.FormatR32g32b32a32Sfloat},
		{"Uint32", gputypes.VertexFormatUint32, vk.FormatR32Uint},
		{"Sint32x4", gputypes.VertexFormatSint32x4, vk.FormatR32g32b32a32Sint},

		// Packed formats
		{"Unorm1010102", gputypes.VertexFormatUnorm1010102, vk.FormatA2b10g10r10UnormPack32},

		// Unknown format defaults to Float32x4
		{"Unknown", gputypes.VertexFormat(255), vk.FormatR32g32b32a32Sfloat},
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
		topology gputypes.PrimitiveTopology
		expect   vk.PrimitiveTopology
	}{
		{"PointList", gputypes.PrimitiveTopologyPointList, vk.PrimitiveTopologyPointList},
		{"LineList", gputypes.PrimitiveTopologyLineList, vk.PrimitiveTopologyLineList},
		{"LineStrip", gputypes.PrimitiveTopologyLineStrip, vk.PrimitiveTopologyLineStrip},
		{"TriangleList", gputypes.PrimitiveTopologyTriangleList, vk.PrimitiveTopologyTriangleList},
		{"TriangleStrip", gputypes.PrimitiveTopologyTriangleStrip, vk.PrimitiveTopologyTriangleStrip},
		{"Unknown defaults to TriangleList", gputypes.PrimitiveTopology(99), vk.PrimitiveTopologyTriangleList},
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
		mode   gputypes.CullMode
		expect vk.CullModeFlags
	}{
		{"None", gputypes.CullModeNone, vk.CullModeFlags(vk.CullModeNone)},
		{"Front", gputypes.CullModeFront, vk.CullModeFlags(vk.CullModeFrontBit)},
		{"Back", gputypes.CullModeBack, vk.CullModeFlags(vk.CullModeBackBit)},
		{"Unknown defaults to None", gputypes.CullMode(99), vk.CullModeFlags(vk.CullModeNone)},
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
		face   gputypes.FrontFace
		expect vk.FrontFace
	}{
		{"CCW", gputypes.FrontFaceCCW, vk.FrontFaceCounterClockwise},
		{"CW", gputypes.FrontFaceCW, vk.FrontFaceClockwise},
		{"Unknown defaults to CCW", gputypes.FrontFace(99), vk.FrontFaceCounterClockwise},
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
		mask   gputypes.ColorWriteMask
		expect vk.ColorComponentFlags
	}{
		{"Red", gputypes.ColorWriteMaskRed, vk.ColorComponentFlags(vk.ColorComponentRBit)},
		{"Green", gputypes.ColorWriteMaskGreen, vk.ColorComponentFlags(vk.ColorComponentGBit)},
		{"Blue", gputypes.ColorWriteMaskBlue, vk.ColorComponentFlags(vk.ColorComponentBBit)},
		{"Alpha", gputypes.ColorWriteMaskAlpha, vk.ColorComponentFlags(vk.ColorComponentABit)},
		{
			"All",
			gputypes.ColorWriteMaskRed | gputypes.ColorWriteMaskGreen | gputypes.ColorWriteMaskBlue | gputypes.ColorWriteMaskAlpha,
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
		factor gputypes.BlendFactor
		expect vk.BlendFactor
	}{
		{"Zero", gputypes.BlendFactorZero, vk.BlendFactorZero},
		{"One", gputypes.BlendFactorOne, vk.BlendFactorOne},
		{"Src", gputypes.BlendFactorSrc, vk.BlendFactorSrcColor},
		{"OneMinusSrc", gputypes.BlendFactorOneMinusSrc, vk.BlendFactorOneMinusSrcColor},
		{"SrcAlpha", gputypes.BlendFactorSrcAlpha, vk.BlendFactorSrcAlpha},
		{"OneMinusSrcAlpha", gputypes.BlendFactorOneMinusSrcAlpha, vk.BlendFactorOneMinusSrcAlpha},
		{"Dst", gputypes.BlendFactorDst, vk.BlendFactorDstColor},
		{"OneMinusDst", gputypes.BlendFactorOneMinusDst, vk.BlendFactorOneMinusDstColor},
		{"DstAlpha", gputypes.BlendFactorDstAlpha, vk.BlendFactorDstAlpha},
		{"OneMinusDstAlpha", gputypes.BlendFactorOneMinusDstAlpha, vk.BlendFactorOneMinusDstAlpha},
		{"SrcAlphaSaturated", gputypes.BlendFactorSrcAlphaSaturated, vk.BlendFactorSrcAlphaSaturate},
		{"Constant", gputypes.BlendFactorConstant, vk.BlendFactorConstantColor},
		{"OneMinusConstant", gputypes.BlendFactorOneMinusConstant, vk.BlendFactorOneMinusConstantColor},
		{"Unknown defaults to One", gputypes.BlendFactor(99), vk.BlendFactorOne},
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
		op     gputypes.BlendOperation
		expect vk.BlendOp
	}{
		{"Add", gputypes.BlendOperationAdd, vk.BlendOpAdd},
		{"Subtract", gputypes.BlendOperationSubtract, vk.BlendOpSubtract},
		{"ReverseSubtract", gputypes.BlendOperationReverseSubtract, vk.BlendOpReverseSubtract},
		{"Min", gputypes.BlendOperationMin, vk.BlendOpMin},
		{"Max", gputypes.BlendOperationMax, vk.BlendOpMax},
		{"Unknown defaults to Add", gputypes.BlendOperation(99), vk.BlendOpAdd},
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
		Compare:     gputypes.CompareFunctionLess,
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
		dim    gputypes.TextureViewDimension
		expect vk.ImageViewType
	}{
		{"1D", gputypes.TextureViewDimension1D, vk.ImageViewType1d},
		{"2D", gputypes.TextureViewDimension2D, vk.ImageViewType2d},
		{"2DArray", gputypes.TextureViewDimension2DArray, vk.ImageViewType2dArray},
		{"Cube", gputypes.TextureViewDimensionCube, vk.ImageViewTypeCube},
		{"CubeArray", gputypes.TextureViewDimensionCubeArray, vk.ImageViewTypeCubeArray},
		{"3D", gputypes.TextureViewDimension3D, vk.ImageViewType3d},
		{"Unknown defaults to 2D", gputypes.TextureViewDimension(99), vk.ImageViewType2d},
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
		aspect gputypes.TextureAspect
		format gputypes.TextureFormat
		expect vk.ImageAspectFlags
	}{
		{"DepthOnly", gputypes.TextureAspectDepthOnly, gputypes.TextureFormatDepth32Float, vk.ImageAspectFlags(vk.ImageAspectDepthBit)},
		{"StencilOnly", gputypes.TextureAspectStencilOnly, gputypes.TextureFormatStencil8, vk.ImageAspectFlags(vk.ImageAspectStencilBit)},
		{"All color", gputypes.TextureAspectAll, gputypes.TextureFormatRGBA8Unorm, vk.ImageAspectFlags(vk.ImageAspectColorBit)},
		{
			"All depth-stencil",
			gputypes.TextureAspectAll,
			gputypes.TextureFormatDepth24PlusStencil8,
			vk.ImageAspectFlags(vk.ImageAspectDepthBit) | vk.ImageAspectFlags(vk.ImageAspectStencilBit),
		},
		{"All depth only", gputypes.TextureAspectAll, gputypes.TextureFormatDepth32Float, vk.ImageAspectFlags(vk.ImageAspectDepthBit)},
		{"Unknown defaults to Color", gputypes.TextureAspect(99), gputypes.TextureFormatRGBA8Unorm, vk.ImageAspectFlags(vk.ImageAspectColorBit)},
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
		aspect gputypes.TextureAspect
		expect vk.ImageAspectFlags
	}{
		{"DepthOnly", gputypes.TextureAspectDepthOnly, vk.ImageAspectFlags(vk.ImageAspectDepthBit)},
		{"StencilOnly", gputypes.TextureAspectStencilOnly, vk.ImageAspectFlags(vk.ImageAspectStencilBit)},
		{"All defaults to Color", gputypes.TextureAspectAll, vk.ImageAspectFlags(vk.ImageAspectColorBit)},
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
		format gputypes.TextureFormat
		expect bool
	}{
		{"Depth24PlusStencil8", gputypes.TextureFormatDepth24PlusStencil8, true},
		{"Depth32FloatStencil8", gputypes.TextureFormatDepth32FloatStencil8, true},
		{"Stencil8", gputypes.TextureFormatStencil8, true},
		{"Depth16Unorm", gputypes.TextureFormatDepth16Unorm, false},
		{"Depth32Float", gputypes.TextureFormatDepth32Float, false},
		{"RGBA8Unorm", gputypes.TextureFormatRGBA8Unorm, false},
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
		dim    gputypes.TextureDimension
		expect vk.ImageViewType
	}{
		{"1D", gputypes.TextureDimension1D, vk.ImageViewType1d},
		{"2D", gputypes.TextureDimension2D, vk.ImageViewType2d},
		{"3D", gputypes.TextureDimension3D, vk.ImageViewType3d},
		{"Unknown defaults to 2D", gputypes.TextureDimension(99), vk.ImageViewType2d},
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
