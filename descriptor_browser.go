//go:build js && wasm

package wgpu

import "github.com/gogpu/gputypes"

// Extent3D is a 3D size.
type Extent3D struct {
	Width              uint32
	Height             uint32
	DepthOrArrayLayers uint32
}

// Origin3D is a 3D origin point.
type Origin3D struct {
	X uint32
	Y uint32
	Z uint32
}

// ImageDataLayout describes the layout of image data in a buffer.
type ImageDataLayout struct {
	Offset       uint64
	BytesPerRow  uint32
	RowsPerImage uint32
}

// BufferDescriptor describes buffer creation parameters.
type BufferDescriptor struct {
	Label            string
	Size             uint64
	Usage            gputypes.BufferUsage
	MappedAtCreation bool
}

// TextureDescriptor describes texture creation parameters.
type TextureDescriptor struct {
	Label         string
	Size          Extent3D
	MipLevelCount uint32
	SampleCount   uint32
	Dimension     gputypes.TextureDimension
	Format        gputypes.TextureFormat
	Usage         gputypes.TextureUsage
	ViewFormats   []gputypes.TextureFormat
}

// TextureViewDescriptor describes texture view creation parameters.
type TextureViewDescriptor struct {
	Label           string
	Format          gputypes.TextureFormat
	Dimension       gputypes.TextureViewDimension
	Aspect          gputypes.TextureAspect
	BaseMipLevel    uint32
	MipLevelCount   uint32
	BaseArrayLayer  uint32
	ArrayLayerCount uint32
}

// SamplerDescriptor describes sampler creation parameters.
type SamplerDescriptor struct {
	Label        string
	AddressModeU gputypes.AddressMode
	AddressModeV gputypes.AddressMode
	AddressModeW gputypes.AddressMode
	MagFilter    gputypes.FilterMode
	MinFilter    gputypes.FilterMode
	MipmapFilter gputypes.FilterMode
	// LodMinClamp is the lowest mip level of detail the sampler reads. Its
	// zero value is the WebGPU default 0: sampling may start at the base level.
	LodMinClamp float32
	// LodMaxClamp is the highest mip level of detail the sampler reads. The
	// zero value means the WebGPU default 32, which clamps nothing, so a
	// zero-value descriptor samples the whole mip chain on every backend.
	//
	// This deviates from the WebGPU spec and from Rust wgpu, which honor an
	// explicit 0 and sample only the base level. To sample only the base level
	// here, bind a texture view with MipLevelCount 1.
	LodMaxClamp float32
	Compare     gputypes.CompareFunction
	Anisotropy  uint16
}

// ShaderModuleDescriptor describes shader module creation parameters.
type ShaderModuleDescriptor struct {
	Label string
	WGSL  string   // WGSL source code
	SPIRV []uint32 // SPIR-V bytecode (alternative to WGSL)
}

// CommandEncoderDescriptor describes command encoder creation.
type CommandEncoderDescriptor struct {
	Label string
}

// BindGroupLayoutDescriptor describes a bind group layout.
type BindGroupLayoutDescriptor struct {
	Label   string
	Entries []gputypes.BindGroupLayoutEntry
}

// BindGroupDescriptor describes a bind group.
type BindGroupDescriptor struct {
	Label   string
	Layout  *BindGroupLayout
	Entries []BindGroupEntry
}

// BindGroupEntry describes a single resource binding in a bind group.
type BindGroupEntry struct {
	Binding     uint32
	Buffer      *Buffer
	Offset      uint64
	Size        uint64
	Sampler     *Sampler
	TextureView *TextureView
}

// PipelineLayoutDescriptor describes a pipeline layout.
type PipelineLayoutDescriptor struct {
	Label            string
	BindGroupLayouts []*BindGroupLayout
}

// Stencil operation constants (webgpu.h spec values).
const (
	StencilOperationKeep           = gputypes.StencilOperationKeep
	StencilOperationZero           = gputypes.StencilOperationZero
	StencilOperationReplace        = gputypes.StencilOperationReplace
	StencilOperationInvert         = gputypes.StencilOperationInvert
	StencilOperationIncrementClamp = gputypes.StencilOperationIncrementClamp
	StencilOperationDecrementClamp = gputypes.StencilOperationDecrementClamp
	StencilOperationIncrementWrap  = gputypes.StencilOperationIncrementWrap
	StencilOperationDecrementWrap  = gputypes.StencilOperationDecrementWrap
)

// StencilFaceState describes stencil operations for a face.
type StencilFaceState struct {
	Compare     gputypes.CompareFunction
	FailOp      gputypes.StencilOperation
	DepthFailOp gputypes.StencilOperation
	PassOp      gputypes.StencilOperation
}

// DepthStencilState describes depth and stencil testing configuration.
type DepthStencilState struct {
	Format            gputypes.TextureFormat
	DepthWriteEnabled bool
	DepthCompare      gputypes.CompareFunction
	StencilFront      StencilFaceState
	StencilBack       StencilFaceState
	// StencilReadMask and StencilWriteMask are taken literally: zero
	// compares and writes no stencil bit (the test still runs, comparing 0
	// with 0). Set 0xFFFFFFFF to use the whole stencil value; the WebGPU
	// default for an absent mask is that value, Rust wgpu's default is 0.
	StencilReadMask     uint32
	StencilWriteMask    uint32
	DepthBias           int32
	DepthBiasSlopeScale float32
	DepthBiasClamp      float32
}

// RenderPipelineDescriptor describes a render pipeline.
type RenderPipelineDescriptor struct {
	Label        string
	Layout       *PipelineLayout
	Vertex       VertexState
	Primitive    gputypes.PrimitiveState
	DepthStencil *DepthStencilState
	Multisample  gputypes.MultisampleState
	Fragment     *FragmentState
}

// VertexState describes the vertex shader stage.
type VertexState struct {
	Module     *ShaderModule
	EntryPoint string
	Buffers    []gputypes.VertexBufferLayout
}

// FragmentState describes the fragment shader stage.
type FragmentState struct {
	Module     *ShaderModule
	EntryPoint string
	// Targets describe the color attachments. A target's WriteMask is
	// taken literally: zero (gputypes.ColorWriteMaskNone) writes nothing,
	// so set gputypes.ColorWriteMaskAll to write every channel. A nil Blend
	// replaces; a zero BlendComponent is the WebGPU default (srcFactor one,
	// dstFactor zero, operation add).
	Targets []gputypes.ColorTargetState
}

// ComputePipelineDescriptor describes a compute pipeline.
type ComputePipelineDescriptor struct {
	Label      string
	Layout     *PipelineLayout
	Module     *ShaderModule
	EntryPoint string

	// Constants are pipeline-overridable constants (WebGPU spec).
	// Browser passes these to GPUDevice.createComputePipeline() constants dict.
	Constants map[string]float64

	// ZeroInitializeWorkgroupMemory controls workgroup memory zero-init.
	// Browser ignores this (WebGPU spec mandates zero-init in browser).
	ZeroInitializeWorkgroupMemory *bool
}

// RenderPassDescriptor describes a render pass.
type RenderPassDescriptor struct {
	Label                  string
	ColorAttachments       []RenderPassColorAttachment
	DepthStencilAttachment *RenderPassDepthStencilAttachment
}

// RenderPassColorAttachment describes a color attachment.
type RenderPassColorAttachment struct {
	View          *TextureView
	ResolveTarget *TextureView
	// DepthSlice is the slice of a 3D View to render to (WebGPU
	// GPURenderPassColorAttachment.depthSlice); nil means undefined. A non-nil
	// value is passed to the browser as given, which validates it.
	DepthSlice *uint32
	LoadOp     gputypes.LoadOp
	StoreOp    gputypes.StoreOp
	ClearValue gputypes.Color
}

// RenderPassDepthStencilAttachment describes a depth/stencil attachment.
type RenderPassDepthStencilAttachment struct {
	View              *TextureView
	DepthLoadOp       gputypes.LoadOp
	DepthStoreOp      gputypes.StoreOp
	DepthClearValue   float32
	DepthReadOnly     bool
	StencilLoadOp     gputypes.LoadOp
	StencilStoreOp    gputypes.StoreOp
	StencilClearValue uint32
	StencilReadOnly   bool
}

// ComputePassDescriptor describes a compute pass.
type ComputePassDescriptor struct {
	Label string
}

// SurfaceConfiguration describes surface settings.
type SurfaceConfiguration struct {
	Width       uint32
	Height      uint32
	Format      gputypes.TextureFormat
	Usage       gputypes.TextureUsage
	PresentMode gputypes.PresentMode
	AlphaMode   gputypes.CompositeAlphaMode
}

// ImageCopyTexture describes a texture subresource and origin for write operations.
type ImageCopyTexture struct {
	Texture  *Texture
	MipLevel uint32
	Origin   Origin3D
	Aspect   gputypes.TextureAspect
}

// TextureUsageTransition defines a texture usage state transition.
type TextureUsageTransition struct {
	OldUsage gputypes.TextureUsage
	NewUsage gputypes.TextureUsage
}

// TextureRange specifies a range of texture subresources.
type TextureRange struct {
	Aspect          gputypes.TextureAspect
	BaseMipLevel    uint32
	MipLevelCount   uint32
	BaseArrayLayer  uint32
	ArrayLayerCount uint32
}

// TextureBarrier defines a texture state transition for synchronization.
type TextureBarrier struct {
	Texture *Texture
	Range   TextureRange
	Usage   TextureUsageTransition
}

// TextureCopy describes a texture-to-texture copy region.
type TextureCopy struct {
	Source      ImageCopyTexture
	Destination ImageCopyTexture
	Size        Extent3D
}

// BufferTextureCopy defines a buffer-texture copy region.
type BufferTextureCopy struct {
	BufferLayout ImageDataLayout
	TextureBase  ImageCopyTexture
	Size         Extent3D
}
