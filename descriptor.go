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
	Usage            BufferUsage
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
	Format          TextureFormat
	Dimension       TextureViewDimension
	Aspect          TextureAspect
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

// toHAL converts a SamplerDescriptor to a hal.SamplerDescriptor.
func (d *SamplerDescriptor) toHAL() *hal.SamplerDescriptor {
	return &hal.SamplerDescriptor{
		Label:        d.Label,
		AddressModeU: d.AddressModeU,
		AddressModeV: d.AddressModeV,
		AddressModeW: d.AddressModeW,
		MagFilter:    d.MagFilter,
		MinFilter:    d.MinFilter,
		MipmapFilter: d.MipmapFilter,
		LodMinClamp:  d.LodMinClamp,
		LodMaxClamp:  d.LodMaxClamp,
		Compare:      d.Compare,
		Anisotropy:   d.Anisotropy,
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

// toHAL converts a CommandEncoderDescriptor to a hal.CommandEncoderDescriptor.
func (d *CommandEncoderDescriptor) toHAL() *hal.CommandEncoderDescriptor {
	return &hal.CommandEncoderDescriptor{
		Label: d.Label,
	}
}

// BindGroupLayoutDescriptor describes a bind group layout.
type BindGroupLayoutDescriptor struct {
	Label   string
	Entries []BindGroupLayoutEntry
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
	case e.TextureView != nil && e.TextureView.hal != nil:
		entry.Resource = gputypes.TextureViewBinding{
			TextureView: e.TextureView.hal.NativeHandle(),
		}
	}

	return entry
}

// PipelineLayoutDescriptor describes a pipeline layout.
type PipelineLayoutDescriptor struct {
	Label            string
	BindGroupLayouts []*BindGroupLayout
}

// StencilOperation describes a stencil operation.
type StencilOperation = hal.StencilOperation

// Stencil operation constants.
const (
	StencilOperationKeep           = hal.StencilOperationKeep
	StencilOperationZero           = hal.StencilOperationZero
	StencilOperationReplace        = hal.StencilOperationReplace
	StencilOperationInvert         = hal.StencilOperationInvert
	StencilOperationIncrementClamp = hal.StencilOperationIncrementClamp
	StencilOperationDecrementClamp = hal.StencilOperationDecrementClamp
	StencilOperationIncrementWrap  = hal.StencilOperationIncrementWrap
	StencilOperationDecrementWrap  = hal.StencilOperationDecrementWrap
)

// StencilFaceState describes stencil operations for a face.
type StencilFaceState struct {
	Compare     CompareFunction
	FailOp      StencilOperation
	DepthFailOp StencilOperation
	PassOp      StencilOperation
}

func (s StencilFaceState) toHAL() hal.StencilFaceState {
	return hal.StencilFaceState{Compare: s.Compare, FailOp: s.FailOp, DepthFailOp: s.DepthFailOp, PassOp: s.PassOp}
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

// toHAL converts a RenderPipelineDescriptor to a hal.RenderPipelineDescriptor.
func (d *RenderPipelineDescriptor) toHAL() *hal.RenderPipelineDescriptor {
	halDesc := &hal.RenderPipelineDescriptor{
		Label:        d.Label,
		Primitive:    d.Primitive,
		Multisample:  d.Multisample,
		DepthStencil: d.DepthStencil.toHAL(),
	}

	if d.Layout != nil {
		halDesc.Layout = d.Layout.hal
	}

	if d.Vertex.Module != nil {
		halDesc.Vertex = hal.VertexState{
			Module:     d.Vertex.Module.hal,
			EntryPoint: d.Vertex.EntryPoint,
			Buffers:    d.Vertex.Buffers,
		}
	}

	if d.Fragment != nil && d.Fragment.Module != nil {
		halDesc.Fragment = &hal.FragmentState{
			Module:     d.Fragment.Module.hal,
			EntryPoint: d.Fragment.EntryPoint,
			Targets:    d.Fragment.Targets,
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
}

// toHAL converts a ComputePipelineDescriptor to a hal.ComputePipelineDescriptor.
func (d *ComputePipelineDescriptor) toHAL() *hal.ComputePipelineDescriptor {
	halDesc := &hal.ComputePipelineDescriptor{
		Label: d.Label,
	}

	if d.Layout != nil {
		halDesc.Layout = d.Layout.hal
	}

	if d.Module != nil {
		halDesc.Compute = hal.ComputeState{
			Module:     d.Module.hal,
			EntryPoint: d.EntryPoint,
		}
	}

	return halDesc
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
		if ca.View != nil {
			halCA.View = ca.View.hal
		}
		if ca.ResolveTarget != nil {
			halCA.ResolveTarget = ca.ResolveTarget.hal
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
			halDS.View = ds.View.hal
		}
		halDesc.DepthStencilAttachment = halDS
	}

	return halDesc
}

// ComputePassDescriptor describes a compute pass.
type ComputePassDescriptor struct {
	Label string
}

// toHAL converts a ComputePassDescriptor to a hal.ComputePassDescriptor.
func (d *ComputePassDescriptor) toHAL() *hal.ComputePassDescriptor {
	return &hal.ComputePassDescriptor{
		Label: d.Label,
	}
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
	Aspect   TextureAspect
}

func (i *ImageCopyTexture) toHAL() *hal.ImageCopyTexture {
	if i == nil || i.Texture == nil {
		return nil
	}

	return &hal.ImageCopyTexture{
		Texture:  i.Texture.hal,
		MipLevel: i.MipLevel,
		Origin:   i.Origin.toHAL(),
		Aspect:   i.Aspect,
	}
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
		t = b.Texture.hal
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
