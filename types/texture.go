package types

// TextureFormat describes the format of a texture.
type TextureFormat uint32

const (
	// TextureFormatUndefined is an undefined format.
	TextureFormatUndefined TextureFormat = iota

	// 8-bit formats
	TextureFormatR8Unorm
	TextureFormatR8Snorm
	TextureFormatR8Uint
	TextureFormatR8Sint

	// 16-bit formats
	TextureFormatR16Uint
	TextureFormatR16Sint
	TextureFormatR16Float
	TextureFormatRG8Unorm
	TextureFormatRG8Snorm
	TextureFormatRG8Uint
	TextureFormatRG8Sint

	// 32-bit formats
	TextureFormatR32Uint
	TextureFormatR32Sint
	TextureFormatR32Float
	TextureFormatRG16Uint
	TextureFormatRG16Sint
	TextureFormatRG16Float
	TextureFormatRGBA8Unorm
	TextureFormatRGBA8UnormSrgb
	TextureFormatRGBA8Snorm
	TextureFormatRGBA8Uint
	TextureFormatRGBA8Sint
	TextureFormatBGRA8Unorm
	TextureFormatBGRA8UnormSrgb

	// Packed formats
	TextureFormatRGB9E5Ufloat
	TextureFormatRGB10A2Uint
	TextureFormatRGB10A2Unorm
	TextureFormatRG11B10Ufloat

	// 64-bit formats
	TextureFormatRG32Uint
	TextureFormatRG32Sint
	TextureFormatRG32Float
	TextureFormatRGBA16Uint
	TextureFormatRGBA16Sint
	TextureFormatRGBA16Float

	// 128-bit formats
	TextureFormatRGBA32Uint
	TextureFormatRGBA32Sint
	TextureFormatRGBA32Float

	// Depth/stencil formats
	TextureFormatStencil8
	TextureFormatDepth16Unorm
	TextureFormatDepth24Plus
	TextureFormatDepth24PlusStencil8
	TextureFormatDepth32Float
	TextureFormatDepth32FloatStencil8

	// BC compressed formats
	TextureFormatBC1RGBAUnorm
	TextureFormatBC1RGBAUnormSrgb
	TextureFormatBC2RGBAUnorm
	TextureFormatBC2RGBAUnormSrgb
	TextureFormatBC3RGBAUnorm
	TextureFormatBC3RGBAUnormSrgb
	TextureFormatBC4RUnorm
	TextureFormatBC4RSnorm
	TextureFormatBC5RGUnorm
	TextureFormatBC5RGSnorm
	TextureFormatBC6HRGBUfloat
	TextureFormatBC6HRGBFloat
	TextureFormatBC7RGBAUnorm
	TextureFormatBC7RGBAUnormSrgb

	// ETC2 compressed formats
	TextureFormatETC2RGB8Unorm
	TextureFormatETC2RGB8UnormSrgb
	TextureFormatETC2RGB8A1Unorm
	TextureFormatETC2RGB8A1UnormSrgb
	TextureFormatETC2RGBA8Unorm
	TextureFormatETC2RGBA8UnormSrgb
	TextureFormatEACR11Unorm
	TextureFormatEACR11Snorm
	TextureFormatEACRG11Unorm
	TextureFormatEACRG11Snorm

	// ASTC compressed formats
	TextureFormatASTC4x4Unorm
	TextureFormatASTC4x4UnormSrgb
	TextureFormatASTC5x4Unorm
	TextureFormatASTC5x4UnormSrgb
	TextureFormatASTC5x5Unorm
	TextureFormatASTC5x5UnormSrgb
	TextureFormatASTC6x5Unorm
	TextureFormatASTC6x5UnormSrgb
	TextureFormatASTC6x6Unorm
	TextureFormatASTC6x6UnormSrgb
	TextureFormatASTC8x5Unorm
	TextureFormatASTC8x5UnormSrgb
	TextureFormatASTC8x6Unorm
	TextureFormatASTC8x6UnormSrgb
	TextureFormatASTC8x8Unorm
	TextureFormatASTC8x8UnormSrgb
	TextureFormatASTC10x5Unorm
	TextureFormatASTC10x5UnormSrgb
	TextureFormatASTC10x6Unorm
	TextureFormatASTC10x6UnormSrgb
	TextureFormatASTC10x8Unorm
	TextureFormatASTC10x8UnormSrgb
	TextureFormatASTC10x10Unorm
	TextureFormatASTC10x10UnormSrgb
	TextureFormatASTC12x10Unorm
	TextureFormatASTC12x10UnormSrgb
	TextureFormatASTC12x12Unorm
	TextureFormatASTC12x12UnormSrgb
)

// TextureDimension describes texture dimensions.
type TextureDimension uint8

const (
	// TextureDimension1D is a 1D texture.
	TextureDimension1D TextureDimension = iota
	// TextureDimension2D is a 2D texture.
	TextureDimension2D
	// TextureDimension3D is a 3D texture.
	TextureDimension3D
)

// TextureViewDimension describes a texture view dimension.
type TextureViewDimension uint8

const (
	// TextureViewDimensionUndefined uses the same dimension as the texture.
	TextureViewDimensionUndefined TextureViewDimension = iota
	// TextureViewDimension1D is a 1D texture view.
	TextureViewDimension1D
	// TextureViewDimension2D is a 2D texture view.
	TextureViewDimension2D
	// TextureViewDimension2DArray is a 2D array texture view.
	TextureViewDimension2DArray
	// TextureViewDimensionCube is a cube texture view.
	TextureViewDimensionCube
	// TextureViewDimensionCubeArray is a cube array texture view.
	TextureViewDimensionCubeArray
	// TextureViewDimension3D is a 3D texture view.
	TextureViewDimension3D
)

// TextureAspect describes which aspects of a texture to access.
type TextureAspect uint8

const (
	// TextureAspectAll accesses all aspects.
	TextureAspectAll TextureAspect = iota
	// TextureAspectStencilOnly accesses only stencil.
	TextureAspectStencilOnly
	// TextureAspectDepthOnly accesses only depth.
	TextureAspectDepthOnly
)

// TextureUsage describes how a texture can be used.
type TextureUsage uint32

const (
	// TextureUsageCopySrc allows the texture to be a copy source.
	TextureUsageCopySrc TextureUsage = 1 << iota
	// TextureUsageCopyDst allows the texture to be a copy destination.
	TextureUsageCopyDst
	// TextureUsageTextureBinding allows texture binding in shaders.
	TextureUsageTextureBinding
	// TextureUsageStorageBinding allows storage binding in shaders.
	TextureUsageStorageBinding
	// TextureUsageRenderAttachment allows use as a render attachment.
	TextureUsageRenderAttachment
)

// TextureDescriptor describes a texture.
type TextureDescriptor struct {
	// Label is a debug label.
	Label string
	// Size is the texture size.
	Size Extent3D
	// MipLevelCount is the number of mip levels.
	MipLevelCount uint32
	// SampleCount is the number of samples (1 for non-multisampled).
	SampleCount uint32
	// Dimension is the texture dimension.
	Dimension TextureDimension
	// Format is the texture format.
	Format TextureFormat
	// Usage describes how the texture will be used.
	Usage TextureUsage
	// ViewFormats lists compatible view formats.
	ViewFormats []TextureFormat
}

// TextureViewDescriptor describes a texture view.
type TextureViewDescriptor struct {
	// Label is a debug label.
	Label string
	// Format is the view format (defaults to texture format).
	Format TextureFormat
	// Dimension is the view dimension.
	Dimension TextureViewDimension
	// Aspect specifies which aspect to view.
	Aspect TextureAspect
	// BaseMipLevel is the first mip level accessible.
	BaseMipLevel uint32
	// MipLevelCount is the number of accessible mip levels.
	MipLevelCount uint32
	// BaseArrayLayer is the first array layer accessible.
	BaseArrayLayer uint32
	// ArrayLayerCount is the number of accessible array layers.
	ArrayLayerCount uint32
}

// Extent3D describes a 3D size.
type Extent3D struct {
	// Width is the size in the X dimension.
	Width uint32
	// Height is the size in the Y dimension.
	Height uint32
	// DepthOrArrayLayers is the size in Z or array layer count.
	DepthOrArrayLayers uint32
}

// Origin3D describes a 3D origin.
type Origin3D struct {
	// X is the X coordinate.
	X uint32
	// Y is the Y coordinate.
	Y uint32
	// Z is the Z coordinate.
	Z uint32
}
