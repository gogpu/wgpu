package wgpu

import "github.com/gogpu/gputypes"

// Backend types
type Backend = gputypes.Backend
type Backends = gputypes.Backends

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

// Feature and limit types
type Features = gputypes.Features
type Limits = gputypes.Limits

// Buffer usage
type BufferUsage = gputypes.BufferUsage

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

// Texture types
type TextureUsage = gputypes.TextureUsage

const (
	TextureUsageCopySrc          = gputypes.TextureUsageCopySrc
	TextureUsageCopyDst          = gputypes.TextureUsageCopyDst
	TextureUsageTextureBinding   = gputypes.TextureUsageTextureBinding
	TextureUsageStorageBinding   = gputypes.TextureUsageStorageBinding
	TextureUsageRenderAttachment = gputypes.TextureUsageRenderAttachment
)

type TextureFormat = gputypes.TextureFormat
type TextureDimension = gputypes.TextureDimension
type TextureViewDimension = gputypes.TextureViewDimension
type TextureAspect = gputypes.TextureAspect

// Commonly used texture format constants
const (
	TextureFormatRGBA8Unorm     = gputypes.TextureFormatRGBA8Unorm
	TextureFormatRGBA8UnormSrgb = gputypes.TextureFormatRGBA8UnormSrgb
	TextureFormatBGRA8Unorm     = gputypes.TextureFormatBGRA8Unorm
	TextureFormatBGRA8UnormSrgb = gputypes.TextureFormatBGRA8UnormSrgb
	TextureFormatDepth24Plus    = gputypes.TextureFormatDepth24Plus
	TextureFormatDepth32Float   = gputypes.TextureFormatDepth32Float
)

// Shader types
type ShaderStages = gputypes.ShaderStages

const (
	ShaderStageVertex   = gputypes.ShaderStageVertex
	ShaderStageFragment = gputypes.ShaderStageFragment
	ShaderStageCompute  = gputypes.ShaderStageCompute
)

// Primitive types
type PrimitiveTopology = gputypes.PrimitiveTopology
type IndexFormat = gputypes.IndexFormat
type FrontFace = gputypes.FrontFace
type CullMode = gputypes.CullMode

type PrimitiveState = gputypes.PrimitiveState
type MultisampleState = gputypes.MultisampleState

// Render types
type LoadOp = gputypes.LoadOp
type StoreOp = gputypes.StoreOp
type Color = gputypes.Color

// Bind group types
type BindGroupLayoutEntry = gputypes.BindGroupLayoutEntry
type VertexBufferLayout = gputypes.VertexBufferLayout
type ColorTargetState = gputypes.ColorTargetState

// Sampler types
type AddressMode = gputypes.AddressMode
type FilterMode = gputypes.FilterMode
type CompareFunction = gputypes.CompareFunction

// Surface/presentation types
type PresentMode = gputypes.PresentMode
type CompositeAlphaMode = gputypes.CompositeAlphaMode

const (
	PresentModeImmediate   = gputypes.PresentModeImmediate
	PresentModeMailbox     = gputypes.PresentModeMailbox
	PresentModeFifo        = gputypes.PresentModeFifo
	PresentModeFifoRelaxed = gputypes.PresentModeFifoRelaxed
)

// Adapter types
type AdapterInfo = gputypes.AdapterInfo
type DeviceType = gputypes.DeviceType
type PowerPreference = gputypes.PowerPreference
type RequestAdapterOptions = gputypes.RequestAdapterOptions

const (
	PowerPreferenceNone            = gputypes.PowerPreferenceNone
	PowerPreferenceLowPower        = gputypes.PowerPreferenceLowPower
	PowerPreferenceHighPerformance = gputypes.PowerPreferenceHighPerformance
)

// Default functions (re-exported for convenience)
var (
	DefaultLimits             = gputypes.DefaultLimits
	DefaultInstanceDescriptor = gputypes.DefaultInstanceDescriptor
)
