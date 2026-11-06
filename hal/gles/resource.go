// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build (windows || linux) && !(js && wasm)

package gles

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/naga/ir"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/gles/gl"
	"github.com/gogpu/wgpu/internal/handletable"
	"github.com/gogpu/wgpu/internal/shaderir"
)

// Surface and SurfaceTexture are defined in platform-specific files (resource_windows.go, resource_linux.go)

// Buffer implements hal.Buffer for OpenGL.
type Buffer struct {
	id     uint32 // GL buffer object ID
	target uint32 // GL_ARRAY_BUFFER, GL_UNIFORM_BUFFER, etc.
	size   uint64
	usage  gputypes.BufferUsage
	glCtx  *gl.Context
	mapped []byte // For mapped buffers
	data   []byte // CPU-side storage for readback (populated by CopyTextureToBuffer)
}

// Destroy releases the buffer.
func (b *Buffer) Destroy() {
	if b.id != 0 && b.glCtx != nil {
		b.glCtx.DeleteBuffers(b.id)
		b.id = 0
	}
}

// NativeHandle returns the GL buffer object ID.
func (b *Buffer) NativeHandle() uintptr { return uintptr(b.id) }

// Texture implements hal.Texture for OpenGL.
type Texture struct {
	id          uint32 // GL texture object ID
	target      uint32 // GL_TEXTURE_2D, GL_TEXTURE_2D_MULTISAMPLE, etc.
	format      gputypes.TextureFormat
	dimension   gputypes.TextureDimension
	size        hal.Extent3D
	mipLevels   uint32
	sampleCount uint32 // 1 for regular textures, >1 for MSAA
	fbo         uint32 // GL framebuffer object ID (0 = no FBO created)
	glCtx       *gl.Context
}

// CurrentUsage returns 0 — GLES has no explicit resource state tracking.
func (t *Texture) CurrentUsage() gputypes.TextureUsage { return 0 }
func (t *Texture) AddPendingRef()                      {}
func (t *Texture) DecPendingRef()                      {}

// Destroy releases the texture and any associated framebuffer object.
func (t *Texture) Destroy() {
	if t.glCtx != nil {
		if t.fbo != 0 {
			t.glCtx.DeleteFramebuffers(t.fbo)
			t.fbo = 0
		}
		if t.id != 0 {
			t.glCtx.DeleteTextures(t.id)
			t.id = 0
		}
	}
}

// NativeHandle returns the GL texture object ID.
func (t *Texture) NativeHandle() uintptr { return uintptr(t.id) }

// TextureView implements hal.TextureView for OpenGL.
type TextureView struct {
	texture    *Texture
	aspect     gputypes.TextureAspect
	baseMip    uint32
	mipCount   uint32
	baseLayer  uint32
	layerCount uint32
	isSurface  bool            // true for default framebuffer (surface texture)
	surfaceTex *SurfaceTexture // non-nil only when isSurface is true

	device       *Device
	nativeHandle uintptr // key in device.views; 0 once destroyed
}

// Destroy drops the view's handle; a GL texture view holds no GL object.
// It may be called more than once.
func (v *TextureView) Destroy() {
	if v.device != nil {
		v.device.views.Remove(v.nativeHandle)
	}
	v.nativeHandle = 0
}

// mipRange returns the view's first mip level and its level count; a count
// of 0 means every level from the first one on.
func (v *TextureView) mipRange() (base, count uint32) {
	base, count = v.baseMip, v.mipCount
	if count == 0 && v.texture != nil && base < v.texture.mipLevels {
		count = v.texture.mipLevels - base
	}
	return base, max(count, 1)
}

// NativeHandle returns the view's opaque per-device handle. A GL texture
// object has no view object, so a bind group needs the view itself (its mip
// level and layer, which a storage texture binds through glBindImageTexture);
// CreateBindGroup resolves the handle back through the device's table
// (newBindGroup). The handle is not the view's address: see package
// handletable.
func (v *TextureView) NativeHandle() uintptr {
	return v.nativeHandle
}

// Sampler implements hal.Sampler for OpenGL using GL sampler objects (GL 3.3+).
type Sampler struct {
	id    uint32 // GL sampler object ID (0 if sampler objects not supported)
	glCtx *gl.Context
}

// Destroy releases the GL sampler object.
func (s *Sampler) Destroy() {
	if s.id != 0 && s.glCtx != nil {
		s.glCtx.DeleteSamplers(s.id)
		s.id = 0
	}
}

// NativeHandle returns the GL sampler object ID.
func (s *Sampler) NativeHandle() uintptr { return uintptr(s.id) }

// ShaderModule implements hal.ShaderModule for OpenGL.
type ShaderModule struct {
	vertexID   uint32 // GL shader object ID for vertex
	fragmentID uint32 // GL shader object ID for fragment
	computeID  uint32 // GL shader object ID for compute
	source     hal.ShaderSource
	irModule   *shaderir.Module // lowered WGSL, specialized per pipeline stage; nil without WGSL
	glCtx      *gl.Context
}

// Destroy releases the shader module.
func (m *ShaderModule) Destroy() {
	if m.vertexID != 0 && m.glCtx != nil {
		m.glCtx.DeleteShader(m.vertexID)
	}
	if m.fragmentID != 0 && m.glCtx != nil {
		m.glCtx.DeleteShader(m.fragmentID)
	}
	if m.computeID != 0 && m.glCtx != nil {
		m.glCtx.DeleteShader(m.computeID)
	}
}

// BindGroupLayout implements hal.BindGroupLayout for OpenGL.
type BindGroupLayout struct {
	entries []gputypes.BindGroupLayoutEntry
}

// Destroy is a no-op for bind group layouts.
func (l *BindGroupLayout) Destroy() {}

// storageTexture returns the storage texture layout of binding, or nil when
// the binding is not a storage texture.
func (l *BindGroupLayout) storageTexture(binding uint32) *gputypes.StorageTextureBindingLayout {
	if l == nil {
		return nil
	}
	for i := range l.entries {
		if l.entries[i].Binding == binding {
			return l.entries[i].StorageTexture
		}
	}
	return nil
}

// sampledTexture returns the sampled texture layout of binding, or nil when
// the binding is not a sampled texture.
func (l *BindGroupLayout) sampledTexture(binding uint32) *gputypes.TextureBindingLayout {
	if l == nil {
		return nil
	}
	for i := range l.entries {
		if l.entries[i].Binding == binding {
			return l.entries[i].Texture
		}
	}
	return nil
}

// sampledViewTarget is the GL target a sampler of a view dimension reads
// through (Rust wgpu-hal gles Texture::target_for_view_dimension, plus the
// multisampled 2D target): 1D collapses to TEXTURE_2D, GLES has no 1D
// textures.
func sampledViewTarget(layout *gputypes.TextureBindingLayout) uint32 {
	switch layout.ViewDimension {
	case gputypes.TextureViewDimension2DArray:
		return gl.TEXTURE_2D_ARRAY
	case gputypes.TextureViewDimensionCube:
		return gl.TEXTURE_CUBE_MAP
	case gputypes.TextureViewDimensionCubeArray:
		return gl.TEXTURE_CUBE_MAP_ARRAY
	case gputypes.TextureViewDimension3D:
		return gl.TEXTURE_3D
	}
	if layout.Multisampled {
		return gl.TEXTURE_2D_MULTISAMPLE
	}
	return gl.TEXTURE_2D
}

// checkSampledView refuses a sampled binding GL cannot express. GL has no
// texture views (glTextureView is GL 4.3 only, never ES): a texture binds
// only through the target it was created with (CreateTexture picks it from
// the size), and a sampler reads all of its layers from layer 0. So the
// view's dimension must need that target, and an array or cube view must
// cover every layer. Rust wgpu-hal gles only logs the target mismatch
// (log_failing_target_heuristics, gfx-rs/wgpu#1574) and samples the wrong
// layers; WebGPU compatibility mode makes both rules validation errors
// (textureBindingViewDimension, compatibility-mode.md item 6). This is a
// deliberate deviation from Rust: a typed refusal at CreateBindGroup is
// better than a bind group that silently samples the wrong texels.
func checkSampledView(binding uint32, view *TextureView, layout *gputypes.TextureBindingLayout) error {
	tex := view.texture
	want := sampledViewTarget(layout)
	if tex.target != want {
		return fmt.Errorf("%w: binding %d: a %v view of a texture of GL target %#x (the %v view needs %#x)",
			hal.ErrTextureViewNotSampleable, binding, layout.ViewDimension, tex.target, layout.ViewDimension, want)
	}
	if tex.target != gl.TEXTURE_2D_ARRAY && tex.target != gl.TEXTURE_CUBE_MAP && tex.target != gl.TEXTURE_CUBE_MAP_ARRAY {
		return nil
	}
	layers := tex.size.DepthOrArrayLayers
	count := view.layerCount
	if count == 0 && view.baseLayer < layers {
		count = layers - view.baseLayer
	}
	if view.baseLayer != 0 || count != layers {
		return fmt.Errorf("%w: binding %d: layers %d..%d of a %d-layer texture (a sampled view must cover every layer)",
			hal.ErrTextureViewNotSampleable, binding, view.baseLayer, view.baseLayer+count, layers)
	}
	return nil
}

// dynamicOffsetIndex returns the index of binding's offset in the dynamic
// offsets of SetBindGroup. WebGPU orders the offsets by binding number,
// whatever the order of the layout or bind group entries (Rust wgpu-hal
// consumes them over the layout entries, which wgpu-core sorts by binding),
// so the index is the number of dynamic-offset buffer bindings below binding.
func (l *BindGroupLayout) dynamicOffsetIndex(binding uint32) int {
	n := 0
	for i := range l.entries {
		e := &l.entries[i]
		if e.Binding < binding && e.Buffer != nil && e.Buffer.HasDynamicOffset {
			n++
		}
	}
	return n
}

// BindGroup implements hal.BindGroup for OpenGL.
type BindGroup struct {
	layout  *BindGroupLayout
	entries []gputypes.BindGroupEntry
	// views holds, index for index with entries, the texture view of each
	// TextureViewBinding entry (nil for other entries and surface views).
	views []*TextureView
}

// newBindGroup resolves the texture views of entries from their handles
// (TextureView.NativeHandle) through the device's view table. An unknown or
// destroyed handle is an error, and so is a sampled view GL cannot bind
// (checkSampledView).
func newBindGroup(views *handletable.Table[TextureView], layout *BindGroupLayout, entries []gputypes.BindGroupEntry) (*BindGroup, error) {
	resolved := make([]*TextureView, len(entries))
	for i, entry := range entries {
		res, ok := entry.Resource.(gputypes.TextureViewBinding)
		if !ok || res.TextureView == 0 {
			continue
		}
		view, ok := views.Get(res.TextureView)
		if !ok {
			return nil, fmt.Errorf("gles: binding %d: unknown texture view handle %#x", entry.Binding, res.TextureView)
		}
		if view.texture == nil {
			continue
		}
		if sampled := layout.sampledTexture(entry.Binding); sampled != nil {
			if err := checkSampledView(entry.Binding, view, sampled); err != nil {
				return nil, err
			}
		}
		resolved[i] = view
	}
	return &BindGroup{layout: layout, entries: entries, views: resolved}, nil
}

// Destroy is a no-op for bind groups.
func (g *BindGroup) Destroy() {}

// BindGroupLayoutInfo stores per-group binding-to-slot mapping computed at
// PipelineLayout creation time. Matches Rust wgpu-hal BindGroupLayoutInfo.
// Each entry in BindingToSlot is indexed by the WGSL binding number and maps
// to the sequential GL slot index for that resource type. 0xFF means unused.
type BindGroupLayoutInfo struct {
	BindingToSlot []uint8 // indexed by binding number, 0xFF = unused
}

// PipelineLayout implements hal.PipelineLayout for OpenGL.
// Stores pre-computed per-type sequential binding indices (Rust wgpu pattern).
// The BindingMap is used by naga GLSL writer, GroupInfos by SetBindGroup at runtime.
type PipelineLayout struct {
	bindGroupLayouts []*BindGroupLayout
	// groupInfos stores per-group binding-to-slot tables computed from per-type
	// sequential counters (samplers, textures, images, uniform buffers, storage buffers).
	// Matches Rust wgpu-hal/src/gles/mod.rs BindGroupLayoutInfo.
	groupInfos []BindGroupLayoutInfo
	// bindingMap maps (group, binding) to flat GL slot index for naga GLSL writer.
	// Computed simultaneously with groupInfos in CreatePipelineLayout.
	bindingMap map[ir.ResourceBinding]uint8
}

// Destroy is a no-op for pipeline layouts.
func (l *PipelineLayout) Destroy() {}

// ColorTargetDesc describes per-render-target blend and write mask state.
// Matches Rust wgpu-hal GLES ColorTargetDesc (mod.rs:770-773).
type ColorTargetDesc struct {
	Blend     *gputypes.BlendState
	WriteMask gputypes.ColorWriteMask
}

// RenderPipeline implements hal.RenderPipeline for OpenGL.
type RenderPipeline struct {
	programID uint32 // GL program object ID
	layout    *PipelineLayout
	glCtx     *gl.Context

	// Pipeline state
	primitiveTopology gputypes.PrimitiveTopology
	cullMode          gputypes.CullMode
	frontFace         gputypes.FrontFace
	depthStencil      *hal.DepthStencilState
	multisample       gputypes.MultisampleState

	// Per-target blend and write mask state for MRT.
	// Matches Rust wgpu-hal GLES RenderPipeline.color_targets (mod.rs:798).
	// When len == 1, uniform blend/mask is applied globally.
	// When len > 1, indexed GL calls are used if available (GLES 3.2 / GL 4.0).
	colorTargets []ColorTargetDesc

	// Vertex buffer layouts from the pipeline descriptor.
	// OpenGL requires explicit glVertexAttribPointer calls to configure
	// how vertex data is interpreted. This is stored here so that
	// SetVertexBuffer can configure attributes using the pipeline's layout.
	vertexBuffers []gputypes.VertexBufferLayout

	// firstInstanceLocation is the naga_vs_first_instance uniform, through
	// which the first instance reaches instance_index without fully featured
	// instancing; -1 when the program has none.
	firstInstanceLocation int32

	// samplerBindMap maps texture unit indices to sampler unit indices.
	// Built from naga GLSL TranslationInfo.TextureMappings at pipeline creation.
	// When binding textures, the associated sampler must be bound to the SAME
	// texture unit (not the sampler's own WGSL binding). This is because naga
	// GLSL generates combined sampler2D on the texture's binding.
	// Matches Rust wgpu-hal GLES SamplerBindMap pattern.
	samplerBindMap samplerBindMap
}

const maxTextureSlots = 32

// Destroy releases the render pipeline.
func (p *RenderPipeline) Destroy() {
	if p.programID != 0 && p.glCtx != nil {
		p.glCtx.DeleteProgram(p.programID)
		p.programID = 0
	}
}

// ComputePipeline implements hal.ComputePipeline for OpenGL.
type ComputePipeline struct {
	programID uint32
	layout    *PipelineLayout
	glCtx     *gl.Context

	// samplerBindMap maps texture units to sampler slots, as on
	// RenderPipeline: a compute shader samples through combined samplers too.
	samplerBindMap samplerBindMap
}

// Destroy releases the compute pipeline.
func (p *ComputePipeline) Destroy() {
	if p.programID != 0 && p.glCtx != nil {
		p.glCtx.DeleteProgram(p.programID)
		p.programID = 0
	}
}

// glFence holds a GL sync object paired with its submission value.
// Matches Rust wgpu-hal/src/gles/fence.rs GLFence struct.
type glFence struct {
	sync  uintptr // GL sync object handle from glFenceSync
	value uint64  // submission index this fence was signaled with
}

// Fence implements hal.Fence using GL sync objects (glFenceSync).
// Tracks pending GL sync objects and polls their completion status.
// Matches Rust wgpu-hal/src/gles/fence.rs Fence struct.
type Fence struct {
	lastCompleted atomic.Uint64 // highest known completed value
	pending       []glFence     // GL sync objects awaiting completion
	glCtx         *gl.Context
}

// NewFence creates a new fence.
func NewFence(glCtx *gl.Context) *Fence {
	return &Fence{
		glCtx: glCtx,
	}
}

// Signal inserts a GL fence sync object into the command stream at the given value.
// Must be called on the GL thread BEFORE glFlush — the flush sends both the
// preceding commands and this fence to the GPU together.
// Returns an error if glFenceSync fails (typically OOM).
// Matches Rust wgpu-hal/src/gles/fence.rs Fence::signal.
func (f *Fence) Signal(value uint64) error {
	if f.glCtx == nil || !f.glCtx.SupportsFenceSync() {
		f.lastCompleted.Store(value)
		return nil
	}
	sync := f.glCtx.FenceSync(gl.SYNC_GPU_COMMANDS_COMPLETE, 0)
	if sync == 0 {
		return fmt.Errorf("gles: glFenceSync failed (OOM)")
	}
	f.pending = append(f.pending, glFence{sync: sync, value: value})
	return nil
}

// GetLatest polls pending sync objects and returns the highest completed value.
// Matches Rust wgpu-hal/src/gles/fence.rs Fence::get_latest.
func (f *Fence) GetLatest() uint64 {
	maxValue := f.lastCompleted.Load()

	if f.glCtx == nil || !f.glCtx.SupportsFenceSync() {
		return maxValue
	}

	for _, gf := range f.pending {
		if gf.value <= maxValue {
			continue // already known complete
		}
		status := f.glCtx.GetSyncStatus(gf.sync)
		if status == gl.SIGNALED {
			maxValue = gf.value
		} else {
			// Anything after the first unsignalled is guaranteed unsignalled.
			break
		}
	}

	// Cache the latest value to avoid redundant queries.
	for {
		old := f.lastCompleted.Load()
		if maxValue <= old {
			break
		}
		if f.lastCompleted.CompareAndSwap(old, maxValue) {
			break
		}
	}

	return maxValue
}

// Maintain cleans up completed sync objects.
// Matches Rust wgpu-hal/src/gles/fence.rs Fence::maintain.
func (f *Fence) Maintain() {
	if f.glCtx == nil || !f.glCtx.SupportsFenceSync() {
		return
	}

	latest := f.GetLatest()

	// Delete completed sync objects.
	n := 0
	for _, gf := range f.pending {
		if gf.value <= latest {
			f.glCtx.DeleteSync(gf.sync)
		} else {
			f.pending[n] = gf
			n++
		}
	}
	f.pending = f.pending[:n]
}

// Wait waits for the fence to reach the specified value with timeout.
// Matches Rust wgpu-hal/src/gles/fence.rs Fence::wait.
func (f *Fence) Wait(waitValue uint64, timeout time.Duration) bool {
	if f.lastCompleted.Load() >= waitValue {
		return true
	}

	if f.glCtx == nil || !f.glCtx.SupportsFenceSync() {
		return f.lastCompleted.Load() >= waitValue
	}

	// Find a matching pending fence.
	var target *glFence
	for i := range f.pending {
		if f.pending[i].value >= waitValue {
			target = &f.pending[i]
			break
		}
	}
	if target == nil {
		return false // value not yet signaled
	}

	// Convert timeout to nanoseconds for glClientWaitSync.
	timeoutNS := uint64(timeout.Nanoseconds())
	if timeoutNS > uint64(^uint32(0)) {
		timeoutNS = uint64(^uint32(0)) // cap to max u32 for safety
	}

	status := f.glCtx.ClientWaitSync(target.sync, gl.SYNC_FLUSH_COMMANDS_BIT, timeoutNS)

	signaled := status == gl.ALREADY_SIGNALED || status == gl.CONDITION_SATISFIED
	if signaled {
		// Update last completed.
		for {
			old := f.lastCompleted.Load()
			if waitValue <= old {
				break
			}
			if f.lastCompleted.CompareAndSwap(old, waitValue) {
				break
			}
		}
	}
	return signaled
}

// GetValue returns the current fence value (non-blocking poll).
func (f *Fence) GetValue() uint64 {
	return f.GetLatest()
}

// Reset resets the fence to the unsignaled state.
func (f *Fence) Reset() {
	f.lastCompleted.Store(0)
	// Clean up any pending sync objects.
	if f.glCtx != nil {
		for _, gf := range f.pending {
			f.glCtx.DeleteSync(gf.sync)
		}
	}
	f.pending = f.pending[:0]
}

// Destroy releases all fence resources.
// Matches Rust wgpu-hal/src/gles/fence.rs Fence::destroy.
func (f *Fence) Destroy() {
	if f.glCtx != nil {
		for _, gf := range f.pending {
			f.glCtx.DeleteSync(gf.sync)
		}
	}
	f.pending = nil
}

// QuerySet implements hal.QuerySet for OpenGL.
// Stores GL query object IDs and the target type (GL_TIMESTAMP or GL_ANY_SAMPLES_PASSED).
// Matches Rust wgpu-hal/src/gles/mod.rs QuerySet struct.
type QuerySet struct {
	queries []uint32 // GL query object IDs
	target  uint32   // GL_TIMESTAMP or GL_ANY_SAMPLES_PASSED_CONSERVATIVE
	glCtx   *gl.Context
}

// Target returns the GL query target type (GL_TIMESTAMP or GL_ANY_SAMPLES_PASSED).
func (q *QuerySet) Target() uint32 { return q.target }

// Destroy releases all GL query objects.
func (q *QuerySet) Destroy() {
	if q.glCtx != nil && len(q.queries) > 0 {
		q.glCtx.DeleteQueries(int32(len(q.queries)), &q.queries[0])
	}
	q.queries = nil
}

// NativeHandle returns 0 (no single native handle for a query set).
func (q *QuerySet) NativeHandle() uintptr { return 0 }
