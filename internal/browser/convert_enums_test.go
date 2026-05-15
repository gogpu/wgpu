package browser

import (
	"testing"

	"github.com/gogpu/gputypes"
)

// TestTextureFormatToJS verifies every texture format maps to the correct WebGPU string.
func TestTextureFormatToJS(t *testing.T) {
	tests := []struct {
		format gputypes.TextureFormat
		want   string
	}{
		// 8-bit
		{gputypes.TextureFormatR8Unorm, "r8unorm"},
		{gputypes.TextureFormatR8Snorm, "r8snorm"},
		{gputypes.TextureFormatR8Uint, "r8uint"},
		{gputypes.TextureFormatR8Sint, "r8sint"},
		// 16-bit
		{gputypes.TextureFormatR16Uint, "r16uint"},
		{gputypes.TextureFormatR16Sint, "r16sint"},
		{gputypes.TextureFormatR16Float, "r16float"},
		{gputypes.TextureFormatRG8Unorm, "rg8unorm"},
		{gputypes.TextureFormatRG8Snorm, "rg8snorm"},
		{gputypes.TextureFormatRG8Uint, "rg8uint"},
		{gputypes.TextureFormatRG8Sint, "rg8sint"},
		// 32-bit
		{gputypes.TextureFormatR32Float, "r32float"},
		{gputypes.TextureFormatR32Uint, "r32uint"},
		{gputypes.TextureFormatR32Sint, "r32sint"},
		{gputypes.TextureFormatRG16Uint, "rg16uint"},
		{gputypes.TextureFormatRG16Sint, "rg16sint"},
		{gputypes.TextureFormatRG16Float, "rg16float"},
		{gputypes.TextureFormatRGBA8Unorm, "rgba8unorm"},
		{gputypes.TextureFormatRGBA8UnormSrgb, "rgba8unorm-srgb"},
		{gputypes.TextureFormatRGBA8Snorm, "rgba8snorm"},
		{gputypes.TextureFormatRGBA8Uint, "rgba8uint"},
		{gputypes.TextureFormatRGBA8Sint, "rgba8sint"},
		{gputypes.TextureFormatBGRA8Unorm, "bgra8unorm"},
		{gputypes.TextureFormatBGRA8UnormSrgb, "bgra8unorm-srgb"},
		// Packed 32-bit
		{gputypes.TextureFormatRGB10A2Uint, "rgb10a2uint"},
		{gputypes.TextureFormatRGB10A2Unorm, "rgb10a2unorm"},
		{gputypes.TextureFormatRG11B10Ufloat, "rg11b10ufloat"},
		{gputypes.TextureFormatRGB9E5Ufloat, "rgb9e5ufloat"},
		// 64-bit
		{gputypes.TextureFormatRG32Float, "rg32float"},
		{gputypes.TextureFormatRG32Uint, "rg32uint"},
		{gputypes.TextureFormatRG32Sint, "rg32sint"},
		{gputypes.TextureFormatRGBA16Uint, "rgba16uint"},
		{gputypes.TextureFormatRGBA16Sint, "rgba16sint"},
		{gputypes.TextureFormatRGBA16Float, "rgba16float"},
		// 128-bit
		{gputypes.TextureFormatRGBA32Float, "rgba32float"},
		{gputypes.TextureFormatRGBA32Uint, "rgba32uint"},
		{gputypes.TextureFormatRGBA32Sint, "rgba32sint"},
		// Depth/stencil
		{gputypes.TextureFormatStencil8, "stencil8"},
		{gputypes.TextureFormatDepth16Unorm, "depth16unorm"},
		{gputypes.TextureFormatDepth24Plus, "depth24plus"},
		{gputypes.TextureFormatDepth24PlusStencil8, "depth24plus-stencil8"},
		{gputypes.TextureFormatDepth32Float, "depth32float"},
		{gputypes.TextureFormatDepth32FloatStencil8, "depth32float-stencil8"},
		// BC compressed
		{gputypes.TextureFormatBC1RGBAUnorm, "bc1-rgba-unorm"},
		{gputypes.TextureFormatBC1RGBAUnormSrgb, "bc1-rgba-unorm-srgb"},
		{gputypes.TextureFormatBC2RGBAUnorm, "bc2-rgba-unorm"},
		{gputypes.TextureFormatBC2RGBAUnormSrgb, "bc2-rgba-unorm-srgb"},
		{gputypes.TextureFormatBC3RGBAUnorm, "bc3-rgba-unorm"},
		{gputypes.TextureFormatBC3RGBAUnormSrgb, "bc3-rgba-unorm-srgb"},
		{gputypes.TextureFormatBC4RUnorm, "bc4-r-unorm"},
		{gputypes.TextureFormatBC4RSnorm, "bc4-r-snorm"},
		{gputypes.TextureFormatBC5RGUnorm, "bc5-rg-unorm"},
		{gputypes.TextureFormatBC5RGSnorm, "bc5-rg-snorm"},
		{gputypes.TextureFormatBC6HRGBUfloat, "bc6h-rgb-ufloat"},
		{gputypes.TextureFormatBC6HRGBFloat, "bc6h-rgb-float"},
		{gputypes.TextureFormatBC7RGBAUnorm, "bc7-rgba-unorm"},
		{gputypes.TextureFormatBC7RGBAUnormSrgb, "bc7-rgba-unorm-srgb"},
		// ETC2 compressed
		{gputypes.TextureFormatETC2RGB8Unorm, "etc2-rgb8unorm"},
		{gputypes.TextureFormatETC2RGB8UnormSrgb, "etc2-rgb8unorm-srgb"},
		{gputypes.TextureFormatETC2RGB8A1Unorm, "etc2-rgb8a1unorm"},
		{gputypes.TextureFormatETC2RGB8A1UnormSrgb, "etc2-rgb8a1unorm-srgb"},
		{gputypes.TextureFormatETC2RGBA8Unorm, "etc2-rgba8unorm"},
		{gputypes.TextureFormatETC2RGBA8UnormSrgb, "etc2-rgba8unorm-srgb"},
		{gputypes.TextureFormatEACR11Unorm, "eac-r11unorm"},
		{gputypes.TextureFormatEACR11Snorm, "eac-r11snorm"},
		{gputypes.TextureFormatEACRG11Unorm, "eac-rg11unorm"},
		{gputypes.TextureFormatEACRG11Snorm, "eac-rg11snorm"},
		// ASTC compressed (spot check)
		{gputypes.TextureFormatASTC4x4Unorm, "astc-4x4-unorm"},
		{gputypes.TextureFormatASTC4x4UnormSrgb, "astc-4x4-unorm-srgb"},
		{gputypes.TextureFormatASTC12x12Unorm, "astc-12x12-unorm"},
		{gputypes.TextureFormatASTC12x12UnormSrgb, "astc-12x12-unorm-srgb"},
		// Undefined returns empty
		{gputypes.TextureFormatUndefined, ""},
	}
	for _, tc := range tests {
		got := TextureFormatToJS(tc.format)
		if got != tc.want {
			t.Errorf("TextureFormatToJS(%v) = %q, want %q", tc.format, got, tc.want)
		}
	}
}

// TestTextureDimensionToJS verifies all texture dimension mappings.
func TestTextureDimensionToJS(t *testing.T) {
	tests := []struct {
		dim  gputypes.TextureDimension
		want string
	}{
		{gputypes.TextureDimension1D, "1d"},
		{gputypes.TextureDimension2D, "2d"},
		{gputypes.TextureDimension3D, "3d"},
		{gputypes.TextureDimensionUndefined, ""},
	}
	for _, tc := range tests {
		got := TextureDimensionToJS(tc.dim)
		if got != tc.want {
			t.Errorf("TextureDimensionToJS(%v) = %q, want %q", tc.dim, got, tc.want)
		}
	}
}

// TestTextureViewDimensionToJS verifies all view dimension mappings.
func TestTextureViewDimensionToJS(t *testing.T) {
	tests := []struct {
		dim  gputypes.TextureViewDimension
		want string
	}{
		{gputypes.TextureViewDimension1D, "1d"},
		{gputypes.TextureViewDimension2D, "2d"},
		{gputypes.TextureViewDimension2DArray, "2d-array"},
		{gputypes.TextureViewDimensionCube, "cube"},
		{gputypes.TextureViewDimensionCubeArray, "cube-array"},
		{gputypes.TextureViewDimension3D, "3d"},
		{gputypes.TextureViewDimensionUndefined, ""},
	}
	for _, tc := range tests {
		got := TextureViewDimensionToJS(tc.dim)
		if got != tc.want {
			t.Errorf("TextureViewDimensionToJS(%v) = %q, want %q", tc.dim, got, tc.want)
		}
	}
}

// TestTextureAspectToJS verifies all texture aspect mappings.
func TestTextureAspectToJS(t *testing.T) {
	tests := []struct {
		aspect gputypes.TextureAspect
		want   string
	}{
		{gputypes.TextureAspectAll, "all"},
		{gputypes.TextureAspectStencilOnly, "stencil-only"},
		{gputypes.TextureAspectDepthOnly, "depth-only"},
		{gputypes.TextureAspectUndefined, ""},
	}
	for _, tc := range tests {
		got := TextureAspectToJS(tc.aspect)
		if got != tc.want {
			t.Errorf("TextureAspectToJS(%v) = %q, want %q", tc.aspect, got, tc.want)
		}
	}
}

// TestAddressModeToJS verifies all address mode mappings.
func TestAddressModeToJS(t *testing.T) {
	tests := []struct {
		mode gputypes.AddressMode
		want string
	}{
		{gputypes.AddressModeClampToEdge, "clamp-to-edge"},
		{gputypes.AddressModeRepeat, "repeat"},
		{gputypes.AddressModeMirrorRepeat, "mirror-repeat"},
		{gputypes.AddressModeUndefined, "clamp-to-edge"},
	}
	for _, tc := range tests {
		got := AddressModeToJS(tc.mode)
		if got != tc.want {
			t.Errorf("AddressModeToJS(%v) = %q, want %q", tc.mode, got, tc.want)
		}
	}
}

// TestFilterModeToJS verifies all filter mode mappings.
func TestFilterModeToJS(t *testing.T) {
	tests := []struct {
		mode gputypes.FilterMode
		want string
	}{
		{gputypes.FilterModeNearest, "nearest"},
		{gputypes.FilterModeLinear, "linear"},
		{gputypes.FilterModeUndefined, "nearest"},
	}
	for _, tc := range tests {
		got := FilterModeToJS(tc.mode)
		if got != tc.want {
			t.Errorf("FilterModeToJS(%v) = %q, want %q", tc.mode, got, tc.want)
		}
	}
}

// TestCompareFunctionToJS verifies all compare function mappings.
func TestCompareFunctionToJS(t *testing.T) {
	tests := []struct {
		fn   gputypes.CompareFunction
		want string
	}{
		{gputypes.CompareFunctionNever, "never"},
		{gputypes.CompareFunctionLess, "less"},
		{gputypes.CompareFunctionEqual, "equal"},
		{gputypes.CompareFunctionLessEqual, "less-equal"},
		{gputypes.CompareFunctionGreater, "greater"},
		{gputypes.CompareFunctionNotEqual, "not-equal"},
		{gputypes.CompareFunctionGreaterEqual, "greater-equal"},
		{gputypes.CompareFunctionAlways, "always"},
		{gputypes.CompareFunctionUndefined, ""},
	}
	for _, tc := range tests {
		got := CompareFunctionToJS(tc.fn)
		if got != tc.want {
			t.Errorf("CompareFunctionToJS(%v) = %q, want %q", tc.fn, got, tc.want)
		}
	}
}

// TestPrimitiveTopologyToJS verifies all primitive topology mappings.
func TestPrimitiveTopologyToJS(t *testing.T) {
	tests := []struct {
		topo gputypes.PrimitiveTopology
		want string
	}{
		{gputypes.PrimitiveTopologyPointList, "point-list"},
		{gputypes.PrimitiveTopologyLineList, "line-list"},
		{gputypes.PrimitiveTopologyLineStrip, "line-strip"},
		{gputypes.PrimitiveTopologyTriangleList, "triangle-list"},
		{gputypes.PrimitiveTopologyTriangleStrip, "triangle-strip"},
	}
	for _, tc := range tests {
		got := PrimitiveTopologyToJS(tc.topo)
		if got != tc.want {
			t.Errorf("PrimitiveTopologyToJS(%v) = %q, want %q", tc.topo, got, tc.want)
		}
	}
}

// TestFrontFaceToJS verifies all front face mappings.
func TestFrontFaceToJS(t *testing.T) {
	tests := []struct {
		face gputypes.FrontFace
		want string
	}{
		{gputypes.FrontFaceCCW, "ccw"},
		{gputypes.FrontFaceCW, "cw"},
	}
	for _, tc := range tests {
		got := FrontFaceToJS(tc.face)
		if got != tc.want {
			t.Errorf("FrontFaceToJS(%v) = %q, want %q", tc.face, got, tc.want)
		}
	}
}

// TestCullModeToJS verifies all cull mode mappings.
func TestCullModeToJS(t *testing.T) {
	tests := []struct {
		mode gputypes.CullMode
		want string
	}{
		{gputypes.CullModeNone, "none"},
		{gputypes.CullModeFront, "front"},
		{gputypes.CullModeBack, "back"},
	}
	for _, tc := range tests {
		got := CullModeToJS(tc.mode)
		if got != tc.want {
			t.Errorf("CullModeToJS(%v) = %q, want %q", tc.mode, got, tc.want)
		}
	}
}

// TestIndexFormatToJS verifies all index format mappings.
func TestIndexFormatToJS(t *testing.T) {
	tests := []struct {
		fmt  gputypes.IndexFormat
		want string
	}{
		{gputypes.IndexFormatUint16, "uint16"},
		{gputypes.IndexFormatUint32, "uint32"},
		{gputypes.IndexFormatUndefined, ""},
	}
	for _, tc := range tests {
		got := IndexFormatToJS(tc.fmt)
		if got != tc.want {
			t.Errorf("IndexFormatToJS(%v) = %q, want %q", tc.fmt, got, tc.want)
		}
	}
}

// TestVertexFormatToJS verifies all vertex format mappings.
func TestVertexFormatToJS(t *testing.T) {
	tests := []struct {
		fmt  gputypes.VertexFormat
		want string
	}{
		{gputypes.VertexFormatUint8x2, "uint8x2"},
		{gputypes.VertexFormatUint8x4, "uint8x4"},
		{gputypes.VertexFormatSint8x2, "sint8x2"},
		{gputypes.VertexFormatSint8x4, "sint8x4"},
		{gputypes.VertexFormatUnorm8x2, "unorm8x2"},
		{gputypes.VertexFormatUnorm8x4, "unorm8x4"},
		{gputypes.VertexFormatSnorm8x2, "snorm8x2"},
		{gputypes.VertexFormatSnorm8x4, "snorm8x4"},
		{gputypes.VertexFormatUint16x2, "uint16x2"},
		{gputypes.VertexFormatUint16x4, "uint16x4"},
		{gputypes.VertexFormatSint16x2, "sint16x2"},
		{gputypes.VertexFormatSint16x4, "sint16x4"},
		{gputypes.VertexFormatUnorm16x2, "unorm16x2"},
		{gputypes.VertexFormatUnorm16x4, "unorm16x4"},
		{gputypes.VertexFormatSnorm16x2, "snorm16x2"},
		{gputypes.VertexFormatSnorm16x4, "snorm16x4"},
		{gputypes.VertexFormatFloat16x2, "float16x2"},
		{gputypes.VertexFormatFloat16x4, "float16x4"},
		{gputypes.VertexFormatFloat32, "float32"},
		{gputypes.VertexFormatFloat32x2, "float32x2"},
		{gputypes.VertexFormatFloat32x3, "float32x3"},
		{gputypes.VertexFormatFloat32x4, "float32x4"},
		{gputypes.VertexFormatUint32, "uint32"},
		{gputypes.VertexFormatUint32x2, "uint32x2"},
		{gputypes.VertexFormatUint32x3, "uint32x3"},
		{gputypes.VertexFormatUint32x4, "uint32x4"},
		{gputypes.VertexFormatSint32, "sint32"},
		{gputypes.VertexFormatSint32x2, "sint32x2"},
		{gputypes.VertexFormatSint32x3, "sint32x3"},
		{gputypes.VertexFormatSint32x4, "sint32x4"},
		{gputypes.VertexFormatUnorm1010102, "unorm10-10-10-2"},
		// Undefined returns empty
		{gputypes.VertexFormatUndefined, ""},
	}
	for _, tc := range tests {
		got := VertexFormatToJS(tc.fmt)
		if got != tc.want {
			t.Errorf("VertexFormatToJS(%v) = %q, want %q", tc.fmt, got, tc.want)
		}
	}
}

// TestVertexStepModeToJS verifies all vertex step mode mappings.
func TestVertexStepModeToJS(t *testing.T) {
	tests := []struct {
		mode gputypes.VertexStepMode
		want string
	}{
		{gputypes.VertexStepModeVertex, "vertex"},
		{gputypes.VertexStepModeInstance, "instance"},
		{gputypes.VertexStepModeUndefined, "vertex"},
	}
	for _, tc := range tests {
		got := VertexStepModeToJS(tc.mode)
		if got != tc.want {
			t.Errorf("VertexStepModeToJS(%v) = %q, want %q", tc.mode, got, tc.want)
		}
	}
}

// TestBlendFactorToJS verifies all blend factor mappings.
func TestBlendFactorToJS(t *testing.T) {
	tests := []struct {
		factor gputypes.BlendFactor
		want   string
	}{
		{gputypes.BlendFactorZero, "zero"},
		{gputypes.BlendFactorOne, "one"},
		{gputypes.BlendFactorSrc, "src"},
		{gputypes.BlendFactorOneMinusSrc, "one-minus-src"},
		{gputypes.BlendFactorSrcAlpha, "src-alpha"},
		{gputypes.BlendFactorOneMinusSrcAlpha, "one-minus-src-alpha"},
		{gputypes.BlendFactorDst, "dst"},
		{gputypes.BlendFactorOneMinusDst, "one-minus-dst"},
		{gputypes.BlendFactorDstAlpha, "dst-alpha"},
		{gputypes.BlendFactorOneMinusDstAlpha, "one-minus-dst-alpha"},
		{gputypes.BlendFactorSrcAlphaSaturated, "src-alpha-saturated"},
		{gputypes.BlendFactorConstant, "constant"},
		{gputypes.BlendFactorOneMinusConstant, "one-minus-constant"},
	}
	for _, tc := range tests {
		got := BlendFactorToJS(tc.factor)
		if got != tc.want {
			t.Errorf("BlendFactorToJS(%v) = %q, want %q", tc.factor, got, tc.want)
		}
	}
}

// TestBlendOperationToJS verifies all blend operation mappings.
func TestBlendOperationToJS(t *testing.T) {
	tests := []struct {
		op   gputypes.BlendOperation
		want string
	}{
		{gputypes.BlendOperationAdd, "add"},
		{gputypes.BlendOperationSubtract, "subtract"},
		{gputypes.BlendOperationReverseSubtract, "reverse-subtract"},
		{gputypes.BlendOperationMin, "min"},
		{gputypes.BlendOperationMax, "max"},
	}
	for _, tc := range tests {
		got := BlendOperationToJS(tc.op)
		if got != tc.want {
			t.Errorf("BlendOperationToJS(%v) = %q, want %q", tc.op, got, tc.want)
		}
	}
}

// TestStencilOperationToJS verifies all stencil operation mappings.
func TestStencilOperationToJS(t *testing.T) {
	tests := []struct {
		op   gputypes.StencilOperation
		want string
	}{
		{gputypes.StencilOperationKeep, "keep"},
		{gputypes.StencilOperationZero, "zero"},
		{gputypes.StencilOperationReplace, "replace"},
		{gputypes.StencilOperationInvert, "invert"},
		{gputypes.StencilOperationIncrementClamp, "increment-clamp"},
		{gputypes.StencilOperationDecrementClamp, "decrement-clamp"},
		{gputypes.StencilOperationIncrementWrap, "increment-wrap"},
		{gputypes.StencilOperationDecrementWrap, "decrement-wrap"},
	}
	for _, tc := range tests {
		got := StencilOperationToJS(tc.op)
		if got != tc.want {
			t.Errorf("StencilOperationToJS(%v) = %q, want %q", tc.op, got, tc.want)
		}
	}
}

// TestBufferBindingTypeToJS verifies all buffer binding type mappings.
func TestBufferBindingTypeToJS(t *testing.T) {
	tests := []struct {
		typ  gputypes.BufferBindingType
		want string
	}{
		{gputypes.BufferBindingTypeUniform, "uniform"},
		{gputypes.BufferBindingTypeStorage, "storage"},
		{gputypes.BufferBindingTypeReadOnlyStorage, "read-only-storage"},
		{gputypes.BufferBindingTypeUndefined, "uniform"},
	}
	for _, tc := range tests {
		got := BufferBindingTypeToJS(tc.typ)
		if got != tc.want {
			t.Errorf("BufferBindingTypeToJS(%v) = %q, want %q", tc.typ, got, tc.want)
		}
	}
}

// TestSamplerBindingTypeToJS verifies all sampler binding type mappings.
func TestSamplerBindingTypeToJS(t *testing.T) {
	tests := []struct {
		typ  gputypes.SamplerBindingType
		want string
	}{
		{gputypes.SamplerBindingTypeFiltering, "filtering"},
		{gputypes.SamplerBindingTypeNonFiltering, "non-filtering"},
		{gputypes.SamplerBindingTypeComparison, "comparison"},
		{gputypes.SamplerBindingTypeUndefined, "filtering"},
	}
	for _, tc := range tests {
		got := SamplerBindingTypeToJS(tc.typ)
		if got != tc.want {
			t.Errorf("SamplerBindingTypeToJS(%v) = %q, want %q", tc.typ, got, tc.want)
		}
	}
}

// TestTextureSampleTypeToJS verifies all texture sample type mappings.
func TestTextureSampleTypeToJS(t *testing.T) {
	tests := []struct {
		typ  gputypes.TextureSampleType
		want string
	}{
		{gputypes.TextureSampleTypeFloat, "float"},
		{gputypes.TextureSampleTypeUnfilterableFloat, "unfilterable-float"},
		{gputypes.TextureSampleTypeDepth, "depth"},
		{gputypes.TextureSampleTypeSint, "sint"},
		{gputypes.TextureSampleTypeUint, "uint"},
		{gputypes.TextureSampleTypeUndefined, "float"},
	}
	for _, tc := range tests {
		got := TextureSampleTypeToJS(tc.typ)
		if got != tc.want {
			t.Errorf("TextureSampleTypeToJS(%v) = %q, want %q", tc.typ, got, tc.want)
		}
	}
}

// TestStorageTextureAccessToJS verifies all storage texture access mappings.
func TestStorageTextureAccessToJS(t *testing.T) {
	tests := []struct {
		access gputypes.StorageTextureAccess
		want   string
	}{
		{gputypes.StorageTextureAccessWriteOnly, "write-only"},
		{gputypes.StorageTextureAccessReadOnly, "read-only"},
		{gputypes.StorageTextureAccessReadWrite, "read-write"},
		{gputypes.StorageTextureAccessUndefined, "write-only"},
	}
	for _, tc := range tests {
		got := StorageTextureAccessToJS(tc.access)
		if got != tc.want {
			t.Errorf("StorageTextureAccessToJS(%v) = %q, want %q", tc.access, got, tc.want)
		}
	}
}

// TestTextureFormatMapCompleteness verifies that every non-Undefined format in
// gputypes has a mapping. Missing entries would cause silent empty strings at runtime.
func TestTextureFormatMapCompleteness(t *testing.T) {
	// All non-Undefined, non-Unorm16/Snorm16 formats from gputypes
	allFormats := []gputypes.TextureFormat{
		gputypes.TextureFormatR8Unorm, gputypes.TextureFormatR8Snorm,
		gputypes.TextureFormatR8Uint, gputypes.TextureFormatR8Sint,
		gputypes.TextureFormatR16Uint, gputypes.TextureFormatR16Sint, gputypes.TextureFormatR16Float,
		gputypes.TextureFormatRG8Unorm, gputypes.TextureFormatRG8Snorm,
		gputypes.TextureFormatRG8Uint, gputypes.TextureFormatRG8Sint,
		gputypes.TextureFormatR32Float, gputypes.TextureFormatR32Uint, gputypes.TextureFormatR32Sint,
		gputypes.TextureFormatRG16Uint, gputypes.TextureFormatRG16Sint, gputypes.TextureFormatRG16Float,
		gputypes.TextureFormatRGBA8Unorm, gputypes.TextureFormatRGBA8UnormSrgb,
		gputypes.TextureFormatRGBA8Snorm, gputypes.TextureFormatRGBA8Uint, gputypes.TextureFormatRGBA8Sint,
		gputypes.TextureFormatBGRA8Unorm, gputypes.TextureFormatBGRA8UnormSrgb,
		gputypes.TextureFormatRGB10A2Uint, gputypes.TextureFormatRGB10A2Unorm,
		gputypes.TextureFormatRG11B10Ufloat, gputypes.TextureFormatRGB9E5Ufloat,
		gputypes.TextureFormatRG32Float, gputypes.TextureFormatRG32Uint, gputypes.TextureFormatRG32Sint,
		gputypes.TextureFormatRGBA16Uint, gputypes.TextureFormatRGBA16Sint, gputypes.TextureFormatRGBA16Float,
		gputypes.TextureFormatRGBA32Float, gputypes.TextureFormatRGBA32Uint, gputypes.TextureFormatRGBA32Sint,
		gputypes.TextureFormatStencil8, gputypes.TextureFormatDepth16Unorm,
		gputypes.TextureFormatDepth24Plus, gputypes.TextureFormatDepth24PlusStencil8,
		gputypes.TextureFormatDepth32Float, gputypes.TextureFormatDepth32FloatStencil8,
		gputypes.TextureFormatBC1RGBAUnorm, gputypes.TextureFormatBC1RGBAUnormSrgb,
		gputypes.TextureFormatBC2RGBAUnorm, gputypes.TextureFormatBC2RGBAUnormSrgb,
		gputypes.TextureFormatBC3RGBAUnorm, gputypes.TextureFormatBC3RGBAUnormSrgb,
		gputypes.TextureFormatBC4RUnorm, gputypes.TextureFormatBC4RSnorm,
		gputypes.TextureFormatBC5RGUnorm, gputypes.TextureFormatBC5RGSnorm,
		gputypes.TextureFormatBC6HRGBUfloat, gputypes.TextureFormatBC6HRGBFloat,
		gputypes.TextureFormatBC7RGBAUnorm, gputypes.TextureFormatBC7RGBAUnormSrgb,
		gputypes.TextureFormatETC2RGB8Unorm, gputypes.TextureFormatETC2RGB8UnormSrgb,
		gputypes.TextureFormatETC2RGB8A1Unorm, gputypes.TextureFormatETC2RGB8A1UnormSrgb,
		gputypes.TextureFormatETC2RGBA8Unorm, gputypes.TextureFormatETC2RGBA8UnormSrgb,
		gputypes.TextureFormatEACR11Unorm, gputypes.TextureFormatEACR11Snorm,
		gputypes.TextureFormatEACRG11Unorm, gputypes.TextureFormatEACRG11Snorm,
		gputypes.TextureFormatASTC4x4Unorm, gputypes.TextureFormatASTC4x4UnormSrgb,
		gputypes.TextureFormatASTC5x4Unorm, gputypes.TextureFormatASTC5x4UnormSrgb,
		gputypes.TextureFormatASTC5x5Unorm, gputypes.TextureFormatASTC5x5UnormSrgb,
		gputypes.TextureFormatASTC6x5Unorm, gputypes.TextureFormatASTC6x5UnormSrgb,
		gputypes.TextureFormatASTC6x6Unorm, gputypes.TextureFormatASTC6x6UnormSrgb,
		gputypes.TextureFormatASTC8x5Unorm, gputypes.TextureFormatASTC8x5UnormSrgb,
		gputypes.TextureFormatASTC8x6Unorm, gputypes.TextureFormatASTC8x6UnormSrgb,
		gputypes.TextureFormatASTC8x8Unorm, gputypes.TextureFormatASTC8x8UnormSrgb,
		gputypes.TextureFormatASTC10x5Unorm, gputypes.TextureFormatASTC10x5UnormSrgb,
		gputypes.TextureFormatASTC10x6Unorm, gputypes.TextureFormatASTC10x6UnormSrgb,
		gputypes.TextureFormatASTC10x8Unorm, gputypes.TextureFormatASTC10x8UnormSrgb,
		gputypes.TextureFormatASTC10x10Unorm, gputypes.TextureFormatASTC10x10UnormSrgb,
		gputypes.TextureFormatASTC12x10Unorm, gputypes.TextureFormatASTC12x10UnormSrgb,
		gputypes.TextureFormatASTC12x12Unorm, gputypes.TextureFormatASTC12x12UnormSrgb,
	}
	for _, f := range allFormats {
		s := TextureFormatToJS(f)
		if s == "" {
			t.Errorf("TextureFormatToJS(%v) returned empty string — missing mapping", f)
		}
	}
}

// TestVertexFormatMapCompleteness verifies that every non-Undefined vertex format
// has a mapping.
func TestVertexFormatMapCompleteness(t *testing.T) {
	allFormats := []gputypes.VertexFormat{
		gputypes.VertexFormatUint8x2, gputypes.VertexFormatUint8x4,
		gputypes.VertexFormatSint8x2, gputypes.VertexFormatSint8x4,
		gputypes.VertexFormatUnorm8x2, gputypes.VertexFormatUnorm8x4,
		gputypes.VertexFormatSnorm8x2, gputypes.VertexFormatSnorm8x4,
		gputypes.VertexFormatUint16x2, gputypes.VertexFormatUint16x4,
		gputypes.VertexFormatSint16x2, gputypes.VertexFormatSint16x4,
		gputypes.VertexFormatUnorm16x2, gputypes.VertexFormatUnorm16x4,
		gputypes.VertexFormatSnorm16x2, gputypes.VertexFormatSnorm16x4,
		gputypes.VertexFormatFloat16x2, gputypes.VertexFormatFloat16x4,
		gputypes.VertexFormatFloat32, gputypes.VertexFormatFloat32x2,
		gputypes.VertexFormatFloat32x3, gputypes.VertexFormatFloat32x4,
		gputypes.VertexFormatUint32, gputypes.VertexFormatUint32x2,
		gputypes.VertexFormatUint32x3, gputypes.VertexFormatUint32x4,
		gputypes.VertexFormatSint32, gputypes.VertexFormatSint32x2,
		gputypes.VertexFormatSint32x3, gputypes.VertexFormatSint32x4,
		gputypes.VertexFormatUnorm1010102,
	}
	for _, f := range allFormats {
		s := VertexFormatToJS(f)
		if s == "" {
			t.Errorf("VertexFormatToJS(%v) returned empty string — missing mapping", f)
		}
	}
}
