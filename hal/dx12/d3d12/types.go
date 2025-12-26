// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows

package d3d12

import "unsafe"

// D3D12_COMMAND_QUEUE_DESC describes a command queue.
type D3D12_COMMAND_QUEUE_DESC struct {
	Type     D3D12_COMMAND_LIST_TYPE
	Priority int32
	Flags    D3D12_COMMAND_QUEUE_FLAGS
	NodeMask uint32
}

// D3D12_HEAP_PROPERTIES describes heap properties.
type D3D12_HEAP_PROPERTIES struct {
	Type                 D3D12_HEAP_TYPE
	CPUPageProperty      D3D12_CPU_PAGE_PROPERTY
	MemoryPoolPreference D3D12_MEMORY_POOL
	CreationNodeMask     uint32
	VisibleNodeMask      uint32
}

// D3D12_HEAP_DESC describes a heap.
type D3D12_HEAP_DESC struct {
	SizeInBytes uint64
	Properties  D3D12_HEAP_PROPERTIES
	Alignment   uint64
	Flags       D3D12_HEAP_FLAGS
}

// D3D12_RESOURCE_DESC describes a resource.
type D3D12_RESOURCE_DESC struct {
	Dimension        D3D12_RESOURCE_DIMENSION
	Alignment        uint64
	Width            uint64
	Height           uint32
	DepthOrArraySize uint16
	MipLevels        uint16
	Format           DXGI_FORMAT
	SampleDesc       DXGI_SAMPLE_DESC
	Layout           D3D12_TEXTURE_LAYOUT
	Flags            D3D12_RESOURCE_FLAGS
}

// DXGI_SAMPLE_DESC describes multi-sampling parameters.
type DXGI_SAMPLE_DESC struct {
	Count   uint32
	Quality uint32
}

// D3D12_RESOURCE_ALLOCATION_INFO describes resource allocation information.
type D3D12_RESOURCE_ALLOCATION_INFO struct {
	SizeInBytes uint64
	Alignment   uint64
}

// D3D12_CLEAR_VALUE describes an optimized clear value.
type D3D12_CLEAR_VALUE struct {
	Format DXGI_FORMAT
	// This is a union in C. We use the larger Color field and reinterpret for DepthStencil.
	Color [4]float32
}

// D3D12_DEPTH_STENCIL_VALUE describes depth/stencil clear values.
type D3D12_DEPTH_STENCIL_VALUE struct {
	Depth   float32
	Stencil uint8
}

// D3D12_RANGE describes a memory range.
type D3D12_RANGE struct {
	Begin uintptr
	End   uintptr
}

// D3D12_CPU_DESCRIPTOR_HANDLE represents a CPU descriptor handle.
type D3D12_CPU_DESCRIPTOR_HANDLE struct {
	Ptr uintptr
}

// Offset returns a new handle offset by the given number of descriptors.
func (h D3D12_CPU_DESCRIPTOR_HANDLE) Offset(index int, incrementSize uint32) D3D12_CPU_DESCRIPTOR_HANDLE {
	return D3D12_CPU_DESCRIPTOR_HANDLE{
		Ptr: h.Ptr + uintptr(index)*uintptr(incrementSize),
	}
}

// D3D12_GPU_DESCRIPTOR_HANDLE represents a GPU descriptor handle.
type D3D12_GPU_DESCRIPTOR_HANDLE struct {
	Ptr uint64
}

// Offset returns a new handle offset by the given number of descriptors.
func (h D3D12_GPU_DESCRIPTOR_HANDLE) Offset(index int, incrementSize uint32) D3D12_GPU_DESCRIPTOR_HANDLE {
	return D3D12_GPU_DESCRIPTOR_HANDLE{
		Ptr: h.Ptr + uint64(index)*uint64(incrementSize),
	}
}

// D3D12_DESCRIPTOR_HEAP_DESC describes a descriptor heap.
type D3D12_DESCRIPTOR_HEAP_DESC struct {
	Type           D3D12_DESCRIPTOR_HEAP_TYPE
	NumDescriptors uint32
	Flags          D3D12_DESCRIPTOR_HEAP_FLAGS
	NodeMask       uint32
}

// D3D12_RESOURCE_BARRIER describes a resource barrier.
type D3D12_RESOURCE_BARRIER struct {
	Type  D3D12_RESOURCE_BARRIER_TYPE
	Flags D3D12_RESOURCE_BARRIER_FLAGS
	// This is a union in C. We use a fixed-size array and interpret based on Type.
	// Transition: 24 bytes, Aliasing: 16 bytes, UAV: 8 bytes
	// We use 24 bytes to accommodate the largest variant.
	Union [24]byte
}

// D3D12_RESOURCE_TRANSITION_BARRIER describes a resource transition barrier.
type D3D12_RESOURCE_TRANSITION_BARRIER struct {
	Resource    *ID3D12Resource
	Subresource uint32
	StateBefore D3D12_RESOURCE_STATES
	StateAfter  D3D12_RESOURCE_STATES
}

// D3D12_RESOURCE_ALIASING_BARRIER describes a resource aliasing barrier.
type D3D12_RESOURCE_ALIASING_BARRIER struct {
	ResourceBefore *ID3D12Resource
	ResourceAfter  *ID3D12Resource
}

// D3D12_RESOURCE_UAV_BARRIER describes a UAV barrier.
type D3D12_RESOURCE_UAV_BARRIER struct {
	Resource *ID3D12Resource
}

// NewTransitionBarrier creates a transition barrier.
func NewTransitionBarrier(resource *ID3D12Resource, stateBefore, stateAfter D3D12_RESOURCE_STATES, subresource uint32) D3D12_RESOURCE_BARRIER {
	var barrier D3D12_RESOURCE_BARRIER
	barrier.Type = D3D12_RESOURCE_BARRIER_TYPE_TRANSITION
	barrier.Flags = D3D12_RESOURCE_BARRIER_FLAG_NONE

	transition := (*D3D12_RESOURCE_TRANSITION_BARRIER)(unsafe.Pointer(&barrier.Union[0]))
	transition.Resource = resource
	transition.Subresource = subresource
	transition.StateBefore = stateBefore
	transition.StateAfter = stateAfter

	return barrier
}

// NewUAVBarrier creates a UAV barrier.
func NewUAVBarrier(resource *ID3D12Resource) D3D12_RESOURCE_BARRIER {
	var barrier D3D12_RESOURCE_BARRIER
	barrier.Type = D3D12_RESOURCE_BARRIER_TYPE_UAV
	barrier.Flags = D3D12_RESOURCE_BARRIER_FLAG_NONE

	uav := (*D3D12_RESOURCE_UAV_BARRIER)(unsafe.Pointer(&barrier.Union[0]))
	uav.Resource = resource

	return barrier
}

// NewAliasingBarrier creates an aliasing barrier.
func NewAliasingBarrier(before, after *ID3D12Resource) D3D12_RESOURCE_BARRIER {
	var barrier D3D12_RESOURCE_BARRIER
	barrier.Type = D3D12_RESOURCE_BARRIER_TYPE_ALIASING
	barrier.Flags = D3D12_RESOURCE_BARRIER_FLAG_NONE

	aliasing := (*D3D12_RESOURCE_ALIASING_BARRIER)(unsafe.Pointer(&barrier.Union[0]))
	aliasing.ResourceBefore = before
	aliasing.ResourceAfter = after

	return barrier
}

// D3D12_VERTEX_BUFFER_VIEW describes a vertex buffer view.
type D3D12_VERTEX_BUFFER_VIEW struct {
	BufferLocation uint64
	SizeInBytes    uint32
	StrideInBytes  uint32
}

// D3D12_INDEX_BUFFER_VIEW describes an index buffer view.
type D3D12_INDEX_BUFFER_VIEW struct {
	BufferLocation uint64
	SizeInBytes    uint32
	Format         DXGI_FORMAT
}

// D3D12_STREAM_OUTPUT_BUFFER_VIEW describes a stream output buffer view.
type D3D12_STREAM_OUTPUT_BUFFER_VIEW struct {
	BufferLocation           uint64
	SizeInBytes              uint64
	BufferFilledSizeLocation uint64
}

// D3D12_CONSTANT_BUFFER_VIEW_DESC describes a constant buffer view.
type D3D12_CONSTANT_BUFFER_VIEW_DESC struct {
	BufferLocation uint64
	SizeInBytes    uint32
}

// D3D12_SHADER_RESOURCE_VIEW_DESC describes a shader resource view.
type D3D12_SHADER_RESOURCE_VIEW_DESC struct {
	Format                  DXGI_FORMAT
	ViewDimension           D3D12_SRV_DIMENSION
	Shader4ComponentMapping uint32
	// Union of different view types - use the largest
	Union [16]byte
}

// D3D12_UNORDERED_ACCESS_VIEW_DESC describes an unordered access view.
type D3D12_UNORDERED_ACCESS_VIEW_DESC struct {
	Format        DXGI_FORMAT
	ViewDimension D3D12_UAV_DIMENSION
	// Union of different view types
	Union [16]byte
}

// D3D12_RENDER_TARGET_VIEW_DESC describes a render target view.
type D3D12_RENDER_TARGET_VIEW_DESC struct {
	Format        DXGI_FORMAT
	ViewDimension D3D12_RTV_DIMENSION
	// Union of different view types
	Union [12]byte
}

// D3D12_DEPTH_STENCIL_VIEW_DESC describes a depth stencil view.
type D3D12_DEPTH_STENCIL_VIEW_DESC struct {
	Format        DXGI_FORMAT
	ViewDimension D3D12_DSV_DIMENSION
	Flags         D3D12_DSV_FLAGS
	// Union of different view types
	Union [8]byte
}

// D3D12_SAMPLER_DESC describes a sampler state.
type D3D12_SAMPLER_DESC struct {
	Filter         D3D12_FILTER
	AddressU       D3D12_TEXTURE_ADDRESS_MODE
	AddressV       D3D12_TEXTURE_ADDRESS_MODE
	AddressW       D3D12_TEXTURE_ADDRESS_MODE
	MipLODBias     float32
	MaxAnisotropy  uint32
	ComparisonFunc D3D12_COMPARISON_FUNC
	BorderColor    [4]float32
	MinLOD         float32
	MaxLOD         float32
}

// D3D12_VIEWPORT describes a viewport.
type D3D12_VIEWPORT struct {
	TopLeftX float32
	TopLeftY float32
	Width    float32
	Height   float32
	MinDepth float32
	MaxDepth float32
}

// D3D12_RECT describes a rectangle.
type D3D12_RECT struct {
	Left   int32
	Top    int32
	Right  int32
	Bottom int32
}

// D3D12_BOX describes a 3D box.
type D3D12_BOX struct {
	Left   uint32
	Top    uint32
	Front  uint32
	Right  uint32
	Bottom uint32
	Back   uint32
}

// D3D12_TEXTURE_COPY_LOCATION describes a texture copy location.
type D3D12_TEXTURE_COPY_LOCATION struct {
	Resource *ID3D12Resource
	Type     D3D12_TEXTURE_COPY_TYPE
	// Union: PlacedFootprint or SubresourceIndex
	Union [48]byte
}

// D3D12_TEXTURE_COPY_TYPE specifies texture copy type.
type D3D12_TEXTURE_COPY_TYPE uint32

// Texture copy type constants.
const (
	D3D12_TEXTURE_COPY_TYPE_SUBRESOURCE_INDEX D3D12_TEXTURE_COPY_TYPE = 0
	D3D12_TEXTURE_COPY_TYPE_PLACED_FOOTPRINT  D3D12_TEXTURE_COPY_TYPE = 1
)

// D3D12_PLACED_SUBRESOURCE_FOOTPRINT describes a placed subresource footprint.
type D3D12_PLACED_SUBRESOURCE_FOOTPRINT struct {
	Offset    uint64
	Footprint D3D12_SUBRESOURCE_FOOTPRINT
}

// D3D12_SUBRESOURCE_FOOTPRINT describes a subresource footprint.
type D3D12_SUBRESOURCE_FOOTPRINT struct {
	Format   DXGI_FORMAT
	Width    uint32
	Height   uint32
	Depth    uint32
	RowPitch uint32
}

// D3D12_TILED_RESOURCE_COORDINATE describes a tiled resource coordinate.
type D3D12_TILED_RESOURCE_COORDINATE struct {
	X           uint32
	Y           uint32
	Z           uint32
	Subresource uint32
}

// D3D12_TILE_REGION_SIZE describes a tile region size.
type D3D12_TILE_REGION_SIZE struct {
	NumTiles uint32
	UseBox   int32 // BOOL
	Width    uint32
	Height   uint16
	Depth    uint16
}

// D3D12_QUERY_HEAP_DESC describes a query heap.
type D3D12_QUERY_HEAP_DESC struct {
	Type     D3D12_QUERY_HEAP_TYPE
	Count    uint32
	NodeMask uint32
}

// D3D12_QUERY_DATA_PIPELINE_STATISTICS contains pipeline statistics query data.
type D3D12_QUERY_DATA_PIPELINE_STATISTICS struct {
	IAVertices    uint64
	IAPrimitives  uint64
	VSInvocations uint64
	GSInvocations uint64
	GSPrimitives  uint64
	CInvocations  uint64
	CPrimitives   uint64
	PSInvocations uint64
	HSInvocations uint64
	DSInvocations uint64
	CSInvocations uint64
}

// D3D12_QUERY_DATA_SO_STATISTICS contains stream output statistics query data.
type D3D12_QUERY_DATA_SO_STATISTICS struct {
	NumPrimitivesWritten    uint64
	PrimitivesStorageNeeded uint64
}

// D3D12_INPUT_ELEMENT_DESC describes an input element.
type D3D12_INPUT_ELEMENT_DESC struct {
	SemanticName         *byte
	SemanticIndex        uint32
	Format               DXGI_FORMAT
	InputSlot            uint32
	AlignedByteOffset    uint32
	InputSlotClass       D3D12_INPUT_CLASSIFICATION
	InstanceDataStepRate uint32
}

// D3D12_INPUT_LAYOUT_DESC describes an input layout.
type D3D12_INPUT_LAYOUT_DESC struct {
	InputElementDescs *D3D12_INPUT_ELEMENT_DESC
	NumElements       uint32
}

// D3D12_SHADER_BYTECODE describes shader bytecode.
type D3D12_SHADER_BYTECODE struct {
	ShaderBytecode unsafe.Pointer
	BytecodeLength uintptr
}

// D3D12_STREAM_OUTPUT_DESC describes stream output.
type D3D12_STREAM_OUTPUT_DESC struct {
	SODeclaration    *D3D12_SO_DECLARATION_ENTRY
	NumEntries       uint32
	BufferStrides    *uint32
	NumStrides       uint32
	RasterizedStream uint32
}

// D3D12_SO_DECLARATION_ENTRY describes a stream output declaration entry.
type D3D12_SO_DECLARATION_ENTRY struct {
	Stream         uint32
	SemanticName   *byte
	SemanticIndex  uint32
	StartComponent uint8
	ComponentCount uint8
	OutputSlot     uint8
}

// D3D12_RASTERIZER_DESC describes the rasterizer state.
type D3D12_RASTERIZER_DESC struct {
	FillMode              D3D12_FILL_MODE
	CullMode              D3D12_CULL_MODE
	FrontCounterClockwise int32 // BOOL
	DepthBias             int32
	DepthBiasClamp        float32
	SlopeScaledDepthBias  float32
	DepthClipEnable       int32 // BOOL
	MultisampleEnable     int32 // BOOL
	AntialiasedLineEnable int32 // BOOL
	ForcedSampleCount     uint32
	ConservativeRaster    D3D12_CONSERVATIVE_RASTERIZATION_MODE
}

// D3D12_BLEND_DESC describes the blend state.
type D3D12_BLEND_DESC struct {
	AlphaToCoverageEnable  int32 // BOOL
	IndependentBlendEnable int32 // BOOL
	RenderTarget           [8]D3D12_RENDER_TARGET_BLEND_DESC
}

// D3D12_RENDER_TARGET_BLEND_DESC describes the blend state for a render target.
type D3D12_RENDER_TARGET_BLEND_DESC struct {
	BlendEnable           int32 // BOOL
	LogicOpEnable         int32 // BOOL
	SrcBlend              D3D12_BLEND
	DestBlend             D3D12_BLEND
	BlendOp               D3D12_BLEND_OP
	SrcBlendAlpha         D3D12_BLEND
	DestBlendAlpha        D3D12_BLEND
	BlendOpAlpha          D3D12_BLEND_OP
	LogicOp               D3D12_LOGIC_OP
	RenderTargetWriteMask uint8
}

// D3D12_DEPTH_STENCIL_DESC describes the depth-stencil state.
type D3D12_DEPTH_STENCIL_DESC struct {
	DepthEnable      int32 // BOOL
	DepthWriteMask   D3D12_DEPTH_WRITE_MASK
	DepthFunc        D3D12_COMPARISON_FUNC
	StencilEnable    int32 // BOOL
	StencilReadMask  uint8
	StencilWriteMask uint8
	FrontFace        D3D12_DEPTH_STENCILOP_DESC
	BackFace         D3D12_DEPTH_STENCILOP_DESC
}

// D3D12_DEPTH_STENCILOP_DESC describes stencil operations.
type D3D12_DEPTH_STENCILOP_DESC struct {
	StencilFailOp      D3D12_STENCIL_OP
	StencilDepthFailOp D3D12_STENCIL_OP
	StencilPassOp      D3D12_STENCIL_OP
	StencilFunc        D3D12_COMPARISON_FUNC
}

// D3D12_CACHED_PIPELINE_STATE describes a cached pipeline state.
type D3D12_CACHED_PIPELINE_STATE struct {
	CachedBlob            unsafe.Pointer
	CachedBlobSizeInBytes uintptr
}

// D3D12_GRAPHICS_PIPELINE_STATE_DESC describes a graphics pipeline state.
type D3D12_GRAPHICS_PIPELINE_STATE_DESC struct {
	RootSignature         *ID3D12RootSignature
	VS                    D3D12_SHADER_BYTECODE
	PS                    D3D12_SHADER_BYTECODE
	DS                    D3D12_SHADER_BYTECODE
	HS                    D3D12_SHADER_BYTECODE
	GS                    D3D12_SHADER_BYTECODE
	StreamOutput          D3D12_STREAM_OUTPUT_DESC
	BlendState            D3D12_BLEND_DESC
	SampleMask            uint32
	RasterizerState       D3D12_RASTERIZER_DESC
	DepthStencilState     D3D12_DEPTH_STENCIL_DESC
	InputLayout           D3D12_INPUT_LAYOUT_DESC
	IBStripCutValue       D3D12_INDEX_BUFFER_STRIP_CUT_VALUE
	PrimitiveTopologyType D3D12_PRIMITIVE_TOPOLOGY_TYPE
	NumRenderTargets      uint32
	RTVFormats            [8]DXGI_FORMAT
	DSVFormat             DXGI_FORMAT
	SampleDesc            DXGI_SAMPLE_DESC
	NodeMask              uint32
	CachedPSO             D3D12_CACHED_PIPELINE_STATE
	Flags                 D3D12_PIPELINE_STATE_FLAGS
}

// D3D12_COMPUTE_PIPELINE_STATE_DESC describes a compute pipeline state.
type D3D12_COMPUTE_PIPELINE_STATE_DESC struct {
	RootSignature *ID3D12RootSignature
	CS            D3D12_SHADER_BYTECODE
	NodeMask      uint32
	CachedPSO     D3D12_CACHED_PIPELINE_STATE
	Flags         D3D12_PIPELINE_STATE_FLAGS
}

// D3D12_ROOT_SIGNATURE_DESC describes a root signature.
type D3D12_ROOT_SIGNATURE_DESC struct {
	NumParameters     uint32
	Parameters        *D3D12_ROOT_PARAMETER
	NumStaticSamplers uint32
	StaticSamplers    *D3D12_STATIC_SAMPLER_DESC
	Flags             D3D12_ROOT_SIGNATURE_FLAGS
}

// D3D12_ROOT_PARAMETER describes a root parameter.
type D3D12_ROOT_PARAMETER struct {
	ParameterType D3D12_ROOT_PARAMETER_TYPE
	// Union of DescriptorTable, Constants, or Descriptor
	Union            [16]byte
	ShaderVisibility D3D12_SHADER_VISIBILITY
}

// D3D12_ROOT_DESCRIPTOR_TABLE describes a descriptor table.
type D3D12_ROOT_DESCRIPTOR_TABLE struct {
	NumDescriptorRanges uint32
	DescriptorRanges    *D3D12_DESCRIPTOR_RANGE
}

// D3D12_DESCRIPTOR_RANGE describes a descriptor range.
type D3D12_DESCRIPTOR_RANGE struct {
	RangeType                         D3D12_DESCRIPTOR_RANGE_TYPE
	NumDescriptors                    uint32
	BaseShaderRegister                uint32
	RegisterSpace                     uint32
	OffsetInDescriptorsFromTableStart uint32
}

// D3D12_ROOT_CONSTANTS describes root constants.
type D3D12_ROOT_CONSTANTS struct {
	ShaderRegister uint32
	RegisterSpace  uint32
	Num32BitValues uint32
}

// D3D12_ROOT_DESCRIPTOR describes a root descriptor.
type D3D12_ROOT_DESCRIPTOR struct {
	ShaderRegister uint32
	RegisterSpace  uint32
}

// D3D12_STATIC_SAMPLER_DESC describes a static sampler.
type D3D12_STATIC_SAMPLER_DESC struct {
	Filter           D3D12_FILTER
	AddressU         D3D12_TEXTURE_ADDRESS_MODE
	AddressV         D3D12_TEXTURE_ADDRESS_MODE
	AddressW         D3D12_TEXTURE_ADDRESS_MODE
	MipLODBias       float32
	MaxAnisotropy    uint32
	ComparisonFunc   D3D12_COMPARISON_FUNC
	BorderColor      D3D12_STATIC_BORDER_COLOR
	MinLOD           float32
	MaxLOD           float32
	ShaderRegister   uint32
	RegisterSpace    uint32
	ShaderVisibility D3D12_SHADER_VISIBILITY
}

// D3D12_COMMAND_SIGNATURE_DESC describes a command signature.
type D3D12_COMMAND_SIGNATURE_DESC struct {
	ByteStride       uint32
	NumArgumentDescs uint32
	ArgumentDescs    *D3D12_INDIRECT_ARGUMENT_DESC
	NodeMask         uint32
}

// D3D12_INDIRECT_ARGUMENT_DESC describes an indirect argument.
type D3D12_INDIRECT_ARGUMENT_DESC struct {
	Type D3D12_INDIRECT_ARGUMENT_TYPE
	// Union for different argument types
	Union [8]byte
}

// D3D12_DISCARD_REGION describes a discard region.
type D3D12_DISCARD_REGION struct {
	NumRects         uint32
	Rects            *D3D12_RECT
	FirstSubresource uint32
	NumSubresources  uint32
}

// D3D12_FEATURE_DATA_SHADER_MODEL describes shader model feature data.
type D3D12_FEATURE_DATA_SHADER_MODEL struct {
	HighestShaderModel D3D_SHADER_MODEL
}

// D3D12_FEATURE_DATA_D3D12_OPTIONS describes D3D12 options feature data.
type D3D12_FEATURE_DATA_D3D12_OPTIONS struct {
	DoublePrecisionFloatShaderOps                                              int32  // BOOL
	OutputMergerLogicOp                                                        int32  // BOOL
	MinPrecisionSupport                                                        uint32 // D3D12_SHADER_MIN_PRECISION_SUPPORT
	TiledResourcesTier                                                         uint32 // D3D12_TILED_RESOURCES_TIER
	ResourceBindingTier                                                        uint32 // D3D12_RESOURCE_BINDING_TIER
	PSSpecifiedStencilRefSupported                                             int32  // BOOL
	TypedUAVLoadAdditionalFormats                                              int32  // BOOL
	ROVsSupported                                                              int32  // BOOL
	ConservativeRasterizationTier                                              uint32 // D3D12_CONSERVATIVE_RASTERIZATION_TIER
	MaxGPUVirtualAddressBitsPerResource                                        uint32
	StandardSwizzle64KBSupported                                               int32  // BOOL
	CrossNodeSharingTier                                                       uint32 // D3D12_CROSS_NODE_SHARING_TIER
	CrossAdapterRowMajorTextureSupported                                       int32  // BOOL
	VPAndRTArrayIndexFromAnyShaderFeedingRasterizerSupportedWithoutGSEmulation int32  // BOOL
	ResourceHeapTier                                                           uint32 // D3D12_RESOURCE_HEAP_TIER
}

// D3D12_FEATURE_DATA_FEATURE_LEVELS describes feature levels.
type D3D12_FEATURE_DATA_FEATURE_LEVELS struct {
	NumFeatureLevels         uint32
	FeatureLevelsRequested   *D3D_FEATURE_LEVEL
	MaxSupportedFeatureLevel D3D_FEATURE_LEVEL
}

// D3D12_RENDER_PASS_RENDER_TARGET_DESC describes a render pass render target.
type D3D12_RENDER_PASS_RENDER_TARGET_DESC struct {
	CPUDescriptor   D3D12_CPU_DESCRIPTOR_HANDLE
	BeginningAccess D3D12_RENDER_PASS_BEGINNING_ACCESS
	EndingAccess    D3D12_RENDER_PASS_ENDING_ACCESS
}

// D3D12_RENDER_PASS_DEPTH_STENCIL_DESC describes a render pass depth stencil.
type D3D12_RENDER_PASS_DEPTH_STENCIL_DESC struct {
	CPUDescriptor          D3D12_CPU_DESCRIPTOR_HANDLE
	DepthBeginningAccess   D3D12_RENDER_PASS_BEGINNING_ACCESS
	StencilBeginningAccess D3D12_RENDER_PASS_BEGINNING_ACCESS
	DepthEndingAccess      D3D12_RENDER_PASS_ENDING_ACCESS
	StencilEndingAccess    D3D12_RENDER_PASS_ENDING_ACCESS
}

// D3D12_RENDER_PASS_BEGINNING_ACCESS describes render pass beginning access.
type D3D12_RENDER_PASS_BEGINNING_ACCESS struct {
	Type D3D12_RENDER_PASS_BEGINNING_ACCESS_TYPE
	// Union for Clear (D3D12_RENDER_PASS_BEGINNING_ACCESS_CLEAR_PARAMETERS)
	Union [20]byte
}

// D3D12_RENDER_PASS_ENDING_ACCESS describes render pass ending access.
type D3D12_RENDER_PASS_ENDING_ACCESS struct {
	Type D3D12_RENDER_PASS_ENDING_ACCESS_TYPE
	// Union for Resolve (D3D12_RENDER_PASS_ENDING_ACCESS_RESOLVE_PARAMETERS)
	Union [48]byte
}

// D3D12_RENDER_PASS_BEGINNING_ACCESS_CLEAR_PARAMETERS describes clear parameters.
type D3D12_RENDER_PASS_BEGINNING_ACCESS_CLEAR_PARAMETERS struct {
	ClearValue D3D12_CLEAR_VALUE
}

// D3D12_DISPATCH_ARGUMENTS describes dispatch arguments.
type D3D12_DISPATCH_ARGUMENTS struct {
	ThreadGroupCountX uint32
	ThreadGroupCountY uint32
	ThreadGroupCountZ uint32
}

// D3D12_DRAW_ARGUMENTS describes draw arguments.
type D3D12_DRAW_ARGUMENTS struct {
	VertexCountPerInstance uint32
	InstanceCount          uint32
	StartVertexLocation    uint32
	StartInstanceLocation  uint32
}

// D3D12_DRAW_INDEXED_ARGUMENTS describes draw indexed arguments.
type D3D12_DRAW_INDEXED_ARGUMENTS struct {
	IndexCountPerInstance uint32
	InstanceCount         uint32
	StartIndexLocation    uint32
	BaseVertexLocation    int32
	StartInstanceLocation uint32
}

// D3D12_WRITEBUFFERIMMEDIATE_PARAMETER describes write buffer immediate parameters.
type D3D12_WRITEBUFFERIMMEDIATE_PARAMETER struct {
	Dest  uint64
	Value uint32
}

// D3D12_WRITEBUFFERIMMEDIATE_MODE specifies write buffer immediate mode.
type D3D12_WRITEBUFFERIMMEDIATE_MODE uint32

// Write buffer immediate mode constants.
const (
	D3D12_WRITEBUFFERIMMEDIATE_MODE_DEFAULT    D3D12_WRITEBUFFERIMMEDIATE_MODE = 0
	D3D12_WRITEBUFFERIMMEDIATE_MODE_MARKER_IN  D3D12_WRITEBUFFERIMMEDIATE_MODE = 1
	D3D12_WRITEBUFFERIMMEDIATE_MODE_MARKER_OUT D3D12_WRITEBUFFERIMMEDIATE_MODE = 2
)
