// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build darwin

package metal

import (
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/types"
)

// Buffer implements hal.Buffer for Metal.
type Buffer struct {
	raw     ID // id<MTLBuffer>
	size    uint64
	usage   types.BufferUsage
	options MTLResourceOptions
	device  *Device
}

// Destroy releases the buffer.
func (b *Buffer) Destroy() {
	if b.device != nil {
		b.device.DestroyBuffer(b)
	}
}

// Contents returns the buffer contents pointer (for mapped buffers).
func (b *Buffer) Contents() uintptr {
	if b.raw == 0 {
		return 0
	}
	return uintptr(MsgSend(b.raw, Sel("contents")))
}

// Texture implements hal.Texture for Metal.
type Texture struct {
	raw        ID // id<MTLTexture>
	format     types.TextureFormat
	width      uint32
	height     uint32
	depth      uint32
	mipLevels  uint32
	samples    uint32
	dimension  types.TextureDimension
	usage      types.TextureUsage
	device     *Device
	isExternal bool
}

// Destroy releases the texture.
func (t *Texture) Destroy() {
	if t.device != nil {
		t.device.DestroyTexture(t)
	}
}

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

// ShaderModule implements hal.ShaderModule for Metal.
type ShaderModule struct {
	source  hal.ShaderSource
	library ID // id<MTLLibrary>
	device  *Device
}

// Destroy releases the shader module.
func (m *ShaderModule) Destroy() {
	if m.device != nil {
		m.device.DestroyShaderModule(m)
	}
}

// BindGroupLayout implements hal.BindGroupLayout for Metal.
type BindGroupLayout struct {
	entries []types.BindGroupLayoutEntry
	device  *Device
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
	entries []types.BindGroupEntry
	device  *Device
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
}

// Destroy releases the pipeline layout.
func (l *PipelineLayout) Destroy() {
	if l.device != nil {
		l.device.DestroyPipelineLayout(l)
	}
}

// RenderPipeline implements hal.RenderPipeline for Metal.
type RenderPipeline struct {
	raw    ID // id<MTLRenderPipelineState>
	device *Device
}

// Destroy releases the render pipeline.
func (p *RenderPipeline) Destroy() {
	if p.device != nil {
		p.device.DestroyRenderPipeline(p)
	}
}

// ComputePipeline implements hal.ComputePipeline for Metal.
type ComputePipeline struct {
	raw    ID // id<MTLComputePipelineState>
	device *Device
}

// Destroy releases the compute pipeline.
func (p *ComputePipeline) Destroy() {
	if p.device != nil {
		p.device.DestroyComputePipeline(p)
	}
}

// Fence implements hal.Fence for Metal.
type Fence struct {
	event  ID // id<MTLEvent>
	value  uint64
	device *Device
}

// Destroy releases the fence.
func (f *Fence) Destroy() {
	if f.device != nil {
		f.device.DestroyFence(f)
	}
}
