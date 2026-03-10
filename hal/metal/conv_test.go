// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build darwin

package metal

import (
	"testing"

	"github.com/gogpu/gputypes"
)

// TestTextureFormatToMTL tests texture format conversions to Metal pixel formats.
func TestTextureFormatToMTL(t *testing.T) {
	tests := []struct {
		name   string
		format gputypes.TextureFormat
		expect MTLPixelFormat
	}{
		// 8-bit formats
		{"R8Unorm", gputypes.TextureFormatR8Unorm, MTLPixelFormatR8Unorm},
		{"R8Snorm", gputypes.TextureFormatR8Snorm, MTLPixelFormatR8Snorm},
		{"R8Uint", gputypes.TextureFormatR8Uint, MTLPixelFormatR8Uint},
		{"R8Sint", gputypes.TextureFormatR8Sint, MTLPixelFormatR8Sint},

		// 16-bit formats
		{"R16Uint", gputypes.TextureFormatR16Uint, MTLPixelFormatR16Uint},
		{"R16Sint", gputypes.TextureFormatR16Sint, MTLPixelFormatR16Sint},
		{"R16Float", gputypes.TextureFormatR16Float, MTLPixelFormatR16Float},
		{"RG8Unorm", gputypes.TextureFormatRG8Unorm, MTLPixelFormatRG8Unorm},
		{"RG8Snorm", gputypes.TextureFormatRG8Snorm, MTLPixelFormatRG8Snorm},
		{"RG8Uint", gputypes.TextureFormatRG8Uint, MTLPixelFormatRG8Uint},
		{"RG8Sint", gputypes.TextureFormatRG8Sint, MTLPixelFormatRG8Sint},

		// 32-bit formats
		{"R32Uint", gputypes.TextureFormatR32Uint, MTLPixelFormatR32Uint},
		{"R32Sint", gputypes.TextureFormatR32Sint, MTLPixelFormatR32Sint},
		{"R32Float", gputypes.TextureFormatR32Float, MTLPixelFormatR32Float},
		{"RG16Uint", gputypes.TextureFormatRG16Uint, MTLPixelFormatRG16Uint},
		{"RG16Sint", gputypes.TextureFormatRG16Sint, MTLPixelFormatRG16Sint},
		{"RG16Float", gputypes.TextureFormatRG16Float, MTLPixelFormatRG16Float},
		{"RGBA8Unorm", gputypes.TextureFormatRGBA8Unorm, MTLPixelFormatRGBA8Unorm},
		{"RGBA8UnormSrgb", gputypes.TextureFormatRGBA8UnormSrgb, MTLPixelFormatRGBA8UnormSRGB},
		{"RGBA8Snorm", gputypes.TextureFormatRGBA8Snorm, MTLPixelFormatRGBA8Snorm},
		{"RGBA8Uint", gputypes.TextureFormatRGBA8Uint, MTLPixelFormatRGBA8Uint},
		{"RGBA8Sint", gputypes.TextureFormatRGBA8Sint, MTLPixelFormatRGBA8Sint},
		{"BGRA8Unorm", gputypes.TextureFormatBGRA8Unorm, MTLPixelFormatBGRA8Unorm},
		{"BGRA8UnormSrgb", gputypes.TextureFormatBGRA8UnormSrgb, MTLPixelFormatBGRA8UnormSRGB},

		// Packed formats
		{"RGB10A2Unorm", gputypes.TextureFormatRGB10A2Unorm, MTLPixelFormatRGB10A2Unorm},
		{"RG11B10Ufloat", gputypes.TextureFormatRG11B10Ufloat, MTLPixelFormatRG11B10Float},
		{"RGB9E5Ufloat", gputypes.TextureFormatRGB9E5Ufloat, MTLPixelFormatRGB9E5Float},

		// 64-bit formats
		{"RG32Uint", gputypes.TextureFormatRG32Uint, MTLPixelFormatRG32Uint},
		{"RG32Sint", gputypes.TextureFormatRG32Sint, MTLPixelFormatRG32Sint},
		{"RG32Float", gputypes.TextureFormatRG32Float, MTLPixelFormatRG32Float},
		{"RGBA16Uint", gputypes.TextureFormatRGBA16Uint, MTLPixelFormatRGBA16Uint},
		{"RGBA16Sint", gputypes.TextureFormatRGBA16Sint, MTLPixelFormatRGBA16Sint},
		{"RGBA16Float", gputypes.TextureFormatRGBA16Float, MTLPixelFormatRGBA16Float},

		// 128-bit formats
		{"RGBA32Uint", gputypes.TextureFormatRGBA32Uint, MTLPixelFormatRGBA32Uint},
		{"RGBA32Sint", gputypes.TextureFormatRGBA32Sint, MTLPixelFormatRGBA32Sint},
		{"RGBA32Float", gputypes.TextureFormatRGBA32Float, MTLPixelFormatRGBA32Float},

		// Depth/stencil formats
		{"Depth16Unorm", gputypes.TextureFormatDepth16Unorm, MTLPixelFormatDepth16Unorm},
		{"Depth32Float", gputypes.TextureFormatDepth32Float, MTLPixelFormatDepth32Float},
		{"Depth24Plus", gputypes.TextureFormatDepth24Plus, MTLPixelFormatDepth32Float},
		{"Depth24PlusStencil8", gputypes.TextureFormatDepth24PlusStencil8, MTLPixelFormatDepth32FloatStencil8},
		{"Depth32FloatStencil8", gputypes.TextureFormatDepth32FloatStencil8, MTLPixelFormatDepth32FloatStencil8},
		{"Stencil8", gputypes.TextureFormatStencil8, MTLPixelFormatStencil8},

		// Unknown format
		{"Unknown", gputypes.TextureFormat(65535), MTLPixelFormatInvalid},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := textureFormatToMTL(tt.format)
			if got != tt.expect {
				t.Errorf("textureFormatToMTL(%v) = %v, want %v", tt.format, got, tt.expect)
			}
		})
	}
}

// TestTextureUsageToMTL tests texture usage flag conversions.
func TestTextureUsageToMTL(t *testing.T) {
	tests := []struct {
		name   string
		usage  gputypes.TextureUsage
		expect MTLTextureUsage
	}{
		{"CopySrc", gputypes.TextureUsageCopySrc, MTLTextureUsageShaderRead},
		{"CopyDst", gputypes.TextureUsageCopyDst, MTLTextureUsageShaderRead},
		{"TextureBinding", gputypes.TextureUsageTextureBinding, MTLTextureUsageShaderRead},
		{"StorageBinding", gputypes.TextureUsageStorageBinding, MTLTextureUsageShaderRead | MTLTextureUsageShaderWrite},
		{"RenderAttachment", gputypes.TextureUsageRenderAttachment, MTLTextureUsageRenderTarget},
		{
			"CopySrc and TextureBinding",
			gputypes.TextureUsageCopySrc | gputypes.TextureUsageTextureBinding,
			MTLTextureUsageShaderRead,
		},
		{
			"All",
			gputypes.TextureUsageCopySrc | gputypes.TextureUsageCopyDst | gputypes.TextureUsageTextureBinding | gputypes.TextureUsageStorageBinding | gputypes.TextureUsageRenderAttachment,
			MTLTextureUsageShaderRead | MTLTextureUsageShaderWrite | MTLTextureUsageRenderTarget,
		},
		{"None defaults to Unknown", gputypes.TextureUsage(0), MTLTextureUsageUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := textureUsageToMTL(tt.usage)
			if got != tt.expect {
				t.Errorf("textureUsageToMTL(%v) = %v, want %v", tt.usage, got, tt.expect)
			}
		})
	}
}

// TestTextureTypeFromDimension tests texture type from dimension conversions.
func TestTextureTypeFromDimension(t *testing.T) {
	tests := []struct {
		name        string
		dimension   gputypes.TextureDimension
		sampleCount uint32
		depth       uint32
		expect      MTLTextureType
	}{
		{"1D", gputypes.TextureDimension1D, 1, 1, MTLTextureType1D},
		{"1DArray", gputypes.TextureDimension1D, 1, 3, MTLTextureType1DArray},
		{"2D", gputypes.TextureDimension2D, 1, 1, MTLTextureType2D},
		{"2DArray", gputypes.TextureDimension2D, 1, 3, MTLTextureType2DArray},
		{"2DMultisample", gputypes.TextureDimension2D, 4, 1, MTLTextureType2DMultisample},
		{"3D", gputypes.TextureDimension3D, 1, 1, MTLTextureType3D},
		{"Unknown defaults to 2D", gputypes.TextureDimension(99), 1, 1, MTLTextureType2D},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := textureTypeFromDimension(tt.dimension, tt.sampleCount, tt.depth)
			if got != tt.expect {
				t.Errorf("textureTypeFromDimension(%v, %d, %d) = %v, want %v",
					tt.dimension, tt.sampleCount, tt.depth, got, tt.expect)
			}
		})
	}
}

// TestTextureViewDimensionToMTL tests texture view dimension conversions.
func TestTextureViewDimensionToMTL(t *testing.T) {
	tests := []struct {
		name      string
		dimension gputypes.TextureViewDimension
		expect    MTLTextureType
	}{
		{"1D", gputypes.TextureViewDimension1D, MTLTextureType1D},
		{"2D", gputypes.TextureViewDimension2D, MTLTextureType2D},
		{"2DArray", gputypes.TextureViewDimension2DArray, MTLTextureType2DArray},
		{"Cube", gputypes.TextureViewDimensionCube, MTLTextureTypeCube},
		{"CubeArray", gputypes.TextureViewDimensionCubeArray, MTLTextureTypeCubeArray},
		{"3D", gputypes.TextureViewDimension3D, MTLTextureType3D},
		{"Unknown defaults to 2D", gputypes.TextureViewDimension(99), MTLTextureType2D},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := textureViewDimensionToMTL(tt.dimension)
			if got != tt.expect {
				t.Errorf("textureViewDimensionToMTL(%v) = %v, want %v", tt.dimension, got, tt.expect)
			}
		})
	}
}

// TestFilterModeToMTL tests filter mode conversions.
func TestFilterModeToMTL(t *testing.T) {
	tests := []struct {
		name   string
		mode   gputypes.FilterMode
		expect MTLSamplerMinMagFilter
	}{
		{"Nearest", gputypes.FilterModeNearest, MTLSamplerMinMagFilterNearest},
		{"Linear", gputypes.FilterModeLinear, MTLSamplerMinMagFilterLinear},
		{"Unknown defaults to Nearest", gputypes.FilterMode(99), MTLSamplerMinMagFilterNearest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filterModeToMTL(tt.mode)
			if got != tt.expect {
				t.Errorf("filterModeToMTL(%v) = %v, want %v", tt.mode, got, tt.expect)
			}
		})
	}
}

// TestMipmapFilterModeToMTL tests mipmap filter mode conversions.
func TestMipmapFilterModeToMTL(t *testing.T) {
	tests := []struct {
		name   string
		mode   gputypes.FilterMode
		expect MTLSamplerMipFilter
	}{
		{"Nearest", gputypes.FilterModeNearest, MTLSamplerMipFilterNearest},
		{"Linear", gputypes.FilterModeLinear, MTLSamplerMipFilterLinear},
		{"Unknown defaults to NotMipmapped", gputypes.FilterMode(99), MTLSamplerMipFilterNotMipmapped},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mipmapFilterModeToMTL(tt.mode)
			if got != tt.expect {
				t.Errorf("mipmapFilterModeToMTL(%v) = %v, want %v", tt.mode, got, tt.expect)
			}
		})
	}
}

// TestAddressModeToMTL tests address mode conversions.
func TestAddressModeToMTL(t *testing.T) {
	tests := []struct {
		name   string
		mode   gputypes.AddressMode
		expect MTLSamplerAddressMode
	}{
		{"ClampToEdge", gputypes.AddressModeClampToEdge, MTLSamplerAddressModeClampToEdge},
		{"Repeat", gputypes.AddressModeRepeat, MTLSamplerAddressModeRepeat},
		{"MirrorRepeat", gputypes.AddressModeMirrorRepeat, MTLSamplerAddressModeMirrorRepeat},
		{"Unknown defaults to ClampToEdge", gputypes.AddressMode(99), MTLSamplerAddressModeClampToEdge},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := addressModeToMTL(tt.mode)
			if got != tt.expect {
				t.Errorf("addressModeToMTL(%v) = %v, want %v", tt.mode, got, tt.expect)
			}
		})
	}
}

// TestCompareFunctionToMTL tests compare function conversions.
func TestCompareFunctionToMTL(t *testing.T) {
	tests := []struct {
		name   string
		fn     gputypes.CompareFunction
		expect MTLCompareFunction
	}{
		{"Never", gputypes.CompareFunctionNever, MTLCompareFunctionNever},
		{"Less", gputypes.CompareFunctionLess, MTLCompareFunctionLess},
		{"Equal", gputypes.CompareFunctionEqual, MTLCompareFunctionEqual},
		{"LessEqual", gputypes.CompareFunctionLessEqual, MTLCompareFunctionLessEqual},
		{"Greater", gputypes.CompareFunctionGreater, MTLCompareFunctionGreater},
		{"NotEqual", gputypes.CompareFunctionNotEqual, MTLCompareFunctionNotEqual},
		{"GreaterEqual", gputypes.CompareFunctionGreaterEqual, MTLCompareFunctionGreaterEqual},
		{"Always", gputypes.CompareFunctionAlways, MTLCompareFunctionAlways},
		{"Unknown defaults to Always", gputypes.CompareFunction(99), MTLCompareFunctionAlways},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := compareFunctionToMTL(tt.fn)
			if got != tt.expect {
				t.Errorf("compareFunctionToMTL(%v) = %v, want %v", tt.fn, got, tt.expect)
			}
		})
	}
}

// TestPrimitiveTopologyToMTL tests primitive topology conversions.
func TestPrimitiveTopologyToMTL(t *testing.T) {
	tests := []struct {
		name     string
		topology gputypes.PrimitiveTopology
		expect   MTLPrimitiveType
	}{
		{"PointList", gputypes.PrimitiveTopologyPointList, MTLPrimitiveTypePoint},
		{"LineList", gputypes.PrimitiveTopologyLineList, MTLPrimitiveTypeLine},
		{"LineStrip", gputypes.PrimitiveTopologyLineStrip, MTLPrimitiveTypeLineStrip},
		{"TriangleList", gputypes.PrimitiveTopologyTriangleList, MTLPrimitiveTypeTriangle},
		{"TriangleStrip", gputypes.PrimitiveTopologyTriangleStrip, MTLPrimitiveTypeTriangleStrip},
		{"Unknown defaults to Triangle", gputypes.PrimitiveTopology(99), MTLPrimitiveTypeTriangle},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := primitiveTopologyToMTL(tt.topology)
			if got != tt.expect {
				t.Errorf("primitiveTopologyToMTL(%v) = %v, want %v", tt.topology, got, tt.expect)
			}
		})
	}
}

// TestBlendFactorToMTL tests blend factor conversions.
func TestBlendFactorToMTL(t *testing.T) {
	tests := []struct {
		name   string
		factor gputypes.BlendFactor
		expect MTLBlendFactor
	}{
		{"Zero", gputypes.BlendFactorZero, MTLBlendFactorZero},
		{"One", gputypes.BlendFactorOne, MTLBlendFactorOne},
		{"Src", gputypes.BlendFactorSrc, MTLBlendFactorSourceColor},
		{"OneMinusSrc", gputypes.BlendFactorOneMinusSrc, MTLBlendFactorOneMinusSourceColor},
		{"SrcAlpha", gputypes.BlendFactorSrcAlpha, MTLBlendFactorSourceAlpha},
		{"OneMinusSrcAlpha", gputypes.BlendFactorOneMinusSrcAlpha, MTLBlendFactorOneMinusSourceAlpha},
		{"Dst", gputypes.BlendFactorDst, MTLBlendFactorDestinationColor},
		{"OneMinusDst", gputypes.BlendFactorOneMinusDst, MTLBlendFactorOneMinusDestinationColor},
		{"DstAlpha", gputypes.BlendFactorDstAlpha, MTLBlendFactorDestinationAlpha},
		{"OneMinusDstAlpha", gputypes.BlendFactorOneMinusDstAlpha, MTLBlendFactorOneMinusDestinationAlpha},
		{"SrcAlphaSaturated", gputypes.BlendFactorSrcAlphaSaturated, MTLBlendFactorSourceAlphaSaturated},
		{"Constant", gputypes.BlendFactorConstant, MTLBlendFactorBlendColor},
		{"OneMinusConstant", gputypes.BlendFactorOneMinusConstant, MTLBlendFactorOneMinusBlendColor},
		{"Unknown defaults to One", gputypes.BlendFactor(99), MTLBlendFactorOne},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := blendFactorToMTL(tt.factor)
			if got != tt.expect {
				t.Errorf("blendFactorToMTL(%v) = %v, want %v", tt.factor, got, tt.expect)
			}
		})
	}
}

// TestBlendOperationToMTL tests blend operation conversions.
func TestBlendOperationToMTL(t *testing.T) {
	tests := []struct {
		name   string
		op     gputypes.BlendOperation
		expect MTLBlendOperation
	}{
		{"Add", gputypes.BlendOperationAdd, MTLBlendOperationAdd},
		{"Subtract", gputypes.BlendOperationSubtract, MTLBlendOperationSubtract},
		{"ReverseSubtract", gputypes.BlendOperationReverseSubtract, MTLBlendOperationReverseSubtract},
		{"Min", gputypes.BlendOperationMin, MTLBlendOperationMin},
		{"Max", gputypes.BlendOperationMax, MTLBlendOperationMax},
		{"Unknown defaults to Add", gputypes.BlendOperation(99), MTLBlendOperationAdd},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := blendOperationToMTL(tt.op)
			if got != tt.expect {
				t.Errorf("blendOperationToMTL(%v) = %v, want %v", tt.op, got, tt.expect)
			}
		})
	}
}

// TestLoadOpToMTL tests load operation conversions.
func TestLoadOpToMTL(t *testing.T) {
	tests := []struct {
		name   string
		op     gputypes.LoadOp
		expect MTLLoadAction
	}{
		{"Clear", gputypes.LoadOpClear, MTLLoadActionClear},
		{"Load", gputypes.LoadOpLoad, MTLLoadActionLoad},
		{"Unknown defaults to DontCare", gputypes.LoadOp(99), MTLLoadActionDontCare},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := loadOpToMTL(tt.op)
			if got != tt.expect {
				t.Errorf("loadOpToMTL(%v) = %v, want %v", tt.op, got, tt.expect)
			}
		})
	}
}

// TestStoreOpToMTL tests store operation conversions.
func TestStoreOpToMTL(t *testing.T) {
	tests := []struct {
		name   string
		op     gputypes.StoreOp
		expect MTLStoreAction
	}{
		{"Store", gputypes.StoreOpStore, MTLStoreActionStore},
		{"Discard", gputypes.StoreOpDiscard, MTLStoreActionDontCare},
		{"Unknown defaults to Store", gputypes.StoreOp(99), MTLStoreActionStore},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := storeOpToMTL(tt.op)
			if got != tt.expect {
				t.Errorf("storeOpToMTL(%v) = %v, want %v", tt.op, got, tt.expect)
			}
		})
	}
}

// TestCullModeToMTL tests cull mode conversions.
func TestCullModeToMTL(t *testing.T) {
	tests := []struct {
		name   string
		mode   gputypes.CullMode
		expect MTLCullMode
	}{
		{"None", gputypes.CullModeNone, MTLCullModeNone},
		{"Front", gputypes.CullModeFront, MTLCullModeFront},
		{"Back", gputypes.CullModeBack, MTLCullModeBack},
		{"Unknown defaults to None", gputypes.CullMode(99), MTLCullModeNone},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cullModeToMTL(tt.mode)
			if got != tt.expect {
				t.Errorf("cullModeToMTL(%v) = %v, want %v", tt.mode, got, tt.expect)
			}
		})
	}
}

// TestFrontFaceToMTL tests front face conversions.
func TestFrontFaceToMTL(t *testing.T) {
	tests := []struct {
		name   string
		face   gputypes.FrontFace
		expect MTLWinding
	}{
		{"CCW", gputypes.FrontFaceCCW, MTLWindingCounterClockwise},
		{"CW", gputypes.FrontFaceCW, MTLWindingClockwise},
		{"Unknown defaults to CCW", gputypes.FrontFace(99), MTLWindingCounterClockwise},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := frontFaceToMTL(tt.face)
			if got != tt.expect {
				t.Errorf("frontFaceToMTL(%v) = %v, want %v", tt.face, got, tt.expect)
			}
		})
	}
}

// TestIndexFormatToMTL tests index format conversions.
func TestIndexFormatToMTL(t *testing.T) {
	tests := []struct {
		name   string
		format gputypes.IndexFormat
		expect MTLIndexType
	}{
		{"Uint16", gputypes.IndexFormatUint16, MTLIndexTypeUInt16},
		{"Uint32", gputypes.IndexFormatUint32, MTLIndexTypeUInt32},
		{"Unknown defaults to Uint32", gputypes.IndexFormat(99), MTLIndexTypeUInt32},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := indexFormatToMTL(tt.format)
			if got != tt.expect {
				t.Errorf("indexFormatToMTL(%v) = %v, want %v", tt.format, got, tt.expect)
			}
		})
	}
}

// TestVertexFormatToMTL tests vertex format conversions.
func TestVertexFormatToMTL(t *testing.T) {
	tests := []struct {
		name   string
		format gputypes.VertexFormat
		expect MTLVertexFormat
	}{
		// 8-bit formats
		{"Uint8x2", gputypes.VertexFormatUint8x2, MTLVertexFormatUChar2},
		{"Uint8x4", gputypes.VertexFormatUint8x4, MTLVertexFormatUChar4},
		{"Sint8x2", gputypes.VertexFormatSint8x2, MTLVertexFormatChar2},
		{"Sint8x4", gputypes.VertexFormatSint8x4, MTLVertexFormatChar4},
		{"Unorm8x2", gputypes.VertexFormatUnorm8x2, MTLVertexFormatUChar2Normalized},
		{"Unorm8x4", gputypes.VertexFormatUnorm8x4, MTLVertexFormatUChar4Normalized},
		{"Snorm8x2", gputypes.VertexFormatSnorm8x2, MTLVertexFormatChar2Normalized},
		{"Snorm8x4", gputypes.VertexFormatSnorm8x4, MTLVertexFormatChar4Normalized},

		// 16-bit formats
		{"Uint16x2", gputypes.VertexFormatUint16x2, MTLVertexFormatUShort2},
		{"Uint16x4", gputypes.VertexFormatUint16x4, MTLVertexFormatUShort4},
		{"Sint16x2", gputypes.VertexFormatSint16x2, MTLVertexFormatShort2},
		{"Sint16x4", gputypes.VertexFormatSint16x4, MTLVertexFormatShort4},
		{"Unorm16x2", gputypes.VertexFormatUnorm16x2, MTLVertexFormatUShort2Normalized},
		{"Unorm16x4", gputypes.VertexFormatUnorm16x4, MTLVertexFormatUShort4Normalized},
		{"Snorm16x2", gputypes.VertexFormatSnorm16x2, MTLVertexFormatShort2Normalized},
		{"Snorm16x4", gputypes.VertexFormatSnorm16x4, MTLVertexFormatShort4Normalized},
		{"Float16x2", gputypes.VertexFormatFloat16x2, MTLVertexFormatHalf2},
		{"Float16x4", gputypes.VertexFormatFloat16x4, MTLVertexFormatHalf4},

		// 32-bit formats
		{"Float32", gputypes.VertexFormatFloat32, MTLVertexFormatFloat},
		{"Float32x2", gputypes.VertexFormatFloat32x2, MTLVertexFormatFloat2},
		{"Float32x3", gputypes.VertexFormatFloat32x3, MTLVertexFormatFloat3},
		{"Float32x4", gputypes.VertexFormatFloat32x4, MTLVertexFormatFloat4},
		{"Uint32", gputypes.VertexFormatUint32, MTLVertexFormatUInt},
		{"Uint32x2", gputypes.VertexFormatUint32x2, MTLVertexFormatUInt2},
		{"Uint32x3", gputypes.VertexFormatUint32x3, MTLVertexFormatUInt3},
		{"Uint32x4", gputypes.VertexFormatUint32x4, MTLVertexFormatUInt4},
		{"Sint32", gputypes.VertexFormatSint32, MTLVertexFormatInt},
		{"Sint32x2", gputypes.VertexFormatSint32x2, MTLVertexFormatInt2},
		{"Sint32x3", gputypes.VertexFormatSint32x3, MTLVertexFormatInt3},
		{"Sint32x4", gputypes.VertexFormatSint32x4, MTLVertexFormatInt4},

		// Packed formats
		{"Unorm1010102", gputypes.VertexFormatUnorm1010102, MTLVertexFormatUInt1010102Normalized},

		// Unknown format
		{"Unknown", gputypes.VertexFormat(255), MTLVertexFormatInvalid},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := vertexFormatToMTL(tt.format)
			if got != tt.expect {
				t.Errorf("vertexFormatToMTL(%v) = %v, want %v", tt.format, got, tt.expect)
			}
		})
	}
}

// TestVertexStepModeToMTL tests vertex step mode conversions.
func TestVertexStepModeToMTL(t *testing.T) {
	tests := []struct {
		name   string
		mode   gputypes.VertexStepMode
		expect MTLVertexStepFunction
	}{
		{"Vertex", gputypes.VertexStepModeVertex, MTLVertexStepFunctionPerVertex},
		{"Instance", gputypes.VertexStepModeInstance, MTLVertexStepFunctionPerInstance},
		{"Unknown defaults to PerVertex", gputypes.VertexStepMode(99), MTLVertexStepFunctionPerVertex},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := vertexStepModeToMTL(tt.mode)
			if got != tt.expect {
				t.Errorf("vertexStepModeToMTL(%v) = %v, want %v", tt.mode, got, tt.expect)
			}
		})
	}
}
