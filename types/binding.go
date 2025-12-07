package types

// BindGroupLayoutDescriptor describes a bind group layout.
type BindGroupLayoutDescriptor struct {
	// Label is a debug label.
	Label string
	// Entries are the layout entries.
	Entries []BindGroupLayoutEntry
}

// BindGroupLayoutEntry describes a single binding in a bind group layout.
type BindGroupLayoutEntry struct {
	// Binding is the binding number.
	Binding uint32
	// Visibility specifies which shader stages can access this binding.
	Visibility ShaderStages
	// Type describes the binding type (one of the following must be set).
	Buffer  *BufferBindingLayout
	Sampler *SamplerBindingLayout
	Texture *TextureBindingLayout
	Storage *StorageTextureBindingLayout
}

// BufferBindingLayout describes a buffer binding.
type BufferBindingLayout struct {
	// Type is the buffer binding type.
	Type BufferBindingType
	// HasDynamicOffset indicates if the buffer has a dynamic offset.
	HasDynamicOffset bool
	// MinBindingSize is the minimum buffer size required.
	MinBindingSize uint64
}

// SamplerBindingLayout describes a sampler binding.
type SamplerBindingLayout struct {
	// Type is the sampler binding type.
	Type SamplerBindingType
}

// TextureBindingLayout describes a texture binding.
type TextureBindingLayout struct {
	// SampleType is the texture sample type.
	SampleType TextureSampleType
	// ViewDimension is the texture view dimension.
	ViewDimension TextureViewDimension
	// Multisampled indicates if the texture is multisampled.
	Multisampled bool
}

// StorageTextureBindingLayout describes a storage texture binding.
type StorageTextureBindingLayout struct {
	// Access specifies the storage texture access mode.
	Access StorageTextureAccess
	// Format is the texture format.
	Format TextureFormat
	// ViewDimension is the texture view dimension.
	ViewDimension TextureViewDimension
}

// TextureSampleType describes the sample type of a texture.
type TextureSampleType uint8

const (
	// TextureSampleTypeFloat samples as floating-point.
	TextureSampleTypeFloat TextureSampleType = iota
	// TextureSampleTypeUnfilterableFloat samples as unfilterable float.
	TextureSampleTypeUnfilterableFloat
	// TextureSampleTypeDepth samples as depth.
	TextureSampleTypeDepth
	// TextureSampleTypeSint samples as signed integer.
	TextureSampleTypeSint
	// TextureSampleTypeUint samples as unsigned integer.
	TextureSampleTypeUint
)

// StorageTextureAccess describes storage texture access mode.
type StorageTextureAccess uint8

const (
	// StorageTextureAccessWriteOnly allows write-only access.
	StorageTextureAccessWriteOnly StorageTextureAccess = iota
	// StorageTextureAccessReadOnly allows read-only access.
	StorageTextureAccessReadOnly
	// StorageTextureAccessReadWrite allows read-write access.
	StorageTextureAccessReadWrite
)

// BindGroupDescriptor describes a bind group.
type BindGroupDescriptor struct {
	// Label is a debug label.
	Label string
	// Layout is the bind group layout.
	Layout BindGroupLayoutHandle
	// Entries are the bind group entries.
	Entries []BindGroupEntry
}

// BindGroupEntry describes a single binding in a bind group.
type BindGroupEntry struct {
	// Binding is the binding number.
	Binding uint32
	// Resource is the bound resource.
	Resource BindingResource
}

// BindingResource is a resource that can be bound.
type BindingResource interface {
	bindingResource()
}

// BufferBinding binds a buffer range.
type BufferBinding struct {
	// Buffer is the buffer handle.
	Buffer BufferHandle
	// Offset is the byte offset into the buffer.
	Offset uint64
	// Size is the byte size of the binding (0 for entire buffer).
	Size uint64
}

func (BufferBinding) bindingResource() {}

// SamplerBinding binds a sampler.
type SamplerBinding struct {
	// Sampler is the sampler handle.
	Sampler SamplerHandle
}

func (SamplerBinding) bindingResource() {}

// TextureViewBinding binds a texture view.
type TextureViewBinding struct {
	// TextureView is the texture view handle.
	TextureView TextureViewHandle
}

func (TextureViewBinding) bindingResource() {}

// Handle types for bind resources.
type (
	BindGroupLayoutHandle uint64
	BufferHandle          uint64
	SamplerHandle         uint64
	TextureViewHandle     uint64
)

// PipelineLayoutDescriptor describes a pipeline layout.
type PipelineLayoutDescriptor struct {
	// Label is a debug label.
	Label string
	// BindGroupLayouts are the bind group layouts.
	BindGroupLayouts []BindGroupLayoutHandle
	// PushConstantRanges describe push constant ranges.
	PushConstantRanges []PushConstantRange
}

// PushConstantRange describes a push constant range.
type PushConstantRange struct {
	// Stages are the shader stages that can access this range.
	Stages ShaderStages
	// Start is the start offset in bytes.
	Start uint32
	// End is the end offset in bytes.
	End uint32
}
