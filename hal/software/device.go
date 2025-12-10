//go:build software

package software

import (
	"fmt"
	"time"

	"github.com/gogpu/wgpu/hal"
)

// Device implements hal.Device for the software backend.
type Device struct{}

// CreateBuffer creates a software buffer with real data storage.
func (d *Device) CreateBuffer(desc *hal.BufferDescriptor) (hal.Buffer, error) {
	// Always allocate real storage for software backend
	data := make([]byte, desc.Size)
	return &Buffer{
		data:  data,
		size:  desc.Size,
		usage: desc.Usage,
	}, nil
}

// DestroyBuffer is a no-op (Go GC handles cleanup).
func (d *Device) DestroyBuffer(_ hal.Buffer) {}

// CreateTexture creates a software texture with real pixel storage.
func (d *Device) CreateTexture(desc *hal.TextureDescriptor) (hal.Texture, error) {
	// Calculate total size needed for texture data
	// Simple calculation: width * height * depth * bytesPerPixel
	// Assuming 4 bytes per pixel (RGBA8) for now
	bytesPerPixel := uint64(4)
	totalSize := uint64(desc.Size.Width) * uint64(desc.Size.Height) * uint64(desc.Size.DepthOrArrayLayers) * bytesPerPixel

	return &Texture{
		data:          make([]byte, totalSize),
		width:         desc.Size.Width,
		height:        desc.Size.Height,
		depth:         desc.Size.DepthOrArrayLayers,
		format:        desc.Format,
		usage:         desc.Usage,
		mipLevelCount: desc.MipLevelCount,
		sampleCount:   desc.SampleCount,
	}, nil
}

// DestroyTexture is a no-op (Go GC handles cleanup).
func (d *Device) DestroyTexture(_ hal.Texture) {}

// CreateTextureView creates a software texture view.
func (d *Device) CreateTextureView(texture hal.Texture, _ *hal.TextureViewDescriptor) (hal.TextureView, error) {
	// Views in software backend just reference the original texture
	if tex, ok := texture.(*Texture); ok {
		return &TextureView{texture: tex}, nil
	}
	return &Resource{}, nil
}

// DestroyTextureView is a no-op.
func (d *Device) DestroyTextureView(_ hal.TextureView) {}

// CreateSampler creates a software sampler.
func (d *Device) CreateSampler(_ *hal.SamplerDescriptor) (hal.Sampler, error) {
	return &Resource{}, nil
}

// DestroySampler is a no-op.
func (d *Device) DestroySampler(_ hal.Sampler) {}

// CreateBindGroupLayout creates a software bind group layout.
func (d *Device) CreateBindGroupLayout(_ *hal.BindGroupLayoutDescriptor) (hal.BindGroupLayout, error) {
	return &Resource{}, nil
}

// DestroyBindGroupLayout is a no-op.
func (d *Device) DestroyBindGroupLayout(_ hal.BindGroupLayout) {}

// CreateBindGroup creates a software bind group.
func (d *Device) CreateBindGroup(_ *hal.BindGroupDescriptor) (hal.BindGroup, error) {
	return &Resource{}, nil
}

// DestroyBindGroup is a no-op.
func (d *Device) DestroyBindGroup(_ hal.BindGroup) {}

// CreatePipelineLayout creates a software pipeline layout.
func (d *Device) CreatePipelineLayout(_ *hal.PipelineLayoutDescriptor) (hal.PipelineLayout, error) {
	return &Resource{}, nil
}

// DestroyPipelineLayout is a no-op.
func (d *Device) DestroyPipelineLayout(_ hal.PipelineLayout) {}

// CreateShaderModule creates a software shader module.
func (d *Device) CreateShaderModule(_ *hal.ShaderModuleDescriptor) (hal.ShaderModule, error) {
	return &Resource{}, nil
}

// DestroyShaderModule is a no-op.
func (d *Device) DestroyShaderModule(_ hal.ShaderModule) {}

// CreateRenderPipeline creates a software render pipeline.
func (d *Device) CreateRenderPipeline(_ *hal.RenderPipelineDescriptor) (hal.RenderPipeline, error) {
	return &Resource{}, nil
}

// DestroyRenderPipeline is a no-op.
func (d *Device) DestroyRenderPipeline(_ hal.RenderPipeline) {}

// CreateComputePipeline returns an error as compute is not supported.
func (d *Device) CreateComputePipeline(_ *hal.ComputePipelineDescriptor) (hal.ComputePipeline, error) {
	return nil, fmt.Errorf("compute pipelines not supported in software backend")
}

// DestroyComputePipeline is a no-op.
func (d *Device) DestroyComputePipeline(_ hal.ComputePipeline) {}

// CreateCommandEncoder creates a software command encoder.
func (d *Device) CreateCommandEncoder(_ *hal.CommandEncoderDescriptor) (hal.CommandEncoder, error) {
	return &CommandEncoder{}, nil
}

// CreateFence creates a software fence with atomic counter.
func (d *Device) CreateFence() (hal.Fence, error) {
	return &Fence{}, nil
}

// DestroyFence is a no-op.
func (d *Device) DestroyFence(_ hal.Fence) {}

// Wait simulates waiting for a fence value.
// Always returns true immediately (fence reached).
func (d *Device) Wait(fence hal.Fence, value uint64, _ time.Duration) (bool, error) {
	f, ok := fence.(*Fence)
	if !ok {
		return true, nil
	}
	// Check if fence has reached the value
	return f.value.Load() >= value, nil
}

// Destroy is a no-op for the software device.
func (d *Device) Destroy() {}
