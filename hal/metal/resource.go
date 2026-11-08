// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build darwin && !(js && wasm)

package metal

import (
	"unsafe"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/naga/ir"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/metal/mslmap"
	"github.com/gogpu/wgpu/internal/shaderir"
)

// Buffer implements hal.Buffer for Metal.
type Buffer struct {
	raw     ID // id<MTLBuffer>
	size    uint64
	usage   gputypes.BufferUsage
	options MTLResourceOptions
	device  *Device
}

// Destroy releases the buffer.
func (b *Buffer) Destroy() {
	if b.device != nil {
		b.device.DestroyBuffer(b)
	}
}

// NativeHandle returns the raw MTLBuffer handle.
func (b *Buffer) NativeHandle() uintptr { return uintptr(b.raw) }

// Contents returns the buffer contents pointer (for mapped buffers).
// Returns unsafe.Pointer to allow safe pointer arithmetic via unsafe.Add
// without triggering go vet "possible misuse of unsafe.Pointer" warnings.
func (b *Buffer) Contents() unsafe.Pointer {
	if b.raw == 0 {
		return nil
	}
	return unsafe.Pointer(MsgSend(b.raw, Sel("contents"))) //nolint:govet // ObjC FFI: pointer from Metal runtime, not Go heap
}

// Texture implements hal.Texture for Metal.
type Texture struct {
	raw        ID // id<MTLTexture>
	format     gputypes.TextureFormat
	width      uint32
	height     uint32
	depth      uint32
	mipLevels  uint32
	samples    uint32
	dimension  gputypes.TextureDimension
	usage      gputypes.TextureUsage
	device     *Device
	isExternal bool
	// isShared is true when this texture was created with MTLStorageModeShared
	// (Apple Silicon UMA only). Shared textures support direct CPU writes via
	// replaceRegion: and honor setPurgeableState(empty) for immediate OS reclaim.
	isShared bool
}

// IsHostWritable reports whether this texture was allocated with Shared storage,
// meaning the CPU can write to it directly without a staging buffer or GPU blit.
// Used by pendingWrites to bypass the staging belt on Apple Silicon (UMA).
func (t *Texture) IsHostWritable() bool { return t.isShared }

// CurrentUsage returns 0 — Metal has no explicit resource state tracking.
func (t *Texture) CurrentUsage() gputypes.TextureUsage { return 0 }
func (t *Texture) AddPendingRef()                      {}
func (t *Texture) DecPendingRef()                      {}

// Destroy releases the texture.
func (t *Texture) Destroy() {
	if t.device != nil {
		t.device.DestroyTexture(t)
	}
}

// NativeHandle returns the raw MTLTexture handle.
func (t *Texture) NativeHandle() uintptr { return uintptr(t.raw) }

// TextureView implements hal.TextureView for Metal.
type TextureView struct {
	raw     ID // id<MTLTexture>
	texture *Texture
	device  *Device
}

// Destroy releases the texture view.
func (v *TextureView) Destroy() {
	if v.device != nil {
		v.device.DestroyTextureView(v)
	}
}

// NativeHandle returns the raw MTLTexture handle (view is also a texture).
func (v *TextureView) NativeHandle() uintptr { return uintptr(v.raw) }

// Sampler implements hal.Sampler for Metal.
type Sampler struct {
	raw    ID // id<MTLSamplerState>
	device *Device
}

// Destroy releases the sampler.
func (s *Sampler) Destroy() {
	if s.device != nil {
		s.device.DestroySampler(s)
	}
}

// NativeHandle returns the raw MTLSamplerState handle.
func (s *Sampler) NativeHandle() uintptr { return uintptr(s.raw) }

// ShaderModule implements hal.ShaderModule for Metal.
//
// WGSL is parsed and lowered to naga IR here. MSL is written later, when the
// pipeline is created, because the buffer slot for naga's `_buffer_sizes`
// argument comes from the pipeline layout.
//
// Reference: Rust wgpu-hal metal/device.rs:138-145 (load_shader takes
// `layout: &super::PipelineLayout`).
type ShaderModule struct {
	source         hal.ShaderSource
	irModule       *shaderir.Module // nil unless the module was created from WGSL
	device         *Device
	workgroupSizes map[string][3]uint32 // entry point name -> workgroup size; not final for a module with overrides (see compiledLibrary)
}

// Destroy releases the shader module.
func (m *ShaderModule) Destroy() {
	if m.device != nil {
		m.device.DestroyShaderModule(m)
	}
}

// Pipeline stages a Metal pipeline layout lays its bindings out for. Each has
// its own argument tables, numbered over the bindings visible to it.
const (
	stageVertex = iota
	stageFragment
	stageCompute
	stageCount
)

// stageVisibility holds the visibility bit of each pipeline stage.
var stageVisibility = [stageCount]gputypes.ShaderStages{
	stageVertex:   gputypes.ShaderStageVertex,
	stageFragment: gputypes.ShaderStageFragment,
	stageCompute:  gputypes.ShaderStageCompute,
}

// BindGroupLayout implements hal.BindGroupLayout for Metal.
type BindGroupLayout struct {
	// entries are sorted by binding number (mslmap.SortEntries).
	entries []gputypes.BindGroupLayoutEntry
	device  *Device

	// stageSlots holds, per pipeline stage, the slot of each entry counted
	// from the group's first slot in its argument table, or -1 if the entry
	// is not visible to the stage (mslmap.GroupSlots).
	stageSlots [stageCount][]int
}

// Destroy releases the bind group layout.
func (l *BindGroupLayout) Destroy() {
	if l.device != nil {
		l.device.DestroyBindGroupLayout(l)
	}
}

// BindGroup implements hal.BindGroup for Metal.
type BindGroup struct {
	layout  *BindGroupLayout
	entries []gputypes.BindGroupEntry
	device  *Device

	// resources holds one resource per layout entry, in binding order.
	resources []bindGroupResource
}

// bindGroupResource is one resource of a bind group with what the encoder
// needs to bind it.
//
// Reference: Rust wgpu-hal metal/mod.rs BindGroup and BufferLikeResource,
// built by metal/device.rs create_bind_group.
type bindGroupResource struct {
	kind mslmap.Kind
	// handle is the id<MTLBuffer>, id<MTLTexture> or id<MTLSamplerState>;
	// zero if the bind group has no resource for the entry.
	handle uintptr
	// offset is the buffer offset of the binding, before the dynamic offset.
	offset uint64
	// dynamicIndex is the index of the binding's dynamic offset among the
	// offsets SetBindGroup gets (binding order), or -1.
	dynamicIndex int
	// binding is the entry's binding number. size is the byte size of a
	// storage buffer binding (sized is true only for those): naga's
	// `_buffer_sizes` argument holds it, and the length and bounds checks of
	// a runtime-sized array come from it.
	binding uint32
	size    uint64
	sized   bool
	// slots holds, per pipeline stage, the BindGroupLayout.stageSlots slot.
	slots [stageCount]int
}

// Destroy releases the bind group.
func (g *BindGroup) Destroy() {
	if g.device != nil {
		g.device.DestroyBindGroup(g)
	}
}

// PipelineLayout implements hal.PipelineLayout for Metal.
type PipelineLayout struct {
	layouts []hal.BindGroupLayout
	device  *Device

	// stages holds, per pipeline stage, the slots its shaders are compiled
	// with and the first slot of each bind group.
	//
	// Reference: Rust wgpu-hal metal/mod.rs PipelineLayout.per_stage_map and
	// bind_group_infos (base_resource_indices).
	stages [stageCount]mslmap.StageLayout
}

// noBindings is the stage layout of a pipeline without a pipeline layout:
// its shaders can use no resources.
var noBindings = mslmap.StageLayout{SizesSlot: -1}

// stage returns the layout of stage s; l may be nil.
func (l *PipelineLayout) stage(s int) *mslmap.StageLayout {
	if l == nil {
		return &noBindings
	}
	return &l.stages[s]
}

// groupBase returns the first slots of bind group index in stage s, or zero
// slots if l is nil or has no such group.
func (l *PipelineLayout) groupBase(s int, index uint32) mslmap.Counts {
	if l == nil || int(index) >= len(l.stages[s].GroupBase) {
		return mslmap.Counts{}
	}
	return l.stages[s].GroupBase[index]
}

// Destroy releases the pipeline layout.
func (l *PipelineLayout) Destroy() {
	if l.device != nil {
		l.device.DestroyPipelineLayout(l)
	}
}

// RenderPipeline implements hal.RenderPipeline for Metal.
type RenderPipeline struct {
	raw           ID // id<MTLRenderPipelineState>
	device        *Device
	layout        *PipelineLayout // for SetBindGroup slot offset lookup
	cullMode      MTLCullMode
	frontFace     MTLWinding
	icbCompatible bool

	// primitiveType is the MTLPrimitiveType every draw with this pipeline
	// passes: Metal takes the topology per draw call, not from the pipeline
	// state (Rust wgpu-hal metal: RenderPipeline.raw_primitive_type).
	primitiveType MTLPrimitiveType

	// vertexSizes and fragmentSizes list, per stage, the bindings whose byte
	// size naga's `_buffer_sizes` argument holds, in naga's field order. A
	// stage without a runtime-sized array has none. vertexSizesSlot and
	// fragmentSizesSlot are the buffer slots each stage was compiled with
	// (mslmap.StageLayout.SizesSlot).
	vertexSizes       []ir.ResourceBinding
	fragmentSizes     []ir.ResourceBinding
	vertexSizesSlot   int
	fragmentSizesSlot int

	depthStencil    ID // id<MTLDepthStencilState>
	depthBias       float32
	depthSlopeScale float32
	depthClamp      float32
}

// Destroy releases the render pipeline.
func (p *RenderPipeline) Destroy() {
	if p.device != nil {
		p.device.DestroyRenderPipeline(p)
	}
}

// ComputePipeline implements hal.ComputePipeline for Metal.
type ComputePipeline struct {
	raw           ID // id<MTLComputePipelineState>
	device        *Device
	layout        *PipelineLayout // for SetBindGroup slot offset lookup
	workgroupSize MTLSize         // workgroup size from shader

	// sizesBindings lists the bindings whose byte size the kernel needs, in the
	// field order that naga uses. It is empty if the kernel has no
	// runtime-sized array. sizesSlot is the buffer slot that the MSL was
	// compiled with (mslmap.StageLayout.SizesSlot).
	sizesBindings []ir.ResourceBinding
	sizesSlot     int
}

// Destroy releases the compute pipeline.
func (p *ComputePipeline) Destroy() {
	if p.device != nil {
		p.device.DestroyComputePipeline(p)
	}
}

// Fence implements hal.Fence for Metal using MTLSharedEvent.
//
// MTLSharedEvent (unlike MTLEvent) exposes signaledValue to the CPU,
// enabling proper blocking waits and non-blocking status queries.
// The GPU updates signaledValue when encodeSignalEvent:value: completes.
type Fence struct {
	event  ID // id<MTLSharedEvent>
	device *Device
}

// Destroy releases the fence.
func (f *Fence) Destroy() {
	if f.device != nil {
		f.device.DestroyFence(f)
	}
}
