// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vulkan

import (
	"testing"

	"github.com/gogpu/wgpu/hal/vulkan/vk"
	"github.com/gogpu/gputypes"
)

// TestBufferHandle tests Buffer Handle method.
func TestBufferHandle(t *testing.T) {
	buffer := &Buffer{
		handle: vk.Buffer(12345),
	}

	got := buffer.Handle()
	if got != vk.Buffer(12345) {
		t.Errorf("Handle() = %v, want 12345", got)
	}
}

// TestBufferSize tests Buffer Size method.
func TestBufferSize(t *testing.T) {
	buffer := &Buffer{
		size: 4096,
	}

	got := buffer.Size()
	if got != 4096 {
		t.Errorf("Size() = %d, want 4096", got)
	}
}

// TestBufferFields tests Buffer struct fields.
func TestBufferFields(t *testing.T) {
	buffer := &Buffer{
		handle: vk.Buffer(100),
		size:   2048,
		usage:  gputypes.BufferUsageVertex | gputypes.BufferUsageIndex,
	}

	if buffer.handle != vk.Buffer(100) {
		t.Errorf("handle = %v, want 100", buffer.handle)
	}
	if buffer.size != 2048 {
		t.Errorf("size = %d, want 2048", buffer.size)
	}
	if buffer.usage != (gputypes.BufferUsageVertex | gputypes.BufferUsageIndex) {
		t.Errorf("usage = %v, want Vertex|Index", buffer.usage)
	}
}

// TestTextureHandle tests Texture Handle method.
func TestTextureHandle(t *testing.T) {
	texture := &Texture{
		handle: vk.Image(67890),
	}

	got := texture.Handle()
	if got != vk.Image(67890) {
		t.Errorf("Handle() = %v, want 67890", got)
	}
}

// TestTextureFields tests Texture struct fields.
func TestTextureFields(t *testing.T) {
	texture := &Texture{
		handle: vk.Image(200),
		size: Extent3D{
			Width:  1024,
			Height: 768,
			Depth:  1,
		},
		format:     gputypes.TextureFormatRGBA8Unorm,
		usage:      gputypes.TextureUsageTextureBinding,
		mipLevels:  1,
		samples:    1,
		dimension:  gputypes.TextureDimension2D,
		isExternal: false,
	}

	if texture.handle != vk.Image(200) {
		t.Errorf("handle = %v, want 200", texture.handle)
	}
	if texture.size.Width != 1024 {
		t.Errorf("size.Width = %d, want 1024", texture.size.Width)
	}
	if texture.size.Height != 768 {
		t.Errorf("size.Height = %d, want 768", texture.size.Height)
	}
	if texture.size.Depth != 1 {
		t.Errorf("size.Depth = %d, want 1", texture.size.Depth)
	}
	if texture.format != gputypes.TextureFormatRGBA8Unorm {
		t.Errorf("format = %v, want RGBA8Unorm", texture.format)
	}
	if texture.mipLevels != 1 {
		t.Errorf("mipLevels = %d, want 1", texture.mipLevels)
	}
	if texture.samples != 1 {
		t.Errorf("samples = %d, want 1", texture.samples)
	}
	if texture.dimension != gputypes.TextureDimension2D {
		t.Errorf("dimension = %v, want 2D", texture.dimension)
	}
	if texture.isExternal != false {
		t.Errorf("isExternal = %v, want false", texture.isExternal)
	}
}

// TestExtent3D tests Extent3D struct.
func TestExtent3D(t *testing.T) {
	tests := []struct {
		name   string
		extent Extent3D
	}{
		{
			name: "1D texture",
			extent: Extent3D{
				Width:  256,
				Height: 1,
				Depth:  1,
			},
		},
		{
			name: "2D texture",
			extent: Extent3D{
				Width:  1920,
				Height: 1080,
				Depth:  1,
			},
		},
		{
			name: "3D texture",
			extent: Extent3D{
				Width:  128,
				Height: 128,
				Depth:  64,
			},
		},
		{
			name: "Zero extent",
			extent: Extent3D{
				Width:  0,
				Height: 0,
				Depth:  0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			extent := tt.extent
			if extent.Width != tt.extent.Width {
				t.Errorf("Width = %d, want %d", extent.Width, tt.extent.Width)
			}
			if extent.Height != tt.extent.Height {
				t.Errorf("Height = %d, want %d", extent.Height, tt.extent.Height)
			}
			if extent.Depth != tt.extent.Depth {
				t.Errorf("Depth = %d, want %d", extent.Depth, tt.extent.Depth)
			}
		})
	}
}

// TestTextureViewHandle tests TextureView Handle method.
func TestTextureViewHandle(t *testing.T) {
	view := &TextureView{
		handle: vk.ImageView(11111),
	}

	got := view.Handle()
	if got != vk.ImageView(11111) {
		t.Errorf("Handle() = %v, want 11111", got)
	}
}

// TestTextureViewFields tests TextureView struct fields.
func TestTextureViewFields(t *testing.T) {
	texture := &Texture{
		handle: vk.Image(300),
	}

	view := &TextureView{
		handle:  vk.ImageView(400),
		texture: texture,
	}

	if view.handle != vk.ImageView(400) {
		t.Errorf("handle = %v, want 400", view.handle)
	}
	if view.texture == nil {
		t.Error("texture should not be nil")
	}
	if view.texture.handle != vk.Image(300) {
		t.Errorf("texture.handle = %v, want 300", view.texture.handle)
	}
}

// TestSamplerFields tests Sampler struct fields.
func TestSamplerFields(t *testing.T) {
	sampler := &Sampler{
		handle: vk.Sampler(500),
	}

	if sampler.handle != vk.Sampler(500) {
		t.Errorf("handle = %v, want 500", sampler.handle)
	}
}

// TestBufferNilDevice tests Buffer behavior with nil device.
func TestBufferNilDevice(t *testing.T) {
	buffer := &Buffer{
		handle: vk.Buffer(100),
		device: nil,
	}

	// Destroy should not panic with nil device
	buffer.Destroy()

	// Should still have valid handle
	if buffer.Handle() != vk.Buffer(100) {
		t.Error("Handle should still be valid after Destroy with nil device")
	}
}

// TestTextureNilDevice tests Texture behavior with nil device.
func TestTextureNilDevice(t *testing.T) {
	texture := &Texture{
		handle: vk.Image(200),
		device: nil,
	}

	// Destroy should not panic with nil device
	texture.Destroy()

	// Should still have valid handle
	if texture.Handle() != vk.Image(200) {
		t.Error("Handle should still be valid after Destroy with nil device")
	}
}

// TestTextureViewNilDevice tests TextureView behavior with nil device.
func TestTextureViewNilDevice(t *testing.T) {
	view := &TextureView{
		handle: vk.ImageView(300),
		device: nil,
	}

	// Destroy should not panic with nil device
	view.Destroy()

	// Should still have valid handle
	if view.Handle() != vk.ImageView(300) {
		t.Error("Handle should still be valid after Destroy with nil device")
	}
}

// TestSamplerNilDevice tests Sampler behavior with nil device.
func TestSamplerNilDevice(t *testing.T) {
	sampler := &Sampler{
		handle: vk.Sampler(400),
		device: nil,
	}

	// Destroy should not panic with nil device
	sampler.Destroy()
}

// TestTextureIsExternal tests external texture flag.
func TestTextureIsExternal(t *testing.T) {
	tests := []struct {
		name       string
		isExternal bool
	}{
		{"Internal texture", false},
		{"External texture (swapchain)", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			texture := &Texture{
				isExternal: tt.isExternal,
			}

			if texture.isExternal != tt.isExternal {
				t.Errorf("isExternal = %v, want %v", texture.isExternal, tt.isExternal)
			}
		})
	}
}

// TestBufferUsageFlags tests various buffer usage combinations.
func TestBufferUsageFlags(t *testing.T) {
	tests := []struct {
		name  string
		usage gputypes.BufferUsage
	}{
		{"Vertex buffer", gputypes.BufferUsageVertex},
		{"Index buffer", gputypes.BufferUsageIndex},
		{"Uniform buffer", gputypes.BufferUsageUniform},
		{"Storage buffer", gputypes.BufferUsageStorage},
		{"Vertex + Index", gputypes.BufferUsageVertex | gputypes.BufferUsageIndex},
		{"Transfer src + dst", gputypes.BufferUsageCopySrc | gputypes.BufferUsageCopyDst},
		{"All flags", gputypes.BufferUsageVertex | gputypes.BufferUsageIndex | gputypes.BufferUsageUniform | gputypes.BufferUsageStorage | gputypes.BufferUsageCopySrc | gputypes.BufferUsageCopyDst},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buffer := &Buffer{
				usage: tt.usage,
			}

			if buffer.usage != tt.usage {
				t.Errorf("usage = %v, want %v", buffer.usage, tt.usage)
			}
		})
	}
}

// TestTextureUsageFlags tests various texture usage combinations.
func TestTextureUsageFlags(t *testing.T) {
	tests := []struct {
		name  string
		usage gputypes.TextureUsage
	}{
		{"Texture binding", gputypes.TextureUsageTextureBinding},
		{"Storage binding", gputypes.TextureUsageStorageBinding},
		{"Render attachment", gputypes.TextureUsageRenderAttachment},
		{"Copy src", gputypes.TextureUsageCopySrc},
		{"Copy dst", gputypes.TextureUsageCopyDst},
		{"Texture + Render", gputypes.TextureUsageTextureBinding | gputypes.TextureUsageRenderAttachment},
		{"All flags", gputypes.TextureUsageTextureBinding | gputypes.TextureUsageStorageBinding | gputypes.TextureUsageRenderAttachment | gputypes.TextureUsageCopySrc | gputypes.TextureUsageCopyDst},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			texture := &Texture{
				usage: tt.usage,
			}

			if texture.usage != tt.usage {
				t.Errorf("usage = %v, want %v", texture.usage, tt.usage)
			}
		})
	}
}

// TestTextureMipLevels tests various mip level configurations.
func TestTextureMipLevels(t *testing.T) {
	tests := []struct {
		name      string
		mipLevels uint32
	}{
		{"No mipmaps", 1},
		{"2 levels", 2},
		{"4 levels", 4},
		{"8 levels", 8},
		{"11 levels (2048x2048)", 11},
		{"13 levels (4096x4096)", 13},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			texture := &Texture{
				mipLevels: tt.mipLevels,
			}

			if texture.mipLevels != tt.mipLevels {
				t.Errorf("mipLevels = %d, want %d", texture.mipLevels, tt.mipLevels)
			}
		})
	}
}

// TestTextureSamples tests various sample count configurations.
func TestTextureSamples(t *testing.T) {
	tests := []struct {
		name    string
		samples uint32
	}{
		{"No MSAA", 1},
		{"2x MSAA", 2},
		{"4x MSAA", 4},
		{"8x MSAA", 8},
		{"16x MSAA", 16},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			texture := &Texture{
				samples: tt.samples,
			}

			if texture.samples != tt.samples {
				t.Errorf("samples = %d, want %d", texture.samples, tt.samples)
			}
		})
	}
}

// TestTextureDimensions tests various texture dimensions.
func TestTextureDimensions(t *testing.T) {
	tests := []struct {
		name      string
		dimension gputypes.TextureDimension
	}{
		{"1D", gputypes.TextureDimension1D},
		{"2D", gputypes.TextureDimension2D},
		{"3D", gputypes.TextureDimension3D},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			texture := &Texture{
				dimension: tt.dimension,
			}

			if texture.dimension != tt.dimension {
				t.Errorf("dimension = %v, want %v", texture.dimension, tt.dimension)
			}
		})
	}
}

// TestTextureFormats tests various texture formats.
func TestTextureFormats(t *testing.T) {
	tests := []struct {
		name   string
		format gputypes.TextureFormat
	}{
		{"RGBA8Unorm", gputypes.TextureFormatRGBA8Unorm},
		{"RGBA8UnormSrgb", gputypes.TextureFormatRGBA8UnormSrgb},
		{"BGRA8Unorm", gputypes.TextureFormatBGRA8Unorm},
		{"R32Float", gputypes.TextureFormatR32Float},
		{"RGBA32Float", gputypes.TextureFormatRGBA32Float},
		{"Depth32Float", gputypes.TextureFormatDepth32Float},
		{"Depth24PlusStencil8", gputypes.TextureFormatDepth24PlusStencil8},
		{"BC1RGBAUnorm", gputypes.TextureFormatBC1RGBAUnorm},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			texture := &Texture{
				format: tt.format,
			}

			if texture.format != tt.format {
				t.Errorf("format = %v, want %v", texture.format, tt.format)
			}
		})
	}
}
