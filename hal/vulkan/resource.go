// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows

package vulkan

import (
	"github.com/gogpu/wgpu/hal/vulkan/memory"
	"github.com/gogpu/wgpu/hal/vulkan/vk"
	"github.com/gogpu/wgpu/types"
)

// Buffer implements hal.Buffer for Vulkan.
type Buffer struct {
	handle vk.Buffer
	memory *memory.MemoryBlock
	size   uint64
	usage  types.BufferUsage
	device *Device
}

// Destroy releases the buffer.
func (b *Buffer) Destroy() {
	if b.device != nil {
		b.device.DestroyBuffer(b)
	}
}

// Handle returns the VkBuffer handle.
func (b *Buffer) Handle() vk.Buffer {
	return b.handle
}

// Size returns the buffer size in bytes.
func (b *Buffer) Size() uint64 {
	return b.size
}

// Texture implements hal.Texture for Vulkan.
type Texture struct {
	handle     vk.Image
	memory     *memory.MemoryBlock
	size       Extent3D
	format     types.TextureFormat
	usage      types.TextureUsage
	mipLevels  uint32
	samples    uint32
	dimension  types.TextureDimension
	device     *Device
	isExternal bool // True if memory is not owned by us (swapchain images)
}

// Extent3D represents 3D dimensions.
type Extent3D struct {
	Width  uint32
	Height uint32
	Depth  uint32
}

// Destroy releases the texture.
func (t *Texture) Destroy() {
	if t.device != nil {
		t.device.DestroyTexture(t)
	}
}

// Handle returns the VkImage handle.
func (t *Texture) Handle() vk.Image {
	return t.handle
}

// TextureView implements hal.TextureView for Vulkan.
type TextureView struct {
	handle  vk.ImageView
	texture *Texture
	device  *Device
}

// Destroy releases the texture view.
func (v *TextureView) Destroy() {
	if v.device != nil {
		v.device.DestroyTextureView(v)
	}
}

// Handle returns the VkImageView handle.
func (v *TextureView) Handle() vk.ImageView {
	return v.handle
}

// Sampler implements hal.Sampler for Vulkan.
type Sampler struct {
	handle vk.Sampler
	device *Device
}

// Destroy releases the sampler.
func (s *Sampler) Destroy() {
	if s.device != nil {
		s.device.DestroySampler(s)
	}
}

// Handle returns the VkSampler handle.
func (s *Sampler) Handle() vk.Sampler {
	return s.handle
}

// ShaderModule implements hal.ShaderModule for Vulkan.
type ShaderModule struct {
	handle vk.ShaderModule
	device *Device
}

// Destroy releases the shader module.
func (m *ShaderModule) Destroy() {
	if m.device != nil {
		m.device.DestroyShaderModule(m)
	}
}

// Handle returns the VkShaderModule handle.
func (m *ShaderModule) Handle() vk.ShaderModule {
	return m.handle
}

// BindGroupLayout implements hal.BindGroupLayout for Vulkan.
type BindGroupLayout struct {
	handle vk.DescriptorSetLayout
	counts DescriptorCounts // Descriptor counts for pool allocation
	device *Device
}

// Destroy releases the bind group layout.
func (l *BindGroupLayout) Destroy() {
	if l.device != nil {
		l.device.DestroyBindGroupLayout(l)
	}
}

// Handle returns the VkDescriptorSetLayout handle.
func (l *BindGroupLayout) Handle() vk.DescriptorSetLayout {
	return l.handle
}

// Counts returns the descriptor counts for this layout.
func (l *BindGroupLayout) Counts() DescriptorCounts {
	return l.counts
}

// BindGroup implements hal.BindGroup for Vulkan.
type BindGroup struct {
	handle vk.DescriptorSet
	pool   *DescriptorPool // Reference to the pool for freeing
	device *Device
}

// Destroy releases the bind group.
func (g *BindGroup) Destroy() {
	if g.device != nil {
		g.device.DestroyBindGroup(g)
	}
}

// Handle returns the VkDescriptorSet handle.
func (g *BindGroup) Handle() vk.DescriptorSet {
	return g.handle
}

// PipelineLayout implements hal.PipelineLayout for Vulkan.
type PipelineLayout struct {
	handle vk.PipelineLayout
	device *Device
}

// Destroy releases the pipeline layout.
func (l *PipelineLayout) Destroy() {
	if l.device != nil {
		l.device.DestroyPipelineLayout(l)
	}
}

// Handle returns the VkPipelineLayout handle.
func (l *PipelineLayout) Handle() vk.PipelineLayout {
	return l.handle
}

// RenderPipeline implements hal.RenderPipeline for Vulkan.
type RenderPipeline struct {
	handle vk.Pipeline
	layout vk.PipelineLayout
	device *Device
}

// Destroy releases the render pipeline.
func (p *RenderPipeline) Destroy() {
	if p.device != nil {
		p.device.DestroyRenderPipeline(p)
	}
}

// ComputePipeline implements hal.ComputePipeline for Vulkan.
type ComputePipeline struct {
	handle vk.Pipeline
	layout vk.PipelineLayout
	device *Device
}

// Destroy releases the compute pipeline.
func (p *ComputePipeline) Destroy() {
	if p.device != nil {
		p.device.DestroyComputePipeline(p)
	}
}

// Fence implements hal.Fence for Vulkan.
type Fence struct {
	handle vk.Fence
	value  uint64 //nolint:unused // Will be used for timeline semaphores
	device *Device
}

// Destroy releases the fence.
func (f *Fence) Destroy() {
	if f.device != nil {
		f.device.DestroyFence(f)
	}
}

// Handle returns the VkFence handle.
func (f *Fence) Handle() vk.Fence {
	return f.handle
}
