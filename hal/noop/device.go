package noop

import (
	"fmt"
	"time"

	"github.com/gogpu/wgpu/hal"
)

// Device implements hal.Device for the noop backend.
type Device struct{}

// CreateBuffer creates a noop buffer.
// Optionally stores data if MappedAtCreation is true.
func (d *Device) CreateBuffer(desc *hal.BufferDescriptor) (hal.Buffer, error) {
	if desc.MappedAtCreation {
		return &Buffer{data: make([]byte, desc.Size)}, nil
	}
	return &Resource{}, nil
}

// DestroyBuffer is a no-op.
func (d *Device) DestroyBuffer(_ hal.Buffer) {}

// CreateTexture creates a noop texture.
func (d *Device) CreateTexture(_ *hal.TextureDescriptor) (hal.Texture, error) {
	return &Texture{}, nil
}

// DestroyTexture is a no-op.
func (d *Device) DestroyTexture(_ hal.Texture) {}

// CreateTextureView creates a noop texture view.
func (d *Device) CreateTextureView(_ hal.Texture, _ *hal.TextureViewDescriptor) (hal.TextureView, error) {
	return &Resource{}, nil
}

// DestroyTextureView is a no-op.
func (d *Device) DestroyTextureView(_ hal.TextureView) {}

// CreateSampler creates a noop sampler.
func (d *Device) CreateSampler(_ *hal.SamplerDescriptor) (hal.Sampler, error) {
	return &Resource{}, nil
}

// DestroySampler is a no-op.
func (d *Device) DestroySampler(_ hal.Sampler) {}

// CreateBindGroupLayout creates a noop bind group layout.
func (d *Device) CreateBindGroupLayout(_ *hal.BindGroupLayoutDescriptor) (hal.BindGroupLayout, error) {
	return &Resource{}, nil
}

// DestroyBindGroupLayout is a no-op.
func (d *Device) DestroyBindGroupLayout(_ hal.BindGroupLayout) {}

// CreateBindGroup creates a noop bind group.
func (d *Device) CreateBindGroup(_ *hal.BindGroupDescriptor) (hal.BindGroup, error) {
	return &Resource{}, nil
}

// DestroyBindGroup is a no-op.
func (d *Device) DestroyBindGroup(_ hal.BindGroup) {}

// CreatePipelineLayout creates a noop pipeline layout.
func (d *Device) CreatePipelineLayout(_ *hal.PipelineLayoutDescriptor) (hal.PipelineLayout, error) {
	return &Resource{}, nil
}

// DestroyPipelineLayout is a no-op.
func (d *Device) DestroyPipelineLayout(_ hal.PipelineLayout) {}

// CreateShaderModule creates a noop shader module.
func (d *Device) CreateShaderModule(_ *hal.ShaderModuleDescriptor) (hal.ShaderModule, error) {
	return &Resource{}, nil
}

// DestroyShaderModule is a no-op.
func (d *Device) DestroyShaderModule(_ hal.ShaderModule) {}

// CreateRenderPipeline creates a noop render pipeline.
func (d *Device) CreateRenderPipeline(_ *hal.RenderPipelineDescriptor) (hal.RenderPipeline, error) {
	return &Resource{}, nil
}

// DestroyRenderPipeline is a no-op.
func (d *Device) DestroyRenderPipeline(_ hal.RenderPipeline) {}

// CreateComputePipeline creates a noop compute pipeline.
func (d *Device) CreateComputePipeline(_ *hal.ComputePipelineDescriptor) (hal.ComputePipeline, error) {
	return &Resource{}, nil
}

// DestroyComputePipeline is a no-op.
func (d *Device) DestroyComputePipeline(_ hal.ComputePipeline) {}

// CreateQuerySet returns ErrTimestampsNotSupported (noop backend has no GPU).
func (d *Device) CreateQuerySet(_ *hal.QuerySetDescriptor) (hal.QuerySet, error) {
	return nil, hal.ErrTimestampsNotSupported
}

// DestroyQuerySet is a no-op.
func (d *Device) DestroyQuerySet(_ hal.QuerySet) {}

// CreateCommandEncoder creates a noop command encoder.
func (d *Device) CreateCommandEncoder(_ *hal.CommandEncoderDescriptor) (hal.CommandEncoder, error) {
	return &CommandEncoder{}, nil
}

// CreateFence creates a noop fence with atomic counter.
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

// ResetFence resets a fence to the unsignaled state.
func (d *Device) ResetFence(fence hal.Fence) error {
	f, ok := fence.(*Fence)
	if !ok {
		return nil
	}
	f.value.Store(0)
	return nil
}

// GetFenceStatus returns true if the fence is signaled (non-blocking).
func (d *Device) GetFenceStatus(fence hal.Fence) (bool, error) {
	f, ok := fence.(*Fence)
	if !ok {
		return false, nil
	}
	return f.value.Load() > 0, nil
}

// FreeCommandBuffer is a no-op for the noop device.
func (d *Device) FreeCommandBuffer(cmdBuffer hal.CommandBuffer) {}

// CreateRenderBundleEncoder is a no-op for the noop device.
func (d *Device) CreateRenderBundleEncoder(desc *hal.RenderBundleEncoderDescriptor) (hal.RenderBundleEncoder, error) {
	return nil, fmt.Errorf("noop: render bundles not supported")
}

// DestroyRenderBundle is a no-op for the noop device.
func (d *Device) DestroyRenderBundle(bundle hal.RenderBundle) {}

// WaitIdle is a no-op for the noop device.
func (d *Device) WaitIdle() error { return nil }

// Destroy is a no-op for the noop device.
func (d *Device) Destroy() {}
