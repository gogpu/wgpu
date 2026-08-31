package wgpu

import "github.com/gogpu/gputypes"

// MinBindGroups is the minimum guaranteed number of bind groups per WebGPU spec.
// All compliant devices support at least 4 bind groups. Portable code should
// use no more than MinBindGroups.
const MinBindGroups = 4

// MaxBindGroups is the HAL hard cap on bind groups (wgpu-hal MAX_BIND_GROUPS = 8).
// Devices may support up to 8, but only MinBindGroups (4) is guaranteed.
const MaxBindGroups = 8

// Backend constants
const (
	BackendVulkan = gputypes.BackendVulkan
	BackendMetal  = gputypes.BackendMetal
	BackendDX12   = gputypes.BackendDX12
	BackendGL     = gputypes.BackendGL
)

// Backends masks
const (
	BackendsAll     = gputypes.BackendsAll
	BackendsPrimary = gputypes.BackendsPrimary
	BackendsVulkan  = gputypes.BackendsVulkan
	BackendsMetal   = gputypes.BackendsMetal
	BackendsDX12    = gputypes.BackendsDX12
	BackendsGL      = gputypes.BackendsGL
)

// Buffer usage constants
const (
	BufferUsageMapRead      = gputypes.BufferUsageMapRead
	BufferUsageMapWrite     = gputypes.BufferUsageMapWrite
	BufferUsageCopySrc      = gputypes.BufferUsageCopySrc
	BufferUsageCopyDst      = gputypes.BufferUsageCopyDst
	BufferUsageIndex        = gputypes.BufferUsageIndex
	BufferUsageVertex       = gputypes.BufferUsageVertex
	BufferUsageUniform      = gputypes.BufferUsageUniform
	BufferUsageStorage      = gputypes.BufferUsageStorage
	BufferUsageIndirect     = gputypes.BufferUsageIndirect
	BufferUsageQueryResolve = gputypes.BufferUsageQueryResolve
)

// Texture usage constants
const (
	TextureUsageCopySrc          = gputypes.TextureUsageCopySrc
	TextureUsageCopyDst          = gputypes.TextureUsageCopyDst
	TextureUsageTextureBinding   = gputypes.TextureUsageTextureBinding
	TextureUsageStorageBinding   = gputypes.TextureUsageStorageBinding
	TextureUsageRenderAttachment = gputypes.TextureUsageRenderAttachment
)

// Texture dimension constants
const (
	TextureDimension1D = gputypes.TextureDimension1D
	TextureDimension2D = gputypes.TextureDimension2D
	TextureDimension3D = gputypes.TextureDimension3D
)

// Commonly used texture format constants
const (
	TextureFormatRGBA8Unorm     = gputypes.TextureFormatRGBA8Unorm
	TextureFormatRGBA8UnormSrgb = gputypes.TextureFormatRGBA8UnormSrgb
	TextureFormatBGRA8Unorm     = gputypes.TextureFormatBGRA8Unorm
	TextureFormatBGRA8UnormSrgb = gputypes.TextureFormatBGRA8UnormSrgb
	TextureFormatDepth24Plus    = gputypes.TextureFormatDepth24Plus
	TextureFormatDepth32Float   = gputypes.TextureFormatDepth32Float
)

// Shader stage constants
const (
	ShaderStageVertex   = gputypes.ShaderStageVertex
	ShaderStageFragment = gputypes.ShaderStageFragment
	ShaderStageCompute  = gputypes.ShaderStageCompute
)

// Present mode constants
const (
	PresentModeImmediate   = gputypes.PresentModeImmediate
	PresentModeMailbox     = gputypes.PresentModeMailbox
	PresentModeFifo        = gputypes.PresentModeFifo
	PresentModeFifoRelaxed = gputypes.PresentModeFifoRelaxed
)

// Power preference constants
const (
	PowerPreferenceNone            = gputypes.PowerPreferenceNone
	PowerPreferenceLowPower        = gputypes.PowerPreferenceLowPower
	PowerPreferenceHighPerformance = gputypes.PowerPreferenceHighPerformance
)

// RequestAdapterOptions controls adapter selection.
//
// Following the WebGPU spec, CompatibleSurface is a typed *Surface pointer
// (not a raw handle). Backends that require a surface for adapter enumeration
// (e.g., GLES/OpenGL which needs a GL context) use this to perform deferred
// enumeration when RequestAdapter is called.
type RequestAdapterOptions struct {
	// PowerPreference indicates power consumption preference.
	PowerPreference gputypes.PowerPreference
	// ForceFallbackAdapter forces the use of a fallback (software) adapter.
	ForceFallbackAdapter bool
	// CompatibleSurface, if non-nil, indicates that the adapter must support
	// rendering to this surface. For GLES backends, this triggers deferred
	// adapter enumeration using the surface's GL context.
	CompatibleSurface *Surface
}

// Default functions (re-exported for convenience)
var (
	DefaultLimits             = gputypes.DefaultLimits
	DefaultInstanceDescriptor = gputypes.DefaultInstanceDescriptor
)
