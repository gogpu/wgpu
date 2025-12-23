// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build darwin

package metal

// ID represents an Objective-C object pointer.
// This is the fundamental type for all Metal objects.
type ID uintptr

// SEL represents an Objective-C selector.
type SEL uintptr

// Class represents an Objective-C class.
type Class uintptr

// IMP represents an Objective-C implementation (function pointer).
type IMP uintptr

// BOOL represents an Objective-C boolean.
type BOOL int8

const (
	// YES is the Objective-C true value.
	YES BOOL = 1
	// NO is the Objective-C false value.
	NO BOOL = 0
)

// NSUInteger is an unsigned integer type used by Cocoa APIs.
type NSUInteger uint

// NSInteger is a signed integer type used by Cocoa APIs.
type NSInteger int

// CGFloat is a floating-point type used by Core Graphics.
type CGFloat float64

// NSRange represents a range in Cocoa.
type NSRange struct {
	Location NSUInteger
	Length   NSUInteger
}

// MTLOrigin represents a 3D origin in Metal.
type MTLOrigin struct {
	X, Y, Z NSUInteger
}

// MTLSize represents a 3D size in Metal.
type MTLSize struct {
	Width, Height, Depth NSUInteger
}

// MTLRegion represents a 3D region in Metal.
type MTLRegion struct {
	Origin MTLOrigin
	Size   MTLSize
}

// CGSize represents a 2D size in Core Graphics.
type CGSize struct {
	Width, Height CGFloat
}

// MTLClearColor represents an RGBA clear color.
type MTLClearColor struct {
	Red, Green, Blue, Alpha float64
}

// MTLViewport represents a viewport transformation.
type MTLViewport struct {
	OriginX, OriginY float64
	Width, Height    float64
	ZNear, ZFar      float64
}

// MTLScissorRect represents a scissor rectangle.
type MTLScissorRect struct {
	X, Y          NSUInteger
	Width, Height NSUInteger
}

// MTLPixelFormat represents a pixel format.
type MTLPixelFormat NSUInteger

// Pixel format constants.
const (
	MTLPixelFormatInvalid              MTLPixelFormat = 0
	MTLPixelFormatA8Unorm              MTLPixelFormat = 1
	MTLPixelFormatR8Unorm              MTLPixelFormat = 10
	MTLPixelFormatR8Snorm              MTLPixelFormat = 12
	MTLPixelFormatR8Uint               MTLPixelFormat = 13
	MTLPixelFormatR8Sint               MTLPixelFormat = 14
	MTLPixelFormatR16Unorm             MTLPixelFormat = 20
	MTLPixelFormatR16Snorm             MTLPixelFormat = 22
	MTLPixelFormatR16Uint              MTLPixelFormat = 23
	MTLPixelFormatR16Sint              MTLPixelFormat = 24
	MTLPixelFormatR16Float             MTLPixelFormat = 25
	MTLPixelFormatRG8Unorm             MTLPixelFormat = 30
	MTLPixelFormatRG8Snorm             MTLPixelFormat = 32
	MTLPixelFormatRG8Uint              MTLPixelFormat = 33
	MTLPixelFormatRG8Sint              MTLPixelFormat = 34
	MTLPixelFormatR32Uint              MTLPixelFormat = 53
	MTLPixelFormatR32Sint              MTLPixelFormat = 54
	MTLPixelFormatR32Float             MTLPixelFormat = 55
	MTLPixelFormatRG16Unorm            MTLPixelFormat = 60
	MTLPixelFormatRG16Snorm            MTLPixelFormat = 62
	MTLPixelFormatRG16Uint             MTLPixelFormat = 63
	MTLPixelFormatRG16Sint             MTLPixelFormat = 64
	MTLPixelFormatRG16Float            MTLPixelFormat = 65
	MTLPixelFormatRGBA8Unorm           MTLPixelFormat = 70
	MTLPixelFormatRGBA8UnormSRGB       MTLPixelFormat = 71
	MTLPixelFormatRGBA8Snorm           MTLPixelFormat = 72
	MTLPixelFormatRGBA8Uint            MTLPixelFormat = 73
	MTLPixelFormatRGBA8Sint            MTLPixelFormat = 74
	MTLPixelFormatBGRA8Unorm           MTLPixelFormat = 80
	MTLPixelFormatBGRA8UnormSRGB       MTLPixelFormat = 81
	MTLPixelFormatRGB10A2Unorm         MTLPixelFormat = 90
	MTLPixelFormatRGB10A2Uint          MTLPixelFormat = 91
	MTLPixelFormatRG11B10Float         MTLPixelFormat = 92
	MTLPixelFormatRGB9E5Float          MTLPixelFormat = 93
	MTLPixelFormatRG32Uint             MTLPixelFormat = 103
	MTLPixelFormatRG32Sint             MTLPixelFormat = 104
	MTLPixelFormatRG32Float            MTLPixelFormat = 105
	MTLPixelFormatRGBA16Unorm          MTLPixelFormat = 110
	MTLPixelFormatRGBA16Snorm          MTLPixelFormat = 112
	MTLPixelFormatRGBA16Uint           MTLPixelFormat = 113
	MTLPixelFormatRGBA16Sint           MTLPixelFormat = 114
	MTLPixelFormatRGBA16Float          MTLPixelFormat = 115
	MTLPixelFormatRGBA32Uint           MTLPixelFormat = 123
	MTLPixelFormatRGBA32Sint           MTLPixelFormat = 124
	MTLPixelFormatRGBA32Float          MTLPixelFormat = 125
	MTLPixelFormatDepth16Unorm         MTLPixelFormat = 250
	MTLPixelFormatDepth32Float         MTLPixelFormat = 252
	MTLPixelFormatStencil8             MTLPixelFormat = 253
	MTLPixelFormatDepth24UnormStencil8 MTLPixelFormat = 255
	MTLPixelFormatDepth32FloatStencil8 MTLPixelFormat = 260
	MTLPixelFormatX32Stencil8          MTLPixelFormat = 261
	MTLPixelFormatX24Stencil8          MTLPixelFormat = 262
)

// MTLResourceOptions represents resource storage and cache options.
type MTLResourceOptions NSUInteger

// Resource option constants.
const (
	MTLResourceCPUCacheModeDefaultCache  MTLResourceOptions = 0
	MTLResourceCPUCacheModeWriteCombined MTLResourceOptions = 1 << 0

	MTLResourceStorageModeShared     MTLResourceOptions = 0 << 4
	MTLResourceStorageModeManaged    MTLResourceOptions = 1 << 4
	MTLResourceStorageModePrivate    MTLResourceOptions = 2 << 4
	MTLResourceStorageModeMemoryless MTLResourceOptions = 3 << 4

	MTLResourceHazardTrackingModeDefault   MTLResourceOptions = 0 << 8
	MTLResourceHazardTrackingModeUntracked MTLResourceOptions = 1 << 8
	MTLResourceHazardTrackingModeTracked   MTLResourceOptions = 2 << 8
)

// MTLStorageMode represents storage mode for resources.
type MTLStorageMode NSUInteger

const (
	MTLStorageModeShared     MTLStorageMode = 0
	MTLStorageModeManaged    MTLStorageMode = 1
	MTLStorageModePrivate    MTLStorageMode = 2
	MTLStorageModeMemoryless MTLStorageMode = 3
)

// MTLCPUCacheMode represents CPU cache mode for resources.
type MTLCPUCacheMode NSUInteger

const (
	MTLCPUCacheModeDefaultCache  MTLCPUCacheMode = 0
	MTLCPUCacheModeWriteCombined MTLCPUCacheMode = 1
)

// MTLTextureType represents texture types.
type MTLTextureType NSUInteger

const (
	MTLTextureType1D            MTLTextureType = 0
	MTLTextureType1DArray       MTLTextureType = 1
	MTLTextureType2D            MTLTextureType = 2
	MTLTextureType2DArray       MTLTextureType = 3
	MTLTextureType2DMultisample MTLTextureType = 4
	MTLTextureTypeCube          MTLTextureType = 5
	MTLTextureTypeCubeArray     MTLTextureType = 6
	MTLTextureType3D            MTLTextureType = 7
)

// MTLTextureUsage represents texture usage flags.
type MTLTextureUsage NSUInteger

const (
	MTLTextureUsageUnknown         MTLTextureUsage = 0
	MTLTextureUsageShaderRead      MTLTextureUsage = 1 << 0
	MTLTextureUsageShaderWrite     MTLTextureUsage = 1 << 1
	MTLTextureUsageRenderTarget    MTLTextureUsage = 1 << 2
	MTLTextureUsagePixelFormatView MTLTextureUsage = 1 << 4
	MTLTextureUsageShaderAtomic    MTLTextureUsage = 1 << 5
)

// MTLSamplerMinMagFilter represents sampler filter modes.
type MTLSamplerMinMagFilter NSUInteger

const (
	MTLSamplerMinMagFilterNearest MTLSamplerMinMagFilter = 0
	MTLSamplerMinMagFilterLinear  MTLSamplerMinMagFilter = 1
)

// MTLSamplerMipFilter represents sampler mip filter modes.
type MTLSamplerMipFilter NSUInteger

const (
	MTLSamplerMipFilterNotMipmapped MTLSamplerMipFilter = 0
	MTLSamplerMipFilterNearest      MTLSamplerMipFilter = 1
	MTLSamplerMipFilterLinear       MTLSamplerMipFilter = 2
)

// MTLSamplerAddressMode represents sampler address modes.
type MTLSamplerAddressMode NSUInteger

const (
	MTLSamplerAddressModeClampToEdge        MTLSamplerAddressMode = 0
	MTLSamplerAddressModeMirrorClampToEdge  MTLSamplerAddressMode = 1
	MTLSamplerAddressModeRepeat             MTLSamplerAddressMode = 2
	MTLSamplerAddressModeMirrorRepeat       MTLSamplerAddressMode = 3
	MTLSamplerAddressModeClampToZero        MTLSamplerAddressMode = 4
	MTLSamplerAddressModeClampToBorderColor MTLSamplerAddressMode = 5
)

// MTLSamplerBorderColor represents border colors for clamped samplers.
type MTLSamplerBorderColor NSUInteger

const (
	MTLSamplerBorderColorTransparentBlack MTLSamplerBorderColor = 0
	MTLSamplerBorderColorOpaqueBlack      MTLSamplerBorderColor = 1
	MTLSamplerBorderColorOpaqueWhite      MTLSamplerBorderColor = 2
)

// MTLCompareFunction represents comparison functions.
type MTLCompareFunction NSUInteger

const (
	MTLCompareFunctionNever        MTLCompareFunction = 0
	MTLCompareFunctionLess         MTLCompareFunction = 1
	MTLCompareFunctionEqual        MTLCompareFunction = 2
	MTLCompareFunctionLessEqual    MTLCompareFunction = 3
	MTLCompareFunctionGreater      MTLCompareFunction = 4
	MTLCompareFunctionNotEqual     MTLCompareFunction = 5
	MTLCompareFunctionGreaterEqual MTLCompareFunction = 6
	MTLCompareFunctionAlways       MTLCompareFunction = 7
)

// MTLStencilOperation represents stencil operations.
type MTLStencilOperation NSUInteger

const (
	MTLStencilOperationKeep           MTLStencilOperation = 0
	MTLStencilOperationZero           MTLStencilOperation = 1
	MTLStencilOperationReplace        MTLStencilOperation = 2
	MTLStencilOperationIncrementClamp MTLStencilOperation = 3
	MTLStencilOperationDecrementClamp MTLStencilOperation = 4
	MTLStencilOperationInvert         MTLStencilOperation = 5
	MTLStencilOperationIncrementWrap  MTLStencilOperation = 6
	MTLStencilOperationDecrementWrap  MTLStencilOperation = 7
)

// MTLPrimitiveType represents primitive types for drawing.
type MTLPrimitiveType NSUInteger

const (
	MTLPrimitiveTypePoint         MTLPrimitiveType = 0
	MTLPrimitiveTypeLine          MTLPrimitiveType = 1
	MTLPrimitiveTypeLineStrip     MTLPrimitiveType = 2
	MTLPrimitiveTypeTriangle      MTLPrimitiveType = 3
	MTLPrimitiveTypeTriangleStrip MTLPrimitiveType = 4
)

// MTLIndexType represents index buffer types.
type MTLIndexType NSUInteger

const (
	MTLIndexTypeUInt16 MTLIndexType = 0
	MTLIndexTypeUInt32 MTLIndexType = 1
)

// MTLCullMode represents face culling modes.
type MTLCullMode NSUInteger

const (
	MTLCullModeNone  MTLCullMode = 0
	MTLCullModeFront MTLCullMode = 1
	MTLCullModeBack  MTLCullMode = 2
)

// MTLWinding represents winding order for front-facing primitives.
type MTLWinding NSUInteger

const (
	MTLWindingClockwise        MTLWinding = 0
	MTLWindingCounterClockwise MTLWinding = 1
)

// MTLTriangleFillMode represents triangle fill modes.
type MTLTriangleFillMode NSUInteger

const (
	MTLTriangleFillModeFill  MTLTriangleFillMode = 0
	MTLTriangleFillModeLines MTLTriangleFillMode = 1
)

// MTLDepthClipMode represents depth clipping modes.
type MTLDepthClipMode NSUInteger

const (
	MTLDepthClipModeClip  MTLDepthClipMode = 0
	MTLDepthClipModeClamp MTLDepthClipMode = 1
)

// MTLBlendFactor represents blend factors.
type MTLBlendFactor NSUInteger

const (
	MTLBlendFactorZero                     MTLBlendFactor = 0
	MTLBlendFactorOne                      MTLBlendFactor = 1
	MTLBlendFactorSourceColor              MTLBlendFactor = 2
	MTLBlendFactorOneMinusSourceColor      MTLBlendFactor = 3
	MTLBlendFactorSourceAlpha              MTLBlendFactor = 4
	MTLBlendFactorOneMinusSourceAlpha      MTLBlendFactor = 5
	MTLBlendFactorDestinationColor         MTLBlendFactor = 6
	MTLBlendFactorOneMinusDestinationColor MTLBlendFactor = 7
	MTLBlendFactorDestinationAlpha         MTLBlendFactor = 8
	MTLBlendFactorOneMinusDestinationAlpha MTLBlendFactor = 9
	MTLBlendFactorSourceAlphaSaturated     MTLBlendFactor = 10
	MTLBlendFactorBlendColor               MTLBlendFactor = 11
	MTLBlendFactorOneMinusBlendColor       MTLBlendFactor = 12
	MTLBlendFactorBlendAlpha               MTLBlendFactor = 13
	MTLBlendFactorOneMinusBlendAlpha       MTLBlendFactor = 14
)

// MTLBlendOperation represents blend operations.
type MTLBlendOperation NSUInteger

const (
	MTLBlendOperationAdd             MTLBlendOperation = 0
	MTLBlendOperationSubtract        MTLBlendOperation = 1
	MTLBlendOperationReverseSubtract MTLBlendOperation = 2
	MTLBlendOperationMin             MTLBlendOperation = 3
	MTLBlendOperationMax             MTLBlendOperation = 4
)

// MTLColorWriteMask represents color channel write masks.
type MTLColorWriteMask NSUInteger

const (
	MTLColorWriteMaskNone  MTLColorWriteMask = 0
	MTLColorWriteMaskRed   MTLColorWriteMask = 1 << 3
	MTLColorWriteMaskGreen MTLColorWriteMask = 1 << 2
	MTLColorWriteMaskBlue  MTLColorWriteMask = 1 << 1
	MTLColorWriteMaskAlpha MTLColorWriteMask = 1 << 0
	MTLColorWriteMaskAll   MTLColorWriteMask = 0xF
)

// MTLLoadAction represents load actions for attachments.
type MTLLoadAction NSUInteger

const (
	MTLLoadActionDontCare MTLLoadAction = 0
	MTLLoadActionLoad     MTLLoadAction = 1
	MTLLoadActionClear    MTLLoadAction = 2
)

// MTLStoreAction represents store actions for attachments.
type MTLStoreAction NSUInteger

const (
	MTLStoreActionDontCare                   MTLStoreAction = 0
	MTLStoreActionStore                      MTLStoreAction = 1
	MTLStoreActionMultisampleResolve         MTLStoreAction = 2
	MTLStoreActionStoreAndMultisampleResolve MTLStoreAction = 3
	MTLStoreActionUnknown                    MTLStoreAction = 4
)

// MTLCommandBufferStatus represents command buffer status.
type MTLCommandBufferStatus NSUInteger

const (
	MTLCommandBufferStatusNotEnqueued MTLCommandBufferStatus = 0
	MTLCommandBufferStatusEnqueued    MTLCommandBufferStatus = 1
	MTLCommandBufferStatusCommitted   MTLCommandBufferStatus = 2
	MTLCommandBufferStatusScheduled   MTLCommandBufferStatus = 3
	MTLCommandBufferStatusCompleted   MTLCommandBufferStatus = 4
	MTLCommandBufferStatusError       MTLCommandBufferStatus = 5
)

// MTLLanguageVersion represents Metal Shading Language versions.
type MTLLanguageVersion NSUInteger

const (
	MTLLanguageVersion1_0 MTLLanguageVersion = (1 << 16) + 0
	MTLLanguageVersion1_1 MTLLanguageVersion = (1 << 16) + 1
	MTLLanguageVersion1_2 MTLLanguageVersion = (1 << 16) + 2
	MTLLanguageVersion2_0 MTLLanguageVersion = (2 << 16) + 0
	MTLLanguageVersion2_1 MTLLanguageVersion = (2 << 16) + 1
	MTLLanguageVersion2_2 MTLLanguageVersion = (2 << 16) + 2
	MTLLanguageVersion2_3 MTLLanguageVersion = (2 << 16) + 3
	MTLLanguageVersion2_4 MTLLanguageVersion = (2 << 16) + 4
	MTLLanguageVersion3_0 MTLLanguageVersion = (3 << 16) + 0
	MTLLanguageVersion3_1 MTLLanguageVersion = (3 << 16) + 1
)

// MTLGPUFamily represents GPU family identifiers.
type MTLGPUFamily NSInteger

const (
	MTLGPUFamilyApple1  MTLGPUFamily = 1001
	MTLGPUFamilyApple2  MTLGPUFamily = 1002
	MTLGPUFamilyApple3  MTLGPUFamily = 1003
	MTLGPUFamilyApple4  MTLGPUFamily = 1004
	MTLGPUFamilyApple5  MTLGPUFamily = 1005
	MTLGPUFamilyApple6  MTLGPUFamily = 1006
	MTLGPUFamilyApple7  MTLGPUFamily = 1007
	MTLGPUFamilyApple8  MTLGPUFamily = 1008
	MTLGPUFamilyApple9  MTLGPUFamily = 1009
	MTLGPUFamilyMac1    MTLGPUFamily = 2001
	MTLGPUFamilyMac2    MTLGPUFamily = 2002
	MTLGPUFamilyCommon1 MTLGPUFamily = 3001
	MTLGPUFamilyCommon2 MTLGPUFamily = 3002
	MTLGPUFamilyCommon3 MTLGPUFamily = 3003
	MTLGPUFamilyMetal3  MTLGPUFamily = 5001
)

// MTLVertexFormat represents vertex attribute formats.
type MTLVertexFormat NSUInteger

const (
	MTLVertexFormatInvalid MTLVertexFormat = 0
	MTLVertexFormatFloat   MTLVertexFormat = 28
	MTLVertexFormatFloat2  MTLVertexFormat = 29
	MTLVertexFormatFloat3  MTLVertexFormat = 30
	MTLVertexFormatFloat4  MTLVertexFormat = 31
	MTLVertexFormatInt     MTLVertexFormat = 32
	MTLVertexFormatInt2    MTLVertexFormat = 33
	MTLVertexFormatInt3    MTLVertexFormat = 34
	MTLVertexFormatInt4    MTLVertexFormat = 35
	MTLVertexFormatUInt    MTLVertexFormat = 36
	MTLVertexFormatUInt2   MTLVertexFormat = 37
	MTLVertexFormatUInt3   MTLVertexFormat = 38
	MTLVertexFormatUInt4   MTLVertexFormat = 39
	MTLVertexFormatHalf2   MTLVertexFormat = 25
	MTLVertexFormatHalf3   MTLVertexFormat = 26
	MTLVertexFormatHalf4   MTLVertexFormat = 27
	MTLVertexFormatUChar4  MTLVertexFormat = 3
	MTLVertexFormatChar4   MTLVertexFormat = 6
	MTLVertexFormatUShort2 MTLVertexFormat = 13
	MTLVertexFormatShort2  MTLVertexFormat = 16
)

// MTLVertexStepFunction represents vertex step functions.
type MTLVertexStepFunction NSUInteger

const (
	MTLVertexStepFunctionConstant    MTLVertexStepFunction = 0
	MTLVertexStepFunctionPerVertex   MTLVertexStepFunction = 1
	MTLVertexStepFunctionPerInstance MTLVertexStepFunction = 2
)
