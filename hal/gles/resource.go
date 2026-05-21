// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build (windows || linux) && !(js && wasm)

package gles

import (
	"sync/atomic"
	"time"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/naga/glsl"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/gles/gl"
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
}

// Destroy is a no-op for texture views in OpenGL.
func (v *TextureView) Destroy() {}

// NativeHandle returns the underlying texture's GL object ID.
func (v *TextureView) NativeHandle() uintptr {
	if v.texture != nil {
		return uintptr(v.texture.id)
	}
	return 0
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

// BindGroup implements hal.BindGroup for OpenGL.
type BindGroup struct {
	layout  *BindGroupLayout
	entries []gputypes.BindGroupEntry
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
	bindingMap map[glsl.BindingMapKey]uint8
}

// Destroy is a no-op for pipeline layouts.
func (l *PipelineLayout) Destroy() {}

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

	// Blend state from the first color target (nil = no blending).
	blend *gputypes.BlendState

	// Color write mask from the first color target.
	colorWriteMask gputypes.ColorWriteMask

	// Vertex buffer layouts from the pipeline descriptor.
	// OpenGL requires explicit glVertexAttribPointer calls to configure
	// how vertex data is interpreted. This is stored here so that
	// SetVertexBuffer can configure attributes using the pipeline's layout.
	vertexBuffers []gputypes.VertexBufferLayout

	// samplerBindMap maps texture unit indices to sampler unit indices.
	// Built from naga GLSL TranslationInfo.TextureMappings at pipeline creation.
	// When binding textures, the associated sampler must be bound to the SAME
	// texture unit (not the sampler's own WGSL binding). This is because naga
	// GLSL generates combined sampler2D on the texture's binding.
	// Matches Rust wgpu-hal GLES SamplerBindMap pattern.
	samplerBindMap [maxTextureSlots]int8 // -1 = no sampler, otherwise = sampler glBinding
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
// Must be called on the GL thread after glFlush.
// Matches Rust wgpu-hal/src/gles/fence.rs Fence::signal.
func (f *Fence) Signal(value uint64) {
	if f.glCtx == nil || !f.glCtx.SupportsFenceSync() {
		// Fallback: no fence sync support, mark as immediately complete.
		f.lastCompleted.Store(value)
		return
	}
	sync := f.glCtx.FenceSync(gl.SYNC_GPU_COMMANDS_COMPLETE, 0)
	if sync == 0 {
		// Fence creation failed — mark as immediately complete to avoid deadlock.
		f.lastCompleted.Store(value)
		return
	}
	f.pending = append(f.pending, glFence{sync: sync, value: value})
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
