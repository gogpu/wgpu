//go:build js && wasm

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package device

// This file is the device domain's implementation on the browser build, moved
// out of the root package by the S11 Move (ADR-070). The root package keeps a
// declaration layer over it and the descriptor conversion; this type owns the
// GPUDevice calls. No public root type appears in a signature below.

import (
	"fmt"
	"syscall/js"
	"time"

	"github.com/gogpu/gputypes"

	"github.com/gogpu/wgpu/internal/browser"
	"github.com/gogpu/wgpu/internal/encoder"
	"github.com/gogpu/wgpu/internal/pipeconst"
	"github.com/gogpu/wgpu/internal/pipeline"
	"github.com/gogpu/wgpu/internal/resource"
)

// Device represents a logical GPU device on the browser build: it wraps a
// GPUDevice through internal/browser.
type Device struct {
	browser  *browser.Device
	features gputypes.Features
	limits   gputypes.Limits
	released bool
}

// NewBrowser builds a device over a browser GPUDevice wrapper. The root
// adapter is its only caller.
func NewBrowser(b *browser.Device, features gputypes.Features, limits gputypes.Limits) *Device {
	return &Device{browser: b, features: features, limits: limits}
}

// Browser returns the browser device, or nil.
func (d *Device) Browser() *browser.Device {
	if d == nil {
		return nil
	}
	return d.browser
}

// IsReleased reports whether the device has been released.
func (d *Device) IsReleased() bool {
	return d != nil && d.released
}

// Features returns the device's enabled features.
func (d *Device) Features() gputypes.Features {
	if d == nil {
		return 0
	}
	return d.features
}

// Limits returns the device's resource limits.
func (d *Device) Limits() gputypes.Limits {
	if d == nil {
		return gputypes.Limits{}
	}
	return d.limits
}

// CreateBuffer creates a GPU buffer. size and usage are kept on the resource
// handle; mappedAtCreation starts the buffer mapped over its whole range.
func (d *Device) CreateBuffer(label string, size uint64, usage gputypes.BufferUsage, mappedAtCreation bool) (*resource.Buffer, error) {
	if d.released {
		return nil, resource.ErrReleased
	}
	jsDesc := browser.BuildBufferDescriptor(label, size, uint64(usage), mappedAtCreation)
	bb := d.browser.CreateBufferFromDesc(jsDesc)

	state := resource.MapStateUnmapped
	if mappedAtCreation {
		// When mappedAtCreation is true the buffer starts in the Mapped state
		// with the entire range [0, size) mapped. Record this so that
		// MappedRange / Unmap work without calling MapAsync first.
		bb.SetMappedAtCreation()
		state = resource.MapStateMapped
	}

	return resource.NewBufferBrowser(bb, size, usage, state), nil
}

// CreateTexture creates a GPU texture from an already-converted descriptor.
func (d *Device) CreateTexture(jsDesc js.Value, format gputypes.TextureFormat) (*resource.Texture, error) {
	if d.released {
		return nil, resource.ErrReleased
	}
	return resource.NewTexture(d.browser.CreateTextureFromDesc(jsDesc), format), nil
}

// CreateTextureView creates a view into texture.
func (d *Device) CreateTextureView(texture *resource.Texture, jsDesc js.Value) (*resource.TextureView, error) {
	if d.released {
		return nil, resource.ErrReleased
	}
	if texture == nil || texture.Browser() == nil {
		return nil, resource.ErrReleased
	}
	bv := texture.Browser().CreateView(jsDesc)
	return resource.NewTextureView(bv, texture), nil
}

// CreateSampler creates a texture sampler from an already-converted descriptor.
func (d *Device) CreateSampler(jsDesc js.Value) (*resource.Sampler, error) {
	if d.released {
		return nil, resource.ErrReleased
	}
	return resource.NewSampler(d.browser.CreateSamplerFromDesc(jsDesc)), nil
}

// CreateShaderModule creates a shader module from an already-converted
// descriptor.
func (d *Device) CreateShaderModule(jsDesc js.Value) (*resource.ShaderModule, error) {
	if d.released {
		return nil, resource.ErrReleased
	}
	return resource.NewShaderModule(d.browser.CreateShaderModuleFromDesc(jsDesc)), nil
}

// CreateBindGroupLayout creates a bind group layout.
func (d *Device) CreateBindGroupLayout(jsDesc js.Value) (*resource.BindGroupLayout, error) {
	if d.released {
		return nil, resource.ErrReleased
	}
	return resource.NewBindGroupLayout(d.browser.CreateBindGroupLayoutFromDesc(jsDesc)), nil
}

// CreatePipelineLayout creates a pipeline layout.
func (d *Device) CreatePipelineLayout(jsDesc js.Value) (*resource.PipelineLayout, error) {
	if d.released {
		return nil, resource.ErrReleased
	}
	return resource.NewPipelineLayout(d.browser.CreatePipelineLayoutFromDesc(jsDesc)), nil
}

// CreateBindGroup creates a bind group.
func (d *Device) CreateBindGroup(jsDesc js.Value) (*resource.BindGroup, error) {
	if d.released {
		return nil, resource.ErrReleased
	}
	return resource.NewBindGroup(d.browser.CreateBindGroupFromDesc(jsDesc)), nil
}

// CreateRenderPipeline creates a render pipeline. The constants are checked
// here, after the released check, before they reach JS.
func (d *Device) CreateRenderPipeline(jsDesc js.Value, vertexConstants, fragmentConstants map[string]float64, hasFragment bool) (*pipeline.RenderPipeline, error) {
	if d.released {
		return nil, resource.ErrReleased
	}
	if err := checkBrowserRenderConstants(vertexConstants, fragmentConstants, hasFragment); err != nil {
		return nil, err
	}
	return pipeline.NewRenderPipeline(d.browser.CreateRenderPipelineFromDesc(jsDesc)), nil
}

// CreateComputePipeline creates a compute pipeline. The constants are checked
// here, after the released check, before they reach JS.
func (d *Device) CreateComputePipeline(jsDesc js.Value, constants map[string]float64) (*pipeline.ComputePipeline, error) {
	if d.released {
		return nil, resource.ErrReleased
	}
	if err := checkBrowserStageConstants("compute", constants); err != nil {
		return nil, err
	}
	return pipeline.NewComputePipeline(d.browser.CreateComputePipelineFromDesc(jsDesc)), nil
}

// CreateCommandEncoder creates a command encoder for recording GPU commands.
func (d *Device) CreateCommandEncoder(label string) (*encoder.CommandEncoder, error) {
	if d.released {
		return nil, resource.ErrReleased
	}
	jsDesc := browser.BuildCommandEncoderDescriptor(label)
	jsEncoder := d.browser.CreateCommandEncoder().Invoke(jsDesc)
	be := browser.NewCommandEncoder(jsEncoder)
	return encoder.NewCommandEncoder(be), nil
}

// CreateFence creates a no-op fence for API compatibility: browser devices
// auto-poll.
func (d *Device) CreateFence() (*resource.Fence, error) {
	if d.released {
		return nil, resource.ErrReleased
	}
	return &resource.Fence{}, nil
}

// ResetFence is a no-op on the browser build: fences auto-reset.
func (d *Device) ResetFence(_ *resource.Fence) error { return nil }

// GetFenceStatus always reports signaled: browser fences resolve via JS
// Promises.
func (d *Device) GetFenceStatus(_ *resource.Fence) (bool, error) { return true, nil }

// WaitForFence returns immediately: GPU sync is handled by the JS event loop.
func (d *Device) WaitForFence(_ *resource.Fence, _ uint64, _ time.Duration) (bool, error) {
	return true, nil
}

// WaitIdle is a no-op on the browser build: the GPU is polled automatically.
func (d *Device) WaitIdle() error {
	return nil
}

// FreeCommandBuffer is a no-op on the browser build: JS GC handles GPU
// resource cleanup.
func (d *Device) FreeCommandBuffer(_ *encoder.CommandBuffer) {}

// Release releases the device.
func (d *Device) Release() {
	if d == nil || d.released {
		return
	}
	d.released = true
	if d.browser != nil {
		d.browser.Destroy()
	}
}

// checkBrowserStageConstants checks a stage's constants as every build does
// (pipeconst.Validate) before they reach JS, where a NaN or Infinity value
// would throw a TypeError instead of returning an error.
func checkBrowserStageConstants(stage string, constants map[string]float64) error {
	if err := pipeconst.Validate(constants); err != nil {
		return fmt.Errorf("wgpu: %s stage: %w", stage, err)
	}
	return nil
}

// checkBrowserRenderConstants applies checkBrowserStageConstants to the vertex
// and, when the pipeline has one, the fragment stage.
func checkBrowserRenderConstants(vertexConstants, fragmentConstants map[string]float64, hasFragment bool) error {
	if err := checkBrowserStageConstants("vertex", vertexConstants); err != nil {
		return err
	}
	if hasFragment {
		return checkBrowserStageConstants("fragment", fragmentConstants)
	}
	return nil
}
