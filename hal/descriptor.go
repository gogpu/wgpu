package hal

import "github.com/gogpu/gputypes"

// InstanceDescriptor describes how to create a GPU instance.
type InstanceDescriptor struct {
	// Backends specifies which backends to enable.
	Backends gputypes.Backends

	// Flags controls instance behavior (debug, validation, etc.).
	Flags gputypes.InstanceFlags

	// Dx12ShaderCompiler specifies the DX12 shader compiler (FXC or DXC).
	Dx12ShaderCompiler gputypes.Dx12ShaderCompiler

	// GLBackend specifies the OpenGL backend flavor (GL or GLES).
	GLBackend gputypes.GLBackend
}

// Capabilities contains detailed adapter capabilities.
type Capabilities struct {
	// Limits are the maximum supported limits.
	Limits gputypes.Limits

	// AlignmentsMask specifies required buffer alignment (bitmask).
	AlignmentsMask Alignments

	// DownlevelCapabilities for GL/GLES backends.
	DownlevelCapabilities DownlevelCapabilities
}

// Alignments specifies buffer alignment requirements.
type Alignments struct {
	// BufferCopyOffset is the required alignment for buffer copy offsets.
	BufferCopyOffset uint64

	// BufferCopyPitch is the required alignment for buffer copy pitch (bytes per row).
	BufferCopyPitch uint64
}

// DownlevelCapabilities describes capabilities for downlevel backends (GL/GLES).
type DownlevelCapabilities struct {
	// ShaderModel is the supported shader model (5.0, 5.1, 6.0, etc.).
	ShaderModel uint32

	// Flags are downlevel-specific capability flags.
	Flags DownlevelFlags
}

// DownlevelFlags are capability flags for downlevel backends.
type DownlevelFlags uint32

const (
	// DownlevelFlagsComputeShaders indicates compute shader support.
	DownlevelFlagsComputeShaders DownlevelFlags = 1 << iota

	// DownlevelFlagsFragmentWritableStorage indicates fragment shader writable storage support.
	DownlevelFlagsFragmentWritableStorage

	// DownlevelFlagsIndirectFirstInstance indicates DrawIndirect with firstInstance support.
	DownlevelFlagsIndirectFirstInstance

	// DownlevelFlagsBaseVertexBaseInstance indicates baseVertex/baseInstance support.
	DownlevelFlagsBaseVertexBaseInstance

	// DownlevelFlagsReadOnlyDepthStencil indicates read-only depth/stencil support.
	DownlevelFlagsReadOnlyDepthStencil

	// DownlevelFlagsAnisotropicFiltering indicates anisotropic filtering support.
	DownlevelFlagsAnisotropicFiltering
)

// TextureFormatCapabilities describes texture format capabilities.
type TextureFormatCapabilities struct {
	// Flags indicate what operations are supported for this format.
	Flags TextureFormatCapabilityFlags
}

// TextureFormatCapabilityFlags are capability flags for texture formats.
type TextureFormatCapabilityFlags uint32

const (
	// TextureFormatCapabilitySampled indicates the format can be sampled in shaders.
	TextureFormatCapabilitySampled TextureFormatCapabilityFlags = 1 << iota

	// TextureFormatCapabilityStorage indicates the format can be used for storage textures.
	TextureFormatCapabilityStorage

	// TextureFormatCapabilityStorageReadWrite indicates read-write storage support.
	TextureFormatCapabilityStorageReadWrite

	// TextureFormatCapabilityRenderAttachment indicates render target support.
	TextureFormatCapabilityRenderAttachment

	// TextureFormatCapabilityBlendable indicates blending support as render target.
	TextureFormatCapabilityBlendable

	// TextureFormatCapabilityMultisample indicates multisampling support.
	TextureFormatCapabilityMultisample

	// TextureFormatCapabilityMultisampleResolve indicates multisample resolve support.
	TextureFormatCapabilityMultisampleResolve
)

// SurfaceCapabilities describes surface capabilities.
type SurfaceCapabilities struct {
	// Formats are the supported surface texture formats.
	Formats []gputypes.TextureFormat

	// PresentModes are the supported presentation modes.
	PresentModes []gputypes.PresentMode

	// AlphaModes are the supported alpha modes.
	AlphaModes []gputypes.CompositeAlphaMode
}

// PresentMode is an alias for gputypes.PresentMode for backward compatibility.
type PresentMode = gputypes.PresentMode

// PresentMode constants for backward compatibility.
const (
	PresentModeImmediate   = gputypes.PresentModeImmediate
	PresentModeMailbox     = gputypes.PresentModeMailbox
	PresentModeFifo        = gputypes.PresentModeFifo
	PresentModeFifoRelaxed = gputypes.PresentModeFifoRelaxed
)

// CompositeAlphaMode is an alias for gputypes.CompositeAlphaMode for backward compatibility.
type CompositeAlphaMode = gputypes.CompositeAlphaMode

// CompositeAlphaMode constants for backward compatibility.
const (
	CompositeAlphaModeAuto            = gputypes.CompositeAlphaModeAuto
	CompositeAlphaModeOpaque          = gputypes.CompositeAlphaModeOpaque
	CompositeAlphaModePremultiplied   = gputypes.CompositeAlphaModePremultiplied
	CompositeAlphaModeUnpremultiplied = gputypes.CompositeAlphaModeUnpremultiplied
	CompositeAlphaModeInherit         = gputypes.CompositeAlphaModeInherit
)

// SurfaceConfiguration describes surface settings.
type SurfaceConfiguration struct {
	// Width of the surface in pixels.
	Width uint32

	// Height of the surface in pixels.
	Height uint32

	// Format is the texture format for surface textures.
	Format gputypes.TextureFormat

	// Usage specifies how surface textures will be used.
	Usage gputypes.TextureUsage

	// PresentMode controls presentation timing.
	PresentMode gputypes.PresentMode

	// AlphaMode controls alpha compositing.
	AlphaMode gputypes.CompositeAlphaMode
}

// BufferDescriptor describes how to create a buffer.
type BufferDescriptor struct {
	// Label is an optional debug name.
	Label string

	// Size in bytes.
	Size uint64

	// Usage specifies how the buffer will be used.
	Usage gputypes.BufferUsage

	// MappedAtCreation creates the buffer pre-mapped for writing.
	MappedAtCreation bool
}

// TextureDescriptor describes how to create a texture.
type TextureDescriptor struct {
	// Label is an optional debug name.
	Label string

	// Size is the texture dimensions.
	Size Extent3D

	// MipLevelCount is the number of mip levels (1+ required).
	MipLevelCount uint32

	// SampleCount is the number of samples per pixel (1 for non-MSAA).
	SampleCount uint32

	// Dimension is the texture dimension (1D, 2D, 3D).
	Dimension gputypes.TextureDimension

	// Format is the texture pixel format.
	Format gputypes.TextureFormat

	// Usage specifies how the texture will be used.
	Usage gputypes.TextureUsage

	// ViewFormats are additional formats for texture views.
	// Required for creating views with different (but compatible) formats.
	ViewFormats []gputypes.TextureFormat
}

// TextureViewDescriptor describes how to create a texture view.
type TextureViewDescriptor struct {
	// Label is an optional debug name.
	Label string

	// Format is the view format (can differ from texture format if compatible).
	// Use TextureFormatUndefined to inherit from the texture.
	Format gputypes.TextureFormat

	// Dimension is the view dimension (can differ from texture dimension).
	// Use TextureViewDimensionUndefined to inherit from the texture.
	Dimension gputypes.TextureViewDimension

	// Aspect specifies which aspect to view (color, depth, stencil).
	Aspect gputypes.TextureAspect

	// BaseMipLevel is the first mip level in the view.
	BaseMipLevel uint32

	// MipLevelCount is the number of mip levels (0 means all remaining levels).
	MipLevelCount uint32

	// BaseArrayLayer is the first array layer in the view.
	BaseArrayLayer uint32

	// ArrayLayerCount is the number of array layers (0 means all remaining layers).
	ArrayLayerCount uint32
}

// SamplerDescriptor describes how to create a sampler.
type SamplerDescriptor struct {
	// Label is an optional debug name.
	Label string

	// AddressModeU is the addressing mode for U coordinates.
	AddressModeU gputypes.AddressMode

	// AddressModeV is the addressing mode for V coordinates.
	AddressModeV gputypes.AddressMode

	// AddressModeW is the addressing mode for W coordinates.
	AddressModeW gputypes.AddressMode

	// MagFilter is the magnification filter.
	MagFilter gputypes.FilterMode

	// MinFilter is the minification filter.
	MinFilter gputypes.FilterMode

	// MipmapFilter is the mipmap filter.
	MipmapFilter gputypes.FilterMode

	// LodMinClamp is the minimum LOD clamp.
	LodMinClamp float32

	// LodMaxClamp is the maximum LOD clamp.
	LodMaxClamp float32

	// Compare is the comparison function for depth textures.
	Compare gputypes.CompareFunction

	// Anisotropy is the anisotropic filtering level (1-16, 1 is off).
	Anisotropy uint16
}

// BindGroupLayoutDescriptor describes a bind group layout.
type BindGroupLayoutDescriptor struct {
	// Label is an optional debug name.
	Label string

	// Entries define the bindings in this layout.
	Entries []gputypes.BindGroupLayoutEntry
}

// BindGroupDescriptor describes a bind group.
type BindGroupDescriptor struct {
	// Label is an optional debug name.
	Label string

	// Layout is the bind group layout.
	Layout BindGroupLayout

	// Entries are the resource bindings.
	Entries []gputypes.BindGroupEntry
}

// PipelineLayoutDescriptor describes a pipeline layout.
type PipelineLayoutDescriptor struct {
	// Label is an optional debug name.
	Label string

	// BindGroupLayouts are the bind group layouts used by the pipeline.
	BindGroupLayouts []BindGroupLayout

	// PushConstantRanges define push constant ranges (Vulkan-specific).
	PushConstantRanges []PushConstantRange
}

// PushConstantRange defines a push constant range.
type PushConstantRange struct {
	// Stages are the shader stages that can access this range.
	Stages gputypes.ShaderStages

	// Range is the byte range of the push constants.
	Range Range
}

// Range is a byte range.
type Range struct {
	Start uint32
	End   uint32
}

// ShaderModuleDescriptor describes a shader module.
type ShaderModuleDescriptor struct {
	// Label is an optional debug name.
	Label string

	// Source is the shader source code.
	// Can be WGSL source code or SPIR-V bytecode.
	Source ShaderSource
}

// ShaderSource represents shader source code or bytecode.
type ShaderSource struct {
	// WGSL is the WGSL source code (if present).
	WGSL string

	// SPIRV is the SPIR-V bytecode (if present).
	SPIRV []uint32
}

// RenderPipelineDescriptor describes a render pipeline.
type RenderPipelineDescriptor struct {
	// Label is an optional debug name.
	Label string

	// Layout is the pipeline layout.
	Layout PipelineLayout

	// Vertex is the vertex stage.
	Vertex VertexState

	// Primitive is the primitive assembly state.
	Primitive gputypes.PrimitiveState

	// DepthStencil is the depth/stencil state (optional).
	DepthStencil *DepthStencilState

	// Multisample is the multisample state.
	Multisample gputypes.MultisampleState

	// Fragment is the fragment stage (optional for depth-only passes).
	Fragment *FragmentState
}

// VertexState describes the vertex shader stage.
type VertexState struct {
	// Module is the shader module.
	Module ShaderModule

	// EntryPoint is the shader entry point function name.
	EntryPoint string

	// Buffers describe the vertex buffer layouts.
	Buffers []gputypes.VertexBufferLayout
}

// FragmentState describes the fragment shader stage.
type FragmentState struct {
	// Module is the shader module.
	Module ShaderModule

	// EntryPoint is the shader entry point function name.
	EntryPoint string

	// Targets describe the render target formats and blend state.
	Targets []gputypes.ColorTargetState
}

// ComputePipelineDescriptor describes a compute pipeline.
type ComputePipelineDescriptor struct {
	// Label is an optional debug name.
	Label string

	// Layout is the pipeline layout.
	Layout PipelineLayout

	// Compute is the compute shader stage.
	Compute ComputeState
}

// ComputeState describes the compute shader stage.
type ComputeState struct {
	// Module is the shader module.
	Module ShaderModule

	// EntryPoint is the shader entry point function name.
	EntryPoint string
}

// CommandEncoderDescriptor describes a command encoder.
type CommandEncoderDescriptor struct {
	// Label is an optional debug name.
	Label string
}

// RenderBundleEncoderDescriptor describes a render bundle encoder.
type RenderBundleEncoderDescriptor struct {
	// Label is an optional debug name.
	Label string

	// ColorFormats are the formats of the color attachments the bundle will render to.
	ColorFormats []gputypes.TextureFormat

	// DepthStencilFormat is the format of the depth/stencil attachment.
	// Use TextureFormatUndefined if no depth/stencil attachment is used.
	DepthStencilFormat gputypes.TextureFormat

	// SampleCount is the number of samples for multisampling.
	// Use 1 for no multisampling.
	SampleCount uint32

	// DepthReadOnly indicates the depth attachment is read-only during the render pass.
	DepthReadOnly bool

	// StencilReadOnly indicates the stencil attachment is read-only during the render pass.
	StencilReadOnly bool
}

// RenderPassDescriptor describes a render pass.
type RenderPassDescriptor struct {
	// Label is an optional debug name.
	Label string

	// ColorAttachments are the color render targets.
	ColorAttachments []RenderPassColorAttachment

	// DepthStencilAttachment is the depth/stencil target (optional).
	DepthStencilAttachment *RenderPassDepthStencilAttachment

	// TimestampWrites are timestamp queries (optional).
	TimestampWrites *RenderPassTimestampWrites
}

// RenderPassColorAttachment describes a color attachment.
type RenderPassColorAttachment struct {
	// View is the texture view to render to.
	View TextureView

	// ResolveTarget is the MSAA resolve target (optional).
	ResolveTarget TextureView

	// LoadOp specifies what to do at pass start.
	LoadOp gputypes.LoadOp

	// StoreOp specifies what to do at pass end.
	StoreOp gputypes.StoreOp

	// ClearValue is the clear color (used if LoadOp is Clear).
	ClearValue gputypes.Color
}

// RenderPassDepthStencilAttachment describes a depth/stencil attachment.
type RenderPassDepthStencilAttachment struct {
	// View is the texture view to use.
	View TextureView

	// DepthLoadOp specifies what to do with depth at pass start.
	DepthLoadOp gputypes.LoadOp

	// DepthStoreOp specifies what to do with depth at pass end.
	DepthStoreOp gputypes.StoreOp

	// DepthClearValue is the depth clear value (used if DepthLoadOp is Clear).
	DepthClearValue float32

	// DepthReadOnly makes the depth aspect read-only.
	DepthReadOnly bool

	// StencilLoadOp specifies what to do with stencil at pass start.
	StencilLoadOp gputypes.LoadOp

	// StencilStoreOp specifies what to do with stencil at pass end.
	StencilStoreOp gputypes.StoreOp

	// StencilClearValue is the stencil clear value (used if StencilLoadOp is Clear).
	StencilClearValue uint32

	// StencilReadOnly makes the stencil aspect read-only.
	StencilReadOnly bool
}

// RenderPassTimestampWrites describes timestamp query writes.
type RenderPassTimestampWrites struct {
	// QuerySet is the query set to write to.
	QuerySet QuerySet

	// BeginningOfPassWriteIndex is the query index for pass start.
	// Use nil to skip.
	BeginningOfPassWriteIndex *uint32

	// EndOfPassWriteIndex is the query index for pass end.
	// Use nil to skip.
	EndOfPassWriteIndex *uint32
}

// QueryType specifies the type of queries in a query set.
type QueryType uint32

const (
	// QueryTypeOcclusion counts the number of samples that pass depth/stencil tests.
	QueryTypeOcclusion QueryType = iota

	// QueryTypeTimestamp writes GPU timestamps for profiling.
	QueryTypeTimestamp
)

// QuerySetDescriptor describes how to create a query set.
type QuerySetDescriptor struct {
	// Label is an optional debug name.
	Label string

	// Type is the type of queries in this set.
	Type QueryType

	// Count is the number of queries in the set.
	Count uint32
}

// QuerySet represents a set of queries.
type QuerySet interface {
	Resource
}

// ComputePassDescriptor describes a compute pass.
type ComputePassDescriptor struct {
	// Label is an optional debug name.
	Label string

	// TimestampWrites are timestamp queries (optional).
	TimestampWrites *ComputePassTimestampWrites
}

// ComputePassTimestampWrites describes timestamp query writes.
type ComputePassTimestampWrites struct {
	// QuerySet is the query set to write to.
	QuerySet QuerySet

	// BeginningOfPassWriteIndex is the query index for pass start.
	// Use nil to skip.
	BeginningOfPassWriteIndex *uint32

	// EndOfPassWriteIndex is the query index for pass end.
	// Use nil to skip.
	EndOfPassWriteIndex *uint32
}

// DepthStencilState describes depth and stencil testing.
type DepthStencilState struct {
	// Format is the depth/stencil texture format.
	Format gputypes.TextureFormat

	// DepthWriteEnabled enables depth writes.
	DepthWriteEnabled bool

	// DepthCompare is the depth comparison function.
	DepthCompare gputypes.CompareFunction

	// StencilFront is the stencil state for front faces.
	StencilFront StencilFaceState

	// StencilBack is the stencil state for back faces.
	StencilBack StencilFaceState

	// StencilReadMask is the stencil read mask.
	StencilReadMask uint32

	// StencilWriteMask is the stencil write mask.
	StencilWriteMask uint32

	// DepthBias is the constant depth bias.
	DepthBias int32

	// DepthBiasSlopeScale is the slope-scaled depth bias.
	DepthBiasSlopeScale float32

	// DepthBiasClamp is the maximum depth bias.
	DepthBiasClamp float32
}

// StencilFaceState describes stencil operations for a face.
type StencilFaceState struct {
	// Compare is the stencil comparison function.
	Compare gputypes.CompareFunction

	// FailOp is the operation when stencil test fails.
	FailOp StencilOperation

	// DepthFailOp is the operation when depth test fails.
	DepthFailOp StencilOperation

	// PassOp is the operation when both tests pass.
	PassOp StencilOperation
}

// StencilOperation describes a stencil operation.
type StencilOperation uint8

const (
	// StencilOperationKeep keeps the current value.
	StencilOperationKeep StencilOperation = iota

	// StencilOperationZero sets the value to zero.
	StencilOperationZero

	// StencilOperationReplace replaces with the reference value.
	StencilOperationReplace

	// StencilOperationInvert inverts the bits.
	StencilOperationInvert

	// StencilOperationIncrementClamp increments and clamps.
	StencilOperationIncrementClamp

	// StencilOperationDecrementClamp decrements and clamps.
	StencilOperationDecrementClamp

	// StencilOperationIncrementWrap increments and wraps.
	StencilOperationIncrementWrap

	// StencilOperationDecrementWrap decrements and wraps.
	StencilOperationDecrementWrap
)
