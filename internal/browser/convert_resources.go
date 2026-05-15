//go:build js && wasm

package browser

import (
	"syscall/js"

	"github.com/gogpu/gputypes"
)

// newJSObject creates a new empty JavaScript object.
func newJSObject() js.Value {
	return js.Global().Get("Object").New()
}

// newJSArray creates a new empty JavaScript array.
func newJSArray() js.Value {
	return js.Global().Get("Array").New()
}

// --- Buffer descriptor ---

// BuildBufferDescriptor constructs a JS GPUBufferDescriptor object.
func BuildBufferDescriptor(label string, size uint64, usage uint64, mappedAtCreation bool) js.Value {
	desc := newJSObject()
	if label != "" {
		desc.Set("label", label)
	}
	desc.Set("size", float64(size))
	desc.Set("usage", float64(usage))
	if mappedAtCreation {
		desc.Set("mappedAtCreation", true)
	}
	return desc
}

// --- Texture descriptor ---

// BuildTextureDescriptor constructs a JS GPUTextureDescriptor object.
func BuildTextureDescriptor(
	label string,
	width, height, depthOrArrayLayers uint32,
	mipLevelCount, sampleCount uint32,
	dimension gputypes.TextureDimension,
	format gputypes.TextureFormat,
	usage gputypes.TextureUsage,
	viewFormats []gputypes.TextureFormat,
) js.Value {
	desc := newJSObject()
	if label != "" {
		desc.Set("label", label)
	}

	// size: GPUExtent3DDict
	size := newJSObject()
	size.Set("width", width)
	if height > 1 {
		size.Set("height", height)
	}
	if depthOrArrayLayers > 1 {
		size.Set("depthOrArrayLayers", depthOrArrayLayers)
	}
	desc.Set("size", size)

	if mipLevelCount > 1 {
		desc.Set("mipLevelCount", mipLevelCount)
	}
	if sampleCount > 1 {
		desc.Set("sampleCount", sampleCount)
	}

	dimStr := TextureDimensionToJS(dimension)
	if dimStr != "" {
		desc.Set("dimension", dimStr)
	}

	desc.Set("format", TextureFormatToJS(format))
	desc.Set("usage", float64(usage))

	if len(viewFormats) > 0 {
		arr := newJSArray()
		for _, vf := range viewFormats {
			arr.Call("push", TextureFormatToJS(vf))
		}
		desc.Set("viewFormats", arr)
	}

	return desc
}

// BuildTextureViewDescriptor constructs a JS GPUTextureViewDescriptor object.
func BuildTextureViewDescriptor(
	label string,
	format gputypes.TextureFormat,
	dimension gputypes.TextureViewDimension,
	aspect gputypes.TextureAspect,
	baseMipLevel, mipLevelCount uint32,
	baseArrayLayer, arrayLayerCount uint32,
) js.Value {
	desc := newJSObject()
	if label != "" {
		desc.Set("label", label)
	}
	if format != gputypes.TextureFormatUndefined {
		desc.Set("format", TextureFormatToJS(format))
	}
	if dimension != gputypes.TextureViewDimensionUndefined {
		desc.Set("dimension", TextureViewDimensionToJS(dimension))
	}
	aspectStr := TextureAspectToJS(aspect)
	if aspectStr != "" {
		desc.Set("aspect", aspectStr)
	}
	if baseMipLevel > 0 {
		desc.Set("baseMipLevel", baseMipLevel)
	}
	if mipLevelCount > 0 {
		desc.Set("mipLevelCount", mipLevelCount)
	}
	if baseArrayLayer > 0 {
		desc.Set("baseArrayLayer", baseArrayLayer)
	}
	if arrayLayerCount > 0 {
		desc.Set("arrayLayerCount", arrayLayerCount)
	}
	return desc
}

// --- Sampler descriptor ---

// BuildSamplerDescriptor constructs a JS GPUSamplerDescriptor object.
func BuildSamplerDescriptor(
	label string,
	addressModeU, addressModeV, addressModeW gputypes.AddressMode,
	magFilter, minFilter gputypes.FilterMode,
	mipmapFilter gputypes.FilterMode,
	lodMinClamp, lodMaxClamp float32,
	compare gputypes.CompareFunction,
	maxAnisotropy uint16,
) js.Value {
	desc := newJSObject()
	if label != "" {
		desc.Set("label", label)
	}

	if addressModeU != gputypes.AddressModeUndefined {
		desc.Set("addressModeU", AddressModeToJS(addressModeU))
	}
	if addressModeV != gputypes.AddressModeUndefined {
		desc.Set("addressModeV", AddressModeToJS(addressModeV))
	}
	if addressModeW != gputypes.AddressModeUndefined {
		desc.Set("addressModeW", AddressModeToJS(addressModeW))
	}

	if magFilter != gputypes.FilterModeUndefined {
		desc.Set("magFilter", FilterModeToJS(magFilter))
	}
	if minFilter != gputypes.FilterModeUndefined {
		desc.Set("minFilter", FilterModeToJS(minFilter))
	}
	if mipmapFilter != gputypes.FilterModeUndefined {
		desc.Set("mipmapFilter", FilterModeToJS(mipmapFilter))
	}

	if lodMinClamp != 0 {
		desc.Set("lodMinClamp", lodMinClamp)
	}
	if lodMaxClamp != 0 {
		desc.Set("lodMaxClamp", lodMaxClamp)
	}

	if compare != gputypes.CompareFunctionUndefined {
		desc.Set("compare", CompareFunctionToJS(compare))
	}

	if maxAnisotropy > 1 {
		desc.Set("maxAnisotropy", maxAnisotropy)
	}

	return desc
}

// --- Shader module descriptor ---

// BuildShaderModuleDescriptor constructs a JS GPUShaderModuleDescriptor object.
// On browser, WGSL code goes directly to the browser's createShaderModule.
func BuildShaderModuleDescriptor(label string, code string) js.Value {
	desc := newJSObject()
	if label != "" {
		desc.Set("label", label)
	}
	desc.Set("code", code)
	return desc
}

// --- Bind group layout descriptor ---

// BuildBindGroupLayoutDescriptor constructs a JS GPUBindGroupLayoutDescriptor object.
func BuildBindGroupLayoutDescriptor(
	label string,
	entries []BindGroupLayoutEntryJS,
) js.Value {
	desc := newJSObject()
	if label != "" {
		desc.Set("label", label)
	}

	arr := newJSArray()
	for _, e := range entries {
		arr.Call("push", e.ToJS())
	}
	desc.Set("entries", arr)
	return desc
}

// BindGroupLayoutEntryJS holds the data needed to build a single
// GPUBindGroupLayoutEntry JS object.
type BindGroupLayoutEntryJS struct {
	Binding    uint32
	Visibility uint32 // ShaderStages bitmask

	// Exactly one of these should be non-nil.
	Buffer         *BufferBindingLayoutJS
	Sampler        *SamplerBindingLayoutJS
	Texture        *TextureBindingLayoutJS
	StorageTexture *StorageTextureBindingLayoutJS
}

// ToJS converts to a JS object.
func (e *BindGroupLayoutEntryJS) ToJS() js.Value {
	obj := newJSObject()
	obj.Set("binding", e.Binding)
	obj.Set("visibility", e.Visibility)

	if e.Buffer != nil {
		obj.Set("buffer", e.Buffer.ToJS())
	}
	if e.Sampler != nil {
		obj.Set("sampler", e.Sampler.ToJS())
	}
	if e.Texture != nil {
		obj.Set("texture", e.Texture.ToJS())
	}
	if e.StorageTexture != nil {
		obj.Set("storageTexture", e.StorageTexture.ToJS())
	}
	return obj
}

// BufferBindingLayoutJS holds buffer binding layout data.
type BufferBindingLayoutJS struct {
	Type             string // "uniform", "storage", "read-only-storage"
	HasDynamicOffset bool
	MinBindingSize   uint64
}

// ToJS converts to a JS object.
func (b *BufferBindingLayoutJS) ToJS() js.Value {
	obj := newJSObject()
	if b.Type != "" {
		obj.Set("type", b.Type)
	}
	if b.HasDynamicOffset {
		obj.Set("hasDynamicOffset", true)
	}
	if b.MinBindingSize > 0 {
		obj.Set("minBindingSize", float64(b.MinBindingSize))
	}
	return obj
}

// SamplerBindingLayoutJS holds sampler binding layout data.
type SamplerBindingLayoutJS struct {
	Type string // "filtering", "non-filtering", "comparison"
}

// ToJS converts to a JS object.
func (s *SamplerBindingLayoutJS) ToJS() js.Value {
	obj := newJSObject()
	if s.Type != "" {
		obj.Set("type", s.Type)
	}
	return obj
}

// TextureBindingLayoutJS holds texture binding layout data.
type TextureBindingLayoutJS struct {
	SampleType    string // "float", "unfilterable-float", "depth", "sint", "uint"
	ViewDimension string // "2d", "cube", etc.
	Multisampled  bool
}

// ToJS converts to a JS object.
func (t *TextureBindingLayoutJS) ToJS() js.Value {
	obj := newJSObject()
	if t.SampleType != "" {
		obj.Set("sampleType", t.SampleType)
	}
	if t.ViewDimension != "" {
		obj.Set("viewDimension", t.ViewDimension)
	}
	if t.Multisampled {
		obj.Set("multisampled", true)
	}
	return obj
}

// StorageTextureBindingLayoutJS holds storage texture binding layout data.
type StorageTextureBindingLayoutJS struct {
	Access        string // "write-only", "read-only", "read-write"
	Format        string // texture format string
	ViewDimension string
}

// ToJS converts to a JS object.
func (s *StorageTextureBindingLayoutJS) ToJS() js.Value {
	obj := newJSObject()
	if s.Access != "" {
		obj.Set("access", s.Access)
	}
	if s.Format != "" {
		obj.Set("format", s.Format)
	}
	if s.ViewDimension != "" {
		obj.Set("viewDimension", s.ViewDimension)
	}
	return obj
}

// --- Bind group descriptor ---

// BindGroupEntryJS holds a single bind group entry for JS conversion.
type BindGroupEntryJS struct {
	Binding uint32
	// Exactly one of these should be set.
	BufferRef      js.Value // GPUBuffer ref
	BufferOffset   uint64
	BufferSize     uint64
	SamplerRef     js.Value // GPUSampler ref
	TextureViewRef js.Value // GPUTextureView ref
}

// BuildBindGroupDescriptor constructs a JS GPUBindGroupDescriptor object.
func BuildBindGroupDescriptor(
	label string,
	layoutRef js.Value,
	entries []BindGroupEntryJS,
) js.Value {
	desc := newJSObject()
	if label != "" {
		desc.Set("label", label)
	}
	desc.Set("layout", layoutRef)

	arr := newJSArray()
	for _, e := range entries {
		entry := newJSObject()
		entry.Set("binding", e.Binding)

		if !e.BufferRef.IsUndefined() && !e.BufferRef.IsNull() {
			resource := newJSObject()
			resource.Set("buffer", e.BufferRef)
			if e.BufferOffset > 0 {
				resource.Set("offset", float64(e.BufferOffset))
			}
			if e.BufferSize > 0 {
				resource.Set("size", float64(e.BufferSize))
			}
			entry.Set("resource", resource)
		} else if !e.SamplerRef.IsUndefined() && !e.SamplerRef.IsNull() {
			entry.Set("resource", e.SamplerRef)
		} else if !e.TextureViewRef.IsUndefined() && !e.TextureViewRef.IsNull() {
			entry.Set("resource", e.TextureViewRef)
		}

		arr.Call("push", entry)
	}
	desc.Set("entries", arr)
	return desc
}

// --- Pipeline layout descriptor ---

// BuildPipelineLayoutDescriptor constructs a JS GPUPipelineLayoutDescriptor.
func BuildPipelineLayoutDescriptor(label string, layoutRefs []js.Value) js.Value {
	desc := newJSObject()
	if label != "" {
		desc.Set("label", label)
	}

	arr := newJSArray()
	for _, ref := range layoutRefs {
		arr.Call("push", ref)
	}
	desc.Set("bindGroupLayouts", arr)
	return desc
}

// --- Render pipeline descriptor ---

// BuildRenderPipelineDescriptor constructs a JS GPURenderPipelineDescriptor.
func BuildRenderPipelineDescriptor(desc *RenderPipelineDescriptorJS) js.Value {
	obj := newJSObject()
	if desc.Label != "" {
		obj.Set("label", desc.Label)
	}

	// layout: "auto" or GPUPipelineLayout reference.
	if desc.LayoutRef.IsUndefined() || desc.LayoutRef.IsNull() {
		obj.Set("layout", "auto")
	} else {
		obj.Set("layout", desc.LayoutRef)
	}

	// vertex state (required)
	obj.Set("vertex", desc.Vertex.ToJS())

	// primitive state
	if desc.Primitive != nil {
		obj.Set("primitive", desc.Primitive.ToJS())
	}

	// depth-stencil state
	if desc.DepthStencil != nil {
		obj.Set("depthStencil", desc.DepthStencil.ToJS())
	}

	// multisample state
	if desc.Multisample != nil {
		obj.Set("multisample", desc.Multisample.ToJS())
	}

	// fragment state
	if desc.Fragment != nil {
		obj.Set("fragment", desc.Fragment.ToJS())
	}

	return obj
}

// RenderPipelineDescriptorJS holds render pipeline creation data.
type RenderPipelineDescriptorJS struct {
	Label        string
	LayoutRef    js.Value // GPUPipelineLayout or js.Undefined() for "auto"
	Vertex       VertexStateJS
	Primitive    *PrimitiveStateJS
	DepthStencil *DepthStencilStateJS
	Multisample  *MultisampleStateJS
	Fragment     *FragmentStateJS
}

// VertexStateJS holds vertex shader state for JS.
type VertexStateJS struct {
	ModuleRef  js.Value // GPUShaderModule
	EntryPoint string
	Buffers    []VertexBufferLayoutJS
}

// ToJS converts to a JS object.
func (v *VertexStateJS) ToJS() js.Value {
	obj := newJSObject()
	obj.Set("module", v.ModuleRef)
	if v.EntryPoint != "" {
		obj.Set("entryPoint", v.EntryPoint)
	}

	if len(v.Buffers) > 0 {
		arr := newJSArray()
		for _, buf := range v.Buffers {
			arr.Call("push", buf.ToJS())
		}
		obj.Set("buffers", arr)
	}
	return obj
}

// VertexBufferLayoutJS holds a vertex buffer layout for JS.
type VertexBufferLayoutJS struct {
	ArrayStride uint64
	StepMode    string // "vertex" or "instance"
	Attributes  []VertexAttributeJS
}

// ToJS converts to a JS object.
func (l *VertexBufferLayoutJS) ToJS() js.Value {
	obj := newJSObject()
	obj.Set("arrayStride", float64(l.ArrayStride))
	if l.StepMode != "" {
		obj.Set("stepMode", l.StepMode)
	}

	attrs := newJSArray()
	for _, a := range l.Attributes {
		attr := newJSObject()
		attr.Set("format", a.Format)
		attr.Set("offset", float64(a.Offset))
		attr.Set("shaderLocation", a.ShaderLocation)
		attrs.Call("push", attr)
	}
	obj.Set("attributes", attrs)
	return obj
}

// VertexAttributeJS holds a vertex attribute for JS.
type VertexAttributeJS struct {
	Format         string // e.g. "float32x2"
	Offset         uint64
	ShaderLocation uint32
}

// PrimitiveStateJS holds primitive assembly state for JS.
type PrimitiveStateJS struct {
	Topology         string // "triangle-list", etc.
	StripIndexFormat string // "uint16", "uint32" (only for strip topologies)
	FrontFace        string // "ccw", "cw"
	CullMode         string // "none", "front", "back"
	UnclippedDepth   bool
}

// ToJS converts to a JS object.
func (p *PrimitiveStateJS) ToJS() js.Value {
	obj := newJSObject()
	if p.Topology != "" {
		obj.Set("topology", p.Topology)
	}
	if p.StripIndexFormat != "" {
		obj.Set("stripIndexFormat", p.StripIndexFormat)
	}
	if p.FrontFace != "" {
		obj.Set("frontFace", p.FrontFace)
	}
	if p.CullMode != "" {
		obj.Set("cullMode", p.CullMode)
	}
	if p.UnclippedDepth {
		obj.Set("unclippedDepth", true)
	}
	return obj
}

// DepthStencilStateJS holds depth-stencil state for JS.
type DepthStencilStateJS struct {
	Format              string
	DepthWriteEnabled   bool
	DepthCompare        string
	StencilFront        *StencilFaceStateJS
	StencilBack         *StencilFaceStateJS
	StencilReadMask     uint32
	StencilWriteMask    uint32
	DepthBias           int32
	DepthBiasSlopeScale float32
	DepthBiasClamp      float32
}

// ToJS converts to a JS object.
func (d *DepthStencilStateJS) ToJS() js.Value {
	obj := newJSObject()
	obj.Set("format", d.Format)
	obj.Set("depthWriteEnabled", d.DepthWriteEnabled)
	if d.DepthCompare != "" {
		obj.Set("depthCompare", d.DepthCompare)
	}
	if d.StencilFront != nil {
		obj.Set("stencilFront", d.StencilFront.ToJS())
	}
	if d.StencilBack != nil {
		obj.Set("stencilBack", d.StencilBack.ToJS())
	}
	if d.StencilReadMask != 0 {
		obj.Set("stencilReadMask", d.StencilReadMask)
	}
	if d.StencilWriteMask != 0 {
		obj.Set("stencilWriteMask", d.StencilWriteMask)
	}
	if d.DepthBias != 0 {
		obj.Set("depthBias", d.DepthBias)
	}
	if d.DepthBiasSlopeScale != 0 {
		obj.Set("depthBiasSlopeScale", d.DepthBiasSlopeScale)
	}
	if d.DepthBiasClamp != 0 {
		obj.Set("depthBiasClamp", d.DepthBiasClamp)
	}
	return obj
}

// StencilFaceStateJS holds stencil operations for a face.
type StencilFaceStateJS struct {
	Compare     string
	FailOp      string
	DepthFailOp string
	PassOp      string
}

// ToJS converts to a JS object.
func (s *StencilFaceStateJS) ToJS() js.Value {
	obj := newJSObject()
	if s.Compare != "" {
		obj.Set("compare", s.Compare)
	}
	if s.FailOp != "" {
		obj.Set("failOp", s.FailOp)
	}
	if s.DepthFailOp != "" {
		obj.Set("depthFailOp", s.DepthFailOp)
	}
	if s.PassOp != "" {
		obj.Set("passOp", s.PassOp)
	}
	return obj
}

// MultisampleStateJS holds multisample state for JS.
type MultisampleStateJS struct {
	Count                  uint32
	Mask                   uint64
	AlphaToCoverageEnabled bool
}

// ToJS converts to a JS object.
func (m *MultisampleStateJS) ToJS() js.Value {
	obj := newJSObject()
	if m.Count > 0 {
		obj.Set("count", m.Count)
	}
	if m.Mask > 0 {
		obj.Set("mask", float64(m.Mask))
	}
	if m.AlphaToCoverageEnabled {
		obj.Set("alphaToCoverageEnabled", true)
	}
	return obj
}

// FragmentStateJS holds fragment shader state for JS.
type FragmentStateJS struct {
	ModuleRef  js.Value // GPUShaderModule
	EntryPoint string
	Targets    []ColorTargetStateJS
}

// ToJS converts to a JS object.
func (f *FragmentStateJS) ToJS() js.Value {
	obj := newJSObject()
	obj.Set("module", f.ModuleRef)
	if f.EntryPoint != "" {
		obj.Set("entryPoint", f.EntryPoint)
	}

	targets := newJSArray()
	for _, t := range f.Targets {
		targets.Call("push", t.ToJS())
	}
	obj.Set("targets", targets)
	return obj
}

// ColorTargetStateJS holds a color target for JS.
type ColorTargetStateJS struct {
	Format    string
	Blend     *BlendStateJS
	WriteMask uint32
}

// ToJS converts to a JS object.
func (c *ColorTargetStateJS) ToJS() js.Value {
	obj := newJSObject()
	obj.Set("format", c.Format)
	if c.Blend != nil {
		obj.Set("blend", c.Blend.ToJS())
	}
	if c.WriteMask > 0 {
		obj.Set("writeMask", c.WriteMask)
	}
	return obj
}

// BlendStateJS holds blend state for JS.
type BlendStateJS struct {
	Color BlendComponentJS
	Alpha BlendComponentJS
}

// ToJS converts to a JS object.
func (b *BlendStateJS) ToJS() js.Value {
	obj := newJSObject()
	obj.Set("color", b.Color.ToJS())
	obj.Set("alpha", b.Alpha.ToJS())
	return obj
}

// BlendComponentJS holds a blend component for JS.
type BlendComponentJS struct {
	SrcFactor string
	DstFactor string
	Operation string
}

// ToJS converts to a JS object.
func (bc *BlendComponentJS) ToJS() js.Value {
	obj := newJSObject()
	if bc.SrcFactor != "" {
		obj.Set("srcFactor", bc.SrcFactor)
	}
	if bc.DstFactor != "" {
		obj.Set("dstFactor", bc.DstFactor)
	}
	if bc.Operation != "" {
		obj.Set("operation", bc.Operation)
	}
	return obj
}

// --- Command encoder descriptor ---

// BuildCommandEncoderDescriptor constructs a JS GPUCommandEncoderDescriptor.
func BuildCommandEncoderDescriptor(label string) js.Value {
	desc := newJSObject()
	if label != "" {
		desc.Set("label", label)
	}
	return desc
}

// --- Render pass descriptor ---

// RenderPassColorAttachmentJS holds data for building a GPURenderPassColorAttachment.
type RenderPassColorAttachmentJS struct {
	View          js.Value // GPUTextureView
	ResolveTarget js.Value // GPUTextureView or js.Undefined()
	LoadOp        string   // "load" or "clear"
	StoreOp       string   // "store" or "discard"
	ClearR        float64
	ClearG        float64
	ClearB        float64
	ClearA        float64
}

// RenderPassDepthStencilAttachmentJS holds data for building a
// GPURenderPassDepthStencilAttachment.
type RenderPassDepthStencilAttachmentJS struct {
	View              js.Value // GPUTextureView
	DepthLoadOp       string   // "load" or "clear"
	DepthStoreOp      string   // "store" or "discard"
	DepthClearValue   float32
	DepthReadOnly     bool
	StencilLoadOp     string // "load" or "clear"
	StencilStoreOp    string // "store" or "discard"
	StencilClearValue uint32
	StencilReadOnly   bool
}

// BuildRenderPassDescriptor constructs a JS GPURenderPassDescriptor.
//
// Matches Rust wgpu's begin_render_pass which builds GpuRenderPassDescriptor
// with color attachments array and optional depth-stencil attachment.
func BuildRenderPassDescriptor(
	label string,
	colorAttachments []RenderPassColorAttachmentJS,
	depthStencil *RenderPassDepthStencilAttachmentJS,
) js.Value {
	// Build color attachments array.
	arr := newJSArray()
	for _, ca := range colorAttachments {
		att := newJSObject()
		att.Set("view", ca.View)
		att.Set("loadOp", ca.LoadOp)
		att.Set("storeOp", ca.StoreOp)

		// Set clear value when loadOp is "clear".
		if ca.LoadOp == "clear" {
			clearColor := newJSObject()
			clearColor.Set("r", ca.ClearR)
			clearColor.Set("g", ca.ClearG)
			clearColor.Set("b", ca.ClearB)
			clearColor.Set("a", ca.ClearA)
			att.Set("clearValue", clearColor)
		}

		if !ca.ResolveTarget.IsUndefined() && !ca.ResolveTarget.IsNull() {
			att.Set("resolveTarget", ca.ResolveTarget)
		}

		arr.Call("push", att)
	}

	desc := newJSObject()
	if label != "" {
		desc.Set("label", label)
	}
	desc.Set("colorAttachments", arr)

	// Depth-stencil attachment (optional).
	if depthStencil != nil {
		ds := newJSObject()
		ds.Set("view", depthStencil.View)

		if !depthStencil.DepthReadOnly {
			ds.Set("depthLoadOp", depthStencil.DepthLoadOp)
			ds.Set("depthStoreOp", depthStencil.DepthStoreOp)
			if depthStencil.DepthLoadOp == "clear" {
				ds.Set("depthClearValue", depthStencil.DepthClearValue)
			}
		}
		ds.Set("depthReadOnly", depthStencil.DepthReadOnly)

		if !depthStencil.StencilReadOnly {
			ds.Set("stencilLoadOp", depthStencil.StencilLoadOp)
			ds.Set("stencilStoreOp", depthStencil.StencilStoreOp)
			if depthStencil.StencilLoadOp == "clear" {
				ds.Set("stencilClearValue", depthStencil.StencilClearValue)
			}
		}
		ds.Set("stencilReadOnly", depthStencil.StencilReadOnly)

		desc.Set("depthStencilAttachment", ds)
	}

	return desc
}

// --- Compute pass descriptor ---

// BuildComputePassDescriptor constructs a JS GPUComputePassDescriptor.
func BuildComputePassDescriptor(label string) js.Value {
	desc := newJSObject()
	if label != "" {
		desc.Set("label", label)
	}
	return desc
}

// --- Image copy descriptors (for copyBufferToTexture / copyTextureToBuffer) ---

// BuildImageCopyBuffer constructs a JS GPUTexelCopyBufferInfo (formerly GPUImageCopyBuffer).
func BuildImageCopyBuffer(buffer js.Value, offset uint64, bytesPerRow, rowsPerImage uint32) js.Value {
	obj := newJSObject()
	obj.Set("buffer", buffer)
	if offset > 0 {
		obj.Set("offset", float64(offset))
	}
	if bytesPerRow > 0 {
		obj.Set("bytesPerRow", bytesPerRow)
	}
	if rowsPerImage > 0 {
		obj.Set("rowsPerImage", rowsPerImage)
	}
	return obj
}

// BuildImageCopyTexture constructs a JS GPUTexelCopyTextureInfo (formerly GPUImageCopyTexture).
func BuildImageCopyTexture(texture js.Value, mipLevel uint32, originX, originY, originZ uint32) js.Value {
	obj := newJSObject()
	obj.Set("texture", texture)
	if mipLevel > 0 {
		obj.Set("mipLevel", mipLevel)
	}
	if originX > 0 || originY > 0 || originZ > 0 {
		origin := newJSObject()
		origin.Set("x", originX)
		origin.Set("y", originY)
		origin.Set("z", originZ)
		obj.Set("origin", origin)
	}
	return obj
}

// BuildExtent3D constructs a JS GPUExtent3DDict.
func BuildExtent3D(width, height, depthOrArrayLayers uint32) js.Value {
	obj := newJSObject()
	obj.Set("width", width)
	if height > 0 {
		obj.Set("height", height)
	}
	if depthOrArrayLayers > 0 {
		obj.Set("depthOrArrayLayers", depthOrArrayLayers)
	}
	return obj
}

// BuildColorDict constructs a JS GPUColorDict { r, g, b, a }.
func BuildColorDict(r, g, b, a float64) js.Value {
	obj := newJSObject()
	obj.Set("r", r)
	obj.Set("g", g)
	obj.Set("b", b)
	obj.Set("a", a)
	return obj
}

// BuildTexelCopyBufferLayout constructs a JS GPUTexelCopyBufferLayout
// (formerly GPUImageDataLayout).
func BuildTexelCopyBufferLayout(offset uint64, bytesPerRow, rowsPerImage uint32) js.Value {
	obj := newJSObject()
	if offset > 0 {
		obj.Set("offset", float64(offset))
	}
	if bytesPerRow > 0 {
		obj.Set("bytesPerRow", bytesPerRow)
	}
	if rowsPerImage > 0 {
		obj.Set("rowsPerImage", rowsPerImage)
	}
	return obj
}

// --- Compute pipeline descriptor ---

// BuildComputePipelineDescriptor constructs a JS GPUComputePipelineDescriptor.
func BuildComputePipelineDescriptor(
	label string,
	layoutRef js.Value,
	moduleRef js.Value,
	entryPoint string,
) js.Value {
	desc := newJSObject()
	if label != "" {
		desc.Set("label", label)
	}

	if layoutRef.IsUndefined() || layoutRef.IsNull() {
		desc.Set("layout", "auto")
	} else {
		desc.Set("layout", layoutRef)
	}

	compute := newJSObject()
	compute.Set("module", moduleRef)
	if entryPoint != "" {
		compute.Set("entryPoint", entryPoint)
	}
	desc.Set("compute", compute)

	return desc
}
