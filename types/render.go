package types

// Color represents an RGBA color.
type Color struct {
	R, G, B, A float64
}

// PredefinedColors
var (
	ColorTransparent = Color{0, 0, 0, 0}
	ColorBlack       = Color{0, 0, 0, 1}
	ColorWhite       = Color{1, 1, 1, 1}
	ColorRed         = Color{1, 0, 0, 1}
	ColorGreen       = Color{0, 1, 0, 1}
	ColorBlue        = Color{0, 0, 1, 1}
)

// LoadOp describes the load operation for an attachment.
type LoadOp uint8

const (
	// LoadOpClear clears the attachment.
	LoadOpClear LoadOp = iota
	// LoadOpLoad loads the existing contents.
	LoadOpLoad
)

// StoreOp describes the store operation for an attachment.
type StoreOp uint8

const (
	// StoreOpDiscard discards the contents.
	StoreOpDiscard StoreOp = iota
	// StoreOpStore stores the contents.
	StoreOpStore
)

// RenderPassColorAttachment describes a color attachment.
type RenderPassColorAttachment struct {
	// View is the texture view to render to.
	View TextureViewHandle
	// ResolveTarget is the texture view for multisample resolve.
	ResolveTarget *TextureViewHandle
	// LoadOp describes how to load the attachment.
	LoadOp LoadOp
	// StoreOp describes how to store the attachment.
	StoreOp StoreOp
	// ClearValue is the clear color.
	ClearValue Color
}

// RenderPassDepthStencilAttachment describes a depth-stencil attachment.
type RenderPassDepthStencilAttachment struct {
	// View is the texture view.
	View TextureViewHandle
	// DepthLoadOp describes how to load depth.
	DepthLoadOp LoadOp
	// DepthStoreOp describes how to store depth.
	DepthStoreOp StoreOp
	// DepthClearValue is the clear depth value.
	DepthClearValue float32
	// DepthReadOnly indicates if depth is read-only.
	DepthReadOnly bool
	// StencilLoadOp describes how to load stencil.
	StencilLoadOp LoadOp
	// StencilStoreOp describes how to store stencil.
	StencilStoreOp StoreOp
	// StencilClearValue is the clear stencil value.
	StencilClearValue uint32
	// StencilReadOnly indicates if stencil is read-only.
	StencilReadOnly bool
}

// RenderPassDescriptor describes a render pass.
type RenderPassDescriptor struct {
	// Label is a debug label.
	Label string
	// ColorAttachments are the color attachments.
	ColorAttachments []RenderPassColorAttachment
	// DepthStencilAttachment is the depth-stencil attachment.
	DepthStencilAttachment *RenderPassDepthStencilAttachment
	// OcclusionQuerySet is the occlusion query set.
	OcclusionQuerySet QuerySetHandle
	// TimestampWrites describe timestamp queries.
	TimestampWrites *RenderPassTimestampWrites
}

// RenderPassTimestampWrites describes timestamp writes.
type RenderPassTimestampWrites struct {
	// QuerySet is the query set for timestamps.
	QuerySet QuerySetHandle
	// BeginningOfPassWriteIndex is the index for start timestamp.
	BeginningOfPassWriteIndex *uint32
	// EndOfPassWriteIndex is the index for end timestamp.
	EndOfPassWriteIndex *uint32
}

// QuerySetHandle is a handle to a query set.
type QuerySetHandle uint64

// BlendState describes color blending.
type BlendState struct {
	// Color describes color channel blending.
	Color BlendComponent
	// Alpha describes alpha channel blending.
	Alpha BlendComponent
}

// BlendComponent describes blending for a single color component.
type BlendComponent struct {
	// SrcFactor is the source blend factor.
	SrcFactor BlendFactor
	// DstFactor is the destination blend factor.
	DstFactor BlendFactor
	// Operation is the blend operation.
	Operation BlendOperation
}

// BlendFactor describes a blend factor.
type BlendFactor uint8

const (
	BlendFactorZero BlendFactor = iota
	BlendFactorOne
	BlendFactorSrc
	BlendFactorOneMinusSrc
	BlendFactorSrcAlpha
	BlendFactorOneMinusSrcAlpha
	BlendFactorDst
	BlendFactorOneMinusDst
	BlendFactorDstAlpha
	BlendFactorOneMinusDstAlpha
	BlendFactorSrcAlphaSaturated
	BlendFactorConstant
	BlendFactorOneMinusConstant
)

// BlendOperation describes a blend operation.
type BlendOperation uint8

const (
	BlendOperationAdd BlendOperation = iota
	BlendOperationSubtract
	BlendOperationReverseSubtract
	BlendOperationMin
	BlendOperationMax
)

// ColorWriteMask describes which color channels to write.
type ColorWriteMask uint8

const (
	ColorWriteMaskRed ColorWriteMask = 1 << iota
	ColorWriteMaskGreen
	ColorWriteMaskBlue
	ColorWriteMaskAlpha
	ColorWriteMaskAll = ColorWriteMaskRed | ColorWriteMaskGreen | ColorWriteMaskBlue | ColorWriteMaskAlpha
)

// ColorTargetState describes a color target in a render pipeline.
type ColorTargetState struct {
	// Format is the texture format.
	Format TextureFormat
	// Blend describes color blending (nil for no blending).
	Blend *BlendState
	// WriteMask specifies which channels to write.
	WriteMask ColorWriteMask
}

// PrimitiveTopology describes how vertices form primitives.
type PrimitiveTopology uint8

const (
	PrimitiveTopologyPointList PrimitiveTopology = iota
	PrimitiveTopologyLineList
	PrimitiveTopologyLineStrip
	PrimitiveTopologyTriangleList
	PrimitiveTopologyTriangleStrip
)

// FrontFace describes the front face winding order.
type FrontFace uint8

const (
	FrontFaceCCW FrontFace = iota
	FrontFaceCW
)

// CullMode describes which faces to cull.
type CullMode uint8

const (
	CullModeNone CullMode = iota
	CullModeFront
	CullModeBack
)

// PrimitiveState describes primitive assembly.
type PrimitiveState struct {
	// Topology is the primitive topology.
	Topology PrimitiveTopology
	// StripIndexFormat is the index format for strip topologies.
	StripIndexFormat *IndexFormat
	// FrontFace is the front face winding.
	FrontFace FrontFace
	// CullMode specifies which faces to cull.
	CullMode CullMode
	// UnclippedDepth enables unclipped depth.
	UnclippedDepth bool
}

// MultisampleState describes multisampling.
type MultisampleState struct {
	// Count is the number of samples.
	Count uint32
	// Mask is the sample mask.
	Mask uint64
	// AlphaToCoverageEnabled enables alpha-to-coverage.
	AlphaToCoverageEnabled bool
}

// DefaultMultisampleState returns the default multisample state.
func DefaultMultisampleState() MultisampleState {
	return MultisampleState{
		Count:                  1,
		Mask:                   0xFFFFFFFF,
		AlphaToCoverageEnabled: false,
	}
}
