// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build darwin

package metal

import (
	"unsafe"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
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
}

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
type ShaderModule struct {
	source         hal.ShaderSource
	library        ID // id<MTLLibrary>
	device         *Device
	workgroupSizes map[string][3]uint32 // entry point name -> workgroup size
}

// Destroy releases the shader module.
func (m *ShaderModule) Destroy() {
	if m.device != nil {
		m.device.DestroyShaderModule(m)
	}
}

// BindGroupLayout implements hal.BindGroupLayout for Metal.
type BindGroupLayout struct {
	entries []gputypes.BindGroupLayoutEntry
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
	entries []gputypes.BindGroupEntry
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
	raw           ID // id<MTLComputePipelineState>
	device        *Device
	workgroupSize MTLSize // workgroup size from shader
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
