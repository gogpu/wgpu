// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows || linux

package gles

import (
	"sync/atomic"
	"time"

	"github.com/gogpu/gputypes"
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

// Sampler implements hal.Sampler for OpenGL.
type Sampler struct {
	glCtx *gl.Context
}

// Destroy releases the sampler.
func (s *Sampler) Destroy() {
	// Note: GL 3.3 core requires sampler objects
	// For now, we use texture-bound state (no GL sampler object)
}

// NativeHandle returns 0 (no GL sampler object).
func (s *Sampler) NativeHandle() uintptr { return 0 }

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

// PipelineLayout implements hal.PipelineLayout for OpenGL.
type PipelineLayout struct {
	bindGroupLayouts []*BindGroupLayout
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
}

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

// Fence implements hal.Fence using GL sync objects.
type Fence struct {
	value    atomic.Uint64
	syncObjs map[uint64]uintptr // GL sync objects by fence value
	glCtx    *gl.Context
}

// NewFence creates a new fence.
func NewFence(glCtx *gl.Context) *Fence {
	return &Fence{
		syncObjs: make(map[uint64]uintptr),
		glCtx:    glCtx,
	}
}

// Wait waits for the fence to reach the specified value.
func (f *Fence) Wait(value uint64, _ time.Duration) bool {
	return f.value.Load() >= value
}

// Signal sets the fence value.
func (f *Fence) Signal(value uint64) {
	f.value.Store(value)
}

// GetValue returns the current fence value.
func (f *Fence) GetValue() uint64 {
	return f.value.Load()
}

// Reset resets the fence to the unsignaled state.
func (f *Fence) Reset() {
	f.value.Store(0)
}

// Destroy releases fence resources.
func (f *Fence) Destroy() {
	// Clean up sync objects if any
	f.syncObjs = nil
}
