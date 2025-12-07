package types

// AddressMode describes texture coordinate addressing.
type AddressMode uint8

const (
	// AddressModeClampToEdge clamps to the edge texel.
	AddressModeClampToEdge AddressMode = iota
	// AddressModeRepeat repeats the texture.
	AddressModeRepeat
	// AddressModeMirrorRepeat repeats with mirroring.
	AddressModeMirrorRepeat
)

// FilterMode describes texture filtering.
type FilterMode uint8

const (
	// FilterModeNearest uses nearest-neighbor filtering.
	FilterModeNearest FilterMode = iota
	// FilterModeLinear uses linear interpolation.
	FilterModeLinear
)

// MipmapFilterMode describes mipmap filtering.
type MipmapFilterMode uint8

const (
	// MipmapFilterModeNearest selects the nearest mip level.
	MipmapFilterModeNearest MipmapFilterMode = iota
	// MipmapFilterModeLinear interpolates between mip levels.
	MipmapFilterModeLinear
)

// CompareFunction describes a comparison function.
type CompareFunction uint8

const (
	// CompareFunctionUndefined is undefined (no comparison).
	CompareFunctionUndefined CompareFunction = iota
	// CompareFunctionNever always fails.
	CompareFunctionNever
	// CompareFunctionLess passes if source < destination.
	CompareFunctionLess
	// CompareFunctionEqual passes if source == destination.
	CompareFunctionEqual
	// CompareFunctionLessEqual passes if source <= destination.
	CompareFunctionLessEqual
	// CompareFunctionGreater passes if source > destination.
	CompareFunctionGreater
	// CompareFunctionNotEqual passes if source != destination.
	CompareFunctionNotEqual
	// CompareFunctionGreaterEqual passes if source >= destination.
	CompareFunctionGreaterEqual
	// CompareFunctionAlways always passes.
	CompareFunctionAlways
)

// SamplerDescriptor describes a sampler.
type SamplerDescriptor struct {
	// Label is a debug label.
	Label string
	// AddressModeU is the U coordinate addressing mode.
	AddressModeU AddressMode
	// AddressModeV is the V coordinate addressing mode.
	AddressModeV AddressMode
	// AddressModeW is the W coordinate addressing mode.
	AddressModeW AddressMode
	// MagFilter is the magnification filter.
	MagFilter FilterMode
	// MinFilter is the minification filter.
	MinFilter FilterMode
	// MipmapFilter is the mipmap filter.
	MipmapFilter MipmapFilterMode
	// LodMinClamp is the minimum level of detail.
	LodMinClamp float32
	// LodMaxClamp is the maximum level of detail.
	LodMaxClamp float32
	// Compare is the comparison function for depth sampling.
	Compare CompareFunction
	// MaxAnisotropy is the max anisotropy level (1-16).
	MaxAnisotropy uint16
}

// DefaultSamplerDescriptor returns the default sampler descriptor.
func DefaultSamplerDescriptor() SamplerDescriptor {
	return SamplerDescriptor{
		AddressModeU:  AddressModeClampToEdge,
		AddressModeV:  AddressModeClampToEdge,
		AddressModeW:  AddressModeClampToEdge,
		MagFilter:     FilterModeNearest,
		MinFilter:     FilterModeNearest,
		MipmapFilter:  MipmapFilterModeNearest,
		LodMinClamp:   0.0,
		LodMaxClamp:   32.0,
		Compare:       CompareFunctionUndefined,
		MaxAnisotropy: 1,
	}
}

// SamplerBindingType describes how a sampler is bound.
type SamplerBindingType uint8

const (
	// SamplerBindingTypeUndefined is undefined.
	SamplerBindingTypeUndefined SamplerBindingType = iota
	// SamplerBindingTypeFiltering supports filtering.
	SamplerBindingTypeFiltering
	// SamplerBindingTypeNonFiltering does not filter.
	SamplerBindingTypeNonFiltering
	// SamplerBindingTypeComparison is for depth comparison.
	SamplerBindingTypeComparison
)
