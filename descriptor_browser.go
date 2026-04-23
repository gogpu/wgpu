//go:build js && wasm

package wgpu

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
	Usage            BufferUsage
	MappedAtCreation bool
}

// TextureDescriptor describes texture creation parameters.
type TextureDescriptor struct {
	Label         string
	Size          Extent3D
	MipLevelCount uint32
	SampleCount   uint32
	Dimension     TextureDimension
	Format        TextureFormat
	Usage         TextureUsage
	ViewFormats   []TextureFormat
}

// TextureViewDescriptor describes texture view creation parameters.
type TextureViewDescriptor struct {
	Label           string
	Format          TextureFormat
	Dimension       TextureViewDimension
	Aspect          TextureAspect
	BaseMipLevel    uint32
	MipLevelCount   uint32
	BaseArrayLayer  uint32
	ArrayLayerCount uint32
}

// SamplerDescriptor describes sampler creation parameters.
type SamplerDescriptor struct {
	Label        string
	AddressModeU AddressMode
	AddressModeV AddressMode
	AddressModeW AddressMode
	MagFilter    FilterMode
	MinFilter    FilterMode
	MipmapFilter FilterMode
	LodMinClamp  float32
	LodMaxClamp  float32
	Compare      CompareFunction
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

// BindGroupLayoutDescriptor describes a bind group layout.
type BindGroupLayoutDescriptor struct {
	Label   string
	Entries []BindGroupLayoutEntry
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
	Compare     CompareFunction
	FailOp      StencilOperation
	DepthFailOp StencilOperation
	PassOp      StencilOperation
}

// DepthStencilState describes depth and stencil testing configuration.
type DepthStencilState struct {
	Format              TextureFormat
	DepthWriteEnabled   bool
	DepthCompare        CompareFunction
	StencilFront        StencilFaceState
	StencilBack         StencilFaceState
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
	Primitive    PrimitiveState
	DepthStencil *DepthStencilState
	Multisample  MultisampleState
	Fragment     *FragmentState
}

// VertexState describes the vertex shader stage.
type VertexState struct {
	Module     *ShaderModule
	EntryPoint string
	Buffers    []VertexBufferLayout
}

// FragmentState describes the fragment shader stage.
type FragmentState struct {
	Module     *ShaderModule
	EntryPoint string
	Targets    []ColorTargetState
}

// ComputePipelineDescriptor describes a compute pipeline.
type ComputePipelineDescriptor struct {
	Label      string
	Layout     *PipelineLayout
	Module     *ShaderModule
	EntryPoint string
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
	LoadOp        LoadOp
	StoreOp       StoreOp
	ClearValue    Color
}

// RenderPassDepthStencilAttachment describes a depth/stencil attachment.
type RenderPassDepthStencilAttachment struct {
	View              *TextureView
	DepthLoadOp       LoadOp
	DepthStoreOp      StoreOp
	DepthClearValue   float32
	DepthReadOnly     bool
	StencilLoadOp     LoadOp
	StencilStoreOp    StoreOp
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
	Format      TextureFormat
	Usage       TextureUsage
	PresentMode PresentMode
	AlphaMode   CompositeAlphaMode
}

// ImageCopyTexture describes a texture subresource and origin for write operations.
type ImageCopyTexture struct {
	Texture  *Texture
	MipLevel uint32
	Origin   Origin3D
	Aspect   TextureAspect
}

// TextureUsageTransition defines a texture usage state transition.
type TextureUsageTransition struct {
	OldUsage TextureUsage
	NewUsage TextureUsage
}

// TextureRange specifies a range of texture subresources.
type TextureRange struct {
	Aspect          TextureAspect
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

// BufferTextureCopy defines a buffer-texture copy region.
type BufferTextureCopy struct {
	BufferLayout ImageDataLayout
	TextureBase  ImageCopyTexture
	Size         Extent3D
}
