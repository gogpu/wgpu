//go:build rust

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
	LodMinClamp  float32
	LodMaxClamp  float32
	Compare      gputypes.CompareFunction
	Anisotropy   uint16
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

// ComputePassDescriptor describes compute pass creation.
type ComputePassDescriptor struct {
	Label string
}

// SurfaceConfiguration configures surface presentation.
type SurfaceConfiguration struct {
	Width       uint32
	Height      uint32
	Format      gputypes.TextureFormat
	Usage       gputypes.TextureUsage
	PresentMode gputypes.PresentMode
	AlphaMode   gputypes.CompositeAlphaMode
}

// StencilOperation describes a stencil operation.
type StencilOperation uint32

// Stencil operation constants.
const (
	StencilOperationKeep           StencilOperation = 0
	StencilOperationZero           StencilOperation = 1
	StencilOperationReplace        StencilOperation = 2
	StencilOperationInvert         StencilOperation = 3
	StencilOperationIncrementClamp StencilOperation = 4
	StencilOperationDecrementClamp StencilOperation = 5
	StencilOperationIncrementWrap  StencilOperation = 6
	StencilOperationDecrementWrap  StencilOperation = 7
)

// StencilFaceState describes stencil operations for a face.
type StencilFaceState struct {
	Compare     gputypes.CompareFunction
	FailOp      StencilOperation
	DepthFailOp StencilOperation
	PassOp      StencilOperation
}

// DepthStencilState describes depth and stencil testing configuration.
type DepthStencilState struct {
	Format              gputypes.TextureFormat
	DepthWriteEnabled   bool
	DepthCompare        gputypes.CompareFunction
	StencilFront        StencilFaceState
	StencilBack         StencilFaceState
	StencilReadMask     uint32
	StencilWriteMask    uint32
	DepthBias           int32
	DepthBiasSlopeScale float32
	DepthBiasClamp      float32
}

// RenderPassDescriptor describes a render pass.
type RenderPassDescriptor struct {
	Label                  string
	ColorAttachments       []RenderPassColorAttachment
	DepthStencilAttachment *RenderPassDepthStencilAttachment
}

// RenderPassColorAttachment describes a color attachment for a render pass.
type RenderPassColorAttachment struct {
	View          *TextureView
	ResolveTarget *TextureView
	LoadOp        gputypes.LoadOp
	StoreOp       gputypes.StoreOp
	ClearValue    gputypes.Color
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

// BindGroupLayoutDescriptor describes a bind group layout.
type BindGroupLayoutDescriptor struct {
	Label   string
	Entries []gputypes.BindGroupLayoutEntry
}

// PipelineLayoutDescriptor describes a pipeline layout.
type PipelineLayoutDescriptor struct {
	Label            string
	BindGroupLayouts []*BindGroupLayout
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
	Targets    []gputypes.ColorTargetState
}

// ComputePipelineDescriptor describes a compute pipeline.
type ComputePipelineDescriptor struct {
	Label      string
	Layout     *PipelineLayout
	Module     *ShaderModule
	EntryPoint string
}

// ImageCopyTexture specifies a texture subresource for copy operations.
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
