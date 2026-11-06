//go:build !rust && !(js && wasm)

package wgpu

import (
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// Extent3D is a 3D size.
type Extent3D struct {
	Width              uint32
	Height             uint32
	DepthOrArrayLayers uint32
}

func (e Extent3D) toHAL() hal.Extent3D {
	return hal.Extent3D{Width: e.Width, Height: e.Height, DepthOrArrayLayers: e.DepthOrArrayLayers}
}

// Origin3D is a 3D origin point.
type Origin3D struct {
	X uint32
	Y uint32
	Z uint32
}

func (o Origin3D) toHAL() hal.Origin3D {
	return hal.Origin3D{X: o.X, Y: o.Y, Z: o.Z}
}

// ImageDataLayout describes the layout of image data in a buffer.
type ImageDataLayout struct {
	Offset       uint64
	BytesPerRow  uint32
	RowsPerImage uint32
}

func (l ImageDataLayout) toHAL() hal.ImageDataLayout {
	return hal.ImageDataLayout{Offset: l.Offset, BytesPerRow: l.BytesPerRow, RowsPerImage: l.RowsPerImage}
}

// BufferDescriptor describes buffer creation parameters.
type BufferDescriptor struct {
	Label            string
	Size             uint64
	Usage            gputypes.BufferUsage
	MappedAtCreation bool
}

// toHAL converts a BufferDescriptor to a hal.BufferDescriptor.
func (d *BufferDescriptor) toHAL() *hal.BufferDescriptor {
	return &hal.BufferDescriptor{
		Label:            d.Label,
		Size:             d.Size,
		Usage:            d.Usage,
		MappedAtCreation: d.MappedAtCreation,
	}
}

// TextureDescriptor describes texture creation parameters.
//
// A zero MipLevelCount, SampleCount, Dimension, Size.Height or
// Size.DepthOrArrayLayers means the WebGPU default: 1, 1,
// gputypes.TextureDimension2D, 1 and 1. Size.Width is required.
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

// toHAL converts a TextureDescriptor to a hal.TextureDescriptor.
func (d *TextureDescriptor) toHAL() *hal.TextureDescriptor {
	return &hal.TextureDescriptor{
		Label:         d.Label,
		Size:          d.Size.toHAL(),
		MipLevelCount: d.MipLevelCount,
		SampleCount:   d.SampleCount,
		Dimension:     d.Dimension,
		Format:        d.Format,
		Usage:         d.Usage,
		ViewFormats:   d.ViewFormats,
	}
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

// toHAL converts a TextureViewDescriptor to a hal.TextureViewDescriptor.
func (d *TextureViewDescriptor) toHAL() *hal.TextureViewDescriptor {
	return &hal.TextureViewDescriptor{
		Label:           d.Label,
		Format:          d.Format,
		Dimension:       d.Dimension,
		Aspect:          d.Aspect,
		BaseMipLevel:    d.BaseMipLevel,
		MipLevelCount:   d.MipLevelCount,
		BaseArrayLayer:  d.BaseArrayLayer,
		ArrayLayerCount: d.ArrayLayerCount,
	}
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

// toHAL converts a SamplerDescriptor to a hal.SamplerDescriptor. The HAL takes
// every field literally, so the zero values of the filters, the address modes,
// LodMaxClamp and Anisotropy are resolved to the WebGPU defaults here.
func (d *SamplerDescriptor) toHAL() *hal.SamplerDescriptor {
	return &hal.SamplerDescriptor{
		Label:        d.Label,
		AddressModeU: normalizeAddressMode(d.AddressModeU),
		AddressModeV: normalizeAddressMode(d.AddressModeV),
		AddressModeW: normalizeAddressMode(d.AddressModeW),
		MagFilter:    normalizeFilterMode(d.MagFilter),
		MinFilter:    normalizeFilterMode(d.MinFilter),
		MipmapFilter: normalizeFilterMode(d.MipmapFilter),
		LodMinClamp:  d.LodMinClamp,
		LodMaxClamp:  normalizeLodMaxClamp(d.LodMaxClamp),
		Compare:      d.Compare,
		Anisotropy:   normalizeAnisotropy(d.Anisotropy),
	}
}

// QueryType specifies the type of queries in a query set.
type QueryType = hal.QueryType

// Query type constants.
const (
	QueryTypeOcclusion = hal.QueryTypeOcclusion
	QueryTypeTimestamp = hal.QueryTypeTimestamp
)

// QuerySetDescriptor describes query set creation parameters.
type QuerySetDescriptor struct {
	Label string
	Type  QueryType
	Count uint32
}

// toHAL converts a QuerySetDescriptor to a hal.QuerySetDescriptor.
func (d *QuerySetDescriptor) toHAL() *hal.QuerySetDescriptor {
	return &hal.QuerySetDescriptor{
		Label: d.Label,
		Type:  d.Type,
		Count: d.Count,
	}
}

// ShaderModuleDescriptor describes shader module creation parameters.
type ShaderModuleDescriptor struct {
	Label string
	WGSL  string   // WGSL source code
	SPIRV []uint32 // SPIR-V bytecode (alternative to WGSL)
}

// toHAL converts a ShaderModuleDescriptor to a hal.ShaderModuleDescriptor.
func (d *ShaderModuleDescriptor) toHAL() *hal.ShaderModuleDescriptor {
	return &hal.ShaderModuleDescriptor{
		Label: d.Label,
		Source: hal.ShaderSource{
			WGSL:  d.WGSL,
			SPIRV: d.SPIRV,
		},
	}
}

// CommandEncoderDescriptor describes command encoder creation.
type CommandEncoderDescriptor struct {
	Label string
}

// RenderBundleEncoderDescriptor describes the render pass compatibility of a
// render bundle recorded by a RenderBundleEncoder.
type RenderBundleEncoderDescriptor struct {
	Label              string
	ColorFormats       []gputypes.TextureFormat
	DepthStencilFormat gputypes.TextureFormat
	SampleCount        uint32
	DepthReadOnly      bool
	StencilReadOnly    bool
}

// RenderBundleDescriptor describes a completed render bundle.
type RenderBundleDescriptor struct {
	Label string
}

// toHAL converts a CommandEncoderDescriptor to a hal.CommandEncoderDescriptor.
func (d *CommandEncoderDescriptor) toHAL() *hal.CommandEncoderDescriptor {
	return &hal.CommandEncoderDescriptor{
		Label: d.Label,
	}
}

// BindGroupLayoutDescriptor describes a bind group layout.
type BindGroupLayoutDescriptor struct {
	Label   string
	Entries []gputypes.BindGroupLayoutEntry
}

// toHAL converts a BindGroupLayoutDescriptor to a hal.BindGroupLayoutDescriptor.
func (d *BindGroupLayoutDescriptor) toHAL() *hal.BindGroupLayoutDescriptor {
	return &hal.BindGroupLayoutDescriptor{
		Label:   d.Label,
		Entries: d.Entries,
	}
}

// BindGroupDescriptor describes a bind group.
type BindGroupDescriptor struct {
	Label   string
	Layout  *BindGroupLayout
	Entries []BindGroupEntry
}

// BindGroupEntry describes a single resource binding in a bind group.
// Exactly one of Buffer, Sampler, or TextureView must be set.
type BindGroupEntry struct {
	Binding     uint32
	Buffer      *Buffer      // For buffer bindings
	Offset      uint64       // Buffer offset
	Size        uint64       // Buffer binding size (0 = rest of buffer)
	Sampler     *Sampler     // For sampler bindings
	TextureView *TextureView // For texture bindings
}

// toHAL converts a BindGroupEntry to a gputypes.BindGroupEntry.
func (e *BindGroupEntry) toHAL() gputypes.BindGroupEntry {
	entry := gputypes.BindGroupEntry{
		Binding: e.Binding,
	}
	var halView hal.TextureView
	if e.TextureView != nil {
		halView = e.TextureView.resolveHAL()
	}

	switch {
	case e.Buffer != nil:
		halBuf := e.Buffer.halBuffer()
		if halBuf != nil {
			entry.Resource = gputypes.BufferBinding{
				Buffer: halBuf.NativeHandle(),
				Offset: e.Offset,
				Size:   e.Size,
			}
		}
	case e.Sampler != nil && e.Sampler.hal != nil:
		entry.Resource = gputypes.SamplerBinding{
			Sampler: e.Sampler.hal.NativeHandle(),
		}
	case halView != nil:
		entry.Resource = gputypes.TextureViewBinding{
			TextureView: halView.NativeHandle(),
		}
	}

	return entry
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

func (s StencilFaceState) toHAL() hal.StencilFaceState {
	return hal.StencilFaceState{Compare: s.Compare, FailOp: s.FailOp, DepthFailOp: s.DepthFailOp, PassOp: s.PassOp}
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

func (d *DepthStencilState) toHAL() *hal.DepthStencilState {
	if d == nil {
		return nil
	}
	return &hal.DepthStencilState{
		Format:              d.Format,
		DepthWriteEnabled:   d.DepthWriteEnabled,
		DepthCompare:        d.DepthCompare,
		StencilFront:        d.StencilFront.toHAL(),
		StencilBack:         d.StencilBack.toHAL(),
		StencilReadMask:     d.StencilReadMask,
		StencilWriteMask:    d.StencilWriteMask,
		DepthBias:           d.DepthBias,
		DepthBiasSlopeScale: d.DepthBiasSlopeScale,
		DepthBiasClamp:      d.DepthBiasClamp,
	}
}

// resolveDepthCompare gives the HAL CompareFunctionAlways for a depthCompare
// the descriptor left undefined. GPUDepthStencilState.depthCompare has no
// default: core validation, which runs first, rejects its absence where the
// spec requires it, and elsewhere the spec's "compare fragment" algorithm
// passes every fragment for an undefined compare, which is "always"
// (index.bs). The backends take the value literally: DX12 mapped Undefined to
// NEVER and drew nothing. Rust wgpu-hal does ds.depth_compare.unwrap_or_default().
func resolveDepthCompare(ds *hal.DepthStencilState) {
	if ds != nil && ds.DepthCompare == gputypes.CompareFunctionUndefined {
		ds.DepthCompare = gputypes.CompareFunctionAlways
	}
}

// RenderPipelineDescriptor describes a render pipeline.
type RenderPipelineDescriptor struct {
	Label        string
	Layout       *PipelineLayout
	Vertex       VertexState
	Primitive    gputypes.PrimitiveState
	DepthStencil *DepthStencilState
	// Multisample: a zero Mask means every sample (0xFFFFFFFF), the WebGPU
	// default, since a mask of 0 would discard every fragment. Only the low
	// 32 bits are used.
	Multisample gputypes.MultisampleState
	Fragment    *FragmentState
}

// VertexState describes the vertex shader stage.
type VertexState struct {
	Module     *ShaderModule
	EntryPoint string

	// Constants are the pipeline-overridable constants of the vertex stage
	// (WebGPU GPUProgrammableStage.constants): keys are the identifiers or
	// @id numbers of the shader's overrides, values are converted to each
	// override's type. Nil or empty means every override takes its default.
	// Validated and applied as ComputePipelineDescriptor.Constants are.
	Constants map[string]float64

	Buffers []gputypes.VertexBufferLayout
}

// FragmentState describes the fragment shader stage.
type FragmentState struct {
	Module     *ShaderModule
	EntryPoint string

	// Constants are the pipeline-overridable constants of the fragment stage
	// (WebGPU GPUProgrammableStage.constants), as in VertexState.Constants.
	Constants map[string]float64

	// Targets describe the color attachments. A target's WriteMask is
	// taken literally: zero (gputypes.ColorWriteMaskNone) writes nothing,
	// so set gputypes.ColorWriteMaskAll to write every channel. A nil Blend
	// replaces; a zero BlendComponent is the WebGPU default (srcFactor one,
	// dstFactor zero, operation add).
	Targets []gputypes.ColorTargetState
}

// toHAL converts a RenderPipelineDescriptor to a hal.RenderPipelineDescriptor,
// with the WebGPU defaults applied to zero stencil faces, blend components and
// a zero sample mask (pipeline_defaults.go). The caller's descriptor is not
// modified.
func (d *RenderPipelineDescriptor) toHAL() *hal.RenderPipelineDescriptor {
	halDesc := &hal.RenderPipelineDescriptor{
		Label:        d.Label,
		Primitive:    d.Primitive,
		Multisample:  normalizeMultisample(d.Multisample),
		DepthStencil: normalizeDepthStencilState(d.DepthStencil).toHAL(),
	}

	if d.Layout != nil {
		halDesc.Layout = d.Layout.hal
	}

	if d.Vertex.Module != nil {
		halDesc.Vertex = hal.VertexState{
			Module:     d.Vertex.Module.hal,
			EntryPoint: d.Vertex.EntryPoint,
			Constants:  d.Vertex.Constants,
			Buffers:    d.Vertex.Buffers,
		}
	}

	if d.Fragment != nil && d.Fragment.Module != nil {
		halDesc.Fragment = &hal.FragmentState{
			Module:     d.Fragment.Module.hal,
			EntryPoint: d.Fragment.EntryPoint,
			Constants:  d.Fragment.Constants,
			Targets:    normalizeColorTargets(d.Fragment.Targets),
		}
	}

	return halDesc
}

// ComputePipelineDescriptor describes a compute pipeline.
type ComputePipelineDescriptor struct {
	Label      string
	Layout     *PipelineLayout
	Module     *ShaderModule
	EntryPoint string

	// Constants are pipeline-overridable constants (WebGPU spec).
	// Keys are constant identifiers defined in the shader, values are float64 overrides.
	// Nil or empty means no overrides are applied.
	//
	// Matches Rust wgpu PipelineConstants (wgpu-types). Each backend applies these
	// during shader compilation via naga's override processing.
	Constants map[string]float64

	// ZeroInitializeWorkgroupMemory controls whether workgroup-scoped shared memory
	// is zero-initialized before compute shader execution (WebGPU spec default: true).
	//
	// When nil (zero value for *bool), defaults to true per WebGPU spec.
	// Set to false only for cross-platform applications that do not rely on
	// zero-initialized workgroup memory and want to avoid the overhead.
	//
	// Matches Rust wgpu ProgrammableStage.zero_initialize_workgroup_memory.
	ZeroInitializeWorkgroupMemory *bool
}

// toHAL converts a ComputePipelineDescriptor to a hal.ComputePipelineDescriptor.
func (d *ComputePipelineDescriptor) toHAL() *hal.ComputePipelineDescriptor {
	halDesc := &hal.ComputePipelineDescriptor{
		Label: d.Label,
	}

	if d.Layout != nil {
		halDesc.Layout = d.Layout.hal
	}

	// WebGPU spec: zero_initialize_workgroup_memory defaults to true.
	// When the caller does not set the field (nil), we default to true.
	zeroInit := true
	if d.ZeroInitializeWorkgroupMemory != nil {
		zeroInit = *d.ZeroInitializeWorkgroupMemory
	}

	if d.Module != nil {
		halDesc.Compute = hal.ComputeState{
			Module:                        d.Module.hal,
			EntryPoint:                    d.EntryPoint,
			Constants:                     d.Constants,
			ZeroInitializeWorkgroupMemory: zeroInit,
		}
	}

	return halDesc
}

// RenderPassDescriptor describes a render pass.
type RenderPassDescriptor struct {
	Label                  string
	ColorAttachments       []RenderPassColorAttachment
	DepthStencilAttachment *RenderPassDepthStencilAttachment
	TimestampWrites        *RenderPassTimestampWrites
}

// RenderPassTimestampWrites describes timestamp query writes at render pass boundaries.
type RenderPassTimestampWrites struct {
	QuerySet                  *QuerySet
	BeginningOfPassWriteIndex *uint32
	EndOfPassWriteIndex       *uint32
}

// RenderPassColorAttachment describes a color attachment.
type RenderPassColorAttachment struct {
	View          *TextureView
	ResolveTarget *TextureView
	// DepthSlice is the slice of a 3D View to render to (WebGPU
	// GPURenderPassColorAttachment.depthSlice). It is required when View is a
	// 3D view and must then be less than the depth of the view's base mip
	// level; it must be nil for any other view. nil means undefined: every
	// non-nil value is validated, 0xFFFFFFFF (webgpu.h
	// WGPU_DEPTH_SLICE_UNDEFINED) included.
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

// toHAL converts a RenderPassDescriptor to a hal.RenderPassDescriptor.
func (d *RenderPassDescriptor) toHAL() *hal.RenderPassDescriptor {
	halDesc := &hal.RenderPassDescriptor{
		Label: d.Label,
	}

	for _, ca := range d.ColorAttachments {
		halCA := hal.RenderPassColorAttachment{
			LoadOp:     ca.LoadOp,
			StoreOp:    ca.StoreOp,
			ClearValue: ca.ClearValue,
		}
		if ca.DepthSlice != nil {
			slice := *ca.DepthSlice
			halCA.DepthSlice = &slice
		}
		if ca.View != nil {
			halCA.View = ca.View.resolveHAL()
		}
		if ca.ResolveTarget != nil {
			halCA.ResolveTarget = ca.ResolveTarget.resolveHAL()
		}
		halDesc.ColorAttachments = append(halDesc.ColorAttachments, halCA)
	}

	if d.DepthStencilAttachment != nil {
		ds := d.DepthStencilAttachment
		halDS := &hal.RenderPassDepthStencilAttachment{
			DepthLoadOp:       ds.DepthLoadOp,
			DepthStoreOp:      ds.DepthStoreOp,
			DepthClearValue:   ds.DepthClearValue,
			DepthReadOnly:     ds.DepthReadOnly,
			StencilLoadOp:     ds.StencilLoadOp,
			StencilStoreOp:    ds.StencilStoreOp,
			StencilClearValue: ds.StencilClearValue,
			StencilReadOnly:   ds.StencilReadOnly,
		}
		if ds.View != nil {
			halDS.View = ds.View.resolveHAL()
		}
		halDesc.DepthStencilAttachment = halDS
	}

	if writes := d.TimestampWrites; writes != nil {
		halQuerySet := writes.QuerySet.halQuerySet()
		if halQuerySet == nil {
			return halDesc
		}
		halDesc.TimestampWrites = &hal.RenderPassTimestampWrites{
			QuerySet:                  halQuerySet,
			BeginningOfPassWriteIndex: writes.BeginningOfPassWriteIndex,
			EndOfPassWriteIndex:       writes.EndOfPassWriteIndex,
		}
	}

	return halDesc
}

// ComputePassDescriptor describes a compute pass.
type ComputePassDescriptor struct {
	Label string
	// TimestampWrites, when set, writes timestamps at the pass boundaries.
	// It needs FeatureTimestampQuery.
	TimestampWrites *ComputePassTimestampWrites
}

// ComputePassTimestampWrites describes timestamp query writes at compute pass
// boundaries (WebGPU GPUComputePassTimestampWrites). At least one index must
// be set, and the two must differ.
type ComputePassTimestampWrites struct {
	QuerySet                  *QuerySet
	BeginningOfPassWriteIndex *uint32
	EndOfPassWriteIndex       *uint32
}

// toHAL converts a ComputePassDescriptor to a hal.ComputePassDescriptor.
func (d *ComputePassDescriptor) toHAL() *hal.ComputePassDescriptor {
	halDesc := &hal.ComputePassDescriptor{
		Label: d.Label,
	}
	if writes := d.TimestampWrites; writes != nil {
		if halQuerySet := writes.QuerySet.halQuerySet(); halQuerySet != nil {
			halDesc.TimestampWrites = &hal.ComputePassTimestampWrites{
				QuerySet:                  halQuerySet,
				BeginningOfPassWriteIndex: writes.BeginningOfPassWriteIndex,
				EndOfPassWriteIndex:       writes.EndOfPassWriteIndex,
			}
		}
	}
	return halDesc
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

// toHAL converts a SurfaceConfiguration to a hal.SurfaceConfiguration.
func (c *SurfaceConfiguration) toHAL() *hal.SurfaceConfiguration {
	return &hal.SurfaceConfiguration{
		Width:       c.Width,
		Height:      c.Height,
		Format:      c.Format,
		Usage:       c.Usage,
		PresentMode: c.PresentMode,
		AlphaMode:   c.AlphaMode,
	}
}

// ImageCopyTexture describes a texture subresource and origin for write operations.
type ImageCopyTexture struct {
	Texture  *Texture
	MipLevel uint32
	Origin   Origin3D
	Aspect   gputypes.TextureAspect
}

func (i *ImageCopyTexture) toHAL() *hal.ImageCopyTexture {
	if i == nil || i.Texture == nil {
		return nil
	}

	return &hal.ImageCopyTexture{
		Texture:  i.Texture.resolveHAL(),
		MipLevel: i.MipLevel,
		Origin:   i.Origin.toHAL(),
		Aspect:   i.Aspect,
	}
}

// TextureCopy describes a texture-to-texture copy region.
// WebGPU spec: GPUCommandEncoder.copyTextureToTexture()
type TextureCopy struct {
	Source      ImageCopyTexture
	Destination ImageCopyTexture
	Size        Extent3D
}

func (t *TextureCopy) toHAL() hal.TextureCopy {
	return hal.TextureCopy{
		SrcBase: *t.Source.toHAL(),
		DstBase: *t.Destination.toHAL(),
		Size:    t.Size.toHAL(),
	}
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
// Required on Vulkan for layout transitions between render pass and copy
// operations. On Metal, GLES, and software backends this is a no-op.
type TextureBarrier struct {
	Texture *Texture
	Range   TextureRange
	Usage   TextureUsageTransition
}

func (b TextureBarrier) toHAL() hal.TextureBarrier {
	var t hal.Texture
	if b.Texture != nil {
		t = b.Texture.resolveHAL()
	}
	return hal.TextureBarrier{
		Texture: t,
		Range: hal.TextureRange{
			Aspect:          b.Range.Aspect,
			BaseMipLevel:    b.Range.BaseMipLevel,
			MipLevelCount:   b.Range.MipLevelCount,
			BaseArrayLayer:  b.Range.BaseArrayLayer,
			ArrayLayerCount: b.Range.ArrayLayerCount,
		},
		Usage: hal.TextureUsageTransition{
			OldUsage: b.Usage.OldUsage,
			NewUsage: b.Usage.NewUsage,
		},
	}
}

// BufferTextureCopy defines a buffer-texture copy region.
type BufferTextureCopy struct {
	BufferLayout ImageDataLayout
	TextureBase  ImageCopyTexture
	Size         Extent3D
}

func (c BufferTextureCopy) toHAL() hal.BufferTextureCopy {
	btc := hal.BufferTextureCopy{
		BufferLayout: c.BufferLayout.toHAL(),
		Size:         c.Size.toHAL(),
	}
	halTex := c.TextureBase.toHAL()
	if halTex != nil {
		btc.TextureBase = *halTex
	}
	return btc
}
