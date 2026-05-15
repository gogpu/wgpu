package browser

import "github.com/gogpu/gputypes"

// TextureFormatToJS converts a gputypes.TextureFormat to the WebGPU JS string.
// Returns "" for TextureFormatUndefined.
func TextureFormatToJS(f gputypes.TextureFormat) string {
	s, ok := textureFormatMap[f]
	if ok {
		return s
	}
	return ""
}

// textureFormatMap maps gputypes texture format constants to WebGPU JS string names.
// Covers all formats defined in the WebGPU specification.
var textureFormatMap = map[gputypes.TextureFormat]string{
	// 8-bit
	gputypes.TextureFormatR8Unorm: "r8unorm",
	gputypes.TextureFormatR8Snorm: "r8snorm",
	gputypes.TextureFormatR8Uint:  "r8uint",
	gputypes.TextureFormatR8Sint:  "r8sint",

	// 16-bit
	gputypes.TextureFormatR16Uint:  "r16uint",
	gputypes.TextureFormatR16Sint:  "r16sint",
	gputypes.TextureFormatR16Float: "r16float",
	gputypes.TextureFormatRG8Unorm: "rg8unorm",
	gputypes.TextureFormatRG8Snorm: "rg8snorm",
	gputypes.TextureFormatRG8Uint:  "rg8uint",
	gputypes.TextureFormatRG8Sint:  "rg8sint",

	// 32-bit
	gputypes.TextureFormatR32Float:       "r32float",
	gputypes.TextureFormatR32Uint:        "r32uint",
	gputypes.TextureFormatR32Sint:        "r32sint",
	gputypes.TextureFormatRG16Uint:       "rg16uint",
	gputypes.TextureFormatRG16Sint:       "rg16sint",
	gputypes.TextureFormatRG16Float:      "rg16float",
	gputypes.TextureFormatRGBA8Unorm:     "rgba8unorm",
	gputypes.TextureFormatRGBA8UnormSrgb: "rgba8unorm-srgb",
	gputypes.TextureFormatRGBA8Snorm:     "rgba8snorm",
	gputypes.TextureFormatRGBA8Uint:      "rgba8uint",
	gputypes.TextureFormatRGBA8Sint:      "rgba8sint",
	gputypes.TextureFormatBGRA8Unorm:     "bgra8unorm",
	gputypes.TextureFormatBGRA8UnormSrgb: "bgra8unorm-srgb",

	// Packed 32-bit
	gputypes.TextureFormatRGB10A2Uint:   "rgb10a2uint",
	gputypes.TextureFormatRGB10A2Unorm:  "rgb10a2unorm",
	gputypes.TextureFormatRG11B10Ufloat: "rg11b10ufloat",
	gputypes.TextureFormatRGB9E5Ufloat:  "rgb9e5ufloat",

	// 64-bit
	gputypes.TextureFormatRG32Float:   "rg32float",
	gputypes.TextureFormatRG32Uint:    "rg32uint",
	gputypes.TextureFormatRG32Sint:    "rg32sint",
	gputypes.TextureFormatRGBA16Uint:  "rgba16uint",
	gputypes.TextureFormatRGBA16Sint:  "rgba16sint",
	gputypes.TextureFormatRGBA16Float: "rgba16float",

	// 128-bit
	gputypes.TextureFormatRGBA32Float: "rgba32float",
	gputypes.TextureFormatRGBA32Uint:  "rgba32uint",
	gputypes.TextureFormatRGBA32Sint:  "rgba32sint",

	// Depth/stencil
	gputypes.TextureFormatStencil8:             "stencil8",
	gputypes.TextureFormatDepth16Unorm:         "depth16unorm",
	gputypes.TextureFormatDepth24Plus:          "depth24plus",
	gputypes.TextureFormatDepth24PlusStencil8:  "depth24plus-stencil8",
	gputypes.TextureFormatDepth32Float:         "depth32float",
	gputypes.TextureFormatDepth32FloatStencil8: "depth32float-stencil8",

	// BC compressed
	gputypes.TextureFormatBC1RGBAUnorm:     "bc1-rgba-unorm",
	gputypes.TextureFormatBC1RGBAUnormSrgb: "bc1-rgba-unorm-srgb",
	gputypes.TextureFormatBC2RGBAUnorm:     "bc2-rgba-unorm",
	gputypes.TextureFormatBC2RGBAUnormSrgb: "bc2-rgba-unorm-srgb",
	gputypes.TextureFormatBC3RGBAUnorm:     "bc3-rgba-unorm",
	gputypes.TextureFormatBC3RGBAUnormSrgb: "bc3-rgba-unorm-srgb",
	gputypes.TextureFormatBC4RUnorm:        "bc4-r-unorm",
	gputypes.TextureFormatBC4RSnorm:        "bc4-r-snorm",
	gputypes.TextureFormatBC5RGUnorm:       "bc5-rg-unorm",
	gputypes.TextureFormatBC5RGSnorm:       "bc5-rg-snorm",
	gputypes.TextureFormatBC6HRGBUfloat:    "bc6h-rgb-ufloat",
	gputypes.TextureFormatBC6HRGBFloat:     "bc6h-rgb-float",
	gputypes.TextureFormatBC7RGBAUnorm:     "bc7-rgba-unorm",
	gputypes.TextureFormatBC7RGBAUnormSrgb: "bc7-rgba-unorm-srgb",

	// ETC2 compressed
	gputypes.TextureFormatETC2RGB8Unorm:       "etc2-rgb8unorm",
	gputypes.TextureFormatETC2RGB8UnormSrgb:   "etc2-rgb8unorm-srgb",
	gputypes.TextureFormatETC2RGB8A1Unorm:     "etc2-rgb8a1unorm",
	gputypes.TextureFormatETC2RGB8A1UnormSrgb: "etc2-rgb8a1unorm-srgb",
	gputypes.TextureFormatETC2RGBA8Unorm:      "etc2-rgba8unorm",
	gputypes.TextureFormatETC2RGBA8UnormSrgb:  "etc2-rgba8unorm-srgb",
	gputypes.TextureFormatEACR11Unorm:         "eac-r11unorm",
	gputypes.TextureFormatEACR11Snorm:         "eac-r11snorm",
	gputypes.TextureFormatEACRG11Unorm:        "eac-rg11unorm",
	gputypes.TextureFormatEACRG11Snorm:        "eac-rg11snorm",

	// ASTC compressed
	gputypes.TextureFormatASTC4x4Unorm:       "astc-4x4-unorm",
	gputypes.TextureFormatASTC4x4UnormSrgb:   "astc-4x4-unorm-srgb",
	gputypes.TextureFormatASTC5x4Unorm:       "astc-5x4-unorm",
	gputypes.TextureFormatASTC5x4UnormSrgb:   "astc-5x4-unorm-srgb",
	gputypes.TextureFormatASTC5x5Unorm:       "astc-5x5-unorm",
	gputypes.TextureFormatASTC5x5UnormSrgb:   "astc-5x5-unorm-srgb",
	gputypes.TextureFormatASTC6x5Unorm:       "astc-6x5-unorm",
	gputypes.TextureFormatASTC6x5UnormSrgb:   "astc-6x5-unorm-srgb",
	gputypes.TextureFormatASTC6x6Unorm:       "astc-6x6-unorm",
	gputypes.TextureFormatASTC6x6UnormSrgb:   "astc-6x6-unorm-srgb",
	gputypes.TextureFormatASTC8x5Unorm:       "astc-8x5-unorm",
	gputypes.TextureFormatASTC8x5UnormSrgb:   "astc-8x5-unorm-srgb",
	gputypes.TextureFormatASTC8x6Unorm:       "astc-8x6-unorm",
	gputypes.TextureFormatASTC8x6UnormSrgb:   "astc-8x6-unorm-srgb",
	gputypes.TextureFormatASTC8x8Unorm:       "astc-8x8-unorm",
	gputypes.TextureFormatASTC8x8UnormSrgb:   "astc-8x8-unorm-srgb",
	gputypes.TextureFormatASTC10x5Unorm:      "astc-10x5-unorm",
	gputypes.TextureFormatASTC10x5UnormSrgb:  "astc-10x5-unorm-srgb",
	gputypes.TextureFormatASTC10x6Unorm:      "astc-10x6-unorm",
	gputypes.TextureFormatASTC10x6UnormSrgb:  "astc-10x6-unorm-srgb",
	gputypes.TextureFormatASTC10x8Unorm:      "astc-10x8-unorm",
	gputypes.TextureFormatASTC10x8UnormSrgb:  "astc-10x8-unorm-srgb",
	gputypes.TextureFormatASTC10x10Unorm:     "astc-10x10-unorm",
	gputypes.TextureFormatASTC10x10UnormSrgb: "astc-10x10-unorm-srgb",
	gputypes.TextureFormatASTC12x10Unorm:     "astc-12x10-unorm",
	gputypes.TextureFormatASTC12x10UnormSrgb: "astc-12x10-unorm-srgb",
	gputypes.TextureFormatASTC12x12Unorm:     "astc-12x12-unorm",
	gputypes.TextureFormatASTC12x12UnormSrgb: "astc-12x12-unorm-srgb",
}

// TextureDimensionToJS converts a gputypes.TextureDimension to the WebGPU JS string.
func TextureDimensionToJS(d gputypes.TextureDimension) string {
	switch d {
	case gputypes.TextureDimension1D:
		return "1d"
	case gputypes.TextureDimension2D:
		return "2d"
	case gputypes.TextureDimension3D:
		return "3d"
	default:
		return ""
	}
}

// TextureViewDimensionToJS converts a gputypes.TextureViewDimension to JS string.
func TextureViewDimensionToJS(d gputypes.TextureViewDimension) string {
	switch d {
	case gputypes.TextureViewDimension1D:
		return "1d"
	case gputypes.TextureViewDimension2D:
		return "2d"
	case gputypes.TextureViewDimension2DArray:
		return "2d-array"
	case gputypes.TextureViewDimensionCube:
		return "cube"
	case gputypes.TextureViewDimensionCubeArray:
		return "cube-array"
	case gputypes.TextureViewDimension3D:
		return "3d"
	default:
		return ""
	}
}

// TextureAspectToJS converts a gputypes.TextureAspect to JS string.
func TextureAspectToJS(a gputypes.TextureAspect) string {
	switch a {
	case gputypes.TextureAspectAll:
		return "all"
	case gputypes.TextureAspectStencilOnly:
		return "stencil-only"
	case gputypes.TextureAspectDepthOnly:
		return "depth-only"
	default:
		return ""
	}
}

// AddressModeToJS converts a gputypes.AddressMode to JS string.
func AddressModeToJS(m gputypes.AddressMode) string {
	switch m {
	case gputypes.AddressModeClampToEdge:
		return "clamp-to-edge"
	case gputypes.AddressModeRepeat:
		return "repeat"
	case gputypes.AddressModeMirrorRepeat:
		return "mirror-repeat"
	default:
		return "clamp-to-edge"
	}
}

// FilterModeToJS converts a gputypes.FilterMode to JS string.
func FilterModeToJS(m gputypes.FilterMode) string {
	switch m {
	case gputypes.FilterModeNearest:
		return "nearest"
	case gputypes.FilterModeLinear:
		return "linear"
	default:
		return "nearest"
	}
}

// CompareFunctionToJS converts a gputypes.CompareFunction to JS string.
func CompareFunctionToJS(f gputypes.CompareFunction) string {
	switch f {
	case gputypes.CompareFunctionNever:
		return "never"
	case gputypes.CompareFunctionLess:
		return "less"
	case gputypes.CompareFunctionEqual:
		return "equal"
	case gputypes.CompareFunctionLessEqual:
		return "less-equal"
	case gputypes.CompareFunctionGreater:
		return "greater"
	case gputypes.CompareFunctionNotEqual:
		return "not-equal"
	case gputypes.CompareFunctionGreaterEqual:
		return "greater-equal"
	case gputypes.CompareFunctionAlways:
		return "always"
	default:
		return ""
	}
}

// PrimitiveTopologyToJS converts a gputypes.PrimitiveTopology to JS string.
func PrimitiveTopologyToJS(t gputypes.PrimitiveTopology) string {
	switch t {
	case gputypes.PrimitiveTopologyPointList:
		return "point-list"
	case gputypes.PrimitiveTopologyLineList:
		return "line-list"
	case gputypes.PrimitiveTopologyLineStrip:
		return "line-strip"
	case gputypes.PrimitiveTopologyTriangleList:
		return "triangle-list"
	case gputypes.PrimitiveTopologyTriangleStrip:
		return "triangle-strip"
	default:
		return "triangle-list"
	}
}

// FrontFaceToJS converts a gputypes.FrontFace to JS string.
func FrontFaceToJS(f gputypes.FrontFace) string {
	switch f {
	case gputypes.FrontFaceCW:
		return "cw"
	default:
		return "ccw"
	}
}

// CullModeToJS converts a gputypes.CullMode to JS string.
func CullModeToJS(m gputypes.CullMode) string {
	switch m {
	case gputypes.CullModeFront:
		return "front"
	case gputypes.CullModeBack:
		return "back"
	default:
		return "none"
	}
}

// IndexFormatToJS converts a gputypes.IndexFormat to JS string.
func IndexFormatToJS(f gputypes.IndexFormat) string {
	switch f {
	case gputypes.IndexFormatUint16:
		return "uint16"
	case gputypes.IndexFormatUint32:
		return "uint32"
	default:
		return ""
	}
}

// VertexFormatToJS converts a gputypes.VertexFormat to JS string.
func VertexFormatToJS(f gputypes.VertexFormat) string {
	s, ok := vertexFormatMap[f]
	if ok {
		return s
	}
	return ""
}

var vertexFormatMap = map[gputypes.VertexFormat]string{
	gputypes.VertexFormatUint8x2:      "uint8x2",
	gputypes.VertexFormatUint8x4:      "uint8x4",
	gputypes.VertexFormatSint8x2:      "sint8x2",
	gputypes.VertexFormatSint8x4:      "sint8x4",
	gputypes.VertexFormatUnorm8x2:     "unorm8x2",
	gputypes.VertexFormatUnorm8x4:     "unorm8x4",
	gputypes.VertexFormatSnorm8x2:     "snorm8x2",
	gputypes.VertexFormatSnorm8x4:     "snorm8x4",
	gputypes.VertexFormatUint16x2:     "uint16x2",
	gputypes.VertexFormatUint16x4:     "uint16x4",
	gputypes.VertexFormatSint16x2:     "sint16x2",
	gputypes.VertexFormatSint16x4:     "sint16x4",
	gputypes.VertexFormatUnorm16x2:    "unorm16x2",
	gputypes.VertexFormatUnorm16x4:    "unorm16x4",
	gputypes.VertexFormatSnorm16x2:    "snorm16x2",
	gputypes.VertexFormatSnorm16x4:    "snorm16x4",
	gputypes.VertexFormatFloat16x2:    "float16x2",
	gputypes.VertexFormatFloat16x4:    "float16x4",
	gputypes.VertexFormatFloat32:      "float32",
	gputypes.VertexFormatFloat32x2:    "float32x2",
	gputypes.VertexFormatFloat32x3:    "float32x3",
	gputypes.VertexFormatFloat32x4:    "float32x4",
	gputypes.VertexFormatUint32:       "uint32",
	gputypes.VertexFormatUint32x2:     "uint32x2",
	gputypes.VertexFormatUint32x3:     "uint32x3",
	gputypes.VertexFormatUint32x4:     "uint32x4",
	gputypes.VertexFormatSint32:       "sint32",
	gputypes.VertexFormatSint32x2:     "sint32x2",
	gputypes.VertexFormatSint32x3:     "sint32x3",
	gputypes.VertexFormatSint32x4:     "sint32x4",
	gputypes.VertexFormatUnorm1010102: "unorm10-10-10-2",
}

// VertexStepModeToJS converts a gputypes.VertexStepMode to JS string.
func VertexStepModeToJS(m gputypes.VertexStepMode) string {
	switch m {
	case gputypes.VertexStepModeInstance:
		return "instance"
	default:
		return "vertex"
	}
}

// BlendFactorToJS converts a gputypes.BlendFactor to JS string.
func BlendFactorToJS(f gputypes.BlendFactor) string {
	switch f {
	case gputypes.BlendFactorZero:
		return "zero"
	case gputypes.BlendFactorOne:
		return "one"
	case gputypes.BlendFactorSrc:
		return "src"
	case gputypes.BlendFactorOneMinusSrc:
		return "one-minus-src"
	case gputypes.BlendFactorSrcAlpha:
		return "src-alpha"
	case gputypes.BlendFactorOneMinusSrcAlpha:
		return "one-minus-src-alpha"
	case gputypes.BlendFactorDst:
		return "dst"
	case gputypes.BlendFactorOneMinusDst:
		return "one-minus-dst"
	case gputypes.BlendFactorDstAlpha:
		return "dst-alpha"
	case gputypes.BlendFactorOneMinusDstAlpha:
		return "one-minus-dst-alpha"
	case gputypes.BlendFactorSrcAlphaSaturated:
		return "src-alpha-saturated"
	case gputypes.BlendFactorConstant:
		return "constant"
	case gputypes.BlendFactorOneMinusConstant:
		return "one-minus-constant"
	default:
		return "one"
	}
}

// BlendOperationToJS converts a gputypes.BlendOperation to JS string.
func BlendOperationToJS(op gputypes.BlendOperation) string {
	switch op {
	case gputypes.BlendOperationAdd:
		return "add"
	case gputypes.BlendOperationSubtract:
		return "subtract"
	case gputypes.BlendOperationReverseSubtract:
		return "reverse-subtract"
	case gputypes.BlendOperationMin:
		return "min"
	case gputypes.BlendOperationMax:
		return "max"
	default:
		return "add"
	}
}

// StencilOperationToJS converts a gputypes.StencilOperation to JS string.
func StencilOperationToJS(op gputypes.StencilOperation) string {
	switch op {
	case gputypes.StencilOperationKeep:
		return "keep"
	case gputypes.StencilOperationZero:
		return "zero"
	case gputypes.StencilOperationReplace:
		return "replace"
	case gputypes.StencilOperationInvert:
		return "invert"
	case gputypes.StencilOperationIncrementClamp:
		return "increment-clamp"
	case gputypes.StencilOperationDecrementClamp:
		return "decrement-clamp"
	case gputypes.StencilOperationIncrementWrap:
		return "increment-wrap"
	case gputypes.StencilOperationDecrementWrap:
		return "decrement-wrap"
	default:
		return "keep"
	}
}

// BufferBindingTypeToJS converts a gputypes.BufferBindingType to JS string.
func BufferBindingTypeToJS(t gputypes.BufferBindingType) string {
	switch t {
	case gputypes.BufferBindingTypeUniform:
		return "uniform"
	case gputypes.BufferBindingTypeStorage:
		return "storage"
	case gputypes.BufferBindingTypeReadOnlyStorage:
		return "read-only-storage"
	default:
		return "uniform"
	}
}

// SamplerBindingTypeToJS converts a gputypes.SamplerBindingType to JS string.
func SamplerBindingTypeToJS(t gputypes.SamplerBindingType) string {
	switch t {
	case gputypes.SamplerBindingTypeFiltering:
		return "filtering"
	case gputypes.SamplerBindingTypeNonFiltering:
		return "non-filtering"
	case gputypes.SamplerBindingTypeComparison:
		return "comparison"
	default:
		return "filtering"
	}
}

// TextureSampleTypeToJS converts a gputypes.TextureSampleType to JS string.
func TextureSampleTypeToJS(t gputypes.TextureSampleType) string {
	switch t {
	case gputypes.TextureSampleTypeFloat:
		return "float"
	case gputypes.TextureSampleTypeUnfilterableFloat:
		return "unfilterable-float"
	case gputypes.TextureSampleTypeDepth:
		return "depth"
	case gputypes.TextureSampleTypeSint:
		return "sint"
	case gputypes.TextureSampleTypeUint:
		return "uint"
	default:
		return "float"
	}
}

// StorageTextureAccessToJS converts a gputypes.StorageTextureAccess to JS string.
func StorageTextureAccessToJS(a gputypes.StorageTextureAccess) string {
	switch a {
	case gputypes.StorageTextureAccessWriteOnly:
		return "write-only"
	case gputypes.StorageTextureAccessReadOnly:
		return "read-only"
	case gputypes.StorageTextureAccessReadWrite:
		return "read-write"
	default:
		return "write-only"
	}
}

// LoadOpToJS converts a gputypes.LoadOp to the WebGPU JS string.
// Returns "load" for LoadOpLoad, "clear" for LoadOpClear, and "load" as default.
func LoadOpToJS(op gputypes.LoadOp) string {
	switch op {
	case gputypes.LoadOpClear:
		return "clear"
	case gputypes.LoadOpLoad:
		return "load"
	default:
		return "load"
	}
}

// StoreOpToJS converts a gputypes.StoreOp to the WebGPU JS string.
// Returns "store" for StoreOpStore, "discard" for StoreOpDiscard, and "store" as default.
func StoreOpToJS(op gputypes.StoreOp) string {
	switch op {
	case gputypes.StoreOpDiscard:
		return "discard"
	case gputypes.StoreOpStore:
		return "store"
	default:
		return "store"
	}
}
